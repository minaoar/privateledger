# Application Design — Components

## Modified Components

### Transaction Model
**Location**: `internal/model/transaction.go`

**Purpose**: Represent imported bank transactions including optional SIC metadata.

**Responsibilities**:
- Add nullable `SICCode *string` field, always stored in normalized form.
- Add read-only joined display field `SICDescription *string` (populated via JOIN, not stored), following the existing `CategoryName`/`CategoryColor` convention.
- Own `NormalizeSICCode` as the single normalization entry point for parser, service, and repository callers.
- Preserve existing transaction categorization fields and deduplication semantics.
- Continue deriving debit/credit type as today.
- `NewTransaction` keeps its current positional signature; `SICCode` is assigned after construction rather than growing a 7th parameter.

### OFX Parser
**Location**: `internal/parser/ofx_parser.go`

**Purpose**: Extract SIC data from OFX/QFX transactions.

**Responsibilities**:
- Read `<SIC>` from `ofxgo.Transaction.SIC` when present/non-zero.
- Store SIC as a normalized string on `model.Transaction` via `model.NormalizeSICCode`.
- Preserve behavior for transactions without SIC.

**Constraints from `ofxgo` v0.1.3**:
- `ofxgo.Transaction.SIC` is typed `Int` (`int64`) and `Int.UnmarshalXML` uses `strconv.ParseInt`, so a non-numeric `<SIC>` fails the parse of the **entire file** — this is pre-existing behavior, not introduced here.
- Imported SIC values are therefore always numeric. Alphanumeric SIC support would require pre-parse extraction outside `ofxgo` and is out of scope.
- The field is `omitempty`, so an absent `<SIC>` and `<SIC>0` both arrive as `0` and are indistinguishable; both are treated as unset.

### Transaction Repository
**Location**: `internal/repository/transaction_repo.go`

**Purpose**: Persist and query transactions with optional SIC.

**Responsibilities**:
- Insert/select `sic_code`.
- Keep duplicate detection unchanged (`sic_code` never participates).
- `LEFT JOIN sic_mapping ON sic_mapping.sic_code = ledger_transaction.sic_code` on read paths (`GetByID`, `List`) to populate `SICDescription`, so categorization modals need no extra fetch per row.
- Add `GetUncategorizedBySICCodes(sicCodes []string)` to back SIC-scoped recategorization, as one `IN (...)` query over `idx_txn_sic`.
- Expose SIC to handlers/templates where required.

### Database Migration
**Location**: `internal/database/`

**Purpose**: Upgrade existing SQLite databases safely.

**Responsibilities**:
- Add `ledger_transaction.sic_code` to existing databases via `ensureColumn` — `schema.sql` uses `CREATE TABLE IF NOT EXISTS`, so a new column in the table definition alone would never reach an existing database.
- Add `sic_code` to the `ledger_transaction` definition in `schema.sql` so fresh databases get it directly.
- Create the `sic_mapping` table in `schema.sql` with `CREATE TABLE IF NOT EXISTS`; no `ensureColumn` needed for a wholly new table.
- Enforce `UNIQUE` on `sic_mapping.sic_code` at the schema level (FR8), rather than relying on an application-level check. No collation qualifier is needed: FR11 restricts SIC codes to digits, which are collation-invariant, so the earlier `COLLATE NOCASE` vs. binary decision no longer exists.
- Add `idx_txn_sic ON ledger_transaction(sic_code)` only after `ensureColumn` guarantees the column
  exists; do not execute this new index in a pre-column schema batch against legacy databases.
- Ensure migrations are idempotent.

## New Components

### SIC Mapping Model
**Location**: `internal/model/sic_mapping.go`

**Purpose**: Represent one SIC-to-category mapping.

**Fields**:
- `SICMappingID int`
- `SICCode string` — always normalized via `NormalizeSICCode`.
- `Description string`
- `DescriptionDetail string`
- `CategoryID *int` — nullable; `NULL` means intentionally no SIC category.
- joined display fields for category name/color/icon where useful.

**Normalization owner**: `model.NormalizeSICCode(string) string` lives beside this model and is the
only place SIC codes are trimmed/canonicalized. Parser, service validation, repository lookups, and
CSV import all call it, so an inserted code and a looked-up code can never diverge. Because FR11
restricts codes to digits, normalization trims whitespace and removes leading zeros — there is no
casing rule to keep in sync with the database collation. UOW-1 Functional Design restricts accepted
mapping values to the positive `int64` domain so every mapping code can match parser output.

### SIC Mapping Repository
**Location**: `internal/repository/sic_mapping_repo.go`

**Purpose**: SQLite persistence for SIC mappings.

**Responsibilities**:
- CRUD mappings.
- Rely on the schema `UNIQUE` constraint for global SIC uniqueness and translate the constraint violation into a domain error.
- Support nullable category IDs.
- Merge uploaded mappings atomically inside a single `database/sql` transaction (upsert each validated normalized code, commit or rollback); never delete omitted mappings.
- Query by normalized SIC code for categorization.

**Note**: transactions store `sic_code` as a value, not a foreign key to `sic_mapping`, so
Merging mappings cannot orphan or corrupt transaction rows.

### Categorizer Extension Point
**Location**: `internal/service/categorizer.go`

**Purpose**: Keep categorization orchestration focused while allowing SIC matching as an extension.

**Responsibilities**:
- Continue loading and applying text patterns first.
- Delegate SIC matching to `SICMappingCategorizer` only when no text pattern matches.
- If SIC mapping has non-empty category, assign rule category.
- If SIC mapping has empty category, leave transaction uncategorized by SIC.
- Preserve manual categorizations.
- Coordinate recategorization at a high level without owning CSV/import/export workflows.
- Guard the pattern cache with `sync.RWMutex`: reloads are triggered from HTTP handlers (including `category_handler.go:342`, which reloads in a bare goroutine today) while imports read the cache concurrently.
- Refactor `RecategorizeAll` and `RecategorizeByCategory` to call `Categorize` per transaction instead of re-implementing pattern matching inline (`categorizer.go:88-99`). Without this, SIC mappings would apply on import but be silently skipped by the Categories page "Recategorize All" action, contradicting FR6 — and every future rule type would have to be added in three places.

### SIC Mapping Categorizer
**Location**: `internal/service/sic_mapping_categorizer.go`

**Purpose**: SIC-specific categorization extension used by the main `Categorizer`.

**Responsibilities**:
- Load/cache SIC mappings from `SICMappingRepository`.
- Match transactions by `Transaction.SICCode`.
- Return a categorized result, an empty-category/no-op result, or no match.
- Keep SIC matching logic isolated from text pattern matching.
- Guard the mapping cache with `sync.RWMutex`; `LoadMappings` is called from mapping CRUD and upload handlers while `Match` runs inside an in-flight import.

### SIC Mapping Service
**Location**: `internal/service/sic_mapping_service.go`

**Purpose**: Own SIC mapping management workflows outside the main categorization algorithm.

**Responsibilities**:
- Create/update/delete SIC mappings.
- Validate SIC mapping data (normalize, reject empty, reject non-digit characters per FR11, enforce max length).
- Validate, import, export, and merge `sic_mappings.csv`.
- Attempt a backup before merge as `sic_mappings.backup-<timestamp>.csv` in the application data directory. Return its path on success; on failure continue the merge and return a prominent warning with no path.
- After a confirmed CSV merge, reload mappings, identify created or category-changed non-empty mappings, and re-categorize via `Categorizer.RecategorizeBySICCodes(affected)`. Omitted mappings are unchanged and explicit deletion is the only removal path.
- Import the optional startup mapping file **only when the mapping table is empty** (`SICMappingRepository.Count() == 0`). An unconditional upsert-on-boot would silently revert every mapping the user edited in the UI the next time the app restarted, since the file on disk is stale by then.
- Handle modal-driven SIC mapping creation/update.
- Trigger rule reload/recategorization through `Categorizer` or `SICMappingCategorizer` as needed.

**Upload shape**: validation and merge/upsert happen in a **single request**. An earlier draft used a
`ValidateCSV → previewID → ReplaceFromPreview` handshake, which would have introduced server-side
ephemeral state (a preview map needing a TTL, eviction, and leak handling) into an application that
has no session store and exactly one local user. The browser confirms before POSTing instead; the
server still validates the whole file before mutating anything.

### SIC Mapping Handler
**Location**: `internal/handler/sic_mapping_handler.go`

**Purpose**: HTTP API for SIC mapping management.

**Responsibilities**:
- Expose CRUD endpoints.
- Expose download endpoint returning CSV.
- Expose a single upload endpoint that validates the whole file, attempts a backup, and atomically merges it.
- Depend on `SICMappingService` only. The service already performs the category joins the handler needs, so a direct repository dependency alongside it would give the same data two owners.
- Coordinate with `SICMappingService` for mapping changes, CSV workflows, and recategorization side effects.

### SIC Mapping Page
**Location**: `cmd/privateledger/web/templates/sic_mappings.html`

**Purpose**: User-facing mapping configuration view.

**Responsibilities**:
- List mappings with SIC code, description, detail, category/empty state.
- Create/update/delete mappings.
- Show all current categories as mapping targets.
- Provide download CSV action.
- Provide an "Import / Update Mappings" CSV action: confirm in the browser, then one POST that validates, attempts a backup, and merges; render created/updated/unchanged counts plus the backup path or warning.

### Transaction Categorization Modal Enhancements
**Location**: `cmd/privateledger/web/templates/transactions.html`

**Purpose**: Expose SIC context in existing transaction categorization workflows.

**Responsibilities**:
- Show SIC code and display description in Change Category and Create Categorization Pattern modals. The description originates on the **mapping**, not the transaction, and arrives as the `SICDescription` joined display field already present on each server-rendered row (`page_handler.go:157` renders the full list), so no per-modal fetch is required.
- Prompt to create/update SIC mapping when category is selected and transaction has SIC.
- In Change Category modal, if user agrees, update current transaction using SIC rule source.
- In Create Categorization Pattern modal, if user agrees, create/update SIC mapping only; do not create text pattern.
- Do not allow modal-created empty-category mappings.
- Send a description along with the mapping upsert (defaulting to the transaction details) so modal-created mappings do not display as bare SIC codes forever.

### Page Handler Extension
**Location**: `internal/handler/page_handler.go`

**Purpose**: Render SIC mapping page.

**Responsibilities**:
- Add page route linked from Categories page.
- Obtain the mappings and category list for the initial render from `SICMappingService.GetPageData()`, **not** by reading `SICMappingRepository` and `CategoryRepository` directly. NFR3 mandates handler → service → repository → SQLite, and a page handler reaching into two repositories to assemble a joined view breaks it.
- This follows an existing precedent rather than introducing a mixed style: `PageHandler` already takes `insightsService` for the dashboard alongside its repositories (`page_handler.go:18-46`), so the SIC page is wired the same way the dashboard is.
- `SICMappingService` is the natural owner because it already resolves categories for CSV validation, so the page and the CSV path share one definition of "a mapping plus its category".
- Existing page routes keep their current direct repository reads; migrating them is out of scope for this feature.

### Main Wiring
**Location**: `cmd/privateledger/main.go`

**Purpose**: Dependency injection and routes.

**Responsibilities**:
- Initialize SIC mapping repository.
- Inject repository into Categorizer.
- Initialize SIC mapping handler.
- Add API and page routes.
- Trigger optional startup mapping CSV import from application data directory through `SICMappingService`.

## Mapping File Format Component Boundary

The SIC mapping file is named `sic_mappings.csv`. It is CSV and contains these columns:

- `SIC_Code` — required, non-empty.
- `Description` — optional display description.
- `Description_Detail` — optional fallback display description.
- `Category_Name` — optional; the only way a category is resolved on import.
- `Category_ID` — optional confirmation of `Category_Name`; never a reference on its own. Omitted/empty together with `Category_Name` means intentionally no SIC category.

**Category reference resolution at this boundary**:

| `Category_Name` | `Category_ID` | Outcome |
|---|---|---|
| empty | empty | Accepted — mapping intentionally has no category (`category_id = NULL`) |
| set | empty | Accepted — resolved by name |
| set | set, agrees with name | Accepted — resolved by name; ID confirmed |
| set | set, disagrees with name | **Rejected** — ambiguous reference |
| empty | set | **Rejected** — ID-only references are unsafe (FR4): a category deleted and recreated leaves a stale ID that still resolves to a valid but *wrong* category, so the row would import silently mis-mapped |
| set | set, name not found | **Rejected** — never fall back to the ID alone |

A row carrying `Category_ID` with no `Category_Name` is **rejected**, not resolved by ID. This is the
enforcement of FR4's "resolving by ID alone is unsafe" — without an explicit rejection the unsafe path
remains reachable on every upload. Enforced in `SICMappingService.ValidateCSV`.

There is **no mapping-ID column** — normalized SIC code is the stable interchange identity and
internal primary keys buy nothing in an interchange file.

**Why not `Category_ID` alone**: if the user deletes and recreates
categories between download and upload, a stale `Category_ID` still resolves to a valid but *wrong*
category, so validation passes and every mapping lands on the wrong category silently. Resolving by
`Category_Name` first, with `Category_ID` as a tiebreaker and rejecting rows where the two disagree,
turns that silent corruption into a loud validation error. Cross-database portability improves as a
side effect but is not the motivation.
