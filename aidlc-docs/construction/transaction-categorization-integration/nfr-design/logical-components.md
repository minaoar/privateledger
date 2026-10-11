# Logical Components — UOW-3 Transaction Categorization Integration

Status: APPROVED by the user on 2026-09-06. All components stay inside the existing single
binary. No infrastructure component, runtime dependency, table, column, or index is added.

## Responsibility Map

| Component / location | Responsibility | Pattern |
|---|---|---|
| `internal/service/categorizer.go` | The single decision function; guarded cache set with all-or-nothing swap; one exported reload entry point; full and per-category recategorization passes with split counts | 01–02, 04–05 |
| `internal/service/sic_categorizer.go` (new) | SIC mapping cache and lookup; scoped recategorization; implements UOW-2's collaborator | 01–02, 04 |
| `internal/repository/transaction_repo.go` | Single-parameter JSON set passing in `BulkUpdateCategory` and `GetUncategorizedBySICCodes`; existing reads otherwise unchanged | 03 |
| `internal/service/import_service.go` | Calls the decision function per transaction; no new query | 06 |
| `internal/handler/category_handler.go` | Synchronous reload; the detached `go LoadPatterns()` is deleted; split counts in the recategorize response | 02, 05 |
| `internal/handler/transaction_handler.go` | Modal mapping creation delegated to UOW-2's mapping service | 07 |
| `cmd/privateledger/main.go` | Wires the SIC categorizer into `Categorizer` and supplies it as UOW-2's collaborator, replacing the no-op | 01, 04 |
| `transactions.html`, `categories.html` | SIC context, mapping-creation choice, pre-run warning, split-count result | 05, 07 |
| Independent verification files | Examples, properties, batching boundaries, race and benchmark evidence | 08 |

Pattern numbers refer to NFRP-U3-01 through NFRP-U3-08.

## Wiring and the Dependency Direction

`main.go` constructs the SIC categorizer, injects it into `Categorizer` as the lookup interface, and
passes the same instance to `SICMappingService` as the recategorization collaborator, replacing
`NewNoopSICRecategorizationCollaborator()`.

One instance serves both roles deliberately: the cache the lookup reads must be the cache the
collaborator reloads, or a mapping change would refresh one view and leave the other stale.

The graph stays directed. `SICMappingService` depends on the collaborator interface UOW-2 already
defined; the SIC categorizer depends on repositories; nothing depends back on the mapping service from
inside the categorizer. UOW-2's interface is implemented as-is — not renamed, widened, or given a
context parameter, since a context would describe a cancellation budget the Functional Design
deliberately declined.

## Recategorization Flow

1. Caller enters a recategorization pass — full, per-category, or scoped by SIC codes.
2. Rules reload through the single entry point. Failure stops the pass before any read.
3. Read the candidate set: all uncategorized rows for a full pass, or only those matching the affected
   codes for a scoped pass.
4. Apply the decision function in memory, recording the rule source per assignment.
5. Group by resolved category and issue one single-statement update per category, each atomic by construction.
6. Return processed, total categorized, and the pattern/SIC split.

The mapping-mutation gate is held by UOW-2 for the whole of a scoped pass. Per the approved Functional
Design there is no processing deadline; NFR-U3-PERF-01 is the evidence that this is acceptable at the
amended 20,000-transaction scale.

## Persistence Boundary

Repository methods own SQL and now own the driver's parameter ceiling, removing it from every caller by passing sets as one JSON parameter rather than expanded placeholders. Services own categorization decisions and
counts.

Only `category_id` and `category_source` are written. SIC codes are never modified by this unit, and no
mapping is created, updated, or deleted except through UOW-2's mapping service. SIC-assigned categories
reuse `category_source = rule`; a fourth source value would change the meaning of existing rows and of
every query filtering that column, including the shipped uncategorized-dashboard work.

## Cache Boundary

The pattern cache and mapping cache are process-local derived state behind one lock, refreshed together
or not at all. They are unbounded by decision, with observed memory recorded rather than a ceiling
enforced: a mapping is a short code, two short strings and a nullable integer, so eviction would add
cache-miss paths to guard a cost that does not arise on a local desktop database.

A stale cache can delay a rule taking effect. It can never produce a write that violates a database
constraint, because SQLite remains authoritative for uniqueness and foreign keys.

## Verification Traceability

| Requirement | Required evidence |
|---|---|
| PERF-01 | 20,000-transaction "Recategorize All" within five seconds, median of five runs after one warm-up, recorded environment |
| PERF-02 | Both import fixtures — SIC-free unchanged, and mappings-populated — against the 10 % median budget |
| SCALE-01 | Observed memory for the rule caches; no eviction asserted |
| CON-01 | Race-detector run over concurrent categorization and reload; all-or-nothing swap on partial reload failure |
| REL-01 | Priority matrix; manual and existing-assignment preservation; scoped pass touching nothing outside the affected set; set-passing correctness past the former ceiling |
| REL-02 | Split counts summing to the total; counts claimed only after commit |
| SEC-01 | Local-only processing; escaped output; no transaction data in logs |
| UX-01 | Pre-run warning, split-count result, modal display and fallback, unknown-outcome handling |
| MAINT-01 | Layer direction, constructor wiring, no new dependency, page-level JS names checked against `app.js` globals |
| TEST-01–05 | Examples, generated properties, batching boundaries, race evidence, benchmarks, and the cross-provider gate |

Infrastructure Design remains skipped. Code Generation planning follows explicit NFR Design approval.
