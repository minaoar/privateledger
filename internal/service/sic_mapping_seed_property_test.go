package service

import (
	"encoding/csv"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/oronno/privateledger/internal/model"
	"pgregory.net/rapid"
)

// genSeedCategoryNames produces distinct, resolvable category names. Names stay
// case-distinct so BR-CAT-04/05 ambiguity is not accidentally triggered.
func genSeedCategoryNames() *rapid.Generator[[]string] {
	return rapid.Custom(func(t *rapid.T) []string {
		n := rapid.IntRange(0, 5).Draw(t, "categoryCount")
		names := make([]string, 0, n)
		for i := 0; i < n; i++ {
			names = append(names, fmt.Sprintf("Category %02d", i))
		}
		return names
	})
}

type seedRow struct {
	Canonical    string
	RawCode      string
	Description  string
	DetailText   string
	CategoryName string
	CategoryID   string
}

// genValidSeedRows produces rows guaranteed valid against the approved rules:
// unique canonical codes, digits-only, and category references that either
// resolve by name or are intentionally empty.
func genValidSeedRows(categoryNames []string, categoryIDs map[string]int) *rapid.Generator[[]seedRow] {
	return rapid.Custom(func(t *rapid.T) []seedRow {
		count := rapid.IntRange(0, 12).Draw(t, "rowCount")
		used := map[string]bool{}
		rows := make([]seedRow, 0, count)
		for i := 0; i < count; i++ {
			var canonical string
			for attempt := 0; attempt < 40; attempt++ {
				n := rapid.OneOf(
					rapid.Int64Range(1, 9999),
					rapid.Int64Range(1, math.MaxInt64),
					rapid.Just(int64(1)),
					rapid.Just(int64(math.MaxInt64)),
				).Draw(t, fmt.Sprintf("code%d", i))
				candidate := strconv.FormatInt(n, 10)
				if !used[candidate] {
					canonical = candidate
					break
				}
			}
			if canonical == "" {
				continue
			}
			used[canonical] = true

			raw := strings.Repeat("0", rapid.IntRange(0, 4).Draw(t, fmt.Sprintf("zeros%d", i))) + canonical

			row := seedRow{
				Canonical:   canonical,
				RawCode:     raw,
				Description: rapid.SampledFrom([]string{"", "Eating Places", "Grocery Stores", "desc with, comma", "quote \" inside"}).Draw(t, fmt.Sprintf("desc%d", i)),
				DetailText:  rapid.SampledFrom([]string{"", "detail", "multi\nline detail"}).Draw(t, fmt.Sprintf("detail%d", i)),
			}
			if len(categoryNames) > 0 && rapid.Bool().Draw(t, fmt.Sprintf("hasCategory%d", i)) {
				name := rapid.SampledFrom(categoryNames).Draw(t, fmt.Sprintf("catName%d", i))
				row.CategoryName = name
				if rapid.Bool().Draw(t, fmt.Sprintf("withID%d", i)) {
					row.CategoryID = strconv.Itoa(categoryIDs[name])
				}
			}
			rows = append(rows, row)
		}
		return rows
	})
}

func renderSeedCSV(rows []seedRow) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"SIC_Code", "Description", "Description_Detail", "Category_Name", "Category_ID"})
	for _, r := range rows {
		_ = w.Write([]string{r.RawCode, r.Description, r.DetailText, r.CategoryName, r.CategoryID})
	}
	w.Flush()
	return b.String()
}

// PBT-02 round trip: a rendered valid CSV parses back to exactly the mappings
// it described, with canonical codes and preserved descriptions (FR4/BR-CSV-08).
func TestProperty_SeedCSVRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		f := newSeedFixture(t)
		names := genSeedCategoryNames().Draw(rt, "categoryNames")
		ids := make(map[string]int, len(names))
		for _, n := range names {
			ids[n] = f.addCategory(n)
		}

		rows := genValidSeedRows(names, ids).Draw(rt, "rows")
		mappings, report, err := f.service.ValidateCSV(strings.NewReader(renderSeedCSV(rows)))
		if err != nil {
			rt.Fatalf("ValidateCSV returned a fatal error for a valid file: %v", err)
		}
		if report.RejectedRows != 0 {
			rt.Fatalf("a generated valid file was rejected: %+v", report.Errors)
		}
		if len(mappings) != len(rows) {
			rt.Fatalf("got %d candidates for %d rows", len(mappings), len(rows))
		}
		for i, m := range mappings {
			if string(m.SICCode) != rows[i].Canonical {
				rt.Fatalf("row %d: code %q, want canonical %q (raw %q)", i, m.SICCode, rows[i].Canonical, rows[i].RawCode)
			}
			if m.Description != rows[i].Description || m.DescriptionDetail != rows[i].DetailText {
				rt.Fatalf("row %d: descriptions not preserved: %q/%q vs %q/%q",
					i, m.Description, m.DescriptionDetail, rows[i].Description, rows[i].DetailText)
			}
			if rows[i].CategoryName == "" {
				if m.CategoryID != nil {
					rt.Fatalf("row %d: expected a NULL category, got %d", i, *m.CategoryID)
				}
				continue
			}
			if m.CategoryID == nil || *m.CategoryID != ids[rows[i].CategoryName] {
				rt.Fatalf("row %d: category %v, want %d", i, m.CategoryID, ids[rows[i].CategoryName])
			}
		}
	})
}

// PBT-03 invariant: a fully valid seed imports every row exactly once, and
// repeating startup never duplicates or reverts it (FR4 / NFR4 / AC8).
func TestProperty_SeedImportIsIdempotent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		f := newSeedFixture(t)
		names := genSeedCategoryNames().Draw(rt, "categoryNames")
		ids := make(map[string]int, len(names))
		for _, n := range names {
			ids[n] = f.addCategory(n)
		}
		rows := genValidSeedRows(names, ids).Draw(rt, "rows")
		f.writeSeed(renderSeedCSV(rows))

		snapshot := func() string {
			all, err := f.sicRepo.GetAll()
			if err != nil {
				rt.Fatalf("GetAll(): %v", err)
			}
			parts := make([]string, 0, len(all))
			for _, m := range all {
				cat := "nil"
				if m.CategoryID != nil {
					cat = strconv.Itoa(*m.CategoryID)
				}
				parts = append(parts, fmt.Sprintf("%s|%s|%s|%s", m.SICCode, m.Description, m.DescriptionDetail, cat))
			}
			sort.Strings(parts)
			return strings.Join(parts, ";")
		}

		var first string
		for run := 1; run <= 3; run++ {
			if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
				rt.Fatalf("run %d: %v", run, err)
			}
			current := snapshot()
			if run == 1 {
				first = current
				if got := f.mappingCount(); got != len(rows) {
					rt.Fatalf("imported %d mappings for %d rows", got, len(rows))
				}
				continue
			}
			if current != first {
				rt.Fatalf("startup %d changed the mapping set:\n first: %s\n  this: %s", run, first, current)
			}
		}
	})
}

// PBT-03 invariant: injecting any single invalid row into an otherwise valid
// file makes the whole seed import zero rows (BR-CSV-05 / BR-CSV-06).
func TestProperty_AnyInvalidRowImportsNothing(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		f := newSeedFixture(t)
		names := genSeedCategoryNames().Draw(rt, "categoryNames")
		ids := make(map[string]int, len(names))
		for _, n := range names {
			ids[n] = f.addCategory(n)
		}
		rows := genValidSeedRows(names, ids).Draw(rt, "rows")

		bad := rapid.SampledFrom([]seedRow{
			{RawCode: "12A4"},
			{RawCode: ""},
			{RawCode: "0"},
			{RawCode: "9223372036854775808"},
			{RawCode: "-7"},
			{RawCode: " 12 34 "},
			{RawCode: "1", CategoryName: "", CategoryID: "1"},
			{RawCode: "2", CategoryName: "Definitely Not A Category"},
		}).Draw(rt, "invalidRow")

		// Guarantee the invalid row is genuinely additional, not a duplicate of a
		// generated row that would change the failure reason.
		all := append(append([]seedRow{}, rows...), bad)
		at := rapid.IntRange(0, len(all)-1).Draw(rt, "position")
		all[len(all)-1], all[at] = all[at], all[len(all)-1]

		f.writeSeed(renderSeedCSV(all))
		if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
			rt.Fatalf("an invalid seed must be non-fatal: %v", err)
		}
		if got := f.mappingCount(); got != 0 {
			rt.Fatalf("a seed containing an invalid row imported %d mappings", got)
		}
	})
}

// PBT-03 invariant: any duplicate canonical code inside one file rejects the
// whole file, regardless of the leading-zero spelling used (BR-SIC-05).
func TestProperty_DuplicateCanonicalCodeRejectsFile(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		f := newSeedFixture(t)
		canonical := strconv.FormatInt(rapid.Int64Range(1, 999999).Draw(rt, "code"), 10)
		zerosA := strings.Repeat("0", rapid.IntRange(0, 5).Draw(rt, "zerosA"))
		zerosB := strings.Repeat("0", rapid.IntRange(0, 5).Draw(rt, "zerosB"))

		rows := []seedRow{
			{RawCode: zerosA + canonical, Description: "first"},
			{RawCode: zerosB + canonical, Description: "second"},
		}
		f.writeSeed(renderSeedCSV(rows))
		if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
			rt.Fatalf("must be non-fatal: %v", err)
		}
		if got := f.mappingCount(); got != 0 {
			rt.Fatalf("duplicate canonical codes %q/%q imported %d mappings",
				rows[0].RawCode, rows[1].RawCode, got)
		}
	})
}

// PBT-03 invariant: whatever the file contains, a non-empty mapping table is
// never mutated by startup seeding (BR-CSV-03 / AC8).
func TestProperty_NonEmptyTableIsNeverMutated(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		f := newSeedFixture(t)
		names := genSeedCategoryNames().Draw(rt, "categoryNames")
		ids := make(map[string]int, len(names))
		for _, n := range names {
			ids[n] = f.addCategory(n)
		}

		existingCategory := (*int)(nil)
		if len(names) > 0 {
			id := ids[names[0]]
			existingCategory = &id
		}
		if err := f.sicRepo.Create(model.NewSICMapping("999999", "USER EDITED", "kept", existingCategory)); err != nil {
			rt.Fatalf("seed existing mapping: %v", err)
		}

		rows := genValidSeedRows(names, ids).Draw(rt, "rows")
		f.writeSeed(renderSeedCSV(rows))

		if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
			rt.Fatalf("skip path must be non-fatal: %v", err)
		}
		all, err := f.sicRepo.GetAll()
		if err != nil {
			rt.Fatalf("GetAll(): %v", err)
		}
		if len(all) != 1 {
			rt.Fatalf("the existing mapping set was mutated: %d rows", len(all))
		}
		if string(all[0].SICCode) != "999999" || all[0].Description != "USER EDITED" || all[0].DescriptionDetail != "kept" {
			rt.Fatalf("the existing mapping was changed: %+v", all[0])
		}
	})
}
