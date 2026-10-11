# NFR Requirements — UOW-1 SIC Import and Storage

## Scope

These requirements govern SIC extraction and persistence, additive SQLite migration, and optional startup seeding from `sic_mappings.csv`. They refine project NFR1-NFR6 for UOW-1 without establishing a universal hardware guarantee or a numeric product-capacity ceiling.

## Performance and Scale

### NFR-U1-PERF-01 — Existing import regression

Importing a fixed OFX/QFX fixture containing no SIC values must have a median elapsed time no more than 10% slower than the pre-UOW-1 baseline when:

- baseline and candidate run on the same recorded machine and database state;
- the fixture, application configuration, and measurement boundary are identical;
- one warm-up run is excluded; and
- at least five measured runs are used for each version.

The comparison protects existing imports; it is not a throughput promise for all machines or files.

### NFR-U1-PERF-02 — Legacy migration target

The complete UOW-1 startup migration must finish within five seconds on the recorded reference environment for a legacy database containing:

- 100,000 transactions;
- 100 categories;
- 1,000 text categorization patterns; and
- 100 import-history rows.

The fixture is benchmark evidence, not a maximum supported database size. Measurement starts immediately before UOW-1 schema initialization and ends after the schema is ready for repository use. Fixture setup is excluded.

### NFR-U1-PERF-03 — Startup seed target

Whole-file validation and atomic insertion of a valid 100,000-row `sic_mappings.csv` must finish within ten seconds on the same recorded reference environment. The measured file must be no larger than the accepted 10 MiB limit; fixture generation and application/database opening are excluded. Measurement begins before file validation and ends after commit.

### NFR-U1-PERF-04 — Reference environment record

Performance evidence must record:

- CPU model and logical CPU count;
- installed RAM;
- operating-system name and version;
- Go version;
- storage type; and
- confirmation that no competing heavy workload was intentionally running.

The targets are acceptance evidence on that machine, not universal guarantees across supported hardware.

### NFR-U1-SCALE-01 — No numeric capacity ceiling

UOW-1 does not impose a transaction or mapping-count product limit beyond existing SQLite and local resource constraints. The implementation must retain the existing local SQLite design, use the SIC index defined by the functional design, and avoid introducing a permanently resident duplicate of transaction data.

## Reliability, Availability, and Compatibility

### NFR-U1-REL-01 — Migration repeatability and preservation

Fresh, legacy, and already-migrated databases must converge on the required schema. Repeated initialization must not produce duplicate-column/table/index errors or change existing account, transaction, category, pattern, or import-history values.

### NFR-U1-REL-02 — Migration failure boundary

If a required migration step fails, the database connection must be closed and application startup must fail with contextual diagnostics. The application must not serve against a partially usable required schema.

### NFR-U1-REL-03 — Optional seed availability behavior

- An absent seed file is normal and must not prevent startup.
- A seed larger than 10 MiB must be rejected before CSV parsing, import zero rows, log a safe diagnostic, and allow startup to continue.
- A malformed or invalid seed must import zero rows, report safe row-level validation errors where practical, and allow startup to continue.
- A persistence failure after successful validation must roll back the seed transaction and return a startup initialization error.
- A non-empty mapping table must remain authoritative and cause startup seeding to skip without mutation.

### NFR-U1-REL-04 — Atomic seed persistence

No startup-seed outcome may expose a partial mapping set. A valid file commits all candidates in one SQLite transaction; validation or persistence failure leaves the pre-operation mapping state unchanged.

### NFR-U1-COMP-01 — Existing input and database compatibility

- Supported OFX/QFX imports without SIC must preserve existing parsing, deduplication, categorization, count, and error behavior.
- Existing databases must be upgraded in place without destructive rebuilds or loss of user data.
- SIC remains excluded from the approved duplicate key.
- Existing transaction and mapping reads must retain their established fields while adding nullable SIC data where required.

No uptime percentage, failover, or disaster-recovery mechanism is required because this is a user-operated local desktop process with no new service topology.

## Privacy and Resource Safety

### NFR-U1-SEC-01 — Local-only processing

SIC values, mappings, category resolution, migration, validation, and persistence must use only local process memory, local files, and the configured local SQLite database. No network lookup, telemetry submission, cloud dependency, or external processing may be introduced.

### NFR-U1-SEC-02 — Safe diagnostics

Logs may contain operation names, paths, row numbers, field names, stable validation codes, counts, and contextual errors. They must not dump OFX/QFX payloads, complete CSV contents, transaction descriptions, account identifiers, or other unnecessary financial data.

**Amended 2026-09-07 by UOW-4 NFR Design (Q1 A).** The startup seed path additionally logs the bounded
diagnostic `Message`, which may contain a category name taken from the seed CSV. That is narrower than
"complete CSV contents", but the word "unnecessary" is doing real work in the sentence above, so the
necessity is recorded rather than left to judgement.

It is necessary because the seed path has no screen. Without it, a user whose `sic_mappings.csv` names a
renamed category is told only `code=category_not_found`, with nothing identifying the stale value or its
replacement, and UOW-4's entire mitigation of U4-01 is unavailable for the longest-lived mapping file in
the product. See NFR-U4-SEC-01 as amended.

**Corrected 2026-09-07 after independent review (U4-R-F02).** An earlier draft of this amendment claimed
a category name cannot be financial data. That overstates the guarantee. The service necessarily trusts
logical column 4 to be `Category_Name`, so a hand-edited or mis-delimited row that still parses as
exactly five fields can place description or account text there, and the seed path will persist it after
bounding it.

The honest guarantee is narrower and positional: **whatever occupies column 4 may be logged, bounded.**
An ordinary stray unquoted comma yields six fields and is rejected as `malformed_csv` before semantic
validation, so exposure needs an offsetting omission or another five-field malformation. Processing is
local, file logging is off by default, each value is control-sanitized and capped at 64 runes, the
message at 512, and only capped diagnostics are logged. The residual is accepted deliberately and
recorded as C4-03.

The remaining prohibitions are unchanged and still binding: no OFX/QFX payload and no complete CSV
contents are logged, and no upload path logs a message.

### NFR-U1-SEC-03 — Seed resource boundary

The startup initializer must determine file size before parsing and reject files larger than 10 MiB. The size rejection is non-fatal, produces no mapping mutation, and must not read the oversized file into memory merely to enforce the limit.

## Architecture and Maintainability

### NFR-U1-MAINT-01 — Layer direction

Implementation must preserve `handler -> service -> repository -> SQLite`. Parser output feeds domain models; repositories must not depend on services or handlers. Dependency construction belongs in `cmd/privateledger/main.go`; no global service state or singleton is permitted.

### NFR-U1-MAINT-02 — Single normalization authority

Parser conversion, startup CSV validation, repository-facing services, and later units must share one SIC normalization/validation authority. Alternate normalization algorithms at transport or persistence boundaries are prohibited.

### NFR-U1-MAINT-03 — Focused dependencies

Production behavior must remain within the approved Go, `ofxgo`, standard-library CSV/filesystem, and `modernc.org/sqlite` stack. A test-only property framework may be added as recorded in `tech-stack-decisions.md`; no runtime or network dependency is justified by UOW-1.

## Verification Requirements

### NFR-U1-TEST-01 — Example-based evidence

Independent verification must cover at minimum:

- present, absent, and zero SIC parser cases;
- transaction and mapping persistence round trips and uniqueness/foreign-key constraints;
- fresh, legacy, failed, and repeated migration paths with data-preservation assertions;
- absent, oversized, valid, invalid, repeated, non-empty-table, and persistence-failure seed scenarios;
- whole-file validation and rollback with no partial mappings; and
- imports without SIC retaining existing behavior.

Tests must use temporary files and databases and must never mutate user data.

### NFR-U1-TEST-02 — Property evidence

Property tests must cover SIC normalization/validation invariants and, where feasible, seed atomicity/idempotency. Generators must include valid positive-int64 values, leading zeros, surrounding whitespace, zero, overflow, non-ASCII/non-digit input, duplicates after canonicalization, and category-reference combinations.

Failing cases must be reproducible and shrunk to a useful minimal counterexample. Seeds or replay information must be reported by the selected framework. A property may be omitted only with a rationale in the independent review artifact showing why example-based evidence is stronger or the property is infeasible.

### NFR-U1-TEST-03 — Benchmark evidence

Automated benchmark-style measurements must exercise NFR-U1-PERF-01 through NFR-U1-PERF-03. Benchmark fixtures must be deterministic and isolated from user files. Results and the reference environment must be recorded in the unit's independent review or build/test evidence. Performance targets are blocking for the recorded acceptance run.

### NFR-U1-TEST-04 — Concurrency evidence

Race-detector verification is required for UOW-1 code only if the implementation introduces or exercises concurrent state. The absence of concurrent UOW-1 code must be explicitly recorded instead of adding artificial concurrency tests.

### NFR-U1-TEST-05 — Ownership gate

The production role modifies production files only. A separate-provider independent review/test role owns verification tests and the independent review artifact. UOW-1 cannot pass Code Generation while a required test or performance target fails, or while a Blocking/High finding remains.

## Traceability

| Source | UOW-1 refinement |
|---|---|
| NFR1 / US-09 | NFR-U1-SEC-01 through SEC-03 |
| NFR2 / US-01 / US-07 | NFR-U1-COMP-01, PERF-01, REL-01, REL-02 |
| NFR3 | NFR-U1-MAINT-01 through MAINT-03 |
| NFR4 / US-07 / US-08 | NFR-U1-REL-01 through REL-04 |
| NFR5 | NFR-U1-TEST-01, TEST-03, TEST-04, TEST-05 |
| NFR6 / PBT-02, PBT-03, PBT-07, PBT-08, PBT-09 | NFR-U1-TEST-02 and the property-framework decision |
| BR-MIG-01 through BR-MIG-06 | PERF-02, REL-01, REL-02, COMP-01 |
| BR-CSV-01 through BR-CSV-08 | PERF-03, REL-03, REL-04, SEC-03 |
| BR-SIC-01 through BR-SIC-05 | MAINT-02, TEST-02 |
