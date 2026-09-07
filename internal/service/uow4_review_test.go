package service

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/oronno/privateledger/internal/model"
	"pgregory.net/rapid"
)

func reviewUOW4Whitespace(t *rapid.T, label string) string {
	runes := rapid.SliceOfN(
		rapid.SampledFrom([]rune{' ', '\t', '\n', '\r', '\v', '\f', '\u00a0', '\u2003'}),
		0,
		8,
	).Draw(t, label)
	return string(runes)
}

func reviewUOW4CaseVariant(t *rapid.T, value, label string) string {
	var b strings.Builder
	for i, r := range value {
		if !unicode.IsLetter(r) {
			b.WriteRune(r)
			continue
		}
		if rapid.Bool().Draw(t, fmt.Sprintf("%s_case_%d", label, i)) {
			b.WriteRune(unicode.ToUpper(r))
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// NFR-U4-TEST-01, accepting direction: every combination of per-column case,
// surrounding Unicode whitespace, and optional leading BOM must match.
func TestReviewU4HeaderNormalizationProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		header := make([]string, len(sicMappingCSVHeader))
		for i, canonical := range sicMappingCSVHeader {
			left := reviewUOW4Whitespace(t, fmt.Sprintf("left_%d", i))
			right := reviewUOW4Whitespace(t, fmt.Sprintf("right_%d", i))
			header[i] = left + reviewUOW4CaseVariant(t, canonical, fmt.Sprintf("column_%d", i)) + right
		}
		if rapid.Bool().Draw(t, "bom") {
			header[0] = utf8BOM + header[0]
		}
		if mismatch := matchSICMappingHeader(header); mismatch != nil {
			t.Fatalf("decorated canonical header rejected: %#v (%s)", header, mismatch.message())
		}
	})
}

// NFR-U4-TEST-01, rejecting direction: tolerance must never accept a changed
// contract shape or name.
func TestReviewU4HeaderStrictnessProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		header := append([]string(nil), sicMappingCSVHeader...)
		mutation := rapid.SampledFrom([]string{"reorder", "omit", "extra", "alter"}).Draw(t, "mutation")
		switch mutation {
		case "reorder":
			first := rapid.IntRange(0, len(header)-2).Draw(t, "first")
			second := rapid.IntRange(first+1, len(header)-1).Draw(t, "second")
			header[first], header[second] = header[second], header[first]
		case "omit":
			at := rapid.IntRange(0, len(header)-1).Draw(t, "omit_at")
			header = append(header[:at], header[at+1:]...)
		case "extra":
			header = append(header, "Unexpected_Column")
		case "alter":
			at := rapid.IntRange(0, len(header)-1).Draw(t, "alter_at")
			header[at] += "_Changed"
		}
		if mismatch := matchSICMappingHeader(header); mismatch == nil {
			t.Fatalf("%s header mutation unexpectedly matched: %#v", mutation, header)
		}
	})
}

func TestReviewU4ObservedHeaderVariantsImport(t *testing.T) {
	cases := map[string]string{
		"lowercase":         "sic_code,description,description_detail,category_name,category_id\n",
		"mixed case":        "SiC_cOdE,dEsCrIpTiOn,DeScRiPtIoN_dEtAiL,cAtEgOrY_nAmE,CaTeGoRy_Id\n",
		"space after comma": "SIC_Code, Description, Description_Detail, Category_Name, Category_ID\n",
		"utf8 BOM":          utf8BOM + seedHeader,
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			f := newSeedFixture(t)
			f.writeSeed(header + "5812,Restaurant,,,\n")
			report, err := f.service.ImportFileIfPresentWithReport(f.seedPath)
			if err != nil {
				t.Fatalf("ImportFileIfPresentWithReport: %v", err)
			}
			if report.Outcome != model.SICMappingImportImported || report.ImportedRows != 1 || f.mappingCount() != 1 {
				t.Fatalf("variant did not import: report=%+v count=%d", report, f.mappingCount())
			}
		})
	}
}

func TestReviewU4RowCSVParsingCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
	}{
		{name: "ragged row with too few fields", row: "5812,Restaurant\n"},
		{name: "ragged row with too many fields", row: "5812,Restaurant,,,,extra\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSeedFixture(t)
			mappings, report, err := f.service.ValidateCSV(strings.NewReader(seedHeader + tc.row))
			if err != nil {
				t.Fatalf("ValidateCSV returned fatal error: %v", err)
			}
			if len(mappings) != 0 || report.RejectedRows != 1 || len(report.Errors) != 1 || report.Errors[0].Code != "malformed_csv" {
				t.Fatalf("ragged row behavior changed: mappings=%d report=%+v", len(mappings), report)
			}
		})
	}

	t.Run("quoted comma quote and embedded newline remain data", func(t *testing.T) {
		f := newSeedFixture(t)
		input := seedHeader + "5812,\"Dining, cafe\",\"Line one\nLine \"\"two\"\"\",,\n"
		mappings, report, err := f.service.ValidateCSV(strings.NewReader(input))
		if err != nil {
			t.Fatalf("ValidateCSV returned fatal error: %v", err)
		}
		if report.RejectedRows != 0 || len(mappings) != 1 {
			t.Fatalf("valid quoted row was rejected: mappings=%d report=%+v", len(mappings), report)
		}
		if mappings[0].Description != "Dining, cafe" || mappings[0].DescriptionDetail != "Line one\nLine \"two\"" {
			t.Fatalf("quoted fields changed: description=%q detail=%q", mappings[0].Description, mappings[0].DescriptionDetail)
		}
	})
}

func TestReviewU4DiagnosticMessages(t *testing.T) {
	t.Run("renamed category names stale and current values", func(t *testing.T) {
		f := newSeedFixture(t)
		foodID := f.addCategory("Food")
		_, report, err := f.service.ValidateCSV(strings.NewReader(
			seedHeader + fmt.Sprintf("5812,,,Groceries,%d\n", foodID)))
		if err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf("Category_Name \"Groceries\" does not exist; Category_ID %d is currently \"Food\"", foodID)
		if len(report.Errors) != 1 || report.Errors[0].Code != "category_not_found" || report.Errors[0].Message != want {
			t.Fatalf("renamed-category diagnostic = %+v, want %q", report.Errors, want)
		}
	})

	t.Run("unknown and malformed IDs keep the short form", func(t *testing.T) {
		for _, id := range []string{"", "999999", "abc", "0", "-1"} {
			f := newSeedFixture(t)
			_, report, err := f.service.ValidateCSV(strings.NewReader(seedHeader + "5812,,,Groceries," + id + "\n"))
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Errors) != 1 || report.Errors[0].Message != "Category_Name \"Groceries\" does not exist" {
				t.Fatalf("Category_ID %q produced %+v", id, report.Errors)
			}
		}
	})

	t.Run("invalid ID validation retains its own code", func(t *testing.T) {
		f := newSeedFixture(t)
		f.addCategory("Food")
		_, report, err := f.service.ValidateCSV(strings.NewReader(seedHeader + "5812,,,Food,abc\n"))
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Errors) != 1 || report.Errors[0].Code != "invalid_category_id" || report.Errors[0].Message != "Category_ID must be a positive integer" {
			t.Fatalf("invalid ID diagnostic = %+v", report.Errors)
		}
	})

	t.Run("ambiguity lists three deterministic names and remainder", func(t *testing.T) {
		f := newSeedFixture(t)
		for _, name := range []string{"food", "FOOD", "fOoD", "Food"} {
			f.addCategory(name)
		}
		_, report, err := f.service.ValidateCSV(strings.NewReader(seedHeader + "5812,,,FoOd,\n"))
		if err != nil {
			t.Fatal(err)
		}
		want := "Category_Name \"FoOd\" matches 4 categories: \"FOOD\", \"Food\", \"fOoD\" and 1 more"
		if len(report.Errors) != 1 || report.Errors[0].Code != "category_ambiguous" || report.Errors[0].Message != want {
			t.Fatalf("ambiguity diagnostic = %+v, want %q", report.Errors, want)
		}
	})

	t.Run("category text cannot become a format string", func(t *testing.T) {
		f := newSeedFixture(t)
		for _, name := range []string{"FOOD%s", "food%s"} {
			f.addCategory(name)
		}
		_, report, err := f.service.ValidateCSV(strings.NewReader(seedHeader + "5812,,,FoOd%s,\n"))
		if err != nil {
			t.Fatal(err)
		}
		want := "Category_Name \"FoOd%s\" matches 2 categories: \"FOOD%s\", \"food%s\""
		if len(report.Errors) != 1 || report.Errors[0].Message != want || strings.Contains(report.Errors[0].Message, "%!") {
			t.Fatalf("format-like category text changed formatting: %+v", report.Errors)
		}
	})

	t.Run("header messages contain only fixed text and counts", func(t *testing.T) {
		f := newSeedFixture(t)
		const received = "PRIVATE_ACCOUNT_HEADER"
		_, report, err := f.service.ValidateCSV(strings.NewReader(
			"SIC_Code,Description,Description_Detail," + received + ",Category_ID\n"))
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Errors) != 1 || report.Errors[0].Message != "CSV header column 4 must be Category_Name" || strings.Contains(report.Errors[0].Message, received) {
			t.Fatalf("wrong-column diagnostic = %+v", report.Errors)
		}

		_, report, err = f.service.ValidateCSV(strings.NewReader("SIC_Code,Description,Description_Detail,Category_Name\n"))
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Errors) != 1 || report.Errors[0].Message != "CSV header must have 5 columns; this file has 4" {
			t.Fatalf("wrong-count diagnostic = %+v", report.Errors)
		}
	})
}

func TestReviewU4ExportRemainsCanonicalAndByteIdentical(t *testing.T) {
	f := newSeedFixture(t)
	diningID := f.addCategory("Dining")
	mapping := model.NewSICMapping("5812", "Dining, cafes", "Line 1\nLine 2", &diningID)
	if err := f.sicRepo.Create(mapping); err != nil {
		t.Fatal(err)
	}

	var got bytes.Buffer
	if err := f.service.ExportCSV(&got); err != nil {
		t.Fatal(err)
	}
	want := seedHeader + fmt.Sprintf("5812,\"Dining, cafes\",\"Line 1\nLine 2\",Dining,%d\n", diningID)
	if got.String() != want {
		t.Fatalf("export bytes changed:\n got %q\nwant %q", got.String(), want)
	}
}
