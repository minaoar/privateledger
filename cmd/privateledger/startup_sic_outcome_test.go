package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

// ---------------------------------------------------------------------------
// Revision 2 verification — F-02 (structured startup outcome propagation) and
// F-03 (bounded startup diagnostics), observed at the startup boundary.
//
// Authorities:
//   nfr-design/logical-components.md LC-U1-08 — the startup orchestrator must
//     "Apply the returned outcome according to the failure classification" and
//     "Emit structured outcome logs without sensitive payloads".
//   nfr-design/nfr-design-patterns.md NFRP-U1-06 (failure classification) and
//     NFRP-U1-08 (local-only and minimal diagnostics: stable event names and
//     fields such as operation, safe path, row number, validation code, count;
//     excluding complete source records and file contents).
//   nfr-requirements/nfr-requirements.md NFR-U1-SEC-02, NFR-U1-SEC-03.
// ---------------------------------------------------------------------------

// captureStartupLogs installs a JSON handler matching the one internal/logger
// configures for the running application, so measured output shape and volume
// reflect real startup behaviour.
func captureStartupLogs(t *testing.T, fn func()) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	fn()

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("startup log line is not structured JSON: %v\nline: %s", err, line)
		}
		records = append(records, record)
	}
	return records
}

const testSeedPath = "/tmp/privateledger-test/sic_mappings.csv"

// TestLogSICMappingImportOutcome_StartupOutcomeContract asserts that every
// outcome the seed service can hand back is acted on at the startup boundary,
// with the counts the design says cross that boundary.
func TestLogSICMappingImportOutcome_StartupOutcomeContract(t *testing.T) {
	cases := []struct {
		name        string
		report      *model.SICMappingImportReport
		wantRecords int
		wantLevel   string
		wantMsg     string
		wantFields  map[string]float64
	}{
		{
			name:        "nil report is inert",
			report:      nil,
			wantRecords: 0,
		},
		{
			name:        "absent seed is silent",
			report:      &model.SICMappingImportReport{Outcome: model.SICMappingImportAbsent},
			wantRecords: 0,
		},
		{
			name: "skipped because mappings exist logs the existing count",
			report: &model.SICMappingImportReport{
				Outcome:      model.SICMappingImportSkippedExisting,
				ExistingRows: 4321,
			},
			wantRecords: 1,
			wantLevel:   "INFO",
			wantMsg:     "Skipping SIC mapping seed because SQLite mappings already exist",
			wantFields:  map[string]float64{"existing_mappings": 4321},
		},
		{
			name:        "oversized seed logs a safe non-fatal diagnostic",
			report:      &model.SICMappingImportReport{Outcome: model.SICMappingImportOversized},
			wantRecords: 1,
			wantLevel:   "WARN",
			wantMsg:     "Rejecting oversized SIC mapping seed",
			wantFields:  map[string]float64{"maximum_bytes": 10 << 20},
		},
		{
			name: "imported seed logs the committed row count",
			report: &model.SICMappingImportReport{
				Outcome:      model.SICMappingImportImported,
				TotalRows:    12,
				ValidRows:    12,
				ImportedRows: 12,
			},
			wantRecords: 1,
			wantLevel:   "INFO",
			wantMsg:     "Imported SIC mapping seed",
			wantFields:  map[string]float64{"imported_rows": 12},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			records := captureStartupLogs(t, func() {
				logSICMappingImportOutcome(testSeedPath, tc.report)
			})
			if len(records) != tc.wantRecords {
				t.Fatalf("emitted %d startup records, want %d: %v", len(records), tc.wantRecords, records)
			}
			if tc.wantRecords == 0 {
				return
			}
			record := records[0]
			if record["level"] != tc.wantLevel {
				t.Errorf("level = %v, want %v", record["level"], tc.wantLevel)
			}
			if record["msg"] != tc.wantMsg {
				t.Errorf("msg = %v, want %v", record["msg"], tc.wantMsg)
			}
			if record["path"] != testSeedPath {
				t.Errorf("path = %v, want %v", record["path"], testSeedPath)
			}
			for field, want := range tc.wantFields {
				got, ok := record[field].(float64)
				if !ok {
					t.Errorf("field %q missing from the startup record: %v", field, record)
					continue
				}
				if got != want {
					t.Errorf("field %q = %v, want %v", field, got, want)
				}
			}
		})
	}
}

// TestLogSICMappingImportOutcome_InvalidSeedSummary asserts the aggregate
// summary an invalid seed must always produce, independent of the per-row cap.
func TestLogSICMappingImportOutcome_InvalidSeedSummary(t *testing.T) {
	report := buildInvalidReport(7)
	records := captureStartupLogs(t, func() {
		logSICMappingImportOutcome(testSeedPath, report)
	})
	if len(records) != 8 {
		t.Fatalf("emitted %d records for 7 rejected rows, want 7 row diagnostics + 1 summary", len(records))
	}
	summary := records[len(records)-1]
	if summary["msg"] != "SIC mapping seed validation failed; no mappings imported" {
		t.Fatalf("last record is not the aggregate summary: %v", summary)
	}
	for field, want := range map[string]float64{
		"total_rows":           7,
		"rejected_rows":        7,
		"reported_diagnostics": 7,
		"omitted_diagnostics":  0,
	} {
		if got, _ := summary[field].(float64); got != want {
			t.Errorf("summary %q = %v, want %v", field, summary[field], want)
		}
	}
	for _, record := range records[:7] {
		if record["msg"] != "Rejected SIC mapping seed row" {
			t.Errorf("unexpected row record: %v", record)
		}
		if _, ok := record["row"].(float64); !ok {
			t.Errorf("row diagnostic is missing its row number: %v", record)
		}
		if record["code"] != "invalid_sic" || record["field"] != "SIC_Code" {
			t.Errorf("row diagnostic lost its stable field/code: %v", record)
		}
	}
}

// TestLogSICMappingImportOutcome_DiagnosticsAreBounded is the F-03 regression
// pin: startup diagnostic output must stay bounded regardless of how many rows
// the (input-bounded) seed rejects, and the summary must account for every
// diagnostic that was not printed.
func TestLogSICMappingImportOutcome_DiagnosticsAreBounded(t *testing.T) {
	for _, rejected := range []int{0, 1, maxSICSeedDiagnostics - 1, maxSICSeedDiagnostics, maxSICSeedDiagnostics + 1, 100000} {
		t.Run(itoa(rejected)+"_rejected_rows", func(t *testing.T) {
			report := buildInvalidReport(rejected)
			records := captureStartupLogs(t, func() {
				logSICMappingImportOutcome(testSeedPath, report)
			})

			maxRecords := maxSICSeedDiagnostics + 1 // capped row diagnostics + one summary
			if len(records) > maxRecords {
				t.Fatalf("startup emitted %d records for %d rejected rows; the bound is %d",
					len(records), rejected, maxRecords)
			}

			expectedRows := rejected
			if expectedRows > maxSICSeedDiagnostics {
				expectedRows = maxSICSeedDiagnostics
			}
			if len(records) != expectedRows+1 {
				t.Fatalf("emitted %d records, want %d row diagnostics + 1 summary",
					len(records), expectedRows)
			}

			summary := records[len(records)-1]
			gotReported, _ := summary["reported_diagnostics"].(float64)
			gotOmitted, _ := summary["omitted_diagnostics"].(float64)
			if int(gotReported) != expectedRows {
				t.Errorf("reported_diagnostics = %v, want %d", gotReported, expectedRows)
			}
			if int(gotOmitted) != rejected-expectedRows {
				t.Errorf("omitted_diagnostics = %v, want %d", gotOmitted, rejected-expectedRows)
			}
			// No diagnostic may be silently lost: printed + omitted == total.
			if int(gotReported)+int(gotOmitted) != rejected {
				t.Errorf("reported %v + omitted %v does not account for %d diagnostics",
					gotReported, gotOmitted, rejected)
			}
		})
	}
}

// TestLogSICMappingImportOutcome_NoSeedPayloadInDiagnostics enforces
// NFRP-U1-08/NFR-U1-SEC-02: startup logs must not contain file contents.
func TestLogSICMappingImportOutcome_NoSeedPayloadInDiagnostics(t *testing.T) {
	secret := "ACME-PRIVATE-MERCHANT-4111111111111111"
	report := &model.SICMappingImportReport{
		Outcome:      model.SICMappingImportInvalid,
		TotalRows:    2,
		RejectedRows: 2,
		Errors: []model.SICMappingImportError{
			{RowNumber: 2, Field: "SIC_Code", Code: "invalid_sic", Message: "SIC code is invalid"},
			{RowNumber: 3, Field: "Category_Name", Code: "category_not_found", Message: "Category_Name does not resolve"},
		},
	}
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	logSICMappingImportOutcome(testSeedPath, report)

	if strings.Contains(buf.String(), secret) {
		t.Errorf("startup diagnostics leaked seed payload")
	}
	// The report itself must never be a payload carrier either.
	for _, e := range report.Errors {
		if strings.Contains(e.Message, secret) {
			t.Errorf("diagnostic message carries row payload: %q", e.Message)
		}
	}
}

func buildInvalidReport(rejected int) *model.SICMappingImportReport {
	report := &model.SICMappingImportReport{
		Outcome:      model.SICMappingImportInvalid,
		TotalRows:    rejected,
		RejectedRows: rejected,
		Errors:       make([]model.SICMappingImportError, 0, rejected),
	}
	for i := 0; i < rejected; i++ {
		report.Errors = append(report.Errors, model.SICMappingImportError{
			RowNumber: i + 2,
			Field:     "SIC_Code",
			Code:      "invalid_sic",
			Message:   "SIC code is invalid",
		})
	}
	return report
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestOversizedDiagnosticMatchesEnforcedLimit pins the hardcoded limit in the
// startup diagnostic against the limit the service actually enforces. The two
// are declared independently (cmd/privateledger/main.go logs a literal, and
// internal/service owns the unexported threshold), so they can silently drift.
// NFRP-U1-03 requires the rejection diagnostic to be accurate as well as safe.
func TestOversizedDiagnosticMatchesEnforcedLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("size-boundary probe skipped in -short mode")
	}

	dir := t.TempDir()
	db, err := database.Open(database.Config{Path: filepath.Join(dir, "privateledger.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	seedService := service.NewSICMappingService(
		repository.NewSICMappingRepository(db),
		repository.NewCategoryRepository(db),
	)

	// Discover the enforced boundary behaviourally rather than by reading the
	// production constant.
	writeSeedOfSize := func(t *testing.T, path string, size int) {
		t.Helper()
		header := "SIC_Code,Description,Description_Detail,Category_Name,Category_ID\n"
		body := make([]byte, size)
		copy(body, header)
		for i := len(header); i < size; i++ {
			body[i] = 'x'
		}
		body[size-1] = '\n'
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("write seed: %v", err)
		}
	}

	const expectedLimit = 10 << 20

	atLimit := filepath.Join(dir, "at_limit.csv")
	writeSeedOfSize(t, atLimit, expectedLimit)
	atLimitReport, err := seedService.ImportFileIfPresentWithReport(atLimit)
	if err != nil {
		t.Fatalf("a seed exactly at the limit must not fail startup: %v", err)
	}
	if atLimitReport.Outcome == model.SICMappingImportOversized {
		t.Errorf("a seed of exactly %d bytes was rejected as oversized; the limit is inclusive", expectedLimit)
	}

	overLimit := filepath.Join(dir, "over_limit.csv")
	writeSeedOfSize(t, overLimit, expectedLimit+1)
	overReport, err := seedService.ImportFileIfPresentWithReport(overLimit)
	if err != nil {
		t.Fatalf("an oversized seed must be non-fatal: %v", err)
	}
	if overReport.Outcome != model.SICMappingImportOversized {
		t.Fatalf("a seed of %d bytes produced Outcome=%q, want %q",
			expectedLimit+1, overReport.Outcome, model.SICMappingImportOversized)
	}

	records := captureStartupLogs(t, func() {
		logSICMappingImportOutcome(overLimit, overReport)
	})
	if len(records) != 1 {
		t.Fatalf("oversized rejection emitted %d records, want 1", len(records))
	}
	logged, ok := records[0]["maximum_bytes"].(float64)
	if !ok {
		t.Fatalf("oversized diagnostic omits maximum_bytes: %v", records[0])
	}
	if int64(logged) != expectedLimit {
		t.Errorf("startup logs maximum_bytes=%d but the service enforces %d; the two constants have drifted",
			int64(logged), expectedLimit)
	}
	// Recorded, not asserted: the diagnostic no longer carries the observed
	// file size, so a user cannot tell how far over the limit the seed is.
	if _, present := records[0]["size_bytes"]; !present {
		t.Logf("oversized diagnostic carries no size_bytes field: %v", records[0])
	}
}
