# Independent Review and Test Handoff — UOW-3 Transaction Categorization Integration

## Provider Separation

UOW-3 production code was authored by **Claude**. The independent review and test authorship must run
in a **different provider's session**, as they did for UOW-2. This session cannot close its own gate.

Review against the approved artifacts. Do not treat the production summary's reasoning as authoritative.

## Revision Under Review

- Branch: `support-mcc-for-category`
- Baseline (last commit before UOW-3 production work): `25046f5c40bcc539c65e34d34f85b1eb9f1a16c8`
- **Production revision under review: `6833a8d8cb0effbf56bd855f60bc386389585371`** (`6833a8d` — "feat: apply SIC mappings during
  categorization")
- Production diff: `git diff 25046f5..6833a8d`

That commit contains production code only. The AI-DLC documentation for this unit is committed
separately, so the diff you review carries no documentation noise.

## Approved Artifacts

- `aidlc-docs/inception/requirements/requirements.md`, `user-stories/stories.md` (US-02, US-03, US-06, US-12, US-13)
- `aidlc-docs/construction/transaction-categorization-integration/functional-design/` (all four)
- `.../nfr-requirements/nfr-requirements.md` and `tech-stack-decisions.md`
- `.../nfr-design/nfr-design-patterns.md` and `logical-components.md`
- `aidlc-docs/construction/plans/transaction-categorization-integration-code-generation-plan.md`
- `PROJECT_GUIDELINES.md`

## Production Scope

**Modified**: `internal/repository/transaction_repo.go`, `internal/service/categorizer.go`,
`internal/service/sic_mapping_service.go`, `internal/handler/category_handler.go`,
`internal/handler/transaction_handler.go`, `cmd/privateledger/main.go`,
`cmd/privateledger/web/templates/transactions.html`, `cmd/privateledger/web/templates/categories.html`,
`API_ROUTES.md`.

**Created**: `internal/service/sic_categorizer.go`.

No test file, fixture, test-only dependency, or benchmark was created or modified by production.

## Commands Already Run

`gofmt`, `go build ./...`, `go vet ./...`, `git diff --check` all clean.
`go test -short -count=1 ./...` passes across all seven packages.

**Not run and required from this role**: race detector, generated properties, NFR-U3-PERF-01
(20,000-transaction "Recategorize All" within five seconds), NFR-U3-PERF-02 (both import fixtures
against the 10 % budget).

## Deviation Requiring Your Adjudication

**BR-U3-19 required `LoadPatterns` to become unexported. It was retained as a wrapper delegating to
`LoadRules`.**

Three of your test files call it — `import_regression_perf_test.go:87`,
`sic_import_e2e_test.go:73,149`. Unexporting broke *compilation*, blocking the whole suite, and
production may not edit those files.

Delegating satisfies the rule's purpose: BR-U3-19 exists so a caller cannot refresh one cache and leave
the other stale, and no exported way to refresh patterns alone now remains. The literal wording is not
met. Confirm this is acceptable, or direct production to unexport it and adjust the call sites yourself.

## Required Verification

- **Priority matrix**: manual preserved; existing category preserved; text pattern beats SIC; SIC applies
  only when no pattern matched; empty-category mapping assigns nothing; missing mapping assigns nothing;
  no-SIC transactions unaffected.
- **Entry-point parity**: `RecategorizeAll` and `RecategorizeByCategory` both route through `decide`,
  so SIC cannot apply on import and be skipped by an explicit recategorization.
- **Scoped recategorization**: touches only uncategorized transactions whose code is in the affected set;
  empty set does no work; returns the count actually assigned.
- **Counts**: `PatternCategorizedCount + SICCategorizedCount == CategorizedCount`; counts claimed only
  after commit.
- **Cache**: all-or-nothing swap when the mapping reload fails — neither cache changes; no exported way
  to refresh one alone; race detector over concurrent categorization and reload.
- **Set passing**: correctness at and beyond the former 32,764 ceiling for both builders; SIC codes
  encoded as JSON **strings** matching the TEXT column; IDs as JSON numbers; empty set issues no SQL;
  `ORDER BY date_posted DESC` still honoured.
- **Modal endpoint**: category required; no-SIC transaction rejected; existing mapping updated rather
  than rejected; no text pattern created on the SIC path.
- **Properties** (`rapid`, shrinking, recorded seed): priority matrix, and recategorization scoping —
  every transaction outside the affected set byte-identical afterwards.
- **Benchmarks**: NFR-U3-PERF-01 and NFR-U3-PERF-02 on the recorded reference environment.

## Areas Worth Particular Scrutiny

1. **The `json_each` query plan was never inspected.** Index use on `sic_code` is inferred from
   timing only. Please confirm with `EXPLAIN QUERY PLAN` rather than from timing.
2. **UOW-2's approved merge fixture produces 50,000 affected codes** and previously never issued the
   scoped query, because the collaborator was a no-op. It now does — that path is newly exercised.
3. **Steps 1 and 6 change behaviour shared with UOW-1 and UOW-2.** Their regressions must be re-run.
4. **No page JavaScript was executed.** Smoke testing covered HTTP endpoints and server-rendered HTML
   only. The template collision sweep against `app.js` globals is clean, but the modal interactions
   themselves are unexercised — this is where U2-F09 hid.
5. **`RecategorizeByCategory` semantics changed subtly**: it now assigns a transaction only if the full
   priority order resolves to that category, so a higher-priority pattern from another category wins.
   Confirm this matches intent.

## Output

Record findings with file/line references, severity, acceptance-criteria traceability, and an explicit
PASS/FAIL in
`aidlc-docs/construction/transaction-categorization-integration/code-review/independent-review.md`.

Production fixes return to the production role. Do not modify production files.


---

## Starting This Review

1. Confirm the working tree is clean and `HEAD` reaches production revision `6833a8d`.
2. Run the existing suite **first**, before writing anything, so any pre-existing failure is
   distinguished from one your new tests introduce. It passed at `6833a8d` across all seven packages.
3. Adjudicate the `LoadPatterns` deviation above before deep review — it may change what production owes.
4. Author tests under the required-verification list, run them, and record findings with file/line
   references, severity, acceptance-criteria traceability, and an explicit PASS/FAIL.

Production owns fixes; you own tests. If a production change is needed, hand it back rather than editing
production files.

## Cross-Unit Reminder

UOW-3 completion **freezes UOW-4's scope**. Two findings are already admitted in
`aidlc-docs/construction/uow-4-findings-register.md` (U4-01 category rename breaking exports and
backups, U4-02 CSV header rejecting cosmetic variation). Anything you find that is a *contract
amendment or missing capability* rather than a UOW-3 regression should be raised so it can be admitted
before that freeze — after it, a new unit would be required.


---

# Re-Review Request — Revision 2

Date: 2026-09-06. Revision 1 (`6833a8d`) was reviewed **FAIL** with findings U3-F01 through U3-F06.
Production has addressed all six.

**Revision 2 production revision: `4bf5068`.** Production diff: `git diff 6833a8d..4bf5068` — five files,
production only. Your test artifacts and the documentation are committed separately.

Please verify each independently rather than trusting this summary, and update `independent-review.md`
with a Revision 2 section and an explicit PASS/FAIL.

## What changed

| Finding | Where to look |
|---|---|
| U3-F01 | `internal/service/sic_categorizer.go` — `decideForTransaction` routes scoped candidates through the shared decision function |
| U3-F02 | `internal/repository/transaction_repo.go` — query now also requires `category_id IS NULL`; the shared guard applies on this path too |
| U3-F03 | `internal/service/categorizer.go` — `sicMappingStager` prepare/commit; `LoadRules` publishes both sets under one write lock; `decide` holds its read lock across both rule sources |
| U3-F04 | `internal/handler/transaction_handler.go` — rule source restated, only when the stored category already matches the mapping |
| U3-F05 | `cmd/privateledger/web/templates/transactions.html` — `setTxnModalBusy` disables and restores both modal forms |
| U3-F06 | `internal/service/categorizer.go` — `LoadPatterns` removed; your adjudication accepted in full |

## Production verification claimed

`gofmt`, `go build`, `go vet`, `git diff --check` clean. `go test -count=1 ./...` passes across all
seven packages; `go test -race -short -count=1 ./...` passes with zero data races. No test file,
fixture, or this artifact was edited by production.

## Points deserving independent judgement

1. **The U3-F03 fallback path.** With a source implementing `prepareMappings`/`commitMappings` — the
   real one — publication is genuinely atomic. A source offering only `ReloadMappings`, such as your
   staged fake, takes a fallback that publishes patterns first and restores them if the mapping reload
   then fails. Confirm the fallback is acceptable, or require the interface be narrowed so no fallback
   is needed.
2. **A regression I introduced and caught.** My first U3-F01 attempt made a missing decider a hard
   error, which broke `TestReviewU3UOW2MergeFiftyThousandAffectedCodes` — a test that passed for you.
   A standalone mapping categorizer now falls back to a pattern-free decision that still preserves
   manual sources and existing categories. Confirm that fallback's semantics.
3. **U3-F04's narrowing.** The rule source is restated only when the stored category already equals the
   mapping's category, so a category the user chose is never changed. Confirm this satisfies BR-U3-31
   without weakening FR7.
4. **U3-F05 needs browser confirmation.** Its acceptance condition requires confirming both modal paths
   in an executing browser. Production could not do that; the change is source-level only.
5. **`TestReviewU3ImportWithMappingsPerformance` failed once on this machine** during a loaded
   full-suite run, and passes isolated at ratio 1.0250. If you see it fail, please check machine load
   before recording it as a regression.

6. **Your retained rapid replay fixture now replays green.**
   `internal/service/testdata/rapid/TestReviewU3ScopedRecategorizationProperty/…​.fail` still exists and
   `TestReviewU3ScopedRecategorizationProperty` passes with it present. Production committed it rather
   than deleting it, since it is your artifact and you recorded it as deliberately retained. Decide
   whether to keep it as evidence of the original counterexample or remove it so a future reader does
   not mistake it for a live failure.


---

# Re-Review Request — Revision 3

Date: 2026-09-06. Revision 2 (`4bf5068`) was reviewed **FAIL** with U3-R2-F01 (High) and U3-R2-F02
(Medium).

**Revision 3 production revision: `693ffa6`.** Production diff: `git diff 4bf5068..693ffa6` — two files,
production only.

**U3-R2-F02 is resolved.** A failed rule-source read or write after a committed mapping now appends a
post-commit warning to the committed result instead of only logging. Both the failure-injection test and
the original happy-path test pass.

**U3-R2-F01's acceptance condition is implemented, and it breaks `TestReviewU3RuleCachesPublishAtomically`.**

Replacement patterns are now published only after the mapping reload succeeds.
`TestReviewU3FallbackFailedReloadNeverPublishesPatterns` passes. The atomic-publication test then fails
with `observed mixed cache generation category 20`.

## Please adjudicate this conflict

The two tests impose opposite requirements on the non-staging fallback, and production cannot know in
advance which outcome a blocking `ReloadMappings` will produce:

- `...PublishAtomically` (reload succeeds) accepts only complete-old (`30`) or complete-new. Complete-old
  is unreachable for a non-stager, because `stagedUOW3Lookup.ReloadMappings` sets `current = next` before
  blocking and the categorizer holds no snapshot of the mapping side. So it requires patterns published
  during the window.
- `...NeverPublishesPatterns` (reload fails) requires patterns not published during the window.

Blocking readers satisfies both and your newer test explicitly allows it, but `...PublishAtomically`
calls `Categorize` on the main goroutine after `<-lookup.started`, so blocking deadlocks it.

**Production takes neither path.** `*SICMappingCategorizer` implements the stager interface and `main.go`
wires that type, so shipped code always builds both sets and publishes them together under one write
lock. Both failing scenarios exist only on the compatibility fallback for a lookup that cannot stage.

Three ways forward, your call:

1. Have `stagedUOW3Lookup` implement `prepareMappings`/`commitMappings`, so the test covers the path
   production uses.
2. Have the fake swap `current` on commit rather than at reload entry, making complete-old observable.
3. Run `Categorize` in a goroutine as your newer test does, permitting the blocking resolution.

I left the test failing rather than reverting, because reverting would restore behaviour you correctly
identified as wrong. If you would rather production carried the earlier ordering while the fallback is
redesigned, say so and I will change it.
