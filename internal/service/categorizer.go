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
	// Scoped recategorization lives on the mapping source but must apply the
	// same priority as every other entry point, so it is given the shared
	// decision function rather than deciding for itself.
	if host, ok := sicLookup.(interface{ attachDecider(sicDecider) }); ok {
		host.attachDecider(c)
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
// mixture. A source that can only reload itself falls back to publishing
// patterns first and restoring them if the mapping reload then fails, so a
// failed reload still leaves rules unchanged.
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

	c.mu.Lock()
	previous := c.patterns
	c.patterns = patterns
	c.mu.Unlock()

	if c.sicLookup != nil {
		if err := c.sicLookup.ReloadMappings(); err != nil {
			c.mu.Lock()
			c.patterns = previous
			c.mu.Unlock()
			return err
		}
	}
	return nil
}

// decide determines the category for one transaction without writing it, and
// reports which rule kind decided.
//
// This is the single place categorization priority is expressed. Import,
// "Recategorize All", per-category recategorization and SIC-scoped
// recategorization all route through it, so the rules cannot differ depending
// on which entry point the user reached.
func (c *Categorizer) decide(txn *model.Transaction) (int, categorySource) {
	// Manual assignments are never revisited.
	if txn.CategorySource == model.CategorySourceManual {
		return 0, sourceNone
	}
	// Automatic categorization fills gaps; it does not revise existing work.
	if txn.CategoryID != nil {
		return 0, sourceNone
	}

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
}

// RecategorizeAll re-categorizes all uncategorized transactions.
// Called when rules change or from the Categories page action.
//
// Because it routes through the decision function, SIC mappings apply here as
// well as during import. Before, this re-implemented pattern matching inline,
// so SIC would have applied on import and been silently skipped by an explicit
// "Recategorize All" - the same rules giving different answers depending on
// which path the user took.
func (c *Categorizer) RecategorizeAll() (*RecategorizeResult, error) {
	if err := c.LoadRules(); err != nil {
		return nil, err
	}

	transactions, err := c.txnRepo.GetUncategorized()
	if err != nil {
		slog.Error("Error getting uncategorized transactions in RecategorizeAll", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get uncategorized transactions: %w", err)
	}

	result := &RecategorizeResult{ProcessedCount: len(transactions)}

	categoryMap := make(map[int][]int)
	patternAssigned := make(map[int]int)
	sicAssigned := make(map[int]int)

	for _, txn := range transactions {
		categoryID, source := c.decide(txn)
		switch source {
		case sourcePattern:
			patternAssigned[txn.TransactionID] = categoryID
		case sourceSIC:
			sicAssigned[txn.TransactionID] = categoryID
		default:
			continue
		}
		categoryMap[categoryID] = append(categoryMap[categoryID], txn.TransactionID)
	}

	// Counts are claimed only after the corresponding update commits, so a
	// failure part-way through never reports rows it did not write.
	for categoryID, txnIDs := range categoryMap {
		if err := c.txnRepo.BulkUpdateCategory(categoryID, model.CategorySourceRule, txnIDs); err != nil {
			slog.Error("Error in bulk update category", slog.Int("category_id", categoryID), slog.String("error", err.Error()))
			return nil, fmt.Errorf("failed to bulk update category %d: %w", categoryID, err)
		}
		for _, id := range txnIDs {
			if _, ok := patternAssigned[id]; ok {
				result.PatternCategorizedCount++
			} else if _, ok := sicAssigned[id]; ok {
				result.SICCategorizedCount++
			}
			result.CategorizedCount++
		}
	}

	return result, nil
}

// RecategorizeByCategory re-categorizes transactions that this category's
// rules now claim. Called when a category's patterns are modified.
//
// It routes through the same decision function, so a transaction is assigned
// here only if the full priority order would assign it - a higher-priority
// pattern belonging to another category still wins.
func (c *Categorizer) RecategorizeByCategory(categoryID int) (*RecategorizeResult, error) {
	if err := c.LoadRules(); err != nil {
		return nil, err
	}

	transactions, err := c.txnRepo.GetUncategorized()
	if err != nil {
		slog.Error("Error getting uncategorized transactions", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get uncategorized transactions: %w", err)
	}

	result := &RecategorizeResult{ProcessedCount: len(transactions)}

	var matchedTxnIDs []int
	patternCount, sicCount := 0, 0

	for _, txn := range transactions {
		resolved, source := c.decide(txn)
		if source == sourceNone || resolved != categoryID {
			continue
		}
		matchedTxnIDs = append(matchedTxnIDs, txn.TransactionID)
		if source == sourceSIC {
			sicCount++
		} else {
			patternCount++
		}
	}

	if len(matchedTxnIDs) > 0 {
		if err := c.txnRepo.BulkUpdateCategory(categoryID, model.CategorySourceRule, matchedTxnIDs); err != nil {
			slog.Error("Error in bulk update transactions", slog.Int("category_id", categoryID), slog.String("error", err.Error()))
			return nil, fmt.Errorf("failed to bulk update transactions: %w", err)
		}
		result.CategorizedCount = len(matchedTxnIDs)
		result.PatternCategorizedCount = patternCount
		result.SICCategorizedCount = sicCount
	}

	return result, nil
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

	// Clear category for each transaction
	for _, txn := range transactions {
		err := c.txnRepo.UpdateCategory(txn.TransactionID, nil, model.CategorySourceNone)
		if err != nil {
			slog.Error("Error clearing category for transaction", slog.Int("transaction_id", txn.TransactionID), slog.String("error", err.Error()))
			return fmt.Errorf("failed to clear category for transaction %d: %w", txn.TransactionID, err)
		}
	}

	return nil
}
