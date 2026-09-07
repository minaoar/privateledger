package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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

		// Check for conflicting patterns
		conflictingPattern := h.findConflictingPattern(patternName)
		if conflictingPattern != "" {
			c.JSON(http.StatusConflict, gin.H{"error": "Pattern '" + patternName + "' conflicts with \"" + conflictingPattern + "\""})
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

	// Creating a pattern is a rule change, so every transaction the rules
	// govern is re-examined, not only the uncategorized ones. Reexamine
	// reloads rules itself.
	if len(patterns) > 0 {
		if _, err := h.categorizer.Reexamine(); err != nil {
			slog.Error("Failed to re-examine after creating category patterns",
				slog.Int("category_id", category.CategoryID),
				slog.String("error", err.Error()))
		}
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

	// Deleting a category is a rule change twice over: the cascade removes its
	// patterns, and its SIC mappings are left with no category so they assign
	// nothing. Re-examination runs after that cascade has completed, never
	// before, so it evaluates against the rules that remain (BR-U5-09).
	//
	// The ClearCategory above is still what detaches this category's own
	// transactions; re-examination then decides where the remaining rules put
	// them, which may be a different category rather than nowhere.
	result, err := h.categorizer.Reexamine()
	if err != nil {
		slog.Error("Failed to re-examine after deleting a category",
			slog.Int("category_id", id),
			slog.String("error", err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "Category deleted successfully"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Category deleted successfully",
		"result":  result,
	})
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

	// Check for conflicting patterns
	conflictingPattern := h.findConflictingPattern(req.PatternName)
	if conflictingPattern != "" {
		c.JSON(http.StatusConflict, gin.H{"error": "Pattern conflicts with \"" + conflictingPattern + "\""})
		return
	}

	// Create pattern
	pattern := model.NewCategoryPattern(req.PatternName, categoryID)
	err = h.patternRepo.Create(pattern)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Adding a pattern is a rule change. Reexamine reloads rules itself.
	if _, err := h.categorizer.Reexamine(); err != nil {
		slog.Error("Failed to re-examine after adding a pattern",
			slog.Int("category_id", categoryID),
			slog.String("error", err.Error()))
	}

	c.JSON(http.StatusCreated, pattern)
}

// findConflictingPattern checks if the new pattern conflicts with any existing pattern
// Returns the conflicting pattern name, or empty string if no conflict
func (h *CategoryHandler) findConflictingPattern(newPattern string) string {
	patterns, err := h.patternRepo.GetAll()
	if err != nil {
		return ""
	}

	newLower := strings.ToLower(newPattern)
	for _, p := range patterns {
		existingLower := strings.ToLower(p.PatternName)
		// Check if new contains existing or existing contains new
		if strings.Contains(newLower, existingLower) || strings.Contains(existingLower, newLower) {
			return p.PatternName
		}
	}
	return ""
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

	// Deleting a pattern is a rule change, so transactions it had categorized
	// are re-examined and follow whatever the remaining rules say — or become
	// uncategorized if nothing claims them. Reexamine reloads rules itself,
	// synchronously; this was once a detached goroutine that raced with
	// in-flight reads of the rule cache.
	if _, err := h.categorizer.Reexamine(); err != nil {
		slog.Error("Failed to re-examine after deleting a pattern",
			slog.String("error", err.Error()))
	}

	c.JSON(http.StatusOK, gin.H{"message": "Pattern deleted successfully"})
}

// RecategorizeAll re-examines every transaction against the current rules.
// POST /api/categories/recategorize
//
// The route and handler name are unchanged so the existing page keeps working;
// what changed is the scope, which now includes transactions a rule already
// categorized.
func (h *CategoryHandler) RecategorizeAll(c *gin.Context) {
	result, err := h.categorizer.Reexamine()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
