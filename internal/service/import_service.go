package service

import (
	"fmt"
	"io"

	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/parser"
	"github.com/oronno/privateledger/internal/repository"
)

// ImportService handles OFX file imports with deduplication and categorization
type ImportService struct {
	parser       *parser.OFXParser
	txnRepo      *repository.TransactionRepository
	accountRepo  *repository.AccountRepository
	categorizer  *Categorizer
	batchRepo    *repository.ImportBatchRepository
}

// NewImportService creates a new ImportService
func NewImportService(
	parser *parser.OFXParser,
	txnRepo *repository.TransactionRepository,
	accountRepo *repository.AccountRepository,
	categorizer *Categorizer,
	batchRepo *repository.ImportBatchRepository,
) *ImportService {
	return &ImportService{
		parser:      parser,
		txnRepo:     txnRepo,
		accountRepo: accountRepo,
		categorizer: categorizer,
		batchRepo:   batchRepo,
	}
}

// ImportResult contains the results of an OFX import operation
type ImportResult struct {
	TotalTransactions int                      `json:"total_transactions"`
	ImportedCount     int                      `json:"imported_count"`
	DuplicateCount    int                      `json:"duplicate_count"`
	CategorizedCount  int                      `json:"categorized_count"`
	ErrorCount        int                      `json:"error_count"`
	Errors            []string                 `json:"errors,omitempty"`
	Transactions      []*model.Transaction     `json:"transactions,omitempty"`
}

// ImportOFX imports transactions from an OFX file
func (s *ImportService) ImportOFX(reader io.Reader, accountID int, batchID *int) (*ImportResult, error) {
	// Verify account exists
	account, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}
	if account == nil {
		return nil, fmt.Errorf("account not found: %d", accountID)
	}

	// Parse OFX file
	parseResult, err := s.parser.ParseOFXFile(reader, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OFX file: %w", err)
	}

	result := &ImportResult{
		TotalTransactions: len(parseResult.Transactions),
		Errors:            make([]string, 0),
		Transactions:      make([]*model.Transaction, 0),
	}

	// Process each transaction
	for _, txn := range parseResult.Transactions {
		// Set batch ID if provided
		if batchID != nil {
			txn.ImportBatchID = batchID
		}

		// Check for duplicates
		duplicate, err := s.txnRepo.FindDuplicate(
			txn.AccountID,
			txn.TrnType,
			txn.FitID,
			txn.DatePosted,
		)
		if err != nil {
			result.ErrorCount++
			result.Errors = append(result.Errors,
				fmt.Sprintf("Error checking duplicate for %s: %v", txn.FitID, err))
			continue
		}

		if duplicate != nil {
			// Transaction already exists
			result.DuplicateCount++
			continue
		}

		// Apply categorization before inserting
		if s.categorizer.Categorize(txn) {
			result.CategorizedCount++
		}

		// Insert transaction
		err = s.txnRepo.Create(txn)
		if err != nil {
			result.ErrorCount++
			result.Errors = append(result.Errors,
				fmt.Sprintf("Error inserting transaction %s: %v", txn.FitID, err))
			continue
		}

		result.ImportedCount++
		result.Transactions = append(result.Transactions, txn)
	}

	// Update batch record if batch ID was provided
	if batchID != nil && s.batchRepo != nil {
		batch, err := s.batchRepo.GetByID(*batchID)
		if err == nil && batch != nil {
			batch.ImportedTransactions = &result.ImportedCount
			batch.DuplicateTransactions = &result.DuplicateCount
			batch.TotalAutoCategorized = &result.CategorizedCount
			s.batchRepo.Update(batch)
		}
	}

	return result, nil
}

// ValidateOFX validates an OFX file without importing
func (s *ImportService) ValidateOFX(reader io.Reader) error {
	return s.parser.ValidateOFXFile(reader)
}
