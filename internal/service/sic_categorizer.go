package service

import (
	"fmt"
	"sync"

	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

// SICMappingCategorizer resolves SIC codes to categories and applies mapping
// changes to already-imported transactions.
//
// It is the SIC half of categorization, kept out of Categorizer so priority
// logic lives in one place and SIC lookup can be substituted in tests. It owns
// no text-pattern logic and makes no priority decision: Categorizer decides
// when to consult it.
//
// The same instance is wired in two roles - as Categorizer's lookup and as the
// recategorization collaborator UOW-2 calls after a mapping change - so the
// cache the lookup reads is always the cache a mapping change reloads.
type SICMappingCategorizer struct {
	sicRepo *repository.SICMappingRepository
	txnRepo *repository.TransactionRepository

	mu       sync.RWMutex
	mappings map[model.SICCode]*model.SICMapping

	// decider supplies the shared priority order to decideForTransaction, so
	// this type never decides for itself.
	decider sicDecider

	// reexaminer is the one re-examination entry point. This type holds it
	// only to satisfy the mapping service's collaborator contract; it performs
	// no recategorization of its own.
	reexaminer reexaminer
}

// reexaminer is the narrow view of Categorizer this type needs, kept narrow so
// the adapter cannot reach anything else on it.
type reexaminer interface {
	Reexamine() (*RecategorizeResult, error)
}

// attachDecider is called during categorizer construction.
func (c *SICMappingCategorizer) attachDecider(d sicDecider) {
	c.decider = d
}

// attachReexaminer is called during categorizer construction, alongside
// attachDecider, so the two are always wired together.
func (c *SICMappingCategorizer) attachReexaminer(r reexaminer) {
	c.reexaminer = r
}

// prepareMappings builds a replacement index without publishing it, so the
// categorizer can publish patterns and mappings as one generation.
func (c *SICMappingCategorizer) prepareMappings() (map[model.SICCode]*model.SICMapping, error) {
	return c.buildMappingIndex()
}

// commitMappings publishes a prepared index. The caller holds the
// categorizer's write lock, which is what makes the swap atomic with patterns.
func (c *SICMappingCategorizer) commitMappings(index map[model.SICCode]*model.SICMapping) {
	c.publishMappingIndex(index)
}

// NewSICMappingCategorizer creates the SIC categorization extension.
func NewSICMappingCategorizer(
	sicRepo *repository.SICMappingRepository,
	txnRepo *repository.TransactionRepository,
) *SICMappingCategorizer {
	return &SICMappingCategorizer{
		sicRepo:  sicRepo,
		txnRepo:  txnRepo,
		mappings: make(map[model.SICCode]*model.SICMapping),
	}
}

// buildMappingIndex reads authoritative mapping state without publishing it.
// Reload builds first and swaps second so a failure leaves the live cache
// untouched.
func (c *SICMappingCategorizer) buildMappingIndex() (map[model.SICCode]*model.SICMapping, error) {
	mappings, err := c.sicRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load SIC mappings: %w", err)
	}
	index := make(map[model.SICCode]*model.SICMapping, len(mappings))
	for _, mapping := range mappings {
		index[mapping.SICCode] = mapping
	}
	return index, nil
}

// publishMappingIndex installs a prepared index.
func (c *SICMappingCategorizer) publishMappingIndex(index map[model.SICCode]*model.SICMapping) {
	c.mu.Lock()
	c.mappings = index
	c.mu.Unlock()
}

// ReloadMappings refreshes the cache from SQLite. It satisfies the UOW-2
// collaborator contract.
//
// Publication happens on this type's own mutex. That is safe for a single
// lookup, and it is deliberately NOT what a re-examination pass relies on: a
// pass resolves the rules it needs up front and evaluates against that
// snapshot, so a reload landing mid-traversal cannot split it. Requiring every
// publisher to cooperate with the categorizer's lock would leave the guarantee
// resting on wiring, which a wrapper or a future lookup could quietly bypass.
func (c *SICMappingCategorizer) ReloadMappings() error {
	index, err := c.buildMappingIndex()
	if err != nil {
		return err
	}
	c.publishMappingIndex(index)
	return nil
}

// LookupCategory returns the category a SIC code maps to.
//
// The two "no category" cases are deliberately indistinguishable to the
// caller: an absent mapping and a mapping with an empty category both assign
// nothing. An empty category is a decision that this code should never
// categorize, not a gap awaiting data.
func (c *SICMappingCategorizer) LookupCategory(sicCode model.SICCode) (int, bool) {
	c.mu.RLock()
	mapping := c.mappings[sicCode]
	c.mu.RUnlock()

	if mapping == nil || !mapping.HasCategory() {
		return 0, false
	}
	return *mapping.CategoryID, true
}

// decideForTransaction applies the shared priority order.
//
// The shared decision function decides whenever a categorizer is attached, so a
// transaction whose description matches a text pattern keeps the
// higher-priority text category. Wired standalone there are no text patterns to
// outrank SIC, but the preservation guards still apply: a manual source or an
// existing category is never revised.
func (c *SICMappingCategorizer) decideForTransaction(txn *model.Transaction) (int, bool) {
	if c.decider != nil {
		return c.decider.decideCategory(txn)
	}
	if txn.CategorySource == model.CategorySourceManual || txn.CategoryID != nil {
		return 0, false
	}
	if txn.SICCode == nil {
		return 0, false
	}
	return c.LookupCategory(*txn.SICCode)
}

// Reexamine satisfies the mapping service's collaborator contract by
// delegating to the one re-examination entry point.
//
// This is a thin adapter on purpose. It replaced RecategorizeBySICCodes, which
// loaded transactions by affected code and applied SIC mappings itself — a
// second implementation of a decision that now has exactly one. Keeping the
// scoped path would have meant two places deciding where a transaction
// belongs, and FR15 is a claim about a single outcome.
//
// A nil decider means no Categorizer was attached, which is a wiring fault
// rather than a state a caller can recover from; production wiring always
// attaches one.
func (c *SICMappingCategorizer) Reexamine() (RecategorizationCounts, error) {
	if c.reexaminer == nil {
		return RecategorizationCounts{}, fmt.Errorf("SIC categorizer has no re-examination target attached")
	}
	result, err := c.reexaminer.Reexamine()
	if err != nil {
		return RecategorizationCounts{}, err
	}
	return RecategorizationCounts{
		Moved:           result.MovedCount,
		Uncategorized:   result.UncategorizedCount,
		ManualProtected: result.ManualProtectedCount,
	}, nil
}
