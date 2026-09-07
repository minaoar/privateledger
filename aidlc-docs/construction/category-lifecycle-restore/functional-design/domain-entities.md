# Domain Entities — UOW-4 Category Lifecycle and Mapping-File Integrity

UOW-4 adds no entity, table, column or index, and changes no persisted shape. It is listed here for
completeness rather than because anything new exists.

## Reused Without Change

| Entity | Role in this unit |
|---|---|
| `SICMapping` | Unchanged. No field is added, and no mapping is written by any path this unit touches |
| `Category` | Read only, for resolution and for naming categories in diagnostics |
| `SICMappingImportError` | The diagnostic carrier. Its `RowNumber`, `Field`, `Code` and `Message` shape is unchanged |
| `SICMappingImportReport` / `SICMappingImportResult` | Unchanged. Counts, outcome and truncation behave exactly as UOW-2 defined |

## Diagnostic Codes

Existing stable codes are retained; only the human-readable message becomes more specific. Keeping the
codes stable means anything matching on them, including the reviewers' tests, is unaffected.

| Code | Message change |
|---|---|
| `category_not_found` | Names the unresolved value, and the current name of the category `Category_ID` refers to when it exists |
| `category_ambiguous` | Names the colliding categories |
| `invalid_header` | Identifies which column failed to match |

No code is added, renamed or removed.

## Header Normalization

Normalization is a comparison-time transformation, not stored state. The canonical column names remain
the single definition of the contract, and export emits them unchanged. A normalized form is never
persisted, returned, or echoed.

## Invariants

- No mapping, category or transaction is written by any path this unit changes.
- A rejected upload mutates nothing.
- Export output is byte-identical to before this unit.
- Diagnostics contain only values from this database and fixed text; never file content.
