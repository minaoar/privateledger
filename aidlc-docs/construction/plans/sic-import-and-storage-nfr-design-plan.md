# NFR Design Plan — UOW-1 SIC Import and Storage

## Design Context and Constraint

UOW-1 must incorporate the approved privacy, compatibility, idempotency, atomicity, resource-safety, performance, and verification requirements into concrete patterns and logical components.

The user has directed this stage to avoid drastic technology changes whenever possible. The design therefore defaults to the existing Go process, `ofxgo`, `modernc.org/sqlite`, standard-library filesystem/CSV APIs, `slog`, constructor injection, and current package layering. No queue, distributed cache, circuit breaker, external service, second database, background worker, or new runtime framework is presumed.

The approved `pgregory.net/rapid` choice is test-only and does not change the production runtime stack.

## Mandatory Category Evaluation

- Resilience Patterns: applicable to SQLite lock contention, fatal migration failures, non-fatal invalid/oversized seed files, and atomic rollback.
- Scalability Patterns: applicable to bounded local-file processing and efficient 100,000-row validation/insertion; horizontal or distributed scaling is not applicable to the local single-process product.
- Performance Patterns: applicable to prepared statements, transaction scope, schema/index ordering, and avoiding redundant database/file passes.
- Security Patterns: applicable to local path handling, symbolic links, size validation, safe logging, and preventing unintended external processing.
- Logical Components: applicable to defining migration, normalization, seed validation, atomic persistence, and startup orchestration boundaries inside existing packages. New infrastructure components are not supported by the requirements.

## NFR Design Questions

### Question 1 — SQLite Lock Resilience

If startup migration or seed persistence encounters a transient SQLite busy/locked condition, what pattern should UOW-1 use?

A) Use the existing connection's bounded SQLite busy timeout if already configured; otherwise add a small bounded timeout through the existing driver, then return the approved fatal startup error when it expires—no custom retry loop (recommended)

B) Fail immediately with contextual error and rely on the user to restart

C) Add an application-level bounded retry loop with backoff around the entire migration/seed transaction

X) Other (describe timeout/retry ownership, limit, and final outcome after the tag)

[Answer]:A

## Follow-up Answer Analysis

- FQ1 selects an exact five-second SQLite busy timeout. After the timeout, migration or seed persistence returns the approved contextual startup failure; no application-level retry loop is added.
- All initial and follow-up answers are complete, consistent with the approved NFR requirements, and compatible with the conservative existing-stack direction. No further clarification is required.

### Question 2 — Bulk Seed Persistence Pattern

How should a fully validated seed of up to 100,000 rows be inserted while retaining the existing SQLite stack?

A) One SQLite transaction with one prepared parameterized INSERT reused for every candidate (recommended; bounded design complexity and atomicity without a new library)

B) One SQLite transaction with dynamically sized multi-row INSERT statements in batches

C) Insert each row through the ordinary single-create repository method inside one service-owned transaction

X) Other (describe batching, transaction ownership, and parameterization after the tag)

[Answer]:A

### Question 3 — Symbolic-Link Handling for the Local Seed

How should startup treat `sic_mappings.csv` when that path is a symbolic link?

A) Allow it, resolve normal filesystem behavior, enforce the 10 MiB limit on the opened target, and never follow links for any write/delete operation in UOW-1 (recommended; compatible with user-managed local files)

B) Reject symbolic links as invalid seed inputs and continue startup without importing

C) Allow only links whose resolved target remains inside the configured application-data directory

X) Other (state the exact read/write and containment rules after the tag)

[Answer]:A

### Question 4 — Logical Component Boundary

Should NFR Design add any new runtime logical component beyond the approved existing migration, domain normalization, repository, seed service, and startup wiring boundaries?

A) No; express resilience, performance, privacy, and resource controls inside those existing components only (recommended; follows the conservative technology direction)

B) Add a dedicated in-process migration coordinator abstraction, but no new library or process

C) Add a dedicated in-process seed pipeline abstraction, but no new library or process

X) Other (name the component, responsibility, dependency direction, and why existing components are insufficient after the tag)

[Answer]:A

## Answer Analysis

- Q2 selects one SQLite transaction with a reused prepared parameterized insert.
- Q3 permits read-only startup import through a symbolic link, applies the 10 MiB limit to the opened target, and prohibits UOW-1 link-following write/delete behavior.
- Q4 keeps all NFR controls inside the already approved logical components and adds no runtime component or infrastructure.
- Q1 selects a driver-level bounded busy timeout with no custom retry loop. The current database configuration sets no busy timeout, and the answer does not specify the new duration, leaving resilience behavior and verification ambiguous.

### Follow-up Question 1 — SQLite Busy Timeout Duration

What exact busy timeout should the existing SQLite connection configure before migrations and repository use?

A) 5 seconds (recommended; conventional bounded wait for a local desktop database, after which startup fails contextually)

B) 2 seconds (shorter startup wait, but more likely to reject brief contention)

C) 10 seconds (more tolerant of contention, but a noticeably longer failed startup)

X) Other (state the exact duration and rationale after the tag)

[Answer]:A

## Planned NFR Design Work

- [x] Analyze approved UOW-1 NFR requirements and technology decisions.
- [x] Evaluate all mandatory NFR design question categories.
- [x] Record unresolved pattern and logical-component choices as explicit questions.
- [x] Validate all answers for completeness and consistency with approved artifacts.
- [x] Add follow-up questions for any vague, combined, or conflicting answer.
- [x] Design failure classification, transaction rollback, lock handling, and restart behavior.
- [x] Design bounded file processing, validation, prepared persistence, and performance measurement seams.
- [x] Design local-file trust, size-check, logging-redaction, and no-network controls.
- [x] Map responsibilities to existing logical components and preserve dependency direction.
- [x] Define verification hooks without moving test ownership into production code.
- [x] Generate `aidlc-docs/construction/sic-import-and-storage/nfr-design/nfr-design-patterns.md`.
- [x] Generate `aidlc-docs/construction/sic-import-and-storage/nfr-design/logical-components.md`.
- [x] Validate traceability to every UOW-1 NFR requirement and approved technology decision.

## Planned Outputs

- `aidlc-docs/construction/sic-import-and-storage/nfr-design/nfr-design-patterns.md`
- `aidlc-docs/construction/sic-import-and-storage/nfr-design/logical-components.md`

Artifact generation must wait until every `[Answer]:` tag is complete and ambiguity analysis finds no unresolved design decision.
