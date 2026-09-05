package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// buildLegacyPerfFixture creates the NFR-U1-PERF-02 reference database:
// 100,000 transactions, 100 categories, 1,000 patterns and 100 import batches
// on the pre-UOW-1 schema. Fixture construction is deliberately outside the
// measured window.
func buildLegacyPerfFixture(t *testing.T, txnCount, categoryCount, patternCount, batchCount int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy_perf.db")

	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys%281%29&_pragma=busy_timeout%285000%29")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(legacySchema(t)); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO account (account_id, name) VALUES (1, 'Perf')`); err != nil {
		t.Fatalf("account: %v", err)
	}

	catStmt, err := tx.Prepare(`INSERT INTO category (category_id, name, category_type) VALUES (?, ?, 2)`)
	if err != nil {
		t.Fatalf("prepare category: %v", err)
	}
	for i := 1; i <= categoryCount; i++ {
		if _, err := catStmt.Exec(i, fmt.Sprintf("Category %05d", i)); err != nil {
			t.Fatalf("insert category: %v", err)
		}
	}
	catStmt.Close()

	patStmt, err := tx.Prepare(`INSERT INTO category_pattern (pattern_name, category_id) VALUES (?, ?)`)
	if err != nil {
		t.Fatalf("prepare pattern: %v", err)
	}
	for i := 1; i <= patternCount; i++ {
		if _, err := patStmt.Exec(fmt.Sprintf("PATTERN %05d", i), (i%categoryCount)+1); err != nil {
			t.Fatalf("insert pattern: %v", err)
		}
	}
	patStmt.Close()

	batchStmt, err := tx.Prepare(`INSERT INTO import_batch (import_batch_id, file_name, account_id) VALUES (?, ?, 1)`)
	if err != nil {
		t.Fatalf("prepare batch: %v", err)
	}
	for i := 1; i <= batchCount; i++ {
		if _, err := batchStmt.Exec(i, fmt.Sprintf("statement-%04d.ofx", i)); err != nil {
			t.Fatalf("insert batch: %v", err)
		}
	}
	batchStmt.Close()

	txnStmt, err := tx.Prepare(`INSERT INTO ledger_transaction
		(account_id, import_batch_id, trn_type, fit_id, date_posted, amount,
		 transaction_details, transaction_type, category_id, category_source)
		VALUES (1, ?, 'DEBIT', ?, ?, ?, ?, 1, ?, ?)`)
	if err != nil {
		t.Fatalf("prepare transaction: %v", err)
	}
	base := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < txnCount; i++ {
		var categoryID any
		source := 0
		if i%3 != 0 {
			categoryID = (i % categoryCount) + 1
			source = 1 + (i % 2)
		}
		if _, err := txnStmt.Exec(
			(i%batchCount)+1,
			fmt.Sprintf("FIT-%08d", i),
			base.Add(time.Duration(i)*time.Minute).Format("2006-01-02 15:04:05"),
			-float64(i%9999)/100.0,
			fmt.Sprintf("MERCHANT %05d LOCATION %05d", i%997, i%89),
			categoryID,
			source,
		); err != nil {
			t.Fatalf("insert transaction %d: %v", i, err)
		}
	}
	txnStmt.Close()

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit fixture: %v", err)
	}
	return path
}

// TestPerformance_LegacyMigration measures NFR-U1-PERF-02. The measured window
// starts immediately before UOW-1 schema initialization and ends once the
// schema is ready for repository use.
func TestPerformance_LegacyMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("performance evidence is skipped in -short mode")
	}
	if raceDetectorEnabled {
		t.Skip("performance targets are acceptance evidence for an uninstrumented build; the race detector adds roughly an order of magnitude of overhead")
	}
	const (
		txnCount      = 100_000
		categoryCount = 100
		patternCount  = 1_000
		batchCount    = 100
		target        = 5 * time.Second
	)

	path := buildLegacyPerfFixture(t, txnCount, categoryCount, patternCount, batchCount)

	start := time.Now()
	db, err := Open(Config{Path: path})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Open() on the 100k-transaction legacy fixture: %v", err)
	}
	defer db.Close()

	// Data preservation is asserted alongside the timing so a fast-but-wrong
	// migration cannot pass the performance gate.
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ledger_transaction`).Scan(&got); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if got != txnCount {
		t.Fatalf("migration changed the transaction count: %d, want %d", got, txnCount)
	}
	if !hasColumn(t, db, "ledger_transaction", "sic_code") {
		t.Fatalf("migration did not add sic_code")
	}

	t.Logf("NFR-U1-PERF-02 first migration of a %d-transaction legacy database: %v (target %v)", txnCount, elapsed, target)
	if elapsed > target {
		t.Errorf("NFR-U1-PERF-02 FAILED: legacy migration took %v, target is %v", elapsed, target)
	}

	// A repeated startup is the common case and must also stay inside the target.
	db.Close()
	repeat := make([]time.Duration, 0, 5)
	for i := 0; i < 5; i++ {
		s := time.Now()
		again, err := Open(Config{Path: path})
		d := time.Since(s)
		if err != nil {
			t.Fatalf("repeat Open(): %v", err)
		}
		again.Close()
		repeat = append(repeat, d)
	}
	sort.Slice(repeat, func(i, j int) bool { return repeat[i] < repeat[j] })
	median := repeat[len(repeat)/2]
	t.Logf("NFR-U1-PERF-02 already-migrated startup median over %d runs: %v (target %v)", len(repeat), median, target)
	if median > target {
		t.Errorf("NFR-U1-PERF-02 FAILED: repeated startup median %v exceeds %v", median, target)
	}
}
