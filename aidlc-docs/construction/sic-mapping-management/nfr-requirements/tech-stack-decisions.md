# Technology Decisions — UOW-2 SIC Mapping Management

Approved by the user on 2026-09-06 alongside `nfr-requirements.md`. Retain UOW-1 technology decisions except
for the explicit refinements below. No dependency upgrade is requested or needed.

| Concern | Decision | Requirement |
|---|---|---|
| Runtime | Existing Go module baseline (Go 1.21); no new process | MAINT-01 |
| HTTP and UI | Existing Gin, embedded Go templates, Bootstrap, HTMX, vanilla JavaScript | UX-01, MAINT-01 |
| Persistence | Existing `modernc.org/sqlite`, parameterized SQL, transactions, foreign keys | REL-01, SEC-01 |
| CSV and files | Standard-library `encoding/csv`, `io`, `os`, `path/filepath` | REL-02, SEC-02 |
| Coordination | Instance-owned serialization using Go standard-library concurrency/context facilities | CON-01 |
| Logging | Existing `log/slog` | SEC-01 |
| Verification | Go `testing`, temporary SQLite/files, race detector, uninstrumented timing harness | TEST-01, TEST-03 |
| Generated properties | Existing test-only `pgregory.net/rapid v1.1.0` in `go.mod` | TEST-02, PBT-09 |
| External services / infrastructure | None; retain single local binary; Infrastructure Design skipped | SEC-01, MAINT-01 |

## TD-U2-01 — Extend the existing service and contracts

Add mapping management through the approved service/repository boundaries. Reuse
`NormalizeSICCode`, `ValidateCSV`, `SICMappingImportReport`, `SICMappingImportError`, and
the shared outcome enumeration. Introduce `merged` for upload success; retain `imported`
for startup insertion. Classify database-side validation failures as `persistence_failed`
to address F-15 without inventing another error vocabulary.

## TD-U2-02 — Bounded admission without a runtime dependency

Use standard Go primitives to implement the service-owned serialization contract with
context-aware, bounded waiting. NFR Design selects the exact primitive and timeout setting;
a plain blocking `sync.Mutex.Lock` alone cannot satisfy cancellation and timed admission.
Retain the lock across reload and the collaborator handoff. Keep SQLite's existing five-second
busy timeout as a separate persistence-layer control. Do not introduce a background job
system or claim a caller timeout stops a collaborator that has not returned.

## TD-U2-03 — Local bounded file exchange

Retain standard CSV encoding for the five-column format. Centralize the 10 MiB size limit
and 50-diagnostic limit so startup and upload cannot drift. Enforce the byte boundary while
receiving the upload, before CSV parsing; cap diagnostics at accumulation. Store unique,
complete backups beside the injected database path, retain all completed backups, and
report backup failures without preventing merge. NFR Design selects safe write and request
handling mechanics. No backup service, retention scheduler, or additional CSV library.

## TD-U2-04 — Existing UI and checkpoint integration

Use existing Bootstrap accessibility/focus conventions and escaped server-rendered content.
Keep browser confirmation and submission state in the existing JavaScript stack. No frontend
framework is added. Constructor injection supplies the production no-op collaborator until
UOW-3; independent tests inject a controlled fake for ordering, contention, and failure cases.

## TD-U2-05 — Reuse the independent verification stack

Keep `rapid` test-only with domain generators, automatic shrinking, and seed/replay reporting.
The Q7 A selection requires generated merge idempotency and omission/merge invariants;
CSV round trips and category resolution use explicit examples under the recorded unit scope
refinement. Retain existing UOW-1 properties. No JavaScript property framework is needed for
this unit's presentation-only browser behavior.

Run race checks separately from the ten-second merge benchmark so instrumentation overhead
does not redefine the production performance target. Record fixture and environment details.
Production and independent verification remain separate provider sessions with artifact-based
handoffs; technology reuse does not remove the ownership gate.
