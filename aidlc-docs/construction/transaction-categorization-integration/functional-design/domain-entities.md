# Domain Entities — UOW-3 Transaction Categorization Integration

UOW-3 adds no table, column, or index. It reads entities UOW-1 and UOW-2 already persist and writes
only `ledger_transaction.category_id` and `category_source`.

## Reused Without Change

| Entity | Role in this unit |
|---|---|
| `Transaction.SICCode` | Nullable canonical code; the lookup key for SIC categorization |
| `Transaction.SICDescription` | Joined display-only field; the entire data source for US-06 |
| `Transaction.CategoryID` / `CategorySource` | The only fields this unit writes |
| `CategoryPattern` | Text-matching rules, tried first |
| `SICMapping` | Code to optional category; a `NULL` category means never categorize |
| `SICRecategorizationCollaborator` | The UOW-2 contract this unit implements |

`CategorySource` keeps its existing values: `0` none, `1` rule, `2` manual. SIC-assigned categories use
`1`, the same as text patterns — the assignment came from a rule either way, and introducing a fourth
source would change the meaning of existing rows and every query that filters on them.

## SICMappingCategorizer

The SIC-specific categorization extension, kept separate from the core `Categorizer` per the approved
application design rather than absorbed into it.

| Responsibility | Detail |
|---|---|
| Mapping cache | Canonical SIC code to mapping, loaded from SQLite, guarded for concurrent access |
| Lookup | Given a canonical code, return the mapped category or nothing |
| Reload | Rebuild the cache from authoritative state |
| Scoped recategorization | Apply the decision function to uncategorized transactions for a set of codes |

It owns no text-pattern logic and makes no priority decision. Priority lives in the single decision
function so it cannot be stated in two places and drift.

## Rule Cache State

Both caches are derived, in-memory, and process-local.

| Cache | Contents | Refreshed |
|---|---|---|
| Text patterns | Ordered patterns, existing behaviour | At startup and on pattern changes |
| SIC mappings | Canonical code to mapping | At startup and on mapping changes |

Both are guarded and refreshed synchronously (BR-U3-16, BR-U3-17). SQLite stays authoritative: a cache
is an optimization, never a source of truth, and a stale entry can never produce a write that
contradicts a database constraint.

## RecategorizeResult — extended

The existing result gains a source breakdown for Q2 B.

| Field | Meaning |
|---|---|
| `ProcessedCount` | Transactions examined (existing) |
| `CategorizedCount` | Total assigned a category (existing; retained so current callers keep working) |
| `PatternCategorizedCount` | Assigned by a text pattern |
| `SICCategorizedCount` | Assigned by a SIC mapping |

`PatternCategorizedCount + SICCategorizedCount == CategorizedCount` always holds; the two new fields
partition the existing total rather than adding to it.

## Modal SIC Context

A transport-shaped view assembled from fields the transaction read already returns.

| Field | Source |
|---|---|
| SIC code | `Transaction.SICCode`; absent when the transaction has none |
| Display description | `Transaction.SICDescription`, already resolved by the UOW-1 join |
| Existing mapping category | Present when a mapping exists, so the modal can show what the code currently does |

No new query is introduced (BR-U3-27).

## Modal Mapping Command

The create-from-modal command carries the transaction's canonical SIC code, a **required** category ID,
and the description used when the mapping is new. It reuses UOW-2's mapping service rather than writing
to the mapping repository directly, so normalization, uniqueness, backup, and recategorization behave
identically to the mapping page (BR-U3-32).

## Invariants

- A transaction's SIC code is never modified by this unit.
- No mapping is created, updated, or deleted except through UOW-2's mapping service.
- Only `category_id` and `category_source` are written to `ledger_transaction`.
- A transaction with `category_source = 2` is never written by any path in this unit.
- Deleting a mapping never clears categories already assigned under it.
