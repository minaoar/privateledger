# Production Revision 1 — UOW-5, Response to Independent Review

All six findings from `code-review/independent-review.md` are addressed. **Production code only**; no
test file was created or modified.

## U5-R-F01 (High) — One re-examination can mix rule generations

**Fixed.** `evaluate` split into `evaluate` (takes the read lock) and `evaluateLocked` (assumes it).
`Reexamine` now holds `c.mu.RLock()` across the **entire decision traversal** rather than per
transaction, and releases it before any write.

The finding was exactly right, and the gap was mine: I wrote BR-U5-10 requiring one pass to see one
generation, restated it as NFR-U5-CON-02, and then implemented a per-decision lock that cannot deliver
it. The comment on `evaluateLocked` now records why the split exists and warns that Go's `RWMutex` is
not reentrant, since re-acquiring it inside the traversal is the obvious wrong fix.

Verified with `-count=20`.

## U5-R-F02 (High) — A concurrent manual choice can be overwritten

**Fixed.** Both bulk writes now carry `AND category_source != 2` in SQL and return rows affected.

The predicate has to hold at **write** time, not read time — that was the substance of the finding.
`Reexamine` counts from rows actually changed, so a transaction the user made manual between the read
and the write is neither overwritten nor reported as moved.

**Consequential change:** assignment batches are now keyed by category **and rule source**, so each
batch is homogeneous. A first attempt attributed the pattern/SIC split by slicing the first `changed`
IDs, which assumes the excluded rows were at the end — they need not be. Homogeneous batches make the
attribution exact instead of plausible.

## U5-R-F03 (High) — Mapping results omit two FR16 counts

**Fixed.** `SICMappingMutationResult` and `SICMappingImportResult` gain `moved_count`,
`uncategorized_count` and `manual_protected_count`, populated by `runPostCommit` for CRUD and merge.
`recategorized_rows` keeps its meaning and equals `moved_count`, so nothing reading the older field
breaks. No `omitempty`: a zero is an answer.

## U5-R-F04 (High) — Pattern rule changes hide counts and post-commit failures

**Fixed.** A shared `reexamineAfterRuleChange` helper returns the three counts, or a
`post_commit_warnings` entry when re-examination fails after a committed rule change. Wired into
category creation with patterns, pattern add, pattern delete and category deletion — all of which now
return the counts and keep the rule mutation committed.

## U5-R-F05 (High) — Category deletion destroys manual provenance

**Fixed.** `ClearCategory` now preserves `category_source` on manual rows, detaching the category
without erasing the fact that the user chose it. A manual row ends with no category and source still
manual: the choice cannot be honoured because its category is gone, but it is still the user's, so no
rule may claim it.

Production raised this in the handoff and parked it as needing a product decision. The reviewer ruled it
a defect. That was the better call: NFR-U3-REL-01's retained manual clause already settled it, so there
was nothing left to decide.

## U5-R-F06 (Medium) — Empty-category mapping creation skips the trigger

**Fixed.** Both the direct create path and merge creation now trigger re-examination for every mapping
creation. The `HasCategory()` condition was an optimization justified by the row being inert, and
BR-U5-01 names creation as a trigger with BR-U5-04 naming the description-only edit as the only mapping
exclusion.

## A UOW-3 contract test caught a regression during this revision

`TestReviewU3CategoryPageFeedbackContract` failed when `categories.html` was rewritten around the FR16
counts, because the pattern/SIC split was dropped. Restored — the approved additive decision (Q3 A) said
the existing fields keep their meaning, and the split answers a different question from FR16's counts.

## New Handoff Finding — `internal/repository` tests no longer compile

Four call sites, two files, both reviewer-owned:

| File | Lines | Symbol |
|---|---|---|
| `internal/repository/uow3_set_passing_review_test.go` | 79, 103 | `BulkUpdateCategory` |
| `internal/repository/uow5_reexamination_review_test.go` | 39, 42 | `BulkClearCategory` |

Both now return `(int, error)`.

This follows directly from F02's acceptance condition — *"Returned moved/uncategorized counts must
reflect rows actually changed, such as by using `RowsAffected`"* — which cannot be satisfied while the
methods return only `error`.

**Worth flagging honestly:** `uow5_reexamination_review_test.go` is the reviewer's own new test, written
against the previous signature, and the acceptance condition said "such as" rather than mandating it. If
the reviewer prefers the counts obtained some other way, production will change it — but every
alternative examined either duplicates the two methods or leaves the counts inaccurate under exactly the
race F02 is about.

## Verification

`gofmt` clean; `go build ./...` clean.

Every package except `internal/repository` passes `go test -count=1` and `go test -race -short -count=1`,
including all six previously failing UOW-5 tests and the full UOW-1 through UOW-4 suites.

`internal/repository`'s production code is unchanged in behaviour beyond the two predicates and return
values; its tests cannot compile until the four call sites above are updated, which is reviewer-owned.
