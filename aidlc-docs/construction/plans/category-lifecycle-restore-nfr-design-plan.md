# NFR Design Plan — UOW-4 Category Lifecycle and Mapping-File Integrity

## Stage Inputs

UOW-4 NFR Requirements approved 2026-09-07 (Q1–Q5 all A, plus NFR-FQ1 A). Functional Design approved
2026-09-07 with Q1 B, Q2 A, Q3 A, Q4 A, FQ1 A.

This stage resolves the four mechanics the approved NFRs assigned to it. It does not reopen settled
preferences. Following the direction established at UOW-2 and UOW-3, the default is the **simplest
mechanism that satisfies the approved requirement** — no new configuration surface, no abstraction
introduced solely for testing.

Scope remains frozen at the 2026-09-06 admission of U4-01 and U4-02.

## Verified Starting State

Measured against the working tree before writing this plan.

| Observation | Evidence |
|---|---|
| There are **ten** diagnostic sites, all in one file | `sic_mapping_service.go` lines 258, 264, 274, 299, 310, 312, 397, 406, 411, 419, 423 |
| One already interpolates, but only an integer | Line 312, `fmt.Sprintf("SIC code duplicates row %d", firstRow)` |
| Upload diagnostics render as one `<p>` per line with `textContent` | `sic_mappings.html:187`, via `describeDiagnostics` at line 361 |
| `category.name` has no length constraint | `schema.sql:53`; `CreateCategory` validates none |
| Categories are already fully loaded before row validation | `categoryRepo.GetAll()` then `indexCategories`, which builds two maps |

**The escaping claim in `frontend-components.md` is correct** — verified rather than repeated. `setStatus`
creates a `<p>` per line and assigns `textContent`, so HTML injection through a category name is not
possible, and no `white-space: pre` styling is applied. This downgrades the control-character clause of
NFR-U4-SEC-01 from an injection defence to a legibility one: a newline in a name produces a run-on
paragraph, not a forged diagnostic line. The clause is still worth keeping — it is nearly free — but the
**length bound is what carries the security weight**, not the sanitization.

## A Gap Found While Verifying — the Mitigation Misses the Seed File

This is the substantive finding of the stage, and it was not visible before tracing the code.

`main.go:271-288` logs seed validation failures. Verified: it emits `path`, `row`, `field` and `code`
per rejected row, plus a summary. **It does not log `Message`.**

That is exactly what NFR-U4-SEC-01 requires — and it means the entire U4-01 mitigation **does not reach
the startup seed path**. A user whose `sic_mappings.csv` carries a stale category name gets:

```
WARN Rejected SIC mapping seed row  path=... row=7 field=Category_Name code=category_not_found
```

No name. No indication of which value is stale or what it should become. The one-edit repair that Q1 B
was chosen to enable is unavailable precisely where it was argued to matter most.

Recall the approved rationale, in the user's own framing: the name-first decision is **strongest for the
seed file**, because `sic_mappings.csv` is a long-lived hand-maintained interchange file read on a
database where `Category_ID` values are meaningless. UOW-4 currently improves the diagnosis for the
upload path, which has a screen, and leaves the seed path — the one with no screen and the longest-lived
files — exactly as opaque as before.

I wrote NFR-U4-SEC-01's "never logged" clause as a carry-forward of UOW-2's rule without tracing what
the seed path does with diagnostics. Having traced it, the blanket rule defeats the unit's own purpose
in its most important case. Raising it rather than quietly designing around it.

**Note on cost:** the existing reviewer tests assert the *number* of log records
(`maxSICSeedDiagnostics + 1`). Adding an attribute to an existing record changes no record count, so
those tests are unaffected.

## Open Questions

Answer each by replacing the `[Answer]:` tag. One option per question is marked **(Recommended)**.

### Q1 — Does the startup seed path get the improved diagnosis?

- A. **(Recommended)** Log the diagnostic `Message` in the seed path, as one added attribute on the
  existing per-row record. Amend NFR-U4-SEC-01 accordingly, dated.
  *Every value inside a `Message` is already bounded to 64 runes and sanitized by NFR-U4-SEC-01, so the
  logged string is bounded by construction — the reason the blanket ban existed does not apply once the
  bound is in place. It is one line, it covers every diagnostic type rather than just the renamed
  category, and without it this unit does nothing for the file the approved rationale called the most
  important one. The seed file is the user's own file beside their own binary, and the log is local.*
- B. Log a bounded `category_name` attribute only for `category_not_found`.
  *Narrower, but it special-cases one code and leaves the other nine as opaque as today, for no real
  reduction in what is written.*
- C. Change nothing; the seed path keeps logging codes only.
  *Keeps NFR-U4-SEC-01 absolute, and leaves U4-01 unmitigated for the seed file. Defensible only if you
  consider logging category names genuinely unwanted, in which case say so and I will record it as the
  reason.*

[Answer]:

### Q2 — How the bound is enforced across diagnostic sites

NFR-U4-SEC-01 requires one shared helper such that a diagnostic added later cannot bypass the bound
silently. Go cannot make bypass impossible; these differ in how conspicuous it is.

- A. **(Recommended)** Add `rejectf(field, code, format string, values ...diagValue)` alongside the
  existing constant-message `reject`, where `diagValue` is an unexported type constructible only through
  the sanitizing constructor. Interpolating a raw string will not compile in that position. Pair it with
  a reviewer-owned **behavioural ceiling test**: upload a CSV with a multi-megabyte category name and
  assert the whole response stays under a fixed size.
  *The type makes the safe path the path of least resistance, and the behavioural test catches a bypass
  at any site, present or future, without depending on anyone remembering a rule. Source-level greps for
  `fmt.Sprintf` would be brittle; a response-size assertion is not.*
- B. A plain `safeName(string) string` helper that each site calls.
  *Simplest, and bypass is a silent omission — the exact failure NFR-U4-SEC-01 named.*
- C. Sanitize inside `AddError` on the whole message.
  *Does not work. The bound is per value; applying 64 runes to a whole message would truncate the fixed
  text that makes it useful.*

[Answer]:

### Q3 — Category ID index construction

- A. **(Recommended)** Build it unconditionally inside the existing `indexCategories`, which becomes a
  function returning three indexes instead of two.
  *One extra pass over a slice already in memory, sized by category count — tens, in this application.
  Building it lazily on first failure adds a branch and a second code path to save an amount of work too
  small to measure.*
- B. Build it lazily, only when a row fails to resolve.

[Answer]:

### Q4 — Exact message wording

Concrete strings, for confirmation rather than preference. `«name»` marks a bounded, sanitized value.

| Code | Message |
|---|---|
| `category_not_found`, ID resolves | `Category_Name "«Groceries»" does not exist; Category_ID 1 is currently "«Food»"` |
| `category_not_found`, ID absent or unknown | `Category_Name "«Groceries»" does not exist` |
| `category_ambiguous` | `Category_Name "«food»" matches 4 categories: "«Food»", "«food»", "«FOOD»" and 1 more` |
| `invalid_header`, wrong name or order | `CSV header column 4 must be Category_Name` |
| `invalid_header`, wrong count | `CSV header must have 5 columns; this file has 4` |

- A. **(Recommended)** Adopt as written.
  *Each names the value to change and what to change it to, which is the one-edit repair. "and 1 more"
  is deliberately worded differently from the report-level "Showing the first 50" so the two truncation
  signals cannot be mistaken for each other.*
- B. Adopt with changes you specify.

[Answer]:

## Execution Checklist

- [x] Confirm NFR Requirements approval and re-verify the diagnostic, rendering and seed paths.
- [x] Verify the `frontend-components.md` escaping claim rather than repeating it.
- [x] Trace the seed logging path and raise the mitigation gap it exposes.
- [ ] Receive answers to Q1 through Q4.
- [ ] Amend NFR-U4-SEC-01 if Q1 changes the logging clause, dated.
- [ ] Generate `logical-components.md` and `nfr-design-patterns.md` under
      `aidlc-docs/construction/category-lifecycle-restore/nfr-design/`.
- [ ] Receive explicit NFR Design approval.

## Out of Scope

- Any finding not admitted before the 2026-09-06 scope freeze, including candidate findings C4-01
  (UTF-16 detection) and C4-02 (unbounded category names at creation).
- A restore capability, restore semantics, and any deletion of mappings.
- Deferred independent findings F-04 and F-05.
- New production dependencies and schema changes. Infrastructure Design remains skipped.
- UOW-5, which remains registered and not started.
