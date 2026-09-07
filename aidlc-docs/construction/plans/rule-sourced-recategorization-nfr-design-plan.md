# NFR Design Plan — UOW-5 Rule-Sourced Recategorization

## Stage Inputs

UOW-5 NFR Requirements approved and pushed 2026-09-07 (`e9b9723`). Binding: FR15, FR16, BR-U5-01 through
BR-U5-20 with BR-U5-02 as amended, NFR-U5-PERF-01 through TEST-08, and TD-U5-01 through TD-U5-05.

This stage resolves the four mechanics the approved NFRs assigned to it, plus two gaps found while
reading the code for them. The standing direction from UOW-2 through UOW-4 applies: the **simplest
mechanism that satisfies the approved requirement** — no configuration surface, no abstraction
introduced solely for testing.

## Verified Starting State

Read from the working tree before writing this plan.

| Observation | Evidence |
|---|---|
| `RecategorizeAll` already groups by target category and batches writes | `categorizer.go:241-273` |
| It claims counts **only after** each update commits | `categorizer.go:259-260`, comment and loop |
| `decide` returns `(categoryID, categorySource)` with `sourcePattern` / `sourceSIC` already distinguished | `categorizer.go:164`, `:42-46` |
| `RecategorizeResult` carries processed, categorized, pattern and SIC counts | `categorizer.go:212-218` |
| Those four fields are rendered by the UI | `categories.html:538-547`, `import.html:188-199` |
| The collaborator is `RecategorizeBySICCodes([]SICCode) (int, error)` | `sic_mapping_service.go:53` |

### Two gaps the requirements did not name

**`BulkUpdateCategory` cannot write NULL.** Its signature is
`BulkUpdateCategory(categoryID int, source model.CategorySource, transactionIDs []int)`. A plain `int`
cannot express "no category", so the batched write path has no way to express R2 A's outcome — the whole
point of this unit's uncategorization case.

**`ClearCategory` is row-at-a-time.** It calls `UpdateCategory` in a loop, one round trip per
transaction (`categorizer.go:341-347`). It was written for category deletion, where the row count is
whatever that category held. Reused for bulk uncategorization it would issue thousands of round trips
and would not survive NFR-U5-PERF-01's 1.5-second bound.

Both are the same missing capability: **a batched clear**. Q2 settles it.

## Open Questions

Answer each by replacing the `[Answer]:` tag. One option per question is marked **(Recommended)**.

### Q1 — How `decide` gains an "ignore the existing category" mode

BR-U5-06 requires evaluating a transaction as if it had no category, without weakening the manual guard.
NFR Requirements additionally asked that the mode be hard to invoke accidentally from the import path.

- A. **(Recommended)** Keep `decide` as the single matcher and give it an unexported parameter, reached
  through two named wrappers: the existing `decideCategory` for import, and a new
  `decideOnReexamination` for the pass. The boolean appears in no exported signature and no caller
  chooses it.
  *One matcher, so the priority order cannot drift between paths — which is the property FR15 rests on.
  Call sites read as intent rather than as a flag, and the import path cannot opt into re-examination
  semantics without someone writing a new wrapper, which is a visible act.*
- B. Two separate decision functions.
  *Two implementations of one priority order is exactly the shape that drifts, and BR-U5-07 says the
  order is identical in both.*
- C. Have the pass pass a copy of the transaction with its category blanked.
  *Neat, and it silently makes the manual guard depend on a field the caller just rewrote. The guard
  reads `CategorySource`, so a careless blanking would disable manual protection.*

[Answer]:A

### Q2 — Writing "became uncategorized" in bulk

- A. **(Recommended)** Add a repository method that clears a set of transaction IDs in one statement,
  built exactly like `BulkUpdateCategory` — `SET category_id = NULL, category_source = 0 WHERE
  transaction_id IN (SELECT value FROM json_each(?))`. Leave `BulkUpdateCategory` unchanged.
  *The `json_each` construction already exists and already handles sets past SQLite's 32,764-variable
  ceiling. A separate method keeps the assign path's signature honest — a `*int` on `BulkUpdateCategory`
  would make every existing caller carry a nullable it never uses.*
- B. Change `BulkUpdateCategory` to take `*int`.
  *One method, and it makes "assign a category" and "remove a category" the same call, so a nil slipping
  through a caller silently uncategorizes rows instead of failing.*
- C. Reuse `ClearCategory`.
  *Row-at-a-time. It would not meet NFR-U5-PERF-01.*

[Answer]:A

### Q3 — The result shape, given the UI already reads four fields

`RecategorizeResult`'s `processed_count`, `categorized_count`, `pattern_categorized_count` and
`sic_categorized_count` are rendered in `categories.html` and `import.html`. FR16 needs three counts that
do not map onto them: moved, uncategorized, manual-protected.

- A. **(Recommended)** Add the three FR16 counts as new fields and keep the existing four, with
  `categorized_count` continuing to mean what it means today. Update the two templates to show the new
  counts where a rule change produced them.
  *Additive, so nothing that reads the response breaks — including the import path, which does not
  re-examine and for which the new counts are simply zero. The existing pattern/SIC split is still
  useful and answers a different question from FR16's.*
- B. Replace the four fields with the three FR16 counts.
  *Cleaner shape, and it breaks both templates and the import result for no requirement that asked for
  it.*
- C. A separate result type for re-examination.
  *Honest about them being different operations, at the cost of two shapes the UI must branch on.*

[Answer]:A

### Q4 — Where the single re-examination entry point lives

Per BR-U5-12 and TD-U5-01, one entry point serves every trigger. Today pattern changes call
`RecategorizeByCategory` in the category handler and mapping changes call the collaborator.

- A. **(Recommended)** On `Categorizer`, which already owns `LoadRules`, `decide` and the batching. The
  collaborator interface becomes a thin adapter onto it, and the category handler calls it directly.
  *The state it needs — both rule caches, the lock, the decision function — is already there. Putting it
  anywhere else means passing that state somewhere or duplicating the reload. `RecategorizeAll` and
  `RecategorizeByCategory` collapse into it, which removes two entry points rather than adding a third.*
- B. A new service alongside `Categorizer`.
  *A clean seam, and it would need the rule caches and the lock, which means either sharing them across
  a boundary or reloading twice.*

[Answer]:A

### Q5 — Whole-table read: materialize or stream

Per BR-U5-02 as amended the pass reads every transaction. `GetUncategorized` and `List` both materialize
a slice today.

- A. **(Recommended)** Materialize, consistent with every existing read. Record the expected memory
  rather than bounding it.
  *A `Transaction` is a handful of scalars and short strings; 20,000 of them is a few megabytes in a
  desktop process, and even 100,000 stays tens of megabytes. Streaming would mean holding a `rows`
  cursor open across the writes the same pass issues, on one SQLite connection — a deadlock shape, in
  exchange for memory nobody is short of.*
- B. Stream with batched writes.
  *Bounded memory, and it introduces read-during-write on one connection plus a partial-completion story
  BR-U5-11 does not want.*

[Answer]:A

## Answer Analysis — 2026-09-07

All five answers are A and are mutually consistent. One omission sits in Q1's own options.

### Q1's three options all assumed the manual guard stays inside the matcher

Every option offered a way to ignore the *existing category*. None accounted for BR-U5-14, which
requires evaluating rules **for manual transactions** in order to produce FR16's third count. A matcher
that stops on `CategorySource == manual` cannot answer "what would the rules have said about this one?"

This does not change the answer. It adds one primitive beneath it:

| Function | Guards | Used by |
|---|---|---|
| `evaluate` | **none** — patterns then SIC, pure matching | Both wrappers, and the manual-count path |
| `decideCategory` | manual, then existing-category | Import |
| `decideOnReexamination` | manual only | The write path |

Q1 A's intent is preserved exactly: **one matcher**, so the priority order cannot drift; the mode
appears in no exported signature; and both wrappers still carry the manual guard. What changes is that
`evaluate` sits below them rather than the guards living inside the matcher itself.

The counting path calls `evaluate` directly. That is the one place a manual transaction is evaluated,
and it is inside the pass where it has no write path available to it — the manual branch counts and
`continue`s before reaching any write. DP-U5-01 states this shape; NFR-U5-TEST-04 verifies it across
every trigger.

Raised here rather than resolved silently because "a function with no manual guard" is exactly the
thing a reviewer should look at hard.

## Execution Checklist

- [x] Confirm NFR Requirements approval and re-read the recategorization, write and result paths.
- [x] Identify the two capability gaps the requirements did not name: no bulk NULL write, and a
      row-at-a-time `ClearCategory`.
- [x] Confirm which result fields the UI actually reads before proposing a shape.
- [x] Receive answers to Q1 through Q5. (A, A, A, A, A)
- [x] Analyze answers for conflicts with the approved artifacts; raise follow-ups rather than resolving
      silently. One omission found in Q1's own options; see below.
- [x] Generate `logical-components.md` and `nfr-design-patterns.md` under
      `aidlc-docs/construction/rule-sourced-recategorization/nfr-design/`.
- [ ] Receive explicit NFR Design approval.

## Out of Scope

- Weakening manual protection.
- Recording which rule categorized a transaction.
- Candidate findings C4-01, C4-02, C4-03; deferred findings F-04, F-05.
- New dependencies and schema changes. Infrastructure Design remains skipped.
