# NFR Requirements — UOW-3 Transaction Categorization Integration

## Status and Inputs

Approved by the user on 2026-09-06. Generated from the approved UOW-3 functional design, project NFR1–NFR6, the UOW-1 and UOW-2
NFR precedents, and stage answers Q1 A, Q2 A, Q3 A, Q4 A, Q5 A. 
These requirements cover categorization priority, scoped and full recategorization, the import hot
path, rule-cache residency and concurrency, and modal display. They introduce no persistence and no new
dependency.

Reference environment carried forward from UOW-1 and UOW-2: Apple M1, 8 logical CPUs, 16 GiB RAM,
macOS 15.5, go1.26.0, APPLE SSD AP1024Q NVMe.

## Performance and Scale

### NFR-U3-PERF-01 — Recategorization measurement (blocking)

**Amended 2026-09-06** on user direction: the transaction scale is 20,000, not the 100,000 originally
carried from the UOW-1 and UOW-2 precedents. Those precedents measured mapping rows and migration rows;
100,000 *uncategorized transactions* is not a realistic personal-finance volume, and a target nobody
will reach is weak evidence. The paired budget is five seconds, taken from the smaller-scale option
presented at the requirements stage.

With 20,000 uncategorized transactions and a populated mapping set, a full "Recategorize All" must
complete within five seconds on the recorded reference environment, measured from service entry to
returned result. Fixture construction, database creation, and browser transfer are excluded. Use one
warm-up run and the median of at least five uninstrumented runs, restoring the same pre-state for each.

A scoped recategorization over a realistic affected-code set must complete well inside the same bound
and be reported with its own measurement, since that is the path holding the mapping-mutation gate.

Record the transaction count, mapping count, category count, database size, and the proportion of
transactions carrying a SIC code. If the reference environment is unavailable, report the difference
rather than silently claiming equivalent evidence.

**This target exists to validate a Functional Design decision, not merely to bound latency.** Q1 B chose
no processing deadline for recategorization, so the mapping-mutation gate is held for its full duration
with no upper bound. A pass here is the evidence that the choice is safe. A failure is not only a
performance finding: it reopens the deadline decision, which is why the contingency was recorded in the
stage plan before any measurement was taken.

The amendment to 20,000 weakens that evidence and the trade is accepted deliberately. A pass at 20,000
transactions says the gate hold is safe at realistic volume; it does not establish behaviour for a user
far outside it. In exchange the target is one somebody might actually reach, so a pass means something
about real use rather than about a synthetic ceiling.

### NFR-U3-PERF-02 — Import regression budget (blocking)

Per Q2 A, two fixtures are required.

1. UOW-1's existing SIC-free import fixture, unchanged, may still regress no more than 10 % in median
   elapsed time across at least five runs after one warm-up.
2. A new fixture with mappings populated and SIC-bearing transactions, held to the same 10 % median
   budget measured against the SIC-free baseline on the same machine and database state.

The first protects users who never adopt SIC categorization. Only the second can detect a regression
introduced by the per-transaction mapping lookup this unit adds to the import path, which no existing
measurement covers.

### NFR-U3-SCALE-01 — Rule-cache residency

Per Q3 A, no cache size limit is imposed and no eviction is implemented. Both the text-pattern cache and
the SIC mapping cache hold their complete sets for the process lifetime.

Record observed memory for a 100,000-mapping cache as evidence rather than enforcing a ceiling. A
mapping is a short canonical code, two short strings, and a nullable integer, so even a pathological set
is a few megabytes in a desktop process. Eviction would introduce cache-miss paths and a second lookup
route to guard against a cost that does not arise at this scale.

This is not a mapping-count limit. Larger sets remain valid subject to local resources.

## Concurrency and Reliability

### NFR-U3-CON-01 — Guarded caches and synchronous reload

Both rule caches must be safe under concurrent read and write. Every reload is synchronous; no reload
runs in a detached goroutine. The existing `go h.categorizer.LoadPatterns()` must be removed.

That call is two defects at once: an unsynchronized write to a slice that in-flight requests read, and
an ordering bug in which a user can add a pattern and immediately recategorize against a cache that has
not refreshed. Both must be gone.

A single exported reload entry point refreshes both caches, so a caller cannot refresh one and forget
the other. Race-detector evidence is required and is no longer conditional, because this unit
deliberately introduces shared mutable state.

### NFR-U3-REL-01 — Categorization correctness and preservation

One decision function serves import, "Recategorize All", and scoped recategorization; no entry point
re-implements matching. Text patterns are tried first and a SIC mapping is consulted only when none
matched. A mapping with an empty category assigns nothing.

`category_source = manual` is never overwritten by any automatic path, and an existing category is never
revised automatically. Scoped recategorization considers only currently uncategorized transactions whose
SIC code is in the supplied affected set. Deleting a mapping never clears categories already assigned.

**Amended 2026-09-07 by UOW-5** (FR7 as amended, FR15). The three sentences after the first are
superseded. An existing *rule-sourced* category **is** revised automatically when the rules change;
scoped recategorization considers rule-sourced transactions as well as uncategorized ones; and deleting
a mapping does re-examine the transactions it had categorized.

**The first clause is unchanged and still binding**: `category_source = manual` is never overwritten by
any automatic path. That guarantee survives UOW-5 intact and is the one thing this amendment does not
touch.

The paragraphs below on collaborator failure and rollback are also unchanged.

A collaborator failure after a committed mapping change remains a committed-with-warning result under
BR-U2-45. A mapping change is never rolled back because categorization failed afterwards.

### NFR-U3-REL-02 — Counts are truthful

"Recategorize All" reports pattern-assigned and SIC-assigned counts separately alongside the processed
total, and `PatternCategorizedCount + SICCategorizedCount` must always equal `CategorizedCount`. A count
is claimed only after the corresponding database update commits.

## Privacy and Safety

### NFR-U3-SEC-01 — Local-only processing and safe output

All categorization, mapping lookup, and cache state remain in the local process. No external service,
telemetry, or cloud lookup is introduced. Parameterized SQL and existing foreign keys are preserved.

Descriptions and category names reaching the page are rendered through template escaping or DOM
`textContent`. Structured logging records safe operation, count, and outcome fields; it never records
transaction details, financial amounts, or complete mapping sets.

## Usability and Maintainability

### NFR-U3-UX-01 — Truthful, accessible feedback (advisory)

Per Q4 A this is verified functionally rather than against a numeric gate.

"Recategorize All" warns before running that SIC mappings will now also be applied, and reports the
split counts afterwards. Both transaction modals show the SIC code with description fallback and render
cleanly when the transaction has no SIC code. Modal-created mappings require a selected category and
state that other uncategorized transactions with that code will also be categorized. Submissions are
disabled while in flight and always restored. Transport failures report an unknown outcome rather than a
definite failure.

### NFR-U3-MAINT-01 — Architecture and naming

`handler -> service -> repository -> SQLite` is preserved, with constructor injection in `main.go` and
no global or singleton. The single binary, embedded assets, and no-CGO build are unchanged. SIC-assigned
categories reuse `category_source = rule` rather than introducing a fourth source value.

Any new page-level JavaScript function name must be verified against the globals declared in
`cmd/privateledger/web/static/js/app.js`. `layout.html` loads `app.js` after page content, so a
colliding name is silently overwritten at runtime — the defect recorded as U2-F09, where mapping
deletion could not work at all while the markup looked correct.

## Verification and Ownership

### NFR-U3-TEST-01 — Independent examples (blocking)

The independent provider must cover the full priority matrix: manual preservation, existing-category
preservation, text-pattern-wins-over-SIC, SIC-applies-when-no-pattern-matched, empty-category mapping
assigning nothing, missing mapping assigning nothing, and transactions with no SIC code.

Also required: that `RecategorizeAll` and `RecategorizeByCategory` route through the decision function
so SIC applies there; scoped recategorization touching only affected uncategorized transactions; the
split counts summing to the total; cache reload ordering after a rule change; modal SIC display
including the description fallback and the no-SIC case; modal-created mappings requiring a category and
creating no text pattern; and that a modal-created mapping recategorizes other matching uncategorized
transactions.

### NFR-U3-TEST-02 — Generated properties (blocking)

Per functional design Q6 B, two property families are required with `pgregory.net/rapid`, retaining
shrinking and a recorded replay seed.

1. **Priority matrix.** Over generated transactions, patterns, and mappings, the decision function must
   never overwrite a manual or existing assignment, must prefer a matching text pattern over any
   mapping, and must assign nothing when the only match is an empty-category mapping.
2. **Recategorization scoping.** Only currently uncategorized transactions whose SIC code is in the
   affected set may change; every other transaction is byte-identical afterwards.

The scoping property is the one protecting data the user categorized by hand, and is the reason
generated coverage is required rather than examples alone.

### NFR-U3-TEST-03 — Race evidence (blocking)

A full race-detector run must pass with zero reports, exercising concurrent categorization against a
cache reload. Race mode is run separately from performance acceptance.

### NFR-U3-TEST-04 — Benchmark evidence (blocking)

NFR-U3-PERF-01 and NFR-U3-PERF-02 must be measured and recorded with fixture composition, run values,
median, and the environment. Per Q4 A these are blocking; NFR-U3-UX-01 is advisory and verified
functionally.

### NFR-U3-TEST-05 — Cross-provider ownership

Production code and verification tests remain owned by different providers in separate sessions. The
independent review must report PASS with required tests passing and no blocking or high findings before
Code Generation completes.

## PBT Compliance

Partial mode. PBT-09 satisfied by retaining `pgregory.net/rapid v1.1.0` unchanged. PBT-02/03 scope is
set by functional design Q6 B and restated in NFR-U3-TEST-02. PBT-07/08 generator, shrinking, and replay
requirements carry into execution. Remaining PBT rules stay advisory.
