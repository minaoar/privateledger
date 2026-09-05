package service

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

const maxSICMappingSeedSize int64 = 10 << 20 // 10 MiB

var sicMappingCSVHeader = []string{
	"SIC_Code",
	"Description",
	"Description_Detail",
	"Category_Name",
	"Category_ID",
}

// SICMappingService owns SIC mapping workflows. UOW-1 provides startup seed
// initialization; later units extend management and categorization workflows.
type SICMappingService struct {
	sicRepo      *repository.SICMappingRepository
	categoryRepo *repository.CategoryRepository
}

// NewSICMappingService creates the UOW-1 mapping service dependencies.
func NewSICMappingService(
	sicRepo *repository.SICMappingRepository,
	categoryRepo *repository.CategoryRepository,
) *SICMappingService {
	return &SICMappingService{sicRepo: sicRepo, categoryRepo: categoryRepo}
}

// ImportFileIfPresent imports a local seed only when the mapping table is
// empty. Invalid and oversized user input is reported but is non-fatal.
func (s *SICMappingService) ImportFileIfPresent(path string) error {
	_, err := s.ImportFileIfPresentWithReport(path)
	return err
}

// ImportFileIfPresentWithReport imports a local seed and returns its explicit
// outcome so the startup boundary can apply logging and lifecycle policy.
func (s *SICMappingService) ImportFileIfPresentWithReport(path string) (*model.SICMappingImportReport, error) {
	report := &model.SICMappingImportReport{Errors: make([]model.SICMappingImportError, 0)}
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		report.Outcome = model.SICMappingImportAbsent
		return report, nil
	}
	if err != nil {
		report.Outcome = model.SICMappingImportReadFailed
		return report, fmt.Errorf("failed to discover SIC mapping seed %q: %w", path, err)
	}

	existing, err := s.sicRepo.Count()
	if err != nil {
		report.Outcome = model.SICMappingImportPersistenceFailed
		return report, fmt.Errorf("failed to check existing SIC mappings: %w", err)
	}
	if existing > 0 {
		report.Outcome = model.SICMappingImportSkippedExisting
		report.ExistingRows = existing
		return report, nil
	}

	file, err := os.Open(path)
	if err != nil {
		report.Outcome = model.SICMappingImportReadFailed
		return report, fmt.Errorf("failed to open SIC mapping seed %q: %w", path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		report.Outcome = model.SICMappingImportReadFailed
		return report, fmt.Errorf("failed to inspect SIC mapping seed %q: %w", path, err)
	}
	if info.Size() > maxSICMappingSeedSize {
		report.Outcome = model.SICMappingImportOversized
		return report, nil
	}

	mappings, report, err := s.ValidateCSV(file)
	if err != nil {
		report.Outcome = model.SICMappingImportReadFailed
		return report, fmt.Errorf("failed to read SIC mapping seed %q: %w", path, err)
	}
	if report.RejectedRows > 0 {
		report.Outcome = model.SICMappingImportInvalid
		return report, nil
	}

	if err := s.sicRepo.BulkInsertAtomic(mappings); err != nil {
		report.Outcome = model.SICMappingImportPersistenceFailed
		return report, fmt.Errorf("failed to persist SIC mapping seed %q: %w", path, err)
	}
	report.Outcome = model.SICMappingImportImported
	report.ImportedRows = len(mappings)
	return report, nil
}

// ValidateCSV parses and validates the complete mapping CSV without mutation.
func (s *SICMappingService) ValidateCSV(reader io.Reader) ([]*model.SICMapping, *model.SICMappingImportReport, error) {
	report := &model.SICMappingImportReport{
		Outcome: model.SICMappingImportInvalid,
		Errors:  make([]model.SICMappingImportError, 0),
	}
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = len(sicMappingCSVHeader)

	header, err := csvReader.Read()
	if err == io.EOF {
		addSICImportError(report, 1, "header", "missing_header", "CSV header is required")
		report.RejectedRows = 1
		return nil, report, nil
	}
	if err != nil {
		if isCSVValidationError(err) {
			addSICImportError(report, 1, "header", "malformed_csv", "CSV header is malformed")
			report.RejectedRows = 1
			return nil, report, nil
		}
		return nil, report, fmt.Errorf("failed to read CSV header: %w", err)
	}
	if !matchesSICMappingHeader(header) {
		addSICImportError(report, 1, "header", "invalid_header", "CSV header does not match the required fields")
		report.RejectedRows = 1
		return nil, report, nil
	}

	categories, err := s.categoryRepo.GetAll()
	if err != nil {
		return nil, report, fmt.Errorf("failed to load categories for mapping validation: %w", err)
	}
	exactCategories, foldedCategories := indexCategories(categories)
	seenSICCodes := make(map[model.SICCode]int)
	mappings := make([]*model.SICMapping, 0)

	for rowNumber := 2; ; rowNumber++ {
		record, readErr := csvReader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if isCSVValidationError(readErr) {
				report.TotalRows++
				addSICImportError(report, rowNumber, "row", "malformed_csv", "CSV row is malformed")
				break
			}
			return nil, report, fmt.Errorf("failed to read CSV row %d: %w", rowNumber, readErr)
		}

		report.TotalRows++
		rowErrorCount := len(report.Errors)
		sicCode, parseErr := model.ParseSICCode(record[0])
		if parseErr != nil {
			addSICImportError(report, rowNumber, "SIC_Code", "invalid_sic", "SIC code is invalid")
		} else if firstRow, duplicate := seenSICCodes[sicCode]; duplicate {
			addSICImportError(report, rowNumber, "SIC_Code", "duplicate_sic", fmt.Sprintf("SIC code duplicates row %d", firstRow))
		} else {
			seenSICCodes[sicCode] = rowNumber
		}

		categoryID := resolveSICCategory(
			report,
			rowNumber,
			record[3],
			record[4],
			exactCategories,
			foldedCategories,
		)
		if len(report.Errors) == rowErrorCount {
			mappings = append(mappings, model.NewSICMapping(sicCode, record[1], record[2], categoryID))
			report.ValidRows++
		}
	}

	report.RejectedRows = report.TotalRows - report.ValidRows
	if report.RejectedRows > 0 {
		return nil, report, nil
	}
	report.Outcome = model.SICMappingImportValidated
	return mappings, report, nil
}

func matchesSICMappingHeader(header []string) bool {
	if len(header) != len(sicMappingCSVHeader) {
		return false
	}
	for i := range header {
		if header[i] != sicMappingCSVHeader[i] {
			return false
		}
	}
	return true
}

func isCSVValidationError(err error) bool {
	var parseErr *csv.ParseError
	return errors.As(err, &parseErr)
}

func addSICImportError(report *model.SICMappingImportReport, row int, field, code, message string) {
	report.Errors = append(report.Errors, model.SICMappingImportError{
		RowNumber: row,
		Field:     field,
		Code:      code,
		Message:   message,
	})
}

func indexCategories(categories []*model.Category) (map[string]*model.Category, map[string][]*model.Category) {
	exact := make(map[string]*model.Category, len(categories))
	folded := make(map[string][]*model.Category, len(categories))
	for _, category := range categories {
		exact[category.Name] = category
		key := strings.ToLower(category.Name)
		folded[key] = append(folded[key], category)
	}
	return exact, folded
}

func resolveSICCategory(
	report *model.SICMappingImportReport,
	rowNumber int,
	rawName string,
	rawID string,
	exact map[string]*model.Category,
	folded map[string][]*model.Category,
) *int {
	name := strings.TrimSpace(rawName)
	idText := strings.TrimSpace(rawID)
	if name == "" && idText == "" {
		return nil
	}
	if name == "" {
		addSICImportError(report, rowNumber, "Category_ID", "id_without_name", "Category_ID requires Category_Name")
		return nil
	}

	category := exact[name]
	if category == nil {
		matches := folded[strings.ToLower(name)]
		switch len(matches) {
		case 0:
			addSICImportError(report, rowNumber, "Category_Name", "category_not_found", "Category_Name does not resolve")
			return nil
		case 1:
			category = matches[0]
		default:
			addSICImportError(report, rowNumber, "Category_Name", "category_ambiguous", "Category_Name is ambiguous")
			return nil
		}
	}

	if idText != "" {
		categoryID, err := strconv.Atoi(idText)
		if err != nil || categoryID <= 0 {
			addSICImportError(report, rowNumber, "Category_ID", "invalid_category_id", "Category_ID must be a positive integer")
			return nil
		}
		if categoryID != category.CategoryID {
			addSICImportError(report, rowNumber, "Category_ID", "category_conflict", "Category_ID does not match Category_Name")
			return nil
		}
	}

	categoryID := category.CategoryID
	return &categoryID
}
