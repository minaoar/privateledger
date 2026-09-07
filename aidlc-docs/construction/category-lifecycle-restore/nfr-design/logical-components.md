# Logical Components — UOW-4 Category Lifecycle and Mapping-File Integrity

Generated 2026-09-07 alongside `nfr-design-patterns.md`, awaiting the same approval.

UOW-4 introduces **no new component**. It changes five existing ones and adds one type to an existing
package. Listed with the specific responsibility each takes on, so the code-generation plan has an exact
surface rather than a description.

## Changed Components

### `internal/model/sic_mapping.go`

| Element | Change |
|---|---|
| `DiagValue` | **New type.** Bounded, sanitized value; constructible only through `NewDiagValue` |
| `NewDiagValue(raw string) DiagValue` | **New.** 64-rune truncation with U+2026, `unicode.IsControl` replaced by U+FFFD, valid UTF-8 guaranteed |
| `AddErrorf(row int, field, code, format string, values ...DiagValue)` | **New.** The only interpolating entry point for text |
| `AddError` | Gains the 512-rune assembled-message backstop (DP-U4-03). Cap, truncation flag and append behaviour otherwise unchanged |
| `SICMappingImportError` | **Unchanged shape.** `RowNumber`, `Field`, `Code`, `Message` |
| `MaxSICMappingDiagnostics` | Unchanged at 50 |

Two new named constants for the 64-rune value bound and the 512-rune message bound. No configuration
surface.

The type lives here rather than in `internal/service` because both the row-validation sites and the
header sites must reach it, and because the bound is a property of the diagnostic, not of the service
that happens to produce it today.

### `internal/service/sic_mapping_service.go`

| Element | Change |
|---|---|
| `matchesSICMappingHeader` | Normalizes per DP-U4-01 and returns why it failed, not a bare boolean. Count checked before names |
| Header diagnostic site (line 274) | Emits a position-and-expected-name message or a count message. Interpolates integers only |
| `indexCategories` | Returns **three** indexes instead of two — exact, folded, by ID — from one pass |
| `resolveSICCategory` | Takes the ID index; emits the two specific `category_not_found` forms and the `category_ambiguous` form |
| `sicRowValidator.rejectf` | **New.** Wraps `AddErrorf`, sets `invalid = true` exactly as `reject` does |
| `sicRowValidator.reject` | Unchanged, retained for constant messages |
| Everything else | Unchanged. No mutation, ordering, gate, backup or merge behaviour is touched |

### `cmd/privateledger/main.go`

One attribute added to the existing per-row seed warning: the bounded `Message` (DP-U4-07). Record
count, seed diagnostic cap, summary record and omitted-diagnostics count all unchanged.

### `cmd/privateledger/web/templates/sic_mappings.html`

**No change.** `describeDiagnostics` already renders `e.message` per row and `setStatus` already assigns
`textContent` per line. Verified at `sic_mappings.html:187` and `:370` rather than assumed — the more
specific messages flow through the existing rendering unmodified.

Recorded because "the UI needs no change" is a claim worth having checked, and because the temptation
during code generation will be to touch this file anyway.

### `internal/handler/sic_mapping_handler.go`

**No change.** `sicMappingFailureMessage` handles service *errors*, a separate path from validation
diagnostics; the `category_not_found` case there is the single-mapping create/update failure, not the
CSV row diagnostic that shares its name. Recorded explicitly because the shared spelling is an easy
mis-edit.

## Unchanged Components — Stated So They Are Not Touched

| Component | Why it appears here |
|---|---|
| `internal/repository/*` | No query, method or index is added. The category ID map is built in memory from an existing call |
| `internal/database/schema.sql` | No schema change. `category.name` keeps case-sensitive uniqueness |
| `internal/service/categorizer.go`, `sic_categorizer.go` | Categorization is untouched; this unit changes validation diagnostics only |
| Export path | Byte-identical output, per NFR-U4-COMPAT-01 |
| Admission gate, backup, merge | Untouched; this unit mutates nothing |

## Dependency and Concurrency Posture

No dependency is added, removed or upgraded. Standard library only: `strings`, `unicode`,
`unicode/utf8`, `fmt`.

No concurrency is introduced. `NewDiagValue` is a pure function, `indexCategories` builds local maps, and
validation remains single-threaded within one admitted request. NFR-U4-TEST-06 requires only that the
existing `-race -short` suite continues to pass; no new race-specific test is warranted.

## Traceability

| Component change | Requirement | Pattern |
|---|---|---|
| `DiagValue`, `NewDiagValue`, `AddErrorf`, `rejectf` | SEC-01 | DP-U4-02 |
| `AddError` 512-rune backstop | SEC-04 | DP-U4-03 |
| `matchesSICMappingHeader`, header diagnostic site | SEC-02, COMPAT-01 | DP-U4-01, DP-U4-05 |
| `indexCategories`, `resolveSICCategory` | PERF-01, SEC-01 | DP-U4-04, DP-U4-05 |
| `main.go` seed attribute | SEC-01 as amended, TEST-04 | DP-U4-07 |
| No template, handler, repository or schema change | COMPAT-01, COMPAT-02, REL-01 | — |
