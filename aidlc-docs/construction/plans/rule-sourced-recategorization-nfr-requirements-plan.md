# NFR Requirements Plan — UOW-5 Rule-Sourced Recategorization

## Stage Inputs

UOW-5 Functional Design approved and pushed 2026-09-07 (`db82db4`). Binding: FR15, FR16, US-15, US-16,
BR-U5-01 through BR-U5-20, and FD-FQ1 A replacing UOW-2's collaborator contract.

Property-based testing remains **Partial**; security-baseline and resiliency-baseline remain opted out.

## Measured Starting State

Taken on the reference environment before writing this plan, by running the existing UOW-3 harness
rather than estimating.

| Measurement | Result |
|---|---|
| Full pass, 20,000 transactions, 1,000 mappings, 100 categories, all recategorized | **median 826.8 ms**, samples 789–893 ms, against NFR-U3-PERF-01's 5 s budget |
| Scoped pass, 100 affected codes, 2,000 matching transactions | **203.7 ms** |
| Import regression with mappings present | ratio 1.0429, inside the 10 % budget |

So the full pass costs roughly **4× the scoped pass** and lands comfortably under a second. That number
is what makes Q1 answerable with evidence instead of caution.

**Important qualifier:** the 827 ms figure includes *writing* all 20,000 rows. Under BR-U5-17 a
re-examination that changes nothing writes nothing, so the common steady-state case should be
substantially cheaper than this. 827 ms is close to the worst case, not the typical one.

## A Verified Gap: the Merge Benchmark Is Blind to This Unit

`sic_management_perf_review_test.go` — the harness behind NFR-U2-PERF-01's 100,000-row, ten-second
merge target — contains **no transactions at all**. Grepping it for `transaction` returns nothing.

That was correct when written: UOW-2's collaborator was a no-op, and UOW-3's was scoped to codes the
fixture had no transactions for. Under UOW-5 a merge triggers a full re-examination, so the benchmark
now omits the dominant cost and would report PASS while a real regression shipped.

## A Contradiction in the Approved Functional Design

Found while working out the read scope. Two rules approved yesterday disagree:

- **BR-U5-02**: "Re-examination considers **all** transactions with `category_source != 2`."
- **BR-U5-14**: "Counting manual protections requires evaluating rules for manual transactions."

Manual transactions cannot be both excluded and evaluated. The reconciliation is that **read scope and
write scope differ**: re-examination *reads* every transaction, and *writes* only non-manual ones.

BR-U5-02 needs amending to say that. This is a correction to my own artifact, not a new decision — Q3 A
already settled that the manual count is reported, and reporting it requires the read. The scale
consequence belongs here, which is why it surfaces at this stage: the read is over the whole table, not
the non-manual subset.

## Open Questions

Answer each by replacing the `[Answer]:` tag. One option per question is marked **(Recommended)**.

### Q1 — Performance target, now that this runs on every rule save

NFR-U3-PERF-01's five seconds was set for an explicit "Recategorize All" the user chose to trigger. The
same work now happens when saving a single pattern. Five seconds of blocking on a save is a different
proposition from five seconds on a bulk action.

- A. **(Recommended)** Two bounds at 20,000 transactions on the reference environment: a full
  re-examination where **every** transaction changes completes within **1.5 s**, and a re-examination
  where **nothing** changes must be measurably faster than that, demonstrating BR-U5-17 genuinely avoids
  writes.
  *1.5 s is grounded: 827 ms measured, worst sample 893 ms, so it is roughly 1.8× headroom over a real
  number rather than a guess. Tightening from five seconds is deliberate, because the work moved onto
  the interactive path. The second bound is stated as a relation rather than a constant because the
  no-write path does not exist yet and inventing a millisecond figure for it would be false precision.*
- B. Keep NFR-U3-PERF-01's five seconds unchanged.
  *One yardstick across units, and it would pass today with six times the headroom. But it would also
  pass if this unit made a rule save six times slower, which is the regression most worth catching.*
- C. Set no target and verify correctness only.
  *Discards a measurement already in hand.*

[Answer]:A

### Q2 — The blind merge benchmark

- A. **(Recommended)** Require NFR-U2-PERF-01's harness to run with a populated transaction table —
  20,000 transactions — keeping its ten-second budget. Recorded as a dated amendment to UOW-2's
  NFR-U2-PERF-01.
  *The budget needs no change: 1.41 s merge plus roughly 0.83 s re-examination is nowhere near ten
  seconds. What changes is that the benchmark starts measuring the thing that now dominates it. Leaving
  it empty means an approved blocking target that cannot fail for the right reason.*
- B. Leave NFR-U2-PERF-01 as it is and add a separate UOW-5 measurement.
  *Avoids touching an approved UOW-2 artifact, at the cost of leaving a benchmark in the suite whose
  PASS means less than it appears to.*
- C. Leave it unchanged.

[Answer]:A

### Q3 — The mapping-mutation gate hold

UOW-3's functional design Q1 B chose **no processing deadline**, so the gate is held for the full
duration of recategorization. That decision rested on the scoped pass. The hold is now a full pass.

- A. **(Recommended)** Keep no deadline. The measurement supports it: the hold grows from ~204 ms to
  ~827 ms worst case, still under a second.
  *The original reasoning was that a deadline describes a cancellation budget nobody needs at these
  timings, and the numbers still say so. Q1's bound is what keeps that true — if it fails, this reopens.*
- B. Introduce a processing deadline now that the hold has grown.
  *Adds a cancellation path and a partial-completion story for a sub-second operation.*

[Answer]:A

### Q4 — Property-based verification of FR15

FR15 is a claim about determinism, which is unusually well suited to a generated property. Testing is
**Partial**, so this is a scope decision rather than a default.

- A. **(Recommended)** Two properties. **Order independence**: build the same rule set in randomly
  generated creation orders and assert the final categorization is identical. **Idempotence**: a second
  consecutive re-examination writes nothing.
  *Order independence is FR15 stated as an executable claim, and it is exactly the property no
  example-based test can establish — the whole point is that no particular order is special. It is also
  US-15's sixth acceptance criterion.*
- B. Example-based tests for a few known orderings.
  *Cheaper, and it verifies the orderings someone thought of, which is the weaker half of the claim.*
- C. Idempotence only.

[Answer]:A

### Q5 — Reading manual transactions for the count

Per the contradiction above, the read is over the whole table while writes stay non-manual.

- A. **(Recommended)** One pass reads every transaction, evaluates all of them, writes only non-manual
  ones, and counts manual matches along the way.
  *One query, one traversal, and the manual count falls out of work already being done. A separate
  counting query would read the same rows twice and could disagree with the pass if a rule reload landed
  between them.*
- B. A separate query for the manual count after the main pass.
  *Keeps the write path reading only what it writes, at the cost of a second traversal and a window
  where the two can disagree.*

[Answer]:A

## Execution Checklist

- [x] Confirm Functional Design approval and re-verify the measured baseline by running the harness.
- [x] Check whether existing benchmarks still measure what they claim. NFR-U2-PERF-01 does not.
- [x] Identify the BR-U5-02 / BR-U5-14 contradiction before proposing a scope.
- [x] Receive answers to Q1 through Q5. (A, A, A, A, A)
- [x] Amend BR-U5-02 to separate read scope from write scope, dated. BR-U5-14 clarified alongside it.
- [x] Amend UOW-2 NFR-U2-PERF-01, dated. Fixture gains 20,000 transactions; the ten-second budget is unchanged.
- [x] Generate `nfr-requirements.md` and `tech-stack-decisions.md` under
      `aidlc-docs/construction/rule-sourced-recategorization/nfr-requirements/`.
- [x] Receive explicit NFR Requirements approval. (Approved 2026-09-07: "push it and continue to next stage".)

## Out of Scope

- Weakening manual protection.
- Recording which rule categorized a transaction. Declined at the requirements stage.
- Candidate findings C4-01, C4-02, C4-03; deferred findings F-04, F-05.
- New dependencies and schema changes. Infrastructure Design remains skipped.
