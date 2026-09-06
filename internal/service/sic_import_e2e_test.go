package service

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/parser"
	"github.com/oronno/privateledger/internal/repository"
)

const e2eHeader = "OFXHEADER:100\r\nDATA:OFXSGML\r\nVERSION:102\r\nSECURITY:NONE\r\n" +
	"ENCODING:USASCII\r\nCHARSET:1252\r\nCOMPRESSION:NONE\r\nOLDFILEUID:NONE\r\nNEWFILEUID:NONE\r\n\r\n"

// e2eOFX builds a two-transaction statement; sicA/sicB are raw <SIC> element
// fragments (or "" to omit the tag entirely).
func e2eOFX(sicA, sicB string) string {
	return e2eHeader + `<OFX>
<SIGNONMSGSRSV1><SONRS>
<STATUS><CODE>0<SEVERITY>INFO</STATUS>
<DTSERVER>20251215120000[-5:EST]
<LANGUAGE>ENG
</SONRS></SIGNONMSGSRSV1>
<BANKMSGSRSV1><STMTTRNRS>
<TRNUID>1
<STATUS><CODE>0<SEVERITY>INFO</STATUS>
<STMTRS>
<CURDEF>CAD
<BANKACCTFROM><BANKID>004<ACCTID>123456<ACCTTYPE>CHECKING</BANKACCTFROM>
<BANKTRANLIST>
<DTSTART>20251201120000[-5:EST]
<DTEND>20251215120000[-5:EST]
<STMTTRN><TRNTYPE>DEBIT<DTPOSTED>20251202120000[-5:EST]<TRNAMT>-12.34<FITID>E2E-1` + sicA + `<NAME>CAFE NOIR</STMTTRN>
<STMTTRN><TRNTYPE>CREDIT<DTPOSTED>20251203120000[-5:EST]<TRNAMT>500.00<FITID>E2E-2` + sicB + `<NAME>PAYROLL DEPOSIT</STMTTRN>
</BANKTRANLIST>
<LEDGERBAL><BALAMT>100.00<DTASOF>20251215120000[-5:EST]</LEDGERBAL>
</STMTRS></STMTTRNRS></BANKMSGSRSV1>
</OFX>`
}

type importHarness struct {
	db        *sql.DB
	txnRepo   *repository.TransactionRepository
	patRepo   *repository.CategoryPatternRepository
	catRepo   *repository.CategoryRepository
	service   *ImportService
	accountID int
}

func newImportHarness(t *testing.T) *importHarness {
	t.Helper()
	db, err := database.Open(database.Config{Path: filepath.Join(t.TempDir(), "privateledger.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	txnRepo := repository.NewTransactionRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	patRepo := repository.NewCategoryPatternRepository(db)
	catRepo := repository.NewCategoryRepository(db)
	batchRepo := repository.NewImportBatchRepository(db)

	account := &model.Account{Name: "Chequing"}
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("create account: %v", err)
	}

	categorizer := NewCategorizer(patRepo, txnRepo)
	if err := categorizer.LoadRules(); err != nil {
		t.Fatalf("load patterns: %v", err)
	}

	return &importHarness{
		db:        db,
		txnRepo:   txnRepo,
		patRepo:   patRepo,
		catRepo:   catRepo,
		service:   NewImportService(parser.NewOFXParser(), txnRepo, accountRepo, categorizer, batchRepo),
		accountID: account.AccountID,
	}
}

// TestImport_StoresSICEndToEnd covers AC1 and AC2 through the real import path:
// a <SIC>-bearing transaction persists its canonical SIC, and a SIC-free
// transaction imports successfully with no SIC.
func TestImport_StoresSICEndToEnd(t *testing.T) {
	h := newImportHarness(t)

	result, err := h.service.ImportOFX(strings.NewReader(e2eOFX("<SIC>0005812", "")), h.accountID, nil)
	if err != nil {
		t.Fatalf("ImportOFX(): %v", err)
	}
	if result.ImportedCount != 2 {
		t.Fatalf("ImportedCount = %d, want 2 (errors: %v)", result.ImportedCount, result.Errors)
	}
	if result.ErrorCount != 0 {
		t.Fatalf("ErrorCount = %d: %v", result.ErrorCount, result.Errors)
	}

	stored, err := h.txnRepo.List(repository.TransactionFilter{AccountID: &h.accountID})
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("expected 2 stored transactions, got %d", len(stored))
	}

	byFitID := map[string]*model.Transaction{}
	for _, txn := range stored {
		byFitID[txn.FitID] = txn
	}
	withSIC := byFitID["E2E-1"]
	if withSIC == nil {
		t.Fatal("E2E-1 was not imported")
	}
	if withSIC.SICCode == nil {
		t.Fatalf("E2E-1 did not persist its SIC value")
	}
	if *withSIC.SICCode != "5812" {
		t.Errorf("E2E-1 SIC = %q, want the canonical %q", *withSIC.SICCode, "5812")
	}
	withoutSIC := byFitID["E2E-2"]
	if withoutSIC == nil {
		t.Fatal("E2E-2 was not imported")
	}
	if withoutSIC.SICCode != nil {
		t.Errorf("E2E-2 has no <SIC> but stored %q", *withoutSIC.SICCode)
	}
}

// TestImport_SICFreeFileBehaviourUnchanged is the NFR2 / NFR-U1-COMP-01
// regression guard: a SIC-free import keeps its existing counts, dedup, and
// pattern categorization behaviour.
func TestImport_SICFreeFileBehaviourUnchanged(t *testing.T) {
	h := newImportHarness(t)

	category := &model.Category{Name: "Dining", CategoryType: model.CategoryTypeExpense}
	if err := h.catRepo.Create(category); err != nil {
		t.Fatalf("create category: %v", err)
	}
	if err := h.patRepo.Create(&model.CategoryPattern{PatternName: "CAFE", CategoryID: category.CategoryID}); err != nil {
		t.Fatalf("create pattern: %v", err)
	}
	categorizer := NewCategorizer(h.patRepo, h.txnRepo)
	if err := categorizer.LoadRules(); err != nil {
		t.Fatalf("load patterns: %v", err)
	}
	accountRepo := repository.NewAccountRepository(h.db)
	svc := NewImportService(parser.NewOFXParser(), h.txnRepo, accountRepo, categorizer,
		repository.NewImportBatchRepository(h.db))

	first, err := svc.ImportOFX(strings.NewReader(e2eOFX("", "")), h.accountID, nil)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.TotalTransactions != 2 || first.ImportedCount != 2 || first.DuplicateCount != 0 {
		t.Errorf("first import counts = total %d imported %d duplicate %d, want 2/2/0",
			first.TotalTransactions, first.ImportedCount, first.DuplicateCount)
	}
	if first.CategorizedCount != 1 {
		t.Errorf("total_auto_categorized = %d, want 1 (text pattern CAFE)", first.CategorizedCount)
	}

	// Re-importing the identical file must be fully deduplicated (FR2).
	second, err := svc.ImportOFX(strings.NewReader(e2eOFX("", "")), h.accountID, nil)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.ImportedCount != 0 || second.DuplicateCount != 2 {
		t.Errorf("re-import counts = imported %d duplicate %d, want 0/2",
			second.ImportedCount, second.DuplicateCount)
	}
}

// TestImport_SICDoesNotAffectDeduplication covers AC "the existing
// deduplication key remains unchanged and does not include SIC" through the
// import service: the same statement re-sent with a different SIC must still
// deduplicate.
func TestImport_SICDoesNotAffectDeduplication(t *testing.T) {
	h := newImportHarness(t)

	first, err := h.service.ImportOFX(strings.NewReader(e2eOFX("<SIC>5812", "<SIC>7011")), h.accountID, nil)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.ImportedCount != 2 {
		t.Fatalf("first import imported %d, want 2", first.ImportedCount)
	}

	// Identical identity, different SIC values.
	second, err := h.service.ImportOFX(strings.NewReader(e2eOFX("<SIC>9999", "")), h.accountID, nil)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.ImportedCount != 0 {
		t.Errorf("changing SIC created %d new transactions; SIC must not be part of the duplicate key", second.ImportedCount)
	}
	if second.DuplicateCount != 2 {
		t.Errorf("DuplicateCount = %d, want 2", second.DuplicateCount)
	}

	// The originally stored SIC values must be untouched by the re-import.
	stored, err := h.txnRepo.List(repository.TransactionFilter{AccountID: &h.accountID})
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(stored))
	}
	want := map[string]string{"E2E-1": "5812", "E2E-2": "7011"}
	for _, txn := range stored {
		if txn.SICCode == nil {
			t.Errorf("%s lost its SIC value", txn.FitID)
			continue
		}
		if string(*txn.SICCode) != want[txn.FitID] {
			t.Errorf("%s SIC = %q, want %q", txn.FitID, *txn.SICCode, want[txn.FitID])
		}
	}
}

// TestImport_SICDoesNotCategorize confirms the UOW-1 boundary: storing SIC must
// not itself categorize anything. Mapping-driven categorization is UOW-3.
func TestImport_SICDoesNotCategorize(t *testing.T) {
	h := newImportHarness(t)

	category := &model.Category{Name: "Dining", CategoryType: model.CategoryTypeExpense}
	if err := h.catRepo.Create(category); err != nil {
		t.Fatalf("create category: %v", err)
	}
	sicRepo := repository.NewSICMappingRepository(h.db)
	if err := sicRepo.Create(model.NewSICMapping("5812", "Eating Places", "", &category.CategoryID)); err != nil {
		t.Fatalf("create mapping: %v", err)
	}

	result, err := h.service.ImportOFX(strings.NewReader(e2eOFX("<SIC>5812", "")), h.accountID, nil)
	if err != nil {
		t.Fatalf("ImportOFX(): %v", err)
	}
	if result.CategorizedCount != 0 {
		t.Errorf("UOW-1 categorized %d transactions by SIC; that behaviour belongs to UOW-3", result.CategorizedCount)
	}

	stored, _ := h.txnRepo.List(repository.TransactionFilter{AccountID: &h.accountID})
	for _, txn := range stored {
		if txn.CategoryID != nil {
			t.Errorf("%s was categorized during UOW-1 import", txn.FitID)
		}
		if txn.CategorySource != model.CategorySourceNone {
			t.Errorf("%s has category_source %d, want 0", txn.FitID, txn.CategorySource)
		}
	}
	// The joined description is still available for downstream modal display.
	for _, txn := range stored {
		if txn.FitID == "E2E-1" {
			if txn.SICDescription == nil || *txn.SICDescription != "Eating Places" {
				t.Errorf("E2E-1 sic_description = %v, want %q", txn.SICDescription, "Eating Places")
			}
		}
	}
}
