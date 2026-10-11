# Logical Components — UOW-2 SIC Mapping Management

Status: APPROVED by the user on 2026-09-06. Components remain inside the existing
single binary; no infrastructure component or runtime dependency is added.

## Responsibility Map

| Component/location | Responsibility | Design pattern |
|---|---|---|
| cmd/privateledger/main.go | Construct one injected mapping service with directory, admission timeout, repositories and no-op collaborator; wire page/API routes; retain startup ordering | 01–02, 05 |
| internal/model/sic_mapping.go | Shared normalization, reports/outcomes, truncation flag and mutation result/domain errors; shared limits without HTTP dependency | 04–05, 07 |
| internal/handler/sic_mapping_handler.go | Standard bounded multipart intake, transport decoding, stable HTTP mapping, CSV attachment | 04, 07 |
| Existing page handler | Request GetPageData from service and render embedded template | 07 |
| internal/service/sic_mapping_service.go | Instance gate, validation, pre-state snapshot/diff, best-effort backup, persistence orchestration, reload/handoff and saved-state outcomes | 01–07 |
| internal/repository/sic_mapping_repo.go | Context-aware new transaction operations, prepared atomic merge, authoritative constraints, numeric ordering and joined display data | 02–03 |
| Existing category repository | Current category choices/resolution; no direct handler access | 03, 05 |
| Injected SIC collaborator | Narrow reload and affected-code categorization contract; checkpoint no-op | 02–03 |
| Embedded sic_mappings.html and existing frontend stack | Accessible CRUD/file workflows and accurate result presentation | 07 |
| Independent verification files | Examples, properties, coordinated concurrency, performance/environment evidence | 08 |

Pattern numbers refer to NFRP-U2-01 through NFRP-U2-08 in nfr-design-patterns.md.

## Service Admission and Configuration

The service owns the capacity-one channel and receives a positive admission duration, defaulting
to a named five seconds in production wiring. Tests may inject shorter durations. Add no
config.json fields or general options framework solely for these controls. Preserve existing
UOW-1 constructor call sites with a small compatible wrapper if needed. Production wiring
explicitly supplies the no-op collaborator; nil must not masquerade as successful work.

## Upload Data Flow

1. Handler bounds the body, uses standard multipart parsing, checks actual file size, and calls service with request context and the opened file.
2. Service acquires admission; expiry/cancellation exits without backup or mutation.
3. Shared ValidateCSV returns candidates and the existing report; invalid/error exits before backup.
4. Service reads authoritative pre-state once and computes counts/affected codes independently of backup.
5. Local backup helper writes the complete snapshot and returns either a confirmed path or warning.
6. Repository MergeAll commits candidates in one transaction or rolls back all rows.
7. Service records MappingCommitted and persisted counts immediately after successful commit.
8. Service synchronously reloads/hands off, retaining gate ownership and recording warnings.
9. Handler renders the result; UI distinguishes rejection/rollback from saved-with-warning.

Multipart storage is bounded per request and cleaned up by the handler; parsed candidates
exist only for the admitted service operation. No persistent duplicate of transaction data is introduced. Export/page
reads use repository results directly through services and do not acquire mutation admission.

## Persistence and Category Boundaries

Repository methods own SQL transactions and map driver errors into stable error categories;
services own business decisions and diff/counts. MergeAll must not call the existing destructive
ReplaceAll method. Numeric ordering uses the canonical numeric SIC storage contract; verify
the real column/model representation before choosing SQL expressions. Joined category reads
provide the display fields required by the page and CSV export.

Category changes outside this service are not serialized by its gate. Validate category
references and preserve database foreign keys. A category deleted between validation and
write may produce a whole-operation persistence failure; no dangling reference is allowed.
No attempt is made to freeze category edits throughout mapping administration.

## Collaborator Contract

Keep the narrow reload/affected-code collaborator required by Functional Design. UOW-2 wires
an immediate no-op; independent tests inject a fake to check ordering, selected codes, and
returned failures. Reload failure skips recategorization; failures after commit are warnings.
Do not add a context-aware adapter or thirty-second budget solely for the no-op checkpoint.

UOW-3 owns real categorization, text-first priority, preservation of existing assignments,
and a suitable processing/cancellation budget. That design must retain serialization until
work returns. UOW-2 bounds waiting callers and reports committed state accurately; it does
not promise a time limit on arbitrary collaborator execution.

## Filesystem Boundary

Keep one private CSV backup helper using standard-library file operations and the supplied
pre-state snapshot/directory. Use unique exclusive creation, check CSV flush/write and file
close, and remove the newly created partial file on failure. Ordinary temporary-directory
tests cover success and unavailable destinations. Introduce a small writer/closer seam only
where needed for meaningful write/close failure verification; no general filesystem interface.
Backup failures leave the snapshot/count calculation intact and return a warning, never a
claimed successful backup path. Completed backups are retained.

## Verification Traceability

| Requirement group | Required evidence |
|---|---|
| PERF-01 | Pattern 08 deterministic ten-second merge benchmark, successful backup, recorded environment |
| SCALE-01, UX-01 | 1,000-row page, labels/focus/errors/warnings, stable controls and confirmation |
| CON-01 | Coordinated wait timeout/cancellation, no late mutation, gate held through collaborator, race checks |
| REL-01 | Atomic rollback, counts/omission/idempotency, truthful CRUD/upload post-commit results |
| REL-02 | Exclusive complete backup, retained files, collision/failure/cleanup results |
| REL-03 | F-15 corrected across shared validation, startup and upload |
| SEC-01–03 | Local paths/escaped content, exact size/envelope enforcement, safe capped diagnostics and accurate rejection counts |
| MAINT-01 | Layer/constructor review, no runtime dependency, UOW-1 compatibility and no-op checkpoint |
| TEST-01–04 | Independent examples/properties, build/vet/race/benchmark evidence and cross-provider gate |

PBT compliance at this design stage: existing rapid framework retained (PBT-09); approved
Q7 scope preserved for PBT-02/03; generator/shrinking/replay requirements carried forward
(PBT-07/08). Test execution remains pending, with no blocking design-stage PBT finding.
Infrastructure Design stays skipped. Code Generation planning follows explicit NFR Design approval.
