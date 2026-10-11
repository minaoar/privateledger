# NFR Design Patterns — UOW-5 Rule-Sourced Recategorization

Generated 2026-09-07 from the approved NFR Requirements and NFR Design answers Q1 A through Q5 A.
Awaiting approval.

The standing direction from UOW-2 through UOW-4 applies: the **simplest mechanism that satisfies the
approved requirement**. No configuration surface, no background work, no abstraction introduced solely
for testing.

## DP-U5-01 — One matcher, guards in named wrappers

Per Q1 A, with one addition Q1's options did not anticipate.

All three options assumed the manual guard stays inside the matcher. None of them could satisfy
BR-U5-14, which requires evaluating rules **for manual transactions** to produce FR16's third count: a
matcher that stops on `CategorySource == manual` cannot answer "what would the rules have said about
this one?"

The structure therefore has one primitive beneath the wrappers:

| Function | Guards | Callers |
|---|---|---|
| `evaluate` | **none** — patterns in order, then SIC if no pattern matched | The two wrappers, and the manual-count path |
| `decideCategory` | manual, then existing-category | Import |
| `decideOnReexamination` | manual only | The re-examination write path |

Q1 A's intent is preserved exactly. There is **one matcher**, so the priority order cannot drift between
paths — the property FR15 rests on. The mode appears in no exported signature. Both wrappers still carry
the manual guard.

**On `evaluate` having no manual guard.** This is the part of the design most worth scrutiny, so it is
stated rather than buried. The manual-count path calls `evaluate` directly, and it is the only caller
that does so without a guard above it. It sits inside the pass, in a branch that counts and `continue`s
**before reaching any write**. There is no code path from that branch to a write.

That is a structural argument, not a guarantee, which is why NFR-U5-TEST-04 verifies manual protection
across every trigger independently of it.

## DP-U5-02 — A batched clear beside the batched assign

Per Q2 A. The repository gains one method that clears a set of transaction IDs in a single statement:

```sql
UPDATE ledger_transaction
SET category_id = NULL, category_source = 0
WHERE transaction_id IN (SELECT value FROM json_each(?))
```

Identical in construction to `BulkUpdateCategory`, which is left unchanged. IDs are encoded as JSON
numbers to match the INTEGER column, and one bound parameter keeps the statement clear of SQLite's
32,764-variable ceiling — the same reason `BulkUpdateCategory` was built this way in UOW-3.

Two existing things this replaces or avoids:

- `ClearCategory`'s row-at-a-time loop stays as it is for category deletion, but is **not** reused by the
  pass. One round trip per transaction would not survive NFR-U5-PERF-01.
- `BulkUpdateCategory` is not widened to `*int`. That would make "assign a category" and "remove a
  category" the same call, so a nil slipping through a caller would silently uncategorize rows instead
  of failing to compile.

## DP-U5-03 — Additive result shape

Per Q3 A. `RecategorizeResult` keeps `processed_count`, `categorized_count`,
`pattern_categorized_count` and `sic_categorized_count`, and gains the three FR16 counts.

`categorized_count` keeps its current meaning. The import path does not re-examine, so its new counts
are simply zero and `import.html` continues to read what it reads today.

The templates that render a rule-change result — `categories.html` and the SIC mapping page — show the
three new counts. `import.html` is untouched.

## DP-U5-04 — Counts claimed only after the write commits

`RecategorizeAll` already does this: it accumulates intended changes, issues each batched write, and
only then increments the counters, so a failure part-way through never reports rows it did not write.

That discipline extends to all three FR16 counts. The uncategorized count is claimed after the batched
clear commits; the moved count after each batched assign commits. The manual-protected count is the
exception and is claimed immediately, because it corresponds to no write at all — nothing can fail
between deciding it and reporting it.

## DP-U5-05 — One entry point on `Categorizer`

Per Q4 A. Re-examination lives on `Categorizer`, which already owns both rule caches, the lock,
`LoadRules` and the batching.

`RecategorizeAll` and `RecategorizeByCategory` collapse into it. This **removes two entry points rather
than adding a third** — and `RecategorizeByCategory` in particular has been misleading since UOW-3: it
takes a category ID and then reads all uncategorized transactions, ignoring it.

UOW-2's collaborator becomes a thin adapter onto the same call, per TD-U5-01, taking no scope and
returning the three counts. The category handler calls it directly.

## DP-U5-06 — Materialized read, memory recorded not bounded

Per Q5 A the pass reads every transaction into a slice, consistent with `GetUncategorized` and `List`.

A `Transaction` is a handful of scalars and short strings. Twenty thousand is a few megabytes in a
desktop process; a hundred thousand stays in the tens of megabytes. Expected memory is recorded with the
performance evidence rather than enforced with a bound.

Streaming was declined for a specific reason beyond complexity: it would hold a `rows` cursor open
across the writes the same pass issues, on one SQLite connection. That is a deadlock shape, traded for
memory nobody is short of.

## DP-U5-07 — Ordering

| Constraint | Mechanism |
|---|---|
| One pass sees one rule generation | `LoadRules` publishes both caches under one write lock, unchanged from UOW-3. The pass reads rules under `RLock` held across both sources |
| Category deletion re-examines the post-deletion rule set | The database cascade completes first — patterns deleted, mappings emptied, transactions nulled — and `LoadRules` runs **after** it. BR-U5-09 |
| A rule change is never rolled back for a re-examination failure | Unchanged from BR-U2-45. The change is already committed; re-examination failure is reported, not compensated |

## DP-U5-08 — Verification shape

Reviewer-owned in full. Production authors none of it.

| Requirement | Mechanism |
|---|---|
| NFR-U5-TEST-01 | `rapid` property: generate a rule set and a permuted creation order, apply in that order, assert identical final categorization across permutations |
| NFR-U5-TEST-02 | `rapid` or example: a second consecutive pass writes nothing and reports three zeros |
| NFR-U5-TEST-03 | Every trigger in BR-U5-01, plus the two non-triggers — a description-only mapping edit and an import |
| NFR-U5-TEST-04 | No `category_source = 2` row is written by any trigger, including the case where rules would have moved it |
| NFR-U5-TEST-05 | Creating a pattern that matches a mapping-categorized transaction moves it to the pattern's category |
| NFR-U5-TEST-06 | The three counts across moves, uncategorizations and manual protections; zeros reported as zeros; unchanged rows neither written nor counted |
| NFR-U5-TEST-07 | The two PERF-01 bounds, the populated PERF-02 merge benchmark, and PERF-03 unchanged |
| NFR-U5-TEST-08 | `-race -short` across all packages |

The order-independence property is the one that matters most. FR15 is a claim about **all** orderings,
and no example-based test can establish it — the whole point is that no particular order is special.

## What Is Deliberately Not Designed

- **No context or cancellation.** Q3 at NFR Requirements kept the gate hold without a deadline, on a
  measurement that says nobody needs one at these timings.
- **No new `category_source` value**, and no column recording which rule categorized a transaction.
  Declined at the requirements stage.
- **No second representation of "uncategorized".** `category_id = NULL, category_source = 0` is what
  category deletion already writes.
- **No configuration** for anything in this unit.
- **No change to the import result shape or `import.html`.**
