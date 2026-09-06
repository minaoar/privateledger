# NFR Design Patterns — UOW-2 SIC Mapping Management

Status: APPROVED by the user on 2026-09-06. No implementation or test execution claimed.
All NFR identifiers below refer to the approved UOW-2 requirements unless labelled UOW-1.

## NFRP-U2-01 — Context-aware mutation admission

Use an instance-owned capacity-one channel as a mutual-exclusion gate in SICMappingService.
Acquire by selecting between the gate, request cancellation, and a timeout context. Never
spawn a goroutine that blocks on a mutex and might acquire it after its caller has returned.
After acquisition, recheck cancellation/deadline before work, then defer release exactly once.
All CRUD and upload paths use this gate. Startup seeding completes before HTTP serving and
retains its existing entry point; no competing startup worker is introduced.

Use a named five-second default admission timeout, supplied through the service constructor.
Tests may inject a shorter positive duration. This is code-level configuration; add no fields
to config.json and no configuration migration. No FIFO fairness promise is made.

This timer bounds admission only. Acquire after HTTP intake but before CSV validation so
competing requests cannot concurrently accumulate parsed candidate sets in this service.
Hold through validation, pre-state read, backup, mutation, reload, and handoff. The five-second
SQLite busy timeout remains separate and unchanged. No application-level automatic retries.

A timed-out waiter receives HTTP 503 with stable error code `mapping_busy`, safe retry text,
and `Retry-After: 1`. It performs no backup or mutation. This is a transport/service error,
not another CSV import outcome. A client-cancelled waiter performs no work; if a response
is still writable, use HTTP 408 with `request_cancelled`. Never report a saved result for
an operation that never acquired admission.

Supports CON-01, REL-01, MAINT-01, TEST-03.

## NFRP-U2-02 — Cancellation and post-commit work

Check request cancellation while waiting for admission and at service phase boundaries before
persistence. Use request context for newly added database transaction operations so cancellation
before commit rolls back the transaction. Keep existing UOW-1 validation and repository APIs
unless the new write path actually needs an extension; no general context-propagation refactor
or custom context-checking CSV reader is required for the already bounded input.

After commit, set saved-state and synchronously finish reload/handoff while retaining admission.
UOW-2 wires the immediate no-op collaborator. Do not add a separate post-commit timeout setting,
worker, or cancellation framework. A returned reload failure skips recategorization; a returned
handoff failure produces a saved warning. Browser cancellation cannot change a committed result
into a rollback claim. Log the saved outcome if the browser can no longer receive it.

For this checkpoint the handoff has no processing budget because it does no work. UOW-3 must
choose and verify cancellation/budget behavior when it introduces real recategorization. Until
then, the gate remains held until the collaborator returns; waiting callers retain their
five-second admission bound. This satisfies bounded admission without claiming that arbitrary
lock-holder work is time-limited. The ten-second checkpoint benchmark remains unchanged.

Supports CON-01, REL-01, PERF-01, UX-01, TEST-03.

## NFRP-U2-03 — Snapshot, prepared merge, and accurate counts

Under admission, validate first, then load one pre-state snapshot for both diff and backup.
Index pre-state by canonical SIC for linear expected-time diff. Use one repository-owned
SQLite transaction and a prepared parameterized INSERT ... ON CONFLICT(sic_code) DO UPDATE
for all candidates. Preserve mapping identity and creation metadata on updates; never use
replacement-delete semantics. Close/check the statement and commit before reporting created,
updated or unchanged counts. Roll back on any earlier error. Empty candidates produce a
successful no-change result through the same transaction contract; the approved backup
attempt still occurs. Explicit deletion remains a separate CRUD operation.

New and category-changed codes with non-empty post-state categories form a de-duplicated
handoff set. Unchanged, description-only, cleared-category, deleted and omitted codes do
not enter it. Reload after successful writes; invoke the collaborator once only for a
non-empty affected set. CRUD follows the same saved-state and warning policy as merge.

Serialization covers mapping service mutations, not category edits or another process.
Foreign keys remain authoritative if categories change concurrently. Do not imply that the
service gate freezes all application state. Category resolution retains its approved
validation-time semantics; no global category lock is introduced. Read-only page/export
requests do not take the mutation gate and never observe a partial merge transaction.

Supports REL-01, PERF-01, MAINT-01, TEST-01–03.

## NFRP-U2-04 — Bounded multipart intake

Use the standard net/http multipart facilities already available through Gin. Before any form
parsing, wrap the request body with http.MaxBytesReader using the shared 10 MiB CSV limit plus
1 MiB for framing. Call ParseMultipartForm with a 1 MiB memory threshold; that threshold controls
spilling to temporary files, not total accepted size. Defer MultipartForm.RemoveAll whenever
form state exists, including error paths, and close any opened file.

Require exactly one uploaded file named `file`; reject missing/extra files. Ordinary unused
form values may be ignored because the complete body is bounded. The multipart parser establishes
the actual file size; reject files over the shared 10 MiB limit before calling ValidateCSV.
Do not trust client Content-Length as enforcement. An exactly-at-limit CSV remains eligible
with ordinary framing. Body-limit errors return 413 oversized; malformed forms return 400.

Pass the opened multipart file to the service as a reader. Reuse the parser's bounded
memory/temp-file storage instead of copying the upload into another buffer or writing a
custom multipart loop. Uploaded filenames never determine backup paths. Byte limits are
per request, not a global process-memory guarantee under arbitrary client concurrency.

Page/API text uses escaped templates or DOM textContent, not raw HTML assembled from mapping
values. Download uses the fixed filename sic_mappings.csv and standard CSV escaping.

Supports SEC-01–02, SCALE-01, UX-01, TEST-01.

## NFRP-U2-05 — Bounded diagnostics without weakening validation

Promote the 50-entry limit to a shared named constant used by the validator and startup
reporting. Retain no more than 50 errors; set DiagnosticsTruncated when dropping any error.
Count invalid rows once per row independently of error count. Use an explicit row-invalid
flag or an uncapped diagnostic counter to determine validity, never len(report.Errors).
The current implementation compares list lengths; preserving that test after capping would
misclassify later invalid rows as valid. A row with multiple errors is one rejected row.

After the cap, continue validation and duplicate detection for all parseable rows; rejected
counts remain authoritative for rows actually processed. Preserve F-04's deferred structural
CSV stop behavior: do not claim counts for unread rows after an unrecoverable parse stop.
Whole-file acceptance still requires EOF with no invalid rows. Fixed safe diagnostic text
must not grow with arbitrary field values.

ValidateCSV sets persistence_failed on category load failure and read_failed on actual reader
failure. Startup/upload wrappers preserve the classified outcome instead of overwriting it.
The startup fatal/non-fatal boundary stays intact. This is the F-15 production correction;
F-04/F-05 remain deferred. Reuse the existing report/error types and add the shared truncation
field once, with stable JSON spelling carried through upload and startup.

Supports SEC-03, REL-03, MAINT-01, TEST-01; BR-U2-40–42.

## NFRP-U2-06 — Complete local backups with exclusive creation

Serialize the pre-state with encoding/csv into a unique exclusive-created file beside the
configured database, using a filename shaped `sic_mappings.backup-<UTC timestamp>-<random>.csv`.
Use os.CreateTemp with that prefix/suffix and mode 0600; the random suffix prevents timestamp
collisions and existing files/symlinks are never opened for overwrite. Use filepath.Join
and the injected application-data directory; never accept a path from upload content.

Check CSV writes, Flush/Error, and file Close. Publish BackupPath in the result
only after all succeed. On failure close and remove only the newly created incomplete file;
if cleanup also fails, report that an incomplete file remains using a safe warning. Never
claim that path as a completed backup. Do not delete or prune prior completed backups.
A crash can leave an unadvertised partial file; no crash-recovery SLA or automatic restoration
is promised. Backup failure does not stop merge and remains in the final result even if
merge later fails. The pre-state snapshot and counts do not depend on successful backup I/O.

Supports REL-02, SEC-01, PERF-01, TEST-01.

## NFRP-U2-07 — Stable transport and UI outcomes

| Condition | HTTP/result |
|---|---|
| Malformed body/route input | 400, safe input error |
| CSV/domain validation | 422, validation report, no saved counts |
| Duplicate normalized mapping on CRUD | 409, stable conflict |
| Missing mapping ID | 404 |
| Upload/request byte overflow | 413, oversized |
| Admission expiration | 503, mapping_busy; Retry-After: 1 |
| Pre-commit infrastructure failure | 500, contextual safe failure; no saved counts |
| Recognized SQLite busy exhaustion before commit | 503, database_busy; no automatic retry |
| Create committed | 201 with saved-state result and any warnings |
| Update/delete/upload committed | 200 with saved-state result and any warnings |

Use a common mutation-result saved flag/warning contract for CRUD, and the approved extended
import report for upload. Delete returns a body so post-commit reload warnings are visible.
Preserve backup outcome on failed merge responses. Never emit a generic 500 after a known
successful commit. Error mapping belongs in handlers; classification belongs in services/
repositories. Avoid matching arbitrary error strings for uniqueness or busy detection.

The page keeps entered values after errors, restores disabled controls in completion paths,
uses accessible status alerts, shows authoritative rejected counts and truncation status,
and distinguishes saved warnings from failures. Hide the checkpoint recategorized count.
Show confirmation before upload/delete; no automatic retry of mutation requests.

Supports REL-01, UX-01, TEST-01.

## NFRP-U2-08 — Verification and performance evidence

Independent tests use temporary databases/directories and injected collaborators to coordinate
admission, cancellation, failures and exact handoff sets. Test timeout/acquisition races,
release after every failure, waiting cancellation with no late mutation, committed warnings,
category foreign-key failure, bounded diagnostics past 50 entries, F-15, exact upload bounds,
multipart cleanup, extra files, and backup collisions/write/close failures. Use real temporary files for ordinary backup success/failure cases. Add a small writer/closer
seam only if needed to exercise write/close failures; do not design a filesystem abstraction
covering every os call. No global test hooks or new framework are required.

Retain the approved example matrix and generated merge/omission/idempotency properties using
rapid, shrinking and replay evidence. CSV round-trip and category resolution remain examples
under the approved Q7 scope. Run race evidence separately from performance acceptance.

Benchmark fixture: 100,000 pre-existing codes; upload 100,000 codes split into 25,000 new,
25,000 changed and 50,000 unchanged, leaving 25,000 pre-existing codes omitted. Use 100
categories and short descriptions so the file is within 10 MiB. Restore pre-state for each
run. Include successful backup and the no-op handoff in the service-entry-to-result boundary;
exclude setup. One warm-up, at least five runs, median at most ten seconds on the recorded
reference environment. Record sizes and environment differences. Verify the unpaginated
1,000-mapping page as usability/functional evidence without an invented latency threshold.

Required build, test, vet, race and benchmark results belong to the separate-provider
independent review. No test authorship or production ownership is transferred by this design.

Supports PERF-01, SCALE-01, TEST-01–04. PBT-09 retained; PBT-02/03 scoped as approved;
PBT-07/08 carried into execution. Other PBT rules remain advisory under Partial mode.
