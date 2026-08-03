package service

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/oronno/privateledger/internal/config"
	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

// Tests for surfacing uncategorized transactions on the dashboard.
//
// The production database happens to contain no categorized income and no
// investment transactions at all, so it cannot exercise those code paths.
// These fixtures deliberately cover them.

const (
	testYear  = 2026
	testMonth = 6
)

// newTestService builds an InsightsService backed by a real (temporary) SQLite
// database, migrated with the production schema.
func newTestService(t *testing.T) (*InsightsService, *sql.DB) {
	t.Helper()

	db, err := database.Open(database.Config{Path: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`INSERT INTO account (account_id, name) VALUES (1, 'Test Account')`); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	cfg := config.DefaultConfig() // StartOfMonth = 1, so periods are calendar months
	svc := NewInsightsService(
		repository.NewTransactionRepository(db),
		repository.NewCategoryRepository(db),
		cfg,
	)
	return svc, db
}

func addCategory(t *testing.T, db *sql.DB, id int, name string, ct model.CategoryType) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO category (category_id, name, category_type) VALUES (?, ?, ?)`,
		id, name, ct,
	); err != nil {
		t.Fatalf("seed category %q: %v", name, err)
	}
}

// addTxn inserts a transaction. Pass categoryID nil for an uncategorized row.
func addTxn(t *testing.T, db *sql.DB, fitID string, day int, amount float64,
	txnType model.TransactionType, categoryID *int, source model.CategorySource) {
	t.Helper()

	date := time.Date(testYear, time.Month(testMonth), day, 0, 0, 0, 0, time.UTC)
	if _, err := db.Exec(
		`INSERT INTO ledger_transaction
			(account_id, trn_type, fit_id, date_posted, amount, transaction_details, transaction_type, category_id, category_source)
		 VALUES (1, 'DEBIT', ?, ?, ?, 'test', ?, ?, ?)`,
		fitID, date, amount, txnType, categoryID, source,
	); err != nil {
		t.Fatalf("seed transaction %q: %v", fitID, err)
	}
}

func intPtr(i int) *int { return &i }

// findRow returns the breakdown row with the given name.
func findRow(t *testing.T, table *CategoryBreakdownTable, name string) CategoryByMonth {
	t.Helper()
	for _, c := range table.Categories {
		if c.CategoryName == name {
			return c
		}
	}
	t.Fatalf("row %q not found; have %v", name, rowNames(table))
	return CategoryByMonth{}
}

func rowNames(table *CategoryBreakdownTable) []string {
	names := make([]string, 0, len(table.Categories))
	for _, c := range table.Categories {
		names = append(names, c.CategoryName)
	}
	return names
}

func hasRow(table *CategoryBreakdownTable, name string) bool {
	for _, c := range table.Categories {
		if c.CategoryName == name {
			return true
		}
	}
	return false
}

// currentLabel returns the period label for the month under test.
func currentLabel(svc *InsightsService) string {
	return svc.GetMonthPeriod(testYear, testMonth).Label
}

func TestUncategorizedRowFor(t *testing.T) {
	tests := []struct {
		name       string
		input      model.CategoryType
		wantName   string
		wantType   model.TransactionType
		wantExists bool
	}{
		{"expense gets a debit row", model.CategoryTypeExpense, "Uncategorized Expenses", model.TransactionTypeDebit, true},
		{"income gets a credit row", model.CategoryTypeIncome, "Uncategorized Incomes", model.TransactionTypeCredit, true},
		{"investment gets no row", model.CategoryTypeInvestment, "", 0, false},
		{"general gets no row", model.CategoryTypeGeneral, "", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotType, gotExists := uncategorizedRowFor(tc.input)
			if gotExists != tc.wantExists || gotName != tc.wantName || gotType != tc.wantType {
				t.Errorf("uncategorizedRowFor(%v) = (%q, %v, %v), want (%q, %v, %v)",
					tc.input, gotName, gotType, gotExists, tc.wantName, tc.wantType, tc.wantExists)
			}
		})
	}
}

func TestIsUncategorizedForType(t *testing.T) {
	tests := []struct {
		name    string
		catType model.CategoryType
		txnType model.TransactionType
		want    bool
	}{
		{"debit counts toward expense", model.CategoryTypeExpense, model.TransactionTypeDebit, true},
		{"credit does not count toward expense", model.CategoryTypeExpense, model.TransactionTypeCredit, false},
		{"credit counts toward income", model.CategoryTypeIncome, model.TransactionTypeCredit, true},
		{"debit does not count toward income", model.CategoryTypeIncome, model.TransactionTypeDebit, false},
		{"investment never counts (debit)", model.CategoryTypeInvestment, model.TransactionTypeDebit, false},
		{"investment never counts (credit)", model.CategoryTypeInvestment, model.TransactionTypeCredit, false},
		{"general never counts", model.CategoryTypeGeneral, model.TransactionTypeDebit, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUncategorizedForType(tc.catType, tc.txnType); got != tc.want {
				t.Errorf("isUncategorizedForType(%v, %v) = %v, want %v",
					tc.catType, tc.txnType, got, tc.want)
			}
		})
	}
}

// Expense table collects uncategorized debits and leaves credits alone.
func TestExpenseBreakdown_UncategorizedRow(t *testing.T) {
	svc, db := newTestService(t)
	addCategory(t, db, 1, "Groceries", model.CategoryTypeExpense)

	addTxn(t, db, "cat-1", 5, -100, model.TransactionTypeDebit, intPtr(1), model.CategorySourceRule)
	addTxn(t, db, "unc-1", 6, -40, model.TransactionTypeDebit, nil, model.CategorySourceNone)
	addTxn(t, db, "unc-2", 7, -60, model.TransactionTypeDebit, nil, model.CategorySourceNone)
	addTxn(t, db, "unc-credit", 8, 500, model.TransactionTypeCredit, nil, model.CategorySourceNone)

	table, err := svc.GetExpenseBreakdownTable(testYear, testMonth, 1)
	if err != nil {
		t.Fatalf("GetExpenseBreakdownTable: %v", err)
	}
	label := currentLabel(svc)

	// -40 + -60; the uncategorized credit must not leak in.
	if got := findRow(t, table, "Uncategorized Expenses").MonthlyTotals[label]; got != -100 {
		t.Errorf("uncategorized expenses = %v, want -100", got)
	}
	if got := findRow(t, table, "Groceries").MonthlyTotals[label]; got != -100 {
		t.Errorf("Groceries = %v, want -100", got)
	}
	// Column total feeds the bar chart, so it must include the uncategorized row.
	if got := table.MonthlyTotals[label]; got != -200 {
		t.Errorf("column total = %v, want -200", got)
	}
}

// Income table collects uncategorized credits. The production database has no
// categorized income at all, so this path is only covered here.
func TestIncomeBreakdown_UncategorizedRow(t *testing.T) {
	svc, db := newTestService(t)
	addCategory(t, db, 1, "Salary", model.CategoryTypeIncome)

	addTxn(t, db, "cat-1", 5, 3000, model.TransactionTypeCredit, intPtr(1), model.CategorySourceManual)
	addTxn(t, db, "unc-1", 6, 250, model.TransactionTypeCredit, nil, model.CategorySourceNone)
	addTxn(t, db, "unc-debit", 7, -80, model.TransactionTypeDebit, nil, model.CategorySourceNone)

	table, err := svc.GetCategoryBreakdownTable(model.CategoryTypeIncome, testYear, testMonth, 1)
	if err != nil {
		t.Fatalf("GetCategoryBreakdownTable: %v", err)
	}
	label := currentLabel(svc)

	if got := findRow(t, table, "Uncategorized Incomes").MonthlyTotals[label]; got != 250 {
		t.Errorf("uncategorized incomes = %v, want 250 (uncategorized debit must not leak in)", got)
	}
	if got := table.MonthlyTotals[label]; got != 3250 {
		t.Errorf("column total = %v, want 3250", got)
	}
}

// Investment and General tables must not grow an uncategorized row, even when
// uncategorized transactions exist in the period.
func TestBreakdown_NoUncategorizedRowForInvestmentOrGeneral(t *testing.T) {
	svc, db := newTestService(t)
	addCategory(t, db, 1, "Brokerage", model.CategoryTypeInvestment)
	addCategory(t, db, 2, "Misc", model.CategoryTypeGeneral)

	addTxn(t, db, "inv-1", 5, -500, model.TransactionTypeDebit, intPtr(1), model.CategorySourceManual)
	addTxn(t, db, "unc-debit", 6, -40, model.TransactionTypeDebit, nil, model.CategorySourceNone)
	addTxn(t, db, "unc-credit", 7, 90, model.TransactionTypeCredit, nil, model.CategorySourceNone)

	for _, ct := range []model.CategoryType{model.CategoryTypeInvestment, model.CategoryTypeGeneral} {
		table, err := svc.GetCategoryBreakdownTable(ct, testYear, testMonth, 1)
		if err != nil {
			t.Fatalf("GetCategoryBreakdownTable(%v): %v", ct, err)
		}
		for _, name := range []string{"Uncategorized Expenses", "Uncategorized Incomes"} {
			if hasRow(table, name) {
				t.Errorf("category type %v unexpectedly has row %q; rows: %v", ct, name, rowNames(table))
			}
		}
	}
}

// The uncategorized row is present even when every period is zero, so the table
// keeps a stable shape month to month.
func TestExpenseBreakdown_UncategorizedRowPresentWhenEmpty(t *testing.T) {
	svc, db := newTestService(t)
	addCategory(t, db, 1, "Groceries", model.CategoryTypeExpense)
	addTxn(t, db, "cat-1", 5, -100, model.TransactionTypeDebit, intPtr(1), model.CategorySourceRule)

	table, err := svc.GetExpenseBreakdownTable(testYear, testMonth, 1)
	if err != nil {
		t.Fatalf("GetExpenseBreakdownTable: %v", err)
	}

	row := findRow(t, table, "Uncategorized Expenses")
	if got := row.MonthlyTotals[currentLabel(svc)]; got != 0 {
		t.Errorf("uncategorized total = %v, want 0", got)
	}
	if row.CategoryID != uncategorizedCategoryID {
		t.Errorf("CategoryID = %d, want sentinel %d", row.CategoryID, uncategorizedCategoryID)
	}
}

// Regression guard for the invariant that motivated the CategoryIsNull filter:
// a transaction must never be counted in both a category row and the
// uncategorized row. A row with a category_id but category_source = 0 is the
// case the broader Uncategorized filter would double-count.
func TestBreakdown_NoDoubleCounting(t *testing.T) {
	svc, db := newTestService(t)
	addCategory(t, db, 1, "Groceries", model.CategoryTypeExpense)

	addTxn(t, db, "cat-rule", 5, -100, model.TransactionTypeDebit, intPtr(1), model.CategorySourceRule)
	addTxn(t, db, "cat-src0", 6, -25, model.TransactionTypeDebit, intPtr(1), model.CategorySourceNone)
	addTxn(t, db, "unc-1", 7, -40, model.TransactionTypeDebit, nil, model.CategorySourceNone)

	table, err := svc.GetExpenseBreakdownTable(testYear, testMonth, 1)
	if err != nil {
		t.Fatalf("GetExpenseBreakdownTable: %v", err)
	}
	label := currentLabel(svc)

	var sum float64
	for _, c := range table.Categories {
		sum += c.MonthlyTotals[label]
	}
	if got := table.MonthlyTotals[label]; sum != got {
		t.Errorf("sum of rows = %v but column total = %v (double-counting)", sum, got)
	}
	// The category_source=0 row belongs to Groceries, not to uncategorized.
	if got := findRow(t, table, "Uncategorized Expenses").MonthlyTotals[label]; got != -40 {
		t.Errorf("uncategorized = %v, want -40 (category_source=0 row must stay with its category)", got)
	}
	if got := sum; got != -165 {
		t.Errorf("total across rows = %v, want -165", got)
	}
}

// Summary cards fold uncategorized amounts into both the current and the
// previous period, so TotalAmount, PreviousAmount and ChangePercent stay
// consistent with each other.
func TestSummaryCards_IncludeUncategorized(t *testing.T) {
	svc, db := newTestService(t)
	addCategory(t, db, 1, "Groceries", model.CategoryTypeExpense)
	addCategory(t, db, 2, "Salary", model.CategoryTypeIncome)
	addCategory(t, db, 3, "Brokerage", model.CategoryTypeInvestment)

	// Current period.
	addTxn(t, db, "e-cat", 5, -100, model.TransactionTypeDebit, intPtr(1), model.CategorySourceRule)
	addTxn(t, db, "e-unc", 6, -50, model.TransactionTypeDebit, nil, model.CategorySourceNone)
	addTxn(t, db, "i-cat", 7, 1000, model.TransactionTypeCredit, intPtr(2), model.CategorySourceManual)
	addTxn(t, db, "i-unc", 8, 200, model.TransactionTypeCredit, nil, model.CategorySourceNone)
	addTxn(t, db, "v-cat", 9, -300, model.TransactionTypeDebit, intPtr(3), model.CategorySourceManual)

	expense, err := svc.getCategoryTypeSummary(model.CategoryTypeExpense, testYear, testMonth)
	if err != nil {
		t.Fatalf("expense summary: %v", err)
	}
	if expense.TotalAmount != 150 { // |-100 + -50|
		t.Errorf("expense TotalAmount = %v, want 150", expense.TotalAmount)
	}
	if expense.TransactionCount != 2 {
		t.Errorf("expense TransactionCount = %d, want 2 (uncategorized counted)", expense.TransactionCount)
	}

	income, err := svc.getCategoryTypeSummary(model.CategoryTypeIncome, testYear, testMonth)
	if err != nil {
		t.Fatalf("income summary: %v", err)
	}
	if income.TotalAmount != 1200 { // 1000 + 200
		t.Errorf("income TotalAmount = %v, want 1200", income.TotalAmount)
	}

	// Investment must ignore uncategorized entirely. The production database has
	// no investment transactions, so this assertion only holds here.
	investment, err := svc.getCategoryTypeSummary(model.CategoryTypeInvestment, testYear, testMonth)
	if err != nil {
		t.Fatalf("investment summary: %v", err)
	}
	if investment.TotalAmount != 300 {
		t.Errorf("investment TotalAmount = %v, want 300 (uncategorized excluded)", investment.TotalAmount)
	}
	if investment.TransactionCount != 1 {
		t.Errorf("investment TransactionCount = %d, want 1", investment.TransactionCount)
	}
}

// PreviousAmount must include uncategorized too. Folding uncategorized only into
// the absolute values would leave PreviousAmount reporting the categorized-only
// figure while TotalAmount reported the combined one.
func TestSummaryCards_PreviousAmountIncludesUncategorized(t *testing.T) {
	svc, db := newTestService(t)
	addCategory(t, db, 1, "Groceries", model.CategoryTypeExpense)

	// Previous period (May): -100 categorized, -100 uncategorized => -200.
	prev := time.Date(testYear, time.Month(testMonth-1), 10, 0, 0, 0, 0, time.UTC)
	for _, tx := range []struct {
		fitID  string
		amount float64
		cat    *int
		src    model.CategorySource
	}{
		{"p-cat", -100, intPtr(1), model.CategorySourceRule},
		{"p-unc", -100, nil, model.CategorySourceNone},
	} {
		if _, err := db.Exec(
			`INSERT INTO ledger_transaction
				(account_id, trn_type, fit_id, date_posted, amount, transaction_details, transaction_type, category_id, category_source)
			 VALUES (1, 'DEBIT', ?, ?, ?, 'test', 1, ?, ?)`,
			tx.fitID, prev, tx.amount, tx.cat, tx.src,
		); err != nil {
			t.Fatalf("seed previous-period transaction: %v", err)
		}
	}

	// Current period (June): -300 total.
	addTxn(t, db, "c-cat", 5, -200, model.TransactionTypeDebit, intPtr(1), model.CategorySourceRule)
	addTxn(t, db, "c-unc", 6, -100, model.TransactionTypeDebit, nil, model.CategorySourceNone)

	summary, err := svc.getCategoryTypeSummary(model.CategoryTypeExpense, testYear, testMonth)
	if err != nil {
		t.Fatalf("expense summary: %v", err)
	}

	if summary.TotalAmount != 300 {
		t.Errorf("TotalAmount = %v, want 300", summary.TotalAmount)
	}
	if summary.PreviousAmount != -200 {
		t.Errorf("PreviousAmount = %v, want -200 (must include uncategorized)", summary.PreviousAmount)
	}
	// 300 vs 200 => +50%. Reading the categorized-only previous (-100) would give +200%.
	if summary.ChangePercent < 49.99 || summary.ChangePercent > 50.01 {
		t.Errorf("ChangePercent = %v, want 50", summary.ChangePercent)
	}
	if summary.ChangeDirection != "up" {
		t.Errorf("ChangeDirection = %q, want \"up\"", summary.ChangeDirection)
	}
}

// The pie chart gains an uncategorized slice for debits, with a color distinct
// from both the "Others" grey and the uncolored-category fallback.
func TestMonthlySummary_PieChartUncategorizedSlice(t *testing.T) {
	svc, db := newTestService(t)
	addCategory(t, db, 1, "Groceries", model.CategoryTypeExpense)

	addTxn(t, db, "cat-1", 5, -100, model.TransactionTypeDebit, intPtr(1), model.CategorySourceRule)
	addTxn(t, db, "unc-1", 6, -75, model.TransactionTypeDebit, nil, model.CategorySourceNone)
	addTxn(t, db, "unc-credit", 7, 400, model.TransactionTypeCredit, nil, model.CategorySourceNone)

	summary, err := svc.GetMonthlySummary(testYear, testMonth)
	if err != nil {
		t.Fatalf("GetMonthlySummary: %v", err)
	}

	var slice *CategoryBreakdown
	for i := range summary.CategoryBreakdown {
		if summary.CategoryBreakdown[i].CategoryName == "Uncategorized" {
			slice = &summary.CategoryBreakdown[i]
		}
	}
	if slice == nil {
		t.Fatal("no Uncategorized slice in pie chart")
	}

	if slice.TotalAmount != -75 {
		t.Errorf("slice total = %v, want -75 (credits excluded)", slice.TotalAmount)
	}
	if slice.Count != 1 {
		t.Errorf("slice count = %d, want 1", slice.Count)
	}
	if slice.CategoryColor == nil {
		t.Fatal("slice has no color")
	}
	for _, reserved := range []string{"#9ca3af", "#6b7280"} { // Others, uncolored fallback
		if *slice.CategoryColor == reserved {
			t.Errorf("slice color %q collides with reserved color %q", *slice.CategoryColor, reserved)
		}
	}
}

// A zero-value slice carries no information on a pie chart, so it is omitted --
// unlike the breakdown tables, where a stable row shape matters.
func TestMonthlySummary_NoPieSliceWhenZero(t *testing.T) {
	svc, db := newTestService(t)
	addCategory(t, db, 1, "Groceries", model.CategoryTypeExpense)
	addTxn(t, db, "cat-1", 5, -100, model.TransactionTypeDebit, intPtr(1), model.CategorySourceRule)

	summary, err := svc.GetMonthlySummary(testYear, testMonth)
	if err != nil {
		t.Fatalf("GetMonthlySummary: %v", err)
	}

	for _, c := range summary.CategoryBreakdown {
		if c.CategoryName == "Uncategorized" {
			t.Error("pie chart has an Uncategorized slice with no uncategorized spending")
		}
	}
}

// GetExpenseBreakdownTable delegates to GetCategoryBreakdownTable; the two must
// stay interchangeable for expenses.
func TestExpenseBreakdownTable_MatchesGenericForExpenses(t *testing.T) {
	svc, db := newTestService(t)
	addCategory(t, db, 1, "Groceries", model.CategoryTypeExpense)
	addCategory(t, db, 2, "Salary", model.CategoryTypeIncome)

	addTxn(t, db, "e-cat", 5, -100, model.TransactionTypeDebit, intPtr(1), model.CategorySourceRule)
	addTxn(t, db, "e-unc", 6, -40, model.TransactionTypeDebit, nil, model.CategorySourceNone)
	addTxn(t, db, "i-cat", 7, 900, model.TransactionTypeCredit, intPtr(2), model.CategorySourceManual)

	viaAlias, err := svc.GetExpenseBreakdownTable(testYear, testMonth, 3)
	if err != nil {
		t.Fatalf("GetExpenseBreakdownTable: %v", err)
	}
	viaGeneric, err := svc.GetCategoryBreakdownTable(model.CategoryTypeExpense, testYear, testMonth, 3)
	if err != nil {
		t.Fatalf("GetCategoryBreakdownTable: %v", err)
	}

	if len(viaAlias.Categories) != len(viaGeneric.Categories) {
		t.Fatalf("row count %d != %d", len(viaAlias.Categories), len(viaGeneric.Categories))
	}
	for i := range viaAlias.Categories {
		a, g := viaAlias.Categories[i], viaGeneric.Categories[i]
		if a.CategoryID != g.CategoryID || a.CategoryName != g.CategoryName {
			t.Errorf("row %d: %v != %v", i, a, g)
		}
		for label, want := range g.MonthlyTotals {
			if a.MonthlyTotals[label] != want {
				t.Errorf("row %q period %q: %v != %v", a.CategoryName, label, a.MonthlyTotals[label], want)
			}
		}
	}
	for label, want := range viaGeneric.MonthlyTotals {
		if viaAlias.MonthlyTotals[label] != want {
			t.Errorf("column total %q: %v != %v", label, viaAlias.MonthlyTotals[label], want)
		}
	}
}

// The repository filters underpinning the feature.
func TestTransactionFilter_CategoryIsNullAndTransactionType(t *testing.T) {
	_, db := newTestService(t)
	addCategory(t, db, 1, "Groceries", model.CategoryTypeExpense)

	addTxn(t, db, "cat-rule", 5, -100, model.TransactionTypeDebit, intPtr(1), model.CategorySourceRule)
	addTxn(t, db, "cat-src0", 6, -25, model.TransactionTypeDebit, intPtr(1), model.CategorySourceNone)
	addTxn(t, db, "unc-debit", 7, -40, model.TransactionTypeDebit, nil, model.CategorySourceNone)
	addTxn(t, db, "unc-credit", 8, 90, model.TransactionTypeCredit, nil, model.CategorySourceNone)

	repo := repository.NewTransactionRepository(db)
	debit, credit := model.TransactionTypeDebit, model.TransactionTypeCredit

	tests := []struct {
		name   string
		filter repository.TransactionFilter
		want   int
	}{
		{"category_id IS NULL only", repository.TransactionFilter{CategoryIsNull: true}, 2},
		{"null and debit", repository.TransactionFilter{CategoryIsNull: true, TransactionType: &debit}, 1},
		{"null and credit", repository.TransactionFilter{CategoryIsNull: true, TransactionType: &credit}, 1},
		{"debit only", repository.TransactionFilter{TransactionType: &debit}, 3},
		// Uncategorized is deliberately broader: it also matches the row that has
		// a category_id but category_source = 0.
		{"Uncategorized is broader than CategoryIsNull", repository.TransactionFilter{Uncategorized: true}, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.List(tc.filter)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != tc.want {
				t.Errorf("got %d transactions, want %d", len(got), tc.want)
			}
		})
	}
}
