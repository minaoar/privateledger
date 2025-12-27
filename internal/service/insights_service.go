package service

import (
	"fmt"
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

// CategoryBreakdown represents spending breakdown by category
type CategoryBreakdown struct {
	CategoryID   *int    `json:"category_id"`
	CategoryName string  `json:"category_name"`
	CategoryColor *string `json:"category_color"`
	TotalAmount  float64 `json:"total_amount"`
	Count        int     `json:"count"`
}

// MonthlySummary contains aggregated financial data for a month
type MonthlySummary struct {
	Period            MonthPeriod          `json:"period"`
	TotalIncome       float64              `json:"total_income"`
	TotalExpenses     float64              `json:"total_expenses"`
	NetAmount         float64              `json:"net_amount"`
	TransactionCount  int                  `json:"transaction_count"`
	UncategorizedCount int                 `json:"uncategorized_count"`
	CategoryBreakdown []CategoryBreakdown  `json:"category_breakdown"`
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

// DashboardStats contains quick statistics for the dashboard
type DashboardStats struct {
	CurrentMonth       MonthlySummary `json:"current_month"`
	AccountCount       int            `json:"account_count"`
	CategoryCount      int            `json:"category_count"`
	UncategorizedCount int            `json:"uncategorized_count"`
}

// GetDashboardStats returns statistics for the dashboard
func (s *InsightsService) GetDashboardStats(accountRepo *repository.AccountRepository) (*DashboardStats, error) {
	// Get current month summary
	currentPeriod := s.GetCurrentMonthPeriod()
	var year, month int
	fmt.Sscanf(currentPeriod.Label, "%d-%d", &year, &month)

	summary, err := s.GetMonthlySummary(year, month)
	if err != nil {
		return nil, fmt.Errorf("failed to get monthly summary: %w", err)
	}

	// Get account count
	accountCount, err := accountRepo.Count()
	if err != nil {
		return nil, fmt.Errorf("failed to get account count: %w", err)
	}

	// Get category count
	categoryCount, err := s.categoryRepo.Count()
	if err != nil {
		return nil, fmt.Errorf("failed to get category count: %w", err)
	}

	// Get total uncategorized count (all time)
	uncategorizedCount, err := s.txnRepo.CountUncategorized()
	if err != nil {
		return nil, fmt.Errorf("failed to get uncategorized count: %w", err)
	}

	return &DashboardStats{
		CurrentMonth:       *summary,
		AccountCount:       accountCount,
		CategoryCount:      categoryCount,
		UncategorizedCount: uncategorizedCount,
	}, nil
}
