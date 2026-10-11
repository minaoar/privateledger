# Requirements — Show Uncategorized Transactions on Dashboard

## Intent Analysis

| Field | Value |
|---|---|
| **User Request** | Uncategorized transactions do not appear on the dashboard. Add them as "Uncategorized Expenses" and "Uncategorized Incomes" pseudo-categories visible in breakdown tables, pie chart, bar chart, and summary cards. |
| **Request Type** | Enhancement |
| **Scope** | Multiple Components (service, repository, handler, frontend) |
| **Complexity** | Moderate |

---

## Classification Decision

**Chosen approach (Q1: A)** — Two pseudo-categories only:
- **"Uncategorized Expenses"**: all uncategorized debits (`transaction_type = 1`, `category_id IS NULL OR category_source = 0`)
- **"Uncategorized Incomes"**: all uncategorized credits (`transaction_type = 2`, `category_id IS NULL OR category_source = 0`)

Investment pseudo-category is omitted: there is no data signal to distinguish an uncategorized investment from an uncategorized expense — both are debits with no category assigned.

---

## Functional Requirements

### FR1 — New repository query: uncategorized monthly totals
Add a method to the insights repository that returns per-period totals for uncategorized transactions filtered by `transaction_type`. The query must:
- Filter: `(category_id IS NULL OR category_source = 0) AND transaction_type = ?`
- Group by custom-month period boundaries (respecting `start_of_month` config, matching the period logic used in `GetCategoryBreakdownTable`)
- Accept a list of period date ranges and return `map[periodKey]float64`

This method is used to build the uncategorized row in the breakdown table and to feed the bar chart via `MonthlyTotals`.

### FR2 — New repository query: uncategorized total for a single period
Add a method (or reuse FR1 with a single period) to retrieve the total absolute amount of uncategorized transactions of a given `transaction_type` within a start/end date range. Used by the summary card (FR5) and the pie chart entry (FR6).

### FR3 — Use `CategoryID = 0` as uncategorized sentinel
No new field is needed. Real categories always have `CategoryID >= 1` (SQLite auto-increment). The uncategorized pseudo-row is identified by `CategoryID = 0`. The dashboard template uses `{{if eq .CategoryID 0}}` to render it differently:
- Without a color swatch
- With muted/grey styling for the category label
- With period cell links pointing to the transactions page (FR9) instead of a category filter

### FR4 — Extend `CategoryBreakdownTable` model with `PeriodRanges`
Add `PeriodRanges map[string][2]string` to `CategoryBreakdownTable`, where the key is the period key (e.g., `"2025-12"`) and the value is `[startDate, endDate]` in ISO date format (`"2006-01-02"`). This allows the template to generate date-ranged links without date arithmetic in Go templates.

### FR5 — Extend `GetCategoryBreakdownTable` with uncategorized row
After building categorized rows in `GetCategoryBreakdownTable`:
- **If `categoryType == CategoryTypeExpense`**: append one `CategoryRow` with `IsUncategorized = true`, `CategoryName = "Uncategorized Expenses"`, and Monthly amounts from FR1 (`transaction_type = 1, debit`). Include these amounts in `MonthlyTotals`.
- **If `categoryType == CategoryTypeIncome`**: append one `CategoryRow` with `IsUncategorized = true`, `CategoryName = "Uncategorized Incomes"`, and Monthly amounts from FR1 (`transaction_type = 2, credit`). Include these amounts in `MonthlyTotals`.
- **If `categoryType == CategoryTypeInvestment`**: no change — skip.
- The uncategorized row is always appended even when all period amounts are $0 (consistent visibility).

Also populate `PeriodRanges` on the returned table (FR4).

### FR6 — Extend summary cards to include uncategorized amounts (Q3: A)
In the function computing `TotalExpenses` and `TotalIncome` for the summary cards:
- Add uncategorized debit total (`transaction_type = 1`) for the current period to `TotalExpenses`.
- Add uncategorized credit total (`transaction_type = 2`) for the current period to `TotalIncome`.
- `TotalInvestment` is unchanged.

### FR7 — Extend "Top Expense Categories" pie chart to include uncategorized (Q2: D)
In `GetMonthlySummary`, after computing `CategoryBreakdown` (top 5 expense categories):
- Query uncategorized expense total for the current month using FR2 (`transaction_type = 1`).
- Append one `CategorySummary` entry: `Name = "Uncategorized"`, `Color = "#6c757d"` (Bootstrap secondary grey), no icon, amount from above query.
- Append even if $0 (consistent pie chart slice).

### FR8 — "Expense Trends" bar chart includes uncategorized (Q2: D — automatic via FR5)
`ExpenseBreakdown.MonthlyTotals` is the sum across all expense rows per period. Because FR5 adds the uncategorized row and includes its amounts in `MonthlyTotals`, the bar chart automatically reflects uncategorized expenses without additional service changes.

### FR9 — Clickable uncategorized row cells link to filtered transactions (Q4: A)
In `dashboard.html`, period cells in the uncategorized row must be rendered as anchor links using `PeriodRanges` (FR4):
```
/transactions?uncategorized=true&start_date={period_start}&end_date={period_end}
```

Existing categorized category rows may or may not be clickable today — the uncategorized row must be clickable regardless.

### FR10 — Transactions page handles `uncategorized=true` query param
In the transaction list handler, parse the `uncategorized` query parameter and set `filter.Uncategorized = true` when the value is `"true"`. The `TransactionRepository.List()` already applies this filter via `WHERE (category_id IS NULL OR category_source = 0)`.

Also parse `start_date` and `end_date` query params (format `2006-01-02`) and set `filter.StartDate` / `filter.EndDate` to enable date-ranged filtering from the dashboard link.

---

## Non-Functional Requirements

### NFR1 — Always visible (no $0 hiding)
Uncategorized rows are always rendered even when all period amounts are $0. This gives a persistent visual reminder that uncategorized transactions may exist.

### NFR2 — Neutral visual treatment
The uncategorized row uses Bootstrap secondary grey (`#6c757d`), no icon, and muted label text to visually distinguish it from user-defined categories without competing for attention.

### NFR3 — No double-counting
Uncategorized amounts are added to `MonthlyTotals` and summary card totals exactly once. The filter `(category_id IS NULL OR category_source = 0)` is the authoritative definition of "uncategorized" throughout.

---

## Out of Scope

- "Uncategorized Investments" pseudo-category — no data signal exists to distinguish from uncategorized expenses
- New API endpoints — all changes are within the existing dashboard data flow
- Changing how existing categorized transactions are displayed
- Security, resiliency, and property-based testing extensions — all opted out
