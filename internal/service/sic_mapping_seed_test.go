package service

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/oronno/privateledger/internal/database"
	"github.com/oronno/privateledger/internal/model"
	"github.com/oronno/privateledger/internal/repository"
)

// seedFixture wires a real service against an isolated temporary database, so
// startup-seed behaviour is observed through the approved public contract only.
type seedFixture struct {
	t        *testing.T
	db       *sql.DB
	dir      string
	seedPath string
	sicRepo  *repository.SICMappingRepository
	catRepo  *repository.CategoryRepository
	service  *SICMappingService
}

func newSeedFixture(t *testing.T) *seedFixture {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(database.Config{Path: filepath.Join(dir, "privateledger.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	sicRepo := repository.NewSICMappingRepository(db)
	catRepo := repository.NewCategoryRepository(db)
	return &seedFixture{
		t:        t,
		db:       db,
		dir:      dir,
		seedPath: filepath.Join(dir, "sic_mappings.csv"),
		sicRepo:  sicRepo,
		catRepo:  catRepo,
		service:  NewSICMappingService(sicRepo, catRepo),
	}
}

func (f *seedFixture) writeSeed(content string) {
	f.t.Helper()
	if err := os.WriteFile(f.seedPath, []byte(content), 0o600); err != nil {
		f.t.Fatalf("write seed file: %v", err)
	}
}

func (f *seedFixture) addCategory(name string) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO category (name, category_type) VALUES (?, 2)`, name)
	if err != nil {
		f.t.Fatalf("insert category %q: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		f.t.Fatalf("category id: %v", err)
	}
	return int(id)
}

func (f *seedFixture) mappingCount() int {
	f.t.Helper()
	count, err := f.sicRepo.Count()
	if err != nil {
		f.t.Fatalf("count mappings: %v", err)
	}
	return count
}

func (f *seedFixture) mappings() map[string]*model.SICMapping {
	f.t.Helper()
	all, err := f.sicRepo.GetAll()
	if err != nil {
		f.t.Fatalf("get all mappings: %v", err)
	}
	out := make(map[string]*model.SICMapping, len(all))
	for _, m := range all {
		out[string(m.SICCode)] = m
	}
	return out
}

const seedHeader = "SIC_Code,Description,Description_Detail,Category_Name,Category_ID\n"

// ---------------------------------------------------------------------------
// Startup decision table - business-rules.md "Startup file decision"
// ---------------------------------------------------------------------------

// TestImportFileIfPresent_AbsentFile covers BR-CSV-02 / AC8: an absent file is
// normal and must not prevent startup.
func TestImportFileIfPresent_AbsentFile(t *testing.T) {
	f := newSeedFixture(t)
	if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
		t.Fatalf("an absent seed file must be non-fatal, got %v", err)
	}
	if f.mappingCount() != 0 {
		t.Errorf("an absent seed file created mappings")
	}
	// Repeating must remain safe.
	if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
		t.Fatalf("repeat with an absent file: %v", err)
	}
}

// TestImportFileIfPresent_ValidSeed covers the happy path of US-08 / FR4:
// canonical codes, stored descriptions, resolved and intentionally-NULL
// categories.
func TestImportFileIfPresent_ValidSeed(t *testing.T) {
	f := newSeedFixture(t)
	dining := f.addCategory("Dining")
	groceries := f.addCategory("Groceries")

	f.writeSeed(seedHeader +
		"5812,Eating Places,restaurants,Dining," + fmt.Sprint(dining) + "\n" +
		"5411,Grocery Stores,supermarkets,Groceries,\n" +
		"7011,Hotels,,,\n" +
		"0008021,Offices of Doctors,,,\n")

	if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
		t.Fatalf("ImportFileIfPresent(): %v", err)
	}

	got := f.mappings()
	if len(got) != 4 {
		t.Fatalf("expected 4 mappings, got %d (%v)", len(got), keysOf(got))
	}

	if m := got["5812"]; m == nil {
		t.Errorf("missing mapping 5812")
	} else {
		if m.Description != "Eating Places" || m.DescriptionDetail != "restaurants" {
			t.Errorf("5812 descriptions not stored: %+v", m)
		}
		if m.CategoryID == nil || *m.CategoryID != dining {
			t.Errorf("5812 category = %v, want %d", m.CategoryID, dining)
		}
	}
	if m := got["5411"]; m == nil || m.CategoryID == nil || *m.CategoryID != groceries {
		t.Errorf("5411 must resolve by name alone (BR-CAT-04): %+v", m)
	}
	if m := got["7011"]; m == nil || m.CategoryID != nil {
		t.Errorf("7011 with both category columns empty must store a NULL category (BR-CAT-01): %+v", m)
	}
	// BR-SIC-02: leading zeros are removed before storage.
	if _, ok := got["8021"]; !ok {
		t.Errorf("0008021 was not canonicalized to 8021; got keys %v", keysOf(got))
	}
	if _, ok := got["0008021"]; ok {
		t.Errorf("a non-canonical code was stored verbatim")
	}
}

// TestImportFileIfPresent_SkipsWhenMappingsExist covers BR-CSV-03 / AC8: SQLite
// stays authoritative and user edits are never reverted by a stale file.
func TestImportFileIfPresent_SkipsWhenMappingsExist(t *testing.T) {
	f := newSeedFixture(t)
	dining := f.addCategory("Dining")

	// Simulate a mapping the user edited in the UI after a previous import.
	edited := model.NewSICMapping("5812", "USER EDITED", "", &dining)
	if err := f.sicRepo.Create(edited); err != nil {
		t.Fatalf("seed existing mapping: %v", err)
	}

	f.writeSeed(seedHeader +
		"5812,Stale File Description,,,\n" +
		"7011,Should Not Appear,,,\n")

	if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
		t.Fatalf("skip path must be non-fatal, got %v", err)
	}

	got := f.mappings()
	if len(got) != 1 {
		t.Fatalf("the stale file was applied over existing mappings: %v", keysOf(got))
	}
	if got["5812"].Description != "USER EDITED" {
		t.Errorf("the user's edit was overwritten with %q", got["5812"].Description)
	}
	if got["5812"].CategoryID == nil || *got["5812"].CategoryID != dining {
		t.Errorf("the user's category assignment was changed")
	}
}

// TestImportFileIfPresent_RepeatedStartupsAreIdempotent covers NFR4 / AC8:
// repeated startups never duplicate or revert mappings.
func TestImportFileIfPresent_RepeatedStartupsAreIdempotent(t *testing.T) {
	f := newSeedFixture(t)
	f.addCategory("Dining")
	f.writeSeed(seedHeader + "5812,Eating Places,,Dining,\n7011,Hotels,,,\n")

	var snapshots []string
	for run := 1; run <= 3; run++ {
		if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		all, err := f.sicRepo.GetAll()
		if err != nil {
			t.Fatalf("run %d GetAll: %v", run, err)
		}
		parts := make([]string, 0, len(all))
		for _, m := range all {
			cat := "nil"
			if m.CategoryID != nil {
				cat = fmt.Sprint(*m.CategoryID)
			}
			parts = append(parts, fmt.Sprintf("%d|%s|%s|%s", m.SICMappingID, m.SICCode, m.Description, cat))
		}
		snapshots = append(snapshots, strings.Join(parts, ";"))
	}
	for i := 1; i < len(snapshots); i++ {
		if snapshots[i] != snapshots[0] {
			t.Errorf("startup %d changed the mapping set:\n first: %s\n  this: %s", i+1, snapshots[0], snapshots[i])
		}
	}
	if f.mappingCount() != 2 {
		t.Errorf("repeated startups produced %d mappings, want 2", f.mappingCount())
	}
}

// TestImportFileIfPresent_OversizedSeed covers NFR-U1-SEC-03 / NFR-U1-REL-03:
// a file above 10 MiB is rejected before parsing, imports nothing, and is
// non-fatal.
func TestImportFileIfPresent_OversizedSeed(t *testing.T) {
	f := newSeedFixture(t)
	f.addCategory("Dining")

	const limit = 10 << 20
	var b strings.Builder
	b.WriteString(seedHeader)
	row := "5812,Eating Places,restaurants,Dining,\n"
	for b.Len() <= limit {
		b.WriteString(row)
	}
	f.writeSeed(b.String())

	info, err := os.Stat(f.seedPath)
	if err != nil {
		t.Fatalf("stat seed: %v", err)
	}
	if info.Size() <= limit {
		t.Fatalf("fixture is not oversized: %d bytes", info.Size())
	}

	if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
		t.Fatalf("an oversized seed must be non-fatal, got %v", err)
	}
	if f.mappingCount() != 0 {
		t.Errorf("an oversized seed imported %d mappings, want 0", f.mappingCount())
	}
}

// TestImportFileIfPresent_SizeBoundary pins the exact accept/reject edge of the
// 10 MiB limit.
func TestImportFileIfPresent_SizeBoundary(t *testing.T) {
	const limit = 10 << 20

	build := func(size int) string {
		var b strings.Builder
		b.WriteString(seedHeader)
		// One real row, then pad the trailing description field of that row.
		prefix := "5812,"
		suffix := ",,,\n"
		padLen := size - b.Len() - len(prefix) - len(suffix)
		if padLen < 0 {
			panic("size too small")
		}
		b.WriteString(prefix)
		b.WriteString(strings.Repeat("d", padLen))
		b.WriteString(suffix)
		return b.String()
	}

	t.Run("exactly at the limit is accepted", func(t *testing.T) {
		f := newSeedFixture(t)
		content := build(limit)
		if len(content) != limit {
			t.Fatalf("fixture is %d bytes, want exactly %d", len(content), limit)
		}
		f.writeSeed(content)
		if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
			t.Fatalf("a file exactly at the limit must be accepted: %v", err)
		}
		if f.mappingCount() != 1 {
			t.Errorf("expected the boundary file to import 1 mapping, got %d", f.mappingCount())
		}
	})

	t.Run("one byte over the limit is rejected", func(t *testing.T) {
		f := newSeedFixture(t)
		content := build(limit + 1)
		if len(content) != limit+1 {
			t.Fatalf("fixture is %d bytes, want exactly %d", len(content), limit+1)
		}
		f.writeSeed(content)
		if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
			t.Fatalf("one byte over the limit must be non-fatal: %v", err)
		}
		if f.mappingCount() != 0 {
			t.Errorf("a file one byte over the limit imported %d mappings", f.mappingCount())
		}
	})
}

// TestImportFileIfPresent_Symlink covers NFRP-U1-03: a symbolic link is a
// permitted read-only input and the size gate binds to the opened target.
func TestImportFileIfPresent_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	t.Run("valid target is followed", func(t *testing.T) {
		f := newSeedFixture(t)
		f.addCategory("Dining")
		target := filepath.Join(f.dir, "real_mappings.csv")
		if err := os.WriteFile(target, []byte(seedHeader+"5812,Eating Places,,Dining,\n"), 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}
		if err := os.Symlink(target, f.seedPath); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
			t.Fatalf("ImportFileIfPresent(): %v", err)
		}
		if f.mappingCount() != 1 {
			t.Errorf("the symlinked seed imported %d mappings, want 1", f.mappingCount())
		}
	})

	t.Run("oversized target is rejected through the link", func(t *testing.T) {
		f := newSeedFixture(t)
		target := filepath.Join(f.dir, "huge.csv")
		var b strings.Builder
		b.WriteString(seedHeader)
		for b.Len() <= 10<<20 {
			b.WriteString("5812,x,,,\n")
		}
		if err := os.WriteFile(target, []byte(b.String()), 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}
		if err := os.Symlink(target, f.seedPath); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
			t.Fatalf("must be non-fatal: %v", err)
		}
		if f.mappingCount() != 0 {
			t.Errorf("the oversized symlink target imported %d mappings", f.mappingCount())
		}
	})

	t.Run("dangling link is treated as absent", func(t *testing.T) {
		f := newSeedFixture(t)
		if err := os.Symlink(filepath.Join(f.dir, "nope.csv"), f.seedPath); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
			t.Fatalf("a dangling link must be treated as absent, got %v", err)
		}
		if f.mappingCount() != 0 {
			t.Errorf("a dangling link created mappings")
		}
	})
}

// TestImportFileIfPresent_InvalidSeedIsNonFatalAndAtomic covers BR-CSV-05 /
// BR-CSV-06 / AC "mixed valid and invalid imports zero rows".
func TestImportFileIfPresent_InvalidSeedIsNonFatalAndAtomic(t *testing.T) {
	cases := []struct {
		name    string
		content func(diningID int) string
	}{
		{
			name:    "one invalid SIC among valid rows",
			content: func(int) string { return seedHeader + "5812,ok,,,\n12A4,bad,,,\n7011,ok,,,\n" },
		},
		{
			name:    "empty SIC",
			content: func(int) string { return seedHeader + "5812,ok,,,\n,bad,,,\n" },
		},
		{
			name:    "zero SIC",
			content: func(int) string { return seedHeader + "0,bad,,,\n" },
		},
		{
			name:    "int64 overflow SIC",
			content: func(int) string { return seedHeader + "9223372036854775808,bad,,,\n" },
		},
		{
			name:    "duplicate canonical SIC after normalization",
			content: func(int) string { return seedHeader + "111,first,,,\n0111,second,,,\n" },
		},
		{
			name:    "exact duplicate SIC",
			content: func(int) string { return seedHeader + "5812,first,,,\n5812,second,,,\n" },
		},
		{
			name:    "category id without name",
			content: func(id int) string { return seedHeader + "5812,ok,," + "," + fmt.Sprint(id) + "\n" },
		},
		{
			name:    "category name does not resolve",
			content: func(int) string { return seedHeader + "5812,ok,,No Such Category,\n" },
		},
		{
			name:    "category name and id conflict",
			content: func(id int) string { return seedHeader + "5812,ok,,Dining," + fmt.Sprint(id+500) + "\n" },
		},
		{
			name:    "non numeric category id",
			content: func(int) string { return seedHeader + "5812,ok,,Dining,abc\n" },
		},
		{
			name:    "zero category id",
			content: func(int) string { return seedHeader + "5812,ok,,Dining,0\n" },
		},
		{
			name:    "negative category id",
			content: func(int) string { return seedHeader + "5812,ok,,Dining,-3\n" },
		},
		{
			name:    "wrong header names",
			content: func(int) string { return "sic,desc,detail,cat,id\n5812,ok,,,\n" },
		},
		{
			name: "header column order swapped",
			content: func(int) string {
				return "Description,SIC_Code,Description_Detail,Category_Name,Category_ID\nok,5812,,,\n"
			},
		},
		{
			name:    "too few header columns",
			content: func(int) string { return "SIC_Code,Description,Description_Detail,Category_Name\n5812,ok,,\n" },
		},
		{
			name:    "empty file",
			content: func(int) string { return "" },
		},
		{
			name:    "header only whitespace",
			content: func(int) string { return "\n" },
		},
		{
			name:    "ragged row with too few fields",
			content: func(int) string { return seedHeader + "5812,ok,,,\n7011,ok\n" },
		},
		{
			name:    "ragged row with too many fields",
			content: func(int) string { return seedHeader + "5812,ok,,,,extra\n" },
		},
		{
			name:    "unterminated quoted field",
			content: func(int) string { return seedHeader + "5812,\"unclosed,,,\n" },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSeedFixture(t)
			dining := f.addCategory("Dining")
			f.writeSeed(tc.content(dining))

			if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
				t.Fatalf("an invalid seed must be non-fatal (BR-CSV-06), got %v", err)
			}
			if got := f.mappingCount(); got != 0 {
				t.Errorf("an invalid seed imported %d mappings, want 0", got)
			}
		})
	}
}

// TestImportFileIfPresent_HeaderOnlySeed documents the header-only case: the
// file is structurally valid and imports zero rows without failing startup.
func TestImportFileIfPresent_HeaderOnlySeed(t *testing.T) {
	f := newSeedFixture(t)
	f.writeSeed(seedHeader)
	if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
		t.Fatalf("a header-only seed must be accepted: %v", err)
	}
	if f.mappingCount() != 0 {
		t.Errorf("a header-only seed imported %d mappings", f.mappingCount())
	}
}

// TestImportFileIfPresent_ReadFailureIsFatal covers NFRP-U1-06: a read failure
// other than absence is a fatal initialization error.
func TestImportFileIfPresent_ReadFailureIsFatal(t *testing.T) {
	t.Run("seed path is a directory", func(t *testing.T) {
		f := newSeedFixture(t)
		if err := os.Mkdir(f.seedPath, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		err := f.service.ImportFileIfPresent(f.seedPath)
		if err == nil {
			t.Fatalf("expected a contextual initialization error for an unreadable seed target")
		}
		if f.mappingCount() != 0 {
			t.Errorf("a failed read created mappings")
		}
	})

	t.Run("seed file is unreadable", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root; permission bits are not enforced")
		}
		f := newSeedFixture(t)
		f.writeSeed(seedHeader + "5812,ok,,,\n")
		if err := os.Chmod(f.seedPath, 0o000); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		t.Cleanup(func() { os.Chmod(f.seedPath, 0o600) })

		if err := f.service.ImportFileIfPresent(f.seedPath); err == nil {
			t.Errorf("expected a contextual initialization error for an unreadable seed file")
		}
		if f.mappingCount() != 0 {
			t.Errorf("a failed read created mappings")
		}
	})
}

// TestImportFileIfPresent_PersistenceFailureRollsBack covers BR-CSV-07 /
// NFR-U1-REL-04 / NFRP-U1-06: a persistence failure after successful validation
// rolls back and returns a startup initialization error with no partial state.
func TestImportFileIfPresent_PersistenceFailureRollsBack(t *testing.T) {
	f := newSeedFixture(t)
	f.addCategory("Dining")

	// Inject a persistence failure that occurs only at INSERT time, after the
	// empty-table gate and whole-file validation have already succeeded.
	if _, err := f.db.Exec(`
		CREATE TRIGGER block_sic_insert BEFORE INSERT ON sic_mapping
		BEGIN SELECT RAISE(ABORT, 'injected persistence failure'); END`); err != nil {
		t.Fatalf("install trigger: %v", err)
	}

	f.writeSeed(seedHeader + "5812,Eating Places,,Dining,\n7011,Hotels,,,\n5411,Grocery,,,\n")

	err := f.service.ImportFileIfPresent(f.seedPath)
	if err == nil {
		t.Fatalf("a persistence failure must return a startup initialization error")
	}
	if !strings.Contains(err.Error(), "persist") {
		t.Errorf("expected a contextual persistence error, got %q", err.Error())
	}

	if _, dropErr := f.db.Exec(`DROP TRIGGER block_sic_insert`); dropErr != nil {
		t.Fatalf("drop trigger: %v", dropErr)
	}
	if got := f.mappingCount(); got != 0 {
		t.Errorf("a rolled-back seed left %d partial mappings, want 0", got)
	}
}

// TestImportFileIfPresent_CountFailureIsFatal covers the database error branch
// of the empty-table gate.
func TestImportFileIfPresent_CountFailureIsFatal(t *testing.T) {
	f := newSeedFixture(t)
	f.writeSeed(seedHeader + "5812,ok,,,\n")
	if _, err := f.db.Exec(`DROP TABLE sic_mapping`); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	if err := f.service.ImportFileIfPresent(f.seedPath); err == nil {
		t.Errorf("a failing mapping count must fail startup")
	}
}

// ---------------------------------------------------------------------------
// Category reference decision table - business-rules.md BR-CAT-01..06
// ---------------------------------------------------------------------------

func TestValidateCSV_CategoryResolutionDecisionTable(t *testing.T) {
	f := newSeedFixture(t)
	food := f.addCategory("Food")
	lowerFood := f.addCategory("food") // case variant; the schema allows both
	travel := f.addCategory("Travel")

	cases := []struct {
		name        string
		nameField   string
		idField     string
		wantErr     bool
		wantCatID   *int
		description string
	}{
		{name: "empty name and empty id accepts NULL", nameField: "", idField: "", wantCatID: nil,
			description: "BR-CAT-01"},
		{name: "id without name is rejected", nameField: "", idField: fmt.Sprint(travel), wantErr: true,
			description: "BR-CAT-02"},
		{name: "exact name resolves", nameField: "Travel", idField: "", wantCatID: &travel,
			description: "BR-CAT-04"},
		{name: "name is trimmed before lookup", nameField: "  Travel  ", idField: "", wantCatID: &travel,
			description: "BR-CAT-03"},
		{name: "unique case-insensitive match resolves", nameField: "TRAVEL", idField: "", wantCatID: &travel,
			description: "BR-CAT-04"},
		{name: "exact match wins over case variants", nameField: "Food", idField: "", wantCatID: &food,
			description: "BR-CAT-04 exact-first"},
		{name: "exact lowercase variant also resolves exactly", nameField: "food", idField: "", wantCatID: &lowerFood,
			description: "BR-CAT-04 exact-first"},
		{name: "ambiguous case-insensitive match rejected", nameField: "FOOD", idField: "", wantErr: true,
			description: "BR-CAT-05"},
		{name: "unknown name rejected", nameField: "Nonexistent", idField: "", wantErr: true,
			description: "BR-CAT-05"},
		{name: "agreeing id confirms the name", nameField: "Travel", idField: fmt.Sprint(travel), wantCatID: &travel,
			description: "BR-CAT-06"},
		{name: "disagreeing id rejected", nameField: "Travel", idField: fmt.Sprint(food), wantErr: true,
			description: "BR-CAT-06 / AC16"},
		{name: "id whitespace tolerated", nameField: "Travel", idField: " " + fmt.Sprint(travel) + " ", wantCatID: &travel,
			description: "BR-CAT-03"},
		{name: "malformed id rejected", nameField: "Travel", idField: "12x", wantErr: true,
			description: "BR-CAT-06"},
		{name: "zero id rejected", nameField: "Travel", idField: "0", wantErr: true,
			description: "BR-CAT-06"},
		{name: "negative id rejected", nameField: "Travel", idField: "-1", wantErr: true,
			description: "BR-CAT-06"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			csv := seedHeader + "5812,desc,detail," + tc.nameField + "," + tc.idField + "\n"
			mappings, report, err := f.service.ValidateCSV(strings.NewReader(csv))
			if err != nil {
				t.Fatalf("%s: ValidateCSV returned a fatal error: %v", tc.description, err)
			}
			if tc.wantErr {
				if report.RejectedRows == 0 {
					t.Fatalf("%s: expected the row to be rejected, report=%+v", tc.description, report)
				}
				if len(mappings) != 0 {
					t.Errorf("%s: rejected row still produced %d candidates", tc.description, len(mappings))
				}
				if len(report.Errors) == 0 {
					t.Errorf("%s: rejection produced no row diagnostic (BR-ERR-02)", tc.description)
				}
				for _, e := range report.Errors {
					if e.RowNumber != 2 {
						t.Errorf("%s: diagnostic points at row %d, want 2", tc.description, e.RowNumber)
					}
					if e.Code == "" || e.Field == "" {
						t.Errorf("%s: diagnostic is missing a stable field/code: %+v", tc.description, e)
					}
				}
				return
			}
			if report.RejectedRows != 0 {
				t.Fatalf("%s: unexpected rejection: %+v", tc.description, report.Errors)
			}
			if len(mappings) != 1 {
				t.Fatalf("%s: expected 1 candidate, got %d", tc.description, len(mappings))
			}
			got := mappings[0].CategoryID
			switch {
			case tc.wantCatID == nil && got != nil:
				t.Errorf("%s: expected a NULL category, got %d", tc.description, *got)
			case tc.wantCatID != nil && got == nil:
				t.Errorf("%s: expected category %d, got NULL", tc.description, *tc.wantCatID)
			case tc.wantCatID != nil && got != nil && *got != *tc.wantCatID:
				t.Errorf("%s: expected category %d, got %d", tc.description, *tc.wantCatID, *got)
			}
		})
	}
}

// TestValidateCSV_DiagnosticsAreSafe covers BR-ERR-01 / NFR-U1-SEC-02: row
// diagnostics must not carry protected description or account content. UOW-4
// deliberately permits the Category_Name field itself after bounding and
// sanitization, so distinct markers keep that exception from weakening the
// original privacy assertion.
func TestValidateCSV_DiagnosticsAreSafe(t *testing.T) {
	f := newSeedFixture(t)
	const protectedDescription = "ACCOUNT-4111111111111111-SECRET"
	const unresolvedCategory = "Nonexistent Category"
	csv := seedHeader + "12A4," + protectedDescription + "," + protectedDescription + "," + unresolvedCategory + ",99\n"

	_, report, err := f.service.ValidateCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ValidateCSV(): %v", err)
	}
	if report.RejectedRows == 0 {
		t.Fatalf("expected the row to be rejected")
	}
	for _, e := range report.Errors {
		blob := fmt.Sprintf("%s|%s|%s", e.Field, e.Code, e.Message)
		if strings.Contains(blob, protectedDescription) {
			t.Errorf("row diagnostic leaks protected description content: %q", blob)
		}
	}
}

// TestValidateCSV_ReportShape checks the counters the domain design defines for
// SICMappingImportReport.
func TestValidateCSV_ReportShape(t *testing.T) {
	f := newSeedFixture(t)
	f.addCategory("Dining")

	t.Run("all rows valid", func(t *testing.T) {
		mappings, report, err := f.service.ValidateCSV(strings.NewReader(
			seedHeader + "5812,a,,Dining,\n7011,b,,,\n5411,c,,,\n"))
		if err != nil {
			t.Fatalf("ValidateCSV(): %v", err)
		}
		if report.TotalRows != 3 || report.ValidRows != 3 || report.RejectedRows != 0 {
			t.Errorf("report counters = %+v, want Total=3 Valid=3 Rejected=0", report)
		}
		if len(mappings) != 3 {
			t.Errorf("expected 3 candidates, got %d", len(mappings))
		}
		// The domain design states ImportedRows stays 0 until the candidate set
		// commits, and that a valid report does not imply persistence.
		if report.ImportedRows != 0 {
			t.Errorf("ValidateCSV reported ImportedRows=%d before any persistence", report.ImportedRows)
		}
	})

	t.Run("mixed rows import nothing", func(t *testing.T) {
		mappings, report, err := f.service.ValidateCSV(strings.NewReader(
			seedHeader + "5812,a,,Dining,\n12A4,b,,,\n5411,c,,,\n"))
		if err != nil {
			t.Fatalf("ValidateCSV(): %v", err)
		}
		if len(mappings) != 0 {
			t.Errorf("a file with any invalid row must yield no candidates, got %d", len(mappings))
		}
		if report.TotalRows != 3 || report.ValidRows != 2 || report.RejectedRows != 1 {
			t.Errorf("report counters = %+v, want Total=3 Valid=2 Rejected=1", report)
		}
	})

	t.Run("multiple invalid rows are each reported", func(t *testing.T) {
		_, report, err := f.service.ValidateCSV(strings.NewReader(
			seedHeader + "12A4,a,,,\n5812,b,,Nonexistent,\nabc,c,,,\n"))
		if err != nil {
			t.Fatalf("ValidateCSV(): %v", err)
		}
		if report.RejectedRows != 3 {
			t.Errorf("RejectedRows = %d, want 3", report.RejectedRows)
		}
		rows := map[int]bool{}
		for _, e := range report.Errors {
			rows[e.RowNumber] = true
		}
		for _, want := range []int{2, 3, 4} {
			if !rows[want] {
				t.Errorf("BR-ERR-02: no diagnostic reported for row %d (got %+v)", want, report.Errors)
			}
		}
	})
}

// TestValidateCSV_CRLFAndQuotedFields covers realistic interchange formatting.
func TestValidateCSV_CRLFAndQuotedFields(t *testing.T) {
	f := newSeedFixture(t)
	f.addCategory("Dining, Fast Food")

	csv := "SIC_Code,Description,Description_Detail,Category_Name,Category_ID\r\n" +
		"5812,\"Eating Places, incl. bars\",\"detail\r\nwith newline\",\"Dining, Fast Food\",\r\n"
	mappings, report, err := f.service.ValidateCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("ValidateCSV(): %v", err)
	}
	if report.RejectedRows != 0 {
		t.Fatalf("unexpected rejection: %+v", report.Errors)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(mappings))
	}
	if mappings[0].Description != "Eating Places, incl. bars" {
		t.Errorf("quoted description not preserved: %q", mappings[0].Description)
	}
	if !strings.Contains(mappings[0].DescriptionDetail, "\n") {
		t.Errorf("quoted multi-line detail not preserved: %q", mappings[0].DescriptionDetail)
	}
	if mappings[0].CategoryID == nil {
		t.Errorf("a quoted category name containing a comma did not resolve")
	}
}

func keysOf(m map[string]*model.SICMapping) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
