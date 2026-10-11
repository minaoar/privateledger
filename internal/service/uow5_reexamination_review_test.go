package service

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oronno/privateledger/internal/model"
)

func TestReviewU5ReexaminationCountsPriorityManualAndIdempotence(t *testing.T) {
	db, txnRepo, patternRepo, sicRepo, _, categorizer, accountID := newUOW3ServiceHarness(t)
	patternCategory := createUOW3Category(t, db, "Pattern target")
	sicCategory := createUOW3Category(t, db, "SIC target")
	oldCategory := createUOW3Category(t, db, "Old")
	if err := patternRepo.Create(model.NewCategoryPattern("CAFE", patternCategory)); err != nil {
		t.Fatal(err)
	}
	if err := sicRepo.Create(model.NewSICMapping("5812", "", "", &sicCategory)); err != nil {
		t.Fatal(err)
	}

	patternMoved := createUOW3Txn(t, txnRepo, accountID, "pattern-move", "CAFE", "5812", &oldCategory, model.CategorySourceRule)
	sicMoved := createUOW3Txn(t, txnRepo, accountID, "sic-move", "HOTEL", "5812", &oldCategory, model.CategorySourceRule)
	cleared := createUOW3Txn(t, txnRepo, accountID, "clear", "UNKNOWN", "9999", &oldCategory, model.CategorySourceRule)
	unchanged := createUOW3Txn(t, txnRepo, accountID, "unchanged", "CAFE", "5812", &patternCategory, model.CategorySourceRule)
	manualMapped := createUOW3Txn(t, txnRepo, accountID, "manual-map", "HOTEL", "5812", &oldCategory, model.CategorySourceManual)
	manualCleared := createUOW3Txn(t, txnRepo, accountID, "manual-clear", "UNKNOWN", "9999", &oldCategory, model.CategorySourceManual)

	// A write to an unchanged row would abort the pass. This makes BR-U5-17
	// observable instead of inferring it from a zero count.
	if _, err := db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_unchanged_uow5
		BEFORE UPDATE OF category_id ON ledger_transaction
		WHEN OLD.transaction_id = %d
		BEGIN SELECT RAISE(ABORT, 'unchanged row was written'); END`, unchanged.TransactionID)); err != nil {
		t.Fatal(err)
	}

	result, err := categorizer.Reexamine()
	if err != nil {
		t.Fatal(err)
	}
	if result.ProcessedCount != 6 || result.MovedCount != 2 || result.UncategorizedCount != 1 ||
		result.ManualProtectedCount != 2 || result.CategorizedCount != 2 ||
		result.PatternCategorizedCount != 1 || result.SICCategorizedCount != 1 {
		t.Fatalf("unexpected re-examination counts: %+v", result)
	}

	checks := []struct {
		txn      *model.Transaction
		category *int
		source   model.CategorySource
	}{
		{patternMoved, &patternCategory, model.CategorySourceRule},
		{sicMoved, &sicCategory, model.CategorySourceRule},
		{cleared, nil, model.CategorySourceNone},
		{unchanged, &patternCategory, model.CategorySourceRule},
		{manualMapped, &oldCategory, model.CategorySourceManual},
		{manualCleared, &oldCategory, model.CategorySourceManual},
	}
	for _, check := range checks {
		stored, err := txnRepo.GetByID(check.txn.TransactionID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.CategorySource != check.source || (stored.CategoryID == nil) != (check.category == nil) ||
			(check.category != nil && *stored.CategoryID != *check.category) {
			t.Errorf("%s stored=%+v, want category=%v source=%v", check.txn.FitID, stored, check.category, check.source)
		}
	}

	// Manual-protected is an observation of current rules and therefore remains
	// repeatable. Idempotence is enforced through zero writes and zero persisted
	// changes; the no-manual property below pins the literal three-zero case.
	second, err := categorizer.Reexamine()
	if err != nil {
		t.Fatal(err)
	}
	if second.MovedCount != 0 || second.UncategorizedCount != 0 || second.ManualProtectedCount != 2 {
		t.Fatalf("second pass was not state-idempotent: %+v", second)
	}
}

func TestReviewU5SecondPassReportsThreeZerosWithoutManualCandidates(t *testing.T) {
	db, txnRepo, _, sicRepo, _, categorizer, accountID := newUOW3ServiceHarness(t)
	category := createUOW3Category(t, db, "Mapped")
	if err := sicRepo.Create(model.NewSICMapping("5812", "", "", &category)); err != nil {
		t.Fatal(err)
	}
	createUOW3Txn(t, txnRepo, accountID, "idempotent", "HOTEL", "5812", nil, model.CategorySourceNone)
	if _, err := categorizer.Reexamine(); err != nil {
		t.Fatal(err)
	}
	result, err := categorizer.Reexamine()
	if err != nil {
		t.Fatal(err)
	}
	if result.MovedCount != 0 || result.UncategorizedCount != 0 || result.ManualProtectedCount != 0 {
		t.Fatalf("second pass counts=%+v, want three zeros", result)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"moved_count":0`, `"uncategorized_count":0`, `"manual_protected_count":0`} {
		if !strings.Contains(string(payload), field) {
			t.Errorf("zero field %s omitted from %s", field, payload)
		}
	}
}

type uow5BlockingLookup struct {
	mu        sync.RWMutex
	category  int
	started   chan struct{}
	release   chan struct{}
	firstOnce sync.Once
	prepares  int
}

// uow5ShippedReloadProbe delegates to the production SIC cache but pauses after
// the pass samples its mapping generation. It supports both the live per-code
// lookup and a prepared whole-index snapshot so the assertion does not require
// one implementation mechanism.
type uow5ShippedReloadProbe struct {
	inner     *SICMappingCategorizer
	observed  chan struct{}
	release   chan struct{}
	firstOnce sync.Once
	prepares  int
}

// uow5FailingPassSnapshotLookup makes the initial LoadRules staging read
// succeed, then fails the pass-level staging read. Its live cache can still be
// reloaded while the fallback resolves individual codes. This is the precise
// failure window in which falling back to per-code reads would reintroduce the
// mixed-generation defect that the atomic snapshot is meant to prevent.
type uow5FailingPassSnapshotLookup struct {
	mu        sync.RWMutex
	current   int
	next      int
	prepares  int
	observed  chan struct{}
	release   chan struct{}
	firstOnce sync.Once
}

func (l *uow5FailingPassSnapshotLookup) LookupCategory(model.SICCode) (int, bool) {
	l.mu.RLock()
	category := l.current
	l.mu.RUnlock()
	l.firstOnce.Do(func() {
		close(l.observed)
		<-l.release
	})
	return category, true
}

func (l *uow5FailingPassSnapshotLookup) ReloadMappings() error {
	l.mu.Lock()
	l.current = l.next
	l.mu.Unlock()
	return nil
}

func (l *uow5FailingPassSnapshotLookup) prepareMappings() (map[model.SICCode]*model.SICMapping, error) {
	l.prepares++
	if l.prepares > 1 {
		return nil, fmt.Errorf("injected pass snapshot failure")
	}
	category := l.current
	return map[model.SICCode]*model.SICMapping{
		"5812": model.NewSICMapping("5812", "", "", &category),
		"5411": model.NewSICMapping("5411", "", "", &category),
	}, nil
}

func (l *uow5FailingPassSnapshotLookup) commitMappings(index map[model.SICCode]*model.SICMapping) {
	l.mu.Lock()
	l.current = *index["5812"].CategoryID
	l.mu.Unlock()
}

func (l *uow5ShippedReloadProbe) pauseAfterSnapshot() {
	l.firstOnce.Do(func() {
		close(l.observed)
		<-l.release
	})
}

func (l *uow5ShippedReloadProbe) LookupCategory(code model.SICCode) (int, bool) {
	categoryID, ok := l.inner.LookupCategory(code)
	l.pauseAfterSnapshot()
	return categoryID, ok
}

func (l *uow5ShippedReloadProbe) ReloadMappings() error { return l.inner.ReloadMappings() }

func (l *uow5ShippedReloadProbe) prepareMappings() (map[model.SICCode]*model.SICMapping, error) {
	index, err := l.inner.prepareMappings()
	l.prepares++
	// Reexamine's initial LoadRules is the first prepare. A production fix may
	// take the pass snapshot with a second atomic prepare instead of calling
	// LookupCategory. Pause after either snapshot mechanism has sampled the old
	// generation so this test observes behavior rather than requiring one call
	// path.
	if err == nil && l.prepares > 1 {
		l.pauseAfterSnapshot()
	}
	return index, err
}

func (l *uow5ShippedReloadProbe) commitMappings(index map[model.SICCode]*model.SICMapping) {
	l.inner.commitMappings(index)
}

func (l *uow5BlockingLookup) pauseAfterSnapshot() {
	l.firstOnce.Do(func() {
		close(l.started)
		<-l.release
	})
}

func (l *uow5BlockingLookup) LookupCategory(model.SICCode) (int, bool) {
	l.pauseAfterSnapshot()
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.category, true
}

func (l *uow5BlockingLookup) ReloadMappings() error { return nil }

func (l *uow5BlockingLookup) prepareMappings() (map[model.SICCode]*model.SICMapping, error) {
	l.mu.RLock()
	category := l.category
	l.mu.RUnlock()
	index := map[model.SICCode]*model.SICMapping{
		"5812": model.NewSICMapping("5812", "", "", &category),
	}
	l.prepares++
	if l.prepares > 1 {
		l.pauseAfterSnapshot()
	}
	return index, nil
}

func (l *uow5BlockingLookup) commitMappings(index map[model.SICCode]*model.SICMapping) {
	l.mu.Lock()
	l.category = *index["5812"].CategoryID
	l.mu.Unlock()
}

func TestReviewU5OnePassSeesOneRuleGeneration(t *testing.T) {
	db, txnRepo, patternRepo, _, _, _, accountID := newUOW3ServiceHarness(t)
	oldCategory := createUOW3Category(t, db, "Old generation")
	newCategory := createUOW3Category(t, db, "New generation")
	lookup := &uow5BlockingLookup{category: oldCategory, started: make(chan struct{}), release: make(chan struct{})}
	categorizer := NewCategorizerWithSIC(patternRepo, txnRepo, lookup)
	first := createUOW3Txn(t, txnRepo, accountID, "generation-1", "MERCHANT", "5812", nil, model.CategorySourceNone)
	second := createUOW3Txn(t, txnRepo, accountID, "generation-2", "MERCHANT", "5812", nil, model.CategorySourceNone)

	done := make(chan error, 1)
	go func() {
		_, err := categorizer.Reexamine()
		done <- err
	}()
	<-lookup.started

	writerAttempted := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		close(writerAttempted)
		categorizer.mu.Lock()
		lookup.mu.Lock()
		lookup.category = newCategory
		lookup.mu.Unlock()
		categorizer.mu.Unlock()
		close(writerDone)
	}()
	<-writerAttempted
	// Let the writer queue behind evaluate's read lock. sync.RWMutex then holds
	// subsequent readers until that writer publishes the next generation.
	for i := 0; i < 20; i++ {
		runtime.Gosched()
	}
	close(lookup.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	<-writerDone

	storedFirst, err := txnRepo.GetByID(first.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	storedSecond, err := txnRepo.GetByID(second.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if storedFirst.CategoryID == nil || storedSecond.CategoryID == nil || *storedFirst.CategoryID != *storedSecond.CategoryID {
		t.Fatalf("one pass mixed rule generations: first=%v second=%v; valid outcomes are both %d or both %d",
			storedFirst.CategoryID, storedSecond.CategoryID, oldCategory, newCategory)
	}
}

func TestReviewU5ShippedMappingReloadCannotSplitOnePass(t *testing.T) {
	db, txnRepo, patternRepo, sicRepo, _, _, accountID := newUOW3ServiceHarness(t)
	oldCategory := createUOW3Category(t, db, "Shipped old generation")
	newCategory := createUOW3Category(t, db, "Shipped new generation")
	mappings := []*model.SICMapping{
		model.NewSICMapping("5812", "", "", &oldCategory),
		model.NewSICMapping("5411", "", "", &oldCategory),
	}
	for _, mapping := range mappings {
		if err := sicRepo.Create(mapping); err != nil {
			t.Fatal(err)
		}
	}

	shipped := NewSICMappingCategorizer(sicRepo, txnRepo)
	probe := &uow5ShippedReloadProbe{
		inner: shipped, observed: make(chan struct{}), release: make(chan struct{}),
	}
	categorizer := NewCategorizerWithSIC(patternRepo, txnRepo, probe)
	// Two distinct codes are essential: a cache snapshot must be atomic across
	// the complete mapping generation, not merely stable for repeated uses of
	// one code.
	first := createUOW3Txn(t, txnRepo, accountID, "shipped-generation-1", "MERCHANT", "5812", nil, model.CategorySourceNone)
	second := createUOW3Txn(t, txnRepo, accountID, "shipped-generation-2", "MERCHANT", "5411", nil, model.CategorySourceNone)

	done := make(chan error, 1)
	go func() {
		_, err := categorizer.Reexamine()
		done <- err
	}()
	<-probe.observed

	for _, mapping := range mappings {
		mapping.CategoryID = &newCategory
		if err := sicRepo.Update(mapping); err != nil {
			t.Fatal(err)
		}
	}
	// This is the exact first operation performed by SICMappingService after a
	// committed mapping mutation. It must not publish a new cache generation
	// while the in-flight pass is pinned to the old one.
	if err := shipped.ReloadMappings(); err != nil {
		t.Fatal(err)
	}
	close(probe.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	storedFirst, err := txnRepo.GetByID(first.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	storedSecond, err := txnRepo.GetByID(second.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if storedFirst.CategoryID == nil || storedSecond.CategoryID == nil || *storedFirst.CategoryID != *storedSecond.CategoryID {
		t.Fatalf("shipped reload split one pass: first=%v second=%v; valid outcomes are both %d or both %d",
			storedFirst.CategoryID, storedSecond.CategoryID, oldCategory, newCategory)
	}
}

func TestReviewU5FailedPassSnapshotNeverFallsBackToSplitGeneration(t *testing.T) {
	db, txnRepo, patternRepo, _, _, _, accountID := newUOW3ServiceHarness(t)
	oldCategory := createUOW3Category(t, db, "Failed snapshot old generation")
	newCategory := createUOW3Category(t, db, "Failed snapshot new generation")
	lookup := &uow5FailingPassSnapshotLookup{
		current:  oldCategory,
		next:     newCategory,
		observed: make(chan struct{}),
		release:  make(chan struct{}),
	}
	categorizer := NewCategorizerWithSIC(patternRepo, txnRepo, lookup)
	first := createUOW3Txn(t, txnRepo, accountID, "failed-snapshot-1", "MERCHANT", "5812", nil, model.CategorySourceNone)
	second := createUOW3Txn(t, txnRepo, accountID, "failed-snapshot-2", "MERCHANT", "5411", nil, model.CategorySourceNone)

	type outcome struct {
		result *RecategorizeResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := categorizer.Reexamine()
		done <- outcome{result: result, err: err}
	}()
	var got outcome
	select {
	case got = <-done:
		// Returning the staging error, or obtaining a complete snapshot through
		// another atomic mechanism, is safe and must not make this test hang.
	case <-lookup.observed:
		if err := lookup.ReloadMappings(); err != nil {
			t.Fatal(err)
		}
		close(lookup.release)
		got = <-done
	case <-time.After(2 * time.Second):
		t.Fatal("re-examination neither returned nor began a fallback lookup")
	}

	storedFirst, err := txnRepo.GetByID(first.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	storedSecond, err := txnRepo.GetByID(second.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if got.err != nil {
		if storedFirst.CategoryID != nil || storedSecond.CategoryID != nil {
			t.Fatalf("failed snapshot returned %v after changing rows: first=%v second=%v", got.err, storedFirst.CategoryID, storedSecond.CategoryID)
		}
		return
	}
	if storedFirst.CategoryID == nil || storedSecond.CategoryID == nil || *storedFirst.CategoryID != *storedSecond.CategoryID {
		t.Fatalf("failed atomic snapshot fell back to a mixed generation: first=%v second=%v; valid outcomes are both %d or both %d",
			storedFirst.CategoryID, storedSecond.CategoryID, oldCategory, newCategory)
	}
}

func TestReviewU5ConcurrentManualChoiceCannotBeOverwritten(t *testing.T) {
	db, txnRepo, patternRepo, _, _, _, accountID := newUOW3ServiceHarness(t)
	ruleCategory := createUOW3Category(t, db, "Rule")
	manualCategory := createUOW3Category(t, db, "Manual")
	lookup := &uow5BlockingLookup{category: ruleCategory, started: make(chan struct{}), release: make(chan struct{})}
	categorizer := NewCategorizerWithSIC(patternRepo, txnRepo, lookup)
	txn := createUOW3Txn(t, txnRepo, accountID, "manual-race", "MERCHANT", "5812", nil, model.CategorySourceNone)

	type reexamineOutcome struct {
		result *RecategorizeResult
		err    error
	}
	done := make(chan reexamineOutcome, 1)
	go func() {
		result, err := categorizer.Reexamine()
		done <- reexamineOutcome{result: result, err: err}
	}()
	<-lookup.started
	if err := txnRepo.UpdateCategory(txn.TransactionID, &manualCategory, model.CategorySourceManual); err != nil {
		t.Fatal(err)
	}
	close(lookup.release)
	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if outcome.result.MovedCount != 0 || outcome.result.UncategorizedCount != 0 {
			t.Fatalf("concurrent manual choice was counted as changed: %+v", outcome.result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("re-examination did not finish")
	}

	stored, err := txnRepo.GetByID(txn.TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CategoryID == nil || *stored.CategoryID != manualCategory || stored.CategorySource != model.CategorySourceManual {
		t.Fatalf("concurrent manual choice was overwritten: stored=%+v", stored)
	}
}
