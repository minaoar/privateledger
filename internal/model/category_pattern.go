package model

import (
	"strings"
	"time"
)

// CategoryPattern represents a text pattern used for auto-categorization
type CategoryPattern struct {
	CategoryPatternID int       `json:"category_pattern_id" db:"category_pattern_id"`
	PatternName       string    `json:"pattern_name" db:"pattern_name"`
	CategoryID        int       `json:"category_id" db:"category_id"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
}

// NewCategoryPattern creates a new CategoryPattern
func NewCategoryPattern(patternName string, categoryID int) *CategoryPattern {
	return &CategoryPattern{
		PatternName: patternName,
		CategoryID:  categoryID,
		CreatedAt:   time.Now(),
	}
}

// Matches checks if the pattern matches the given text (case-insensitive contains)
func (cp *CategoryPattern) Matches(text string) bool {
	return strings.Contains(
		strings.ToLower(text),
		strings.ToLower(cp.PatternName),
	)
}
