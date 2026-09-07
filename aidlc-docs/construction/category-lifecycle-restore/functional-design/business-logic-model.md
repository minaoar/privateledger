# Business Logic Model — UOW-4 Category Lifecycle and Mapping-File Integrity

## Scope and Boundary

UOW-4 makes a mapping CSV survive harmless formatting differences, and makes the failures it cannot
accept precise enough to repair in one edit. It changes what is *accepted* only for the header, and what
is *reported* everywhere else.

It adds no table, column, index, endpoint, page, or dependency. It creates, updates and deletes nothing
that the current code does not already. Stage answers governing this design: Q1 B, Q2 A, Q3 A, Q4 A,
FQ1 A.

## Header Matching

The header is compared after normalization rather than byte-for-byte.

1. Strip a leading UTF-8 byte-order mark from the first field, if present.
2. Trim surrounding whitespace from each column name.
3. Compare each name against the canonical spelling case-insensitively.

Column **order and count remain required**, and export continues to emit the canonical spelling
unchanged. A file whose columns are wrong, missing or extra is still rejected, and the diagnostic still
identifies the header rather than echoing its content.

The byte-order mark matters disproportionately: a spreadsheet on Windows writes one by default, so a
file the user round-tripped through Excel is rejected today with a message that gives no hint an
invisible byte is the cause.

## Category Resolution — What Does Not Change

`Category_Name` remains the only resolver. `Category_ID` remains a confirmation that never resolves a
row on its own. A row whose name and ID disagree is still rejected. A row carrying an ID with no name is
still rejected.

**Resolving by `Category_ID` when the name has gone stale was considered and declined.** Nothing in a
file distinguishes the two cases it would cover:

| Situation | Name resolves | `Category_ID` points at | Trusting the ID would be |
|---|---|---|---|
| Category renamed | no | the same category | correct |
| Category deleted and recreated | no | possibly a different category | wrong |

The second is exactly the hazard FR4 was written to prevent, and it is silent when it happens. UOW-4
therefore improves the diagnosis rather than widening what is accepted.

## Diagnostics

Three messages become specific enough to act on. All remain safe: they name categories that exist in
this database and the file's own column, never arbitrary file content.

| Situation | Today | After |
|---|---|---|
| `Category_Name` does not resolve | "Category_Name does not resolve" | Names the unresolved value, and when `Category_ID` refers to an existing category, that category's current name |
| `Category_Name` matches several categories case-insensitively | "Category_Name is ambiguous" | Names the colliding categories |
| Header mismatch | "CSV header does not match the required fields" | Identifies which column failed |

The first is the one that matters for a renamed category: told that `Groceries` is unknown and that
`Category_ID` 1 is now `Food`, the user makes one find-and-replace and the file imports.

## What This Deliberately Does Not Fix

A renamed category still makes a file fail to import. U4-01 is **mitigated, not resolved**. The residual
step is a single edit the user performs knowingly, chosen over trusting an ID that cannot be verified.

Recorded so it is revisited deliberately if it proves annoying, rather than rediscovered.

## Why the ID Fallback Was Declined — User Rationale, 2026-09-07

Recorded because "why B" is the question a future reader is most likely to reopen.

A CSV import is a **deliberate bulk operation**. The intended workflow is: download the current
mappings, edit the file, upload it. The file is a snapshot of the database it came from. Editing
categories in the UI midway through that workflow means the snapshot no longer describes the database,
and the correct response is to say so rather than guess which of two conflicting edits the user meant.

Under that model a stale category name is not a product defect the application should paper over. It is
a signal that the world moved underneath the file, and the user is the only one who can say what should
happen.

This reasoning is strongest for the **startup seed file**. FR4 makes `sic_mappings.csv` a long-lived,
hand-maintained interchange file, read on a database with no mappings — a reinstall or a new machine,
where `Category_ID` values are meaningless or belong to entirely different categories. Name-first is not
merely safer there; it is the only sane key, and trusting the ID would be actively wrong. The proposed
ID fallback would have been worst precisely where files live longest.

It is weakest for the automatically written backups, which the user never chose to download and to which
the download-first discipline never applied. That case stays narrow: a backup is written immediately
before every merge, so the one a user would restore is seconds old and a rename in that window is
improbable.

## Failure Ordering

Unchanged from UOW-2. Size rejection precedes validation; validation precedes any backup or mutation;
one invalid row rejects the whole file. Nothing in this unit mutates a mapping, so no ordering around
backup or merge changes.

| Failure | Mapping mutation | Result |
|---|---|---|
| Header cannot be normalized to the canonical columns | None | Invalid; header diagnostic |
| Category name unresolved, ambiguous, or disagreeing with its ID | None | Invalid; row diagnostic naming what to fix |
| Everything else | Unchanged from UOW-2 | Unchanged |

## Traceability

| Story | Logic |
|---|---|
| US-14 | Header normalization; the three specific diagnostics; no change to what is accepted beyond the header; no mutation on any rejected upload |
