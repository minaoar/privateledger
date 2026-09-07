package main

import (
	"strings"
	"testing"
)

func TestReviewU5RuleChangePagesRenderAllThreeCounts(t *testing.T) {
	for _, path := range []string{"web/templates/categories.html", "web/templates/sic_mappings.html"} {
		page, err := embeddedFiles.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(page)
		for _, field := range []string{"moved_count", "uncategorized_count", "manual_protected_count"} {
			if !strings.Contains(source, field) {
				t.Errorf("%s cannot render %s", path, field)
			}
		}
	}
}

func TestReviewU5EveryRuleChangePageConsumesItsReportedCounts(t *testing.T) {
	read := func(path string) string {
		t.Helper()
		page, err := embeddedFiles.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(page)
	}

	categories := read("web/templates/categories.html")
	// Definition plus invocations for category-with-pattern creation, pattern
	// addition, category deletion, pattern deletion, and manual re-examination.
	if calls := strings.Count(categories, "describeReexamination("); calls < 6 {
		t.Errorf("categories page consumes rule-change results in %d places, want all five trigger paths plus the formatter definition", calls-1)
	}

	mappings := read("web/templates/sic_mappings.html")
	// Definition plus mapping save, mapping deletion, and upload invocations.
	if calls := strings.Count(mappings, "describeRuleChangeCounts("); calls < 4 {
		t.Errorf("SIC mapping page consumes count results in %d places, want create/update, delete, and upload", calls-1)
	}

	transactions := read("web/templates/transactions.html")
	for _, field := range []string{"moved_count", "uncategorized_count", "manual_protected_count", "post_commit_warnings"} {
		if !strings.Contains(transactions, field) {
			t.Errorf("transaction modal mapping result does not consume %s", field)
		}
	}
}
