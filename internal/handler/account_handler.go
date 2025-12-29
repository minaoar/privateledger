package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

// AccountHandler handles account-related HTTP requests
type AccountHandler struct {
	repo *repository.AccountRepository
}

// NewAccountHandler creates a new AccountHandler
func NewAccountHandler(repo *repository.AccountRepository) *AccountHandler {
	return &AccountHandler{repo: repo}
}

// ListAccounts returns all accounts
// GET /api/accounts
func (h *AccountHandler) ListAccounts(c *gin.Context) {
	accounts, err := h.repo.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, accounts)
}

// GetAccount returns a single account by ID
// GET /api/accounts/:id
func (h *AccountHandler) GetAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	account, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if account == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	c.JSON(http.StatusOK, account)
}

// CreateAccountRequest represents the request body for creating account(s)
type CreateAccountRequest struct {
	Names []string `json:"names" binding:"required"`
}

// CreateAccount creates one or more accounts
// POST /api/accounts
func (h *AccountHandler) CreateAccount(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Filter out empty names
	var validNames []string
	for _, name := range req.Names {
		if name != "" {
			validNames = append(validNames, name)
		}
	}

	if len(validNames) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one account name is required"})
		return
	}

	createdAccounts := make([]*model.Account, 0)
	errors := make([]string, 0)

	for _, name := range validNames {
		// Check if account with same name already exists
		existing, err := h.repo.GetByName(name)
		if err != nil {
			errors = append(errors, "Error checking account '"+name+"': "+err.Error())
			continue
		}
		if existing != nil {
			errors = append(errors, "Account '"+name+"' already exists")
			continue
		}

		account := model.NewAccount(name)
		err = h.repo.Create(account)
		if err != nil {
			errors = append(errors, "Error creating account '"+name+"': "+err.Error())
			continue
		}

		createdAccounts = append(createdAccounts, account)
	}

	response := gin.H{
		"created": createdAccounts,
		"count":   len(createdAccounts),
	}

	if len(errors) > 0 {
		response["errors"] = errors
	}

	// Return 201 if at least one account was created, otherwise 400
	if len(createdAccounts) > 0 {
		c.JSON(http.StatusCreated, response)
	} else {
		c.JSON(http.StatusBadRequest, response)
	}
}

// UpdateAccountRequest represents the request body for updating an account
type UpdateAccountRequest struct {
	Name string `json:"name" binding:"required"`
}

// UpdateAccount updates an existing account
// PUT /api/accounts/:id
func (h *AccountHandler) UpdateAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if account exists
	account, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if account == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	// Check if new name conflicts with another account
	existing, err := h.repo.GetByName(req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil && existing.AccountID != id {
		c.JSON(http.StatusConflict, gin.H{"error": "Account with this name already exists"})
		return
	}

	account.Name = req.Name
	err = h.repo.Update(account)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, account)
}

// DeleteAccount deletes an account
// DELETE /api/accounts/:id
func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID"})
		return
	}

	err = h.repo.Delete(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Account deleted successfully"})
}
