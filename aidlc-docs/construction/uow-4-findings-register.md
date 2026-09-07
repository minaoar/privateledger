# UOW-4 Findings Register — Category Lifecycle and Mapping-File Restore Integrity

Working register for findings admitted to UOW-4. This is not an approved design artifact; it collects
evidence so that UOW-4's Inception and Construction stages start from reproductions rather than memory.

**Status:** **SCOPE FROZEN 2026-09-06**, at UOW-3 Code Generation completion. Admitted findings: U4-01,
U4-02. Further findings need a scope amendment or a new unit.

## Admission Rule

| Finding type | Where it goes |
|---|---|
| Breaks approved behaviour of a shipped unit | Fixed **in that unit**, as U2-F09 was — not parked here |
| Contract amendment, missing capability, or an unexamined interaction between units | Admitted here |

A finding is admitted only with a reproduction or a cited artifact contradiction. "Something felt wrong"
is a prompt to investigate, not an entry.

## Admitted Findings

### U4-01 — A category rename makes previously exported mapping files and backups unrestorable

**Admitted** 2026-09-06. Founding finding for this unit.
**Reported by** the user, while reviewing the UOW-3 Code Generation plan.
**Severity** Medium for exports; higher for backups, because a backup's only purpose is restoration.

#### Reproduction

Verified against a temporary database on an isolated port, 2026-09-06.

1. Create category `Groceries` (id 1).
2. Create mapping `5412` -> category 1.
3. `GET /api/sic-mappings/download` returns:
   ```
   SIC_Code,Description,Description_Detail,Category_Name,Category_ID
   5412,Grocery store,,Groceries,1
   ```
4. Rename category 1 from `Groceries` to `Food` (HTTP 200).
5. Re-upload the file from step 3.

**Observed:** HTTP 422, `outcome: invalid`, `rejected_rows: 1`,
`row 2 Category_Name category_not_found — Category_Name does not resolve`. The entire upload is
rejected, because one invalid row rejects the whole file.

#### What is *not* affected

Live categorization is untouched. Transactions and mappings both store `category_id`; the category name
is never persisted and is only joined for display. After the rename in step 4, mapping `5412` still
resolves to category 1, now displayed as `Food`. Nothing needs re-running.

Category deletion is also handled correctly and is not part of this finding: `DeleteCategory` calls
`ClearCategory` first, resetting affected transactions to `category_id = NULL, category_source = 0`, so
they become genuinely uncategorized rather than orphaned with a stale source. The schema then nulls the
mapping's category via `ON DELETE SET NULL`.

#### Why it happens

FR4 deliberately made `Category_Name` authoritative and `Category_ID` merely confirmatory, so that a
stale ID after a delete-and-recreate cannot resolve to a valid but wrong category. BR-U2-18 makes an ID
without a resolvable name invalid.

That protection is correct for a user-authored file. It is wrong for a machine-generated backup taken
from this same database moments earlier, where the ID is provably consistent and the name is a
convenience column. Both are currently served by one code path.

| Source | Is the ID trustworthy? | Current treatment |
|---|---|---|
| User-authored CSV, possibly hand-edited or from another machine | No — the name must win | name-first |
| Application-written backup from this database | Yes — provably consistent | name-first |

#### The adjacent gap

**No story covers restoring from a backup.** US-11 mentions restore only as a hoped-for side effect of
upload. Backups are written before every merge, but restoring one was never designed. This is why the
finding had no owning unit, and it is the reason UOW-4 exists rather than this being a one-line fix.

#### Candidate directions

Not yet decided; UOW-4's own stages own the decision.

- A restore path that trusts `Category_ID` for application-generated backups while leaving user-authored
  uploads name-first exactly as FR4 requires. Needs a way to distinguish the two — a header marker, a
  separate endpoint, or the file's provenance.
- Cheaper interim: omit `Category_Name` from backups, or mark backups as ID-authoritative in a header
  line.
- Provisional story **US-14 — Restore mappings from an application backup**.

#### Artifacts implicated

`requirements.md` FR4; `business-rules.md` BR-U2-16 through BR-U2-18, BR-U2-23 through BR-U2-25;
`sic_mapping_service.go` `resolveSICCategory` and `writeBackup`; `stories.md` US-10 and US-11.

### U4-02 — CSV header rejects cosmetic variation, including Excel's byte-order mark

**Admitted** 2026-09-06.
**Reported by** the user: "while importing the csv file, the header is case sensitive. in general, all
csv content (including header and body) should be case insensitive and should not fail any operation."
**Severity** Medium. The BOM case is the sharpest: a file saved by Excel on Windows is rejected with an
error message that gives no hint why.

#### Verified behaviour

Tested against a temporary database on an isolated port, 2026-09-06. One category `Groceries` existed.

| Input | Result |
|---|---|
| Canonical header | 200 `merged` |
| Lowercase header `sic_code,description,…` | **422** `invalid_header` |
| Mixed-case header `Sic_Code,…` | **422** `invalid_header` |
| Header with a space after a comma | **422** `invalid_header` |
| UTF-8 BOM before a canonical header | **422** `invalid_header` |
| Body: `groceries` (lowercase) | 200 `merged` |
| Body: `GROCERIES` (uppercase) | 200 `merged` |

`matchesSICMappingHeader` compares each column with `!=`, so any difference in case, surrounding
whitespace, or a leading BOM byte fails the whole file.

#### Correction to the reported scope

The report says all CSV content is case-sensitive. The **body already is not**: `resolveSICCategory`
tries an exact match, then falls back to a case-insensitive one, so `groceries` and `GROCERIES` both
resolve today. SIC codes are digits and descriptions are free text stored verbatim, so case does not
apply to them either.

The gap is confined to the header. Recording that accurately matters — chasing case-insensitivity
through the body would be work with nothing to fix, and would miss the BOM, which is not a case problem
at all but fails for the same reason and is the one a real user is most likely to hit.

#### Where "should not fail any operation" has a genuine limit

`category.name` is `TEXT NOT NULL UNIQUE` with no `COLLATE NOCASE`, so uniqueness is case-sensitive and
two categories may differ only by case. Verified: creating `Food` and then `food` both returned 201.

With both present, a CSV naming `FOOD` cannot be resolved — it matches two categories. The upload is
rejected with `category_ambiguous`, and that rejection is correct: silently picking one would assign
transactions to a category the user did not choose.

So case-insensitive matching cannot be made unconditionally non-failing while the database permits
case-distinct category names. UOW-4 has to decide between:

- Making category names case-insensitively unique, which needs a migration and a decision about existing
  databases that already contain such a pair.
- Keeping case-sensitive uniqueness and accepting that ambiguity remains a legitimate rejection, while
  improving the message so the user is told which categories collided.

BR-U2-16's exact-match-wins rule already resolves the unambiguous half of this: an exact `food` still
matches `food` even when `Food` exists. Only a spelling that matches neither exactly is ambiguous.

#### Candidate directions

Not yet decided; UOW-4's own stages own this.

- Normalize the header before comparison: strip a UTF-8 BOM, trim surrounding whitespace per column, and
  compare case-insensitively. Column order and count stay required.
- Keep the accepted spelling out of the error path — report which column failed rather than echoing file
  content, preserving NFR-U2-SEC-01.
- Decide the category-name uniqueness question above, since it bounds what "never fails" can mean.
- Consider whether export should keep emitting the canonical header regardless of what was accepted on
  import, so round-tripping stays predictable.

#### Artifacts implicated

`business-rules.md` BR-U2-10 (column order), BR-U2-16 through BR-U2-18 (category resolution);
`sic_mapping_service.go` `matchesSICMappingHeader` and `resolveSICCategory`; `schema.sql` `category.name`
uniqueness; `requirements.md` FR4.

---

## Deliberately Not Admitted

Recorded so the reasoning is visible rather than re-argued later.

| Item | Why not |
|---|---|
| U2-F09 delete-handler collision | Broke approved UOW-2 behaviour; fixed in UOW-2 at `88213dc` and independently re-reviewed. Parking it would have left deletion broken. |
| F-04, F-05 | Deferred UOW-1 independent findings with existing owners; they are not new capability gaps. |
| UOW-2's 100,000-code merge fixture | An approved UOW-2 artifact. Reducing it is a UOW-2 amendment and remains the user's decision. |
