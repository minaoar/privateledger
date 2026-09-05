# Business Rules — UOW-1 SIC Import and Storage

## SIC Value Rules

### BR-SIC-01 — Optional transaction value

`Transaction.SICCode` is nullable. An absent OFX `<SIC>` and parsed SIC zero both produce `nil`/SQL `NULL`.

### BR-SIC-02 — Canonical representation

A present SIC is stored as the minimal decimal string for a positive signed 64-bit integer. Surrounding whitespace and leading zeroes are not part of identity.

### BR-SIC-03 — Valid mapping domain

User/file mapping codes must:

- be non-empty after trimming;
- contain ASCII digits only before canonicalization;
- canonicalize to a value greater than zero;
- be no greater than `9223372036854775807`;
- use at most 19 canonical digits.

This domain exactly matches positive SIC values representable by the parser.

### BR-SIC-04 — Normalization ownership

One domain normalization function owns whitespace trimming, leading-zero removal, zero handling, and canonical decimal formatting. Validation adds the positive-int64 constraint. Parser, startup CSV, later CRUD/upload, uniqueness checks, and lookups must not implement alternate normalization.

### BR-SIC-05 — Global mapping uniqueness

Only one mapping may exist for a canonical SIC code. The database unique constraint is authoritative. Consequently, `0111` and `111` are duplicates.

## Transaction Rules

### BR-TXN-01 — SIC is not identity

SIC is excluded from the transaction duplicate key. Adding or changing SIC support must not alter duplicate detection results.

### BR-TXN-02 — SIC persistence

Every new non-duplicate transaction persists its optional canonical SIC alongside existing fields. Every repository read needed by downstream units returns it.

### BR-TXN-03 — Existing behavior preservation

Transactions without SIC follow the same parser, categorization, insertion, counting, and API behavior as before UOW-1.

### BR-TXN-04 — Parser failure boundary

A non-numeric `<SIC>` is rejected by `ofxgo` as part of whole-file parsing. UOW-1 does not pre-extract or recover such values.

## Schema and Migration Rules

### BR-MIG-01 — Fresh schema

Fresh databases create:

- nullable `ledger_transaction.sic_code`;
- `sic_mapping` with a primary key, canonical unique `sic_code`, descriptions, nullable category reference, and creation timestamp;
- an index on transaction SIC values.

### BR-MIG-02 — Existing schema

Existing databases gain the nullable transaction column through explicit column detection and additive alteration. Adding the column to `CREATE TABLE IF NOT EXISTS` alone is insufficient.

### BR-MIG-03 — Data preservation

Migration must not drop, rebuild, truncate, or overwrite existing financial tables or rows.

### BR-MIG-04 — Repeatability

Running migration any number of times produces the same schema and no duplicate-column/table/index error.

Migration order is mandatory: create/ensure tables, ensure `ledger_transaction.sic_code`, then create
`idx_txn_sic`. The new index must not execute in a pre-column schema batch against legacy databases.

### BR-MIG-05 — Foreign-key semantics

`sic_mapping.category_id` is nullable and references `category`. Category deletion sets the mapping reference to `NULL`, preserving the SIC mapping as intentionally unmapped rather than deleting it.

### BR-MIG-06 — Migration failure

The application must not continue with a database whose required migration failed. The open operation closes the connection and reports contextual failure.

## Startup Mapping File Rules

### BR-CSV-01 — Fixed discovery location

The optional file is named `sic_mappings.csv` and resides in the same application data directory as `config.json` and `privateledger.db`.

### BR-CSV-02 — Absence is normal

If the file does not exist, startup continues successfully without logging an error and without creating mappings.

### BR-CSV-03 — SQLite is authoritative

If the mapping table contains one or more rows, startup skips the file, logs the reason, and performs no comparison, update, deletion, or overwrite.

### BR-CSV-04 — Required columns

The seed file uses exactly the named interchange fields `SIC_Code`, `Description`, `Description_Detail`, `Category_Name`, and `Category_ID`. Internal mapping IDs are never imported.

### BR-CSV-05 — Whole-file validation

Every row is parsed and validated before persistence. Duplicate canonical SIC codes, malformed structure, invalid codes, invalid categories, or conflicting references make the seed invalid.

### BR-CSV-06 — Invalid seed is non-fatal and atomic

If any row is invalid, zero rows are imported, a per-row report is logged, and application startup continues with an empty mapping table.

### BR-CSV-07 — Valid seed persistence

A fully valid seed is inserted in one database transaction. Any persistence failure rolls back every seed row.

### BR-CSV-08 — Descriptions

`Description` and `Description_Detail` may be empty and are stored for downstream display/help. They do not affect SIC uniqueness or category resolution.

## Category Reference Rules

### BR-CAT-01 — Empty reference

Empty `Category_Name` and empty `Category_ID` are accepted and produce `category_id = NULL`.

### BR-CAT-02 — Name required for categorized mappings

A non-empty `Category_ID` with an empty `Category_Name` is rejected. Category ID never resolves a mapping by itself.

### BR-CAT-03 — Name normalization

Surrounding whitespace is trimmed from CSV category names before lookup.

### BR-CAT-04 — Name matching precedence

Resolution first seeks an exact case-sensitive category name. If none exists, it accepts a case-insensitive match only when exactly one stored category matches.

### BR-CAT-05 — Ambiguous or absent name

When no exact match exists, zero case-insensitive matches means not found and multiple matches means ambiguous. Both reject the row.

### BR-CAT-06 — ID confirmation

When both name and ID are present, name resolves the category and ID must equal that resolved category's ID. Mismatch, malformed ID, zero, or negative ID rejects the row.

## Error and Logging Rules

### BR-ERR-01 — No sensitive payload logging

Logs may include file path, row number, field name, error code, and counts, but must not dump transaction data or entire financial files.

### BR-ERR-02 — Row diagnostics

Invalid startup CSV reporting identifies every detected invalid row where practical, with deterministic reasons such as invalid SIC, duplicate SIC, missing category, ambiguous category, unsafe ID-only reference, or name/ID conflict.

### BR-ERR-03 — Skip diagnostics

A non-empty mapping table produces an informational skip log including the existing mapping count; it is not an error.

## Decision Tables

### Startup file decision

| File | Mapping count | Validation | Action |
|---|---:|---|---|
| Absent | any | Not run | Continue; no mutation |
| Present | > 0 | Not run | Log skip; no mutation |
| Present | 0 | Invalid | Log report; import none; continue |
| Present | 0 | Valid | Atomically insert all; continue |

### Category reference decision

| Name | ID | Resolution | Result |
|---|---|---|---|
| Empty | Empty | None | Accept `NULL` category |
| Empty | Set | ID-only | Reject |
| Set | Empty | Exact or unique case-insensitive | Accept resolved category |
| Set | Set | Name resolves and ID agrees | Accept resolved category |
| Set | Set | Name resolves and ID disagrees | Reject |
| Set | Any | Name absent or ambiguous | Reject |

## Traceability Matrix

| Requirement | Rules |
|---|---|
| FR1 | BR-SIC-01 through BR-SIC-04, BR-TXN-02 |
| FR2 | BR-TXN-01 through BR-TXN-03, BR-MIG-01 |
| FR3 | BR-MIG-01 through BR-MIG-06 |
| FR4 | BR-CSV-01 through BR-CSV-08, BR-CAT-01 through BR-CAT-06 |
| FR11 foundation | BR-SIC-02 through BR-SIC-05 |
| NFR1 | BR-ERR-01 and local-only boundary |
| NFR2 | BR-TXN-03, BR-MIG-03 |
| NFR3 | BR-SIC-04 and repository/service ownership |
| NFR4 | BR-MIG-04, BR-CSV-03, BR-CSV-06, BR-CSV-07 |
