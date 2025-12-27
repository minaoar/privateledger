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

	// Initialize Gin router
	if !gin.IsDebugging() {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.Default()

	// TODO: Register routes here
	// - Page routes (/, /transactions, /accounts, /categories, /import)
	// - API routes (/api/...)

	// Placeholder route
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "PrivateLedger API",
			"version": "0.1.0",
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
