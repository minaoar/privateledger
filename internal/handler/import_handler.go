package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/service"
)

// ImportHandler handles OFX import-related HTTP requests
type ImportHandler struct {
	importService *service.ImportService
}

// NewImportHandler creates a new ImportHandler
func NewImportHandler(importService *service.ImportService) *ImportHandler {
	return &ImportHandler{
		importService: importService,
	}
}

// ImportOFX handles OFX file upload and import
// POST /api/import
// Content-Type: multipart/form-data
// Fields:
//   - file: OFX file
//   - account_id: Target account ID
func (h *ImportHandler) ImportOFX(c *gin.Context) {
	// Get account_id from form
	accountIDStr := c.PostForm("account_id")
	if accountIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id is required"})
		return
	}

	accountID, err := strconv.Atoi(accountIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account_id"})
		return
	}

	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}

	// Validate file extension (optional but good practice)
	// OFX files can be .ofx, .qfx, or sometimes .xml
	// We'll allow any extension and rely on content validation

	// Open file
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer src.Close()

	// Import transactions
	result, err := h.importService.ImportOFX(src, accountID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ValidateOFX validates an OFX file without importing
// POST /api/import/validate
// Content-Type: multipart/form-data
// Fields:
//   - file: OFX file
func (h *ImportHandler) ValidateOFX(c *gin.Context) {
	// Get uploaded file
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File is required"})
		return
	}

	// Open file
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer src.Close()

	// Validate
	err = h.importService.ValidateOFX(src)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "valid": false})
		return
	}

	c.JSON(http.StatusOK, gin.H{"valid": true, "message": "OFX file is valid"})
}
