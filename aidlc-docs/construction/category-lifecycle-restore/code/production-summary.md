# Production Summary — UOW-4 Category Lifecycle and Mapping-File Integrity

Production generation completed 2026-09-07 against the approved code generation plan. **Production code
only.** No `_test.go` file, fixture, benchmark or test dependency was created or modified.

Three production files changed. No template, handler, repository, schema, route or dependency change.

## `internal/model/sic_mapping.go`

| Added | Purpose | Requirement |
|---|---|---|
| `maxDiagValueRunes = 64`, `maxDiagMessageRunes = 512` | The two bounds, named | SEC-01, SEC-04 |
| `DiagValue` | A struct wrapping an unexported string, so a raw `string` cannot be converted or passed where one is required | SEC-01 |
| `NewDiagValue` | Replaces `unicode.IsControl` runes with U+FFFD, **then** truncates to 64 runes, appending U+2026 only when it truncated | SEC-01 |
| `AddErrorf` | The only interpolating entry point for text; accepts `...DiagValue` | SEC-01 |
| `truncateRunes` | Rune-counted truncation shared by the backstop | SEC-04 |

`AddError` now truncates the assembled message at 512 runes, applied **after** the diagnostics-cap check
so cap and truncation semantics are unchanged. `MaxSICMappingDiagnostics`, `DiagnosticsTruncated` and the
append behaviour are untouched.

Sanitizing before truncating is deliberate: the reverse order could cut a multi-byte rune at the
boundary and then replace half of it.

## `internal/service/sic_mapping_service.go`

| Changed | Detail |
|---|---|
| `matchesSICMappingHeader` → `matchSICMappingHeader` | Returns a `*headerMismatch` describing *why*, not a bare boolean |
| `headerMismatch` | Carries a 1-based column position, or a pair of counts. No text from the file |
| Normalization | BOM stripped from the **first column only**; `strings.TrimSpace`; `strings.EqualFold` |
| Count before names | So a four-column file is diagnosed as a count problem, not against a shifted position |
| `indexCategories` | Returns a `categoryIndex` with exact, folded and **by-ID** maps from one pass |
| `sicRowValidator.rejectf` | Wraps `AddErrorf`, sets `invalid = true` exactly as `reject` does |
| `rejectCategoryNotFound` | Names the unresolved value, and the current name behind `Category_ID` when it resolves |
| `rejectAmbiguousCategory` | Names up to three colliding categories, then `and N more` |
| `diagInt` | Renders counts; an integer cannot be unbounded |

`sicMappingCSVHeader` is untouched, so export output is byte-identical.

**One change was needed that the plan did not anticipate.** `csvReader.FieldsPerRecord` was being set to
the canonical count *before* the header read, so a header with the wrong number of columns failed inside
`csv.Reader` as `malformed_csv` and never reached the count branch — Q3 A would have shipped as dead
code. The header is now read with `FieldsPerRecord = -1`, and the canonical count is set immediately
after the header matches, so row behaviour is unchanged. Verified: a ragged row still reports
`CSV row is malformed`.

This was found by exercising the running binary, not by reading the code.

The ID-naming form of `category_not_found` fires only when the ID parses, is positive **and** resolves.
A malformed ID keeps the short form — this diagnostic must not double as ID validation, which
`invalid_category_id` owns.

`rejectAmbiguousCategory` builds its format string from constants and a placeholder count, never from
user text, and passes each name as its own bounded `DiagValue`. Bounding the assembled list as one value
would have truncated three 64-rune names to 64 total.

## `cmd/privateledger/main.go`

One attribute added to the existing per-row seed warning: `slog.String("message", ...)`.

Record count, `maxSICSeedDiagnostics`, the summary record and the omitted-diagnostics count are all
unchanged, so record-count assertions hold.

## Verification Run by Production

`gofmt` clean, `go vet ./...` clean, `go build ./...` clean.

`go test -count=1 ./...` and `go test -race -short -count=1 ./...` — **two reviewer-owned tests fail in
both, and only those two.** Both fail against deliberately changed behaviour. Neither was edited. See
`independent-review-handoff.md`, HF-01 and HF-02.

### Behaviour verified against the running binary

Exercised on a throwaway database and a non-default port. Not a substitute for the independent tests;
recorded so the reviewer knows which paths were observed working rather than merely compiled.

| Case | Result |
|---|---|
| Header with BOM + lowercase + a padded column name | Accepted |
| Seed row, stale name, `Category_ID` resolves | `Category_Name "Nonexistent Cat" does not exist; Category_ID 1 is currently "Food"` |
| Seed row, stale name, no ID | `Category_Name "No Such Name" does not exist` |
| Four case-colliding categories | `Category_Name "FooD" matches 4 categories: "FOOD", "Food", "fOOd" and 1 more` |
| 300-character category name | Truncated to exactly 64 runes plus U+2026 |
| Header with 4 columns / 6 columns | `CSV header must have 5 columns; this file has 4` / `... has 6` |
| Header with a misspelled column | `CSV header column 4 must be Category_Name` |
| Ragged data row | `CSV row is malformed`, unchanged |

The seed cases were run twice: once on a database with no categories, confirming the short form, and
again after creating category 1, confirming the ID-naming form.

## Not Done

- No test authored. The independent provider owns all six verification obligations in DP-U4-06.
- No UTF-16 detection (C4-01), no category-name length bound at creation (C4-02).
- No restore capability, no deletion, no schema change.
