# PrivateLedger - Design Document

> **PrivateLedger - Personal Expenditure Insight**

A local-only personal finance application that consolidates bank transactions from OFX files, providing categorization and spending insights while keeping all data on your machine.

---

## Table of Contents

1. [Overview](#overview)
2. [Tech Stack](#tech-stack)
3. [Project Structure](#project-structure)
4. [Database Schema](#database-schema)
5. [Configuration](#configuration)
6. [Core Features](#core-features)
7. [Data Flows](#data-flows)
8. [UI Pages](#ui-pages)
9. [API Routes](#api-routes)
10. [OFX Parsing Notes](#ofx-parsing-notes)
11. [Build & Distribution](#build--distribution)

---

## Overview

### Goals

- **Privacy-first**: All data stays local. No cloud, no APIs, no tracking.
- **Simple**: Single binary, minimal configuration.
- **Practical**: Designed for ~200 transactions/month across multiple accounts.

### Non-Goals

- Multi-user support
- Mobile app
- Bank API integrations (manual OFX download only)

---

## Tech Stack

| Component | Technology | Notes |
|-----------|------------|-------|
| Language | Go | Cross-platform, single binary |
| Web Framework | Gin | Familiar, performant |
| Database | SQLite via `modernc.org/sqlite` | Pure Go, no CGO, easy cross-compile |
| OFX Parser | `github.com/aclindsa/ofxgo` | Handles SGML quirks from Canadian banks |
| Frontend | Bootstrap 5 + HTMX | Simple, no build step, dynamic without SPA complexity |
| Assets | `go:embed` | HTML/CSS/JS embedded in binary |

---

## Project Structure

```
privateledger/
├── cmd/
│   └── server/
│       └── main.go                  # Entry point, wire up dependencies
│
├── internal/
│   ├── config/
│   │   └── config.go                # Load/save config.json
│   │
│   ├── database/
│   │   ├── db.go                    # SQLite connection, migrations
│   │   └── schema.sql               # Schema file (embedded)
│   │
│   ├── model/
│   │   ├── account.go
│   │   ├── transaction.go
│   │   ├── category.go
│   │   └── category_pattern.go
│   │
│   ├── repository/                  # Data access layer
│   │   ├── account_repo.go
│   │   ├── transaction_repo.go
│   │   ├── category_repo.go
│   │   └── category_pattern_repo.go
│   │
│   ├── service/                     # Business logic
│   │   ├── import_service.go        # OFX parsing, dedup, insert
│   │   ├── categorizer.go           # Pattern matching logic
│   │   └── insights_service.go      # Monthly aggregations, trends
│   │
│   ├── handler/                     # HTTP handlers (Gin)
│   │   ├── account_handler.go
│   │   ├── transaction_handler.go
│   │   ├── category_handler.go
│   │   ├── import_handler.go
│   │   └── insights_handler.go
│   │
│   └── parser/
│       └── ofx_parser.go            # Wrapper around ofxgo
│
├── web/                             # Embedded static assets
│   ├── templates/
│   │   ├── layout.html              # Base template (nav, footer)
│   │   ├── dashboard.html
│   │   ├── transactions.html
│   │   ├── accounts.html
│   │   ├── categories.html          # Category + pattern management
│   │   ├── import.html
│   │   └── partials/                # HTMX partials
│   │       ├── transaction_table.html
│   │       ├── category_form.html
│   │       └── ...
│   │
│   └── static/
│       ├── css/
│       │   └── app.css              # Custom styles (minimal)
│       └── js/
│           └── app.js               # Minimal JS (mostly HTMX handles it)
│
├── embed.go                         # go:embed directives
├── go.mod
├── go.sum
├── Makefile                         # Build targets
├── README.md
└── DESIGN.md                        # This file
```

---

## Database Schema

SQLite database file: `privateledger.db` (created beside binary)

```sql
-- User-defined account
CREATE TABLE account (
    account_id      INTEGER PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- All transactions (deduplicated)
CREATE TABLE transaction (
    transaction_id  INTEGER PRIMARY KEY,
    account_id      INTEGER NOT NULL REFERENCES account(account_id),
    
    -- OFX fields (composite key for dedup)
    trn_type        TEXT NOT NULL,
    fit_id          TEXT NOT NULL,
    date_posted     DATE NOT NULL,
    amount          DECIMAL(10,2) NOT NULL,
    
    -- Processed fields
    transaction_details TEXT,
    transaction_type    INTEGER NOT NULL,  -- 1=debit, 2=credit
    
    -- Categorization
    category_id     INTEGER REFERENCES category(category_id),
    category_source INTEGER DEFAULT 0,     -- 0=none, 1=rule, 2=manual
    
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(account_id, trn_type, fit_id, date_posted)
);

-- Category definitions
CREATE TABLE category (
    category_id     INTEGER PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    color           TEXT,
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Category patterns (one-to-many)
CREATE TABLE category_pattern (
    category_pattern_id INTEGER PRIMARY KEY,
    pattern_name        TEXT NOT NULL UNIQUE,
    category_id         INTEGER NOT NULL REFERENCES category(category_id),
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_txn_date ON transaction(date_posted);
CREATE INDEX idx_txn_category ON transaction(category_id);
CREATE INDEX idx_txn_account ON transaction(account_id);
```

### Schema Notes

- **Deduplication**: Composite unique constraint on `(account_id, trn_type, fit_id, date_posted)`
- **transaction_type**: Derived from `trn_type` and `amount` sign
  - `trn_type` = "DEBIT" → 1
  - `trn_type` = "CREDIT" → 2
  - Other types: positive amount → 2, negative amount → 1
- **category_source**: Tracks how category was assigned
  - 0 = none (uncategorized)
  - 1 = rule (matched by pattern)
  - 2 = manual (user override)
- **pattern_name**: Case-insensitive "contains" matching. Unique across all categories.

---

## Configuration

File: `config.json` (beside binary)

```json
{
  "version": 1,
  "server": {
    "port": 8080,
    "auto_open_browser": true
  },
  "start_of_month": 19
}
```

### Config Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | int | 1 | Config schema version for future migrations |
| `server.port` | int | 8080 | HTTP server port |
| `server.auto_open_browser` | bool | true | Open browser on startup |
| `start_of_month` | int | 1 | Day of month when "month" starts (1-28) |

### Custom Month Logic

When `start_of_month: 19`:

- "December 2025" = Dec 19, 2025 → Jan 18, 2026
- "January 2026" = Jan 19, 2026 → Feb 18, 2026

This allows aligning with pay cycles or billing periods.

---

## Core Features

### 1. Account Management

- User creates accounts manually (e.g., "TD Credit Card", "RBC Chequing")
- No auto-detection from OFX metadata
- User selects account when importing OFX file

### 2. OFX Import

- Supports OFX/SGML format (version 102) from Canadian banks
- Tested with: TD, RBC, CIBC
- Parses both bank accounts (`BANKMSGSRSV1`) and credit cards (`CREDITCARDMSGSRSV1`)
- Merges `NAME` and `MEMO` fields into `transaction_details`

### 3. Deduplication

On import, each transaction is checked against existing records using:
```
(account_id, trn_type, fit_id, date_posted)
```

- Match found → Skip (log as duplicate)
- No match → Insert

### 4. Categorization

**Pattern Matching (Rule-based)**:
- Case-insensitive "contains" match
- First matching pattern wins
- Patterns are unique across all categories

**Manual Override**:
- User can change category for any transaction
- Sets `category_source = 2` (manual)
- Manual assignments are never overwritten by rules

### 5. Insights

- Monthly spending by category (pie chart)
- Trends over time (line chart)
- Respects custom month boundaries

---

## Data Flows

### Import Flow

```
User selects OFX file
    │
    ▼
User selects target account from dropdown
    │
    ▼
Click "Import"
    │
    ▼
Backend: Parse OFX (ofxgo)
    │
    ▼
For each transaction:
    ├── Merge NAME + MEMO → transaction_details
    ├── Determine transaction_type (1=debit, 2=credit)
    ├── Build composite key
    ├── Check if exists in DB
    │       ├── Exists → Skip, count as duplicate
    │       └── New → Continue
    ├── Apply categorization rules
    │       ├── Match found → Set category_id, category_source=1
    │       └── No match → category_id=NULL, category_source=0
    └── Insert into DB
    │
    ▼
Return summary: {imported: 45, duplicates: 12, errors: 0}
```

### Categorization Flow

```
Load all patterns from category_pattern table
    │
    ▼
For each uncategorized transaction:
    │
    ▼
For each pattern (case-insensitive):
    ├── transaction_details CONTAINS pattern_name?
    │       ├── Yes → Set category, source=1, STOP
    │       └── No → Continue
    │
    ▼
No match → Remains uncategorized (category_source=0)
```

### Re-categorization

When user adds new patterns:
1. Find all transactions where `category_source = 0` (uncategorized)
2. Re-run categorization logic
3. Update matched transactions

Manual assignments (`category_source = 2`) are never touched.

---

## UI Pages

### Dashboard (`/`)

- Monthly summary (total income, total expenses, net)
- Category breakdown (pie chart)
- Spending trend (line chart, last 6 months)
- Quick stats (uncategorized count, accounts count)

**First-run behavior**: If no categories exist, redirect to `/categories` with welcome message.

### Transactions (`/transactions`)

- List view with columns: Date, Description, Amount, Category, Account
- Month selector (dropdown + prev/next arrows)
- Filter by account, category
- Click category to change (inline dropdown)
- Visual indicator for category_source (rule vs manual vs none)

### Accounts (`/accounts`)

- List of accounts
- Add new account (name only)
- Delete account (with confirmation, cascades to transactions)

### Categories (`/categories`)

- List of categories with color indicator
- Expand to see patterns
- Add category (name + color + initial patterns)
- Add pattern to existing category
- Delete pattern
- Delete category (set related transactions to uncategorized)

### Import (`/import`)

- File input for OFX
- Account dropdown
- Import button
- Results summary (imported, duplicates, errors)
- Link to view imported transactions

---

## API Routes

### Pages (HTML)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Dashboard |
| GET | `/transactions` | Transactions list |
| GET | `/accounts` | Accounts management |
| GET | `/categories` | Categories management |
| GET | `/import` | Import page |

### API (JSON/HTMX partials)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/accounts` | List accounts |
| POST | `/api/accounts` | Create account |
| DELETE | `/api/accounts/:id` | Delete account |
| GET | `/api/transactions` | List transactions (with filters) |
| PATCH | `/api/transactions/:id` | Update transaction (category) |
| GET | `/api/categories` | List categories with patterns |
| POST | `/api/categories` | Create category |
| DELETE | `/api/categories/:id` | Delete category |
| POST | `/api/categories/:id/patterns` | Add pattern |
| DELETE | `/api/patterns/:id` | Delete pattern |
| POST | `/api/import` | Import OFX file |
| GET | `/api/insights/monthly` | Monthly aggregation data |
| GET | `/api/insights/trends` | Trend data for charts |

### Query Parameters

**GET `/api/transactions`**:
- `month`: Month label (e.g., "2025-12")
- `account_id`: Filter by account
- `category_id`: Filter by category
- `uncategorized`: If "true", show only uncategorized

**GET `/api/insights/monthly`**:
- `month`: Month label (e.g., "2025-12")

**GET `/api/insights/trends`**:
- `months`: Number of months to include (default: 6)

---

## OFX Parsing Notes

### Tested Banks (Canada)

| Bank | Format | FITID Style | Has NAME | Has MEMO |
|------|--------|-------------|----------|----------|
| TD | Pretty-printed | Numeric (`25326171322410010`) | Yes | No |
| RBC | Single-line | Date+hash (`90000010020251118V003B93C0CB3`) | Yes | Yes |
| CIBC | Single-line | Long numeric (`25353353031407922341120000`) | Sometimes | Yes |

### Quirks Handled

- TD uses `<NAME>` tag (sometimes lowercase `<n>`)
- Date formats vary: `20251215020000[-5:EST]`, `20251215120000[-5]`, `20251219120000.000[-5:EST]`
- RBC MEMO contains location info
- CIBC MEMO contains payment context
- XML entities in content (e.g., `&amp;`)

### Transaction Type Derivation

```go
func deriveTransactionType(trnType string, amount float64) int {
    switch strings.ToUpper(trnType) {
    case "DEBIT":
        return 1
    case "CREDIT":
        return 2
    default:
        if amount >= 0 {
            return 2 // credit
        }
        return 1 // debit
    }
}
```

---

## Build & Distribution

### Dependencies

```
github.com/gin-gonic/gin
modernc.org/sqlite
github.com/aclindsa/ofxgo
```

### Makefile

```makefile
.PHONY: build build-all run clean

BINARY=privateledger
VERSION?=0.1.0

build:
	go build -o $(BINARY) ./cmd/server

build-all:
	GOOS=linux   GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64 ./cmd/server
	GOOS=linux   GOARCH=arm64 go build -o dist/$(BINARY)-linux-arm64 ./cmd/server
	GOOS=darwin  GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64 ./cmd/server
	GOOS=darwin  GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64 ./cmd/server
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe ./cmd/server

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
	rm -rf dist/
```

### Distribution Package

```
privateledger-v0.1.0-linux-amd64/
├── privateledger           # Binary
├── config.json             # Default config (created on first run if missing)
└── README.txt              # Quick start guide
```

On first run:
- Creates `config.json` if not exists (with defaults)
- Creates `privateledger.db` if not exists
- Opens browser to `http://localhost:8080`

---

## Future Considerations (Out of Scope for MVP)

- Export to CSV/PDF
- Budget setting per category
- Recurring transaction detection
- Multiple currency support
- Dark mode
- Data backup/restore

---

## Development Notes

### Getting Started

```bash
# Clone repo
git clone <repo>
cd privateledger

# Install dependencies
go mod tidy

# Run in development
go run ./cmd/server

# Build
make build

# Build all platforms
make build-all
```

### Testing OFX Parsing

Place test OFX files in `testdata/` directory. Files are not committed (add to .gitignore).

---

*Document version: 1.0*
*Last updated: December 2024*
