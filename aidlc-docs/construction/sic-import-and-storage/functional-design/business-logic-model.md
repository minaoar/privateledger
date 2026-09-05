# Business Logic Model — UOW-1 SIC Import and Storage

## Purpose and Boundary

UOW-1 turns optional SIC data in OFX/QFX transactions into stable local domain data, upgrades existing databases without data loss, and optionally seeds SIC mappings from a local CSV. It does not apply SIC mappings to transactions or provide mapping-management UI.

## Actors and Inputs

| Actor/Input | Role |
|---|---|
| Local user | Starts PrivateLedger and imports an OFX/QFX file |
| OFX/QFX document | Supplies transactions and optional numeric `<SIC>` values |
| Existing SQLite database | May require an in-place schema upgrade |
| `sic_mappings.csv` | Optional startup seed beside the database/configuration |
| Existing categories | Resolve CSV category references locally |

All inputs and outputs remain on the user's machine.

## Flow 1 — Database Initialization and Migration

```text
Open configured SQLite database
        |
        v
Enable foreign-key enforcement
        |
        v
Execute table definitions idempotently
        |
        v
Does ledger_transaction contain sic_code?
   | yes                         | no
   v                             v
Continue                 ALTER TABLE to add nullable sic_code
   |                             |
   +--------------+--------------+
                  v
Create idx_txn_sic only after sic_code is guaranteed to exist
                  |
                  v
Return usable database or fail startup
```

### Migration outcomes

- A fresh database receives `ledger_transaction.sic_code` and `sic_mapping` from the table definitions, then receives `idx_txn_sic` in the post-column migration step.
- An existing database receives the missing nullable transaction column without rebuilding or deleting the table.
- The SIC index is deliberately excluded from any schema batch that runs before `ensureColumn`; otherwise an existing database would fail on `CREATE INDEX ... (sic_code)` before the missing column could be added.
- Existing accounts, transactions, categories, patterns, and import history remain unchanged.
- Repeated migration observes that all objects exist and makes no data changes.
- Any migration failure prevents the application from using a partially initialized database and fails startup with context.

## Flow 2 — OFX/QFX Transaction SIC Extraction

```text
Read bounded local import stream
        |
        v
Parse document through ofxgo
        |
        +-- parse failure --> reject whole OFX/QFX import
        |
        v
For each structurally valid transaction
        |
        +-- SIC == 0 --> SICCode = unset
        |
        +-- SIC > 0 --> decimal string --> NormalizeSICCode --> SICCode = canonical value
        |
        v
Apply existing transaction construction and categorization flow
        |
        v
Check existing composite duplicate key
        |
        +-- duplicate --> skip as today
        |
        v
Insert transaction including nullable sic_code
```

### Transformation

For parser-produced values, `ofxgo.Transaction.SIC` is an `int64`. A positive value is converted to base-10 digits and normalized. The integer parse has already removed source leading zeros. Zero represents both absent `<SIC>` and `<SIC>0`; both remain unset.

The transaction deduplication identity remains:

```text
(account_id, trn_type, fit_id, date_posted)
```

SIC never changes whether two imported transactions are duplicates.

## Flow 3 — SIC Normalization and Validation

Normalization is deterministic and shared by parser, CSV, services, and repository lookups:

1. Trim surrounding whitespace.
2. Require at least one character.
3. Require every character to be an ASCII decimal digit.
4. Remove leading zeroes.
5. If no digit remains, the canonical value is `0` and validation rejects it.
6. Require the canonical value to contain at most 19 digits.
7. Parse/range-check it as a positive signed 64-bit integer.
8. Return its minimal base-10 representation.

Accepted domain: `1` through `9223372036854775807`, inclusive.

Examples:

| Raw value | Result |
|---|---|
| ` 0111 ` | `111` |
| `1` | `1` |
| `000` | rejected: zero/unmatchable |
| `12A4` | rejected: non-digit |
| `9223372036854775807` | accepted |
| `9223372036854775808` | rejected: overflow |

## Flow 4 — Optional Startup Mapping Import

Startup mapping initialization runs only after migration succeeds and repositories are available.

```text
Resolve <application-data-dir>/sic_mappings.csv
        |
        +-- file absent --> continue startup; no warning/error
        |
        v
Count mappings in SQLite
        |
        +-- count > 0 --> log skip; existing SQLite mappings remain authoritative
        |
        v
Read and parse complete CSV
        |
        v
Validate header and every row without mutation
        |
        +-- any invalid row --> log per-row report; import nothing; continue startup
        |
        v
Atomically insert all validated mappings
        |
        +-- persistence failure --> rollback; report startup initialization error
        |
        v
Log imported count; continue startup
```

### CSV row transformation

Expected columns, in interchange order:

```text
SIC_Code,Description,Description_Detail,Category_Name,Category_ID
```

For each row:

1. Normalize and validate `SIC_Code` using the shared positive-int64 rule.
2. Preserve description text as data; surrounding field whitespace is not semantically significant for category/code resolution.
3. Resolve category fields using the category-resolution algorithm below.
4. Detect duplicate canonical SIC codes within the same file.
5. Construct an in-memory mapping candidate.

No row is written until every row passes.

## Category Resolution Algorithm

1. Trim surrounding whitespace from `Category_Name` and `Category_ID`.
2. If both are empty, resolve to `category_id = NULL` (intentional no-category mapping).
3. If name is empty and ID is set, reject the row; ID-only references are unsafe.
4. If name is set:
   - Prefer one exact case-sensitive stored-name match.
   - Otherwise find case-insensitive matches.
   - Accept when exactly one case-insensitive match exists.
   - Reject when none or more than one exists.
5. If `Category_ID` is also set, require it to parse as a valid positive identifier and equal the category resolved by name; otherwise reject.

This permits user-friendly case-insensitive CSV names without silently choosing between categories such as `Food` and `food`.

## Atomicity and Idempotency

- Migration steps are idempotent by schema inspection and `IF NOT EXISTS` behavior.
- Startup import is gated on an empty mapping table. Once any mapping exists, the file is never reapplied automatically.
- CSV validation is pure: it produces candidates and a report without persistence.
- A mixed valid/invalid startup file imports zero rows.
- Persistence of a fully valid seed is one transaction: all mappings commit or none do.
- A successful first import makes subsequent startups skip the seed because the mapping table is no longer empty.

## Error Model

| Failure | Mutation | Startup/import outcome |
|---|---|---|
| Database cannot open or foreign keys cannot enable | None/connection closed | Application startup fails |
| Schema migration fails | Transaction/schema engine semantics; database closed | Application startup fails |
| OFX parse fails, including non-numeric SIC | No imported transactions from file | File import fails |
| Transaction lacks FITID/date | Existing behavior: transaction skipped | Other valid transactions continue |
| Optional seed file absent | None | Startup continues silently |
| Mapping table non-empty | None | Startup continues; skip logged |
| Seed CSV malformed or any row invalid | None | Startup continues; per-row errors logged |
| Seed persistence fails | Atomic rollback | Startup initialization returns an error; no partial mappings |

## Traceability

| Story/Requirement | Functional coverage |
|---|---|
| US-01 / FR1, FR2, FR12 | Flow 2, nullable persistence, unchanged deduplication and result contract |
| US-07 / FR3, NFR2, NFR4 | Flow 1 and migration idempotency |
| US-08 / FR4, NFR4 | Flow 4, category resolution, validation atomicity, empty-table gate |
| US-09 / NFR1 | Local actors/inputs and no external interaction |
| NFR3 | Parser/model/repository/service responsibilities remain layered |
