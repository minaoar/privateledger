# Performance Test Instructions

Covers all units built on this project. Newest first.

---

# Unit: sic-mcc-categorization (UOW-1 through UOW-5)

**Branch**: `support-mcc-for-category` · **Date**: 2026-09-07 · **GitHub issue #5**

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

---

# Unit: uncategorized-dashboard and earlier

**Branch**: `show-uncategorized-transactions` · **Date**: 2026-08-03. Retained verbatim; predates the per-unit heading convention.

## Applicability

**Formal load, stress, and concurrency testing is not applicable to this unit.** PrivateLedger is a local-only, single-user desktop application: one process, one user, no network exposure, no shared infrastructure. Throughput and concurrent-user targets have no meaning here.

What *is* worth tracking is **single-request dashboard latency**, because this unit adds queries to the page's hot path.

## Measured Baseline

Taken against a copy of the production database on an isolated port (see `integration-test-instructions.md` for the safe-setup procedure).

| Dataset | Endpoint | Median | Range |
|---|---|---|---|
| 179 transactions, 16 categories | `GET /` (dashboard HTML) | **11 ms** | 10–12 ms |
| 179 transactions, 16 categories | `GET /api/insights/dashboard` | **10 ms** | 9–10 ms |

Comfortably imperceptible. No optimization is warranted at this size.

```bash
for i in $(seq 1 10); do curl -s -o /dev/null -w "%{time_total}\n" http://localhost:8899/; done \
  | sort -n | awk '{a[NR]=$1} END {printf "min=%.3f median=%.3f max=%.3f\n", a[1], a[int(NR/2)+1], a[NR]}'
```

## Query Cost of This Unit

The dashboard already used an N+1 query pattern before this change: each breakdown table issues one query **per category per period**. With 16 categories over 6 periods that is roughly 100 queries per page load.

This unit adds **12 queries** — one per period (6) for the expense uncategorized row, and the same for income. Investment adds none.

It adds no queries to the summary cards or the pie chart. Those figures are derived from transaction slices the functions already held in memory:

- `getCategoryTypeSummary` already loads both periods unfiltered and filters in memory
- `GetMonthlySummary` already walks every transaction in the period

An earlier draft of the plan would have issued 5 additional queries there; reusing the in-memory data avoided them.

## Scaling Note (Not Tested)

The N+1 pattern means query count grows as *categories × periods*, independent of transaction count. At the current data volume this is irrelevant. Behavior at substantially larger datasets was **not measured** — a large-dataset benchmark was considered and deliberately deferred as not currently a concern.

If the dashboard ever feels slow, the fix is structural rather than incremental: replace the per-category loop with a single `GROUP BY category_id, period` aggregation. That would subsume this unit's 12 added queries as well. Filed as a potential follow-up, not a current need.

## If Latency Regresses

1. Set `"log_level": "debug"` in `config.json` — every query is logged by `TransactionRepository.List`.
2. Count queries per dashboard load; a jump well above ~110 suggests a new N+1 loop.
3. Check whether a summary or chart path started issuing per-period queries instead of reusing an in-memory slice.
