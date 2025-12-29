package handler

import (
	"embed"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

// PageHandler handles serving HTML pages
type PageHandler struct {
	files           embed.FS
	accountRepo     *repository.AccountRepository
	transactionRepo *repository.TransactionRepository
	categoryRepo    *repository.CategoryRepository
	patternRepo     *repository.CategoryPatternRepository
	insightsService *service.InsightsService
}

// NewPageHandler creates a new PageHandler
func NewPageHandler(
	files embed.FS,
	accountRepo *repository.AccountRepository,
	transactionRepo *repository.TransactionRepository,
	categoryRepo *repository.CategoryRepository,
	patternRepo *repository.CategoryPatternRepository,
	insightsService *service.InsightsService,
) *PageHandler {
	return &PageHandler{
		files:           files,
		accountRepo:     accountRepo,
		transactionRepo: transactionRepo,
		categoryRepo:    categoryRepo,
		patternRepo:     patternRepo,
		insightsService: insightsService,
	}
}

// parseTemplate parses layout.html with a specific page template
func (h *PageHandler) parseTemplate(pageName string) *template.Template {
	return template.Must(template.ParseFS(h.files,
		"web/templates/layout.html",
		"web/templates/"+pageName+".html",
	))
}

// Dashboard renders the dashboard page
func (h *PageHandler) Dashboard(c *gin.Context) {
	// Check if onboarding is needed
	categories, err := h.categoryRepo.GetAll()
	if err == nil && len(categories) == 0 {
		c.Redirect(http.StatusFound, "/onboarding")
		return
	}

	stats, err := h.insightsService.GetDashboardStats(h.accountRepo)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error loading dashboard: %v", err)
		return
	}

	data := gin.H{
		"Title":      "Dashboard",
		"ActivePage": "dashboard",
		"Stats":      stats,
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	h.parseTemplate("dashboard").ExecuteTemplate(c.Writer, "layout.html", data)
}

// Accounts renders the accounts management page
func (h *PageHandler) Accounts(c *gin.Context) {
	accounts, err := h.accountRepo.GetAll()
	if err != nil {
		c.String(http.StatusInternalServerError, "Error loading accounts: %v", err)
		return
	}

	data := gin.H{
		"Title":      "Accounts",
		"ActivePage": "accounts",
		"Accounts":   accounts,
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	h.parseTemplate("accounts").ExecuteTemplate(c.Writer, "layout.html", data)
}

// Categories renders the categories management page
func (h *PageHandler) Categories(c *gin.Context) {
	categories, err := h.categoryRepo.GetAllWithPatterns(h.patternRepo)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error loading categories: %v", err)
		return
	}

	data := gin.H{
		"Title":      "Categories",
		"ActivePage": "categories",
		"Categories": categories,
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	h.parseTemplate("categories").ExecuteTemplate(c.Writer, "layout.html", data)
}

// Transactions renders the transactions list page
func (h *PageHandler) Transactions(c *gin.Context) {
	filter := repository.TransactionFilter{
		Limit: 100, // Default limit
	}

	// Parse filters from query params
	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		if accountID, err := strconv.Atoi(accountIDStr); err == nil {
			filter.AccountID = &accountID
		}
	}

	if categoryIDStr := c.Query("category_id"); categoryIDStr != "" {
		if categoryID, err := strconv.Atoi(categoryIDStr); err == nil {
			filter.CategoryID = &categoryID
		}
	}

	if c.Query("uncategorized") == "true" {
		filter.Uncategorized = true
	}

	// Parse date filters
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		if startDate, err := time.Parse("2006-01-02", startDateStr); err == nil {
			filter.StartDate = &startDate
		}
	}

	if endDateStr := c.Query("end_date"); endDateStr != "" {
		if endDate, err := time.Parse("2006-01-02", endDateStr); err == nil {
			// Set to end of day to include all transactions on the end date
			endDate = endDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			filter.EndDate = &endDate
		}
	}

	transactions, err := h.transactionRepo.List(filter)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error loading transactions: %v", err)
		return
	}

	// Get accounts for filter dropdown
	accounts, _ := h.accountRepo.GetAll()

	// Get categories for filter dropdown
	categories, _ := h.categoryRepo.GetAll()

	// Prepare filter values for template
	filterValues := gin.H{
		"AccountID":     c.Query("account_id"),
		"CategoryID":    c.Query("category_id"),
		"Uncategorized": c.Query("uncategorized"),
		"StartDate":     c.Query("start_date"),
		"EndDate":       c.Query("end_date"),
	}

	data := gin.H{
		"Title":        "Transactions",
		"ActivePage":   "transactions",
		"Transactions": transactions,
		"Accounts":     accounts,
		"Categories":   categories,
		"Filter":       filterValues,
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	h.parseTemplate("transactions").ExecuteTemplate(c.Writer, "layout.html", data)
}

// Import renders the OFX import page
func (h *PageHandler) Import(c *gin.Context) {
	accounts, err := h.accountRepo.GetAll()
	if err != nil {
		c.String(http.StatusInternalServerError, "Error loading accounts: %v", err)
		return
	}

	data := gin.H{
		"Title":      "Import",
		"ActivePage": "import",
		"Accounts":   accounts,
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	h.parseTemplate("import").ExecuteTemplate(c.Writer, "layout.html", data)
}

// Onboarding renders the onboarding wizard page
func (h *PageHandler) Onboarding(c *gin.Context) {
	// If categories already exist, redirect to dashboard
	categories, err := h.categoryRepo.GetAll()
	if err == nil && len(categories) > 0 {
		c.Redirect(http.StatusFound, "/")
		return
	}

	data := gin.H{
		"Title": "Welcome to PrivateLedger",
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	tmpl := template.Must(template.ParseFS(h.files, "web/templates/onboarding.html"))
	tmpl.Execute(c.Writer, data)
}
