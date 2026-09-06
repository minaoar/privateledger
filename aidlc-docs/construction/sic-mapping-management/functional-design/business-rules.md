# Business Rules — UOW-2 SIC Mapping Management

## Mapping Rules

- BR-U2-01: Every write uses `model.NormalizeSICCode`; no handler or repository invents another normalization path.
- BR-U2-02: Valid codes are canonical digits in the positive `int64` range established by UOW-1.
- BR-U2-03: Normalized SIC code is globally unique; database enforcement is authoritative.
- BR-U2-04: Create and update accept trimmed `Description` and `Description_Detail` values.
- BR-U2-05: `category_id = NULL` means intentional no SIC categorization and is valid on the dedicated page and in CSV.
- BR-U2-06: A non-null category ID must identify an existing category.
- BR-U2-07: Update may change the SIC code but may not collide with another mapping.
- BR-U2-08: Delete is the only upload-independent way to remove one mapping and requires explicit user confirmation.
- BR-U2-09: Delete and SIC-code changes never clear categories previously assigned to transactions.

## CSV Rules

- BR-U2-10: Column order is exactly `SIC_Code`, `Description`, `Description_Detail`, `Category_Name`, `Category_ID`.
- BR-U2-11: Export always includes the header and uses authoritative SQLite state.
- BR-U2-12: A header-only upload is valid and produces no database changes.
- BR-U2-13: The complete upload is validated before backup or mutation; one invalid row rejects all rows. Size rejection precedes validation (BR-U2-39).
- BR-U2-14: Duplicate normalized SIC codes within one upload reject the complete upload.
- BR-U2-15: Empty category name and ID produce `NULL` category.
- BR-U2-16: A non-empty name resolves case-insensitively; exact case wins, otherwise exactly one case-insensitive match is required.
- BR-U2-17: When name and ID are both supplied, they must resolve to the same category.
- BR-U2-18: Category ID without category name is invalid; an unresolved or ambiguous name is invalid.
- BR-U2-19: Merge inserts new codes and overwrites every editable value for matching codes.
- BR-U2-20: Codes omitted from upload remain unchanged. Upload never deletes.
- BR-U2-21: Applying the same valid file repeatedly is idempotent: later applications classify every row as unchanged.
- BR-U2-22: All row mutations commit or roll back together in one SQLite transaction.

## Shared Contract and Resource Rules

- BR-U2-39: Upload enforces the same size bound as the UOW-1 startup seed. The limit is one shared
  named constant — never a duplicated literal — and the handler rejects an oversized upload before
  parsing, reporting `oversized`. This also settles independent review finding F-14, which recorded a
  `10 << 20` literal duplicating the unexported `maxSICMappingSeedSize`.
- BR-U2-40: Row diagnostics are bounded in the service that produces them, not only at the point of
  display. UOW-1's `maxSICSeedDiagnostics` caps startup logging in the main package, so the upload path
  inherits no bound; a bound at the accumulation site is required so a large invalid upload can neither
  retain unbounded diagnostics nor return an unbounded response body. This closes independent review
  finding F-13, which measured roughly 47.9 MiB retained at the size limit for 50 diagnostics ever
  shown. When truncation occurs, `RejectedRows` stays authoritative and the result sets
  `DiagnosticsTruncated`.
- BR-U2-41: Upload reuses the UOW-1 `ValidateCSV` service method and its `SICMappingImportReport` and
  `SICMappingImportError` types. UOW-2 introduces no parallel validation entry point and no second
  diagnostic type.
- BR-U2-42: `Outcome` values come from the single shared `SICMappingImportOutcome` set. Upload emits
  `merged` on success, never UOW-1's insert-only `imported`; `absent` and `skipped_existing` stay
  startup-only. The UOW-1 design's five-value list is amended to the full shared set, resolving
  independent review finding F-16.
- BR-U2-43: The pre-upload snapshot used for the created/updated/unchanged diff is taken independently
  of the backup write, so a backup failure never invalidates the merge counts.
- BR-U2-44: The service serializes all CRUD and upload mutations with an instance-owned mutex covering
  pre-state capture through reload/handoff. NFR Design defines bounded waiting and concurrency tests.

## Backup and Result Rules

- BR-U2-23: After validation and before mutation, the service attempts a complete pre-upload export to a timestamped backup beside the database.
- BR-U2-24: Backup failure does not prevent merge, but must produce a prominent, non-empty warning and no claimed backup path.
- BR-U2-25: Backup success returns its path and no backup warning.
- BR-U2-26: Import results distinguish created, updated, unchanged, rejected, and recategorized counts.
- BR-U2-27: No result claims rows were merged until the SQLite transaction commits.
- BR-U2-45: After commit, reload or collaborator failure is returned as committed-with-warning with
  `MappingCommitted=true`; handlers return success status and must not imply that retry is required.

## Recategorization Handoff Rules

- BR-U2-28: UOW-2 depends on a narrow injected collaborator, never on handler callbacks or repository-to-service calls.
- BR-U2-29: Affected codes are de-duplicated and include a created code with non-empty category or an existing code whose category changes to a different non-empty category.
- BR-U2-30: Description-only changes, unchanged categories, post-state empty categories, omitted codes, and deletions do not trigger the collaborator.
- BR-U2-31: CRUD invokes the collaborator for its one affected code; upload invokes it once for the complete affected set.
- BR-U2-32: UOW-3 owns the guarantee that only currently uncategorized matching transactions are considered and text patterns retain priority.
- BR-U2-46: Until UOW-3, production wiring supplies a no-op collaborator returning zero; UOW-2 tests
  use a fake to verify calls and affected codes. Because the checkpoint count is structurally zero, the
  UI must not present it as a measured result (see `frontend-components.md`).

## UI and API Rules

- BR-U2-33: Page create/edit uses an existing-stack Bootstrap modal; no new client framework is introduced.
- BR-U2-34: The page labels upload as “Import / Update Mappings” and explains that omitted codes remain.
- BR-U2-35: Destructive delete and import/update each require browser confirmation before the request.
- BR-U2-36: Validation errors identify CSV row, field, and stable error code without echoing unnecessary file content.
- BR-U2-37: Interactive controls use stable `data-testid` values.
- BR-U2-38: Handlers map malformed input, validation/conflict, not-found, and internal failures consistently while leaving business decisions in the service.
