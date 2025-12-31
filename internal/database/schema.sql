-- PrivateLedger Database Schema
-- SQLite schema for local-only personal finance tracking

-- User-defined account
CREATE TABLE IF NOT EXISTS account (
    account_id      INTEGER PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Import batch tracking
CREATE TABLE IF NOT EXISTS import_batch (
    import_batch_id         INTEGER PRIMARY KEY,
    file_name               TEXT NOT NULL,
    account_id              INTEGER NOT NULL REFERENCES account(account_id) ON DELETE CASCADE,
    created_at              DATETIME DEFAULT CURRENT_TIMESTAMP,
    imported_transactions   INTEGER,
    duplicate_transactions  INTEGER,
    total_auto_categorized  INTEGER
);

-- All transactions (deduplicated) - used 'ledger_transaction' to avoid SQL reserved keyword 'transaction'
CREATE TABLE IF NOT EXISTS ledger_transaction (
    transaction_id  INTEGER PRIMARY KEY,
    account_id      INTEGER NOT NULL REFERENCES account(account_id) ON DELETE CASCADE,
    import_batch_id INTEGER REFERENCES import_batch(import_batch_id) ON DELETE SET NULL,

    -- OFX fields (composite key for dedup)
    trn_type        TEXT NOT NULL,
    fit_id          TEXT NOT NULL,
    date_posted     DATE NOT NULL,
    amount          DECIMAL(10,2) NOT NULL,

    -- Processed fields
    transaction_details TEXT,
    transaction_type    INTEGER NOT NULL,  -- 1=debit, 2=credit

    -- Categorization
    category_id     INTEGER REFERENCES category(category_id) ON DELETE SET NULL,
    category_source INTEGER DEFAULT 0,     -- 0=none, 1=rule, 2=manual

    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(account_id, trn_type, fit_id, date_posted)
);

-- Category definitions
CREATE TABLE IF NOT EXISTS category (
    category_id     INTEGER PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    category_type   TEXT NOT NULL DEFAULT 'General',  -- General/Expense/Income
    color           TEXT,
    icon  TEXT,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Category patterns (one-to-many)
CREATE TABLE IF NOT EXISTS category_pattern (
    category_pattern_id INTEGER PRIMARY KEY,
    pattern_name        TEXT NOT NULL UNIQUE,
    category_id         INTEGER NOT NULL REFERENCES category(category_id) ON DELETE CASCADE,
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for query performance
CREATE INDEX IF NOT EXISTS idx_txn_date ON ledger_transaction(date_posted);
CREATE INDEX IF NOT EXISTS idx_txn_category ON ledger_transaction(category_id);
CREATE INDEX IF NOT EXISTS idx_txn_account ON ledger_transaction(account_id);
CREATE INDEX IF NOT EXISTS idx_txn_batch ON ledger_transaction(import_batch_id);
CREATE INDEX IF NOT EXISTS idx_pattern_category ON category_pattern(category_id);
CREATE INDEX IF NOT EXISTS idx_batch_account ON import_batch(account_id);
