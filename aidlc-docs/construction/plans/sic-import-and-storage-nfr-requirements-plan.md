# NFR Requirements Plan — UOW-1 SIC Import and Storage

## Unit and Functional Context

UOW-1 imports optional SIC values, evolves fresh and existing SQLite databases, persists transaction SIC data and SIC mappings, and optionally seeds mappings from a local CSV. The approved functional design requires local-only operation, additive/idempotent migration, an empty-table seed gate, whole-file validation, and atomic persistence.

Applicable approved requirements are NFR1 (local privacy), NFR2 (backward compatibility), NFR3 (clean architecture), NFR4 (idempotency), and the unit-specific portions of NFR5/NFR6 (testability and property-based testing). No infrastructure or external-service choice is in scope.

## Question Category Evaluation

- Scalability: applicable to local transaction/mapping counts and migration/index behavior.
- Performance: applicable to startup migration and whole-file CSV validation/import.
- Availability and reliability: applicable to migration failure, optional seed failure, rollback, and continued startup behavior.
- Security and privacy: the local-only boundary is approved; a defensive seed-file resource limit still needs a decision.
- Tech stack: the approved Go, `ofxgo`, SQLite, and standard-library CSV stack remains fixed. The property-testing framework is deferred to the consolidated UOW-3 test design unless UOW-1 needs a unit-specific choice.
- Maintainability and testability: applicable to legacy-schema fixtures, repeatability, atomicity, and normalization invariants.
- Usability/accessibility: not applicable because UOW-1 has no UI.

## NFR Clarification Questions

### Question 1 — Supported Local Database Scale

What transaction scale should UOW-1 explicitly support without requiring a different storage design?

A) Up to 1,000,000 transactions and 100,000 SIC mappings in one local SQLite database (recommended; generous for a personal-finance workload while remaining measurable)

B) Up to 100,000 transactions and 10,000 mappings

C) Do not state numeric capacity; require only that performance remains comparable to the current application

X) Other (state transaction and mapping counts after the tag)

[Answer]:C

### Question 2 — Existing-Database Startup Migration Target

For a database at the selected supported scale, what maximum elapsed time should the additive SIC migration target on a typical supported desktop?

A) 5 seconds (recommended; startup is blocked, but the migration is additive and runs once)

B) 2 seconds

C) 10 seconds

X) Other (state the target and reference environment after the tag)

[Answer]:A

## Final Answer Analysis

- FQ3 selects a blocking regression threshold: for a fixed SIC-free OFX/QFX fixture, median elapsed import time may be no more than 10% slower than the pre-UOW-1 baseline across at least five measured runs after one warm-up run.
- All initial and follow-up answers are complete, mutually consistent, measurable, and within UOW-1 scope. No further clarification is required.

### Question 3 — Valid Startup Seed Target

For a valid `sic_mappings.csv` containing 100,000 rows on a typical supported desktop, what startup validation-and-import target should apply?

A) 10 seconds (recommended; validation is whole-file and persistence is one local transaction)

B) 5 seconds

C) 30 seconds

X) Other (state the row count, target, and reference environment after the tag)

[Answer]:A

### Question 4 — Defensive Seed-File Limit

Should startup reject an excessively large local seed before parsing to prevent accidental memory exhaustion?

A) Yes; reject files larger than 50 MiB, log a safe diagnostic, import nothing, and continue startup (recommended)

B) Yes; use a 10 MiB limit with the same behavior

C) No explicit byte limit; rely on CSV parsing and available local resources

X) Other (state the exact limit and outcome after the tag)

[Answer]:B

### Question 5 — UOW-1 Verification Depth

Which verification is required for the migration and startup seed NFRs in addition to example-based functional tests?

A) Automated legacy/fresh/repeat migration tests, atomic seed tests, normalization invariant/property tests, and benchmark-style measurements for the selected scale targets; race testing only where concurrent code exists (recommended)

B) The same automated correctness/property tests, but performance targets are verified by documented manual measurements rather than committed benchmarks

C) Example-based correctness tests only; defer properties and performance verification to UOW-3

X) Other (describe the required evidence after the tag)

[Answer]:A

## Answer Analysis

- Q1 selects a relative compatibility requirement rather than a numeric capacity ceiling.
- Q3 fixes a measurable 100,000-row/10-second seed target.
- Q4 fixes a 10 MiB pre-parse limit with non-fatal startup behavior.
- Q5 requires automated correctness, property, and benchmark-style verification, with race testing only for concurrent code.
- Q2 selects five seconds but refers to the "selected supported scale." Because Q1 selected no numeric scale, the migration benchmark has no defined database size. Q2 and Q3 also use "typical supported desktop," which is not reproducible enough for the required benchmark evidence.

### Follow-up Question 1 — Migration Benchmark Dataset

Without making this a product capacity ceiling, what reference legacy database should be used to verify the five-second migration target?

A) 1,000,000 transactions, 100 categories, 1,000 text patterns, and 100 import-history rows (recommended; a demanding, reproducible personal-finance fixture)

B) 100,000 transactions, 100 categories, 1,000 text patterns, and 100 import-history rows

C) Use a sanitized copy/profile of the largest available real-world database, recording its row counts

X) Other (state the exact fixture composition after the tag)

[Answer]:B

### Follow-up Question 2 — Performance Reference Environment

What reference environment should govern the five-second migration and ten-second seed targets?

A) Document CPU model, logical CPU count, RAM, OS, Go version, and storage type for the machine used; run locally with no competing heavy workload, and treat the targets as acceptance evidence on that recorded machine rather than universal guarantees (recommended)

B) Define a fixed minimum machine now (state its CPU, RAM, OS, and storage requirements after the tag)

C) Run in the project's CI environment and record the runner specification; targets apply only to that runner class

X) Other (state the reproducible environment and applicability rule after the tag)

[Answer]:A

## Follow-up Answer Analysis

- FQ1 selects a reproducible legacy migration fixture with 100,000 transactions, 100 categories, 1,000 text patterns, and 100 import-history rows. This fixture is benchmark evidence, not a product capacity ceiling.
- FQ2 makes the performance targets acceptance evidence on a recorded local machine and requires its CPU, logical CPU count, RAM, OS, Go version, and storage type to be documented.
- The Q1 phrase "performance remains comparable to the current application" still lacks a numeric tolerance and workload, so it cannot yet be evaluated by the benchmark-style evidence required by Q5.

### Follow-up Question 3 — Backward-Compatible Import Performance

For importing a fixed OFX/QFX fixture containing no SIC values, what regression threshold should apply when comparing the UOW-1 implementation with the pre-UOW-1 baseline on the same recorded machine and database state?

A) Median elapsed time must be no more than 10% slower across at least five measured runs after one warm-up run (recommended)

B) Median elapsed time must be no more than 20% slower across at least five measured runs after one warm-up run

C) Record and compare results, but impose no blocking numeric threshold

X) Other (state the workload, statistic, run count, and threshold after the tag)

[Answer]:A

## Planned NFR Work

- [x] Analyze the approved UOW-1 functional design and inherited project constraints.
- [x] Evaluate scalability, performance, availability, security, reliability, maintainability, usability, and technology-choice relevance.
- [x] Record all material unresolved quality targets as explicit questions.
- [x] Validate every answer for completeness, consistency, feasibility, and unit scope.
- [x] Add follow-up questions for any vague or conflicting answer.
- [x] Define measurable UOW-1 scalability and performance requirements.
- [x] Define privacy, resource-safety, compatibility, failure, rollback, and idempotency requirements.
- [x] Define maintainability and verification requirements, including applicable PBT obligations.
- [x] Confirm fixed technology decisions and document their rationale without changing the approved architecture.
- [x] Generate `aidlc-docs/construction/sic-import-and-storage/nfr-requirements/nfr-requirements.md`.
- [x] Generate `aidlc-docs/construction/sic-import-and-storage/nfr-requirements/tech-stack-decisions.md`.
- [x] Validate traceability to NFR1-NFR6, the UOW-1 stories, and the approved functional design.

## Planned Outputs

- `aidlc-docs/construction/sic-import-and-storage/nfr-requirements/nfr-requirements.md`
- `aidlc-docs/construction/sic-import-and-storage/nfr-requirements/tech-stack-decisions.md`

Artifact generation must wait until all `[Answer]:` tags are complete and ambiguity analysis finds no unresolved decisions.
