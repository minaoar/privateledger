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

A recommendation is marked **(Recommended)** on one option per question, with the reason on the line
below it. They are suggestions, not defaults — pick whatever you prefer and I will design to it.

### Q1 — Recategorization budget and cancellation

UOW-2's NFR Design explicitly deferred this: the mapping-mutation gate is held for the whole
collaborator call, so a slow recategorization blocks every other mapping change, and a mapping upload
touching many codes could run long. UOW-2 bounds only how long a *competing caller waits*, not how
long the lock holder runs.

- A. Bound the recategorization work with a processing deadline; on expiry, commit what was already
  applied and report a partial result with a warning. Mapping changes stay committed.
- B. **(Recommended)** No deadline. Recategorization runs to completion, and a failure remains a
  post-commit warning exactly as BR-U2-45 already specifies.
  *Measured evidence says the feared delay does not occur at this scale: merging 100,000 mappings took
  1.4 s, and recategorization is one indexed `GetUncategorizedBySICCodes` query plus bulk updates. A
  deadline would buy nothing while introducing partial-completion state to report and test. The gate
  only blocks other mapping mutations, by one person, in a local app.*
- C. No deadline, but do the recategorization in bounded batches so progress is visible and the
  operation can report counts as it goes.

**Correction to this question:** option B originally read "the mapping change and its recategorization
are always all-or-nothing together." That was wrong of me — it contradicts approved rule BR-U2-45,
under which the mapping commit lands first and a later collaborator failure is a committed-with-warning
success, never a rollback. Choosing all-or-nothing would mean reopening an approved UOW-2 decision. B is
restated above to match the approved contract.

[Answer]:B

### Q2 — "Recategorize All" blast radius

Application design resolution 6 requires `RecategorizeAll` to route through `Categorize` so SIC
applies there too. The consequence: the first time an existing user clicks "Recategorize All" after
upgrading, their entire uncategorized backlog becomes eligible for SIC-based assignment in one action.

- A. Accept it. This is FR6 working as specified, and the action is already explicitly user-initiated.
- B. **(Recommended)** Accept it, but the Categories page action must warn beforehand that SIC mappings
  will now also be applied, and report SIC-assigned and pattern-assigned counts separately in the result.
  *Same behaviour as A, plus the two things you would actually want the first time this runs: notice
  before it touches your whole backlog, and a split count telling you how much came from SIC — which is
  the only quick read on whether your mappings are right.*
- C. Keep "Recategorize All" text-pattern-only and add a separate explicit action for SIC.
  *Note: this contradicts FR6 and application design resolution 6, so it would need a requirement
  amendment rather than just a design decision.*

[Answer]:B

### Q3 — Cache reload concurrency

The existing `go h.categorizer.LoadPatterns()` is a real data race, and UOW-3 adds a second cache.

- A. **(Recommended)** Make both caches mutex-protected and make every reload synchronous, removing the
  bare goroutine.
  *A reload is one small query, so the latency saved by the goroutine is negligible — and asynchrony
  costs correctness: today you can add a pattern and immediately recategorize against a stale cache.
  Making it synchronous removes both the race and that ordering bug.*
- B. Keep reload asynchronous but guard both caches with `sync.RWMutex`, so the race is fixed while
  request latency is unchanged.
  *Fixes the race but keeps the stale-read-after-write ordering problem.*
- C. Load rules from the database on each categorization run instead of caching, removing the
  concurrency question entirely at the cost of per-run queries.
  *Would add a query per transaction during import, risking the approved SIC-free import benchmark.*

[Answer]:A

### Q4 — Import-time SIC lookup

Import categorizes each transaction as it is inserted. SIC categorization needs mapping data during
that loop.

- A. **(Recommended)** Use the in-memory mapping cache, loaded once at startup and refreshed on mapping
  changes.
  *Mirrors how text patterns already work, so there is one cache lifecycle to understand rather than
  two, and it pairs directly with the Q3 answer.*
- B. Query the mapping per transaction during import. Always current, but adds a query per row and
  risks regressing the approved SIC-free import benchmark.
- C. Load a mapping snapshot once at the start of each import run.
  *Also correct, but introduces a second, different cache lifecycle alongside the pattern cache.*

[Answer]:A

### Q5 — Modal-created mapping scope

US-12 lets a user create a SIC mapping from the Change Category or Create Pattern modal. FR14 scopes
recategorization to mapping changes, so a mapping created this way is a mapping change.

- A. **(Recommended)** Apply to the current transaction and immediately recategorize all other currently
  uncategorized transactions sharing that SIC code, consistent with how the mapping page behaves.
  *Creating a mapping should mean the same thing wherever you create it. FR14 already scopes
  recategorization to mapping changes, and this is one.*
- B. Apply to the current transaction only; other matching transactions wait for the next explicit
  recategorization.
  *Makes the same action behave differently depending on which screen you did it from.*
- C. Apply to the current transaction, then show how many other uncategorized transactions share that
  SIC code and let the user decide whether to apply.
  *Better UX in isolation, but adds a round trip and a second decision point mid-categorization.*

[Answer]:A

### Q6 — Property-based testing scope for UOW-3

Property-based testing is configured Partial. UOW-1 covered normalization and seed invariants; UOW-2
covered merge idempotency, omission, and counts under the approved Q7 scope.

- A. Generated properties for the categorization priority matrix only — text-before-SIC, empty-mapping
  no-op, and manual preservation across arbitrary transaction and rule sets. Everything else by example.
- B. **(Recommended)** The above plus generated properties for recategorization scoping, that only
  currently uncategorized transactions matching the affected codes ever change.
  *The scoping invariant is the one that protects data you have already categorised by hand. It is
  exactly the kind of property that holds for every example someone thinks to write and fails on the
  combination nobody did, so generating it is worth more here than anywhere else in the feature.*
- C. Examples only for UOW-3; rely on the property coverage already established in UOW-1 and UOW-2.
  *US-13 explicitly calls for selected property-based tests in this unit.*

[Answer]:B

## Execution Checklist

- [x] Read project guidelines, lifecycle state, and the approved UOW-1/UOW-2 artifacts.
- [x] Inspect the current categorizer, transaction repository, handlers, and UOW-2 collaborator seam.
- [x] Separate decisions already settled by approved artifacts from genuinely open ones.
- [x] Record the verified starting state, including the two pre-existing defects assigned to this unit.
- [x] Receive answers to Q1 through Q6. (Q1 B, Q2 B, Q3 A, Q4 A, Q5 A, Q6 B)
- [x] Analyze answers for conflicts with approved requirements and raise follow-ups if needed. No conflicts; no follow-up required.
- [x] Generate `business-logic-model.md`, `business-rules.md`, `domain-entities.md`, and
      `frontend-components.md` under `aidlc-docs/construction/transaction-categorization-integration/functional-design/`.
- [x] Receive explicit Functional Design approval. (2026-09-06, user: "Continue")

## Out of Scope

- Anything already delivered by UOW-1 or UOW-2.
- Deferred independent findings F-04 and F-05.
- New production dependencies, schema migrations beyond what UOW-1 established, and any change to the
  deployment topology. Infrastructure Design remains skipped.
