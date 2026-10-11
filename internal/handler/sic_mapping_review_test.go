package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

const handlerReviewHeader = "SIC_Code,Description,Description_Detail,Category_Name,Category_ID\n"

type handlerReviewFixture struct {
	db     *sql.DB
	repo   *repository.SICMappingRepository
	s      *service.SICMappingService
	router *gin.Engine
	dir    string
}

func newHandlerReview(t *testing.T) *handlerReviewFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	db, e := database.Open(database.Config{Path: filepath.Join(dir, "test.db")})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	repo := repository.NewSICMappingRepository(db)
	s := service.NewSICMappingManagementService(repo, repository.NewCategoryRepository(db), dir, 20*time.Millisecond, service.NewNoopSICRecategorizationCollaborator())
	h := NewSICMappingHandler(s)
	r := gin.New()
	r.GET("/api/sic-mappings", h.List)
	r.POST("/api/sic-mappings", h.Create)
	r.PUT("/api/sic-mappings/:id", h.Update)
	r.DELETE("/api/sic-mappings/:id", h.Delete)
	r.GET("/api/sic-mappings/download", h.Download)
	r.POST("/api/sic-mappings/upload", h.Upload)
	return &handlerReviewFixture{db, repo, s, r, dir}
}
func (f *handlerReviewFixture) request(method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}
func reviewMultipart(t *testing.T, parts map[string][]string) *http.Request {
	t.Helper()
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	for name, files := range parts {
		for _, content := range files {
			w, e := m.CreateFormFile(name, "../../untrusted.csv")
			if e != nil {
				t.Fatal(e)
			}
			if _, e = io.WriteString(w, content); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := m.Close(); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "/api/sic-mappings/upload", &b)
	r.Header.Set("Content-Type", m.FormDataContentType())
	return r
}
func TestReviewU2HandlerCRUD(t *testing.T) {
	f := newHandlerReview(t)
	cases := []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/sic-mappings", "{", 400},
		{"POST", "/api/sic-mappings", `{"sic_code":"bad"}`, 422},
		{"POST", "/api/sic-mappings", `{"sic_code":"2","category_id":99999}`, 422},
		{"POST", "/api/sic-mappings", `{"sic_code":"0002","description":" hi "}`, 201},
		{"POST", "/api/sic-mappings", `{"sic_code":"2"}`, 409},
		{"PUT", "/api/sic-mappings/nope", `{}`, 400},
		{"PUT", "/api/sic-mappings/99999", `{"sic_code":"3"}`, 404},
		{"DELETE", "/api/sic-mappings/99999", "", 404},
	}
	for _, tc := range cases {
		w := f.request(tc.method, tc.path, tc.body)
		if w.Code != tc.status {
			t.Errorf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	m, e := f.repo.GetByCode("2")
	if e != nil || m == nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ method, body string }{{"PUT", `{"sic_code":"3"}`}, {"DELETE", ""}} {
		w := f.request(tc.method, fmt.Sprintf("/api/sic-mappings/%d", m.SICMappingID), tc.body)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"mapping_committed":true`) {
			t.Fatalf("saved: %d %s", w.Code, w.Body.String())
		}
	}
}
func TestReviewU2HandlerUploadBoundaries(t *testing.T) {
	limit := int(model.MaxSICMappingFileSize)
	exact := handlerReviewHeader + "1," + strings.Repeat("x", limit-len(handlerReviewHeader)-6) + ",,,\n"
	if len(exact) != limit {
		t.Fatalf("fixture size %d", len(exact))
	}
	cases := []struct {
		name   string
		parts  map[string][]string
		length int64
		want   int
	}{
		{"valid", map[string][]string{"file": {handlerReviewHeader + "1,,,,\n"}}, -1, 200},
		{"exact", map[string][]string{"file": {exact}}, -1, 200},
		{"one_over", map[string][]string{"file": {exact + "\n"}}, 1, 413},
		{"body_over", map[string][]string{"file": {exact + strings.Repeat("x", (1<<20)+1)}}, -1, 413},
		{"missing", map[string][]string{}, -1, 400},
		{"wrong_name", map[string][]string{"other": {handlerReviewHeader}}, -1, 400},
		{"two_same_field", map[string][]string{"file": {handlerReviewHeader, handlerReviewHeader}}, -1, 400},
		{"extra_other_field", map[string][]string{"file": {handlerReviewHeader + "1,,,,\n"}, "other": {handlerReviewHeader}}, -1, 400},
		{"invalid", map[string][]string{"file": {handlerReviewHeader + "x,,,,\n"}}, -1, 422},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newHandlerReview(t)
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			r := reviewMultipart(t, tc.parts)
			r.ContentLength = tc.length
			w := httptest.NewRecorder()
			f.router.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Errorf("status=%d want=%d body=%.300s", w.Code, tc.want, w.Body.String())
			}
			entries, e := os.ReadDir(tmp)
			if e != nil || len(entries) != 0 {
				t.Errorf("multipart files leaked: %v %v", entries, e)
			}
			n, e := f.repo.Count()
			if e != nil {
				t.Fatal(e)
			}
			if tc.want != 200 && n != 0 {
				t.Errorf("rejected request persisted %d mappings", n)
			}
			backups, _ := filepath.Glob(filepath.Join(f.dir, "sic_mappings.backup-*"))
			if tc.want != 200 && len(backups) > 0 {
				t.Errorf("rejected request wrote backup")
			}
		})
	}
}
func TestReviewU2HandlerDownloadFailure(t *testing.T) {
	f := newHandlerReview(t)
	if _, e := f.db.Exec("DROP TABLE sic_mapping"); e != nil {
		t.Fatal(e)
	}
	w := f.request("GET", "/api/sic-mappings/download", "")
	if w.Code != 500 {
		t.Errorf("database failure returned successful empty CSV: status=%d body=%q", w.Code, w.Body.String())
	}
}
func TestReviewU2HandlerDownloadAndF15(t *testing.T) {
	f := newHandlerReview(t)
	w := f.request("GET", "/api/sic-mappings/download", "")
	if w.Code != 200 || w.Body.String() != handlerReviewHeader || !strings.Contains(w.Header().Get("Content-Disposition"), "sic_mappings.csv") {
		t.Fatal(w)
	}
	if _, e := f.db.Exec("DROP TABLE category"); e != nil {
		t.Fatal(e)
	}
	r := reviewMultipart(t, map[string][]string{"file": {handlerReviewHeader}})
	w = httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	var body struct{ Result model.SICMappingImportResult }
	if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
		t.Fatal(e)
	}
	if w.Code != 500 || body.Result.Outcome != model.SICMappingImportPersistenceFailed || body.Result.MappingCommitted {
		t.Fatalf("F15 %d %s", w.Code, w.Body.String())
	}
}
func TestReviewU2HandlerCancelledAdmission(t *testing.T) {
	f := newHandlerReview(t)
	r := httptest.NewRequest("POST", "/api/sic-mappings", strings.NewReader(`{"sic_code":"1"}`))
	r.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	if w.Code != 408 || !strings.Contains(w.Body.String(), "request_cancelled") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	n, _ := f.repo.Count()
	if n != 0 {
		t.Fatal("cancelled admission mutated")
	}
}

type reviewHandlerCollaborator struct {
	entered chan struct{}
	resume  chan struct{}
	fail    bool
}

func (c reviewHandlerCollaborator) ReloadMappings() error {
	if c.entered != nil {
		close(c.entered)
		<-c.resume
	}
	if c.fail {
		return fmt.Errorf("injected reload failure")
	}
	return nil
}
func (c reviewHandlerCollaborator) Reexamine() (service.RecategorizationCounts, error) {
	return service.RecategorizationCounts{}, nil
}
func TestReviewU2HandlerBusyAndCommittedWarning(t *testing.T) {
	f := newHandlerReview(t)
	entered, resume := make(chan struct{}), make(chan struct{})
	s := service.NewSICMappingManagementService(f.repo, repository.NewCategoryRepository(f.db), f.dir, 20*time.Millisecond, reviewHandlerCollaborator{entered: entered, resume: resume})
	done := make(chan error, 1)
	go func() { _, e := s.CreateMapping(context.Background(), model.SICMappingInput{SICCode: "1"}); done <- e }()
	<-entered
	h := NewSICMappingHandler(s)
	r := gin.New()
	r.POST("/api/sic-mappings", h.Create)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/sic-mappings", strings.NewReader(`{"sic_code":"2"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	close(resume)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if w.Code != 503 || w.Header().Get("Retry-After") != "1" || !strings.Contains(w.Body.String(), "mapping_busy") {
		t.Fatalf("busy: %d %s", w.Code, w.Body.String())
	}
	s = service.NewSICMappingManagementService(f.repo, repository.NewCategoryRepository(f.db), f.dir, time.Second, reviewHandlerCollaborator{fail: true})
	h = NewSICMappingHandler(s)
	r = gin.New()
	r.POST("/api/sic-mappings/upload", h.Upload)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reviewMultipart(t, map[string][]string{"file": {handlerReviewHeader + "3,,,,\n"}}))
	var result model.SICMappingImportResult
	if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	if w.Code != 200 || !result.MappingCommitted || len(result.PostCommitWarnings) != 1 {
		t.Fatalf("warning: %d %s", w.Code, w.Body.String())
	}
}
func TestReviewU2HandlerRollbackKeepsBackupResult(t *testing.T) {
	for _, backupOK := range []bool{true, false} {
		t.Run(fmt.Sprint(backupOK), func(t *testing.T) {
			f := newHandlerReview(t)
			if _, e := f.db.Exec(`CREATE TRIGGER fail_review BEFORE INSERT ON sic_mapping BEGIN SELECT RAISE(ABORT, 'forced'); END`); e != nil {
				t.Fatal(e)
			}
			dir := f.dir
			if !backupOK {
				dir = filepath.Join(dir, "missing")
			}
			s := service.NewSICMappingManagementService(f.repo, repository.NewCategoryRepository(f.db), dir, time.Second, service.NewNoopSICRecategorizationCollaborator())
			h := NewSICMappingHandler(s)
			r := gin.New()
			r.POST("/api/sic-mappings/upload", h.Upload)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, reviewMultipart(t, map[string][]string{"file": {handlerReviewHeader + "1,,,,\n"}}))
			var body struct{ Result model.SICMappingImportResult }
			if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
				t.Fatal(e)
			}
			if w.Code != 500 || body.Result.MappingCommitted || body.Result.CreatedRows != 0 || body.Result.Outcome != model.SICMappingImportPersistenceFailed {
				t.Fatalf("rollback %d %s", w.Code, w.Body.String())
			}
			if backupOK && body.Result.BackupPath == "" || !backupOK && body.Result.BackupWarning == "" {
				t.Fatalf("backup lost: %s", w.Body.String())
			}
		})
	}
}
