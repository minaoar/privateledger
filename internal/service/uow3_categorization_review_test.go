package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

type uow3Lookup struct {
	mu         sync.RWMutex
	categories map[model.SICCode]int
	reloadErr  error
}

// TestReviewU3UOW2MergeFiftyThousandAffectedCodes exercises the UOW-2 merge
// fixture's newly-live handoff with the real UOW-3 collaborator. The 25,000
// changed and 25,000 created mappings produce one 50,000-code scoped query.
func TestReviewU3UOW2MergeFiftyThousandAffectedCodes(t *testing.T) {
	if testing.Short() {
		t.Skip("large cross-unit regression fixture")
	}
	f := newSeedFixture(t)
	categories := make([]int, 100)
	for i := range categories {
		categories[i] = f.addCategory(fmt.Sprintf("U3CrossUnit%03d", i))
	}
	preexisting := make([]*model.SICMapping, 100_000)
	for i := range preexisting {
		categoryID := categories[i%len(categories)]
		preexisting[i] = model.NewSICMapping(model.SICCode(fmt.Sprint(i+1)), "same", "", &categoryID)
	}
	if err := f.sicRepo.BulkInsertAtomic(preexisting); err != nil {
		t.Fatal(err)
	}
	rows := make([][]string, 0, 100_000)
	for code := 1; code <= 75_000; code++ {
		description := "same"
		if code <= 25_000 {
			description = "changed"
		}
		idx := (code - 1) % len(categories)
		rows = append(rows, []string{fmt.Sprint(code), description, "", fmt.Sprintf("U3CrossUnit%03d", idx), fmt.Sprint(categories[idx])})
	}
	for code := 100_001; code <= 125_000; code++ {
		idx := (code - 1) % len(categories)
		rows = append(rows, []string{fmt.Sprint(code), "new", "", fmt.Sprintf("U3CrossUnit%03d", idx), fmt.Sprint(categories[idx])})
	}
	txnRepo := repository.NewTransactionRepository(f.db)
	sic := NewSICMappingCategorizer(f.sicRepo, txnRepo)
	mappingService := NewSICMappingManagementService(f.sicRepo, f.catRepo, f.dir, 5*time.Second, sic)
	result, err := mappingService.MergeUpload(context.Background(), strings.NewReader(reviewCSV(t, rows)))
	if err != nil {
		t.Fatalf("merge through real collaborator: %v", err)
	}
	if result.CreatedRows != 25_000 || result.UpdatedRows != 25_000 || result.UnchangedRows != 50_000 || result.RecategorizedRows != 0 || len(result.PostCommitWarnings) != 0 {
		t.Fatalf("unexpected cross-unit merge result: %+v", result)
	}
}

func (l *uow3Lookup) LookupCategory(code model.SICCode) (int, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	categoryID, ok := l.categories[code]
	return categoryID, ok && categoryID != 0
}

func (l *uow3Lookup) ReloadMappings() error { return l.reloadErr }

func uow3SIC(code string) *model.SICCode {
	sic := model.SICCode(code)
	return &sic
}

func uow3Category(id int) *int { return &id }

// TestReviewU3PriorityMatrix covers BR-U3-02 through BR-U3-07 at the single
// decision function used by import and the two full recategorization paths.
func TestReviewU3PriorityMatrix(t *testing.T) {
	lookup := &uow3Lookup{categories: map[model.SICCode]int{"5812": 22}}
	categorizer := &Categorizer{
		patterns:  []*model.CategoryPattern{{PatternName: "CAFE", CategoryID: 11}},
		sicLookup: lookup,
	}

	cases := []struct {
		name       string
		txn        *model.Transaction
		wantChange bool
		wantCat    *int
	}{
		{"manual preserved", &model.Transaction{TransactionDetails: "CAFE", SICCode: uow3SIC("5812"), CategoryID: uow3Category(7), CategorySource: model.CategorySourceManual}, false, uow3Category(7)},
		{"existing rule preserved", &model.Transaction{TransactionDetails: "CAFE", SICCode: uow3SIC("5812"), CategoryID: uow3Category(8), CategorySource: model.CategorySourceRule}, false, uow3Category(8)},
		{"pattern beats SIC", &model.Transaction{TransactionDetails: "CAFE NOIR", SICCode: uow3SIC("5812")}, true, uow3Category(11)},
		{"SIC fills pattern miss", &model.Transaction{TransactionDetails: "HOTEL", SICCode: uow3SIC("5812")}, true, uow3Category(22)},
		{"missing mapping is no-op", &model.Transaction{TransactionDetails: "HOTEL", SICCode: uow3SIC("9999")}, false, nil},
		{"no SIC is no-op", &model.Transaction{TransactionDetails: "HOTEL"}, false, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := categorizer.Categorize(tc.txn)
			if changed != tc.wantChange {
				t.Fatalf("Categorize changed=%v, want %v", changed, tc.wantChange)
			}
			if tc.wantCat == nil {
				if tc.txn.CategoryID != nil {
					t.Fatalf("category=%d, want nil", *tc.txn.CategoryID)
				}
			} else if tc.txn.CategoryID == nil || *tc.txn.CategoryID != *tc.wantCat {
				t.Fatalf("category=%v, want %d", tc.txn.CategoryID, *tc.wantCat)
			}
			if changed && tc.txn.CategorySource != model.CategorySourceRule {
				t.Fatalf("category_source=%v, want rule", tc.txn.CategorySource)
			}
		})
	}

	lookup.mu.Lock()
	lookup.categories["5812"] = 0
	lookup.mu.Unlock()
	empty := &model.Transaction{TransactionDetails: "HOTEL", SICCode: uow3SIC("5812")}
	if categorizer.Categorize(empty) || empty.CategoryID != nil {
		t.Fatalf("empty-category mapping assigned category %v", empty.CategoryID)
	}
}

func newUOW3ServiceHarness(t *testing.T) (*sql.DB, *repository.TransactionRepository, *repository.CategoryPatternRepository, *repository.SICMappingRepository, *SICMappingCategorizer, *Categorizer, int) {
	t.Helper()
	db, err := database.Open(database.Config{Path: filepath.Join(t.TempDir(), "uow3.db")})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	accountResult, err := db.Exec(`INSERT INTO account (name) VALUES ('Review')`)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID64, err := accountResult.LastInsertId()
	if err != nil {
		t.Fatalf("account id: %v", err)
	}
	txnRepo := repository.NewTransactionRepository(db)
	patternRepo := repository.NewCategoryPatternRepository(db)
	sicRepo := repository.NewSICMappingRepository(db)
	sic := NewSICMappingCategorizer(sicRepo, txnRepo)
	categorizer := NewCategorizerWithSIC(patternRepo, txnRepo, sic)
	return db, txnRepo, patternRepo, sicRepo, sic, categorizer, int(accountID64)
}

func createUOW3Category(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	result, err := db.Exec(`INSERT INTO category (name, category_type) VALUES (?, 2)`, name)
	if err != nil {
		t.Fatalf("create category %q: %v", name, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("category id: %v", err)
	}
	return int(id)
}

func createUOW3Txn(t *testing.T, repo *repository.TransactionRepository, accountID int, fitID, details, code string, categoryID *int, source model.CategorySource) *model.Transaction {
	t.Helper()
	txn := &model.Transaction{
		AccountID: accountID, TrnType: "DEBIT", FitID: fitID,
		DatePosted: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC), Amount: -10,
		TransactionDetails: details, TransactionType: model.TransactionTypeDebit,
		CategoryID: categoryID, CategorySource: source,
	}
	if code != "" {
		txn.SICCode = uow3SIC(code)
	}
	if err := repo.Create(txn); err != nil {
		t.Fatalf("create transaction %s: %v", fitID, err)
	}
	return txn
}

// TestReviewU3RecategorizeEntryPointParity verifies the two explicit
// recategorization entry points use the same text-before-SIC decision as import.
func TestReviewU3RecategorizeEntryPointParity(t *testing.T) {
	for _, byCategory := range []bool{false, true} {
		name := "all"
		if byCategory {
			name = "by category"
		}
		t.Run(name, func(t *testing.T) {
			db, txnRepo, patternRepo, sicRepo, _, categorizer, accountID := newUOW3ServiceHarness(t)
			patternCategory := createUOW3Category(t, db, "Pattern")
			sicCategory := createUOW3Category(t, db, "SIC")
			if err := patternRepo.Create(model.NewCategoryPattern("CAFE", patternCategory)); err != nil {
				t.Fatal(err)
			}
			if err := sicRepo.Create(model.NewSICMapping("5812", "Dining", "", &sicCategory)); err != nil {
				t.Fatal(err)
			}
			txn := createUOW3Txn(t, txnRepo, accountID, "priority", "CAFE NOIR", "5812", nil, model.CategorySourceNone)

			var result *RecategorizeResult
			var err error
			if byCategory {
				result, err = categorizer.RecategorizeByCategory(patternCategory)
			} else {
				result, err = categorizer.RecategorizeAll()
			}
			if err != nil {
				t.Fatalf("recategorize: %v", err)
			}
			stored, err := txnRepo.GetByID(txn.TransactionID)
			if err != nil || stored.CategoryID == nil || *stored.CategoryID != patternCategory {
				t.Fatalf("stored category=%v err=%v, want pattern category %d", stored.CategoryID, err, patternCategory)
			}
			if result.CategorizedCount != 1 || result.PatternCategorizedCount != 1 || result.SICCategorizedCount != 0 {
				t.Fatalf("counts=%+v, want total/pattern/SIC 1/1/0", result)
			}
		})
	}
}

func TestReviewU3RecategorizeSplitCounts(t *testing.T) {
	db, txnRepo, patternRepo, sicRepo, _, categorizer, accountID := newUOW3ServiceHarness(t)
	patternCategory := createUOW3Category(t, db, "Pattern")
	sicCategory := createUOW3Category(t, db, "SIC")
	if err := patternRepo.Create(model.NewCategoryPattern("CAFE", patternCategory)); err != nil {
		t.Fatal(err)
	}
	if err := sicRepo.Create(model.NewSICMapping("5812", "Dining", "", &sicCategory)); err != nil {
		t.Fatal(err)
	}
	createUOW3Txn(t, txnRepo, accountID, "pattern", "CAFE", "5812", nil, model.CategorySourceNone)
	createUOW3Txn(t, txnRepo, accountID, "sic", "HOTEL", "5812", nil, model.CategorySourceNone)
	createUOW3Txn(t, txnRepo, accountID, "none", "UNKNOWN", "9999", nil, model.CategorySourceNone)

	result, err := categorizer.RecategorizeAll()
	if err != nil {
		t.Fatal(err)
	}
	if result.ProcessedCount != 3 || result.CategorizedCount != 2 || result.PatternCategorizedCount != 1 || result.SICCategorizedCount != 1 {
		t.Fatalf("split counts=%+v, want processed/total/pattern/SIC 3/2/1/1", result)
	}
	if result.PatternCategorizedCount+result.SICCategorizedCount != result.CategorizedCount {
		t.Fatalf("split does not partition total: %+v", result)
	}
}

func TestReviewU3RecategorizeByCategoryAppliesSIC(t *testing.T) {
	db, txnRepo, _, sicRepo, _, categorizer, accountID := newUOW3ServiceHarness(t)
	sicCategory := createUOW3Category(t, db, "SIC")
	if err := sicRepo.Create(model.NewSICMapping("5812", "Dining", "", &sicCategory)); err != nil {
		t.Fatal(err)
	}
	txn := createUOW3Txn(t, txnRepo, accountID, "sic-only", "HOTEL", "5812", nil, model.CategorySourceNone)
	result, err := categorizer.RecategorizeByCategory(sicCategory)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := txnRepo.GetByID(txn.TransactionID)
	if err != nil || stored.CategoryID == nil || *stored.CategoryID != sicCategory {
		t.Fatalf("stored=%+v err=%v, want SIC category %d", stored, err, sicCategory)
	}
	if result.CategorizedCount != 1 || result.SICCategorizedCount != 1 || result.PatternCategorizedCount != 0 {
		t.Fatalf("counts=%+v, want total/pattern/SIC 1/0/1", result)
	}
}

func TestReviewU3ScopedEmptySetDoesNoWork(t *testing.T) {
	_, _, _, _, sic, _, _ := newUOW3ServiceHarness(t)
	count, err := sic.RecategorizeBySICCodes(nil)
	if err != nil || count != 0 {
		t.Fatalf("empty scoped pass count=%d err=%v, want 0 nil", count, err)
	}
}

// TestReviewU3ScopedRecategorizationUsesSharedPriority is intentionally
// load-bearing: BR-U3-01/04/05 require a matching text pattern to beat SIC even
// when a mapping mutation starts the scoped pass.
func TestReviewU3ScopedRecategorizationUsesSharedPriority(t *testing.T) {
	db, txnRepo, patternRepo, sicRepo, sic, categorizer, accountID := newUOW3ServiceHarness(t)
	patternCategory := createUOW3Category(t, db, "Text rule")
	sicCategory := createUOW3Category(t, db, "SIC rule")
	if err := patternRepo.Create(model.NewCategoryPattern("CAFE", patternCategory)); err != nil {
		t.Fatal(err)
	}
	if err := sicRepo.Create(model.NewSICMapping("5812", "Dining", "", &sicCategory)); err != nil {
		t.Fatal(err)
	}
	if err := categorizer.LoadRules(); err != nil {
		t.Fatalf("load rules: %v", err)
	}
	txn := createUOW3Txn(t, txnRepo, accountID, "scoped-priority", "CAFE NOIR", "5812", nil, model.CategorySourceNone)

	count, err := sic.RecategorizeBySICCodes([]model.SICCode{"5812"})
	if err != nil {
		t.Fatalf("RecategorizeBySICCodes: %v", err)
	}
	stored, err := txnRepo.GetByID(txn.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || stored.CategoryID == nil || *stored.CategoryID != patternCategory {
		t.Fatalf("scoped recategorization count=%d category=%v, want text-rule category %d", count, stored.CategoryID, patternCategory)
	}
}

// An inconsistent legacy row can carry category_id with category_source=none.
// BR-U3-03 protects every existing category regardless of source metadata.
func TestReviewU3ScopedRecategorizationPreservesExistingCategory(t *testing.T) {
	db, txnRepo, _, sicRepo, sic, categorizer, accountID := newUOW3ServiceHarness(t)
	originalCategory := createUOW3Category(t, db, "Existing")
	sicCategory := createUOW3Category(t, db, "SIC rule")
	if err := sicRepo.Create(model.NewSICMapping("5812", "Dining", "", &sicCategory)); err != nil {
		t.Fatal(err)
	}
	if err := categorizer.LoadRules(); err != nil {
		t.Fatal(err)
	}
	txn := createUOW3Txn(t, txnRepo, accountID, "existing-category", "MERCHANT", "5812", &originalCategory, model.CategorySourceNone)

	count, err := sic.RecategorizeBySICCodes([]model.SICCode{"5812"})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := txnRepo.GetByID(txn.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || stored.CategoryID == nil || *stored.CategoryID != originalCategory {
		t.Fatalf("existing category changed: count=%d category=%v, want unchanged %d", count, stored.CategoryID, originalCategory)
	}
}

type stagedUOW3Lookup struct {
	mu      sync.RWMutex
	current int
	next    int
	started chan struct{}
	release chan struct{}
}

func (l *stagedUOW3Lookup) LookupCategory(model.SICCode) (int, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.current, true
}

func (l *stagedUOW3Lookup) ReloadMappings() error {
	l.mu.Lock()
	l.current = l.next
	l.mu.Unlock()
	close(l.started)
	<-l.release
	return nil
}

// TestReviewU3RuleCachesPublishAtomically proves categorization cannot observe
// one half of a reload. The only valid results are the complete old rule set or
// the complete new rule set.
func TestReviewU3RuleCachesPublishAtomically(t *testing.T) {
	db, _, patternRepo, _, _, _, _ := newUOW3ServiceHarness(t)
	lookup := &stagedUOW3Lookup{current: 30, next: 20, started: make(chan struct{}), release: make(chan struct{})}
	categorizer := &Categorizer{
		patternRepo: patternRepo,
		patterns:    []*model.CategoryPattern{{PatternName: "OLD ONLY", CategoryID: 40}},
		sicLookup:   lookup,
	}
	// A new text pattern would win over the new SIC mapping once the complete
	// replacement rule set is visible.
	newCategory := createUOW3Category(t, db, "New text rule")
	newPattern := &model.CategoryPattern{PatternName: "CAFE", CategoryID: newCategory}
	if err := patternRepo.Create(newPattern); err != nil {
		t.Fatalf("create replacement pattern: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- categorizer.LoadRules() }()
	<-lookup.started
	txn := &model.Transaction{TransactionDetails: "CAFE", SICCode: uow3SIC("5812")}
	if !categorizer.Categorize(txn) || txn.CategoryID == nil {
		t.Fatal("categorization unexpectedly produced no result")
	}
	observed := *txn.CategoryID
	close(lookup.release)
	if err := <-done; err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if observed != 30 && observed != newCategory {
		t.Fatalf("observed mixed cache generation category %d; valid old/new results are 30 or %d", observed, newCategory)
	}
}

func TestReviewU3FailedMappingReloadKeepsPatternCache(t *testing.T) {
	_, _, patternRepo, _, _, _, _ := newUOW3ServiceHarness(t)
	lookup := &uow3Lookup{categories: map[model.SICCode]int{}, reloadErr: errors.New("forced mapping reload failure")}
	categorizer := &Categorizer{
		patternRepo: patternRepo,
		patterns:    []*model.CategoryPattern{{PatternName: "OLD", CategoryID: 7}},
		sicLookup:   lookup,
	}
	if err := categorizer.LoadRules(); err == nil {
		t.Fatal("LoadRules succeeded despite forced mapping failure")
	}
	txn := &model.Transaction{TransactionDetails: "OLD"}
	if !categorizer.Categorize(txn) || txn.CategoryID == nil || *txn.CategoryID != 7 {
		t.Fatalf("failed reload replaced old pattern cache: category=%v", txn.CategoryID)
	}
}

func TestReviewU3ConcurrentCategorizeAndReload(t *testing.T) {
	_, _, patternRepo, _, _, _, _ := newUOW3ServiceHarness(t)
	lookup := &uow3Lookup{categories: map[model.SICCode]int{"5812": 2}}
	categorizer := &Categorizer{patternRepo: patternRepo, patterns: []*model.CategoryPattern{{PatternName: "CAFE", CategoryID: 1}}, sicLookup: lookup}

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				txn := &model.Transaction{TransactionDetails: fmt.Sprintf("CAFE %d", worker), SICCode: uow3SIC("5812")}
				categorizer.Categorize(txn)
			}
		}(worker)
	}
	for i := 0; i < 50; i++ {
		if err := categorizer.LoadRules(); err != nil {
			t.Fatalf("LoadRules: %v", err)
		}
	}
	wg.Wait()
}
