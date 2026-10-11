# Technology Decisions — UOW-3 Transaction Categorization Integration

Approved by the user on 2026-09-06 alongside `nfr-requirements.md`. UOW-1 and UOW-2
technology decisions are retained unchanged. No dependency is added, removed, or upgraded.

| Concern | Decision | Requirement |
|---|---|---|
| Runtime | Existing Go module baseline; no new process | MAINT-01 |
| HTTP and UI | Existing Gin, embedded Go templates, Bootstrap, HTMX, vanilla JavaScript | UX-01, MAINT-01 |
| Persistence | Existing `modernc.org/sqlite`, parameterized SQL, transactions, foreign keys | REL-01, SEC-01 |
| Rule caches | In-process maps and slices guarded by standard-library `sync` primitives | CON-01, SCALE-01 |
| Scoped reads | Existing `GetUncategorizedBySICCodes` over `idx_txn_sic` | PERF-01 |
| Logging | Existing `log/slog` | SEC-01 |
| Verification | Go `testing`, temporary SQLite databases, race detector, uninstrumented timing harness | TEST-01, TEST-03, TEST-04 |
| Generated properties | Existing test-only `pgregory.net/rapid v1.1.0` | TEST-02, PBT-09 |
| External services / infrastructure | None; single local binary retained; Infrastructure Design skipped | SEC-01, MAINT-01 |

## TD-U3-01 — Implement the UOW-2 collaborator rather than redefining it

UOW-2 already defines `ReloadMappings` and `RecategorizeBySICCodes` and wires an explicit no-op. UOW-3
supplies the real implementation behind that same interface. The contract is not renamed, widened, or
given a context parameter.

Adding a context now would only describe a cancellation budget that Q1 B deliberately declined. If
NFR-U3-PERF-01 fails and the deadline decision is reopened, that is the moment to revisit the signature
— not before.

## TD-U3-02 — Standard-library synchronization, no new concurrency machinery

Guarding two caches and making reload synchronous needs `sync.RWMutex` and nothing more. No worker pool,
queue, background refresher, or third-party cache library is introduced.

The removal of `go h.categorizer.LoadPatterns()` is a deletion, not a replacement: the reload becomes
part of the request that triggered it.

## TD-U3-03 — Reuse `category_source = rule` for SIC assignments

No new `CategorySource` value is introduced. A category assigned from a SIC mapping came from a rule
exactly as a pattern-assigned one did.

A fourth value would change the meaning of existing rows and of every query filtering on
`category_source`, including the uncategorized-dashboard work already shipped, for no behavioural gain.
Where the distinction matters — the "Recategorize All" result — it is carried as separate counts rather
than as persisted state.

## TD-U3-04 — No new persistence

UOW-3 adds no table, column, or index. UOW-1 already provides `ledger_transaction.sic_code`,
`idx_txn_sic`, the joined `SICDescription`, and `GetUncategorizedBySICCodes`. This unit reads those and
writes only `category_id` and `category_source`.

## TD-U3-05 — Mapping writes go through the UOW-2 service

Modal-created mappings call `SICMappingService`, never the mapping repository directly, so
normalization, uniqueness, the admission gate, backup, and recategorization behave identically whether a
mapping is created from the mapping page or from a transaction modal.

## TD-U3-06 — Existing templates only

`transactions.html` and `categories.html` are modified in place. No template, page, route, or frontend
library is added. New page-level JavaScript names are checked against `app.js` globals before use, per
NFR-U3-MAINT-01 and the U2-F09 defect.
