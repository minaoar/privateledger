# Production Summary — UOW-5 Rule-Sourced Recategorization

Production generation completed 2026-09-07 against the approved code generation plan. **Production code
only.** No `_test.go` file, fixture, benchmark or test dependency was created or modified.

Six production files plus two templates. No schema change, no dependency change.

## `internal/service/categorizer.go`

| Element | Change |
|---|---|
| `evaluate` | **New, unexported.** Patterns in order, then SIC. **No guards at all** |
| `decide` | Now a guard layer — manual, then existing-category — over `evaluate`. Import unchanged |
| `decideOnReexamination` | **New.** Manual guard only, then `evaluate` |
| `Reexamine` | **New.** The single entry point |
| `wouldChangeCategory` | **New.** Shared by the write decision and the manual count, so "would have moved" means the same in the report as in the writes beside it |
| `RecategorizeAll`, `RecategorizeByCategory` | **Deleted** |
| `RecategorizeResult` | Gains `moved_count`, `uncategorized_count`, `manual_protected_count`. Existing four unchanged, no `omitempty` |

`evaluate` carries a comment stating that it has no manual guard, that every caller must guard, and that
the one unguarded caller has no write reachable from it — recorded as a property to preserve, not an
accident.

## `internal/repository/transaction_repo.go`

- **`BulkClearCategory`** — one `json_each` statement setting `category_id = NULL, category_source = 0`.
  Built exactly like `BulkUpdateCategory`, which is unchanged and not widened to `*int`.
- **`GetAllForReexamination`** — every transaction including manual ones, column set mirroring
  `GetUncategorized` minus its `WHERE`, ordered by `transaction_id` so batching and counts are
  reproducible.
- `GetUncategorized` and `GetUncategorizedBySICCodes` left in place, unmodified.

## `internal/service/sic_mapping_service.go`

- `SICRecategorizationCollaborator.RecategorizeBySICCodes([]SICCode) (int, error)` → `Reexamine()
  (RecategorizationCounts, error)`.
- `runPostCommit` takes `rulesChanged bool` instead of an affected-code set.
- `diffSICMappings` returns `rulesChanged bool` instead of `affected []SICCode`.

**One behavioural change beyond a rename.** The old code handed off nothing when a mapping's category
was *cleared*, reasoning that an empty mapping assigns nothing. Under FR15 it very much matters:
transactions that mapping had categorized must be re-examined, and most become uncategorized. The
`HasCategory()` condition was therefore dropped from the update path, and deletion now passes `true`.

The surviving exclusion is BR-U5-04: a description-only edit changes no categorization, so it still
triggers nothing.

## `internal/service/sic_categorizer.go`

`RecategorizeBySICCodes` — which loaded transactions by affected code and applied mappings itself —
is replaced by a thin `Reexamine` that forwards to `Categorizer.Reexamine`. A new `attachReexaminer`
mirrors `attachDecider`, wired at the same place.

## `internal/handler/category_handler.go`

Pattern create, pattern add and pattern delete now call `Reexamine`. Category delete calls it **after**
`categoryRepo.Delete` so the cascade has completed (BR-U5-09), and returns the counts.

**`UpdateCategory` deliberately does not trigger.** It changes only name, type, colour and icon;
patterns and mappings key on `category_id`, which does not change. The code generation plan listed it as
a trigger and that was wrong — corrected in the plan with the reason.

## Templates

`categories.html` gains `describeReexamination`, reporting moved, uncategorized and manual-protected.
`sic_mappings.html` now shows `recategorized_rows`, withheld in UOW-2 only because the collaborator was
a no-op. **`import.html` is unchanged.**

## Verification

`gofmt` clean, `go build ./...` clean.

**Two packages do not compile their tests**, as the plan's decision A accepted:

| Package | Reason |
|---|---|
| `internal/service` | `reviewCollaborator` in `sic_management_property_review_test.go` (and `sic_management_review_test.go`) implements the old collaborator signature |
| `internal/handler` | `reviewHandlerCollaborator` in `sic_mapping_review_test.go` implements the old collaborator signature |

`cmd/privateledger` was predicted to break and **does not** — it compiles and passes.

Packages that compile all pass: `internal/model`, `internal/parser`, `internal/database`,
`internal/repository`, `cmd/privateledger`.

### Behaviour verified by running the binary

Throwaway database, non-default port, five seeded transactions including one manual.

| Step | Result |
|---|---|
| Create mapping 5412 → Groceries | 3 uncategorized transactions categorized; manual untouched |
| **Repoint 5412 → Travel** | All 3 moved. This is the defect that created the unit |
| Create pattern `AIRLINE` → Dining | `AIRLINE TICKET` moved off its mapping category. **Patterns outrank mappings** (BR-U5-20) |
| Delete the mapping | Its two transactions became uncategorized; the pattern-matched one kept Dining |
| Recategorize twice | Both passes report moved 0, uncategorized 0. **Idempotent** (BR-U5-18) |
| Delete the Dining category | Cascade removed its pattern, then re-examination moved 2 transactions |
| Import an OFX with SIC 5412 | New transaction categorized; **an existing transaction deliberately pointed at the "wrong" category stayed there**, proving import does not re-examine (BR-U5-03) |

## Not Done

- No test authored. The independent provider owns all eight obligations in DP-U5-08.
- `NFR-U2-PERF-01`'s fixture still has no transactions; populating it is a reviewer-owned change.
- The manual-marker finding below was **not** fixed; it needs a product decision.
