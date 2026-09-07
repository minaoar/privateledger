# Code Generation Plan — UOW-5 Rule-Sourced Recategorization

## Authority and Scope

This plan is the single source of truth for UOW-5 Code Generation. Production executes these steps in
order and may not add behaviour outside the approved requirements, functional design and NFR design.

UOW-5 implements **US-15** and **US-16** under **FR15** and **FR16**. It adds no table, column, index,
dependency, page or route.

## Mandatory Ownership Boundary

### Production role (this session)

- May modify the production Go files, templates and production documentation listed below.
- **Must not** create or modify `_test.go` files, fixtures, benchmarks, property tests, test-only
  dependencies, or the independent review artifact.
- **Must not** modify the independent role's existing tests. A test failing — or failing to compile —
  against deliberately changed behaviour is reported as a handoff finding, never edited.

### Independent review/test role (separate provider session)

- Owns review, every test, and
  `aidlc-docs/construction/rule-sourced-recategorization/code-review/independent-review.md`.
- Must report **PASS** before UOW-5 Code Generation completes.

## The Problem This Plan Has to Face First

Unlike UOW-4, where no test referenced any changing symbol, **UOW-5's approved contract change will break
test compilation across three packages.**

Verified by grep against the current tree:

| Symbol | Test references | Files |
|---|---|---|
| `RecategorizeBySICCodes` | 8 | `sic_management_review_test.go` (fake), `sic_mapping_review_test.go` (fake), `uow3_categorization_review_test.go`, `uow3_categorization_property_review_test.go`, `uow3_performance_review_test.go` |
| `RecategorizeAll` | 8 | `uow3_performance_review_test.go`, `uow3_categorization_review_test.go`, `uow3_browser_fixture_test.go` |
| `RecategorizeByCategory` | 3 | `uow3_categorization_review_test.go` |
| `GetUncategorizedBySICCodes` | 16 | `sic_repo_test.go`, `uow3_set_passing_review_test.go` |

Two of those are **fakes implementing `SICRecategorizationCollaborator`**. TD-U5-01 changes that
interface, which is approved. A fake implementing the old signature stops satisfying the interface, so
**compilation breakage is unavoidable** — there is no production-side way to avoid it without abandoning
an approved decision.

Packages that will not compile after this change: `internal/service`, `internal/handler`,
`cmd/privateledger`.

### The consequence, stated plainly

**Production cannot run `go test` to verify its own work.** That is a real loss, not a formality.

### Decision required before Step 1

- A. **(Recommended)** Accept the breakage. Change the contract as approved, leave every test untouched,
  list the affected files and lines precisely in the handoff, and verify production behaviour by
  building and running the binary against a throwaway database — the method that found the real defects
  in UOW-4.
  *The reviewer owns tests and an approved contract change means their fakes must change; that is the
  workflow operating normally, not a failure. The alternative below is worse than the problem.*
- B. Ship compatibility shims: keep `RecategorizeBySICCodes`, `RecategorizeAll` and
  `RecategorizeByCategory` as delegating wrappers so the suite still compiles.
  *Keeps the suite building and behavioural failures informative. But it means shipping methods that
  accept arguments they ignore — the exact smell FD-FQ1 rejected for this very interface — and someone
  has to remember to remove them. A deliberately misleading API is a poor price for a green build.*
- C. Change the contract but leave the old interface in place alongside the new one.
  *Two contracts for one operation, which BR-U5-12 exists to prevent.*

[Answer]:A

**Answered A on 2026-09-07.** Compilation breakage is accepted; no test is edited and no shim is shipped.

## Verified Starting State

| Observation | Location |
|---|---|
| `RecategorizeAll` groups by category and claims counts only after each write commits | `categorizer.go:241-273` |
| `RecategorizeByCategory` takes a category ID and ignores it, reading all uncategorized | `categorizer.go:284-293` |
| `BulkUpdateCategory` takes a non-nullable `int`; cannot write NULL | `transaction_repo.go:462` |
| `ClearCategory` loops one round trip per transaction | `categorizer.go:341-347` |
| The collaborator interface and its no-op | `sic_mapping_service.go:48-71` |
| Its single call site | `sic_mapping_service.go:769` |
| UI reads four result fields | `categories.html:538-547`, `import.html:188-199` |

## Expected Production Files

**Modify**: `internal/service/categorizer.go`, `internal/service/sic_categorizer.go`,
`internal/service/sic_mapping_service.go`, `internal/repository/transaction_repo.go`,
`internal/handler/category_handler.go`, `cmd/privateledger/main.go` (wiring only if the constructor
shape changes), `cmd/privateledger/web/templates/categories.html`,
`cmd/privateledger/web/templates/sic_mappings.html`.

**Create**: `aidlc-docs/construction/rule-sourced-recategorization/code/production-summary.md`,
`aidlc-docs/construction/rule-sourced-recategorization/code/independent-review-handoff.md`.

**Explicitly not modified**: `internal/database/schema.sql`, `internal/model/`, the import service,
`cmd/privateledger/web/templates/import.html`, `go.mod`, `go.sum`.

---

## Step 1 — `evaluate`, the guard-free matcher

File: `internal/service/categorizer.go`

- [x] Extract `evaluate(txn) (int, categorySource)` — patterns in order, then SIC if none matched. **No
      guards.** It holds `c.mu.RLock()` across both rule sources so one decision sees one generation.
- [x] Document at the declaration that it deliberately has no manual guard, that every caller except the
      manual-count path must guard, and that the manual-count path has no write available to it.
- [x] `decideCategory` becomes: manual guard, existing-category guard, then `evaluate`. Import's
      behaviour is unchanged.
- [x] Add `decideOnReexamination(txn)`: manual guard only, then `evaluate`.

## Step 2 — `BulkClearCategory`

File: `internal/repository/transaction_repo.go`

- [x] Add `BulkClearCategory(transactionIDs []int) error` — one statement,
      `SET category_id = NULL, category_source = 0 WHERE transaction_id IN (SELECT value FROM json_each(?))`.
- [x] Encode IDs as JSON numbers, matching `BulkUpdateCategory` exactly. One bound parameter.
- [x] Return early on an empty slice, as `BulkUpdateCategory` does.
- [x] Leave `BulkUpdateCategory` unchanged. Do not widen it to `*int`.

## Step 3 — The whole-table read

File: `internal/repository/transaction_repo.go`

- [x] Add a read returning **every** transaction, including manual ones, with the fields `evaluate`
      needs. Per BR-U5-02 as amended: read scope is the whole table, write scope is non-manual.
- [x] Leave `GetUncategorized` and `GetUncategorizedBySICCodes` in place and unmodified.

## Step 4 — `Reexamine`

File: `internal/service/categorizer.go`

- [x] Add `Reexamine() (*RecategorizeResult, error)`: reload rules, read every transaction, and in **one
      traversal** accumulate assignments by target category, the clear set, and the manual count.
- [x] Manual branch: call `evaluate`, increment the manual-protected count when the result differs from
      the stored category, then `continue`. **No write is reachable from this branch.**
- [x] Non-manual branch: `decideOnReexamination`. Unchanged outcome writes nothing and counts nothing
      (BR-U5-17). A different category joins the assign batch; no match joins the clear set.
- [x] Issue batched writes, then claim counts — preserving the existing after-commit discipline. The
      manual count is claimed immediately, since it corresponds to no write.
- [x] Delete `RecategorizeAll` and `RecategorizeByCategory`, whose callers move to `Reexamine`.

## Step 5 — Result shape

File: `internal/service/categorizer.go`

- [x] Add three fields to `RecategorizeResult`: moved, uncategorized, manual-protected, with JSON tags.
- [x] Keep the existing four unchanged, including `categorized_count`'s current meaning.
- [x] Zero values are reported, never omitted — no `omitempty` on the new fields.

## Step 6 — The collaborator contract

Files: `internal/service/sic_mapping_service.go`, `internal/service/sic_categorizer.go`

- [x] Replace `RecategorizeBySICCodes([]model.SICCode) (int, error)` with a no-scope re-examination call
      returning the three counts.
- [x] Update the no-op implementation to match.
- [x] Update the call site at `:769`; the affected-set computation feeding it retires with BR-U5-13.
- [x] `SICMappingCategorizer` becomes a thin adapter onto `Categorizer.Reexamine`, keeping
      `ReloadMappings` and its mapping cache.

## Step 7 — Trigger call sites

File: `internal/handler/category_handler.go`

- [x] Pattern create, pattern add, pattern delete and category delete call `Reexamine`.
      **Step corrected during generation:** this originally listed *category update* as a trigger. It is
      not one. `UpdateCategory` changes only name, type, colour and icon; patterns and mappings key on
      `category_id`, which does not change, so a rename cannot alter any categorization. Triggering a
      full pass there would be a provable no-op, the same reason BR-U5-04 excludes a description-only
      mapping edit.
- [x] **Category deletion calls it after the cascade completes** (BR-U5-09), so re-examination sees the
      post-deletion rule set.
- [x] A description-only mapping edit still triggers nothing (BR-U5-04).
- [x] Import is untouched (BR-U5-03).

## Step 8 — Templates

- [x] `categories.html` and `sic_mappings.html` render the three new counts.
- [x] Escape through `textContent`, never assembled HTML.
- [x] Any new page-level JavaScript function must not share a name with an `app.js` global — defect
      U2-F09, where `layout.html` loads `app.js` after page content and silently overwrote a page
      function.
- [x] **`import.html` is not modified.**

## Step 9 — Production documentation

- [x] `code/production-summary.md`: what changed per file, with the requirement each satisfies, and the
      behaviour verified by running the binary.
- [x] `code/independent-review-handoff.md` containing at minimum:
      - the design-decisions record across all four UOW-5 stages, including R1a A and FD-FQ1 A;
      - **the exact list of test files and lines that no longer compile**, with the reason each does;
      - the `evaluate` structure and the explicit statement that it has no manual guard, why that is
        safe, and that it is a structural argument rather than a guarantee;
      - the amendments to BR-U5-02, NFR-U2-PERF-01, BR-U3-03, BR-U2-29 through BR-U2-31, NFR-U3-REL-01;
      - that `NFR-U2-PERF-01`'s fixture must gain 20,000 transactions before its PASS means anything;
      - the eight verification obligations of DP-U5-08.

## Step 10 — Verification by production

- [x] `gofmt`, `go vet`, `go build ./...` — production code must build.
- [x] `go test -count=1 ./internal/model/ ./internal/parser/ ./internal/database/ ./internal/repository/`
      — the packages that still compile must pass.
- [x] Record precisely which packages fail to compile and why. **Do not edit a test to fix it.**
      Result: two packages, not the three predicted — `internal/service` and `internal/handler`, both
      because reviewer fakes implement the old collaborator signature. `cmd/privateledger` compiles and
      passes.
- [x] Verify behaviour by building the binary and exercising it against a throwaway database on a
      non-default port: each trigger, the uncategorized outcome, manual protection, the three counts,
      idempotence, and that import does not re-examine.

## Step 11 — Handoff

- [x] Stage explicit paths only — never `git add -A` or `git add .` — and verify the staged set.
- [x] Commit and push.
- [x] Hand off to the independent provider session. **Production does not close this gate.**

---

## Out of Scope

- Weakening manual protection.
- Recording which rule categorized a transaction.
- Candidate findings C4-01, C4-02, C4-03; deferred findings F-04, F-05.
- New dependencies, schema changes, and any change to the import path or `import.html`.
