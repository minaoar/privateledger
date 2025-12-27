package handler

import (
	"embed"
	"html/template"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

// PageHandler handles serving HTML pages
type PageHandler struct {
	templates       *template.Template
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
	tmpl := template.Must(template.ParseFS(files, "web/templates/*.html"))

	return &PageHandler{
		templates:       tmpl,
		accountRepo:     accountRepo,
		transactionRepo: transactionRepo,
		categoryRepo:    categoryRepo,
		patternRepo:     patternRepo,
		insightsService: insightsService,
	}
}

// Dashboard renders the dashboard page
func (h *PageHandler) Dashboard(c *gin.Context) {
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
	h.templates.ExecuteTemplate(c.Writer, "layout.html", data)
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
	h.templates.ExecuteTemplate(c.Writer, "layout.html", data)
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
	h.templates.ExecuteTemplate(c.Writer, "layout.html", data)
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

	transactions, err := h.transactionRepo.List(filter)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error loading transactions: %v", err)
		return
	}

	// Get accounts for filter dropdown
	accounts, _ := h.accountRepo.GetAll()

	// Get categories for filter dropdown
	categories, _ := h.categoryRepo.GetAll()

	data := gin.H{
		"Title":        "Transactions",
		"ActivePage":   "transactions",
		"Transactions": transactions,
		"Accounts":     accounts,
		"Categories":   categories,
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	h.templates.ExecuteTemplate(c.Writer, "layout.html", data)
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
	h.templates.ExecuteTemplate(c.Writer, "layout.html", data)
}
