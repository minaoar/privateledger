# NFR Design Patterns — UOW-4 Category Lifecycle and Mapping-File Integrity

Generated 2026-09-07 from NFR Requirements (approved, Q1–Q5 A, NFR-FQ1 A) and NFR Design answers
Q1 A, Q2 A, Q3 A, Q4 A. Awaiting explicit approval.

The standing direction from UOW-2 and UOW-3 applies: the **simplest mechanism that satisfies the
approved requirement**. No configuration surface, no background work, no abstraction introduced solely
for testing.

## DP-U4-01 — Header normalization as a comparison-time transformation

`matchesSICMappingHeader` compares after normalizing, and returns enough information to say *why* it
failed rather than a bare boolean.

Normalization, per column, in order:

1. On the **first column only**, strip a leading UTF-8 byte-order mark.
2. `strings.TrimSpace` the column name. Its `unicode.IsSpace` definition already covers the non-breaking
   space a spreadsheet may emit, so no bespoke whitespace set is introduced.
3. Compare with `strings.EqualFold` against the canonical name.

Column count is checked before column names, so a four-column file is diagnosed as a count problem
rather than reported against a position that has shifted.

Nothing normalized is stored, returned or echoed. The canonical `sicMappingCSVHeader` remains the single
definition of the contract, and export emits it unchanged — the tolerance is one-directional by
construction, which is what keeps NFR-U4-COMPAT-01's byte-identical export true without a separate
guard.

## DP-U4-02 — A constructed type as the interpolation boundary

Per Q2 A. `internal/model` gains a bounded, sanitized value type alongside the diagnostic it protects:

```go
// DiagValue is a bounded, sanitized value safe to interpolate into a
// diagnostic message. It can only be constructed through NewDiagValue,
// so a diagnostic cannot carry unbounded user text by omission.
type DiagValue struct{ s string }

func NewDiagValue(raw string) DiagValue
func (v DiagValue) String() string
```

`NewDiagValue` truncates to 64 runes counted with rune iteration, appends U+2026 when it truncated,
replaces every `unicode.IsControl` rune with U+FFFD, and returns valid UTF-8.

The report gains one interpolating entry point:

```go
func (r *SICMappingImportReport) AddErrorf(row int, field, code, format string, values ...DiagValue)
```

`sicRowValidator.rejectf` wraps it, preserving the `invalid = true` flag that `reject` sets — row
validity must still never be inferred from `len(report.Errors)`, for the reason already recorded above
`sicRowValidator`.

**What this does and does not guarantee.** A raw `string` will not compile in a `...DiagValue`
position, so the safe path is the path of least resistance. It is not a proof: a future site could still
call `fmt.Sprintf` into the constant-message `AddError`. Go cannot prevent that, and claiming otherwise
would be worse than the gap. Two further mechanisms close it in practice — DP-U4-03 and the behavioural
ceiling test in DP-U4-06.

Header diagnostics interpolate only integers and therefore use plain `AddError` with `fmt.Sprintf`.
An integer cannot be unbounded, so this is not the bypass path; it is the reason the bypass path exists
at all, and it is recorded here rather than left for a reader to notice.

## DP-U4-03 — Assembled-message backstop in AddError

`AddError` truncates any message beyond 512 runes, satisfying NFR-U4-SEC-04.

The largest well-formed message is roughly 250 runes, so this never fires in normal operation. Its
purpose is to bound the worst case of a bypass that DP-U4-02 discourages but cannot forbid: a diagnostic
that forgets `NewDiagValue` produces an ugly message rather than a multi-megabyte response.

Placing it in `AddError` rather than at each call site means every present and future diagnostic is
covered, including any added by a later unit that never reads this document.

## DP-U4-04 — One category index pass, three indexes

Per Q3 A. `indexCategories` returns three maps built in a single pass over the slice
`categoryRepo.GetAll()` already returns: exact name, case-folded name, and category ID.

No repository method is added and no query is issued. Cost is one pass over categories — tens of rows in
this application — per upload, not per row. A lazy build would add a branch and a second code path to
avoid work too small to measure.

**Recorded coupling.** The colliding-category list in an ambiguity diagnostic follows the order of the
folded map's slice, which follows `GetAll`'s `ORDER BY name ASC` (`category_repo.go:87`). Diagnostics
are therefore deterministic without an explicit sort. If that `ORDER BY` ever changes, diagnostic content
changes with it and the reviewer-owned tests fail — loudly, which is the correct outcome. No redundant
sort is added; the coupling is documented instead.

## DP-U4-05 — Message wording

Per Q4 A. `«name»` marks a value passed through `NewDiagValue`.

| Code | Condition | Message |
|---|---|---|
| `category_not_found` | `Category_ID` present and resolves | `Category_Name "«Groceries»" does not exist; Category_ID 1 is currently "«Food»"` |
| `category_not_found` | `Category_ID` absent or unknown | `Category_Name "«Groceries»" does not exist` |
| `category_ambiguous` | more than one folded match | `Category_Name "«food»" matches 4 categories: "«Food»", "«food»", "«FOOD»" and 1 more` |
| `invalid_header` | wrong name or order | `CSV header column 4 must be Category_Name` |
| `invalid_header` | wrong count | `CSV header must have 5 columns; this file has 4` |

Two truncation signals can appear in one response and are worded so they cannot be confused: **"and N
more"** counts categories inside a single message, while the report-level **"Showing the first 50"**
rendered by `describeDiagnostics` counts diagnostics. They are independent — the per-message cap is
three names, the report cap is `MaxSICMappingDiagnostics`, and neither influences the other.

Three of the eleven diagnostic sites change. The other eight — `missing_header`, `malformed_csv` at the
header and at a row, `invalid_sic`, `duplicate_sic`, `id_without_name`, `invalid_category_id` and
`category_conflict` — keep their current constant messages. This unit changes only what U4-01 and U4-02
require.

## DP-U4-06 — Verification shape

Reviewer-owned in full, per the cross-provider rule. Production authors none of it.

| Requirement | Mechanism |
|---|---|
| NFR-U4-TEST-01 | `rapid` property over decorated canonical headers, both directions: decoration always matches, mutation never does |
| NFR-U4-TEST-02 | `rapid` property over arbitrary strings including invalid UTF-8: output ≤ 64 runes plus ellipsis, no control runes, valid UTF-8 |
| NFR-U4-SEC-01 bypass | **Behavioural ceiling test**: upload a CSV carrying a multi-megabyte category name and assert total response size stays under a fixed bound |
| NFR-U4-TEST-03 | Four example uploads reproducing the failures recorded when U4-02 was admitted |
| NFR-U4-TEST-04 | Message content per DP-U4-05, plus seed-path log content per DP-U4-07 |
| NFR-U4-PERF-01 | Re-run UOW-2's existing 100,000-row merge harness; record the median |

The ceiling test is the mechanism that actually enforces the bound. A source-level check for
`fmt.Sprintf` near a diagnostic would be brittle and easy to defeat accidentally; a response-size
assertion holds regardless of how a future site is written.

## DP-U4-07 — The seed path receives the same diagnosis

Per Q1 A, and the substantive change this stage added.

`main.go` logs seed validation failures per rejected row with `path`, `row`, `field` and `code`. It gains
one attribute: the bounded `Message`.

```
WARN Rejected SIC mapping seed row  path=... row=7 field=Category_Name
     code=category_not_found
     message="Category_Name \"Groceries\" does not exist; Category_ID 1 is currently \"Food\""
```

Without this, UOW-4 improves the diagnosis only for the upload path — the one with a screen — and leaves
`sic_mappings.csv` exactly as opaque as before. That is the file the approved rationale identified as
mattering most: long-lived, hand-maintained, and read on databases where `Category_ID` values are
meaningless.

The record **count** is unchanged, so the existing reviewer tests asserting `maxSICSeedDiagnostics + 1`
records are unaffected. The seed cap, the summary record and the omitted-diagnostics count all stay as
they are.

## What Is Deliberately Not Designed

- **No sanitization inside `AddError` on whole messages.** The bound is per value; a 64-rune cap on an
  assembled message would truncate the fixed text that makes it useful. DP-U4-03's 512-rune backstop is
  a different mechanism serving a different purpose.
- **No configuration** for the 64-rune or 512-rune bounds. Two named constants, no config surface.
- **No change to failure ordering, the admission gate, backup, or merge.** This unit mutates nothing.
- **No UTF-16 detection.** Candidate finding C4-01; declined at Q5.
- **No length bound on category names at creation.** Candidate finding C4-02; UOW-4 bounds the
  consequence at the diagnostic boundary, not the cause.
