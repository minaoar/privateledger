# Technology Decisions — UOW-5 Rule-Sourced Recategorization

Generated 2026-09-07 alongside `nfr-requirements.md`, awaiting the same approval. UOW-1 through UOW-4
technology decisions are retained. **No dependency is added, removed or upgraded**, and no schema change
is made.

| Concern | Decision | Requirement |
|---|---|---|
| Runtime | Existing Go module baseline; no new process | REL-03 |
| HTTP and UI | Existing Gin, embedded templates, Bootstrap, HTMX, vanilla JavaScript | — |
| Persistence | Existing `modernc.org/sqlite`; no table, column or index added | SCALE-01 |
| Whole-table read | An existing repository read widened to cover every transaction | SCALE-01 |
| Write batching | Existing `json_each` set-passing in `BulkUpdateCategory` | SCALE-02 |
| Rule caches | Existing `sync.RWMutex`-guarded caches with atomic publication | CON-02 |
| Logging | Existing `log/slog`, counts and codes | — |
| Verification | Go `testing`, temporary SQLite databases, race detector, the existing uninstrumented timing harness | TEST-07, TEST-08 |
| Generated properties | Existing test-only `pgregory.net/rapid v1.1.0` | TEST-01, TEST-02, PBT-09 |
| External services / infrastructure | None; single local binary retained; Infrastructure Design skipped | — |

## TD-U5-01 — Replace the collaborator contract rather than widen it

Per FD-FQ1 A. UOW-2's `RecategorizeBySICCodes(sicCodes []model.SICCode) (int, error)` becomes a
re-examination call taking no scope and returning the three FR16 counts.

Two independent reasons, either sufficient. There is no affected set to pass, so the parameter would be
accepted and ignored — the kind of thing that reads as a defect and invites someone to reintroduce
scoping. And a single `int` cannot express three counts, so the return type had to change regardless of
the argument.

## TD-U5-02 — Retain `json_each`, retire only the scoped query

The scoping use of `GetUncategorizedBySICCodes` retires with the affected set. The `json_each`
construction it proved does not: `BulkUpdateCategory` still needs exactly that, because a worst-case
pass writes every non-manual transaction, far past the 32,764-variable ceiling measured against
`modernc.org/sqlite v1.34.4` during UOW-3.

Recorded because "the scoped query is no longer used" is easy to misread as "the technique was
unnecessary". It was necessary and remains so on the write side.

## TD-U5-03 — Standard-library synchronization, unchanged

One decision function, two caches, one `sync.RWMutex`, atomic publication. No worker pool, queue,
background refresher or job system is introduced.

Q3 A kept the gate hold without a deadline, so no cancellation machinery is added either. Introducing
`context` cancellation now would describe a budget the measurement says nobody needs, and UOW-3 declined
it on the same grounds.

## TD-U5-04 — No new `category_source` value

`0=none, 1=rule, 2=manual` is unchanged. Recording which rule categorized a transaction was declined at
the requirements stage (R1 B), specifically to avoid the schema change and the unanswerable backfill it
would require for transactions already categorized.

A transaction that loses its category is written `category_id = NULL, category_source = 0` — the same
representation category deletion already produces. A second representation of "uncategorized" would
silently break the dashboard and every query that defines the term.

## TD-U5-05 — One traversal, no second counting query

Per Q5 A the pass reads every transaction and accumulates all three counts in flight.

A separate query for the manual count would read the same rows twice and could disagree with the writes
beside it if a rule reload landed between them. That is precisely the inconsistency BR-U5-10's
one-generation rule exists to prevent, and it would be self-inflicted.
