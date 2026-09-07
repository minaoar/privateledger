# Production Revision 2 — UOW-5, Response to the Revision 1 Re-review

Three of the four open findings are fixed. **U5-R-F05 is not**, because the reviewer requires a recorded
product decision before it can be, and that decision is the user's.

Production code only; no test file was created or modified.

## U5-R-F01 — the shipped mapping reload could still split a pass

**Fixed, and the first attempt was the wrong shape.**

Revision 1 held `Categorizer.mu` across the traversal. That covers publications made through
`Categorizer.LoadRules` and nothing else — `SICMappingCategorizer.ReloadMappings` publishes on its own
mutex and never touches the categorizer's.

My first fix here routed `ReloadMappings` through the categorizer when one is attached. The reviewer's
test defeats it, and correctly: it wraps the real categorizer in a probe, so attachment reaches the
wrapper and the inner object still publishes on its own. **Any fix that depends on every publisher
cooperating can be bypassed by a wrapper or by a lookup written later.**

The fix is a snapshot instead. `Reexamine` now collects the distinct SIC codes its materialized
transactions actually use, resolves them once through the **public** `LookupCategory` interface under one
read lock, captures the pattern slice alongside them, and evaluates the whole traversal against that
immutable `ruleGeneration`. No lookup call happens during the traversal, so no publication can reach it,
whatever the lookup is underneath.

The attachment-based routing was reverted; it was extra machinery for a guarantee the snapshot gives
unconditionally.

Verified at `-count=10`.

## U5-R-F03 and U5-R-F04 — screens discarded the counts they were given

**Fixed.** The server work was already correct; the UI was not consuming it.

| Screen | Trigger | Now |
|---|---|---|
| Categories | create with patterns, add pattern, delete pattern, delete category, recategorize | Parses the response and reports all three counts |
| SIC mappings | create/update, delete, upload | Same, via `describeRuleChangeCounts` |
| Transactions | modal mapping create/update | All three counts plus post-commit warnings, replacing the positive-only `recategorized_rows` |

Two details the finding called out specifically:

- **Zeros are reported.** The old modal text appeared only when `recategorized_rows` was truthy, so a
  change that uncategorized transactions or spared manual ones said nothing at all.
- **Warnings do not invite a retry.** A rule change is durable before re-examination runs, so the wording
  states what happened and adds that the change itself was saved. Repeating it would apply the rule
  twice, not recover anything.

Mapping save and delete now show the counts *before* reloading the page, rather than reloading out from
under the only report the user gets.

## U5-R1-F01 — two responses were changed rather than extended

**Fixed.** Revision 1 nested `CategoryWithPatterns` under a `category` key and the pattern under
`pattern`, which silently hands an existing consumer a zero-valued object.

A `withRuleChangeCounts` helper now marshals the original payload and merges the count fields at the top
level, so the legacy fields stay exactly where callers expect them.

## U5-R-F05 — open, awaiting the product decision

Revision 1 wrote `category_id = NULL, category_source = 2` for a manual row whose category is deleted.
The reviewer is right that this creates a second representation of "uncategorized" and that the
repository APIs now disagree — `List(Uncategorized:true)` counts the row, `GetUncategorized` and
`CountUncategorized` do not.

The first review said this needed the user's decision or an explicit artifact amendment. Revision 1 made
the choice instead of asking. That was the error, not just the state it picked.

The decision is with the user. `TestReviewU5CategoryDeletionDoesNotSplitUncategorizedSemantics` fails
until it is made and implemented.

## Verification

`gofmt`, `go vet` and `go build ./...` clean. Every package passes `go test -count=1` except
`internal/handler`, which fails only `TestReviewU5CategoryDeletionDoesNotSplitUncategorizedSemantics` —
the F05 test.

The repository package compiles again: the reviewer updated the four call sites for the `(int, error)`
signatures.

---

# Revision 3 Addendum — U5-R-F05 Decided and Implemented

The user chose **option A** on 2026-09-07: deleting a category clears it from every transaction it held,
manual ones included, and those transactions become uncategorized and are re-examined like any other.

## Why this option

FR7 protects a manual assignment from being revised by a **rule**. Deleting the category is not a rule
acting — it is the user removing the very thing they chose. Once the category is gone the choice cannot
be honoured in any form.

The alternative kept `category_source = manual` on a transaction with no category. That preserves a
marker for a choice that can no longer be applied, at the cost of a **second representation of
"uncategorized"**: `List(Uncategorized:true)` counted such a row while `GetUncategorized` and
`CountUncategorized` did not. One meaning of uncategorized is worth more than a marker for an
unhonourable choice.

## Artifacts amended before the code changed

The reviewer's acceptance condition required exactly this order — *"If the user chooses different
semantics, amend the approved artifacts explicitly before changing this test."*

| Artifact | Amendment |
|---|---|
| `requirements.md` FR7 | Records the decision, why deleting a category is not a rule revising a choice, and that the alternative was declined for creating a second uncategorized representation |
| `nfr-requirements.md` NFR-U5-REL-02 | Scope clarified: the guarantee is about rules, and does not survive the user deleting the category itself |
| `functional-design/business-rules.md` BR-U5-05 | Same clarification, pointing at BR-U5-08's single representation |

Manual assignments in categories that still exist are untouched, under every trigger. That is what
NFR-U5-TEST-04 verifies and it continues to pass.

## Verified against the running binary

A transaction manually assigned to *Travel*, with SIC 5412 mapped to *Groceries*. Deleting *Travel*
returned `moved_count 1, uncategorized_count 0, manual_protected_count 0`, and the transaction ended in
*Groceries* with `category_source = 1`.

## Consequent handoff finding

`TestReviewU5CategoryDeletionPreservesManualChoiceMarker` now fails. It asserts the semantics the user
declined, and per the acceptance condition the artifacts were amended first so that the test can follow.
`TestReviewU5CategoryDeletionDoesNotSplitUncategorizedSemantics` passes.

That is the only failing test in the repository.
