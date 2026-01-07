package service

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/oronno/privateledger/internal/config"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

// InsightsService handles financial insights and aggregations
type InsightsService struct {
	txnRepo      *repository.TransactionRepository
	categoryRepo *repository.CategoryRepository
	config       *config.Config
}

// NewInsightsService creates a new InsightsService
func NewInsightsService(
	txnRepo *repository.TransactionRepository,
	categoryRepo *repository.CategoryRepository,
	cfg *config.Config,
) *InsightsService {
	return &InsightsService{
		txnRepo:      txnRepo,
		categoryRepo: categoryRepo,
		config:       cfg,
	}
}

// MonthPeriod represents a custom month period based on config.start_of_month
type MonthPeriod struct {
	Label     string    `json:"label"`      // e.g., "2025-01"
	StartDate time.Time `json:"start_date"` // Inclusive
	EndDate   time.Time `json:"end_date"`   // Inclusive
}

// GetMonthPeriod calculates the start and end dates for a given month label
// Month label format: "YYYY-MM" (e.g., "2025-01")
func (s *InsightsService) GetMonthPeriod(year int, month int) *MonthPeriod {
	startDay := s.config.StartOfMonth

	// Calculate start date: year-month-startDay
	startDate := time.Date(year, time.Month(month), startDay, 0, 0, 0, 0, time.UTC)

	// Calculate end date: next month's (startDay - 1)
	endDate := startDate.AddDate(0, 1, -1)

	// Set end date to end of day
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, time.UTC)

	label := fmt.Sprintf("%04d-%02d", year, month)

	return &MonthPeriod{
		Label:     label,
		StartDate: startDate,
		EndDate:   endDate,
	}
}

// GetCurrentMonthPeriod returns the current month period based on today's date
func (s *InsightsService) GetCurrentMonthPeriod() *MonthPeriod {
	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	// If we're before the start_of_month day, we're still in the previous month
	if now.Day() < s.config.StartOfMonth {
		month--
		if month < 1 {
			month = 12
			year--
		}
	}

	return s.GetMonthPeriod(year, month)
}

// GetPeriodForDate returns the month period that contains the given date
func (s *InsightsService) GetPeriodForDate(date time.Time) *MonthPeriod {
	year := date.Year()
	month := int(date.Month())

	// If we're before the start_of_month day, we're in the previous month's period
	if date.Day() < s.config.StartOfMonth {
		month--
		if month < 1 {
			month = 12
			year--
		}
	}

	return s.GetMonthPeriod(year, month)
}

// CategoryBreakdown represents spending breakdown by category
type CategoryBreakdown struct {
	CategoryID    *int    `json:"category_id"`
	CategoryName  string  `json:"category_name"`
	CategoryColor *string `json:"category_color"`
	TotalAmount   float64 `json:"total_amount"`
	Count         int     `json:"count"`
}

// MonthlySummary contains aggregated financial data for a month
type MonthlySummary struct {
	Period             MonthPeriod         `json:"period"`
	TotalIncome        float64             `json:"total_income"`
	TotalExpenses      float64             `json:"total_expenses"`
	NetAmount          float64             `json:"net_amount"`
	TransactionCount   int                 `json:"transaction_count"`
	UncategorizedCount int                 `json:"uncategorized_count"`
	CategoryBreakdown  []CategoryBreakdown `json:"category_breakdown"`
}

// GetMonthlySummary calculates financial summary for a specific month
func (s *InsightsService) GetMonthlySummary(year int, month int) (*MonthlySummary, error) {
	period := s.GetMonthPeriod(year, month)

	slog.Info("GetMonthlySummary called",
		slog.Int("year", year),
		slog.Int("month", month),
		slog.String("period_start", period.StartDate.Format("2006-01-02")),
		slog.String("period_end", period.EndDate.Format("2006-01-02")))

	// Get transactions for this period
	filter := repository.TransactionFilter{
		StartDate: &period.StartDate,
		EndDate:   &period.EndDate,
	}

	transactions, err := s.txnRepo.List(filter)
	if err != nil {
		slog.Error("Error getting transactions for monthly summary", slog.Int("year", year), slog.Int("month", month), slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}

	slog.Info("Transactions retrieved for monthly summary", slog.Int("count", len(transactions)))

	summary := &MonthlySummary{
		Period:            *period,
		CategoryBreakdown: make([]CategoryBreakdown, 0),
	}

	// Calculate totals and category breakdown
	categoryMap := make(map[int]*CategoryBreakdown)
	categorizedCount := 0
	uncategorizedCount := 0

	for _, txn := range transactions {
		summary.TransactionCount++

		// Check if uncategorized
		if txn.IsUncategorized() {
			summary.UncategorizedCount++
			uncategorizedCount++
		} else {
			categorizedCount++
		}

		// Add to income or expenses
		if txn.TransactionType == model.TransactionTypeCredit {
			summary.TotalIncome += txn.Amount
		} else {
			summary.TotalExpenses += txn.Amount
		}

		if txn.CategoryID != nil {
			categoryKey := *txn.CategoryID
			if breakdown, exists := categoryMap[categoryKey]; exists {
				breakdown.TotalAmount += txn.Amount
				breakdown.Count++
			} else {
				categoryMap[categoryKey] = &CategoryBreakdown{
					CategoryID:  txn.CategoryID,
					TotalAmount: txn.Amount,
					Count:       1,
				}
			}
		}
	}

	slog.Info("Transaction processing complete",
		slog.Int("total_transactions", summary.TransactionCount),
		slog.Int("categorized", categorizedCount),
		slog.Int("uncategorized", uncategorizedCount),
		slog.Int("unique_categories", len(categoryMap)))

	// Calculate net amount
	summary.NetAmount = summary.TotalIncome - summary.TotalExpenses

	// Load category names and colors
	categories, err := s.categoryRepo.GetAll()
	if err != nil {
		slog.Error("Error getting categories for monthly summary", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get categories: %w", err)
	}

	categoryLookup := make(map[int]*model.Category)
	for _, cat := range categories {
		categoryLookup[cat.CategoryID] = cat
	}

	// Build category breakdown with names (only Expense categories for pie chart)
	expenseBreakdowns := make([]CategoryBreakdown, 0)
	for categoryID, breakdown := range categoryMap {
		// Only include categorized transactions in the breakdown
		if cat, exists := categoryLookup[categoryID]; exists {
			// Only include Expense categories (category_type = 2)
			if cat.CategoryType == model.CategoryTypeExpense {
				breakdown.CategoryName = cat.Name
				breakdown.CategoryColor = cat.Color
				expenseBreakdowns = append(expenseBreakdowns, *breakdown)
			}
		} else {
			slog.Warn("Category ID not found in lookup",
				slog.Int("category_id", categoryID))
		}
	}

	// Sort by absolute amount (descending) and limit to top 5
	// Sort expense breakdowns by absolute total amount
	for i := 0; i < len(expenseBreakdowns); i++ {
		for j := i + 1; j < len(expenseBreakdowns); j++ {
			absI := expenseBreakdowns[i].TotalAmount
			if absI < 0 {
				absI = -absI
			}
			absJ := expenseBreakdowns[j].TotalAmount
			if absJ < 0 {
				absJ = -absJ
			}
			if absJ > absI {
				expenseBreakdowns[i], expenseBreakdowns[j] = expenseBreakdowns[j], expenseBreakdowns[i]
			}
		}
	}

	// Limit to top 5, combine rest as "Others"
	if len(expenseBreakdowns) > 5 {
		top5 := expenseBreakdowns[:5]
		others := expenseBreakdowns[5:]

		// Sum up the "Others"
		var othersTotal float64
		var othersCount int
		for _, other := range others {
			othersTotal += other.TotalAmount
			othersCount += other.Count
		}

		// Add "Others" category
		othersBreakdown := CategoryBreakdown{
			CategoryID:    nil,
			CategoryName:  "Others",
			CategoryColor: nil,
			TotalAmount:   othersTotal,
			Count:         othersCount,
		}

		summary.CategoryBreakdown = append(top5, othersBreakdown)
		slog.Info("Combined remaining categories as 'Others'",
			slog.Int("others_count", len(others)),
			slog.Float64("others_total", othersTotal))
	} else {
		summary.CategoryBreakdown = expenseBreakdowns
	}

	slog.Info("Category breakdown built", "breakdown_count", len(summary.CategoryBreakdown), "CategoryBreakdown", summary.CategoryBreakdown)

	return summary, nil
}

// CategoryTypeSummary contains financial summary for a specific category type
type CategoryTypeSummary struct {
	TotalAmount      float64 `json:"total_amount"`
	PreviousAmount   float64 `json:"previous_amount"`
	ChangePercent    float64 `json:"change_percent"`
	ChangeDirection  string  `json:"change_direction"` // "up", "down", "same"
	CurrentPeriod    string  `json:"current_period"`   // e.g., "Dec 19 - Jan 18"
	TransactionCount int     `json:"transaction_count"`
}

// ExpenseCategoryByMonth represents a category's expenses across multiple months
type ExpenseCategoryByMonth struct {
	CategoryID    int                `json:"category_id"`
	CategoryName  string             `json:"category_name"`
	CategoryIcon  *string            `json:"category_icon"`
	CategoryColor *string            `json:"category_color"`
	MonthlyTotals map[string]float64 `json:"monthly_totals"` // key is period label like "2026-01"
}

// ExpenseBreakdownTable contains expense breakdown by category across months
type ExpenseBreakdownTable struct {
	Periods       []MonthPeriod            `json:"periods"`        // Last 6 months
	Categories    []ExpenseCategoryByMonth `json:"categories"`     // Expense categories
	MonthlyTotals map[string]float64       `json:"monthly_totals"` // Column totals by period
}

// DashboardStats contains quick statistics for the dashboard
type DashboardStats struct {
	CurrentMonth       MonthlySummary        `json:"current_month"`
	ExpenseSummary     CategoryTypeSummary   `json:"expense_summary"`
	IncomeSummary      CategoryTypeSummary   `json:"income_summary"`
	InvestmentSummary  CategoryTypeSummary   `json:"investment_summary"`
	ExpenseBreakdown   ExpenseBreakdownTable `json:"expense_breakdown"`
	AccountCount       int                   `json:"account_count"`
	CategoryCount      int                   `json:"category_count"`
	UncategorizedCount int                   `json:"uncategorized_count"`
}

// GetExpenseBreakdownTable builds a table of expense categories across the last N months
// Starting from the specified year/month and going back
func (s *InsightsService) GetExpenseBreakdownTable(year, month, months int) (*ExpenseBreakdownTable, error) {
	if months < 1 {
		months = 6 // Default to 6 months
	}

	slog.Info("GetExpenseBreakdownTable called",
		slog.Int("year", year),
		slog.Int("month", month),
		slog.Int("months", months))

	// Build list of periods (last N months)
	periods := make([]MonthPeriod, 0, months)
	for i := months - 1; i >= 0; i-- {
		targetMonth := month - i
		targetYear := year
		for targetMonth < 1 {
			targetMonth += 12
			targetYear--
		}
		period := s.GetMonthPeriod(targetYear, targetMonth)
		periods = append(periods, *period)
	}

	// Get all expense categories
	allCategories, err := s.categoryRepo.GetAll()
	if err != nil {
		slog.Error("Error getting categories for expense breakdown", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get categories: %w", err)
	}

	expenseCategories := make([]*model.Category, 0)
	for _, cat := range allCategories {
		if cat.CategoryType == model.CategoryTypeExpense {
			expenseCategories = append(expenseCategories, cat)
		}
	}

	// Build category breakdown for each month
	categoryBreakdown := make([]ExpenseCategoryByMonth, 0)
	monthlyTotals := make(map[string]float64)

	for _, cat := range expenseCategories {
		categoryMonthly := ExpenseCategoryByMonth{
			CategoryID:    cat.CategoryID,
			CategoryName:  cat.Name,
			CategoryIcon:  cat.Icon,
			CategoryColor: cat.Color,
			MonthlyTotals: make(map[string]float64),
		}

		// For each period, get transactions for this category
		for _, period := range periods {
			filter := repository.TransactionFilter{
				CategoryID: &cat.CategoryID,
				StartDate:  &period.StartDate,
				EndDate:    &period.EndDate,
			}

			transactions, err := s.txnRepo.List(filter)
			if err != nil {
				slog.Error("Error getting transactions for expense breakdown",
					slog.String("category", cat.Name),
					slog.String("period", period.Label),
					slog.String("error", err.Error()))
				continue
			}

			// Sum up expenses for this category in this period
			var total float64
			for _, txn := range transactions {
				total += txn.Amount
			}

			categoryMonthly.MonthlyTotals[period.Label] = total
			monthlyTotals[period.Label] += total
		}

		categoryBreakdown = append(categoryBreakdown, categoryMonthly)
	}

	slog.Info("Expense breakdown table built",
		slog.Int("categories", len(categoryBreakdown)),
		slog.Int("periods", len(periods)))

	return &ExpenseBreakdownTable{
		Periods:       periods,
		Categories:    categoryBreakdown,
		MonthlyTotals: monthlyTotals,
	}, nil
}

// getCategoryTypeSummary calculates summary for a specific category type (Expense/Income/Investment)
func (s *InsightsService) getCategoryTypeSummary(categoryType model.CategoryType, year, month int) (*CategoryTypeSummary, error) {
	// Get current month period
	currentPeriod := s.GetMonthPeriod(year, month)

	// Get previous month
	prevMonth := month - 1
	prevYear := year
	if prevMonth < 1 {
		prevMonth = 12
		prevYear--
	}
	previousPeriod := s.GetMonthPeriod(prevYear, prevMonth)

	// Get all categories of this type
	allCategories, err := s.categoryRepo.GetAll()
	if err != nil {
		return nil, err
	}

	categoryIDs := make([]int, 0)
	for _, cat := range allCategories {
		if cat.CategoryType == categoryType {
			categoryIDs = append(categoryIDs, cat.CategoryID)
		}
	}

	// Get current month transactions
	currentFilter := repository.TransactionFilter{
		StartDate: &currentPeriod.StartDate,
		EndDate:   &currentPeriod.EndDate,
	}
	currentTxns, err := s.txnRepo.List(currentFilter)
	if err != nil {
		return nil, err
	}

	// Get previous month transactions
	previousFilter := repository.TransactionFilter{
		StartDate: &previousPeriod.StartDate,
		EndDate:   &previousPeriod.EndDate,
	}
	previousTxns, err := s.txnRepo.List(previousFilter)
	if err != nil {
		return nil, err
	}

	// Calculate totals
	var currentTotal float64
	var currentCount int
	for _, txn := range currentTxns {
		if txn.CategoryID != nil {
			for _, catID := range categoryIDs {
				if *txn.CategoryID == catID {
					currentTotal += txn.Amount
					currentCount++
					break
				}
			}
		}
	}

	var previousTotal float64
	for _, txn := range previousTxns {
		if txn.CategoryID != nil {
			for _, catID := range categoryIDs {
				if *txn.CategoryID == catID {
					previousTotal += txn.Amount
					break
				}
			}
		}
	}

	// Calculate change using absolute values (expenses/investments are negative)
	absPrevious := previousTotal
	if absPrevious < 0 {
		absPrevious = -absPrevious
	}
	absCurrent := currentTotal
	if absCurrent < 0 {
		absCurrent = -absCurrent
	}

	var changePercent float64
	var changeDirection string
	if absPrevious > 0 {
		changePercent = ((absCurrent - absPrevious) / absPrevious) * 100
	} else if absCurrent > 0 {
		changePercent = 100
	}

	if changePercent > 0.01 {
		changeDirection = "up"
	} else if changePercent < -0.01 {
		changeDirection = "down"
	} else {
		changeDirection = "same"
	}

	// Format period string
	periodStr := fmt.Sprintf("%s - %s",
		currentPeriod.StartDate.Format("Jan 2"),
		currentPeriod.EndDate.Format("Jan 2"))

	// For display purposes, show absolute value of total (investments/expenses are negative but should display positive)
	displayTotal := currentTotal
	if displayTotal < 0 {
		displayTotal = -displayTotal
	}

	slog.Info("Category type summary calculated",
		slog.String("category_type", fmt.Sprintf("%d", categoryType)),
		slog.Float64("current_total", currentTotal),
		slog.Float64("previous_total", previousTotal),
		slog.Float64("display_total", displayTotal),
		slog.Float64("change_percent", changePercent),
		slog.String("change_direction", changeDirection))

	return &CategoryTypeSummary{
		TotalAmount:      displayTotal,
		PreviousAmount:   previousTotal,
		ChangePercent:    changePercent,
		ChangeDirection:  changeDirection,
		CurrentPeriod:    periodStr,
		TransactionCount: currentCount,
	}, nil
}

// GetDashboardStats returns statistics for the dashboard
func (s *InsightsService) GetDashboardStats(accountRepo *repository.AccountRepository) (*DashboardStats, error) {
	// Get the most recent transaction date to determine which month to show
	mostRecentDateStr, err := s.txnRepo.GetMostRecentTransactionDate()
	if err != nil {
		slog.Error("Error getting most recent transaction date", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get most recent transaction date: %w", err)
	}
	slog.Info("GetMostRecentTransactionDate returned", slog.String("mostRecentDate", mostRecentDateStr))

	// Determine which period to show based on last transaction
	var currentPeriod *MonthPeriod
	if mostRecentDateStr == "" {
		// No transactions yet, use current month
		currentPeriod = s.GetCurrentMonthPeriod()
	} else {
		// Parse the most recent transaction date
		mostRecentDate, err := time.Parse(time.RFC3339, mostRecentDateStr)
		if err != nil {
			// Try date-only format
			mostRecentDate, err = time.Parse(time.DateTime, mostRecentDateStr)
			if err != nil {
				slog.Error("Error parsing most recent transaction date", slog.String("error", err.Error()))
				// Fall back to current month
				currentPeriod = s.GetCurrentMonthPeriod()
			} else {
				currentPeriod = s.GetPeriodForDate(mostRecentDate)
			}
		} else {
			currentPeriod = s.GetPeriodForDate(mostRecentDate)
		}
	}

	var year, month int
	fmt.Sscanf(currentPeriod.Label, "%d-%d", &year, &month)

	slog.Info("Dashboard showing period",
		slog.Int("year", year),
		slog.Int("month", month),
		slog.String("period_label", currentPeriod.Label))

	summary, err := s.GetMonthlySummary(year, month)
	if err != nil {
		slog.Error("Error getting monthly summary for dashboard", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get monthly summary: %w", err)
	}

	slog.Info("Dashboard monthly summary retrieved",
		slog.Int("transaction_count", summary.TransactionCount),
		slog.Int("uncategorized_count", summary.UncategorizedCount),
		slog.Int("category_breakdown_count", len(summary.CategoryBreakdown)),
		slog.Float64("total_income", summary.TotalIncome),
		slog.Float64("total_expenses", summary.TotalExpenses))

	// Get category type summaries
	expenseSummary, err := s.getCategoryTypeSummary(model.CategoryTypeExpense, year, month)
	if err != nil {
		slog.Error("Error getting expense summary", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get expense summary: %w", err)
	}

	incomeSummary, err := s.getCategoryTypeSummary(model.CategoryTypeIncome, year, month)
	if err != nil {
		slog.Error("Error getting income summary", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get income summary: %w", err)
	}

	investmentSummary, err := s.getCategoryTypeSummary(model.CategoryTypeInvestment, year, month)
	slog.Info("investmentSummary by getCategoryTypeSummary", "investmentSummary", investmentSummary)
	if err != nil {
		slog.Error("Error getting investment summary", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get investment summary: %w", err)
	}

	// Get account count
	accountCount, err := accountRepo.Count()
	if err != nil {
		slog.Error("Error getting account count for dashboard", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get account count: %w", err)
	}

	// Get category count
	categoryCount, err := s.categoryRepo.Count()
	if err != nil {
		slog.Error("Error getting category count for dashboard", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get category count: %w", err)
	}

	// Get total uncategorized count (all time)
	uncategorizedCount, err := s.txnRepo.CountUncategorized()
	if err != nil {
		slog.Error("Error getting uncategorized count for dashboard", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get uncategorized count: %w", err)
	}

	// Get expense breakdown table (last 6 months from the same period as dashboard)
	expenseBreakdown, err := s.GetExpenseBreakdownTable(year, month, 6)
	if err != nil {
		slog.Error("Error getting expense breakdown for dashboard", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get expense breakdown: %w", err)
	}

	return &DashboardStats{
		CurrentMonth:       *summary,
		ExpenseSummary:     *expenseSummary,
		IncomeSummary:      *incomeSummary,
		InvestmentSummary:  *investmentSummary,
		ExpenseBreakdown:   *expenseBreakdown,
		AccountCount:       accountCount,
		CategoryCount:      categoryCount,
		UncategorizedCount: uncategorizedCount,
	}, nil
}
