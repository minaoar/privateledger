package service

import (
	"fmt"

	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

// Categorizer handles automatic categorization of transactions based on patterns
type Categorizer struct {
	patternRepo *repository.CategoryPatternRepository
	txnRepo     *repository.TransactionRepository
	patterns    []*model.CategoryPattern
}

// NewCategorizer creates a new Categorizer
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

// LoadPatterns loads all category patterns from the database
func (c *Categorizer) LoadPatterns() error {
	patterns, err := c.patternRepo.GetAll()
	if err != nil {
		return fmt.Errorf("failed to load patterns: %w", err)
	}
	c.patterns = patterns
	return nil
}

// Categorize attempts to categorize a single transaction based on loaded patterns
// Returns true if a category was assigned, false otherwise
func (c *Categorizer) Categorize(txn *model.Transaction) bool {
	// Don't override manual categorizations
	if txn.CategorySource == model.CategorySourceManual {
		return false
	}

	// Try to match against patterns
	for _, pattern := range c.patterns {
		if pattern.Matches(txn.TransactionDetails) {
			// Found a match - assign category
			txn.SetCategory(pattern.CategoryID, model.CategorySourceRule)
			return true
		}
	}

	// No match found
	return false
}

// RecategorizeResult contains the results of a re-categorization operation
type RecategorizeResult struct {
	ProcessedCount   int `json:"processed_count"`
	CategorizedCount int `json:"categorized_count"`
}

// RecategorizeAll re-categorizes all uncategorized transactions
// This is called when new patterns are added
func (c *Categorizer) RecategorizeAll() (*RecategorizeResult, error) {
	// Reload patterns to get latest
	if err := c.LoadPatterns(); err != nil {
		return nil, err
	}

	// Get all uncategorized transactions
	transactions, err := c.txnRepo.GetUncategorized()
	if err != nil {
		return nil, fmt.Errorf("failed to get uncategorized transactions: %w", err)
	}

	result := &RecategorizeResult{
		ProcessedCount: len(transactions),
	}

	// Group transactions by category for bulk update
	categoryMap := make(map[int][]int) // categoryID -> []transactionIDs

	for _, txn := range transactions {
		// Try to categorize
		for _, pattern := range c.patterns {
			if pattern.Matches(txn.TransactionDetails) {
				// Found a match
				categoryMap[pattern.CategoryID] = append(categoryMap[pattern.CategoryID], txn.TransactionID)
				result.CategorizedCount++
				break // First match wins
			}
		}
	}

	// Bulk update transactions by category
	for categoryID, txnIDs := range categoryMap {
		err := c.txnRepo.BulkUpdateCategory(categoryID, model.CategorySourceRule, txnIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to bulk update category %d: %w", categoryID, err)
		}
	}

	return result, nil
}

// RecategorizeByCategory re-categorizes transactions for a specific category
// This is called when patterns for a category are modified
func (c *Categorizer) RecategorizeByCategory(categoryID int) (*RecategorizeResult, error) {
	// Reload patterns to get latest
	if err := c.LoadPatterns(); err != nil {
		return nil, err
	}

	// Get patterns for this category
	patterns, err := c.patternRepo.GetByCategoryID(categoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to get patterns for category: %w", err)
	}

	// Get all uncategorized transactions
	transactions, err := c.txnRepo.GetUncategorized()
	if err != nil {
		return nil, fmt.Errorf("failed to get uncategorized transactions: %w", err)
	}

	result := &RecategorizeResult{
		ProcessedCount: len(transactions),
	}

	var matchedTxnIDs []int

	// Find transactions that match any pattern for this category
	for _, txn := range transactions {
		for _, pattern := range patterns {
			if pattern.Matches(txn.TransactionDetails) {
				matchedTxnIDs = append(matchedTxnIDs, txn.TransactionID)
				result.CategorizedCount++
				break // First match wins
			}
		}
	}

	// Bulk update matched transactions
	if len(matchedTxnIDs) > 0 {
		err := c.txnRepo.BulkUpdateCategory(categoryID, model.CategorySourceRule, matchedTxnIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to bulk update transactions: %w", err)
		}
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
		return fmt.Errorf("failed to get transactions for category: %w", err)
	}

	// Clear category for each transaction
	for _, txn := range transactions {
		err := c.txnRepo.UpdateCategory(txn.TransactionID, nil, model.CategorySourceNone)
		if err != nil {
			return fmt.Errorf("failed to clear category for transaction %d: %w", txn.TransactionID, err)
		}
	}

	return nil
}
