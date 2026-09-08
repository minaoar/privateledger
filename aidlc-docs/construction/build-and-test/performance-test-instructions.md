# Performance Test Instructions

## Reference environment

Apple M1, 8 logical CPUs, 16 GiB RAM, macOS 15.5, go1.26.0, APPLE SSD AP1024Q NVMe.

Every figure below was measured here. **If you run elsewhere, report the difference rather than claiming
equivalent evidence** — that rule has applied since UOW-1 and is why the numbers are comparable across
five units.

## Protocol

One warm-up run discarded, median of at least five uninstrumented runs, the same pre-state restored for
each. Fixture construction, database creation and browser transfer are excluded from timings.

```bash
go test -count=1 ./internal/database ./internal/service
```

The gates run as part of the ordinary suite; there is no separate benchmark command.

## The gates

| Requirement | Target | Measured | File |
|---|---|---|---|
| NFR-U1-PERF-01 | 100,000-row legacy migration ≤ 5 s | pass | `internal/database/migration_perf_test.go` |
| NFR-U1-PERF-02 | 100,000-row seed ≤ 10 s | pass | `internal/service/sic_seed_perf_test.go` |
| NFR-U1-PERF-03 / NFR-U3-PERF-02 | SIC-bearing import ≤ 10 % over SIC-free | **5.50 %** | `internal/service/import_regression_perf_test.go` |
| NFR-U2-PERF-01 / NFR-U5-PERF-02 | 100,000-row merge ≤ 10 s, **with 20,000 transactions present** | **2.704 s** | `internal/service/sic_management_perf_review_test.go` |
| NFR-U3-PERF-01 | 20,000-transaction full pass ≤ 5 s | 826.8 ms | `internal/service/uow3_performance_review_test.go` |
| NFR-U5-PERF-01 | 20,000-transaction re-examination ≤ 1.5 s, and a no-op pass measurably faster | **532.9 ms**, no-op **80.4 ms** | `internal/service/uow3_performance_review_test.go` |

## Two of these are not ordinary latency gates

**NFR-U5-PERF-01's second half is a correctness test wearing a stopwatch.** BR-U5-17 says a transaction
whose re-examined category equals its stored category is not written. The only external evidence is that
a pass changing nothing is faster than one changing everything. 80.4 ms against 532.9 ms is that
evidence. **If the no-op pass stops being faster, BR-U5-17 has stopped working**, whatever the absolute
number says.

**NFR-U2-PERF-01's fixture must contain 20,000 transactions.** It originally contained none, which was
correct when UOW-2's collaborator was a no-op. Once a merge triggers a full re-examination, an empty
fixture omits the cost that dominates it — the benchmark would report PASS while a real regression
shipped. A benchmark that cannot fail for the right reason is worse than no benchmark, because its PASS
is read as evidence.

## The budget that tightened, and why

NFR-U3-PERF-01 allowed 5 s for an explicit "Recategorize All" the user chose to trigger. NFR-U5-PERF-01
allows 1.5 s for the same work, because it now runs **when saving a single pattern**. A budget tolerating
a sixfold slowdown on an interactive save is not a useful budget.

1.5 s is ~1.8× the 826.8 ms measured at the time — headroom over a real number, not a round figure.

## If a gate fails

NFR-U5-CON-01 records the contingency in advance: the mapping-mutation gate is held for the full
duration of re-examination with no processing deadline, and that decision rests on these measurements.
**If NFR-U5-PERF-01 fails, reopen the deadline decision — do not relax the budget.** Recorded before any
measurement was taken, so a failure is a planned branch rather than an argument under pressure.
