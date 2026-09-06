# Production Summary — UOW-2 SIC Mapping Management

Production role: Claude (this session). Verification tests, fixtures, test-only dependencies,
benchmarks, and the independent review artifact were **not** authored here and remain owned by a
separate-provider session.

Stories implemented: US-04, US-05, US-10, US-11.

## Production Files

### Modified

| File | Change |
|---|---|
| `internal/model/sic_mapping.go` | Shared size/diagnostic bounds, sentinel errors, `merged` outcome, `DiagnosticsTruncated`, bounded `AddError`, `SICMappingInput`, `SICMappingPageData`, `SICMappingMutationResult`, `SICMappingImportResult` |
| `internal/service/sic_mapping_service.go` | Row-validity fix, diagnostic bounding, F-15 classification, admission gate, collaborator contract, page data, CRUD, export, backup helper, merge upload |
| `internal/repository/sic_mapping_repo.go` | `MergeAll` atomic upsert, numeric ordering, driver-error classification, sentinel not-found |
| `internal/handler/page_handler.go` | `SICMappings` page reading through the service |
| `cmd/privateledger/main.go` | Management-service wiring, no-op collaborator, page + six API routes, shared constants |
| `cmd/privateledger/web/templates/layout.html` | SIC Mappings navigation entry |
| `API_ROUTES.md` | Six endpoints, response shapes, and stable error-code table |

### Created

| File | Purpose |
|---|---|
| `internal/handler/sic_mapping_handler.go` | Mapping API with bounded multipart intake and stable status mapping |
| `cmd/privateledger/web/templates/sic_mappings.html` | Management page, modals, and result presentation |

No schema migration was needed: `sic_mapping` already had the required columns and `UNIQUE` constraint.
No new production dependency was added.

## Implemented Contracts

- **Admission gate** — capacity-one channel in `SICMappingService`, five-second timeout injected via
  `NewSICMappingManagementService`. Acquire selects over gate / cancellation / timeout, rechecks
  cancellation after acquisition, and releases through a `sync.Once` deferred immediately. No goroutine
  can acquire after its caller returned. `NewSICMappingService` keeps its UOW-1 two-argument signature.
- **Merge** — `MergeAll` uses one transaction and one prepared
  `INSERT ... ON CONFLICT(sic_code) DO UPDATE`, preserving mapping ID and `created_at`. It never calls
  `ReplaceAll` and never deletes. An empty candidate set commits a successful no-change transaction.
- **Counts and handoff** — one pre-state snapshot feeds both the diff and the backup, so backup failure
  cannot alter counts. Affected codes are de-duplicated and sorted: created-with-category, or
  category-changed-to-a-different-non-empty-category. Description-only, cleared-category, unchanged, and
  omitted codes are excluded.
- **Post-commit** — reload failure skips recategorization; either failure becomes a warning on a
  successful result with `MappingCommitted=true`.
- **Backup** — `os.CreateTemp` with a `sic_mappings.backup-<UTC>-<random>.csv` pattern at mode 0600.
  Write, flush, and close are all checked; the path is published only after all succeed; a partial file
  is removed, and if removal fails the caller is told a file remains.
- **Upload intake** — `http.MaxBytesReader` at 10 MiB + 1 MiB framing before parsing, 1 MiB memory
  threshold, `RemoveAll` deferred on every path including errors, exactly one `file` part required, and
  the parsed size re-checked against the shared bound. Client `Content-Length` is never trusted.

## Corrections to Shared UOW-1 Behavior

These change the startup seed path as well as upload. Independent re-verification of UOW-1 coverage is
required.

1. **F-15** — `ValidateCSV` now sets `persistence_failed` when `categoryRepo.GetAll()` fails and
   `read_failed` only on genuine reader failure; `ImportFileIfPresentWithReport` preserves the classified
   outcome instead of overwriting it with `read_failed`.
2. **Row validity** — validity is now tracked by an explicit per-row flag
   (`sicRowValidator.invalid`) instead of comparing `len(report.Errors)`. This was a prerequisite for
   bounding diagnostics: with the cap in place, the old length comparison would have classified every
   invalid row after the 50th as valid and merged it. Verified empirically — see below.
3. **F-14 / BR-U2-39** — the 10 MiB bound and the 50-diagnostic bound are now single domain constants.
   `maxSICMappingSeedSize` and `maxSICSeedDiagnostics` are retained as aliases so existing call sites and
   identifiers keep working.
4. **F-13 / BR-U2-40** — diagnostics are bounded at the accumulation site, with `DiagnosticsTruncated`
   set when entries are dropped. `RejectedRows` stays authoritative.
5. **F-16 / BR-U2-42** — `merged` added to the single shared outcome set.

F-04 and F-05 remain deferred and untouched.

## Commands Run

| Command | Result |
|---|---|
| `gofmt -l ./cmd ./internal` | clean |
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `git diff --check` | clean |
| `go test -count=1 ./...` | 3 pre-existing tests fail; see below. All others pass. |

## Pre-Existing Tests Now Failing

Production did not modify any test file. All three failures are tests whose assertions encode behavior
the approved UOW-2 design deliberately replaces. They are reported for the independent role rather than
edited here.

| Test | Assertion | Why it now fails |
|---|---|---|
| `internal/repository/sic_repo_test.go:483` `TestSICMappingRepository_UniqueCodeRejected` | error text contains `"UNIQUE"` | The repository now returns `model.ErrSICMappingDuplicate` instead of leaking driver text. NFRP-U2-07 requires stable classification rather than error-string matching. Suggested: assert `errors.Is(err, model.ErrSICMappingDuplicate)`. |
| `cmd/privateledger/startup_sic_diagnostic_volume_test.go:168` `TestStartupDiagnosticVolumeForLargeInvalidSeed` | `len(report.Errors) == invalidSeedRows` (100,000) | Asserts unbounded retention, which BR-U2-40 closes. Its second assertion compares bounded output against an "unbounded shape" that no longer exists. |
| `cmd/privateledger/startup_sic_diagnostic_volume_test.go:239` `TestStartupDiagnosticRetentionForLargeInvalidSeed` | `len(report.Errors) == invalidSeedRows` | Same: it measures the F-13 defect that BR-U2-40 fixes. |

The behavior these tests otherwise guard still holds: for a 100,000-row fully invalid seed the run still
reports `Outcome=invalid`, `RejectedRows=100000`, `ImportedRows=0`, and a mapping count of 0.

## Production Smoke Verification

Run against a temporary database on an isolated port; the user's real database and port 8844 were not
touched. This is production verification, not test authorship — no test file was created.

| Check | Result |
|---|---|
| `/sic-mappings` page render | 200 (template parses and executes) |
| Create with `"05412"` | normalized to `5412`, `mapping_committed: true` |
| Duplicate create | 409 `duplicate_sic_code` |
| Non-digit code | 422 `validation_failed` |
| Delete unknown ID | 404 |
| Numeric ordering | `999, 1000, 5412` — lexicographic would give `1000, 5412, 999` |
| Merge upload | created 2, updated 0, unchanged 1; backup path returned |
| Omitted codes | `999`, `1000` retained after a merge that did not mention them |
| Re-upload same file | created 0, updated 0, unchanged 3 (idempotent) |
| Header-only upload | `total_rows: 0`, committed, `merged` |
| Backup files | mode `-rw-------`; three backups in the same second did not collide |
| 80 invalid + 1 valid rows | 422; `rejected_rows: 80` (authoritative), 50 diagnostics, `diagnostics_truncated: true`, and the valid row was **not** merged |
| 11 MiB upload | 413 `oversized` |
| Update clearing category | committed, `category_id: null` |
| CSV export | five columns, numeric order, header-only when empty |

The 80-invalid-row case is the direct regression check for correction 2 above.

## Known Limitations

- No verification tests exist for any UOW-2 behavior; all of it is owned by the independent role.
- The recategorization collaborator is a no-op, so `recategorized_rows` is structurally zero and the page
  deliberately does not display it.
- Driver-error classification matches SQLite text for unique-violation and busy detection. The match is
  confined to the repository, but it is still string-based and is worth review.
- Backup files accumulate without pruning, as approved under NFR-U2-REL-02 / Q4 B.
- The page is unpaginated by design; the approved 1,000-row usability verification has not been run.
- No performance benchmark was run here; PERF-01 evidence belongs to the independent role.

## Plan Discrepancies

None. All Step 1–11 checkboxes were executed as approved. One item worth flagging for review: Step 1
called for deleting the duplicated literals, and `maxSICSeedDiagnostics` was initially removed from
`main.go`, which broke compilation of an existing test that referenced it. It was restored as an alias of
the shared constant, which satisfies the single-source-of-truth intent without deleting an identifier
existing tests use.

---

# Revision 2 — Independent Review Findings Addressed

Date: 2026-09-06. Responds to `code-review/independent-review.md` (gate BLOCKED, findings U2-F01
through U2-F08, all OPEN). Every finding was reproduced against the reviewer's tests before being
fixed. No test file, fixture, or the review artifact was modified by the production role.

| Finding | Severity | Resolution |
|---|---|---|
| U2-F01 | Medium | The upload handler now counts file parts across **every** form file field and requires exactly one, in `file`. Previously it inspected only `MultipartForm.File["file"]`, so a second upload under another field name passed through. |
| U2-F02 | Medium | `Download` renders into a buffer and only commits headers and a 200 after the export succeeds; a repository failure now returns 500 with no attachment. My original comment claiming headers were already committed was wrong — no response bytes are written on that path. |
| U2-F03 | Medium | Added `ensureActive` checks at the approved phase boundaries: before the write in create, update, and delete, and in merge after validation and again before the backup. A caller who gave up while blocked on a database connection no longer mutates, and a merge cancelled during validation no longer leaves a backup file. |
| U2-F04 | Medium | `NewSICMappingManagementService` now panics on a nil collaborator instead of substituting the no-op. The design requires that nil never masquerade as successful work; the substitution would have hidden a UOW-3 wiring error. The UOW-1 compatibility constructor still passes an explicit no-op, and `main.go` still wires the checkpoint explicitly. |
| U2-F05 | Medium | The three `fetch` catch paths now report an **unknown** outcome via `setUnknownOutcome`, stating the change may or may not have been saved and directing the user to refresh and check before retrying. A lost response says nothing about whether the server committed. |
| U2-F06 | Medium | Removed every timed `window.location.reload`. Results persist until dismissed, with an explicit "Refresh the mapping list" control. Clean CRUD success still reloads immediately because there is nothing to preserve. The failed-merge branch now also surfaces `backup_path`, not just `backup_warning`. |
| U2-F07 | Low | Classification now uses the driver's typed `*sqlite.Error` result codes (`SQLITE_CONSTRAINT_UNIQUE`, `SQLITE_CONSTRAINT_PRIMARYKEY`, `SQLITE_BUSY`) instead of English substrings, and `MergeAll` routes its `BeginTx`, `Prepare`, `Close`, and `Commit` failures through the same classifier. |
| U2-F08 | Low | Added `logSavedOutcome`, which records a committed change through `slog` when the caller's context is already done, so a durable result whose response was never delivered still leaves a record. Only operation, counts, and safe outcome fields are logged. |

## Files Modified in Revision 2

- `internal/service/sic_mapping_service.go` — phase-boundary cancellation, nil-collaborator rejection, saved-outcome logging
- `internal/handler/sic_mapping_handler.go` — buffered download, cross-field file counting
- `internal/repository/sic_mapping_repo.go` — typed driver-code classification, consistent `MergeAll` boundaries
- `cmd/privateledger/web/templates/sic_mappings.html` — unknown-outcome reporting, persistent results, backup path on failed merge

## Revision 2 Verification

| Command | Result |
|---|---|
| `gofmt -l ./cmd ./internal` | clean |
| `go build ./...`, `go vet ./...` | pass |
| `go test -short -count=1 ./...` | **all packages pass** |
| `go test -count=1 ./...` (long suite included) | **all packages pass** |
| `go test -race -short -count=1 ./...` | **all packages pass, no data races** |

The five previously failing top-level tests now pass unmodified:
`TestReviewU2HandlerUploadBoundaries/extra_other_field`, `TestReviewU2HandlerDownloadFailure`,
`TestReviewU2CancelledValidationDoesNotBackup`,
`TestReviewU2CRUDCancellationBeforePersistence/create` and `/update`,
`TestReviewU2NilCollaboratorIsNotSilentSuccess`.

Smoke re-verified on an isolated port: page render 200, download 200, create 201, upload 200, and a
request carrying a second file under `other` correctly rejected with 400.

## Notes for Re-Review

- U2-F04 is resolved by panicking at construction. The reviewer's test accepts either construction
  rejection or a use-time error; a panic was chosen because nil wiring is a programmer error
  discoverable at startup rather than a runtime condition to be reported per request.
- U2-F07 declares the three SQLite result codes as local named constants rather than importing the
  driver's large platform-specific constants package. The values are stable SQLite C API codes.
  `internal/repository` now imports `modernc.org/sqlite` directly, which is a new import for that
  package though not a new module dependency.
- Verification gaps 1 through 5 in the review remain open and are unchanged by this revision. Gap 1
  (a real `writeBackup` Close fault) would need the small writer/closer seam the review describes;
  it was not added, since the review assigns that decision to production only "if needed".

---

# Revision 3 — Documentation Finding U2-R2-F01

Date: 2026-09-06. The Revision 2 re-review returned **PASS** with one Low documentation finding.

| Finding | Severity | Resolution |
|---|---|---|
| U2-R2-F01 | Low | `NewSICMappingManagementService`'s doc comment still said a nil collaborator "falls back to the explicit no-op", contradicting both the adjacent implementation comment and the actual panic. Rewritten to state that nil is rejected and that callers wanting checkpoint behaviour must pass `NewNoopSICRecategorizationCollaborator()` explicitly. |

Comment-only change to `internal/service/sic_mapping_service.go`; no executable behaviour altered.

Verification: `gofmt` clean; `go build ./...` and `go vet ./...` pass; `go test -short -count=1 ./...`,
the full `go test -count=1 ./...` including long tests, and `go test -race -short -count=1 ./...` all
pass across all seven packages with zero data races.

All findings from both review passes (U2-F01 through U2-F08, U2-R2-F01) are now closed.
