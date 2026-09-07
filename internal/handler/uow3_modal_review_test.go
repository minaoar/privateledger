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

type uow3ModalFixture struct {
	db          *sql.DB
	txnRepo     *repository.TransactionRepository
	sicRepo     *repository.SICMappingRepository
	patternRepo *repository.CategoryPatternRepository
	router      *gin.Engine
	accountID   int
	categoryA   int
	categoryB   int
}

func newUOW3ModalFixture(t *testing.T) *uow3ModalFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	db, err := database.Open(database.Config{Path: filepath.Join(dir, "uow3-modal.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
	accountID := insert(`INSERT INTO account (name) VALUES ('Modal review')`)
	categoryA := insert(`INSERT INTO category (name, category_type) VALUES ('Dining', 2)`)
	categoryB := insert(`INSERT INTO category (name, category_type) VALUES ('Travel', 2)`)

	txnRepo := repository.NewTransactionRepository(db)
	sicRepo := repository.NewSICMappingRepository(db)
	patternRepo := repository.NewCategoryPatternRepository(db)
	sicCategorizer := service.NewSICMappingCategorizer(sicRepo, txnRepo)
	_ = service.NewCategorizerWithSIC(patternRepo, txnRepo, sicCategorizer)
	mappingService := service.NewSICMappingManagementService(
		sicRepo, repository.NewCategoryRepository(db), dir, time.Second, sicCategorizer,
	)
	handler := NewTransactionHandlerWithSIC(txnRepo, mappingService)
	router := gin.New()
	router.PATCH("/api/transactions/:id/category", handler.UpdateTransactionCategory)
	router.POST("/api/transactions/:id/sic-mapping", handler.CreateSICMappingForTransaction)
	return &uow3ModalFixture{db, txnRepo, sicRepo, patternRepo, router, accountID, categoryA, categoryB}
}

func (f *uow3ModalFixture) createTransaction(t *testing.T, fitID, sic string) *model.Transaction {
	t.Helper()
	txn := &model.Transaction{
		AccountID: f.accountID, TrnType: "DEBIT", FitID: fitID,
		DatePosted: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC), Amount: -10,
		TransactionDetails: "CAFE", TransactionType: model.TransactionTypeDebit,
		CategorySource: model.CategorySourceNone,
	}
	if sic != "" {
		code := model.SICCode(sic)
		txn.SICCode = &code
	}
	if err := f.txnRepo.Create(txn); err != nil {
		t.Fatalf("create transaction: %v", err)
	}
	return txn
}

func (f *uow3ModalFixture) request(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func TestReviewU3ModalEndpointValidationAndUpsert(t *testing.T) {
	f := newUOW3ModalFixture(t)
	withSIC := f.createTransaction(t, "with-sic", "5812")
	withoutSIC := f.createTransaction(t, "without-sic", "")

	cases := []struct {
		name, path, body, code string
		status                 int
	}{
		{"category required", fmt.Sprintf("/api/transactions/%d/sic-mapping", withSIC.TransactionID), `{}`, "category_required", 422},
		{"no SIC", fmt.Sprintf("/api/transactions/%d/sic-mapping", withoutSIC.TransactionID), fmt.Sprintf(`{"category_id":%d}`, f.categoryA), "no_sic_code", 422},
		{"missing transaction", "/api/transactions/999999/sic-mapping", fmt.Sprintf(`{"category_id":%d}`, f.categoryA), "not_found", 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := f.request(http.MethodPost, tc.path, tc.body)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("status/body=%d %s, want %d code=%s", w.Code, w.Body.String(), tc.status, tc.code)
			}
		})
	}

	path := fmt.Sprintf("/api/transactions/%d/sic-mapping", withSIC.TransactionID)
	w := f.request(http.MethodPost, path, fmt.Sprintf(`{"category_id":%d,"description":"Eating Places"}`, f.categoryA))
	if w.Code != http.StatusOK {
		t.Fatalf("create mapping: %d %s", w.Code, w.Body.String())
	}
	w = f.request(http.MethodPost, path, fmt.Sprintf(`{"category_id":%d}`, f.categoryB))
	if w.Code != http.StatusOK {
		t.Fatalf("update existing mapping: %d %s", w.Code, w.Body.String())
	}
	mapping, err := f.sicRepo.GetByCode("5812")
	if err != nil || mapping == nil || mapping.CategoryID == nil || *mapping.CategoryID != f.categoryB {
		t.Fatalf("updated mapping=%+v err=%v, want category %d", mapping, err, f.categoryB)
	}
	var patternCount int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM category_pattern`).Scan(&patternCount); err != nil {
		t.Fatal(err)
	}
	if patternCount != 0 {
		t.Fatalf("SIC endpoint created %d text patterns", patternCount)
	}
}

// This drives the exact Change Category browser sequence: PATCH the current
// transaction, then opt in to POST the mapping. US-12/BR-U3-31 require the
// current transaction to end with rule source when the mapping is accepted.
func TestReviewU3ChangeCategoryWithMappingUsesRuleSource(t *testing.T) {
	f := newUOW3ModalFixture(t)
	txn := f.createTransaction(t, "change-and-map", "5812")
	path := fmt.Sprintf("/api/transactions/%d", txn.TransactionID)
	if w := f.request(http.MethodPatch, path+"/category", fmt.Sprintf(`{"category_id":%d}`, f.categoryA)); w.Code != http.StatusOK {
		t.Fatalf("PATCH category: %d %s", w.Code, w.Body.String())
	}
	if w := f.request(http.MethodPost, path+"/sic-mapping", fmt.Sprintf(`{"category_id":%d}`, f.categoryA)); w.Code != http.StatusOK {
		t.Fatalf("POST mapping: %d %s", w.Code, w.Body.String())
	}
	stored, err := f.txnRepo.GetByID(txn.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CategoryID == nil || *stored.CategoryID != f.categoryA || stored.CategorySource != model.CategorySourceRule {
		body, _ := json.Marshal(stored)
		t.Fatalf("change+mapping left current transaction outside rule semantics: %s", body)
	}
}

// A mapping commit cannot be rolled back when the follow-up source restatement
// fails. The success response must therefore disclose that committed partial
// outcome instead of silently claiming the requested rule semantics.
func TestReviewU3RuleSourceWriteFailureIsReported(t *testing.T) {
	f := newUOW3ModalFixture(t)
	txn := f.createTransaction(t, "source-write-failure", "5812")
	path := fmt.Sprintf("/api/transactions/%d", txn.TransactionID)
	if w := f.request(http.MethodPatch, path+"/category", fmt.Sprintf(`{"category_id":%d}`, f.categoryA)); w.Code != http.StatusOK {
		t.Fatalf("PATCH category: %d %s", w.Code, w.Body.String())
	}
	if _, err := f.db.Exec(fmt.Sprintf(`
		CREATE TRIGGER fail_rule_source
		BEFORE UPDATE OF category_source ON ledger_transaction
		WHEN OLD.transaction_id = %d AND NEW.category_source = 1
		BEGIN
			SELECT RAISE(ABORT, 'forced rule-source write failure');
		END`, txn.TransactionID)); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	w := f.request(http.MethodPost, path+"/sic-mapping", fmt.Sprintf(`{"category_id":%d}`, f.categoryA))
	var result model.SICMappingMutationResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response %d %q: %v", w.Code, w.Body.String(), err)
	}
	if !result.MappingCommitted {
		t.Fatalf("mapping was not committed: %d %s", w.Code, w.Body.String())
	}
	if len(result.PostCommitWarnings) == 0 {
		t.Fatalf("committed rule-source failure was not reported: %d %s", w.Code, w.Body.String())
	}
}
