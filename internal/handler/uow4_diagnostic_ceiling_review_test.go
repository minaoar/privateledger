package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/oronno/privateledger/internal/model"
)

// NFR-U4-SEC-01/DP-U4-06: this end-to-end response-size assertion is the
// enforcement for the behavioral ceiling even if a future call site bypasses
// DiagValue and formats raw text into AddError.
func TestReviewU4UploadDiagnosticResponseHasFixedCeiling(t *testing.T) {
	f := newHandlerReview(t)
	const responseCeiling = 64 << 10
	category := strings.Repeat("A", 3<<20) + "PRIVATE_TAIL_MARKER"
	file := handlerReviewHeader + "5812,,," + category + ",\n"

	req := reviewMultipart(t, map[string][]string{"file": {file}})
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d, want 422; body prefix=%.300s", w.Code, w.Body.String())
	}
	if w.Body.Len() >= responseCeiling {
		t.Fatalf("diagnostic response is %d bytes, fixed ceiling is %d", w.Body.Len(), responseCeiling)
	}
	t.Logf("uploaded_csv_bytes=%d response_bytes=%d ceiling=%d", len(file), w.Body.Len(), responseCeiling)

	var result model.SICMappingImportResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(result.Errors) != 1 || !utf8.ValidString(result.Errors[0].Message) {
		t.Fatalf("unexpected bounded diagnostic: %+v", result.Errors)
	}
	if strings.Contains(w.Body.String(), "PRIVATE_TAIL_MARKER") {
		t.Fatalf("response retained text beyond the category-name bound")
	}
}
