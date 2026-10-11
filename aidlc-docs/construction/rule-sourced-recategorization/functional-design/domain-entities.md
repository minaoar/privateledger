# Domain Entities — UOW-5 Rule-Sourced Recategorization

UOW-5 adds no table, column or index, and changes no persisted shape. What changes is which rows an
existing operation reads and writes, and the shape of one in-memory result.

## Reused Without Change

| Entity | Role in this unit |
|---|---|
| `Transaction` | Read for every non-manual row; `category_id` and `category_source` are written where the outcome differs. No field is added |
| `CategoryPattern` | Read only. A change to any pattern is a trigger |
| `SICMapping` | Read only. A change to any mapping is a trigger |
| `Category` | Read only. Its deletion is a trigger, via the cascades below |

## Persisted Values

`category_source` keeps its three values — `0=none, 1=rule, 2=manual`. **No value is added.** R1 B, which
would have required recording which rule categorized each transaction, was declined at the requirements
stage precisely to avoid that.

A transaction that loses its category is written `category_id = NULL, category_source = 0`, which is the
representation category deletion already produces. Introducing a second would silently break the
uncategorized dashboard and every query that defines "uncategorized".

## Cascades This Unit Depends On

Not new, but load-bearing here for the first time, and verified against `schema.sql`:

| Constraint | Effect when a category is deleted |
|---|---|
| `category_pattern.category_id` `ON DELETE CASCADE` | Its patterns are deleted — a rule change |
| `sic_mapping.category_id` `ON DELETE SET NULL` | Its mappings become empty-category, assigning nothing — also a rule change |
| `ledger_transaction.category_id` `ON DELETE SET NULL` | Its transactions are nulled before re-examination sees them |

BR-U5-09 depends on all three completing before rules are reloaded.

## Result Shape

The re-examination result carries three counts rather than the single integer UOW-2's collaborator
returns today: moved, uncategorized, and manual-protected. This is an in-memory and transport shape, not
a persisted one.

The single integer could not express FR16's three counts, which is one half of why FD-FQ1 replaced the
contract rather than keeping it.

## Invariants

- No schema change, no migration, no new column or index.
- `category_source = 2` rows are read for counting and **never written**.
- Exactly one representation of "uncategorized" exists in the database.
- Re-examination is idempotent: a second consecutive pass writes nothing.
