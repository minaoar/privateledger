# Independent Review — UOW-5 Rule-Sourced Recategorization

## Gate Result

**FAIL — BLOCKED pending production revision**

Production revision `3bd7903061805962efd06cc8f1c2ca915075be21` does not yet satisfy the
approved UOW-5 gate. Independent tests found five High findings and one Medium finding. Required
tests for one-generation consistency, concurrent manual protection, trigger coverage, result counts,
post-commit warning visibility, and the short race-suite command are not green.

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
| Runtime scope reviewed | `internal/service/categorizer.go`; `internal/service/sic_categorizer.go`; `internal/service/sic_mapping_service.go`; `internal/repository/transaction_repo.go`; `internal/handler/category_handler.go`; `cmd/privateledger/web/templates/categories.html`; `cmd/privateledger/web/templates/sic_mappings.html` |
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

## Final Status

**FAIL — BLOCKED.** Return U5-R-F01 through U5-R-F06 to the production role. Re-review the pinned
production revision after fixes and any approved category-deletion/empty-mapping contract amendments.
