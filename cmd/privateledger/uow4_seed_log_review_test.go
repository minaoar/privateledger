package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

func TestReviewU4SeedLogCarriesBoundedRepairMessage(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(database.Config{Path: filepath.Join(dir, "privateledger.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	result, err := db.Exec(`INSERT INTO category (name, category_type) VALUES (?, 2)`, "Food")
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	seedPath := filepath.Join(dir, "sic_mappings.csv")
	seed := fmt.Sprintf("SIC_Code,Description,Description_Detail,Category_Name,Category_ID\n5812,,,Groceries,%d\n", id)
	if err := os.WriteFile(seedPath, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}

	svc := service.NewSICMappingService(repository.NewSICMappingRepository(db), repository.NewCategoryRepository(db))
	report, err := svc.ImportFileIfPresentWithReport(seedPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Outcome != model.SICMappingImportInvalid || len(report.Errors) != 1 {
		t.Fatalf("seed did not produce one rejected-row diagnostic: %+v", report)
	}

	records := captureStartupLogs(t, func() { logSICMappingImportOutcome(seedPath, report) })
	if len(records) != 2 {
		t.Fatalf("seed logging emitted %d records, want row plus summary: %v", len(records), records)
	}
	want := fmt.Sprintf("Category_Name \"Groceries\" does not exist; Category_ID %d is currently \"Food\"", id)
	if got := records[0]["message"]; got != want {
		t.Fatalf("seed row message = %v, want %q", got, want)
	}
	message, ok := records[0]["message"].(string)
	if !ok || !utf8.ValidString(message) || utf8.RuneCountInString(message) > 512 {
		t.Fatalf("seed row message is not bounded valid UTF-8: %v", records[0]["message"])
	}
	for _, r := range message {
		if unicode.IsControl(r) {
			t.Fatalf("seed row message contains control rune %U", r)
		}
	}
	if records[0]["code"] != "category_not_found" || records[0]["field"] != "Category_Name" {
		t.Fatalf("seed row lost stable diagnostic fields: %v", records[0])
	}
}
