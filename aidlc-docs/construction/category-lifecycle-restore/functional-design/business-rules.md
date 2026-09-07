# Business Rules — UOW-4 Category Lifecycle and Mapping-File Integrity

## Header Matching

- BR-U4-01: The header is matched after normalization, not byte-for-byte.
- BR-U4-02: A leading UTF-8 byte-order mark on the first header field is stripped before comparison.
- BR-U4-03: Surrounding whitespace is trimmed from each header column name before comparison.
- BR-U4-04: Header column names are compared case-insensitively.
- BR-U4-05: Column order and column count remain required. A file with reordered, missing or extra
  columns is still rejected.
- BR-U4-06: Export continues to emit the canonical header spelling. Tolerance on input never changes
  output.
- BR-U4-07: A header that cannot be normalized to the canonical columns is rejected with a diagnostic
  identifying the header, never echoing its content.

## Category Resolution — Unchanged

- BR-U4-08: `Category_Name` remains the only resolver; `Category_ID` never resolves a row alone.
  BR-U2-16 through BR-U2-18 stand unchanged.
- BR-U4-09: Resolving by `Category_ID` when the name has gone stale is explicitly declined. A file
  cannot distinguish a rename, where the ID is correct, from a delete-and-recreate, where it may point
  at a different category, and the failure would be silent.
- BR-U4-10: This unit changes what the user is told, not what is accepted, outside the header.

## Diagnostics

- BR-U4-11: When `Category_Name` does not resolve, the diagnostic names the unresolved value, and when
  `Category_ID` refers to an existing category, that category's current name.
- BR-U4-12: When `Category_Name` matches more than one category case-insensitively, the diagnostic names
  the colliding categories.
- BR-U4-13: Diagnostics name categories that exist in this database and the file's own column. They
  never echo arbitrary file content, preserving NFR-U2-SEC-01.

  **Amended 2026-09-07 at NFR Requirements (Q1 A, NFR-FQ1 A).** This rule was written before the stage
  that decided how BR-U4-11's "names the unresolved value" would be reconciled with NFR-U2-SEC-01, and
  as written it forbids what BR-U4-11 requires. The reconciliation is that *arbitrary* now carries the
  weight: a diagnostic may echo the file-supplied `Category_Name`, but only after truncation to 64 runes
  and control-character replacement through one shared helper, per NFR-U4-SEC-01.

  The amendment also corrects a false premise. This rule treated database-sourced names as inherently
  safe. They are not: `category.name` is `TEXT NOT NULL UNIQUE` with no length constraint and
  `CreateCategory` validates no length, so a category name is unbounded user-controlled text that merely
  happens to live in the database. The same bound therefore applies to every echoed name regardless of
  source, and the ambiguity diagnostic is additionally capped at three names plus an "and N more" count.
- BR-U4-14: Diagnostics remain bounded by the existing shared cap, and `RejectedRows` remains
  authoritative when truncation occurs. BR-U2-40 stands unchanged.

## Category Name Uniqueness

- BR-U4-15: Category names remain unique case-sensitively. No schema migration is introduced.
- BR-U4-16: A case-insensitive collision is reported, never resolved by choosing one. Assigning
  transactions to a category the user did not pick is worse than rejecting the row.

## Scope Constraints

- BR-U4-17: No mapping is created, updated or deleted by any path this unit changes. A rejected upload
  mutates nothing, as before.
- BR-U4-18: No restore capability, replacement semantics, or bulk deletion is introduced.
- BR-U4-19: No table, column, index, endpoint, page, route or dependency is added.
- BR-U4-20: U4-01 is recorded as mitigated rather than resolved. A renamed category still makes a file
  fail to import; the failure becomes repairable in one edit.
