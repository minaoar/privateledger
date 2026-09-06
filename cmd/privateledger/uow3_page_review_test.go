package main

import (
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/handler"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

func TestReviewU3TransactionModalRendering(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(database.Config{Path: filepath.Join(t.TempDir(), "uow3-page.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	accountResult, err := db.Exec(`INSERT INTO account (name) VALUES ('Review')`)
	if err != nil {
		t.Fatal(err)
	}
	accountID64, _ := accountResult.LastInsertId()
	categoryResult, err := db.Exec(`INSERT INTO category (name, category_type) VALUES ('Dining', 2)`)
	if err != nil {
		t.Fatal(err)
	}
	categoryID64, _ := categoryResult.LastInsertId()
	categoryID := int(categoryID64)
	sicRepo := repository.NewSICMappingRepository(db)
	for _, mapping := range []*model.SICMapping{
		model.NewSICMapping("5812", "Primary Description", "ignored", &categoryID),
		model.NewSICMapping("7011", "", "Fallback Detail", nil),
		model.NewSICMapping("7999", "", "", nil),
	} {
		if err := sicRepo.Create(mapping); err != nil {
			t.Fatal(err)
		}
	}
	txnRepo := repository.NewTransactionRepository(db)
	create := func(fitID, details, code string) {
		t.Helper()
		txn := &model.Transaction{
			AccountID: int(accountID64), TrnType: "DEBIT", FitID: fitID,
			DatePosted: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC), Amount: -1,
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
	}
	create("primary", "PRIMARY", "5812")
	create("fallback", "FALLBACK", "7011")
	create("bare", "BARE", "7999")
	create("none", "NO SIC", "")

	accountRepo := repository.NewAccountRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	patternRepo := repository.NewCategoryPatternRepository(db)
	mappingService := service.NewSICMappingService(sicRepo, categoryRepo)
	pageHandler := handler.NewPageHandler(embeddedFiles, accountRepo, txnRepo, categoryRepo, patternRepo, nil, mappingService, "review")
	router := gin.New()
	router.GET("/transactions", pageHandler.Transactions)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/transactions", nil))
	html := w.Body.String()
	if w.Code != 200 {
		t.Fatalf("render status=%d body=%s", w.Code, html)
	}
	for _, required := range []string{
		`data-testid="txn-sic-context"`, `data-testid="txn-sic-create-mapping-toggle"`,
		`data-testid="txn-pattern-type-choice"`, "Primary Description", "Fallback Detail",
		"Other uncategorized transactions with this SIC code will be categorized too.",
		"The SIC mapping result is unknown. Refresh and check before retrying.",
	} {
		if !strings.Contains(html, required) {
			t.Errorf("rendered transactions page is missing %q", required)
		}
	}
	if regexp.MustCompile(`(?i)<th[^>]*>\s*SIC\s*</th>`).MatchString(html) {
		t.Error("main transaction table gained a SIC column")
	}
}

func TestReviewU3CategoryPageFeedbackContract(t *testing.T) {
	page, err := embeddedFiles.ReadFile("web/templates/categories.html")
	if err != nil {
		t.Fatal(err)
	}
	source := string(page)
	for _, required := range []string{
		"Both text patterns and SIC mappings will be applied.",
		"Categories you assigned manually are never changed.",
		"pattern_categorized_count", "sic_categorized_count",
		"The result is unknown. Refresh and check before retrying.",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("category feedback is missing %q", required)
		}
	}
}

func TestReviewU3PageFunctionsDoNotCollideWithSharedScript(t *testing.T) {
	appBytes, err := embeddedFiles.ReadFile("web/static/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	transactionsBytes, err := embeddedFiles.ReadFile("web/templates/transactions.html")
	if err != nil {
		t.Fatal(err)
	}
	categoriesBytes, err := embeddedFiles.ReadFile("web/templates/categories.html")
	if err != nil {
		t.Fatal(err)
	}
	definition := regexp.MustCompile(`(?m)(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	shared := map[string]bool{}
	for _, match := range definition.FindAllStringSubmatch(string(appBytes), -1) {
		shared[match[1]] = true
	}
	for _, page := range []string{string(transactionsBytes), string(categoriesBytes)} {
		for _, match := range definition.FindAllStringSubmatch(page, -1) {
			if shared[match[1]] {
				t.Errorf("page function %s is shadowed by app.js", match[1])
			}
		}
	}
}
