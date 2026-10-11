# Frontend Components — UOW-2 SIC Mapping Management

## Technology Boundary

Use the existing server-rendered templates, Bootstrap components, HTMX where already conventional,
and vanilla JavaScript. Add no frontend framework or production library.

## Component Hierarchy

```text
SIC Mapping Page
├── Page heading and link back to Categories
├── Action bar
│   ├── Create Mapping button
│   ├── Download CSV link
│   └── Import / Update Mappings button + hidden file input
├── Status/alert region
├── Mapping table
│   └── Mapping row: code, description, detail, category/empty badge, Edit, Delete
├── Create/Edit Bootstrap modal
└── Delete confirmation modal/dialog
```

## Mapping Table

- Server-render all mappings in numeric SIC order.
- Show both description fields and category display, using an explicit “No SIC category” state for
  `NULL` category.
- Store mapping values needed by the edit modal in safe row data attributes or a compact existing-style
  client model; never generate unstable selectors.
- Initial scope has no server pagination. Optional client filtering may be used only through existing
  conventions and must not change API contracts.

## Create/Edit Modal

State includes mode, mapping ID for edit, SIC code, description, description detail, optional category,
field errors, and submitting status. Every field is editable. On edit, changing the SIC code is
permitted and server uniqueness remains authoritative.

Validation shown before submit mirrors obvious rules (required digits-only code), but server validation
is authoritative. On success, refresh or update the table using established page conventions. On
failure, retain entered values and show field/general errors.

## Import / Update Flow

1. User selects one CSV file.
2. Confirmation states: uploaded codes will be added or updated; codes absent from the file will remain;
   deletion is separate.
3. Submit one multipart request to `POST /api/sic-mappings/upload`.
4. Display created, updated, unchanged, and rejected counts. Show the recategorized count only when a
   real collaborator is wired; at the UOW-2 checkpoint the no-op returns zero (BR-U2-46), so displaying
   it unlabelled would read as "nothing matched" rather than "not yet implemented".
5. If backup succeeded, display its local path. If it failed, show a prominent warning while still
   showing the merge outcome.
6. Render row diagnostics for invalid input; no database changes occurred in this case. When
   `DiagnosticsTruncated` is true, show the authoritative `RejectedRows` total alongside the shown
   subset rather than implying the list is complete (BR-U2-40).
7. A header-only success displays a no-change message.
8. A file rejected for exceeding the upload size bound reports `oversized` with the limit, and states
   that nothing was parsed or changed (BR-U2-39).
9. If `MappingCommitted` is true with post-commit warnings, state clearly that mappings were saved and
   should not be blindly re-uploaded; display the reload/recategorization warning separately.

## Download and Delete

- Download uses `GET /api/sic-mappings/download` and preserves filename `sic_mappings.csv`.
- Delete requires confirmation naming the SIC code, calls `DELETE /api/sic-mappings/:id`, and removes
  the row only after success. Explain that existing transaction category assignments are not cleared.

## API Integration

| Interaction | Endpoint |
|---|---|
| List/refresh mappings | `GET /api/sic-mappings` |
| Create | `POST /api/sic-mappings` |
| Update | `PUT /api/sic-mappings/:id` |
| Delete | `DELETE /api/sic-mappings/:id` |
| Download | `GET /api/sic-mappings/download` |
| Import/update | `POST /api/sic-mappings/upload` |

## Stable Automation Identifiers

Use stable identifiers such as `sic-mapping-create-button`, `sic-mapping-download-link`,
`sic-mapping-upload-input`, `sic-mapping-upload-button`, `sic-mapping-form`,
`sic-mapping-form-submit-button`, `sic-mapping-delete-confirm-button`, and
`sic-mapping-status-alert`. Row actions may suffix the persistent mapping ID.

## Accessibility and Failure State

- Associate labels and validation messages with inputs.
- Move focus into opened modals and restore it on close using Bootstrap behavior.
- Use accessible alert semantics for validation, backup warnings, and operational errors.
- Disable duplicate submissions while a request is in flight, then always restore controls.
