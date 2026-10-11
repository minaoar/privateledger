# Independent Review — UOW-3 Transaction Categorization Integration

## Review Identity and Scope

- Independent review and test provider: OpenAI Codex
- Production provider: Claude
- Branch: `support-mcc-for-category`
- Baseline: `25046f5c40bcc539c65e34d34f85b1eb9f1a16c8`
- Production revision: `6833a8d8cb0effbf56bd855f60bc386389585371`
- Reviewed working-tree HEAD: `ac7cc54758fb8bda737ef0b37381d3b2110e6dd5`
- Production diff: `git diff 25046f5..6833a8d` (the ten production files named in the handoff)
- Review date: 2026-09-06

The review used `PROJECT_GUIDELINES.md`, `aidlc-docs/aidlc-state.md`, the approved requirements and
US-02/03/06/12/13, all UOW-3 functional-design and NFR artifacts, the code-generation plan, and the
independent-review handoff. No production file was changed by this role.

## Revision 1 Gate Result

**FAIL.** Four High production findings remain open, required example and property tests fail, and the
full race-mode suite exits non-zero because it includes those correctness failures. NFR-U3-TEST-05
requires all required tests to pass with no Blocking or High finding before Code Generation completes.

The untouched suite was run before any review test was authored and passed across all seven packages:

```text
go test -count=1 ./...
PASS (all seven packages; internal/service 58.568s)
```

After the independent tests were added, the consolidated suite fails only on the five assertions that
demonstrate findings U3-F01 through U3-F04. Other packages and existing tests pass.

## LoadPatterns Adjudication

**Production must unexport `LoadPatterns`.** The wrapper at `internal/service/categorizer.go:106-114`
delegates safely to `LoadRules`, so it does not expose a patterns-only refresh. It still does not meet
BR-U3-19's explicit approved wording, the business-logic model, or NFRP-U3-02, all of which say that
`LoadPatterns` becomes unexported and one exported reload entry point replaces it.

The compilation issue described in the handoff does not require a production compatibility API because
the affected callers are independently owned tests. This review migrated
`internal/service/import_regression_perf_test.go:87` and
`internal/service/sic_import_e2e_test.go:73,149` to `LoadRules`. U3-F06 records the remaining production
change.

## Findings

### U3-F01 — Scoped recategorization bypasses text-pattern priority

- Severity: **High**
- Production references: `internal/service/sic_categorizer.go:118-128`; compare the central priority
  implementation at `internal/service/categorizer.go:116-148`
- Acceptance trace: BR-U3-01, BR-U3-04, BR-U3-05, NFR-U3-REL-01, NFR-U3-TEST-01, US-02, US-03
- Evidence: `TestReviewU3ScopedRecategorizationUsesSharedPriority` at
  `internal/service/uow3_categorization_review_test.go:288` fails. A transaction matching both a text
  pattern and an affected SIC mapping receives the SIC category instead of the higher-priority text
  category.

`RecategorizeBySICCodes` calls `LookupCategory` directly and groups the resulting category. This is an
independent decision path despite the approved requirement that scoped recategorization use the same
decision function as import and full recategorization.

Acceptance condition: route every scoped candidate through the shared decision function and make the
failing test pass while retaining affected-code scoping.

### U3-F02 — Scoped recategorization can overwrite an existing category

- Severity: **High**
- Production references: `internal/repository/transaction_repo.go:364-370` and
  `internal/service/sic_categorizer.go:118-132`
- Acceptance trace: BR-U3-03, BR-U3-10, BR-U3-12, NFR-U3-REL-01, NFR-U3-TEST-01/02, US-02, US-03
- Evidence: `TestReviewU3ScopedRecategorizationPreservesExistingCategory` at
  `internal/service/uow3_categorization_review_test.go:318` fails. The repository selects
  `category_source = 0` without also requiring an empty `category_id`, and the scoped service bypasses
  `decide`, which is where existing categories are preserved.
- Generated evidence: `TestReviewU3ScopedRecategorizationProperty` at
  `internal/service/uow3_categorization_property_review_test.go:88` fails with recorded seed
  `20260906`. Rapid shrank the counterexample to one affected code (`300`) and one transaction with
  source `none` and an existing category. The service reports one changed row when zero rows are
  eligible. The replay fixture is retained under
  `internal/service/testdata/rapid/TestReviewU3ScopedRecategorizationProperty/`.

Acceptance condition: a transaction with any existing category remains byte-identical during scoped
automatic recategorization, regardless of its stored source, and both the example and generated property
pass.

### U3-F03 — A successful reload publishes mixed cache generations

- Severity: **High**
- Production references: `internal/service/categorizer.go:17-24,85-103,133-146` and
  `internal/service/sic_categorizer.go:27-28,58-73,82-90`
- Acceptance trace: NFRP-U3-02, NFR-U3-CON-01, NFR-U3-REL-01, NFR-U3-TEST-01/03, BR-U3-01,
  BR-U3-16 through BR-U3-19
- Evidence: `TestReviewU3RuleCachesPublishAtomically` at
  `internal/service/uow3_categorization_review_test.go:369` deterministically observes category `20`,
  the new SIC mapping combined with the old text-pattern set. Complete old and new generations would
  produce categories `30` and `1`, respectively.

`LoadRules` asks the lookup to reload and publish its mapping cache under the lookup's mutex, then later
publishes patterns under the categorizer's different mutex. This contradicts the required single guarded
cache set and all-or-nothing swap. The two-method `SICCategoryLookup` also exposes reload through an
interface approved as lookup-only.

The narrower failure case does pass: `TestReviewU3FailedMappingReloadKeepsPatternCache` confirms that a
mapping-load error leaves patterns unchanged. Race-focused concurrent lookup/reload also reports no Go
data race. Those results do not prevent a logically inconsistent cache snapshot during a successful
reload.

Acceptance condition: build both replacement sets before publication, publish them as one generation
under one synchronization boundary, let categorization read that same generation, and make the atomic
publication test pass.

### U3-F04 — Change Category plus mapping leaves the current row manual

- Severity: **High**
- Production references: `cmd/privateledger/web/templates/transactions.html:558-590,753-765` and
  `internal/handler/transaction_handler.go:264-305`
- Acceptance trace: BR-U3-06, BR-U3-31, NFR-U3-MAINT-01, NFR-U3-TEST-01, US-12
- Evidence: `TestReviewU3ChangeCategoryWithMappingUsesRuleSource` at
  `internal/handler/uow3_modal_review_test.go:144` fails. The final transaction JSON has the selected
  category but `category_source: 2` (`manual`) rather than `1` (`rule`).

The page first calls the generic category PATCH, which always marks a non-empty selection manual, and
then creates the mapping. Scoped recategorization cannot correct the current row after the PATCH because
it is no longer uncategorized.

Acceptance condition: after a successful opted-in Change Category plus SIC mapping flow, the current
transaction has the selected category with `category_source = rule`; ordinary Change Category remains
manual. The wider mapping effect and committed-with-warning behavior must remain intact.

### U3-F05 — Modal controls remain enabled while submissions are in flight

- Severity: **Medium**
- Production references: `cmd/privateledger/web/templates/transactions.html:223-264,279-345,732-766,820-877`
- Acceptance trace: NFR-U3-UX-01 (advisory) and Functional Design `frontend-components.md:63-70`
- Browser evidence: with delayed responses in the isolated review server, the Change Category form
  reported `selectDisabled:false`, `submitDisabled:false`, and `toggleDisabled:false`; the Create Pattern
  SIC path reported `categoryDisabled:false`, `ruleDisabled:false`, and `submitDisabled:false`.

The handlers await their requests without disabling their form controls. A second submit can therefore
start another mutation while the first is unresolved.

Acceptance condition: disable applicable submission controls immediately before the request and restore
them on every success, HTTP failure, and transport-failure path. Confirm both modal paths in an executing
browser.

### U3-F06 — LoadPatterns remains exported contrary to the approved reload API

- Severity: **Medium**
- Production reference: `internal/service/categorizer.go:106-114`
- Acceptance trace: BR-U3-19, NFRP-U3-02, NFR-U3-CON-01
- Evidence: the method remains exported. Its delegation mitigates stale one-cache refreshes but does not
  meet the approved public-surface contract. The independent test callers have already been migrated.

Acceptance condition: remove the exported method or make it unexported, leaving `LoadRules` as the sole
exported reload entry point.

## Verification Results

| Area | Evidence | Result |
|---|---|---|
| Untouched pre-review suite | `go test -count=1 ./...` before test changes | PASS |
| Priority matrix | Manual/existing preservation, pattern over SIC, SIC fallback, empty/missing mapping, and no-SIC examples; Rapid property | PASS |
| Full/per-category entry parity | `RecategorizeAll` and `RecategorizeByCategory`, including the approved higher-priority-pattern semantics | PASS |
| Scoped recategorization | Empty set and affected-code boundary pass; priority, existing-category preservation, and generated scoping fail | **FAIL** |
| Split counts | Processed 3, categorized 2, pattern 1, SIC 1; sum invariant holds | PASS |
| Cache failure behavior | Mapping reload failure retains old patterns | PASS |
| Cache atomic publication | Mixed old-pattern/new-mapping generation observed | **FAIL** |
| Set passing | 50,000 SIC strings; 32,765 numeric IDs; empty inputs issue no SQL; descending date order retained | PASS |
| UOW-2 merge integration | 50,000 affected codes through the real collaborator | PASS (2.43s) |
| Modal endpoint | Required category, no-SIC rejection, missing transaction, upsert, no text pattern, and wider recategorization | PASS |
| Change Category mapping source | Current transaction remains manual | **FAIL** |
| Rendered page contract | SIC display, description/detail fallback, no-SIC state, warning, split feedback, and global-name collision sweep | PASS |
| Executing browser interactions | Both modals, no-SIC display, fallback/empty description, rule switch, mapping request, and native confirmation invocation exercised | PASS with U3-F04/U3-F05 findings |
| Build/static checks | `go build ./...`, `go vet ./...`, `gofmt -l ./cmd ./internal`, `git diff --check` | PASS |

### SQLite query-plan evidence

`EXPLAIN QUERY PLAN` for the production `json_each` SIC query returned:

```text
SEARCH ledger_transaction USING INDEX idx_txn_sic (sic_code=?)
LIST SUBQUERY 1
SCAN json_each VIRTUAL TABLE INDEX 1:
USE TEMP B-TREE FOR ORDER BY
```

The required `idx_txn_sic` lookup is used. JSON SIC codes are encoded as strings matching the TEXT
column; bulk-update IDs are encoded as numbers matching the INTEGER column.

### Performance and scale evidence

Reference environment matches the approved target: Apple M1, 8 logical CPUs, 16 GiB RAM, macOS 15.5,
Go 1.26.0, APPLE SSD AP1024Q NVMe.

NFR-U3-PERF-01 fixture: 20,000 uncategorized transactions, 1,000 mappings, 100 categories, 100% carrying
SIC, database size 4,386,816 bytes. After warm-up, samples were 578.221ms, 597.412ms, 570.094ms,
530.349ms, and 549.599ms; median **570.094ms**, within the five-second gate. The scoped fixture used
100 affected codes and 2,000 matching transactions and completed in **110.902ms**.

NFR-U3-PERF-02 same-state paired fixture: SIC-free median **3.090724s** from five samples; SIC-bearing
median **3.188102s** from five samples; overhead **3.15%**, within the 10% budget. The unchanged UOW-1
SIC-free harness measured **3.059946s** at the candidate versus **3.117756s** at baseline `25046f5`, a
**1.85% improvement**.

NFR-U3-SCALE-01 observed heap growth for 100,000 cached mappings was **22,695,224 bytes** (3,281,896 to
25,977,120 bytes). This is informational because the approved NFR sets no ceiling.

### Race evidence

The focused concurrent categorization/reload race test passes under `-race` with no report. The full
command `go test -race -short -count=1 ./...` produced no `WARNING: DATA RACE`, but exits non-zero because
the required correctness tests for U3-F01 through U3-F04 fail. NFR-U3-TEST-03 therefore remains **FAIL**
at the gate level until production is corrected and the complete command passes.

## Browser-Driven Review Notes

Production templates and handlers were executed against an isolated temporary database. The Change
Category modal showed code `5812` with its primary description and hid SIC context for a transaction
without a code. Create Pattern showed detail fallback for `7011`, rendered bare code `7999` cleanly, and
correctly toggled the text input's disabled/required state when SIC was selected. The Categories action
invoked a native confirmation dialog, and the automated page assertion verifies its SIC warning text.

Accepting the native confirmation through the browser extension reset the extension connection, so the
browser run does not claim end-to-end inspection of the post-confirmation response. The server-rendered
contract and handler tests cover the warning/result content and split counts. This tooling limitation is
separate from the application findings above.

## Test Artifacts Owned by This Review

- `internal/service/uow3_categorization_review_test.go`
- `internal/service/uow3_categorization_property_review_test.go`
- `internal/service/uow3_performance_review_test.go`
- `internal/service/testdata/rapid/TestReviewU3ScopedRecategorizationProperty/`
- `internal/repository/uow3_set_passing_review_test.go`
- `internal/handler/uow3_modal_review_test.go`
- `cmd/privateledger/uow3_page_review_test.go`
- `cmd/privateledger/uow3_browser_fixture_test.go` (explicit `browserreview` build-tag fixture)
- Reviewer-owned call-site updates in `internal/service/import_regression_perf_test.go` and
  `internal/service/sic_import_e2e_test.go`

No test-only dependency was added; the repository's existing `pgregory.net/rapid v1.1.0` remains in use.

## Cross-Unit Assessment

No new UOW-4 candidate was found. U3-F01 through U3-F06 are regressions or deviations from already
approved UOW-3 behavior rather than contract amendments or missing future capabilities. Nothing from
this review should be admitted to `aidlc-docs/construction/uow-4-findings-register.md`.

## Production Handback and Re-review Conditions

Return U3-F01 through U3-F06 to the production provider. Re-review must run the complete ordinary and
race suites and confirm all new independent tests pass. The gate remains **FAIL** until the four High
findings are closed, required tests are green, and the approved reload API is restored.

---

## Revision 2 Re-Review

### Scope and Result

- Revision 2 production commit: `4bf5068` (`fix: address UOW-3 review findings U3-F01 through U3-F06`)
- Re-review working-tree HEAD before independent test additions: `da7ba64`
- Production patch inspected with `git show 4bf5068`: five production files, no test or fixture changes
- Provider separation remains intact: Claude authored production; OpenAI Codex authored and ran this re-review
- Re-review date: 2026-09-06

**REVISION 2: FAIL.** U3-F01, U3-F02, U3-F05, and U3-F06 are resolved. U3-F03 is only partially
resolved because `LoadRules` retains a fallback that exposes a partial cache generation. U3-F04's happy
path is resolved, but its new post-commit source write silently fails without warning. One High and one
Medium Revision 2 finding remain, and the independent tests demonstrating them fail.

### Revision 1 Finding Disposition

| Finding | Revision 2 result | Evidence |
|---|---|---|
| U3-F01 — scoped priority | **RESOLVED** | `RecategorizeBySICCodes` calls the attached shared decider; example and Rapid priority/scoping tests pass. |
| U3-F02 — existing-category overwrite | **RESOLVED** | Query requires both `category_source = 0` and `category_id IS NULL`; example and retained Rapid replay pass. |
| U3-F03 — atomic cache publication | **PARTIALLY RESOLVED / HIGH REMAINS** | The concrete staged path and original atomic-publication test pass. The non-staged fallback violates the same all-or-nothing contract; see U3-R2-F01. |
| U3-F04 — current row remains manual | **HAPPY PATH RESOLVED** | The original Change Category integration test passes and leaves the selected category rule-sourced. A post-commit write failure is hidden; see U3-R2-F02. |
| U3-F05 — controls enabled in flight | **RESOLVED** | Executing-browser checks cover both forms during requests and restoration after success and HTTP failure. |
| U3-F06 — exported `LoadPatterns` | **RESOLVED** | `LoadPatterns` is removed; `LoadRules` is the only exported categorizer reload entry point. |

The standalone `SICMappingCategorizer` compatibility behavior called out in the handoff is accepted.
`TestReviewU3StandaloneSICCategorizerFallback` proves that it assigns an eligible SIC-mapped row while
preserving manual and existing categories. The 50,000-code UOW-2 merge regression also passes through
the real collaborator.

### U3-R2-F01 — Fallback reload temporarily publishes patterns from a reload that fails

- Severity: **High**
- Production reference: `internal/service/categorizer.go:109-147`
- Acceptance trace: NFRP-U3-02, NFR-U3-CON-01, NFR-U3-REL-01, NFR-U3-TEST-01/03,
  BR-U3-16 through BR-U3-19
- Independent test: `TestReviewU3FallbackFailedReloadNeverPublishesPatterns` at
  `internal/service/uow3_categorization_review_test.go:483`

The concrete `sicMappingStager` path prepares mappings and publishes both sets while the categorizer
write lock is held. That path satisfies the atomic-observation contract. The fallback path instead
publishes replacement patterns at lines 134-137, releases the lock, and then calls `ReloadMappings`.
If mapping reload blocks and later fails, concurrent categorization can use replacement patterns that
the method subsequently rolls back.

The new test pauses a fallback mapping reload before its forced failure. During that pending reload,
`Categorize` assigns the replacement `NEW` pattern. This is observable partial publication from a reload
whose final result is failure, contrary to the approved rule that neither cache changes if either build
fails.

Acceptance condition: remove the non-atomic fallback or keep replacement patterns unpublished until the
mapping reload succeeds. While a reload is pending, categorization may see the complete old generation
or block; it must never see rules from a reload that later fails. The new independent test must pass.

### U3-R2-F02 — Rule-source write failure is logged but omitted from the committed response

- Severity: **Medium**
- Production reference: `internal/handler/transaction_handler.go:137-153`
- Acceptance trace: BR-U3-31, US-12, NFR-U3-UX-01, and the committed-with-warning failure model
- Independent test: `TestReviewU3RuleSourceWriteFailureIsReported` at
  `internal/handler/uow3_modal_review_test.go:167`

The happy path correctly changes the current row from manual to rule source when its stored category
matches the requested mapping category. If the follow-up `GetByID` or `UpdateCategory` fails after the
mapping commits, the handler only logs the failure and still returns the original successful mutation
result without a post-commit warning.

The new test injects a SQLite trigger that rejects only the rule-source update. The mapping remains
durable and the endpoint returns HTTP 200 with `mapping_committed:true`, but
`post_commit_warnings` is absent and the current row remains manual. The response therefore hides that
the requested BR-U3-31 outcome was not completed.

Acceptance condition: preserve the durable mapping result and report a post-commit warning when the
source read or write cannot complete. A caller must be able to distinguish full success from a committed
mapping whose current-transaction rule source was not applied.

### Revision 2 Verification

The complete ordinary suite was run before adding the two new failure-path tests and passed across all
seven packages (`internal/service` 129.799s). This confirms all Revision 1 tests, the retained Rapid
replay, the 50,000-code cross-unit fixture, and the performance fixtures were green against Revision 2.

After adding the new tests:

| Check | Revision 2 result |
|---|---|
| `go test -short -count=1 ./...` | **FAIL** only on U3-R2-F01 and U3-R2-F02 tests |
| `go test -race -short -count=1 ./...` | **FAIL** on the same two correctness tests; zero `WARNING: DATA RACE` reports |
| Focused `-race` over concurrent categorize/reload, original atomic publication, and standalone collaborator | PASS |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `gofmt` and `git diff --check` | PASS |
| `EXPLAIN QUERY PLAN` | PASS; still reports `SEARCH ledger_transaction USING INDEX idx_txn_sic (sic_code=?)` |

The full race command does not satisfy NFR-U3-TEST-03 because its exit status is non-zero, even though
the race detector itself reports no data race.

### Revision 2 Performance and Scale

The reference environment is unchanged. Measurements were run isolated from the race suite.

- NFR-U3-PERF-01 full recategorization: 20,000 transactions, 1,000 mappings, 100 categories, 100% SIC,
  4,386,816-byte database; samples 833.594ms, 818.704ms, 804.680ms, 909.958ms, 790.454ms; median
  **818.704ms** against the five-second limit — **PASS**.
- Scoped recategorization: 100 affected codes and 2,000 matching transactions in **166.686ms** — **PASS**.
- NFR-U3-PERF-02 SIC-free import median **4.713760s**; SIC-bearing median **4.873574s**; ratio
  **1.0339** or 3.39% overhead against the 10% limit — **PASS**.
- NFR-U3-SCALE-01: 100,000 mappings increased observed heap by **22,694,984 bytes**
  (3,333,040 to 26,028,024 bytes); informational — **PASS**.

### Revision 2 Browser Evidence

The production templates and handlers ran against the reviewer-owned isolated fixture on
`127.0.0.1:18843`; no user database or application process was used.

- Change Category: select, mapping toggle, and submit button were all disabled during the delayed
  request and all restored after success.
- Change Category with a forced mapping HTTP failure: all three controls were disabled in flight and
  restored afterwards.
- Create Pattern SIC path: both rule radios, category select, and submit button were disabled in flight.
  With a forced HTTP failure, controls were restored and the modal stayed open.
- SIC/text switching still disables the inapplicable pattern field, and existing display/fallback tests
  remain green.

This closes U3-F05's required executing-browser acceptance condition.

### Rapid Replay Decision

Keep the retained `.fail` file under
`internal/service/testdata/rapid/TestReviewU3ScopedRecategorizationProperty/`. It is a regression corpus
entry carrying the shrunk Revision 1 counterexample and recorded seed `20260906`; Rapid replays it green
on Revision 2. Keeping a formerly failing minimized case is useful evidence and does not mark the current
test as failed.

### Revision 2 Ownership and Cross-Unit Assessment

This re-review modified only independent test files, the browser fixture, and this review artifact.
Production files remain untouched. No new UOW-4 candidate was found: both Revision 2 findings concern
already-approved UOW-3 cache and modal-result behavior.

Return U3-R2-F01 and U3-R2-F02 to the production provider. The UOW-3 independent gate remains
**REVISION 2 FAIL** until both tests pass and the complete ordinary and race suites return success.

---

## Revision 3 Cache-Test Adjudication

- Revision 3 production commit: `693ffa6`
- Production patch: two production files; no test or fixture changes
- Adjudication date: 2026-09-06
- Current decision: **FAIL — U3-R2-F01 remains open**

U3-R2-F02 is **RESOLVED**. Both
`TestReviewU3ChangeCategoryWithMappingUsesRuleSource` and
`TestReviewU3RuleSourceWriteFailureIsReported` pass. A source read or write failure after the mapping
commit is now appended to `post_commit_warnings`, preserving the durable result while disclosing the
incomplete follow-up.

The production provider correctly identified a limitation in the original orchestration of
`TestReviewU3RuleCachesPublishAtomically`: it called `Categorize` on the same goroutine that later
released the blocked fake reload, so the test could not accommodate a correct implementation that
blocks readers during fallback reload. The independent role corrected the test at
`internal/service/uow3_categorization_review_test.go:430` by running `Categorize` in a goroutine and
accepting a blocked decision until reload completes.

The two cache tests do not impose contradictory requirements after that correction:

- A successful fallback reload may expose the complete old generation, block until completion, or
  expose the complete new generation. It must not expose a mixed generation.
- A fallback reload that later fails may expose the complete old generation or block. It must never
  expose replacement patterns from the failed reload.

Revision 3 still fails the corrected atomic-publication test:

```text
TestReviewU3RuleCachesPublishAtomically:
observed mixed cache generation category 20; valid old/new results are 30 or 1
```

The fake's early mapping publication is valid for the interface being tested. A non-staging
`ReloadMappings` implementation may mutate its internal state before returning; the caller has no
contract permitting it to assume publication occurs only at method return. Moving the fake's assignment
after its blocking point would hide this valid interleaving rather than fix production behavior.

At `internal/service/categorizer.go:133-143`, Revision 3 calls fallback `ReloadMappings` without holding
the categorizer write lock and publishes patterns only after it returns. While the call is pending,
categorization can therefore observe the old pattern set with already-published new mappings. That is
the mixed category `20` reproduced by the corrected test.

The simplest accepted resolution is to remove the non-staging fallback and require staged publication,
because the shipped `SICMappingCategorizer` already implements staging. If compatibility is retained,
the fallback must exclude categorization readers across mapping reload and pattern publication, for
example by holding the categorizer write lock across that sequence. The corrected independent test now
permits that blocking behavior.

Making `stagedUOW3Lookup` implement the staging interface is sufficient only if the production fallback
is removed. Otherwise it would test the shipped staged path while leaving production fallback behavior
unverified.

Focused Revision 3 results:

| Check | Result |
|---|---|
| U3-R2-F02 happy path and injected write failure | PASS |
| Failed fallback reload never publishes replacement patterns | PASS |
| Failed mapping reload retains prior pattern cache | PASS |
| Concurrent categorize/reload | PASS |
| Corrected successful fallback atomic-publication test | **FAIL — mixed generation `20`** |

The independent gate remains **REVISION 3 FAIL** until U3-R2-F01 is resolved and the complete ordinary
and race suites pass.

---

## Revision 4 Re-Review

### Scope and Result

- Revision 4 production commit: `bb44d40` (`fix: exclude readers across the non-staging fallback reload`)
- Production patch inspected with `git show bb44d40`: one production file,
  `internal/service/categorizer.go`; no test or fixture changes
- Re-review working-tree HEAD: `49346d4`; the tree was clean before verification
- Provider separation remains intact: Claude authored production; OpenAI Codex authored and ran this
  re-review
- Re-review date: 2026-09-06

**REVISION 4: PASS.** U3-R2-F01 and U3-R2-F02 are resolved. All required tests pass, including the
corrected successful-fallback atomic-publication test, the failed-fallback test, the complete ordinary
suite, and the complete short race suite. No Blocking, High, or Medium finding remains. One Low
documentation finding is recorded below and does not affect runtime behavior or the independent gate.

### Revision 2 Finding Disposition

| Finding | Revision 4 result | Evidence |
|---|---|---|
| U3-R2-F01 — fallback cache publication | **RESOLVED** | `internal/service/categorizer.go:134-150` holds the categorizer write lock across non-staging `ReloadMappings` and pattern publication. Categorization cannot observe the interval. All four cache acceptance tests pass. |
| U3-R2-F02 — hidden post-commit source failure | **RESOLVED in Revision 3; reconfirmed** | Both `TestReviewU3ChangeCategoryWithMappingUsesRuleSource` and `TestReviewU3RuleSourceWriteFailureIsReported` pass. |

Revision 4 implements the compatibility option accepted in the Revision 3 adjudication. Pattern data
is prepared before entering the fallback. The categorizer then acquires its write lock, completes the
mapping reload, and publishes the replacement patterns before releasing the lock. A concurrent
categorization decision either completes against the old generation before the lock is acquired or
waits and resumes against the new generation. If mapping reload returns an error, replacement patterns
are not published.

The shipped `*SICMappingCategorizer` path remains staged and was not changed by Revision 4. Its mapping
index and the replacement pattern set continue to be built before their joint publication under the
categorizer write lock.

### U3-R4-F01 — `LoadRules` comment describes the superseded fallback algorithm

- Severity: **Low**
- Production reference: `internal/service/categorizer.go:109-114`
- Acceptance trace: NFR-U3-MAINT-01
- Gate effect: **Non-blocking**

The leading `LoadRules` comment says a non-staging source publishes patterns first and restores them if
mapping reload fails. Revision 4 now does the opposite: it holds the write lock, reloads mappings, and
publishes patterns only after success. The detailed compatibility-path comment at lines 134-142 matches
the implementation, but the function-level comment does not. Update lines 112-114 to describe the
current blocking fallback when production documentation is next touched.

### Revision 4 Verification

| Check | Result |
|---|---|
| Clean baseline `go test -count=1 ./...` | **PASS** across all seven packages; `internal/service` 130.094s |
| Four cache acceptance tests | **PASS**: successful atomic publication, failed fallback publication, failed mapping reload retention, and concurrent categorize/reload |
| U3-R2-F02 handler happy path and injected write failure | **PASS** |
| `go test -race -short -count=1 ./...` | **PASS** across all seven packages; zero race reports |
| Generated Rapid priority and scoping properties | **PASS**, 100 cases each; recorded replay seed `20260906` |
| UOW-2 50,000-affected-code regression | **PASS** |
| JSON set passing beyond the former variable limit and empty-set behavior | **PASS** |
| `EXPLAIN QUERY PLAN` | **PASS**; `SEARCH ledger_transaction USING INDEX idx_txn_sic (sic_code=?)` |
| `go build ./...` | **PASS** |
| `go vet ./...` | **PASS** |
| `gofmt -l` over the changed production file and reviewer-owned Go files | **PASS**, no output |
| `git diff --check` | **PASS** |

### Revision 4 Performance and Scale

The performance checks ran independently from the race suite on the same reference environment used by
the prior reviews.

- NFR-U3-PERF-01 full recategorization: 20,000 transactions, 1,000 mappings, 100 categories, 100% SIC;
  samples 808.938ms, 856.322ms, 930.112ms, 798.217ms, and 793.997ms; median **808.938ms** against the
  five-second limit — **PASS**.
- Scoped recategorization: 100 affected codes and 2,000 matching transactions in **180.080ms** —
  **PASS**.
- NFR-U3-PERF-02 SIC-free import median **4.738528s**; SIC-bearing median **4.914342s**; ratio
  **1.0371**, or 3.71% overhead against the 10% limit — **PASS**.
- NFR-U3-SCALE-01: 100,000 mappings increased observed heap by **22,695,096 bytes** (3,330,320 to
  26,025,416 bytes); informational — **PASS**.

One SIC-bearing import sample took 6.828s, but the approved gate uses the median of five samples. The
median remained stable and passed with substantial margin. The initial complete ordinary suite also
passed the same performance test before isolated measurement.

### Browser, Ownership, and Cross-Unit Assessment

Revision 4 changes only categorizer locking and does not touch a handler, route, template, JavaScript,
or browser fixture. The executing-browser evidence recorded in Revision 2 therefore remains applicable;
the ordinary suite also reconfirmed all page and modal source-level tests.

This re-review modified only this independent review artifact. Production files and production-owned
documentation were not changed. No new UOW-4 candidate was found: the resolved cache finding and the Low
comment mismatch concern existing UOW-3 behavior.

The UOW-3 independent review/test gate is **PASS at Revision 4 (`bb44d40`)**.
