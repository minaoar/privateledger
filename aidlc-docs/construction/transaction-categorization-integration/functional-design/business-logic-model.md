# Business Logic Model — UOW-3 Transaction Categorization Integration

## Scope and Boundary

UOW-3 makes stored SIC codes actually categorize transactions. It consumes UOW-1's persistence
contracts and UOW-2's mapping-management contracts, and replaces the no-op collaborator UOW-2 wired.

It introduces no new persistence, no schema change, and no new dependency. Mapping administration
stays in UOW-2; this unit only reads mappings and writes transaction categories.

Stage answers governing this design: Q1 B, Q2 B, Q3 A, Q4 A, Q5 A, Q6 B.

## Categorization Decision

One decision function serves import, "Recategorize All", and scoped recategorization, so the priority
rules cannot drift between entry points.

1. If `category_source = manual`, stop. Nothing automatic ever changes it.
2. **[Superseded 2026-09-07 by UOW-5 — FR7 as amended, FR15.]** If the transaction already has a
   category, stop. Automatic categorization fills gaps; it does not
   revise existing assignments.
3. Try text patterns in their existing order. First match wins, assigning `category_source = rule`.
4. Only if no text pattern matched, and the transaction carries a SIC code, look up the mapping.
5. If a mapping exists **and** carries a non-empty category, assign it with `category_source = rule`.
6. If the mapping is missing, or exists with an empty category, assign nothing. The transaction stays
   uncategorized.

Steps 5 and 6 are the whole of FR6's SIC behaviour. An empty-category mapping is a deliberate
"never categorize this code" statement, not a gap to fill later.

## Correcting the Existing Recategorization Paths

`RecategorizeAll` and `RecategorizeByCategory` currently re-implement pattern matching inline rather
than calling the decision function. Left alone, SIC would apply during import but be silently skipped
by "Recategorize All" — the same rules producing different answers depending on which button was
pressed. Both are changed to route through the single decision function.

## Import

Import categorizes each transaction as it is inserted, using the same decision function. Mapping data
comes from the in-memory cache (Q4 A), so import adds no per-row query and the approved SIC-free
import benchmark is not put at risk.

## Recategorize All

Triggered from the Categories page. It loads rules, reads every uncategorized transaction, applies the
decision function, and bulk-updates by category.

Because SIC now participates, the first run after upgrade can categorize a large backlog through
mappings the user has never exercised. Per Q2 B the action therefore warns before running and reports
how many transactions were categorized by text pattern and how many by SIC mapping, counted
separately. The split is what tells the user whether their mappings are behaving as intended; a single
combined total would hide it.

## Scoped Recategorization — the UOW-2 Collaborator

UOW-3 supplies the real implementation of the contract UOW-2 defined.

- `ReloadMappings` refreshes the mapping cache from SQLite.
- **[Scope widened 2026-09-07 by UOW-5 — see FR15; the affected-set definition is UOW-5 Functional
  Design's to settle.]** `RecategorizeBySICCodes` takes the de-duplicated affected codes, reads only
  currently uncategorized
  transactions carrying those codes, applies the decision function, and returns how many changed.

Scope is deliberately narrow: a mapping change may only fill gaps. It never revisits a manual
assignment, never revisits an existing rule assignment, and never touches a transaction whose SIC code
was not among the affected set.

Per Q1 B this work carries no processing deadline. It runs to completion, and a failure remains a
post-commit warning under the already-approved BR-U2-45 — the mapping change is committed first and is
never rolled back because categorization failed afterwards. The choice is deliberate rather than an
omission: recategorization is one indexed query over `idx_txn_sic` plus bulk updates, and UOW-2's
measured merge of 100,000 mappings completed in 1.4 s, so a deadline would introduce
partial-completion state to report and test without addressing a delay that occurs at this scale.

## Rule Caches

Both the text-pattern cache and the new mapping cache are guarded, and every reload is synchronous
(Q3 A). The existing fire-and-forget `go LoadPatterns()` is removed.

This fixes two defects at once. The goroutine is an unsynchronized write to a slice that requests read
concurrently — a genuine data race. It is also an ordering bug: a user can add a pattern and
immediately run "Recategorize All" against a cache that has not been refreshed yet. Making reload part
of the request removes both. A reload is one small query, so the latency the goroutine saved is not
worth either defect.

A single exported reload entry point replaces the currently exported `LoadPatterns`, so callers cannot
refresh one cache and forget the other.

## Transaction Modals

**Display (US-06).** Both the Change Category and Create Categorization Pattern modals show the
transaction's SIC code when it has one, with the mapping `Description`, falling back to
`Description_Detail`, and showing the bare code cleanly when neither exists. UOW-1 already joins
`SICDescription` into the transaction read paths, so no new query is required. The transactions table
itself gains no SIC column.

**Creating a mapping (US-12).** Both modals can create or update a SIC mapping for the transaction's
code, and both require a selected category — a modal is not a way to create an empty mapping. Change
Category applies the rule source when the user agrees. Create Pattern creates a SIC mapping *instead
of* a text pattern, never both.

Per Q5 A, a mapping created this way is an ordinary mapping change: it applies to the current
transaction and then recategorizes other currently uncategorized transactions sharing that code,
exactly as the mapping page does. Creating a mapping means the same thing wherever it is created.

## Failure Ordering

| Failure | Category writes | Result |
|---|---|---|
| Rule reload before categorization | None | Operation fails; nothing categorized |
| Mapping lookup finds nothing, or an empty category | None for that transaction | Not an error; the transaction stays uncategorized |
| Bulk category update | Rolled back for that batch | Operation fails and reports the error |
| Collaborator failure after a committed mapping change | Mapping change stands | Committed-with-warning per BR-U2-45; never presented as a rollback |

## Traceability

| Story | Logic |
|---|---|
| US-02 | Decision function priority; shared by import, Recategorize All, and scoped recategorization |
| US-03 | Manual and existing assignments excluded; scoped recategorization limited to affected codes |
| US-06 | SIC code and description fallback in both modals; no new table column |
| US-12 | Modal-created mappings with required category, rule source, and mapping-change recategorization |
| US-13 | Layer direction, guarded caches, single reload entry point, properties and full test run |

## UOW-5 Amendment Notice — 2026-09-07

Two steps above are marked superseded. Step 2's "already has a category, stop" guard no longer applies
to rule-sourced transactions, and `RecategorizeBySICCodes`'s uncategorized-only scope widens to include
them. The manual guard in step 1 is unchanged and still absolute.

This document is left as approved with inline markers rather than rewritten. It describes what UOW-3
built and what is in shipped code today; UOW-5 has not yet changed either. Rewriting it now would
describe code that does not exist.
