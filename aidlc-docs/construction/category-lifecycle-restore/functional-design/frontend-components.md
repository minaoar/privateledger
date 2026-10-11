# Frontend Components — UOW-4 Category Lifecycle and Mapping-File Integrity

## Technology Boundary

Existing server-rendered templates, Bootstrap 5, HTMX where conventional, and vanilla JavaScript. No new
template, page, route, framework or library. UOW-4 changes no markup structure.

## What Changes on Screen

Only the text of upload diagnostics. The mapping page's existing diagnostic list already renders
row-level errors from the upload response and shows the authoritative `RejectedRows` total alongside the
shown subset when truncation occurs. Those mechanics are unchanged; the messages within them become
specific.

| Situation | What the user now reads |
|---|---|
| Category renamed since the file was produced | The unresolved name, and the current name of the category the file's `Category_ID` refers to — enough to repair the file in one find-and-replace |
| Two categories differ only by case | Both colliding category names |
| Header wrong | Which column failed |

## Naming Constraint

No JavaScript is added by this unit. The constraint still applies to any that is: `layout.html` loads
`/static/js/app.js` after page content, so a page-level function sharing a name with an `app.js` global
is silently overwritten at runtime while the markup still looks correct — defect U2-F09, where mapping
deletion could not work at all. The reviewer-owned collision regression added in UOW-2 covers this
automatically.

## Escaping

Diagnostics now contain category names, which are user-controlled text. They continue to be rendered
through DOM `textContent`, as the mapping page already does for its diagnostic list, never assembled as
raw HTML.

## Explicitly Unchanged

- No new control, modal, page or route.
- No change to the upload, download, create, edit or delete flows.
- No change to confirmation behaviour or to the busy/disabled handling on any form.
- Export continues to produce the canonical header.
