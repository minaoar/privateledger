# Independent Review — UOW-5 Rule-Sourced Recategorization

## Gate Result

**FAIL — BLOCKED after production Revision 3**

The current branch through `60d0c0c` resolves U5-R-F02, U5-R-F04, U5-R-F05, U5-R-F06, and
U5-R1-F01. The independent gate remains blocked by two High findings: resolving distinct SIC codes one
at a time is not an atomic mapping snapshot, and mapping save/delete warning results omit the three
FR16 counts. Browser execution also found one Medium display defect on the Categories page.

The main re-examination behavior, rule priority, state idempotence, non-triggers, batching, generated
order independence, and all three performance obligations pass. Production files were not modified.

## Reviewer and Scope

| Item | Value |
|---|---|
| Independent provider | OpenAI |
| Model | GPT-6 Codex |
| Role | Independent review and test author |
| Production provider | Anthropic / Claude, per the handoff and commit metadata |
| Branch | `support-mcc-for-category` |
| Base revision | `e9b972303e4921461c347850a0b84bfd0d300bdd` |
| Production revision | `3bd7903061805962efd06cc8f1c2ca915075be21` |
| Latest re-review revision | `60d0c0c` (latest production code `f5c7d33`; Revision 2 starts at `073f1b3`) |
| Runtime scope reviewed | `internal/service/categorizer.go`; `internal/service/sic_categorizer.go`; `internal/service/sic_mapping_service.go`; `internal/repository/transaction_repo.go`; `internal/handler/category_handler.go`; `cmd/privateledger/web/templates/categories.html`; `cmd/privateledger/web/templates/sic_mappings.html`; `cmd/privateledger/web/templates/transactions.html` |
| Ownership boundary | Reviewer changed test files and this review artifact only; production files were not modified |

The handoff says “six production files and two templates” but enumerates five Go files and two
templates. The commit's runtime scope matches the enumerated list. `go.mod`, `go.sum`, schema, and model
production files are unchanged.

## Artifacts Consulted

- `PROJECT_GUIDELINES.md`, `.claude/CLAUDE.md`,
  `.claude/agents/independent-test-reviewer.md`, and
  `.codex/skills/independent-test-reviewer/SKILL.md`
- `aidlc-docs/aidlc-state.md`, `.aidlc-rule-details/construction/code-generation.md`, and the property
  testing rule
- `aidlc-docs/construction/rule-sourced-recategorization/code/independent-review-handoff.md`
- All approved UOW-5 files under `functional-design/`, `nfr-requirements/`, and `nfr-design/`
- The approved UOW-5 code-generation plan, FR15, FR16, US-15, and the amended cross-unit artifacts named
  by the handoff
- Production commit `git show 3bd7903`, its parent diff, surrounding runtime code, and the existing
  UOW-2/UOW-3 independent tests and performance harnesses

## Baseline Adjudication

Before reviewer edits, `go test -count=1 ./...` failed to compile `internal/service` and
`internal/handler` because reviewer-owned fakes still implemented
`RecategorizeBySICCodes([]SICCode) (int, error)`. This is the exact approved Decision A break described
by the handoff: TD-U5-01 replaces that collaborator operation with
`Reexamine() (RecategorizationCounts, error)`. The reviewer tests were migrated to the approved
interface and approved FR15 behavior. No compatibility shim is required from production.

## Acceptance and NFR Traceability

| Obligation | Independent evidence | Result |
|---|---|---|
| NFR-U5-TEST-01 — order independence | `TestReviewU5RuleCreationOrderIndependenceProperty` uses Rapid to generate active rules and random creation permutations against real repositories and SQLite. Final categorization is identical. Seed `20260906`; shrinking enabled; no retained failure artifact. | PASS |
| NFR-U5-TEST-02 — idempotence | `TestReviewU5ReexaminationCountsPriorityManualAndIdempotence` proves a second pass performs no state writes. `TestReviewU5SecondPassReportsThreeZerosWithoutManualCandidates` proves the literal three zero counts in a fixture with no manual candidate. | PASS |
| NFR-U5-TEST-03 — triggers and non-triggers | Pattern creation/add/delete, mapping create/update/delete/upload, category deletion, description-only edit, and import were exercised. Empty-category mapping creation skips re-examination; see U5-R-F06. | FAIL |
| NFR-U5-TEST-04 — manual protection | Pattern and mapping triggers protect stable manual rows. Category deletion destroys the manual marker, and a concurrent manual selection can be overwritten; see U5-R-F02 and U5-R-F05. | FAIL |
| NFR-U5-TEST-05 — priority | Creating a pattern moves an existing mapping-assigned transaction to the pattern category. Pattern-over-SIC priority and fallback behavior pass. | PASS |
| NFR-U5-TEST-06 — counts | Core `Reexamine` moved/uncategorized/manual counts, zero values, and unchanged-row suppression pass. Mapping and pattern rule-change responses do not carry all three counts; see U5-R-F03 and U5-R-F04. | FAIL |
| NFR-U5-TEST-07 / PERF-01 | 20,000 transactions: median re-examination 532.897 ms against 1.5 s. No-op median 80.434 ms, measurably faster than the changing pass. | PASS |
| NFR-U5-TEST-07 / PERF-02 | Populated merge fixture: 100,000 existing mappings, 100,000 uploaded mappings, 20,000 transactions, all 20,000 moved on every measured run. Median 2.704 s against 10 s. | PASS |
| NFR-U5-TEST-07 / PERF-03 | Existing 20,000-row import fixtures: SIC-free median 3.092 s; SIC-bearing median 3.262 s; 5.50% overhead against the 10% budget. | PASS |
| NFR-U5-TEST-08 — race command | `go test -race -short -count=1 ./...` produced no Go race report, but the command exits nonzero on the deterministic UOW-5 correctness failures. A required gate command must exit successfully. | FAIL |
| BR-U5-10 / NFR-U5-CON-02 | A deterministic staged-reload test proves one pass can observe old rules for one transaction and new rules for another; see U5-R-F01. | FAIL |
| BR-U5-13 / SCALE-02 | `BulkClearCategory` succeeds with an empty set and 40,000 IDs, beyond SQLite's parameter ceiling. | PASS |
| BR-U5-09 | Category deletion re-examines after the database cascade; a transaction falls through the deleted pattern to a remaining SIC rule. | PASS |
| BR-U5-03/04 | Import does not revise an existing categorization, and a description-only mapping edit does not invoke re-examination. | PASS |

## Production Review

### Re-examination core

`Reexamine` materializes all transactions once and partitions writes by target category at
`internal/service/categorizer.go:289-367`. It preserves manual rows in the ordinary sequential path,
uses pattern-before-SIC priority, omits unchanged writes, and batches assignments and clears. The new
repository clear operation chunks large ID sets and passes the 40,000-ID boundary test.

The implementation does not pin the rule generation for the whole traversal. `LoadRules` completes at
`:290`, but every call to `evaluate` independently acquires and releases the read lock at `:212-232`.
The pass therefore has a gap between every transaction in which another reload can publish a new
generation. See U5-R-F01.

The materialized read and later bulk writes also form a time-of-check/time-of-use gap. The SQL updates
in `internal/repository/transaction_repo.go:478-482,512-516` constrain only transaction IDs, so a row
changed to manual after the read remains writable. See U5-R-F02.

### Trigger integration and reporting

Mapping create/update/delete/upload call the post-commit collaborator after durable writes, and
description-only edits correctly skip the traversal. `runPostCommit` receives all three collaborator
counts at `internal/service/sic_mapping_service.go:796-803`, but copies only `Moved` into the legacy
`recategorized_rows` field. The model result types at `internal/model/sic_mapping.go:340-376` have no
FR16 fields, and the mapping page only conditionally renders `recategorized_rows` at
`cmd/privateledger/web/templates/sic_mappings.html:347-363`. See U5-R-F03.

Category and pattern handlers call `Reexamine` after committing the rule mutation. However, category
creation with patterns, pattern addition, and pattern deletion discard the result and only log errors
at `internal/handler/category_handler.go:154-167,326-333,370-380`. Their page paths use generic success
toasts at `cmd/privateledger/web/templates/categories.html:415-516`. See U5-R-F04.

### Manual category deletion

Category deletion calls `ClearCategory` before deleting the category. `ClearCategory` explicitly reads
both rule and manual transactions and writes `category_source = 0` at
`internal/service/categorizer.go:391-411`. The subsequent UOW-5 pass can no longer identify the former
manual row and can assign it from another rule. See U5-R-F05.

## Dated-Amendment Adjudication

| Amendment | Judgment |
|---|---|
| `requirements.md` FR7 and FR14 | Honest reversal limited to rule-sourced persistence. The retained manual guarantee remains binding. |
| FR15 and FR16 | Clear additions that govern current-rule determinism and the three user-visible counts. Findings U5-R-F01, F03, and F04 apply these requirements directly. |
| US-03 and UOW-3 BR-U3-03 | Honest supersession for rule-sourced transactions; the reviewer changed old “existing category never revised” expectations accordingly. |
| UOW-2 BR-U2-29 through BR-U2-31 | Honest retirement of the affected-code scope. Production correctly performs whole-table re-examination. |
| UOW-3 NFR-U3-REL-01 | Honest narrowing for rule-sourced rows while explicitly retaining manual protection. U5-R-F02 and F05 are therefore defects, not authorized consequences. |
| UOW-5 BR-U5-02 | Necessary correction separating all-row read scope for the manual count from non-manual write scope. Production's ordinary traversal follows it. |
| UOW-2 NFR-U2-PERF-01 | Honest fixture correction. The reviewer populated 20,000 transactions and measured real re-examination. |

The NFR-U5-TEST-02 phrase “three zero counts” conflicts with BR-U5-14 if a protected manual transaction
continues to differ from current rules: the manual-protected count will correctly be repeated on every
pass even though no state changes. The review therefore proves state idempotence with a manual candidate
and proves literal three-zero reporting in a separate fixture without manual candidates. This is a test
interpretation, not a production finding.

## Findings

### U5-R-F01 — High — One re-examination can mix rule generations

**References:** `internal/service/categorizer.go:212-232,289-320`;
`internal/service/uow5_reexamination_review_test.go:151-202`; BR-U5-10;
NFR-U5-CON-02; FR15.

`LoadRules` publishes a generation before the transaction traversal, but the traversal does not retain
a generation-level read guard or immutable snapshot. Each transaction locks independently inside
`evaluate`. A deterministic staged lookup test publishes the next generation between two decisions and
observes a mixed result: one transaction follows the old category and the next follows the new category.
The test fails on 20 of 20 repetitions.

**Acceptance condition:** make one `Reexamine` decision traversal use one complete pattern-and-mapping
generation throughout. Preserve UOW-3's atomic cross-cache publication and avoid recursively acquiring
the same `RWMutex`. `TestReviewU5OnePassSeesOneRuleGeneration` must pass, including repeated execution.

**Status:** Open; blocks the gate.

### U5-R-F02 — High — A concurrent manual choice can be overwritten after the materialized read

**References:** `internal/service/categorizer.go:294-367`;
`internal/repository/transaction_repo.go:478-482,512-516`;
`internal/service/uow5_reexamination_review_test.go:204-238`; NFR-U5-REL-02;
NFR-U5-TEST-04.

Re-examination decides from the materialized row, then later updates by ID. If a user assigns a manual
category in that interval, both bulk update paths still overwrite the row because their predicates do
not check its current source. The deterministic barrier test reproduces this loss on 20 of 20 runs.

**Acceptance condition:** both assignment and clearing writes must exclude rows whose current
`category_source` is manual at write time. Returned moved/uncategorized counts must reflect rows actually
changed, such as by using `RowsAffected`. `TestReviewU5ConcurrentManualChoiceCannotBeOverwritten` and
the complete trigger matrix must pass.

**Status:** Open; blocks the gate.

### U5-R-F03 — High — Mapping rule-change results omit two FR16 counts and their zero values

**References:** `internal/service/sic_mapping_service.go:787-803,955-984`;
`internal/model/sic_mapping.go:336-376`;
`cmd/privateledger/web/templates/sic_mappings.html:347-363`;
`internal/service/uow5_mapping_contract_review_test.go:94-123`;
`cmd/privateledger/uow5_page_review_test.go:8-20`; FR16; BR-U5-15 through BR-U5-17;
NFR-U5-TEST-06.

The collaborator returns moved, uncategorized, and manual-protected counts, but mapping CRUD and merge
results expose only the legacy moved-equivalent `recategorized_rows`. The page cannot show the other two
counts and omits even the moved message when its value is zero. This contradicts the approved additive
result decision and FR16's requirement to report all three outcomes, including zeros.

**Acceptance condition:** extend both mapping mutation and import result contracts additively with
`moved_count`, `uncategorized_count`, and `manual_protected_count`; populate all three after successful
re-examination; retain post-commit warning semantics; and render all three, including zero values, on
the mapping page. The mapping contract and page tests must pass.

**Status:** Open; blocks the gate.

### U5-R-F04 — High — Pattern rule changes hide counts and post-commit failures

**References:** `internal/handler/category_handler.go:154-167,265-271,326-333,370-380`;
`cmd/privateledger/web/templates/categories.html:415-516`;
`internal/handler/uow5_rule_change_review_test.go:108-137,244-263`; FR16; BR-U5-11,
BR-U5-15, BR-U5-16; NFR-U5-REL-03.

Pattern-bearing category creation, pattern addition, and pattern deletion discard the successful
`Reexamine` result and return only their pre-existing payloads. A re-examination error after the pattern
commit is logged but omitted from the response, so the caller is told only that the operation succeeded
and cannot distinguish full success from a committed rule whose follow-up failed. The UI also does not
parse or display the three counts.

**Acceptance condition:** return a committed rule-change result containing all three counts and any
post-commit warning for every pattern mutation and category deletion, while keeping the rule mutation
committed. Update the page to display the outcome, including zeros. The response-count and injected
post-commit-failure tests must pass.

**Status:** Open; blocks the gate.

### U5-R-F05 — High — Category deletion destroys manual provenance before re-examination

**References:** `internal/service/categorizer.go:391-411`;
`internal/handler/category_handler.go:235-271`;
`internal/handler/uow5_rule_change_review_test.go:186-217`; FR7's retained manual clause;
NFR-U5-REL-02; NFR-U5-TEST-04.

The handoff raised this concern accurately. Deleting a category first rewrites every assignment in that
category, including manual ones, to source none. Re-examination then assigns a former manual row from a
remaining SIC rule and reports no manual protection because the marker has already been erased. UOW-5
makes the pre-existing provenance loss silently change the user's outcome, and the required all-trigger
manual-protection test fails.

**Acceptance condition:** obtain and record the product decision for a manual assignment whose category
is deleted, then implement it without silently converting that row into an ordinary rule candidate. If
the durable manual guarantee is retained, preserve a distinguishable manual state through the cascade
and report it consistently. If the user chooses different semantics, amend the approved artifacts
explicitly before changing this test.

**Status:** Open product decision; blocks the gate because a required test and retained manual invariant
currently fail.

### U5-R-F06 — Medium — Empty-category mapping creation skips the required trigger

**References:** `internal/service/sic_mapping_service.go:665-668,1001-1012`;
`internal/service/uow5_mapping_contract_review_test.go:15-29`; BR-U5-01;
NFR-U5-TEST-03.

Direct creation and merge creation invoke re-examination only when the new mapping already names a
category. BR-U5-01 says mapping creation triggers the pass, while BR-U5-04 names only a description-only
edit as the mapping exclusion. An empty-category creation is semantically inert, but that optimization
is not in the approved trigger contract.

**Acceptance condition:** invoke re-examination once for every mapping creation, including an empty
category, or explicitly amend the approved trigger artifacts to add this no-op exclusion. Cover direct
creation and merge creation.

**Status:** Open; required trigger coverage remains failing.

## Reviewer-Owned Test Changes

- Migrated existing UOW-2/UOW-3 reviewer fakes and assertions from the retired scoped collaborator to
  `Reexamine`, including FR15's revision of already rule-sourced transactions.
- Added `internal/service/uow5_reexamination_review_test.go` for core counts, priority, state
  idempotence, literal zero reporting, one-generation consistency, and concurrent manual protection.
- Added `internal/service/uow5_mapping_contract_review_test.go` for mapping trigger coverage, all-trigger
  manual protection, and additive result counts.
- Added `internal/service/uow5_nontrigger_review_test.go` for the description-only and import exclusions.
- Added `internal/service/uow5_order_property_review_test.go` for the required Rapid permutation
  property, using real repositories and SQLite.
- Added `internal/handler/uow5_rule_change_review_test.go` for pattern/category triggers, response
  counts, post-cascade evaluation, category-delete manual provenance, and post-commit failure reporting.
- Added `internal/repository/uow5_reexamination_review_test.go` for empty and 40,000-ID bulk clears.
- Added `cmd/privateledger/uow5_page_review_test.go` for the three user-visible count fields.
- Strengthened the existing UOW-2 merge performance fixture with 20,000 transactions and changed the
  UOW-5 re-examination performance harness to verify real changing and no-op passes.

## Commands and Results

| Command | Result |
|---|---|
| `go test -count=1 ./...` before reviewer edits | Expected compile FAIL in reviewer-owned service and handler fakes using the retired collaborator API; other packages passed. Adjudicated as approved Decision A. |
| `go test -count=1 ./...` after reviewer test changes | FAIL on the findings recorded above. All unaffected packages and tests pass; `internal/service` completed in 101.453 s. |
| UOW-5 core/property/non-trigger focused run | PASS in service and handler. |
| `go test -short -count=1 -run 'ReviewU5' ./...` | FAIL only on U5-R-F01 through F06 assertions; all other UOW-5 short tests pass. |
| Both deterministic concurrency tests with `-count=20` | FAIL on all repetitions: mixed generation and overwritten concurrent manual choice. |
| `go test -race -short -count=1 ./...` | FAIL on the same deterministic correctness assertions; no Go data-race warning emitted. |
| `go test ./internal/service -run '^TestReviewU5ReexaminationPerformance$' -count=1 -v` | PASS; changing samples 599.221, 583.687, 532.897, 521.746, 528.415 ms; median 532.897 ms. No-op samples 85.426, 80.434, 80.203, 79.629, 82.566 ms; median 80.434 ms. |
| `go test ./internal/service -run '^TestReviewU2MergePerformance$' -count=1 -v` | PASS; 20,000 transactions; measured 2.769, 2.667, 2.700, 2.704, 2.777 s; median 2.704 s against 10 s. |
| `go test ./internal/service -run '^TestReviewU3ImportWithMappingsPerformance$' -count=1 -v` | PASS; SIC-free median 3.092 s, SIC-bearing median 3.262 s, 5.50% overhead against 10%. |
| `go vet ./...` | PASS |
| `go build ./...` | PASS; Go emitted a non-fatal module-cache stat warning after successful compilation because the sandbox does not allow writing that external cache metadata path. |
| `git diff --check` | PASS |

## Coverage Limits

- Browser JavaScript was not executed in this revision because the API and template contract tests
  already establish that the required count data is absent. Browser-driven count and warning rendering
  must be exercised after those production paths exist.
- The current application exposes pattern creation and deletion paths; no separate pattern-update route
  exists. Tests cover the available create-with-category, add, and delete mutations.
- The race detector did not report a memory race. Its required command remains red because deterministic
  synchronization tests expose higher-level concurrency correctness failures.

## Revision 1 Re-review — `f95066c`

Revision 1 changes six production Go files and two templates. It also changes AI-DLC state/audit files
and adds `code/revision-1-summary.md`. Production did not modify reviewer tests. The reviewer corrected
four test call sites for the approved repository return-value change and independently tested the new
behavior.

### Finding disposition

| Finding | Revision 1 assessment | Status |
|---|---|---|
| U5-R-F01 — one generation per pass | The new traversal-level read lock fixes publication through `Categorizer.LoadRules`, and the original deterministic test passes. `SICMappingCategorizer.ReloadMappings` still publishes under its separate mutex and bypasses that lock. The shipped-path test observes a mixed pass on 20 of 20 runs. | **Open — High** |
| U5-R-F02 — concurrent manual overwrite | Both set-based writes exclude source 2 at SQL write time and return `RowsAffected`. The deterministic race test preserves the manual choice and reports zero changed rows. | **Resolved** |
| U5-R-F03 — mapping counts | CRUD and upload transport results now contain all three counts, including zeros. Upload displays them. Dedicated mapping create/update/delete and transaction-modal mapping flows do not consume them. | **Open — High** |
| U5-R-F04 — pattern counts and warnings | Category/pattern handlers now return all counts and post-commit warnings. The Categories page ignores those bodies and continues to show generic success messages. | **Open — High** |
| U5-R-F05 — category deletion/manual provenance | The rule no longer reassigns the former manual row. The chosen `category_id=NULL, category_source=2` state contradicts the approved one-representation invariant and causes repository APIs to disagree about whether the row is uncategorized. | **Open — High; product/artifact decision required** |
| U5-R-F06 — empty-category mapping creation | Direct creation and merge creation now invoke re-examination. | **Resolved** |

### U5-R-F01 remains open — the shipped mapping reload bypasses the generation lock

**References:** `internal/service/categorizer.go:304-359`;
`internal/service/sic_categorizer.go:95-122`;
`internal/service/sic_mapping_service.go:799-820`;
`internal/service/uow5_reexamination_review_test.go:234-285`; BR-U5-10;
NFR-U5-CON-02; TD-U5-03.

`Reexamine` now holds `Categorizer.mu.RLock` for its entire decision traversal. That protects pattern and
mapping publication performed through `Categorizer.LoadRules`. The mapping service, however, first
calls the collaborator's public `ReloadMappings`; the shipped `SICMappingCategorizer` publishes that
cache through its own `mu` without acquiring `Categorizer.mu`. A committed mapping change can therefore
replace mappings while an already-running pass remains under the old pattern generation.

The new test wraps the real `SICMappingCategorizer` only to pause after transaction one samples the old
cache. It then performs the exact shipped `ReloadMappings` operation before transaction two. The first
transaction takes the old mapping and the second takes the new one, deterministically on 20 of 20 runs.

**Acceptance condition:** route every mapping-cache publication that can overlap re-examination through
the same generation lock, or avoid the standalone publication on rule-changing post-commit paths and
let the ensuing `Reexamine` perform the atomic load. Both one-generation tests must pass repeatedly.

### U5-R-F03 and U5-R-F04 remain open — responses carry counts that the screens discard

**References:** `cmd/privateledger/web/templates/categories.html:416-516`;
`cmd/privateledger/web/templates/sic_mappings.html:250-329,360-377`;
`cmd/privateledger/web/templates/transactions.html:725-748`;
`cmd/privateledger/uow5_page_review_test.go:23-52`; FR16; US-16;
`functional-design/frontend-components.md:8-20`; code-plan Step 8.

The server result work is correct. The category create/add/delete callbacks never parse their successful
response bodies and show only fixed success text. Mapping create/update/delete parse bodies only for
warnings and otherwise reload immediately; only upload calls the new count formatter. Transaction
modals continue to read the legacy positive-only `recategorized_rows` and ignore the other counts and
post-commit warnings.

The frontend design explicitly requires the Categories page, every SIC mapping mutation/upload, and
both transaction modal mapping flows to show all three counts. The independent page test now checks
that every trigger path consumes its returned result; all three screens fail that test.

**Acceptance condition:** display moved, uncategorized, and manual-protected counts, including zeros,
after every listed rule-change trigger. Display post-commit warnings without inviting a retry. Execute
the real page JavaScript in the next re-review because this is the UI boundary where U2-F09 escaped.

### U5-R-F05 remains open — the fix creates conflicting uncategorized semantics

**References:** `internal/service/categorizer.go:423-461`;
`internal/repository/transaction_repo.go:201-203,302-341,412-415`;
`internal/handler/uow5_rule_change_review_test.go:251-288`;
`functional-design/domain-entities.md:17-23,37-48`;
`nfr-requirements/tech-stack-decisions.md:49-57`; DP-U5-03; TD-U5-04.

Preserving source 2 prevents immediate rule reassignment, so the original manual-marker regression test
now passes. It also leaves the row with no category and source manual. The approved design says no
second representation of uncategorized may be introduced and pins that state to category NULL/source
0. The consequences are already observable: `List(Uncategorized:true)` includes this row because its
category is NULL, while `GetUncategorized` and `CountUncategorized` exclude it because its source is 2.
The new consistency test records `page=1/get=0/count=0` for the same database.

Production Revision 1 made a product choice that the first review explicitly required the user or an
approved artifact amendment to settle. The retained manual guarantee and the one-representation rule
cannot both be satisfied for a deleted manual category with the current three-state model without a
specific semantic decision.

**Acceptance condition:** obtain and record the category-deletion decision, then align the database
state, all uncategorized queries, re-examination eligibility, counts, and user-visible behavior. Do not
accept `category_id=NULL/category_source=2` under the current approved artifacts.

### U5-R1-F01 — Medium — Category creation and pattern creation responses are not additive

**References:** `internal/handler/category_handler.go:196-208,360-369`;
`internal/handler/uow5_rule_change_review_test.go:139-169`; DP-U5-03; brownfield compatibility.

Before Revision 1, category-with-pattern creation returned `CategoryWithPatterns` with `category_id`,
`name`, `category_type`, and `patterns` at the top level. Pattern addition returned the pattern fields at
the top level. Revision 1 nests those legacy objects under `category` and `pattern` to place counts
beside them. Existing consumers decoding the prior response types now receive zero-valued objects.

**Acceptance condition:** extend the two successful response shapes with count/warning fields while
retaining their existing top-level fields, or document and approve an intentional breaking transport
change. `TestReviewU5PatternRuleChangeResponsesRemainAdditive` must pass.

**Status:** Open; required regression test fails.

### Revision 1 commands and results

| Command | Result |
|---|---|
| `go test -short -count=1 -run 'TestReviewU5' ./...` | FAIL on the four independently reproducible acceptance failures documented below; all other packages pass or have no matching tests. |
| `go test -count=1 ./...` before reviewer corrections | Expected compile FAIL only at four reviewer-owned call sites after `BulkUpdateCategory` and `BulkClearCategory` changed from `error` to `(int, error)`; all packages that compiled passed. |
| `go test -count=1 ./internal/repository` after corrections | PASS; returned counts are now asserted for empty, 32,765-row, and 40,000-row cases. |
| Focused original UOW-5 service/handler/main tests | All six Revision 0 defect tests pass after the signature correction. |
| `go test -short -count=1 -run 'TestReviewU5' ./internal/service` | FAIL only `TestReviewU5ShippedMappingReloadCannotSplitOnePass`; all other UOW-5 service tests pass. |
| Shipped reload test with `-count=20` | FAIL on all 20 runs with transaction one on category 1 and transaction two on category 2. |
| `go test -short -count=1 -run 'TestReviewU5' ./internal/handler` | FAIL on non-additive response shapes and split uncategorized semantics; original count, warning, cascade-order, and manual-marker assertions pass. |
| `go test -short -count=1 -run 'TestReviewU5' ./cmd/privateledger` | FAIL because category, mapping CRUD, and transaction-modal trigger paths do not consume the reported counts. |
| `go test -race -short -count=1 ./...` | FAIL on the deterministic correctness/UI assertions above; no Go data-race warning emitted. |
| Three required performance tests | PASS. Re-examination median 601.845 ms and no-op median 85.725 ms; populated merge median 2.885 s against 10 s; import medians 3.109/3.272 s with 5.25% SIC overhead against 10%. |
| Two required Rapid properties | PASS in 2.010 s with the established deterministic seed and shrinking configuration. |
| `go vet ./...`; `go build ./...`; `git diff --check` | PASS. Build emitted only the known non-fatal external module-cache metadata warning. |
| Chrome browser fixture | The isolated Categories page loaded successfully from `127.0.0.1:18843`; the extension detached before interaction. Static trigger-path tests already prove the successful callbacks discard the count bodies. Full interaction remains required after production fixes. |

### Reviewer-owned Revision 1 test changes

- Updated repository reviewer tests for `(int, error)` and asserted exact affected-row counts.
- Strengthened the concurrent-manual test to require zero moved/uncategorized counts when SQL excludes
  the newly manual row.
- Added a deterministic shipped-cache reload test for the bypass around `Categorizer.mu`.
- Added trigger-by-trigger page consumption checks for Categories, SIC mappings, and transaction modals.
- Added category/pattern transport compatibility checks for the prior top-level fields.
- Added an observable consistency test across the three uncategorized repository paths after deleting a
  manually assigned category.

## Revision 2/3 Re-review — `073f1b3`, `f5c7d33`, `60d0c0c`

The review began at Revision 2 (`073f1b3`). While it was running, the branch advanced with the approved
U5-R-F05 decision and implementation (`f5c7d33`) and the production provider's generation-test note
(`60d0c0c`). The final tests and verdict use `60d0c0c` plus the reviewer-owned working-tree changes
listed below.

### Finding disposition

| Finding | Current assessment | Status |
|---|---|---|
| U5-R-F01 — one generation per pass | The pass copies mapping answers one SIC code at a time. A reload between two distinct lookups produces a mixed old/new map. | **Open — High** |
| U5-R-F02 — concurrent manual overwrite | The write-time SQL guard and affected-row counts continue to pass. | **Resolved** |
| U5-R-F03 — mapping counts | Normal CRUD, upload, and modal paths display all three counts. Mapping save/delete warning branches omit them. | **Open — High** |
| U5-R-F04 — pattern counts and warnings | Server responses and all Categories trigger callbacks consume counts and warnings. | **Resolved** |
| U5-R-F05 — category deletion/manual provenance | The user approved option A; the artifacts were amended before production changed. Former manual assignments become ordinary uncategorized rows and may be assigned by remaining rules. Both deletion-semantics tests pass after correcting the obsolete reviewer expectation. | **Resolved** |
| U5-R-F06 — empty-category mapping creation | Direct and merge creation continue to invoke re-examination. | **Resolved** |
| U5-R1-F01 — additive response compatibility | Legacy category and pattern fields remain at the top level beside the new fields. | **Resolved** |
| U5-R3-F01 — Categories source breakdown | Rule-change responses omit the optional pattern/SIC split, but the page renders each missing value as zero beside a nonzero moved count. | **Open — Medium** |

### U5-R-F01 remains open — the mapping “snapshot” is assembled across generations

**References:** `internal/service/categorizer.go:245-284`;
`internal/service/sic_categorizer.go:101-136`;
`internal/service/uow5_reexamination_review_test.go:125-168,259-320`; BR-U5-10;
NFR-U3-CON-01; NFR-U5-CON-02.

`snapshotRules` prevents lookups during the transaction traversal, but it builds `sicByCode` by calling
`LookupCategory` once per distinct code. Each call independently acquires and releases
`SICMappingCategorizer.mu`. A mapping reload can therefore publish between two calls, so the resulting
map is not one mapping generation.

The strengthened shipped-path test uses two SIC codes that both map to category 1 in the old cache and
category 2 in the new cache. It pauses after the first code samples the old cache, publishes the new
cache through the shipped `ReloadMappings`, and releases the pass. One transaction reaches category 1
and the other reaches category 2 on 10 of 10 runs.

The production note correctly observed that an atomic implementation may use `prepareMappings` and
never call `LookupCategory`. The reviewer probes now pause after either the live lookup or the second
prepared snapshot, so such an implementation will not hang and is free to choose the atomic mechanism.

**Acceptance condition:** capture the complete mapping index atomically for the pass, alongside its
pattern generation. Both generation tests must pass repeatedly without adding a discarded lookup call.

### U5-R-F03 remains open — mapping warning branches hide the counts

**References:** `cmd/privateledger/web/templates/sic_mappings.html:277-285,317-323`;
`cmd/privateledger/uow5_page_review_test.go:54-78`; FR16;
`functional-design/frontend-components.md:8-20`; the Revision 1 acceptance condition above.

Mapping save and delete display the count formatter only on full success. When the response contains a
post-commit warning, both callbacks return after rendering the warnings and the durable-state message;
the moved, uncategorized, and manual-protected values are omitted. Upload already combines its count
lines and warnings correctly.

This is observable in Chrome with an injected re-examination failure: the mapping-save warning says
the mapping was saved and explains the failure, but contains none of the three counts. The independent
source-path test fails both mutation warning branches.

**Acceptance condition:** include `describeRuleChangeCounts(body)` in the displayed lines for mapping
save and mapping delete whether or not `post_commit_warnings` is nonempty. Preserve the factual,
non-retry warning wording.

### U5-R3-F01 — Medium — Categories invents a zero source breakdown

**References:** `cmd/privateledger/web/templates/categories.html:555-565`;
`internal/handler/category_handler.go:107-151`; FR16.

`describeReexamination` always prints the legacy pattern/SIC split and defaults missing split fields to
zero. Pattern/category rule-change responses contain the three FR16 counts but not those two optional
split fields. Browser execution after adding a pattern therefore displayed “1 moved ... (0 by pattern,
0 by SIC mapping).” The three required counts are correct, but the added explanation contradicts them.

**Acceptance condition:** render the source split only when the response actually contains it, or add
the real split fields to the rule-change response. Never synthesize a zero/zero split for a nonzero move.

### U5-R-F05 test adjudication

Commit `f5c7d33` records the user's “go ahead with A” decision and amends FR7, BR-U5-05, and
NFR-U5-REL-02 before changing `ClearCategory`. That satisfies the prior acceptance condition. The
reviewer replaced the obsolete marker-preservation assertion with the approved behavior: the deleted
manual assignment is cleared, re-examined to the remaining SIC category, stored as rule-sourced, and
reported as one move with zero manual protections. The separate one-representation test also passes.

### Browser execution

Chrome executed the production templates and handlers against the isolated browser-review database.

- Categories add-pattern displayed all three FR16 counts, but also exposed U5-R3-F01.
- SIC mapping save displayed `2/0/0`; deletion displayed `0/1/0` and remained on screen until refresh.
- Transaction-modal mapping displayed `1/0/0` after the delayed real handler completed.
- An injected post-commit re-examination failure kept the mapping committed and rendered the warning,
  but omitted every count, confirming U5-R-F03.

### Reviewer-owned changes in this re-review

- Strengthened the shipped reload test to use two distinct SIC codes.
- Made both generation probes observable through either `LookupCategory` or an atomic
  `prepareMappings` snapshot, removing the mechanism-specific timeout identified by production.
- Corrected the category-deletion test to the approved option A semantics.
- Added the mapping warning-result count test.
- Extended the browser-only fixture with SIC mapping routes and a test-only failure injection trigger.

The shared-worktree production commits include earlier reviewer-authored test/review changes. The
reviewer independently adjudicated and reran them; no production source or production artifact was
modified during this re-review.

### Revision 2/3 commands and results

| Command | Result |
|---|---|
| `go test -count=1 ./...` baseline at Revision 2 | FAIL on the then-open category-deletion consistency test and one noisy import performance sample; all other packages completed. |
| `go test -count=1 ./...` final current-HEAD run | FAIL only the shipped two-code generation test and mapping warning-count test; all other packages pass, with `internal/service` completing in 97.164 s. |
| Category-deletion focused tests after approved test correction | PASS. |
| Both generation tests at `-count=10` | General generation test passes; shipped two-code reload test fails on all 10 repetitions without hanging. |
| `go test -short -count=1 -run 'TestReviewU5' ./...` | FAIL on the shipped two-code generation test and the mapping warning-count test; all other UOW-5 tests pass after the approved category-deletion correction. |
| `go test -race -short -count=1 ./...` | FAIL on those same deterministic assertions; no Go data-race warning emitted. |
| Two required Rapid properties | PASS in 1.774 s. |
| Re-examination performance | PASS: changing median 542.108 ms; no-op median 80.681 ms; target 1.5 s. |
| Populated merge performance | PASS: median 2.719 s; target 10 s. |
| Import performance, isolated rerun | PASS: SIC-free median 3.108 s; SIC-bearing median 3.218 s; 3.52% overhead against 10%. |
| `go vet ./...`; `go build ./...`; `git diff --check` | PASS; build emitted only the known external module-cache metadata warning. |

## Final Status

**FAIL — BLOCKED after Revision 3.** Return U5-R-F01 and U5-R-F03 to the production role. U5-R3-F01
is Medium and does not independently block the gate, but should be corrected with the two High findings.
