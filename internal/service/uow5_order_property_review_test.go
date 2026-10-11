package service

import (
	"fmt"
	"testing"

	"github.com/oronno/privateledger/internal/model"
	"pgregory.net/rapid"
)

type uow5RuleDef struct {
	pattern  string
	code     model.SICCode
	category int
}

type uow5TxnDef struct {
	details  string
	code     string
	category *int
	source   model.CategorySource
}

type uow5Outcome struct {
	category int
	has      bool
	source   model.CategorySource
}

func TestReviewU5RuleCreationOrderIndependenceProperty(t *testing.T) {
	withUOW3RapidSeed(t)
	db, txnRepo, patternRepo, sicRepo, _, categorizer, accountID := newUOW3ServiceHarness(t)
	categories := []int{
		createUOW3Category(t, db, "Alpha"),
		createUOW3Category(t, db, "Beta"),
		createUOW3Category(t, db, "Gamma"),
	}
	oldCategory := createUOW3Category(t, db, "Historical")
	allRules := []uow5RuleDef{
		{pattern: "ALPHA", code: "100", category: categories[0]},
		{pattern: "BETA", code: "200", category: categories[1]},
		{pattern: "GAMMA", code: "300", category: categories[2]},
	}

	rapid.Check(t, func(rt *rapid.T) {
		active := make([]uow5RuleDef, 0, len(allRules))
		for i, rule := range allRules {
			if rapid.Bool().Draw(rt, fmt.Sprintf("include rule %d", i)) {
				active = append(active, rule)
			}
		}
		if len(active) == 0 {
			active = append(active, allRules[rapid.IntRange(0, len(allRules)-1).Draw(rt, "forced rule")])
		}
		permuted := rapid.Permutation(active).Draw(rt, "creation permutation")

		txnCount := rapid.IntRange(1, 14).Draw(rt, "transaction count")
		transactions := make([]uow5TxnDef, txnCount)
		for i := range transactions {
			details := rapid.SampledFrom([]string{"ALPHA shop", "BETA shop", "GAMMA shop", "NO PATTERN"}).Draw(rt, fmt.Sprintf("details %d", i))
			code := rapid.SampledFrom([]string{"100", "200", "300", "999"}).Draw(rt, fmt.Sprintf("code %d", i))
			state := rapid.IntRange(0, 2).Draw(rt, fmt.Sprintf("state %d", i))
			spec := uow5TxnDef{details: details, code: code, source: model.CategorySourceNone}
			if state == 1 {
				spec.category = &oldCategory
				spec.source = model.CategorySourceRule
			} else if state == 2 {
				spec.category = &oldCategory
				spec.source = model.CategorySourceManual
			}
			transactions[i] = spec
		}

		run := func(order []uow5RuleDef) []uow5Outcome {
			if _, err := db.Exec(`DELETE FROM ledger_transaction`); err != nil {
				rt.Fatal(err)
			}
			if _, err := db.Exec(`DELETE FROM category_pattern`); err != nil {
				rt.Fatal(err)
			}
			if _, err := db.Exec(`DELETE FROM sic_mapping`); err != nil {
				rt.Fatal(err)
			}
			for _, rule := range order {
				if err := patternRepo.Create(model.NewCategoryPattern(rule.pattern, rule.category)); err != nil {
					rt.Fatal(err)
				}
				if err := sicRepo.Create(model.NewSICMapping(rule.code, "", "", &rule.category)); err != nil {
					rt.Fatal(err)
				}
			}
			created := make([]*model.Transaction, len(transactions))
			for i, spec := range transactions {
				created[i] = createUOW3Txn(t, txnRepo, accountID, fmt.Sprintf("order-%d", i), spec.details, spec.code, spec.category, spec.source)
			}
			if _, err := categorizer.Reexamine(); err != nil {
				rt.Fatal(err)
			}
			outcomes := make([]uow5Outcome, len(created))
			for i, txn := range created {
				stored, err := txnRepo.GetByID(txn.TransactionID)
				if err != nil {
					rt.Fatal(err)
				}
				outcomes[i].source = stored.CategorySource
				if stored.CategoryID != nil {
					outcomes[i].has = true
					outcomes[i].category = *stored.CategoryID
				}
			}
			return outcomes
		}

		first := run(active)
		second := run(permuted)
		if len(first) != len(second) {
			rt.Fatalf("outcome lengths differ: %d vs %d", len(first), len(second))
		}
		for i := range first {
			if first[i] != second[i] {
				rt.Fatalf("transaction %d depends on rule creation order: first=%+v permuted=%+v", i, first[i], second[i])
			}
		}
	})
}
