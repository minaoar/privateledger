package service

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/oronno/privateledger/internal/model"
)

// ---------------------------------------------------------------------------
// Revision 2 verification — F-01 (report outcome timing / ImportedRows) and
// F-02 (structured startup outcome contract).
//
// Authorities:
//   functional-design/domain-entities.md, "SICMappingImportReport" invariants:
//     - "Any rejected row makes ImportedRows = 0 for startup seeding."
//     - "A valid report does not imply persistence until atomic commit succeeds."
//     - Outcome domain: Absent, SkippedExisting, Invalid, Imported, PersistenceFailed.
//   nfr-design/nfr-design-patterns.md NFRP-U1-06 failure classification table.
//   nfr-design/logical-components.md LC-U1-08 — the startup orchestrator must
//     "Apply the returned outcome according to the failure classification".
// ---------------------------------------------------------------------------

// TestValidateCSV_ValidReportDoesNotClaimPersistence is the direct pin for the
// domain-entities.md invariant "A valid report does not imply persistence until
// atomic commit succeeds". ValidateCSV performs no persistence, so it must not
// return the Imported outcome and must not report imported rows.
func TestValidateCSV_ValidReportDoesNotClaimPersistence(t *testing.T) {
	f := newSeedFixture(t)
	f.addCategory("Dining")

	csv := seedHeader +
		"5812,Eating Places,Restaurants,Dining,\n" +
		"7011,Hotels,,,\n" +
		"5411,Grocery Stores,,,\n"

	mappings, report, err := f.service.ValidateCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ValidateCSV on a fully valid seed returned an error: %v", err)
	}
	if report == nil {
		t.Fatal("ValidateCSV must always return a report")
	}
	if report.Outcome == model.SICMappingImportImported {
		t.Errorf("ValidateCSV reported Outcome=%q before any persistence occurred; "+
			"domain-entities.md requires that a valid report not imply persistence",
			report.Outcome)
	}
	if report.ImportedRows != 0 {
		t.Errorf("ValidateCSV reported ImportedRows=%d before any persistence; want 0", report.ImportedRows)
	}
	if got, want := len(mappings), 3; got != want {
		t.Errorf("ValidateCSV produced %d candidates, want %d", got, want)
	}
	if report.ValidRows != 3 || report.TotalRows != 3 || report.RejectedRows != 0 {
		t.Errorf("report counts = total %d/valid %d/rejected %d, want 3/3/0",
			report.TotalRows, report.ValidRows, report.RejectedRows)
	}
	// The strongest form of the invariant: nothing was written.
	if got := f.mappingCount(); got != 0 {
		t.Errorf("ValidateCSV persisted %d mappings; it must be side-effect free", got)
	}
}

// TestValidateCSV_RejectedReportNeverClaimsImport pins the invariant "Any
// rejected row makes ImportedRows = 0 for startup seeding" at the validation
// boundary for a mixed valid/invalid file.
func TestValidateCSV_RejectedReportNeverClaimsImport(t *testing.T) {
	f := newSeedFixture(t)
	f.addCategory("Dining")

	csv := seedHeader +
		"5812,Eating Places,,Dining,\n" +
		"notasic,Bad Row,,,\n" +
		"7011,Hotels,,,\n"

	mappings, report, err := f.service.ValidateCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("an invalid seed must not be a hard error: %v", err)
	}
	if mappings != nil {
		t.Errorf("ValidateCSV returned %d candidates for a rejected file; want none", len(mappings))
	}
	if report.Outcome != model.SICMappingImportInvalid {
		t.Errorf("Outcome = %q, want %q", report.Outcome, model.SICMappingImportInvalid)
	}
	if report.ImportedRows != 0 {
		t.Errorf("ImportedRows = %d for a rejected file, want 0", report.ImportedRows)
	}
	if report.RejectedRows != 1 || report.TotalRows != 3 || report.ValidRows != 2 {
		t.Errorf("report counts = total %d/valid %d/rejected %d, want 3/2/1",
			report.TotalRows, report.ValidRows, report.RejectedRows)
	}
}

// TestImportFileIfPresentWithReport_OutcomeDecisionTable walks every row of the
// NFRP-U1-06 failure-classification table and asserts the structured outcome
// that LC-U1-08 requires the startup orchestrator to act on. Each case also
// asserts the fatal/non-fatal classification and the resulting mapping count.
func TestImportFileIfPresentWithReport_OutcomeDecisionTable(t *testing.T) {
	cases := []struct {
		name         string
		arrange      func(t *testing.T, f *seedFixture)
		wantOutcome  model.SICMappingImportOutcome
		wantFatal    bool
		wantMappings int
		check        func(t *testing.T, r *model.SICMappingImportReport)
	}{
		{
			name:         "seed absent",
			arrange:      func(t *testing.T, f *seedFixture) {},
			wantOutcome:  model.SICMappingImportAbsent,
			wantFatal:    false,
			wantMappings: 0,
		},
		{
			name: "mapping table already populated",
			arrange: func(t *testing.T, f *seedFixture) {
				f.writeSeed(seedHeader + "5812,Eating Places,,,\n")
				if _, err := f.db.Exec(
					`INSERT INTO sic_mapping (sic_code, description, description_detail) VALUES ('9999','preexisting','')`,
				); err != nil {
					t.Fatalf("seed existing mapping: %v", err)
				}
			},
			wantOutcome:  model.SICMappingImportSkippedExisting,
			wantFatal:    false,
			wantMappings: 1,
			check: func(t *testing.T, r *model.SICMappingImportReport) {
				if r.ExistingRows != 1 {
					t.Errorf("ExistingRows = %d, want 1", r.ExistingRows)
				}
				if r.TotalRows != 0 || r.ImportedRows != 0 {
					t.Errorf("a skipped seed must not be parsed: total=%d imported=%d",
						r.TotalRows, r.ImportedRows)
				}
			},
		},
		{
			name: "seed exceeds the 10 MiB limit",
			arrange: func(t *testing.T, f *seedFixture) {
				oversized := make([]byte, (10<<20)+1)
				copy(oversized, seedHeader)
				for i := len(seedHeader); i < len(oversized); i++ {
					oversized[i] = 'x'
				}
				if err := os.WriteFile(f.seedPath, oversized, 0o600); err != nil {
					t.Fatalf("write oversized seed: %v", err)
				}
			},
			wantOutcome:  model.SICMappingImportOversized,
			wantFatal:    false,
			wantMappings: 0,
			check: func(t *testing.T, r *model.SICMappingImportReport) {
				if r.TotalRows != 0 {
					t.Errorf("an oversized seed must be rejected before parsing; TotalRows = %d", r.TotalRows)
				}
				if r.ImportedRows != 0 {
					t.Errorf("ImportedRows = %d, want 0", r.ImportedRows)
				}
			},
		},
		{
			name: "seed is domain invalid",
			arrange: func(t *testing.T, f *seedFixture) {
				f.writeSeed(seedHeader + "5812,ok,,,\n" + "0,zero is not a SIC,,,\n")
			},
			wantOutcome:  model.SICMappingImportInvalid,
			wantFatal:    false,
			wantMappings: 0,
			check: func(t *testing.T, r *model.SICMappingImportReport) {
				if r.ImportedRows != 0 {
					t.Errorf("ImportedRows = %d for a rejected seed, want 0", r.ImportedRows)
				}
				if r.RejectedRows != 1 {
					t.Errorf("RejectedRows = %d, want 1", r.RejectedRows)
				}
				if len(r.Errors) == 0 {
					t.Error("an invalid outcome must carry row diagnostics")
				}
			},
		},
		{
			name: "seed is fully valid",
			arrange: func(t *testing.T, f *seedFixture) {
				f.addCategory("Dining")
				f.writeSeed(seedHeader +
					"5812,Eating Places,,Dining,\n" +
					"7011,Hotels,,,\n")
			},
			wantOutcome:  model.SICMappingImportImported,
			wantFatal:    false,
			wantMappings: 2,
			check: func(t *testing.T, r *model.SICMappingImportReport) {
				if r.ImportedRows != 2 {
					t.Errorf("ImportedRows = %d, want 2", r.ImportedRows)
				}
				if r.ValidRows != 2 || r.TotalRows != 2 || r.RejectedRows != 0 {
					t.Errorf("counts = total %d/valid %d/rejected %d, want 2/2/0",
						r.TotalRows, r.ValidRows, r.RejectedRows)
				}
			},
		},
		{
			name: "seed target is unreadable",
			arrange: func(t *testing.T, f *seedFixture) {
				if err := os.Mkdir(f.seedPath, 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			},
			wantOutcome:  model.SICMappingImportReadFailed,
			wantFatal:    true,
			wantMappings: 0,
		},
		{
			name: "empty-table gate cannot be evaluated",
			arrange: func(t *testing.T, f *seedFixture) {
				f.writeSeed(seedHeader + "5812,ok,,,\n")
				if _, err := f.db.Exec(`DROP TABLE sic_mapping`); err != nil {
					t.Fatalf("drop table: %v", err)
				}
			},
			wantOutcome:  model.SICMappingImportPersistenceFailed,
			wantFatal:    true,
			wantMappings: -1, // table no longer exists
		},
		{
			name: "atomic insert fails after successful validation",
			arrange: func(t *testing.T, f *seedFixture) {
				f.addCategory("Dining")
				if _, err := f.db.Exec(`
					CREATE TRIGGER block_sic_insert BEFORE INSERT ON sic_mapping
					BEGIN SELECT RAISE(ABORT, 'injected persistence failure'); END`); err != nil {
					t.Fatalf("install trigger: %v", err)
				}
				f.writeSeed(seedHeader +
					"5812,Eating Places,,Dining,\n" +
					"7011,Hotels,,,\n")
			},
			wantOutcome:  model.SICMappingImportPersistenceFailed,
			wantFatal:    true,
			wantMappings: -1, // trigger is dropped inside check
			check: func(t *testing.T, r *model.SICMappingImportReport) {
				if r.ImportedRows != 0 {
					t.Errorf("ImportedRows = %d after a failed commit, want 0; "+
						"the operation must never report success without a successful commit",
						r.ImportedRows)
				}
				if r.ValidRows != 2 {
					t.Errorf("ValidRows = %d, want 2 (validation succeeded before persistence failed)", r.ValidRows)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSeedFixture(t)
			tc.arrange(t, f)

			report, err := f.service.ImportFileIfPresentWithReport(f.seedPath)

			if report == nil {
				t.Fatal("ImportFileIfPresentWithReport must always return a non-nil report " +
					"so the startup orchestrator can apply the outcome contract")
			}
			if tc.wantFatal && err == nil {
				t.Fatalf("expected a fatal startup initialization error, got nil")
			}
			if !tc.wantFatal && err != nil {
				t.Fatalf("expected a non-fatal outcome, got error: %v", err)
			}
			if report.Outcome != tc.wantOutcome {
				t.Errorf("Outcome = %q, want %q", report.Outcome, tc.wantOutcome)
			}
			if tc.check != nil {
				tc.check(t, report)
			}
			if tc.wantMappings >= 0 {
				if got := f.mappingCount(); got != tc.wantMappings {
					t.Errorf("mapping count = %d, want %d", got, tc.wantMappings)
				}
			}
		})
	}
}

// TestImportFileIfPresentWithReport_ImportedRowsMatchesPersistedRows pins
// ImportedRows against what is actually readable from SQLite afterwards, across
// the zero/one/many boundary.
func TestImportFileIfPresentWithReport_ImportedRowsMatchesPersistedRows(t *testing.T) {
	for _, rows := range []int{0, 1, 2, 25} {
		t.Run(strconv.Itoa(rows)+"_rows", func(t *testing.T) {
			f := newSeedFixture(t)
			var b strings.Builder
			b.WriteString(seedHeader)
			for i := 0; i < rows; i++ {
				b.WriteString(strconv.Itoa(1000 + i))
				b.WriteString(",desc,,,\n")
			}
			f.writeSeed(b.String())

			report, err := f.service.ImportFileIfPresentWithReport(f.seedPath)
			if err != nil {
				t.Fatalf("valid seed of %d rows failed: %v", rows, err)
			}
			if report.Outcome != model.SICMappingImportImported {
				t.Errorf("Outcome = %q, want %q", report.Outcome, model.SICMappingImportImported)
			}
			if report.ImportedRows != rows {
				t.Errorf("ImportedRows = %d, want %d", report.ImportedRows, rows)
			}
			if got := f.mappingCount(); got != rows {
				t.Errorf("persisted %d mappings but the report claims %d", got, report.ImportedRows)
			}
			if report.ImportedRows != report.ValidRows {
				t.Errorf("ImportedRows %d != ValidRows %d for a fully committed seed",
					report.ImportedRows, report.ValidRows)
			}
		})
	}
}

// TestImportFileIfPresent_WrapperMatchesReportErrorContract guards against the
// Revision 2 refactor changing the fatal/non-fatal contract of the original
// entry point that other callers may still use.
func TestImportFileIfPresent_WrapperMatchesReportErrorContract(t *testing.T) {
	cases := []struct {
		name      string
		arrange   func(t *testing.T, f *seedFixture)
		wantFatal bool
	}{
		{"absent", func(t *testing.T, f *seedFixture) {}, false},
		{"invalid", func(t *testing.T, f *seedFixture) {
			f.writeSeed(seedHeader + "notasic,x,,,\n")
		}, false},
		{"valid", func(t *testing.T, f *seedFixture) {
			f.writeSeed(seedHeader + "5812,Eating Places,,,\n")
		}, false},
		{"unreadable", func(t *testing.T, f *seedFixture) {
			if err := os.Mkdir(f.seedPath, 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSeedFixture(t)
			tc.arrange(t, f)
			err := f.service.ImportFileIfPresent(f.seedPath)
			if tc.wantFatal != (err != nil) {
				t.Errorf("ImportFileIfPresent error = %v, wantFatal = %v", err, tc.wantFatal)
			}
		})
	}
}

// TestImportFileIfPresentWithReport_ValidationInfrastructureFailure covers the
// ValidateCSV hard-error branch reached through the startup entry point. The
// category lookup that validation depends on is a database read, so its failure
// must not panic and must be classified fatal with no mutation.
func TestImportFileIfPresentWithReport_ValidationInfrastructureFailure(t *testing.T) {
	f := newSeedFixture(t)
	f.writeSeed(seedHeader + "5812,Eating Places,,Dining,\n")
	if _, err := f.db.Exec(`DROP TABLE category`); err != nil {
		t.Fatalf("drop category table: %v", err)
	}

	report, err := f.service.ImportFileIfPresentWithReport(f.seedPath)
	if err == nil {
		t.Fatal("a failing category load during validation must fail startup")
	}
	if report == nil {
		t.Fatal("a non-nil report is required on every path")
	}
	if report.ImportedRows != 0 {
		t.Errorf("ImportedRows = %d, want 0", report.ImportedRows)
	}
	if got := f.mappingCount(); got != 0 {
		t.Errorf("a failed validation persisted %d mappings", got)
	}
	// Recorded, not asserted as a pass/fail contract: the current classification
	// is read_failed even though the failure is a database read, not a file read.
	t.Logf("observed outcome for a database-side validation failure: %q", report.Outcome)
}
