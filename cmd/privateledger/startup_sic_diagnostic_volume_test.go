package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
	"github.com/oronno/privateledger/internal/service"
)

// ---------------------------------------------------------------------------
// Revision 2 verification — F-03 diagnostic-volume re-measurement.
//
// Revision 1 recorded, on the same reference machine and with the same fixture
// size, 100,001 log lines / 23,889,163 bytes of startup diagnostics for a
// 1,400,066-byte fully invalid seed (17x input-to-output amplification, emitted
// on every startup because a rejected seed is never consumed).
//
// This test regenerates a fixture of exactly that size, drives the real startup
// path, and measures the bounded output. It also reproduces the Revision 1
// emission shape in-harness against the identical handler and path so the
// before/after byte comparison is apples to apples.
// ---------------------------------------------------------------------------

const (
	invalidSeedRows          = 100000
	revision1DiagnosticLines = 100001
	revision1DiagnosticBytes = 23889163
	revision1SeedBytes       = 1400066
)

// writeFullyInvalidSeed writes invalidSeedRows rows of exactly 14 bytes each,
// every one rejected with a single invalid_sic diagnostic. Header (66 bytes)
// plus 100,000 x 14 bytes reproduces the 1,400,066-byte Revision 1 fixture.
func writeFullyInvalidSeed(t *testing.T, path string) int64 {
	t.Helper()
	var buf bytes.Buffer
	buf.Grow(revision1SeedBytes)
	buf.WriteString("SIC_Code,Description,Description_Detail,Category_Name,Category_ID\n")
	for i := 1; i <= invalidSeedRows; i++ {
		fmt.Fprintf(&buf, "-%08d,,,,\n", i)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write invalid seed: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat invalid seed: %v", err)
	}
	return info.Size()
}

type volumeWriter struct {
	bytes int
	lines int
}

func (w *volumeWriter) Write(p []byte) (int, error) {
	w.bytes += len(p)
	w.lines += bytes.Count(p, []byte("\n"))
	return len(p), nil
}

// TestStartupDiagnosticVolumeForLargeInvalidSeed re-measures F-03 end to end.
func TestStartupDiagnosticVolumeForLargeInvalidSeed(t *testing.T) {
	if testing.Short() {
		t.Skip("volume measurement skipped in -short mode")
	}

	dir := t.TempDir()
	seedPath := filepath.Join(dir, "sic_mappings.csv")
	seedBytes := writeFullyInvalidSeed(t, seedPath)
	if seedBytes != revision1SeedBytes {
		t.Fatalf("fixture is %d bytes; the Revision 1 comparison fixture was %d bytes",
			seedBytes, revision1SeedBytes)
	}
	if seedBytes > 10<<20 {
		t.Fatalf("fixture must stay inside the accepted 10 MiB seed limit")
	}

	db, err := database.Open(database.Config{Path: filepath.Join(dir, "privateledger.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	sicRepo := repository.NewSICMappingRepository(db)
	seedService := service.NewSICMappingService(sicRepo, repository.NewCategoryRepository(db))

	start := time.Now()
	report, err := seedService.ImportFileIfPresentWithReport(seedPath)
	validationElapsed := time.Since(start)
	if err != nil {
		t.Fatalf("a fully invalid seed must remain non-fatal: %v", err)
	}
	if report.Outcome != model.SICMappingImportInvalid {
		t.Fatalf("Outcome = %q, want %q", report.Outcome, model.SICMappingImportInvalid)
	}
	if report.RejectedRows != invalidSeedRows {
		t.Fatalf("RejectedRows = %d, want %d", report.RejectedRows, invalidSeedRows)
	}
	if report.ImportedRows != 0 {
		t.Fatalf("ImportedRows = %d, want 0", report.ImportedRows)
	}
	if count, cErr := sicRepo.Count(); cErr != nil || count != 0 {
		t.Fatalf("mapping count = %d (err %v), want 0", count, cErr)
	}

	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	// Revision 2 (current production) output.
	current := &volumeWriter{}
	slog.SetDefault(slog.New(slog.NewJSONHandler(current, &slog.HandlerOptions{Level: slog.LevelInfo})))
	emitStart := time.Now()
	logSICMappingImportOutcome(seedPath, report)
	emitElapsed := time.Since(emitStart)

	// Revision 1 emission shape, reproduced in-harness with the identical
	// handler and identical path so the byte comparison is apples to apples.
	baseline := &volumeWriter{}
	baselineLogger := slog.New(slog.NewJSONHandler(baseline, &slog.HandlerOptions{Level: slog.LevelInfo}))
	for _, validationErr := range report.Errors {
		baselineLogger.Warn("Rejected SIC mapping seed row",
			slog.String("path", seedPath),
			slog.Int("row", validationErr.RowNumber),
			slog.String("field", validationErr.Field),
			slog.String("code", validationErr.Code))
	}
	baselineLogger.Warn("SIC mapping seed validation failed; no mappings imported",
		slog.String("path", seedPath),
		slog.Int("total_rows", report.TotalRows),
		slog.Int("rejected_rows", report.RejectedRows))

	slog.SetDefault(previous)

	maxLines := maxSICSeedDiagnostics + 1
	if current.lines > maxLines {
		t.Errorf("startup emitted %d diagnostic lines for a %d-row invalid seed; the bound is %d",
			current.lines, invalidSeedRows, maxLines)
	}
	if current.bytes >= baseline.bytes {
		t.Errorf("bounded output (%d bytes) did not reduce the unbounded shape (%d bytes)",
			current.bytes, baseline.bytes)
	}
	// The core F-03 concern was input-to-output amplification: a seed bounded
	// at 10 MiB produced many times its own size in diagnostics on every
	// startup. The bounded output must not amplify the input at all.
	//
	// An absolute byte threshold is deliberately NOT asserted here: the JSON
	// records embed the seed path, so byte totals scale with the length of the
	// temporary directory and are not reproducible across machines. The
	// reproducible contracts are the line bound and non-amplification.
	if int64(current.bytes) >= seedBytes {
		t.Errorf("bounded output %d bytes still amplifies the %d-byte seed", current.bytes, seedBytes)
	}
	// Every retained diagnostic must still be accounted for in the summary.
	if len(report.Errors) != invalidSeedRows {
		t.Errorf("report retained %d diagnostics for %d rejected rows", len(report.Errors), invalidSeedRows)
	}

	t.Logf("F-03 re-measurement (fixture %d bytes, %d rows, all rejected)", seedBytes, invalidSeedRows)
	t.Logf("  validation elapsed:            %v", validationElapsed)
	t.Logf("  diagnostic emission elapsed:   %v", emitElapsed)
	t.Logf("  Revision 1 recorded:           %d lines / %d bytes", revision1DiagnosticLines, revision1DiagnosticBytes)
	t.Logf("  Revision 1 shape, this run:    %d lines / %d bytes", baseline.lines, baseline.bytes)
	t.Logf("  Revision 2 measured:           %d lines / %d bytes", current.lines, current.bytes)
	t.Logf("  line reduction:                %.1fx", float64(baseline.lines)/float64(current.lines))
	t.Logf("  byte reduction:                %.1fx", float64(baseline.bytes)/float64(current.bytes))
	t.Logf("  Revision 2 output / seed size: %.4f%%", 100*float64(current.bytes)/float64(seedBytes))
	t.Logf("  diagnostics retained in memory: %d (%d reported, %d omitted)",
		len(report.Errors), maxSICSeedDiagnostics, len(report.Errors)-maxSICSeedDiagnostics)

	// Guard the privacy boundary at volume: no seed payload in the output.
	var sample bytes.Buffer
	sampleLogger := slog.New(slog.NewJSONHandler(&sample, &slog.HandlerOptions{Level: slog.LevelInfo}))
	previousDefault := slog.Default()
	slog.SetDefault(sampleLogger)
	logSICMappingImportOutcome(seedPath, report)
	slog.SetDefault(previousDefault)
	if strings.Contains(sample.String(), "-00000001") {
		t.Errorf("startup diagnostics echoed rejected row content")
	}
}

// TestStartupDiagnosticRetentionForLargeInvalidSeed measures the residual side
// of F-03. Revision 2 bounds diagnostic *output* at the startup boundary, but
// SICMappingImportReport.Errors still accumulates one entry per rejected row
// during validation, so the retained diagnostic set remains proportional to the
// rejected-row count rather than to the reporting bound.
//
// This is a measurement, not a pass/fail contract: no approved artifact places
// a numeric ceiling on retained diagnostics, and NFRP-U1-04 bounds the input at
// 10 MiB. The numbers are recorded so the production role can judge whether the
// retention deserves a cap of its own.
func TestStartupDiagnosticRetentionForLargeInvalidSeed(t *testing.T) {
	if testing.Short() {
		t.Skip("retention measurement skipped in -short mode")
	}

	dir := t.TempDir()
	seedPath := filepath.Join(dir, "sic_mappings.csv")
	seedBytes := writeFullyInvalidSeed(t, seedPath)

	db, err := database.Open(database.Config{Path: filepath.Join(dir, "privateledger.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	seedService := service.NewSICMappingService(
		repository.NewSICMappingRepository(db),
		repository.NewCategoryRepository(db),
	)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	report, err := seedService.ImportFileIfPresentWithReport(seedPath)
	if err != nil {
		t.Fatalf("invalid seed must be non-fatal: %v", err)
	}

	runtime.GC()
	runtime.ReadMemStats(&after)
	retainedBytes := int64(after.HeapAlloc) - int64(before.HeapAlloc)

	if len(report.Errors) != invalidSeedRows {
		t.Fatalf("retained %d diagnostics, expected one per rejected row (%d)",
			len(report.Errors), invalidSeedRows)
	}

	perError := float64(retainedBytes) / float64(len(report.Errors))
	limitRows := float64(10<<20) / 14.0 // 14-byte rows fill the 10 MiB allowance
	t.Logf("F-03 residual retention measurement")
	t.Logf("  seed size:                      %d bytes (%d rows, all rejected)", seedBytes, invalidSeedRows)
	t.Logf("  diagnostics retained:           %d", len(report.Errors))
	t.Logf("  diagnostics reported at startup: %d", maxSICSeedDiagnostics)
	t.Logf("  heap retained by the report:    %d bytes (%.1f MiB)", retainedBytes, float64(retainedBytes)/(1<<20))
	t.Logf("  per retained diagnostic:        %.1f bytes", perError)
	t.Logf("  extrapolated at the 10 MiB seed limit (~%.0f rows): %.1f MiB retained",
		limitRows, perError*limitRows/(1<<20))

	// runtime.KeepAlive-style guard: the report must still be live at the
	// second measurement for the delta to mean anything.
	if report.Outcome != model.SICMappingImportInvalid {
		t.Fatalf("Outcome = %q", report.Outcome)
	}
}
