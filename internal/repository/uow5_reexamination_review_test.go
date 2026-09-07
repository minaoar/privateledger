package repository

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
)

func TestReviewU5BulkClearCategoryBeyondSQLiteVariableLimit(t *testing.T) {
	db, err := database.Open(database.Config{Path: filepath.Join(t.TempDir(), "uow5-bulk-clear.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO account (name) VALUES ('Bulk clear')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO category (name, category_type) VALUES ('Assigned', 2)`); err != nil {
		t.Fatal(err)
	}
	const count = 40_000
	if _, err := db.Exec(`
		WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < ?)
		INSERT INTO ledger_transaction
			(account_id, trn_type, fit_id, date_posted, amount, transaction_details,
			 transaction_type, category_id, category_source)
		SELECT 1, 'DEBIT', printf('clear-%06d', x), '2026-01-01 00:00:00', -1,
			'MERCHANT', 1, 1, 1 FROM n`, count); err != nil {
		t.Fatal(err)
	}
	ids := make([]int, count)
	for i := range ids {
		ids[i] = i + 1
	}
	repo := NewTransactionRepository(db)
	if err := repo.BulkClearCategory(nil); err != nil {
		t.Fatalf("empty clear: %v", err)
	}
	if err := repo.BulkClearCategory(ids); err != nil {
		t.Fatalf("bulk clear beyond variable limit: %v", err)
	}
	var cleared, wrongSource int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ledger_transaction WHERE category_id IS NULL`).Scan(&cleared); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM ledger_transaction WHERE category_source != ?`, model.CategorySourceNone).Scan(&wrongSource); err != nil {
		t.Fatal(err)
	}
	if cleared != count || wrongSource != 0 {
		t.Fatalf("cleared=%d wrong_source=%d, want %d/0", cleared, wrongSource, count)
	}
	t.Logf("BulkClearCategory cleared %s rows with one bound JSON parameter", fmt.Sprint(count))
}
