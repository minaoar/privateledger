# Performance Test Instructions

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
