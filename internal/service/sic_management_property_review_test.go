package service

import (
	"context"
	"flag"
	"fmt"
	"github.com/oronno/privateledger/internal/model"
	"pgregory.net/rapid"
	"reflect"
	"strings"
	"testing"
)

// Each generated record is valid in the shared positive-int64 SIC domain.
// Small overlapping sets expose merge and omission behavior; boundary codes,
// null/category transitions and CSV punctuation exercise meaningful partitions.
func TestReviewU2MergeProperties(t *testing.T) {
	// This package has no parallel tests. Use a recorded default for ordinary
	// CI runs, preserve an explicitly supplied replay seed, and restore the
	// test-only flag afterward so other property suites retain their policy.
	seedFlag := flag.Lookup("rapid.seed")
	originalSeed := seedFlag.Value.String()
	if originalSeed == "0" {
		if err := flag.Set("rapid.seed", "20260906"); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = flag.Set("rapid.seed", originalSeed) }()
	}
	t.Logf("UOW-2 property seed=%s", seedFlag.Value.String())

	f := newSeedFixture(t)
	a := f.addCategory("A")
	b := f.addCategory("B")
	s := reviewService(f, reviewCollaborator{})
	rapid.Check(t, func(rt *rapid.T) {
		if _, e := f.db.Exec("DELETE FROM sic_mapping"); e != nil {
			rt.Fatal(e)
		}
		codes := []string{"1", "2", "9", "10", "999", "1000", "9223372036854775806", "9223372036854775807"}
		type value struct {
			desc, detail string
			cat          int
		}
		before := map[string]value{}
		after := map[string]value{}
		upload := map[string]value{}
		drawValue := func(label string) value {
			return value{rapid.SampledFrom([]string{"", "plain", "comma,quote\"", "line\nbreak", "日本語"}).Draw(rt, label+"description"), rapid.SampledFrom([]string{"", "detail"}).Draw(rt, label+"detail"), rapid.SampledFrom([]int{0, a, b}).Draw(rt, label+"category")}
		}
		var pre []*model.SICMapping
		var rows [][]string
		for _, code := range codes {
			if rapid.Bool().Draw(rt, "existing "+code) {
				v := drawValue("pre " + code)
				before[code] = v
				after[code] = v
				var cat *int
				if v.cat != 0 {
					c := v.cat
					cat = &c
				}
				pre = append(pre, model.NewSICMapping(model.SICCode(code), v.desc, v.detail, cat))
			}
			if rapid.Bool().Draw(rt, "upload "+code) {
				v, ok := before[code]
				if !ok || rapid.Bool().Draw(rt, "change "+code) {
					v = drawValue("upload " + code)
				}
				upload[code] = v
				after[code] = v
				name, id := "", ""
				if v.cat != 0 {
					id = fmt.Sprint(v.cat)
					name = "A"
					if v.cat == b {
						name = "B"
					}
				}
				rows = append(rows, []string{code, v.desc, v.detail, name, id})
			}
		}
		if e := f.sicRepo.BulkInsertAtomic(pre); e != nil {
			rt.Fatal(e)
		}
		body := reviewCSV(t, rows)
		r, e := s.MergeUpload(context.Background(), strings.NewReader(body))
		if e != nil || !r.MappingCommitted {
			rt.Fatalf("merge %+v %v", r, e)
		}
		created, changed, same := 0, 0, 0
		for code, v := range upload {
			old, ok := before[code]
			if !ok {
				created++
			} else if old == v {
				same++
			} else {
				changed++
			}
		}
		if r.CreatedRows != created || r.UpdatedRows != changed || r.UnchangedRows != same || r.CreatedRows+r.UpdatedRows+r.UnchangedRows != len(upload) {
			rt.Fatalf("count invariant %+v want %d/%d/%d", r, created, changed, same)
		}
		read := func() map[string]value {
			got := map[string]value{}
			all, e := f.sicRepo.GetAll()
			if e != nil {
				rt.Fatal(e)
			}
			for _, m := range all {
				cat := 0
				if m.CategoryID != nil {
					cat = *m.CategoryID
				}
				got[string(m.SICCode)] = value{m.Description, m.DescriptionDetail, cat}
			}
			return got
		}
		if got := read(); !reflect.DeepEqual(got, after) {
			rt.Fatalf("union/omission/value invariant got=%v want=%v", got, after)
		}
		r, e = s.MergeUpload(context.Background(), strings.NewReader(body))
		if e != nil || r.CreatedRows != 0 || r.UpdatedRows != 0 || r.UnchangedRows != len(upload) || !reflect.DeepEqual(read(), after) {
			rt.Fatalf("idempotency %+v %v", r, e)
		}
	})
}
