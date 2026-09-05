# NFR Design Patterns — UOW-1 SIC Import and Storage

## Design Boundary

UOW-1 applies its NFRs inside the existing Go process and current parser, domain, database, repository, service, and startup boundaries. It adds no queue, cache, background worker, circuit breaker, external service, second database, or runtime framework.

## Pattern Summary

| Concern | Pattern | Existing owner |
|---|---|---|
| SQLite contention | Driver-level bounded busy wait | Database opener/connection configuration |
| Migration safety | Ordered, idempotent, fail-closed schema evolution | Database migration functions |
| Seed validation | Size gate then validate-before-write | SIC mapping service |
| Seed persistence | Unit of Work with prepared statement | SIC mapping repository |
| Failure behavior | Explicit fatal/non-fatal classification | Database/service/startup boundary |
| SIC consistency | Canonical value-object boundary | Domain normalization helper |
| Privacy | Local-only data path and log minimization | All UOW-1 components |
| Scale/performance | Bounded input plus single-pass validation | File/service/repository boundaries |
| Compatibility | Additive schema and unchanged duplicate key | Database/import/repository paths |
| Verification | Deterministic seams and isolated fixtures | Independent test role |

## NFRP-U1-01 — Driver-Level Bounded Busy Wait

### Design

Configure SQLite with a five-second busy timeout before migration and repository use. The configuration must apply to every physical connection that can execute UOW-1 SQL, not merely whichever pooled connection happens to execute an initial `PRAGMA` statement.

Use the existing driver's supported connection configuration mechanism. Do not implement an application retry loop around migration or seed import.

```text
Open configured SQLite connection
        |
        v
Apply per-connection foreign-key + 5s busy settings
        |
        v
Run ordered migration / later repository work
        |
        +-- lock clears within 5s --> continue
        |
        +-- timeout/error ---------> contextual error -> close/fail startup
```

### Constraints

- Lock waiting is bounded at five seconds.
- The timeout is not multiplied by an outer retry loop.
- Context cancellation and non-busy errors return immediately according to driver behavior.
- Migration and seed persistence retain their approved fatal startup boundary after timeout.
- Verification must prove the configured value is active and that prolonged contention fails rather than hanging indefinitely.

## NFRP-U1-02 — Ordered Idempotent Migration

### Design

Evolve the schema through deterministic ordered steps:

1. Open SQLite with required connection settings.
2. Create/ensure fresh-schema tables using idempotent DDL.
3. Inspect `ledger_transaction` for `sic_code`.
4. Add the nullable column only when absent.
5. Create `idx_txn_sic` only after the column is guaranteed to exist.
6. Return the database only after all required steps succeed.

Each decision is based on current schema state, so interruption followed by restart safely resumes from the observed state. Existing tables are not rebuilt, copied, truncated, or replaced.

### Failure pattern

Migration is fail-closed: any required error is wrapped with the failing operation, the connection is closed, and startup stops. This avoids serving against a partially usable required schema. Recovery is a normal restart after the underlying local problem is corrected; no automated rollback of already committed idempotent DDL is required.

## NFRP-U1-03 — Size Gate Before Parse

### Design

The startup seed reader follows this order:

1. Resolve `<application-data-dir>/sic_mappings.csv` with `filepath.Join`.
2. Open the path for read-only access using normal filesystem link resolution.
3. Inspect the opened target's metadata.
4. Reject a target larger than 10 MiB before CSV parsing or whole-file buffering.
5. Parse only an accepted target.

Opening before inspecting binds validation to the target actually read and reduces path-replacement ambiguity. A symbolic link is permitted for this read-only input. UOW-1 performs no seed write, rename, truncate, or delete through the path.

### Outcomes

| Condition | Mutation | Startup |
|---|---|---|
| Path absent | None | Continue silently |
| Target exceeds 10 MiB | None | Safe diagnostic; continue |
| Read/metadata failure other than absence | None | Contextual initialization error |
| Accepted target | None yet | Continue to parsing/validation |

## NFRP-U1-04 — Validate Before Write

### Design

CSV processing is a bounded two-phase pattern:

```text
Phase 1: accepted file -> parse + normalize + resolve + collect candidates/errors
                              |
                              +-- any error -> discard candidates; log report; continue startup
                              |
Phase 2: all valid -> begin transaction -> prepared inserts -> commit
```

The 10 MiB input limit bounds the file data entering the validation phase. Validation performs no mapping mutation. It collects canonical SIC codes in a set to detect duplicates after normalization and builds candidates only for persistence after the complete file is valid.

Category data needed for name resolution should be loaded once through the approved repository/service boundary and indexed in memory for exact and case-insensitive lookup during this one initialization operation. This temporary index ends with the operation and is not global or permanently resident.

## NFRP-U1-05 — Atomic Prepared Bulk Insert

### Design

For a fully valid seed:

1. Repository begins one SQLite transaction.
2. Repository prepares one parameterized `INSERT` on that transaction.
3. Repository reuses the statement for each validated candidate.
4. Any prepare/execute error closes the statement and rolls back.
5. Successful completion closes the statement and commits.
6. A commit error is returned; the operation never reports success without a successful commit.

Transaction ownership stays in the repository operation that performs the atomic bulk insert. The service must not issue raw SQL or expose `*sql.Tx`. Ordinary single-row CRUD does not replace this bulk operation.

### Performance rationale

Statement reuse avoids repeated SQL compilation and preserves simple parameterized SQL. One transaction avoids per-row commit cost. Dynamic multi-row SQL and a new batching library are unnecessary under the 10 MiB/100,000-row accepted workload.

## NFRP-U1-06 — Explicit Failure Classification

| Failure | Classification | Required response |
|---|---|---|
| Database open/configuration/migration failure | Fatal | Close database where opened; fail startup |
| Busy timeout during required migration | Fatal | Return contextual error; fail startup |
| Seed absent | Expected | No mutation; continue silently |
| Mapping table non-empty | Expected skip | Log count; continue |
| Seed oversized | Non-fatal input rejection | Import none; safe log; continue |
| Seed structurally/domain invalid | Non-fatal input rejection | Import none; row diagnostics; continue |
| Seed read failure other than absence | Fatal initialization error | Import none; fail startup |
| Seed transaction/commit failure, including busy timeout | Fatal initialization error | Roll back/no success; fail startup |

Errors must retain causes for diagnosis while logs apply the approved redaction boundary.

## NFRP-U1-07 — Canonical Value Boundary

All SIC-producing entry points call one domain normalization/validation authority. Parser conversion supplies a positive integer decimal representation; CSV supplies raw text. Both converge on the same canonical string before persistence or equality comparison.

Repositories accept validated domain values from services but retain database uniqueness and foreign-key constraints as defense-in-depth. No handler, SQL query, or CSV-specific helper defines an alternate canonical form.

## NFRP-U1-08 — Local-Only and Minimal Diagnostics

UOW-1 contains no network client or external endpoint. All paths terminate in local memory, an explicitly configured local file, or the configured SQLite database.

Structured logs use stable event names and fields such as operation, safe path, row number, validation code, and count. They exclude complete source records, transaction descriptions, account identifiers, and file contents. Wrapped errors must not accidentally embed rejected row payloads.

## NFRP-U1-09 — Compatibility and Performance Evidence

Production design exposes no test-only global switches. Verification uses normal constructors, temporary paths/databases, deterministic fixtures, and package-visible behavior.

- SIC-free import baseline/candidate measurements use identical fixtures and state, one warm-up, and at least five measured runs; candidate median may regress by at most 10%.
- Migration measurement surrounds only schema initialization on the approved 100,000-transaction legacy fixture and must finish within five seconds.
- Seed measurement surrounds validation through commit for an accepted 100,000-row file and must finish within ten seconds.
- Environment metadata is recorded with results.

The independent review/test role owns tests, benchmark harnesses, property generators, and review evidence. Production code is structured for clean dependency injection but is not altered solely to satisfy a synthetic benchmark.

## Traceability

| Pattern | NFR/decision coverage |
|---|---|
| NFRP-U1-01 | NFR-U1-REL-02, REL-03, PERF-02, PERF-03; Q1/FQ1 |
| NFRP-U1-02 | NFR-U1-REL-01, REL-02, COMP-01; TD-U1-03 |
| NFRP-U1-03 | NFR-U1-REL-03, SEC-03; TD-U1-04; Q3 |
| NFRP-U1-04 | NFR-U1-PERF-03, REL-03, REL-04, SCALE-01 |
| NFRP-U1-05 | NFR-U1-PERF-03, REL-04; TD-U1-03; Q2 |
| NFRP-U1-06 | NFR-U1-REL-01 through REL-04 |
| NFRP-U1-07 | NFR-U1-MAINT-02, COMP-01; TD-U1-02 |
| NFRP-U1-08 | NFR-U1-SEC-01, SEC-02; TD-U1-05 |
| NFRP-U1-09 | NFR-U1-PERF-01 through PERF-04, TEST-01 through TEST-05; TD-U1-06, TD-U1-07 |
