# Integration Test Instructions

The units share three contracts, and the tests that cross them are where a change in one unit shows up
as a defect in another.

## The cross-unit contracts

| Contract | Established | Consumed by |
|---|---|---|
| SIC persistence — `sic_code` on transactions, `sic_mapping` table | UOW-1 | UOW-2, UOW-3, UOW-5 |
| Mapping management — the collaborator interface | UOW-2 | UOW-3 (implements), UOW-5 (replaced its signature) |
| The single decision function | UOW-3 | UOW-4 (untouched), UOW-5 (snapshots it) |

## Running

```bash
go test -count=1 ./internal/service ./internal/handler ./cmd/privateledger
```

These three exercise the seams. `internal/repository` and `internal/database` are closer to unit scope;
`internal/model` and `internal/parser` are pure.

## What each seam must show

**Import → categorization.** `sic_import_e2e_test.go` drives a real OFX file through parsing,
deduplication and categorization. Import applies rules to what it brings in and **must not re-examine
existing transactions** — re-examining on import would make the outcome depend on import order, which
FR15 forbids.

**Mapping change → re-examination.** Every mapping mutation and the upload merge invoke the collaborator
exactly once. A description-only edit invokes nothing. Creation invokes it even with an empty category.

**Pattern change → re-examination.** Pattern create, add and delete, and category deletion, all reach
the same entry point as mapping changes. One implementation, so priority cannot drift between them.

**Priority across sources.** Text patterns outrank SIC mappings. Creating a pattern can move a
transaction *off* a mapping-assigned category — intended, and the case most likely to be mistaken for a
defect.

**Manual protection, everywhere.** No trigger writes a `category_source = 2` row. The write predicates
enforce this in SQL at write time, not at read time, so a manual choice made between a pass's read and
its write is not overwritten.

**Category deletion.** Clears its transactions including manual ones, which then become uncategorized
and are re-examined — the approved FR7 amendment. Exactly one representation of "uncategorized" exists:
`category_id = NULL, category_source = 0`.

## Browser-executed verification

The reviewer ran Chrome against the real handlers and templates for UOW-5. That is the level at which
defect U2-F09 escaped every Go test: `layout.html` loads `app.js` **after** page content, so a page
function sharing a name with an `app.js` global is silently overwritten while the markup still looks
correct.

Any change adding page-level JavaScript should be executed in a browser, not only compiled.

## Manual end-to-end check

Against a throwaway directory and a non-default port:

1. Create categories and an account; import an OFX file with SIC codes.
2. Create a mapping — uncategorized transactions take it, manual ones do not.
3. **Repoint the mapping** — transactions it had categorized follow it. This is the behaviour UOW-5 exists for.
4. Create a matching text pattern — those transactions move to it.
5. Delete the mapping — its transactions become uncategorized unless a pattern claims them.
6. Re-run "Recategorize All" twice — the second reports three zeros.
7. Import again — existing categorizations are untouched.
