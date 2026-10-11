# Independent Review and Test Handoff — UOW-2 SIC Mapping Management

## Provider Separation

Production code for UOW-2 was authored by **Claude**. This inverts the UOW-1 arrangement, in which
Claude performed the independent review. The UOW-2 independent review and test authorship must
therefore run in a **different provider's session**. This session cannot close its own gate.

Do not treat the production summary's reasoning as authoritative. Review against the approved
artifacts.

## Revision Under Review

- Branch: `support-mcc-for-category`
- Baseline (last commit before UOW-2 production work): `de92e85644ef49810d01ca76af307e6b09946ba6`
- **Production revision under review: `ee24446e669c6892f73cd3448776c9f95a2d1e58`** (`ee24446` — "feat: add SIC mapping management page,
  API, and merge upload")
- Production diff: `git diff de92e85..ee24446`

That commit contains production code only. The AI-DLC documentation for this unit is committed
separately so the production diff stays free of documentation noise.

## Approved Artifacts

Review against these, not against the implementation:

- `aidlc-docs/inception/requirements/requirements.md`
- `aidlc-docs/inception/user-stories/stories.md` (US-04, US-05, US-10, US-11)
- `aidlc-docs/construction/sic-mapping-management/functional-design/` (all four documents)
- `aidlc-docs/construction/sic-mapping-management/nfr-requirements/nfr-requirements.md`
- `aidlc-docs/construction/sic-mapping-management/nfr-design/` (both documents)
- `aidlc-docs/construction/plans/sic-mapping-management-code-generation-plan.md`
- `PROJECT_GUIDELINES.md`

## Production Scope

Modified: `internal/model/sic_mapping.go`, `internal/service/sic_mapping_service.go`,
`internal/repository/sic_mapping_repo.go`, `internal/handler/page_handler.go`,
`cmd/privateledger/main.go`, `cmd/privateledger/web/templates/layout.html`, `API_ROUTES.md`.

Created: `internal/handler/sic_mapping_handler.go`,
`cmd/privateledger/web/templates/sic_mappings.html`.

No test file, fixture, test-only dependency, or benchmark was created or modified by the production
role. `pgregory.net/rapid` is untouched.

## Commands Already Run

`gofmt -l ./cmd ./internal` (clean), `go build ./...` (pass), `go vet ./...` (pass),
`git diff --check` (clean), `go test -count=1 ./...` (three pre-existing failures, below).

Race detector, property tests, the PERF-01 benchmark, and the SCALE-01 page verification have **not**
been run and are required from this role.

## Three Pre-Existing Test Failures Requiring Your Adjudication

Production did not edit any test. Each of these asserts behavior the approved design replaces.
Confirm independently that each is genuinely superseded rather than a masked regression, then correct
the test and document why — or raise it as a production finding if you disagree.

1. `internal/repository/sic_repo_test.go:483` `TestSICMappingRepository_UniqueCodeRejected`
   asserts the error text contains `"UNIQUE"`. The repository now returns
   `model.ErrSICMappingDuplicate`. NFRP-U2-07 requires stable classification instead of error-string
   matching, and the raw driver text previously reached the API response body.

2. `cmd/privateledger/startup_sic_diagnostic_volume_test.go:168`
   `TestStartupDiagnosticVolumeForLargeInvalidSeed` asserts
   `len(report.Errors) == 100000` and expects bounded output to shrink an unbounded shape.

3. `cmd/privateledger/startup_sic_diagnostic_volume_test.go:239`
   `TestStartupDiagnosticRetentionForLargeInvalidSeed` asserts one retained diagnostic per rejected
   row.

Items 2 and 3 measure the F-13 defect that BR-U2-40 closes. Verify that authoritative counts survive:
a 100,000-row fully invalid seed should still report `Outcome=invalid`, `RejectedRows=100000`,
`ImportedRows=0`, and a mapping count of 0.

## Required Verification

Per the approved plan Step 14:

- **Admission**: timeout and acquisition races; release on every failure path; a cancelled waiter
  performing no late mutation; the gate held through the collaborator; no goroutine acquiring after its
  caller returned.
- **Merge**: atomicity and rollback; omission preservation; idempotency; created/updated/unchanged
  counts; header-only no-op; that `ReplaceAll` is never reached from a merge path.
- **Post-commit**: committed-with-warning on reload failure and on collaborator failure;
  `MappingCommitted` correctness; that no committed write is ever reported as a rollback.
- **Validation**: bounded diagnostics past 50 entries with `DiagnosticsTruncated` and authoritative
  `RejectedRows`; the F-15 classification across validation, startup, and upload; category
  foreign-key failure.
- **Upload boundary**: exact size bounds including exactly-at-limit; multipart cleanup; missing and
  extra file parts; that client `Content-Length` is not trusted.
- **Backup**: filename collision, write failure, close failure, partial cleanup, 0600 mode, retention.
- **Reads**: numeric ordering; CSV round-trip; that page and export do not take the mutation gate.
- **Properties** (`rapid`, approved Q7 scope): merge idempotency, omission preservation, count
  invariants, with shrinking and replay evidence.
- **Race detector**: now required — UOW-2 introduces concurrent state (NFR-U1-TEST-04).
- **PERF-01**: 100,000 pre-existing codes; upload of 100,000 split 25,000 new / 25,000 changed /
  50,000 unchanged, leaving 25,000 omitted; 100 categories; within 10 MiB; pre-state restored per run;
  successful backup and no-op handoff included; measured service-entry to result, excluding fixture
  setup; one warm-up, at least five runs, median at most ten seconds on the recorded reference
  environment.
- **SCALE-01**: unpaginated 1,000-mapping page as usability and functional evidence, no invented
  latency threshold.

## Areas Worth Particular Scrutiny

- The `sync.Once` release in `acquire` and every early return between acquisition and release.
- Driver-error string matching in `classifySICMappingError` — confined to the repository, but still
  string-based for both unique-violation and busy detection.
- `Download` writes headers before streaming, so a mid-stream export failure cannot change the
  status code.
- Whether the affected-code rules in `diffSICMappings` and `UpdateMapping` match BR-U2-29/30/31
  exactly, including the case where an update changes the SIC code itself.
- Whether `MergeUpload` returning a non-nil result alongside an error is handled correctly by the
  handler in every branch.

## Output

Record findings with file/line references, severity, acceptance-criteria traceability, and an explicit
final PASS/FAIL in:

`aidlc-docs/construction/sic-mapping-management/code-review/independent-review.md`

Production fixes return to the production role. Do not modify production files.


---

# Re-Review Request — Revision 2

Date: 2026-09-06. Revision 1 (`ee24446`) was reviewed as **BLOCKED** with findings U2-F01 through
U2-F08, all OPEN. Production has addressed all eight.

Please re-review the new production revision, verify each finding independently rather than trusting
this summary, and update `independent-review.md` with a Revision 2 section and an explicit PASS/FAIL.

## What changed

| Finding | Where to look |
|---|---|
| U2-F01 extra files | `internal/handler/sic_mapping_handler.go` — file parts counted across all form fields |
| U2-F02 download failure | `internal/handler/sic_mapping_handler.go` — export buffered before headers commit |
| U2-F03 phase cancellation | `internal/service/sic_mapping_service.go` — `ensureActive` before each write, and in merge after validation and before backup |
| U2-F04 nil collaborator | `internal/service/sic_mapping_service.go` — `NewSICMappingManagementService` panics on nil |
| U2-F05 blind retry | `sic_mappings.html` — `setUnknownOutcome` on every transport failure |
| U2-F06 lost warnings | `sic_mappings.html` — timed reloads removed, explicit refresh control, backup path on failed merge |
| U2-F07 driver strings | `internal/repository/sic_mapping_repo.go` — typed `*sqlite.Error` codes; `MergeAll` boundaries classified |
| U2-F08 saved-outcome logging | `internal/service/sic_mapping_service.go` — `logSavedOutcome` |

## Production verification claimed

`gofmt`, `go build`, `go vet` clean. `go test -short -count=1 ./...`, the full `go test -count=1 ./...`
including long tests, and `go test -race -short -count=1 ./...` all pass with no data races. The five
previously failing top-level tests pass unmodified. No test file, fixture, test-only dependency, or
this review artifact was edited by production.

## Points deserving independent judgement

1. **U2-F04 resolution shape.** Production chose to panic at construction. Confirm that is acceptable
   for a wiring error, or require a use-time error instead.
2. **U2-F07 constant declaration.** The three SQLite result codes are declared locally rather than
   imported from the driver's platform-specific constants package, and `internal/repository` now
   imports `modernc.org/sqlite` directly. Confirm both choices.
3. **U2-F03 coverage.** Verify the cancellation checks sit at every boundary the design intends, not
   only the ones the existing tests exercise, and that a committed write is still never reported as
   cancelled.
4. **U2-F06 behaviour.** Clean CRUD success still reloads immediately. Confirm nothing is lost there
   and that the persistent-result path is reachable for every outcome carrying a backup path or warning.
5. **Verification gaps 1-5 from Revision 1 remain open.** Gap 1 in particular: the writer/closer seam
   for a real `writeBackup` Close fault was not added. Advise whether production should add it.


---

# Re-Review Request — Navigation Defect U2-USER-02

Date: 2026-09-06. **Production revision `8e1d981`.** Diff: `git diff HEAD~1..8e1d981` — two templates.

A post-gate defect reported by the user: the SIC mappings page was reachable from a top-level navigation
entry, but the approved decision is a secondary link from the Categories page.

`application-design-plan.md` Question 1 was answered `[Answer]: B.` — "Add it as a secondary link from
the existing Categories page" — with the top-level option and the both option explicitly not chosen.
`application-design.md` Key Design Decisions records "UI navigation: SIC mapping page is linked from the
existing Categories page, not top-level navigation."

## Fix

- `layout.html`: SIC Mappings navigation entry removed.
- `categories.html`: secondary link added beside the primary Add Category action, styled
  `btn-outline-secondary`, `data-testid="categories-sic-mappings-link"`.
- The `/sic-mappings` route is unchanged. No JavaScript added; the `app.js` global sweep is clean.

## Verification

`gofmt`, `go build ./...`, `go vet ./...`, `git diff --check` clean. `go test -count=1 ./...` passes all
seven packages. Smoke-tested on an isolated port: `/categories` 200, `/sic-mappings` 200, one link
occurrence on the Categories page, zero SIC occurrences inside the `<nav>` block, and the reverse link on
the SIC page still present.

## Worth Your Attention

This is the second UOW-2 defect found after the gate closed, after U2-F09. Both sit in the same blind
spot: UI behaviour and placement that no automated check covers.

Neither was reachable from the artifacts you were given. Your page tests assert the SIC page's own
contents; nothing asserted what `layout.html` contains or what `categories.html` links to, and the
design-decisions record — `application-design.md` and the answered `application-design-plan.md` — was
never in the handoff list. It is worth adding both to the artifacts you review against, and worth
considering a check that asserts navigation placement, since it is a decision no functional test encodes.
