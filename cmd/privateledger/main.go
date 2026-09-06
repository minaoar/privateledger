package main

import (
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/config"
	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/handler"
	"github.com/oronno/privateledger/internal/logger"
	"github.com/oronno/privateledger/internal/middleware"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/parser"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

const (
	defaultConfigFile     = "config.json"
	defaultDBFile         = "privateledger.db"
	defaultSICMappingFile = "sic_mappings.csv"

	// maxSICSeedDiagnostics bounds how many rejected seed rows are logged at
	// startup. It is an alias of the shared domain bound rather than its own
	// literal, so the startup log and the upload response cannot drift apart.
	maxSICSeedDiagnostics = model.MaxSICMappingDiagnostics

	// sicAdmissionTimeout bounds how long a competing SIC mapping mutation
	// waits for the service gate before the caller is told the service is busy.
	sicAdmissionTimeout = 5 * time.Second
)

var (
	// Version is the application version (injected at build time)
	Version = "dev"
	// BuildTime is the build timestamp (injected at build time)
	BuildTime = "unknown"
)

func main() {
	// Determine config and database paths (relative to executable)
	execPath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v", err)
	}
	execDir := filepath.Dir(execPath)
	configPath := filepath.Join(execDir, defaultConfigFile)
	dbPath := filepath.Join(execDir, defaultDBFile)
	sicMappingPath := filepath.Join(execDir, defaultSICMappingFile)

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup structured logging
	logFile, err := logger.Setup(cfg, execDir)
	if err != nil {
		log.Fatalf("Failed to setup logger: %v", err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	slog.Info("PrivateLedger starting",
		slog.String("version", Version),
		slog.String("build_time", BuildTime),
		slog.String("config_path", configPath),
		slog.String("database_path", dbPath),
		slog.Bool("file_logging", cfg.Logging.EnableFileLogging),
		slog.String("log_level", cfg.Logging.LogLevel),
	)

	// Open database
	db, err := database.Open(database.Config{Path: dbPath})
	if err != nil {
		slog.Error("Failed to open database", slog.String("error", err.Error()))
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close(db)

	slog.Info("Database initialized successfully")

	// Initialize repositories
	accountRepo := repository.NewAccountRepository(db)
	transactionRepo := repository.NewTransactionRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	patternRepo := repository.NewCategoryPatternRepository(db)
	importBatchRepo := repository.NewImportBatchRepository(db)
	sicMappingRepo := repository.NewSICMappingRepository(db)

	// Initialize services
	ofxParser := parser.NewOFXParser()
	categorizer := service.NewCategorizer(patternRepo, transactionRepo)
	importService := service.NewImportService(ofxParser, transactionRepo, accountRepo, categorizer, importBatchRepo)
	insightsService := service.NewInsightsService(transactionRepo, categoryRepo, cfg)
	// The no-op recategorization collaborator is supplied explicitly: UOW-3
	// replaces it with the real adapter, and passing nil would let "did no
	// work" be mistaken for "worked".
	sicMappingService := service.NewSICMappingManagementService(
		sicMappingRepo,
		categoryRepo,
		execDir,
		sicAdmissionTimeout,
		service.NewNoopSICRecategorizationCollaborator(),
	)

	sicImportReport, err := sicMappingService.ImportFileIfPresentWithReport(sicMappingPath)
	if err != nil {
		attributes := []any{slog.String("error", err.Error())}
		if sicImportReport != nil {
			attributes = append(attributes, slog.String("outcome", string(sicImportReport.Outcome)))
		}
		slog.Error("Failed to initialize SIC mappings", attributes...)
		log.Fatalf("Failed to initialize SIC mappings: %v", err)
	}
	logSICMappingImportOutcome(sicMappingPath, sicImportReport)

	// Load patterns for categorizer
	if err := categorizer.LoadPatterns(); err != nil {
		slog.Warn("Failed to load categorizer patterns", slog.String("error", err.Error()))
	}

	// Initialize handlers
	accountHandler := handler.NewAccountHandler(accountRepo)
	transactionHandler := handler.NewTransactionHandler(transactionRepo)
	categoryHandler := handler.NewCategoryHandler(categoryRepo, patternRepo, categorizer)
	importHandler := handler.NewImportHandler(importService)
	importBatchHandler := handler.NewImportBatchHandler(importBatchRepo, importService)
	insightsHandler := handler.NewInsightsHandler(insightsService, accountRepo)
	sicMappingHandler := handler.NewSICMappingHandler(sicMappingService)
	pageHandler := handler.NewPageHandler(embeddedFiles, accountRepo, transactionRepo, categoryRepo, patternRepo, insightsService, sicMappingService, Version)

	// Initialize Gin router
	if cfg.DebugMode {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
		// Disable Gin's default logger output to console
		gin.DisableConsoleColor()
	}

	router := gin.New()

	// Add global middleware
	router.Use(gin.Recovery())

	// API routes with logging middleware
	api := router.Group("/api")
	api.Use(middleware.LoggingMiddleware())
	{
		// Account routes
		api.GET("/accounts", accountHandler.ListAccounts)
		api.GET("/accounts/:id", accountHandler.GetAccount)
		api.POST("/accounts", accountHandler.CreateAccount)
		api.PUT("/accounts/:id", accountHandler.UpdateAccount)
		api.DELETE("/accounts/:id", accountHandler.DeleteAccount)

		// Transaction routes
		api.GET("/transactions", transactionHandler.ListTransactions)
		api.GET("/transactions/:id", transactionHandler.GetTransaction)
		api.PATCH("/transactions/:id/category", transactionHandler.UpdateTransactionCategory)
		api.DELETE("/transactions/:id", transactionHandler.DeleteTransaction)

		// Category routes
		api.GET("/categories", categoryHandler.ListCategories)
		api.GET("/categories/:id", categoryHandler.GetCategory)
		api.POST("/categories", categoryHandler.CreateCategory)
		api.PUT("/categories/:id", categoryHandler.UpdateCategory)
		api.DELETE("/categories/:id", categoryHandler.DeleteCategory)
		api.POST("/categories/recategorize", categoryHandler.RecategorizeAll)

		// Pattern routes
		api.POST("/categories/:id/patterns", categoryHandler.AddPattern)
		api.DELETE("/patterns/:id", categoryHandler.DeletePattern)

		// Import routes
		api.POST("/import", importHandler.ImportOFX)
		api.POST("/import/validate", importHandler.ValidateOFX)

		// Import batch/history routes
		api.POST("/import/history", importBatchHandler.CreateBatch)
		api.GET("/import/history", importBatchHandler.ListBatches)
		api.GET("/import/history/:id", importBatchHandler.GetBatch)
		api.PUT("/import/history/:id", importBatchHandler.UpdateBatch)
		api.DELETE("/import/history/:id", importBatchHandler.DeleteBatch)

		// Insights routes
		api.GET("/insights/dashboard", insightsHandler.GetDashboard)
		api.GET("/insights/monthly", insightsHandler.GetMonthlySummary)
		api.GET("/insights/current-period", insightsHandler.GetCurrentPeriod)

		// SIC mapping management
		api.GET("/sic-mappings", sicMappingHandler.List)
		api.POST("/sic-mappings", sicMappingHandler.Create)
		api.PUT("/sic-mappings/:id", sicMappingHandler.Update)
		api.DELETE("/sic-mappings/:id", sicMappingHandler.Delete)
		api.GET("/sic-mappings/download", sicMappingHandler.Download)
		api.POST("/sic-mappings/upload", sicMappingHandler.Upload)
	}

	// Serve static files
	staticFS, _ := fs.Sub(embeddedFiles, "web/static")
	router.StaticFS("/static", http.FS(staticFS))

	// Page routes (HTML)
	router.GET("/", pageHandler.Dashboard)
	router.GET("/onboarding", pageHandler.Onboarding)
	router.GET("/accounts", pageHandler.Accounts)
	router.GET("/categories", pageHandler.Categories)
	router.GET("/sic-mappings", pageHandler.SICMappings)
	router.GET("/transactions", pageHandler.Transactions)
	router.GET("/import", pageHandler.Import)
	router.GET("/how-to-download", pageHandler.HowToDownload)
	router.GET("/about", pageHandler.About)

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	serverURL := fmt.Sprintf("http://localhost%s", addr)

	slog.Info("Starting server",
		slog.String("address", addr),
		slog.String("url", serverURL),
		slog.Bool("auto_open_browser", cfg.Server.AutoOpenBrowser),
	)

	// Auto-open browser if configured
	if cfg.Server.AutoOpenBrowser {
		go openBrowser(serverURL)
	}

	if err := router.Run(addr); err != nil {
		slog.Error("Failed to start server", slog.String("error", err.Error()))
		log.Fatalf("Failed to start server: %v", err)
	}
}

func logSICMappingImportOutcome(path string, report *model.SICMappingImportReport) {
	if report == nil {
		return
	}
	switch report.Outcome {
	case model.SICMappingImportAbsent:
		return
	case model.SICMappingImportSkippedExisting:
		slog.Info("Skipping SIC mapping seed because SQLite mappings already exist",
			slog.String("path", path),
			slog.Int("existing_mappings", report.ExistingRows))
	case model.SICMappingImportOversized:
		slog.Warn("Rejecting oversized SIC mapping seed",
			slog.String("path", path),
			slog.Int64("maximum_bytes", model.MaxSICMappingFileSize))
	case model.SICMappingImportInvalid:
		limit := len(report.Errors)
		if limit > maxSICSeedDiagnostics {
			limit = maxSICSeedDiagnostics
		}
		for _, validationErr := range report.Errors[:limit] {
			slog.Warn("Rejected SIC mapping seed row",
				slog.String("path", path),
				slog.Int("row", validationErr.RowNumber),
				slog.String("field", validationErr.Field),
				slog.String("code", validationErr.Code))
		}
		slog.Warn("SIC mapping seed validation failed; no mappings imported",
			slog.String("path", path),
			slog.Int("total_rows", report.TotalRows),
			slog.Int("rejected_rows", report.RejectedRows),
			slog.Int("reported_diagnostics", limit),
			slog.Int("omitted_diagnostics", len(report.Errors)-limit))
	case model.SICMappingImportImported:
		slog.Info("Imported SIC mapping seed",
			slog.String("path", path),
			slog.Int("imported_rows", report.ImportedRows))
	}
}

// openBrowser attempts to open the default browser to the given URL
func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		slog.Warn("Unable to open browser", slog.String("os", runtime.GOOS))
		return
	}

	if err := cmd.Start(); err != nil {
		slog.Warn("Failed to open browser", slog.String("error", err.Error()))
	}
}
