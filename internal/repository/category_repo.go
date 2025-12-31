package repository

import (
	"database/sql"
	"fmt"

	"github.com/oronno/privateledger/internal/model"
)

// CategoryRepository handles database operations for categories
type CategoryRepository struct {
	db *sql.DB
}

// NewCategoryRepository creates a new CategoryRepository
func NewCategoryRepository(db *sql.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

// Create inserts a new category into the database
func (r *CategoryRepository) Create(category *model.Category) error {
	query := `INSERT INTO category (name, category_type, color, icon) VALUES (?, ?, ?, ?)`
	result, err := r.db.Exec(query, category.Name, category.CategoryType, category.Color, category.Icon)
	if err != nil {
		return fmt.Errorf("failed to create category: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get category ID: %w", err)
	}

	category.CategoryID = int(id)
	return nil
}

// GetByID retrieves a category by its ID
func (r *CategoryRepository) GetByID(categoryID int) (*model.Category, error) {
	query := `SELECT category_id, name, category_type, color, icon, created_at FROM category WHERE category_id = ?`

	var category model.Category
	err := r.db.QueryRow(query, categoryID).Scan(
		&category.CategoryID,
		&category.Name,
		&category.CategoryType,
		&category.Color,
		&category.Icon,
		&category.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get category: %w", err)
	}

	return &category, nil
}

// GetByName retrieves a category by its name
func (r *CategoryRepository) GetByName(name string) (*model.Category, error) {
	query := `SELECT category_id, name, category_type, color, icon, created_at FROM category WHERE name = ?`

	var category model.Category
	err := r.db.QueryRow(query, name).Scan(
		&category.CategoryID,
		&category.Name,
		&category.CategoryType,
		&category.Color,
		&category.Icon,
		&category.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get category by name: %w", err)
	}

	return &category, nil
}

// GetAll retrieves all categories
func (r *CategoryRepository) GetAll() ([]*model.Category, error) {
	query := `SELECT category_id, name, category_type, color, icon, created_at FROM category ORDER BY name ASC`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query categories: %w", err)
	}
	defer rows.Close()

	var categories []*model.Category
	for rows.Next() {
		var category model.Category
		err := rows.Scan(
			&category.CategoryID,
			&category.Name,
			&category.CategoryType,
			&category.Color,
			&category.Icon,
			&category.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan category: %w", err)
		}
		categories = append(categories, &category)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating categories: %w", err)
	}

	return categories, nil
}

// GetAllWithPatterns retrieves all categories with their associated patterns
func (r *CategoryRepository) GetAllWithPatterns(patternRepo *CategoryPatternRepository) ([]*model.CategoryWithPatterns, error) {
	categories, err := r.GetAll()
	if err != nil {
		return nil, err
	}

	result := make([]*model.CategoryWithPatterns, 0, len(categories))
	for _, cat := range categories {
		patterns, err := patternRepo.GetByCategoryID(cat.CategoryID)
		if err != nil {
			return nil, fmt.Errorf("failed to get patterns for category %d: %w", cat.CategoryID, err)
		}

		cwp := &model.CategoryWithPatterns{
			Category: *cat,
			Patterns: make([]model.CategoryPattern, 0, len(patterns)),
		}

		// Convert []*CategoryPattern to []CategoryPattern
		for _, p := range patterns {
			cwp.Patterns = append(cwp.Patterns, *p)
		}

		result = append(result, cwp)
	}

	return result, nil
}

// Update updates a category's name, type, color, and icon
func (r *CategoryRepository) Update(category *model.Category) error {
	query := `UPDATE category SET name = ?, category_type = ?, color = ?, icon = ? WHERE category_id = ?`
	result, err := r.db.Exec(query, category.Name, category.CategoryType, category.Color, category.Icon, category.CategoryID)
	if err != nil {
		return fmt.Errorf("failed to update category: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("category not found")
	}

	return nil
}

// Delete deletes a category by ID (cascades to patterns, sets transactions to NULL)
func (r *CategoryRepository) Delete(categoryID int) error {
	query := `DELETE FROM category WHERE category_id = ?`
	result, err := r.db.Exec(query, categoryID)
	if err != nil {
		return fmt.Errorf("failed to delete category: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("category not found")
	}

	return nil
}

// Count returns the total number of categories
func (r *CategoryRepository) Count() (int, error) {
	query := `SELECT COUNT(*) FROM category`

	var count int
	err := r.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count categories: %w", err)
	}

	return count, nil
}
