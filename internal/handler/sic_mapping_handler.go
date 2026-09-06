package handler

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/service"
)

const (
	// multipartFramingAllowance is the headroom granted above the CSV limit for
	// multipart boundaries and part headers, so a file exactly at the limit is
	// still accepted with ordinary framing.
	multipartFramingAllowance int64 = 1 << 20

	// multipartMemoryThreshold bounds how much of the parsed form is held in
	// memory before the parser spills to a temporary file. It bounds memory,
	// not the total accepted size.
	multipartMemoryThreshold int64 = 1 << 20
)

// SICMappingHandler serves the SIC mapping API. It depends only on the
// service: transport decoding and status mapping live here, while validation
// and persistence decisions stay behind the service boundary.
type SICMappingHandler struct {
	sicMappingService *service.SICMappingService
}

// NewSICMappingHandler creates a new SICMappingHandler.
func NewSICMappingHandler(sicMappingService *service.SICMappingService) *SICMappingHandler {
	return &SICMappingHandler{sicMappingService: sicMappingService}
}

// List returns all mappings in canonical numeric order.
// GET /api/sic-mappings
func (h *SICMappingHandler) List(c *gin.Context) {
	mappings, err := h.sicMappingService.ListMappings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to load SIC mappings",
			"code":  "internal_error",
		})
		return
	}
	c.JSON(http.StatusOK, mappings)
}

// Create adds one mapping.
// POST /api/sic-mappings
func (h *SICMappingHandler) Create(c *gin.Context) {
	var input model.SICMappingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid SIC mapping payload",
			"code":  "invalid_request",
		})
		return
	}

	result, err := h.sicMappingService.CreateMapping(c.Request.Context(), input)
	if err != nil {
		h.writeMutationError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

// Update changes one mapping identified by ID.
// PUT /api/sic-mappings/:id
func (h *SICMappingHandler) Update(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	var input model.SICMappingInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid SIC mapping payload",
			"code":  "invalid_request",
		})
		return
	}

	result, err := h.sicMappingService.UpdateMapping(c.Request.Context(), id, input)
	if err != nil {
		h.writeMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// Delete removes one mapping. It returns a body so that any post-commit
// warning stays visible to the page.
// DELETE /api/sic-mappings/:id
func (h *SICMappingHandler) Delete(c *gin.Context) {
	id, ok := h.parseID(c)
	if !ok {
		return
	}
	result, err := h.sicMappingService.DeleteMapping(c.Request.Context(), id)
	if err != nil {
		h.writeMutationError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// Download streams the authoritative mapping set as a CSV attachment.
// GET /api/sic-mappings/download
func (h *SICMappingHandler) Download(c *gin.Context) {
	// Render into a buffer first. Writing straight to the response would
	// commit a 200 and an attachment before a repository failure surfaced,
	// so a failed export would download as an empty but apparently valid
	// backup file. A valid empty export still carries the header row.
	var buf bytes.Buffer
	if err := h.sicMappingService.ExportCSV(&buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to export SIC mappings. No file was produced.",
			"code":  "internal_error",
		})
		return
	}

	c.Header("Content-Disposition", `attachment; filename="sic_mappings.csv"`)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// Upload merges an uploaded mapping CSV into the existing mappings.
// POST /api/sic-mappings/upload
//
// Codes present in the file are added or updated; codes absent from it are
// left untouched. Upload never deletes.
func (h *SICMappingHandler) Upload(c *gin.Context) {
	// Bound the whole request body before any parsing, so an oversized upload
	// is rejected without being buffered. Client Content-Length is never
	// trusted as enforcement.
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		model.MaxSICMappingFileSize+multipartFramingAllowance,
	)

	if err := c.Request.ParseMultipartForm(multipartMemoryThreshold); err != nil {
		defer h.cleanupMultipart(c)
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.writeOversized(c)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Upload must be a valid multipart form",
			"code":  "invalid_request",
		})
		return
	}
	defer h.cleanupMultipart(c)

	// Count parts across every file field, not just "file". Checking only the
	// expected field would silently accept a request carrying additional
	// uploads under other names.
	totalFiles := 0
	for _, parts := range c.Request.MultipartForm.File {
		totalFiles += len(parts)
	}
	files := c.Request.MultipartForm.File["file"]
	if totalFiles != 1 || len(files) != 1 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Exactly one CSV file must be uploaded, in the \"file\" field",
			"code":  "invalid_request",
		})
		return
	}

	// The parser has now established the real size, so enforce the shared CSV
	// bound before handing anything to validation.
	header := files[0]
	if header.Size > model.MaxSICMappingFileSize {
		h.writeOversized(c)
		return
	}

	file, err := header.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to open the uploaded file",
			"code":  "internal_error",
		})
		return
	}
	defer file.Close()

	result, err := h.sicMappingService.MergeUpload(c.Request.Context(), file)
	if err != nil {
		h.writeUploadError(c, result, err)
		return
	}
	if result.Outcome == model.SICMappingImportInvalid {
		c.JSON(http.StatusUnprocessableEntity, result)
		return
	}
	c.JSON(http.StatusOK, result)
}

// cleanupMultipart removes any temporary files the parser spilled to disk.
func (h *SICMappingHandler) cleanupMultipart(c *gin.Context) {
	if c.Request.MultipartForm != nil {
		_ = c.Request.MultipartForm.RemoveAll()
	}
}

func (h *SICMappingHandler) writeOversized(c *gin.Context) {
	c.JSON(http.StatusRequestEntityTooLarge, gin.H{
		"error": fmt.Sprintf(
			"The mapping file exceeds the %d MiB limit. Nothing was parsed and no mappings changed.",
			model.MaxSICMappingFileSize/(1<<20),
		),
		"code":    "oversized",
		"outcome": string(model.SICMappingImportOversized),
	})
}

func (h *SICMappingHandler) parseID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid SIC mapping ID",
			"code":  "invalid_request",
		})
		return 0, false
	}
	return id, true
}

// writeMutationError maps a CRUD failure to its transport status. Every branch
// here keys off a stable sentinel, never driver text.
func (h *SICMappingHandler) writeMutationError(c *gin.Context, err error) {
	status, code := classifySICMappingFailure(err)
	if code == "mapping_busy" {
		c.Header("Retry-After", "1")
	}
	c.JSON(status, gin.H{
		"error": sicMappingFailureMessage(err, code, "Failed to save the SIC mapping."),
		"code":  code,
	})
}

// writeUploadError maps an upload failure, preserving the partial result so a
// backup outcome recorded before the failure is not lost.
func (h *SICMappingHandler) writeUploadError(c *gin.Context, result *model.SICMappingImportResult, err error) {
	status, code := classifySICMappingFailure(err)
	if code == "mapping_busy" {
		c.Header("Retry-After", "1")
	}
	body := gin.H{
		"code":  code,
		"error": sicMappingFailureMessage(err, code, "Failed to merge the uploaded mappings. No mappings were changed."),
	}
	if result != nil {
		body["result"] = result
	}
	c.JSON(status, body)
}

// sicMappingFailureMessage produces the client-facing text for a failure.
// Recognized classes get their own wording; anything unrecognized falls back
// to a generic message so wrapped internal or driver detail is never returned
// to the browser.
func sicMappingFailureMessage(err error, code, fallback string) string {
	switch code {
	case "mapping_busy":
		return "Another mapping change is in progress. Nothing was changed - please try again."
	case "request_cancelled":
		return "The request was cancelled before any change was made."
	case "duplicate_sic_code":
		return "A mapping already exists for that SIC code."
	case "not_found":
		return "That SIC mapping no longer exists."
	case "category_not_found":
		return "The selected category no longer exists."
	case "database_busy":
		return "The database is busy. Nothing was changed - please try again."
	case "validation_failed":
		// Validation text comes from the domain and describes the user's own
		// input, so it is safe to show verbatim.
		return err.Error()
	case "oversized":
		return fmt.Sprintf("The mapping file exceeds the %d MiB limit.", model.MaxSICMappingFileSize/(1<<20))
	default:
		return fallback
	}
}

// classifySICMappingFailure turns a service error into a status and a stable
// error code.
func classifySICMappingFailure(err error) (int, string) {
	switch {
	case errors.Is(err, model.ErrSICMappingBusy):
		return http.StatusServiceUnavailable, "mapping_busy"
	case errors.Is(err, model.ErrSICMappingCancelled):
		return http.StatusRequestTimeout, "request_cancelled"
	case errors.Is(err, model.ErrSICMappingDuplicate):
		return http.StatusConflict, "duplicate_sic_code"
	case errors.Is(err, model.ErrSICMappingNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, model.ErrSICCategoryNotFound):
		return http.StatusUnprocessableEntity, "category_not_found"
	case errors.Is(err, model.ErrSICMappingValidation):
		return http.StatusUnprocessableEntity, "validation_failed"
	case errors.Is(err, model.ErrSICMappingDatabaseBusy):
		return http.StatusServiceUnavailable, "database_busy"
	case errors.Is(err, model.ErrSICMappingOversized):
		return http.StatusRequestEntityTooLarge, "oversized"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}
