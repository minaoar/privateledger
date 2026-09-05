package repository

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
)

// newTestDB creates an isolated, fully migrated database. Tests never touch the
// user's privateledger.db (PROJECT_GUIDELINES build-and-verification rules).
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(database.Config{Path: filepath.Join(t.TempDir(), "test.db")})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestAccount(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO account (name) VALUES (?)`, name)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("account id: %v", err)
	}
	return int(id)
}

func newTestCategory(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO category (name, category_type) VALUES (?, 2)`, name)
	if err != nil {
		t.Fatalf("insert category %q: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("category id: %v", err)
	}
	return int(id)
}

func sicPtr(s string) *model.SICCode {
	c := model.SICCode(s)
	return &c
}

// ---------------------------------------------------------------------------
// Transaction repository - FR2 / BR-TXN-01..03
// ---------------------------------------------------------------------------

// TestTransactionRepository_SICRoundTrip covers FR2: a canonical SIC survives a
// full write/read cycle and an absent SIC round trips as nil.
func TestTransactionRepository_SICRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewTransactionRepository(db)
	accountID := newTestAccount(t, db, "Chequing")

	cases := []struct {
		name string
		sic  *model.SICCode
	}{
		{name: "with SIC", sic: sicPtr("5812")},
		{name: "without SIC", sic: nil},
		{name: "max int64 SIC", sic: sicPtr("9223372036854775807")},
		{name: "minimum SIC", sic: sicPtr("1")},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			txn := &model.Transaction{
				AccountID:          accountID,
				TrnType:            "DEBIT",
				FitID:              "FIT-" + tc.name,
				DatePosted:         time.Date(2025, 12, 2+i, 12, 0, 0, 0, time.UTC),
				Amount:             -12.34,
				TransactionDetails: "MERCHANT",
				TransactionType:    model.TransactionTypeDebit,
				SICCode:            tc.sic,
			}
			if err := repo.Create(txn); err != nil {
				t.Fatalf("Create(): %v", err)
			}
			if txn.TransactionID == 0 {
				t.Fatalf("Create() did not assign a transaction id")
			}

			got, err := repo.GetByID(txn.TransactionID)
			if err != nil {
				t.Fatalf("GetByID(): %v", err)
			}
			assertSICEqual(t, "GetByID", got.SICCode, tc.sic)

			dupe, err := repo.FindDuplicate(txn.AccountID, txn.TrnType, txn.FitID, txn.DatePosted)
			if err != nil {
				t.Fatalf("FindDuplicate(): %v", err)
			}
			if dupe == nil {
				t.Fatalf("FindDuplicate() returned nil for an existing transaction")
			}
			assertSICEqual(t, "FindDuplicate", dupe.SICCode, tc.sic)

			listed, err := repo.List(TransactionFilter{AccountID: &accountID})
			if err != nil {
				t.Fatalf("List(): %v", err)
			}
			var found *model.Transaction
			for _, l := range listed {
				if l.TransactionID == txn.TransactionID {
					found = l
				}
			}
			if found == nil {
				t.Fatalf("List() did not return the created transaction")
			}
			assertSICEqual(t, "List", found.SICCode, tc.sic)
		})
	}
}

func assertSICEqual(t *testing.T, op string, got, want *model.SICCode) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s: expected no SIC, got %q", op, *got)
	case want != nil && got == nil:
		t.Errorf("%s: expected SIC %q, got nil", op, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s: expected SIC %q, got %q", op, *want, *got)
	}
}

// TestTransactionRepository_SICIsNotPartOfDeduplication covers BR-TXN-01 and
// AC "the existing deduplication key remains unchanged and does not include SIC".
func TestTransactionRepository_SICIsNotPartOfDeduplication(t *testing.T) {
	db := newTestDB(t)
	repo := NewTransactionRepository(db)
	accountID := newTestAccount(t, db, "Chequing")
	posted := time.Date(2025, 12, 2, 12, 0, 0, 0, time.UTC)

	base := &model.Transaction{
		AccountID: accountID, TrnType: "DEBIT", FitID: "F1", DatePosted: posted,
		Amount: -1, TransactionDetails: "A", TransactionType: model.TransactionTypeDebit,
		SICCode: sicPtr("5812"),
	}
	if err := repo.Create(base); err != nil {
		t.Fatalf("create base: %v", err)
	}

	// Same identity, different SIC: must still be detected as a duplicate.
	found, err := repo.FindDuplicate(accountID, "DEBIT", "F1", posted)
	if err != nil {
		t.Fatalf("FindDuplicate(): %v", err)
	}
	if found == nil {
		t.Fatalf("a transaction with a different SIC was not recognised as a duplicate")
	}

	differentSIC := &model.Transaction{
		AccountID: accountID, TrnType: "DEBIT", FitID: "F1", DatePosted: posted,
		Amount: -1, TransactionDetails: "A", TransactionType: model.TransactionTypeDebit,
		SICCode: sicPtr("7011"),
	}
	if err := repo.Create(differentSIC); err == nil {
		t.Errorf("changing only SIC allowed a second row for the same duplicate key")
	}

	// A different fit_id is a different transaction even with the same SIC.
	other := &model.Transaction{
		AccountID: accountID, TrnType: "DEBIT", FitID: "F2", DatePosted: posted,
		Amount: -1, TransactionDetails: "A", TransactionType: model.TransactionTypeDebit,
		SICCode: sicPtr("5812"),
	}
	if err := repo.Create(other); err != nil {
		t.Errorf("a distinct transaction sharing a SIC was rejected: %v", err)
	}

	// A transaction with no SIC at all is unaffected by the new column.
	noSIC := &model.Transaction{
		AccountID: accountID, TrnType: "DEBIT", FitID: "F3", DatePosted: posted,
		Amount: -1, TransactionDetails: "A", TransactionType: model.TransactionTypeDebit,
	}
	if err := repo.Create(noSIC); err != nil {
		t.Errorf("a SIC-free transaction was rejected after the schema change: %v", err)
	}
}

// TestTransactionRepository_GetUncategorizedBySICCodes covers the downstream
// query contract added by plan Step 4.
func TestTransactionRepository_GetUncategorizedBySICCodes(t *testing.T) {
	db := newTestDB(t)
	repo := NewTransactionRepository(db)
	accountID := newTestAccount(t, db, "Chequing")
	categoryID := newTestCategory(t, db, "Dining")

	mk := func(fitID string, sic *model.SICCode, cat *int, source model.CategorySource) *model.Transaction {
		txn := &model.Transaction{
			AccountID: accountID, TrnType: "DEBIT", FitID: fitID,
			DatePosted:      time.Date(2025, 12, 2, 12, 0, 0, 0, time.UTC),
			Amount:          -1,
			TransactionType: model.TransactionTypeDebit,
			SICCode:         sic, CategoryID: cat, CategorySource: source,
		}
		if err := repo.Create(txn); err != nil {
			t.Fatalf("create %s: %v", fitID, err)
		}
		return txn
	}

	uncategorized := mk("U1", sicPtr("5812"), nil, model.CategorySourceNone)
	mk("U2", sicPtr("7011"), nil, model.CategorySourceNone)
	mk("R1", sicPtr("5812"), &categoryID, model.CategorySourceRule)   // already rule-categorized
	mk("M1", sicPtr("5812"), &categoryID, model.CategorySourceManual) // manual, protected
	mk("N1", nil, nil, model.CategorySourceNone)                      // no SIC

	t.Run("empty input short circuits", func(t *testing.T) {
		got, err := repo.GetUncategorizedBySICCodes(nil)
		if err != nil {
			t.Fatalf("GetUncategorizedBySICCodes(nil): %v", err)
		}
		if got == nil {
			t.Errorf("expected a non-nil empty slice for an empty input")
		}
		if len(got) != 0 {
			t.Errorf("expected 0 results for an empty input, got %d", len(got))
		}
		got, err = repo.GetUncategorizedBySICCodes([]string{})
		if err != nil || len(got) != 0 {
			t.Errorf("GetUncategorizedBySICCodes([]) = %v, %v", got, err)
		}
	})

	t.Run("matches only uncategorized rows with that SIC", func(t *testing.T) {
		got, err := repo.GetUncategorizedBySICCodes([]string{"5812"})
		if err != nil {
			t.Fatalf("GetUncategorizedBySICCodes(): %v", err)
		}
		if len(got) != 1 || got[0].TransactionID != uncategorized.TransactionID {
			ids := make([]string, len(got))
			for i, g := range got {
				ids[i] = g.FitID
			}
			t.Fatalf("expected only U1, got %v", ids)
		}
		if got[0].SICCode == nil || *got[0].SICCode != "5812" {
			t.Errorf("returned row lost its SIC value")
		}
	})

	t.Run("multiple codes", func(t *testing.T) {
		got, err := repo.GetUncategorizedBySICCodes([]string{"5812", "7011", "9999"})
		if err != nil {
			t.Fatalf("GetUncategorizedBySICCodes(): %v", err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 matches, got %d", len(got))
		}
	})

	t.Run("unknown code matches nothing", func(t *testing.T) {
		got, err := repo.GetUncategorizedBySICCodes([]string{"4242"})
		if err != nil {
			t.Fatalf("GetUncategorizedBySICCodes(): %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected 0 matches, got %d", len(got))
		}
	})

	t.Run("value is parameterized not interpolated", func(t *testing.T) {
		got, err := repo.GetUncategorizedBySICCodes([]string{"5812') OR 1=1 --"})
		if err != nil {
			t.Fatalf("a hostile value must be bound as a parameter, not break the query: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("SQL injection attempt returned %d rows", len(got))
		}
	})
}

// TestTransactionRepository_SICDescriptionJoin covers the joined display field
// the model exposes for downstream units (FR10 description fallback).
func TestTransactionRepository_SICDescriptionJoin(t *testing.T) {
	db := newTestDB(t)
	txnRepo := NewTransactionRepository(db)
	sicRepo := NewSICMappingRepository(db)
	accountID := newTestAccount(t, db, "Chequing")

	for _, m := range []*model.SICMapping{
		{SICCode: "5812", Description: "Eating Places", DescriptionDetail: "restaurants"},
		{SICCode: "7011", Description: "", DescriptionDetail: "Hotels and Motels"},
		{SICCode: "8021", Description: "", DescriptionDetail: ""},
	} {
		if err := sicRepo.Create(m); err != nil {
			t.Fatalf("create mapping %s: %v", m.SICCode, err)
		}
	}

	cases := []struct {
		fitID string
		sic   *model.SICCode
		want  *string
	}{
		{fitID: "D1", sic: sicPtr("5812"), want: strPtr("Eating Places")},
		{fitID: "D2", sic: sicPtr("7011"), want: strPtr("Hotels and Motels")},
		{fitID: "D3", sic: sicPtr("8021"), want: strPtr("")},
		{fitID: "D4", sic: sicPtr("9999"), want: nil}, // no mapping
		{fitID: "D5", sic: nil, want: nil},            // no SIC
	}
	for i, tc := range cases {
		txn := &model.Transaction{
			AccountID: accountID, TrnType: "DEBIT", FitID: tc.fitID,
			DatePosted: time.Date(2025, 12, 2+i, 12, 0, 0, 0, time.UTC),
			Amount:     -1, TransactionType: model.TransactionTypeDebit, SICCode: tc.sic,
		}
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("create %s: %v", tc.fitID, err)
		}
		got, err := txnRepo.GetByID(txn.TransactionID)
		if err != nil {
			t.Fatalf("GetByID(%s): %v", tc.fitID, err)
		}
		switch {
		case tc.want == nil && got.SICDescription != nil:
			t.Errorf("%s: expected no SIC description, got %q", tc.fitID, *got.SICDescription)
		case tc.want != nil && got.SICDescription == nil:
			t.Errorf("%s: expected SIC description %q, got nil", tc.fitID, *tc.want)
		case tc.want != nil && got.SICDescription != nil && *got.SICDescription != *tc.want:
			t.Errorf("%s: expected SIC description %q, got %q", tc.fitID, *tc.want, *got.SICDescription)
		}
	}

	// The join must not multiply rows in List().
	all, err := txnRepo.List(TransactionFilter{})
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(all) != len(cases) {
		t.Errorf("the sic_mapping join changed the List() row count: got %d, want %d", len(all), len(cases))
	}
}

func strPtr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// SIC mapping repository - FR4 / FR8 / BR-CSV-07 / NFR-U1-REL-04
// ---------------------------------------------------------------------------

func TestSICMappingRepository_CRUDRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewSICMappingRepository(db)
	categoryID := newTestCategory(t, db, "Dining")

	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count() on an empty table: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected an empty mapping table, got %d", count)
	}

	mapping := model.NewSICMapping("5812", "Eating Places", "restaurants", &categoryID)
	if err := repo.Create(mapping); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if mapping.SICMappingID == 0 {
		t.Errorf("Create() did not assign an id")
	}

	got, err := repo.GetByID(mapping.SICMappingID)
	if err != nil {
		t.Fatalf("GetByID(): %v", err)
	}
	if got == nil {
		t.Fatalf("GetByID() returned nil for an existing mapping")
	}
	if got.SICCode != "5812" || got.Description != "Eating Places" || got.DescriptionDetail != "restaurants" {
		t.Errorf("GetByID() lost mapping data: %+v", got)
	}
	if got.CategoryID == nil || *got.CategoryID != categoryID {
		t.Errorf("GetByID() lost the category reference: %+v", got.CategoryID)
	}
	if got.CategoryName == nil || *got.CategoryName != "Dining" {
		t.Errorf("GetByID() did not join the category display name: %+v", got.CategoryName)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("GetByID() did not read created_at")
	}

	// GetByCode must apply the shared normalization authority (BR-SIC-04).
	for _, raw := range []string{"5812", " 5812 ", "0005812"} {
		byCode, err := repo.GetByCode(raw)
		if err != nil {
			t.Fatalf("GetByCode(%q): %v", raw, err)
		}
		if byCode == nil {
			t.Errorf("GetByCode(%q) did not resolve to the canonical mapping", raw)
			continue
		}
		if byCode.SICMappingID != mapping.SICMappingID {
			t.Errorf("GetByCode(%q) returned mapping %d, want %d", raw, byCode.SICMappingID, mapping.SICMappingID)
		}
	}
	if _, err := repo.GetByCode("not-a-code"); err == nil {
		t.Errorf("GetByCode() accepted a non-digit code")
	}
	missing, err := repo.GetByCode("4242")
	if err != nil || missing != nil {
		t.Errorf("GetByCode() for an unmapped code = %v, %v; want nil, nil", missing, err)
	}

	// Nullable category is an intentional value (FR5 / BR-CAT-01).
	unmapped := model.NewSICMapping("7011", "", "Hotels", nil)
	if err := repo.Create(unmapped); err != nil {
		t.Fatalf("Create() with a NULL category: %v", err)
	}
	fetched, err := repo.GetByID(unmapped.SICMappingID)
	if err != nil {
		t.Fatalf("GetByID(): %v", err)
	}
	if fetched.CategoryID != nil {
		t.Errorf("expected a NULL category, got %d", *fetched.CategoryID)
	}
	if fetched.HasCategory() {
		t.Errorf("HasCategory() must be false for an intentionally unmapped code")
	}

	all, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll(): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("GetAll() returned %d mappings, want 2", len(all))
	}

	// Update and Delete.
	mapping.Description = "Restaurants"
	mapping.CategoryID = nil
	if err := repo.Update(mapping); err != nil {
		t.Fatalf("Update(): %v", err)
	}
	updated, _ := repo.GetByID(mapping.SICMappingID)
	if updated.Description != "Restaurants" || updated.CategoryID != nil {
		t.Errorf("Update() did not persist changes: %+v", updated)
	}

	if err := repo.Delete(mapping.SICMappingID); err != nil {
		t.Fatalf("Delete(): %v", err)
	}
	if deleted, _ := repo.GetByID(mapping.SICMappingID); deleted != nil {
		t.Errorf("Delete() left the mapping readable")
	}
	if err := repo.Delete(mapping.SICMappingID); err == nil {
		t.Errorf("deleting a missing mapping should report not found")
	}
	if err := repo.Update(&model.SICMapping{SICMappingID: 99999, SICCode: "1"}); err == nil {
		t.Errorf("updating a missing mapping should report not found")
	}
}

// TestSICMappingRepository_UniqueCodeRejected covers FR8 / AC6.
func TestSICMappingRepository_UniqueCodeRejected(t *testing.T) {
	db := newTestDB(t)
	repo := NewSICMappingRepository(db)

	if err := repo.Create(model.NewSICMapping("5812", "", "", nil)); err != nil {
		t.Fatalf("first Create(): %v", err)
	}
	err := repo.Create(model.NewSICMapping("5812", "different", "", nil))
	if err == nil {
		t.Fatalf("a duplicate SIC mapping was accepted")
	}
	if !strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
		t.Errorf("expected a uniqueness error, got %v", err)
	}
	count, _ := repo.Count()
	if count != 1 {
		t.Errorf("rejected duplicate left %d rows, want 1", count)
	}
}

// TestSICMappingRepository_InvalidCategoryRejected covers the database-
// authoritative foreign key required by BR-MIG-05 and plan Step 5.
func TestSICMappingRepository_InvalidCategoryRejected(t *testing.T) {
	db := newTestDB(t)
	repo := NewSICMappingRepository(db)

	missing := 12345
	if err := repo.Create(model.NewSICMapping("5812", "", "", &missing)); err == nil {
		t.Fatalf("a mapping referencing a non-existent category was accepted")
	}
	count, _ := repo.Count()
	if count != 0 {
		t.Errorf("rejected mapping left %d rows, want 0", count)
	}
}

// TestSICMappingRepository_BulkInsertAtomic covers BR-CSV-07 / NFR-U1-REL-04:
// all candidates commit together or none do.
func TestSICMappingRepository_BulkInsertAtomic(t *testing.T) {
	t.Run("commits a valid set", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewSICMappingRepository(db)
		categoryID := newTestCategory(t, db, "Dining")

		set := []*model.SICMapping{
			model.NewSICMapping("5812", "Eating Places", "", &categoryID),
			model.NewSICMapping("7011", "Hotels", "", nil),
			model.NewSICMapping("5411", "Grocery", "detail", &categoryID),
		}
		if err := repo.BulkInsertAtomic(set); err != nil {
			t.Fatalf("BulkInsertAtomic(): %v", err)
		}
		count, _ := repo.Count()
		if count != 3 {
			t.Errorf("expected 3 mappings, got %d", count)
		}
	})

	t.Run("empty set is a no-op", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewSICMappingRepository(db)
		if err := repo.BulkInsertAtomic(nil); err != nil {
			t.Fatalf("BulkInsertAtomic(nil): %v", err)
		}
		count, _ := repo.Count()
		if count != 0 {
			t.Errorf("expected 0 mappings, got %d", count)
		}
	})

	t.Run("rolls back a duplicate in the middle", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewSICMappingRepository(db)
		set := []*model.SICMapping{
			model.NewSICMapping("5812", "", "", nil),
			model.NewSICMapping("7011", "", "", nil),
			model.NewSICMapping("5812", "", "", nil), // duplicate
			model.NewSICMapping("5411", "", "", nil),
		}
		if err := repo.BulkInsertAtomic(set); err == nil {
			t.Fatalf("expected a uniqueness failure")
		}
		count, err := repo.Count()
		if err != nil {
			t.Fatalf("Count(): %v", err)
		}
		if count != 0 {
			t.Errorf("a failed bulk insert left %d partial rows; the operation must be atomic", count)
		}
	})

	t.Run("rolls back an invalid category reference", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewSICMappingRepository(db)
		missing := 4242
		set := []*model.SICMapping{
			model.NewSICMapping("5812", "", "", nil),
			model.NewSICMapping("7011", "", "", &missing),
		}
		if err := repo.BulkInsertAtomic(set); err == nil {
			t.Fatalf("expected a foreign-key failure")
		}
		count, _ := repo.Count()
		if count != 0 {
			t.Errorf("a failed bulk insert left %d partial rows", count)
		}
	})

	t.Run("does not disturb existing rows on failure", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewSICMappingRepository(db)
		if err := repo.Create(model.NewSICMapping("1111", "keep me", "", nil)); err != nil {
			t.Fatalf("seed: %v", err)
		}
		set := []*model.SICMapping{
			model.NewSICMapping("2222", "", "", nil),
			model.NewSICMapping("1111", "", "", nil), // collides with the existing row
		}
		if err := repo.BulkInsertAtomic(set); err == nil {
			t.Fatalf("expected a uniqueness failure")
		}
		existing, err := repo.GetByCode("1111")
		if err != nil {
			t.Fatalf("GetByCode(): %v", err)
		}
		if existing == nil || existing.Description != "keep me" {
			t.Errorf("the pre-existing mapping was modified by a failed bulk insert: %+v", existing)
		}
		count, _ := repo.Count()
		if count != 1 {
			t.Errorf("expected the pre-operation state (1 row), got %d", count)
		}
	})
}

// TestSICMappingRepository_ReplaceAll covers the UOW-2-facing atomic
// replacement primitive that plan Step 5 requires UOW-1 to establish.
func TestSICMappingRepository_ReplaceAll(t *testing.T) {
	t.Run("replaces the authoritative set", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewSICMappingRepository(db)
		if err := repo.BulkInsertAtomic([]*model.SICMapping{
			model.NewSICMapping("1111", "old", "", nil),
			model.NewSICMapping("2222", "old", "", nil),
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := repo.ReplaceAll([]*model.SICMapping{
			model.NewSICMapping("3333", "new", "", nil),
		}); err != nil {
			t.Fatalf("ReplaceAll(): %v", err)
		}
		all, _ := repo.GetAll()
		if len(all) != 1 || all[0].SICCode != "3333" {
			t.Errorf("ReplaceAll() produced %+v", all)
		}
	})

	t.Run("rolls back and preserves the previous set on failure", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewSICMappingRepository(db)
		if err := repo.BulkInsertAtomic([]*model.SICMapping{
			model.NewSICMapping("1111", "old", "", nil),
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		missing := 999
		if err := repo.ReplaceAll([]*model.SICMapping{
			model.NewSICMapping("3333", "new", "", nil),
			model.NewSICMapping("4444", "new", "", &missing),
		}); err == nil {
			t.Fatalf("expected a foreign-key failure")
		}
		all, _ := repo.GetAll()
		if len(all) != 1 || all[0].SICCode != "1111" || all[0].Description != "old" {
			t.Errorf("a failed ReplaceAll() did not restore the previous mapping set: %+v", all)
		}
	})

	t.Run("empty replacement clears the table", func(t *testing.T) {
		db := newTestDB(t)
		repo := NewSICMappingRepository(db)
		if err := repo.Create(model.NewSICMapping("1111", "", "", nil)); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if err := repo.ReplaceAll(nil); err != nil {
			t.Fatalf("ReplaceAll(nil): %v", err)
		}
		count, _ := repo.Count()
		if count != 0 {
			t.Errorf("expected an empty table, got %d rows", count)
		}
	})
}

// TestSICMappingRepository_CategoryDeletionPreservesMapping covers BR-MIG-05
// at the repository level.
func TestSICMappingRepository_CategoryDeletionPreservesMapping(t *testing.T) {
	db := newTestDB(t)
	repo := NewSICMappingRepository(db)
	categoryID := newTestCategory(t, db, "Dining")

	mapping := model.NewSICMapping("5812", "Eating Places", "", &categoryID)
	if err := repo.Create(mapping); err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if _, err := db.Exec(`DELETE FROM category WHERE category_id = ?`, categoryID); err != nil {
		t.Fatalf("delete category: %v", err)
	}

	got, err := repo.GetByID(mapping.SICMappingID)
	if err != nil {
		t.Fatalf("GetByID(): %v", err)
	}
	if got == nil {
		t.Fatalf("deleting a category deleted the SIC mapping; BR-MIG-05 requires SET NULL")
	}
	if got.CategoryID != nil {
		t.Errorf("expected a NULL category after deletion, got %d", *got.CategoryID)
	}
	if got.CategoryName != nil {
		t.Errorf("expected no joined category name, got %q", *got.CategoryName)
	}
}
