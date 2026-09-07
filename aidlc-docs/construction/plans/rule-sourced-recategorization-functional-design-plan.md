# Functional Design Plan — UOW-5 Rule-Sourced Recategorization

## Stage Inputs

UOW-5 requirements and story amendment approved and pushed 2026-09-07 (`2e6b777`). Binding decisions:
R1 A, R1a A, R2 A, R3 A, R4 A, R5 A, R6 A. Governing requirement **FR15**; result reporting **FR16**;
stories **US-15** and **US-16**.

FR15 is the constraint every question below answers to:

> Categorization is a function of the current rule set and the user's manual choices. It is **not** a
> function of rule history or import order.

## Verified Starting State

Measured against the working tree before writing this plan.

| Observation | Evidence |
|---|---|
| `RecategorizeAll` reads `GetUncategorized()` | `categorizer.go:233` |
| `RecategorizeByCategory` also reads `GetUncategorized()`, despite its name | `categorizer.go:289` |
| Scoped SIC recategorization reads `GetUncategorizedBySICCodes` | `sic_categorizer.go:151` |
| `decide` stops on a manual source **and** on any existing category | `categorizer.go:166-172` |
| Patterns are evaluated before SIC mappings | `categorizer.go:181-191` |
| Deleting a category **cascades to delete its patterns** | `schema.sql:64`, `ON DELETE CASCADE` |
| Deleting a category **nulls the category on its SIC mappings** | `schema.sql:74`, `ON DELETE SET NULL` |
| Deleting a category nulls it on transactions | `schema.sql:42`, `ON DELETE SET NULL` |

Every current entry point reads only uncategorized transactions. That is the single largest change this
unit makes: all three must now also read rule-sourced ones.

## A Subtlety Worth Settling Before Q1

It is tempting to keep the existing SIC-scoped query for mapping changes, on the grounds that a mapping
change cannot affect a transaction whose SIC code is not in the affected set. That is **almost** true,
and the exception matters.

For a mapping change, consider each transaction:

| Transaction | Can a mapping change alter its outcome? |
|---|---|
| Manual | No — excluded always |
| A text pattern matches it | No — patterns are unchanged and outrank mappings |
| No pattern matches, SIC code **is** in the affected set | **Yes** — in scope either way |
| No pattern matches, SIC code is not in the affected set | No — its mapping is unchanged |
| No pattern matches, **no SIC code**, yet rule-sourced | **Yes, and a scoped query misses it** |

The last row is the exception. A transaction can only be in that state if the pattern that categorized
it was deleted without re-examination — which is exactly what happens **before** UOW-5 ships, and what
R6 A accepts as a one-time backlog.

So scoping is sound **once the FR15 invariant holds continuously**, and unsound for the pre-existing
rows R6 A hands us. Any scoped design therefore owes a one-time full pass. That is not an argument
against scoping; it is the cost that has to be named.

## Open Questions

Answer each by replacing the `[Answer]:` tag. One option per question is marked **(Recommended)**.

### Q1 — Scope of re-examination

- A. **(Recommended)** Every rule change re-examines all non-manual transactions. One path, one query,
  no invariant to maintain.
  *Correct by construction rather than by argument, and it removes the special case above instead of
  managing it. It also collapses three entry points into one behaviour, which is what FR15 describes.
  Cost is a full pass per rule change; UOW-3 measured 20,000 transactions well inside five seconds, and
  NFR Requirements will measure this directly.*
- B. Scope by SIC code for mapping changes, full pass for pattern changes and deletions, plus a one-time
  full pass to clear the R6 backlog.
  *Faster for the most frequent operation and preserves the existing indexed query. The cost is that
  correctness now depends on an invariant holding continuously — every future entry point must maintain
  it, and a missed one strands transactions silently.*
- C. Scope by SIC code for mapping changes with no one-time pass.
  *Leaves the R6 backlog permanently stranded, which contradicts R6 A.*

[Answer]:A

### Q2 — Is deleting a category a rule change?

Verified above: deleting a category **deletes its patterns** (cascade) and **empties its mappings**
(set null). Both are rule changes by any reading. Today deletion nulls the category on affected
transactions and stops there — they are left uncategorized rather than re-examined.

- A. **(Recommended)** Yes. Category deletion triggers re-examination like any other rule change, so a
  transaction it orphans can be picked up by whatever rule now claims it.
  *FR15 makes this unavoidable: after deleting a category the rule set has changed, and the categories
  must reflect the rules that remain. Leaving orphans uncategorized when a mapping would now match is
  the same history-dependence in a different disguise.*
- B. No. Deletion keeps its current behaviour of nulling and stopping.
  *Smaller, but it means "delete a category" is the one rule change that does not follow FR15, with no
  principle behind the exception.*

[Answer]:A

### Q3 — What the FR16 manual count counts

FR16 requires reporting transactions "left unchanged because they are manual". Two readings.

- A. **(Recommended)** Manual transactions that the current rules **would otherwise have moved**.
  *Answers the question a user actually has — "how many of my hand-set categories did this protect?" —
  and the number means something. It requires evaluating rules for manual transactions and discarding
  the outcome, which is cheap and changes nothing.*
- B. All manual transactions in the database.
  *Trivial to compute and nearly meaningless: it reports the size of a set the operation never
  considered, and it would not change between two very different rule edits.*

[Answer]:A

### Q4 — One re-examination contract, or two paths?

Recorded as a cross-unit question in `unit-of-work-dependency.md`. Today pattern changes go through
`RecategorizeByCategory` / `LoadRules` in the category handler, and mapping changes go through UOW-2's
collaborator. R3 A requires both to re-examine.

- A. **(Recommended)** One contract. Both triggers converge on a single re-examination entry point.
  *Two paths that must produce identical results is exactly the shape that drifts. FR15 is a claim about
  one outcome, so one implementation of that outcome is the honest structure. It also means the
  reviewer's determinism test exercises the real path rather than one of two.*
- B. Two paths sharing the decision function but not the entry point.
  *Smaller diff, and it leaves UOW-2's collaborator contract untouched. The cost is two places that must
  stay in agreement about scope, ordering, counting and reporting.*

[Answer]:A

### Q5 — What "becomes uncategorized" writes

Per R2 A a re-examined transaction matching no rule loses its category.

- A. **(Recommended)** `category_id = NULL` and `category_source = 0`, matching what category deletion
  already writes today.
  *One representation of "uncategorized" in the database. A second would make the uncategorized
  dashboard and every existing query wrong in a way nothing would catch.*
- B. `category_id = NULL` while leaving `category_source = 1`.
  *Records "categorized by a rule" for a transaction with no category, which is a contradiction.*

[Answer]:A

### Q6 — Does importing transactions trigger re-examination?

- A. **(Recommended)** No. Import categorizes the transactions it brings in and does not re-examine
  existing ones. Import is not a rule change.
  *FR15 says the outcome depends on the rules, not on import order — and re-examining on import would
  make the outcome depend on import order. A is what FR15 requires, not merely what is cheaper.*
- B. Yes, import triggers a full re-examination.
  *Slower on the most common operation, and it makes an import a bulk rewrite of untouched data.*

[Answer]:A

## Answer Analysis — 2026-09-07

All six answers are A, mutually consistent and consistent with FR15, FR16, US-15 and US-16. One
consequence follows that no question asked about, and it changes an approved UOW-2 contract.

### Q1 A retires the scoped query, and with it the collaborator's argument

UOW-2 defined the collaborator as `RecategorizeBySICCodes(sicCodes []model.SICCode)`, and UOW-3
implemented it over `GetUncategorizedBySICCodes` — the query that needed the `json_each` set-passing fix
after the SQLite parameter ceiling was measured at 32,764 usable IDs.

Under Q1 A there is no affected set: every rule change re-examines all non-manual transactions, so the
codes argument describes a scope the implementation no longer uses. Under Q4 A both triggers converge on
one entry point, and a pattern change has no SIC codes to pass at all.

Leaving the signature as-is would mean a parameter that is accepted, documented and ignored — which
reads as a bug to the next person and eventually gets "fixed" by someone reintroducing scoping.

**What is not lost:** the `json_each` construction stays the proven way to pass a large set to SQLite,
and `BulkUpdateCategory` still needs exactly that to write results back. Only the *scoping* use retires.

### FD-FQ1 — The collaborator contract under one entry point

- A. **(Recommended)** Replace the codes argument. The contract becomes a re-examination call taking no
  scope and returning the FR16 counts. BR-U2-31 is amended accordingly.
  *Says what the operation now is. A caller cannot pass a scope that will be ignored, and the returned
  counts are what FR16 requires the caller to report — today the collaborator returns a single int,
  which cannot express three counts.*
- B. Keep the signature and ignore the argument.
  *Smallest diff to UOW-2, and actively misleading.*
- C. Keep the argument for reporting which codes triggered the change.
  *The trigger is already known to the caller, and a pattern change has no codes to pass.*

[Answer]:A

**Proceeding under A.** The artifacts are written on that basis; if the answer changes, only the
collaborator contract section of `business-logic-model.md` is affected.

### Consequences Recorded for Later Stages

Not questions — things the next stages must handle, recorded so they are not rediscovered.

| Consequence | Owner |
|---|---|
| Every rule change now reads all non-manual transactions. UOW-3 measured 20,000 within five seconds for one full pass; this makes that the cost of every mapping CRUD edit | NFR Requirements |
| `decide` must evaluate a transaction as if it had no category, without weakening the manual guard | NFR Design |
| Q3 A needs rules evaluated for manual transactions purely to count them, and that evaluation must never write | NFR Design; stated as BR-U5-14 |
| Category deletion cascades before re-examination runs, so rules must be reloaded after the cascade | NFR Design; stated as BR-U5-09 |

## Execution Checklist

- [x] Confirm the requirements amendment is approved and pushed.
- [x] Re-verify all entry points, the decision guards, and the category-deletion cascades.
- [x] Identify the scoping exception the R6 backlog creates, before proposing a scope.
- [x] Receive answers to Q1 through Q6. (A, A, A, A, A, A)
- [x] Analyze answers for conflicts with FR15, FR16, US-15 and US-16; raise follow-ups rather than
      resolving silently. One consequence found; follow-up FD-FQ1 below.
- [x] Generate the functional design artifacts under
      `aidlc-docs/construction/rule-sourced-recategorization/functional-design/`.
- [x] Receive explicit Functional Design approval. (Approved 2026-09-07: "push it and continue to next stage".)

## Out of Scope

- Weakening manual protection. `category_source = 2` stays excluded under every option.
- Recording which rule categorized a transaction. R1 B was declined; no schema change is proposed.
- Candidate findings C4-01, C4-02, C4-03, and deferred findings F-04 and F-05.
- New dependencies. Infrastructure Design remains skipped.
