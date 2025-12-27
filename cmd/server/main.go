package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/config"
	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/handler"
	"github.com/oronno/privateledger/internal/parser"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

const (
	defaultConfigFile = "config.json"
	defaultDBFile     = "privateledger.db"
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

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("PrivateLedger v0.1.0")
	log.Printf("Config loaded from: %s", configPath)
	log.Printf("Database: %s", dbPath)

	// Open database
	db, err := database.Open(database.Config{Path: dbPath})
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close(db)

	log.Printf("Database initialized successfully")

	// Initialize repositories
	accountRepo := repository.NewAccountRepository(db)
	transactionRepo := repository.NewTransactionRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	patternRepo := repository.NewCategoryPatternRepository(db)

	// Initialize services
	ofxParser := parser.NewOFXParser()
	categorizer := service.NewCategorizer(patternRepo, transactionRepo)
	importService := service.NewImportService(ofxParser, transactionRepo, accountRepo, categorizer)
	insightsService := service.NewInsightsService(transactionRepo, categoryRepo, cfg)

	// Load patterns for categorizer
	if err := categorizer.LoadPatterns(); err != nil {
		log.Printf("Warning: Failed to load categorizer patterns: %v", err)
	}

	// Initialize handlers
	accountHandler := handler.NewAccountHandler(accountRepo)
	transactionHandler := handler.NewTransactionHandler(transactionRepo)
	categoryHandler := handler.NewCategoryHandler(categoryRepo, patternRepo, categorizer)
	importHandler := handler.NewImportHandler(importService)
	insightsHandler := handler.NewInsightsHandler(insightsService, accountRepo)

	// Initialize Gin router
	if !gin.IsDebugging() {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

	// API routes
	api := router.Group("/api")
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

		// Insights routes
		api.GET("/insights/dashboard", insightsHandler.GetDashboard)
		api.GET("/insights/monthly", insightsHandler.GetMonthlySummary)
		api.GET("/insights/trends", insightsHandler.GetTrends)
		api.GET("/insights/current-period", insightsHandler.GetCurrentPeriod)
	}

	// Root route - API info
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"name":    "PrivateLedger API",
			"version": "0.1.0",
			"endpoints": gin.H{
				"accounts":     "/api/accounts",
				"transactions": "/api/transactions",
				"categories":   "/api/categories",
				"import":       "/api/import",
				"insights":     "/api/insights",
			},
		})
	})

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Starting server on http://localhost%s", addr)

	// Auto-open browser if configured
	if cfg.Server.AutoOpenBrowser {
		go openBrowser(fmt.Sprintf("http://localhost%s", addr))
	}

	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
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
		log.Printf("Unable to open browser on OS: %s", runtime.GOOS)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open browser: %v", err)
	}
}
