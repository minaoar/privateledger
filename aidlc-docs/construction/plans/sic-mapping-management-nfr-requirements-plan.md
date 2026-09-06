# NFR Requirements Plan — UOW-2 SIC Mapping Management

## Stage Inputs

- Approved UOW-2 Functional Design: `aidlc-docs/construction/sic-mapping-management/functional-design/`
- Approved UOW-1 NFR Requirements and tech-stack decisions (refined here, not restated)
- Project NFR1–NFR6 in `aidlc-docs/inception/requirements/requirements.md`
- Extension Configuration in `aidlc-docs/aidlc-state.md`: security-baseline **No**, property-based-testing **Partial**, resiliency-baseline **No**
- Carried forward from Functional Design approval: the mutation mutex spans a call into the UOW-3
  collaborator, so hold time is not bounded by UOW-2 alone and sits above UOW-1's SQLite busy timeout

## Execution Checklist

- [x] Read approved UOW-2 functional design artifacts
- [x] Read UOW-1 NFR requirements to identify what UOW-2 refines rather than repeats
- [x] Identify NFR areas where the design defers a decision to this stage
- [x] Generate clarification questions below
- [x] Collect answers to all questions
- [x] Analyze answers for ambiguity and raise follow-ups if needed
- [x] Generate `aidlc-docs/construction/sic-mapping-management/nfr-requirements/nfr-requirements.md`
- [x] Generate `aidlc-docs/construction/sic-mapping-management/nfr-requirements/tech-stack-decisions.md`
- [x] Present completion message and receive explicit approval (2026-09-06: "approved.")

## What Changed Since UOW-1

UOW-1 was a startup-time, single-threaded, insert-only path. UOW-2 introduces the first HTTP write
surface for mappings, the first deliberate concurrent state (BR-U2-44's mutex), a file the application
writes on its own (the timestamped backup), and a cross-unit call made while holding a lock. Each of
those is a new NFR surface rather than a restatement of UOW-1.

Two UOW-1 NFRs need explicit successors:

- **NFR-U1-TEST-04** made race-detector evidence conditional on the unit introducing concurrent state.
  UOW-2 does introduce it, so that condition is now met.
- **NFR-U1-SEC-03** bounded the startup seed at 10 MiB before parsing. The upload path needs its own
  statement of the same bound (BR-U2-39).

---

# Clarification Questions

## Question 1
BR-U2-44 serializes all mapping mutations with a service-owned mutex whose scope includes the UOW-3
recategorization handoff. A caller that arrives during a long merge must do something. What is the
required waiting behavior?

A) Bounded wait with a configured timeout; on timeout the request fails with a "busy, retry" response and no mutation

B) Unbounded wait — requests queue until the lock is free, matching SQLite's own busy-timeout behavior

C) Fail fast — if the lock is held, reject immediately without waiting

D) Bounded wait, but narrow the mutex scope so it is released before the UOW-3 handoff, leaving only the mutation under lock

X) Other (please describe after [Answer]: tag below)

[Answer]:A

## Question 2
BR-U2-39 requires the upload to enforce "the same size bound as the UOW-1 startup seed" — 10 MiB. Is
that the right limit for an interactive HTTP upload, given a 10 MiB CSV is roughly 700,000 mapping rows?

A) Keep 10 MiB, shared with the startup seed as one named constant

B) Keep one shared constant but raise it for both paths

C) Give the upload its own smaller limit, since it is interactive and a user-facing error is cheap

X) Other (please describe after [Answer]: tag below)

[Answer]:A

## Question 3
BR-U2-40 requires diagnostics bounded where they accumulate. UOW-1 logs at most 50 at startup. What
bound should the upload response carry?

A) 50, matching UOW-1's `maxSICSeedDiagnostics`

B) A larger bound such as 500, since the upload response is the user's only view of what failed

C) Bound by response size rather than count

X) Other (please describe after [Answer]: tag below)

[Answer]:A

## Question 4
Every successful upload writes `sic_mappings.backup-<timestamp>.csv` beside the database, and nothing
in the approved design removes them. Over time these accumulate without limit. What is required?

A) Retain the N most recent backups and delete older ones after a successful merge

B) Retain everything — unbounded growth is acceptable for a local single-user app, and deleting user data automatically is worse

C) Retain by age rather than count

D) Retain everything, but require the UI to surface the backup directory so the user can prune manually

X) Other (please describe after [Answer]: tag below)

[Answer]:B

## Question 5
What performance targets should be blocking acceptance evidence for UOW-2, measured on the same
recorded reference environment as UOW-1?

A) Merge upload, page load, and CSV export — full coverage of the three paths a user waits on

B) Merge upload only — the other two are trivially fast and cost benchmark maintenance for little value

C) Merge upload and page load; export is a streamed download and needs no target

X) Other (please describe after [Answer]: tag below)

[Answer]:B

## Question 6
The SIC mapping page server-renders every mapping with no pagination. At what mapping count must that
page still meet its target?

A) 10,000 mappings — comfortably above a realistic hand-maintained set

B) 100,000 mappings — consistent with UOW-1's seed and migration fixtures

C) 1,000 mappings — the realistic ceiling for a page a human curates

X) Other (please describe after [Answer]: tag below)

[Answer]:C

## Question 7
Property-based testing is **Partial**, with PBT-02, PBT-03, PBT-07, PBT-08, PBT-09 enforced. UOW-1
used `pgregory.net/rapid` for normalization and seed properties. Which UOW-2 properties should be
required?

A) Merge idempotency (BR-U2-21) and merge/omission invariants (BR-U2-19, BR-U2-20) — the properties the merge contract actually rests on

B) Those, plus CSV export/import round-trip (export then re-upload is a no-op)

C) Those in A and B, plus category-resolution properties for the name/ID tiebreaker rules (BR-U2-16 through BR-U2-18)

D) None — example-based tests are sufficient for UOW-2

X) Other (please describe after [Answer]: tag below)

[Answer]:A

## Question 8
UOW-1's independent review left F-04, F-05, and F-15 open and deferred. F-15 is in this unit's blast
radius: a `categoryRepo.GetAll()` failure is labelled `read_failed`, and UOW-2's upload path reuses
that same validation code and now surfaces the label to a user. How should UOW-2 treat it?

A) Fold F-15 into UOW-2 scope — the upload response makes the wrong label user-visible, so fix it here

B) Keep it deferred — it is a UOW-1 finding and reopening it widens UOW-2's scope

C) Keep it deferred but require UOW-2's NFRs to state that the label is known-inaccurate, so the independent reviewer does not re-report it

X) Other (please describe after [Answer]: tag below)

[Answer]:A

## Question 9
Is there any NFR area above — or one not asked about — where you want a stricter or looser requirement
than the UOW-1 precedent implies?

A) No, follow the UOW-1 precedent throughout

B) Yes (describe after [Answer]: tag below)

X) Other (please describe after [Answer]: tag below)

[Answer]:A

## Answer Analysis and Generated Artifacts

All nine answers are complete: A, A, A, B, B, C, A, A, A.

- Q5 B excludes blocking page/export timing; Q6 C supplies a 1,000-row usability fixture, not a latency threshold or capacity cap.
- Q9 carries forward the UOW-1 ten-second/100,000-row benchmark precedent as a proposed merge requirement, with the UOW-2 backup and checkpoint handoff included.
- Q7 A is recorded as an explicit unit-scoped generated-property selection; CSV round-trip and category-resolution examples remain required. The broader Partial extension configuration remains in place.
- Q1 defines bounded admission; concrete timeout/configuration and collaborator cancellation mechanics remain assigned to NFR Design as already required by Functional Design.
- Q8 brings F-15 into scope without claiming the code finding is fixed. F-04/F-05 remain deferred.

Generated both NFR artifacts. No unanswered preference remains; artifacts approved on 2026-09-06. No production or test files changed.

Stage approval received on 2026-09-06: "approved." NFR Requirements complete; proceed to NFR Design.
