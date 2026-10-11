# Application Design — Component Methods

## Model Constructors / Helpers

### `model.NewSICMapping(sicCode, description, descriptionDetail string, categoryID *int) *SICMapping`
Creates a SIC mapping model after caller-level validation.

### `(*SICMapping) HasCategory() bool`
Returns true when `CategoryID != nil`.

### `(*SICMapping) DisplayDescription() string`
Returns `Description` when non-empty, otherwise `DescriptionDetail`, otherwise empty string.

### `model.NormalizeSICCode(raw string) string`
Single normalization owner for SIC codes: trims whitespace and removes leading zeros so mapping
inputs match the minimal decimal string produced from `ofxgo`'s `int64` SIC value. FR11 restricts SIC
codes to digits, so there is no casing rule and nothing to keep in sync with the
`sic_mapping.sic_code` collation. Called by the parser, service validation, repository lookups, and
CSV import so a stored code and a looked-up code can never diverge. Functional Design further fixes
the accepted domain at positive `int64` (`1..9223372036854775807`); zero and overflow are rejected by
service validation because neither can match an imported transaction.

## Repository Methods

### `TransactionRepository.Create(txn *model.Transaction) error`
Modified to insert `sic_code`.

### `TransactionRepository.GetByID(transactionID int) (*model.Transaction, error)`
Modified to select `sic_code` and `LEFT JOIN sic_mapping` for `SICDescription`, matching `List`. Both
read paths must populate the joined field or the categorization modals lose their description
depending on which path served the row.

### `TransactionRepository.List(filter TransactionFilter) ([]*model.Transaction, error)`
Modified to select `sic_code` and `LEFT JOIN sic_mapping` for `SICDescription`.

### `TransactionRepository.GetUncategorizedBySICCodes(sicCodes []string) ([]*model.Transaction, error)`
New. Returns currently uncategorized transactions whose normalized `sic_code` is in the given set,
backing `Categorizer.RecategorizeBySICCodes`. One `IN (...)` query over `idx_txn_sic` — not one query
per code, and not a full uncategorized scan filtered in memory. An empty input returns no rows without
querying.

### `SICMappingRepository.Create(mapping *model.SICMapping) error`
Creates one mapping with nullable category.

### `SICMappingRepository.GetByID(id int) (*model.SICMapping, error)`
Loads one mapping.

### `SICMappingRepository.GetByCode(sicCode string) (*model.SICMapping, error)`
Loads one mapping by normalized SIC code.

### `SICMappingRepository.GetAll() ([]*model.SICMapping, error)`
Lists mappings, joined with category display fields.

### `SICMappingRepository.Update(mapping *model.SICMapping) error`
Updates SIC code, descriptions, and nullable category.

### `SICMappingRepository.Delete(id int) error`
Deletes one mapping.

### `SICMappingRepository.MergeAll(mappings []*model.SICMapping) (*SICMappingMergeCounts, error)`
Atomically inserts new normalized SIC codes and updates matching codes inside one `database/sql`
transaction. Codes omitted from the input remain unchanged; a header-only input is a no-op.

### `SICMappingRepository.Count() (int, error)`
Returns mapping count. Its specific purpose is gating the startup CSV import: the import runs only
when this returns `0`.

## Categorizer Methods

### `NewCategorizer(patternRepo *CategoryPatternRepository, txnRepo *TransactionRepository, sicCategorizer *SICMappingCategorizer) *Categorizer`
Constructor extended with a SIC categorization extension, not SIC management workflows.

### `(*Categorizer) loadPatterns() error`
Existing `LoadPatterns` becomes unexported. Keeping both an exported `LoadPatterns` and an exported
`LoadRules` leaves callers a coin flip — existing call sites in `category_handler.go` would keep
reloading patterns only, which is correct today but silently wrong the moment another rule type is
added.

### `(*Categorizer) LoadRules() error`
The single exported reload entry point. Loads text patterns and asks `SICMappingCategorizer` to load
SIC mappings. All existing `LoadPatterns()` call sites in `category_handler.go` migrate to this.

**Concurrency**: the pattern cache is guarded by `sync.RWMutex` — `LoadRules` takes the write lock,
`Categorize` takes the read lock. `category_handler.go:342` currently calls
`go h.categorizer.LoadPatterns()`, which races with in-flight imports today; that call site is fixed
as part of this work.

### `(*Categorizer) Categorize(txn *model.Transaction) bool`
Modified behavior:
1. Skip manual categorization.
2. Apply text pattern first.
3. Delegate SIC matching to `SICMappingCategorizer` only if no pattern matched.
4. Assign category only if SIC match has non-empty category.
5. Leave uncategorized if SIC mapping has empty category.

### `(*Categorizer) RecategorizeAll() (*RecategorizeResult, error)`
**Modified.** Currently re-implements pattern matching inline over `c.patterns`
(`categorizer.go:88-99`), which would skip SIC mappings entirely. Refactored to call
`Categorize(txn)` per transaction and bulk-update from the resulting `txn.CategoryID`, so text-first
then-SIC priority is applied identically on import and on bulk recategorization (FR6).

### `(*Categorizer) RecategorizeByCategory(categoryID int) (*RecategorizeResult, error)`
**Modified.** Same refactor: matches via `Categorize` and keeps only transactions that resolved to
`categoryID`, instead of duplicating the matching loop.

### `(*Categorizer) RecategorizeBySICCodes(sicCodes []string) (*RecategorizeResult, error)`
Recategorizes currently uncategorized transactions matching any of the given SIC codes, sourcing them
from `TransactionRepository.GetUncategorizedBySICCodes` and routing each through `Categorize` so
text-pattern-first priority still applies. This is the FR14 sweep: it touches only transactions
matching the codes that were actually created or updated, never the whole uncategorized set.

### `(*Categorizer) RecategorizeBySICCode(sicCode string) (*RecategorizeResult, error)`
Single-code convenience wrapper delegating to `RecategorizeBySICCodes` — one implementation, used by
mapping CRUD and the modal upsert path where exactly one code changes.

## SIC Mapping Categorizer Methods

### `NewSICMappingCategorizer(sicRepo *SICMappingRepository) *SICMappingCategorizer`
Creates the SIC-specific categorization extension.

### `(*SICMappingCategorizer) LoadMappings() error`
Loads SIC mappings into an internal cache keyed by normalized SIC code. Guarded by `sync.RWMutex`:
called from mapping CRUD and upload handlers while `Match` runs inside an in-flight import.

### `(*SICMappingCategorizer) Match(txn *model.Transaction) SICMatchResult`
Returns no match, mapped category, or empty-category/no-op result for a transaction.

### `(*SICMappingCategorizer) MatchByCode(sicCode string) SICMatchResult`
Matches a normalized SIC code directly.

## SIC Mapping Service Methods

### `NewSICMappingService(sicRepo *SICMappingRepository, categoryRepo *CategoryRepository, txnRepo *TransactionRepository, categorizer *Categorizer, sicCategorizer *SICMappingCategorizer) *SICMappingService`
Creates the service that owns SIC mapping management workflows.

### `(*SICMappingService) UpsertMapping(mapping *model.SICMapping) (*model.SICMapping, error)`
Creates or updates mapping, reloads SIC mappings, and triggers eligible recategorization.

### `(*SICMappingService) DeleteMapping(id int) error`
Deletes one mapping and reloads SIC mappings.

### `(*SICMappingService) ExportCSV() ([]byte, error)`
Returns `sic_mappings.csv` bytes for download/backup.

### `(*SICMappingService) ValidateCSV(r io.Reader) ([]*model.SICMapping, *SICMappingImportReport, error)`
Pure parse + validate: normalizes SIC codes, enforces the digits-only rule (FR11), resolves the
category reference, and returns the parsed mappings plus a per-row report. Mutates nothing. No preview
token, no server-side state.

**Category reference resolution — exhaustive rules**:

| `Category_Name` | `Category_ID` | Outcome |
|---|---|---|
| empty | empty | Accepted — mapping intentionally has no category (`category_id = NULL`) |
| set | empty | Accepted — resolved by name |
| set | set, agrees with name | Accepted — resolved by name; ID confirmed |
| set | set, disagrees with name | **Rejected** — ambiguous reference |
| empty | set | **Rejected** — ID-only references are unsafe (FR4): a category deleted and recreated leaves a stale ID that still resolves to a valid but *wrong* category, so the row would import silently mis-mapped |
| set | set, name not found | **Rejected** — never fall back to the ID alone |

The `Category_Name`-empty/`Category_ID`-set row is the one that matters most and is easy to get wrong:
it must be an explicit **rejection**, not a silent fallback to ID lookup. Without it, FR4's
"resolving a category by ID alone is unsafe" is stated but never enforced, and the unsafe path stays
reachable through every upload.

A single rejected row fails the whole upload (nothing is mutated) — the exact reporting format is
deferred to Functional Design.

### `(*SICMappingService) MergeFromCSV(r io.Reader) (*SICMappingImportResult, error)`
One-request merge: calls `ValidateCSV`, aborts on any validation failure, attempts a backup via
`ExportCSV`, then calls `SICMappingRepository.MergeAll`. It reloads SIC mappings, derives affected
codes from created or category-changed non-empty mappings, invokes the injected SIC-scoped
recategorization collaborator, and returns created/updated/unchanged counts, recategorized count,
and either the backup path or a prominent backup warning.

The pre-upload mappings are already in hand from export/backup preparation, so the diff costs no extra
query. `RecategorizeAll` must **not** be used here: it would also categorize transactions via text
patterns and via SIC mappings this upload never touched, which is outside FR14's scope. See
`services.md` → Affected-code diff for the exact affected/not-affected rules. Replaces the earlier `ReplaceFromPreview(previewID)`, which required a server-side preview
store (TTL, eviction, leak-on-abandon) that this application has no session infrastructure for.

### `(*SICMappingService) ImportFileIfPresent(path string) error`
Startup hook. Returns nil when the file is absent. When present, imports **only if
`SICMappingRepository.Count() == 0`**; otherwise logs that existing mappings take precedence and
skips. Without this gate, a stale file would overwrite every UI edit on the next restart, which
contradicts "user edits after import live in SQLite" (FR4).

### `(*SICMappingService) CreateOrUpdateFromTransaction(transactionID int, categoryID int, description string, applyToCurrent bool) error`
Creates/updates a SIC mapping from the modal workflow and optionally applies rule categorization to
the current transaction. `description` (defaulting to the transaction details) is carried through so
modal-created mappings are not permanently description-less in the mapping page and modals.

## Handler Methods

### `SICMappingHandler.ListMappings(c *gin.Context)`
`GET /api/sic-mappings`

### `SICMappingHandler.CreateMapping(c *gin.Context)`
`POST /api/sic-mappings`

### `SICMappingHandler.UpdateMapping(c *gin.Context)`
`PUT /api/sic-mappings/:id`

### `SICMappingHandler.DeleteMapping(c *gin.Context)`
`DELETE /api/sic-mappings/:id`

### `SICMappingHandler.DownloadMappings(c *gin.Context)`
`GET /api/sic-mappings/download`

### `SICMappingHandler.UploadMappings(c *gin.Context)`
`POST /api/sic-mappings/upload`

Single-step: validate the entire file, attempt a backup, atomically merge/upsert, and reload. Returns a JSON summary
(created/updated/unchanged counts, rejected rows, recategorized count, backup path or warning). The browser confirms with the user before POSTing; the
server never holds partial upload state. Matches the endpoint list in FR13.

### `SICMappingHandler.UpsertFromTransaction(c *gin.Context)`
`POST /api/transactions/:id/sic-mapping`

Uses `SICMappingService`, not `Categorizer`, for mapping management.

## Page Methods

### `SICMappingService.GetPageData() (*SICMappingPageData, error)`
Returns everything the SIC mapping page needs for its initial render: all mappings (with joined
category display fields) and all categories as selectable targets. Exists so `PageHandler` does not
assemble a joined view out of two repositories, which NFR3 forbids. Categories are read at request
time, so categories created or updated after startup appear without a restart (FR9).

### `PageHandler.SICMappings(c *gin.Context)`
Renders `sic_mappings.html` from a single `SICMappingService.GetPageData()` call. Mirrors how
`PageHandler.Dashboard` already delegates to `insightsService` rather than querying repositories.

## Migration Methods

### `database.migrate(db *sql.DB) error`
Extended to run idempotent table creation (including the new `sic_mapping` table), then the
column-add migration for `ledger_transaction.sic_code`, and only then
`CREATE INDEX IF NOT EXISTS idx_txn_sic`. Order matters: placing the SIC index in the schema batch
that precedes `ensureColumn` would fail on a legacy database because the indexed column would not
exist yet. Fresh and existing databases both converge on the same final schema.

### `database.ensureColumn(db *sql.DB, tableName, columnName, alterSQL string) error`
Helper to add missing columns safely, checking `PRAGMA table_info(<table>)` before issuing
`ALTER TABLE`. Deliberately no `schema_version` table: one additive column does not justify a
versioned migration framework in a single-binary local application, and `ensureColumn` stays
idempotent under repeated startups (NFR4).
