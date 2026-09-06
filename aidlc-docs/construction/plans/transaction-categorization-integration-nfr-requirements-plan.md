# NFR Requirements Plan — UOW-3 Transaction Categorization Integration

## Stage Inputs

UOW-3 Functional Design approved 2026-09-06. UOW-1 and UOW-2 are complete with independent gates PASS.
Property-based testing remains configured **Partial**; security-baseline and resiliency-baseline remain
opted out.

Stage answers already binding on this unit: Q1 B (no recategorization deadline), Q2 B (warn plus split
counts), Q3 A (synchronous guarded reload), Q4 A (in-memory mapping cache), Q5 A (modal-created mapping
behaves as a mapping-page change), Q6 B (priority-matrix plus scoping properties).

## New NFR Surfaces Relative to UOW-1 and UOW-2

| Surface | Why it is new here |
|---|---|
| Categorization decision in the import hot path | Import now consults the mapping cache per transaction. UOW-1 measured a SIC-free import; nothing has yet measured import with mappings populated. |
| Scoped recategorization | The UOW-2 collaborator was a no-op that returned instantly. It now does real work while the mapping-mutation gate is held. |
| "Recategorize All" | Previously a text-pattern scan; it now also applies SIC across the entire uncategorized backlog. |
| Mapping cache residency | The full mapping set is held in memory for the process lifetime. UOW-2 bounds the CSV file at 10 MiB but places no ceiling on stored mapping count. |
| Deliberate concurrent state | Two guarded caches with synchronous reload. Race-detector evidence, conditional in UOW-1, is now unambiguously required. |

## Carried-Forward Precedents

- UOW-1: 100,000-row legacy migration within 5 s; 100,000-row seed within 10 s; SIC-free import may
  regress no more than 10 % in median elapsed time across at least five runs after one warm-up.
- UOW-2: 100,000-row merge within 10 s; 1,000-mapping page verified for usability with no latency gate.
- Recorded reference environment: Apple M1, 8 logical CPUs, 16 GiB RAM, macOS 15.5, go1.26.0.

## Contingency Recorded Before Measuring

Functional Design Q1 B chose **no processing deadline** for recategorization, so the mapping-mutation
gate is held for its full duration with no upper bound. That choice rested on UOW-2's merge benchmark
and on the scoped query being indexed — recategorization itself has never been measured against a large
transaction set.

If the Q1 measurement below lands unfavourably, revisiting Q1 is cheap at this stage and expensive after
code generation. This is recorded now so that outcome is a planned branch rather than a surprise.

## Open Questions

Answer each by replacing the `[Answer]:` tag. One option per question is marked **(Recommended)** with
the reason below it; they are suggestions, not defaults.

### Q1 — Recategorization performance target

This is the measurement the Functional Design deferred. It covers both scoped recategorization after a
mapping change and a full "Recategorize All".

- A. **(Recommended)** Carry the established 100,000-row / 10-second precedent. With 100,000
  uncategorized transactions and a populated mapping set, "Recategorize All" completes within 10 s, and
  a scoped recategorization over a realistic affected-code set completes well inside it, on the recorded
  reference environment.
  *Keeps one comparable yardstick across all three units, and sets the bar high enough that a pass is
  real evidence the unbounded gate hold from Q1 B is safe.*
- B. Use a smaller target matched to typical personal-finance volume, for example 20,000 transactions
  within 5 s.
  *Closer to real usage, but a pass would say less about the unbounded gate hold.*
- C. No numeric target; verify functional correctness only.
  *Leaves the Q1 B decision resting on inference rather than measurement.*

[Answer]:A

### Q2 — Import regression budget with mappings present

UOW-1's ≤10 % budget was measured on a SIC-free fixture. Import now performs a cache lookup per
transaction, and that path has never been measured with mappings loaded.

- A. **(Recommended)** Keep UOW-1's SIC-free check unchanged, and add a second fixture with mappings
  populated and SIC-bearing transactions, held to the same ≤10 % median budget against the SIC-free
  baseline.
  *The SIC-free check protects users who never adopt the feature; the new one protects users who do.
  Only the second can catch a regression introduced by the lookup itself.*
- B. Keep only the existing SIC-free regression check.
  *Would leave the newly added hot-path work unmeasured.*
- C. No import performance requirement for this unit.

[Answer]:A

### Q3 — Mapping cache residency

The cache holds every mapping for the process lifetime. UOW-2 caps the upload file at 10 MiB but sets no
ceiling on stored mapping count.

- A. **(Recommended)** No cache size limit. Record expected memory rather than enforcing a bound, and
  state that mappings are small and local.
  *A mapping is a short code, two short strings and a nullable ID. Even a pathological 100,000-mapping
  set is a few MB in a desktop process, so eviction would add cache-miss paths and a second lookup route
  to protect against a cost that does not arise.*
- B. Bound the cache and evict beyond the bound.
- C. Drop the cache and query per lookup, with a small LRU.
  *Reopens the approved Functional Design Q4 A decision.*

[Answer]:A

### Q4 — Which targets block Code Generation

- A. **(Recommended)** The recategorization and import targets are blocking; modal and page rendering
  are advisory and verified functionally.
  *Blocking exactly where a regression would be silent and hard to notice later, advisory where a
  problem is immediately visible on screen.*
- B. All targets blocking, including page and modal rendering.
- C. All targets advisory.

[Answer]:A

### Q5 — Verification depth

- A. **(Recommended)** Automated correctness examples, the generated properties already scoped by
  Functional Design Q6 B, race-detector evidence for the guarded caches, and benchmark evidence on the
  recorded reference environment.
  *Matches what UOW-1 and UOW-2 were held to, and race evidence is no longer optional now that the unit
  deliberately introduces shared mutable state.*
- B. Correctness examples and race evidence only, with no benchmark.
- C. Correctness examples only.

[Answer]:A

## Execution Checklist

- [x] Read the approved UOW-3 functional design, project NFRs, and the UOW-1/UOW-2 NFR precedents.
- [x] Identify the NFR surfaces new to this unit.
- [x] Record the Q1 contingency before any measurement is taken.
- [x] Receive answers to Q1 through Q5. (all A)
- [x] Analyze answers for conflicts and raise follow-ups if needed. No conflicts; no follow-up required.
- [x] Generate `nfr-requirements.md` and `tech-stack-decisions.md` under
      `aidlc-docs/construction/transaction-categorization-integration/nfr-requirements/`.
- [x] Receive explicit NFR Requirements approval. (2026-09-06, user: "proceed")

## Out of Scope

- New production dependencies. The existing Go, SQLite, standard-library and `rapid` test-only stack is
  retained; PBT-09 is satisfied by keeping `rapid` unchanged.
- Any change to deployment topology. Infrastructure Design remains skipped.
- Deferred independent findings F-04 and F-05.
