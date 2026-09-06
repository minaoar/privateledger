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

## Gate Result

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
