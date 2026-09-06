package repository

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
)

func TestReviewU2RepositoryMergeForeignKeyRollback(t *testing.T) {
	db := newTestDB(t)
	r := NewSICMappingRepository(db)
	if e := r.Create(model.NewSICMapping("1", "old", "", nil)); e != nil {
		t.Fatal(e)
	}
	missing := 99999
	e := r.MergeAll(context.Background(), []*model.SICMapping{model.NewSICMapping("1", "changed", "", nil), model.NewSICMapping("2", "new", "", nil), model.NewSICMapping("3", "bad", "", &missing)})
	if e == nil {
		t.Fatal("foreign key accepted")
	}
	m, _ := r.GetByCode("1")
	n, _ := r.Count()
	if n != 1 || m.Description != "old" {
		t.Fatal("partial merge")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = r.MergeAll(ctx, []*model.SICMapping{model.NewSICMapping("4", "", "", nil)}); e == nil {
		t.Fatal("cancelled transaction accepted")
	}
	if e = r.MergeAll(context.Background(), nil); e != nil {
		t.Fatal(e)
	}
	for _, code := range []model.SICCode{"1000", "999", "10", "2", "9223372036854775807"} {
		if e = r.Create(model.NewSICMapping(code, "", "", nil)); e != nil {
			t.Fatal(e)
		}
	}
	all, e := r.GetAll()
	if e != nil {
		t.Fatal(e)
	}
	for i, code := range []model.SICCode{"1", "2", "10", "999", "1000", "9223372036854775807"} {
		if all[i].SICCode != code {
			t.Fatalf("numeric order %v", all)
		}
	}
}

func TestReviewU2RepositoryClassifiesRealSQLiteBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
	db1, err := database.Open(database.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer db1.Close()
	db2, err := database.Open(database.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	db2.SetMaxOpenConns(1)
	if _, err = db2.Exec("PRAGMA busy_timeout=25"); err != nil {
		t.Fatal(err)
	}

	tx, err := db1.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`
		INSERT INTO sic_mapping (sic_code, description, description_detail, category_id)
		VALUES ('1', '', '', NULL)
	`); err != nil {
		t.Fatal(err)
	}

	repo := NewSICMappingRepository(db2)
	err = repo.Create(model.NewSICMapping("2", "", "", nil))
	if !errors.Is(err, model.ErrSICMappingDatabaseBusy) {
		t.Fatalf("real SQLite lock was not classified as database busy: %v", err)
	}
}
