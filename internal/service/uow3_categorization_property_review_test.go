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

// TestReviewU3ScopedRecategorizationProperty generates affected sets and
// transaction states. Only a category-less, source-none row inside the set may
// change; every other row must remain byte-for-byte identical in the fields the
// pass owns.
func TestReviewU3ScopedRecategorizationProperty(t *testing.T) {
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
		affected := map[model.SICCode]bool{}
		var codes []model.SICCode
		for _, code := range []model.SICCode{"100", "200", "300"} {
			if rapid.Bool().Draw(rt, "affect "+string(code)) {
				affected[code] = true
				codes = append(codes, code)
			}
		}
		count := rapid.IntRange(1, 16).Draw(rt, "transaction count")
		before := map[int]uow3TxnSnapshot{}
		transactionCodes := map[int]model.SICCode{}
		eligible := 0
		for i := 0; i < count; i++ {
			code := rapid.SampledFrom([]model.SICCode{"100", "200", "300", "999"}).Draw(rt, fmt.Sprintf("code %d", i))
			source := rapid.SampledFrom([]model.CategorySource{model.CategorySourceNone, model.CategorySourceRule, model.CategorySourceManual}).Draw(rt, fmt.Sprintf("source %d", i))
			hasCategory := rapid.Bool().Draw(rt, fmt.Sprintf("has category %d", i))
			var category *int
			if hasCategory {
				category = &otherCategory
			}
			txn := createUOW3Txn(t, txnRepo, accountID, fmt.Sprintf("prop-%d", i), "MERCHANT", string(code), category, source)
			oldCategory := sql.NullInt64{Valid: category != nil}
			if category != nil {
				oldCategory.Int64 = int64(*category)
			}
			before[txn.TransactionID] = uow3TxnSnapshot{category: oldCategory, source: source}
			transactionCodes[txn.TransactionID] = code
			if affected[code] && category == nil && source == model.CategorySourceNone {
				eligible++
			}
		}

		gotCount, err := sic.RecategorizeBySICCodes(codes)
		if err != nil {
			rt.Fatal(err)
		}
		if gotCount != eligible {
			rt.Fatalf("changed count=%d, want %d eligible rows", gotCount, eligible)
		}
		for id, old := range before {
			stored, err := txnRepo.GetByID(id)
			if err != nil {
				rt.Fatal(err)
			}
			mayChange := affected[transactionCodes[id]] && !old.category.Valid && old.source == model.CategorySourceNone
			if mayChange {
				if stored.CategoryID == nil || *stored.CategoryID != mappedCategory || stored.CategorySource != model.CategorySourceRule {
					rt.Fatalf("eligible transaction %d not mapped: category=%v source=%v", id, stored.CategoryID, stored.CategorySource)
				}
				continue
			}
			newCategory := sql.NullInt64{}
			if stored.CategoryID != nil {
				newCategory.Valid = true
				newCategory.Int64 = int64(*stored.CategoryID)
			}
			if newCategory != old.category || stored.CategorySource != old.source {
				rt.Fatalf("out-of-scope transaction %d changed from category=%v source=%v to category=%v source=%v", id, old.category, old.source, newCategory, stored.CategorySource)
			}
		}
	})
}
