package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

// TransactionHandler handles transaction-related HTTP requests
type TransactionHandler struct {
	repo *repository.TransactionRepository
}

// NewTransactionHandler creates a new TransactionHandler
func NewTransactionHandler(repo *repository.TransactionRepository) *TransactionHandler {
	return &TransactionHandler{repo: repo}
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
