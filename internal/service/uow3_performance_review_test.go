package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/parser"
	"github.com/oronno/privateledger/internal/repository"
)

type uow3RecategorizeMeasurement struct {
	elapsed      time.Duration
	noOpElapsed  time.Duration
	databaseSize int64
	result       *RecategorizeResult
}

func buildUOW3RecategorizeFixture(t *testing.T, transactionCount, mappingCount, categoryCount int) (*SICMappingCategorizer, *Categorizer, string) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "recategorize.db")
	db, err := database.Open(database.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO account (name) VALUES ('Performance')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < ?)
		INSERT INTO category (name, category_type)
		SELECT printf('Category %03d', x), 2 FROM n`, categoryCount); err != nil {
		t.Fatalf("create categories: %v", err)
	}
	mappings := make([]*model.SICMapping, mappingCount)
	for i := range mappings {
		categoryID := (i % categoryCount) + 1
		mappings[i] = model.NewSICMapping(model.SICCode(fmt.Sprint(i+1)), "Merchant", "", &categoryID)
	}
	sicRepo := repository.NewSICMappingRepository(db)
	if err := sicRepo.BulkInsertAtomic(mappings); err != nil {
		t.Fatalf("create mappings: %v", err)
	}
	if _, err := db.Exec(`
		WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < ?)
		INSERT INTO ledger_transaction
			(account_id, trn_type, fit_id, date_posted, amount, transaction_details, transaction_type, sic_code, category_source)
		SELECT 1, 'DEBIT', printf('recat-%06d', x), '2026-01-01 00:00:00', -1,
			'MERCHANT', 1, CAST(((x-1) % ?) + 1 AS TEXT), 0 FROM n`, transactionCount, mappingCount); err != nil {
		t.Fatalf("create transactions: %v", err)
	}
	txnRepo := repository.NewTransactionRepository(db)
	sic := NewSICMappingCategorizer(sicRepo, txnRepo)
	categorizer := NewCategorizerWithSIC(repository.NewCategoryPatternRepository(db), txnRepo, sic)
	return sic, categorizer, dbPath
}

func runUOW3RecategorizeAllOnce(t *testing.T) uow3RecategorizeMeasurement {
	t.Helper()
	_, categorizer, dbPath := buildUOW3RecategorizeFixture(t, 20_000, 1_000, 100)
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	result, err := categorizer.Reexamine()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RecategorizeAll: %v", err)
	}
	if result.ProcessedCount != 20_000 || result.CategorizedCount != 20_000 || result.PatternCategorizedCount != 0 || result.SICCategorizedCount != 20_000 {
		t.Fatalf("unexpected result: %+v", result)
	}
	noOpStart := time.Now()
	noOpResult, err := categorizer.Reexamine()
	noOpElapsed := time.Since(noOpStart)
	if err != nil {
		t.Fatalf("no-op Reexamine: %v", err)
	}
	if noOpResult.MovedCount != 0 || noOpResult.UncategorizedCount != 0 || noOpResult.ManualProtectedCount != 0 {
		t.Fatalf("no-op result: %+v", noOpResult)
	}
	return uow3RecategorizeMeasurement{elapsed: elapsed, noOpElapsed: noOpElapsed, databaseSize: info.Size(), result: result}
}

func durationMedian(samples []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

func TestReviewU5ReexaminationPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("performance evidence is skipped in short mode")
	}
	if raceDetectorEnabled {
		t.Skip("performance evidence is measured without race instrumentation")
	}
	runUOW3RecategorizeAllOnce(t) // warm-up, discarded
	const runs = 5
	samples := make([]time.Duration, 0, runs)
	noOpSamples := make([]time.Duration, 0, runs)
	var databaseSize int64
	for i := 0; i < runs; i++ {
		measurement := runUOW3RecategorizeAllOnce(t)
		samples = append(samples, measurement.elapsed)
		noOpSamples = append(noOpSamples, measurement.noOpElapsed)
		databaseSize = measurement.databaseSize
	}
	median := durationMedian(samples)
	noOpMedian := durationMedian(noOpSamples)
	t.Logf("NFR-U5-PERF-01 fixture: transactions=20000 manual=0 (0%%) mappings=1000 categories=100 SIC proportion=100%% database_bytes=%d", databaseSize)
	t.Logf("NFR-U5-PERF-01 worst-case samples=%v median=%v; no-op samples=%v median=%v", samples, median, noOpSamples, noOpMedian)
	if median > 1500*time.Millisecond {
		t.Fatalf("NFR-U5-PERF-01 worst-case median %v exceeds 1.5s", median)
	}
	if noOpMedian >= median {
		t.Fatalf("NFR-U5-PERF-01 no-op median %v is not measurably faster than worst-case %v", noOpMedian, median)
	}
}

func addSICToPerformanceOFX(ofx string, mappingCount int) string {
	parts := strings.Split(ofx, "<STMTTRN>")
	for i := 1; i < len(parts); i++ {
		code := ((i - 1) % mappingCount) + 1
		parts[i] = strings.Replace(parts[i], "<NAME>", fmt.Sprintf("<SIC>%d<NAME>", code), 1)
	}
	return strings.Join(parts, "<STMTTRN>")
}

func runUOW3ImportOnce(t *testing.T, ofx string) time.Duration {
	t.Helper()
	db, err := database.Open(database.Config{Path: filepath.Join(t.TempDir(), "import.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	txnRepo := repository.NewTransactionRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	patternRepo := repository.NewCategoryPatternRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	sicRepo := repository.NewSICMappingRepository(db)
	batchRepo := repository.NewImportBatchRepository(db)
	account := &model.Account{Name: "Import performance"}
	if err := accountRepo.Create(account); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		category := &model.Category{Name: fmt.Sprintf("Category %03d", i), CategoryType: model.CategoryTypeExpense}
		if err := categoryRepo.Create(category); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 4; j++ {
			if err := patternRepo.Create(&model.CategoryPattern{PatternName: fmt.Sprintf("MERCHANT %05d", i*4+j), CategoryID: category.CategoryID}); err != nil {
				t.Fatal(err)
			}
		}
	}
	mappings := make([]*model.SICMapping, 1_000)
	for i := range mappings {
		categoryID := (i % 50) + 1
		mappings[i] = model.NewSICMapping(model.SICCode(fmt.Sprint(i+1)), "", "", &categoryID)
	}
	if err := sicRepo.BulkInsertAtomic(mappings); err != nil {
		t.Fatal(err)
	}
	sic := NewSICMappingCategorizer(sicRepo, txnRepo)
	categorizer := NewCategorizerWithSIC(patternRepo, txnRepo, sic)
	if err := categorizer.LoadRules(); err != nil {
		t.Fatal(err)
	}
	importer := NewImportService(parser.NewOFXParser(), txnRepo, accountRepo, categorizer, batchRepo)
	start := time.Now()
	result, err := importer.ImportOFX(strings.NewReader(ofx), account.AccountID, nil)
	elapsed := time.Since(start)
	if err != nil || result.ImportedCount != perfTxnCount {
		t.Fatalf("import result=%+v err=%v", result, err)
	}
	return elapsed
}

func TestReviewU3ImportWithMappingsPerformance(t *testing.T) {
	if testing.Short() || raceDetectorEnabled {
		t.Skip("performance evidence requires an uninstrumented non-short run")
	}
	sicFree := buildSICFreeOFX(perfTxnCount)
	withSIC := addSICToPerformanceOFX(sicFree, 1_000)
	runUOW3ImportOnce(t, sicFree)
	runUOW3ImportOnce(t, withSIC)
	const runs = 5
	freeSamples := make([]time.Duration, 0, runs)
	sicSamples := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		freeSamples = append(freeSamples, runUOW3ImportOnce(t, sicFree))
		sicSamples = append(sicSamples, runUOW3ImportOnce(t, withSIC))
	}
	freeMedian, sicMedian := durationMedian(freeSamples), durationMedian(sicSamples)
	ratio := float64(sicMedian) / float64(freeMedian)
	t.Logf("NFR-U3-PERF-02 SIC-free samples=%v median=%v", freeSamples, freeMedian)
	t.Logf("NFR-U3-PERF-02 SIC-bearing samples=%v median=%v ratio=%.4f", sicSamples, sicMedian, ratio)
	if ratio > 1.10 {
		t.Fatalf("SIC-bearing import ratio %.4f exceeds 1.10 budget", ratio)
	}
}

func TestReviewU3MappingCacheObservedMemory(t *testing.T) {
	if testing.Short() || raceDetectorEnabled {
		t.Skip("memory observation requires an uninstrumented non-short run")
	}
	db, err := database.Open(database.Config{Path: filepath.Join(t.TempDir(), "cache.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO category (name, category_type) VALUES ('Cache', 2)`); err != nil {
		t.Fatal(err)
	}
	mappings := make([]*model.SICMapping, 100_000)
	categoryID := 1
	for i := range mappings {
		mappings[i] = model.NewSICMapping(model.SICCode(fmt.Sprint(i+1)), "Description", "Detail", &categoryID)
	}
	sicRepo := repository.NewSICMappingRepository(db)
	if err := sicRepo.BulkInsertAtomic(mappings); err != nil {
		t.Fatal(err)
	}
	mappings = nil
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	cache := NewSICMappingCategorizer(sicRepo, repository.NewTransactionRepository(db))
	if err := cache.ReloadMappings(); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if _, ok := cache.LookupCategory("100000"); !ok {
		t.Fatal("loaded cache did not retain the final mapping")
	}
	delta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("NFR-U3-SCALE-01 100000-mapping cache observed_heap_delta_bytes=%d before=%d after=%d", delta, before.HeapAlloc, after.HeapAlloc)
}
