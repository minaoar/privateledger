# Unit of Work Story Map — Issue #5 SIC Auto-Categorization

## Assignment Rules

- Every story has exactly one primary unit assignment.
- A story may consume another unit's contract without being assigned twice.
- Requirement and acceptance-criteria traceability is preserved at the primary-unit boundary.
- Cross-cutting verification is consolidated in UOW-3, while each unit still owns tests for its behavior.

## Complete Story Assignment

| Story | Primary Unit | Delivery Capability | Upstream/Downstream Coordination |
|---|---|---|---|
| US-01 — Import transactions with SIC data | UOW-1 — SIC Import and Storage | Parser extraction, normalized model value, persistence, unchanged dedup/count contract | Supplies SIC-bearing transactions to UOW-3 |
| US-02 — Use existing text patterns before SIC mappings | UOW-3 — Transaction Categorization Integration | Text-first-then-SIC matching and empty-category no-op | Consumes UOW-1 transaction SIC and UOW-2 mappings |
| US-03 — Preserve manual categorization | UOW-3 — Transaction Categorization Integration | Protect manual/existing rule assignments; scope recategorization | Invoked by eligible mapping changes from UOW-2 |
| US-04 — Manage SIC mappings in a separate configuration page | UOW-2 — SIC Mapping Management | Dedicated page, CRUD API, current categories, mapping-change orchestration | Consumes UOW-1 persistence; invokes UOW-3 recategorization contract |
| US-05 — Prevent duplicate SIC mappings | UOW-2 — SIC Mapping Management | Normalization, validation, global uniqueness, nullable category | Relies on UOW-1 model/schema constraint |
| US-06 — Inspect SIC in transaction detail context | UOW-3 — Transaction Categorization Integration | Joined SIC description and modal display without table clutter | Consumes UOW-1 stored SIC and UOW-2 mapping description |
| US-07 — Upgrade existing local databases safely | UOW-1 — SIC Import and Storage | Idempotent in-place migration and data preservation | Enables all later units on existing installations |
| US-08 — Initialize SIC mappings from an existing mapping file | UOW-1 — SIC Import and Storage | Optional startup seed, empty-table gate, local category resolution | Produces mappings later managed by UOW-2 and consumed by UOW-3 |
| US-09 — Preserve local-only privacy | UOW-1 — SIC Import and Storage | Establish local-only data and processing boundary | Constraint inherited by UOW-2 and UOW-3 |
| US-10 — Download current SIC mappings | UOW-2 — SIC Mapping Management | CSV export of authoritative SQLite mappings | Uses UOW-1 persistence/data-directory contract |
| US-11 — Upload SIC mappings and overwrite existing mappings | UOW-2 — SIC Mapping Management | Whole-file validation, backup, atomic replacement, affected-code handoff | Uses UOW-1 repository; invokes UOW-3 scoped recategorization |
| US-12 — Create SIC mappings from transaction categorization modals | UOW-3 — Transaction Categorization Integration | Modal prompts, mapping upsert, rule-source assignment, pattern suppression | Calls UOW-2 service contract |
| US-13 — Maintain reliable implementation boundaries and tests | UOW-3 — Transaction Categorization Integration | Cross-unit layering, regression suite, concurrency safety, selected PBT | Verifies contracts delivered by all three units |

## UOW-1 Story Detail — SIC Import and Storage

| Story | Requirements | Unit-Level Acceptance Focus |
|---|---|---|
| US-01 | FR1, FR2, FR12 | SIC extracted when non-zero; missing SIC remains unset; database round trip succeeds; deduplication and combined auto-category count shape remain unchanged |
| US-07 | FR3, NFR2, NFR4 | Existing data retained; column/table/index added safely; repeated startup has no duplicate-schema failure |
| US-08 | FR4, NFR4 | File imported only when present and table empty; absent file is non-fatal; name-first category resolution and description storage enforced; existing mappings never overwritten at startup |
| US-09 | NFR1 | Parsing, persistence, file exchange, and initialization use only local files and SQLite; no external lookup introduced |

### UOW-1 Exit Evidence

- Parser examples with present, missing, and zero SIC.
- Fresh-schema and legacy-database migration tests.
- Transaction and mapping repository round-trip/constraint tests.
- Startup file absent, valid, invalid, repeated, and non-empty-table scenarios.

## UOW-2 Story Detail — SIC Mapping Management

| Story | Requirements | Unit-Level Acceptance Focus |
|---|---|---|
| US-04 | FR8, FR9, FR11, FR13, FR14 | Dedicated page lists current mappings/categories; CRUD supports categorized and intentionally unmapped codes; service layer mediates repository access |
| US-05 | FR8, FR11 | Whitespace normalization; empty/non-digit/overlength rejection; database-backed global uniqueness; empty category accepted |
| US-10 | FR9, FR13 | Download filename and five-column schema correct; current SQLite state exported; empty database yields header-only CSV |
| US-11 | FR8, FR9, FR11, FR13 | Browser confirmation precedes request; full validation precedes mutation; duplicates and category conflicts reject whole upload; durable backup path returned; replacement atomic |

### UOW-2 Exit Evidence

- CRUD service/repository and API response tests.
- Exhaustive category name/ID CSV resolution-table tests.
- CSV export/import round-trip and header-only export tests.
- Invalid-upload no-mutation and valid-upload atomic replacement tests.
- Backup creation and returned-path tests.

## UOW-3 Story Detail — Transaction Categorization Integration

| Story | Requirements | Unit-Level Acceptance Focus |
|---|---|---|
| US-02 | FR5, FR6, FR12 | Text pattern wins dual match; non-empty SIC mapping is fallback; empty mapping and missing mapping assign nothing; combined count increments for either automatic source |
| US-03 | FR7, FR14 | Manual and existing rule-based categories survive mapping changes; only matching currently uncategorized transactions are candidates |
| US-06 | FR10, FR14 | Both existing modals show code and description fallback; missing SIC is clean; no prominent transaction-table column added |
| US-12 | FR10, FR14 | Change Category can upsert mapping and apply rule source; Create Pattern can choose SIC mapping instead of text pattern; selected category required |
| US-13 | NFR3, NFR5, NFR6, AC17 | Layer direction maintained; race-safe caches; example tests plus selected normalization/CSV properties; full `go test ./...` succeeds |

### UOW-3 Exit Evidence

- Categorization priority matrix tests.
- Manual/rule preservation and affected-SIC scoping tests.
- Mapping cache reload/concurrency tests, including race detector where practical.
- Transaction read/handler/template tests for modal SIC context.
- Modal upsert integration tests verifying source and no unintended pattern creation.
- Selected property-based tests and complete build/test run.

## Requirement Coverage by Unit

| Requirement | Primary Unit(s) | Coverage Note |
|---|---|---|
| FR1-FR4 | UOW-1 | Parse, store, migrate, and optionally seed mappings |
| FR5-FR7 | UOW-3 | Categorization behavior and preservation |
| FR8 | UOW-2 | Mapping uniqueness and management constraints |
| FR9 | UOW-2 | Dedicated page and file workflows |
| FR10 | UOW-3 | Transaction modal visibility and behavior |
| FR11 | UOW-1, UOW-2 | Shared normalization contract; management validation |
| FR12 | UOW-1, UOW-3 | Existing result field retained; both automatic paths contribute |
| FR13 | UOW-2 | Mapping API; modal endpoint consumed in UOW-3 |
| FR14 | UOW-2, UOW-3 | Change events originate in management; scoped behavior executes in categorization integration |
| NFR1-NFR4 | UOW-1 with inherited constraints | Local operation, compatibility, architecture baseline, idempotency |
| NFR5-NFR6 | All units; consolidated in UOW-3 | Unit-specific tests plus cross-unit and selected property tests |

## Coverage Validation

- Stories defined: 13 (US-01 through US-13).
- Primary assignments: 13.
- Unassigned stories: none.
- Duplicate primary assignments: none.
- UOW-1 assignments: US-01, US-07, US-08, US-09.
- UOW-2 assignments: US-04, US-05, US-10, US-11.
- UOW-3 assignments: US-02, US-03, US-06, US-12, US-13.
