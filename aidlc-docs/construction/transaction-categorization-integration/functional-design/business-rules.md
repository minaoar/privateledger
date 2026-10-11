# Business Rules — UOW-3 Transaction Categorization Integration

## Categorization Priority

- BR-U3-01: One decision function serves import, "Recategorize All", and scoped recategorization. No
  entry point re-implements matching inline.
- BR-U3-02: `category_source = manual` is never overwritten by any automatic path.
- BR-U3-03: ~~A transaction that already has a category is never revised automatically. Automatic
  categorization fills gaps only.~~

  **Superseded 2026-09-07 by UOW-5** (FR7 as amended, FR15). A transaction whose category came from a
  rule is re-examined whenever any rule is created, changed or deleted, and takes whatever the current
  rules give it — or none. A transaction with `category_source = manual` is still never revised; that is
  BR-U3-02 and it is untouched.

  This rule is left visible rather than deleted because it explains why UOW-1 through UOW-4 behave as
  they do, and because the guard it describes is still in shipped code at `Categorizer.decide` until
  UOW-5 changes it.
- BR-U3-04: Text patterns are tried first, in their existing order, first match wins.
- BR-U3-05: A SIC mapping is consulted only when no text pattern matched and the transaction carries a
  SIC code.
- BR-U3-06: A matching mapping with a non-empty category assigns that category with
  `category_source = rule`.
- BR-U3-07: A missing mapping, or a mapping with an empty category, assigns nothing. An empty-category
  mapping is a deliberate "never categorize this code", not an absence of data.
- BR-U3-08: `RecategorizeAll` and `RecategorizeByCategory` route through the decision function, so SIC
  cannot apply on import while being skipped by an explicit recategorization.

## Scoped Recategorization

- BR-U3-09: UOW-3 implements the collaborator contract UOW-2 defined; UOW-2's interface is not changed.
- BR-U3-10: `RecategorizeBySICCodes` considers only currently uncategorized transactions whose SIC code
  is in the supplied set, read through `GetUncategorizedBySICCodes` over `idx_txn_sic`.
- BR-U3-11: An empty affected-code set performs no work and returns zero.
- BR-U3-12: The returned count is the number of transactions actually assigned a category.
- BR-U3-13: Scoped recategorization carries no processing deadline (Q1 B). It runs to completion.
- BR-U3-14: A collaborator failure after a committed mapping change is a committed-with-warning result
  under BR-U2-45. A mapping change is never rolled back because categorization failed afterwards.
- BR-U3-15: Rule reload precedes categorization. If reload fails, no categorization is attempted, so
  transactions are never categorized against stale rules.

## Rule Caches

- BR-U3-16: The text-pattern cache and the SIC mapping cache are both guarded against concurrent read
  and write.
- BR-U3-17: Every reload is synchronous (Q3 A). No reload runs in a detached goroutine.
- BR-U3-18: The existing `go h.categorizer.LoadPatterns()` call is removed. It is both an
  unsynchronized write and an ordering bug that lets a request read rules the caller believes were
  already refreshed.
- BR-U3-19: A single exported reload entry point refreshes both caches, so a caller cannot refresh one
  and forget the other. `LoadPatterns` becomes unexported.
- BR-U3-20: The mapping cache is loaded at startup and refreshed on mapping changes (Q4 A). Import
  performs no per-transaction mapping query, protecting the approved SIC-free import benchmark.
- BR-U3-21: Cache contents are derived state only. SQLite remains authoritative, and a stale cache can
  never produce a write that contradicts a database constraint.

## Recategorize All

- BR-U3-22: The Categories page action warns before running that SIC mappings will now also be applied
  (Q2 B).
- BR-U3-23: The result reports transactions categorized by text pattern and by SIC mapping as separate
  counts, alongside the processed total. A single combined figure is not sufficient, because the split
  is what shows whether mappings behave as intended.
- BR-U3-24: The existing combined total remains available so current callers are not broken.

## Transaction Modals

- BR-U3-25: Both the Change Category and Create Categorization Pattern modals show the SIC code when
  the transaction has one.
- BR-U3-26: The displayed description is the mapping `Description`, falling back to
  `Description_Detail`; with neither, the code is shown alone and cleanly.
- BR-U3-27: SIC context is read from the already-joined `SICDescription` field. No per-modal query is
  added.
- BR-U3-28: The transactions table gains no SIC column.
- BR-U3-29: A modal-created mapping requires a selected category. Empty mappings are created only on
  the mapping page.
- BR-U3-30: Create Categorization Pattern creates a SIC mapping *instead of* a text pattern when the
  user chooses SIC. It never creates both.
- BR-U3-31: Change Category applies the SIC rule source when the user agrees to create the mapping.
- BR-U3-32: A modal-created mapping is an ordinary mapping change (Q5 A): it recategorizes other
  currently uncategorized transactions sharing that code, exactly as the mapping page does.

## Layering, Naming, and Verification

- BR-U3-33: `handler -> service -> repository -> SQLite` is preserved. Handlers make no categorization
  decisions and read no repository directly for this feature.
- BR-U3-34: Dependencies are constructor-injected in `main.go`. No global or singleton is introduced.
- BR-U3-35: Any new page-level JavaScript function name must not collide with a global declared in
  `cmd/privateledger/web/static/js/app.js`. `layout.html` loads `app.js` after page content, so a
  colliding name is silently overwritten at runtime — the defect recorded as U2-F09, where the mapping
  page's delete handler resolved to app.js's `confirmDelete` and deletion could not work at all.
- BR-U3-36: Property-based tests cover the categorization priority matrix — text-before-SIC,
  empty-mapping no-op, and manual preservation — over generated transaction and rule sets (Q6 B).
- BR-U3-37: Property-based tests also cover recategorization scoping: only currently uncategorized
  transactions whose SIC code is in the affected set may change (Q6 B). This is the invariant that
  protects manually categorized data.
- BR-U3-38: `go test ./...` succeeds, with race-detector evidence for the guarded caches.
