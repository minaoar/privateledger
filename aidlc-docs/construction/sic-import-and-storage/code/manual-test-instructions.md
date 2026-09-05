# Manual Test Instructions — UOW-1 SIC Import and Storage

## Purpose

This guide manually verifies UOW-1 startup migration, SIC extraction and persistence, startup CSV seeding, compatibility, and error behavior.

Manual testing supplements the independent automated review gate. UOW-1 is not complete until the separate-provider independent review reports `PASS`, all required automated tests and performance checks pass, and no Blocking/High finding remains.

## Safety Rules

- Never run these checks against the user's real `privateledger.db`, `config.json`, logs, or financial exports.
- Use only sanitized OFX/QFX inputs.
- Run the application from a temporary directory because it stores `config.json` and `privateledger.db` beside its executable.
- Keep the repository's development server stopped if it already uses the selected test port.

## Prerequisites

- Go and `make` are installed.
- The `sqlite3` command-line tool is installed.
- A sanitized pre-UOW-1 database is available for the legacy migration scenario.
- Sanitized OFX/QFX fixtures are available for present, absent, and zero SIC scenarios.

## 1. Build and Prepare an Isolated Application

From the repository root:

```bash
make build
manual_test_dir=$(mktemp -d)
cp ./privateledger "$manual_test_dir/privateledger"
cd "$manual_test_dir"
```

The application creates a default `config.json` on first startup. To avoid opening a browser and reduce the chance of a port collision, start it once, stop it with `Ctrl+C`, and change the generated configuration to an unused local port with `auto_open_browser` set to `false`.

Record the isolated path so it can be inspected and removed after testing:

```bash
pwd
```

## 2. Verify Fresh-Database Initialization

Start the isolated application:

```bash
./privateledger
```

After startup succeeds, stop it with `Ctrl+C`, then inspect the generated database:

```bash
sqlite3 privateledger.db "PRAGMA table_info(ledger_transaction);"
sqlite3 privateledger.db ".schema sic_mapping"
sqlite3 privateledger.db ".schema idx_txn_sic"
```

Verify:

- `ledger_transaction` contains nullable `sic_code`.
- `sic_mapping` exists.
- `sic_mapping.sic_code` is unique.
- `sic_mapping.category_id` is nullable and references `category(category_id)` with `ON DELETE SET NULL`.
- `idx_txn_sic` exists on `ledger_transaction(sic_code)`.

Start and stop the application at least two more times. There must be no duplicate-column, table, or index errors.

## 3. Verify Existing-Database Migration

Create a new isolated directory and copy the application plus a sanitized pre-UOW-1 database into it. Do not use the real database.

Before starting the new binary, capture row counts:

```bash
sqlite3 privateledger.db "
SELECT 'account', COUNT(*) FROM account;
SELECT 'transaction', COUNT(*) FROM ledger_transaction;
SELECT 'category', COUNT(*) FROM category;
SELECT 'pattern', COUNT(*) FROM category_pattern;
SELECT 'batch', COUNT(*) FROM import_batch;
"
```

Start and stop the application, repeat the count query, and inspect the new schema:

```bash
sqlite3 privateledger.db "PRAGMA table_info(ledger_transaction);"
sqlite3 privateledger.db ".schema sic_mapping"
sqlite3 privateledger.db ".schema idx_txn_sic"
```

Verify:

- All recorded row counts and existing records remain unchanged.
- The nullable transaction SIC column, mapping table, and SIC index were added.
- Repeated startups succeed without further schema errors or data changes.

## 4. Verify OFX/QFX SIC Import

Use sanitized files containing otherwise valid transactions for these cases:

1. A positive `<SIC>` value.
2. No `<SIC>` element.
3. `<SIC>0`.

Import each file through the existing application import page. Then inspect recently inserted transactions:

```bash
sqlite3 -header -column privateledger.db \
  "SELECT transaction_id, fit_id, sic_code
   FROM ledger_transaction
   ORDER BY transaction_id DESC;"
```

Verify:

- A positive SIC is stored as canonical decimal text.
- A source value with leading zeros is stored without leading zeros because `ofxgo` exposes it numerically.
- Missing and zero SIC values become SQL `NULL`.
- Transaction type, amount, details, and existing import counts behave as before.

Import the same fixture again and verify it is still detected as a duplicate. SIC must not change the duplicate key:

```text
account_id, trn_type, fit_id, date_posted
```

## 5. Prepare Categories for Startup Seed Tests

Create test categories through the existing Categories page. Record their IDs and exact names:

```bash
sqlite3 -header -column privateledger.db \
  "SELECT category_id, name FROM category ORDER BY category_id;"
```

Use only names and IDs from this isolated database in the following CSV examples.

## 6. Verify a Valid Startup Seed

Stop the application and ensure the mapping table is empty:

```bash
sqlite3 privateledger.db "DELETE FROM sic_mapping;"
```

Create `sic_mappings.csv` beside the isolated executable with the exact header:

```csv
SIC_Code,Description,Description_Detail,Category_Name,Category_ID
0111,Agriculture,Farm services,Food,1
5411,Grocery Stores,,Food,1
9999,Unmapped Example,,,
```

Replace `Food` and `1` with a matching category name and ID from the isolated database. Start and stop the application, then inspect the mappings:

```bash
sqlite3 -header -column privateledger.db \
  "SELECT sic_code, description, description_detail, category_id
   FROM sic_mapping
   ORDER BY sic_code;"
```

Verify:

- `0111` is stored canonically as `111`.
- Descriptions are preserved.
- Name and ID resolve to the expected category.
- Empty category fields produce SQL `NULL`.
- Every valid row is inserted together.

Restart with the same file and verify rows are neither duplicated nor overwritten because a non-empty SQLite mapping table is authoritative.

## 7. Verify Invalid-Seed Atomicity

For each scenario, stop the application, empty `sic_mapping`, replace the CSV content, start the application, and query:

```bash
sqlite3 privateledger.db "SELECT COUNT(*) FROM sic_mapping;"
```

Create a CSV containing at least one valid row and one invalid row. Test invalid cases such as:

- Nonnumeric SIC.
- SIC `0`.
- SIC above `9223372036854775807`.
- Canonical duplicates such as `0111` and `111`.
- Category ID without a category name.
- Unknown category name.
- Name and ID that resolve to different categories.
- Multiple stored category names differing only by case, referenced with a non-exact ambiguous casing.
- Missing or incorrect CSV header fields.

Verify for every invalid file:

- The mapping count remains `0`; a valid row from the same file is not partially imported.
- Application startup continues.
- Logs identify safe row numbers and validation reasons without dumping complete CSV rows or financial data.

## 8. Verify Other Startup Outcomes

### Missing file

Remove or rename `sic_mappings.csv`, then start the application. Startup must succeed without treating the absence as an error.

### Non-empty mapping table

Keep at least one database mapping and provide a different valid CSV. Start the application and verify the existing mapping is unchanged and the file is skipped with an informational log.

### Oversized file

Provide a seed whose opened target is larger than 10 MiB. Startup must continue, import no rows, and log a safe size rejection without parsing the file.

### Symbolic link

Place a valid CSV elsewhere in the temporary test area and create `sic_mappings.csv` as a symbolic link to it. With an empty mapping table, startup should follow the link for read-only import, enforce the size limit on the target, and import valid mappings.

## 9. Verify SQLite Busy Timeout

Using a second `sqlite3` session against the isolated database, begin an exclusive write transaction and leave it open:

```sql
BEGIN EXCLUSIVE;
```

While the lock is held, start the application or trigger the isolated startup seed persistence path. Verify it waits only for the configured interval and fails with a contextual SQLite busy/locked error after approximately five seconds rather than hanging indefinitely.

Release the test lock:

```sql
ROLLBACK;
```

Then restart the application and verify normal startup succeeds.

## 10. Record Results

Record at least:

- Tested application commit.
- OS and version.
- Go version.
- SQLite CLI version.
- Each scenario as `PASS` or `FAIL`.
- Relevant safe log excerpts.
- Any unexpected database mutation.

Suggested checklist:

| Scenario | Result | Notes |
|---|---|---|
| Fresh schema |  |  |
| Repeated fresh startup |  |  |
| Legacy migration and data preservation |  |  |
| Repeated legacy startup |  |  |
| Positive SIC import |  |  |
| Missing/zero SIC import |  |  |
| Duplicate import unchanged |  |  |
| Valid startup seed |  |  |
| Empty-category seed |  |  |
| Invalid seed atomicity |  |  |
| Missing seed |  |  |
| Non-empty-table skip |  |  |
| Oversized seed |  |  |
| Symlink seed |  |  |
| Five-second busy timeout |  |  |

## Cleanup

Stop the isolated application and remove the temporary test directory only after confirming the resolved path is the directory created specifically for this test. Prefer moving it to the operating system trash if recovery may be useful.
