package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

// InsightsHandler handles insights and analytics-related HTTP requests
type InsightsHandler struct {
	insightsService *service.InsightsService
	accountRepo     *repository.AccountRepository
}

// NewInsightsHandler creates a new InsightsHandler
func NewInsightsHandler(
	insightsService *service.InsightsService,
	accountRepo *repository.AccountRepository,
) *InsightsHandler {
	return &InsightsHandler{
		insightsService: insightsService,
		accountRepo:     accountRepo,
	}
}

// GetDashboard returns dashboard statistics
// GET /api/insights/dashboard
func (h *InsightsHandler) GetDashboard(c *gin.Context) {
	stats, err := h.insightsService.GetDashboardStats(h.accountRepo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetMonthlySummary returns financial summary for a specific month
// GET /api/insights/monthly?year=2025&month=1
func (h *InsightsHandler) GetMonthlySummary(c *gin.Context) {
	// Parse year
	yearStr := c.Query("year")
	if yearStr == "" {
		// Default to current month
		period := h.insightsService.GetCurrentMonthPeriod()
		var year, month int
		fmt.Sscanf(period.Label, "%d-%d", &year, &month)
		summary, err := h.insightsService.GetMonthlySummary(year, month)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, summary)
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid year"})
		return
	}

	// Parse month
	monthStr := c.Query("month")
	if monthStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "month is required when year is specified"})
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid month (must be 1-12)"})
		return
	}

	summary, err := h.insightsService.GetMonthlySummary(year, month)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// GetTrends returns financial trends over multiple months
// GET /api/insights/trends?months=6
func (h *InsightsHandler) GetTrends(c *gin.Context) {
	// Parse months parameter (default: 6)
	months := 6
	if monthsStr := c.Query("months"); monthsStr != "" {
		m, err := strconv.Atoi(monthsStr)
		if err != nil || m < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid months parameter"})
			return
		}
		months = m
	}

	trends, err := h.insightsService.GetTrends(months)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, trends)
}

// GetCurrentPeriod returns the current month period based on config
// GET /api/insights/current-period
func (h *InsightsHandler) GetCurrentPeriod(c *gin.Context) {
	period := h.insightsService.GetCurrentMonthPeriod()
	c.JSON(http.StatusOK, period)
}
