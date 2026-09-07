# NFR Requirements — UOW-2 SIC Mapping Management

## Status and Inputs

Approved by the user on 2026-09-06. Based on the approved UOW-2 functional design, project NFR1–NFR6,
UOW-1 NFR precedent, and the completed NFR plan answers: Q1 A, Q2 A, Q3 A, Q4 B,
Q5 B, Q6 C, Q7 A, Q8 A, Q9 A.

These requirements cover local mapping CRUD, page/API access, CSV export and atomic merge,
backup outcomes, and the UOW-3 collaborator boundary. They do not implement categorization.

## Performance and Scale

### NFR-U2-PERF-01 — Blocking merge measurement

Carry forward UOW-1's ten-second, 100,000-row precedent per Q9. A valid upload of
100,000 rows, no larger than 10 MiB, must complete validation, pre-state capture, successful
local backup, atomic merge, reload, and the checkpoint no-op handoff within ten seconds
on the recorded UOW-1 reference environment. This is an approved UOW-2 acceptance target, not a previously measured result.

Use a deterministic pre-state and upload containing new, changed, and unchanged codes,
plus pre-existing codes omitted from the upload. Record the distribution, file size, and
database size. Measure from entry into upload service processing to the returned result;
exclude fixture creation, database opening, browser/network transfer, and competing requests.
Restore the same pre-state for each run. Exclude one warm-up and use the median of at least
five uninstrumented runs. Record CPU, logical cores, RAM, OS, Go version, storage, and
absence of intentionally competing heavy work. If that environment is unavailable, report
the difference for review instead of silently claiming equivalent acceptance evidence.

The target includes backup I/O. A backup-failure path cannot substitute for the successful
backup benchmark. UOW-3 must assess the real recategorization cost separately; this target
does not promise ten-second completion for arbitrary collaborator workloads or hardware.
Page load and export have no blocking latency benchmark under Q5 B.

### NFR-U2-SCALE-01 — Page coverage and capacity

Verify the complete, unpaginated page with 1,000 mappings, including numeric ordering,
current category choices, edit/delete controls, and readable feedback. Q6 C defines the
page usability fixture; Q5 B means it creates no numeric page-latency gate. It is not a
mapping-count limit. Larger mapping sets remain valid subject to local resources and the
CSV byte limit. Do not truncate stored mappings or exports at 1,000 rows.

## Concurrency and Reliability

### NFR-U2-CON-01 — Bounded mutation admission

All CRUD and upload mutations through the injected service instance must share serialization
covering pre-state capture, backup, mutation, reload, and affected-code handoff. A competing
caller waits only up to a configured finite timeout; expiration returns a stable busy/retry
result without performing that caller's mutation or backup. Cancelled waiters must not later
acquire ownership and mutate in the background. Every completion/failure path releases ownership.

NFR Design must select the timeout value, configuration/default, context-aware admission
mechanism, and HTTP mapping. UOW-1's five-second SQLite busy timeout does not bound waiting
for this service lock. Preserve the approved lock scope across the collaborator: do not
release it early simply to meet a deadline. Document the separate admission, database, and
collaborator time budgets and cancellation behavior before code generation. A bounded wait
for a competing caller is not a guarantee of a bounded lock-holder runtime.

### NFR-U2-REL-01 — Atomicity and authoritative outcomes

Validate the complete accepted upload before backup or mutation. One invalid row rejects
the upload; a persistence failure rolls back every row. Omitted mappings remain untouched.
Repeated identical uploads preserve mapping state and classify rows as unchanged; a header-only
upload makes no mapping changes. Compute merge counts from a serialized pre-state independently
of backup success. Claim persisted counts only after commit.

After commit, reload or collaborator failure must return success with explicit saved-state
and post-commit warnings, never an ordinary failure implying rollback. Apply the same truthful
saved-state distinction to CRUD side-effect failures. Cancellation after a successful commit
cannot turn that commit into a rollback claim. NFR Design must define how this distinction
survives each transport/result path.

### NFR-U2-REL-02 — Backup integrity and retention

After validation, attempt a complete pre-upload CSV backup beside the configured database.
Return a backup path only after the complete write succeeds. Never overwrite an earlier
backup on timestamp collision, and never advertise a partial file as a recoverable backup.
NFR Design must specify filename uniqueness, write/close failure handling, and partial-file
cleanup. Backup failure remains non-blocking, with a prominent non-empty warning retained
even if the later merge fails.

Retain all completed backups with no automatic count/age pruning, per Q4 B. Disk growth is
an accepted local operational tradeoff. A backup contains mappings only and is not a complete
financial database recovery mechanism. No new uptime, failover, or recovery-time SLA is introduced.

### NFR-U2-REL-03 — Accurate infrastructure error classification

Bring independent finding F-15 into UOW-2 scope per Q8 A. A category repository read failure
during CSV validation must be classified as `persistence_failed`, with no mutation, rather
than `read_failed`. Reserve `read_failed` for CSV/file read failures. Preserve contextual
errors and existing startup fatal/non-fatal behavior when correcting the shared path.
This refines the shared outcome semantics without adding another outcome enumeration.
F-15 remains open until production correction and independent verification; F-04 and F-05
remain deferred.

## Privacy and Resource Safety

### NFR-U2-SEC-01 — Local processing and safe presentation

Keep mappings, categories, uploaded bytes, backups, validation, and categorization handoffs
local. Preserve parameterized SQL, foreign keys, and existing application access/deployment
conventions. Introduce no external service, telemetry, authentication system, or cloud dependency.
Use escaped template/DOM text for user-controlled descriptions and diagnostics. Do not log
complete uploads, transaction data, or unnecessary category/description contents; retain safe
operation, row, field, stable code, count, and contextual error information through `slog`.
Resolve backup paths from the injected database directory, never the uploaded filename.

**Amended 2026-09-07 by UOW-4 NFR Requirements (Q1 A, NFR-FQ1 A).** This requirement predates the
decision to name an unresolved `Category_Name` in a diagnostic, so its prohibition on echoing file
content is narrowed by NFR-U4-SEC-01: a diagnostic may carry a file-supplied category name, bounded to
64 runes and stripped of control characters through one shared helper. NFR-U4-SEC-01 extends the same
bound to database-sourced category names, which this requirement had implicitly treated as safe.

The DOM-escaping clause above is **unchanged and still binding**: rendering remains `textContent`.

**Further amended 2026-09-07 after independent review (U4-R-F01).** The sentence that stood here said
echoed names are never written to a log. That was true when written and was made false later the same
day by NFR Design Q1 A, which requires the bounded message on the startup seed path. The accurate rule:
**upload paths log no diagnostic message; the startup seed path logs the bounded one** (DP-U4-07). The
prohibition on logging upload contents is otherwise unchanged.

### NFR-U2-SEC-02 — Shared upload boundary

Startup seed and HTTP upload share one named 10 MiB file-size constant. Enforce the upload
limit before CSV parsing, including when client size metadata is absent or inaccurate. Do
not buffer an unbounded HTTP body to determine its size. Report `oversized` and no mutation.
NFR Design must distinguish multipart overhead from CSV bytes, bound request consumption,
and define cleanup of temporary upload resources. Exactly-at-limit input remains eligible
for validation. Reuse `ValidateCSV` and its existing report/error types.

### NFR-U2-SEC-03 — Diagnostics bounded at accumulation

Retain at most 50 row diagnostics in the producing service, sharing the bound with startup
reporting where applicable. Keep authoritative rejected-row counts and set
`DiagnosticsTruncated` whenever diagnostics are dropped. Do not collect all errors and slice
only for display. Messages must not echo arbitrary-length uploaded field values. Preserve
the distinction between row validation errors and an infrastructure/read failure; the count
cap must not hide a fatal operation outcome.

## Usability and Maintainability

### NFR-U2-UX-01 — Accessible, truthful feedback

Use the approved Bootstrap modal and server-rendered page with labelled inputs, associated
validation feedback, modal focus behavior, accessible alerts, and stable `data-testid` values.
Prevent duplicate submission while a request is pending and restore controls afterward.
Retain entered values after errors. Confirm deletion and import/update before sending their
requests. Explain retained omitted codes and preserved existing transaction assignments.

Clearly distinguish busy/no-change, invalid/no-change, rollback, saved-with-backup-warning,
and saved-with-post-commit-warning outcomes. Show authoritative rejected totals and truncated
diagnostic status. Hide the no-op collaborator's structural zero recategorization count at
the checkpoint. Do not expose internal implementation details as product instructions.

### NFR-U2-MAINT-01 — Existing architecture and compatibility

Preserve `handler -> service -> repository -> SQLite`, constructor injection in `main.go`,
embedded templates/assets, and the no-CGO single binary. Share domain normalization, CSV
validation/report types, and outcome values with UOW-1. No global coordinator or singleton.
Preserve optional empty-table startup seeding, existing databases/imports, and transaction
deduplication. Wire an explicit no-op collaborator for UOW-2; UOW-3 owns its real implementation.

## Verification and Ownership

### NFR-U2-TEST-01 — Independent examples and failure evidence

The independent provider must cover CRUD normalization/uniqueness/category references,
not-found results, numeric ordering, live categories, five-column and header-only exports,
CSV escaping and export/re-upload examples, the complete category resolution table, duplicate
normalized rows, invalid-upload no-mutation, atomic rollback, omitted-code retention,
header-only merge, repeated merge counts, backup success/failure/collision, and post-commit
warnings. Verify the exact affected-code set and call count using an injected fake.

Include upload size boundaries and absent/untrusted size metadata; more than 50 rejected rows
with bounded retained diagnostics; F-15 classification in shared validation/startup/upload
paths; HTTP/UI outcome semantics; and the 1,000-row page fixture. Preserve existing regressions.
Use temporary files/databases only.

### NFR-U2-TEST-02 — Property scope selected by the user

Require generated merge idempotency and merge/omission invariants per Q7 A. For generated
valid pre-state S and upload U, post-state keys equal the union of their normalized keys,
uploaded editable values win, omitted records remain equal, and a second identical merge
leaves state unchanged with every uploaded row counted unchanged. Idempotency concerns
mapping state, not backup file creation. Exercise empty sets, disjoint/overlapping codes,
null/non-null and changed categories, unchanged values, and canonical code boundaries.

Use reusable domain generators with `pgregory.net/rapid`; retain shrinking, deterministic
seed/replay information, and normal test-runner/CI inclusion. Q7 A explicitly narrows this
unit's generated-property scope: CSV round-trip and category-resolution coverage remain
required examples, not additional generated suites. Record this scoped user override of the
broader PBT-02/PBT-03 rule wording; it does not disable the project's Partial configuration.

### NFR-U2-TEST-03 — Concurrency and performance gates

Require race-detector evidence because this unit introduces shared concurrent state.
Use coordinated concurrent CRUD/upload tests and a deliberately held fake collaborator to
prove serialized snapshots/counts/backups, bounded waiting without late mutation, cancellation,
and ownership release after errors. Do not infer service-lock behavior solely from SQLite
pragma values. Run performance acceptance separately without race instrumentation.
Required examples/properties, `go test ./...`, applicable race checks, build/vet checks, and
NFR-U2-PERF-01 must pass; record evidence in the independent review artifact.

### NFR-U2-TEST-04 — Cross-provider gate

Production changes belong to the production provider. A separate provider/session owns tests
and its independent review artifact. Production findings return to the production role.
Do not pass Code Generation while required tests/performance fail or Blocking/High findings
remain. Design amendments for F-13/F-14/F-16 still require implementation verification;
their design resolution is not proof that the production code is fixed.

## Traceability and Stage Compliance

| Source | Refinement |
|---|---|
| Q1; BR-U2-44 | CON-01, TEST-03 |
| Q2–Q3; BR-U2-39–42 | SEC-02–03, TEST-01 |
| Q4; BR-U2-23–27, 43, 45 | REL-01–02, UX-01 |
| Q5–Q6, Q9; UOW-1 performance precedent | PERF-01, SCALE-01 |
| Q7; NFR5–NFR6 | TEST-01–02 |
| Q8; F-15 | REL-03, TEST-01 |
| US-04, US-05, US-10, US-11 | REL-01, UX-01, TEST-01 |
| NFR1–NFR4; BR-U2-28–32, 46 | SEC-01, MAINT-01 |
| Cross-provider project convention | TEST-04 |

PBT-09 is compliant at NFR Requirements: the existing test-only framework is selected in
`tech-stack-decisions.md` and already listed in `go.mod`. PBT-02/03 have the explicit Q7
scope refinement above; PBT-07/08 are carried into verification requirements. Their execution
evidence belongs to Code Generation/Build and Test, not this documentation stage. PBT-01,
04–06, and 10 remain advisory under Partial enforcement. No blocking PBT finding at this stage.

NFR Design must resolve the concrete admission timeout/configuration, cancellation and
collaborator budget, busy HTTP contract, bounded multipart processing, and safe backup write
mechanics. These are assigned design decisions, not unanswered product-preference questions.
