# Unit Test Instructions

## Running

```bash
go test -count=1 ./...              # full suite
make test                           # same, with -v
go test -v ./internal/service -run TestCategorizer
```

`-count=1` matters: without it Go serves cached results, and several suites here build temporary SQLite
databases whose behaviour a cache would hide.

Expect roughly **100 seconds** in `internal/service`; every other package finishes in a few seconds. The
time is real work — property generation and three performance gates — not slow tests.

## Layout

44 test files. Two kinds, and the distinction is a project rule rather than a convention:

| Kind | Named | Owner |
|---|---|---|
| Original unit tests | `*_test.go` | Either provider |
| Independent review tests | `*_review_test.go`, `uow*_review_test.go` | **Independent provider only** |

Production must never edit a `*_review_test.go` file. A test failing against deliberately changed
behaviour is reported as a handoff finding, never edited — that rule is what made the UOW-5 review
findings meaningful rather than negotiable.

## Property-based tests

Configured **Partial**. `pgregory.net/rapid v1.1.0`, replay seed `20260906`.

| File | Property |
|---|---|
| `internal/model/sic_mapping_property_test.go` | SIC code parsing and normalization |
| `internal/model/uow4_diagnostic_review_test.go` | Diagnostic values: ≤64 runes, no control runes, valid UTF-8 |
| `internal/service/sic_mapping_seed_property_test.go` | Seed CSV validation |
| `internal/service/sic_management_property_review_test.go` | Merge classification |
| `internal/service/uow3_categorization_property_review_test.go` | Categorization priority |
| `internal/service/uow4_review_test.go` | Header normalization, both directions |
| `internal/service/uow5_order_property_review_test.go` | **Order independence — FR15 stated executably** |

The last one is the centrepiece. FR15 claims the same rule book yields the same categorization whatever
order the rules were created in; that is a statement about *all* orderings, which no example-based test
can establish.

To reproduce a failure, run with the seed printed in the failure output. `rapid` shrinks automatically.

## Concurrency

```bash
go test -race -short -count=1 ./...
```

Required, not optional. Two guarantees depend on it: atomic cross-cache publication (UOW-3) and one rule
generation per re-examination pass (UOW-5).

**A clean race report is not sufficient.** The UOW-5 generation defects produced no data race at all —
they were higher-level consistency failures, caught by deterministic staged tests, not by the detector.
Run the generation tests repeatedly:

```bash
go test -count=20 -run 'TestReviewU5OnePassSeesOneRuleGeneration|TestReviewU5ShippedMappingReloadCannotSplitOnePass|TestReviewU5FailedPassSnapshotNeverFallsBackToSplitGeneration' ./internal/service/
```

## Data safety

Every test builds its own temporary SQLite database. Nothing reads or writes a real `privateledger.db`,
and no test binds port 8844. Manual verification against a running binary should use a throwaway
directory and a non-default port, as every unit's production verification did.
