package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/oronno/privateledger/internal/model"
)

// TransactionRepository handles database operations for transactions
type TransactionRepository struct {
	db *sql.DB
}

// NewTransactionRepository creates a new TransactionRepository
func NewTransactionRepository(db *sql.DB) *TransactionRepository {
	return &TransactionRepository{db: db}
}

// Create inserts a new transaction into the database
func (r *TransactionRepository) Create(txn *model.Transaction) error {
	query := `
		INSERT INTO ledger_transaction (
			account_id, import_batch_id, trn_type, fit_id, date_posted, amount,
			transaction_details, transaction_type, sic_code, category_id, category_source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.Exec(query,
		txn.AccountID,
		txn.ImportBatchID,
		txn.TrnType,
		txn.FitID,
		formatDatePosted(txn.DatePosted),
		txn.Amount,
		txn.TransactionDetails,
		txn.TransactionType,
		txn.SICCode,
		txn.CategoryID,
		txn.CategorySource,
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get transaction ID: %w", err)
	}

	txn.TransactionID = int(id)
	return nil
}

// formatDatePosted parse datePosted as string in simplified format without timezone ("2006-01-02 15:04:05") while interacting with DB.
// The modernc.org/sqlite driver would otherwise call time.Time.String() which includes timezone.
// In some cases, OFX-parsed timestamps look like "2025-10-22 12:00:00 -0500 -0500" in sqlite DB, which can cause issues when parsing the value back. Stripping the timezone eliminates these problems.
func formatDatePosted(datePosted time.Time) string {
	return datePosted.Format(time.DateTime)
}

// FindDuplicate checks if a transaction already exists (for deduplication)
func (r *TransactionRepository) FindDuplicate(accountID int, trnType, fitID string, datePosted time.Time) (*model.Transaction, error) {
	query := `
		SELECT transaction_id, account_id, trn_type, fit_id, date_posted, amount,
			transaction_details, transaction_type, sic_code, category_id, category_source, created_at
		FROM ledger_transaction
		WHERE account_id = ? AND trn_type = ? AND fit_id = ? AND date_posted = ?
	`

	var txn model.Transaction
	err := r.db.QueryRow(query, accountID, trnType, fitID, formatDatePosted(datePosted)).Scan(
		&txn.TransactionID,
		&txn.AccountID,
		&txn.TrnType,
		&txn.FitID,
		&txn.DatePosted,
		&txn.Amount,
		&txn.TransactionDetails,
		&txn.TransactionType,
		&txn.SICCode,
		&txn.CategoryID,
		&txn.CategorySource,
		&txn.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find duplicate: %w", err)
	}

	return &txn, nil
}

// GetByID retrieves a transaction by its ID
func (r *TransactionRepository) GetByID(transactionID int) (*model.Transaction, error) {
	query := `
		SELECT t.transaction_id, t.account_id, t.trn_type, t.fit_id, t.date_posted, t.amount,
			t.transaction_details, t.transaction_type, t.sic_code, t.category_id, t.category_source, t.created_at,
			COALESCE(NULLIF(sm.description, ''), sm.description_detail) AS sic_description
		FROM ledger_transaction t
		LEFT JOIN sic_mapping sm ON t.sic_code = sm.sic_code
		WHERE t.transaction_id = ?
	`

	var txn model.Transaction
	err := r.db.QueryRow(query, transactionID).Scan(
		&txn.TransactionID,
		&txn.AccountID,
		&txn.TrnType,
		&txn.FitID,
		&txn.DatePosted,
		&txn.Amount,
		&txn.TransactionDetails,
		&txn.TransactionType,
		&txn.SICCode,
		&txn.CategoryID,
		&txn.CategorySource,
		&txn.CreatedAt,
		&txn.SICDescription,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	return &txn, nil
}

// TransactionFilter holds filter criteria for listing transactions
type TransactionFilter struct {
	AccountID     *int                // Filter by account
	CategoryID    *int                // Filter by category
	CategoryType  *model.CategoryType // Filter by category type (Expense/Income/Investment)
	BatchID       *int                // Filter by import batch
	Uncategorized bool                // Only uncategorized transactions (category_id IS NULL OR category_source = 0)
	StartDate     *time.Time          // Date range start (inclusive)
	EndDate       *time.Time          // Date range end (inclusive)
	Limit         int                 // Max results (0 = no limit)
	Offset        int                 // Pagination offset

	// CategoryIsNull matches only transactions with no category_id at all. This is
	// strictly narrower than Uncategorized, which also matches rows that have a
	// category_id but category_source = 0. Use this filter when the result is summed
	// alongside per-category totals, so a row cannot be counted in both.
	CategoryIsNull bool

	// TransactionType filters by debit (1) or credit (2).
	TransactionType *model.TransactionType
}

// List retrieves transactions with optional filters
func (r *TransactionRepository) List(filter TransactionFilter) ([]*model.Transaction, error) {
	slog.Info("fetching transactions in List()", "filter", filter)
	query := `
		SELECT
			t.transaction_id, t.account_id, t.import_batch_id, t.trn_type, t.fit_id, t.date_posted, t.amount,
			t.transaction_details, t.transaction_type, t.sic_code, t.category_id, t.category_source, t.created_at,
			a.name as account_name,
			c.name as category_name,
			c.color as category_color,
			c.icon as category_icon,
			COALESCE(NULLIF(sm.description, ''), sm.description_detail) AS sic_description
		FROM ledger_transaction t
		LEFT JOIN account a ON t.account_id = a.account_id
		LEFT JOIN category c ON t.category_id = c.category_id
		LEFT JOIN sic_mapping sm ON t.sic_code = sm.sic_code
		WHERE 1=1
	`
	var args []interface{}

	// Apply filters
	if filter.AccountID != nil {
		query += " AND t.account_id = ?"
		args = append(args, *filter.AccountID)
	}

	if filter.CategoryID != nil {
		query += " AND t.category_id = ?"
		args = append(args, *filter.CategoryID)
	}

	if filter.CategoryType != nil {
		query += " AND c.category_type = ?"
		args = append(args, *filter.CategoryType)
	}

	if filter.BatchID != nil {
		query += " AND t.import_batch_id = ?"
		args = append(args, *filter.BatchID)
	}

	if filter.Uncategorized {
		query += " AND (t.category_id IS NULL OR t.category_source = 0)"
	}

	if filter.CategoryIsNull {
		query += " AND t.category_id IS NULL"
	}

	if filter.TransactionType != nil {
		query += " AND t.transaction_type = ?"
		args = append(args, *filter.TransactionType)
	}

	if filter.StartDate != nil {
		query += " AND t.date_posted >= ?"
		args = append(args, *filter.StartDate)
	}

	if filter.EndDate != nil {
		query += " AND t.date_posted <= ?"
		args = append(args, *filter.EndDate)
	}

	// Order by date descending (most recent first)
	query += " ORDER BY t.date_posted DESC, t.transaction_id DESC"

	// Apply pagination
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query transactions: %w", err)
	}
	defer rows.Close()

	var transactions []*model.Transaction
	for rows.Next() {
		var txn model.Transaction
		err := rows.Scan(
			&txn.TransactionID,
			&txn.AccountID,
			&txn.ImportBatchID,
			&txn.TrnType,
			&txn.FitID,
			&txn.DatePosted,
			&txn.Amount,
			&txn.TransactionDetails,
			&txn.TransactionType,
			&txn.SICCode,
			&txn.CategoryID,
			&txn.CategorySource,
			&txn.CreatedAt,
			&txn.AccountName,
			&txn.CategoryName,
			&txn.CategoryColor,
			&txn.CategoryIcon,
			&txn.SICDescription,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, &txn)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transactions: %w", err)
	}

	slog.Info("fetching transactions: returning result", "total-transaction", len(transactions), "filter", filter)

	return transactions, nil
}

// UpdateCategory updates the category assignment for a transaction
func (r *TransactionRepository) UpdateCategory(transactionID int, categoryID *int, source model.CategorySource) error {
	query := `UPDATE ledger_transaction SET category_id = ?, category_source = ? WHERE transaction_id = ?`
	result, err := r.db.Exec(query, categoryID, source, transactionID)
	if err != nil {
		return fmt.Errorf("failed to update transaction category: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("transaction not found")
	}

	return nil
}

// GetUncategorized retrieves all transactions without a category (category_source = 0)
func (r *TransactionRepository) GetUncategorized() ([]*model.Transaction, error) {
	query := `
		SELECT transaction_id, account_id, trn_type, fit_id, date_posted, amount,
			transaction_details, transaction_type, sic_code, category_id, category_source, created_at
		FROM ledger_transaction
		WHERE category_source = 0
		ORDER BY date_posted DESC
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query uncategorized transactions: %w", err)
	}
	defer rows.Close()

	var transactions []*model.Transaction
	for rows.Next() {
		var txn model.Transaction
		err := rows.Scan(
			&txn.TransactionID,
			&txn.AccountID,
			&txn.TrnType,
			&txn.FitID,
			&txn.DatePosted,
			&txn.Amount,
			&txn.TransactionDetails,
			&txn.TransactionType,
			&txn.SICCode,
			&txn.CategoryID,
			&txn.CategorySource,
			&txn.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, &txn)
	}

	return transactions, nil
}

// GetUncategorizedBySICCodes returns genuinely uncategorized transactions
// matching any canonical SIC code. An empty input avoids issuing invalid SQL.
//
// Both a zero source and an absent category are required: a row can carry a
// category while its source reads none, and automatic categorization must
// never revise a category that is already there.
func (r *TransactionRepository) GetUncategorizedBySICCodes(sicCodes []string) ([]*model.Transaction, error) {
	if len(sicCodes) == 0 {
		return []*model.Transaction{}, nil
	}

	// The codes travel as a single JSON array parameter rather than one
	// placeholder each. SQLite accepts 32,766 bound variables, and a mapping
	// upload can affect more codes than that, so expanding placeholders would
	// fail on exactly the large merges this query exists to serve.
	//
	// Codes are encoded as JSON strings because sic_code is a TEXT column;
	// json_each over [7011] would yield an INTEGER whose comparison is decided
	// by type affinity, while ["7011"] yields TEXT and matches.
	encodedCodes, err := json.Marshal(sicCodes)
	if err != nil {
		return nil, fmt.Errorf("failed to encode SIC codes for lookup: %w", err)
	}

	query := `
		SELECT transaction_id, account_id, import_batch_id, trn_type, fit_id, date_posted, amount,
			transaction_details, transaction_type, sic_code, category_id, category_source, created_at
		FROM ledger_transaction
		WHERE category_source = 0
			AND category_id IS NULL
			AND sic_code IN (SELECT value FROM json_each(?))
		ORDER BY date_posted DESC
	`

	rows, err := r.db.Query(query, string(encodedCodes))
	if err != nil {
		return nil, fmt.Errorf("failed to query uncategorized transactions by SIC codes: %w", err)
	}
	defer rows.Close()

	transactions := make([]*model.Transaction, 0)
	for rows.Next() {
		var txn model.Transaction
		if err := rows.Scan(
			&txn.TransactionID,
			&txn.AccountID,
			&txn.ImportBatchID,
			&txn.TrnType,
			&txn.FitID,
			&txn.DatePosted,
			&txn.Amount,
			&txn.TransactionDetails,
			&txn.TransactionType,
			&txn.SICCode,
			&txn.CategoryID,
			&txn.CategorySource,
			&txn.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan transaction by SIC code: %w", err)
		}
		transactions = append(transactions, &txn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transactions by SIC codes: %w", err)
	}
	return transactions, nil
}

// CountUncategorized returns the number of uncategorized transactions
func (r *TransactionRepository) CountUncategorized() (int, error) {
	query := `SELECT COUNT(*) FROM ledger_transaction WHERE category_source = 0`

	var count int
	err := r.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count uncategorized transactions: %w", err)
	}

	return count, nil
}

// DeleteByBatchID deletes all transactions belonging to a given import batch and returns the count deleted.
func (r *TransactionRepository) DeleteByBatchID(batchID int) (int, error) {
	query := `DELETE FROM ledger_transaction WHERE import_batch_id = ?`
	result, err := r.db.Exec(query, batchID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete transactions for batch %d: %w", batchID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rows), nil
}

// Delete deletes a transaction by ID
func (r *TransactionRepository) Delete(transactionID int) error {
	query := `DELETE FROM ledger_transaction WHERE transaction_id = ?`
	result, err := r.db.Exec(query, transactionID)
	if err != nil {
		return fmt.Errorf("failed to delete transaction: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("transaction not found")
	}

	return nil
}

// BulkUpdateCategory assigns one category to a set of transactions and reports
// how many rows it actually changed.
//
// Manual rows are excluded in SQL, not by the caller. Re-examination decides
// from a materialized read and writes later, so a row can become manual in
// between; a predicate on IDs alone would then overwrite a choice the user made
// after the read. The guarantee that manual is never overwritten has to hold at
// write time to mean anything.
//
// The returned count is rows affected rather than len(transactionIDs), so a row
// excluded by that predicate is not reported as moved.
func (r *TransactionRepository) BulkUpdateCategory(categoryID int, source model.CategorySource, transactionIDs []int) (int, error) {
	if len(transactionIDs) == 0 {
		return 0, nil
	}

	// One JSON array parameter instead of one placeholder per ID. Beyond
	// SQLite's 32,766 variable ceiling the expanded form fails outright, and
	// chunking around it would need an explicit transaction to stay
	// all-or-nothing. A single statement is atomic by construction.
	//
	// IDs are encoded as JSON numbers to match the INTEGER column.
	encodedIDs, err := json.Marshal(transactionIDs)
	if err != nil {
		return 0, fmt.Errorf("failed to encode transaction IDs for update: %w", err)
	}

	query := `UPDATE ledger_transaction
		SET category_id = ?, category_source = ?
		WHERE transaction_id IN (SELECT value FROM json_each(?))
		  AND category_source != ?`

	res, err := r.db.Exec(query, categoryID, source, string(encodedIDs), model.CategorySourceManual)
	if err != nil {
		return 0, fmt.Errorf("failed to bulk update categories: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read updated row count: %w", err)
	}

	return int(affected), nil
}

// BulkClearCategory removes the category from a set of transactions in one
// statement, writing the same representation of "uncategorized" that category
// deletion already produces.
//
// This exists because BulkUpdateCategory takes a plain int and cannot express
// "no category", and because ClearCategory issues one round trip per row —
// fine for deleting a category, far too slow for a re-examination that may
// uncategorize thousands.
//
// Built exactly like BulkUpdateCategory: one JSON array parameter rather than
// one placeholder per ID, so the statement stays clear of SQLite's 32,766
// variable ceiling and is atomic by construction.
func (r *TransactionRepository) BulkClearCategory(transactionIDs []int) (int, error) {
	if len(transactionIDs) == 0 {
		return 0, nil
	}

	encodedIDs, err := json.Marshal(transactionIDs)
	if err != nil {
		return 0, fmt.Errorf("failed to encode transaction IDs for clear: %w", err)
	}

	// Excludes manual rows for the same reason as BulkUpdateCategory: removing
	// a category the user chose by hand is at least as damaging as replacing
	// it, so the predicate has to hold at write time rather than at read time.
	query := `UPDATE ledger_transaction
		SET category_id = NULL, category_source = ?
		WHERE transaction_id IN (SELECT value FROM json_each(?))
		  AND category_source != ?`

	res, err := r.db.Exec(query, model.CategorySourceNone, string(encodedIDs), model.CategorySourceManual)
	if err != nil {
		return 0, fmt.Errorf("failed to bulk clear categories: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read cleared row count: %w", err)
	}

	return int(affected), nil
}

// GetAllForReexamination returns every transaction, including manually
// categorized ones.
//
// Read scope and write scope differ here, which is deliberate (BR-U5-02 as
// amended). Re-examination writes only non-manual transactions, but it must
// read the manual ones too, because FR16 reports how many of them the current
// rules would otherwise have moved. Filtering them out in SQL would make that
// count unanswerable.
func (r *TransactionRepository) GetAllForReexamination() ([]*model.Transaction, error) {
	// Column set mirrors GetUncategorized exactly, minus its WHERE clause. The
	// deterministic order by transaction_id keeps a re-examination's batching
	// and its reported counts reproducible across runs.
	query := `
		SELECT transaction_id, account_id, trn_type, fit_id, date_posted, amount,
			transaction_details, transaction_type, sic_code, category_id, category_source, created_at
		FROM ledger_transaction
		ORDER BY transaction_id
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query transactions for re-examination: %w", err)
	}
	defer rows.Close()

	transactions := make([]*model.Transaction, 0)
	for rows.Next() {
		var txn model.Transaction
		if err := rows.Scan(
			&txn.TransactionID,
			&txn.AccountID,
			&txn.TrnType,
			&txn.FitID,
			&txn.DatePosted,
			&txn.Amount,
			&txn.TransactionDetails,
			&txn.TransactionType,
			&txn.SICCode,
			&txn.CategoryID,
			&txn.CategorySource,
			&txn.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan transaction for re-examination: %w", err)
		}
		transactions = append(transactions, &txn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transactions for re-examination: %w", err)
	}

	return transactions, nil
}

// GetMostRecentTransactionDate returns the date of the most recent transaction
func (r *TransactionRepository) GetMostRecentTransactionDate() (string, error) {
	query := `SELECT date_posted FROM ledger_transaction ORDER BY date_posted DESC LIMIT 1`

	var datePosted string
	err := r.db.QueryRow(query).Scan(&datePosted)
	if err == sql.ErrNoRows {
		return "", nil // No transactions
	}
	if err != nil {
		return "", fmt.Errorf("failed to get most recent transaction date: %w", err)
	}

	return datePosted, nil
}
