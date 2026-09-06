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
