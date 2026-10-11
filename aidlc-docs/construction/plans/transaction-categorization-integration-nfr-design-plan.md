# NFR Design Plan — UOW-3 Transaction Categorization Integration

## Stage Inputs

UOW-3 NFR Requirements approved 2026-09-06 (all five answers A). Functional Design approved with Q1 B,
Q2 B, Q3 A, Q4 A, Q5 A, Q6 B.

This stage resolves implementation mechanics assigned to it by the approved NFRs. It does not reopen
settled preferences. Following the direction given during UOW-2's NFR Design, the default here is the
**simplest mechanism that satisfies the approved requirement** — no new configuration surface, no
background workers, no abstraction introduced solely for testing.

## Verified Starting State — a Blocking Scale Defect

Measured against the working tree before writing this plan.

**SQLite's parameter ceiling is 32,766 variables.** Verified empirically against
`modernc.org/sqlite v1.34.4`: an `UPDATE ... WHERE id IN (...)` with two leading arguments succeeds at
32,764 IDs and fails at 32,765 with `too many SQL variables`.

Two existing query builders expand one placeholder per element with no batching:

| Site | Expands | Fails when |
|---|---|---|
| `internal/repository/transaction_repo.go:450` `BulkUpdateCategory` | one placeholder per transaction ID | more than 32,764 transactions resolve to a single category |
| `internal/repository/transaction_repo.go:346` `GetUncategorizedBySICCodes` | one placeholder per SIC code | more than 32,766 affected codes in one recategorization |

Three consequences follow. The scale amendment below changes which of them still bite.

**Scale amendment, 2026-09-06.** NFR-U3-PERF-01 was amended on user direction from 100,000
uncategorized transactions to 20,000. That is below the 32,764 ceiling, so it removes the
transaction-side collision from the benchmark. It removes neither of the other two.

1. **`RecategorizeAll` is broken above 32,764 transactions in one category, today.** It groups
   transactions by category and calls `BulkUpdateCategory` once per category. This is a pre-existing
   latent defect, and after the amendment it now sits *outside* the measured envelope rather than inside
   it — real, but no longer demonstrated by our own benchmark.
2. **~~NFR-U3-PERF-01 will hit it deliberately.~~** No longer true. At 20,000 transactions the update
   stays under the ceiling, so the transaction-side path is no longer exercised past its limit. This was
   the strongest argument for Q1 and the amendment removes it; saying so plainly matters more than
   keeping the argument tidy.
3. **UOW-2's approved benchmark still breaks once the real collaborator is wired.** That fixture uploads
   100,000 *codes* as 25,000 new plus 25,000 changed — **50,000 affected codes**, past the ceiling for
   `GetUncategorizedBySICCodes`. This is unaffected by the transaction amendment, because it is driven by
   mapping-code count, not transaction count. It passed in UOW-2 only because the collaborator was a
   no-op that never issued the query.

So Q1 is still required, but for a narrower reason than when this plan was first written: consequence 3
plus latent robustness, rather than a benchmark that fails on both sides. If you would also like the
UOW-2 merge fixture reduced, that is a separate amendment to an approved UOW-2 artifact and I have not
made it unilaterally.

## Open Questions

Answer each by replacing the `[Answer]:` tag. One option per question is marked **(Recommended)** with
the reason below it.

### Q1 — Where to fix the parameter ceiling

- A. **(Recommended)** Batch inside the two repository methods. Each chunks its input at a named limit
  below 32,766 and issues successive statements, so every caller is safe without knowing the limit
  exists. `BulkUpdateCategory` performs its chunks in one transaction so a category update stays
  all-or-nothing.
  *The limit is a property of the driver, so the layer that owns the driver should absorb it. Fixing it
  at the call sites means every future caller has to remember, and the failure mode is a runtime SQL
  error under load rather than a compile-time mistake.*
- B. Batch in the service layer, leaving the repository methods as they are.
  *Leaves two loaded guns for the next caller, including anything UOW-3 does not touch.*
- C. Reduce scope so the limit is never reached — for example, cap how many codes one recategorization
  may affect.
  *Would make an approved UOW-2 upload silently skip work, contradicting FR14.*

[Answer]:A

**Mechanism improved after answering, 2026-09-06.** The answer's intent is preserved exactly — the
repository absorbs the driver's limitation and callers stay unaware it exists. Only the mechanism
changed: instead of chunking into successive statements, the set is encoded once as a JSON array and
bound as a single parameter via `IN (SELECT value FROM json_each(?))`.

Measured on the same 200,000-row fixture, updating 100,000 ids took 2.74 s chunked across four
statements versus 0.18 s as one statement with three bound parameters. Set-passing also keeps
`ORDER BY` in SQL rather than repairing it in Go, and makes each update atomic by construction instead
of needing an explicit wrapping transaction. See NFRP-U3-03.

### Q2 — Reading the uncategorized set in one pass

`RecategorizeAll` currently calls `GetUncategorized()`, which loads every uncategorized transaction into
memory before any work begins.

- A. **(Recommended)** Keep the single read. Record observed memory at the 20,000-transaction target as
  evidence, consistent with how NFR-U3-SCALE-01 treats the mapping cache.
  *A transaction row is small, and the approved target is a one-off maintenance action on a local
  desktop database. Streaming would mean holding a read cursor open across the writes the same pass
  issues, which is a harder correctness problem than the one it solves.*
- B. Stream with a cursor and apply updates as rows are read.
- C. Page the read with LIMIT/OFFSET across repeated queries.
  *Rows shift between pages as they are categorized, so a page can be skipped.*

[Answer]:A

### Q3 — Cache locking and partial reload failure

Two caches now reload together through one entry point.

- A. **(Recommended)** One `sync.RWMutex` guarding both caches. Build both replacement sets first,
  swap them under a single write lock, and on any failure leave **both** caches untouched and return
  the error.
  *All-or-nothing is what makes the guarantee statable: rules are either fully refreshed or unchanged,
  never half-old. It also means a categorization pass can never see patterns from after a change beside
  mappings from before it.*
- B. Separate locks per cache, each reloading independently.
  *Allows a state where patterns are fresh and mappings stale, which is exactly the inconsistency the
  single decision function is meant to avoid.*
- C. One lock, but apply whichever cache reloaded successfully.

[Answer]:A

### Q4 — How the decision function reaches mapping data

Priority logic lives in one function; the SIC lookup lives in the mapping categorizer.

- A. **(Recommended)** Inject the SIC categorizer into `Categorizer` as a small interface with a single
  lookup method. The decision function calls text patterns first and that interface second.
  *Keeps priority in one readable place, keeps SIC logic out of the core categorizer, and lets the
  independent role substitute a fake lookup without a database.*
- B. Merge the mapping lookup directly into `Categorizer`.
  *Reopens the approved application-design decision to keep them separate.*
- C. Have the caller resolve the mapping and pass the result into the decision function.
  *Moves priority knowledge back out to every call site.*

[Answer]:A

## Execution Checklist

- [x] Read the approved UOW-3 functional design and NFR requirements.
- [x] Inspect the existing categorizer, repository query builders, and UOW-2 collaborator seam.
- [x] Measure the driver parameter ceiling rather than assuming it.
- [x] Record the blocking scale defect and its effect on the already-approved UOW-2 benchmark.
- [x] Receive answers to Q1 through Q4. (all A)
- [x] Analyze answers for conflicts and raise follow-ups if needed. No conflicts; no follow-up required.
- [x] Generate `nfr-design-patterns.md` and `logical-components.md` under
      `aidlc-docs/construction/transaction-categorization-integration/nfr-design/`.
- [x] Receive explicit NFR Design approval. (2026-09-06, user: "go ahead.")

## Out of Scope

- New configuration settings, background workers, queues, or filesystem abstractions.
- Any new dependency or schema change.
- Deferred independent findings F-04 and F-05.
