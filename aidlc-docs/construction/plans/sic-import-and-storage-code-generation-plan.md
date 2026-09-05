# Code Generation Plan — UOW-1 SIC Import and Storage

## Authority and Scope

This plan is the single source of truth for UOW-1 Code Generation. Production generation must execute these steps in order and may not add behavior outside the approved requirements and design artifacts.

UOW-1 implements:

- US-01 — Import transactions with SIC data.
- US-07 — Upgrade existing local databases safely.
- US-08 — Initialize SIC mappings from an existing mapping file.
- US-09 — Preserve local-only privacy.

It establishes persistent contracts consumed by UOW-2 and UOW-3. It does not implement mapping-management handlers/UI, upload/download/backup workflows, mapping-driven categorization, modal behavior, or cache concurrency; those belong to later units.

## Mandatory Ownership Boundary

### Production role

- May modify production Go, SQL, and production documentation listed by this plan.
- Must not create or modify `_test.go` files, `testdata/`, test helpers, benchmark code, fuzz/property tests, test-only configuration, or the independent review artifact.
- Must not add `pgregory.net/rapid`; that test-only dependency belongs to the independent review/test role if its tests require it.

### Independent review/test role

- Must run in a separate provider session after production generation.
- Owns production-code review, test strategy, all verification tests/fixtures/benchmarks/property tests, test-only dependencies, and `aidlc-docs/construction/sic-import-and-storage/code-review/independent-review.md`.
- Must report PASS with all required tests and performance evidence passing and no unresolved Blocking/High findings before UOW-1 Code Generation can complete.

## Existing Dependencies and Contracts

- Runtime: Go 1.21, `ofxgo`, `database/sql`, `modernc.org/sqlite`, standard-library CSV/filesystem APIs, and `slog`.
- Layering: parser/domain -> service -> repository -> SQLite; startup dependency wiring in `cmd/privateledger/main.go`.
- Database: `ledger_transaction`, `category`, and existing foreign-key behavior.
- Transaction identity remains exactly `(account_id, trn_type, fit_id, date_posted)`.
- UOW-1 provides downstream nullable transaction SIC, canonical SIC mapping storage, mapping repository contracts, and startup initialization behavior.

## Expected Production Files

### Modify in place

- `internal/model/transaction.go`
- `internal/parser/ofx_parser.go`
- `internal/database/schema.sql`
- `internal/database/db.go`
- `internal/repository/transaction_repo.go`
- `cmd/privateledger/main.go`

### Create

- `internal/model/sic_mapping.go`
- `internal/repository/sic_mapping_repo.go`
- `internal/service/sic_mapping_service.go`
- `aidlc-docs/construction/sic-import-and-storage/code/production-summary.md`
- `aidlc-docs/construction/sic-import-and-storage/code/independent-review-handoff.md`

Exact file placement may use an existing same-responsibility file discovered immediately before a step, but duplicate replacement files such as `_new.go` or `_modified.go` are prohibited. No handler, template, static asset, README, API documentation, or deployment artifact is required for UOW-1.

## Sequential Generation Steps

### Step 1 — Establish SIC domain model and normalization

- [ ] Create `internal/model/sic_mapping.go` with `SICCode`/normalization-validation behavior, `SICMapping`, nullable category semantics, descriptions, creation metadata, import row/report/outcome domain types needed by startup seeding, and safe validation errors.
- [ ] Modify `internal/model/transaction.go` to carry nullable canonical `SICCode` plus downstream display-only SIC description where approved, without changing `NewTransaction` callers unnecessarily or changing debit/credit derivation.
- [ ] Enforce the canonical positive-int64 domain: ASCII digits only, trim whitespace, remove leading zeros, reject zero/empty/overflow, accept through `9223372036854775807`.
- [ ] Keep domain helpers deterministic and free of CSV, SQL, handler, or service dependencies.
- [ ] Trace to US-01, US-08; BR-SIC-01 through BR-SIC-05; NFR-U1-MAINT-02; NFRP-U1-07.

### Step 2 — Extract SIC through the existing OFX parser

- [ ] Modify `internal/parser/ofx_parser.go` so positive `ofxgo.Transaction.SIC` values populate the transaction's canonical SIC and zero remains absent.
- [ ] Preserve existing whole-file parser errors, invalid-transaction skipping, details merging, transaction construction, and behavior for files without SIC.
- [ ] Do not pre-parse OFX/XML, add external lookup, or alter the existing 32 MiB OFX input boundary.
- [ ] Trace to US-01, US-09; BR-TXN-03/04; NFR-U1-COMP-01; TD-U1-02.

### Step 3 — Implement additive schema and connection resilience

- [ ] Modify `internal/database/schema.sql` for fresh databases: nullable `ledger_transaction.sic_code` and `sic_mapping` with canonical unique code, descriptions, nullable `category_id REFERENCES category(category_id) ON DELETE SET NULL`, and timestamp.
- [ ] Keep `idx_txn_sic` out of the pre-column schema batch used against legacy databases.
- [ ] Modify `internal/database/db.go` to configure foreign keys and a five-second SQLite busy timeout for every physical connection used by UOW-1, using the existing driver and no application retry loop.
- [ ] Add safe schema inspection/`ensureColumn` logic for legacy `ledger_transaction.sic_code`, then create `idx_txn_sic` only after the column exists.
- [ ] Preserve fail-closed connection cleanup and contextual errors; do not rebuild or destructively migrate existing tables.
- [ ] Trace to US-07, US-09; BR-MIG-01 through BR-MIG-06; NFR-U1-PERF-02; NFR-U1-REL-01/02; NFRP-U1-01/02.

### Step 4 — Extend transaction persistence contracts

- [ ] Modify all affected insert/select/scan lists in `internal/repository/transaction_repo.go` to write and return nullable SIC without positional misalignment.
- [ ] Keep duplicate lookup and its SQL key exactly unchanged except for any non-identity result field required by the model.
- [ ] Add the approved SIC-scoped uncategorized query contract needed downstream, using parameterized placeholders, `category_source = 0`, `idx_txn_sic`, and an empty-input short circuit; do not add categorization behavior in UOW-1.
- [ ] Preserve existing filtering, ordering, error context, and row-iteration checks.
- [ ] Trace to US-01; BR-TXN-01 through BR-TXN-03; NFR-U1-COMP-01; LC-U1-04.

### Step 5 — Implement SIC mapping repository

- [ ] Create `internal/repository/sic_mapping_repo.go` using constructor injection of `*sql.DB`.
- [ ] Implement the approved foundational mapping contracts needed by UOW-1 and downstream units: count, create, get by ID/code, list with nullable/joined category display data, update, delete, and atomic bulk insertion/replacement primitives where the approved component methods require them.
- [ ] Implement startup `BulkInsertAtomic` as one SQLite transaction with one reused prepared parameterized insert; rollback on prepare/execute failure and return commit failure.
- [ ] Preserve unique SIC and nullable foreign-key behavior as database-authoritative constraints and check row iteration/errors consistently.
- [ ] Do not implement handler/UI behavior or recategorization orchestration.
- [ ] Trace to US-08; BR-CSV-07; BR-MIG-05; NFR-U1-REL-04; NFRP-U1-05; LC-U1-07.

### Step 6 — Implement startup seed validation and orchestration

- [ ] Create the UOW-1 portion of `internal/service/sic_mapping_service.go` with explicit mapping/category repository dependencies and no globals.
- [ ] Implement `ImportFileIfPresent` with the mapping-count gate before file parsing: absent file continues silently; non-empty table logs an informational skip and remains authoritative.
- [ ] Open `sic_mappings.csv` read-only with normal symbolic-link resolution, inspect the opened target, and reject targets over 10 MiB before CSV parsing or buffering; UOW-1 must not write/delete through this seed path.
- [ ] Parse the exact five-column header, validate the entire file without mutation, normalize/deduplicate SIC, load category resolution data once, apply exact-then-unique-case-insensitive name matching, require supplied ID agreement, and reject ID-only references.
- [ ] Accumulate safe row diagnostics without dumping record contents. Any validation error imports zero rows and continues startup; non-absence read errors and persistence/commit failures return contextual startup errors.
- [ ] Persist only a fully valid candidate set through the repository's atomic prepared bulk insert.
- [ ] Keep temporary candidates/category indexes operation-scoped; add no cache, worker, queue, or new runtime component.
- [ ] Trace to US-08, US-09; BR-CSV-01 through BR-CSV-08; BR-CAT-01 through BR-CAT-06; BR-ERR-01 through BR-ERR-03; NFRP-U1-03/04/06/08.

### Step 7 — Wire startup dependencies

- [ ] Modify `cmd/privateledger/main.go` to define the fixed `sic_mappings.csv` name, construct the SIC mapping repository/service after successful migration, and invoke startup import before the server accepts requests.
- [ ] Apply the approved failure classification: continue for absent, existing-table, oversized, or validation-rejected seed outcomes; fail startup for required database/read/persistence initialization errors.
- [ ] Log structured safe outcomes through `slog`; do not log CSV rows, transaction data, or financial payloads.
- [ ] Keep wiring in `main.go`; do not introduce service globals/singletons or routes/UI belonging to UOW-2/UOW-3.
- [ ] Trace to US-08, US-09; NFR-U1-SEC-01/02; NFR-U1-MAINT-01; LC-U1-08.

### Step 8 — Production-only formatting and static verification

- [ ] Run `gofmt` on changed production Go files.
- [ ] Run `go build ./...` and `go vet ./...` without creating/modifying tests.
- [ ] Run the existing test suite only as regression feedback; do not author, modify, delete, skip, or weaken tests.
- [ ] Inspect the production diff for accidental sensitive logging, hardcoded user paths, non-parameterized SQL values, changed duplicate keys, global state, layer inversion, and files outside UOW-1.
- [ ] Verify no duplicate replacement files were created.
- [ ] If a production failure is found, fix production only within the preceding approved steps and rerun checks.

### Step 9 — Production documentation and traceability summary

- [ ] Create `aidlc-docs/construction/sic-import-and-storage/code/production-summary.md` listing production files modified/created, implemented contracts, commands/results, known limitations, and traceability to US-01/07/08/09.
- [ ] State explicitly that the production role authored no verification tests or fixtures.
- [ ] Record any approved-plan discrepancy rather than silently expanding scope.
- [ ] No README, API documentation, frontend documentation, or deployment artifact is generated because UOW-1 adds no user-facing endpoint, UI, or deployment topology.

### Step 10 — Prepare independent review/test handoff

- [ ] Create `aidlc-docs/construction/sic-import-and-storage/code/independent-review-handoff.md` containing approved artifact paths, the production diff/revision identifier, exact production-file scope, known limitations, and commands already run.
- [ ] Require independent review against approved artifacts rather than production reasoning.
- [ ] Require file/line findings with severity, acceptance-criteria traceability, and final PASS/FAIL.
- [ ] Do not create the independent review artifact or prescribe exact test implementations; ownership remains with the independent role.

### Step 11 — Independent review and verification gate

- [ ] In a separate provider session, invoke the repository's independent review/test role with the handoff and approved artifacts.
- [ ] Require independently authored tests for parser SIC cases; fresh/legacy/repeated/failed migration and data preservation; connection timeout behavior; transaction/mapping round trips and constraints; seed absent/oversized/symlink/valid/invalid/repeated/non-empty/persistence-failure behavior; atomicity; and SIC-free compatibility.
- [ ] Require independently owned `pgregory.net/rapid` property tests for normalization/validation and feasible seed invariants, including shrinking/replay evidence.
- [ ] Require performance evidence for the approved SIC-free regression, 100,000-row legacy migration, and accepted 100,000-row seed targets on a recorded environment.
- [ ] Require race-detector evidence only if UOW-1 introduces/exercises concurrent state; otherwise record why it is not applicable.
- [ ] Production findings return to the production role; the independent role alone corrects tests that contradict approved artifacts and documents why.
- [ ] Repeat review after material production fixes until required tests and performance targets pass and no Blocking/High findings remain.
- [ ] Gate closes only when `aidlc-docs/construction/sic-import-and-storage/code-review/independent-review.md` exists and reports final status PASS.

## Story Completion Checklist

- [ ] US-01 — SIC-bearing and SIC-absent transaction import/storage contracts implemented.
- [ ] US-07 — Fresh and legacy database migration is additive, repeatable, bounded, and data-preserving.
- [ ] US-08 — Optional startup seed follows discovery, empty-table, validation, category-resolution, and atomicity rules.
- [ ] US-09 — All UOW-1 SIC processing remains local-only with privacy-safe diagnostics.

## Exit Criteria

- [ ] Every production step and story checkbox is complete.
- [ ] Production build/vet/format and existing regression tests pass.
- [ ] Production summary and independent handoff are complete.
- [ ] Independent review/test provider has authored and run required verification.
- [ ] Performance and property evidence is recorded.
- [ ] Independent review final status is PASS.
- [ ] No required test fails and no Blocking/High finding remains.
- [ ] User explicitly approves completed UOW-1 Code Generation before the workflow advances.
