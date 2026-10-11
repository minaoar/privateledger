# Independent Review Handoff — UOW-5 Rule-Sourced Recategorization

Production code is complete and authored by a different provider. This document carries everything
needed to review it against the **approved decisions**, not against a fresh reading of what the code
ought to do.

## Scope

Six production files and two templates. No schema change, no dependency change. `go.mod` and `go.sum`
untouched.

`internal/service/categorizer.go`, `internal/service/sic_categorizer.go`,
`internal/service/sic_mapping_service.go`, `internal/repository/transaction_repo.go`,
`internal/handler/category_handler.go`, `cmd/privateledger/web/templates/categories.html`,
`cmd/privateledger/web/templates/sic_mappings.html`.

## Read This First: Two Packages Do Not Compile Their Tests

This was decided deliberately before any code was written, as option A of the code generation plan.

| Package | Failing symbol | Reason |
|---|---|---|
| `internal/service` | `reviewCollaborator` in `sic_management_property_review_test.go:34` and `sic_management_review_test.go:35` | Implements `RecategorizeBySICCodes([]SICCode) (int, error)`; the interface now requires `Reexamine() (RecategorizationCounts, error)` |
| `internal/handler` | `reviewHandlerCollaborator` in `sic_mapping_review_test.go:234,240` | Same |

**Compilation breakage is unavoidable.** TD-U5-01 changes the collaborator interface, which is approved,
and a fake implementing the old signature stops satisfying it. There is no production-side way around it
that does not abandon an approved decision.

The alternative — shipping compatibility shims — was rejected because it means methods accepting
arguments they ignore, the exact smell FD-FQ1 rejected for this very interface.

Also expect **behavioural** failures once those compile again, in tests asserting that an existing
category is never revised. That assertion is what FR15 reverses.

`cmd/privateledger` was predicted to break and does not. It compiles and passes.

## Production Could Not Run `go test`, So It Ran the Binary

Every behaviour below was verified against a throwaway database on a non-default port. This is evidence,
not a substitute for your tests.

| Step | Result |
|---|---|
| Create mapping 5412 → Groceries | 3 categorized, manual untouched |
| Repoint 5412 → Travel | All 3 moved — the defect that created this unit |
| Create pattern `AIRLINE` → Dining | Moved a transaction off its mapping category |
| Delete the mapping | Its transactions became uncategorized; the pattern-matched one kept its category |
| Recategorize twice | moved 0, uncategorized 0 both times |
| Delete a category | Cascade first, then re-examination moved 2 |
| Import OFX with SIC 5412 | New row categorized; an existing row deliberately pointed at the "wrong" category **stayed there** |

## A Finding Production Did Not Fix

**Deleting a category silently converts a manual categorization into a rule-sourced one.**

Observed: a transaction manually set to *Dining* ended as *Groceries* with `category_source = 1` after
*Dining* was deleted.

The cause is **pre-existing and not introduced here**. `Categorizer.ClearCategory` selects every
transaction in the doomed category — its own comment says "both rule and manual" — and writes
`category_source = 0`. The manual marker is destroyed before any UOW-5 code runs.

What UOW-5 changes is the consequence. Before, such a transaction ended uncategorized and appeared on
the dashboard, where the user would notice. Now re-examination gives it whatever the rules say, so a
manual choice is replaced by a rule-sourced one **with nothing reporting it** — it is not counted as
manual-protected, because by then it is not manual.

Not fixed, because fixing it means deciding what a manual choice pointing at a deleted category should
become, and that is a product decision outside this unit's approved scope. Raised for the user with your
assessment.

## The Design-Decisions Record

Review against these. Anything the code does that these do not authorize is a finding; anything these
authorize is not, however it reads.

### Requirements amendment

R1 A all rule-sourced transactions are re-examined; **R1a A creation triggers it too**; R2 A a
transaction matching no rule becomes uncategorized; R3 A patterns and mappings both; R4 A automatic;
R5 A three counts; R6 A pre-existing rule-sourced rows are re-examined with everything else.

**FR15 is the governing requirement**, in the user's words: *"I want the same rule book to create the
same transaction categorization, irrespective of when the rules were created."*

### Functional Design

Q1 A full scope, no affected set; Q2 A category deletion is a rule change; Q3 A the manual count reports
transactions the rules would otherwise have moved; Q4 A one contract; Q5 A uncategorized writes NULL and
source 0; Q6 A import does not trigger; **FD-FQ1 A** replaces the collaborator contract.

### NFR Requirements and NFR Design

Q1 A the 1.5 s bound and a no-op pass that must be measurably faster; Q2 A the merge benchmark gains
transactions; Q3 A no deadline; Q4 A both properties; Q5 A one traversal. Then Q1 A one matcher; Q2 A a
batched clear; Q3 A additive result; Q4 A entry point on `Categorizer`; Q5 A materialize.

## The Part To Scrutinize Hardest

**`evaluate` has no manual guard.** That is required: FR16 reports how many manual transactions the rules
would otherwise have moved, and a matcher that stopped on manual could not answer it.

The manual-count call in `Reexamine` is the only caller reaching `evaluate` without a guard above it. It
sits in a branch that counts and `continue`s **before any write is reachable**.

That is a structural argument, not a guarantee. Please attack it directly, and treat NFR-U5-TEST-04 as
the thing that actually enforces manual protection.

## Amendments to Previously Approved Artifacts

Check these are honest narrowings, not rationalizations.

| Artifact | What changed |
|---|---|
| `requirements.md` FR7 | Two rule-permanence bullets struck |
| `requirements.md` FR14 | The "do not clear categories already assigned" bullet struck |
| `requirements.md` | **FR15** and **FR16** added |
| `stories.md` US-03 | Rule-sourced criterion replaced; manual criterion untouched |
| UOW-3 BR-U3-03 | Superseded for rule-sourced transactions |
| UOW-2 BR-U2-29 through BR-U2-31 | Affected-set definition superseded |
| UOW-3 NFR-U3-REL-01 | Three sentences superseded; the manual clause explicitly retained |
| UOW-5 BR-U5-02 | Read scope separated from write scope |
| UOW-2 NFR-U2-PERF-01 | Fixture must gain 20,000 transactions |

## Verification Obligations — DP-U5-08

1. **NFR-U5-TEST-01** — `rapid` order-independence: the same rule set built in permuted creation orders
   yields identical categorization. **This is the one property no example-based test can establish**, and
   it is FR15 stated executably.
2. **NFR-U5-TEST-02** — idempotence: a second consecutive pass writes nothing, three zero counts.
3. **NFR-U5-TEST-03** — every trigger in BR-U5-01, plus the two non-triggers: a description-only mapping
   edit, and an import.
4. **NFR-U5-TEST-04** — no `category_source = 2` row written by any trigger, including where the rules
   would have moved it.
5. **NFR-U5-TEST-05** — creating a pattern moves a transaction off a mapping-assigned category.
   Intended (BR-U5-20), and the case most likely to be mistaken for a defect.
6. **NFR-U5-TEST-06** — three counts correct; zeros reported as zeros; unchanged rows neither written
   nor counted.
7. **NFR-U5-TEST-07** — PERF-01's two bounds; **PERF-02 requires populating the merge fixture with
   20,000 transactions before its PASS means anything**; PERF-03 unchanged.
8. **NFR-U5-TEST-08** — `-race -short`.

## Ownership

You own review, all tests, and
`aidlc-docs/construction/rule-sourced-recategorization/code-review/independent-review.md`. Production
owns production fixes. **Production does not close this gate; your PASS does.**
