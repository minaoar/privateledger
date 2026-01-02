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

	summary := &MonthlySummary{
		Period:            *period,
		CategoryBreakdown: make([]CategoryBreakdown, 0),
	}

	// Calculate totals and category breakdown
	categoryMap := make(map[*int]*CategoryBreakdown) // Use pointer to int for nil support

	for _, txn := range transactions {
		summary.TransactionCount++

		// Check if uncategorized
		if txn.IsUncategorized() {
			summary.UncategorizedCount++
		}

		// Add to income or expenses
		if txn.TransactionType == model.TransactionTypeCredit {
			summary.TotalIncome += txn.Amount
		} else {
			summary.TotalExpenses += txn.Amount
		}

		// Group by category
		categoryKey := txn.CategoryID
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

	// Build category breakdown with names
	for categoryID, breakdown := range categoryMap {
		if categoryID != nil {
			if cat, exists := categoryLookup[*categoryID]; exists {
				breakdown.CategoryName = cat.Name
				breakdown.CategoryColor = cat.Color
			} else {
				breakdown.CategoryName = "Unknown"
			}
		} else {
			breakdown.CategoryName = "Uncategorized"
		}
		summary.CategoryBreakdown = append(summary.CategoryBreakdown, *breakdown)
	}

	return summary, nil
}

// TrendDataPoint represents a single point in a trend chart
type TrendDataPoint struct {
	Period        MonthPeriod `json:"period"`
	TotalIncome   float64     `json:"total_income"`
	TotalExpenses float64     `json:"total_expenses"`
	NetAmount     float64     `json:"net_amount"`
}

// GetTrends calculates financial trends over multiple months
// Returns data for the last N months (including current month)
func (s *InsightsService) GetTrends(months int) ([]TrendDataPoint, error) {
	if months < 1 {
		months = 6 // Default to 6 months
	}

	trends := make([]TrendDataPoint, 0, months)
	currentPeriod := s.GetCurrentMonthPeriod()

	// Parse current period label to get year/month
	var year, month int
	fmt.Sscanf(currentPeriod.Label, "%d-%d", &year, &month)

	// Go back N months and calculate each
	for i := months - 1; i >= 0; i-- {
		// Calculate month offset
		targetMonth := month - i
		targetYear := year

		// Handle year boundary
		for targetMonth < 1 {
			targetMonth += 12
			targetYear--
		}

		period := s.GetMonthPeriod(targetYear, targetMonth)

		// Get transactions for this period
		filter := repository.TransactionFilter{
			StartDate: &period.StartDate,
			EndDate:   &period.EndDate,
		}

		transactions, err := s.txnRepo.List(filter)
		if err != nil {
			slog.Error("Error getting transactions for trends", slog.String("period", period.Label), slog.String("error", err.Error()))
			return nil, fmt.Errorf("failed to get transactions for %s: %w", period.Label, err)
		}

		dataPoint := TrendDataPoint{
			Period: *period,
		}

		// Calculate totals
		for _, txn := range transactions {
			if txn.TransactionType == model.TransactionTypeCredit {
				dataPoint.TotalIncome += txn.Amount
			} else {
				dataPoint.TotalExpenses += txn.Amount
			}
		}

		dataPoint.NetAmount = dataPoint.TotalIncome - dataPoint.TotalExpenses
		trends = append(trends, dataPoint)
	}

	return trends, nil
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

// DashboardStats contains quick statistics for the dashboard
type DashboardStats struct {
	CurrentMonth       MonthlySummary      `json:"current_month"`
	ExpenseSummary     CategoryTypeSummary `json:"expense_summary"`
	IncomeSummary      CategoryTypeSummary `json:"income_summary"`
	InvestmentSummary  CategoryTypeSummary `json:"investment_summary"`
	AccountCount       int                 `json:"account_count"`
	CategoryCount      int                 `json:"category_count"`
	UncategorizedCount int                 `json:"uncategorized_count"`
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

	// Calculate change
	var changePercent float64
	var changeDirection string
	if previousTotal > 0 {
		changePercent = ((currentTotal - previousTotal) / previousTotal) * 100
	} else if currentTotal > 0 {
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

	return &CategoryTypeSummary{
		TotalAmount:      currentTotal,
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

	summary, err := s.GetMonthlySummary(year, month)
	if err != nil {
		slog.Error("Error getting monthly summary for dashboard", slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get monthly summary: %w", err)
	}

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

	return &DashboardStats{
		CurrentMonth:       *summary,
		ExpenseSummary:     *expenseSummary,
		IncomeSummary:      *incomeSummary,
		InvestmentSummary:  *investmentSummary,
		AccountCount:       accountCount,
		CategoryCount:      categoryCount,
		UncategorizedCount: uncategorizedCount,
	}, nil
}
