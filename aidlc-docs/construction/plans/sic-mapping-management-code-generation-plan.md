# Code Generation Plan — UOW-2 SIC Mapping Management

## Authority and Scope

This plan is the single source of truth for UOW-2 Code Generation. Production generation must execute
these steps in order and may not add behavior outside the approved requirements, functional design, and
NFR design artifacts.

UOW-2 implements:

- US-04 — Manage SIC mappings in a separate configuration page.
- US-05 — Prevent duplicate SIC mappings.
- US-10 — Download current SIC mappings.
- US-11 — Upload SIC mappings and merge with existing mappings.

It does **not** implement transaction categorization, text-first priority, manual-assignment protection,
the transaction-modal mapping workflow (US-12), or SIC display in transaction detail (US-06). Those
belong to UOW-3. UOW-2 defines the recategorization collaborator contract and wires an explicit
production no-op.

## Mandatory Ownership Boundary

### Production role

- May modify production Go, SQL, templates, static assets, and production documentation listed here.
- Must not create or modify `_test.go` files, `testdata/`, test helpers, benchmark code, fuzz/property
  tests, test-only configuration, or the independent review artifact.
- Must not add or modify `pgregory.net/rapid`; that test-only dependency belongs to the independent role.
- Must not modify the 15 existing UOW-1 test files. If a UOW-1 test fails after a production change,
  report it as a finding for the independent role rather than editing the test.

### Independent review/test role

- Must run in a separate provider session after production generation.
- Owns production-code review, test strategy, all verification tests/fixtures/benchmarks/property tests,
  test-only dependencies, and `aidlc-docs/construction/sic-mapping-management/code-review/independent-review.md`.
- Must report PASS with all required tests and performance evidence passing and no unresolved
  Blocking/High findings before UOW-2 Code Generation can complete.

## Existing Dependencies and Contracts

- Runtime: Go, Gin, `database/sql`, `modernc.org/sqlite`, `encoding/csv`, `net/http` multipart, `slog`.
  No new production dependency is introduced.
- Layering: `handler -> service -> repository -> SQLite`; constructor injection in
  `cmd/privateledger/main.go`. No singleton or global coordinator (NFR3, NFR-U2-MAINT-01).
- Frontend: server-rendered embedded templates, Bootstrap 5, HTMX where conventional, vanilla JS.
- UOW-1 supplies: `model.NormalizeSICCode` / `ParseSICCode`, `model.SICMapping`,
  `SICMappingImportError`, `SICMappingImportReport`, `SICMappingImportOutcome`,
  `SICMappingRepository` (Count/Create/GetByID/GetByCode/GetAll/Update/Delete/BulkInsertAtomic),
  `SICMappingService.ValidateCSV`, and startup seeding.
- `sic_mapping.sic_code` is `TEXT NOT NULL UNIQUE`; the canonical domain is positive-`int64` digits.

## Verified Pre-Existing Conditions

Confirmed against the working tree before planning. Each is addressed by a numbered step.

| Observation | Location | Step |
|---|---|---|
| F-15: a `categoryRepo.GetAll()` database failure is relabelled `read_failed` by the caller | `internal/service/sic_mapping_service.go:90-94` | 2 |
| Row validity is decided by `len(report.Errors)`, which a diagnostic cap would silently break | `internal/service/sic_mapping_service.go:161,179` | 2 |
| F-14: `10<<20` duplicates the unexported `maxSICMappingSeedSize` | `cmd/privateledger/main.go:234` vs `internal/service/sic_mapping_service.go:16` | 1 |
| Diagnostic cap `50` lives in the main package and bounds startup logging only | `cmd/privateledger/main.go:30` | 1, 2 |
| Mapping order is lexicographic (`1000` before `999`), not the required numeric order | `internal/repository/sic_mapping_repo.go:100` | 3 |
| `ReplaceAll` exists and is destructive; merge must never call it | `internal/repository/sic_mapping_repo.go:171` | 3 |

## Expected Production Files

### Modify in place

- `internal/model/sic_mapping.go`
- `internal/service/sic_mapping_service.go`
- `internal/repository/sic_mapping_repo.go`
- `internal/handler/page_handler.go`
- `cmd/privateledger/main.go`
- `cmd/privateledger/web/templates/layout.html`
- `API_ROUTES.md`

### Create

- `internal/handler/sic_mapping_handler.go`
- `cmd/privateledger/web/templates/sic_mappings.html`
- `aidlc-docs/construction/sic-mapping-management/code/production-summary.md`
- `aidlc-docs/construction/sic-mapping-management/code/independent-review-handoff.md`

Exact file placement may use an existing same-responsibility file discovered immediately before a step,
but duplicate replacement files such as `_new.go` or `_v2.go` are prohibited. No schema migration is
required: `sic_mapping` already exists with the needed columns and constraints.

---

## Sequential Generation Steps

### Step 1 — Shared domain contracts, limits, and result types

- [x] Modify `internal/model/sic_mapping.go` to own the shared bounds with no HTTP or service dependency:
      an exported max mapping-file size (10 MiB) and an exported max retained row diagnostics (50).
- [x] Repoint `internal/service/sic_mapping_service.go:16` and `cmd/privateledger/main.go:30,234` at the
      shared constants and delete the duplicated literals. This closes F-14 and satisfies BR-U2-39/40.
- [x] Add `SICMappingImportMerged SICMappingImportOutcome = "merged"` to the existing enumeration and
      keep `absent`/`skipped_existing` startup-only (BR-U2-42, resolving F-16). Add no second outcome type.
- [x] Add `DiagnosticsTruncated bool` to `SICMappingImportReport` with stable JSON spelling shared by the
      startup and upload paths.
- [x] Add `SICMappingImportResult` as an extension of `SICMappingImportReport` carrying `CreatedRows`,
      `UpdatedRows`, `UnchangedRows`, `RecategorizedRows`, `BackupPath`, `BackupWarning`,
      `MappingCommitted`, and `PostCommitWarnings`. Shared fields keep their UOW-1 meanings and JSON names;
      `ImportedRows` stays zero on upload. Do not duplicate `SICMappingImportError`.
- [x] Add `SICMappingInput` (transport-neutral create/update command: code, descriptions, optional category
      ID) and `SICMappingPageData` (numerically ordered mappings plus current categories).
- [x] Add a mutation result carrying the saved flag and post-commit warnings so CRUD and upload share one
      truthful saved-state contract (NFRP-U2-07).
- [x] Add stable sentinel domain errors for duplicate code, mapping not found, and admission busy so
      handlers map outcomes without matching driver error strings.
- [x] Keep all additions free of CSV, SQL, Gin, and `net/http` dependencies.
- [x] Trace to US-04, US-05, US-11; BR-U2-39/40/42; NFR-U2-SEC-02/03, NFR-U2-MAINT-01; NFRP-U2-05/07.

### Step 2 — Correct shared validation: F-15 and bounded diagnostics

- [x] Modify `ValidateCSV` to decide row validity with an explicit per-row invalid flag (or an uncapped
      invalid counter), never `len(report.Errors)`. Without this, capping diagnostics would classify every
      invalid row after the cap as valid and merge it. A row with multiple errors remains one rejected row.
- [x] Bound retained diagnostics at the shared 50 constant inside the producing service — the accumulation
      site, not the display site — and set `DiagnosticsTruncated` when any diagnostic is dropped.
      `RejectedRows` remains authoritative and continues to be derived from processed rows (BR-U2-40).
- [x] Continue validation and duplicate detection for all parseable rows after the cap.
- [x] Set `persistence_failed` on category-load failure and reserve `read_failed` for genuine reader
      failures. Fix `internal/service/sic_mapping_service.go:90-94` so the wrapper preserves the classified
      outcome instead of overwriting it. This is the F-15 correction (NFR-U2-REL-03).
- [x] Preserve the existing startup fatal/non-fatal boundary and contextual error wrapping.
- [x] Preserve F-04's deferred structural-stop behavior: do not claim counts for rows never read. F-04 and
      F-05 remain deferred and are out of scope for this plan.
- [x] Keep diagnostic message text fixed and safe; never echo arbitrary-length uploaded field values.
- [x] Trace to US-11; BR-U2-40/41; NFR-U2-SEC-03, NFR-U2-REL-03; NFRP-U2-05.

### Step 3 — Extend the mapping repository with atomic merge and numeric ordering

- [x] Add `MergeAll` to `internal/repository/sic_mapping_repo.go` performing every insert and update in one
      SQLite transaction using a single reused prepared parameterized
      `INSERT ... ON CONFLICT(sic_code) DO UPDATE`, rolling back on any prepare/execute error and returning
      commit failure. Preserve mapping identity and creation metadata on update.
- [x] `MergeAll` must not call the destructive `ReplaceAll`, and must not delete rows. An empty candidate
      set commits a successful no-change transaction through the same contract.
- [x] Accept a `context.Context` on `MergeAll` and on any other new transaction operation so cancellation
      before commit rolls back. Do not perform a general context-propagation refactor of existing UOW-1
      repository methods.
- [x] Change mapping ordering from lexicographic `ORDER BY sm.sic_code` to numeric ordering, since the
      column is `TEXT` and canonical codes otherwise sort `1000` before `999`. Apply it to the read paths
      backing the page and CSV export.
- [x] Keep unique SIC and nullable category foreign keys database-authoritative; map driver errors into the
      stable domain error categories from Step 1 rather than returning raw driver text.
- [x] Extend joined display fields only where the page or export actually renders them; keep row iteration
      and `rows.Err()` checks consistent with existing methods.
- [x] Trace to US-04, US-05, US-10, US-11; BR-U2-03/19/20/22; NFR-U2-REL-01, NFR-U2-PERF-01; NFRP-U2-03.

### Step 4 — Service admission gate and collaborator contract

- [x] Modify `SICMappingService` to own a capacity-one channel used as a mutual-exclusion gate covering all
      CRUD and upload mutations (BR-U2-44).
- [x] Acquire by selecting between the gate, request cancellation, and a timeout. Never spawn a goroutine
      that could acquire the gate after its caller returned. Recheck cancellation after acquisition, then
      release exactly once via `defer` on every completion and failure path.
- [x] Accept a positive admission timeout through the constructor, defaulting to a named five seconds in
      production wiring. Add no `config.json` field, no configuration migration, and no options framework.
      Make no FIFO fairness promise.
- [x] Define the narrow recategorization collaborator interface (de-duplicated normalized SIC codes in, a
      recategorized count or error out) and a production no-op implementation returning zero. `nil` must
      never masquerade as successful work.
- [x] Preserve existing UOW-1 constructor call sites, adding a small compatible wrapper if needed so
      startup seeding and its existing tests keep working.
- [x] Read-only page and export paths must not take the mutation gate.
- [x] Trace to US-04, US-11; BR-U2-44/46; NFR-U2-CON-01, NFR-U2-MAINT-01; NFRP-U2-01/02.

### Step 5 — Service page data, CRUD, and CSV export

- [x] Implement `GetPageData` returning numerically ordered mappings plus current categories in one call so
      `PageHandler` never reads repositories directly (NFR3 layering).
- [x] Implement create and update: normalize and validate the SIC code, trim descriptions, verify any
      supplied category exists, and return the stable duplicate error on collision. Update targets the
      mapping ID and may change the SIC code, which must remain globally unique; the mapping ID stays stable.
- [x] Implement delete by mapping ID returning not-found for a missing ID. Deletion never clears categories
      already assigned to transactions.
- [x] Invoke the collaborator after a successful CRUD mutation only when the post-state category is
      non-empty and the change is a creation or a category change; description-only edits and cleared
      categories do not trigger it (BR-U2-29/30/31).
- [x] Apply the same saved-state contract as merge: once the write commits, a reload or collaborator failure
      is a committed-with-warning success, never a failure implying rollback (BR-U2-45).
- [x] Implement CSV export from authoritative SQLite state emitting exactly
      `SIC_Code,Description,Description_Detail,Category_Name,Category_ID` in numeric order with standard
      escaping; an empty database yields the header only.
- [x] Run every mutation under the Step 4 gate; keep export and page reads outside it.
- [x] Trace to US-04, US-05, US-10; BR-U2-01 through BR-U2-11, BR-U2-45; NFR-U2-REL-01; NFRP-U2-03/07.

### Step 6 — Local backup helper

- [x] Add one private CSV backup helper in the service using the injected application-data directory and
      `filepath.Join`; never derive a path from uploaded content or an uploaded filename.
- [x] Create the file with `os.CreateTemp` using prefix/suffix shaped
      `sic_mappings.backup-<UTC timestamp>-<random>.csv` and mode `0600`, so an existing file or symlink is
      never opened for overwrite and timestamp collisions cannot clobber an earlier backup.
- [x] Check CSV write errors, `Flush`/`Error`, and file `Close`. Publish the backup path only after all
      succeed.
- [x] On failure, close and remove only the newly created incomplete file; if cleanup also fails, report a
      safe warning that an incomplete file remains. Never advertise a partial file as a backup.
- [x] Retain all completed backups; implement no pruning by age or count.
- [x] Trace to US-11; BR-U2-23/24/25; NFR-U2-REL-02, NFR-U2-SEC-01; NFRP-U2-06.

### Step 7 — Service upload merge orchestration

- [x] Implement the upload workflow in this order under one admission acquisition: acquire, validate the
      complete input via the shared `ValidateCSV`, exit before any backup or mutation if any row is
      rejected, read one authoritative pre-state snapshot, compute the diff, attempt the backup, merge,
      then reload and hand off.
- [x] Use the single pre-state snapshot for both the created/updated/unchanged diff and the backup contents,
      indexed by canonical SIC for linear expected-time comparison, so a backup failure can never invalidate
      the counts (BR-U2-43).
- [x] Assign `MappingCommitted` and the persisted counts only after `MergeAll` returns successfully; report
      `merged` on success. Header-only input is a successful no-op with all counts zero and the backup
      attempt still performed.
- [x] Build the de-duplicated affected-code set from newly created codes and category-changed codes whose
      post-state category is non-empty. Exclude unchanged, description-only, cleared-category, deleted, and
      omitted codes. Invoke the collaborator once, only for a non-empty set.
- [x] After commit, finish reload and handoff synchronously while retaining admission. A reload failure
      skips recategorization; reload or collaborator failure becomes a post-commit warning on a successful
      result. Browser cancellation after commit can never be reported as a rollback; log the saved outcome
      if the response can no longer be delivered.
- [x] Add no post-commit timeout, worker, queue, or cancellation framework for the no-op checkpoint; UOW-3
      owns the real collaborator's budget and cancellation design.
- [x] Trace to US-11; BR-U2-12 through BR-U2-22, BR-U2-26 through BR-U2-31, BR-U2-43/45/46;
      NFR-U2-REL-01/02, NFR-U2-PERF-01; NFRP-U2-02/03/06.

### Step 8 — Mapping API handler with bounded multipart intake

- [x] Create `internal/handler/sic_mapping_handler.go` depending only on `SICMappingService` — never on a
      repository — and implement `GET /api/sic-mappings`, `POST /api/sic-mappings`,
      `PUT /api/sic-mappings/:id`, `DELETE /api/sic-mappings/:id`, `GET /api/sic-mappings/download`, and
      `POST /api/sic-mappings/upload`.
- [x] Before any form parsing, wrap the request body with `http.MaxBytesReader` at the shared 10 MiB limit
      plus 1 MiB for multipart framing. Call `ParseMultipartForm` with a 1 MiB memory threshold, understanding
      that this bounds in-memory spill, not total accepted size.
- [x] Require exactly one uploaded file named `file`; reject missing or extra files. Determine the actual
      file size from the parsed part and reject over the shared limit before calling into validation. Never
      trust client `Content-Length` as enforcement. An exactly-at-limit CSV remains eligible.
- [x] `defer` `MultipartForm.RemoveAll` wherever form state exists, including error paths, and close any
      opened file. Pass the opened multipart file to the service as a reader rather than copying it into
      another buffer or writing a custom multipart loop.
- [x] Serve download as attachment `sic_mappings.csv` with standard CSV escaping; the uploaded filename never
      influences any server path.
- [x] Map outcomes exactly as approved: 400 malformed input; 422 validation report with no saved counts;
      409 duplicate on CRUD; 404 missing ID; 413 oversized; 503 `mapping_busy` with `Retry-After: 1` on
      admission expiry; 408 `request_cancelled` when a cancelled waiter can still be answered; 500 contextual
      pre-commit infrastructure failure; 503 `database_busy` on recognized SQLite busy exhaustion before
      commit with no automatic retry; 201 on create committed; 200 on update, delete, and upload committed.
      Delete returns a body so post-commit warnings remain visible.
- [x] Never emit a generic 500 after a known successful commit. Keep error mapping in the handler and
      classification in the service and repository; do not match arbitrary driver error strings.
- [x] Log through `slog` without recording complete uploads, CSV rows, or financial payloads.
- [x] Trace to US-04, US-05, US-10, US-11; BR-U2-36/38/39; NFR-U2-SEC-01/02, NFR-U2-REL-01; NFRP-U2-04/07.

### Step 9 — Page route, wiring, and documentation

- [x] Modify `internal/handler/page_handler.go` to add the SIC mappings page, obtaining its data from
      `SICMappingService.GetPageData` and following the existing `insightsService` precedent. Do not add
      repository dependencies to `PageHandler`.
- [x] Modify `cmd/privateledger/main.go` to construct the service with the application-data directory, the
      named five-second admission default, its repositories, and the explicit no-op collaborator; construct
      the mapping handler; and register the page route and the six API routes in the existing `api` group.
- [x] Preserve existing startup ordering: migration, then seeding, then route registration and serving. Keep
      all wiring in `main.go`; introduce no service global or singleton.
- [x] Add the SIC mappings link to `cmd/privateledger/web/templates/layout.html` following existing
      navigation conventions.
- [x] Update `API_ROUTES.md` with the six endpoints, their status codes, and stable error codes.
- [x] Trace to US-04, US-10, US-11; NFR-U2-MAINT-01, NFR-U2-SEC-01; NFRP-U2-07.

### Step 10 — SIC mappings page template

- [x] Create `cmd/privateledger/web/templates/sic_mappings.html` using the existing Bootstrap/HTMX/vanilla-JS
      stack only. Add no frontend framework or production library.
- [x] Render the action bar (create, download, import/update with hidden file input), status/alert region,
      and the full mapping table in numeric SIC order with both description fields and an explicit
      "No SIC category" state for a `NULL` category. No server-side pagination.
- [x] Implement the create/edit Bootstrap modal with every field editable, labels associated with inputs,
      focus moved into the modal and restored on close, retained values after errors, and server validation
      authoritative.
- [x] Label upload "Import / Update Mappings" and confirm before sending that uploaded codes will be added or
      updated and omitted codes will remain. Confirm deletion by naming the SIC code and explain that
      existing transaction assignments are not cleared.
- [x] Present outcomes distinctly: created/updated/unchanged/rejected counts; the authoritative
      `RejectedRows` total alongside the shown subset when `DiagnosticsTruncated` is true; the backup path on
      success or a prominent warning on failure; an `oversized` rejection stating the limit and that nothing
      was parsed; a header-only no-change message; and, when `MappingCommitted` is true with post-commit
      warnings, a clear statement that mappings were saved and should not be blindly re-uploaded.
- [x] Hide the recategorized count at this checkpoint, since the no-op collaborator makes it structurally
      zero and an unlabelled zero would read as "nothing matched" (BR-U2-46).
- [x] Escape all user-controlled descriptions and diagnostics through template escaping or DOM `textContent`;
      never assemble raw HTML from mapping values.
- [x] Disable duplicate submission while a request is in flight and always restore controls afterward. Use
      accessible alert semantics and never auto-retry a mutation.
- [x] Use the approved stable identifiers `sic-mapping-create-button`, `sic-mapping-download-link`,
      `sic-mapping-upload-input`, `sic-mapping-upload-button`, `sic-mapping-form`,
      `sic-mapping-form-submit-button`, `sic-mapping-delete-confirm-button`, and `sic-mapping-status-alert`,
      suffixing row actions with the persistent mapping ID.
- [x] Trace to US-04, US-10, US-11; BR-U2-33 through BR-U2-37, BR-U2-40/46; NFR-U2-UX-01, NFR-U2-SEC-01;
      NFRP-U2-07.

### Step 11 — Production-only formatting and static verification

- [x] Run `gofmt` on changed production Go files.
- [x] Run `go build ./...` and `go vet ./...` without creating or modifying tests.
- [x] Run the existing test suite only as regression feedback. Do not author, modify, delete, skip, or
      weaken any test. Report a UOW-1 test that fails against corrected behavior as a handoff finding.
- [x] Run `git diff --check`.
- [x] Inspect the production diff for sensitive logging, hardcoded user paths, non-parameterized SQL,
      unescaped template output, global state, layer inversion, a gate held across an early return, a missing
      `RemoveAll`, and files outside UOW-2.
- [x] Verify no duplicate replacement file was created and that `ReplaceAll` is not called by any merge path.
- [x] If a production failure is found, fix production only within the preceding approved steps and rerun.

### Step 12 — Production documentation and traceability summary

- [x] Create `aidlc-docs/construction/sic-mapping-management/code/production-summary.md` listing production
      files modified and created, implemented contracts, commands run with results, known limitations, and
      traceability to US-04, US-05, US-10, and US-11.
- [x] Record the F-14 and F-15 corrections and the diagnostic-cap validity change explicitly, including their
      effect on the shared UOW-1 startup path.
- [x] State explicitly that the production role authored no verification tests or fixtures.
- [x] Record any approved-plan discrepancy rather than silently expanding scope.

### Step 13 — Prepare independent review/test handoff

- [x] Create `aidlc-docs/construction/sic-mapping-management/code/independent-review-handoff.md` containing
      approved artifact paths, the production revision identifier, exact production-file scope, known
      limitations, and commands already run.
- [x] Require review against approved artifacts rather than production reasoning.
- [x] Require file/line findings with severity, acceptance-criteria traceability, and a final PASS/FAIL.
- [x] Note that UOW-1 behavior changed on the shared validation path, so UOW-1 regression coverage must be
      re-run and the F-15 and diagnostic-cap corrections independently verified.
- [x] Do not create the independent review artifact or prescribe exact test implementations.

### Step 14 — Independent review and verification gate

- [ ] In a separate provider session, invoke the repository's independent review/test role with the handoff
      and approved artifacts.
- [ ] Require independently authored tests for: admission timeout and acquisition races; gate release on
      every failure path; a cancelled waiter performing no late mutation; the gate held through the
      collaborator; merge atomicity and rollback; omission, idempotency, and header-only no-op; created/
      updated/unchanged counts; committed-with-warning on reload and collaborator failure; category
      foreign-key failure; bounded diagnostics past 50 entries with `DiagnosticsTruncated` and authoritative
      `RejectedRows`; the F-15 classification across validation, startup, and upload; exact upload size
      bounds including exactly-at-limit; multipart cleanup and extra/missing file rejection; backup
      collision, write failure, close failure, and partial cleanup; numeric ordering; and CSV round-trip.
- [ ] Require independently owned `pgregory.net/rapid` properties for merge idempotency, omission
      preservation, and count invariants under the approved Q7 scope, with shrinking and replay evidence.
      CSV round-trip and category resolution remain examples.
- [ ] Require race-detector evidence, now triggered because UOW-2 introduces concurrent state
      (NFR-U1-TEST-04), run separately from performance acceptance.
- [ ] Require the approved merge benchmark: 100,000 pre-existing codes; an upload of 100,000 codes split
      25,000 new / 25,000 changed / 50,000 unchanged, leaving 25,000 pre-existing codes omitted; 100
      categories; file within 10 MiB; pre-state restored per run; successful backup and the no-op handoff
      included; measured from service entry to returned result excluding fixture setup; one warm-up, at
      least five runs, median at most ten seconds on the recorded reference environment.
- [ ] Require the unpaginated 1,000-mapping page verified as usability and functional evidence with no
      invented latency threshold.
- [ ] Production findings return to the production role; the independent role alone corrects tests that
      contradict approved artifacts and documents why.
- [ ] Repeat review after material production fixes until required tests and performance targets pass and no
      Blocking/High findings remain.
- [ ] Gate closes only when
      `aidlc-docs/construction/sic-mapping-management/code-review/independent-review.md` exists and reports
      final status PASS.

---

## Story Completion Checklist

- [x] US-04 — Dedicated page, CRUD API, current categories per request, explicit confirmed deletion.
- [x] US-05 — Shared normalization, validation, database-authoritative uniqueness, nullable category.
- [x] US-10 — Five-column export in numeric order, header-only when empty, fixed download filename.
- [x] US-11 — Whole-file validation, best-effort backup, atomic merge/upsert, affected-code handoff.

## Exit Criteria

- [ ] Every production step and story checkbox is complete.
- [ ] Production build, vet, format, and existing regression tests pass.
- [ ] Production summary and independent handoff are complete.
- [ ] Independent review/test provider has authored and run required verification.
- [ ] Performance, property, and race evidence is recorded.
- [ ] Independent review final status is PASS.
- [ ] No required test fails and no Blocking/High finding remains.
- [ ] User explicitly approves completed UOW-2 Code Generation before the workflow advances.

## Out of Scope

- UOW-3 work: categorization execution, text-first priority, manual-assignment protection, US-06 transaction
  SIC display, US-12 modal mapping creation, and the real collaborator's processing/cancellation budget.
- Deferred independent findings F-04 (structural CSV stop) and F-05 (`sqliteDSN` `?` handling).
- New `config.json` settings, configuration migration, schema migration, backup pruning, server-side
  pagination, and any new production dependency or frontend framework.
