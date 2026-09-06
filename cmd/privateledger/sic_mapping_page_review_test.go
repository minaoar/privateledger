package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/handler"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewU2PageScaleAndEscaping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	db, e := database.Open(database.Config{Path: filepath.Join(dir, "test.db")})
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	repo := repository.NewSICMappingRepository(db)
	s := service.NewSICMappingService(repo, repository.NewCategoryRepository(db))
	pre := make([]*model.SICMapping, 1000)
	for i := range pre {
		pre[i] = model.NewSICMapping(model.SICCode(fmt.Sprint(i+1)), fmt.Sprintf("Description %04d", i+1), "detail", nil)
	}
	pre[0].Description = `<script>alert("review")</script>`
	if e = repo.BulkInsertAtomic(pre); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO category(name,category_type) VALUES ('Current choice',2)`); e != nil {
		t.Fatal(e)
	}
	h := handler.NewPageHandler(embeddedFiles, nil, nil, nil, nil, nil, s, "review")
	r := gin.New()
	r.GET("/sic-mappings", h.SICMappings)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/sic-mappings", nil))
	html := w.Body.String()
	if w.Code != 200 || strings.Count(html, `data-testid="sic-mapping-row-`) != 1000 {
		t.Fatalf("render %d rows=%d", w.Code, strings.Count(html, `data-testid="sic-mapping-row-`))
	}
	if strings.Contains(html, pre[0].Description) || !strings.Contains(html, "&lt;script&gt;") {
		t.Fatal("unsafe/unrendered description")
	}
	last := -1
	for i := 2; i <= 1000; i++ {
		p := strings.Index(html, fmt.Sprintf("Description %04d", i))
		if p <= last {
			t.Fatalf("numeric order at %d", i)
		}
		last = p
	}
	for _, required := range []string{"Current choice", "No SIC category", "sic-mapping-form-submit-button", "sic-mapping-delete-confirm-button", "aria-labelledby=", "Import / Update Mappings"} {
		if !strings.Contains(html, required) {
			t.Errorf("missing %s", required)
		}
	}
	for _, required := range []string{
		"function setUnknownOutcome(action)",
		"The result of this change is unknown.",
		"Refresh the mapping list and check before trying again.",
		"sic-mapping-refresh-button",
		"partial.backup_path",
		"partial.backup_warning",
	} {
		if !strings.Contains(html, required) {
			t.Errorf("missing persistent result behavior %q", required)
		}
	}
	if strings.Contains(html, "setTimeout(") {
		t.Error("rendered page still contains a timed status reload")
	}
	if out := os.Getenv("SIC_REVIEW_RENDER_PATH"); out != "" {
		if e = os.WriteFile(out, []byte(html), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
