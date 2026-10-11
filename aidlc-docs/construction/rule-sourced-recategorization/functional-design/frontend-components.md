# Frontend Components — UOW-5 Rule-Sourced Recategorization

## Technology Boundary

Existing server-rendered templates, Bootstrap 5, HTMX where conventional, vanilla JavaScript. No new
template, page, route, framework or library.

## What Changes on Screen

Only what a rule change reports afterwards. Every screen that triggers a rule change already shows a
result; those results gain the FR16 counts.

| Screen | Trigger | What the user now reads |
|---|---|---|
| Categories page | Create, edit or delete a pattern; delete a category | How many transactions moved, how many became uncategorized, and how many manual ones were protected |
| SIC mappings page | Create, edit or delete a mapping; upload a file | The same three counts, alongside the existing created/updated/unchanged row counts |
| Transaction modals | Create a mapping from a modal | The same three counts |

The manual count is the one worth surfacing prominently. After a bulk rewrite the question a user asks
is whether their hand-set categories survived, and this answers it with a number.

## What the User Must Not Be Surprised By

Two behaviours are new and visible, and the wording should not bury either:

- A rule change can now **remove** a category, leaving the transaction uncategorized. It appears on the
  uncategorized dashboard, which is the screen built for deciding what to do with it.
- Creating a **text pattern** can move transactions away from categories a SIC mapping assigned, because
  patterns outrank mappings.

Neither is a defect and neither should be softened into vagueness. A count that says "3 became
uncategorized" is more use than one that does not distinguish them from moves.

## Naming Constraint

No JavaScript is added by this unit. The constraint still applies to any that is: `layout.html` loads
`/static/js/app.js` **after** page content, so a page-level function sharing a name with an `app.js`
global is silently overwritten at runtime while the markup still looks correct. That was defect U2-F09,
where mapping deletion could not work at all. The reviewer-owned collision regression from UOW-2 covers
this automatically.

## Escaping

The counts are integers and carry no user-controlled text. Where a result renders category names, it
continues to go through DOM `textContent`, and any name echoed into a diagnostic still passes through
`model.NewDiagValue` per NFR-U4-SEC-01.

## Explicitly Unchanged

- No new page, route, modal or control.
- No change to the uncategorized dashboard, which already lists what it needs to.
- No change to the import result, since import does not trigger re-examination.
