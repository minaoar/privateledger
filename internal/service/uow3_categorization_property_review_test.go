package service

import (
	"database/sql"
	"flag"
	"fmt"
	"testing"

	"github.com/oronno/privateledger/internal/model"
	"pgregory.net/rapid"
)

func withUOW3RapidSeed(t *testing.T) {
	t.Helper()
	seed := flag.Lookup("rapid.seed")
	original := seed.Value.String()
	if original == "0" {
		if err := flag.Set("rapid.seed", "20260906"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = flag.Set("rapid.seed", original) })
	}
	t.Logf("UOW-3 property replay seed=%s", seed.Value.String())
}

// TestReviewU3PriorityProperty generates the meaningful priority partitions
// and checks them against the single decision function. Rapid records and
// shrinks any counterexample.
func TestReviewU3PriorityProperty(t *testing.T) {
	withUOW3RapidSeed(t)
	rapid.Check(t, func(rt *rapid.T) {
		manual := rapid.Bool().Draw(rt, "manual")
		existing := rapid.Bool().Draw(rt, "existing category")
		patternMatches := rapid.Bool().Draw(rt, "pattern matches")
		hasSIC := rapid.Bool().Draw(rt, "has SIC")
		mappingState := rapid.IntRange(0, 2).Draw(rt, "mapping state: 0 missing, 1 empty, 2 categorized")

		lookup := &uow3Lookup{categories: map[model.SICCode]int{}}
		if mappingState == 1 {
			lookup.categories["5812"] = 0
		} else if mappingState == 2 {
			lookup.categories["5812"] = 22
		}
		categorizer := &Categorizer{sicLookup: lookup}
		if patternMatches {
			categorizer.patterns = []*model.CategoryPattern{{PatternName: "MATCH", CategoryID: 11}}
		} else {
			categorizer.patterns = []*model.CategoryPattern{{PatternName: "OTHER", CategoryID: 11}}
		}
		txn := &model.Transaction{TransactionDetails: "MATCH merchant"}
		if manual {
			txn.CategorySource = model.CategorySourceManual
		}
		if existing {
			txn.CategoryID = uow3Category(7)
		}
		if hasSIC {
			txn.SICCode = uow3SIC("5812")
		}

		changed := categorizer.Categorize(txn)
		wantChanged, wantCategory := false, 0
		switch {
		case manual || existing:
		case patternMatches:
			wantChanged, wantCategory = true, 11
		case hasSIC && mappingState == 2:
			wantChanged, wantCategory = true, 22
		}
		if changed != wantChanged {
			rt.Fatalf("changed=%v want=%v for manual=%v existing=%v pattern=%v SIC=%v mapping=%d", changed, wantChanged, manual, existing, patternMatches, hasSIC, mappingState)
		}
		if wantChanged && (txn.CategoryID == nil || *txn.CategoryID != wantCategory || txn.CategorySource != model.CategorySourceRule) {
			rt.Fatalf("assignment=(%v,%v), want category=%d source=rule", txn.CategoryID, txn.CategorySource, wantCategory)
		}
	})
}

type uow3TxnSnapshot struct {
	category sql.NullInt64
	source   model.CategorySource
}

// TestReviewU5WholeTableReexaminationProperty checks the amended whole-table
// contract across generated transaction states. Current rules decide every
// non-manual row; manual rows remain byte-for-byte unchanged.
func TestReviewU5WholeTableReexaminationProperty(t *testing.T) {
	withUOW3RapidSeed(t)
	db, txnRepo, _, sicRepo, sic, categorizer, accountID := newUOW3ServiceHarness(t)
	mappedCategory := createUOW3Category(t, db, "Mapped")
	otherCategory := createUOW3Category(t, db, "Existing")
	for _, code := range []model.SICCode{"100", "200", "300"} {
		if err := sicRepo.Create(model.NewSICMapping(code, "", "", &mappedCategory)); err != nil {
			t.Fatal(err)
		}
	}
	if err := categorizer.LoadRules(); err != nil {
		t.Fatal(err)
	}

	rapid.Check(t, func(rt *rapid.T) {
		if _, err := db.Exec(`DELETE FROM ledger_transaction`); err != nil {
			rt.Fatal(err)
		}
		count := rapid.IntRange(1, 16).Draw(rt, "transaction count")
		before := map[int]uow3TxnSnapshot{}
		transactionCodes := map[int]model.SICCode{}
		wantMoved, wantUncategorized, wantManual := 0, 0, 0
		for i := 0; i < count; i++ {
			code := rapid.SampledFrom([]model.SICCode{"100", "200", "300", "999"}).Draw(rt, fmt.Sprintf("code %d", i))
			hasCategory := rapid.Bool().Draw(rt, fmt.Sprintf("has category %d", i))
			var category *int
			source := model.CategorySourceNone
			if hasCategory {
				category = &otherCategory
				source = rapid.SampledFrom([]model.CategorySource{model.CategorySourceNone, model.CategorySourceRule, model.CategorySourceManual}).Draw(rt, fmt.Sprintf("source %d", i))
			}
			txn := createUOW3Txn(t, txnRepo, accountID, fmt.Sprintf("prop-%d", i), "MERCHANT", string(code), category, source)
			oldCategory := sql.NullInt64{Valid: category != nil}
			if category != nil {
				oldCategory.Int64 = int64(*category)
			}
			before[txn.TransactionID] = uow3TxnSnapshot{category: oldCategory, source: source}
			transactionCodes[txn.TransactionID] = code
			hasMapping := code != "999"
			wouldChange := hasMapping && (category == nil || *category != mappedCategory) || !hasMapping && category != nil
			if source == model.CategorySourceManual {
				if wouldChange {
					wantManual++
				}
			} else if wouldChange {
				if hasMapping {
					wantMoved++
				} else {
					wantUncategorized++
				}
			}
		}

		got, err := sic.Reexamine()
		if err != nil {
			rt.Fatal(err)
		}
		if got.Moved != wantMoved || got.Uncategorized != wantUncategorized || got.ManualProtected != wantManual {
			rt.Fatalf("counts=%+v, want moved=%d uncategorized=%d manual=%d", got, wantMoved, wantUncategorized, wantManual)
		}
		for id, old := range before {
			stored, err := txnRepo.GetByID(id)
			if err != nil {
				rt.Fatal(err)
			}
			if old.source == model.CategorySourceManual {
				newCategory := sql.NullInt64{}
				if stored.CategoryID != nil {
					newCategory = sql.NullInt64{Valid: true, Int64: int64(*stored.CategoryID)}
				}
				if newCategory != old.category || stored.CategorySource != old.source {
					rt.Fatalf("manual transaction %d changed from category=%v source=%v to category=%v source=%v", id, old.category, old.source, newCategory, stored.CategorySource)
				}
				continue
			}
			if transactionCodes[id] == "999" {
				if stored.CategoryID != nil || stored.CategorySource != model.CategorySourceNone {
					rt.Fatalf("unmatched transaction %d stored=%+v, want uncategorized", id, stored)
				}
			} else if stored.CategoryID == nil || *stored.CategoryID != mappedCategory || stored.CategorySource != model.CategorySourceRule {
				rt.Fatalf("mapped transaction %d stored=%+v, want category=%d source=rule", id, stored, mappedCategory)
			}
		}
	})
}
