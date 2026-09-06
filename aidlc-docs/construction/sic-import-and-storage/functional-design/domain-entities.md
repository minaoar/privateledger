# Domain Entities — UOW-1 SIC Import and Storage

## Domain Overview

```text
Account 1 ---- * Transaction * ---- 0..1 ImportBatch
                     |
                     | optional SICCode value
                     v
               canonical digits
                     |
                     | matched by value (not foreign key)
                     v
                0..1 SICMapping * ---- 0..1 Category

StartupMappingFile --validates--> SICMapping candidates --atomic seed--> SQLite
```

Transactions deliberately do not reference a mapping row. They retain the source SIC value even when mappings are deleted or replaced.

## SICCode Value Object

### Purpose

Represents a canonical, matchable SIC value shared by imported transactions and mappings.

### Logical representation

| Property | Definition |
|---|---|
| Raw input | String from CSV/API or decimal form of parser `int64` |
| Canonical form | Minimal base-10 string without whitespace or leading zeros |
| Valid range | `1` through `9223372036854775807` |
| Equality | Exact equality of canonical strings |
| Nullability | The value object itself is non-null; owning transaction may omit it |

### Operations

- `Normalize(raw) -> canonical candidate`
- `Validate(canonical candidate) -> valid SICCode or domain error`
- `Equals(other) -> boolean`

### Invariants

- Contains digits only.
- Never empty or zero.
- Fits positive signed 64-bit range.
- Has no leading zero.
- Equivalent numeric inputs such as `111`, `0111`, and ` 00111 ` share one identity.

## Transaction

### Relevant fields

| Field | Type | Required | Rule |
|---|---|---:|---|
| TransactionID | Integer | After persistence | Internal identity |
| AccountID | Integer | Yes | Existing account relationship |
| ImportBatchID | Optional integer | No | Existing import history relationship |
| TrnType | String | Yes | Existing duplicate-key member |
| FitID | String | Yes | Existing duplicate-key member |
| DatePosted | Timestamp | Yes | Existing duplicate-key member |
| Amount | Decimal/number | Yes | Existing transaction data |
| TransactionDetails | String | Yes | Existing merged name/memo |
| TransactionType | Enum | Yes | Existing debit/credit derivation |
| SICCode | Optional SICCode | No | New source metadata; not identity |
| CategoryID / Source | Existing optional categorization | No | Unchanged by UOW-1 semantics |

### Lifecycle

1. Constructed from a valid parsed OFX transaction.
2. Optional positive parser SIC is converted to a SICCode.
3. Checked for duplicates using the existing composite identity.
4. Persisted with SIC as nullable text.
5. Read later by UOW-3 for categorization and modal context.

### Invariants

- SIC absence cannot prevent import.
- SIC does not participate in equality/deduplication.
- Stored SIC, when present, is canonical.

## SICMapping

### Relevant fields

| Field | Type | Required | Rule |
|---|---|---:|---|
| SICMappingID | Integer | After persistence | Internal identity; never imported/exported |
| SICCode | SICCode | Yes | Globally unique canonical value |
| Description | String | No | Display/help metadata |
| DescriptionDetail | String | No | Secondary display/help metadata |
| CategoryID | Optional integer | No | `NULL` means intentionally no SIC category |
| CreatedAt | Timestamp | After persistence | Audit metadata |
| Category display fields | Derived | No | Joined values, not stored in mapping domain state |

### Relationships

- References zero or one Category.
- Category deletion sets `CategoryID` to `NULL` and preserves the mapping.
- Matches any number of transactions by SICCode value; there is no database foreign key from transaction to mapping.

### Invariants

- Exactly one mapping per canonical SICCode.
- An absent category is valid and intentional.
- Descriptions do not determine identity.

## Category

### Role in UOW-1

Category is an existing aggregate referenced during startup CSV resolution. UOW-1 does not create or modify categories.

### Relevant invariants

- Stored names are unique under the current case-sensitive database constraint.
- Case variants may coexist, so case-insensitive lookup may be ambiguous.
- Category name is the primary interchange resolver; ID only confirms a name result.

### Resolution operation

`ResolveCategory(name, optionalID) -> optional Category or row error`

Resolution prefers an exact stored name, then a unique case-insensitive match. A supplied ID must agree with the name result.

## StartupMappingFile

### Purpose

Represents the optional local import/export interchange file discovered during startup.

### Fields

| Field | Meaning |
|---|---|
| Path | Fixed application-data path ending in `sic_mappings.csv` |
| Header | Five required interchange columns |
| Rows | Ordered mapping input records |

### Lifecycle

- Absent: no domain object is created; startup proceeds.
- Present with existing database mappings: discovered but not parsed; skip is logged.
- Present with empty mapping table: parsed fully into candidates and an import report.
- Valid: candidates are atomically persisted.
- Invalid: candidates are discarded, report is logged, startup proceeds.

## SICMappingInputRow

| Field | Input semantics |
|---|---|
| RowNumber | Diagnostic identity; not persisted |
| SIC_Code | Required raw SIC input |
| Description | Optional display text |
| Description_Detail | Optional detail text |
| Category_Name | Optional primary category reference |
| Category_ID | Optional confirming category reference; unsafe alone |

The row is not a persistent entity. It becomes a SICMapping candidate only after code and category validation.

## SICMappingImportReport

### Purpose

Carries validation and startup diagnostics without mutating persistence.

### Logical fields

| Field | Meaning |
|---|---|
| TotalRows | Data rows encountered |
| ValidRows | Rows that independently validated |
| RejectedRows | Rows with one or more errors |
| Errors | Row number, field/code, and safe diagnostic message |
| ImportedRows | Zero unless the complete candidate set commits |
| Outcome | One of the shared `SICMappingImportOutcome` set: Absent, SkippedExisting, Invalid, Oversized, Validated, Imported, ReadFailed, PersistenceFailed, or Merged |

### Invariants

- Any rejected row makes `ImportedRows = 0` for startup seeding.
- Diagnostics do not contain full financial transaction data.
- A valid report does not imply persistence until atomic commit succeeds.

> **Amendment (2026-09-06, UOW-2 Functional Design)**: the outcome list above previously named five
> values while the UOW-1 implementation declared eight, recorded as independent review finding F-16.
> The list is now the single shared set used by both units. `Merged` is added by UOW-2's upload path;
> `Absent` and `SkippedExisting` remain startup-seed-only; `Imported` denotes the insert-only startup
> seed and is never emitted by upload. See UOW-2 `business-rules.md` BR-U2-42.

## Repository Contracts

### Transaction persistence

- Create accepts optional SICCode and writes nullable `sic_code`.
- Reads return optional SICCode.
- Duplicate lookup remains SIC-independent.
- SIC-scoped query support is exposed for downstream UOW-3.

### Mapping persistence

- Count supports the startup empty-table gate.
- Create/atomic bulk insert enforces unique SIC and valid category foreign keys.
- Reads return canonical SIC and nullable category.
- Persistence errors are translated with enough context for service-level decisions.

## Ownership and Layering

| Concern | Owner |
|---|---|
| SIC canonicalization | Domain model/value helper |
| OFX extraction | Parser |
| Transaction construction/orchestration | Existing import flow |
| Schema evolution | Database migration layer |
| Transaction/mapping storage | Repositories |
| Startup file discovery and validation orchestration | SIC mapping service startup operation |
| Logging startup outcome | Startup/service boundary |

No handler or frontend participates in UOW-1.

## Downstream Contracts

### For UOW-2

- Stable SICMapping entity and canonical SIC identity.
- Nullable category semantics and schema uniqueness.
- Repository CRUD/count/atomic replacement foundations.
- Category resolution behavior to reuse for uploads.

### For UOW-3

- Transaction SIC stored as optional canonical text.
- Transaction reads expose SIC without changing deduplication.
- Transactions are not coupled by foreign key to mappings.
- SIC index/query foundation supports scoped recategorization.
