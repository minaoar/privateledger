package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

func TestReviewU5EmptyCategoryMappingCreationStillInvokesReexamination(t *testing.T) {
	f := newSeedFixture(t)
	calls := 0
	service := reviewService(f, reviewCollaborator{handoff: func() (RecategorizationCounts, error) {
		calls++
		return RecategorizationCounts{}, nil
	}})
	result, err := service.CreateMapping(context.Background(), model.SICMappingInput{SICCode: "5812"})
	if err != nil || result == nil || !result.MappingCommitted {
		t.Fatalf("create result=%+v err=%v", result, err)
	}
	if calls != 1 {
		t.Fatalf("mapping creation invoked Reexamine %d times, want exactly once under BR-U5-01", calls)
	}
}

func TestReviewU5MappingTriggersNeverWriteManualTransactions(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete", "upload"} {
		t.Run(operation, func(t *testing.T) {
			f := newSeedFixture(t)
			manualCategory := f.addCategory("Manual")
			targetA := f.addCategory("Target A")
			targetB := f.addCategory("Target B")
			if _, err := f.db.Exec(`INSERT INTO account (name) VALUES ('Manual trigger review')`); err != nil {
				t.Fatal(err)
			}
			txnRepo := repository.NewTransactionRepository(f.db)
			manual := &model.Transaction{
				AccountID: 1, TrnType: "DEBIT", FitID: "manual-" + operation,
				DatePosted: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC), Amount: -1,
				TransactionDetails: "MERCHANT", TransactionType: model.TransactionTypeDebit,
				SICCode: uow3SIC("5812"), CategoryID: &manualCategory, CategorySource: model.CategorySourceManual,
			}
			if err := txnRepo.Create(manual); err != nil {
				t.Fatal(err)
			}
			sic := NewSICMappingCategorizer(f.sicRepo, txnRepo)
			_ = NewCategorizerWithSIC(repository.NewCategoryPatternRepository(f.db), txnRepo, sic)
			svc := NewSICMappingManagementService(f.sicRepo, f.catRepo, f.dir, time.Second, sic)

			switch operation {
			case "create":
				if _, err := svc.CreateMapping(context.Background(), model.SICMappingInput{SICCode: "5812", CategoryID: &targetA}); err != nil {
					t.Fatal(err)
				}
			case "update":
				mapping := model.NewSICMapping("5812", "", "", &targetA)
				if err := f.sicRepo.Create(mapping); err != nil {
					t.Fatal(err)
				}
				if _, err := svc.UpdateMapping(context.Background(), mapping.SICMappingID, model.SICMappingInput{SICCode: "5812", CategoryID: &targetB}); err != nil {
					t.Fatal(err)
				}
			case "delete":
				mapping := model.NewSICMapping("5812", "", "", &targetA)
				if err := f.sicRepo.Create(mapping); err != nil {
					t.Fatal(err)
				}
				if _, err := svc.DeleteMapping(context.Background(), mapping.SICMappingID); err != nil {
					t.Fatal(err)
				}
			case "upload":
				body := reviewCSV(t, [][]string{{"5812", "", "", "Target A", fmt.Sprint(targetA)}})
				if _, err := svc.MergeUpload(context.Background(), strings.NewReader(body)); err != nil {
					t.Fatal(err)
				}
			}

			stored, err := txnRepo.GetByID(manual.TransactionID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.CategoryID == nil || *stored.CategoryID != manualCategory || stored.CategorySource != model.CategorySourceManual {
				t.Fatalf("%s trigger changed manual transaction: %+v", operation, stored)
			}
		})
	}
}

func TestReviewU5MappingResultsCarryAllThreeCounts(t *testing.T) {
	f := newSeedFixture(t)
	category := f.addCategory("Mapped")
	service := reviewService(f, reviewCollaborator{handoff: func() (RecategorizationCounts, error) {
		return RecategorizationCounts{Moved: 4, Uncategorized: 3, ManualProtected: 2}, nil
	}})
	result, err := service.CreateMapping(context.Background(), model.SICMappingInput{SICCode: "5812", CategoryID: &category})
	if err != nil {
		t.Fatal(err)
	}
	assertCounts := func(label string, value any) {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, expected := range []string{`"moved_count":4`, `"uncategorized_count":3`, `"manual_protected_count":2`} {
			if !strings.Contains(string(payload), expected) {
				t.Errorf("%s result does not report %s: %s", label, expected, payload)
			}
		}
	}
	assertCounts("mapping mutation", result)

	upload, err := service.MergeUpload(context.Background(), strings.NewReader(
		reviewCSV(t, [][]string{{"7011", "", "", "Mapped", fmt.Sprint(category)}})))
	if err != nil {
		t.Fatal(err)
	}
	assertCounts("mapping upload", upload)
}
