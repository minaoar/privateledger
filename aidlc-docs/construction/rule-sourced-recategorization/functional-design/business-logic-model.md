# Business Logic Model — UOW-5 Rule-Sourced Recategorization

## Scope and Boundary

UOW-5 makes categorization a function of the current rules. A transaction whose category came from a
rule follows the rules when they change; a transaction the user categorized by hand is never touched.

Stage answers governing this design: R1 A, R1a A, R2 A, R3 A, R4 A, R5 A, R6 A (requirements amendment)
and Q1 A, Q2 A, Q3 A, Q4 A, Q5 A, Q6 A, FD-FQ1 A (this stage).

It adds no table, column or index. It adds no dependency. It changes which transactions are read and
written by existing operations, and it changes one interface contract.

## The Governing Requirement

FR15, in the user's framing:

> I want the same rule book to create the same transaction categorization, irrespective of when the
> rules were created.

Every decision below follows from it. Where a cheaper option existed, FR15 is the reason it was not
taken — most visibly at Q6, where re-examining on import would have made the outcome depend on import
order, which FR15 forbids in terms.

## Re-examination

One operation, one meaning: **evaluate every non-manual transaction against the current rules and write
what those rules say.**

1. Reload the rules.
2. Read all transactions with `category_source != 2`.
3. For each, decide as if it had no category — patterns first, then SIC mapping.
4. Write the result where it differs from what is stored.

Step 3 is the change. The existing decision function stops when a transaction already has a category;
during re-examination that guard does not apply. The manual guard does, and always.

### What triggers it

| Trigger | Why |
|---|---|
| A text pattern created, changed or deleted | R3 A |
| A SIC mapping created, changed or deleted | R3 A |
| A category deleted | Q2 A — deleting a category deletes its patterns by cascade and empties its mappings, so it is a rule change twice over |
| A mapping upload | One invocation for the whole file, as BR-U2-31 already required |

**Import does not trigger it** (Q6 A). Import categorizes the transactions it brings in and leaves
existing ones alone. This is FR15's requirement rather than a saving: re-examining on import would make
the result depend on when a transaction arrived.

A description-only mapping edit still triggers nothing. It cannot alter any categorization, so a pass
would be a provable no-op.

### Why the scope is everything, not an affected set

Q1 A chose a full pass over the scoped SIC query UOW-3 built. The scoped query is *almost* sufficient
for a mapping change — a manual transaction, a pattern-matched transaction, and a transaction whose code
is outside the affected set can none of them change outcome.

The exception is a rule-sourced transaction with **no matching pattern and no SIC code**. It can only
exist if the pattern that categorized it was deleted without re-examination — which is precisely the
behaviour before this unit, and precisely the backlog R6 A accepts.

Scoping is therefore sound only once the FR15 invariant holds continuously, and unsound for exactly the
rows this unit inherits. A full pass is correct by construction instead of by argument, and it has no
invariant for a future entry point to break.

## The Collaborator Contract

UOW-2 defined `RecategorizeBySICCodes(sicCodes)`, returning a count. Under Q1 A there is no affected set
to pass, and under Q4 A a text-pattern change reaches the same entry point with no codes to offer.

Per FD-FQ1 A the contract takes **no scope** and returns **the three FR16 counts**. Keeping a parameter
that is accepted and ignored would read as a defect and invite someone to reintroduce scoping. The
single integer it returns today cannot express three counts in any case.

**BR-U2-31's shape survives**: one invocation per CRUD change, one per upload.

The `json_each` set-passing construction is not lost. It remains the proven way to pass a large set to
SQLite, and `BulkUpdateCategory` still needs exactly that to write results back. Only the *scoping* use
of `GetUncategorizedBySICCodes` retires.

## Priority, Unchanged

Manual stops. Then text patterns, in order, first match wins. Then the SIC mapping, if the transaction
carries a code. A mapping with an empty category assigns nothing.

The one consequence worth stating plainly: **because patterns outrank mappings, creating a pattern can
move transactions away from a category a mapping assigned.** US-15's last acceptance criterion pins it.
Under FR15 the alternative would make the result depend on whether the pattern or the mapping came
first.

## What a Rule Change Reports

Three counts, per FR16 and R5 A:

| Count | Meaning |
|---|---|
| Moved | Re-examined transactions that took a different category |
| Uncategorized | Re-examined transactions that matched no rule and lost their category |
| Manual protected | Manual transactions the current rules **would** have moved, had they not been manual |

The third is the one that answers the question a user actually has after a bulk rewrite. Computing it
means evaluating rules for manual transactions and discarding the outcome — an evaluation that must
never write anything.

A transaction whose re-examined category equals its stored category is not written and is not counted as
moved. Zero counts are reported as zero, never omitted.

## Ordering

| Step | Constraint |
|---|---|
| Category deletion | The database cascade runs first — patterns deleted, mappings emptied, transactions nulled. Rules are reloaded **after** the cascade, so re-examination sees the post-deletion rule set |
| Every trigger | Rules are reloaded before re-examination, and one pass sees one rule generation |
| Failure | A rule change is never rolled back because re-examination failed afterwards. That remains BR-U2-45's committed-with-warning result |

## Determinism

Two properties follow from FR15 and are worth stating as testable claims rather than leaving implicit:

- **Idempotence.** Running re-examination twice changes nothing the second time.
- **Order independence.** Two databases with identical transactions, rules and manual assignments
  categorize identically, whatever order their rules were created in or their transactions imported.

The second is US-15's sixth acceptance criterion and is the direct expression of FR15.

## Traceability

| Story | Logic |
|---|---|
| US-15 | Re-examination, its triggers, the full scope, the unchanged priority, the uncategorized outcome, manual protection, and both determinism properties |
| US-16 | The three counts, zero reporting, and the no-write-when-unchanged rule |
