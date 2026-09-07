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
