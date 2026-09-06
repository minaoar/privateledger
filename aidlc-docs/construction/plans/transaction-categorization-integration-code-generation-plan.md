# Code Generation Plan — UOW-3 Transaction Categorization Integration

## Authority and Scope

This plan is the single source of truth for UOW-3 Code Generation. Production generation executes these
steps in order and may not add behavior outside the approved requirements, functional design, and NFR
design.

UOW-3 is the final unit. It implements **US-02** (text patterns before SIC), **US-03** (preserve manual
categorization), **US-06** (SIC in transaction modals), **US-12** (create mappings from the modals), and
**US-13** (layering, race-safe caches, tests).

It adds no table, column, index, dependency, page, or route beyond the one modal endpoint below.

## Mandatory Ownership Boundary

### Production role

- May modify production Go, templates, and production documentation listed here.
- Must not create or modify `_test.go` files, `testdata/`, fixtures, benchmarks, property tests,
  test-only dependencies, or the independent review artifact.
- Must not modify the independent role's existing UOW-1 and UOW-2 tests. A test failing against
  deliberately changed behavior is reported as a handoff finding, never edited.

### Independent review/test role

- Runs in a separate provider session after production generation.
- Owns review, all verification tests, and
  `aidlc-docs/construction/transaction-categorization-integration/code-review/independent-review.md`.
- Must report PASS before UOW-3 Code Generation completes.

## Verified Starting State

| Observation | Location |
|---|---|
| `RecategorizeAll` and `RecategorizeByCategory` re-implement pattern matching inline | `internal/service/categorizer.go:89-99`, `:116+` |
| `go h.categorizer.LoadPatterns()` — unsynchronized write plus stale-read ordering bug | `internal/handler/category_handler.go:342` |
| Two further synchronous `LoadPatterns()` call sites | `internal/handler/category_handler.go:152,301` |
| Import already routes through `Categorize` | `internal/service/import_service.go:105` |
| Both expanding query builders | `internal/repository/transaction_repo.go:346,450` |
| SQLite parameter ceiling measured at 32,764 usable IDs | verified against `modernc.org/sqlite v1.34.4` |
| `SICDescription` already joined; `GetUncategorizedBySICCodes` already exists | `transaction_repo.go:107,175,344` |
| UOW-2 wires the no-op collaborator and rejects nil | `cmd/privateledger/main.go`, `sic_mapping_service.go` |

## Expected Production Files

**Modify**: `internal/service/categorizer.go`, `internal/repository/transaction_repo.go`,
`internal/service/import_service.go` (only if the decision-function signature requires it),
`internal/handler/category_handler.go`, `internal/handler/transaction_handler.go`,
`cmd/privateledger/main.go`, `cmd/privateledger/web/templates/transactions.html`,
`cmd/privateledger/web/templates/categories.html`, `API_ROUTES.md`.

**Create**: `internal/service/sic_categorizer.go`,
`aidlc-docs/construction/transaction-categorization-integration/code/production-summary.md`,
`aidlc-docs/construction/transaction-categorization-integration/code/independent-review-handoff.md`.

No duplicate replacement files. No schema migration.

---

## Sequential Generation Steps

### Step 1 — Single-parameter JSON set passing in the repository

- [ ] Replace expanded placeholders in `GetUncategorizedBySICCodes` and `BulkUpdateCategory` with
      `IN (SELECT value FROM json_each(?))`, binding the set as one JSON parameter.
- [ ] Encode SIC codes as JSON **strings** — `sic_code` is a TEXT column and `json_each` over `[7011]`
      yields an INTEGER whose comparison is governed by type affinity. Encode transaction IDs as JSON
      numbers against the INTEGER column.
- [ ] Keep the existing empty-input short circuit in both methods; no SQL is issued for an empty set.
- [ ] Keep `ORDER BY date_posted DESC` in SQL. One statement preserves it natively; no Go-side sort.
- [ ] `BulkUpdateCategory` remains a single statement, atomic by construction; add no wrapping
      transaction.
- [ ] Preserve existing error wrapping, row iteration, and `rows.Err()` checks.
- [ ] Trace to NFRP-U3-03; NFR-U3-PERF-01, NFR-U3-REL-01.

### Step 2 — SIC mapping categorizer

- [ ] Create `internal/service/sic_categorizer.go` with constructor injection of the mapping and
      transaction repositories.
- [ ] Implement the mapping cache keyed by canonical SIC code, a lookup returning the mapped category or
      nothing, and a reload that rebuilds from SQLite.
- [ ] Implement scoped recategorization over a de-duplicated affected-code set using
      `GetUncategorizedBySICCodes`, returning the count actually assigned. An empty set does no work and
      returns zero.
- [ ] Implement UOW-2's `SICRecategorizationCollaborator` interface exactly as defined — no rename, no
      widening, no context parameter.
- [ ] Own no text-pattern logic and make no priority decision.
- [ ] Trace to US-02, US-03; BR-U3-09 through BR-U3-12; NFRP-U3-01, NFRP-U3-04.

### Step 3 — Decision function and guarded cache set

- [ ] Modify `internal/service/categorizer.go` to hold both caches behind one `sync.RWMutex`, with
      categorization taking a read lock.
- [ ] Add the single unexported decision function in the fixed order: manual stops, existing category
      stops, text patterns in existing order, then the SIC lookup, and an empty-category mapping assigns
      nothing. Report which source assigned the category.
- [ ] Accept the SIC lookup as a small injected interface with one method, so priority stays in one place
      and the independent role can substitute a fake without a database.
- [ ] Add one exported reload entry point refreshing both caches: build both replacement sets first, swap
      under a single write lock, and on any failure leave **both** untouched and return the error.
- [ ] Make `LoadPatterns` unexported.
- [ ] Trace to US-02, US-13; BR-U3-01 through BR-U3-07, BR-U3-16 through BR-U3-21; NFRP-U3-01/02.

### Step 4 — Recategorization passes and split counts

- [ ] Replace the inline matching in `RecategorizeAll` and `RecategorizeByCategory` with calls to the
      decision function, so SIC cannot apply on import while being skipped by an explicit
      recategorization.
- [ ] Reload rules before reading in every pass; a reload failure stops the pass before any read.
- [ ] Keep the single `GetUncategorized` read for the full pass; add no streaming or paging.
- [ ] Extend `RecategorizeResult` with `PatternCategorizedCount` and `SICCategorizedCount`, counted as
      the pass proceeds so they partition `CategorizedCount` by construction rather than being derived
      separately. Retain the existing fields.
- [ ] Claim counts only after the corresponding update commits.
- [ ] Trace to US-02, US-03; BR-U3-08, BR-U3-23/24; NFRP-U3-04/05.

### Step 5 — Import path

- [ ] Confirm `internal/service/import_service.go:105` continues to route through the decision function
      and that SIC resolves from the cache under a read lock.
- [ ] Add no per-transaction mapping query.
- [ ] Change import only if the decision-function signature requires it; make no unrelated edit.
- [ ] Trace to US-02; NFRP-U3-06; NFR-U3-PERF-02.

### Step 6 — Category handler

- [ ] Delete `go h.categorizer.LoadPatterns()` at `category_handler.go:342`; the reload becomes part of
      the request.
- [ ] Repoint the three reload call sites at the single exported entry point and handle its error rather
      than discarding it.
- [ ] Return the pattern/SIC split counts from the recategorize endpoint alongside the existing total.
- [ ] Trace to US-13; BR-U3-17/18/19/23; NFRP-U3-02/05.

### Step 7 — Modal mapping endpoint

- [ ] Add `POST /api/transactions/:id/sic-mapping` to `internal/handler/transaction_handler.go`,
      delegating to UOW-2's `SICMappingService` — never to the mapping repository.
- [ ] Require a selected category; reject a request without one.
- [ ] Return the created or updated mapping plus the recategorized count, reusing UOW-2's mutation result
      shape and its stable error codes.
- [ ] Create no text pattern on this path.
- [ ] Trace to US-12; BR-U3-29 through BR-U3-32; NFRP-U3-07.

### Step 8 — Transaction and category templates

- [ ] `transactions.html`: add the SIC context block to both modals, shown only when the transaction has
      a SIC code, with `Description` falling back to `Description_Detail` and the bare code rendered
      cleanly when neither exists.
- [ ] Change Category gains an opt-in, unchecked-by-default control to create a mapping for the code
      using the selected category, whose confirmation states that other uncategorized transactions with
      that code will also be categorized.
- [ ] Create Pattern gains a mutually exclusive choice between text pattern (default) and SIC mapping,
      with inapplicable fields disabled rather than hidden.
- [ ] `categories.html`: warn before "Recategorize All" that SIC mappings will now also be applied, and
      report pattern and SIC counts separately afterwards; a run that categorizes nothing says so plainly.
- [ ] **Check every new function name against the globals in `cmd/privateledger/web/static/js/app.js`
      before use.** `layout.html` loads `app.js` after page content, so a colliding name is silently
      overwritten at runtime while the markup still looks correct — defect U2-F09, where mapping deletion
      could not work at all.
- [ ] Escape all user-controlled text; disable submissions in flight and always restore; report unknown
      outcome on transport failure.
- [ ] Use the approved `data-testid` values; add no SIC column to the transactions table.
- [ ] Trace to US-06, US-12; BR-U3-22, BR-U3-25 through BR-U3-35; NFRP-U3-07.

### Step 9 — Wiring and documentation

- [ ] `main.go`: construct one SIC categorizer, inject it into `Categorizer` as the lookup, and pass the
      **same instance** to `SICMappingService` as the collaborator, replacing
      `NewNoopSICRecategorizationCollaborator()`. One instance serves both roles so the cache the lookup
      reads is the cache the collaborator reloads.
- [ ] Register the modal endpoint in the existing `api` group; keep all wiring in `main.go`.
- [ ] Load rules once at startup before serving.
- [ ] Update `API_ROUTES.md` with the modal endpoint and the extended recategorize response.
- [ ] Trace to US-13; NFR-U3-MAINT-01; logical-components wiring section.

### Step 10 — Production verification

- [ ] `gofmt` on changed files; `go build ./...`; `go vet ./...`; `git diff --check`.
- [ ] Run the existing suite as regression feedback only. Do not author, modify, weaken, or skip a test.
- [ ] Inspect the diff for a detached goroutine, an unlocked cache read, a lost reload error, expanded
      placeholders left behind, layer inversion, unescaped template output, and a page function name
      colliding with an `app.js` global.
- [ ] Fix production only within the preceding approved steps and rerun.

### Step 11 — Production documentation

- [ ] Create `production-summary.md` listing files, implemented contracts, commands and results, known
      limitations, and traceability to US-02/03/06/12/13.
- [ ] Record the set-passing change and its effect on the shared repository methods, which UOW-2 also
      calls.
- [ ] State that the production role authored no verification tests.

### Step 12 — Independent handoff

- [ ] Create `independent-review-handoff.md` with the approved artifact paths, the production revision,
      exact file scope, known limitations, and commands already run.
- [ ] Flag for verification: that the `sic_code` index is still chosen under a `json_each` subquery
      rather than a scan — measured timings are consistent with index use but the query plan was not
      confirmed.
- [ ] Flag that UOW-2's approved merge fixture produces 50,000 affected codes and previously never issued
      the scoped query, because the collaborator was a no-op; it now does.
- [ ] Note that Steps 1 and 6 change behavior shared with UOW-1 and UOW-2, so their regressions must be
      re-run.

### Step 13 — Independent review and verification gate

- [ ] In a separate provider session, invoke the independent review/test role with the handoff.
- [ ] Require the full priority matrix; both recategorize entry points routing through the decision
      function; scoped recategorization touching nothing outside the affected set; split counts summing
      to the total; all-or-nothing cache swap on partial reload failure; set passing at and beyond the
      former 32,764 ceiling for both builders; SIC codes as JSON strings matching a TEXT column; empty
      sets issuing no SQL; modal display and fallback; and modal-created mappings requiring a category
      and creating no text pattern.
- [ ] Require generated properties for the priority matrix and recategorization scoping, with shrinking
      and a recorded replay seed.
- [ ] Require race evidence over concurrent categorization and reload, run separately from performance.
- [ ] Require the 20,000-transaction "Recategorize All" benchmark within five seconds, and both import
      fixtures against the 10 % budget, on the recorded reference environment.
- [ ] Production findings return to the production role; the independent role owns test corrections.
- [ ] Gate closes only when the review artifact reports PASS.

## Story Completion Checklist

- [ ] US-02 — Text patterns before SIC; empty mapping assigns nothing; one decision function.
- [ ] US-03 — Manual and existing assignments preserved; scoped recategorization bounded.
- [ ] US-06 — SIC code and description fallback in both modals; no table column.
- [ ] US-12 — Modal-created mappings with required category, no text pattern, and scoped recategorization.
- [ ] US-13 — Layering, guarded caches, single reload entry point, properties, and full test run.

## Exit Criteria

- [ ] Every production step and story checkbox complete.
- [ ] Build, vet, format, and existing regressions pass.
- [ ] Production summary and handoff complete.
- [ ] Independent tests authored and run; properties, race, and benchmark evidence recorded.
- [ ] Independent review final status PASS with no blocking or high finding open.
- [ ] User explicitly approves completed UOW-3 Code Generation.

## Out of Scope

- Deferred independent findings F-04 and F-05.
- Any new dependency, schema change, page, or frontend framework.
- Reducing UOW-2's approved 100,000-code merge fixture, which remains a separate amendment.
