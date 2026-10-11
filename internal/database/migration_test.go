package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

// legacySchema is the verbatim pre-UOW-1 schema captured from the baseline
// revision. Building the fixture from it - rather than from the current
// schema.sql - is what makes the upgrade path (US-07 / BR-MIG-02) observable.
func legacySchema(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "legacy_schema_pre_uow1.sql"))
	if err != nil {
		t.Fatalf("read legacy schema fixture: %v", err)
	}
	return string(b)
}

// newLegacyDatabase creates a pre-UOW-1 database populated with realistic user
// data so migration data preservation can be asserted (BR-MIG-03 / NFR2).
func newLegacyDatabase(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "privateledger.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer raw.Close()

	if _, err := raw.Exec(legacySchema(t)); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}
	seed := []string{
		`INSERT INTO account (account_id, name) VALUES (1, 'Chequing'), (2, 'Visa')`,
		`INSERT INTO category (category_id, name, category_type, color, icon) VALUES
			(10, 'Groceries', 2, '#00FF00', 'cart'),
			(11, 'Salary', 3, NULL, NULL)`,
		`INSERT INTO category_pattern (category_pattern_id, pattern_name, category_id) VALUES
			(20, 'SUPERSTORE', 10), (21, 'PAYROLL', 11)`,
		`INSERT INTO import_batch (import_batch_id, file_name, account_id, imported_transactions, duplicate_transactions, total_auto_categorized)
			VALUES (30, 'jan.ofx', 1, 2, 0, 1)`,
		`INSERT INTO ledger_transaction
			(transaction_id, account_id, import_batch_id, trn_type, fit_id, date_posted, amount,
			 transaction_details, transaction_type, category_id, category_source)
		 VALUES
			(40, 1, 30, 'DEBIT', 'L-1', '2025-12-02 12:00:00', -12.34, 'SUPERSTORE #123', 1, 10, 1),
			(41, 1, 30, 'CREDIT', 'L-2', '2025-12-03 12:00:00', 500.00, 'PAYROLL DEPOSIT', 2, 11, 2),
			(42, 2, NULL, 'DEBIT', 'L-3', '2025-12-04 12:00:00', -5.00, 'UNKNOWN MERCHANT', 1, NULL, 0)`,
	}
	for _, stmt := range seed {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("seed legacy database: %v", err)
		}
	}
	return path
}

func columnNames(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info(%s): %v", table, err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info(%s): %v", table, err)
	}
	return names
}

func hasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	for _, name := range columnNames(t, db, table) {
		if name == column {
			return true
		}
	}
	return false
}

func objectExists(t *testing.T, db *sql.DB, objType, name string) bool {
	t.Helper()
	var found string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = ? AND name = ?", objType, name).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatalf("sqlite_master lookup %s %s: %v", objType, name, err)
	}
	return true
}

// TestMigrate_FreshDatabase covers BR-MIG-01: a brand new database receives the
// nullable transaction column, the sic_mapping table, and idx_txn_sic.
func TestMigrate_FreshDatabase(t *testing.T) {
	db, err := Open(Config{Path: filepath.Join(t.TempDir(), "fresh.db")})
	if err != nil {
		t.Fatalf("Open() on a fresh database: %v", err)
	}
	defer db.Close()

	if !hasColumn(t, db, "ledger_transaction", "sic_code") {
		t.Errorf("fresh schema is missing ledger_transaction.sic_code")
	}
	if !objectExists(t, db, "table", "sic_mapping") {
		t.Errorf("fresh schema is missing the sic_mapping table")
	}
	if !objectExists(t, db, "index", "idx_txn_sic") {
		t.Errorf("fresh schema is missing idx_txn_sic")
	}

	// BR-MIG-01: the transaction SIC column must be nullable.
	if _, err := db.Exec(`INSERT INTO account (account_id, name) VALUES (1, 'A')`); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO ledger_transaction
		(account_id, trn_type, fit_id, date_posted, amount, transaction_details, transaction_type)
		VALUES (1, 'DEBIT', 'F1', '2025-01-01', -1.00, 'x', 1)`); err != nil {
		t.Fatalf("insert transaction without SIC: %v", err)
	}
	var sic sql.NullString
	if err := db.QueryRow("SELECT sic_code FROM ledger_transaction").Scan(&sic); err != nil {
		t.Fatalf("read sic_code: %v", err)
	}
	if sic.Valid {
		t.Errorf("sic_code defaulted to %q, want SQL NULL", sic.String)
	}
}

// TestMigrate_LegacyDatabasePreservesData covers US-07, BR-MIG-02 and BR-MIG-03.
func TestMigrate_LegacyDatabasePreservesData(t *testing.T) {
	path := newLegacyDatabase(t)

	db, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("Open() on a legacy database: %v", err)
	}
	defer db.Close()

	if !hasColumn(t, db, "ledger_transaction", "sic_code") {
		t.Fatalf("legacy upgrade did not add ledger_transaction.sic_code")
	}
	if !objectExists(t, db, "table", "sic_mapping") {
		t.Errorf("legacy upgrade did not add the sic_mapping table")
	}
	if !objectExists(t, db, "index", "idx_txn_sic") {
		t.Errorf("legacy upgrade did not add idx_txn_sic")
	}

	counts := map[string]int{
		"account": 2, "category": 2, "category_pattern": 2, "import_batch": 1, "ledger_transaction": 3,
	}
	for table, want := range counts {
		var got int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Errorf("%s row count = %d, want %d (migration must preserve data)", table, got, want)
		}
	}

	// Field-level preservation, including the protected manual categorization.
	type txnRow struct {
		id       int
		details  string
		category sql.NullInt64
		source   int
		amount   float64
		sic      sql.NullString
	}
	rows, err := db.Query(`SELECT transaction_id, transaction_details, category_id, category_source, amount, sic_code
		FROM ledger_transaction ORDER BY transaction_id`)
	if err != nil {
		t.Fatalf("read transactions: %v", err)
	}
	defer rows.Close()

	var got []txnRow
	for rows.Next() {
		var r txnRow
		if err := rows.Scan(&r.id, &r.details, &r.category, &r.source, &r.amount, &r.sic); err != nil {
			t.Fatalf("scan transaction: %v", err)
		}
		got = append(got, r)
	}
	want := []txnRow{
		{id: 40, details: "SUPERSTORE #123", category: sql.NullInt64{Int64: 10, Valid: true}, source: 1, amount: -12.34},
		{id: 41, details: "PAYROLL DEPOSIT", category: sql.NullInt64{Int64: 11, Valid: true}, source: 2, amount: 500},
		{id: 42, details: "UNKNOWN MERCHANT", source: 0, amount: -5},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d transactions, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].id != want[i].id || got[i].details != want[i].details ||
			got[i].category != want[i].category || got[i].source != want[i].source ||
			got[i].amount != want[i].amount {
			t.Errorf("transaction %d changed during migration: got %+v want %+v", want[i].id, got[i], want[i])
		}
		if got[i].sic.Valid {
			t.Errorf("transaction %d gained a non-NULL sic_code %q during migration", want[i].id, got[i].sic.String)
		}
	}

	// The approved duplicate key must survive migration untouched (BR-TXN-01).
	if _, err := db.Exec(`INSERT INTO ledger_transaction
		(account_id, trn_type, fit_id, date_posted, amount, transaction_details, transaction_type, sic_code)
		VALUES (1, 'DEBIT', 'L-1', '2025-12-02 12:00:00', -99.00, 'dupe', 1, '5812')`); err == nil {
		t.Errorf("expected the (account_id, trn_type, fit_id, date_posted) unique key to reject a duplicate")
	} else if !strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
		t.Errorf("expected a UNIQUE constraint error, got %v", err)
	}
}

// TestMigrate_RepeatedIsIdempotent covers BR-MIG-04 / NFR-U1-REL-01: repeated
// startup must converge without duplicate-column/table/index errors.
func TestMigrate_RepeatedIsIdempotent(t *testing.T) {
	path := newLegacyDatabase(t)

	var lastColumns []string
	for run := 1; run <= 4; run++ {
		db, err := Open(Config{Path: path})
		if err != nil {
			t.Fatalf("Open() run %d: %v", run, err)
		}
		cols := columnNames(t, db, "ledger_transaction")
		if run > 1 && strings.Join(cols, ",") != strings.Join(lastColumns, ",") {
			t.Errorf("run %d changed ledger_transaction columns: %v -> %v", run, lastColumns, cols)
		}
		lastColumns = cols

		sicCount := 0
		for _, c := range cols {
			if c == "sic_code" {
				sicCount++
			}
		}
		if sicCount != 1 {
			t.Errorf("run %d: ledger_transaction has %d sic_code columns, want exactly 1", run, sicCount)
		}

		var txnCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM ledger_transaction").Scan(&txnCount); err != nil {
			t.Fatalf("run %d count: %v", run, err)
		}
		if txnCount != 3 {
			t.Errorf("run %d: transaction count = %d, want 3", run, txnCount)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("run %d close: %v", run, err)
		}
	}
}

// TestMigrate_AlreadyMigratedDatabaseIsUnchanged covers the third convergence
// case in NFR-U1-REL-01.
func TestMigrate_AlreadyMigratedDatabaseIsUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.db")
	first, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("first Open(): %v", err)
	}
	before := schemaSnapshot(t, first)
	first.Close()

	second, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("second Open(): %v", err)
	}
	defer second.Close()
	after := schemaSnapshot(t, second)

	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Errorf("re-running migration changed the schema:\nbefore:\n%s\nafter:\n%s",
			strings.Join(before, "\n"), strings.Join(after, "\n"))
	}
}

func schemaSnapshot(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query("SELECT type, name, IFNULL(sql, '') FROM sqlite_master")
	if err != nil {
		t.Fatalf("schema snapshot: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var typ, name, ddl string
		if err := rows.Scan(&typ, &name, &ddl); err != nil {
			t.Fatalf("scan schema snapshot: %v", err)
		}
		out = append(out, typ+"|"+name+"|"+strings.Join(strings.Fields(ddl), " "))
	}
	sort.Strings(out)
	return out
}

// TestMigrate_IndexIsCreatedAfterColumn pins BR-MIG-04's mandatory ordering:
// idx_txn_sic must not appear in a schema batch that runs before ensureColumn,
// otherwise a legacy database fails on CREATE INDEX before the column exists.
func TestMigrate_IndexIsCreatedAfterColumn(t *testing.T) {
	if strings.Contains(schemaSQL, "idx_txn_sic") {
		t.Errorf("schema.sql contains idx_txn_sic; the approved design requires it to be created only after ensureColumn")
	}
	// The legacy path is the actual proof: it must succeed end to end.
	db, err := Open(Config{Path: newLegacyDatabase(t)})
	if err != nil {
		t.Fatalf("legacy migration failed, which is the ordering symptom BR-MIG-04 forbids: %v", err)
	}
	defer db.Close()
	if !objectExists(t, db, "index", "idx_txn_sic") {
		t.Errorf("idx_txn_sic was not created after the column was ensured")
	}
}

// TestOpen_MigrationFailureIsFatalAndClosesDB covers BR-MIG-06 / NFR-U1-REL-02.
func TestOpen_MigrationFailureIsFatalAndClosesDB(t *testing.T) {
	t.Run("conflicting object blocks the SIC index", func(t *testing.T) {
		path := newLegacyDatabase(t)
		raw, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatalf("open raw: %v", err)
		}
		if _, err := raw.Exec("CREATE TABLE idx_txn_sic (x INTEGER)"); err != nil {
			t.Fatalf("inject conflicting object: %v", err)
		}
		raw.Close()

		db, err := Open(Config{Path: path})
		if err == nil {
			db.Close()
			t.Fatal("expected Open() to fail when a required migration step cannot complete")
		}
		if db != nil {
			t.Errorf("Open() returned a usable *sql.DB alongside an error; the connection must be closed")
		}
		if !strings.Contains(err.Error(), "migration") && !strings.Contains(err.Error(), "SIC index") {
			t.Errorf("expected a contextual migration error, got %q", err.Error())
		}
	})

	t.Run("unusable database file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "corrupt.db")
		if err := os.WriteFile(path, []byte("this is not a sqlite database at all"), 0o600); err != nil {
			t.Fatalf("write corrupt file: %v", err)
		}
		db, err := Open(Config{Path: path})
		if err == nil {
			db.Close()
			t.Fatal("expected Open() to fail on a file that is not a SQLite database")
		}
		if db != nil {
			t.Errorf("Open() returned a usable *sql.DB alongside an error")
		}
	})
}

// TestOpen_ConnectionSettingsApplyToEveryConnection covers NFRP-U1-01 and the
// project invariant that SQLite foreign keys must be enabled. The pre-UOW-1
// implementation ran PRAGMA foreign_keys on a single pooled connection, so this
// asserts the setting on several distinct physical connections.
func TestOpen_ConnectionSettingsApplyToEveryConnection(t *testing.T) {
	db, err := Open(Config{Path: filepath.Join(t.TempDir(), "pragmas.db")})
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)

	const connections = 6
	conns := make([]*sql.Conn, 0, connections)
	for i := 0; i < connections; i++ {
		c, err := db.Conn(context.Background())
		if err != nil {
			t.Fatalf("acquire connection %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	for i, c := range conns {
		var foreignKeys, busyTimeout int
		if err := c.QueryRowContext(context.Background(), "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
			t.Fatalf("connection %d foreign_keys: %v", i, err)
		}
		if err := c.QueryRowContext(context.Background(), "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
			t.Fatalf("connection %d busy_timeout: %v", i, err)
		}
		if foreignKeys != 1 {
			t.Errorf("connection %d has foreign_keys=%d, want 1", i, foreignKeys)
		}
		if busyTimeout != 5000 {
			t.Errorf("connection %d has busy_timeout=%d, want the approved 5000ms bound", i, busyTimeout)
		}
	}
	for _, c := range conns {
		c.Close()
	}
}

// TestOpen_ForeignKeysAreEnforced proves the setting has effect, not just a
// pragma readback (PROJECT_GUIDELINES database invariants, BR-MIG-05).
func TestOpen_ForeignKeysAreEnforced(t *testing.T) {
	db, err := Open(Config{Path: filepath.Join(t.TempDir(), "fk.db")})
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO sic_mapping (sic_code, category_id) VALUES ('5812', 9999)`); err == nil {
		t.Errorf("expected a foreign-key violation for a non-existent category")
	}

	if _, err := db.Exec(`INSERT INTO category (category_id, name, category_type) VALUES (5, 'Dining', 2)`); err != nil {
		t.Fatalf("insert category: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sic_mapping (sic_code, description, category_id) VALUES ('5812', 'Eating Places', 5)`); err != nil {
		t.Fatalf("insert mapping: %v", err)
	}

	// BR-MIG-05: deleting a category nulls the mapping reference and keeps the
	// mapping row as an intentionally unmapped code.
	if _, err := db.Exec(`DELETE FROM category WHERE category_id = 5`); err != nil {
		t.Fatalf("delete category: %v", err)
	}
	var count int
	var categoryID sql.NullInt64
	if err := db.QueryRow(`SELECT COUNT(*) FROM sic_mapping`).Scan(&count); err != nil {
		t.Fatalf("count mappings: %v", err)
	}
	if count != 1 {
		t.Fatalf("category deletion removed the SIC mapping; want the row preserved, got %d rows", count)
	}
	if err := db.QueryRow(`SELECT category_id FROM sic_mapping WHERE sic_code = '5812'`).Scan(&categoryID); err != nil {
		t.Fatalf("read mapping category: %v", err)
	}
	if categoryID.Valid {
		t.Errorf("category deletion left category_id = %d, want NULL (ON DELETE SET NULL)", categoryID.Int64)
	}
}

// TestSICMappingUniqueConstraint covers FR8 / BR-SIC-05 at the schema level,
// which the design names as the authoritative uniqueness boundary.
func TestSICMappingUniqueConstraint(t *testing.T) {
	db, err := Open(Config{Path: filepath.Join(t.TempDir(), "unique.db")})
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO sic_mapping (sic_code) VALUES ('5812')`); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sic_mapping (sic_code) VALUES ('5812')`); err == nil {
		t.Errorf("expected the unique sic_code constraint to reject a duplicate")
	}
	// Documented boundary: the UNIQUE index compares stored strings, so it does
	// not by itself collapse '0005812' and '5812'. BR-SIC-04 assigns that job to
	// the single domain normalization authority, which every write path must use
	// before reaching the repository (verified in internal/model and
	// internal/repository tests). This assertion pins the actual schema
	// behaviour so a future change of that division of labour is visible.
	if _, err := db.Exec(`INSERT INTO sic_mapping (sic_code) VALUES ('0005812')`); err != nil {
		t.Errorf("unexpected schema-level rejection of a non-canonical string: %v", err)
	}
}

// TestOpen_ConcurrentAccessUnderPooledConnections is the NFR-U1-TEST-04 /
// NFRP-U1-01 evidence. UOW-1 introduces no concurrent in-process state of its
// own, but it did change how connection-local pragmas are established, so this
// exercises the pool from several goroutines. It is meaningful under -race.
func TestOpen_ConcurrentAccessUnderPooledConnections(t *testing.T) {
	db, err := Open(Config{Path: filepath.Join(t.TempDir(), "concurrent.db")})
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO account (account_id, name) VALUES (1, 'A')`); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	const workers = 8
	const perWorker = 25
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if _, err := db.Exec(`INSERT INTO sic_mapping (sic_code, description) VALUES (?, ?)`,
					fmt.Sprintf("%d%03d", w+1, i), "concurrent"); err != nil {
					errCh <- fmt.Errorf("worker %d insert %d: %w", w, i, err)
					return
				}
				var n int
				if err := db.QueryRow(`SELECT COUNT(*) FROM sic_mapping`).Scan(&n); err != nil {
					errCh <- fmt.Errorf("worker %d count %d: %w", w, i, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent access failed (the 5s busy timeout should absorb normal contention): %v", err)
	}

	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sic_mapping`).Scan(&total); err != nil {
		t.Fatalf("final count: %v", err)
	}
	if total != workers*perWorker {
		t.Errorf("expected %d mappings, got %d", workers*perWorker, total)
	}
}
