# Code Generation Plan — Show Uncategorized Transactions on Dashboard

> **Revision 2** (2026-08-03) — revised after code review of Revision 1 against actual source.
> See "Revision 2 Changes" at the bottom for what changed and why.

## Unit Context

- **Unit Name**: uncategorized-dashboard
- **Branch**: show-uncategorized-transactions
- **Workspace Root**: /Users/tanzil/Documents/GitHub/privateledger
- **Project Type**: Brownfield — modify existing files in-place

---

## Pre-Generation Notes (Verified Against Source)

**Verified correct — no work needed:**

- **FR10 already complete**: `transaction_handler.go:63-85` and `page_handler.go:188-199` already parse `uncategorized=true`, `start_date`, `end_date`. No handler changes.
- **FR4 (PeriodRanges) not needed**: `CategoryBreakdownTable.Periods []MonthPeriod` already carries `StartDate`/`EndDate time.Time`. No model changes.
- **Template helpers exist**: `abs` (`page_handler.go:75`) and `formatDateForURL` (`page_handler.go:81`) are registered in the FuncMap.
- **`CategoryID = 0` sentinel is safe**: `CategoryByMonth.CategoryID` is `int`, not `*int` (`insights_service.go:288`). Real categories are SQLite auto-increment, always `>= 1`.
- **Transaction type constants**: `model.TransactionTypeDebit = 1`, `model.TransactionTypeCredit = 2` (`model/transaction.go:12-13`).
- **Bar chart is automatic**: reads `ExpenseBreakdown.MonthlyTotals`. Adding the uncategorized row to `monthlyTotals` updates it with no separate step.
- **Pie chart template needs no change**: `dashboard.html:419` reads
  `{{if eq .CategoryName "Others"}}'#9ca3af'{{else}}{{if .CategoryColor}}'{{.CategoryColor}}'{{else}}'#6b7280'{{end}}{{end}}`
  — an explicit `CategoryColor` is honored.

**Key findings that shape this plan:**

- **The two breakdown functions are duplicates.** `GetExpenseBreakdownTable` (`:324-413`) and `GetCategoryBreakdownTable` (`:416-519`) are functionally identical; the former only hardcodes `CategoryTypeExpense`. Type aliases (`ExpenseBreakdownTable = CategoryBreakdownTable`, `ExpenseCategoryByMonth = CategoryByMonth`) make their returns the same type. Both are called only from `GetDashboardStats` (`:746`, `:753`, `:760`). → Collapse into one implementation (Step 2).
- **Transactions are already in memory in two places.** `getCategoryTypeSummary` (`:549-566`) loads **all** transactions for the current and previous periods with **no category filter**, then filters in memory. `GetMonthlySummary` (`:134`) does the same for one period and already identifies uncategorized rows at `:156`. → Reuse those slices instead of issuing new queries (Steps 4, 5).
- **`PreviousAmount` returns the raw value.** `CategoryTypeSummary` returns `TotalAmount: displayTotal` (absolute) but `PreviousAmount: previousTotal` (raw, `:641-642`). Uncategorized must be folded in **before** the abs block at `:596`, or the two fields disagree.

---

## Decision: Canonical "Uncategorized" Definition

Three conflicting definitions exist in the codebase today:

| Location | Definition |
|---|---|
| `transaction_repo.go:185` (`Uncategorized` filter) | `category_id IS NULL OR category_source = 0` |
| `transaction.go:78` (`IsUncategorized()`) | `CategoryID == nil \|\| CategorySource == None` |
| `transaction_repo.go:280,317` | `category_source = 0` only |

Meanwhile the **categorized** rows in the breakdown tables filter on `CategoryID` alone, with no `category_source` check (`:377`, `:482`).

**Risk**: a row with `category_id=5, category_source=0` would be counted in *both* the category-5 row and the new uncategorized row, inflating the `monthlyTotals` column total. That state appears unreachable today (`SetCategory` always pairs both fields; `UpdateCategory(id, nil, None)` nulls both) but no DB constraint enforces it.

**Decision — for this feature, "uncategorized" means `category_id IS NULL`.** This is the exact complement of what the categorized rows include, which makes double-counting structurally impossible rather than merely unlikely.

**Implementation**: this feature uses `txn.CategoryID == nil` in service-layer loops and a new `CategoryIsNull` repository filter for the per-period queries. It does **not** use the existing `Uncategorized` filter, whose `OR category_source = 0` clause is what creates the overlap. The existing filter is left untouched — the transactions-page link target relies on it, and its broader definition is correct in that context (it is not being summed alongside categorized rows).

**Out of scope**: reconciling the three definitions codebase-wide. Worth a follow-up ticket.

---

## Files to Modify

| # | File | Change Type | Reason |
|---|---|---|---|
| 1 | `internal/repository/transaction_repo.go` | Minor additive | Add `TransactionType` and `CategoryIsNull` to `TransactionFilter`; handle both in `List()` |
| 2 | `internal/service/insights_service.go` | Additive + dedup | Collapse duplicate breakdown fn; add uncategorized to tables, summary cards, pie chart |
| 3 | `cmd/privateledger/web/templates/dashboard.html` | Minor additive | Render uncategorized rows with correct links and styling |

**Scope confirmation**: Investment breakdown table gets **no** uncategorized row (per requirements Q1=A — two pseudo-categories only).

---

## Step-by-Step Plan

### Step 1 — Extend `TransactionFilter`
- [x] **File**: `internal/repository/transaction_repo.go`
- **What**: Add two fields to the `TransactionFilter` struct (`:133-143`):
  ```go
  CategoryIsNull  bool             // Only transactions with category_id IS NULL
  TransactionType *model.TransactionType // Filter by debit (1) / credit (2)
  ```
- **SQL in `List()`** — add after the existing `Uncategorized` block (`:184-186`):
  ```go
  if filter.CategoryIsNull {
      query += " AND t.category_id IS NULL"
  }

  if filter.TransactionType != nil {
      query += " AND t.transaction_type = ?"
      args = append(args, *filter.TransactionType)
  }
  ```
- **Why `CategoryIsNull` is separate from `Uncategorized`**: see "Canonical Definition" above — the existing filter's `OR category_source = 0` clause overlaps with the categorized rows.
- **Type note**: `TransactionType` is typed `*model.TransactionType` (not `*int`) to match the field on `model.Transaction` and avoid conversions at call sites.

---

### Step 2 — Collapse `GetExpenseBreakdownTable` into `GetCategoryBreakdownTable`
- [x] **File**: `internal/service/insights_service.go`
- **What**: Replace the body of `GetExpenseBreakdownTable` (`:324-413`, ~90 lines) with a delegate:
  ```go
  // GetExpenseBreakdownTable builds a table of expense categories across the last N months.
  // Retained for backwards compatibility; delegates to GetCategoryBreakdownTable.
  func (s *InsightsService) GetExpenseBreakdownTable(year, month, months int) (*ExpenseBreakdownTable, error) {
      return s.GetCategoryBreakdownTable(model.CategoryTypeExpense, year, month, months)
  }
  ```
- **Why**: the two functions are byte-equivalent apart from the hardcoded type filter and log strings. Collapsing means the uncategorized logic in Step 3 is written **once** instead of twice.
- **Safety**: both are called only from `GetDashboardStats` (`:746`, `:753`, `:760`); return types are aliases of each other, so no call-site or JSON-shape change.
- **Verification**: after this step the expense table must render identically to before — no uncategorized row yet.

---

### Step 3 — Add uncategorized row to `GetCategoryBreakdownTable`
- [x] **File**: `internal/service/insights_service.go`
- **What**: after the `categoryBreakdown` loop (`:506`) and before the return (`:514`), append an uncategorized row for expense and income types only.
- **Logic**:
  ```
  var uncatName string
  var uncatTxnType model.TransactionType
  switch categoryType {
  case model.CategoryTypeExpense:
      uncatName, uncatTxnType = "Uncategorized Expenses", model.TransactionTypeDebit
  case model.CategoryTypeIncome:
      uncatName, uncatTxnType = "Uncategorized Incomes", model.TransactionTypeCredit
  default:
      // Investment and General: no uncategorized row
      return &CategoryBreakdownTable{...}, nil
  }

  uncatRow := CategoryByMonth{
      CategoryID:    0,        // sentinel — not a real category
      CategoryName:  uncatName,
      CategoryIcon:  nil,
      CategoryColor: nil,
      MonthlyTotals: make(map[string]float64),
  }

  for _, period := range periods {
      filter := repository.TransactionFilter{
          CategoryIsNull:  true,
          TransactionType: &uncatTxnType,
          StartDate:       &period.StartDate,
          EndDate:         &period.EndDate,
      }
      transactions, err := s.txnRepo.List(filter)
      if err != nil { slog.Error(...); continue }   // match existing error style at :488-493

      var total float64
      for _, txn := range transactions { total += txn.Amount }

      uncatRow.MonthlyTotals[period.Label] = total
      monthlyTotals[period.Label] += total          // feeds the bar chart (FR8)
  }

  categoryBreakdown = append(categoryBreakdown, uncatRow)
  ```
- **Always append** (NFR1): the row is added even when every period is $0, so the table shape stays stable month to month.
- **Bar chart (FR8)**: adding to `monthlyTotals` is what updates it — no separate step.
- **Error handling**: matches the existing `continue`-on-error pattern used by the category loop.

---

### Step 4 — Include uncategorized in summary cards (`getCategoryTypeSummary`)
- [x] **File**: `internal/service/insights_service.go`
- **What**: extend the two existing in-memory loops (`:571-593`). **No new queries** — `currentTxns` and `previousTxns` are already unfiltered full-period loads (`:549-566`).
- **Current loop** (`:571-581`) — add an `else` branch:
  ```go
  for _, txn := range currentTxns {
      if txn.CategoryID != nil {
          for _, catID := range categoryIDs {
              if *txn.CategoryID == catID {
                  currentTotal += txn.Amount
                  currentCount++
                  break
              }
          }
      } else if isUncategorizedForType(categoryType, txn.TransactionType) {
          currentTotal += txn.Amount
          currentCount++
      }
  }
  ```
- **Previous loop** (`:584-593`): same `else` branch, adding to `previousTotal` (no count — the existing loop tracks no count).
- **Helper** (new, package-private):
  ```go
  // isUncategorizedForType reports whether an uncategorized transaction of the
  // given debit/credit type belongs under the given category type's summary.
  func isUncategorizedForType(ct model.CategoryType, tt model.TransactionType) bool {
      switch ct {
      case model.CategoryTypeExpense:
          return tt == model.TransactionTypeDebit
      case model.CategoryTypeIncome:
          return tt == model.TransactionTypeCredit
      default:
          return false // Investment and General unchanged
      }
  }
  ```
- **CRITICAL — fold in before the abs block**: uncategorized is added to `currentTotal`/`previousTotal`, which are consumed at `:596-603` to derive `absCurrent`/`absPrevious`, at `:607-619` for `changePercent`/`changeDirection`, and at `:627-630` for `displayTotal`. Modifying the raw totals upstream makes **every** downstream field consistent. Adding to `absCurrent`/`absPrevious` directly would leave `PreviousAmount` (`:642`, which returns raw `previousTotal`) excluding uncategorized while `TotalAmount` includes it.
- **`TransactionCount`**: uncategorized transactions bump `currentCount`, so the count matches the total it accompanies.

---

### Step 5 — Add uncategorized slice to the pie chart (`GetMonthlySummary`)
- [x] **File**: `internal/service/insights_service.go`
- **What**: accumulate uncategorized totals inside the **existing** transaction loop (`:152-183`) — that loop already walks every transaction in the period and already branches on uncategorized at `:156`. **No new query.**
- **In the loop**, alongside the existing `summary.UncategorizedCount++`:
  ```go
  if txn.CategoryID == nil && txn.TransactionType == model.TransactionTypeDebit {
      uncatTotal += txn.Amount   // negative for debits
      uncatCount++
  }
  ```
- **After the top-5 / "Others" block** (`:242-269`), append:
  ```go
  if uncatTotal != 0 {
      uncatColor := uncategorizedSliceColor   // "#4b5563"
      summary.CategoryBreakdown = append(summary.CategoryBreakdown, CategoryBreakdown{
          CategoryID:    nil,
          CategoryName:  "Uncategorized",
          CategoryColor: &uncatColor,
          TotalAmount:   uncatTotal,   // negative — template applies abs()
          Count:         uncatCount,
      })
  }
  ```
- **Color choice**: `#4b5563` (medium-dark grey), settled after visual review on the running dashboard. Progression: `#6c757d` (rejected in plan review — indistinguishable from the fallback) → `#8b5cf6` violet (too purple) → `#374151` (too dark) → **`#4b5563`**.
- **Constraint**: the template's no-color fallback is `#6b7280` and "Others" is `#9ca3af` (`dashboard.html:419`), both mid-greys. The uncategorized slice must stay clear of both, which bounds how light it can go. `TestMonthlySummary_PieChartUncategorizedSlice` asserts this and will fail if a future change drifts into either.
- **Guard**: only append when `uncatTotal != 0` — a $0 slice adds nothing to a pie chart. (Unlike the tables, which always show the row for shape stability.)
- **Deliberate**: uncategorized is appended **after** the top-5 cut, so it is exempt from ranking and always visible when non-zero. The chart may therefore show up to 7 slices.
- **Consistency benefit**: sourcing from the same loop that computes `summary.UncategorizedCount` guarantees the pie chart agrees with the uncategorized count shown elsewhere on the dashboard.

---

### Step 6 — `dashboard.html`: Expense Breakdown table
- [x] **File**: `cmd/privateledger/web/templates/dashboard.html`
- **What**: inside `{{range $category := .Stats.ExpenseBreakdown.Categories}}` (`:208`), branch on `CategoryID == 0`.
- **Category name cell** (`~:211-213`):
  ```html
  <td class="ps-3">
      {{if eq $category.CategoryID 0}}
          <span class="text-muted fst-italic">{{$category.CategoryName}}</span>
      {{else}}
          {{if $category.CategoryIcon}}<i class="bi bi-{{$category.CategoryIcon}} me-2"></i>{{end}}
          {{$category.CategoryName}}
      {{end}}
  </td>
  ```
- **Amount cells** (`~:215-225`, link currently at `:218`):
  ```html
  <td class="text-end">
      {{$value := index $category.MonthlyTotals .Label}}
      {{if ne $value 0.0}}
          {{if eq $category.CategoryID 0}}
              <a href="/transactions?uncategorized=true&start_date={{formatDateForURL .StartDate}}&end_date={{formatDateForURL .EndDate}}"
                 class="text-muted text-decoration-none"
                 data-testid="uncategorized-expense-link"
                 title="View uncategorized expenses in this period">
                  ${{printf "%.0f" (abs $value)}}
              </a>
          {{else}}
              <a href="/transactions?category_id={{$category.CategoryID}}&start_date={{formatDateForURL .StartDate}}&end_date={{formatDateForURL .EndDate}}"
                 class="expense-breakdown-link"
                 title="View transactions for {{$category.CategoryName}} in this period">
                  ${{printf "%.0f" (abs $value)}}
              </a>
          {{end}}
      {{else}}
          <span class="text-muted">-</span>
      {{end}}
  </td>
  ```
- **Link definition note**: the link uses `uncategorized=true`, whose transactions-page definition is broader than the `category_id IS NULL` used to compute the row total. In practice these return the same set (see "Canonical Definition"); the broader filter is the right behavior for a browse view.
- **Guard**: `{{if gt (len .Stats.ExpenseBreakdown.Categories) 0}}` (`:188`) is preserved — now always true.

---

### Step 7 — `dashboard.html`: Income Breakdown table
- [x] **File**: `cmd/privateledger/web/templates/dashboard.html`
- **What**: same pattern as Step 6, applied to the income table (`{{range}}` at `:279`, link at `:289`).
- **Category name cell** (`~:282-284`): muted italic when `CategoryID == 0`.
- **Amount cells** (`~:285-298`):
  - `CategoryID == 0` → `/transactions?uncategorized=true&start_date=...&end_date=...`, `class="text-muted text-decoration-none"`, `data-testid="uncategorized-income-link"`
  - otherwise → existing `category_id` link with `class="text-success text-decoration-none"`
- **Guard**: `{{if gt (len .Stats.IncomeBreakdown.Categories) 0}}` (`:259`) preserved.
- **Investment table** (`:330-360`): **unchanged** — no uncategorized row.

---

### Step 8 — Pie chart: verify only, no change expected
- [x] **File**: `cmd/privateledger/web/templates/dashboard.html`
- **What**: confirm `:419` renders the dark grey from Step 5 via its `{{if .CategoryColor}}` branch. Verified by inspection — **no edit expected**. Flag if the rendered chart disagrees.

---

### Step 9 — Build verification
- [x] `make build` completes with no errors
- [x] `go vet ./...` passes
- [x] `go test ./...` passes (no regressions from the Step 2 collapse)

---

### Step 10 — Manual dashboard testing
- [x] `make run`, open dashboard
- [x] "Uncategorized Expenses" row present in Expense Breakdown (amounts or dashes)
- [x] "Uncategorized Incomes" row present in Income Breakdown
- [x] Investment Breakdown has **no** uncategorized row
- [x] Violet "Uncategorized" slice in pie chart, visually distinct from "Others" and from uncolored categories
- [x] Bar chart monthly totals include uncategorized amounts
- [x] Expenses summary card total includes uncategorized debits; % change recomputed against a previous period that also includes them
- [x] Income summary card total includes uncategorized credits
- [x] **No double-count**: sum of visible category rows for one period equals that period's column total
- [x] Click an uncategorized amount → lands on `/transactions?uncategorized=true&start_date=...&end_date=...` with correctly filtered results
- [x] Existing categorized rows, links, and the expense table's pre-change output are unaffected

---

## Requirement Traceability

| Requirement | Implementation | Step | File |
|---|---|---|---|
| FR1/FR2 — Uncategorized query | `CategoryIsNull` + `TransactionType` filters | 1 | `transaction_repo.go` |
| FR3 — Sentinel `CategoryID=0` | Used directly; no model change | 3 | — |
| FR4 — Period dates in template | Already on `Periods[]` | — | (no change) |
| FR5 — Uncategorized row in tables | Single implementation after fn collapse | 2, 3 | `insights_service.go` |
| FR6 — Summary cards | Reuse in-memory txns; fold into raw totals | 4 | `insights_service.go` |
| FR7 — Pie chart | Accumulate in existing loop | 5 | `insights_service.go` |
| FR8 — Bar chart | Automatic via `monthlyTotals` | 3 | `insights_service.go` |
| FR9 — Clickable links | Conditional link targets | 6, 7 | `dashboard.html` |
| FR10 — Handler params | Already implemented | — | (no change) |

---

## Revision 2 Changes

Revision 1 was reviewed against the actual source. Five issues were found and corrected:

| # | Issue in Revision 1 | Fix |
|---|---|---|
| 1 | Steps 4 & 5 issued 5 new DB queries | Both functions already hold the needed transactions in memory — reuse the existing loops. **5 fewer queries per dashboard load.** |
| 2 | Step 4 added uncategorized to `absCurrent`/`absPrevious`, but `PreviousAmount` returns raw `previousTotal` → the two fields would disagree | Fold into `currentTotal`/`previousTotal` **before** the abs block at `:596`; also decided `TransactionCount` includes uncategorized |
| 3 | Three conflicting "uncategorized" definitions created a latent double-count that the plan's own quality gate did not guarantee | Pinned one definition (`category_id IS NULL`) via a new `CategoryIsNull` filter; documented the rationale and the follow-up |
| 4 | Steps 2 & 3 duplicated the uncategorized logic into two already-identical functions | Collapse `GetExpenseBreakdownTable` into a delegate first, then write the logic once. **~90 fewer lines.** |
| 5 | Pie slice `#6c757d` was indistinguishable from the `#6b7280` no-color fallback and near "Others" `#9ca3af` | Changed to a clearly darker grey `#4b5563` |

Also settled: Investment breakdown gets no uncategorized row (confirms requirements Q1=A).

Unchanged from Revision 1: Steps 1, 6, 7, 8, 9, 10 (Step 1 gained one field; Step 9 gained a test run).
