package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

type uow5CategoryHandlerFixture struct {
	db           *sql.DB
	txnRepo      *repository.TransactionRepository
	patternRepo  *repository.CategoryPatternRepository
	sicRepo      *repository.SICMappingRepository
	categoryRepo *repository.CategoryRepository
	router       *gin.Engine
	accountID    int
}

func newUOW5CategoryHandlerFixture(t *testing.T) *uow5CategoryHandlerFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Open(database.Config{Path: filepath.Join(t.TempDir(), "uow5-handler.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	result, err := db.Exec(`INSERT INTO account (name) VALUES ('UOW-5 review')`)
	if err != nil {
		t.Fatal(err)
	}
	accountID, _ := result.LastInsertId()
	txnRepo := repository.NewTransactionRepository(db)
	patternRepo := repository.NewCategoryPatternRepository(db)
	sicRepo := repository.NewSICMappingRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	sic := service.NewSICMappingCategorizer(sicRepo, txnRepo)
	categorizer := service.NewCategorizerWithSIC(patternRepo, txnRepo, sic)
	handler := NewCategoryHandler(categoryRepo, patternRepo, categorizer)
	router := gin.New()
	router.POST("/api/categories", handler.CreateCategory)
	router.POST("/api/categories/:id/patterns", handler.AddPattern)
	router.DELETE("/api/patterns/:id", handler.DeletePattern)
	router.DELETE("/api/categories/:id", handler.DeleteCategory)
	return &uow5CategoryHandlerFixture{db, txnRepo, patternRepo, sicRepo, categoryRepo, router, int(accountID)}
}

func (f *uow5CategoryHandlerFixture) category(t *testing.T, name string) int {
	t.Helper()
	category := model.NewCategory(name, model.CategoryTypeExpense, nil, nil)
	if err := f.categoryRepo.Create(category); err != nil {
		t.Fatal(err)
	}
	return category.CategoryID
}

func (f *uow5CategoryHandlerFixture) transaction(t *testing.T, fitID, details, sic string, categoryID *int, source model.CategorySource) *model.Transaction {
	t.Helper()
	txn := &model.Transaction{
		AccountID: f.accountID, TrnType: "DEBIT", FitID: fitID,
		DatePosted: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC), Amount: -1,
		TransactionDetails: details, TransactionType: model.TransactionTypeDebit,
		CategoryID: categoryID, CategorySource: source,
	}
	if sic != "" {
		code := model.SICCode(sic)
		txn.SICCode = &code
	}
	if err := f.txnRepo.Create(txn); err != nil {
		t.Fatal(err)
	}
	return txn
}

func (f *uow5CategoryHandlerFixture) request(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func requireUOW5CountsInResponse(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code < 200 || w.Code >= 300 {
		t.Fatalf("rule change failed: status=%d body=%s", w.Code, w.Body.String())
	}
	for _, field := range []string{`"moved_count"`, `"uncategorized_count"`, `"manual_protected_count"`} {
		if !strings.Contains(w.Body.String(), field) {
			t.Errorf("rule-change response omitted %s: %s", field, w.Body.String())
		}
	}
}

func TestReviewU5PatternRuleChangesReportAllCounts(t *testing.T) {
	t.Run("category creation with pattern", func(t *testing.T) {
		f := newUOW5CategoryHandlerFixture(t)
		old := f.category(t, "Old")
		f.transaction(t, "create-pattern", "MATCH", "", &old, model.CategorySourceRule)
		w := f.request(http.MethodPost, "/api/categories", `{"name":"New","category_type":2,"patterns":["MATCH"]}`)
		requireUOW5CountsInResponse(t, w)
	})

	t.Run("add pattern", func(t *testing.T) {
		f := newUOW5CategoryHandlerFixture(t)
		old := f.category(t, "Old")
		target := f.category(t, "Target")
		f.transaction(t, "add-pattern", "MATCH", "", &old, model.CategorySourceRule)
		w := f.request(http.MethodPost, fmt.Sprintf("/api/categories/%d/patterns", target), `{"pattern_name":"MATCH"}`)
		requireUOW5CountsInResponse(t, w)
	})

	t.Run("delete pattern", func(t *testing.T) {
		f := newUOW5CategoryHandlerFixture(t)
		target := f.category(t, "Target")
		pattern := model.NewCategoryPattern("MATCH", target)
		if err := f.patternRepo.Create(pattern); err != nil {
			t.Fatal(err)
		}
		f.transaction(t, "delete-pattern", "MATCH", "", &target, model.CategorySourceRule)
		w := f.request(http.MethodDelete, fmt.Sprintf("/api/patterns/%d", pattern.CategoryPatternID), "")
		requireUOW5CountsInResponse(t, w)
	})
}

func TestReviewU5PatternRuleChangeResponsesRemainAdditive(t *testing.T) {
	f := newUOW5CategoryHandlerFixture(t)
	w := f.request(http.MethodPost, "/api/categories", `{"name":"Created","category_type":2,"patterns":["MATCH"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create category status=%d body=%s", w.Code, w.Body.String())
	}
	var categoryBody map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &categoryBody); err != nil {
		t.Fatal(err)
	}
	for _, legacyField := range []string{"category_id", "name", "category_type", "patterns"} {
		if _, ok := categoryBody[legacyField]; !ok {
			t.Errorf("category-with-pattern response moved legacy top-level field %q: %s", legacyField, w.Body.String())
		}
	}

	target := f.category(t, "Target")
	w = f.request(http.MethodPost, fmt.Sprintf("/api/categories/%d/patterns", target), `{"pattern_name":"SECOND"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("add pattern status=%d body=%s", w.Code, w.Body.String())
	}
	var patternBody map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &patternBody); err != nil {
		t.Fatal(err)
	}
	for _, legacyField := range []string{"category_pattern_id", "pattern_name", "category_id"} {
		if _, ok := patternBody[legacyField]; !ok {
			t.Errorf("pattern response moved legacy top-level field %q: %s", legacyField, w.Body.String())
		}
	}
}

func TestReviewU5PatternTriggersNeverWriteManualTransactions(t *testing.T) {
	for _, operation := range []string{"create-with-category", "add", "delete"} {
		t.Run(operation, func(t *testing.T) {
			f := newUOW5CategoryHandlerFixture(t)
			manualCategory := f.category(t, "Manual")
			target := f.category(t, "Target")
			manual := f.transaction(t, "manual-"+operation, "MATCH", "", &manualCategory, model.CategorySourceManual)

			switch operation {
			case "create-with-category":
				w := f.request(http.MethodPost, "/api/categories", `{"name":"Created","category_type":2,"patterns":["MATCH"]}`)
				if w.Code != http.StatusCreated {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
			case "add":
				w := f.request(http.MethodPost, fmt.Sprintf("/api/categories/%d/patterns", target), `{"pattern_name":"MATCH"}`)
				if w.Code != http.StatusCreated {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
			case "delete":
				pattern := model.NewCategoryPattern("MATCH", target)
				if err := f.patternRepo.Create(pattern); err != nil {
					t.Fatal(err)
				}
				// Make the manual choice equal the deleted rule's category; once the
				// rule disappears, the hypothetical outcome is uncategorized.
				if err := f.txnRepo.UpdateCategory(manual.TransactionID, &target, model.CategorySourceManual); err != nil {
					t.Fatal(err)
				}
				w := f.request(http.MethodDelete, fmt.Sprintf("/api/patterns/%d", pattern.CategoryPatternID), "")
				if w.Code != http.StatusOK {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
				manualCategory = target
			}

			stored, err := f.txnRepo.GetByID(manual.TransactionID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.CategoryID == nil || *stored.CategoryID != manualCategory || stored.CategorySource != model.CategorySourceManual {
				t.Fatalf("%s trigger changed manual transaction: %+v", operation, stored)
			}
		})
	}
}

func TestReviewU5CategoryDeletionPreservesManualChoiceMarker(t *testing.T) {
	f := newUOW5CategoryHandlerFixture(t)
	deletedCategory := f.category(t, "Manual choice")
	fallbackCategory := f.category(t, "Rule fallback")
	if err := f.sicRepo.Create(model.NewSICMapping("5812", "", "", &fallbackCategory)); err != nil {
		t.Fatal(err)
	}
	txn := f.transaction(t, "manual-delete", "MERCHANT", "5812", &deletedCategory, model.CategorySourceManual)

	w := f.request(http.MethodDelete, fmt.Sprintf("/api/categories/%d", deletedCategory), "")
	if w.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", w.Code, w.Body.String())
	}
	stored, err := f.txnRepo.GetByID(txn.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CategorySource != model.CategorySourceManual {
		t.Fatalf("category deletion destroyed the manual marker and allowed rule reassignment: stored=%+v", stored)
	}
	var body struct {
		Result struct {
			ManualProtectedCount int `json:"manual_protected_count"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Result.ManualProtectedCount != 1 {
		t.Fatalf("manual protection count=%d, want 1: %s", body.Result.ManualProtectedCount, w.Body.String())
	}
}

func TestReviewU5CategoryDeletionDoesNotSplitUncategorizedSemantics(t *testing.T) {
	f := newUOW5CategoryHandlerFixture(t)
	deletedCategory := f.category(t, "Deleted manual category")
	txn := f.transaction(t, "manual-delete-query", "MERCHANT", "", &deletedCategory, model.CategorySourceManual)

	w := f.request(http.MethodDelete, fmt.Sprintf("/api/categories/%d", deletedCategory), "")
	if w.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", w.Code, w.Body.String())
	}

	pageRows, err := f.txnRepo.List(repository.TransactionFilter{Uncategorized: true})
	if err != nil {
		t.Fatal(err)
	}
	uncategorizedRows, err := f.txnRepo.GetUncategorized()
	if err != nil {
		t.Fatal(err)
	}
	uncategorizedCount, err := f.txnRepo.CountUncategorized()
	if err != nil {
		t.Fatal(err)
	}
	contains := func(rows []*model.Transaction) bool {
		for _, row := range rows {
			if row.TransactionID == txn.TransactionID {
				return true
			}
		}
		return false
	}
	if contains(pageRows) != contains(uncategorizedRows) || len(pageRows) != uncategorizedCount {
		t.Fatalf("category deletion created conflicting uncategorized representations: page=%d/get=%d/count=%d transaction page=%t/get=%t",
			len(pageRows), len(uncategorizedRows), uncategorizedCount, contains(pageRows), contains(uncategorizedRows))
	}
}

func TestReviewU5CategoryDeletionReexaminesAfterCascade(t *testing.T) {
	f := newUOW5CategoryHandlerFixture(t)
	deletedCategory := f.category(t, "Deleted rule")
	fallbackCategory := f.category(t, "Remaining SIC rule")
	pattern := model.NewCategoryPattern("MATCH", deletedCategory)
	if err := f.patternRepo.Create(pattern); err != nil {
		t.Fatal(err)
	}
	if err := f.sicRepo.Create(model.NewSICMapping("5812", "", "", &fallbackCategory)); err != nil {
		t.Fatal(err)
	}
	txn := f.transaction(t, "cascade-order", "MATCH", "5812", &deletedCategory, model.CategorySourceRule)
	w := f.request(http.MethodDelete, fmt.Sprintf("/api/categories/%d", deletedCategory), "")
	if w.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", w.Code, w.Body.String())
	}
	stored, err := f.txnRepo.GetByID(txn.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CategoryID == nil || *stored.CategoryID != fallbackCategory || stored.CategorySource != model.CategorySourceRule {
		t.Fatalf("category deletion did not use post-cascade rules: %+v", stored)
	}
}

func TestReviewU5PatternCommitReportsPostCommitReexaminationFailure(t *testing.T) {
	f := newUOW5CategoryHandlerFixture(t)
	old := f.category(t, "Old")
	target := f.category(t, "Target")
	txn := f.transaction(t, "post-commit-warning", "MATCH", "", &old, model.CategorySourceRule)
	if _, err := f.db.Exec(fmt.Sprintf(`
		CREATE TRIGGER fail_uow5_reexamination
		BEFORE UPDATE OF category_id ON ledger_transaction
		WHEN OLD.transaction_id = %d
		BEGIN SELECT RAISE(ABORT, 'forced re-examination failure'); END`, txn.TransactionID)); err != nil {
		t.Fatal(err)
	}
	w := f.request(http.MethodPost, fmt.Sprintf("/api/categories/%d/patterns", target), `{"pattern_name":"MATCH"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("committed pattern status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"post_commit_warnings"`) {
		t.Fatalf("committed pattern hid its failed re-examination: %s", w.Body.String())
	}
}
