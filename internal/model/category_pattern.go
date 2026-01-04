package model

import (
	"regexp"
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

var spaceNormalizer = regexp.MustCompile(`[\s\x{00A0}]+`) // includes &nbsp;

// Remove multiple space, \t, &nbsp from string to match
func normalizeSpaces(s string) string {
	return spaceNormalizer.ReplaceAllString(strings.TrimSpace(s), " ")
}

// Matches checks if the pattern matches the given text (case-insensitive contains)
func (cp *CategoryPattern) Matches(text string) bool {
	return strings.Contains(
		strings.ToLower(normalizeSpaces(text)),
		strings.ToLower(normalizeSpaces(cp.PatternName)),
	)
}
