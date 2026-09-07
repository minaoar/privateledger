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
**Disposition (2026-09-07)** — **MITIGATED, not resolved.** UOW-4 Q1 chose to keep rejecting the row and
make the diagnostic name the unresolved value and the current name of the category its `Category_ID`
refers to, so the file is repairable in one edit. Resolving by `Category_ID` was considered and declined:
nothing in a file distinguishes a rename, where the ID is correct, from a delete-and-recreate, where it
may not be. The residual manual step is accepted deliberately and left visible here.

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
- ~~Provisional story **US-14 — Restore mappings from an application backup**.~~ **Superseded
  2026-09-07.** The restore capability was removed from scope, and US-14 is now
  "Accept cosmetic mapping-file variation and explain what must be fixed". A restore story, if wanted,
  needs a new number and its own justification.

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
| UTF-16 input detection (C4-01, below) | Raised at NFR Requirements as Q5, answered A. Never admitted before the 2026-09-06 freeze, and it would require a diagnostic code the approved `domain-entities.md` forbids adding. |

## Candidate Findings — Recorded, Not Admitted

Raised during UOW-4 but outside its frozen scope. Recorded so they are picked up deliberately rather
than rediscovered.

### C4-01 — A UTF-16 file fails with a diagnostic that does not mention encoding

**Raised**: 2026-09-07, NFR Requirements Q5. **Answered**: A — decline for this unit.

A file beginning `FF FE` or `FE FF` is UTF-16. Read as UTF-8 it produces a header that matches nothing,
and the user is told the header is wrong rather than that the encoding is. Detecting the two byte
sequences is a few lines.

Declined for three reasons, none of which is difficulty:

1. It was not admitted before the 2026-09-06 scope freeze.
2. It needs a new diagnostic code, which `domain-entities.md` and TD-U4-04 both forbid — a larger
   commitment than the line count suggests, since diagnostic codes are a matching surface for the
   independent reviewers' tests.
3. The practical exposure is narrower than it looks. Excel's "CSV UTF-8 (Comma delimited)" writes UTF-8
   with a BOM, which Q2 A already handles; its UTF-16 output is tab-delimited `.txt`, not CSV.

Cheapness is not admission criteria. This unit already had a restore capability struck from it for
arriving the same way — from an adjacent observation rather than from a finding.

### C4-02 — Category names have no length bound

**Raised**: 2026-09-07, NFR Requirements answer analysis. **Not admitted.**

`category.name` is `TEXT NOT NULL UNIQUE` with no length constraint (`schema.sql:53`) and
`CreateCategory` validates no length (`category_handler.go:96`). A category name is therefore unbounded
user-controlled text.

UOW-4 **mitigates the consequence** rather than the cause: NFR-U4-SEC-01 bounds every name echoed into a
diagnostic to 64 runes regardless of source, so an unbounded name cannot inflate an upload response.
Whether names should be bounded at creation is a separate question about a table this unit does not
touch, and it would need a decision about existing rows.
