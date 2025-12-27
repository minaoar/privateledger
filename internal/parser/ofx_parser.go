package parser

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aclindsa/ofxgo"
	"github.com/oronno/privateledger/internal/model"
)

// OFXParser handles parsing of OFX/SGML files from Canadian banks
type OFXParser struct{}

// NewOFXParser creates a new OFXParser instance
func NewOFXParser() *OFXParser {
	return &OFXParser{}
}

// ParseResult holds the result of parsing an OFX file
type ParseResult struct {
	Transactions []*model.Transaction
	BankID       string
	AccountID    string
	AccountType  string // "CHECKING", "SAVINGS", "CREDITCARD", etc.
	Currency     string
	StartDate    *time.Time
	EndDate      *time.Time
}

// ParseOFXFile parses an OFX file and extracts transactions
func (p *OFXParser) ParseOFXFile(reader io.Reader, accountID int) (*ParseResult, error) {
	// Parse OFX using ofxgo library
	response, err := ofxgo.ParseResponse(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OFX file: %w", err)
	}

	result := &ParseResult{
		Transactions: make([]*model.Transaction, 0),
	}

	// Try to extract bank statement transactions (checking/savings accounts)
	if response.Bank != nil && len(response.Bank) > 0 {
		for _, msg := range response.Bank {
			// Type assert to StatementResponse
			if stmt, ok := msg.(*ofxgo.StatementResponse); ok {
				p.extractBankTransactions(stmt, accountID, result)
			}
		}
	}

	// Try to extract credit card statement transactions
	if response.CreditCard != nil && len(response.CreditCard) > 0 {
		for _, msg := range response.CreditCard {
			// Type assert to CCStatementResponse
			if stmt, ok := msg.(*ofxgo.CCStatementResponse); ok {
				p.extractCreditCardTransactions(stmt, accountID, result)
			}
		}
	}

	if len(result.Transactions) == 0 {
		return nil, fmt.Errorf("no transactions found in OFX file")
	}

	return result, nil
}

// extractBankTransactions extracts transactions from bank statement messages
func (p *OFXParser) extractBankTransactions(stmt *ofxgo.StatementResponse, accountID int, result *ParseResult) {
	// Set account metadata
	if stmt.BankAcctFrom.BankID != "" {
		result.BankID = string(stmt.BankAcctFrom.BankID)
	}
	if stmt.BankAcctFrom.AcctID != "" {
		result.AccountID = string(stmt.BankAcctFrom.AcctID)
	}
	acctType := stmt.BankAcctFrom.AcctType.String()
	if acctType != "" {
		result.AccountType = acctType
	}
	if valid, _ := stmt.CurDef.Valid(); valid {
		result.Currency = stmt.CurDef.String()
	}

	// Set date range
	if !stmt.BankTranList.DtStart.IsZero() {
		startDate := stmt.BankTranList.DtStart.Time
		result.StartDate = &startDate
	}
	if !stmt.BankTranList.DtEnd.IsZero() {
		endDate := stmt.BankTranList.DtEnd.Time
		result.EndDate = &endDate
	}

	// Extract transactions
	for _, txn := range stmt.BankTranList.Transactions {
		transaction := p.convertOFXTransaction(&txn, accountID)
		if transaction != nil {
			result.Transactions = append(result.Transactions, transaction)
		}
	}
}

// extractCreditCardTransactions extracts transactions from credit card statement messages
func (p *OFXParser) extractCreditCardTransactions(stmt *ofxgo.CCStatementResponse, accountID int, result *ParseResult) {
	// Set account metadata
	if stmt.CCAcctFrom.AcctID != "" {
		result.AccountID = string(stmt.CCAcctFrom.AcctID)
	}
	result.AccountType = "CREDITCARD"
	if valid, _ := stmt.CurDef.Valid(); valid {
		result.Currency = stmt.CurDef.String()
	}

	// Set date range
	if !stmt.BankTranList.DtStart.IsZero() {
		startDate := stmt.BankTranList.DtStart.Time
		result.StartDate = &startDate
	}
	if !stmt.BankTranList.DtEnd.IsZero() {
		endDate := stmt.BankTranList.DtEnd.Time
		result.EndDate = &endDate
	}

	// Extract transactions
	for _, txn := range stmt.BankTranList.Transactions {
		transaction := p.convertOFXTransaction(&txn, accountID)
		if transaction != nil {
			result.Transactions = append(result.Transactions, transaction)
		}
	}
}

// convertOFXTransaction converts an ofxgo.Transaction to our model.Transaction
func (p *OFXParser) convertOFXTransaction(ofxTxn *ofxgo.Transaction, accountID int) *model.Transaction {
	// Extract required fields
	if ofxTxn.FiTID == "" || ofxTxn.DtPosted.IsZero() {
		// Skip invalid transactions
		return nil
	}

	fitID := string(ofxTxn.FiTID)
	datePosted := ofxTxn.DtPosted.Time

	// Convert amount to float64
	amountFloat, _ := ofxTxn.TrnAmt.Rat.Float64()

	// Extract transaction type (default to "OTHER" if not specified)
	trnType := ofxTxn.TrnType.String()
	if trnType == "" {
		trnType = "OTHER"
	}

	// Merge NAME and MEMO into transaction_details
	details := p.mergeDetails(ofxTxn)

	// Create transaction model
	transaction := model.NewTransaction(
		accountID,
		trnType,
		fitID,
		datePosted,
		amountFloat,
		details,
	)

	return transaction
}

// mergeDetails merges NAME and MEMO fields into a single description
// Canadian banks use these fields differently:
// - TD: Uses NAME, no MEMO
// - RBC: Uses both NAME and MEMO (MEMO has location)
// - CIBC: Sometimes NAME, always MEMO (payment context)
func (p *OFXParser) mergeDetails(txn *ofxgo.Transaction) string {
	var parts []string

	// Add NAME if present
	if txn.Name != "" {
		name := strings.TrimSpace(string(txn.Name))
		if name != "" {
			parts = append(parts, name)
		}
	}

	// Add MEMO if present and different from NAME
	if txn.Memo != "" {
		memo := strings.TrimSpace(string(txn.Memo))
		if memo != "" {
			// Only add memo if it's different from name (avoid duplication)
			if len(parts) == 0 || parts[0] != memo {
				parts = append(parts, memo)
			}
		}
	}

	// Fallback to FITID if no description available
	if len(parts) == 0 && txn.FiTID != "" {
		parts = append(parts, fmt.Sprintf("Transaction %s", string(txn.FiTID)))
	}

	// Join with " - " separator
	return strings.Join(parts, " - ")
}

// ValidateOFXFile performs basic validation on an OFX file without full parsing
func (p *OFXParser) ValidateOFXFile(reader io.Reader) error {
	// Try to parse the OFX file
	_, err := ofxgo.ParseResponse(reader)
	if err != nil {
		return fmt.Errorf("invalid OFX file: %w", err)
	}
	return nil
}
