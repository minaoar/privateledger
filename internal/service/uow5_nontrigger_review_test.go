package service

import (
	"context"
	"strings"
	"testing"

	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/parser"
	"github.com/oronno/privateledger/internal/repository"
)

func TestReviewU5DescriptionOnlyMappingEditDoesNotReexamine(t *testing.T) {
	f := newSeedFixture(t)
	category := f.addCategory("Mapped")
	calls := 0
	svc := reviewService(f, reviewCollaborator{handoff: func() (RecategorizationCounts, error) {
		calls++
		return RecategorizationCounts{}, nil
	}})
	created, err := svc.CreateMapping(context.Background(), model.SICMappingInput{SICCode: "5812", Description: "before", CategoryID: &category})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("create calls=%d, want 1", calls)
	}
	if _, err := svc.UpdateMapping(context.Background(), created.Mapping.SICMappingID,
		model.SICMappingInput{SICCode: "5812", Description: "after", DescriptionDetail: "metadata only", CategoryID: &category}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("description-only edit invoked Reexamine; calls=%d", calls)
	}
}

func TestReviewU5ImportLeavesExistingCategorizationUntouched(t *testing.T) {
	db, txnRepo, patternRepo, sicRepo, _, categorizer, accountID := newUOW3ServiceHarness(t)
	oldCategory := createUOW3Category(t, db, "Existing")
	currentCategory := createUOW3Category(t, db, "Current rule")
	if err := patternRepo.Create(model.NewCategoryPattern("CAFE", currentCategory)); err != nil {
		t.Fatal(err)
	}
	if err := sicRepo.Create(model.NewSICMapping("5812", "", "", &currentCategory)); err != nil {
		t.Fatal(err)
	}
	if err := categorizer.LoadRules(); err != nil {
		t.Fatal(err)
	}
	existing := createUOW3Txn(t, txnRepo, accountID, "existing-before-import", "CAFE", "5812", &oldCategory, model.CategorySourceRule)
	importer := NewImportService(parser.NewOFXParser(), txnRepo, repository.NewAccountRepository(db), categorizer, repository.NewImportBatchRepository(db))
	if _, err := importer.ImportOFX(strings.NewReader(e2eOFX("<SIC>5812", "")), accountID, nil); err != nil {
		t.Fatal(err)
	}
	stored, err := txnRepo.GetByID(existing.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CategoryID == nil || *stored.CategoryID != oldCategory || stored.CategorySource != model.CategorySourceRule {
		t.Fatalf("import re-examined an existing transaction: %+v", stored)
	}
}
