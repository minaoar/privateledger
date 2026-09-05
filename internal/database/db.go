package database

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Config holds database configuration
type Config struct {
	Path string // Path to SQLite database file
}

// Open opens a connection to the SQLite database and runs migrations
func Open(cfg Config) (*sql.DB, error) {
	// Open database connection
	db, err := sql.Open("sqlite", sqliteDSN(cfg.Path))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Run migrations (create tables if not exist)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// sqliteDSN configures connection-local pragmas through the driver DSN so
// every physical connection opened by database/sql gets the same settings.
func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=foreign_keys%281%29&_pragma=busy_timeout%285000%29"
}

// migrate executes the embedded schema SQL
func migrate(db *sql.DB) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}
	if err := ensureColumn(
		db,
		"ledger_transaction",
		"sic_code",
		"ALTER TABLE ledger_transaction ADD COLUMN sic_code TEXT",
	); err != nil {
		return fmt.Errorf("failed to ensure transaction SIC column: %w", err)
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_txn_sic ON ledger_transaction(sic_code)"); err != nil {
		return fmt.Errorf("failed to create transaction SIC index: %w", err)
	}
	return nil
}

// ensureColumn performs an additive, idempotent migration. Its identifiers and
// ALTER statement are internal constants, never user-provided SQL values.
func ensureColumn(db *sql.DB, tableName, columnName, alterSQL string) error {
	rows, err := db.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		return fmt.Errorf("failed to inspect table %s: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("failed to inspect column in %s: %w", tableName, err)
		}
		if name == columnName {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed while inspecting table %s: %w", tableName, err)
	}
	if _, err := db.Exec(alterSQL); err != nil {
		return fmt.Errorf("failed to add column %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

// Close closes the database connection
func Close(db *sql.DB) error {
	if db != nil {
		return db.Close()
	}
	return nil
}
