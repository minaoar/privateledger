# Functional Design Plan — UOW-2 SIC Mapping Management

## Unit Context

UOW-2 builds the local administration workflow on the UOW-1 SIC mapping model, normalization rules,
SQLite repository, and application-data-directory convention. It owns service-mediated CRUD, the
dedicated mapping page and APIs, CSV download, validated merge/upsert upload, and the affected-
SIC handoff consumed by UOW-3. It does not implement transaction modal behavior or the categorization
algorithm.

Assigned stories:

- US-04 — Manage SIC mappings in a separate configuration page
- US-05 — Prevent duplicate SIC mappings
- US-10 — Download current SIC mappings
- US-11 — Upload SIC mappings and merge with existing mappings

Technology constraint: preserve the existing Go, Gin, Bootstrap, HTMX/vanilla JavaScript, and SQLite
stack. No new production library, service, process, database, or client framework is planned.

## Question Category Evaluation

- Business Logic Modeling: applicable; CRUD, download, and backup-plus-merge workflows need precise outcomes.
- Domain Model: applicable; editable fields, mapping identity, nullable categories, and affected-code results need definition.
- Business Rules: applicable; normalized duplicates, empty uploads, edit semantics, and backup failures need explicit rules.
- Data Flow: applicable; CSV validation, backup, merge/upsert, and downstream affected-code handoff cross several boundaries.
- Integration Points: applicable only inside the monolith; UOW-3 supplies the categorization collaborator after UOW-2 establishes its contract.
- Error Handling: applicable; validation, backup, persistence, stale IDs, and missing records must produce deterministic outcomes.
- Business Scenarios: applicable; header-only files, concurrent-looking requests, category deletion, and no-op edits need coverage.
- Frontend Components: applicable; the dedicated page needs an interaction and display model using the existing UI stack.

## Functional Design Questions

### Question 1 — Description Management

Should the dedicated page allow users to edit both SIC description fields as well as the category?

A) Allow `SIC_Code`, `Description`, `Description_Detail`, and optional category to be entered on create and edited later (recommended; keeps UI and CSV state symmetrical)

B) Show descriptions read-only and allow only category changes

C) Hide descriptions from the management page and preserve them only through CSV workflows

X) Other (describe the exact editable and displayed fields after the tag)

[Answer]:A

### Question 2 — Editing the SIC Code

When editing an existing mapping, may the normalized SIC code itself be changed?

A) Yes; treat it as replacing the mapping key, reject conflicts, and report the new non-empty categorized code as affected while never clearing assignments for the old code (recommended)

B) No; keep the SIC code immutable and require delete plus create to change it

X) Other (describe the identity/update rule after the tag)

[Answer]:A

### Question 3 — Page Interaction Pattern

How should create and edit work on the dedicated SIC mapping page?

A) Use the existing Bootstrap modal and vanilla JavaScript/HTMX conventions, with a table for mappings and modal forms for create/edit (recommended)

B) Use inline editable table rows

C) Use separate create and edit pages

X) Other (describe the desired interaction after the tag)

[Answer]:A

### Question 4 — Mapping List Order and Scale

What initial list behavior should the page provide?

A) Sort by numeric SIC value ascending and show the complete local list without pagination; add client-side filtering only if the existing page conventions already support it (recommended)

B) Sort lexicographically by normalized SIC code and show the complete list

C) Add server-side pagination and search in this unit

X) Other (describe ordering, filtering, and pagination after the tag)

[Answer]:A

### Question 5 — Header-Only Upload

What should a valid CSV containing the required header but no mapping rows do?

A) Treat it as an intentional request to clear all mappings, after confirmation and successful backup (recommended; preserves download/upload round-trip semantics)

B) Reject it so upload can never remove all mappings

C) Accept it as a no-op and retain existing mappings

X) Other (describe the exact outcome after the tag)

[Answer]: X. the upload should be idempotent. If the uplaoded CSV contains new SIC code, it should be added. If the csv contains existing SIC codes, the CSV values should overwrite the database. Deleting of SIC codes should be done only by explicit user action - when the user selects SIC code in the SIC code view table and chooses delete. 

### Question 6 — Backup Failure

If the pre-overwrite backup cannot be durably written, should the database replacement proceed?

A) Abort before database mutation and return an error; overwrite is allowed only after backup succeeds (recommended)

B) Continue the database replacement but warn that no backup was created

X) Other (describe the failure and mutation behavior after the tag)

[Answer]:X. A prior backup failure should have no impact on the upload because the upload will not delete existing SIC codes. New/Existing SIC codes should be applied as answered for the question 6 above.  

### Question 7 — UOW-3 Recategorization Boundary

UOW-2 must identify mapping changes that require SIC-scoped recategorization, while UOW-3 owns the
actual categorization implementation. How should this unit handle that dependency?

A) Define and invoke a narrow injected collaborator contract; UOW-2 tests the affected-code handoff with a fake, and final transaction behavior is completed in UOW-3 (recommended; matches the approved dependency design)

B) Return affected SIC codes to the handler and postpone all collaborator invocation until UOW-3

X) Other (describe the cross-unit contract after the tag)

[Answer]:A

## Answer Analysis

- Q1-Q4 and Q7 select complete, internally consistent rules aligned with the existing stack and UOW boundaries.
- Q5 changes the approved FR9/US-11 upload behavior from whole-set replacement to normalized-code merge/upsert: uploaded codes are inserted or updated, while database mappings omitted from the file remain unchanged. This also changes the meaning of a header-only upload from "clear all" to "no-op."
- Q6 appears to refer back to the Q5 merge/upsert decision and says backup failure must not block upload. The approved FR9/US-11 contract currently requires a durable backup before replacement. Because uploaded rows may still overwrite existing values, clarification is required on whether a backup is attempted and how its failure is reported.
- These are valid product decisions, but they require explicit amendment of the approved requirements, story acceptance criteria, application design, and UOW-2 wording before Functional Design artifacts are generated.

Both follow-up answers were approved by the user's response "sounds good" after reviewing the recommended design. The amendments below therefore govern UOW-2: upload is atomic merge/upsert, omitted mappings are retained, a header-only file is a no-op, deletion remains explicit, and backup is best-effort with a prominent warning if it fails.

### Follow-up Question 1 — Confirm Upload Contract Amendment

Should the approved upload contract be amended everywhere to use merge/upsert semantics?

A) Yes; normalize and fully validate the whole file, insert new codes, overwrite matching codes, retain database codes omitted from the file, and treat a header-only file as a successful no-op (recommended based on your answer)

B) No; retain the approved whole-set replacement behavior, where omitted database codes are deleted and a header-only file clears all mappings

X) Other (describe precisely how uploaded, matching, and omitted codes behave after the tag)

[Answer]:A

### Follow-up Question 2 — Backup Attempt and Failure

Under merge/upsert semantics, what should happen with the pre-upload backup?

A) Attempt a full backup first, but if it fails continue the validated atomic merge/upsert and return a prominent backup warning with no backup path (recommended based on your answer)

B) Do not create an automatic backup for merge/upsert uploads

C) Require a successful backup before any uploaded value may overwrite an existing mapping

X) Other (describe whether backup is attempted and the exact failure behavior after the tag)

[Answer]:A

## Planned Functional Design Steps

The following steps execute after every answer is complete and ambiguity analysis finds no unresolved decision:

- [x] Validate all answers against the approved requirements, application design, UOW-1 contracts, and deferred findings.
- [x] Model mapping page load and service-mediated CRUD workflows, including normalized uniqueness and nullable category behavior.
- [x] Define create/update/delete inputs, results, identity semantics, validation failures, and affected-code rules.
- [x] Model CSV export, including fixed five-column ordering and header-only output.
- [x] Model upload validation, duplicate detection, category reference resolution, best-effort backup, atomic merge/upsert, and failure ordering.
- [x] Define the UOW-2-to-UOW-3 recategorization collaborator contract without implementing UOW-3 categorization logic.
- [x] Define API/page response models, user-visible validation reporting, confirmation behavior, and stable automation identifiers.
- [x] Define frontend component hierarchy, state, interaction flows, and existing-stack API integration.
- [x] Trace US-04, US-05, US-10, and US-11 plus FR8, FR9, FR11, FR13, and applicable FR14 behavior to rules and scenarios.
- [x] Generate `aidlc-docs/construction/sic-mapping-management/functional-design/business-logic-model.md`.
- [x] Generate `aidlc-docs/construction/sic-mapping-management/functional-design/business-rules.md`.
- [x] Generate `aidlc-docs/construction/sic-mapping-management/functional-design/domain-entities.md`.
- [x] Generate `aidlc-docs/construction/sic-mapping-management/functional-design/frontend-components.md`.
- [x] Validate unit boundaries, downstream contracts, local-only behavior, and avoidance of drastic technology changes.

## Planned Outputs

- `aidlc-docs/construction/sic-mapping-management/functional-design/business-logic-model.md`
- `aidlc-docs/construction/sic-mapping-management/functional-design/business-rules.md`
- `aidlc-docs/construction/sic-mapping-management/functional-design/domain-entities.md`
- `aidlc-docs/construction/sic-mapping-management/functional-design/frontend-components.md`
