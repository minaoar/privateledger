# Logical Components — UOW-1 SIC Import and Storage

## Component Strategy

No new runtime logical component or infrastructure element is introduced. NFR responsibilities are assigned to the components already approved by Functional Design and Application Design.

```text
cmd/privateledger startup wiring
        |
        +--> Database Open/Migration --> SQLite
        |
        +--> SIC Mapping Seed Service
                  |
                  +--> SIC normalization
                  +--> Category Repository
                  +--> SIC Mapping Repository --> SQLite transaction

OFX/QFX Parser --> Transaction Model --> Import flow --> Transaction Repository --> SQLite
```

Dependencies remain explicit and flow toward domain/persistence boundaries. There is no global coordinator, queue, cache, second database, background worker, or external service.

## LC-U1-01 — Database Open and Migration

### Existing location

`internal/database`

### Responsibilities

- Open the configured SQLite database.
- Establish per-connection foreign-key enforcement and the five-second busy timeout.
- Execute ordered idempotent schema creation, column detection/addition, and post-column index creation.
- Close the database and return contextual errors if required setup fails.

### Inputs and outputs

| Input | Output |
|---|---|
| Database path/config | Fully initialized `*sql.DB` or contextual error |

### NFR constraints

- Settings must apply to every physical connection used by the pool.
- No custom application retry loop.
- No table rebuild or destructive migration.
- The connection is not exposed to application wiring until required migration succeeds.

## LC-U1-02 — SIC Normalization Authority

### Existing approved boundary

Domain model/value helper under `internal/model` or the approved domain-equivalent package.

### Responsibilities

- Trim surrounding whitespace for raw string inputs.
- require ASCII digits;
- remove leading zeros;
- reject zero, empty, overflow, and values above positive `int64`;
- return the minimal decimal canonical form.

### Consumers

- OFX parser conversion.
- Startup seed validation.
- Later mapping CRUD/upload services.
- Repository lookup inputs where normalization has not already been guaranteed.

The helper is deterministic, side-effect-free, and independent of SQL, CSV, handlers, and services, making it directly property-testable without production test hooks.

## LC-U1-03 — OFX/QFX Parser Adapter

### Existing location

`internal/parser`

### Responsibilities

- Continue parsing through `ofxgo`.
- Convert a positive parsed SIC to decimal text and pass it through the shared normalization authority.
- Convert parser zero to absent transaction SIC.
- Preserve whole-file failure for malformed/non-numeric SIC as defined by `ofxgo`.

It adds no file pre-parser, recovery layer, or external SIC lookup.

## LC-U1-04 — Transaction Model and Repository

### Existing locations

`internal/model` and `internal/repository`

### Responsibilities

- Carry nullable canonical SIC in the transaction domain model.
- Write SQL `NULL` for absent SIC and canonical text when present.
- Return nullable SIC from downstream-required reads.
- Preserve the exact existing duplicate key.
- Support the approved indexed SIC query contract for downstream UOW-3.

The repository uses parameterized SQL and never derives business normalization rules from stored strings.

## LC-U1-05 — Seed File Reader and Validator

### Existing approved boundary

Startup-file operation in the SIC mapping service, using standard-library filesystem and CSV APIs.

### Responsibilities

1. Resolve the fixed seed path from the injected application-data directory.
2. Open read-only, permitting normal symbolic-link resolution.
3. inspect the opened target and enforce the 10 MiB limit before parsing;
4. parse the exact required CSV columns;
5. normalize SIC codes and detect canonical duplicates;
6. resolve category names with the approved exact/unique-case-insensitive algorithm;
7. validate optional confirming category IDs;
8. accumulate candidates and safe row diagnostics without persistence; and
9. call atomic repository persistence only when the complete file is valid.

### Temporary data structures

- Candidate slice bounded indirectly by the 10 MiB source limit.
- Canonical-SIC set for duplicate detection.
- Operation-scoped exact-name and folded-name category indexes.
- Validation report containing row coordinates/codes, not full records.

All are released after startup initialization; none becomes global state.

## LC-U1-06 — Category Repository Contract

### Existing location

`internal/repository`

### Responsibilities for UOW-1

- Provide category data needed for local seed resolution through the approved service boundary.
- Preserve current category storage semantics.
- Avoid a query per CSV row by allowing the service to obtain the relevant category set once.

The repository does not parse CSV or decide ambiguity policy.

## LC-U1-07 — SIC Mapping Repository

### Existing approved boundary

`internal/repository`

### Responsibilities

- Count mappings for the empty-table startup gate.
- Persist a completely validated candidate set atomically.
- Begin one SQLite transaction, prepare one parameterized insert, reuse it for all candidates, and commit once.
- Roll back on prepare/execute failure and report commit failure.
- Enforce database uniqueness and category foreign keys.

### Contract shape

```text
Count() -> count/error
BulkInsertAtomic(validatedCandidates) -> error
```

The contract accepts domain candidates, not CSV rows, and does not expose `*sql.Tx` to the service.

## LC-U1-08 — Startup Orchestrator

### Existing location

`cmd/privateledger/main.go`

### Responsibilities

- Resolve the application-data path.
- Open and migrate the database before constructing repositories.
- Construct repositories/services with explicit dependencies.
- Invoke optional seed initialization after migration and before serving requests.
- Apply the returned outcome according to the failure classification: continue for expected/non-fatal input outcomes, fail startup for database/read/persistence initialization errors.
- Emit structured outcome logs without sensitive payloads.

It contains wiring and lifecycle decisions only; it does not parse CSV, execute repository SQL, or normalize SIC.

## LC-U1-09 — Verification Components

### Ownership

These are test-only artifacts owned by the separate-provider independent review/test role, not runtime components.

### Components

- Temporary legacy/fresh SQLite fixture builder.
- Deterministic OFX/QFX fixtures under package `testdata/` where appropriate.
- Deterministic seed CSV generator constrained to 10 MiB.
- Go benchmark/measurement harness with environment recorder.
- `pgregory.net/rapid` SIC and feasible seed-property generators.
- Independent review artifact at the approved UOW-1 review path.

Production files do not contain test switches or benchmark-only branches. Verification exercises normal component contracts with temporary paths and databases.

## Interaction Sequences

### Startup success with valid seed

```text
Main -> Database: Open(path)
Database -> SQLite: configure connections (FK + busy=5s)
Database -> SQLite: ordered idempotent migration
Database --> Main: ready DB
Main -> Repositories/Service: construct dependencies
Main -> Seed Service: Initialize(appDataDir)
Seed Service -> Mapping Repository: Count()
Mapping Repository --> Seed Service: 0
Seed Service -> File: open read-only + inspect target size
Seed Service -> Category Repository: list resolution data
Seed Service -> Normalizer: validate each SIC
Seed Service -> Mapping Repository: BulkInsertAtomic(candidates)
Mapping Repository -> SQLite: begin + prepare + execute* + commit
Seed Service --> Main: Imported(count)
Main -> slog: safe outcome
```

### Invalid or oversized seed

```text
Seed Service -> File: inspect/parse
File/Validator --> Seed Service: oversized or validation report
Seed Service --> Main: non-fatal rejected outcome, imported=0
Main -> slog: safe reason/counts
Main: continue startup
```

### Prolonged database contention

```text
Database/Repository -> SQLite: required operation
SQLite: wait up to 5 seconds
SQLite --> caller: busy/locked error
caller: wrap operation context; rollback if transaction exists
Main: close database; fail startup
```

## Responsibility and Traceability Matrix

| Component | Primary patterns | NFR coverage |
|---|---|---|
| Database Open/Migration | Bounded Busy Wait, Ordered Migration | PERF-02, REL-01, REL-02, COMP-01 |
| SIC Normalization | Canonical Value Boundary | MAINT-02, TEST-02 |
| Parser Adapter | Compatibility, Local-only | PERF-01, COMP-01, SEC-01 |
| Transaction Model/Repository | Additive persistence, indexed reads | SCALE-01, COMP-01 |
| Seed Reader/Validator | Size Gate, Validate Before Write | PERF-03, REL-03, SEC-01 through SEC-03 |
| Category Repository | Operation-scoped resolution data | PERF-03, MAINT-01 |
| SIC Mapping Repository | Atomic Prepared Bulk Insert | PERF-03, REL-04 |
| Startup Orchestrator | Failure Classification, explicit wiring | REL-02, REL-03, MAINT-01 |
| Verification Components | Isolated deterministic evidence | PERF-01 through PERF-04, TEST-01 through TEST-05 |

## Excluded Logical Components

| Component | Reason excluded |
|---|---|
| Queue/background worker | Startup completion is deliberately synchronous and local |
| Distributed/local cache service | No shared/distributed state; operation-scoped indexes are sufficient |
| Circuit breaker | No remote dependency exists |
| Application retry subsystem | Five-second driver busy timeout is the approved bounded contention pattern |
| Second database/staging store | SQLite transaction already supplies atomicity |
| File watcher | Startup seed is checked once and SQLite becomes authoritative |
| New migration framework | Existing additive migration functions are sufficient |
| New runtime observability stack | Structured local `slog` diagnostics satisfy the requirement |
