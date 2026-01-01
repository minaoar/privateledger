package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

// CategoryHandler handles category and pattern-related HTTP requests
type CategoryHandler struct {
	categoryRepo *repository.CategoryRepository
	patternRepo  *repository.CategoryPatternRepository
	categorizer  *service.Categorizer
}

// NewCategoryHandler creates a new CategoryHandler
func NewCategoryHandler(
	categoryRepo *repository.CategoryRepository,
	patternRepo *repository.CategoryPatternRepository,
	categorizer *service.Categorizer,
) *CategoryHandler {
	return &CategoryHandler{
		categoryRepo: categoryRepo,
		patternRepo:  patternRepo,
		categorizer:  categorizer,
	}
}

// ListCategories returns all categories with their patterns
// GET /api/categories
func (h *CategoryHandler) ListCategories(c *gin.Context) {
	categories, err := h.categoryRepo.GetAllWithPatterns(h.patternRepo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, categories)
}

// GetCategory returns a single category by ID with patterns
// GET /api/categories/:id
func (h *CategoryHandler) GetCategory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	category, err := h.categoryRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if category == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	// Get patterns for this category
	patterns, err := h.patternRepo.GetByCategoryID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result := &model.CategoryWithPatterns{
		Category: *category,
		Patterns: make([]model.CategoryPattern, 0, len(patterns)),
	}
	for _, p := range patterns {
		result.Patterns = append(result.Patterns, *p)
	}

	c.JSON(http.StatusOK, result)
}

// CreateCategoryRequest represents the request body for creating a category
type CreateCategoryRequest struct {
	Name         string   `json:"name" binding:"required"`
	CategoryType int      `json:"category_type"` // 1=General, 2=Expense, 3=Income, 4=Investment
	Color        *string  `json:"color"`
	Icon         *string  `json:"icon"`     // Bootstrap icon name (optional)
	Patterns     []string `json:"patterns"` // Initial patterns (optional)
}

// CreateCategory creates a new category with optional initial patterns
// POST /api/categories
func (h *CategoryHandler) CreateCategory(c *gin.Context) {
	var req CreateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if category with same name already exists
	existing, err := h.categoryRepo.GetByName(req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Category with this name already exists"})
		return
	}

	// Default to General (1) if category_type is not provided
	categoryType := model.CategoryType(req.CategoryType)
	if req.CategoryType == 0 {
		categoryType = model.CategoryTypeGeneral
	}

	// Create category
	category := model.NewCategory(req.Name, categoryType, req.Color, req.Icon)
	err = h.categoryRepo.Create(category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create initial patterns if provided
	patterns := make([]model.CategoryPattern, 0)
	for _, patternName := range req.Patterns {
		if patternName == "" {
			continue
		}

		// Check if pattern already exists
		existingPattern, err := h.patternRepo.GetByPatternName(patternName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if existingPattern != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Pattern '" + patternName + "' already exists"})
			return
		}

		pattern := model.NewCategoryPattern(patternName, category.CategoryID)
		err = h.patternRepo.Create(pattern)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		patterns = append(patterns, *pattern)
	}

	// Trigger re-categorization if patterns were added
	if len(patterns) > 0 {
		go func() {
			h.categorizer.LoadPatterns()
			h.categorizer.RecategorizeByCategory(category.CategoryID)
		}()
	}

	result := &model.CategoryWithPatterns{
		Category: *category,
		Patterns: patterns,
	}

	c.JSON(http.StatusCreated, result)
}

// UpdateCategoryRequest represents the request body for updating a category
type UpdateCategoryRequest struct {
	Name         string  `json:"name" binding:"required"`
	CategoryType int     `json:"category_type"` // 1=General, 2=Expense, 3=Income, 4=Investment
	Color        *string `json:"color"`
	Icon         *string `json:"icon"` // Bootstrap icon name (optional)
}

// UpdateCategory updates an existing category
// PUT /api/categories/:id
func (h *CategoryHandler) UpdateCategory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	var req UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if category exists
	category, err := h.categoryRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if category == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	// Check if new name conflicts with another category
	existing, err := h.categoryRepo.GetByName(req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil && existing.CategoryID != id {
		c.JSON(http.StatusConflict, gin.H{"error": "Category with this name already exists"})
		return
	}

	// Default to General (1) if category_type is not provided
	categoryType := model.CategoryType(req.CategoryType)
	if req.CategoryType == 0 {
		categoryType = model.CategoryTypeGeneral
	}

	category.Name = req.Name
	category.CategoryType = categoryType
	category.Color = req.Color
	category.Icon = req.Icon
	err = h.categoryRepo.Update(category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, category)
}

// DeleteCategory deletes a category and clears it from transactions
// DELETE /api/categories/:id
func (h *CategoryHandler) DeleteCategory(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	// Clear category from transactions (sets to uncategorized)
	err = h.categorizer.ClearCategory(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Delete category (cascades to patterns)
	err = h.categoryRepo.Delete(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category deleted successfully"})
}

// AddPatternRequest represents the request body for adding a pattern to a category
type AddPatternRequest struct {
	PatternName string `json:"pattern_name" binding:"required"`
}

// AddPattern adds a new pattern to a category
// POST /api/categories/:id/patterns
func (h *CategoryHandler) AddPattern(c *gin.Context) {
	categoryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category ID"})
		return
	}

	var req AddPatternRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if category exists
	category, err := h.categoryRepo.GetByID(categoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if category == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
		return
	}

	// Check if pattern already exists
	existingPattern, err := h.patternRepo.GetByPatternName(req.PatternName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existingPattern != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Pattern already exists"})
		return
	}

	// Create pattern
	pattern := model.NewCategoryPattern(req.PatternName, categoryID)
	err = h.patternRepo.Create(pattern)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Trigger re-categorization for this category
	go func() {
		h.categorizer.LoadPatterns()
		h.categorizer.RecategorizeByCategory(categoryID)
	}()

	c.JSON(http.StatusCreated, pattern)
}

// DeletePattern deletes a pattern
// DELETE /api/patterns/:id
func (h *CategoryHandler) DeletePattern(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pattern ID"})
		return
	}

	err = h.patternRepo.Delete(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Reload patterns for categorizer
	go h.categorizer.LoadPatterns()

	c.JSON(http.StatusOK, gin.H{"message": "Pattern deleted successfully"})
}

// RecategorizeAll triggers re-categorization of all uncategorized transactions
// POST /api/categories/recategorize
func (h *CategoryHandler) RecategorizeAll(c *gin.Context) {
	result, err := h.categorizer.RecategorizeAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
