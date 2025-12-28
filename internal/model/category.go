package model

import "time"

// Category represents a spending category
type Category struct {
	CategoryID   int       `json:"category_id" db:"category_id"`
	Name         string    `json:"name" db:"name"`
	CategoryType string    `json:"category_type" db:"category_type"` // General, Expense, or Income
	Color        *string   `json:"color" db:"color"`                 // Nullable, hex color code (e.g., "#FF5733")
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// NewCategory creates a new Category with the given name, type, and optional color
func NewCategory(name string, categoryType string, color *string) *Category {
	return &Category{
		Name:         name,
		CategoryType: categoryType,
		Color:        color,
		CreatedAt:    time.Now(),
	}
}

// CategoryWithPatterns extends Category with its associated patterns
// Used for API responses that need to include pattern information
type CategoryWithPatterns struct {
	Category
	Patterns []CategoryPattern `json:"patterns"`
}
