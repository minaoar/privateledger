# Logical Components — UOW-5 Rule-Sourced Recategorization

Generated 2026-09-07 alongside `nfr-design-patterns.md`, awaiting the same approval.

UOW-5 introduces **no new component**. It changes five existing ones and one interface. Listed with the
specific responsibility each takes on, so the code-generation plan has an exact surface.

## Changed Components

### `internal/service/categorizer.go`

| Element | Change |
|---|---|
| `evaluate` | **New, unexported.** Pure matching: patterns in order, then SIC if none matched. No guards |
| `decide` | Becomes `decideCategory`'s guard layer — manual, then existing category — over `evaluate` |
| `decideOnReexamination` | **New, unexported.** Manual guard only, then `evaluate` |
| `Reexamine` | **New.** The single entry point. Reloads rules, reads every transaction, writes non-manual outcomes, returns the three counts |
| `RecategorizeAll` | Collapses into `Reexamine` |
| `RecategorizeByCategory` | Collapses into `Reexamine`. Its category argument has been inert since UOW-3 — it reads all uncategorized transactions and ignores it |
| `RecategorizeResult` | Gains three fields: moved, uncategorized, manual-protected. The existing four are unchanged |
| `ClearCategory` | **Unchanged.** Still used by category deletion; not reused by the pass |

### `internal/repository/transaction_repo.go`

| Element | Change |
|---|---|
| `BulkClearCategory` | **New.** Clears a set of IDs in one `json_each` statement. Mirrors `BulkUpdateCategory` |
| `BulkUpdateCategory` | **Unchanged.** Not widened to `*int` |
| A whole-table read | Either a new method or an existing one widened, returning every transaction including manual ones |
| `GetUncategorizedBySICCodes` | Retained but no longer called by the pass. Its scoping use retires with the affected set |

### `internal/service/sic_mapping_service.go`

| Element | Change |
|---|---|
| `SICRecategorizationCollaborator` | `RecategorizeBySICCodes([]SICCode) (int, error)` becomes a no-scope re-examination call returning the three counts, per TD-U5-01 |
| `noopSICRecategorizationCollaborator` | Signature follows the interface |
| The call site at `:769` | Passes no affected set; the affected-set computation feeding it retires with BR-U5-13 |

### `internal/service/sic_categorizer.go`

Becomes a thin adapter onto `Categorizer.Reexamine`. It keeps `ReloadMappings` and its mapping cache;
what it loses is its own scoped recategorization path.

### `internal/handler/category_handler.go`

Pattern create, pattern delete, category delete and category update call the one entry point instead of
`RecategorizeByCategory` and bare `LoadRules`. Category deletion calls it **after** the cascade
completes, per BR-U5-09.

### `cmd/privateledger/web/templates/`

`categories.html` and `sic_mappings.html` render the three new counts. **`import.html` is unchanged** —
import does not re-examine, so its counts are zero and it reads what it reads today.

## Unchanged Components — Stated So They Are Not Touched

| Component | Why it appears here |
|---|---|
| `internal/database/schema.sql` | No table, column, index or migration. `category_source` keeps three values |
| `internal/model/` | No new type. `RecategorizeResult` lives in the service package |
| Import service | Import is not a rule change. Its path and its result shape are untouched |
| The admission gate, backup and merge | Untouched. Only what the collaborator does inside the gate changes |
| Diagnostics and `model.DiagValue` | Untouched. UOW-4's bounds still govern any name echoed |

## Dependency and Concurrency Posture

No dependency is added, removed or upgraded. Standard library only.

No new concurrency. `LoadRules` keeps publishing both caches under one write lock, and the pass reads
under `RLock` held across both rule sources so one pass sees one generation. What changes is the
**duration** of the gate hold, from roughly 204 ms to roughly 827 ms worst case — measured, and the
reason NFR-U5-CON-01 could keep UOW-3's no-deadline decision.

## Traceability

| Component change | Requirement | Pattern |
|---|---|---|
| `evaluate`, `decideOnReexamination` | BR-U5-06, BR-U5-14, REL-02 | DP-U5-01 |
| `BulkClearCategory` | BR-U5-08, PERF-01, SCALE-02 | DP-U5-02 |
| `RecategorizeResult` fields | FR16, BR-U5-15 through BR-U5-17 | DP-U5-03, DP-U5-04 |
| `Reexamine`, collaborator signature | BR-U5-12, BR-U5-13, TD-U5-01 | DP-U5-05 |
| Whole-table read | BR-U5-02 as amended, SCALE-01 | DP-U5-06 |
| Handler call sites, deletion ordering | BR-U5-01, BR-U5-09, CON-02 | DP-U5-07 |
| Template counts | FR16, US-16 | DP-U5-03 |
