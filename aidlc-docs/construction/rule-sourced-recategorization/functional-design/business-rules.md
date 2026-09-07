# Business Rules — UOW-5 Rule-Sourced Recategorization

## Triggers and Scope

- BR-U5-01: Re-examination is triggered by any rule change — a text pattern or SIC mapping created,
  changed or deleted, and a category deleted.
- BR-U5-02: Re-examination **reads every transaction** and **writes only those with
  `category_source != 2`**. There is no scoped variant, and no caller supplies a scope.

  **Amended 2026-09-07 at NFR Requirements (Q5 A).** As first written this rule said re-examination
  considers only transactions with `category_source != 2`, which contradicted BR-U5-14: manual
  transactions cannot be both excluded and evaluated, and FR16's manual count requires evaluating them.
  Read scope and write scope differ, and the rule now says so.

  This is a correction, not a new decision — the requirements stage had already settled that the manual
  count is reported. Its consequence is one of scale, and it is why the read covers the whole table
  rather than a subset.
- BR-U5-03: Importing transactions does not trigger re-examination. Import categorizes what it brings in
  and leaves existing transactions alone.
- BR-U5-04: A description-only SIC mapping edit triggers nothing, because it cannot alter any
  categorization. BR-U2-30's other exclusions do not survive; see BR-U5-13.

## Decision

- BR-U5-05: `category_source = 2` is never written by re-examination. This guarantee is unchanged from
  BR-U3-02 and is not weakened anywhere in this unit.

  **Scope clarified 2026-09-07 after independent finding U5-R-F05.** Re-examination never overwrites a
  manual assignment. Deleting a category is a separate act by the user, and it clears every assignment
  in that category, manual ones included; the resulting transaction is uncategorized and re-examination
  may then claim it like any other. See FR7 as amended and BR-U5-08 — a transaction with no category
  always carries `category_source = 0`, and this unit introduces no exception to that.
- BR-U5-06: A re-examined transaction is evaluated **as if it had no category**. BR-U3-03's
  existing-category guard does not apply during re-examination; it still applies to import.
- BR-U5-07: Text patterns are evaluated first, in their existing order, first match wins. A SIC mapping
  is consulted only when no pattern matched and the transaction carries a code. A mapping with an empty
  category assigns nothing. Unchanged from BR-U3-04 through BR-U3-06.
- BR-U5-08: A re-examined transaction matching no rule is written `category_id = NULL`,
  `category_source = 0` — the same representation of "uncategorized" that category deletion already
  writes. No second representation is introduced.

## Ordering

- BR-U5-09: When a category is deleted, the database cascade completes first — its patterns are deleted
  and its mappings emptied — and rules are reloaded **after** the cascade. Re-examination therefore
  evaluates against the rule set that remains, never the one being removed.
- BR-U5-10: Rules are reloaded before every re-examination, and one pass sees one rule generation
  throughout. This preserves NFR-U3-CON-01's atomic publication.
- BR-U5-11: A rule change is never rolled back because re-examination failed afterwards. BR-U2-45's
  committed-with-warning result stands unchanged.

## Contract

- BR-U5-12: One re-examination entry point serves every trigger. Pattern changes and mapping changes do
  not take separate paths.
- BR-U5-13: The collaborator contract takes **no scope argument** and returns the three FR16 counts.
  This supersedes UOW-2's `RecategorizeBySICCodes(sicCodes)` and the parts of BR-U2-29 and BR-U2-30 that
  define an affected set. BR-U2-31's invocation shape survives: one call per CRUD change, one per
  upload.

## Reporting

- BR-U5-14: Counting manual protections requires evaluating rules for manual transactions. That
  evaluation **must never write**. It exists only to produce the count. Per Q5 A it happens in the same
  single traversal as the rest of the pass, so the count cannot disagree with the writes beside it.
- BR-U5-15: A rule change reports three counts: transactions moved to a different category,
  transactions that became uncategorized, and manual transactions the rules would otherwise have moved.
- BR-U5-16: Zero counts are reported as zero, never omitted. A rule change that moved nothing says so.
- BR-U5-17: A transaction whose re-examined category equals its stored category is not written and is
  not counted as moved.

## Determinism

- BR-U5-18: Re-examination is idempotent. Running it twice changes nothing the second time.
- BR-U5-19: The categorization of a transaction is a function of the current rule set and the user's
  manual assignments only. It does not depend on the order rules were created, changed or deleted, nor
  on the order transactions were imported. This is FR15 restated at unit scope.
- BR-U5-20: Because patterns outrank mappings, creating a text pattern may move transactions away from a
  category a SIC mapping assigned. This is intended, not incidental; under BR-U5-19 the alternative
  would make the outcome depend on creation order.

## Superseded Rules

Stated here so a reader of an earlier unit does not apply a rule this one replaces.

| Rule | Status |
|---|---|
| BR-U3-03 — an existing category is never revised automatically | Superseded for rule-sourced transactions by BR-U5-06. Still true for import |
| BR-U2-29, BR-U2-30 — affected-set definition and exclusions | Superseded by BR-U5-02 and BR-U5-13, except the description-only exclusion retained as BR-U5-04 |
| BR-U2-31 — invocation shape | Retained. Only the argument changes |
| BR-U3-02 — manual is never overwritten | **Retained and unchanged.** Restated as BR-U5-05 because three of the rules above sit beside it |
