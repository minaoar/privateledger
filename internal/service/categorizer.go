package service

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

// SICCategoryLookup resolves a canonical SIC code to a category.
//
// Categorizer depends on this narrow interface rather than the concrete SIC
// categorizer so that priority stays here in one place, SIC logic stays out of
// this file, and the lookup can be substituted without a database.
type SICCategoryLookup interface {
	// LookupCategory returns the mapped category, or false when the code has
	// no mapping or maps to an intentionally empty category.
	LookupCategory(sicCode model.SICCode) (int, bool)

	// ReloadMappings refreshes the underlying mapping state.
	ReloadMappings() error
}

// sicDecider exposes the shared decision function to the mapping source.
type sicDecider interface {
	decideCategory(txn *model.Transaction) (int, bool)
}

// decideCategory applies the shared priority order and reports whether a
// category should be assigned.
func (c *Categorizer) decideCategory(txn *model.Transaction) (int, bool) {
	categoryID, source := c.decide(txn)
	return categoryID, source != sourceNone
}

// categorySource identifies which kind of rule assigned a category, so a
// caller can report the split without inferring it afterwards.
type categorySource int

const (
	sourceNone categorySource = iota
	sourcePattern
	sourceSIC
)

// Categorizer handles automatic categorization of transactions.
//
// Both rule sets sit behind one lock and are refreshed together. Splitting
// them would allow a pass to see patterns from after a change beside mappings
// from before it, which is exactly the inconsistency a single decision
// function is meant to prevent.
type Categorizer struct {
	patternRepo *repository.CategoryPatternRepository
	txnRepo     *repository.TransactionRepository

	mu       sync.RWMutex
	patterns []*model.CategoryPattern

	// sicLookup is nil only in the compatibility constructor, where SIC
	// categorization is simply absent rather than broken.
	sicLookup SICCategoryLookup
}

// NewCategorizer creates a Categorizer without SIC support. Retained so
// existing call sites keep working; SIC categorization is inactive.
func NewCategorizer(
	patternRepo *repository.CategoryPatternRepository,
	txnRepo *repository.TransactionRepository,
) *Categorizer {
	return &Categorizer{
		patternRepo: patternRepo,
		txnRepo:     txnRepo,
		patterns:    make([]*model.CategoryPattern, 0),
	}
}

// NewCategorizerWithSIC creates a Categorizer that consults SIC mappings when
// no text pattern matches.
func NewCategorizerWithSIC(
	patternRepo *repository.CategoryPatternRepository,
	txnRepo *repository.TransactionRepository,
	sicLookup SICCategoryLookup,
) *Categorizer {
	c := NewCategorizer(patternRepo, txnRepo)
	c.sicLookup = sicLookup
	// The mapping source applies the same priority as every other entry point,
	// so it is given the shared decision function rather than deciding for
	// itself.
	if host, ok := sicLookup.(interface{ attachDecider(sicDecider) }); ok {
		host.attachDecider(c)
	}
	// It also serves as the mapping service's collaborator, where it does
	// nothing but forward to the one re-examination entry point.
	if host, ok := sicLookup.(interface{ attachReexaminer(reexaminer) }); ok {
		host.attachReexaminer(c)
	}
	return c
}

// sicMappingStager is implemented by a mapping source that can prepare a
// replacement index without publishing it. When available it lets LoadRules
// publish patterns and mappings as one generation under one lock, which is the
// only way a reader cannot observe half a reload.
type sicMappingStager interface {
	prepareMappings() (map[model.SICCode]*model.SICMapping, error)
	commitMappings(map[model.SICCode]*model.SICMapping)
}

// LoadRules refreshes every rule source. It is the only exported reload entry
// point, so a caller cannot refresh one rule set and forget the other.
//
// Where the mapping source can stage its replacement, both sets are published
// together under this categorizer's write lock, so categorization observes
// either the complete old generation or the complete new one and never a
// mixture, and readers are never blocked.
//
// A source that can only reload itself may publish its new mappings before
// returning, so the fallback instead holds the write lock across both the
// reload and the pattern publication. Categorization blocks for that interval
// rather than observing half of it, and a failed reload publishes nothing.
func (c *Categorizer) LoadRules() error {
	patterns, err := c.patternRepo.GetAll()
	if err != nil {
		slog.Error("Error loading patterns", slog.String("error", err.Error()))
		return fmt.Errorf("failed to load patterns: %w", err)
	}

	if stager, ok := c.sicLookup.(sicMappingStager); ok {
		index, err := stager.prepareMappings()
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.patterns = patterns
		stager.commitMappings(index)
		c.mu.Unlock()
		return nil
	}

	// Compatibility path for a source that cannot stage its replacement. Such a
	// source may publish its new mappings as soon as the reload begins, which is
	// legitimate for this interface, so the categorizer cannot let a reader
	// observe the interval at all: the write lock is held across the reload and
	// the pattern publication together.
	//
	// Categorization therefore blocks for the duration of a fallback reload and
	// resumes on one complete generation. If the reload fails, patterns are
	// never published and the previous generation stands.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sicLookup != nil {
		if err := c.sicLookup.ReloadMappings(); err != nil {
			return err
		}
	}
	c.patterns = patterns
	return nil
}

// decide determines the category for one transaction without writing it, and
// reports which rule kind decided.
//
// This is the single place categorization priority is expressed. Import,
// "Recategorize All", per-category recategorization and SIC-scoped
// recategorization all route through it, so the rules cannot differ depending
// on which entry point the user reached.
// decide applies the guards that import needs, then the shared matcher.
func (c *Categorizer) decide(txn *model.Transaction) (int, categorySource) {
	// Manual assignments are never revisited.
	if txn.CategorySource == model.CategorySourceManual {
		return 0, sourceNone
	}
	// Import fills gaps; it does not revise existing work. Re-examination is
	// the path that does, and it uses decideOnReexamination instead.
	if txn.CategoryID != nil {
		return 0, sourceNone
	}
	return c.evaluate(txn)
}

// decideOnReexamination applies the current rules to a transaction as if it
// carried no category, which is what FR15 requires: what a transaction is
// categorized as must follow from the rules that exist now, not from the rules
// that existed when it was first seen.
//
// The manual guard still applies and is the only guard that does. BR-U5-06.
func (g ruleGeneration) decideOnReexamination(txn *model.Transaction) (int, categorySource) {
	if txn.CategorySource == model.CategorySourceManual {
		return 0, sourceNone
	}
	return g.evaluate(txn)
}

// evaluate is the shared matcher: text patterns in order, then a SIC mapping
// if none matched. One implementation, so the priority order cannot drift
// between import and re-examination — the property FR15 rests on.
//
// It deliberately carries NO guards, not even the manual one. That is required
// by BR-U5-14: FR16 reports how many manual transactions the rules would
// otherwise have moved, and a matcher that stopped on manual could not answer
// that question.
//
// Every caller must therefore guard for itself. decide and
// decideOnReexamination both do. The one caller that does not is the
// manual-protection count inside Reexamine, and that call sits in a branch
// which counts and continues before reaching any write: there is no code path
// from it to a database write. Treat that as a property to preserve, not an
// accident — if a write ever becomes reachable from an unguarded evaluate,
// manual protection is gone.
func (c *Categorizer) evaluate(txn *model.Transaction) (int, categorySource) {
	// The lock is held across both rule sources so one decision sees one
	// generation. Sampling patterns, releasing, then consulting mappings would
	// let a concurrent reload land in between.
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, pattern := range c.patterns {
		if pattern.Matches(txn.TransactionDetails) {
			return pattern.CategoryID, sourcePattern
		}
	}

	// SIC is consulted only when no text pattern matched.
	if c.sicLookup != nil && txn.SICCode != nil {
		if categoryID, ok := c.sicLookup.LookupCategory(*txn.SICCode); ok {
			return categoryID, sourceSIC
		}
	}

	return 0, sourceNone
}

// ruleGeneration is one immutable reading of the rules, taken once and used
// for an entire re-examination.
//
// A per-decision lock is not enough for a pass. Re-examination makes thousands
// of decisions, and any gap between them lets a reload publish new rules — so
// one pass could judge the first transaction by the old rules and the next by
// the new ones. Holding the categorizer's lock across the traversal fixes the
// publications that go through it, but the mapping cache has its own mutex and
// a wrapper can publish without touching the categorizer at all. A snapshot
// does not care how a publisher behaves.
type ruleGeneration struct {
	patterns []*model.CategoryPattern

	// sicByCode holds only the codes this pass will actually ask about,
	// resolved up front through the same public lookup a single decision uses.
	// Resolving through that interface rather than reaching for the underlying
	// cache is the point: it works for any lookup, including one that wraps
	// another.
	sicByCode map[model.SICCode]int
}

// snapshotRules resolves every rule this pass needs, once.
//
// Patterns are captured by reference because LoadRules replaces the slice
// wholesale rather than mutating it, so the captured value cannot change under
// the pass.
func (c *Categorizer) snapshotRules(transactions []*model.Transaction) ruleGeneration {
	codes := make(map[model.SICCode]struct{})
	for _, txn := range transactions {
		if txn.SICCode != nil {
			codes[*txn.SICCode] = struct{}{}
		}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	generation := ruleGeneration{
		patterns:  c.patterns,
		sicByCode: make(map[model.SICCode]int, len(codes)),
	}
	if c.sicLookup != nil {
		for code := range codes {
			if categoryID, ok := c.sicLookup.LookupCategory(code); ok {
				generation.sicByCode[code] = categoryID
			}
		}
	}
	return generation
}

// evaluate applies the snapshot's rules. Same priority order as the live path,
// and no guards, for the same reason: see evaluate on Categorizer.
func (g ruleGeneration) evaluate(txn *model.Transaction) (int, categorySource) {
	for _, pattern := range g.patterns {
		if pattern.Matches(txn.TransactionDetails) {
			return pattern.CategoryID, sourcePattern
		}
	}

	// SIC is consulted only when no text pattern matched.
	if txn.SICCode != nil {
		if categoryID, ok := g.sicByCode[*txn.SICCode]; ok {
			return categoryID, sourceSIC
		}
	}

	return 0, sourceNone
}

// Categorize attempts to categorize a single transaction in place.
// Returns true if a category was assigned.
func (c *Categorizer) Categorize(txn *model.Transaction) bool {
	categoryID, source := c.decide(txn)
	if source == sourceNone {
		return false
	}
	txn.SetCategory(categoryID, model.CategorySourceRule)
	return true
}

// RecategorizeResult contains the results of a re-categorization operation.
//
// The per-source counts are accumulated as the pass runs rather than derived
// afterwards, so PatternCategorizedCount + SICCategorizedCount always equals
// CategorizedCount. They partition the total; they do not sit beside it.
type RecategorizeResult struct {
	ProcessedCount   int `json:"processed_count"`
	CategorizedCount int `json:"categorized_count"`

	PatternCategorizedCount int `json:"pattern_categorized_count"`
	SICCategorizedCount     int `json:"sic_categorized_count"`

	// FR16. A rule change reports what it moved, what it uncategorized, and
	// what it left alone because the user had set it by hand.
	//
	// Added rather than replacing the four fields above, which the categories
	// and import pages already render. Import does not re-examine, so these
	// three are simply zero there.
	//
	// No omitempty: FR16 requires a rule change that moved nothing to say so,
	// and an absent field reads as "unknown" rather than "none".
	MovedCount           int `json:"moved_count"`
	UncategorizedCount   int `json:"uncategorized_count"`
	ManualProtectedCount int `json:"manual_protected_count"`
}

// Reexamine applies the current rules to every transaction the user did not
// categorize by hand, and is the single entry point for every rule change:
// a pattern created, changed or deleted; a SIC mapping created, changed or
// deleted; a category deleted; a mapping file uploaded.
//
// It replaces RecategorizeAll and RecategorizeByCategory. Both read only
// uncategorized transactions, which is the behaviour FR15 reverses — a
// category that came from a rule must follow that rule when it changes.
// RecategorizeByCategory additionally took a category ID it never used.
//
// Import does not call this. Re-examining on import would make the result
// depend on the order transactions arrived, which is exactly what FR15
// forbids.
//
// Read scope and write scope differ. Every transaction is read, because FR16
// reports how many manual ones the rules would otherwise have moved; only
// non-manual ones are written.
func (c *Categorizer) Reexamine() (*RecategorizeResult, error) {
	if err := c.LoadRules(); err != nil {
		return nil, err
	}

	transactions, err := c.txnRepo.GetAllForReexamination()
	if err != nil {
		slog.Error("Error reading transactions for re-examination", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to read transactions for re-examination: %w", err)
	}

	result := &RecategorizeResult{ProcessedCount: len(transactions)}

	// Keyed by category *and* rule source, so every batch is homogeneous in
	// both. That matters once a write can affect fewer rows than it was given:
	// with a mixed batch there would be no way to attribute the pattern/SIC
	// split to the rows that actually changed without inventing it.
	assignments := make(map[assignmentKey][]int)
	clearIDs := make([]int, 0)

	// One immutable snapshot of the rules for the whole pass. This is what
	// makes BR-U5-10 true, and a snapshot rather than a held lock because the
	// guarantee must not depend on every publisher cooperating: the mapping
	// cache has its own mutex, and a wrapper or a future lookup could publish
	// without ever touching this one.
	generation := c.snapshotRules(transactions)

	for _, txn := range transactions {
		if txn.CategorySource == model.CategorySourceManual {
			// Counting only. The generation applies no manual guard, which is
			// what makes this question answerable at all; the guard is here,
			// and this branch continues before any write is reachable.
			categoryID, source := generation.evaluate(txn)
			if wouldChangeCategory(txn, categoryID, source) {
				result.ManualProtectedCount++
			}
			continue
		}

		categoryID, source := generation.decideOnReexamination(txn)
		if !wouldChangeCategory(txn, categoryID, source) {
			// BR-U5-17: an unchanged outcome is neither written nor counted.
			continue
		}

		if source == sourceNone {
			// The rules that put this transaction here no longer say so.
			clearIDs = append(clearIDs, txn.TransactionID)
			continue
		}

		key := assignmentKey{categoryID: categoryID, source: source}
		assignments[key] = append(assignments[key], txn.TransactionID)
	}

	// Counts are claimed only after the corresponding write commits, so a
	// failure part-way through never reports rows it did not write. The manual
	// count is the exception and was claimed above, because it corresponds to
	// no write at all.
	for key, txnIDs := range assignments {
		changed, err := c.txnRepo.BulkUpdateCategory(key.categoryID, model.CategorySourceRule, txnIDs)
		if err != nil {
			slog.Error("Error assigning categories during re-examination",
				slog.Int("category_id", key.categoryID), slog.String("error", err.Error()))
			return nil, fmt.Errorf("failed to assign category %d during re-examination: %w", key.categoryID, err)
		}
		// Counted from rows actually written, not from what was intended. A
		// transaction the user made manual between the read and the write is
		// excluded by the statement itself, and must not be reported as moved.
		result.MovedCount += changed
		result.CategorizedCount += changed
		switch key.source {
		case sourcePattern:
			result.PatternCategorizedCount += changed
		case sourceSIC:
			result.SICCategorizedCount += changed
		}
	}

	if len(clearIDs) > 0 {
		cleared, err := c.txnRepo.BulkClearCategory(clearIDs)
		if err != nil {
			slog.Error("Error clearing categories during re-examination", slog.String("error", err.Error()))
			return nil, fmt.Errorf("failed to clear categories during re-examination: %w", err)
		}
		result.UncategorizedCount = cleared
	}

	slog.Info("Re-examined transactions against current rules",
		slog.Int("processed", result.ProcessedCount),
		slog.Int("moved", result.MovedCount),
		slog.Int("uncategorized", result.UncategorizedCount),
		slog.Int("manual_protected", result.ManualProtectedCount))

	return result, nil
}

// assignmentKey batches writes by target category and by the rule that chose
// it, so a partially applied write can be attributed without guessing.
type assignmentKey struct {
	categoryID int
	source     categorySource
}

// wouldChangeCategory reports whether applying the rules would leave the
// transaction somewhere other than where it is now.
//
// Used for both the write decision and the manual count, so "would have been
// moved" means exactly the same thing in the report as it does in the writes
// beside it.
func wouldChangeCategory(txn *model.Transaction, categoryID int, source categorySource) bool {
	if source == sourceNone {
		return txn.CategoryID != nil
	}
	return txn.CategoryID == nil || *txn.CategoryID != categoryID
}

// ClearCategory removes category assignments for a deleted category
// Sets transactions to uncategorized state
func (c *Categorizer) ClearCategory(categoryID int) error {
	// Get all transactions with this category (both rule and manual)
	filter := repository.TransactionFilter{
		CategoryID: &categoryID,
	}

	transactions, err := c.txnRepo.List(filter)
	if err != nil {
		slog.Error("Error getting transactions for category", slog.Int("category_id", categoryID), slog.String("error", err.Error()))
		return fmt.Errorf("failed to get transactions for category: %w", err)
	}

	// Every transaction in the deleted category becomes uncategorized, manual
	// ones included.
	//
	// This is a decision, not an oversight, recorded in FR7's 2026-09-07
	// amendment. Deleting a category is the user removing their own choice: FR7
	// protects a manual assignment from being revised by a *rule*, not from the
	// user's own deliberate act. Once the category is gone the choice cannot be
	// honoured at all, and the row is then an ordinary uncategorized
	// transaction that the current rules may claim.
	//
	// A revision of this code briefly kept category_source = manual here, to
	// preserve the marker. It produced a second meaning of "uncategorized" —
	// List(Uncategorized) counted such a row while GetUncategorized and
	// CountUncategorized did not — which is the one thing the approved design
	// forbids outright.
	for _, txn := range transactions {
		if err := c.txnRepo.UpdateCategory(txn.TransactionID, nil, model.CategorySourceNone); err != nil {
			slog.Error("Error clearing category for transaction", slog.Int("transaction_id", txn.TransactionID), slog.String("error", err.Error()))
			return fmt.Errorf("failed to clear category for transaction %d: %w", txn.TransactionID, err)
		}
	}

	return nil
}
