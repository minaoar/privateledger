# NFR Design Patterns — UOW-3 Transaction Categorization Integration

Status: APPROVED by the user on 2026-09-06. No implementation or test execution claimed.
Stage answers: Q1 A, Q2 A, Q3 A, Q4 A. NFR identifiers refer to the approved UOW-3 requirements unless
labelled otherwise.

Design posture, carried from the direction given during UOW-2 NFR Design: the simplest mechanism that
satisfies each approved requirement. No new configuration surface, background worker, queue, or
abstraction introduced solely for testing.

## NFRP-U3-01 — One decision function with an injected lookup

A single unexported decision function decides every categorization, and import, "Recategorize All", and
scoped recategorization all call it. No entry point re-implements matching.

`Categorizer` receives the SIC lookup as a small interface with one method: given a canonical code,
return the mapped category or nothing. Priority therefore lives in one readable place while SIC logic
stays in the mapping categorizer, and the independent role can substitute a fake lookup with no
database.

Order inside the function is fixed: manual assignment stops first, then an existing category stops,
then text patterns in their existing order, then the SIC lookup, and a mapping with an empty category
assigns nothing. `RecategorizeAll` and `RecategorizeByCategory` replace their inline matching with calls
to it, which is what stops SIC applying on import while being skipped by an explicit recategorization.

Supports REL-01, MAINT-01, TEST-01–02.

## NFRP-U3-02 — One guarded cache set with an all-or-nothing swap

Both rule caches sit behind a single `sync.RWMutex`. Categorization takes a read lock; reload takes the
write lock.

Reload builds both replacement sets first and swaps them under one write lock. If either build fails,
**neither** cache changes and the error is returned. Rules are therefore either fully refreshed or
entirely unchanged, never half-old, and a categorization pass can never see patterns from after a change
beside mappings from before it.

One exported reload entry point refreshes both, so a caller cannot refresh one and forget the other.
`LoadPatterns` becomes unexported. The existing `go h.categorizer.LoadPatterns()` is deleted rather than
replaced: reload becomes part of the request that triggered it. That call is two defects at once — an
unsynchronized write to a slice in-flight requests read, and an ordering bug letting a user add a
pattern and immediately recategorize against a cache that has not refreshed. A reload is one small
query, so the latency the goroutine saved does not justify either.

Caches are derived state. SQLite stays authoritative and a stale entry can never produce a write that
contradicts a database constraint.

Supports CON-01, REL-01, SCALE-01, TEST-03.

## NFRP-U3-03 — Pass the set as one JSON parameter, not as expanded placeholders

SQLite accepts 32,766 bound variables. Measured against `modernc.org/sqlite v1.34.4`, an `UPDATE` with
two leading arguments succeeds at 32,764 IDs and fails at 32,765 with `too many SQL variables`.

Both expanding query builders stop expanding one placeholder per element. The set is encoded once as a
JSON array and bound as a **single** parameter, matched with `IN (SELECT value FROM json_each(?))`.

```
WHERE sic_code IN (SELECT value FROM json_each(?))   -- 1 parameter, any number of codes
WHERE transaction_id IN (SELECT value FROM json_each(?))
```

This was chosen over chunking after measuring both on the same 200,000-row fixture:

| Approach | Bound parameters | 100,000 ids | 50,000 codes |
|---|---|---|---|
| Chunked `IN (?,?,…)`, 4 statements in one transaction | 32,764 per chunk | 2.74 s | — |
| Single `json_each` parameter, 1 statement | 3 total | 0.18 s | 0.19 s |

Chunking would have solved the ceiling while leaving three problems that set-passing simply does not
have:

1. **The documented ordering survives.** Chunked reads are each ordered internally but their
   concatenation is not, so `ORDER BY date_posted DESC` would have to be repaired in Go. One statement
   keeps the sort in SQL where the contract says it happens. Repairing it in Go would also have left a
   sort that looks removable to anyone who does not know why it is there.
2. **Atomicity is intrinsic.** A chunked update needs an explicit wrapping transaction so a mid-batch
   failure cannot leave rows categorized by a rule the caller was told had failed. A single statement is
   all-or-nothing by definition, with nothing to remember.
3. **It is roughly fifteen times faster** at 100,000 ids, and the gap widens with size.

Implementation constraints that matter:

- **Encode SIC codes as JSON strings, not numbers.** `sic_code` is a `TEXT` column, and `json_each` over
  `[7011]` yields an INTEGER whose comparison against TEXT is governed by type affinity. `["7011"]`
  yields TEXT and matches. Transaction IDs are an INTEGER column and are encoded as JSON numbers.
- An empty set still short-circuits before issuing SQL, as it does today.
- The JSON payload is one large string parameter rather than many small ones; this is a memory shape
  change, not a memory increase, and is bounded by the same set the caller already holds.

Not independently verified here: that the index on `sic_code` is still chosen with a `json_each`
subquery rather than a scan. The measured timings are consistent with index use, but the independent
role should confirm the query plan rather than infer it from timing.

This work is required independently of the amended 20,000-transaction target, which sits below the
ceiling. UOW-2's approved merge fixture produces 50,000 affected codes and exceeds it, and
`RecategorizeAll` is already reachable past it today by any user with more than 32,764 uncategorized
transactions matching a single pattern.

Supports PERF-01, REL-01, TEST-01, TEST-04.

## NFRP-U3-04 — Two recategorization passes over one mechanism

**Full pass.** Read the uncategorized set once through the existing `GetUncategorized`, apply the
decision function in memory, group results by resolved category, and issue one batched update per
category. Observed memory at the 20,000-transaction target is recorded as evidence rather than bounded,
consistent with how SCALE-01 treats the mapping cache. Streaming was rejected because it would hold a
read cursor open across the writes the same pass issues — a harder correctness problem than the one it
solves — and `LIMIT`/`OFFSET` paging was rejected because rows shift between pages as they are
categorized, so a page can be skipped.

**Scoped pass.** Read only uncategorized transactions whose SIC code is in the affected set, then apply
the same decision function and the same batched update. An empty affected set does no work and returns
zero.

Both passes reload rules before reading. If reload fails, no categorization is attempted, so
transactions are never categorized against rules the caller believes were already refreshed.

Supports PERF-01, REL-01, TEST-01.

## NFRP-U3-05 — Counts that cannot disagree

The decision function reports which rule source assigned a category, so the caller counts pattern and
SIC assignments as it goes rather than inferring them afterwards.

`PatternCategorizedCount + SICCategorizedCount == CategorizedCount` holds by construction: the two new
fields partition the existing total instead of being derived separately, so they cannot drift from it.
Counts are claimed only after the corresponding update commits.

Supports REL-02, UX-01, TEST-01.

## NFRP-U3-06 — Import hot path

Import calls the same decision function per transaction, resolving SIC through the in-memory cache under
a read lock. No per-transaction query is added.

Two fixtures are measured. The existing SIC-free import keeps its 10 % median regression budget
unchanged, protecting users who never adopt the feature. A second fixture with mappings populated and
SIC-bearing transactions is held to the same budget against the SIC-free baseline, which is the only
measurement able to catch a regression introduced by the lookup itself.

Supports PERF-02, MAINT-01, TEST-04.

## NFRP-U3-07 — Modal SIC context and mapping creation

Both transaction modals read SIC context from the already-joined `SICDescription` on the transaction, so
no per-modal query is added. Display falls back from `Description` to `Description_Detail`, and renders
the bare code cleanly when neither exists. Transactions with no SIC code simply omit the block.

Modal-created mappings go through UOW-2's `SICMappingService`, never the mapping repository directly, so
normalization, uniqueness, the admission gate, backup, and recategorization behave identically whether
the mapping is created from the mapping page or from a modal. A category is required. Create Pattern
creates a SIC mapping instead of a text pattern, never both.

Every new page-level JavaScript name is checked against the globals in `app.js` before use.
`layout.html` loads `app.js` after page content, so a colliding name is silently overwritten at runtime
while the markup still looks correct — defect U2-F09, where mapping deletion could not work at all.

Supports UX-01, SEC-01, MAINT-01.

## NFRP-U3-08 — Verification and performance evidence

Independent tests use temporary databases and injected fakes for the SIC lookup and the collaborator, so
priority, scoping, and call counts can be asserted without a populated database.

Required coverage: the full priority matrix; that both recategorize entry points route through the
decision function; scoped recategorization touching only affected uncategorized transactions; split
counts summing to the total; reload ordering after a rule change; **set-passing correctness at and beyond the
former parameter ceiling for both builders, including SIC codes encoded as JSON strings matching a TEXT
column, and an empty set issuing no SQL**; modal display including the fallback and no-SIC cases; and modal-created mappings requiring a
category and creating no text pattern.

Generated properties, per the approved scope, cover the priority matrix and recategorization scoping,
with shrinking and a recorded replay seed. Race evidence is required and no longer conditional, run
separately from performance acceptance.

Benchmarks: "Recategorize All" over 20,000 uncategorized transactions with a populated mapping set,
median of at least five runs after one warm-up, within five seconds on the recorded reference
environment; plus both import fixtures against the 10 % budget. Record fixture composition, run values,
median, and environment.

Supports PERF-01, PERF-02, SCALE-01, TEST-01–05. PBT-09 retained; PBT-02/03 as approved; PBT-07/08
carried into execution.
