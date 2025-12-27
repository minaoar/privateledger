package database

import (
	"database/sql"
	_ "embed"
	"fmt"

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
	db, err := sql.Open("sqlite", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys (SQLite disables by default)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Run migrations (create tables if not exist)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// migrate executes the embedded schema SQL
func migrate(db *sql.DB) error {
	_, err := db.Exec(schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
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
