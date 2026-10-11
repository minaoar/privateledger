package service

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestPerformance_StartupSeed measures NFR-U1-PERF-03: whole-file validation
// plus atomic insertion of a valid 100,000-row seed within ten seconds, using a
// file that stays inside the accepted 10 MiB limit.
func TestPerformance_StartupSeed(t *testing.T) {
	if testing.Short() {
		t.Skip("performance evidence is skipped in -short mode")
	}
	if raceDetectorEnabled {
		t.Skip("performance targets are acceptance evidence for an uninstrumented build; the race detector adds roughly an order of magnitude of overhead")
	}
	const (
		rowCount  = 100_000
		sizeLimit = 10 << 20
		target    = 10 * time.Second
	)

	f := newSeedFixture(t)
	categoryIDs := make([]int, 0, 10)
	categoryNames := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("Cat%02d", i)
		categoryNames = append(categoryNames, name)
		categoryIDs = append(categoryIDs, f.addCategory(name))
	}

	// Fixture generation is deliberately outside the measured window.
	var b strings.Builder
	b.Grow(sizeLimit)
	b.WriteString(seedHeader)
	for i := 1; i <= rowCount; i++ {
		idx := i % len(categoryNames)
		switch i % 3 {
		case 0: // intentionally unmapped
			fmt.Fprintf(&b, "%d,D%05d,X%05d,,\n", i, i%9999, i%777)
		case 1: // resolved by name
			fmt.Fprintf(&b, "%d,D%05d,X%05d,%s,\n", i, i%9999, i%777, categoryNames[idx])
		default: // resolved by name and confirmed by id
			fmt.Fprintf(&b, "%d,D%05d,X%05d,%s,%d\n", i, i%9999, i%777, categoryNames[idx], categoryIDs[idx])
		}
	}
	content := b.String()
	if len(content) > sizeLimit {
		t.Fatalf("the %d-row fixture is %d bytes, above the accepted %d-byte limit", rowCount, len(content), sizeLimit)
	}
	f.writeSeed(content)

	info, err := os.Stat(f.seedPath)
	if err != nil {
		t.Fatalf("stat seed: %v", err)
	}

	start := time.Now()
	if err := f.service.ImportFileIfPresent(f.seedPath); err != nil {
		t.Fatalf("ImportFileIfPresent(): %v", err)
	}
	elapsed := time.Since(start)

	if got := f.mappingCount(); got != rowCount {
		t.Fatalf("imported %d mappings, want %d", got, rowCount)
	}

	t.Logf("NFR-U1-PERF-03 validation+commit of a %d-row / %d-byte seed: %v (target %v)",
		rowCount, info.Size(), elapsed, target)
	if elapsed > target {
		t.Errorf("NFR-U1-PERF-03 FAILED: seed import took %v, target is %v", elapsed, target)
	}
}
