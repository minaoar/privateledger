# Requirements Clarification Questions
# Feature: Show Uncategorized Transactions on Dashboard

Please answer each question by filling in the letter choice after the `[Answer]:` tag.
If none of the options match your needs, choose the last option (Other/X) and describe your preference.

---

## Question 1
There is a key data constraint: transactions only record whether money went **out (debit)** or **in (credit)**. There is no signal in the data to distinguish an "uncategorized expense" from an "uncategorized investment" — both are debits. How should we classify uncategorized transactions?

A) Two pseudo-categories only: **"Uncategorized Expenses"** (all uncategorized debits) and **"Uncategorized Incomes"** (all uncategorized credits). Skip "Uncategorized Investments" entirely since it cannot be derived.

B) All three pseudo-categories: **"Uncategorized Expenses"** (debits), **"Uncategorized Incomes"** (credits), and **"Uncategorized Investments"** (always $0 — a visible placeholder until user categorizes those transactions).

C) One combined pseudo-category per section: show a single **"Uncategorized"** row in each of the three breakdown tables (Expense, Income, Investment), all showing the same total uncategorized amount for that period.

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## Question 2
Where on the dashboard should the uncategorized pseudo-categories be visible?

A) Only in the **breakdown tables** (as extra rows in Expense Breakdown, Income Breakdown, Investment Breakdown).

B) In the **breakdown tables AND the "Top Expense Categories" pie chart** (uncategorized expenses appear as a slice in the pie chart).

C) In the **breakdown tables AND the "Expense Trends" bar chart** (uncategorized expenses added as a stacked bar segment).

D) In all three: breakdown tables, pie chart, AND trends bar chart.

X) Other (please describe after [Answer]: tag below)

[Answer]: D

---

## Question 3
Currently the **Expenses / Income / Investment summary cards** at the top of the dashboard only count categorized transactions. Should uncategorized amounts be included in those totals?

A) Yes — add uncategorized debit amounts into the Expenses card and uncategorized credit amounts into the Income card total (complete financial picture).

B) No — keep the summary cards showing only categorized amounts (current behaviour, no change to cards).

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## Question 4
The breakdown tables show the last 6 months of data. Should the uncategorized rows be clickable links (navigating to the filtered Transactions page for that period)?

A) Yes — clicking an uncategorized amount navigates to `/transactions?uncategorized=true&start_date=...&end_date=...`

B) No — show the amounts as plain text (not clickable).

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## Question 5: Security Extension
Should security extension rules be enforced for this feature?

A) Yes — enforce all SECURITY rules as blocking constraints (recommended for production-grade applications)

B) No — skip all SECURITY rules (suitable for PoCs, prototypes, and experimental projects)

X) Other (please describe after [Answer]: tag below)

[Answer]: Skip for now

---

## Question 6: Resiliency Extension
Should the resiliency baseline be applied to this feature?

A) Yes — apply the resiliency baseline as directional best practices

B) No — skip the resiliency baseline

X) Other (please describe after [Answer]: tag below)

[Answer]: Skip for now

---

## Question 7: Property-Based Testing Extension
Should property-based testing rules be enforced for this feature?

A) Yes — enforce all PBT rules as blocking constraints

B) Partial — enforce PBT rules only for pure functions and data transformations

C) No — skip all PBT rules

X) Other (please describe after [Answer]: tag below)

[Answer]: Skip for now
