package repository

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oronno/privateledger/internal/model"
)

// TestReviewU3SICSetPassingBeyondVariableLimit covers the real 50,000-code
// UOW-2 affected set, JSON TEXT affinity, and retained date ordering.
func TestReviewU3SICSetPassingBeyondVariableLimit(t *testing.T) {
	db := newTestDB(t)
	repo := NewTransactionRepository(db)
	accountID := newTestAccount(t, db, "Set passing")

	create := func(fitID, code string, posted time.Time) int {
		t.Helper()
		txn := &model.Transaction{
			AccountID: accountID, TrnType: "DEBIT", FitID: fitID,
			DatePosted: posted, Amount: -1, TransactionDetails: fitID,
			TransactionType: model.TransactionTypeDebit, SICCode: sicPtr(code),
			CategorySource: model.CategorySourceNone,
		}
		if err := repo.Create(txn); err != nil {
			t.Fatalf("create %s: %v", fitID, err)
		}
		return txn.TransactionID
	}
	older := create("older", "1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	newer := create("newer", "50000", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	create("outside", "50001", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))

	codes := make([]string, 50_000)
	for i := range codes {
		codes[i] = fmt.Sprint(i + 1)
	}
	got, err := repo.GetUncategorizedBySICCodes(codes)
	if err != nil {
		t.Fatalf("GetUncategorizedBySICCodes(50,000): %v", err)
	}
	if len(got) != 2 || got[0].TransactionID != newer || got[1].TransactionID != older {
		t.Fatalf("ordered matches=%v, want newer %d then older %d", transactionReviewIDs(got), newer, older)
	}
}

func transactionReviewIDs(txns []*model.Transaction) []int {
	ids := make([]int, len(txns))
	for i, txn := range txns {
		ids[i] = txn.TransactionID
	}
	return ids
}

// TestReviewU3BulkUpdateBeyondVariableLimit crosses the former 32,764-ID
// ceiling in one call and verifies the entire update committed.
func TestReviewU3BulkUpdateBeyondVariableLimit(t *testing.T) {
	db := newTestDB(t)
	repo := NewTransactionRepository(db)
	accountID := newTestAccount(t, db, "Bulk set")
	categoryID := newTestCategory(t, db, "Bulk category")

	const count = 32_765
	if _, err := db.Exec(`
		WITH RECURSIVE n(x) AS (
			VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < ?
		)
		INSERT INTO ledger_transaction
			(account_id, trn_type, fit_id, date_posted, amount, transaction_details, transaction_type, category_source)
		SELECT ?, 'DEBIT', printf('bulk-%05d', x), '2026-01-01 00:00:00', -1, 'BULK', 1, 0 FROM n`, count, accountID); err != nil {
		t.Fatalf("create %d transactions: %v", count, err)
	}
	ids := make([]int, count)
	for i := range ids {
		ids[i] = i + 1
	}
	changed, err := repo.BulkUpdateCategory(categoryID, model.CategorySourceRule, ids)
	if err != nil {
		t.Fatalf("BulkUpdateCategory(%d): %v", count, err)
	}
	if changed != count {
		t.Fatalf("BulkUpdateCategory changed=%d, want %d", changed, count)
	}
	var updated int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ledger_transaction WHERE category_id = ? AND category_source = 1`, categoryID).Scan(&updated); err != nil {
		t.Fatal(err)
	}
	if updated != count {
		t.Fatalf("updated=%d, want %d", updated, count)
	}
}

// Closing the database makes any SQL attempt fail, so successful empty calls
// demonstrate that both repository methods return before issuing a query.
func TestReviewU3EmptySetsIssueNoSQL(t *testing.T) {
	db := newTestDB(t)
	repo := NewTransactionRepository(db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetUncategorizedBySICCodes(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty SIC set touched closed DB: got=%v err=%v", got, err)
	}
	changed, err := repo.BulkUpdateCategory(1, model.CategorySourceRule, nil)
	if err != nil {
		t.Fatalf("empty ID set touched closed DB: %v", err)
	}
	if changed != 0 {
		t.Fatalf("empty ID set changed=%d, want 0", changed)
	}
}

// TestReviewU3SICJSONQueryUsesIndex inspects the production query shape with
// EXPLAIN QUERY PLAN. The scalar-list virtual table may be scanned; the ledger
// table itself must be searched through idx_txn_sic.
func TestReviewU3SICJSONQueryUsesIndex(t *testing.T) {
	db := newTestDB(t)
	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT transaction_id, account_id, import_batch_id, trn_type, fit_id, date_posted, amount,
			transaction_details, transaction_type, sic_code, category_id, category_source, created_at
		FROM ledger_transaction
		WHERE category_source = 0
			AND sic_code IN (SELECT value FROM json_each(?))
		ORDER BY date_posted DESC`, `["5812","7011"]`)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(details, " | ")
	if !strings.Contains(plan, "SEARCH ledger_transaction USING INDEX idx_txn_sic") {
		t.Fatalf("ledger lookup did not use idx_txn_sic; plan: %s", plan)
	}
	t.Logf("EXPLAIN QUERY PLAN: %s", plan)
}
