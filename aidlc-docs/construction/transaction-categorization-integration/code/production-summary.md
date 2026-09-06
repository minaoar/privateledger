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
