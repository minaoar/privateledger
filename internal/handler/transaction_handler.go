package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

// TransactionHandler handles transaction-related HTTP requests
type TransactionHandler struct {
	repo *repository.TransactionRepository

	// sicMappingService is nil when mapping creation from the transaction
	// modals is not wired. Mapping writes go through the service, never the
	// mapping repository, so normalization, uniqueness, admission, backup and
	// recategorization behave the same as they do on the mapping page.
	sicMappingService *service.SICMappingService
}

// NewTransactionHandler creates a new TransactionHandler
func NewTransactionHandler(repo *repository.TransactionRepository) *TransactionHandler {
	return &TransactionHandler{repo: repo}
}

// NewTransactionHandlerWithSIC creates a handler that can also create SIC
// mappings from the transaction categorization modals.
func NewTransactionHandlerWithSIC(
	repo *repository.TransactionRepository,
	sicMappingService *service.SICMappingService,
) *TransactionHandler {
	return &TransactionHandler{repo: repo, sicMappingService: sicMappingService}
}

// CreateSICMappingRequest creates or updates the mapping for a transaction's
// SIC code. CategoryID is required: a modal is not a way to create an
// intentionally empty mapping, which belongs on the mapping page.
type CreateSICMappingRequest struct {
	CategoryID  *int   `json:"category_id"`
	Description string `json:"description"`
}

// CreateSICMappingForTransaction maps the transaction's SIC code to a category.
// POST /api/transactions/:id/sic-mapping
//
// This is an ordinary mapping change, so it also recategorizes other currently
// uncategorized transactions carrying the same code. Creating a mapping means
// the same thing wherever it is created.
func (h *TransactionHandler) CreateSICMappingForTransaction(c *gin.Context) {
	if h.sicMappingService == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": "SIC mapping creation is not available",
			"code":  "not_available",
		})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid transaction ID", "code": "invalid_request"})
		return
	}

	var req CreateSICMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload", "code": "invalid_request"})
		return
	}
	if req.CategoryID == nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "A category must be selected to create a SIC mapping",
			"code":  "category_required",
		})
		return
	}

	transaction, err := h.repo.GetByID(id)
	if err != nil {
		slog.Error("Error loading transaction for SIC mapping", slog.Int("transaction_id", id), slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load the transaction", "code": "internal_error"})
		return
	}
	if transaction == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found", "code": "not_found"})
		return
	}
	if transaction.SICCode == nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "This transaction has no SIC code to map",
			"code":  "no_sic_code",
		})
		return
	}

	input := model.SICMappingInput{
		SICCode:     string(*transaction.SICCode),
		Description: req.Description,
		CategoryID:  req.CategoryID,
	}

	// An existing mapping for this code is updated rather than rejected: from
	// the modal the user is stating what the code should mean, not asserting
	// that no mapping exists yet.
	existing, err := h.sicMappingService.FindMappingByCode(string(*transaction.SICCode))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check existing mappings", "code": "internal_error"})
		return
	}

	var result *model.SICMappingMutationResult
	if existing != nil {
		if input.Description == "" {
			input.Description = existing.Description
		}
		input.DescriptionDetail = existing.DescriptionDetail
		result, err = h.sicMappingService.UpdateMapping(c.Request.Context(), existing.SICMappingID, input)
	} else {
		result, err = h.sicMappingService.CreateMapping(c.Request.Context(), input)
	}
	if err != nil {
		status, code := classifySICMappingFailure(err)
		if code == "mapping_busy" {
			c.Header("Retry-After", "1")
		}
		c.JSON(status, gin.H{
			"error": sicMappingFailureMessage(err, code, "Failed to save the SIC mapping."),
			"code":  code,
		})
		return
	}

	// The mapping is now the rule that explains this code, so the current
	// transaction is restated as rule-sourced rather than left manual. Only the
	// source changes and only when the stored category already matches the
	// mapping, so this never alters a category the user chose. A transaction
	// that was uncategorized has already been handled by the mapping's own
	// scoped recategorization.
	// The mapping is already durable at this point, so a failure here is
	// reported as a post-commit warning rather than an error. Logging alone
	// would leave the caller believing the requested outcome was applied.
	stored, readErr := h.repo.GetByID(id)
	switch {
	case readErr != nil:
		slog.Error("Failed to re-read transaction after modal mapping",
			slog.Int("transaction_id", id), slog.String("error", readErr.Error()))
		result.PostCommitWarnings = append(result.PostCommitWarnings,
			"The mapping was saved, but this transaction could not be re-checked, so it may still be marked as a manual choice.")
	case stored != nil && stored.CategoryID != nil && *stored.CategoryID == *req.CategoryID &&
		stored.CategorySource != model.CategorySourceRule:
		if err := h.repo.UpdateCategory(id, req.CategoryID, model.CategorySourceRule); err != nil {
			slog.Error("Failed to apply rule source after modal mapping",
				slog.Int("transaction_id", id), slog.String("error", err.Error()))
			result.PostCommitWarnings = append(result.PostCommitWarnings,
				"The mapping was saved, but this transaction is still marked as a manual choice rather than following the new mapping.")
		}
	}

	c.JSON(http.StatusOK, result)
}

// ListTransactions returns transactions with optional filters
// GET /api/transactions?account_id=1&category_id=2&uncategorized=true&start_date=2025-01-01&end_date=2025-01-31&limit=100&offset=0
func (h *TransactionHandler) ListTransactions(c *gin.Context) {
	filter := repository.TransactionFilter{}

	// Parse account_id filter
	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		accountID, err := strconv.Atoi(accountIDStr)
		if err != nil {
			slog.Error("Invalid account_id in ListTransactions", slog.String("account_id", accountIDStr), slog.String("error", err.Error()))
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account_id"})
			return
		}
		filter.AccountID = &accountID
	}

	// Parse category_id filter
	if categoryIDStr := c.Query("category_id"); categoryIDStr != "" {
		categoryID, err := strconv.Atoi(categoryIDStr)
		if err != nil {
			slog.Error("Invalid category_id in ListTransactions", slog.String("category_id", categoryIDStr), slog.String("error", err.Error()))
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category_id"})
			return
		}
		filter.CategoryID = &categoryID
	}

	// Parse category_type filter
	if categoryTypeStr := c.Query("category_type"); categoryTypeStr != "" {
		categoryType, err := strconv.Atoi(categoryTypeStr)
		if err != nil {
			slog.Error("Invalid category_type in ListTransactions", slog.String("category_type", categoryTypeStr), slog.String("error", err.Error()))
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category_type"})
			return
		}
		ct := model.CategoryType(categoryType)
		filter.CategoryType = &ct
	}

	// Parse uncategorized filter
	if uncategorizedStr := c.Query("uncategorized"); uncategorizedStr == "true" {
		filter.Uncategorized = true
	}

	// Parse start_date filter
	if startDateStr := c.Query("start_date"); startDateStr != "" {
		startDate, err := time.Parse("2006-01-02", startDateStr)
		if err != nil {
			slog.Error("Invalid start_date in ListTransactions", slog.String("start_date", startDateStr), slog.String("error", err.Error()))
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid start_date format (use YYYY-MM-DD)"})
			return
		}
		filter.StartDate = &startDate
	}

	// Parse end_date filter
	if endDateStr := c.Query("end_date"); endDateStr != "" {
		endDate, err := time.Parse("2006-01-02", endDateStr)
		if err != nil {
			slog.Error("Invalid end_date in ListTransactions", slog.String("end_date", endDateStr), slog.String("error", err.Error()))
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid end_date format (use YYYY-MM-DD)"})
			return
		}
		// Set to end of day
		endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, endDate.Location())
		filter.EndDate = &endDate
	}

	// Parse limit
	if limitStr := c.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit"})
			return
		}
		filter.Limit = limit
	}

	// Parse offset
	if offsetStr := c.Query("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid offset"})
			return
		}
		filter.Offset = offset
	}

	transactions, err := h.repo.List(filter)
	if err != nil {
		slog.Error("Error listing transactions", slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	slog.Info("ListTransactions: returning transactions", slog.Int("total", len(transactions)))

	c.JSON(http.StatusOK, transactions)
}

// GetTransaction returns a single transaction by ID
// GET /api/transactions/:id
func (h *TransactionHandler) GetTransaction(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid transaction ID"})
		return
	}

	transaction, err := h.repo.GetByID(id)
	if err != nil {
		slog.Error("Error getting transaction by ID", slog.Int("transaction_id", id), slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if transaction == nil {
		slog.Warn("Transaction not found", slog.Int("transaction_id", id))
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		return
	}

	c.JSON(http.StatusOK, transaction)
}

// UpdateTransactionCategoryRequest represents the request body for updating a transaction's category
type UpdateTransactionCategoryRequest struct {
	CategoryID *int `json:"category_id"` // Nullable - null means uncategorize
}

// UpdateTransactionCategory updates a transaction's category
// PATCH /api/transactions/:id/category
func (h *TransactionHandler) UpdateTransactionCategory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid transaction ID"})
		return
	}

	var req UpdateTransactionCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("Invalid JSON in UpdateTransactionCategory", slog.Int("transaction_id", id), slog.String("error", err.Error()))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if transaction exists
	transaction, err := h.repo.GetByID(id)
	if err != nil {
		slog.Error("Error getting transaction by ID in UpdateTransactionCategory", slog.Int("transaction_id", id), slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if transaction == nil {
		slog.Warn("Transaction not found in UpdateTransactionCategory", slog.Int("transaction_id", id))
		c.JSON(http.StatusNotFound, gin.H{"error": "Transaction not found"})
		return
	}

	// Determine source: manual if changing, none if clearing
	source := model.CategorySourceManual
	if req.CategoryID == nil {
		source = model.CategorySourceNone
	}

	// Update category
	err = h.repo.UpdateCategory(id, req.CategoryID, source)
	if err != nil {
		slog.Error("Error updating transaction category", slog.Int("transaction_id", id), slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Fetch updated transaction
	transaction, err = h.repo.GetByID(id)
	if err != nil {
		slog.Error("Error fetching updated transaction", slog.Int("transaction_id", id), slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, transaction)
}

// DeleteTransaction deletes a transaction
// DELETE /api/transactions/:id
func (h *TransactionHandler) DeleteTransaction(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid transaction ID"})
		return
	}

	err = h.repo.Delete(id)
	if err != nil {
		slog.Error("Error deleting transaction", slog.Int("transaction_id", id), slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Transaction deleted successfully"})
}
