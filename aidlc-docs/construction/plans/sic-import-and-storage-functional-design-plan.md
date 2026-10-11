# Functional Design Plan — UOW-1 SIC Import and Storage

## Unit Context

UOW-1 establishes the persistent and import-time foundation for the SIC feature. It owns SIC extraction, normalization, transaction persistence, the SIC mapping schema/repository contract, in-place migration, optional startup CSV seeding, and local-only behavior.

Assigned stories:

- US-01 — Import transactions with SIC data
- US-07 — Upgrade existing local databases safely
- US-08 — Initialize SIC mappings from an existing mapping file
- US-09 — Preserve local-only privacy

Downstream consumers:

- UOW-2 uses the SIC mapping model, normalization, schema, repository, and application-data-directory contracts.
- UOW-3 uses stored transaction SIC values, SIC-aware reads, and SIC-scoped transaction queries.

## Question Category Evaluation

- Business Logic Modeling: applicable; startup seeding needs a deterministic valid/invalid workflow.
- Domain Model: applicable; integer parsing makes leading-zero semantics and the representable SIC domain material.
- Business Rules: applicable; the requirements call for a reasonable maximum length but do not select one.
- Data Flow: applicable; startup CSV can contain mixed valid and invalid rows and needs an atomicity rule.
- Integration Points: no external integration is permitted; the local OFX parser, filesystem, repositories, and SQLite boundaries are already fixed. Category-name matching semantics still require confirmation at the local CSV/repository boundary.
- Error Handling: applicable; an invalid optional seed file must either abort startup, be skipped, or partially import.
- Business Scenarios: applicable; duplicate rows, leading zeros, invalid category references, a non-empty mapping table, and an absent file require explicit outcomes.
- Frontend Components: not applicable; UOW-1 has no user interface. Mapping UI belongs to UOW-2 and transaction modals belong to UOW-3.

## Functional Design Questions

### Question 1 — Leading-Zero Canonicalization

`ofxgo` parses `<SIC>` as an integer, so an OFX value such as `0111` reaches the application as `111`. How should mapping codes with leading zeros behave?

A) Canonicalize every valid SIC by removing leading zeros (`0111` becomes `111`), while canonicalizing all-zero input to `0`; this ensures CSV mappings can match parser output (recommended)

B) Reject mapping codes with leading zeros because they cannot exactly match parser output

C) Preserve leading zeros in mappings even though they will not match the same numeric value parsed from OFX

X) Other (describe the exact canonicalization and matching rule after the tag)

[Answer]:A

### Question 2 — Maximum SIC Length

What maximum should digits-only SIC validation enforce?

A) 19 digits, matching the positive `int64` domain accepted by the OFX parser; values outside that parser domain can never match imported transactions (recommended)

B) 4 digits, enforcing the traditional SIC code width

C) 10 digits, allowing bank extensions while applying a smaller defensive limit

X) Other (state the exact maximum and rationale after the tag)

[Answer]:A

### Question 3 — Invalid Startup CSV Atomicity

When `sic_mappings.csv` exists, the mapping table is empty, and one or more rows are invalid, what should startup import do?

A) Import nothing, log a per-row validation report, and continue application startup with an empty mapping table (recommended)

B) Import all valid rows, skip invalid rows, log each rejection, and continue startup

C) Import nothing and fail application startup

X) Other (describe the mutation and startup behavior after the tag)

[Answer]:A

### Question 4 — Category Name Matching

How should `Category_Name` from the startup CSV resolve against the unique category names stored in SQLite?

A) Exact case-sensitive match after trimming surrounding whitespace (recommended; matches the current repository/database semantics)

B) Case-insensitive match after trimming surrounding whitespace

C) Exact match without trimming whitespace

X) Other (describe normalization and matching after the tag)

[Answer]:B

## Answer Analysis

- Q1, Q2, and Q3 select explicit rules, but Q1/Q2 leave two numeric-domain edge cases: canonical `0` cannot match an imported transaction because `ofxgo` uses zero for an absent SIC, and a length-only 19-digit rule admits values larger than positive `int64`.
- Q4 selects case-insensitive matching. The current database uniqueness constraint is case-sensitive, so categories such as `Food` and `food` can coexist; a tie-breaking or rejection rule is required.
- The startup import remains all-or-nothing on validation errors and non-fatal to application startup.

### Follow-up Question 1 — Matchable Numeric Domain

After trimming and removing leading zeros, which numeric values should be accepted as SIC mapping codes?

A) Accept only values from `1` through the positive `int64` maximum (`9223372036854775807`); reject canonical `0` and overflow values because neither can match an imported transaction (recommended)

B) Accept `0` through the positive `int64` maximum even though `0` cannot match an imported transaction

C) Enforce only the 19-digit length and accept values above positive `int64`

X) Other (describe the exact inclusive range after the tag)

[Answer]:A

### Follow-up Question 2 — Case-Insensitive Category Ambiguity

If case-insensitive lookup finds multiple stored categories that differ only by case, how should `Category_Name` resolve?

A) Prefer an exact case-sensitive name match; otherwise accept a case-insensitive match only when exactly one category matches, and reject multiple matches as ambiguous (recommended)

B) Reject whenever multiple case-insensitive matches exist, even if one is an exact case-sensitive match

C) Select the lowest `category_id` among case-insensitive matches

X) Other (describe the deterministic resolution rule after the tag)

[Answer]:A

## Planned Functional Design Steps

The following steps will execute after all answers are complete and ambiguity analysis finds no unresolved decisions:

- [x] Validate all answers for completeness, consistency, and alignment with approved requirements and application design.
- [x] Model the OFX-to-domain-to-SQLite transaction SIC flow.
- [x] Model fresh-database creation and idempotent existing-database migration.
- [x] Model optional startup CSV discovery, empty-table gating, validation, category resolution, and persistence.
- [x] Define domain entities, value semantics, relationships, nullability, and lifecycle ownership.
- [x] Define SIC normalization, validation, uniqueness, deduplication exclusion, and category-reference business rules.
- [x] Define error outcomes for parser failures, migration failures, repository failures, absent files, invalid files, and skipped imports.
- [x] Trace US-01, US-07, US-08, US-09 and FR1-FR4/NFR1-NFR4 to functional rules and scenarios.
- [x] Generate `aidlc-docs/construction/sic-import-and-storage/functional-design/business-logic-model.md`.
- [x] Generate `aidlc-docs/construction/sic-import-and-storage/functional-design/business-rules.md`.
- [x] Generate `aidlc-docs/construction/sic-import-and-storage/functional-design/domain-entities.md`.
- [x] Validate the artifacts against UOW-1 boundaries and downstream UOW-2/UOW-3 contracts.

## Planned Outputs

- `aidlc-docs/construction/sic-import-and-storage/functional-design/business-logic-model.md`
- `aidlc-docs/construction/sic-import-and-storage/functional-design/business-rules.md`
- `aidlc-docs/construction/sic-import-and-storage/functional-design/domain-entities.md`

No `frontend-components.md` is planned because UOW-1 contains no frontend behavior.
