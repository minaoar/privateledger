# Application Design — Services

## Categorization Service Split

The design separates categorization from SIC mapping management:

- `Categorizer` remains the high-level transaction categorization orchestrator.
- `SICMappingCategorizer` is a SIC-specific extension used by `Categorizer` after text patterns fail.
- `SICMappingService` owns SIC mapping CRUD, CSV import/export, upload overwrite, startup file import, and modal-driven mapping workflows.

### Categorizer Responsibilities

- Load text patterns.
- Call `SICMappingCategorizer.LoadMappings()` as part of `LoadRules()`.
- Apply text-pattern-first priority.
- Delegate SIC matching to `SICMappingCategorizer`.
- Re-categorize eligible transactions after rule changes, routing `RecategorizeAll` and
  `RecategorizeByCategory` through `Categorize` so bulk paths honour the same priority as import.
- Guard the pattern cache with `sync.RWMutex` (reloads come from HTTP handlers, reads from imports).

### SICMappingCategorizer Responsibilities

- Load/cache SIC mappings, keyed by normalized SIC code, under a `sync.RWMutex`.
- Match transactions by SIC code.
- Distinguish mapped-category, empty-category/no-op, and no-match outcomes.

### SICMappingService Responsibilities

- Validate and persist SIC mappings (normalize via `model.NormalizeSICCode`, reject empty, reject non-digit characters per FR11, cap length).
- Export and import SIC mapping CSV files.
- Import optional startup `sic_mappings.csv`, only when the mapping table is empty.
- Coordinate validation, backup, and overwrite within a single upload request.
- Support modal-driven SIC mapping creation/update.
- Serve the SIC mapping page's initial render data via `GetPageData`, so `PageHandler` never assembles a joined view from repositories itself (NFR3).
- Trigger SIC mapping reload and eligible recategorization through the categorization components.

## Import-Time Categorization Flow

```text
ImportService parses transaction
  -> Categorizer.Categorize(txn)
       -> skip if manual source
       -> check text patterns
       -> if no pattern, delegate to SICMappingCategorizer
       -> if SIC mapping has category, assign rule category
       -> if SIC mapping has NULL category, leave uncategorized
  -> TransactionRepository.Create(txn)
```

## Dedicated SIC Mapping Page Flow

```text
GET /sic-mappings
  -> PageHandler.SICMappings
  -> SICMappingService.GetPageData   (mappings + categories; NFR3 layering)
  -> renders page

CRUD actions
  -> SICMappingHandler
  -> SICMappingService.Upsert/Delete mapping orchestration
  -> SICMappingRepository persistence
  -> SICMappingCategorizer.LoadMappings
  -> optional recategorization of currently uncategorized transactions
```

### Recategorization Scope After Mapping Changes

| Trigger | Scope |
|---|---|
| Mapping created/updated (page or modal) | `Categorizer.RecategorizeBySICCode` over `TransactionRepository.GetUncategorizedBySICCode` — uncategorized only |
| Mapping deleted | reload cache only; existing assignments untouched (FR14) |
| CSV upload overwrite | reload cache, then `Categorizer.RecategorizeBySICCodes(affected)` where `affected` is the pre/post mapping diff — uncategorized only. FR14 covers "created or updated from the dedicated SIC mapping page **or file upload**", but scopes the sweep to transactions matching the SIC codes the upload actually created or updated |
| Categories page "Recategorize All" | `Categorizer.RecategorizeAll`, now text-pattern-first then SIC, uncategorized only |
| Startup file import | load mappings only; **no** automatic sweep. FR14 scopes re-categorization to page/upload/modal triggers, and a startup sweep would silently rewrite categories on every boot of a fresh install. A user with pre-existing transactions applies them via the Categories page "Recategorize All" action. |

Transactions with `category_source = 2` (manual) are excluded from every path above, because all
paths read from the uncategorized set.

## Mapping File Download Flow

```text
User clicks Download
  -> GET /api/sic-mappings/download
  -> SICMappingService.ExportCSV
  -> SICMappingRepository.GetAll
  -> HTTP CSV attachment
```

Downloaded filename should be `sic_mappings.csv`.

CSV includes:
- `SIC_Code`
- `Description`
- `Description_Detail`
- `Category_Name`
- `Category_ID`

An empty `Category_Name` **and** `Category_ID` means intentionally no SIC categorization. On import,
`Category_Name` is the only thing that resolves a category and `Category_ID` merely confirms it; rows
where the two disagree are rejected, and so is a row carrying `Category_ID` with no `Category_Name`
(FR4: ID-only references are unsafe). See `component-methods.md` →
`SICMappingService.ValidateCSV` for the exhaustive rule table.

## Mapping File Upload Flow

Upload is a **single request** with validate-before-mutate and backup-before-overwrite semantics.

```text
User selects CSV
  -> browser confirms overwrite (client-side dialog)
  -> POST /api/sic-mappings/upload
       -> SICMappingService.ReplaceFromCSV
            -> ValidateCSV (normalize, resolve categories, per-row report)
            -> abort with report if any row invalid; nothing mutated
            -> ExportCSV -> write sic_mappings.backup-<timestamp>.csv beside the DB
            -> SICMappingRepository.ReplaceAll (single SQL transaction)
            -> SICMappingCategorizer.LoadMappings
            -> diff pre/post mappings -> affected SIC codes
            -> Categorizer.RecategorizeBySICCodes(affected)
                 (FR14: uncategorized only, manual preserved)
  -> JSON summary: imported count, rejected rows, recategorized count, backup file path
```

Validation must complete before any mutation, and `ReplaceAll` must be atomic.

**Why not a preview/confirm handshake**: a `validate → previewID → confirm` pair requires the server
to hold parsed uploads between requests — a map with a TTL, eviction, and leak handling — in an
application with no session store and exactly one local user. A client-side confirm gives the same
protection with none of that state. Whole-file validation still happens server-side before mutation.

**Why the backup is a file, not response bytes**: one HTTP response cannot be both a JSON summary and
a file attachment, and a disk backup survives the user closing the tab. The path is returned in the
JSON so the UI can show it.

**Why the diff, and not `RecategorizeAll`**: an earlier revision used `RecategorizeAll` here on the
grounds that an overwrite can change every mapping anyway. That was wrong. `RecategorizeAll` sweeps
the entire uncategorized set, so it would also categorize transactions via **text patterns** and via
**SIC mappings the upload never touched** — turning a mapping upload into a trigger for unrelated
categorization. FR14 scopes the sweep to transactions matching the SIC codes the upload created or
updated, so the diff is a behavioral requirement, not an optimization.

### Affected-code diff

`ReplaceFromCSV` already holds the pre-overwrite mappings — `ExportCSV` reads them to build the
backup — so the diff needs no extra query. A SIC code is **affected** when:

- it is absent from the pre-set and present in the post-set with a non-empty category, **or**
- it is present in both and its `category_id` changed to a different non-empty value.

A code is **not** affected when:

- its category is unchanged — it was not created or updated by this upload,
- its post-state category is empty/`NULL` — an empty-category mapping never assigns anything, **or**
- it was removed by the overwrite — FR14 keeps deletion from clearing existing assignments.

`RecategorizeBySICCodes` then routes each candidate transaction through `Categorize`, so
text-pattern-first priority still holds for the transactions it does touch. The set it touches is
exactly the FR14 set.

## Startup Optional Mapping File Import Flow

```text
main.go determines app data directory (execDir, as used today for config.json and privateledger.db)
  -> path beside config.json/privateledger.db named sic_mappings.csv
  -> SICMappingService.ImportFileIfPresent(path)
       -> if file missing: return nil
       -> if SICMappingRepository.Count() > 0: log "existing mappings take precedence", skip
       -> otherwise: parse/validate/import, reload SIC mappings
```

Missing file never fails startup. Existing invalid file should be logged and handled according to
Functional Design detail.

**Why the empty-table gate**: the mapping file is also a user-facing download/upload artifact, so the
copy on disk goes stale as soon as the user edits mappings in the UI. An unconditional upsert on every
boot would silently revert those edits, contradicting FR4's "user edits after import live in SQLite".
Gating on an empty table keeps the import idempotent in the sense FR4 actually needs: safe to run
repeatedly, never destructive.

## Modal-Driven Mapping Flow

### Change Category Modal

```text
User opens modal for transaction with SIC
  -> UI shows SIC code + Description fallback
User selects category
  -> UI prompts to create/update SIC mapping
If user agrees:
  -> POST /api/transactions/:id/sic-mapping
  -> SICMappingService creates/updates mapping (description defaults to transaction details)
  -> applies mapped category to current transaction with rule source
If user declines:
  -> existing manual category update flow may continue
```

### Create Categorization Pattern Modal

```text
User opens modal for transaction with SIC
  -> UI shows SIC code + Description fallback
User selects category
  -> UI prompts to create/update SIC mapping instead of text pattern
If user agrees:
  -> SICMappingService creates/updates SIC mapping only
  -> do not create text pattern
If user declines:
  -> existing text pattern creation flow continues
```

Modal-created mappings require a selected category; empty-category mappings are only created through
dedicated SIC page or CSV upload.

The SIC code and description shown in both modals come from the `SICCode` / `SICDescription` fields
already present on each server-rendered transaction row (`TransactionRepository` reads
`LEFT JOIN sic_mapping`), so opening a modal issues no additional request.

## Category Availability

The SIC mapping page and modals must load categories from `CategoryRepository` or existing category APIs at interaction time so newly created/updated categories are available without app restart.
