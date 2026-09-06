# Application Design — Issue #5 SIC Auto-Categorization

## Overview

This design adds SIC-based categorization while preserving PrivateLedger’s local-only single-binary clean architecture. The existing `Categorizer` remains focused on categorization orchestration, `SICMappingCategorizer` becomes a SIC-specific categorization extension, and `SICMappingService` owns SIC mapping management workflows such as CSV import/export and upload merge/upsert.

## Key Design Decisions

- UI navigation: SIC mapping page is linked from the existing Categories page, not top-level navigation.
- Mapping file name: `sic_mappings.csv`.
- CSV columns: `SIC_Code`, `Description`, `Description_Detail`, `Category_Name`, `Category_ID`. No mapping-ID column — an interchange file gains nothing from internal primary keys.
- Category references: `Category_Name` is the only resolver; `Category_ID` only confirms it. Rows where name and ID disagree are rejected, **and so is a row carrying `Category_ID` with no `Category_Name`** — ID-only matching is unsafe because a deleted-and-recreated category leaves a stale ID that still resolves to a valid but wrong category. See `component-methods.md` → `ValidateCSV` for the exhaustive rule table.
- Empty category: omit/empty `Category_ID`; persisted as nullable `category_id`.
- Startup mapping file location: `sic_mappings.csv` beside `config.json` and `privateledger.db` in the application data directory.
- Upload merge: a single request validates the entire file, attempts a timestamped backup CSV beside the database, then atomically inserts new codes and updates matching codes while retaining omitted mappings. Backup failure is returned as a prominent warning but does not block the merge. The browser confirms before POSTing; no server-side preview/token state is introduced.
- Backup delivery: backup is written to `sic_mappings.backup-<timestamp>.csv` in the application data directory and its path is returned in the JSON response (one HTTP response cannot be both JSON and a file attachment).
- Startup mapping file import: runs only when the SIC mapping table is empty, so user edits made in the UI are never reverted by a stale file on restart.
- SIC normalization: `model.NormalizeSICCode` is the single owner; every write and lookup path goes through it. Global uniqueness is enforced by a DB `UNIQUE` constraint, not by application checks alone.
- SIC value domain: digits only, per amended FR11. `ofxgo` parses `<SIC>` as `int64`, so imported SIC values are always numeric; an alphanumeric mapping code could never match a transaction. Absent SIC and `SIC = 0` are indistinguishable and both treated as unset. UOW-1 Functional Design refines `NormalizeSICCode` to trim whitespace and remove leading zeros, and accepts mapping values only in the positive `int64` domain. `sic_mapping.sic_code` needs no collation qualifier.
- Rule caches: both `Categorizer.patterns` and `SICMappingCategorizer` mappings are guarded by `sync.RWMutex`, because reloads are triggered from HTTP handlers while imports read the caches.
- Transaction SIC description: exposed as a joined display field via `LEFT JOIN sic_mapping`, following the existing `CategoryName`/`CategoryColor` convention.
- Transaction SIC display: shown in Change Category and Create Categorization Pattern modals.
- Modal mapping creation: modal-created SIC mappings require selected category.

## Component Summary

See `components.md` for full component details.

### Modified Components

- Transaction model: add optional `SICCode`.
- OFX parser: extract `<SIC>`.
- Transaction repository: persist/select `sic_code`.
- Database migration: add SIC schema idempotently.
- Categorizer: apply text patterns first, then delegate SIC matching.
- SICMappingCategorizer: SIC-specific matching extension.
- SICMappingService: SIC mapping CRUD and CSV workflow owner.
- Transactions template: show SIC context in categorization modals.
- Main wiring: initialize new repository/handler/routes/startup import.

### New Components

- SIC mapping model.
- SIC mapping repository.
- SIC mapping handler.
- SIC mapping page template.

Page reads go through `SICMappingService.GetPageData`, keeping NFR3's handler → service → repository layering intact (the dashboard's existing `insightsService` dependency is the precedent).

## Service Summary

See `services.md` for full orchestration details.

Primary categorization owner: `Categorizer`.
Primary SIC matching extension: `SICMappingCategorizer`.
Primary SIC management owner: `SICMappingService`.

The Categorizer will:

1. Load text patterns.
2. Ask `SICMappingCategorizer` to load SIC mappings as part of rule loading.
3. Categorize transactions using text-pattern-first priority.
4. Delegate SIC matching only after text patterns fail.
5. Leave transactions uncategorized for empty-category SIC mappings.
6. Apply the same priority in bulk recategorization: `RecategorizeAll` and `RecategorizeByCategory` are refactored to call `Categorize` per transaction instead of re-implementing pattern matching inline, so SIC participates in every recategorization path.

The SICMappingService will:

1. Import the optional startup CSV mapping file, only when the mapping table is empty.
2. Validate an entire uploaded CSV before any mutation.
3. Export CSV for downloads and best-effort pre-upload backups.
4. Atomically merge mappings in one request after validation; deletion remains explicit.
5. Create/update mappings from transaction modals.
6. Trigger SIC mapping reload/recategorization through the categorization components.

## Method Summary

See `component-methods.md` for high-level method signatures.

Notable new methods include:

- `model.NormalizeSICCode`
- `SICMappingRepository.MergeAll`
- `TransactionRepository.GetUncategorizedBySICCodes`
- `Categorizer.RecategorizeBySICCodes`
- `Categorizer.LoadRules`
- `SICMappingCategorizer.LoadMappings`
- `SICMappingCategorizer.Match`
- `SICMappingService.ExportCSV`
- `SICMappingService.ValidateCSV`
- `SICMappingService.MergeFromCSV`
- `SICMappingService.ImportFileIfPresent`
- `SICMappingService.CreateOrUpdateFromTransaction`
- `SICMappingHandler.UploadMappings`
- `SICMappingHandler.UpsertFromTransaction`

## Dependency Summary

See `component-dependency.md` for matrix and flow diagrams.

The design keeps dependencies one-directional:

```text
Handler -> Service (Categorizer / SICMappingService) -> Repository -> SQLite
```

The parser feeds `model.Transaction` into the existing import service. The categorizer uses loaded rule caches and repositories but repositories remain service-free.

## Requirements Coverage

| Requirement Area | Design Coverage |
|---|---|
| Parse/store SIC | Transaction model, parser, repository, migration |
| SIC mapping CRUD | SIC model/repository/handler/page |
| Empty-category mappings | nullable `category_id`; categorizer no-op behavior |
| Text pattern priority | Categorizer ordering |
| Mapping CSV startup import | `SICMappingService.ImportFileIfPresent` from app data directory, gated on empty mapping table |
| Download/upload merge | SICMappingService CSV export, single-request validate + best-effort backup + atomic merge/upsert |
| Text pattern priority in bulk recategorization | `Categorizer.RecategorizeAll` / `RecategorizeByCategory` refactored onto `Categorize` |
| SIC normalization and uniqueness | `model.NormalizeSICCode` + DB `UNIQUE` constraint |
| Modal SIC behavior | transaction template + upsert-from-transaction endpoint |
| Existing DB compatibility | idempotent database migration |
| Local-only privacy | no external dependencies |
| PBT partial setup | carried forward to NFR Requirements/NFR Design |

## Deferred to Later Stages

Functional Design will define detailed rules for:

- CSV validation and error reporting (per-row error format, whether a single bad row fails the whole upload).
- Invalid startup mapping file behavior.
- Backup file retention (whether old `sic_mappings.backup-*.csv` files are pruned).
- Detailed UI prompts and confirm-dialog wording.

NFR stages will define:

- PBT framework selection and applicable property tests.
- Idempotency and rollback patterns.
- Local-only/privacy checks.

## Design Review Resolutions (2026-08-24)

A best-practices/simplicity review of these artifacts against `requirements.md` and the existing
codebase produced the following resolutions, now folded into the design:

| # | Finding | Resolution |
|---|---|---|
| 1 | Startup CSV import would silently revert UI edits on every restart | Import gated on `SICMappingRepository.Count() == 0` |
| 2 | `Category_ID`-only CSV can silently map to a wrong-but-valid category | `Category_Name` resolved first, `Category_ID` as tiebreaker, conflicts rejected |
| 3 | Preview-token two-step upload added server-side ephemeral state | Collapsed to one `POST /api/sic-mappings/upload` with browser-side confirm |
| 4 | Backup returned as `[]byte` alongside JSON is not expressible in one response | Backup written to disk; path returned in JSON |
| 5 | Rule caches reloaded from handlers while imports read them | `sync.RWMutex` on both caches; existing `go LoadPatterns()` race fixed |
| 6 | `RecategorizeAll` / `RecategorizeByCategory` would skip SIC entirely | Both refactored to call `Categorize`; `GetUncategorizedBySICCodes` added |
| 7 | SIC normalization had no owner | `model.NormalizeSICCode` + DB `UNIQUE` constraint |
| 8 | Modal SIC description source unspecified | `LEFT JOIN sic_mapping` display field on `TransactionRepository` reads |
| 9 | Modal-created mappings had no description | `CreateOrUpdateFromTransaction` accepts a description |
| 10 | `LoadPatterns` / `LoadRules` dual entry points | `loadPatterns` unexported; callers use `LoadRules` |

### Requirement amendments — APPLIED 2026-08-24

All three amendments are applied in `requirements.md`, and `stories.md` is aligned:

- **FR4** — import now runs only when the SIC mapping table is empty; the file is explicitly
  non-authoritative once the user edits mappings in the UI. Resolves the conflict between "idempotent
  and safe to run repeatedly" and "user edits after import live in SQLite".
- **FR4** — CSV columns are `SIC_Code`, `Description`, `Description_Detail`, `Category_Name`,
  `Category_ID`; the "internal/import ID column" allowance is removed and name-first resolution with
  ID as tiebreaker is now a requirement.
- **FR11** — SIC codes are **digits only**, with the rationale recorded. Non-numeric SIC values from
  OFX/QFX moved to Out of Scope. New acceptance criteria cover non-digit rejection and
  name/ID-disagreement rejection.

FR13 needed **no amendment**: it already specified a single `POST /api/sic-mappings/upload`. The
two-step `/upload/validate` + `/upload/confirm` pair was design drift, now corrected. Optionally add
`POST /api/transactions/:id/sic-mapping` to FR13's list for completeness — FR13 says "recommended
minimum endpoints", so its absence is not a contradiction.
