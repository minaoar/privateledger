package service

import (
	"fmt"
	"log/slog"
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

// RecategorizeBySICCodes categorizes currently uncategorized transactions
// carrying any of the supplied codes, and returns how many changed.
//
// It satisfies the UOW-2 collaborator contract. Scope is deliberately narrow:
// a mapping change may only fill gaps, so manual assignments and existing
// categories are never revisited, and no transaction outside the supplied set
// is touched.
func (c *SICMappingCategorizer) RecategorizeBySICCodes(sicCodes []model.SICCode) (int, error) {
	if len(sicCodes) == 0 {
		return 0, nil
	}

	codes := make([]string, 0, len(sicCodes))
	for _, code := range sicCodes {
		codes = append(codes, string(code))
	}

	transactions, err := c.txnRepo.GetUncategorizedBySICCodes(codes)
	if err != nil {
		return 0, fmt.Errorf("failed to load transactions for SIC recategorization: %w", err)
	}
	if len(transactions) == 0 {
		return 0, nil
	}

	byCategory := make(map[int][]int)
	for _, txn := range transactions {
		if txn.SICCode == nil {
			continue
		}
		categoryID, ok := c.LookupCategory(*txn.SICCode)
		if !ok {
			continue
		}
		byCategory[categoryID] = append(byCategory[categoryID], txn.TransactionID)
	}

	recategorized := 0
	for categoryID, ids := range byCategory {
		if err := c.txnRepo.BulkUpdateCategory(categoryID, model.CategorySourceRule, ids); err != nil {
			slog.Error("Failed to apply SIC recategorization",
				slog.Int("category_id", categoryID),
				slog.Int("transaction_count", len(ids)),
				slog.String("error", err.Error()))
			return recategorized, fmt.Errorf("failed to recategorize by SIC mapping: %w", err)
		}
		recategorized += len(ids)
	}
	return recategorized, nil
}
