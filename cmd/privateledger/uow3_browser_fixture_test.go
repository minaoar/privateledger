//go:build browserreview

package main

import (
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/handler"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

// TestUOW3BrowserFixtureServer serves the production templates and handlers
// against an isolated temporary ledger for browser-driven review. It is
// excluded from ordinary test runs and stopped by the review harness.
func TestUOW3BrowserFixtureServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	db, err := database.Open(database.Config{Path: filepath.Join(dir, "browser-review.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	insert := func(query string, args ...any) int {
		t.Helper()
		result, err := db.Exec(query, args...)
		if err != nil {
			t.Fatal(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return int(id)
	}
	accountID := insert(`INSERT INTO account (name) VALUES ('Browser Review')`)
	diningID := insert(`INSERT INTO category (name, category_type, color, icon) VALUES ('Dining', 2, '#008000', 'cup')`)
	travelID := insert(`INSERT INTO category (name, category_type, color, icon) VALUES ('Travel', 2, '#0000ff', 'airplane')`)

	txnRepo := repository.NewTransactionRepository(db)
	sicRepo := repository.NewSICMappingRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	patternRepo := repository.NewCategoryPatternRepository(db)
	for _, mapping := range []*model.SICMapping{
		model.NewSICMapping("5812", "Primary Description", "ignored detail", &diningID),
		model.NewSICMapping("7011", "", "Fallback Detail", nil),
		model.NewSICMapping("7999", "", "", nil),
	} {
		if err := sicRepo.Create(mapping); err != nil {
			t.Fatal(err)
		}
	}
	createTxn := func(fitID, details, code string) int {
		t.Helper()
		txn := &model.Transaction{
			AccountID: accountID, TrnType: "DEBIT", FitID: fitID,
			DatePosted: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC), Amount: -10,
			TransactionDetails: details, TransactionType: model.TransactionTypeDebit,
			CategorySource: model.CategorySourceNone,
		}
		if code != "" {
			sic := model.SICCode(code)
			txn.SICCode = &sic
		}
		if err := txnRepo.Create(txn); err != nil {
			t.Fatal(err)
		}
		return txn.TransactionID
	}
	primaryTxn := createTxn("browser-primary", "CAFE PRIMARY", "5812")
	fallbackTxn := createTxn("browser-fallback", "HOTEL FALLBACK", "7011")
	bareTxn := createTxn("browser-bare", "BARE CODE", "7999")
	noSICTxn := createTxn("browser-none", "NO SIC", "")

	sicCategorizer := service.NewSICMappingCategorizer(sicRepo, txnRepo)
	categorizer := service.NewCategorizerWithSIC(patternRepo, txnRepo, sicCategorizer)
	if err := categorizer.LoadRules(); err != nil {
		t.Fatal(err)
	}
	mappingService := service.NewSICMappingManagementService(sicRepo, categoryRepo, dir, time.Second, sicCategorizer)
	transactionHandler := handler.NewTransactionHandlerWithSIC(txnRepo, mappingService)
	categoryHandler := handler.NewCategoryHandler(categoryRepo, patternRepo, categorizer)
	pageHandler := handler.NewPageHandler(embeddedFiles,
		repository.NewAccountRepository(db), txnRepo, categoryRepo, patternRepo, nil, mappingService, "uow3-review")

	router := gin.New()
	router.Use(gin.Recovery())
	staticFS, _ := fs.Sub(embeddedFiles, "web/static")
	router.StaticFS("/static", http.FS(staticFS))
	router.GET("/transactions", pageHandler.Transactions)
	router.GET("/categories", pageHandler.Categories)
	router.PATCH("/api/transactions/:id/category", func(c *gin.Context) {
		time.Sleep(3 * time.Second)
		transactionHandler.UpdateTransactionCategory(c)
	})
	router.POST("/api/transactions/:id/sic-mapping", func(c *gin.Context) {
		time.Sleep(3 * time.Second)
		if c.Param("id") == fmt.Sprint(bareTxn) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "forced browser-review failure"})
			return
		}
		transactionHandler.CreateSICMappingForTransaction(c)
	})
	router.POST("/api/categories/:id/patterns", categoryHandler.AddPattern)
	router.POST("/api/categories/recategorize", categoryHandler.RecategorizeAll)
	router.GET("/api/review/transactions/:id", transactionHandler.GetTransaction)

	listener, err := net.Listen("tcp", "127.0.0.1:18843")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Logf("UOW3_BROWSER_URL=http://127.0.0.1:18843/transactions primary=%d fallback=%d bare=%d no_sic=%d travel=%d",
		primaryTxn, fallbackTxn, bareTxn, noSICTxn, travelID)
	server := &http.Server{Handler: router}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		t.Fatalf("browser fixture server stopped: %v", err)
	case <-time.After(15 * time.Minute):
		_ = server.Close()
		t.Fatal(fmt.Errorf("browser fixture server timed out"))
	}
}
