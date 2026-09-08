# Build and Test Summary

## Status

All five units complete with independent gates **PASS**. The full suite passes under `go test -count=1`
and `go test -race -short -count=1` in every package. No blocking, high, medium or low finding remains
open against any unit.

## One command each

```bash
gofmt -l ./cmd ./internal          # must print nothing
go vet ./...                       # must be clean
make build                         # host binary
go test -count=1 ./...             # full suite, ~110 s
go test -race -short -count=1 ./...
```

## What the units delivered

| Unit | Delivered | Gate |
|---|---|---|
| UOW-1 | SIC persistence: `sic_code` on transactions, the `sic_mapping` table, startup seed | PASS |
| UOW-2 | Mapping management: CRUD, upload merge with backup, admission gate | PASS |
| UOW-3 | One decision function; patterns before mappings; import and recategorization integrated | PASS |
| UOW-4 | Tolerant CSV header; bounded, specific diagnostics | PASS |
| UOW-5 | Rule changes reach the transactions those rules categorized; FR16 counts | PASS at Revision 5 |

## The invariants worth re-checking after any future change

These are the ones that took the most work to establish and are the easiest to break silently.

**Manual is never overwritten by a rule.** Enforced in SQL at write time, not decided at read time. A
manual choice made between a pass's read and its write is excluded by the statement itself.

**One rule generation per pass.** A re-examination takes one atomic read of the whole mapping index. It
does not resolve code by code, and it does not fall back to doing so — if the atomic read fails the pass
returns an error before writing anything. Both defects here produced **no data race**; the detector will
not catch a regression, only the deterministic generation tests will.

**Exactly one representation of "uncategorized":** `category_id = NULL, category_source = 0`. A second
one makes `List(Uncategorized:true)`, `GetUncategorized` and `CountUncategorized` disagree about the same
row.

**Categorization depends on the current rules, never on their history** (FR15). Any proposal whose result
depends on when a rule was created contradicts an approved requirement.

**Diagnostics are bounded.** 64 runes per value, 512 per assembled message. `category.name` has no length
constraint, so a database-sourced name is unbounded user text too.

## Known open items, none blocking

| Item | Status |
|---|---|
| C4-01 — UTF-16 input detection | Candidate finding; declined at UOW-4 as outside the frozen scope |
| C4-02 — category names have no length bound | Candidate; UOW-4 bounds the consequence at the diagnostic boundary, not the cause |
| C4-03 — column 4 is trusted positionally | **Accepted residual.** A mis-delimited seed row can put description text in `Category_Name`, which the seed path logs bounded |
| F-04, F-05 | Deferred UOW-1 independent findings with existing owners |

## Cross-provider ownership

Production code and verification tests were authored by different providers in separate sessions
throughout. Production never edited a `*_review_test.go` file; where an approved contract change broke
one, it was reported as a handoff finding and the independent provider updated it.

That separation is what produced the UOW-5 findings. The generation defects, the write-time manual race,
and the silent fallback were all found by tests production did not write and could not have written to
pass.
