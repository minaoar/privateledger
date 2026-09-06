# Frontend Components — UOW-3 Transaction Categorization Integration

## Technology Boundary

Existing server-rendered templates, Bootstrap 5, HTMX where already conventional, and vanilla
JavaScript. No frontend framework or production library is added. UOW-3 modifies two existing
templates and adds none.

## Naming Constraint — Non-Negotiable

`layout.html` renders page content before it loads `/static/js/app.js`, so any page-level function
sharing a name with an `app.js` global is **silently overwritten at runtime**. The markup still looks
correct; only the resolved target is wrong.

This is not hypothetical. It is defect U2-F09: the mapping page named its delete handler
`confirmDelete`, app.js declares a global `confirmDelete` that only calls the browser's native
`confirm()`, and mapping deletion could not work at all — two prompts, no request, and nothing in any
log, because the click called a real working function that was simply the wrong one.

Every function this unit adds to `transactions.html` or `categories.html` therefore takes a
page-specific name. `app.js` currently declares `formatCurrency`, `formatDate`, `showToast`, and
`confirmDelete`. This must be re-checked when adding names, not assumed from that list (BR-U3-35).

## Change Category Modal — `transactions.html`

Adds a SIC context block, shown only when the transaction has a SIC code.

- The code, in monospace, with the mapping description beside it — `Description`, falling back to
  `Description_Detail`, and the code alone when neither exists.
- When a mapping already exists, the category it currently assigns, so the user can see what the code
  does before changing anything.
- When no mapping exists, an opt-in control offering to create one for this code using the category
  being selected. Unchecked by default: changing one transaction's category must not silently create a
  rule affecting others.
- When the control is used, the confirmation states plainly that other uncategorized transactions with
  this SIC code will also be categorized (BR-U3-32), so the wider effect is visible before it happens.

The modal continues to work unchanged for transactions with no SIC code; the block is simply absent.

## Create Categorization Pattern Modal — `transactions.html`

Adds a choice, presented only when the transaction has a SIC code: create a text pattern as today, or
create a SIC mapping for this code instead.

- Text pattern remains the default, preserving current behaviour for anyone not using SIC.
- Choosing SIC requires a selected category (BR-U3-29) and creates a mapping **only** — never a text
  pattern as well (BR-U3-30).
- The two choices are mutually exclusive and visibly so, with the inapplicable fields disabled rather
  than hidden, so the form does not appear to have lost inputs.

## Categories Page — `categories.html`

"Recategorize All" gains a warning and a fuller result.

- **Before running**, the confirmation states that SIC mappings will now also be applied, not only text
  patterns. This is the first action that can apply SIC across an entire existing backlog, and it is
  worth saying so once rather than having it discovered afterwards (BR-U3-22).
- **After running**, the result reports transactions categorized by text pattern and by SIC mapping as
  separate counts alongside the processed total (BR-U3-23). A single combined number would hide
  precisely the information that shows whether the mappings are behaving as intended.
- A run that categorizes nothing says so plainly rather than showing a bare zero.

## Shared Behaviour

- All user-controlled text — descriptions, category names, diagnostics — is rendered through template
  escaping or DOM `textContent`, never assembled as raw HTML.
- Submissions are disabled while a request is in flight and always restored afterwards, including on
  failure.
- Transport failures report an unknown outcome rather than a definite failure, matching the contract
  established on the mapping page: a lost response says nothing about whether the server committed.
- Existing modal focus behaviour, label association, and accessible alert semantics are preserved.

## Stable Automation Identifiers

New interactive elements use stable `data-testid` values: `txn-sic-context`,
`txn-sic-create-mapping-toggle`, `txn-pattern-type-choice`, `categories-recategorize-button`, and
`categories-recategorize-result`. Existing identifiers on both pages are unchanged.

## Explicitly Not Changed

- No SIC column is added to the transactions table (BR-U3-28).
- No new page, route, or template is introduced.
- The SIC mapping page from UOW-2 is untouched by this unit.
