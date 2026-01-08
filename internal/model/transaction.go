package model

import (
	"strings"
	"time"
)

// TransactionType represents whether a transaction is a debit or credit
type TransactionType int

const (
	TransactionTypeDebit  TransactionType = 1 // Money out
	TransactionTypeCredit TransactionType = 2 // Money in
)

// Transaction represents a bank transaction
type Transaction struct {
	TransactionID int       `json:"transaction_id" db:"transaction_id"`
	AccountID     int       `json:"account_id" db:"account_id"`
	ImportBatchID *int      `json:"import_batch_id" db:"import_batch_id"` // Nullable
	CreatedAt     time.Time `json:"created_at" db:"created_at"`

	// OFX fields (used for deduplication)
	TrnType    string    `json:"trn_type" db:"trn_type"`
	FitID      string    `json:"fit_id" db:"fit_id"`
	DatePosted time.Time `json:"date_posted" db:"date_posted"`
	Amount     float64   `json:"amount" db:"amount"`

	// Processed fields
	TransactionDetails string          `json:"transaction_details" db:"transaction_details"`
	TransactionType    TransactionType `json:"transaction_type" db:"transaction_type"`

	// Categorization
	CategoryID     *int           `json:"category_id" db:"category_id"` // Nullable
	CategorySource CategorySource `json:"category_source" db:"category_source"`

	// Display fields (populated via JOIN, not stored in DB)
	AccountName   *string `json:"account_name,omitempty" db:"account_name"`
	CategoryName  *string `json:"category_name,omitempty" db:"category_name"`
	CategoryColor *string `json:"category_color,omitempty" db:"category_color"`
	CategoryIcon  *string `json:"category_icon,omitempty" db:"category_icon"`
}

// NewTransaction creates a new Transaction from OFX data
func NewTransaction(accountID int, trnType, fitID string, datePosted time.Time, amount float64, details string) *Transaction {
	return &Transaction{
		AccountID:          accountID,
		TrnType:            trnType,
		FitID:              fitID,
		DatePosted:         datePosted,
		Amount:             amount,
		TransactionDetails: details,
		TransactionType:    DeriveTransactionType(trnType, amount),
		CategorySource:     CategorySourceNone,
		CreatedAt:          time.Now(),
	}
}

// DeriveTransactionType determines if a transaction is a debit or credit
// based on the OFX transaction type and amount
func DeriveTransactionType(trnType string, amount float64) TransactionType {
	switch strings.ToUpper(trnType) {
	case "DEBIT":
		return TransactionTypeDebit
	case "CREDIT":
		return TransactionTypeCredit
	default:
		// If not explicitly labeled, use amount sign
		if amount >= 0 {
			return TransactionTypeCredit
		}
		return TransactionTypeDebit
	}
}

// IsUncategorized returns true if the transaction has no category assigned
func (t *Transaction) IsUncategorized() bool {
	return t.CategoryID == nil || t.CategorySource == CategorySourceNone
}

// SetCategory assigns a category to the transaction
func (t *Transaction) SetCategory(categoryID int, source CategorySource) {
	t.CategoryID = &categoryID
	t.CategorySource = source
}

// ClearCategory removes the category assignment
func (t *Transaction) ClearCategory() {
	t.CategoryID = nil
	t.CategorySource = CategorySourceNone
}
