package service

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/parser"
	"github.com/oronno/privateledger/internal/repository"
)

// This file is the NFR-U1-PERF-01 harness. It uses only APIs that exist in both
// the pre-UOW-1 baseline revision and the candidate revision, so the identical
// file can be copied into a baseline worktree and produce a comparable number.
// It is owned by the independent review/test role.

const perfTxnCount = 4000

// buildSICFreeOFX renders a deterministic OFX statement that contains no <SIC>
// element at all, which is the fixture NFR-U1-PERF-01 specifies.
func buildSICFreeOFX(count int) string {
	var b strings.Builder
	b.WriteString("OFXHEADER:100\r\nDATA:OFXSGML\r\nVERSION:102\r\nSECURITY:NONE\r\n")
	b.WriteString("ENCODING:USASCII\r\nCHARSET:1252\r\nCOMPRESSION:NONE\r\nOLDFILEUID:NONE\r\nNEWFILEUID:NONE\r\n\r\n")
	b.WriteString("<OFX>\n<SIGNONMSGSRSV1><SONRS>\n<STATUS><CODE>0<SEVERITY>INFO</STATUS>\n")
	b.WriteString("<DTSERVER>20251215120000[-5:EST]\n<LANGUAGE>ENG\n</SONRS></SIGNONMSGSRSV1>\n")
	b.WriteString("<BANKMSGSRSV1><STMTTRNRS>\n<TRNUID>1\n<STATUS><CODE>0<SEVERITY>INFO</STATUS>\n<STMTRS>\n")
	b.WriteString("<CURDEF>CAD\n<BANKACCTFROM><BANKID>004<ACCTID>123456<ACCTTYPE>CHECKING</BANKACCTFROM>\n")
	b.WriteString("<BANKTRANLIST>\n<DTSTART>20251201120000[-5:EST]\n<DTEND>20251215120000[-5:EST]\n")
	for i := 0; i < count; i++ {
		day := (i % 28) + 1
		hour := (i / 28) % 24
		minute := i % 60
		fmt.Fprintf(&b,
			"<STMTTRN><TRNTYPE>DEBIT<DTPOSTED>202512%02d%02d%02d00[-5:EST]<TRNAMT>-%d.%02d<FITID>PERF-%07d<NAME>MERCHANT %05d<MEMO>LOCATION %05d</STMTTRN>\n",
			day, hour, minute, (i%400)+1, i%100, i, i%997, i%89)
	}
	b.WriteString("</BANKTRANLIST>\n<LEDGERBAL><BALAMT>100.00<DTASOF>20251215120000[-5:EST]</LEDGERBAL>\n")
	b.WriteString("</STMTRS></STMTTRNRS></BANKMSGSRSV1>\n</OFX>")
	return b.String()
}

// runSICFreeImportOnce performs one complete measured import against a fresh
// temporary database. Fixture rendering and database setup are outside the
// measured window; only ImportOFX is timed.
func runSICFreeImportOnce(t *testing.T, ofx string) time.Duration {
	t.Helper()

	db, err := database.Open(database.Config{Path: filepath.Join(t.TempDir(), "perf.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	txnRepo := repository.NewTransactionRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	patternRepo := repository.NewCategoryPatternRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	batchRepo := repository.NewImportBatchRepository(db)

	account := &model.Account{Name: "Perf"}
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("create account: %v", err)
	}
	// A realistic categorization workload: 50 categories and 200 text patterns.
	for i := 0; i < 50; i++ {
		category := &model.Category{Name: fmt.Sprintf("Category %03d", i), CategoryType: model.CategoryTypeExpense}
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("create category: %v", err)
		}
		for j := 0; j < 4; j++ {
			if err := patternRepo.Create(&model.CategoryPattern{
				PatternName: fmt.Sprintf("MERCHANT %05d", i*4+j),
				CategoryID:  category.CategoryID,
			}); err != nil {
				t.Fatalf("create pattern: %v", err)
			}
		}
	}

	categorizer := NewCategorizer(patternRepo, txnRepo)
	if err := categorizer.LoadRules(); err != nil {
		t.Fatalf("load patterns: %v", err)
	}
	importService := NewImportService(parser.NewOFXParser(), txnRepo, accountRepo, categorizer, batchRepo)

	start := time.Now()
	result, err := importService.ImportOFX(strings.NewReader(ofx), account.AccountID, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ImportOFX(): %v", err)
	}
	if result.ImportedCount != perfTxnCount {
		t.Fatalf("imported %d transactions, want %d (errors: %v)", result.ImportedCount, perfTxnCount, result.Errors)
	}
	return elapsed
}

// TestPerformance_SICFreeImport records the NFR-U1-PERF-01 measurement: one
// discarded warm-up run followed by five measured runs, reported as a median.
// The 10% comparison is made against the same harness executed in a worktree of
// the pre-UOW-1 baseline commit and is recorded in the independent review.
func TestPerformance_SICFreeImport(t *testing.T) {
	if testing.Short() {
		t.Skip("performance evidence is skipped in -short mode")
	}
	if raceDetectorEnabled {
		t.Skip("performance targets are acceptance evidence for an uninstrumented build; the race detector adds roughly an order of magnitude of overhead")
	}
	ofx := buildSICFreeOFX(perfTxnCount)

	runSICFreeImportOnce(t, ofx) // warm-up, discarded

	const runs = 5
	samples := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		samples = append(samples, runSICFreeImportOnce(t, ofx))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	median := samples[len(samples)/2]

	t.Logf("NFR-U1-PERF-01 SIC-free import of %d transactions: samples=%v", perfTxnCount, samples)
	t.Logf("PERF01_MEDIAN_NS=%d", median.Nanoseconds())
	t.Logf("NFR-U1-PERF-01 median: %v", median)
}
