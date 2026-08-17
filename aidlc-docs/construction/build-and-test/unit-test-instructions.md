# Unit Test Execution

## Run Unit Tests

```bash
make test           # or: go test ./...
go test -v ./internal/service/          # verbose, this unit's tests
go test -cover ./internal/service/      # with coverage
```

## Current State

| Metric | Value |
|---|---|
| Test files in repo before this unit | **0** |
| Test files after this unit | 1 — `internal/service/insights_uncategorized_test.go` |
| Test functions | 12 (25 cases including subtests) |
| Result | all pass |
| Coverage, `internal/service` | **40.7%** of statements |
| Coverage, all other packages | **0.0%** |

**Read `go test ./...` output carefully.** Packages with no test files report `ok ... [no test files]`, which is not a pass — it means nothing was verified. Before this unit, the entire suite was in that state, so `make test` succeeded while testing nothing.

## What the tests cover

Located in `internal/service/insights_uncategorized_test.go`. Each opens a **real temporary SQLite database** migrated with the production schema (`t.TempDir()`, cleaned up automatically) — no mocks, so repository SQL is exercised for real.

| Test | Guards |
|---|---|
| `TestUncategorizedRowFor` | which category types get a pseudo-row, and its debit/credit pairing |
| `TestIsUncategorizedForType` | summary-card routing of uncategorized debits/credits |
| `TestExpenseBreakdown_UncategorizedRow` | expense row sums debits only; column total includes it |
| `TestIncomeBreakdown_UncategorizedRow` | income row sums credits only |
| `TestBreakdown_NoUncategorizedRowForInvestmentOrGeneral` | Investment/General never gain a row |
| `TestExpenseBreakdown_UncategorizedRowPresentWhenEmpty` | row persists at zero (stable table shape) + sentinel `CategoryID` |
| `TestBreakdown_NoDoubleCounting` | **key invariant** — sum of rows equals the column total |
| `TestSummaryCards_IncludeUncategorized` | expense/income cards include uncategorized; investment excludes it |
| `TestSummaryCards_PreviousAmountIncludesUncategorized` | `PreviousAmount` and `ChangePercent` consistent with `TotalAmount` |
| `TestMonthlySummary_PieChartUncategorizedSlice` | slice value, count, and a color distinct from reserved ones |
| `TestMonthlySummary_NoPieSliceWhenZero` | no zero-value slice |
| `TestExpenseBreakdownTable_MatchesGenericForExpenses` | the delegate refactor stays equivalent to the generic function |
| `TestTransactionFilter_CategoryIsNullAndTransactionType` | repository filters, incl. `Uncategorized` being deliberately broader |

### Why the fixtures look artificial

The production database contains **no categorized income and no investment transactions**, so real data cannot exercise those branches — a check against it passes trivially (`0 == 0`). The fixtures construct those cases explicitly. This is the reason the tests exist rather than relying on a data spot-check.

## Mutation verification

A passing test proves nothing until it has been shown to fail. Four deliberate defects were injected and each was caught by the expected test:

| Injected defect | Caught by |
|---|---|
| Previous period ignores uncategorized (the original Revision-1 bug) | `TestSummaryCards_PreviousAmountIncludesUncategorized` |
| Broader `Uncategorized` filter used for row sums (double-count) | `TestBreakdown_NoDoubleCounting` |
| Investment gains an uncategorized row | `TestUncategorizedRowFor`, `TestIsUncategorizedForType`, `TestSummaryCards_IncludeUncategorized` |
| Pie chart counts credits as expenses | `TestMonthlySummary_PieChartUncategorizedSlice` |

Source was restored and confirmed byte-identical afterwards. Worth repeating whenever these tests are materially changed.

## Known gap: templates are untested

`dashboard.html` has **no automated coverage**. Template errors are not build errors — `parseTemplate` uses `template.Must`, so a malformed template panics on first render. The template changes in this unit were verified by rendering the real page (HTTP 200) and asserting on the HTML, but that check is manual and lives outside the repo.

To close this gap, a handler-level test could parse each template with the same `FuncMap` and execute it against a synthetic `DashboardStats`. Not implemented in this unit.

## If tests fail

1. Run `go test -v ./internal/service/` to identify the failing case.
2. Failure messages state expected vs. actual with the fixture amounts, e.g. `PreviousAmount = -100, want -200 (must include uncategorized)`.
3. `TestBreakdown_NoDoubleCounting` failing usually means a row-summing query switched from `CategoryIsNull` to the broader `Uncategorized` filter — see the "Canonical Definition" section of the code generation plan.
