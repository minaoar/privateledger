# Functional Design Plan — UOW-4 Category Lifecycle and Mapping-File Integrity

## Scope Correction — 2026-09-07

This plan was rewritten after the user questioned whether a restore capability was warranted. It was
not, and the earlier version's scope came from a judgement of mine rather than from the finding.

When U4-01 was admitted I added a section titled "The adjacent gap", observing that no story covers
restoring from a backup, and concluded that this "is the reason UOW-4 exists rather than this being a
one-line fix". That observation is true but it is a *separate want* from the defect. The defect is that
a stale category name makes a mapping file unusable. Carrying my framing forward produced a plan with an
explicit restore action, replace-versus-merge semantics, and a question about deleting mappings the
backup did not contain — none of which the finding requires.

**Removed from scope:** the restore action, restore semantics, and the extras question. Nothing in this
unit deletes anything.

**Retained:** the two findings admitted before the 2026-09-06 scope freeze, fixed as narrowly as each
allows.

If a restore capability is wanted, it should be proposed on its own merits as new work, not inherited
from this bug.

## Stage Inputs

UOW-1, UOW-2 and UOW-3 are complete with independent gates PASS. UOW-4's admitted findings:

- **U4-01** — a category rename makes previously exported mapping files, including the backups written
  before every merge, fail to import.
- **U4-02** — the CSV header rejects lowercase, mixed case, stray whitespace, and a UTF-8 BOM.

Technology constraint: existing Go / Gin / Bootstrap / HTMX / vanilla JavaScript / SQLite stack. No new
production dependency, no schema change.

## Verified Starting State

Re-confirmed against the current tree on 2026-09-07.

| Observation | Evidence |
|---|---|
| A rename makes a previously downloaded export fail to import | Reproduced: HTTP 422, `category_not_found`, whole file rejected |
| Live categorization is unaffected by a rename | Mappings and transactions key on `category_id`; the name is only joined for display |
| Body category resolution is already case-insensitive | `resolveSICCategory` tries exact, then folded |
| The header comparison is exact per column | `matchesSICMappingHeader` |
| Header rejects lowercase, mixed case, a space after a comma, and a UTF-8 BOM | All four reproduced |
| `category.name` is `TEXT NOT NULL UNIQUE`, case-sensitive, so `Food` and `food` can coexist | `schema.sql`; both creations returned 201 |

**Unverified:** whether delete-and-recreate produces a name/ID disagreement. A test of it merged
successfully, but only because SQLite reissued the same rowid to the recreated category. With other
categories present the ID would differ. Treat as unconfirmed.

## The Constraint That Makes Q1 a Real Decision

The obvious fix for U4-01 — fall back to `Category_ID` when `Category_Name` does not resolve —
reintroduces exactly the hazard FR4 was written to prevent.

| Scenario | Name resolves? | `Category_ID` points at | ID fallback is |
|---|---|---|---|
| Category renamed | no | the same category | correct |
| Category deleted and recreated | no | possibly a different category | wrong |

Nothing in the file distinguishes the two. That is why FR4 made `Category_Name` the only resolver and
`Category_ID` a confirmation that never resolves alone. Any fix has to decide what to do in that
ambiguity rather than pretend it is absent.

## Open Questions

Answer each by replacing the `[Answer]:` tag. One option per question is marked **(Recommended)**.

### Q1 — When the name no longer resolves but the ID does

- A. **(Recommended)** Resolve by `Category_ID` and report it per affected row, for example
  *"row 2: Category_Name 'Groceries' no longer exists; matched by Category_ID to 'Food'"*. Validation
  runs before any mutation, so the disclosure appears in the same response as the result.
  *Makes your own backup importable again, which is the actual complaint, while keeping FR4's intent:
  the ID never resolves silently. The residual risk is that the warning goes unread, and the case it
  would matter for — a category deleted and recreated between export and import — is narrow next to the
  rename case you actually hit.*
- B. Keep rejecting the row, but make the message actionable, for example
  *"Category_Name 'Groceries' not found; Category_ID 1 currently refers to 'Food'"*.
  *Zero risk of resolving to a wrong category, and the user is told exactly how to repair the file. But
  the file still does not import without hand-editing, so it arguably does not fix the finding.*
- C. Resolve by `Category_ID` silently, with no report.
  *Reintroduces the FR4 hazard with nothing to catch it. Listed for completeness; I would not choose it.*

[Answer]:B

### Q2 — How tolerant should the CSV header be?

- A. **(Recommended)** Strip a UTF-8 BOM, trim surrounding whitespace per column, and compare
  case-insensitively. Column order and count stay required.
  *Covers every failure actually observed, including Excel's BOM, without loosening what the file must
  contain. Diagnostics keep naming the failing column rather than echoing file content.*
- B. The above, plus accept any column order by matching on name.
  *More tolerant, but accepts a shape the documented contract does not describe, and export still emits
  one fixed order.*
- C. Strip the BOM only.
  *Fixes the Excel case; lowercase and stray whitespace keep failing.*

[Answer]:A

### Q3 — Category names that differ only by case

`Food` and `food` can both exist. With both present, a CSV naming `FOOD` matches two categories and is
rejected as ambiguous — correctly, since choosing one silently would categorize transactions against a
category nobody picked. "Never fails" has a real limit here.

- A. **(Recommended)** Keep case-sensitive uniqueness and name the colliding categories in the message
  so the user can resolve it.
  *The ambiguity is genuine and reporting it is the correct behaviour. No migration, no risk to an
  existing database.*
- B. Make category names case-insensitively unique, with a schema migration.
  *Removes the ambiguity at source, but an existing database may already hold such a pair and the
  migration would have to decide its fate — a data-loss decision about the user's own categories.*
- C. Resolve deterministically, for example exact match then lowest category ID.
  *Never fails, and silently assigns transactions to a category the user did not choose.*

[Answer]:A

### Q4 — Story shape

This unit still has no assigned stories; defining them is part of this stage.

- A. **(Recommended)** One story, **US-14 — Import mapping files that survive ordinary category and
  formatting changes**, covering both findings.
  *Both are the same user-visible promise: a file the application produced should still import. One
  story keeps that promise in one place.*
- B. Two stories, one per finding.
  *Cleaner traceability to U4-01 and U4-02, at the cost of splitting a single promise across two.*

[Answer]:A

## Execution Checklist

- [x] Confirm UOW-4 scope is frozen and read both admitted findings.
- [x] Re-verify the starting state against the current tree.
- [x] Remove the restore capability from scope and record why.
- [x] Receive answers to Q1 through Q4. (B, A, A, A)
- [x] Analyze answers for conflicts with approved requirements and raise follow-ups if needed. One
      conflict found between Q1 B and Q4 A; follow-up below.

## Follow-up — raised 2026-09-07

**Q1 B and Q4 A's story title disagree.**

Q4 A named the story "US-14 — Import mapping files that survive ordinary category and formatting
changes". Under Q1 A that title would have been accurate: a renamed category would no longer break the
file.

Q1 B keeps rejecting the row. So a file does **not** survive a category rename — it is rejected with a
precise, one-edit-fixable message instead. Only *formatting* variation is survived. Shipping the story
under its current title would promise something the unit does not deliver.

This also changes how U4-01 should be recorded at completion. It is **mitigated, not resolved**: the
diagnosis becomes precise enough to repair in a single find-and-replace, but the file still does not
import unaided. That is a legitimate outcome of the chosen option, and worth stating plainly rather than
marking the finding closed.

### FQ1 — Story title and U4-01 disposition

- A. **(Recommended)** Retitle to **US-14 — Accept cosmetic mapping-file variation and explain what must
  be fixed**, and record U4-01 as *mitigated* with the residual manual step named.
  *Describes what the unit actually delivers under Q1 B. Leaves U4-01's remaining friction visible
  instead of closed, so it can be revisited deliberately if it proves annoying in practice.*
- B. Keep the original title and record U4-01 as resolved.
  *Overstates the outcome: a renamed category still breaks the file.*
- C. Revisit Q1 and choose A there after all, making the original title accurate.

[Answer]:A

- [x] Amend `requirements.md` and `stories.md`, marked as UOW-4 amendments.
- [x] Promote the story or stories into `unit-of-work.md` and `unit-of-work-story-map.md`.
- [x] Generate the functional design artifacts under
      `aidlc-docs/construction/category-lifecycle-restore/functional-design/`.
- [x] Receive explicit Functional Design approval. (Approved 2026-09-07: "continue to next stage".)

## Out of Scope

- A restore capability, restore semantics, and any deletion of mappings. If wanted, propose separately.
- Any finding not admitted before the 2026-09-06 scope freeze.
- Deferred independent findings F-04 and F-05.
- New production dependencies and schema changes. Infrastructure Design remains skipped.
