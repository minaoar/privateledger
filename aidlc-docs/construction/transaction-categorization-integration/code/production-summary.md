# Production Summary — UOW-3 Transaction Categorization Integration

Production role: Claude. Verification tests, fixtures, test-only dependencies, benchmarks, and the
independent review artifact were **not** authored here and remain owned by a separate-provider session.

Stories: US-02, US-03, US-06, US-12, US-13.

## Production Files

### Modified

| File | Change |
|---|---|
| `internal/repository/transaction_repo.go` | `GetUncategorizedBySICCodes` and `BulkUpdateCategory` now bind the set as one JSON array via `json_each` instead of expanding placeholders |
| `internal/service/categorizer.go` | Single `decide` function; both caches behind one `sync.RWMutex`; `LoadRules` as the reload entry point; both recategorize passes routed through `decide`; split counts |
| `internal/handler/category_handler.go` | Detached `go LoadPatterns()` deleted; reload errors handled; split counts returned |
| `internal/handler/transaction_handler.go` | `CreateSICMappingForTransaction` for the modal workflow |
| `internal/service/sic_mapping_service.go` | Added `FindMappingByCode` read |
| `cmd/privateledger/main.go` | One SIC categorizer wired as both lookup and collaborator, replacing the no-op; modal route registered |
| `cmd/privateledger/web/templates/transactions.html` | SIC context in both modals; opt-in mapping creation; mutually exclusive rule-type choice |
| `cmd/privateledger/web/templates/categories.html` | Pre-run warning and split-count result |
| `API_ROUTES.md` | Modal endpoint documented |

### Created

`internal/service/sic_categorizer.go` — mapping cache, lookup, scoped recategorization, and the UOW-2
collaborator implementation.

No schema change, no new dependency, no new page or template.

## Implemented Contracts

- **One decision function.** `decide` fixes the order: manual stops, existing category stops, text
  patterns, then SIC, and an empty-category mapping assigns nothing. Import, "Recategorize All", and
  per-category recategorization all call it, so the rules cannot differ by entry point.
- **Guarded caches with an all-or-nothing swap.** `LoadRules` builds the pattern set and reloads
  mappings before publishing anything; if either fails, neither cache changes.
- **JSON set passing.** Both expanding builders bind one parameter. Codes are encoded as JSON strings to
  match the TEXT column; IDs as JSON numbers. `BulkUpdateCategory` stays a single atomic statement.
- **Split counts.** Accumulated during the pass, so
  `PatternCategorizedCount + SICCategorizedCount == CategorizedCount` by construction.
- **Single-instance wiring.** One `SICMappingCategorizer` is both `Categorizer`'s lookup and UOW-2's
  collaborator, so the cache the lookup reads is the cache a mapping change reloads.

## Deviation from the Approved Plan

**BR-U3-19 required `LoadPatterns` to become unexported. It was retained as a wrapper that delegates to
`LoadRules`.**

Three independent-role test files call `categorizer.LoadPatterns()`
(`import_regression_perf_test.go:87`, `sic_import_e2e_test.go:73,149`). Unexporting it broke
*compilation*, not an assertion, which would have blocked the entire suite including tests that pass.
Production must not edit those files.

Delegating to `LoadRules` satisfies the rule's actual purpose: BR-U3-19 exists so a caller cannot
refresh one cache and leave the other stale, and after this change there is no exported way to refresh
patterns alone. The literal wording is not met; the guarantee is. Flagged for independent adjudication.

## Commands Run

| Command | Result |
|---|---|
| `gofmt -l ./cmd ./internal` | clean |
| `go build ./...`, `go vet ./...` | pass |
| `git diff --check` | clean |
| `go test -short -count=1 ./...` | all seven packages pass |
| Template/app.js collision sweep | no overlap |

## Production Smoke Verification

Isolated port and temporary database; the user's real database and port 8844 untouched. Seven
transactions were inserted directly to exercise the priority matrix.

| Transaction | SIC | Expected | Observed |
|---|---|---|---|
| SUPERMART STORE | 5412 | Groceries via SIC | Groceries, src=1 |
| AIRLINE TICKET | 5412 | Travel via **pattern** — text beats SIC | Travel, src=1 |
| HOTEL STAY | 7011 → empty category | unchanged | uncategorized |
| UNKNOWN SHOP | 9999 → no mapping | unchanged | uncategorized |
| SUPERMART MANUAL | 5412, `source=2` | untouched | Travel, src=2 |
| SUPERMART RULED | 5412, already categorized | untouched | Travel, src=1 |
| PLAIN NO SIC | none | unchanged | uncategorized |

`AIRLINE TICKET` is the load-bearing case: it matches both a SIC mapping (Groceries) and a text pattern
(Travel), and the pattern won.

Counts returned `processed 5, categorized 2, pattern 1, SIC 1` — the partition holds, and the two
already-categorized rows were excluded by the uncategorized query rather than examined and skipped.

Scoped recategorization: creating a mapping for `9999` returned `recategorized_rows: 1` and categorized
`UNKNOWN SHOP`; a CSV merge giving `7011` a real category returned `recategorized_rows: 1` and
categorized `HOTEL STAY`, confirming the real collaborator replaced the no-op.

Modal endpoint: a transaction with no SIC code returns 422 `no_sic_code`; a request without a category
returns 422 `category_required`. All three pages render 200.

## Known Limitations

- No verification test covers any UOW-3 behavior; all of it belongs to the independent role.
- No benchmark was run. NFR-U3-PERF-01 (20,000 transactions in five seconds) and NFR-U3-PERF-02 (both
  import fixtures) are unmeasured.
- The `json_each` query plan was not inspected; index use is inferred from timing only.
- No page JavaScript was executed — the smoke test exercised HTTP endpoints and server-rendered HTML.

---

# Revision 2 — Independent Review Findings Addressed

Date: 2026-09-06. Responds to the UOW-3 independent review (gate FAIL, four High and two Medium
findings). Every finding was reproduced against the reviewer's tests before being fixed. No test file,
fixture, or review artifact was modified by the production role.

| Finding | Sev | Resolution |
|---|---|---|
| U3-F01 | High | `RecategorizeBySICCodes` called `LookupCategory` directly, giving the scoped path its own priority logic in which SIC beat text patterns. It now routes every candidate through the shared decision function via a decider attached during `NewCategorizerWithSIC`. |
| U3-F02 | High | `GetUncategorizedBySICCodes` filtered only `category_source = 0`; a row can carry a category while its source reads none. It now also requires `category_id IS NULL`, and the shared decision function's existing-category guard applies on this path too. |
| U3-F03 | High | `LoadRules` published mappings under the source's mutex and patterns under the categorizer's, so a reader could see new mappings beside old patterns. `SICMappingCategorizer` now exposes `prepareMappings`/`commitMappings`, and `LoadRules` publishes both under the categorizer's write lock as one generation. `decide` holds its read lock across both rule sources, closing a second window where it sampled patterns, released, then consulted SIC. |
| U3-F04 | High | The page's PATCH marked the row manual before the mapping existed, and scoped recategorization could not correct a row that was no longer uncategorized. The mapping endpoint now restates the row as rule-sourced — only when the stored category already equals the mapping's category, so it can never change a category the user chose. |
| U3-F05 | Medium | Both modal forms now disable their controls immediately before the request and restore them on success, HTTP failure, and transport failure. Controls already disabled for other reasons are left disabled on restore. |
| U3-F06 | Medium | `LoadPatterns` removed entirely. The reviewer migrated their own call sites to `LoadRules`, which removed the compilation obstacle behind the Revision 1 deviation. |

## LoadPatterns Adjudication Accepted

The Revision 1 argument for keeping an exported wrapper is withdrawn. The reviewer was right that the
affected callers were independently owned tests, so no production compatibility API was required.
`LoadRules` is now the sole exported reload entry point, as BR-U3-19 and NFRP-U3-02 always said.

## A Regression Introduced and Caught During This Revision

The first attempt at U3-F01 made a missing decider a hard error. That broke
`TestReviewU3UOW2MergeFiftyThousandAffectedCodes`, which wires a `SICMappingCategorizer` directly as
UOW-2's collaborator without a categorizer — a test that had **passed** for the reviewer.

The hard error was wrong. A standalone mapping categorizer must still work; it simply has no text
patterns to outrank SIC. `decideForTransaction` now delegates to the shared decision function when a
categorizer is attached, and otherwise applies the preservation guards — manual source and existing
category are still never revised — skipping only the pattern step that genuinely does not exist in that
wiring. This is recorded because the regression was self-inflicted while fixing a different finding.

## Verification

| Command | Result |
|---|---|
| `gofmt -l ./cmd ./internal` | clean |
| `go build ./...`, `go vet ./...`, `git diff --check` | clean |
| `go test -count=1 ./...` | **all seven packages pass** (internal/service 178.7s) |
| `go test -race -short -count=1 ./...` | **all seven packages pass, zero data races** |
| Template/`app.js` collision sweep | no overlap |

### A failure that was environmental, not a defect

`TestReviewU3ImportWithMappingsPerformance` failed during an early full-suite run on this machine while
other work was loading it. Run isolated it passes at ratio **1.0250** — 2.5% against the 10% budget,
consistent with the reviewer's measured 3.15%. Recorded rather than passed over, because a performance
failure that is really contention is easy to mistake for a real regression later.

## Independent Smoke Evidence

Beyond the reviewer's tests, the scoped path was exercised through the real HTTP API on an isolated
port. Three transactions all carrying SIC `5412`, with a text pattern `AIRLINE` mapped to Travel, and a
new mapping `5412 -> Grocery` created to trigger scoped recategorization:

```
AIRLINE TICKET    -> Travel    src=1    text pattern beat SIC in the scoped path
SUPERMART         -> Grocery   src=1    SIC applied where no pattern matched
AIRLINE PREPAID   -> Grocery   src=0    existing category left untouched
```

`recategorized_rows` returned 2, matching the two rows that actually changed. The first line is the
direct evidence for U3-F01 and the third for U3-F02.

## Still Not Verified by Production

Benchmarks beyond the isolated PERF-02 re-run, browser execution of the modal disable/restore behaviour
required by U3-F05's acceptance condition, and the query plan under `json_each` — which the reviewer has
now confirmed uses `idx_txn_sic`.

---

# Revision 3 — U3-R2-F01 and U3-R2-F02

Date: 2026-09-06. Responds to the Revision 2 re-review (FAIL: one High, one Medium).

## U3-R2-F02 — resolved

The rule-source follow-up write after a committed mapping was logged but omitted from the response, so
a caller could not tell that the requested BR-U3-31 outcome had not been applied. Both the failed read
and the failed update now append a post-commit warning to the committed result, matching the
committed-with-warning model used elsewhere. `TestReviewU3RuleSourceWriteFailureIsReported` and the
original happy-path test both pass.

## U3-R2-F01 — acceptance condition implemented, and it conflicts with an existing review test

The acceptance condition was implemented literally: replacement patterns are now published only after
the mapping reload succeeds, so a reload that later fails can never expose them.
`TestReviewU3FallbackFailedReloadNeverPublishesPatterns` passes.

`TestReviewU3RuleCachesPublishAtomically` now fails:

```text
observed mixed cache generation category 20; valid old/new results are 30 or 1
```

### The two tests are contradictory for a non-staging lookup

Both fakes block inside `ReloadMappings`, and production cannot know which outcome is coming.

| Test | Reload outcome | Requires during the pending window |
|---|---|---|
| `...PublishAtomically` | succeeds | patterns **already published** |
| `...NeverPublishesPatterns` | fails | patterns **not published** |

`...PublishAtomically` accepts only `30` (complete old) or the new pattern's category (complete new).
Complete-old is unreachable for a non-stager: `stagedUOW3Lookup.ReloadMappings` sets `current = next`
before it blocks, so the mapping side has already changed and no snapshot of it exists on the
categorizer. That leaves complete-new, which requires publishing patterns during the window — exactly
what U3-R2-F01 forbids.

Blocking readers resolves the conflict and the newer test explicitly permits it, but
`...PublishAtomically` calls `Categorize` on the main goroutine after `<-lookup.started`, so blocking
deadlocks it.

### Production is unaffected either way

`*SICMappingCategorizer` implements `prepareMappings`/`commitMappings`, and `main.go` wires exactly that
type, so shipped code always takes the staged path: both sets are built first and published together
under one write lock. Neither failure scenario is reachable in production. Both concern the
compatibility fallback for a lookup that cannot stage, which no shipped code uses.

### Why the failing test was left failing

Reverting would restore behaviour the reviewer correctly identified as wrong — publishing rules from a
reload that fails. Keeping the correct behaviour and reporting the conflict seemed better than turning
a suite green by reintroducing a known defect.

Suggested resolutions, for the reviewer to choose:

1. Have `stagedUOW3Lookup` implement `prepareMappings`/`commitMappings`, so the test exercises the path
   production actually takes.
2. Have the fake swap `current` when its mappings are committed rather than at reload entry, so
   complete-old is observable.
3. Call `Categorize` in a goroutine, as `...NeverPublishesPatterns` does, allowing the blocking
   resolution.

## Verification

`gofmt`, `go build ./...`, `go vet ./...`, `git diff --check` clean.
`go test -count=1 ./...`: six of seven packages pass; `internal/service` fails on
`TestReviewU3RuleCachesPublishAtomically` alone, for the reason above. Every other Revision 1 and
Revision 2 test passes, including the retained Rapid replay, the 50,000-code cross-unit fixture, the
standalone fallback test, and the performance fixtures.

---

# Revision 4 — U3-R2-F01 Fallback Reader Exclusion

Date: 2026-09-06. Responds to the Revision 3 cache-test adjudication.

## Correction to the Revision 3 analysis

Revision 3 argued the two cache tests were contradictory and proposed moving the fake's mapping
assignment after its blocking point. **That analysis was wrong on both counts.**

The reviewer corrected `TestReviewU3RuleCachesPublishAtomically` to run `Categorize` in a goroutine and
tolerate a blocked decision, which removes the deadlock that made blocking look impossible. With that,
the two tests state one coherent rule: a pending fallback reload may expose the complete old generation
or block, and must never expose a mixed generation or patterns from a reload that fails.

More importantly, the proposed fake change would have been harmful. As the adjudication puts it, the
caller has no contract permitting it to assume publication occurs only at method return. A non-staging
`ReloadMappings` may legitimately mutate its state before returning, so production must be safe against
that. Editing the fake would have hidden a valid interleaving rather than fixed the code. Revision 3
reached for the one outcome its own caveat had flagged as the risk — the production author concluding
the test was at fault.

## Fix

`LoadRules`'s non-staging fallback now holds the categorizer write lock across **both** the
`ReloadMappings` call and the pattern publication. Categorization blocks for the duration of a fallback
reload and resumes on one complete generation; if the reload fails, patterns are never published and
the previous generation stands.

The reviewer's preferred option — removing the fallback entirely — was not taken. Their fakes are
non-stagers whose tests require `ReloadMappings` to be invoked and to block, so removing it would have
required editing independent tests, which the adjudication forbids. The adjudication anticipates this:
making `stagedUOW3Lookup` a stager is "sufficient only if the production fallback is removed".

The shipped path is unchanged. `*SICMappingCategorizer` implements staging, so production still
prepares both rule sets and publishes them together without blocking readers.

## Verification

Acceptance command from the adjudication, all four passing:

```text
go test ./internal/service -run '^TestReviewU3(RuleCachesPublishAtomically|FallbackFailedReloadNeverPublishesPatterns|FailedMappingReloadKeepsPatternCache|ConcurrentCategorizeAndReload)$' -count=1
--- PASS: TestReviewU3RuleCachesPublishAtomically
--- PASS: TestReviewU3FailedMappingReloadKeepsPatternCache
--- PASS: TestReviewU3FallbackFailedReloadNeverPublishesPatterns
--- PASS: TestReviewU3ConcurrentCategorizeAndReload
```

| Command | Result |
|---|---|
| `gofmt -l ./cmd ./internal`, `go vet ./...`, `git diff --check` | clean |
| `go test -race -short -count=1 ./...` | **all seven packages pass, zero data races** |
| `go test -count=1 ./...` | six of seven pass; `internal/service` failed only on `TestReviewU3ImportWithMappingsPerformance` |

### The performance failure, measured rather than assumed

`TestReviewU3ImportWithMappingsPerformance` failed in the loaded full-suite run and was re-run in
isolation rather than dismissed as contention a second time:

```text
SIC-free   median 4.7036785s
SIC-bearing median 4.878474125s   ratio 1.0372
--- PASS (60.83s)
```

3.7% against the 10% budget, consistent with the reviewer's 3.15%. There is also structural reason it
cannot be this revision: the fixture builds a real `SICMappingCategorizer` through
`NewCategorizerWithSIC`, so it takes the staged path, and this change touched only the non-staging
fallback. The measurement is the evidence; the structure only corroborates it.
