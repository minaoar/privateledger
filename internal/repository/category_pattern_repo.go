package repository

import (
	"database/sql"
	"fmt"

	"github.com/oronno/privateledger/internal/model"
)

// CategoryPatternRepository handles database operations for category patterns
type CategoryPatternRepository struct {
	db *sql.DB
}

// NewCategoryPatternRepository creates a new CategoryPatternRepository
func NewCategoryPatternRepository(db *sql.DB) *CategoryPatternRepository {
	return &CategoryPatternRepository{db: db}
}

// Create inserts a new category pattern into the database
func (r *CategoryPatternRepository) Create(pattern *model.CategoryPattern) error {
	query := `INSERT INTO category_pattern (pattern_name, category_id) VALUES (?, ?)`
	result, err := r.db.Exec(query, pattern.PatternName, pattern.CategoryID)
	if err != nil {
		return fmt.Errorf("failed to create category pattern: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get category pattern ID: %w", err)
	}

	pattern.CategoryPatternID = int(id)
	return nil
}

// GetByID retrieves a category pattern by its ID
func (r *CategoryPatternRepository) GetByID(patternID int) (*model.CategoryPattern, error) {
	query := `
		SELECT category_pattern_id, pattern_name, category_id, created_at
		FROM category_pattern
		WHERE category_pattern_id = ?
	`

	var pattern model.CategoryPattern
	err := r.db.QueryRow(query, patternID).Scan(
		&pattern.CategoryPatternID,
		&pattern.PatternName,
		&pattern.CategoryID,
		&pattern.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get category pattern: %w", err)
	}

	return &pattern, nil
}

// GetByPatternName retrieves a category pattern by its pattern name
func (r *CategoryPatternRepository) GetByPatternName(patternName string) (*model.CategoryPattern, error) {
	query := `
		SELECT category_pattern_id, pattern_name, category_id, created_at
		FROM category_pattern
		WHERE pattern_name = ?
	`

	var pattern model.CategoryPattern
	err := r.db.QueryRow(query, patternName).Scan(
		&pattern.CategoryPatternID,
		&pattern.PatternName,
		&pattern.CategoryID,
		&pattern.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get category pattern by name: %w", err)
	}

	return &pattern, nil
}

// GetByCategoryID retrieves all patterns for a specific category
func (r *CategoryPatternRepository) GetByCategoryID(categoryID int) ([]*model.CategoryPattern, error) {
	query := `
		SELECT category_pattern_id, pattern_name, category_id, created_at
		FROM category_pattern
		WHERE category_id = ?
		ORDER BY pattern_name ASC
	`

	rows, err := r.db.Query(query, categoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to query category patterns: %w", err)
	}
	defer rows.Close()

	var patterns []*model.CategoryPattern
	for rows.Next() {
		var pattern model.CategoryPattern
		err := rows.Scan(
			&pattern.CategoryPatternID,
			&pattern.PatternName,
			&pattern.CategoryID,
			&pattern.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan category pattern: %w", err)
		}
		patterns = append(patterns, &pattern)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating category patterns: %w", err)
	}

	return patterns, nil
}

// GetAll retrieves all category patterns
func (r *CategoryPatternRepository) GetAll() ([]*model.CategoryPattern, error) {
	query := `
		SELECT category_pattern_id, pattern_name, category_id, created_at
		FROM category_pattern
		ORDER BY pattern_name ASC
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query category patterns: %w", err)
	}
	defer rows.Close()

	var patterns []*model.CategoryPattern
	for rows.Next() {
		var pattern model.CategoryPattern
		err := rows.Scan(
			&pattern.CategoryPatternID,
			&pattern.PatternName,
			&pattern.CategoryID,
			&pattern.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan category pattern: %w", err)
		}
		patterns = append(patterns, &pattern)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating category patterns: %w", err)
	}

	return patterns, nil
}

// Update updates a category pattern's name
func (r *CategoryPatternRepository) Update(pattern *model.CategoryPattern) error {
	query := `UPDATE category_pattern SET pattern_name = ? WHERE category_pattern_id = ?`
	result, err := r.db.Exec(query, pattern.PatternName, pattern.CategoryPatternID)
	if err != nil {
		return fmt.Errorf("failed to update category pattern: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("category pattern not found")
	}

	return nil
}

// Delete deletes a category pattern by ID
func (r *CategoryPatternRepository) Delete(patternID int) error {
	query := `DELETE FROM category_pattern WHERE category_pattern_id = ?`
	result, err := r.db.Exec(query, patternID)
	if err != nil {
		return fmt.Errorf("failed to delete category pattern: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("category pattern not found")
	}

	return nil
}

// Count returns the total number of category patterns
func (r *CategoryPatternRepository) Count() (int, error) {
	query := `SELECT COUNT(*) FROM category_pattern`

	var count int
	err := r.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count category patterns: %w", err)
	}

	return count, nil
}

