# NFR Requirements — UOW-5 Rule-Sourced Recategorization

## Status and Inputs

Generated 2026-09-07 from the approved UOW-5 functional design, FR15 and FR16, and the UOW-1 through
UOW-4 NFR precedents. Stage answers Q1 A, Q2 A, Q3 A, Q4 A, Q5 A. Awaiting approval.

UOW-5 adds no table, column, index, dependency, page or route. What changes is scale and placement: an
operation that used to run over a scoped subset, on an action the user explicitly chose, now runs over
the whole table every time a rule is saved.

Reference environment, carried forward: Apple M1, 8 logical CPUs, 16 GiB RAM, macOS 15.5, go1.26.0,
APPLE SSD AP1024Q NVMe.

## Measured Baseline

Taken before these requirements were written, by running the existing UOW-3 harness.

| Operation | Median | Notes |
|---|---|---|
| Full pass, 20,000 transactions, all recategorized | **826.8 ms** | Samples 789–893 ms. Includes writing all 20,000 rows |
| Scoped pass, 2,000 matching transactions | **203.7 ms** | The path UOW-5 retires |
| 100,000-row merge (NFR-U2-PERF-01) | **1.41 s** | Against a ten-second budget, measured during UOW-4 review |

Every target below is derived from these numbers rather than chosen for roundness.

## Performance

### NFR-U5-PERF-01 — Re-examination bounds (blocking)

Per Q1 A, two bounds at 20,000 transactions on the reference environment.

**Worst case — every transaction changes: within 1.5 seconds.** Measured from entry into the
re-examination call to the returned result. Fixture construction, database creation and browser transfer
excluded. One warm-up run discarded, median of at least five uninstrumented runs, the same pre-state
restored for each.

1.5 s is roughly 1.8× the measured 826.8 ms median and 1.7× the worst observed sample. It is
deliberately **tighter than NFR-U3-PERF-01's five seconds**, and the reason is placement rather than
scale: five seconds was set for an explicit "Recategorize All" the user chose to trigger, and this same
work now happens when saving a single pattern. A budget that would tolerate a sixfold slowdown on an
interactive save is not a useful budget.

**Common case — nothing changes: measurably faster than the worst case**, and reported alongside it.

Stated as a relation, not a constant, on purpose. BR-U5-17 says a transaction whose re-examined category
equals its stored category is not written, so a no-op pass should skip the write entirely — but that
path does not exist yet, and inventing a millisecond figure for unwritten code would be false precision.
What the requirement does assert is testable and is the point: **if the no-op pass is not faster,
BR-U5-17 is not working**, whatever the absolute number.

### NFR-U5-PERF-02 — Merge benchmark must measure re-examination (blocking)

Per Q2 A, NFR-U2-PERF-01's harness must run with **20,000 transactions** present. Its ten-second budget
is unchanged and needs no relief: 1.41 s of merge plus roughly 0.83 s of re-examination is nowhere near
it.

This exists because the harness contains no transactions today. That was correct when written — UOW-2's
collaborator was a no-op and UOW-3's was scoped to codes the fixture had no transactions for. Under
UOW-5 a merge triggers a full re-examination, so an approved blocking target now omits the cost that
dominates it. **A benchmark that cannot fail for the right reason is worse than no benchmark**, because
its PASS is read as evidence.

Recorded as a dated amendment on NFR-U2-PERF-01 itself, so a reader of UOW-2 does not run the old shape.

### NFR-U5-PERF-03 — Import path unchanged

Import does not trigger re-examination (Q6 A at Functional Design), so NFR-U3-PERF-02's two-fixture
regression check applies unchanged and must continue to pass within its 10 % median budget.

If import timings move in this unit, something is triggering re-examination that should not be. That
makes this an early warning about correctness, not only about speed.

## Concurrency

### NFR-U5-CON-01 — Gate hold, still without a deadline

Per Q3 A, UOW-3's decision to hold the mapping-mutation gate for the full duration of recategorization,
with no processing deadline, stands.

The hold grows from roughly 204 ms to roughly 827 ms worst case. UOW-3's reasoning was that a deadline
describes a cancellation budget nobody needs at these timings, and the measurement still supports it.

**NFR-U5-PERF-01 is what keeps this true.** If the 1.5-second bound fails, this decision reopens, and it
should be reopened rather than the budget relaxed. That contingency is recorded now so a failure is a
planned branch rather than an argument had under pressure.

### NFR-U5-CON-02 — One rule generation per pass

BR-U5-10 requires one re-examination to see one rule generation throughout. UOW-3's atomic publication
under a single write lock is unchanged and must remain race-clean under `-race`.

BR-U5-09's ordering is part of this: on category deletion the database cascade completes first, and
rules are reloaded after it, so a pass never evaluates against rules that are being removed.

## Scale

### NFR-U5-SCALE-01 — Whole-table read

Per Q5 A the pass reads **every** transaction, including manual ones, and writes only non-manual ones.
The manual count required by FR16 falls out of the same traversal.

One traversal, not two. A separate counting query would read the same rows a second time and could
disagree with the writes beside it if a rule reload landed in between — which is the kind of
inconsistency BR-U5-10 exists to prevent.

Record the transaction count, the manual proportion, the mapping count and the database size with any
measurement, since the manual proportion changes how much of the read is write-eligible.

### NFR-U5-SCALE-02 — Write batching unchanged

`BulkUpdateCategory`'s `json_each` construction is retained and still required: a worst-case pass writes
every non-manual transaction, far past SQLite's measured 32,764-variable ceiling.

Only the *scoping* use of `GetUncategorizedBySICCodes` retires. The set-passing technique it proved is
what makes the write side viable at this scale.

## Reliability

### NFR-U5-REL-01 — Determinism as a runtime property

FR15 restated as an acceptance property: categorization is a function of the current rule set and the
user's manual assignments, never of the order rules were created or transactions imported.

Two consequences must hold at runtime, not merely in review:

- **Order independence.** The same rule set built in different creation orders yields identical
  categorization.
- **Idempotence.** A second consecutive re-examination writes nothing.

### NFR-U5-REL-02 — Manual protection

`category_source = 2` is never written by any re-examination path. Manual transactions are read and
evaluated only to produce FR16's count, and that evaluation must never write.

This is the one guarantee UOW-5 does not touch, and it is restated here because this unit removes the
guard beside it — BR-U3-03's existing-category stop — and a reader could take the pair as weakened
together.

**Scope clarified 2026-09-07 after independent finding U5-R-F05.** The guarantee is about *rules* not
revising a manual assignment. It does not survive the user deleting the category itself: that clears the
assignment along with everything else in the category, and the transaction becomes uncategorized and
re-examinable. See FR7 as amended. Manual assignments in existing categories are protected under every
trigger, which is what NFR-U5-TEST-04 verifies.

### NFR-U5-REL-03 — Failure isolation

A rule change is never rolled back because re-examination failed afterwards. BR-U2-45's
committed-with-warning result stands unchanged.

## Verification

Cross-provider ownership applies: production authors none of the tests below.

### NFR-U5-TEST-01 — Order-independence property (blocking)

Per Q4 A, using the existing test-only `pgregory.net/rapid`. Generate a rule set and a randomly permuted
creation order, apply the rules in that order, and assert the final categorization of every transaction
is identical across permutations.

**This is the one property no example-based test can establish**, because the whole claim is that no
particular ordering is special. It is also US-15's sixth acceptance criterion, stated executably.

### NFR-U5-TEST-02 — Idempotence property (blocking)

A second consecutive re-examination writes nothing and reports all three counts as zero.

### NFR-U5-TEST-03 — Trigger coverage (blocking)

Every trigger in BR-U5-01 re-examines: pattern created, changed, deleted; mapping created, changed,
deleted; category deleted; mapping upload. And the two non-triggers hold: a description-only mapping
edit, and an import, both leave existing categorizations untouched.

The non-triggers matter as much as the triggers. Q6 A was chosen because re-examining on import would
make the outcome depend on import order, so a test that catches it is testing FR15, not an optimization.

### NFR-U5-TEST-04 — Manual protection (blocking)

Across every trigger, no transaction with `category_source = 2` is written. Includes the case where the
current rules would have moved it, which is exactly the case FR16's third count reports.

### NFR-U5-TEST-05 — Priority and the sharp edge (blocking)

Patterns outrank mappings after re-examination as before it. Explicitly: creating a text pattern that
matches a transaction a SIC mapping had categorized **moves that transaction to the pattern's category**.
This is BR-U5-20 and US-15's last acceptance criterion — intended behaviour, and the case most likely to
be mistaken for a defect.

### NFR-U5-TEST-06 — Counts (blocking)

The three FR16 counts are correct across moves, uncategorizations and manual protections; zero counts are
reported as zero rather than omitted; and a transaction whose category does not change is neither written
nor counted as moved.

### NFR-U5-TEST-07 — Performance evidence (blocking)

NFR-U5-PERF-01's two bounds, NFR-U5-PERF-02's populated merge benchmark, and NFR-U5-PERF-03's unchanged
import regression. Report medians and fixture composition; if the reference environment is unavailable,
report the difference rather than claiming equivalent evidence.

### NFR-U5-TEST-08 — Race evidence (blocking)

`go test -race -short -count=1 ./...` passes. This unit adds no concurrency but changes what happens
under an existing lock, so the evidence is required rather than conditional.

## Traceability

| Source | Requirement |
|---|---|
| Q1 A; BR-U5-17; measured baseline | PERF-01 |
| Q2 A; the empty UOW-2 fixture | PERF-02 |
| Functional Design Q6 A; BR-U5-03 | PERF-03 |
| Q3 A; UOW-3 Functional Design Q1 B | CON-01 |
| BR-U5-09, BR-U5-10 | CON-02 |
| Q5 A; BR-U5-02 as amended, BR-U5-14 | SCALE-01 |
| BR-U5-13; UOW-3's measured parameter ceiling | SCALE-02 |
| FR15; BR-U5-18, BR-U5-19 | REL-01 |
| FR7 first bullet; BR-U5-05, BR-U5-14 | REL-02 |
| BR-U5-11; BR-U2-45 | REL-03 |
| Q4 A; US-15 | TEST-01, TEST-02 |
| BR-U5-01 through BR-U5-04 | TEST-03 |
| BR-U5-05; FR16 | TEST-04 |
| BR-U5-07, BR-U5-20; US-15 | TEST-05 |
| FR16; BR-U5-15 through BR-U5-17 | TEST-06 |
| Cross-provider project convention | TEST-01 through TEST-08 ownership |

## Extension Compliance

- **security-baseline**: opted out. N/A — this unit adds no new data exposure; diagnostics are unchanged
  and NFR-U4-SEC-01's bounds still govern any name echoed.
- **resiliency-baseline**: opted out. NFR-U5-REL-03 carries UOW-2's failure isolation forward regardless.
- **property-based testing (Partial)**: PBT-09 compliant, `pgregory.net/rapid v1.1.0` already in
  `go.mod`, no new dependency. PBT-02/03 satisfied by NFR-U5-TEST-01 and TEST-02 — FR15 is the clearest
  characterizable property this project has produced, since determinism is precisely a statement about
  all orderings. PBT-07/08 carry into verification. PBT-01, 04–06 and 10 remain advisory. No blocking
  PBT finding.

## Amendments to Approved Artifacts

| Artifact | Amendment |
|---|---|
| `rule-sourced-recategorization/functional-design/business-rules.md` BR-U5-02 | Read scope and write scope separated, resolving its contradiction with BR-U5-14 |
| `sic-mapping-management/nfr-requirements/nfr-requirements.md` NFR-U2-PERF-01 | Fixture gains 20,000 transactions; ten-second budget unchanged |

## Assigned to NFR Design

Design decisions, not unanswered preferences:

- How `decide` evaluates a transaction as if it had no category without weakening the manual guard, and
  how that mode is made hard to invoke accidentally from the import path.
- Where the single re-examination entry point lives, and how the category handler and the mapping
  service both reach it.
- How the three counts are accumulated in one traversal without a second query.
- Whether the whole-table read streams or materializes, given a worst-case write of every non-manual row.
