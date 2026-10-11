# Business Logic Model — UOW-2 SIC Mapping Management

## Scope and Boundary

UOW-2 manages SIC mappings through the existing handler → service → repository → SQLite flow. It
uses UOW-1 normalization and persistence contracts and exposes a narrow recategorization collaborator
for UOW-3. Transaction modal behavior and categorization internals remain outside this unit.

## Page Load

1. `PageHandler.SICMappings` asks `SICMappingService.GetPageData` for mappings and categories.
2. The service reads both repositories and returns a single page model.
3. Mappings are ordered by numeric SIC value ascending.
4. The server renders the full table. No server-side pagination is introduced.
5. Categories are read per request, so changes made on the Categories page appear on revisit.

## Create and Update

1. The Bootstrap modal collects SIC code, description, description detail, and optional category.
2. The handler decodes transport input only and delegates to the service.
3. The service normalizes and validates SIC, trims descriptions, and verifies any category exists.
4. Create rejects an existing normalized code. Update identifies the row by mapping ID and may change
   its SIC code; the resulting code must remain globally unique.
5. Repository mutation succeeds or returns a stable domain error.
6. The service reloads mapping state and invokes the injected collaborator only when the post-state
   has a non-empty category and represents a created code or a category change.
7. Changing the SIC code never clears categories already assigned under the old code.

## Delete

1. The page requires explicit confirmation for the selected mapping.
2. The service deletes by mapping ID and reloads mapping state.
3. Missing IDs return not-found; successful deletion is idempotent only at the UI level (the row is
   removed after success), not by silently accepting a repeated API delete.
4. No recategorization or clearing of existing transaction assignments occurs.

## CSV Export

1. The service loads authoritative SQLite mappings with category display fields.
2. It emits exactly `SIC_Code,Description,Description_Detail,Category_Name,Category_ID`.
3. Records follow numeric SIC order and use standard CSV escaping.
4. A database with no mappings produces the header only.
5. The handler returns the bytes as attachment `sic_mappings.csv`.

## CSV Import / Update

1. The browser confirms that uploaded rows will add or update mappings and will not delete omitted
   mappings, then sends one request.
2. The handler rejects a request whose uploaded file exceeds the shared upload size bound before any
   parsing, and reports `oversized` (BR-U2-39).
3. `ValidateCSV` parses the entire input without mutation. It normalizes codes, rejects duplicate
   normalized codes, validates descriptions and category references, and returns per-row diagnostics.
   UOW-2 calls the UOW-1 service method as implemented — it returns the validated candidate mappings,
   a `SICMappingImportReport`, and an error — and adds no second validation entry point. The upload
   result is that report extended with merge and backup fields; diagnostics accumulate under the shared
   bound in BR-U2-40 rather than growing with rejected-row count.
4. Any rejected row aborts the request before backup or database mutation.
5. After successful validation, the service snapshots current mappings and attempts to write
   `sic_mappings.backup-<timestamp>.csv` beside the database.
6. Backup success records its path. Backup failure records a prominent warning and does not stop the
   merge.
7. `MergeAll` performs every insert/update in one SQLite transaction. Any database failure rolls back
   the entire merge. Codes absent from the file remain unchanged. Header-only input is a no-op.
8. The service compares pre-state and validated rows to count created, updated, and unchanged rows.
9. It reloads mapping state after commit and invokes the collaborator once with de-duplicated affected
   codes: newly created or category-changed codes whose post-state category is non-empty.
10. The result reports counts, diagnostics, recategorized count, backup path or warnings. Once the
   merge commits, cache-reload or recategorization failures produce a successful committed result with
   prominent post-commit warnings, never an ordinary response implying rollback or inviting a blind retry.

## Failure Ordering

| Failure | Mapping mutation | Result |
|---|---|---|
| Upload exceeds size bound | None | `oversized`; rejected before parsing |
| File/format/row validation | None | Validation report |
| Backup write | Merge continues | Success or later failure plus prominent backup warning |
| SQLite merge | Full rollback | Persistence error; backup outcome retained |
| Cache reload | Merge committed | HTTP success; `MappingCommitted=true` plus prominent post-commit warning |
| Recategorization collaborator | Merge committed | HTTP success; committed mapping counts plus prominent post-commit warning |

## UOW-2 Checkpoint Collaborator

UOW-2 defines the collaborator interface and wires a production no-op implementation that returns zero
until UOW-3 supplies the real adapter. Independent UOW-2 tests inject a fake and verify the exact
affected-code set. This keeps the application runnable after UOW-2 without implementing UOW-3 early.

## Mutation Consistency

All mapping mutations through the single injected service instance—CRUD and CSV merge—are serialized by a
service-owned mutex. The mutex covers pre-state capture, backup attempt, SQLite mutation, reload, and
affected-code handoff. It is not global state, and no singleton is introduced: the instance is
constructor-injected in `main.go` like every other service, satisfying NFR3. NFR Design must specify
timeout/contention handling and verification so counts and backups cannot be calculated from a
competing mutation's state. Note for NFR Design: because the scope includes the affected-code handoff,
the lock spans a call into the UOW-3 collaborator, so hold time is not bounded by UOW-2 alone and sits
above UOW-1's SQLite busy timeout.

## Traceability

| Story | Logic |
|---|---|
| US-04 | Page load, CRUD, live category list, explicit deletion |
| US-05 | Shared normalization, validation, DB uniqueness, nullable category |
| US-10 | Five-column and header-only export |
| US-11 | Whole-file validation, best-effort backup, atomic merge/upsert, affected-code handoff |
