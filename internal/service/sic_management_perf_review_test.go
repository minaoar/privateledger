package service

import (
	"context"
	"fmt"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestReviewU2MergePerformance(t *testing.T) {
	if testing.Short() || raceDetectorEnabled {
		t.Skip("uninstrumented performance acceptance only")
	}
	f := newSeedFixture(t)
	cats := make([]int, 100)
	for i := range cats {
		cats[i] = f.addCategory(fmt.Sprintf("ReviewCategory%03d", i))
	}
	pre := make([]*model.SICMapping, 100000)
	for i := range pre {
		cat := cats[i%100]
		pre[i] = model.NewSICMapping(model.SICCode(fmt.Sprint(i+1)), "same", "", &cat)
	}
	rows := make([][]string, 0, 100000)
	for code := 1; code <= 75000; code++ {
		desc := "same"
		if code <= 25000 {
			desc = "changed"
		}
		idx := (code - 1) % 100
		rows = append(rows, []string{fmt.Sprint(code), desc, "", fmt.Sprintf("ReviewCategory%03d", idx), fmt.Sprint(cats[idx])})
	}
	for code := 100001; code <= 125000; code++ {
		idx := (code - 1) % 100
		rows = append(rows, []string{fmt.Sprint(code), "new", "", fmt.Sprintf("ReviewCategory%03d", idx), fmt.Sprint(cats[idx])})
	}
	body := reviewCSV(t, rows)
	if int64(len(body)) > model.MaxSICMappingFileSize {
		t.Fatal("oversize fixture")
	}
	if _, err := f.db.Exec(`INSERT INTO account (name) VALUES ('UOW-5 merge performance')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`
		WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x < 20000)
		INSERT INTO ledger_transaction
			(account_id, trn_type, fit_id, date_posted, amount, transaction_details,
			 transaction_type, sic_code, category_source)
		SELECT 1, 'DEBIT', printf('merge-reexam-%06d', x), '2026-01-01 00:00:00', -1,
			'MERCHANT', 1, CAST(((x-1) % 1000) + 1 AS TEXT), 0 FROM n`); err != nil {
		t.Fatal(err)
	}
	txnRepo := repository.NewTransactionRepository(f.db)
	sic := NewSICMappingCategorizer(f.sicRepo, txnRepo)
	_ = NewCategorizerWithSIC(repository.NewCategoryPatternRepository(f.db), txnRepo, sic)
	s := reviewService(f, sic)
	var elapsed []time.Duration
	for run := 0; run < 6; run++ {
		if _, e := f.db.Exec("DELETE FROM sic_mapping"); e != nil {
			t.Fatal(e)
		}
		if e := f.sicRepo.BulkInsertAtomic(pre); e != nil {
			t.Fatal(e)
		}
		if _, e := f.db.Exec(`UPDATE ledger_transaction SET category_id = NULL, category_source = 0`); e != nil {
			t.Fatal(e)
		}
		start := time.Now()
		r, e := s.MergeUpload(context.Background(), strings.NewReader(body))
		duration := time.Since(start)
		if e != nil || !r.MappingCommitted || r.CreatedRows != 25000 || r.UpdatedRows != 25000 || r.UnchangedRows != 50000 || r.RecategorizedRows != 20000 || r.BackupPath == "" || r.BackupWarning != "" {
			t.Fatalf("run %d %+v %v", run, r, e)
		}
		if _, e := os.Stat(r.BackupPath); e != nil {
			t.Fatal(e)
		}
		if f.mappingCount() != 125000 {
			t.Fatal("omitted mappings lost")
		}
		t.Logf("run=%d (0 warmup), elapsed=%v csv_bytes=%d", run, duration, len(body))
		if run > 0 {
			elapsed = append(elapsed, duration)
		}
	}
	sort.Slice(elapsed, func(i, j int) bool { return elapsed[i] < elapsed[j] })
	info, err := os.Stat(filepath.Join(f.dir, "privateledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("NFR-U5-PERF-02 fixture: mappings_before=100000 mappings_after=125000 transactions=20000 manual=0 (0%%) database_bytes=%d", info.Size())
	t.Logf("PERF-01 median=%v target=10s", elapsed[2])
	if elapsed[2] > 10*time.Second {
		t.Fatalf("merge median exceeds target: %v", elapsed[2])
	}
}
