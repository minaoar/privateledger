package service

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

// maxSICMappingSeedSize is the shared mapping-file bound. The startup seed and
// the HTTP upload boundary enforce the same value from one domain constant.
const maxSICMappingSeedSize int64 = model.MaxSICMappingFileSize

var sicMappingCSVHeader = []string{
	"SIC_Code",
	"Description",
	"Description_Detail",
	"Category_Name",
	"Category_ID",
}

// defaultSICAdmissionTimeout bounds how long a competing mutation waits for
// the service gate. It is code-level configuration supplied through the
// constructor, deliberately not a config.json setting.
const defaultSICAdmissionTimeout = 5 * time.Second

// SICRecategorizationCollaborator is the narrow contract UOW-2 uses to hand
// mapping changes to categorization. UOW-2 decides when it is called and with
// which codes; UOW-3 owns transaction selection and categorization semantics.
//
// It takes no context: at the UOW-2 checkpoint the implementation is an
// immediate no-op, so adding a processing budget here would describe a
// deadline nothing needs. UOW-3 owns that decision when it introduces real
// recategorization work.
type SICRecategorizationCollaborator interface {
	// ReloadMappings refreshes cached mapping rules after a committed change.
	ReloadMappings() error

	// Reexamine applies the current rules to every transaction the user did
	// not categorize by hand, and reports what moved, what became
	// uncategorized, and what was left alone because it is manual.
	//
	// It takes no scope. Under FR15 a rule change re-examines everything the
	// rules govern, so there is no affected set to pass — and a text-pattern
	// change, which reaches the same entry point, has no SIC codes to offer.
	// This replaced RecategorizeBySICCodes(sicCodes) for that reason, and
	// because a single count cannot carry FR16's three.
	Reexamine() (RecategorizationCounts, error)
}

// RecategorizationCounts carries FR16's three counts across the collaborator
// boundary without exposing the service-layer result type.
type RecategorizationCounts struct {
	Moved           int
	Uncategorized   int
	ManualProtected int
}

// noopSICRecategorizationCollaborator is the UOW-2 checkpoint implementation.
// Production wiring supplies it explicitly so a nil collaborator can never be
// mistaken for successful work.
type noopSICRecategorizationCollaborator struct{}

func (noopSICRecategorizationCollaborator) ReloadMappings() error { return nil }

func (noopSICRecategorizationCollaborator) Reexamine() (RecategorizationCounts, error) {
	return RecategorizationCounts{}, nil
}

// NewNoopSICRecategorizationCollaborator returns the checkpoint collaborator
// used until UOW-3 supplies the real adapter.
func NewNoopSICRecategorizationCollaborator() SICRecategorizationCollaborator {
	return noopSICRecategorizationCollaborator{}
}

// SICMappingService owns SIC mapping workflows: UOW-1 startup seeding plus
// UOW-2 management, export, and merge upload.
//
// All mutations through one injected instance are serialized by gate, a
// capacity-one channel. Sending acquires and receiving releases, so admission
// never needs a goroutine that might acquire after its caller returned.
type SICMappingService struct {
	sicRepo      *repository.SICMappingRepository
	categoryRepo *repository.CategoryRepository

	// dataDir is where backups are written. It is the injected application
	// data directory and is never derived from uploaded content.
	dataDir string

	gate             chan struct{}
	admissionTimeout time.Duration
	collaborator     SICRecategorizationCollaborator
}

// NewSICMappingService creates the mapping service for startup seeding. Its
// signature is unchanged from UOW-1 so existing call sites keep working; the
// management fields take safe defaults.
func NewSICMappingService(
	sicRepo *repository.SICMappingRepository,
	categoryRepo *repository.CategoryRepository,
) *SICMappingService {
	return NewSICMappingManagementService(
		sicRepo,
		categoryRepo,
		"",
		defaultSICAdmissionTimeout,
		NewNoopSICRecategorizationCollaborator(),
	)
}

// NewSICMappingManagementService creates the fully wired UOW-2 service.
// A non-positive admissionTimeout falls back to the default.
//
// A nil collaborator is rejected, not defaulted: callers that want checkpoint
// behaviour must pass NewNoopSICRecategorizationCollaborator() explicitly.
func NewSICMappingManagementService(
	sicRepo *repository.SICMappingRepository,
	categoryRepo *repository.CategoryRepository,
	dataDir string,
	admissionTimeout time.Duration,
	collaborator SICRecategorizationCollaborator,
) *SICMappingService {
	if admissionTimeout <= 0 {
		admissionTimeout = defaultSICAdmissionTimeout
	}
	// A missing collaborator is a wiring mistake, not a default. Substituting
	// the no-op here would let a UOW-3 integration error look like successful
	// recategorization; callers that genuinely want the checkpoint behaviour
	// pass NewNoopSICRecategorizationCollaborator() explicitly.
	if collaborator == nil {
		panic("service: SICMappingService requires an explicit recategorization collaborator")
	}
	return &SICMappingService{
		sicRepo:          sicRepo,
		categoryRepo:     categoryRepo,
		dataDir:          dataDir,
		gate:             make(chan struct{}, 1),
		admissionTimeout: admissionTimeout,
		collaborator:     collaborator,
	}
}

// acquire takes the mutation gate, returning a release function. It bounds
// waiting at admissionTimeout and honours caller cancellation, so a competing
// request fails fast instead of queueing without limit.
//
// The returned release is idempotent and must be deferred immediately: every
// completion and failure path has to release, or the service deadlocks.
func (s *SICMappingService) acquire(ctx context.Context) (func(), error) {
	timer := time.NewTimer(s.admissionTimeout)
	defer timer.Stop()

	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, model.ErrSICMappingCancelled
	case <-timer.C:
		return nil, model.ErrSICMappingBusy
	}

	var once sync.Once
	release := func() {
		once.Do(func() { <-s.gate })
	}

	// Recheck cancellation after acquisition so a caller that gave up while
	// waiting cannot go on to mutate.
	select {
	case <-ctx.Done():
		release()
		return nil, model.ErrSICMappingCancelled
	default:
	}
	return release, nil
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
		// Preserve the outcome ValidateCSV classified. Overwriting it here
		// reported a database failure as read_failed, which is what
		// independent finding F-15 recorded.
		if report.Outcome != model.SICMappingImportPersistenceFailed {
			report.Outcome = model.SICMappingImportReadFailed
		}
		return report, fmt.Errorf("failed to validate SIC mapping seed %q: %w", path, err)
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
	// The header is read without a field-count expectation so a wrong column
	// count is diagnosed here, naming both counts, rather than surfacing as a
	// generic malformed-CSV error that says nothing about columns. Rows are
	// held to the canonical count immediately afterwards, unchanged.
	csvReader.FieldsPerRecord = -1

	header, err := csvReader.Read()
	if err == io.EOF {
		report.AddError(1, "header", "missing_header", "CSV header is required")
		report.RejectedRows = 1
		return nil, report, nil
	}
	if err != nil {
		if isCSVValidationError(err) {
			report.AddError(1, "header", "malformed_csv", "CSV header is malformed")
			report.RejectedRows = 1
			return nil, report, nil
		}
		// A non-parse failure here is the reader itself failing, not invalid
		// user content.
		report.Outcome = model.SICMappingImportReadFailed
		return nil, report, fmt.Errorf("failed to read CSV header: %w", err)
	}
	if mismatch := matchSICMappingHeader(header); mismatch != nil {
		report.AddError(1, "header", "invalid_header", mismatch.message())
		report.RejectedRows = 1
		return nil, report, nil
	}
	// The header matched, so every row must carry the canonical column count.
	csvReader.FieldsPerRecord = len(sicMappingCSVHeader)

	categories, err := s.categoryRepo.GetAll()
	if err != nil {
		// Reading categories is a database operation. Classifying it as
		// persistence_failed keeps read_failed meaning "the CSV could not be
		// read", so a database outage is never reported as bad user input.
		report.Outcome = model.SICMappingImportPersistenceFailed
		return nil, report, fmt.Errorf("failed to load categories for mapping validation: %w", err)
	}
	categoryIndexes := indexCategories(categories)
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
				report.AddError(rowNumber, "row", "malformed_csv", "CSV row is malformed")
				break
			}
			report.Outcome = model.SICMappingImportReadFailed
			return nil, report, fmt.Errorf("failed to read CSV row %d: %w", rowNumber, readErr)
		}

		report.TotalRows++
		row := &sicRowValidator{report: report, row: rowNumber}
		sicCode, parseErr := model.ParseSICCode(record[0])
		if parseErr != nil {
			row.reject("SIC_Code", "invalid_sic", "SIC code is invalid")
		} else if firstRow, duplicate := seenSICCodes[sicCode]; duplicate {
			row.reject("SIC_Code", "duplicate_sic", fmt.Sprintf("SIC code duplicates row %d", firstRow))
		} else {
			seenSICCodes[sicCode] = rowNumber
		}

		categoryID := resolveSICCategory(
			row,
			record[3],
			record[4],
			categoryIndexes,
		)
		if !row.invalid {
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

// utf8BOM is the byte-order mark a spreadsheet on Windows writes at the start
// of a UTF-8 CSV. It is invisible, so a header carrying one previously failed
// with a diagnostic that gave no hint the cause was a byte nobody can see.
const utf8BOM = "\ufeff"

// headerMismatch describes why a header failed, so the diagnostic can name the
// specific repair instead of restating that something is wrong. It carries no
// text from the file: a position and two counts are all that is needed, and
// echoing nothing keeps the header path outside the value-bounding rules
// entirely rather than dependent on them.
type headerMismatch struct {
	// column is the 1-based position that failed, zero when the count is wrong.
	column int
	// gotColumns and wantColumns are set only for a count mismatch.
	gotColumns  int
	wantColumns int
}

func (m headerMismatch) message() string {
	if m.column == 0 {
		return fmt.Sprintf("CSV header must have %d columns; this file has %d", m.wantColumns, m.gotColumns)
	}
	return fmt.Sprintf("CSV header column %d must be %s", m.column, sicMappingCSVHeader[m.column-1])
}

// matchSICMappingHeader compares the header after normalization, returning nil
// when it matches. Normalization strips a leading byte-order mark, trims
// surrounding whitespace per column, and compares case-insensitively.
//
// Column order and count remain required, and the canonical spelling in
// sicMappingCSVHeader is untouched, so export output does not drift: the
// tolerance is one-directional by construction.
func matchSICMappingHeader(header []string) *headerMismatch {
	if len(header) != len(sicMappingCSVHeader) {
		// Checked before names so a file with a missing column is diagnosed as
		// a count problem, rather than against a position that has shifted.
		return &headerMismatch{gotColumns: len(header), wantColumns: len(sicMappingCSVHeader)}
	}
	for i := range header {
		name := header[i]
		if i == 0 {
			// The mark can only appear at the start of the file, so only the
			// first column can carry one.
			name = strings.TrimPrefix(name, utf8BOM)
		}
		// TrimSpace uses unicode.IsSpace, which already covers the
		// non-breaking space a spreadsheet may emit.
		if !strings.EqualFold(strings.TrimSpace(name), sicMappingCSVHeader[i]) {
			return &headerMismatch{column: i + 1}
		}
	}
	return nil
}

func isCSVValidationError(err error) bool {
	var parseErr *csv.ParseError
	return errors.As(err, &parseErr)
}

// sicRowValidator accumulates bounded diagnostics for one CSV row while
// tracking that row's validity independently of how many diagnostics were
// retained. Row validity must never be inferred from len(report.Errors):
// once the diagnostic cap is reached the list stops growing, so a length
// comparison would classify every later invalid row as valid and merge it.
type sicRowValidator struct {
	report  *model.SICMappingImportReport
	row     int
	invalid bool
}

// reject marks the row invalid and records a bounded diagnostic. A row with
// several errors is still exactly one rejected row.
func (v *sicRowValidator) reject(field, code, message string) {
	v.invalid = true
	v.report.AddError(v.row, field, code, message)
}

// rejectf is reject for a message naming user-controlled values. Taking
// model.DiagValue rather than any or string means a raw name cannot be
// interpolated here without being bounded first.
func (v *sicRowValidator) rejectf(field, code, format string, values ...model.DiagValue) {
	v.invalid = true
	v.report.AddErrorf(v.row, field, code, format, values...)
}

// rejectCategoryNotFound reports a name that does not resolve, naming the
// category the row's Category_ID currently refers to when there is one. That
// pairing is the whole point: told that "Groceries" is unknown and that
// Category_ID 1 is now "Food", the user repairs the file with one
// find-and-replace.
//
// The ID form is used only when the ID parses, is positive and resolves. A
// malformed ID keeps the short form, because this diagnostic must not double
// as ID validation, which invalid_category_id owns.
func (v *sicRowValidator) rejectCategoryNotFound(name, idText string, categories categoryIndex) {
	if idText != "" {
		if id, err := strconv.Atoi(idText); err == nil && id > 0 {
			if current := categories.byID[id]; current != nil {
				v.rejectf("Category_Name", "category_not_found",
					"Category_Name %q does not exist; Category_ID %s is currently %q",
					model.NewDiagValue(name), diagInt(id), model.NewDiagValue(current.Name))
				return
			}
		}
	}
	v.rejectf("Category_Name", "category_not_found",
		"Category_Name %q does not exist", model.NewDiagValue(name))
}

// rejectAmbiguousCategory names the colliding categories so the user can
// resolve the collision, rather than the application choosing one and
// categorizing transactions against a category nobody picked.
//
// Each name is bounded individually and the list is capped, because bounding
// per name alone does not bound this message: the number of collisions is
// itself unbounded. The format string is assembled from constants and counts
// only, never from user text.
func (v *sicRowValidator) rejectAmbiguousCategory(name string, matches []*model.Category) {
	shown := matches
	if len(shown) > maxNamedCollisions {
		shown = shown[:maxNamedCollisions]
	}

	values := make([]model.DiagValue, 0, len(shown)+3)
	values = append(values, model.NewDiagValue(name), diagInt(len(matches)))

	placeholders := make([]string, len(shown))
	for i, category := range shown {
		placeholders[i] = "%q"
		values = append(values, model.NewDiagValue(category.Name))
	}

	format := "Category_Name %q matches %s categories: " + strings.Join(placeholders, ", ")
	if remaining := len(matches) - len(shown); remaining > 0 {
		format += " and %s more"
		values = append(values, diagInt(remaining))
	}
	v.rejectf("Category_Name", "category_ambiguous", format, values...)
}

// diagInt renders a count for a diagnostic. An integer cannot be unbounded, so
// this is a formatting convenience rather than a way around the value bound.
func diagInt(n int) model.DiagValue {
	return model.NewDiagValue(strconv.Itoa(n))
}

// categoryIndex holds the three lookups row validation needs, built in one
// pass over categories the caller has already loaded. The by-ID map exists so
// a diagnostic can name the category a stale Category_ID currently refers to;
// it costs one extra pass over a slice already in memory and issues no query.
type categoryIndex struct {
	exact  map[string]*model.Category
	folded map[string][]*model.Category
	byID   map[int]*model.Category
}

func indexCategories(categories []*model.Category) categoryIndex {
	index := categoryIndex{
		exact:  make(map[string]*model.Category, len(categories)),
		folded: make(map[string][]*model.Category, len(categories)),
		byID:   make(map[int]*model.Category, len(categories)),
	}
	for _, category := range categories {
		index.exact[category.Name] = category
		key := strings.ToLower(category.Name)
		// Appended in GetAll order, which is ORDER BY name ASC. That is what
		// makes an ambiguity diagnostic deterministic without a redundant
		// sort here; if that ordering ever changes, these messages change and
		// the tests covering them fail loudly, which is the right outcome.
		index.folded[key] = append(index.folded[key], category)
		index.byID[category.CategoryID] = category
	}
	return index
}

// maxNamedCollisions bounds how many colliding categories an ambiguity
// diagnostic lists. A per-name length bound alone does not bound that message,
// because the number of collisions is itself unbounded.
const maxNamedCollisions = 3

func resolveSICCategory(
	row *sicRowValidator,
	rawName string,
	rawID string,
	categories categoryIndex,
) *int {
	name := strings.TrimSpace(rawName)
	idText := strings.TrimSpace(rawID)
	if name == "" && idText == "" {
		return nil
	}
	if name == "" {
		row.reject("Category_ID", "id_without_name", "Category_ID requires Category_Name")
		return nil
	}

	category := categories.exact[name]
	if category == nil {
		matches := categories.folded[strings.ToLower(name)]
		switch len(matches) {
		case 0:
			row.rejectCategoryNotFound(name, idText, categories)
			return nil
		case 1:
			category = matches[0]
		default:
			row.rejectAmbiguousCategory(name, matches)
			return nil
		}
	}

	if idText != "" {
		categoryID, err := strconv.Atoi(idText)
		if err != nil || categoryID <= 0 {
			row.reject("Category_ID", "invalid_category_id", "Category_ID must be a positive integer")
			return nil
		}
		if categoryID != category.CategoryID {
			row.reject("Category_ID", "category_conflict", "Category_ID does not match Category_Name")
			return nil
		}
	}

	categoryID := category.CategoryID
	return &categoryID
}

// ---------------------------------------------------------------------------
// UOW-2 mapping management
// ---------------------------------------------------------------------------

// GetPageData assembles one server render of the mapping page. Categories are
// read per request so a category added on the Categories page appears here on
// revisit. This read path deliberately does not take the mutation gate.
func (s *SICMappingService) GetPageData() (*model.SICMappingPageData, error) {
	mappings, err := s.sicRepo.GetAll()
	if err != nil {
		return nil, err
	}
	categories, err := s.categoryRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load categories for the mapping page: %w", err)
	}
	return &model.SICMappingPageData{Mappings: mappings, Categories: categories}, nil
}

// FindMappingByCode returns the mapping for a canonical code, or nil when
// none exists. It is a read and does not take the mutation gate.
func (s *SICMappingService) FindMappingByCode(rawSICCode string) (*model.SICMapping, error) {
	return s.sicRepo.GetByCode(rawSICCode)
}

// ListMappings returns all mappings in canonical numeric order.
func (s *SICMappingService) ListMappings() ([]*model.SICMapping, error) {
	return s.sicRepo.GetAll()
}

// prepareMapping normalizes and validates one create/update command.
func (s *SICMappingService) prepareMapping(input model.SICMappingInput) (*model.SICMapping, error) {
	sicCode, err := model.ParseSICCode(input.SICCode)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", model.ErrSICMappingValidation, err)
	}
	if input.CategoryID != nil {
		category, err := s.categoryRepo.GetByID(*input.CategoryID)
		if err != nil {
			return nil, err
		}
		if category == nil {
			return nil, model.ErrSICCategoryNotFound
		}
	}
	return model.NewSICMapping(
		sicCode,
		strings.TrimSpace(input.Description),
		strings.TrimSpace(input.DescriptionDetail),
		input.CategoryID,
	), nil
}

// CreateMapping adds one mapping. The collaborator runs only when the new
// mapping actually assigns a category.
func (s *SICMappingService) CreateMapping(ctx context.Context, input model.SICMappingInput) (*model.SICMappingMutationResult, error) {
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	candidate, err := s.prepareMapping(input)
	if err != nil {
		return nil, err
	}

	existing, err := s.sicRepo.GetByCode(string(candidate.SICCode))
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, model.ErrSICMappingDuplicate
	}
	if err := ensureActive(ctx); err != nil {
		return nil, err
	}
	if err := s.sicRepo.Create(candidate); err != nil {
		return nil, err
	}

	result := &model.SICMappingMutationResult{Mapping: candidate, MappingCommitted: true}
	// A new mapping with a category is a rule change. One with an empty
	// category assigns nothing and changes nothing.
	s.runPostCommit(candidate.HasCategory(), &result.RecategorizedRows, &result.PostCommitWarnings)
	logSavedOutcome(ctx, "create_sic_mapping",
		slog.Int("warnings", len(result.PostCommitWarnings)))
	return result, nil
}

// UpdateMapping changes one mapping identified by ID. The SIC code may change;
// the mapping ID stays stable and the resulting code must remain unique.
func (s *SICMappingService) UpdateMapping(ctx context.Context, id int, input model.SICMappingInput) (*model.SICMappingMutationResult, error) {
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	existing, err := s.sicRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, model.ErrSICMappingNotFound
	}

	candidate, err := s.prepareMapping(input)
	if err != nil {
		return nil, err
	}
	candidate.SICMappingID = id

	if candidate.SICCode != existing.SICCode {
		collision, err := s.sicRepo.GetByCode(string(candidate.SICCode))
		if err != nil {
			return nil, err
		}
		if collision != nil && collision.SICMappingID != id {
			return nil, model.ErrSICMappingDuplicate
		}
	}
	if err := ensureActive(ctx); err != nil {
		return nil, err
	}
	if err := s.sicRepo.Update(candidate); err != nil {
		return nil, err
	}

	result := &model.SICMappingMutationResult{Mapping: candidate, MappingCommitted: true}
	// A code change or a category change alters what this mapping assigns; a
	// description-only edit does not (BR-U5-04).
	//
	// The HasCategory() condition this replaced is deliberately gone. Clearing
	// a mapping's category used to hand off nothing, on the reasoning that an
	// empty mapping assigns nothing and so could not affect anyone. Under FR15
	// it very much can: transactions that mapping had categorized must now be
	// re-examined, and most will become uncategorized.
	codeChanged := candidate.SICCode != existing.SICCode
	categoryChanged := !sameCategoryID(candidate.CategoryID, existing.CategoryID)
	s.runPostCommit(codeChanged || categoryChanged, &result.RecategorizedRows, &result.PostCommitWarnings)
	logSavedOutcome(ctx, "update_sic_mapping",
		slog.Int("warnings", len(result.PostCommitWarnings)))
	return result, nil
}

// DeleteMapping removes one mapping. Transactions that mapping had categorized
// are re-examined afterwards and follow whatever the remaining rules say, or
// become uncategorized if nothing claims them.
//
// This reverses the UOW-2 behaviour recorded here, where deletion handed off
// nothing because assigned categories were never cleared. FR14's bullet saying
// so was struck by UOW-5: leaving a category assigned by a mapping that no
// longer exists is exactly the history-dependence FR15 forbids.
func (s *SICMappingService) DeleteMapping(ctx context.Context, id int) (*model.SICMappingMutationResult, error) {
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	if err := ensureActive(ctx); err != nil {
		return nil, err
	}
	if err := s.sicRepo.Delete(id); err != nil {
		return nil, err
	}
	result := &model.SICMappingMutationResult{MappingCommitted: true}
	s.runPostCommit(true, &result.RecategorizedRows, &result.PostCommitWarnings)
	logSavedOutcome(ctx, "delete_sic_mapping",
		slog.Int("warnings", len(result.PostCommitWarnings)))
	return result, nil
}

// runPostCommit performs reload and handoff after a durable write. Everything
// here happens after the data is committed, so failures become warnings on a
// successful result and never imply the caller should retry the mutation.
//
// A reload failure skips recategorization: handing codes to a collaborator
// working from stale rules would categorize against the pre-change state.
// logSavedOutcome records a durable result whose caller has disconnected.
// Without this the only record of a committed change would be a response
// nobody received. Only counts and safe outcome fields are logged - never
// uploaded rows, descriptions, or financial data.
func logSavedOutcome(ctx context.Context, operation string, attrs ...any) {
	if ctx.Err() == nil {
		return
	}
	slog.Warn("SIC mapping change committed but the response could not be delivered",
		append([]any{
			slog.String("operation", operation),
			slog.Bool("mapping_committed", true),
		}, attrs...)...)
}

// rulesChanged says whether a committed mapping write could alter any
// categorization. It replaces the affected-code set, which described a scope
// nothing uses now: under FR15 the question is no longer "which codes" but
// simply "is a re-examination warranted".
//
// It is still worth asking. BR-U5-04 keeps the one exclusion that survives —
// a description-only edit changes no categorization, so a full pass there
// would be a provable no-op.
func (s *SICMappingService) runPostCommit(rulesChanged bool, recategorized *int, warnings *[]string) {
	if err := s.collaborator.ReloadMappings(); err != nil {
		*warnings = append(*warnings,
			fmt.Sprintf("Mappings were saved, but reloading categorization rules failed: %v", err))
		return
	}
	if !rulesChanged {
		return
	}
	counts, err := s.collaborator.Reexamine()
	if err != nil {
		*warnings = append(*warnings,
			fmt.Sprintf("Mappings were saved, but re-examining transactions failed: %v", err))
		return
	}
	// RecategorizedRows keeps its meaning: transactions this change moved.
	*recategorized = counts.Moved
}

// ensureActive reports whether the caller is still waiting for this result.
// It is checked at phase boundaries after any blocking read, so work that was
// abandoned while waiting on a database connection does not go on to mutate.
func ensureActive(ctx context.Context) error {
	if ctx.Err() != nil {
		return model.ErrSICMappingCancelled
	}
	return nil
}

func sameCategoryID(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ---------------------------------------------------------------------------
// CSV export and backup
// ---------------------------------------------------------------------------

// ExportCSV writes the authoritative mapping set. An empty database produces
// the header alone.
func (s *SICMappingService) ExportCSV(w io.Writer) error {
	mappings, err := s.sicRepo.GetAll()
	if err != nil {
		return err
	}
	return writeSICMappingCSV(w, mappings)
}

func writeSICMappingCSV(w io.Writer, mappings []*model.SICMapping) error {
	writer := csv.NewWriter(w)
	if err := writer.Write(sicMappingCSVHeader); err != nil {
		return fmt.Errorf("failed to write SIC mapping header: %w", err)
	}
	for _, mapping := range mappings {
		categoryName := ""
		if mapping.CategoryName != nil {
			categoryName = *mapping.CategoryName
		}
		categoryID := ""
		if mapping.CategoryID != nil {
			categoryID = strconv.Itoa(*mapping.CategoryID)
		}
		record := []string{
			string(mapping.SICCode),
			mapping.Description,
			mapping.DescriptionDetail,
			categoryName,
			categoryID,
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write SIC mapping row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("failed to flush SIC mapping CSV: %w", err)
	}
	return nil
}

// writeBackup serializes the pre-merge snapshot beside the database.
//
// os.CreateTemp creates the file exclusively at mode 0600 and fills in the
// random segment, so an existing file or symlink is never opened for
// overwrite and two backups in the same second cannot collide. The path is
// returned only after every write, flush, and close has succeeded; a partial
// file is removed rather than advertised as a backup.
func (s *SICMappingService) writeBackup(mappings []*model.SICMapping) (string, error) {
	if s.dataDir == "" {
		return "", fmt.Errorf("no application data directory is configured for SIC mapping backups")
	}
	pattern := fmt.Sprintf("sic_mappings.backup-%s-*.csv", time.Now().UTC().Format("20060102T150405Z"))
	file, err := os.CreateTemp(s.dataDir, pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create SIC mapping backup: %w", err)
	}
	path := file.Name()

	if err := writeSICMappingCSV(file, mappings); err != nil {
		return "", discardPartialBackup(file, path, err)
	}
	if err := file.Close(); err != nil {
		return "", discardPartialBackup(nil, path, fmt.Errorf("failed to close SIC mapping backup: %w", err))
	}
	return path, nil
}

// discardPartialBackup removes an incomplete backup so it can never be mistaken
// for a usable one. If removal also fails the caller is told the file remains.
func discardPartialBackup(file *os.File, path string, cause error) error {
	if file != nil {
		_ = file.Close()
	}
	if removeErr := os.Remove(path); removeErr != nil {
		return fmt.Errorf("%w; an incomplete backup file remains at %s", cause, filepath.Base(path))
	}
	return cause
}

// ---------------------------------------------------------------------------
// CSV merge upload
// ---------------------------------------------------------------------------

// MergeUpload validates an entire uploaded mapping CSV, backs up the current
// mappings, and merges the rows atomically.
//
// Codes absent from the upload are left untouched: this adds and updates, it
// never deletes. Nothing is written until the whole file validates, so one bad
// row costs the caller nothing but the error report.
func (s *SICMappingService) MergeUpload(ctx context.Context, reader io.Reader) (*model.SICMappingImportResult, error) {
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	candidates, report, err := s.ValidateCSV(reader)
	if err != nil {
		return &model.SICMappingImportResult{SICMappingImportReport: *report}, err
	}
	if report.RejectedRows > 0 {
		report.Outcome = model.SICMappingImportInvalid
		return &model.SICMappingImportResult{SICMappingImportReport: *report}, nil
	}

	// Validation consumes the whole upload, so re-check before doing anything
	// with side effects. A caller who gave up during validation must not leave
	// a backup file behind.
	if err := ensureActive(ctx); err != nil {
		return nil, err
	}

	result := &model.SICMappingImportResult{SICMappingImportReport: *report}

	// One snapshot serves both the diff and the backup, so a backup failure
	// can never change the counts we report.
	preState, err := s.sicRepo.GetAll()
	if err != nil {
		result.Outcome = model.SICMappingImportPersistenceFailed
		return result, err
	}
	previous := make(map[model.SICCode]*model.SICMapping, len(preState))
	for _, mapping := range preState {
		previous[mapping.SICCode] = mapping
	}

	created, updated, unchanged, rulesChanged := diffSICMappings(previous, candidates)

	if err := ensureActive(ctx); err != nil {
		return nil, err
	}

	if backupPath, backupErr := s.writeBackup(preState); backupErr != nil {
		result.BackupWarning = fmt.Sprintf("Existing mappings could not be backed up: %v", backupErr)
	} else {
		result.BackupPath = backupPath
	}

	if err := s.sicRepo.MergeAll(ctx, candidates); err != nil {
		result.Outcome = model.SICMappingImportPersistenceFailed
		return result, err
	}

	result.Outcome = model.SICMappingImportMerged
	result.MappingCommitted = true
	result.CreatedRows = created
	result.UpdatedRows = updated
	result.UnchangedRows = unchanged

	s.runPostCommit(rulesChanged, &result.RecategorizedRows, &result.PostCommitWarnings)
	logSavedOutcome(ctx, "merge_sic_mappings",
		slog.Int("created_rows", result.CreatedRows),
		slog.Int("updated_rows", result.UpdatedRows),
		slog.Int("unchanged_rows", result.UnchangedRows),
		slog.Bool("backup_written", result.BackupPath != ""),
		slog.Int("warnings", len(result.PostCommitWarnings)))
	return result, nil
}

// diffSICMappings classifies each uploaded row against the pre-merge state and
// reports whether any of them changed what the rules assign.
//
// It used to collect a set of affected codes. That set described a scope
// nothing consumes now: under FR15 a rule change re-examines everything the
// rules govern, so the only question left is whether a re-examination is
// warranted at all.
//
// One exclusion survives, and only one. A description-only edit changes no
// categorization, so a merge containing nothing else is a provable no-op
// (BR-U5-04). Clearing a category no longer qualifies: an emptied mapping
// assigns nothing, which means the transactions it had categorized must be
// re-examined and will mostly become uncategorized.
func diffSICMappings(
	previous map[model.SICCode]*model.SICMapping,
	candidates []*model.SICMapping,
) (created, updated, unchanged int, rulesChanged bool) {
	for _, candidate := range candidates {
		prior, existed := previous[candidate.SICCode]
		switch {
		case !existed:
			created++
			if candidate.HasCategory() {
				rulesChanged = true
			}
		case prior.Description == candidate.Description &&
			prior.DescriptionDetail == candidate.DescriptionDetail &&
			sameCategoryID(prior.CategoryID, candidate.CategoryID):
			unchanged++
		default:
			updated++
			if !sameCategoryID(prior.CategoryID, candidate.CategoryID) {
				rulesChanged = true
			}
		}
	}

	return created, updated, unchanged, rulesChanged
}
