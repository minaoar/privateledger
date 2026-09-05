# Independent Review and Test Report — UOW-1 SIC Import and Storage

## 1. Role and Model Identification

| Field | Value |
|---|---|
| Role | Independent code reviewer and test author (AI-DLC Code Generation Part 3) |
| Model | Claude Opus 5 (`claude-opus-5`), Anthropic |
| Provider session | Separate from the production-code provider session (Codex) |
| Agent definition | `.claude/agents/independent-test-reviewer.md` |
| Ownership exercised | Production-code review; test strategy and design; all `_test.go` files, fixtures and testdata; test-only dependency (`pgregory.net/rapid`); this artifact |
| Ownership NOT exercised | No production Go file, `schema.sql`, template, configuration, or production documentation was modified |
| Review date | 2026-09-05 |

The production summary at `aidlc-docs/construction/sic-import-and-storage/code/production-summary.md` was treated as an unverified claim. Every command result recorded below was executed independently by this role.

## 2. Revision Reviewed

| Field | Value |
|---|---|
| Production revision | `513e23b4ffb920a2744e248b2ce5523829b39c8b` — "feat: add SIC import and storage foundation" |
| Baseline | `abd99633903c21d72be6c5e96c332d526f36acf6` |
| Branch | `support-mcc-for-category` |
| Diff command | `git diff abd9963 513e23b` |
| Production diff scope | 6 modified + 3 created Go/SQL files (1313 insertions, 89 deletions including documentation) |

Production files reviewed:

- Modified: `internal/model/transaction.go`, `internal/parser/ofx_parser.go`, `internal/database/schema.sql`, `internal/database/db.go`, `internal/repository/transaction_repo.go`, `cmd/privateledger/main.go`
- Created: `internal/model/sic_mapping.go`, `internal/repository/sic_mapping_repo.go`, `internal/service/sic_mapping_service.go`

Surrounding source read for regression assessment but not in the diff: `internal/service/import_service.go`, `internal/service/categorizer.go`, `internal/repository/category_repo.go`, `internal/service/insights_service.go`.

## 3. Artifacts Consulted

Authorities used to derive expected behaviour (production reasoning was deliberately excluded as an authority):

1. `PROJECT_GUIDELINES.md`
2. `aidlc-docs/aidlc-state.md` (Extension Configuration: property-based testing = Partial)
3. `aidlc-docs/inception/requirements/requirements.md` (FR1-FR14, NFR1-NFR6, AC1-AC17)
4. `aidlc-docs/inception/user-stories/stories.md` (US-01, US-07, US-08, US-09 assigned to UOW-1)
5. `aidlc-docs/inception/application-design/unit-of-work.md`
6. `aidlc-docs/inception/application-design/unit-of-work-dependency.md`
7. `aidlc-docs/inception/application-design/unit-of-work-story-map.md`
8. `aidlc-docs/construction/sic-import-and-storage/functional-design/{domain-entities,business-rules,business-logic-model}.md`
9. `aidlc-docs/construction/sic-import-and-storage/nfr-requirements/{nfr-requirements,tech-stack-decisions}.md`
10. `aidlc-docs/construction/sic-import-and-storage/nfr-design/{nfr-design-patterns,logical-components}.md`
11. `aidlc-docs/construction/plans/sic-import-and-storage-code-generation-plan.md`
12. `aidlc-docs/construction/sic-import-and-storage/code/independent-review-handoff.md`

## 4. Acceptance-Criteria Traceability Matrix

### 4.1 UOW-1 stories and their unit-level acceptance focus

| Story / Criterion | Requirement | Verdict | Evidence |
|---|---|---|---|
| US-01 — SIC present in OFX/QFX is stored | FR1, FR2 | PASS | `TestParseOFXFile_SICExtraction`, `TestImport_StoresSICEndToEnd`, `TestTransactionRepository_SICRoundTrip` |
| US-01 — missing `<SIC>` imports successfully with no SIC | FR1, AC2 | PASS | `TestParseOFXFile_SICExtraction/no_SIC_element`, `TestParseOFXFile_SICFreeFileRegression`, `TestImport_SICFreeFileBehaviourUnchanged` |
| US-01 — parsed SIC `0` remains unset | BR-SIC-01 | PASS | `TestParseOFXFile_SICExtraction/{explicit_zero_SIC,zero_padded_zero_SIC}` |
| US-01 — dedup key unchanged, excludes SIC | FR2, BR-TXN-01, AC1 | PASS | `TestTransactionRepository_SICIsNotPartOfDeduplication`, `TestImport_SICDoesNotAffectDeduplication`, `TestMigrate_LegacyDatabasePreservesData` |
| US-01 — combined `total_auto_categorized` shape unchanged | FR12 | PASS | `TestImport_SICFreeFileBehaviourUnchanged`, `TestImport_SICDoesNotCategorize` |
| US-07 — legacy DB gains schema without data loss | FR3, AC7 | PASS | `TestMigrate_LegacyDatabasePreservesData` (row counts + field-level assertions on a fixture built from the verbatim baseline schema) |
| US-07 — accounts/transactions/categories/patterns/import history preserved | NFR2, BR-MIG-03 | PASS | `TestMigrate_LegacyDatabasePreservesData` |
| US-07 — repeated startup has no duplicate-schema error | NFR4, BR-MIG-04 | PASS | `TestMigrate_RepeatedIsIdempotent` (4 runs), `TestMigrate_AlreadyMigratedDatabaseIsUnchanged` |
| US-07 — index created only after the column exists | BR-MIG-04 ordering | PASS | `TestMigrate_IndexIsCreatedAfterColumn` |
| US-07 — failed migration is fatal and closes the connection | BR-MIG-06, NFR-U1-REL-02 | PASS | `TestOpen_MigrationFailureIsFatalAndClosesDB` (2 injection modes) |
| US-08 — file present + empty table imports mappings | FR4, AC8 | PASS | `TestImportFileIfPresent_ValidSeed` |
| US-08 — absent file is non-fatal | FR4, AC8 | PASS | `TestImportFileIfPresent_AbsentFile`, `TestImportFileIfPresent_Symlink/dangling_link_is_treated_as_absent` |
| US-08 — non-empty table skips and preserves user edits | FR4, BR-CSV-03, AC8 | PASS | `TestImportFileIfPresent_SkipsWhenMappingsExist`, `TestProperty_NonEmptyTableIsNeverMutated` |
| US-08 — repeated startups never duplicate or revert | NFR4, AC8 | PASS | `TestImportFileIfPresent_RepeatedStartupsAreIdempotent`, `TestProperty_SeedImportIsIdempotent` |
| US-08 — name-first category resolution, ID as confirmation | FR4, BR-CAT-01..06, AC16 | PASS | `TestValidateCSV_CategoryResolutionDecisionTable` (15 cases covering the full decision table) |
| US-08 — descriptions stored for display | BR-CSV-08 | PASS | `TestImportFileIfPresent_ValidSeed`, `TestProperty_SeedCSVRoundTrip` |
| US-08 — omitted category columns give an intentional NULL | FR5, BR-CAT-01 | PASS | `TestImportFileIfPresent_ValidSeed`, `TestSICMappingRepository_CRUDRoundTrip` |
| US-08 — invalid rows reported/skipped per the design | BR-CSV-05/06 | PASS | `TestImportFileIfPresent_InvalidSeedIsNonFatalAndAtomic` (21 cases), `TestValidateCSV_ReportShape`, `TestProperty_AnyInvalidRowImportsNothing` |
| US-09 — no external SIC lookup | NFR1, NFR-U1-SEC-01 | PASS | Static: no `net/*` import in any UOW-1 file; `go list -deps` shows `net/http` reachability is identical at baseline and candidate and originates only in the pre-existing `ofxgo` client |
| US-09 — local files and SQLite only | NFR1 | PASS | Reviewed `os`/`encoding/csv`/`database/sql` usage only; all tests run against temp dirs |
| AC6 — duplicate SIC mapping rejected | FR8 | PASS | `TestSICMappingRepository_UniqueCodeRejected`, `TestSICMappingUniqueConstraint` |
| AC15 (UOW-1 foundation) — non-digit SIC rejected | FR11 | PASS | `TestParseSICCode_Boundaries`, `TestProperty_NonDigitAlwaysRejected` |
| AC17 — `go test ./...` passes | NFR5 | PASS | Section 7 |

### 4.2 NFR traceability

| NFR | Verdict | Evidence |
|---|---|---|
| NFR-U1-PERF-01 SIC-free import regression <= 10% | PASS | Section 8.1: +0.90% |
| NFR-U1-PERF-02 legacy 100k migration <= 5s | PASS | Section 8.2: 42.49 ms |
| NFR-U1-PERF-03 100k-row seed <= 10s | PASS | Section 8.3: 565.42 ms |
| NFR-U1-PERF-04 environment recorded | PASS | Section 8.0 |
| NFR-U1-SCALE-01 no capacity ceiling, index used, no resident duplicate | PASS | Review of `internal/database/schema.sql` + `db.go:47`; `GetUncategorizedBySICCodes` uses `idx_txn_sic`; no in-memory transaction cache added |
| NFR-U1-REL-01 fresh/legacy/already-migrated convergence | PASS | `TestMigrate_FreshDatabase`, `TestMigrate_LegacyDatabasePreservesData`, `TestMigrate_AlreadyMigratedDatabaseIsUnchanged`, `TestMigrate_RepeatedIsIdempotent` |
| NFR-U1-REL-02 migration failure boundary | PASS | `TestOpen_MigrationFailureIsFatalAndClosesDB` |
| NFR-U1-REL-03 seed availability behaviour | PASS | `TestImportFileIfPresent_{AbsentFile,OversizedSeed,SizeBoundary,InvalidSeed...,ReadFailureIsFatal,PersistenceFailureRollsBack,SkipsWhenMappingsExist}` |
| NFR-U1-REL-04 atomic seed persistence | PASS | `TestSICMappingRepository_BulkInsertAtomic` (5 sub-cases), `TestImportFileIfPresent_PersistenceFailureRollsBack` |
| NFR-U1-COMP-01 existing input/database compatibility | PASS | `TestImport_SICFreeFileBehaviourUnchanged`, `TestParseOFXFile_SICDoesNotDisturbExistingFields`, full pre-existing test suite still green |
| NFR-U1-SEC-01 local-only processing | PASS | See US-09 row above |
| NFR-U1-SEC-02 safe diagnostics | PARTIAL | `TestValidateCSV_DiagnosticsAreSafe` and `TestProperty_RejectionErrorsAreSafe` pass; see findings F-03 (volume) and F-08 (overflow literal) |
| NFR-U1-SEC-03 seed resource boundary (10 MiB, no buffering to enforce) | PASS | `TestImportFileIfPresent_SizeBoundary` pins the exact accept/reject edge; `service.go:68-80` uses `file.Stat()` on the opened target, never a read |
| NFR-U1-MAINT-01 layer direction, no globals | PASS | Constructor injection at `main.go:91,97`; `SICMappingService` depends on repositories only; no package-level mutable state in any new file |
| NFR-U1-MAINT-02 single normalization authority | PASS | Only three non-test call sites exist, all routed to `model.ParseSICCode`: `parser/ofx_parser.go:206`, `repository/sic_mapping_repo.go:59`, `service/sic_mapping_service.go:163` |
| NFR-U1-MAINT-03 focused dependencies | PASS | No production dependency added; `pgregory.net/rapid v1.1.0` added by this role as test-only |
| NFR-U1-TEST-01 example evidence | PASS | Section 6 |
| NFR-U1-TEST-02 property evidence | PASS | Section 6.3 |
| NFR-U1-TEST-03 benchmark evidence | PASS | Section 8 |
| NFR-U1-TEST-04 concurrency evidence | PASS with rationale | Section 9 |
| NFR-U1-TEST-05 ownership gate | PASS | Section 1 |

## 5. Findings

No Blocking and no High findings. Severity counts: **Blocking 0, High 0, Medium 3, Low 2, Informational 7.**

### Medium

#### F-01 (Medium) — `ValidateCSV` labels a purely-validating report as `Imported`

- Location: `internal/service/sic_mapping_service.go:190` (`report.Outcome = model.SICMappingImportImported`), initialized at `:113`.
- Rule violated: `functional-design/domain-entities.md`, SICMappingImportReport invariants — "A valid report does not imply persistence until atomic commit succeeds"; `business-logic-model.md` Flow 4 places `Imported` after the atomic commit step.
- Detail: `ValidateCSV` performs no persistence, yet returns `Outcome = imported`. Additionally `report.ImportedRows` is never assigned anywhere in the codebase, so a successful seed never records its imported count (the count is only emitted through `slog` at `:104-106`).
- Impact in UOW-1: contained. `ImportFileIfPresent` only reads `report.RejectedRows` at `:86`, so no observable UOW-1 behaviour is wrong; all seed decision-path tests pass.
- Impact downstream: this is the report contract UOW-2 must reuse for the FR13/AC "per-row report of what failed" upload response. Shipping an outcome enum that asserts persistence before persistence happens will mislead that consumer.
- Test evidence: `TestValidateCSV_ReportShape` asserts the observable, correct parts (`TotalRows`, `ValidRows`, `RejectedRows`, `ImportedRows == 0`). It deliberately does not assert `Outcome`, because doing so would fail on a contract that has no UOW-1 behavioural consequence; the deviation is recorded here instead.
- Recommendation (production role): set `Outcome` to `SICMappingImportInvalid`/a neutral `validated` state inside `ValidateCSV`, and set `Imported` plus `ImportedRows` only after `BulkInsertAtomic` succeeds.

#### F-02 (Medium) — The startup seed outcome contract is not surfaced; four outcome constants are dead

- Location: `internal/service/sic_mapping_service.go:44` (`func ... ImportFileIfPresent(path string) error`); constants at `internal/model/sic_mapping.go:114-121`; consumer at `cmd/privateledger/main.go:98-101`.
- Rule violated: `nfr-design/logical-components.md` LC-U1-08 — the startup orchestrator must "Apply the returned outcome according to the failure classification" and "Emit structured outcome logs"; the approved interaction sequence shows `Seed Service --> Main: Imported(count)`.
- Detail: `ImportFileIfPresent` returns only `error`. `SICMappingImportAbsent`, `SICMappingImportSkippedExisting`, `SICMappingImportOversized` and `SICMappingImportPersistenceFailed` are declared but never assigned anywhere. `main.go` can therefore only distinguish error from no-error, and all outcome logging happens inside the service instead of at the startup boundary.
- Impact: the *behaviour* mandated by the NFRP-U1-06 failure-classification table is correct and fully verified (all seven rows tested). What is missing is the structured outcome value the design says crosses the service/startup boundary. `domain-entities.md` "Ownership and Layering" does permit "Startup/service boundary" for logging, which is why this is Medium rather than High.
- Recommendation (production role): return `(*model.SICMappingImportReport, error)` and populate the outcome at each decision point, or explicitly record an approved-plan discrepancy.

#### F-03 (Medium) — Unbounded per-row diagnostic logging for an invalid seed

- Location: `internal/service/sic_mapping_service.go:86-98` — one `slog.Warn` per rejected row inside `for _, validationErr := range report.Errors`, with no cap.
- Rule tension: NFR-U1-SEC-03 explicitly bounds the seed *input* at 10 MiB, and NFRP-U1-08 requires minimal diagnostics; nothing bounds the resulting *output*. BR-ERR-02 asks for every invalid row "where practical".
- Measured impact (this reviewer, same reference machine): a fully invalid seed of 100,000 rows occupying **1,400,066 bytes** — only 13% of the permitted limit — produced **100,001 log lines / 23,889,163 bytes** of diagnostic output in 139 ms. That is a 17x input-to-output amplification. At the full 10 MiB allowance the file could hold roughly 750,000 rows and emit on the order of 180 MB per startup. Because the seed file is never consumed or renamed on rejection, this repeats on **every** startup until the user removes or fixes the file, and with `logging.enable_file_logging` enabled it is written to `server.log`.
- Not Blocking/High because startup remains fast, non-fatal and correct (`TestImportFileIfPresent_InvalidSeedIsNonFatalAndAtomic` passes for all 21 malformed-input classes), and no financial payload is written — only row numbers, field names and stable codes.
- Recommendation (production role): cap per-row warnings (for example the first 50) and always emit the aggregate `total_rows`/`rejected_rows` summary that already exists at `:94-97`.

### Low

#### F-04 (Low) — A malformed CSV row stops all further row diagnostics

- Location: `internal/service/sic_mapping_service.go:150-157` — on a `*csv.ParseError` the reader loop records one diagnostic and `break`s.
- Rule: BR-ERR-02 — "identifies every detected invalid row where practical".
- Impact: the *outcome* is correct and verified (zero rows imported, non-fatal, one diagnostic). But a structural error in row 2 of a 5,000-row file hides every other problem, so the user must fix errors one restart at a time. `TestImportFileIfPresent_InvalidSeedIsNonFatalAndAtomic/{ragged_row_with_too_few_fields,ragged_row_with_too_many_fields,unterminated_quoted_field}` confirm the current behaviour.
- Recommendation: for `csv.ErrFieldCount` specifically, record the diagnostic and continue; reserve `break` for unrecoverable reader states.

#### F-05 (Low) — `sqliteDSN` cannot express a database path containing `?`

- Location: `internal/database/db.go:39-46`.
- Detail: the `strings.Contains(path, "?")` branch treats an existing `?` as the start of a DSN query string, but `cfg.Path` is a filesystem path derived from `filepath.Join(execDir, "privateledger.db")`. I verified empirically that opening `<dir>/q?mark.db` creates and uses a database file literally named `q`.
- Regression assessment: **not a regression.** I ran the identical probe against baseline `abd9963`, where `sql.Open("sqlite", cfg.Path)` produced exactly the same file named `q`. The truncation is pre-existing `modernc.org/sqlite` DSN parsing behaviour. Paths containing space, `%`, `#` and `&` were verified to work correctly on both revisions.
- Impact: the new branch gives the appearance of handling a case it does not handle. Extremely unlikely in practice (the path is fixed as `privateledger.db` beside the executable; `?` is illegal in Windows paths).
- Recommendation: either drop the misleading branch or build the DSN as `file:` + `url.PathEscape`-style encoding of the path. No UOW-1 behaviour change required.

### Informational

| ID | Location | Observation |
|---|---|---|
| F-06 | `internal/parser/ofx_parser.go:204-210` | The `slog.Warn("Ignoring invalid parsed SIC value", ...)` branch is unreachable. `ofxgo.Transaction.SIC` is `ofxgo.Int` (= `int64`), so any value satisfying `> 0` lies in `1..MaxInt64` and `ParseSICCode(strconv.FormatInt(...))` cannot fail. Harmless but misleading dead code. Verified by `TestParseOFXFile_SICExtraction/max_int64_SIC` and `TestParseOFXFile_SICOverflowIsWholeFileFailure` (overflow is rejected earlier, inside `ofxgo`). |
| F-07 | `internal/model/sic_mapping.go:16-23` | `NormalizeSICCode` is exported but performs no validation: `NormalizeSICCode("abc")` returns `"abc"` and `NormalizeSICCode("-5")` returns `"-5"`. This matches BR-SIC-04's split of responsibilities, but it is an exported footgun for UOW-3's `SICMappingCategorizer` cache — using it as a lookup/cache key without `ParseSICCode` would admit non-canonical keys and break BR-SIC-05. Recommend a doc-comment warning or unexporting it. Pinned by `TestNormalizeSICCode`. |
| F-08 | `internal/model/sic_mapping.go:46` | The overflow path wraps `strconv.ParseInt`, whose message embeds the rejected literal (`... parsing "9223372036854775808": value out of range`), while every other rejection uses a fixed message. Found by rapid shrinking (see Section 6.3). Not judged a BR-ERR-01/NFR-U1-SEC-02 violation: the protected payload classes are OFX/CSV contents, transaction descriptions and account identifiers, not the single numeric SIC field, and the seed path discards this error in favour of the stable `invalid_sic` code at `:165`. Recommend flattening for consistency. |
| F-09 | `internal/service/sic_mapping_service.go:133-136` | A UTF-8 BOM before the header (Excel's "CSV UTF-8" export) makes `header[0]` equal `"﻿SIC_Code"`, so the whole seed is rejected with `invalid_header` and zero rows import. This is BR-CSV-04-compliant and non-fatal, and the pre-existing OFX parser already strips a BOM (`TestParseOFXFile_HeaderVariants`). Pinned by `TestImportFileIfPresent_InvalidSeedIsNonFatalAndAtomic/utf8_BOM_before_the_header` so UOW-2 can make a deliberate decision for the user-facing upload. |
| F-10 | `internal/repository/transaction_repo.go:100-130`, `:160-270` | `GetByID`/`List` gained `LEFT JOIN sic_mapping` and a `COALESCE(NULLIF(sm.description,''), sm.description_detail)` projection. The FR10 description fallback is assigned to UOW-3/US-06 in the story map, but plan Step 1 authorizes "downstream display-only SIC description where approved", so this is within the letter of the plan. Verified correct: the `UNIQUE` `sic_code` prevents row multiplication (`TestTransactionRepository_SICDescriptionJoin` asserts the `List()` row count), all filters are table-qualified so no column became ambiguous, and NULL `sic_code` yields a NULL description. Measured import cost is inside the PERF-01 budget. Recorded as a scope observation only. |
| F-11 | `internal/database/db.go:57` | `ensureColumn` interpolates the table name into `PRAGMA table_info(...)`. SQLite does not accept bound identifiers in `PRAGMA`, the identifier is an internal constant, the function documents this, and every *value* in the codebase remains parameterized. No action; PROJECT_GUIDELINES "use parameterized SQL for all values" is satisfied. |
| F-12 | `cmd/privateledger/main.go:98-101` vs `:79` | `log.Fatalf` calls `os.Exit(1)`, which skips `defer database.Close(db)`. This exactly matches the pre-existing pattern at `:76-77` and `:207-208`, and process exit releases the handle. Mixing `slog.Error` with `log.Fatalf` also matches existing code, so PROJECT_GUIDELINES' "use `slog`, not `log`" is not newly violated. No action for UOW-1. |

### Explicitly checked and found correct

The following were reviewed against approved rules and are correct; they are recorded so a later reviewer does not have to re-derive them:

- Connection-local pragmas now apply to **every** physical pooled connection, not just the one that happened to run `PRAGMA foreign_keys = ON` at baseline. Verified on six simultaneously-held `*sql.Conn` values: `foreign_keys=1`, `busy_timeout=5000` (NFRP-U1-01). This corrects a latent baseline defect.
- Foreign keys are actually *enforced*, not merely reported: an invalid `sic_mapping.category_id` is rejected, and category deletion sets the mapping reference to NULL while preserving the mapping row (BR-MIG-05).
- Migration ordering is correct: `schema.sql` contains no `idx_txn_sic`; the index is created in `db.go:47` only after `ensureColumn` (BR-MIG-04). A legacy database therefore never hits `CREATE INDEX` on a missing column.
- `Create`/`FindDuplicate`/`GetByID`/`List`/`GetUncategorized` insert and scan lists are positionally aligned with the new `sic_code` column; a misalignment would have surfaced as a scan-type error in the round-trip tests.
- `GetUncategorizedBySICCodes` binds every code as a parameter and short-circuits on empty input; a hostile value returns zero rows rather than altering the query.
- `BulkInsertAtomic`/`ReplaceAll` use one transaction, one reused prepared statement, deferred rollback guarded by a `committed` flag, and report commit failure (NFRP-U1-05). The double `stmt.Close()` (deferred plus explicit) is a documented no-op in `database/sql`.
- The seed size gate inspects the **opened** target via `file.Stat()`, so the check binds to the file actually read and a symlink cannot be swapped for a larger target after validation (NFRP-U1-03).
- Category resolution data is loaded exactly once per operation (`service.go:138`), not once per row (NFRP-U1-04, PERF-03).
- No new production dependency, no global state, no singleton, no handler/UI/route from UOW-2 or UOW-3 leaked into this diff.

## 6. Tests Created

All test files below were authored by this independent role. No production file was modified. No pre-existing test was deleted, skipped, loosened or rewritten.

### 6.1 New test files

| File | Purpose | Test funcs |
|---|---|---|
| `internal/model/sic_mapping_test.go` | SIC normalization/validation boundaries, equivalence classes, mapping display helpers, exact int64 edge | 6 |
| `internal/model/sic_mapping_property_test.go` | `pgregory.net/rapid` properties for canonicalization (PBT-02/03/07/08) | 6 |
| `internal/parser/ofx_sic_test.go` | Parser SIC present/absent/zero/padded/whitespace/max/negative, whole-file failure boundary, per-transaction isolation, SIC-free regression | 7 |
| `internal/database/migration_test.go` | Fresh/legacy/repeated/already-migrated migration, data preservation, ordering, fatal-failure boundary, pooled-connection pragmas, FK enforcement, uniqueness, concurrency | 10 |
| `internal/database/migration_perf_test.go` | NFR-U1-PERF-02 measurement plus its 100k legacy fixture builder | 1 |
| `internal/database/testdata/legacy_schema_pre_uow1.sql` | Verbatim pre-UOW-1 schema captured from `abd9963`, used to build genuine legacy fixtures | fixture |
| `internal/repository/sic_repo_test.go` | Transaction SIC round trips, dedup independence, SIC-scoped query, description join; mapping CRUD, uniqueness, FK, atomic bulk insert/replace, category-deletion behaviour | 9 |
| `internal/service/sic_mapping_seed_test.go` | Full startup-seed decision table, size boundary, symlinks, 21 invalid-input classes, category-resolution decision table, report shape, diagnostic safety, CSV formatting | 11 |
| `internal/service/sic_mapping_seed_property_test.go` | Seed CSV round trip, import idempotency, any-invalid-row atomicity, duplicate-canonical rejection, non-empty-table immutability | 5 |
| `internal/service/sic_import_e2e_test.go` | End-to-end import through `ImportService`: SIC persistence, SIC-free regression, dedup independence, UOW-1 boundary (no SIC categorization) | 4 |
| `internal/service/sic_seed_perf_test.go` | NFR-U1-PERF-03 measurement | 1 |
| `internal/service/import_regression_perf_test.go` | NFR-U1-PERF-01 harness, written against APIs common to baseline and candidate so it can be copied into a baseline worktree | 1 |
| `internal/{database,service}/race_{enabled,disabled}_test.go` | Build-tagged `raceDetectorEnabled` constant so performance assertions skip under `-race` (see Section 9) | 4 files, 0 test funcs |

Total: 61 top-level test functions plus 90+ table-driven sub-cases across 5 packages, and 4 build-tagged support files.

### 6.2 Test-only dependency

`pgregory.net/rapid v1.1.0` added to `go.mod`/`go.sum` per TD-U1-07. `v1.1.0` was chosen deliberately over `v1.3.0`: `go get pgregory.net/rapid@latest` rewrote the module's `go` directive from `1.21` to `1.23`, which would have been an unauthorized change to production build configuration. `v1.1.0` installs cleanly while preserving `go 1.21`.

`go mod tidy` also reclassified `github.com/aclindsa/ofxgo` from `// indirect` to a direct requirement. This is a metadata correction — `internal/parser` has always imported it directly — and changes no resolved version and no production behaviour.

### 6.3 Property-based testing evidence (NFR6 / PBT-02, 03, 07, 08, 09)

- **PBT-09 Framework selection**: `pgregory.net/rapid`, exactly as recorded in `tech-stack-decisions.md` TD-U1-07. Test-only; it appears in no production import.
- **PBT-07 Generator quality**: generators target named domain partitions rather than unconstrained random strings — `genCanonicalSIC` (biased to 1, 9, MaxInt64, MaxInt64-1, typical 4-digit values, and the full `1..MaxInt64` range), `genAsciiWhitespace`, `genDecorated` (leading zeros + surrounding whitespace), `genNonDigitPayload` (ASCII, Latin-1, Arabic-Indic, fullwidth, Greek, NUL and DEL), `genOverflow`, plus service-level `genSeedCategoryNames` and `genValidSeedRows` covering the category-reference combinations.
- **PBT-02 Round-trip**: `TestProperty_CanonicalRoundTrip` (decoration-insensitivity and idempotence of both `ParseSICCode` and `NormalizeSICCode`); `TestProperty_SeedCSVRoundTrip` (rendered valid CSV parses back to exactly the mappings it described, with canonical codes, preserved descriptions and resolved categories).
- **PBT-03 Invariants**: `TestProperty_AcceptedCodeIsCanonical`, `TestProperty_EqualNumbersShareIdentity` (BR-SIC-05), `TestProperty_NonDigitAlwaysRejected` (FR11), `TestProperty_OverflowAlwaysRejected`, `TestProperty_RejectionErrorsAreSafe`, `TestProperty_SeedImportIsIdempotent` (NFR4/AC8), `TestProperty_AnyInvalidRowImportsNothing` (BR-CSV-05/06), `TestProperty_DuplicateCanonicalCodeRejectsFile`, `TestProperty_NonEmptyTableIsNeverMutated` (BR-CSV-03).
- **PBT-08 Shrinking and reproducibility — demonstrated twice, with preserved output**:

  Run 1 (production behaviour surfaced, `internal/model`):

  ```text
  --- FAIL: TestProperty_RejectionErrorsAreSafe (0.00s)
      sic_mapping_property_test.go:195: [rapid] failed after 11 tests: validation error echoes the rejected payload:
        "SIC code exceeds the positive int64 range: strconv.ParseInt: parsing \"9223372036854775808\": value out of range"
          To reproduce, specify -run="TestProperty_RejectionErrorsAreSafe"
            -rapid.failfile="testdata/rapid/TestProperty_RejectionErrorsAreSafe/TestProperty_RejectionErrorsAreSafe-20260905162410-50228.fail"
            (or -rapid.seed=14963172158685939070)
      sic_mapping_property_test.go:196: [rapid] draw raw: "9223372036854775808"
  ```

  rapid shrank an unconstrained overflow generator to the exact minimal counterexample `9223372036854775808` — `math.MaxInt64 + 1`. Recorded as finding F-08.

  Run 2 (test-generator artefact, corrected by this role):

  ```text
  --- FAIL: TestProperty_RejectionErrorsAreSafe (0.00s)
      sic_mapping_property_test.go:208: [rapid] failed after 32 tests: validation error echoes the rejected payload:
        "SIC code must contain ASCII digits only"
          To reproduce, ... (or -rapid.seed=3372624220835020619)
      sic_mapping_property_test.go:209: [rapid] draw raw: "a"
  ```

  rapid shrank to the single character `"a"`, which is an incidental substring of the fixed message "ASCII". This was a defect in my property, not in production code.

- **Corrections made to my own tests, with rationale** (per the reviewer rule that a test contradicting an approved artifact must be fixed and documented):
  1. `TestProperty_RejectionErrorsAreSafe` originally forbade a rejection error from containing *any* rejected payload, including digits-only overflow values. The approved redaction boundary (BR-ERR-01, NFR-U1-SEC-02) protects OFX/CSV payloads, transaction descriptions and account identifiers — not the single numeric SIC field currently being validated, which BR-ERR-02 already permits to be identified by field name and stable code. The property was narrowed to non-digit payloads, and a scope note is embedded in the test source. The behaviour is still reported as F-08.
  2. The same property additionally required a minimum payload length of 8 characters before treating containment as a leak, to eliminate the `"a"`-in-`"ASCII"` generator artefact.
  3. A fixture expectation in `TestMigrate_LegacyDatabasePreservesData` initially asserted `category_source = 1` for a seeded row that the fixture inserts with `category_source = 0`. My fixture was wrong; the expectation was corrected to match the fixture, not the other way round.
  4. `TestSICMappingUniqueConstraint` originally logged rather than asserted the `'0005812'` case. It now asserts the true schema behaviour (the raw-string UNIQUE index does not collapse non-canonical spellings) with a comment recording that BR-SIC-04 assigns collapsing to the domain normalization authority, which is separately verified.
  5. `db.Conn(t.Context())` was replaced with `context.Background()` because `testing.T.Context` requires Go 1.24 and the module declares `go 1.21`; `go vet` correctly rejected it.
  6. The three NFR-U1-PERF assertions were made to skip under `-race`. See Section 9 for the measurement and rationale; the uninstrumented acceptance assertions are unchanged.

No failing test was weakened to obtain a green run. The only assertions relaxed were the two documented generator artefacts above, both of which were demonstrably wrong about the approved artifacts rather than about production behaviour.

## 7. Commands Executed and Exact Results

All commands run from `/Users/tanzil/Documents/GitHub/privateledger` with `GOCACHE=/private/tmp/privateledger-go-cache`.

```text
$ gofmt -l internal/ cmd/
(no output — all files formatted)                                            PASS

$ go build ./...
(no output)                                                                   PASS

$ go vet ./...
(no output)                                                                   PASS

$ go test -count=1 ./... -timeout 30m
?   github.com/oronno/privateledger/cmd/privateledger  [no test files]
?   github.com/oronno/privateledger/internal/config    [no test files]
ok  github.com/oronno/privateledger/internal/database   2.429s
?   github.com/oronno/privateledger/internal/handler   [no test files]
?   github.com/oronno/privateledger/internal/logger    [no test files]
?   github.com/oronno/privateledger/internal/middleware [no test files]
ok  github.com/oronno/privateledger/internal/model      0.185s
ok  github.com/oronno/privateledger/internal/parser     0.698s
ok  github.com/oronno/privateledger/internal/repository 0.587s
ok  github.com/oronno/privateledger/internal/service   23.736s
                                                                              PASS

$ go test -race -count=1 ./... -timeout 40m
ok  github.com/oronno/privateledger/internal/database   2.307s
ok  github.com/oronno/privateledger/internal/model      1.306s
ok  github.com/oronno/privateledger/internal/parser     2.645s
ok  github.com/oronno/privateledger/internal/repository 1.955s
ok  github.com/oronno/privateledger/internal/service   15.297s
(no data races reported; performance assertions skip under -race)             PASS

$ go test -race -count=1 -short ./... -timeout 30m
ok  github.com/oronno/privateledger/internal/database   2.176s
ok  github.com/oronno/privateledger/internal/model      1.852s
ok  github.com/oronno/privateledger/internal/parser     2.172s
ok  github.com/oronno/privateledger/internal/repository 2.250s
ok  github.com/oronno/privateledger/internal/service    6.457s
(no data races reported)                                                      PASS

$ go test ./internal/model/ -count=3
ok  github.com/oronno/privateledger/internal/model      0.244s
(property tests re-drawn with fresh seeds three times)                        PASS
```

An earlier full-suite run failed on `TestProperty_RejectionErrorsAreSafe`. That failure was in a test authored by this role, was analysed against the approved artifacts, and was corrected as documented in Section 6.3. It did not correspond to a production defect requiring a fix, and no production code was changed.

Static verification of the local-only boundary:

```text
$ grep -rn "net/http\|net\.\|http\.\|url\." <all six UOW-1 production files>
(no matches)                                                                  PASS

$ go list -deps ./internal/parser | grep -x net/http     # candidate
net/http
$ (cd <baseline worktree> && go list -deps ./internal/parser | grep -x net/http)
net/http
(identical; originates in the pre-existing github.com/aclindsa/ofxgo client)   PASS
```

## 8. Performance Evidence

### 8.0 Reference environment (NFR-U1-PERF-04)

| Item | Value |
|---|---|
| CPU model | Apple M1 |
| Logical CPUs | 8 (8 physical) |
| Installed RAM | 16 GiB (17,179,869,184 bytes) |
| Operating system | macOS 15.5 (build 24F74), Darwin 24.5.0 arm64 |
| Go version | go1.26.0 darwin/arm64 |
| Storage | Internal SSD, Apple Fabric protocol (`Solid State: Yes`) |
| Competing workload | None intentionally running; measurements taken back to back on an otherwise idle machine |

These targets are acceptance evidence on this machine, not universal guarantees across supported hardware.

### 8.1 NFR-U1-PERF-01 — SIC-free import regression (<= 10% median)

Protocol followed exactly as specified: identical fixture, identical application configuration, identical measurement boundary (`ImportService.ImportOFX` only), one discarded warm-up run, five measured runs per revision, median compared. Each run used a fresh temporary database, one account, 50 categories and 200 text patterns, importing a deterministic 4,000-transaction OFX statement containing no `<SIC>` element. The candidate harness file was copied verbatim into a `git worktree` of baseline `abd9963` — it deliberately uses only APIs present in both revisions.

| Revision | Samples (sorted) | Median |
|---|---|---|
| Baseline `abd9963` | 3.0118s, 3.0718s, **3.0899s**, 3.1057s, 3.1503s | 3.0899 s |
| Candidate `513e23b` | 3.0786s, 3.0902s, **3.1176s**, 3.1183s, 3.1372s | 3.1176 s |

Ratio **1.0090 (+0.90%)** against a **+10.00%** budget. The two sample ranges overlap substantially, so the difference is within measurement noise. **PASS.**

### 8.2 NFR-U1-PERF-02 — Legacy migration (<= 5s)

Fixture built on the verbatim pre-UOW-1 schema with 100,000 transactions, 100 categories, 1,000 text patterns and 100 import-history rows. Fixture construction is outside the measured window; measurement starts immediately before `database.Open` and ends once the schema is ready for repository use.

| Measurement | Result | Target |
|---|---|---|
| First migration (adds column + table + index) | **42.49 ms** | 5 s |
| Already-migrated startup, median of 5 runs | **289.96 µs** | 5 s |

Data preservation was asserted inside the same test: the post-migration transaction count is exactly 100,000 and `sic_code` is present. **PASS** with roughly 118x headroom.

### 8.3 NFR-U1-PERF-03 — Startup seed (<= 10s)

A valid 100,000-row `sic_mappings.csv` of 2,558,962 bytes (inside the 10 MiB limit), mixing intentionally-unmapped rows, name-resolved rows and name+ID-confirmed rows across 10 categories. Fixture generation and database opening are excluded; measurement spans validation through commit.

| Measurement | Result | Target |
|---|---|---|
| Whole-file validation + atomic insert of 100,000 rows | **565.42 ms** | 10 s |

All 100,000 mappings were verified present after the commit. **PASS** with roughly 17.7x headroom.

## 9. Concurrency Evidence (NFR-U1-TEST-04)

Per NFR-U1-TEST-04, race-detector verification is required only if the implementation introduces or exercises concurrent state, and the absence of such state must be explicitly recorded rather than met with artificial concurrency tests.

**Recorded determination**: UOW-1 introduces no concurrent in-process state. `SICMappingService`, `SICMappingRepository`, the migration functions and the parser change contain no goroutine, channel, mutex, or shared mutable package-level variable. Startup seeding is strictly synchronous and completes before the HTTP server accepts requests. The thread-safe `SICMappingCategorizer` cache that will need race coverage belongs to UOW-3.

However, the diff *does* change how connection-local settings are established on a `*sql.DB` that the Gin server shares across request goroutines (`db.go:23,39-46`). That is a genuine concurrency-adjacent change, so one targeted test was added rather than an artificial one:
`TestOpen_ConcurrentAccessUnderPooledConnections` drives 8 goroutines x 25 interleaved writes and counts through the shared pool. It passes cleanly under `-race`, and the full suite was additionally run under `-race` with no data race reported (Section 7).

### Race detector and performance measurement are mutually exclusive

An intermediate `go test -race -count=1 ./...` run failed `TestPerformance_StartupSeed` at **21.08 s** against its 10 s target — the same workload that completes in **565.42 ms** uninstrumented. This is race-detector overhead (roughly 37x on this workload), not a production regression, and NFR-U1-PERF-03 specifies acceptance evidence on a recorded reference environment rather than an instrumented build.

Rather than leave a misleading failure or loosen the target, I added build-tagged `raceDetectorEnabled` constants (`internal/{database,service}/race_{enabled,disabled}_test.go`) so the three NFR-U1-PERF assertions skip under `-race`. This is a test-only change that weakens no assertion in the configuration the NFR actually governs: the uninstrumented acceptance numbers in Section 8 remain fully asserted, and the full suite now passes under `-race` with and without `-short`.

## 10. Coverage Gaps and Untestable Requirements

| Gap | Reason | Mitigation |
|---|---|---|
| Busy-timeout behaviour under *prolonged* real lock contention (NFRP-U1-01 "fails rather than hanging indefinitely") | Reliably holding a SQLite write lock past 5 s from within a single test process is timing-dependent and would add a 5+ second wall-clock test with a real flake risk | The configured value is proven active on every physical connection (`busy_timeout=5000` read back from 6 simultaneous connections), and normal contention is proven absorbed by `TestOpen_ConcurrentAccessUnderPooledConnections`. The unverified part is only the driver's own documented timeout semantics. |
| `cmd/privateledger/main.go` startup wiring | `main()` performs `os.Exit` via `log.Fatalf` and resolves paths from `os.Executable()`; it has no injectable seam and the plan explicitly forbids adding test-only production hooks (NFRP-U1-09) | Every branch main.go can take is exercised at the service level: nil error for absent/skip/oversized/invalid, non-nil error for read failure, count failure and persistence failure. The wiring itself is 4 lines and was reviewed by inspection. |
| `report.Outcome` value assertions | Asserting the documented enum would fail on finding F-01, which has no observable UOW-1 behavioural consequence | Recorded as Medium finding F-01 rather than as a failing test, so the production role can correct the contract before UOW-2 consumes it. `ImportedRows` is asserted. |
| Windows and Linux path/symlink behaviour | Only macOS/arm64 was available | Symlink tests skip on Windows via `runtime.GOOS`; `filepath.Join` is used throughout; the permission-bit test skips when running as root. |
| Non-numeric `<SIC>` recovery | Explicitly out of scope (BR-TXN-04, requirements "Out of Scope") | `TestParseOFXFile_NonNumericSICFailsWholeFile` pins the documented whole-file failure boundary so the exclusion stays deliberate. |
| UOW-2/UOW-3 behaviour (mapping UI/API, upload/download/backup, categorization priority, modal display, cache concurrency) | Out of the UOW-1 completion boundary | Not tested here by design. `TestImport_SICDoesNotCategorize` pins the UOW-1 boundary so a later unit cannot silently move categorization into import. |

## 11. Production Fixes Requested and Status

No fix is required for this gate to close. The following are carried forward for the production role.

| ID | Severity | Requested change | Blocking this gate? | Status |
|---|---|---|---|---|
| F-01 | Medium | Set `Outcome`/`ImportedRows` only after a successful atomic commit (`sic_mapping_service.go:190`) | No | Open — recommended before UOW-2 consumes the report contract |
| F-02 | Medium | Return the `SICMappingImportReport` from `ImportFileIfPresent` and populate all five outcomes, or record an approved-plan discrepancy (`sic_mapping_service.go:44`) | No | Open — recommended before UOW-2 |
| F-03 | Medium | Cap per-row seed warning logs and retain the aggregate summary (`sic_mapping_service.go:86-98`) | No | Open |
| F-04 | Low | Continue collecting diagnostics after `csv.ErrFieldCount` (`sic_mapping_service.go:150-157`) | No | Open |
| F-05 | Low | Remove or correct the misleading `?` branch in `sqliteDSN` (`db.go:39-46`) | No | Open — pre-existing driver behaviour, not a regression |
| F-06 | Informational | Remove unreachable SIC parse-error branch (`ofx_parser.go:204-210`) | No | Open |
| F-07 | Informational | Document or unexport `NormalizeSICCode` before UOW-3 uses it as a cache key (`sic_mapping.go:16-23`) | No | Open |
| F-08 | Informational | Use a fixed message on the overflow path (`sic_mapping.go:46`) | No | Open |
| F-09 | Informational | Decide BOM handling deliberately in the UOW-2 upload path | No | Open — UOW-2 decision |
| F-10 | Informational | None; scope observation about the `sic_mapping` join arriving in UOW-1 | No | Noted |
| F-11 / F-12 | Informational | None required | No | Noted |

No production defect was found that causes incorrect observable UOW-1 behaviour, so no re-review pass after a production fix was necessary. If the production role addresses F-01 or F-02, those are material contract changes and require another independent review pass before UOW-2 begins.

## 12. Final Gate Status

```text
FINAL STATUS: PASS
```

Justification against the gate rules:

- Required tests: all pass. `go build ./...`, `go vet ./...`, `gofmt -l`, `go test -count=1 ./...` and `go test -race -count=1 -short ./...` are all green (Section 7).
- Required performance targets: all pass on the recorded reference environment. NFR-U1-PERF-01 +0.90% against a +10% budget; NFR-U1-PERF-02 42.49 ms against 5 s; NFR-U1-PERF-03 565.42 ms against 10 s (Section 8).
- Property evidence: present, with generator-quality coverage and two preserved shrinking/replay records including framework seeds (Section 6.3).
- Concurrency evidence: the absence of UOW-1 concurrent state is explicitly recorded, and the one concurrency-adjacent change (connection configuration) is covered and clean under `-race` (Section 9).
- Findings: **0 Blocking, 0 High**, 3 Medium, 2 Low, 7 Informational. All Medium and Low findings are contract-quality, diagnostic-volume or cosmetic issues with no incorrect observable UOW-1 behaviour, and none blocks the gate.
- Ownership: production code and verification tests were produced by different providers in separate sessions. This role modified no production file.

UOW-1 acceptance criteria AC1, AC2, AC6, AC7, AC8, AC15 (foundation), AC16 and AC17 are satisfied. AC3, AC4, AC5, AC9-AC14 belong to UOW-2 and UOW-3 and are correctly absent from this unit.

---

# Revision 2 — Independent Re-Review

## R2.1 Role and Model Identification

| Field | Value |
|---|---|
| Role | Independent code reviewer and test author (AI-DLC Code Generation Part 3) |
| Model | Claude Opus 5 (`claude-opus-5`), Anthropic |
| Provider session | Separate from the production-code provider session (Codex) |
| Agent definition | `.claude/agents/independent-test-reviewer.md` |
| Ownership exercised | Re-review of the Revision 2 production diff; new verification tests; this artifact |
| Ownership NOT exercised | No production Go file, `schema.sql`, template, configuration, or production documentation was modified |
| Re-review date | 2026-09-05 |

Sections 1-12 above are the Revision 1 record and are preserved unchanged. This section supersedes them only where it states an explicit new status.

The Revision 2 production summary and `aidlc-state.md` claims (`aidlc-docs/construction/sic-import-and-storage/code/production-summary.md`, "Revision 2 after independent findings") were treated as unverified claims. Every verdict below rests on evidence produced by this role.

## R2.2 Revision Reviewed

| Field | Value |
|---|---|
| Production revision | `a51211a` — "fix: address SIC import review findings" (HEAD of `support-mcc-for-category`) |
| Previously reviewed revision | `513e23b` — Revision 1, gate PASS |
| Original baseline | `abd9963` |
| Diff command | `git diff 513e23b a51211a` |
| Production diff scope | 3 Go files (`cmd/privateledger/main.go`, `internal/model/sic_mapping.go`, `internal/service/sic_mapping_service.go`); the remaining changed paths are AI-DLC documentation |

The complete diff was reviewed, not only the three claimed fixes. No production file outside those three changed, and no schema, repository, parser, or database change is present in Revision 2. `git status` confirms every Revision 1 test file is still present and unmodified.

## R2.3 Pre-Change Baseline Run

Required first step: the unchanged Revision 1 suite was executed against Revision 2 production code **before** any new test was written.

```text
$ GOCACHE=/private/tmp/privateledger-go-cache go test -count=1 ./...
?   	github.com/oronno/privateledger/cmd/privateledger	[no test files]
?   	github.com/oronno/privateledger/internal/config	[no test files]
ok  	github.com/oronno/privateledger/internal/database	2.734s
?   	github.com/oronno/privateledger/internal/handler	[no test files]
?   	github.com/oronno/privateledger/internal/logger	[no test files]
?   	github.com/oronno/privateledger/internal/middleware	[no test files]
ok  	github.com/oronno/privateledger/internal/model	0.302s
ok  	github.com/oronno/privateledger/internal/parser	0.500s
ok  	github.com/oronno/privateledger/internal/repository	0.875s
ok  	github.com/oronno/privateledger/internal/service	23.772s
[exited with code 0]                                                          PASS
```

No Revision 1 test regressed under Revision 2. No Revision 1 test was deleted, skipped, loosened or rewritten at any point in this re-review.

## R2.4 Resolution Verdicts for Revision 1 Findings

### F-01 (Medium) — report outcome timing and `ImportedRows` — **RESOLVED**

Production change: `ValidateCSV` now terminates with `report.Outcome = model.SICMappingImportValidated` (`internal/service/sic_mapping_service.go:189`) instead of `Imported`. `Imported` and `ImportedRows` are assigned only after `BulkInsertAtomic` returns successfully (`:104-105`).

Independent evidence:

| Assertion | Test | Result |
|---|---|---|
| A validating call never claims persistence (`domain-entities.md:183`) | `TestValidateCSV_ValidReportDoesNotClaimPersistence` — `Outcome != imported`, `ImportedRows == 0`, and the `sic_mapping` table is still empty afterwards | PASS |
| A rejected file reports zero imported rows (`domain-entities.md:181`) | `TestValidateCSV_RejectedReportNeverClaimsImport` — `Outcome == invalid`, `ImportedRows == 0`, counts 3/2/1 | PASS |
| `ImportedRows` equals what SQLite actually holds, across the zero/one/many boundary | `TestImportFileIfPresentWithReport_ImportedRowsMatchesPersistedRows` (0, 1, 2, 25 rows) — `ImportedRows == persisted count == ValidRows` | PASS |
| A failed commit never reports imported rows | `TestImportFileIfPresentWithReport_OutcomeDecisionTable/atomic_insert_fails_after_successful_validation` — `Outcome == persistence_failed`, `ImportedRows == 0`, `ValidRows == 2`, zero rows persisted | PASS |

The Revision 1 coverage gap "`report.Outcome` value assertions" is now closed: outcome values are asserted directly rather than deliberately omitted.

### F-02 (Medium) — startup outcome contract and dead outcome constants — **RESOLVED**

Production change: `ImportFileIfPresentWithReport(path) (*model.SICMappingImportReport, error)` (`internal/service/sic_mapping_service.go:50`) assigns an outcome at every decision point — `Absent` (`:54`), `ReadFailed` (`:58`, `:75`, `:82`, `:92`), `PersistenceFailed` (`:64`, `:101`), `SkippedExisting` (`:68`), `Oversized` (`:86`), `Invalid` (`:96`), `Imported` (`:104`). The startup orchestrator consumes it at `cmd/privateledger/main.go:100-109` and applies it in `logSICMappingImportOutcome` (`:220-258`), satisfying LC-U1-08 ("Apply the returned outcome according to the failure classification" and "Emit structured outcome logs without sensitive payloads").

Independent evidence — every row of the NFRP-U1-06 failure-classification table, asserted on the structured outcome, the fatal/non-fatal classification, and the resulting mapping count:

| NFRP-U1-06 row | Outcome asserted | Fatal? | Mappings | Result |
|---|---|---|---|---|
| Seed absent | `absent` | no | 0 | PASS |
| Mapping table non-empty | `skipped_existing`, `ExistingRows == 1`, `TotalRows == 0` (not parsed) | no | 1 preserved | PASS |
| Seed oversized (10 MiB + 1) | `oversized`, `TotalRows == 0` (rejected before parsing) | no | 0 | PASS |
| Seed domain invalid | `invalid`, `ImportedRows == 0`, `RejectedRows == 1`, diagnostics present | no | 0 | PASS |
| Seed fully valid | `imported`, `ImportedRows == 2` | no | 2 | PASS |
| Seed read failure other than absence | `read_failed` | yes | 0 | PASS |
| Empty-table gate fails | `persistence_failed` | yes | n/a | PASS |
| Commit failure after validation | `persistence_failed`, `ImportedRows == 0` | yes | 0 | PASS |

`TestImportFileIfPresentWithReport_OutcomeDecisionTable` additionally asserts a **non-nil report on every path**, including all fatal ones, so the startup orchestrator can always read an outcome.

At the startup boundary, `TestLogSICMappingImportOutcome_StartupOutcomeContract` asserts the emitted structured records (real `slog.JSONHandler`, matching `internal/logger`):

- `absent` and a `nil` report emit **no** record ("Continue silently", NFRP-U1-03).
- `skipped_existing` emits one INFO record carrying `existing_mappings`.
- `oversized` emits one WARN record carrying `maximum_bytes`.
- `imported` emits one INFO record carrying `imported_rows`.
- Every record carries the safe `path` field and stable event name.

### F-03 (Medium) — unbounded per-row diagnostic logging — **RESOLVED for output; residual retention recorded as F-13**

Production change: `maxSICSeedDiagnostics = 50` (`cmd/privateledger/main.go:30`); the per-row loop is capped at `:236-240`; the aggregate summary at `:247-252` now also emits `reported_diagnostics` and `omitted_diagnostics`.

Re-measurement, same reference machine, fixture regenerated to exactly the Revision 1 size (1,400,066 bytes = 66-byte header + 100,000 rows of 14 bytes, every row rejected with one `invalid_sic` diagnostic), driven end to end through `ImportFileIfPresentWithReport` and `logSICMappingImportOutcome`:

| Metric | Revision 1 (recorded) | Revision 1 shape, reproduced in this harness | Revision 2 measured |
|---|---|---|---|
| Diagnostic lines | 100,001 | 100,001 | **51** |
| Diagnostic bytes | 23,889,163 | 28,478,315 | **14,443** |
| Output vs. 1,400,066-byte seed | ~17x amplification | ~20x amplification | **1.03% of the seed** |
| Emission time | 139 ms | — | 218 µs |

The two byte figures for the Revision 1 shape differ because each JSON record embeds the seed path and this run used a longer temporary directory; the line count is path-independent and matches exactly. Against the in-harness reproduction the reduction is **1961x by line and 1972x by byte**. Validation itself took 32.5 ms and remained non-fatal with zero rows persisted.

Regression pins (`TestLogSICMappingImportOutcome_DiagnosticsAreBounded`, 6 sub-cases at 0, 1, 49, 50, 51 and 100,000 rejected rows) assert that output never exceeds `maxSICSeedDiagnostics + 1` records, that `reported_diagnostics + omitted_diagnostics` always equals the total diagnostic count, and that the aggregate summary is always emitted. `TestLogSICMappingImportOutcome_InvalidSeedSummary` pins the summary fields and the stable `field`/`code` shape of each row diagnostic. `TestLogSICMappingImportOutcome_NoSeedPayloadInDiagnostics` and a volume-scale check confirm no rejected row content reaches the log (NFRP-U1-08, NFR-U1-SEC-02).

The bound is not free-standing: it lives in the startup orchestrator, and the report still accumulates one diagnostic per rejected row. That residual is recorded as F-13 below rather than folded into this verdict.

## R2.5 New Findings in Revision 2

Severity counts for Revision 2: **Blocking 0, High 0, Medium 0, Low 3 new (F-13, F-14, F-15), Informational 3 new (F-16, F-17, F-18).**

### Low

#### F-13 (Low) — Diagnostic retention is still proportional to rejected rows

- Location: `internal/service/sic_mapping_service.go:210-217` (`addSICImportError` appends unconditionally), reached from `:154`, `:164`, `:166`, `:244`, `:253`, `:258`, `:266`, `:270`.
- Detail: Revision 2 bounds diagnostic *output* at the startup boundary but not diagnostic *accumulation*. `SICMappingImportReport.Errors` still grows one entry per rejected row (potentially more than one per row, since SIC and category errors are appended independently).
- Measured (`TestStartupDiagnosticRetentionForLargeInvalidSeed`, heap delta around the call with the report kept live, `runtime.GC()` on both sides): **6,707,480 bytes (6.4 MiB) retained for 100,000 diagnostics — 67.1 bytes each**. Extrapolated to the accepted 10 MiB seed limit (~748,983 rows of this shape): **~47.9 MiB retained**, of which 50 entries are ever printed.
- Why Low, not Medium: the allocation is transient and released when startup finishes; NFRP-U1-04 bounds the input at 10 MiB, and no approved artifact places a numeric ceiling on retained diagnostics. BR-ERR-02's "every detected invalid row where practical" arguably favours retention.
- Recommendation (production role): cap accumulation in `ValidateCSV` at a small multiple of the reporting bound while continuing to count `RejectedRows` exactly, so `omitted_diagnostics` stays accurate without retaining every entry.

#### F-14 (Low) — The oversized diagnostic lost the observed size and hardcodes the limit

- Location: `cmd/privateledger/main.go:232-234`; authority constant at `internal/service/sic_mapping_service.go:16`.
- Detail: two separate problems introduced by moving this branch out of the service.
  1. `slog.Int64("maximum_bytes", 10<<20)` is a literal in `main.go` duplicating the unexported `maxSICMappingSeedSize`. The two can drift silently; nothing links them.
  2. Revision 1 logged `slog.Int64("size_bytes", info.Size())`. Revision 2 dropped it: `sic_mapping_service.go:85-88` no longer records the observed size and `SICMappingImportReport` has no field to carry it, so the diagnostic can no longer tell the user how far over the limit the seed is.
- Rule: NFRP-U1-03 and NFRP-U1-06 require a *safe diagnostic* on oversized rejection. A byte count is a safe count under NFRP-U1-08 (which explicitly permits "count" fields), so nothing forced its removal.
- Evidence: `TestOversizedDiagnosticMatchesEnforcedLimit` derives the enforced boundary behaviourally (exactly 10 MiB accepted, 10 MiB + 1 rejected as `oversized`) and asserts the logged `maximum_bytes` equals it. The constants **currently agree** at 10,485,760; the test exists so a future drift fails. The same test records that no `size_bytes` field is present.
- Recommendation (production role): carry the observed size on the report and export or otherwise share the single limit constant.

#### F-15 (Low) — A database-side validation failure is classified `read_failed`

- Location: `internal/service/sic_mapping_service.go:90-94`.
- Detail: when `ValidateCSV` returns a hard error the outcome is unconditionally set to `SICMappingImportReadFailed`. One of `ValidateCSV`'s hard-error paths is the category load at `:138-141` (`s.categoryRepo.GetAll()`), which is a **database** read, not a file read. NFRP-U1-06 classifies "Seed read failure other than absence" separately from database failures, and both LC-U1-08 and the F-02 fix exist precisely so the startup boundary can act on an accurate structured outcome.
- Impact: behaviour is correct — fatal, no mutation, contextual wrapped error. Only the structured label misattributes the cause, which matters because Revision 2 now logs that label at `main.go:104`.
- Evidence: `TestImportFileIfPresentWithReport_ValidationInfrastructureFailure` drops the `category` table, confirms the call is fatal, non-panicking, and leaves zero mappings, and records the observed outcome (`read_failed`) via `t.Log` rather than asserting an incorrect contract.
- Recommendation (production role): distinguish the CSV-reader error path from the category-load error path, classifying the latter as `persistence_failed` (or a dedicated value).

### Informational

| ID | Location | Observation |
|---|---|---|
| F-16 | `internal/model/sic_mapping.go:113-122` vs `functional-design/domain-entities.md:177` | The outcome enum now declares eight values; the approved design enumerates five (Absent, SkippedExisting, Invalid, Imported, PersistenceFailed). `Oversized` arrived in Revision 1; `Validated` and `ReadFailed` in Revision 2. The additions are coherent and `Validated` directly serves the `:183` invariant that F-01 was raised against, but the functional design has not been amended to match. Recommend amending `domain-entities.md` so the approved contract and the code agree before UOW-2 consumes the report. |
| F-17 | `cmd/privateledger/main.go:224-257` | The outcome `switch` has no `default` case, so `validated`, `read_failed` and `persistence_failed` produce no record from this function. Currently harmless and verified so: `validated` never escapes `ImportFileIfPresentWithReport` (it is either replaced at `:96` or overwritten at `:104`), and `read_failed`/`persistence_failed` always accompany a non-nil error handled at `main.go:101-108`. All eight reachable outcomes are enumerated by `TestImportFileIfPresentWithReport_OutcomeDecisionTable` and none is `validated`. A future outcome value would be dropped silently. |
| F-18 | `internal/service/sic_mapping_service.go:43-46` | `ImportFileIfPresent` now has no production caller — a grep across all non-test Go files finds only its own definition and the `main.go` call to the report variant. It is retained as a compatibility wrapper for later units. `TestImportFileIfPresent_WrapperMatchesReportErrorContract` pins its fatal/non-fatal contract across absent/invalid/valid/unreadable so the wrapper cannot drift away from the method it delegates to. |

### Explicitly checked in Revision 2 and found correct

- `mappings, report, err := s.ValidateCSV(file)` at `:90` **reassigns** the outer `report` declared at `:51` (same function scope, `mappings` supplies the new variable), so `:92` operates on the returned report, not a stale one. All eight `return` paths in `ValidateCSV` (`:122`, `:128`, `:130`, `:135`, `:140`, `:157`, `:187`, `:190`) return the non-nil report allocated at `:111`, so `:92` cannot nil-dereference. Exercised by `TestImportFileIfPresentWithReport_ValidationInfrastructureFailure`.
- `err` at `main.go:100` is a reassignment of the `err` already in scope from `:58`, not a shadow; no later code depends on the previous value.
- `report.Errors[:limit]` at `main.go:240` is safe for an empty or nil `Errors` slice; the `0_rejected_rows` sub-case exercises it.
- The oversized branch still inspects the **opened** target via `file.Stat()` (`:80-88`), so the NFRP-U1-03 symlink/TOCTOU property established in Revision 1 is unchanged.
- A header-only seed still yields `imported` with `ImportedRows == 0`, consistent with "ImportedRows: zero unless the complete candidate set commits" for an empty candidate set. Pinned by the `0_rows` sub-case.
- No new production dependency, no global state, no test-only production hook (NFRP-U1-09), and no UOW-2/UOW-3 handler, route or UI leaked into Revision 2.
- Layering is preserved: outcome classification lives in the service, lifecycle and logging policy in the startup orchestrator, exactly as `domain-entities.md` "Ownership and Layering" assigns them.

## R2.6 Carried-Forward Findings — Current Status

| ID | Severity | Location (Revision 2 lines) | Status at `a51211a` |
|---|---|---|---|
| F-01 | Medium | `internal/service/sic_mapping_service.go:104-105`, `:189` | **Resolved** — verified in R2.4 |
| F-02 | Medium | `internal/service/sic_mapping_service.go:50`, `cmd/privateledger/main.go:100-109`, `:220-258` | **Resolved** — verified in R2.4 |
| F-03 | Medium | `cmd/privateledger/main.go:30`, `:236-252` | **Resolved for output** — verified in R2.4; residual retention reopened as F-13 (Low) |
| F-04 | Low | `internal/service/sic_mapping_service.go:151-158` | **Open, unchanged.** The `*csv.ParseError` branch still `break`s the row loop, so one structural error hides all later diagnostics (BR-ERR-02 "where practical"). Not touched by Revision 2. |
| F-05 | Low | `internal/database/db.go:39-46` | **Open, unchanged.** `db.go` is not in the Revision 2 diff. Remains verified as pre-existing `modernc.org/sqlite` DSN behaviour at baseline `abd9963`, not a regression. |
| F-06 | Informational | `internal/parser/ofx_parser.go:204-210` | Open, unchanged — file not in the Revision 2 diff. |
| F-07 | Informational | `internal/model/sic_mapping.go:16-23` | Open, unchanged — Revision 2 touched only the constant block at `:113-122`. |
| F-08 | Informational | `internal/model/sic_mapping.go:46` | Open, unchanged. |
| F-09 | Informational | `internal/service/sic_mapping_service.go:132-136` | Open — UOW-2 decision on BOM handling; behaviour unchanged. |
| F-10 | Informational | `internal/repository/transaction_repo.go` | Noted, unchanged — file not in the Revision 2 diff. |
| F-11 | Informational | `internal/database/db.go:57` | Noted, unchanged. |
| F-12 | Informational | `cmd/privateledger/main.go:101-108` | Noted, unchanged. Revision 2 rewrote this block but preserved the pre-existing `slog.Error` + `log.Fatalf` pattern, so `defer database.Close(db)` is still skipped on this path exactly as elsewhere in `main`. |

## R2.7 Tests Created in Revision 2

All files below were authored by this independent role in this pass. No production file was modified. No Revision 1 test file was modified, deleted, skipped or loosened.

| File | Purpose | Test funcs | Sub-cases |
|---|---|---|---|
| `internal/service/sic_seed_outcome_test.go` | F-01/F-02 at the service contract: outcome timing, `ImportedRows` vs persisted rows, the full NFRP-U1-06 outcome decision table, non-nil report on every path, wrapper error contract, validation-infrastructure failure | 6 | 16 |
| `cmd/privateledger/startup_sic_outcome_test.go` | F-02/F-03 at the startup boundary (`package main`): structured outcome records per outcome, invalid-seed summary shape, the diagnostic bound, payload-free diagnostics, oversized limit/enforcement consistency | 5 | 11 |
| `cmd/privateledger/startup_sic_diagnostic_volume_test.go` | F-03 re-measurement against the 1,400,066-byte invalid seed, with the Revision 1 emission shape reproduced in-harness for an apples-to-apples comparison; plus the F-13 heap-retention measurement | 2 | — |

Total: 13 new top-level test functions and 27 sub-cases across 2 packages. `cmd/privateledger` gains test coverage for the first time, closing the Revision 1 coverage gap "`cmd/privateledger/main.go` startup wiring" for the seed-outcome portion of `main`. No test-only production hook was added: `logSICMappingImportOutcome` is exercised as an existing package-visible function from a `package main` test, which NFRP-U1-09 permits ("package-visible behavior").

One assertion authored during this pass was corrected before being accepted: an initial check that bounded output must stay under 1% of the seed size failed at 1.03%. That threshold was an invented number with no basis in any approved artifact, and it is path-length dependent because each JSON record embeds the seed path. It was replaced with the two reproducible contracts the finding actually concerns — the `maxSICSeedDiagnostics + 1` line bound and non-amplification of the input — with the reasoning recorded in the test comment. No production behaviour was accommodated by the change.

No test-only dependency was added or changed in Revision 2. `pgregory.net/rapid v1.1.0` remains pinned for the reason recorded in Section 6.2; `go.mod`'s `go 1.21` directive is unchanged.

## R2.8 Commands Executed and Exact Results — Revision 2

All commands run from `/Users/tanzil/Documents/GitHub/privateledger` with `GOCACHE=/private/tmp/privateledger-go-cache`.

```text
$ go test -count=1 ./...                       # pre-change baseline, Section R2.3
(all packages ok)                                                             PASS

$ gofmt -l internal/service/sic_seed_outcome_test.go \
         cmd/privateledger/startup_sic_outcome_test.go \
         cmd/privateledger/startup_sic_diagnostic_volume_test.go
(no output)                                                                   PASS

$ gofmt -l internal/ cmd/
(no output)                                                                   PASS

$ go build ./...
(no output)                                                                   PASS

$ go vet ./...
(no output)                                                                   PASS

$ go test -count=1 ./... -timeout 30m
ok  	github.com/oronno/privateledger/cmd/privateledger	0.656s
?   	github.com/oronno/privateledger/internal/config	[no test files]
ok  	github.com/oronno/privateledger/internal/database	2.300s
?   	github.com/oronno/privateledger/internal/handler	[no test files]
?   	github.com/oronno/privateledger/internal/logger	[no test files]
?   	github.com/oronno/privateledger/internal/middleware	[no test files]
ok  	github.com/oronno/privateledger/internal/model	0.876s
ok  	github.com/oronno/privateledger/internal/parser	0.618s
ok  	github.com/oronno/privateledger/internal/repository	0.843s
ok  	github.com/oronno/privateledger/internal/service	25.234s
[exited with code 0]                                                          PASS

$ go test -race -count=1 ./... -timeout 30m
ok  	github.com/oronno/privateledger/cmd/privateledger	3.882s
?   	github.com/oronno/privateledger/internal/config	[no test files]
ok  	github.com/oronno/privateledger/internal/database	2.343s
?   	github.com/oronno/privateledger/internal/handler	[no test files]
?   	github.com/oronno/privateledger/internal/logger	[no test files]
?   	github.com/oronno/privateledger/internal/middleware	[no test files]
ok  	github.com/oronno/privateledger/internal/model	1.221s
ok  	github.com/oronno/privateledger/internal/parser	2.719s
ok  	github.com/oronno/privateledger/internal/repository	2.578s
ok  	github.com/oronno/privateledger/internal/service	16.299s
[exited with code 0]                                                          PASS
```

The full `-race` run was executed without `-short` this time. Zero data races were reported. The three performance tests skip under `-race` through the pre-existing build-tagged `raceDetectorEnabled` constant (Section 9); no performance target was loosened to obtain this result. The new `cmd/privateledger` tests do run under `-race` and are clean.

## R2.9 Performance Evidence — Revision 2

Reference environment re-confirmed as identical to Section 8.0: Apple M1, 8 logical/8 physical cores, 16 GiB RAM, macOS 15.5 (build 24F74), Darwin 24.5.0 arm64, go1.26.0 darwin/arm64, internal SSD, otherwise idle machine.

### NFR-U1-PERF-01 — SIC-free import regression (budget: median +10%)

Both revisions re-measured back to back in this session using the identical harness, protocol (one discarded warm-up, five measured runs, median) and fixture. The baseline was measured in a `git worktree` of `abd9963` with the harness and its two build-tagged support files copied in verbatim; the worktree was removed afterwards.

| Revision | Samples (sorted) | Median |
|---|---|---|
| Baseline `abd9963` | 3.088209167s, 3.091147792s, **3.130470292s**, 3.158906750s, 3.228210792s | 3.130470292 s |
| Candidate `a51211a` | 3.061950417s, 3.078200125s, **3.099974834s**, 3.102668834s, 3.140336125s | 3.099974834 s |

Ratio **0.9903 (-0.97%)** against a **+10.00%** budget. The candidate median is marginally *faster* than the baseline and the sample ranges overlap heavily, which is the expected result: Revision 2 changes nothing on the import path. Revision 1 measured +0.90% on the same harness. **PASS.**

### NFR-U1-PERF-02 — Legacy migration (target: 5 s)

```text
NFR-U1-PERF-02 first migration of a 100000-transaction legacy database: 43.346458ms (target 5s)
NFR-U1-PERF-02 already-migrated startup median over 5 runs: 289.584µs (target 5s)
```

43.35 ms against a 5 s target, 115x margin. Revision 1 measured 42.49 ms. **PASS.**

### NFR-U1-PERF-03 — Startup seed (target: 10 s)

```text
NFR-U1-PERF-03 validation+commit of a 100000-row / 2558962-byte seed: 545.33725ms (target 10s)
```

545.34 ms against a 10 s target, 18x margin. Revision 1 measured 565.42 ms. **PASS.**

### Diagnostic-path timing (not an approved target; recorded for F-03)

Validation of the 100,000-row fully invalid seed took 32.5 ms and diagnostic emission 218 µs, against Revision 1's 139 ms of emission alone.

## R2.10 Coverage Gaps — Revision 2

The Revision 1 gap table (Section 10) still applies except where noted.

| Gap | Change at Revision 2 |
|---|---|
| `cmd/privateledger/main.go` startup wiring | **Partially closed.** `logSICMappingImportOutcome` is now directly tested from `package main` across every outcome, including the nil-report guard. The remaining untested part is the `main()` body itself — path resolution from `os.Executable()`, the `log.Fatalf` exit at `:107`, and dependency wiring — which still has no injectable seam and which NFRP-U1-09 forbids adding one for. That residue is 10 lines and was reviewed by inspection. |
| `report.Outcome` value assertions | **Closed.** Outcome values are now asserted directly at both the service and startup boundaries. |
| Busy-timeout under prolonged real lock contention | Unchanged — still not directly exercised, for the reason recorded in Section 10. |
| Windows and Linux path/symlink behaviour | Unchanged — only macOS/arm64 available. |
| Retained-diagnostic ceiling | **New gap.** F-13 is measured, not asserted, because no approved artifact states a retention limit. If the production role adopts a cap, that cap becomes assertable. |
| Non-numeric `<SIC>` recovery; UOW-2/UOW-3 behaviour | Unchanged — out of the UOW-1 boundary by design. |

Property-based testing evidence (Section 6.3) is unchanged and still valid: Revision 2 altered no normalization, validation or seeding logic that the `rapid` properties cover, and those properties passed in every full-suite run above.

## R2.11 Production Fixes Requested and Status — Revision 2

| ID | Severity | Requested change | Blocking this gate? | Status |
|---|---|---|---|---|
| F-01 | Medium | Assign `Outcome`/`ImportedRows` only after a successful atomic commit | No | **Resolved at `a51211a`**, independently verified |
| F-02 | Medium | Return the report and populate all outcomes; apply them at the startup boundary | No | **Resolved at `a51211a`**, independently verified |
| F-03 | Medium | Cap per-row seed warnings and retain the aggregate summary | No | **Resolved at `a51211a`** for output, independently re-measured; residual retention reopened as F-13 |
| F-13 | Low | Cap diagnostic accumulation in `ValidateCSV`, keeping `RejectedRows` and `omitted_diagnostics` exact (`sic_mapping_service.go:210-217`) | No | Open — new in Revision 2 |
| F-14 | Low | Share one limit constant and carry the observed seed size in the oversized diagnostic (`main.go:232-234`, `sic_mapping_service.go:16`, `:85-88`) | No | Open — new in Revision 2 |
| F-15 | Low | Stop classifying a category-load failure as `read_failed` (`sic_mapping_service.go:90-94`, `:138-141`) | No | Open — new in Revision 2 |
| F-04 | Low | Continue collecting diagnostics after `csv.ErrFieldCount` (`sic_mapping_service.go:151-158`) | No | Open — carried forward |
| F-05 | Low | Remove or correct the misleading `?` branch in `sqliteDSN` (`db.go:39-46`) | No | Open — carried forward, pre-existing |
| F-16 | Informational | Amend `domain-entities.md:177` so the approved outcome enum matches the eight implemented values | No | Open — new in Revision 2; recommend before UOW-2 |
| F-17 | Informational | Add a `default` case to the startup outcome switch (`main.go:224-257`) | No | Open — new in Revision 2 |
| F-18 | Informational | None; `ImportFileIfPresent` is an intentional compatibility wrapper with no current production caller | No | Noted |
| F-06 - F-12 | Informational | As recorded in Section 11 | No | Open/Noted — carried forward unchanged |

No finding in Revision 2 causes incorrect observable UOW-1 behaviour. F-13, F-14 and F-15 are quality and diagnostic-accuracy issues introduced or exposed by the Revision 2 fixes; none changes a persisted value, a fatal/non-fatal classification, or an acceptance criterion.

Because all three Revision 1 Medium findings were material contract changes, this second independent pass was required and has now been performed, as Section 11 anticipated.

## R2.12 Acceptance-Criteria Status — Revision 2

The Section 4 traceability matrix is unchanged in substance: Revision 2 altered no parser, schema, repository, migration or import behaviour, and the full Revision 1 suite passes unmodified (Section R2.3). Two entries strengthen:

| Criterion | Change |
|---|---|
| AC16 / AC17 (startup seed behaviour and diagnostics) | Now verified through asserted structured outcomes at both the service and startup boundaries, and with bounded, payload-free diagnostics re-measured at volume, rather than through error/no-error inference alone. |
| NFR-U1-SEC-02, NFRP-U1-08 (safe, minimal diagnostics) | Strengthened: diagnostics are bounded in volume as well as in content, and the bound is regression-pinned. |

UOW-1 acceptance criteria AC1, AC2, AC6, AC7, AC8, AC15 (foundation), AC16 and AC17 remain satisfied. AC3, AC4, AC5 and AC9-AC14 belong to UOW-2 and UOW-3 and are correctly absent from this unit.

## R2.13 Final Gate Status — Revision 2

```text
REVISION 2 (a51211a) FINAL STATUS: PASS
```

Justification against the gate rules:

- **Required tests: all pass.** `gofmt -l`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...` and `go test -race -count=1 ./...` are green (Section R2.8). The unchanged Revision 1 suite passed against Revision 2 before any new test was written, and again afterwards alongside 13 new test functions.
- **Required performance targets: all pass** on the recorded reference environment. PERF-01 -0.97% against a +10% budget (both revisions re-measured back to back this session); PERF-02 43.35 ms against 5 s; PERF-03 545.34 ms against 10 s (Section R2.9). No target was loosened; the `-race` skip is the pre-existing build-tagged guard.
- **Findings: 0 Blocking, 0 High.** All three Revision 1 Medium findings are resolved and independently verified. Revision 2 adds 3 Low and 3 Informational findings, none of which affects observable UOW-1 behaviour or an acceptance criterion.
- **Ownership:** production code and verification tests were produced by different providers in separate sessions. This role modified no production file, no migration, no runtime configuration and no application documentation during this pass.

The gate is PASS for `a51211a`. F-13 through F-18 are returned to the production role as non-blocking follow-ups; F-16 (amending the approved outcome enum in `domain-entities.md`) is recommended before UOW-2 consumes the report contract.
