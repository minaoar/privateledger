# Functional Design Plan — UOW-3 Transaction Categorization Integration

## Stage Inputs

UOW-1 and UOW-2 are complete with independent gates PASS. UOW-3 is the final unit. It consumes the
persistence contracts from UOW-1 and the mapping-management contracts from UOW-2, and replaces the
no-op collaborator UOW-2 wired.

Assigned stories: **US-02** (text patterns before SIC), **US-03** (preserve manual categorization),
**US-06** (SIC visible in transaction modals), **US-12** (create mappings from the modals),
**US-13** (layering, race-safe caches, tests).

Technology constraint: existing Go / Gin / Bootstrap / HTMX / vanilla JavaScript / SQLite stack. No
new production dependency, no drastic technology change.

## Verified Starting State

Confirmed against the working tree before planning.

| Observation | Location | Consequence for this unit |
|---|---|---|
| `RecategorizeAll` re-implements pattern matching inline instead of calling `Categorize` | `internal/service/categorizer.go:89-99` | SIC would apply on import but be skipped by "Recategorize All", contradicting FR6. Application design resolution 6 requires routing through `Categorize`. |
| `RecategorizeByCategory` has the same inline duplication | `internal/service/categorizer.go:116+` | Same. |
| `go h.categorizer.LoadPatterns()` mutates the pattern cache from a goroutine with no synchronization | `internal/handler/category_handler.go:342` | Pre-existing data race. Application design resolution 5 assigns the fix here. |
| `LoadPatterns` is exported and called from four sites | `category_handler.go:152,301,342`, `main.go` | Resolution 10 makes it unexported behind a single `LoadRules` entry point. |
| `Categorizer` holds `patterns []*model.CategoryPattern` with no mutex | `internal/service/categorizer.go:12-16` | A SIC mapping cache will be added alongside it; both need race-safe reload. |
| `GetUncategorizedBySICCodes` already exists | `internal/repository/transaction_repo.go:344` | The scoped recategorization query is available; UOW-3 consumes it. |
| `SICDescription` is already joined in `List` and `GetByID` | `internal/repository/transaction_repo.go:107,175` | US-06 needs no new read path; the modals need only to render it. |
| UOW-2 wires `NewNoopSICRecategorizationCollaborator()` and rejects nil | `cmd/privateledger/main.go`, `internal/service/sic_mapping_service.go` | UOW-3 supplies the real adapter implementing `ReloadMappings` and `RecategorizeBySICCodes`. |

## Already Settled by Approved Artifacts — Not Re-Opened Here

- **FR6 priority**: text patterns first, SIC mappings second; a matching mapping with an empty
  category assigns nothing.
- **FR7 preservation**: `category_source = 2` is never overwritten; mapping changes only affect
  currently uncategorized transactions; existing rule-based assignments are untouched.
- **FR10 modal display**: no new prominent SIC column in the transactions table; both existing modals
  show the code plus `Description`, falling back to `Description_Detail`, and render cleanly when
  neither exists.
- **Adapter shape**: a SIC-specific categorizer extension rather than absorbing SIC logic into the
  core `Categorizer` (application design, 2026-08-24).
- **Modal semantics**: Change Category applies the SIC rule source when the user agrees; Create
  Pattern creates a SIC mapping only, never a text pattern; modal-created mappings require a selected
  category.

## Open Questions

Answer each by replacing the `[Answer]:` tag. These are the decisions this stage cannot settle from
the approved artifacts.

### Q1 — Recategorization budget and cancellation

UOW-2's NFR Design explicitly deferred this: the mapping-mutation gate is held for the whole
collaborator call, so a slow recategorization blocks every other mapping change, and a mapping upload
touching many codes could run long. UOW-2 bounds only how long a *competing caller waits*, not how
long the lock holder runs.

- A. Bound the recategorization work with a processing deadline; on expiry, commit what was already
  applied and report a partial result with a warning. Mapping changes stay committed.
- B. No deadline. Recategorization runs to completion however long it takes; the mapping change and
  its recategorization are always all-or-nothing together.
- C. No deadline, but do the recategorization in bounded batches so progress is visible and the
  operation can report counts as it goes.

[Answer]:

### Q2 — "Recategorize All" blast radius

Application design resolution 6 requires `RecategorizeAll` to route through `Categorize` so SIC
applies there too. The consequence: the first time an existing user clicks "Recategorize All" after
upgrading, their entire uncategorized backlog becomes eligible for SIC-based assignment in one action.

- A. Accept it. This is FR6 working as specified, and the action is already explicitly user-initiated.
- B. Accept it, but the Categories page action must warn beforehand that SIC mappings will now also be
  applied, and report SIC-assigned and pattern-assigned counts separately in the result.
- C. Keep "Recategorize All" text-pattern-only and add a separate explicit action for SIC.

[Answer]:

### Q3 — Cache reload concurrency

The existing `go h.categorizer.LoadPatterns()` is a real data race, and UOW-3 adds a second cache.

- A. Make both caches mutex-protected and make every reload synchronous, removing the bare goroutine.
  Simplest to reason about; the reload becomes part of the request.
- B. Keep reload asynchronous but guard both caches with `sync.RWMutex`, so the race is fixed while
  request latency is unchanged.
- C. Load rules from the database on each categorization run instead of caching, removing the
  concurrency question entirely at the cost of per-run queries.

[Answer]:

### Q4 — Import-time SIC lookup

Import categorizes each transaction as it is inserted. SIC categorization needs mapping data during
that loop.

- A. Use the in-memory mapping cache, loaded once at startup and refreshed on mapping changes.
  Fastest; consistent with how patterns already work.
- B. Query the mapping per transaction during import. Always current, but adds a query per row and
  risks regressing the approved SIC-free import benchmark.
- C. Load a mapping snapshot once at the start of each import run.

[Answer]:

### Q5 — Modal-created mapping scope

US-12 lets a user create a SIC mapping from the Change Category or Create Pattern modal. FR14 scopes
recategorization to mapping changes, so a mapping created this way is a mapping change.

- A. Apply to the current transaction and immediately recategorize all other currently uncategorized
  transactions sharing that SIC code, consistent with how the mapping page behaves.
- B. Apply to the current transaction only; other matching transactions wait for the next explicit
  recategorization.
- C. Apply to the current transaction, then show how many other uncategorized transactions share that
  SIC code and let the user decide whether to apply.

[Answer]:

### Q6 — Property-based testing scope for UOW-3

Property-based testing is configured Partial. UOW-1 covered normalization and seed invariants; UOW-2
covered merge idempotency, omission, and counts under the approved Q7 scope.

- A. Generated properties for the categorization priority matrix only — text-before-SIC, empty-mapping
  no-op, and manual preservation across arbitrary transaction and rule sets. Everything else by example.
- B. The above plus generated properties for recategorization scoping, that only currently
  uncategorized transactions matching the affected codes ever change.
- C. Examples only for UOW-3; rely on the property coverage already established in UOW-1 and UOW-2.

[Answer]:

## Execution Checklist

- [x] Read project guidelines, lifecycle state, and the approved UOW-1/UOW-2 artifacts.
- [x] Inspect the current categorizer, transaction repository, handlers, and UOW-2 collaborator seam.
- [x] Separate decisions already settled by approved artifacts from genuinely open ones.
- [x] Record the verified starting state, including the two pre-existing defects assigned to this unit.
- [ ] Receive answers to Q1 through Q6.
- [ ] Analyze answers for conflicts with approved requirements and raise follow-ups if needed.
- [ ] Generate `business-logic-model.md`, `business-rules.md`, `domain-entities.md`, and
      `frontend-components.md` under `aidlc-docs/construction/transaction-categorization-integration/functional-design/`.
- [ ] Receive explicit Functional Design approval.

## Out of Scope

- Anything already delivered by UOW-1 or UOW-2.
- Deferred independent findings F-04 and F-05.
- New production dependencies, schema migrations beyond what UOW-1 established, and any change to the
  deployment topology. Infrastructure Design remains skipped.
