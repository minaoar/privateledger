# Application Design — Component Dependencies

## Dependency Matrix

| Component | Depends On | Used By |
|---|---|---|
| Transaction Model | none | Parser, TransactionRepository, Categorizer |
| SIC Mapping Model | Category model by reference | SICMappingRepository, SICMappingCategorizer, SICMappingService, SICMappingHandler (as request/response DTO only) |
| OFX Parser | Transaction Model, ofxgo | ImportService |
| TransactionRepository | SQLite, Transaction Model, `sic_mapping` table (read-only LEFT JOIN for display) | ImportService, Categorizer, SICMappingService, Handlers |
| SICMappingRepository | SQLite, SIC Mapping Model | SICMappingCategorizer, SICMappingService |
| CategoryRepository | SQLite, Category Model | SICMappingService, PageHandler (existing pages) |
| CategoryPatternRepository | SQLite, CategoryPattern Model | Categorizer |
| SICMappingCategorizer | SICMappingRepository | Categorizer, SICMappingService |
| Categorizer | PatternRepo, TransactionRepo, SICMappingCategorizer | ImportService, CategoryHandler, SICMappingService |
| SICMappingService | SICMappingRepository, CategoryRepository, TransactionRepository, Categorizer, SICMappingCategorizer | SICMappingHandler, main startup |
| SICMappingHandler | SICMappingService | Gin routes |
| PageHandler | AccountRepository, TransactionRepository, CategoryRepository, CategoryPatternRepository, InsightsService (existing pages); SICMappingService (SIC mapping page) | HTML page routes |
| Templates | API routes, page data | Browser |
| main.go | all constructors | Application startup |

## Architectural Direction

The feature preserves existing layering:

```text
Browser / Templates
  -> Handler
  -> Service (Categorizer, SICMappingCategorizer, SICMappingService)
  -> Repository
  -> SQLite
```

No external service or cloud dependency is introduced.

## Import Data Flow

```text
OFX/QFX file
  -> OFXParser.convertOFXTransaction
  -> model.Transaction{SICCode}
  -> ImportService
  -> Categorizer.Categorize
       -> CategoryPatternRepository cache (RWMutex-guarded)
       -> SICMappingCategorizer.Match
       -> SICMappingRepository-backed cache (RWMutex-guarded)
  -> TransactionRepository.Create
  -> SQLite ledger_transaction.sic_code
```

## SIC Mapping Page Flow

```text
GET /sic-mappings
  -> PageHandler.SICMappings
  -> SICMappingService.GetPageData          (NFR3: handler -> service -> repository)
       -> SICMappingRepository.GetAll
       -> CategoryRepository.GetAll
  -> sic_mappings.html

Browser API operations
  -> /api/sic-mappings...
  -> SICMappingHandler
  -> SICMappingService
  -> SICMappingRepository
  -> SQLite sic_mapping table
```

## Upload/Download Flow

```text
Download:
SICMappingHandler.DownloadMappings
  -> SICMappingService.ExportCSV
  -> SICMappingRepository.GetAll
  -> HTTP attachment

Upload (single request):
SICMappingHandler.UploadMappings
  -> SICMappingService.MergeFromCSV
       -> ValidateCSV (CategoryRepository resolves Category_Name, Category_ID tiebreaker)
       -> abort with per-row report if invalid; nothing mutated
       -> ExportCSV -> best-effort sic_mappings.backup-<timestamp>.csv on disk
       -> SICMappingRepository.MergeAll (single SQL transaction; omitted codes retained)
       -> SICMappingCategorizer.LoadMappings
       -> diff pre/post mappings -> affected SIC codes
       -> Categorizer.RecategorizeBySICCodes(affected) (FR14; uncategorized only)
  -> JSON summary + recategorized count + backup path or warnings
```

## Startup Flow

```text
main.go
  -> database.Open/migrate
  -> repositories
  -> SICMappingCategorizer
  -> Categorizer (injected with SICMappingCategorizer)
  -> SICMappingService
  -> SICMappingService.ImportFileIfPresent(execDir/sic_mappings.csv)   # gated on Count() == 0
  -> Categorizer.LoadRules                                            # after import, so caches are warm
  -> routes
```

Wiring order note: `ImportFileIfPresent` runs **before** `LoadRules` so the rule caches are loaded
once, already reflecting any startup import. There is no dependency cycle —
`Categorizer -> SICMappingCategorizer` and `SICMappingService -> {Categorizer, SICMappingCategorizer}`
form a DAG, so `SICMappingCategorizer` is constructed first, then `Categorizer`, then
`SICMappingService`.

For the UOW-2 checkpoint, `SICMappingService` receives a local no-op implementation of the narrow
SIC recategorization collaborator; it returns zero and performs no transaction work. UOW-3 replaces
that constructor argument with the real `Categorizer` adapter. Independent UOW-2 tests inject a fake
to verify affected-code selection without pulling UOW-3 behavior into this unit.

## Modal Flow

```text
transactions.html modal
  -> transaction row/modal receives SICCode and display description
  -> user selected category + SIC prompt
  -> /api/transactions/:id/sic-mapping
  -> SICMappingHandler.UpsertFromTransaction
  -> SICMappingService.CreateOrUpdateFromTransaction
  -> SICMappingRepository upsert
  -> SICMappingCategorizer.LoadMappings
  -> TransactionRepository.UpdateCategory(rule source) when Change Category modal path requires it
```

## Dependency Constraints

- Repositories must not call services.
- Handlers should not directly implement CSV parsing or categorization logic.
- `SICMappingHandler` depends on `SICMappingService` only, not on repositories alongside it — the service already performs the category joins the handler needs.
- `PageHandler.SICMappings` reads through `SICMappingService.GetPageData`, not through `SICMappingRepository`/`CategoryRepository` directly. Assembling a joined view inside a handler would violate NFR3; the dashboard's existing `insightsService` dependency is the precedent.
- Mutable rule caches (`Categorizer.patterns`, `SICMappingCategorizer` mappings) must be `sync.RWMutex`-guarded: reloads originate from HTTP handlers while imports read them.
- All SIC code writes and lookups pass through `model.NormalizeSICCode`; no ad-hoc trimming anywhere else. SIC codes are digits-only (FR11), so no casing or collation rule is involved.
- `sic_mapping.sic_code` uniqueness is enforced by a schema `UNIQUE` constraint, not application checks alone.
- `Categorizer` owns categorization orchestration only.
- `SICMappingCategorizer` owns SIC matching.
- `SICMappingService` owns SIC CRUD, CSV import/export, upload merge/upsert, startup import, and modal-created mapping workflows.
- Empty-category mappings must use nullable `category_id`; do not create sentinel categories.
- Mapping file category references resolve by `Category_Name` only; `Category_ID` merely confirms it. Rows where the two disagree are rejected, and a row with `Category_ID` but no `Category_Name` is rejected rather than resolved by ID (FR4).
- Transactions reference SIC by value (`sic_code`), never by foreign key to `sic_mapping`, so merge/upsert and explicit deletion cannot orphan transaction rows.
