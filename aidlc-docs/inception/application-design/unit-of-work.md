# Units of Work — Issue #5 SIC Auto-Categorization

## Decomposition Strategy

The feature is decomposed into three end-to-end vertical units. Each unit may modify the existing model, parser, repository, service, handler, and UI layers needed to deliver its capability. These are development and review boundaries, not deployment boundaries: all units remain in the existing PrivateLedger process, single binary, and shared SQLite database.

One maintainer/team owns the complete feature. Existing clean-architecture direction remains mandatory:

```text
Browser -> Handler -> Service -> Repository -> SQLite
```

## UOW-1 — SIC Import and Storage

### Outcome

Imported OFX/QFX transactions retain normalized SIC values, and both fresh and existing local databases safely support transaction SIC data and the mapping schema.

### Responsibilities

- Add transaction and SIC mapping domain models, including the shared digits-only `NormalizeSICCode` boundary.
- Extract non-zero `ofxgo.Transaction.SIC` values and attach them to imported transactions.
- Add `ledger_transaction.sic_code`, the `sic_mapping` table, uniqueness constraints, foreign-key behavior, and `idx_txn_sic`.
- Upgrade existing databases idempotently without changing the transaction deduplication key or losing financial data.
- Persist and read SIC values through repositories.
- Initialize mappings from an optional local `sic_mappings.csv` only when the mapping table is empty.
- Keep startup successful when the mapping file is absent and keep all processing local.
- Establish foundational parser, migration, repository, and startup-import tests.

### Primary Components

- `internal/model/transaction.go`
- `internal/model/sic_mapping.go`
- `internal/parser/ofx_parser.go`
- `internal/database/schema.sql` and startup migration code
- `internal/repository/transaction_repo.go`
- `internal/repository/sic_mapping_repo.go`
- Startup wiring in `cmd/privateledger/main.go`
- The startup-file portion of `internal/service/sic_mapping_service.go`

### Assigned Stories

- US-01 — Import transactions with SIC data
- US-07 — Upgrade existing local databases safely
- US-08 — Initialize SIC mappings from an existing mapping file
- US-09 — Preserve local-only privacy

### Completion Boundary

The unit is complete when SIC data survives import and database round trips, migrations are repeatable against existing databases, optional startup seeding obeys the empty-table gate, and no external service is introduced. User-facing mapping management and applying mappings during categorization belong to later units.

## UOW-2 — SIC Mapping Management

### Outcome

Users can safely manage, download, and atomically merge SIC mappings through a dedicated local configuration workflow.

### Responsibilities

- Implement SIC mapping CRUD with globally unique normalized codes and nullable categories.
- Validate digits-only codes, maximum length, category references, and CSV rows.
- Resolve CSV categories by `Category_Name`, using `Category_ID` only as confirmation; reject ID-only and conflicting references.
- Provide the dedicated SIC mapping page and REST endpoints.
- Expose current categories through `SICMappingService.GetPageData` so handlers do not bypass the service layer.
- Export valid `sic_mappings.csv`, including headers when no mappings exist.
- Validate an entire uploaded file before mutation, attempt to back up current mappings beside the database, and atomically merge new/matching codes without deleting omitted mappings.
- Reload mapping caches and request SIC-scoped recategorization for affected non-empty post-upload mappings through the categorization contract implemented by UOW-3.
- Add CRUD, validation, CSV round-trip, backup-outcome, atomic merge/upsert, and handler tests.

### Primary Components

- `internal/model/sic_mapping.go`
- `internal/repository/sic_mapping_repo.go`
- `internal/service/sic_mapping_service.go`
- `internal/handler/sic_mapping_handler.go`
- `cmd/privateledger/web/templates/sic_mappings.html`
- Page routing, API routing, navigation, and dependency wiring

### Assigned Stories

- US-04 — Manage SIC mappings in a separate configuration page
- US-05 — Prevent duplicate SIC mappings
- US-10 — Download current SIC mappings
- US-11 — Upload SIC mappings and merge with existing mappings

### Completion Boundary

The unit is complete when mapping administration works through service-mediated APIs/UI, CSV merge is validate-before-mutate and atomic, backup success or failure is clearly reported, and resulting mapping changes can invoke the categorization integration contract. Transaction modal behavior is excluded from this unit.

## UOW-3 — Transaction Categorization Integration

### Outcome

Transactions use text patterns before SIC mappings, preserve deliberate categorization, expose SIC context in existing modals, and can create mappings directly from those workflows.

### Responsibilities

- Implement the thread-safe `SICMappingCategorizer` cache and exact normalized SIC matching.
- Refactor `Categorizer.LoadRules`, `Categorize`, and bulk/scoped recategorization so text patterns always run before SIC.
- Treat mappings with empty categories as an intentional SIC no-op.
- Preserve manual and existing rule-based categorizations; mapping changes affect only currently uncategorized matching transactions.
- Implement affected-SIC recategorization using one repository query and the common `Categorize` path.
- Populate joined `SICDescription` fields and show SIC context in Change Category and Create Categorization Pattern modals without adding a main-table column.
- Implement modal-driven mapping upsert behavior and rule-source assignment for the current transaction.
- Ensure import results retain the existing combined `total_auto_categorized` count.
- Complete cross-unit architecture, categorization-priority, concurrency, modal, and regression tests, including selected property-based tests.

### Primary Components

- `internal/service/categorizer.go`
- `internal/service/sic_mapping_categorizer.go`
- SIC recategorization portions of `internal/service/sic_mapping_service.go`
- `internal/repository/transaction_repo.go`
- Transaction and categorization handlers
- `cmd/privateledger/web/templates/transactions.html`
- Application wiring and integration tests

### Assigned Stories

- US-02 — Use existing text patterns before SIC mappings
- US-03 — Preserve manual categorization
- US-06 — Inspect SIC in transaction detail context
- US-12 — Create SIC mappings from transaction categorization modals
- US-13 — Maintain reliable implementation boundaries and tests

### Completion Boundary

The unit is complete when all categorization entry points use text-first-then-SIC priority, protected categories remain unchanged, both modals implement the approved SIC behavior, and the complete feature passes build and test verification.

## UOW-4 — Category Lifecycle and Mapping-File Restore Integrity

**Added 2026-09-06**, after UOW-1 and UOW-2 completed and while UOW-3 was in Code Generation planning.
This amends the originally approved three-unit decomposition.

### Outcome

A category rename, retype, or deletion leaves both live categorization and the local mapping files
usable, and a backup written by the application can actually be restored through the application.

### Founding Problem

Renaming a category silently breaks every previously exported mapping CSV, including the backups the
application writes before each merge. Verified 2026-09-06: export a mapping while its category is named
`Groceries`, rename that category to `Food`, then re-upload the exported file — the whole upload is
rejected with `category_not_found` on `Category_Name`.

Live categorization is unaffected, because transactions and mappings both key on `category_id` and the
name is only ever joined for display. The damage is confined to file exchange and restore.

The root cause is a sound decision with an unexamined consequence. FR4 deliberately made
`Category_Name` authoritative and `Category_ID` merely confirmatory, so a stale ID after a
delete-and-recreate cannot resolve to a valid but wrong category. That protection is right for a
user-authored file. It is wrong for a machine-generated backup taken from this same database moments
earlier, where the ID is provably consistent and the name is a convenience column. The two trust
contexts are currently served by one code path.

A related gap makes this hard to place anywhere else: **no story covers restoring from a backup.**
US-11 mentions restore only as a hoped-for side effect of upload. Backups are written but restoring one
was never designed, which is why this defect has no owning unit.

### Scope

Candidate scope, to be confirmed when the unit's scope freezes:

- A restore path that trusts `Category_ID` for application-generated backups while leaving user-authored
  uploads name-first exactly as FR4 requires.
- A story for restoring from a backup file, provisionally US-14.
- Category rename, retype, and delete behaviour across mappings, exports, and backups.
- Further findings admitted under the rule below.

### Admission Rule

Not every later finding belongs here.

| Finding type | Where it goes |
|---|---|
| Breaks approved behaviour of a shipped unit | Fixed **in that unit**, as U2-F09 was |
| Contract amendment, missing capability, or an unexamined interaction between units | UOW-4 |

Without this line, a genuine regression gets deferred merely because a queue exists. U2-F09 — where
mapping deletion could not work at all — would have been a candidate for parking, and parking it would
have been wrong.

### Scope Freeze

Scope stays open for admissions until UOW-3 Code Generation completes, then freezes. A unit defined as
"whatever we find later" has no completion boundary, and every stage of this workflow depends on having
one. Freezing at UOW-3 completion is what allows the boundary below to be stated at all.

### Assigned Stories

To be defined when scope freezes. Candidate stories and admitted findings are recorded in
`aidlc-docs/construction/uow-4-findings-register.md` until then. No acceptance criteria are invented in
advance of requirements gathering.

### Completion Boundary

The unit is complete when a backup produced by the application can be restored through the application
after any category rename or retype, when user-authored uploads retain name-first resolution unchanged,
and when every finding admitted before the scope freeze is resolved or explicitly deferred with a
recorded reason.

## Cross-Unit Rules

- A story has exactly one primary unit even when its acceptance criteria require another unit's contract.
- Shared files may be touched by multiple units; ownership here describes behavior, not exclusive file ownership.
- UOW-1 establishes persistent data contracts, UOW-2 establishes mapping-management contracts, UOW-3 integrates those contracts into categorization, and UOW-4 repairs cross-cutting lifecycle and restore gaps found once the first three are in use.
- Database schema and APIs are evolved in place; no service extraction, additional process, cloud lookup, or second database is introduced.
- Infrastructure Design remains skipped because the deployment topology does not change.

## Delivery Order

1. Complete UOW-1 so SIC transaction and mapping persistence contracts exist.
2. Complete UOW-2 so mappings can be managed and exchanged locally.
3. Complete UOW-3 so import and user workflows consume mappings with the approved priority and preservation rules.
4. Freeze UOW-4 scope at UOW-3 completion, then complete UOW-4 so category lifecycle changes cannot strand a mapping file or a backup.

Incremental vertical checkpoints should keep each unit buildable and testable, while cross-unit verification is finalized in UOW-3.
