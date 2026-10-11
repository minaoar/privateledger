# Domain Entities — UOW-2 SIC Mapping Management

## SICMapping

UOW-2 extends use of the UOW-1 entity without redefining its persistence contract.

| Field | Type | Meaning |
|---|---|---|
| `SICMappingID` | integer | SQLite identity; API update/delete target |
| `SICCode` | string | Required canonical positive-int64 digits; globally unique |
| `Description` | string | Editable primary display text |
| `DescriptionDetail` | string | Editable fallback/detail text |
| `CategoryID` | nullable integer | Existing category or intentional empty mapping |
| Category display fields | joined/read-only | Name, color, and icon for page/API output |

The mapping ID remains stable during update, including when the SIC code changes. CSV identity is the
normalized SIC code; internal IDs are never imported.

## SICMappingInput

Transport-neutral create/update command containing SIC code, descriptions, and optional category ID.
It never contains joined display fields. Update additionally carries the route-resolved mapping ID.

## SICMappingPageData

Contains the numerically ordered mappings and current category choices required for one server render.
It is assembled by `SICMappingService`, not the page handler.

## SICMappingCSVRow

| Field | Required | Rule |
|---|---|---|
| `SIC_Code` | yes | Normalize, then validate and de-duplicate |
| `Description` | no | Stored exactly after agreed trimming |
| `Description_Detail` | no | Stored exactly after agreed trimming |
| `Category_Name` | no | Primary category resolver |
| `Category_ID` | no | Confirmation only; invalid without name |

## Row Diagnostics — reused from UOW-1

UOW-2 defines no new diagnostic type. Row-level diagnostics reuse the UOW-1 `SICMappingImportError`
contract — row number, field, stable code, and safe message — so startup seeding and upload share one
validation vocabulary. Diagnostics describe validation only and do not imply that any row was
persisted.

Diagnostics are bounded per BR-U2-40. When the bound truncates the list, `RejectedRows` remains the
authoritative total and the result sets `DiagnosticsTruncated`.

## SICMappingMergeCounts

Contains `Created`, `Updated`, and `Unchanged`. Their sum equals the number of validated uploaded rows
after a successful commit. For header-only input all three are zero.

## SICMappingImportResult

Upload extends the UOW-1 `SICMappingImportReport` rather than replacing it. The shared fields keep
their UOW-1 meanings and JSON names; UOW-2 adds only the merge-specific and backup fields below. A
service returning this result must be able to produce the UOW-1 report shape unchanged for the startup
seed path.

Shared with `SICMappingImportReport`: `TotalRows`, `ValidRows`, `RejectedRows`, `ExistingRows`,
`Errors`, and `Outcome`. UOW-1's `ImportedRows` is superseded for the merge path by the
created/updated/unchanged breakdown below, and stays zero on upload.

| Field | Meaning |
|---|---|
| `TotalRows` | Data rows parsed |
| `ValidRows` / `RejectedRows` | Validation outcome |
| `CreatedRows` | Codes absent before merge |
| `UpdatedRows` | Existing codes whose stored editable values changed |
| `UnchangedRows` | Existing codes already equal to uploaded values |
| `RecategorizedRows` | Count returned by the collaborator |
| `BackupPath` | Non-empty only when backup succeeded |
| `BackupWarning` | Non-empty only when backup failed but processing continued |
| `MappingCommitted` | True only after the SQLite merge commits; disambiguates post-commit warnings |
| `PostCommitWarnings` | Cache-reload or collaborator failures occurring after a successful commit |
| `Diagnostics` | The UOW-1 `Errors` list surfaced under the upload result; bounded safe per-row validation details (BR-U2-40) |
| `DiagnosticsTruncated` | True when the bound dropped diagnostics; `RejectedRows` then exceeds `len(Diagnostics)` |

Persistence counts stay zero on validation or merge failure. Backup outcome remains reportable even
when a later stage fails.

### Outcome vocabulary

`Outcome` is the UOW-1 `SICMappingImportOutcome` enumeration, extended once here so both units share
one set. The upload path uses `invalid`, `read_failed`, `oversized`, `validated`, `persistence_failed`,
and the merge-specific `merged`; `absent` and `skipped_existing` remain startup-seed-only. UOW-1's
`imported` denotes an insert-only startup seed and is never emitted by upload — a successful merge
reports `merged`, whose row breakdown is `CreatedRows` / `UpdatedRows` / `UnchangedRows`.

This reconciles independent review finding F-16, which recorded that the implementation declared eight
outcomes while the approved UOW-1 design listed five. The UOW-1 `domain-entities.md` outcome list is
amended to match.

## SICRecategorizationCollaborator

A narrow service-level contract accepting a de-duplicated set of normalized SIC codes and returning a
recategorized count or error. UOW-2 owns when and with which codes it is invoked; UOW-3 owns transaction
selection and categorization semantics. Constructor injection supplies the collaborator, preserving a
directed dependency graph and testability without global state.

At the UOW-2 checkpoint, production wiring uses a no-op collaborator that returns zero. UOW-3 replaces
it with the real adapter. Independent UOW-2 verification uses a fake to assert selected codes and call
count.

## Relationships and Invariants

- `SICMapping.CategoryID` optionally references Category with the existing delete behavior.
- Transactions hold SIC values, not mapping IDs; mapping edits/deletes cannot orphan transactions.
- One normalized SIC code identifies at most one mapping.
- Upload is a partial-state merge, not a snapshot replacement.
- Explicit delete is the only removal workflow in UOW-2.
