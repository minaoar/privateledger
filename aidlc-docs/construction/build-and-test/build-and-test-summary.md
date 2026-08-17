# Build and Test Summary

Covers all units built on this project. Newest first.

---

# Unit: uncategorized-dashboard

**Branch**: `show-uncategorized-transactions` · **Date**: 2026-08-03

Detailed instructions for this unit live in `build-instructions.md`, `unit-test-instructions.md`, `integration-test-instructions.md`, and `performance-test-instructions.md`.

## Build Status

| | |
|---|---|
| Build tool | Go + Make (`modernc.org/sqlite`, no CGO) |
| `go build ./...` | **Success** |
| `go vet ./...` | **Clean** |
| `gofmt` | Clean |
| Artifact | `./privateledger` (~33 MB, embedded web assets) |

## Test Execution Summary

### Unit Tests

| | |
|---|---|
| Test file added | `internal/service/insights_uncategorized_test.go` |
| Test functions | 12 (25 cases with subtests) |
| Passed | **12** |
| Failed | **0** |
| Coverage, `internal/service` | **40.7%** (was 0.0%) |
| Coverage, all other packages | **0.0%** (unchanged) |
| Status | **Pass** |

This is the **first test file in the repository**. Before it, `make test` reported success while executing nothing.

Each test opens a real temporary SQLite database migrated with the production schema — repository SQL is exercised, not mocked.

**Mutation-verified.** Four defects were injected; each was caught by the expected test, and the source was restored byte-identical afterwards:

| Injected defect | Caught by |
|---|---|
| Previous period ignores uncategorized | `TestSummaryCards_PreviousAmountIncludesUncategorized` |
| Broader `Uncategorized` filter for row sums | `TestBreakdown_NoDoubleCounting` |
| Investment gains an uncategorized row | 3 tests |
| Pie chart counts credits as expenses | `TestMonthlySummary_PieChartUncategorizedSlice` |

### Integration Tests

Five **manual** end-to-end scenarios, run against a copy of the production database on an isolated port. Not automated.

| Scenario | Result |
|---|---|
| Dashboard renders (catches template panics) | Pass — HTTP 200 |
| Uncategorized rows + `data-testid` reach the HTML | Pass |
| No double-counting across all 3 tables | Pass |
| Figures match raw SQL | Pass — expense 698.27, income 936.21 |
| Uncategorized links filter correctly | Pass — 56 txns, 0 wrong-category, 0 out-of-range |

### Performance Tests

Not applicable in the load/stress sense — local, single-user application. Latency measured instead: **11 ms** median dashboard render (179 transactions, 16 categories). This unit adds 12 queries to an existing ~100-query N+1 pattern, and none to the summary cards or pie chart.

### Additional Tests

| | |
|---|---|
| Contract tests | N/A — no inter-service APIs (single binary) |
| Security tests | N/A — all extensions opted out at Requirements Analysis; no auth, network, or input-handling changes |
| E2E tests | Covered by the manual integration scenarios above |

## Overall Status

| | |
|---|---|
| Build | **Success** |
| All tests | **Pass** |
| Ready for Operations | **Yes**, with the gaps below understood |

## Known Gaps

These are real and unclosed. None blocks the feature, but none should be assumed covered.

1. **Templates have no automated tests.** `dashboard.html` changes were verified by rendering the real page and asserting on HTML — a manual check that lives outside the repo. A malformed template is not a build error; it panics at first render. Closing this needs a handler-level test that parses each template with the production `FuncMap` and executes it against a synthetic `DashboardStats`.

2. ~~**Visual appearance unverified.**~~ **CLOSED 2026-08-03.** The user reviewed the running dashboard. The pie slice color was iterated during that review — `#8b5cf6` (violet, rejected) → `#374151` (too dark) → **`#4b5563`** (accepted). Muted italic row styling accepted as-is.

3. **Coverage is 40.7% in one package and 0% everywhere else.** `parser`, `handler`, `repository`, and the import/categorizer service logic remain untested.

4. **Custom `start_of_month` untested for this feature.** All verification ran with `start_of_month: 1` (calendar months). Pay-cycle boundaries (e.g. `19`) are unexercised for the uncategorized rows.

5. **Integration scenarios are manual.** Documented and reproducible, but they will not run in CI.

6. **Large-dataset performance unmeasured.** Deliberately deferred — not a current concern at this data volume.

7. **Three definitions of "uncategorized" remain in the codebase.** This unit pinned one locally (`category_id IS NULL`) and documented why; reconciling `IsUncategorized()`, the `Uncategorized` filter, and `GetUncategorized()` codebase-wide is a separate follow-up.

## Follow-Up Candidates

| Item | Rationale |
|---|---|
| Template rendering tests | Only way to catch template panics before a user hits them |
| Reconcile "uncategorized" definitions | Three variants invite future double-counting bugs |
| Replace breakdown N+1 with `GROUP BY` | Structural fix if the dashboard ever slows |

## Next Steps

Ready to proceed to the Operations stage. Recommended first: run `make run` and look at the dashboard, which closes gap 2 in under a minute.

---

# Unit: import-revert

Build and test instructions retained from the previous workflow.

## Build

```bash
# Verify compilation (already confirmed clean)
go build ./...

# Build binary
make build

# Run from source (development)
go run ./cmd/privateledger
```

## Unit Tests

```bash
# Run all tests
make test

# Run service-layer tests specifically
go test -v ./internal/service -run TestCategorizer
```

> Note: as of the uncategorized-dashboard unit, the only test file in the repository is
> `internal/service/insights_uncategorized_test.go`. The `TestCategorizer` target above
> matches nothing — no categorizer tests were ever written.

## Manual Integration Test Checklist

### Preconditions
- App running (`make run` or `go run ./cmd/privateledger`)
- At least one account created
- One OFX/QFX file available

### Test 1: Import + Revert via Import History tab
1. Navigate to `/import`
2. Import an OFX file → note the `imported_transactions` count in the success banner
3. Click "Import History" tab
4. Verify the batch row shows the correct file name, imported count, and a revert button (↩ icon)
5. Click the revert button (↩)
6. Verify confirmation modal appears with:
   - Correct file name
   - Correct transaction count
   - Manual-categorization warning (if applicable)
7. Click "Revert Import"
8. Verify the batch row disappears from the history list
9. Navigate to `/transactions` → verify the transactions from that import are gone

### Test 2: Re-import same file after revert
1. After Test 1, re-import the same OFX file
2. Verify `imported_transactions` matches original count (not 0 / all-duplicates)
3. Confirms the revert truly removed the transactions

### Test 3: Revert via Transactions page batch filter
1. Import an OFX file, note the `batch_id` from the Import History tab (or the View link URL)
2. Navigate to `/transactions?batch_id=<id>`
3. Verify the blue info banner appears: "Viewing import: filename.ofx  [↩ Revert this import]"
4. Click "Revert this import"
5. Verify confirmation modal, then confirm
6. Verify redirect to `/transactions` (no batch filter) and transactions are gone

### Test 4: Re-import deduplication still works
1. Import a file
2. Import the **same file** a second time
3. Verify second import shows `imported_count = 0`, `duplicate_count = N`
4. Verify only one batch record exists per import attempt in Import History

### Test 5: Manually categorized warning
1. Import an OFX file
2. Manually categorize one or more transactions (category_source = 2)
3. Navigate to Import History and click Revert on that batch
4. Verify the confirmation modal shows the warning: "X transactions were manually categorized and will also be deleted"
5. Confirm revert and verify all transactions (including manually categorized) are deleted

### Test 6: Delete non-existent batch
```bash
curl -X DELETE http://localhost:8844/api/import/history/99999
# Expected: 404 {"error": "Import batch not found"}
```

### Test 7: API response shape
```bash
# After a successful revert:
curl -X DELETE http://localhost:8844/api/import/history/<id>
# Expected 200:
# {"batch_id": N, "file_name": "example.ofx", "deleted_transactions": N}
```
