package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Shared SIC mapping bounds. These live in the domain package so the startup
// seed, the mapping service, and the HTTP upload boundary all enforce one
// value without a duplicated literal and without the domain depending on
// net/http.
const (
	// MaxSICMappingFileSize bounds any mapping CSV, whether it arrives as a
	// startup seed file or an HTTP upload.
	MaxSICMappingFileSize int64 = 10 << 20 // 10 MiB

	// MaxSICMappingDiagnostics bounds the row diagnostics retained by the
	// service that produces them, so a large invalid file can neither retain
	// unbounded diagnostics nor return an unbounded response body.
	MaxSICMappingDiagnostics = 50

	// maxDiagValueRunes bounds one user-controlled value interpolated into a
	// diagnostic message, per NFR-U4-SEC-01. A category name is unbounded from
	// both directions: a CSV field may be as large as the whole upload, and
	// category.name carries no length constraint in the schema. Sixty-four
	// runes is far beyond any real category name, so the value stays useful
	// for the single edit that repairs the file.
	maxDiagValueRunes = 64

	// maxDiagMessageRunes bounds an assembled diagnostic message, per
	// NFR-U4-SEC-04. It is a backstop, not the primary bound: the largest
	// well-formed message is roughly 250 runes, so this never fires in normal
	// operation. It exists because DiagValue makes bypass conspicuous without
	// making it impossible, and it bounds the worst case of a future
	// diagnostic that interpolates a raw string directly.
	maxDiagMessageRunes = 512
)

// Stable mapping errors. Handlers map these to transport status codes without
// matching driver error strings.
var (
	// ErrSICMappingDuplicate reports a normalized SIC code collision.
	ErrSICMappingDuplicate = errors.New("SIC mapping already exists for this code")

	// ErrSICMappingNotFound reports an unknown mapping ID.
	ErrSICMappingNotFound = errors.New("SIC mapping not found")

	// ErrSICCategoryNotFound reports a category reference that does not resolve.
	ErrSICCategoryNotFound = errors.New("category not found")

	// ErrSICMappingValidation reports rejected user input, as distinct from an
	// infrastructure failure. Wrapping keeps the specific reason readable while
	// letting callers classify the failure without matching message text.
	ErrSICMappingValidation = errors.New("invalid SIC mapping")

	// ErrSICMappingBusy reports that admission to the mutation gate expired
	// before this caller acquired it. No backup or mutation was performed.
	ErrSICMappingBusy = errors.New("SIC mapping service is busy")

	// ErrSICMappingCancelled reports that the caller cancelled before acquiring
	// admission. No backup or mutation was performed.
	ErrSICMappingCancelled = errors.New("SIC mapping request cancelled")

	// ErrSICMappingOversized reports input beyond MaxSICMappingFileSize.
	ErrSICMappingOversized = errors.New("SIC mapping file exceeds the maximum size")

	// ErrSICMappingDatabaseBusy reports that SQLite stayed locked past its busy
	// timeout before the write committed. It is retryable by the user.
	ErrSICMappingDatabaseBusy = errors.New("database is busy")
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

// SICMappingImportOutcome identifies a mapping import result. One set is
// shared by the startup seed and the HTTP upload path so both units speak one
// vocabulary. "absent" and "skipped_existing" are startup-only; "merged" is
// upload-only; "imported" denotes an insert-only startup seed and is never
// emitted by upload.
type SICMappingImportOutcome string

const (
	SICMappingImportAbsent            SICMappingImportOutcome = "absent"
	SICMappingImportSkippedExisting   SICMappingImportOutcome = "skipped_existing"
	SICMappingImportInvalid           SICMappingImportOutcome = "invalid"
	SICMappingImportOversized         SICMappingImportOutcome = "oversized"
	SICMappingImportValidated         SICMappingImportOutcome = "validated"
	SICMappingImportImported          SICMappingImportOutcome = "imported"
	SICMappingImportMerged            SICMappingImportOutcome = "merged"
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

	// Errors is the bounded row diagnostics list. It is capped at
	// MaxSICMappingDiagnostics entries; RejectedRows remains the authoritative
	// total regardless of how many diagnostics survived the cap.
	Errors []SICMappingImportError `json:"errors,omitempty"`

	// DiagnosticsTruncated is true when the cap dropped at least one
	// diagnostic, so RejectedRows then exceeds len(Errors).
	DiagnosticsTruncated bool `json:"diagnostics_truncated"`
}

// DiagValue is a bounded, sanitized value that is safe to interpolate into a
// diagnostic message. It wraps an unexported field, so the only way to obtain
// one is NewDiagValue: a raw string cannot be converted into a DiagValue, and
// cannot be passed where one is required.
//
// That is the point. Diagnostics name values the user must fix, and those
// values are unbounded user-controlled text from a CSV field or a category
// name. Requiring this type at the interpolation boundary makes the bounded
// path the path of least resistance.
type DiagValue struct {
	s string
}

// NewDiagValue bounds and sanitizes one value for use in a diagnostic.
//
// Control characters are replaced before truncation, so a truncation boundary
// can never split a replaced rune. The result is valid UTF-8, so JSON encoding
// and DOM insertion cannot produce mojibake or silently dropped bytes.
func NewDiagValue(raw string) DiagValue {
	var b strings.Builder
	kept := 0
	truncated := false
	for _, r := range raw {
		if kept == maxDiagValueRunes {
			truncated = true
			break
		}
		if unicode.IsControl(r) {
			r = utf8.RuneError
		}
		b.WriteRune(r)
		kept++
	}
	if truncated {
		b.WriteRune('\u2026')
	}
	return DiagValue{s: b.String()}
}

// String renders the bounded value, so a DiagValue formats with %s.
func (v DiagValue) String() string {
	return v.s
}

// AddError appends a row diagnostic under the shared bound. Diagnostics past
// the cap are counted as truncated rather than retained. Callers must not use
// len(Errors) to decide whether a row was valid: once the cap is reached the
// list stops growing while rows keep being rejected.
//
// The message is truncated to maxDiagMessageRunes as a final backstop. Callers
// interpolating user-controlled text must use AddErrorf, which bounds each
// value individually and keeps the message readable; this cap only limits the
// damage when something bypasses that.
func (r *SICMappingImportReport) AddError(row int, field, code, message string) {
	if r == nil {
		return
	}
	if len(r.Errors) >= MaxSICMappingDiagnostics {
		r.DiagnosticsTruncated = true
		return
	}
	r.Errors = append(r.Errors, SICMappingImportError{
		RowNumber: row,
		Field:     field,
		Code:      code,
		Message:   truncateRunes(message, maxDiagMessageRunes),
	})
}

// AddErrorf appends a row diagnostic whose message interpolates user-controlled
// values. Accepting only DiagValue means a raw string will not compile here,
// so a value reaches a diagnostic bounded or not at all.
func (r *SICMappingImportReport) AddErrorf(row int, field, code, format string, values ...DiagValue) {
	if r == nil {
		return
	}
	args := make([]any, len(values))
	for i, v := range values {
		args[i] = v
	}
	r.AddError(row, field, code, fmt.Sprintf(format, args...))
}

// truncateRunes bounds a string by rune count, appending an ellipsis only when
// it actually truncated. Counting runes rather than bytes keeps a multi-byte
// value from being cut mid-character.
func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	var b strings.Builder
	kept := 0
	for _, r := range s {
		if kept == limit {
			break
		}
		b.WriteRune(r)
		kept++
	}
	b.WriteRune('\u2026')
	return b.String()
}

// SICMappingInput is a transport-neutral create/update command. It never
// carries joined display fields; update additionally carries the
// route-resolved mapping ID.
type SICMappingInput struct {
	SICCode           string `json:"sic_code"`
	Description       string `json:"description"`
	DescriptionDetail string `json:"description_detail"`
	CategoryID        *int   `json:"category_id"`
}

// SICMappingPageData carries everything one server render of the mapping page
// requires. It is assembled by the service, never by the page handler.
type SICMappingPageData struct {
	Mappings   []*SICMapping `json:"mappings"`
	Categories []*Category   `json:"categories"`
}

// SICMappingMutationResult reports a single CRUD mutation truthfully. Once
// MappingCommitted is true the write is durable, and any PostCommitWarnings
// describe follow-up work that failed afterwards - never a reason to retry
// the mutation.
type SICMappingMutationResult struct {
	Mapping           *SICMapping `json:"mapping,omitempty"`
	MappingCommitted  bool        `json:"mapping_committed"`
	RecategorizedRows int         `json:"recategorized_rows"`

	// FR16's three counts. RecategorizedRows above keeps its original meaning
	// and equals MovedCount; both are reported so nothing reading the older
	// field breaks. No omitempty: FR16 requires a rule change that moved
	// nothing to say so, and an absent field reads as "unknown" rather than
	// "none".
	MovedCount           int `json:"moved_count"`
	UncategorizedCount   int `json:"uncategorized_count"`
	ManualProtectedCount int `json:"manual_protected_count"`

	PostCommitWarnings []string `json:"post_commit_warnings,omitempty"`
}

// SICMappingImportResult extends SICMappingImportReport for the upload merge
// path. Embedding keeps every shared field's UOW-1 meaning and JSON name, so
// the startup seed can still produce the report shape unchanged. The embedded
// Errors list is the bounded row diagnostics referenced by the design as
// "Diagnostics"; no second diagnostic type is introduced.
//
// The embedded ImportedRows stays zero on upload: the merge path reports its
// row breakdown as CreatedRows/UpdatedRows/UnchangedRows instead.
type SICMappingImportResult struct {
	SICMappingImportReport

	CreatedRows       int `json:"created_rows"`
	UpdatedRows       int `json:"updated_rows"`
	UnchangedRows     int `json:"unchanged_rows"`
	RecategorizedRows int `json:"recategorized_rows"`

	// FR16's three counts. RecategorizedRows above keeps its original meaning
	// and equals MovedCount; both are reported so nothing reading the older
	// field breaks. No omitempty: FR16 requires a rule change that moved
	// nothing to say so, and an absent field reads as "unknown" rather than
	// "none".
	MovedCount           int `json:"moved_count"`
	UncategorizedCount   int `json:"uncategorized_count"`
	ManualProtectedCount int `json:"manual_protected_count"`

	// BackupPath is non-empty only when the complete backup write succeeded.
	BackupPath string `json:"backup_path,omitempty"`

	// BackupWarning is non-empty only when the backup failed while processing
	// continued. Backup failure never blocks the merge.
	BackupWarning string `json:"backup_warning,omitempty"`

	// MappingCommitted is true only after the SQLite merge commits. It
	// disambiguates post-commit warnings from a rollback.
	MappingCommitted bool `json:"mapping_committed"`

	// PostCommitWarnings records cache-reload or collaborator failures that
	// occurred after a successful commit.
	PostCommitWarnings []string `json:"post_commit_warnings,omitempty"`
}
