# Independent Review — UOW-2 SIC Mapping Management

## Gate: BLOCKED

Review date: 2026-09-06. Production has reproducible contract failures. Required tests do
not all pass, so Code Generation Part 3 cannot close. Production fixes belong to the
production provider; the failing tests remain enabled. No production file was modified.

## Identity and Revision

- Independent provider/model: OpenAI / GPT-6 (Codex), independent review/test role.
- Production provider: Claude, per handoff. This is the separate OpenAI provider session
  requested by the user; no production-provider reasoning was used as acceptance authority.
- Production revision: `ee24446e669c6892f73cd3448776c9f95a2d1e58`.
- Baseline: `de92e85644ef49810d01ca76af307e6b09946ba6`.
- Working HEAD: `55478c39514b92d9eac41924110a4e8bb543e6d2`.
- HEAD differs from the production revision only in AI-DLC documentation. Working tree
  was clean at review start. Reviewed production files remain unchanged from that revision.
- Ownership followed: test files and this review artifact only. State, audit, approved plans,
  production documentation, runtime source/templates and dependencies were not edited.

## Authority Consulted

- `PROJECT_GUIDELINES.md` and `aidlc-docs/aidlc-state.md`.
- `.codex/skills/independent-test-reviewer/SKILL.md` and Code Generation Part 3 rules.
- Inception requirements, assigned stories US-04/05/10/11, and unit/story-map context.
- UOW-2 functional design: business logic, business rules, domain entities, frontend components.
- Approved UOW-2 NFR requirements/technology decisions and both simplified NFR Design artifacts.
- `aidlc-docs/construction/plans/sic-mapping-management-code-generation-plan.md`.
- Production handoff, production diff, and existing UOW-1 independent tests/findings.

The simplified NFR Design is authoritative: no additional JSON settings, general cancellation
framework, background worker, or broad filesystem abstraction is requested by this review.

## Findings Requiring Production Attention

### U2-F01 — Medium: extra uploaded files under another field are accepted

Location: `internal/handler/sic_mapping_handler.go:155`.

The handler checks only `MultipartForm.File["file"]`. A request containing one valid `file`
part and a second file named `other` returns 200, commits a mapping and creates a backup.
The approved contract requires rejection of extra files before service processing.

- Reproduction: `TestReviewU2HandlerUploadBoundaries/extra_other_field`.
- Expected: 400, zero persisted rows, no backup.
- Observed: 200, one persisted row, one backup.
- Trace: Code Generation Step 8; NFRP-U2-04; NFR-U2-SEC-02/TEST-01; US-11.
- Fix: count file parts across all form file fields and require exactly one in `file`.
  Ordinary non-file form values can remain ignored as the simplified design permits.
- Status: OPEN; required test fails.

### U2-F02 — Medium: database export failure is reported as a successful empty download

Location: `internal/handler/sic_mapping_handler.go:114–121`.

When the repository fails before CSV output begins, Download still returns HTTP 200 with an
empty CSV attachment. The comment that setting headers has already committed them is incorrect:
no response bytes have been written in this failure path. A user can mistake a failed export
for a successful backup. A valid empty export must contain the five-column header.

- Reproduction: `TestReviewU2HandlerDownloadFailure` drops the mapping table in an isolated DB.
- Expected: 500 with an operational error, not a successful attachment.
- Observed: 200 and an empty body.
- Trace: US-10, BR-U2-11/38, NFR-U2-REL-01/UX-01, NFRP-U2-07.
- Fix: handle repository/pre-write failure before committing CSV output; only treat actual
  write failures after response bytes begin as non-recoverable streaming failures.
- Status: OPEN; required test fails.

### U2-F03 — Medium: missing pre-persistence cancellation checks

Locations: `internal/service/sic_mapping_service.go:481–493`, `:515–538`, `:706–737`.

Cancellation is checked during admission, but not at the approved service phase boundaries.
Create/update can wait on a database connection, have their context cancelled, and then save
successfully once that connection becomes available. A merge cancelled during CSV validation
still reads the snapshot and writes a backup before its context-aware transaction rejects it.

- Reproduction: `TestReviewU2CRUDCancellationBeforePersistence/create` and `/update` hold the
  only database connection, wait for service admission, cancel, and release the connection.
  Both report success and change persistent state.
- Reproduction: `TestReviewU2CancelledValidationDoesNotBackup` cancels from the validation
  reader; a backup is written, although the eventual merge correctly persists nothing.
- Trace: simplified NFRP-U2-02; NFR-U2-CON-01/REL-01; Code Generation admission/cancellation scope.
- Fix: add the specified cancellation checks before backup/write after blocking validation
  or reads, and return the stable cancellation outcome. This does not require a general
  context refactor of existing UOW-1 APIs. Preserve committed state once a write succeeds.
- Status: OPEN; required tests fail. Cancelled admission itself works and is independently tested.

### U2-F04 — Medium: nil collaborator silently becomes successful no-op work

Location: `internal/service/sic_mapping_service.go:120–122`.

The management constructor silently replaces nil with a no-op despite the explicit approved
requirement that nil must not masquerade as successful work. Main currently wires the no-op
explicitly and correctly, but an integration wiring mistake at the UOW-3 boundary would be hidden.

- Reproduction: `TestReviewU2NilCollaboratorIsNotSilentSuccess` supplies nil, then creates a mapping;
  the mutation reports committed success without a warning or rejection.
- Trace: Code Generation Step 4; NFR-U2-MAINT-01; logical-components service boundary.
- Fix: reject missing management collaborator wiring. Keep the compatibility startup constructor's
  deliberate explicit no-op and main.go's explicit checkpoint wiring.
- Status: OPEN; required test fails.

### U2-F05 — Medium: network errors invite blind mutation retries

Locations: `cmd/privateledger/web/templates/sic_mappings.html:257–258`, `:289–290`, `:390–391`.

The three fetch catch paths say the operation failed/was not saved or deleted and invite
"Please try again." A response can be lost after a successful commit. The UI then claims a
failure it cannot know and encourages a second mutation, contrary to the approved uncertainty
and saved-state behavior. This can create duplicate-create conflicts, misleading delete errors,
extra backups, or overwrite intervening edits on re-upload.

- Evidence: direct inspection of the catch paths (not a browser execution claim).
- Trace: NFRP-U2-02/07, NFR-U2-UX-01/REL-01.
- Fix: report an unknown outcome on transport failure and direct the user to refresh/check saved
  mappings before deciding whether to retry. Keep definite rollback wording for definite server
  failures only.
- Status: OPEN; browser-level failure simulation remains to be verified after correction.

### U2-F06 — Medium: automatic reload removes warnings and backup information

Locations: `cmd/privateledger/web/templates/sic_mappings.html:253`, `:285`, `:389`.

Success/warning results are followed by an unconditional reload after 2.5 seconds. The freshly
rendered page has an empty status region, so backup paths and post-commit warnings disappear.
Users reading a longer warning, or using assistive technology, lose the required information.
The upload error branch also ignores `body.result.backup_path` when backup succeeded but merge failed.

- Evidence: direct inspection of timer branches, empty initial status region and failed-upload rendering.
- Trace: US-11 backup reporting; BR-U2-24/25/45; NFR-U2-UX-01.
- Fix: keep results available until dismissal (or persist feedback across refresh), and surface
  either backup path or warning on a later merge failure. No new UI framework is needed.
- Status: OPEN; interactive verification pending.

### U2-F07 — Low: repository classification still matches driver message strings

Location: `internal/repository/sic_mapping_repo.go:226–233`.

Stable sentinels are returned, but classification relies on English substrings for unique/busy
errors. The approved design explicitly avoids arbitrary driver-message matching; moving the
match into a repository does not satisfy that decision. Also, MergeAll's BeginTx/prepare/commit
errors bypass this classifier, so error mapping is not consistently applied at those boundaries.

- Trace: NFRP-U2-07; Code Generation Steps 3/8.
- Fix: use the existing driver's typed error/result codes and consistently classify applicable
  repository failures. The real driver duplicate case already has a passing sentinel assertion.
- Status: OPEN; no driver upgrade or new dependency requested.

### U2-F08 — Low: saved outcomes on disconnected requests are not logged

Locations: `internal/service/sic_mapping_service.go:742–749`,
`internal/handler/sic_mapping_handler.go:182–191` (equivalent CRUD response paths).

The plan requires logging the saved outcome when it can no longer be delivered. The new service
and handler contain no slog calls or response-delivery/context checks for this case. Generic
request logging does not record mapping commit/backup outcome or post-commit warnings.

- Trace: Code Generation Step 7; NFRP-U2-02; NFR-U2-SEC-01/UX-01.
- Fix: record safe operation/outcome/count fields through slog on the relevant path, without
  uploading/logging file contents. Do not add a new logging subsystem.
- Status: OPEN; source inspection.

No Blocking or High defect was established in this pass. The gate is nevertheless BLOCKED
because required tests fail and required verification gaps remain. Severity does not waive tests.

## Acceptance-Criteria Coverage

| Story / acceptance focus | Evidence and result |
|---|---|
| US-04 view mappings and intentional empty category | 1,000-row embedded page render, numeric order, null-state label: PASS; live browser usability pending |
| US-04 create/edit categorized and empty mappings | TestReviewU2CRUDAndAffectedCodes; handler CRUD matrix: PASS on normal inputs |
| US-04 edit code and global uniqueness | Canonical duplicate rejection, identity preservation, code-change handoff: PASS |
| US-04 deletion | Delete + repeated not-found + no affected-code call: PASS; database operation does not update transaction categories by inspection |
| US-04 current category choices after startup | Category inserted after service construction appears in rendered page: PASS |
| US-05 whitespace, empty, digit/range validation | CRUD test and existing normalization/seed examples and properties: PASS |
| US-05 globally unique code and nullable category | Stable duplicate assertion, CRUD tests, real foreign-key rollback: PASS |
| US-10 fixed filename, schema, nullable categories, escaping | Handler header-only export and populated CSV export/re-upload test: PASS |
| US-10 valid empty download | Header-only happy path PASS; infrastructure failure response FAIL (U2-F02) |
| US-11 browser confirmation before one request | Source inspection confirms confirmation before fetch; interactive confirmation not executed |
| US-11 insert/update/omission/counts | Real service merge and generated properties: PASS |
| US-11 header-only no-op | Counts zero, rows preserved, backup attempt: PASS |
| US-11 invalid file and duplicate normalized codes | Bounded 80-row/multi-error tests plus existing seed/category/duplicate examples: PASS |
| US-11 category name/ID resolution table | Existing TestValidateCSV_CategoryResolutionDecisionTable covers exact/folded/ambiguous/name-only/ID-only/conflicting/empty inputs: PASS |
| US-11 empty-category handoff exclusion | CRUD matrix and merge properties: PASS at checkpoint; real transaction categorization belongs to UOW-3 |
| US-11 backup success/failure feedback | API and service path/warning preservation: PASS; transient/missing UI feedback remains U2-F06 |
| NFR CON-01 | Busy wait, cancelled acquisition race (100 iterations), no late mutation, held collaborator and concurrent snapshot serialization: PASS; phase cancellation FAIL (U2-F03) |
| NFR REL-01 | Trigger-induced full rollback, FK rollback, identity/created_at preservation, committed warnings across CRUD/upload: PASS |
| NFR REL-02 | Actual CSV snapshot bytes, 0600, unique names, retained prior backups, creation failure, writer failure and cleanup helper: PASS; actual Close failure integration gap below |
| NFR REL-03 | F-15 direct validation, startup and upload/API database classification: PASS |
| NFR SEC-01 | Local-only runtime changes, parameterized writes, escaped template content: PASS by source/automated checks; safe outcome logging incomplete |
| NFR SEC-02 | Exact 10 MiB, one byte over, whole-body overflow, missing/lying Content-Length, temp cleanup: PASS; extra-file rejection FAIL |
| NFR SEC-03 | 50 retained diagnostics, truncation, 100,000 rejected rows, zero persisted rows and bounded log output: PASS |
| NFR UX-01 / SCALE-01 | Automated full render PASS; network/warning issues and unavailable browser prevent complete usability evidence |
| NFR MAINT-01 | Single binary/layers/explicit main wiring retained; no dependencies added; nil contract FAIL |
| NFR PERF-01 | Isolated reference-machine median 1.404 s against 10 s: PASS |
| NFR TEST-01–04 | Independent examples/properties authored; required functional failures and gaps prevent gate closure |

The update-code handoff is interpreted as creation of the newly mapped code, consistent with the
business-logic model. The old code's assignments are not cleared. No finding is raised solely for
that interpretation. Color/icon display fields are not required by the approved plan where unused.
CSV descriptions retain the shared UOW-1 parser behavior; CRUD trimming is tested. The domain CSV
wording about "agreed trimming" is ambiguous relative to that shared contract, so no new CSV trim
requirement has been invented in a test.

## Independently Authored Tests and Adjudicated Existing Tests

New test files:

- `internal/service/sic_management_review_test.go`: service behavior, concurrency, failures,
  cancellation and nil-wiring regressions; temporary files/DBs, controlled collaborator.
- `internal/service/sic_management_property_review_test.go`: generated merge/value/union/omission,
  count and idempotency invariants with rapid.
- `internal/service/sic_management_perf_review_test.go`: exact approved benchmark fixture and timing.
- `internal/repository/sic_merge_review_test.go`: real FK rollback, cancellation, no-op and numeric bounds.
- `internal/handler/sic_mapping_review_test.go`: HTTP contracts, multipart limits/cleanup, error results.
- `cmd/privateledger/sic_mapping_page_review_test.go`: real embedded-template render of 1,000 rows,
  escaped descriptions, current category choices and controls. Optional temporary HTML export for QA.

Existing tests changed with independent justification:

1. `internal/repository/sic_repo_test.go`: uniqueness assertion now uses
   errors.Is(err, model.ErrSICMappingDuplicate). Retains the duplicate rejection and one-row
   persistence assertions; the superseded driver-text expectation is removed.
2. `cmd/privateledger/startup_sic_diagnostic_volume_test.go`: both tests now require 50 retained
   errors and DiagnosticsTruncated while preserving 100,000 rejected rows, zero imported rows,
   invalid outcome and zero persisted mappings. Historical unbounded logging is reconstructed
   from the deterministic fixture's row count, not from the newly bounded report. Removed the
   invalid extrapolation that assumes retained diagnostics still grow with all rejected rows.

These corrections implement BR-U2-40 and stable error classification. They do not relax a valid
production invariant. Existing UOW-1 tests otherwise remain intact. Test helpers use real temporary
SQLite databases; no user's config.json, privateledger.db or financial exports were accessed.

## Commands and Results

| Command | Result |
|---|---|
| Initial `go test -count=1 ./...` before test edits | Three pre-existing superseded failures confirmed |
| `go test -count=1 ./...` after independent additions | FAIL in handler/service on new regressions; other packages and retained regression tests pass |
| Final `go test -short -count=1 ./...` | FAIL on five top-level test cases listed below; other packages pass |
| Final `go test -race -short -count=1 ./...` | Same contract failures; no DATA RACE reports |
| `go test -race ./internal/service -run '^TestReviewU2CancelledWaiterAcquisitionRace$' -count=1` | PASS; added admission race check, 100 iterations |
| `go test ./internal/service -run '^TestReviewU2MergeProperties$' -rapid.seed=20260906 -count=1 -v` | PASS, 100 generated cases with explicit replay seed |
| `go test ./internal/service -run '^TestReviewU2MergePerformance$' -count=1 -v` | PASS, one warm-up and five isolated measurements |
| `SIC_REVIEW_RENDER_PATH=/tmp/sic-u2-page.html go test ./cmd/privateledger -run 'TestReviewU2Page|TestStartupDiagnostic' -count=1 -v` | PASS; render and corrected large-invalid-seed tests |
| `go build ./...`, `go vet ./...` | PASS |
| `gofmt -l` on authored tests, `git diff --check` | Clean |

Final failing top-level tests (subtests identify the exact scenarios):

- TestReviewU2HandlerUploadBoundaries/extra_other_field.
- TestReviewU2HandlerDownloadFailure.
- TestReviewU2CancelledValidationDoesNotBackup.
- TestReviewU2CRUDCancellationBeforePersistence/create and /update.
- TestReviewU2NilCollaboratorIsNotSilentSuccess.

The uninstrumented full-suite run preceded the last nil/HTTP/race test additions; those additions
were then exercised in targeted and final short/race runs. Long tests were separately retained and
executed, including the corrected 100,000-row diagnostic tests and isolated benchmark. No failing
test was skipped to claim PASS. Race mode intentionally excludes performance timing.

Initial sandboxed build/cache and race setup failed. Approved reruns outside the sandbox succeeded
in building/running; these environment failures are not reported as production defects. Raw session
logs are temporary at `/tmp/sic-u2-final.log`, `/tmp/sic-u2-short-final.log`,
`/tmp/sic-u2-race-final.log`, `/tmp/sic-u2-targeted.log`, `/tmp/sic-u2-performance-isolated.log`
and `/tmp/sic-u2-page.log`; conclusions and key numbers are retained in this artifact.

## Performance Evidence

Reference environment independently checked: Apple M1, 8 logical CPUs, 16 GiB RAM, macOS 15.5
build 24F74, go1.26.0 darwin/arm64, APPLE SSD AP1024Q NVMe storage. This matches the recorded
UOW-1 environment. No other heavy test/build workload was intentionally run during the final
isolated benchmark; ordinary system background activity was not controlled.

Fixture: 100,000 existing mappings, 100 categories; 100,000 uploaded rows comprising 25,000 new,
25,000 changed and 50,000 unchanged, with 25,000 existing mappings omitted. CSV is 3,355,960 bytes.
Each run restores pre-state, excludes fixture/database setup, and includes service validation,
snapshot, complete successful local backup, atomic commit, reload and no-op handoff. Each run
verifies 125,000 final mappings and the exact returned counts.

| Run | Elapsed |
|---|---|
| Warm-up (excluded) | 1.400345667 s |
| 1 | 1.398523792 s |
| 2 | 1.403538291 s |
| 3 | 1.411360209 s |
| 4 | 1.431943375 s |
| 5 | 1.392014791 s |

Median **1.403538291 s**, target **10 s**: PASS. This is local acceptance evidence, not a
performance promise for arbitrary hardware or UOW-3 recategorization.

## PBT Compliance

- PBT-02/03: approved Q7 scope honored. Generated merge state/count/omission invariants and
  repeated-merge idempotency; CSV round trips and category resolution use example tests.
- PBT-07: domain generators use positive-int64 boundary codes, empty/overlapping/disjoint sets,
  null/non-null and changed categories, unchanged rows, comma/quote/newline and Unicode descriptions.
- PBT-08: rapid shrinking remains enabled; explicit seed 20260906 replay passes 100 cases.
  The new test defaults to seed 20260906 and logs it for ordinary CI runs while preserving
  an explicitly supplied rapid.seed for exploration/replay. The test restores the flag afterward;
  this package has no parallel tests. No failing property was found to shrink. It is included
  by the current CI go test command. No test-only dependency or CI workflow changed.
- PBT-09: existing rapid v1.1.0 retained. Other PBT rules advisory under Partial mode.

## Remaining Verification Gaps

1. Actual backup Close failure is not independently injected end-to-end. Production hardcodes
   os.CreateTemp returning *os.File. Checked close and cleanup branches exist by inspection;
   tests exercise failing CSV writers and real partial-file cleanup, not a failure of the exact
   writeBackup Close call. A minimal writer/closer seam, if needed, belongs to the production
   role under the simplified design. Do not count the existing helper test as complete close-fault evidence.
2. Browser runtime initialized, but getDefault reported "No browser is available" and documented
   discovery returned an empty list. Automated embedded render/escaping/order passed; visual
   layout, keyboard focus restoration, confirmation interaction and delayed-warning usability
   were not exercised in a real browser. No substitute browser or user session was manipulated.
3. No exhaustive injected prepare/commit/OS-cleanup-failure matrix was claimed. Real rollback/FK
   failures and the main error branches were verified; real long SQLite busy exhaustion was not
   forced. Admission timeout is exercised independently of SQLite configuration.
4. A dedicated boundary test for a service timer expiring simultaneously with gate release remains
   absent. Cancellation/acquisition races and an occupied gate's timeout are tested; source currently
   rechecks request cancellation, but not the elapsed service admission deadline after acquiring.
5. UOW-3 priority/manual-assignment integration remains out of scope. The checkpoint no-op cannot
   establish real recategorization correctness or its cancellation budget.

## Required Handoff and Resolution Status

All U2-F01 through U2-F08 are OPEN. F-13/F-14/F-15/F-16 from UOW-1 are verified corrected for the
implemented UOW-2 scope (bounded diagnostic accumulation and validity, shared size limit, database
classification, shared outcomes). F-04/F-05 remain deferred as approved; no unrelated repair requested.

Return this artifact and enabled regression tests to the production provider. Fix production without
editing these tests. Supply a new revision for independent re-review, rerun affected tests and required
gates, and close the listed verification gaps. Do not mark UOW-2 complete based on the passing
benchmark or absence of data races while behavioral tests fail.

**Final status: BLOCKED (gate FAIL).**
