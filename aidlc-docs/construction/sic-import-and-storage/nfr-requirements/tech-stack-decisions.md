# Technology Decisions — UOW-1 SIC Import and Storage

## Decision Summary

| Concern | Decision | Scope |
|---|---|---|
| Runtime language | Go 1.21 baseline from `go.mod` | Existing; retain |
| OFX/QFX parsing | `github.com/aclindsa/ofxgo` | Existing; retain |
| Persistence | SQLite via `modernc.org/sqlite` | Existing; retain |
| CSV and local files | Go standard library (`encoding/csv`, `os`, `io`, `path/filepath`) | Runtime |
| Logging | `log/slog` | Runtime |
| Unit/integration/benchmark harness | Go `testing` package | Test-only |
| Property-based testing | `pgregory.net/rapid` | Test-only; add during independent verification |
| External services | None | Prohibited for UOW-1 |

## TD-U1-01 — Preserve the Existing Runtime Stack

### Decision

Implement UOW-1 within the current Go single-binary application and its existing package boundaries. Do not add a runtime framework, process, database, service, or network dependency.

### Rationale

- The feature is an additive local data capability.
- The approved architecture already provides parser, model, database, repository, service, and startup boundaries.
- A new runtime technology would expand privacy, packaging, migration, and maintenance risk without supplying a required capability.

## TD-U1-02 — Retain `ofxgo` and Normalize at the Domain Boundary

### Decision

Continue using `ofxgo` for OFX/QFX parsing. Convert its positive numeric SIC value to the shared canonical domain representation; treat parser zero as absent. Do not pre-parse XML or fork the library to recover non-numeric SIC.

### Consequences

- Existing parser compatibility remains centralized.
- Non-numeric SIC continues to fail at the established whole-file parser boundary.
- CSV/API normalization must share the domain helper rather than depend on parser-specific behavior.

## TD-U1-03 — SQLite Additive Migration

### Decision

Use SQLite schema inspection plus additive DDL through the existing `modernc.org/sqlite` connection. Ensure the transaction SIC column before creating its index. Use SQLite transactions for atomic startup seed insertion and retain foreign-key enforcement.

### Rationale

- This is compatible with fresh and legacy local databases.
- It preserves the no-CGO, single-binary architecture.
- SQLite uniqueness, foreign keys, and transactions provide the authoritative persistence constraints required by the functional design.

### Rejected alternatives

- Rebuilding existing tables: unnecessary and raises data-loss risk.
- A second mapping database or in-memory-only store: breaks local source-of-truth and lifecycle contracts.
- A separate migration framework: unjustified for the focused additive schema evolution.

## TD-U1-04 — Standard-Library CSV and File Controls

### Decision

Use `encoding/csv` for structured parsing and standard filesystem APIs for local discovery. Inspect file metadata before opening/parsing the seed and reject a size above 10 MiB. Resolve paths with `filepath.Join` from the injected application-data directory.

### Rationale

- The fixed five-column format requires no third-party CSV feature.
- Standard APIs are cross-platform and avoid a runtime dependency.
- Pre-parse metadata checking enforces the resource boundary without buffering an oversized file.

## TD-U1-05 — Structured Local Logging

### Decision

Use `slog` with contextual structured fields for migration and startup-seed outcomes. Record safe paths, row numbers, validation codes, and counts while excluding payloads and financial descriptions.

### Rationale

This follows project conventions and supplies actionable local diagnostics without violating the privacy boundary.

## TD-U1-06 — Go Testing and Benchmark Harness

### Decision

Use the standard `testing` package for example tests, temporary-database integration tests, benchmark-style measurements, and fixture orchestration. Record environment metadata alongside acceptance results.

### Rationale

- It integrates with `go test ./...` and the existing build workflow.
- It supports repeatable benchmarks without a production dependency.
- Temporary directories and deterministic fixtures protect user data.

The five-run median baseline/candidate comparison may use a small test helper or documented harness owned by the independent test role; it must not alter production code solely to manufacture a benchmark result.

## TD-U1-07 — Property Testing with `pgregory.net/rapid`

### Decision

Select `pgregory.net/rapid` as a test-only dependency for UOW-1 normalization/validation and feasible seed properties. The independent review/test role adds and owns the test dependency and property tests.

### Rationale

- It is native to Go tests and supports generated cases, shrinking, and reproducible failures.
- These capabilities directly address PBT-07 generator quality and PBT-08 shrinking/reproducibility.
- Custom generators can cover positive-int64 boundaries, canonical duplicates, invalid Unicode/digit forms, and category-reference combinations.

### Rejected alternatives

- `testing/quick`: minimal dependency cost, but its shrinking and diagnostic ergonomics are insufficient for the explicitly enforced PBT expectations.
- Handwritten random loops: do not provide a disciplined shrinking/replay contract and would duplicate a testing framework.
- Runtime fuzzing as the sole approach: valuable as supplemental evidence, but not a substitute for the selected deterministic property suite and its required shrinking/reproducibility behavior.

### Constraints

- The dependency remains test-only and must not appear in production imports.
- Generators must target meaningful domain partitions rather than unconstrained random strings alone.
- Failure output must retain the framework seed/replay information.

## TD-U1-08 — No Infrastructure or UI Technology Change

### Decision

Infrastructure Design remains skipped for UOW-1. No UI framework decision is needed because this unit has no frontend. Startup wiring remains explicit in `cmd/privateledger/main.go`.

## Decision Traceability

| Decision | Requirements supported |
|---|---|
| TD-U1-01 | NFR-U1-SEC-01, MAINT-01, MAINT-03 |
| TD-U1-02 | NFR-U1-COMP-01, MAINT-02 |
| TD-U1-03 | NFR-U1-PERF-02, REL-01, REL-02, REL-04 |
| TD-U1-04 | NFR-U1-PERF-03, REL-03, SEC-03 |
| TD-U1-05 | NFR-U1-SEC-02 |
| TD-U1-06 | NFR-U1-PERF-01 through PERF-04, TEST-01, TEST-03, TEST-04 |
| TD-U1-07 | NFR-U1-TEST-02, NFR6/PBT-07/PBT-08/PBT-09 |
| TD-U1-08 | NFR-U1-MAINT-01 and approved UOW-1 boundary |
