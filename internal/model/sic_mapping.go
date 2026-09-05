package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SICCode is the canonical decimal representation of a positive SIC value.
type SICCode string

// NormalizeSICCode trims surrounding whitespace and removes leading zeroes.
// Validation is intentionally separate so every input boundary can report an
// appropriate error while sharing the same canonical representation.
func NormalizeSICCode(raw string) string {
	normalized := strings.TrimSpace(raw)
	normalized = strings.TrimLeft(normalized, "0")
	if normalized == "" {
		return "0"
	}
	return normalized
}

// ParseSICCode validates and canonicalizes a SIC code. Matchable SIC values
// occupy the positive int64 domain because that is what ofxgo can produce.
func ParseSICCode(raw string) (SICCode, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("SIC code is required")
	}
	for _, char := range trimmed {
		if char < '0' || char > '9' {
			return "", fmt.Errorf("SIC code must contain ASCII digits only")
		}
	}

	normalized := NormalizeSICCode(trimmed)
	if normalized == "0" {
		return "", fmt.Errorf("SIC code must be greater than zero")
	}
	if len(normalized) > 19 {
		return "", fmt.Errorf("SIC code exceeds the positive int64 range")
	}
	if _, err := strconv.ParseInt(normalized, 10, 64); err != nil {
		return "", fmt.Errorf("SIC code exceeds the positive int64 range: %w", err)
	}

	return SICCode(normalized), nil
}

// SICMapping associates a canonical SIC code with an optional category.
type SICMapping struct {
	SICMappingID      int       `json:"sic_mapping_id" db:"sic_mapping_id"`
	SICCode           SICCode   `json:"sic_code" db:"sic_code"`
	Description       string    `json:"description" db:"description"`
	DescriptionDetail string    `json:"description_detail" db:"description_detail"`
	CategoryID        *int      `json:"category_id" db:"category_id"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`

	// Display fields are populated by repository joins and are not persisted.
	CategoryName *string `json:"category_name,omitempty" db:"category_name"`
}

// NewSICMapping constructs a mapping after the caller has validated sicCode.
func NewSICMapping(sicCode SICCode, description, descriptionDetail string, categoryID *int) *SICMapping {
	return &SICMapping{
		SICCode:           sicCode,
		Description:       description,
		DescriptionDetail: descriptionDetail,
		CategoryID:        categoryID,
		CreatedAt:         time.Now(),
	}
}

// HasCategory reports whether this mapping assigns a category.
func (m *SICMapping) HasCategory() bool {
	return m != nil && m.CategoryID != nil
}

// DisplayDescription returns the preferred human-readable description.
func (m *SICMapping) DisplayDescription() string {
	if m == nil {
		return ""
	}
	if m.Description != "" {
		return m.Description
	}
	return m.DescriptionDetail
}

// SICMappingInputRow is an unpersisted row from a mapping CSV file.
type SICMappingInputRow struct {
	RowNumber         int
	SICCode           string
	Description       string
	DescriptionDetail string
	CategoryName      string
	CategoryID        string
}

// SICMappingImportError is a safe row-level diagnostic that excludes row data.
type SICMappingImportError struct {
	RowNumber int    `json:"row_number"`
	Field     string `json:"field"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// SICMappingImportOutcome identifies the startup seed result.
type SICMappingImportOutcome string

const (
	SICMappingImportAbsent            SICMappingImportOutcome = "absent"
	SICMappingImportSkippedExisting   SICMappingImportOutcome = "skipped_existing"
	SICMappingImportInvalid           SICMappingImportOutcome = "invalid"
	SICMappingImportOversized         SICMappingImportOutcome = "oversized"
	SICMappingImportValidated         SICMappingImportOutcome = "validated"
	SICMappingImportImported          SICMappingImportOutcome = "imported"
	SICMappingImportReadFailed        SICMappingImportOutcome = "read_failed"
	SICMappingImportPersistenceFailed SICMappingImportOutcome = "persistence_failed"
)

// SICMappingImportReport summarizes validation and persistence without
// retaining full CSV records.
type SICMappingImportReport struct {
	TotalRows    int                     `json:"total_rows"`
	ValidRows    int                     `json:"valid_rows"`
	RejectedRows int                     `json:"rejected_rows"`
	ImportedRows int                     `json:"imported_rows"`
	ExistingRows int                     `json:"existing_rows"`
	Outcome      SICMappingImportOutcome `json:"outcome"`
	Errors       []SICMappingImportError `json:"errors,omitempty"`
}
