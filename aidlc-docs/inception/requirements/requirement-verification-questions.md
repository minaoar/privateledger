# Requirement Verification Questions — Issue #5 SIC/MCC Auto-Categorization

Please answer each question by filling in the letter choice after the `[Answer]:` tag. If none of the options match, choose `X` and describe your preference after the tag.

## Question 1
How should the application treat the OFX/QFX `<SIC>` tag semantically?

A) Treat `<SIC>` as the payment-scheme MCC/SIC code source and label it as “SIC/MCC” in the UI

B) Treat `<SIC>` strictly as SIC only, while leaving room for a future separate MCC field

C) Store and display it generically as “Merchant Code” to avoid committing to SIC vs MCC terminology

X) Other (please describe after [Answer]: tag below)

[Answer]: B

## Question 2
What should be the primary matching priority during auto-categorization?

A) SIC/MCC code mappings first, then existing text patterns if no code mapping matches

B) Existing text patterns first, then SIC/MCC code mappings if no text pattern matches

C) Use whichever rule was created first across both text patterns and code mappings

X) Other (please describe after [Answer]: tag below)

[Answer]: X. Existing text patterns first. Then SIC/MCC code should be mapped to the existing text patterns only (this mapping should be prepopulated in a config file). If there is new SIC/MCC code which are not mapped to existing text pattern, there should be UI to update it. Finally, the transaction should be categorized to the text pattern.

## Question 3
When a new SIC/MCC mapping is created, which existing transactions should be re-categorized?

A) Only currently uncategorized transactions, preserving all manual and existing rule-based categorizations

B) Uncategorized transactions plus existing rule-based categorizations, while preserving manual categorizations

C) All matching transactions, including manual categorizations

X) Other (please describe after [Answer]: tag below)

[Answer]: A

## Question 4
How should duplicate or overlapping SIC/MCC mappings be handled?

A) A code may map to only one category globally; exact duplicate code mappings are rejected

B) A code may map to different categories by account; duplicate code mappings are rejected only within the same account

C) A code may map to multiple categories and the categorizer chooses the first match

X) Other (please describe after [Answer]: tag below)

[Answer]: A

## Question 5
Where should users manage SIC/MCC-to-category mappings?

A) Add SIC/MCC mappings inside the existing Categories page, alongside text patterns per category

B) Add a separate SIC/MCC configuration page with a table of code-to-category mappings

C) Support both: category-level management and a global table view

X) Other (please describe after [Answer]: tag below)

[Answer]: B

## Question 6
Should the Transactions page show the parsed SIC/MCC code for each transaction?

A) Yes, show it as a small muted badge or metadata field when present

B) No, store and use it only for auto-categorization

C) Show it only in a transaction details or edit modal, not in the main table

X) Other (please describe after [Answer]: tag below)

[Answer]: C

## Question 7
How strict should SIC/MCC code validation be for user-entered mappings?

A) Digits only, 1–6 characters, normalized by trimming whitespace

B) Digits only, exactly 4 characters, matching common MCC length

C) Allow alphanumeric codes because some institutions may emit non-standard values

X) Other (please describe after [Answer]: tag below)

[Answer]: C

## Question 8
Should import duplicate detection remain unchanged?

A) Yes, keep the existing composite key: account_id, trn_type, fit_id, date_posted

B) Include SIC/MCC code in duplicate detection when present

C) Revisit deduplication separately before implementing this feature

X) Other (please describe after [Answer]: tag below)

[Answer]: A

## Question 9
Should import results distinguish text-pattern auto-categorized vs SIC/MCC auto-categorized counts?

A) No, keep the existing single `total_auto_categorized` count for both rule types

B) Yes, add separate counts for text-pattern matches and SIC/MCC matches in API response and import history

C) Add separate counts only to logs, not user-facing API/UI

X) Other (please describe after [Answer]: tag below)

[Answer]: A

## Question 10
How should existing databases be migrated?

A) Add lightweight startup migrations for the new transaction column and mapping table, preserving all local data

B) Update schema only; users can recreate the local database if needed

C) Provide an external one-time migration command instead of automatic startup migration

X) Other (please describe after [Answer]: tag below)

[Answer]: X. Why there should be a migration. It's a new feature.

## Question 11
Should security extension rules be enforced for this project?

A) Yes — enforce all SECURITY rules as blocking constraints (recommended for production-grade applications)

B) No — skip all SECURITY rules (suitable for PoCs, prototypes, and experimental projects)

X) Other (please describe after [Answer]: tag below)

[Answer]: B

## Question 12
Should property-based testing (PBT) rules be enforced for this project?

A) Yes — enforce all PBT rules as blocking constraints (recommended for projects with business logic, data transformations, serialization, or stateful components)

B) Partial — enforce PBT rules only for pure functions and serialization round-trips (suitable for projects with limited algorithmic complexity)

C) No — skip all PBT rules (suitable for simple CRUD applications, UI-only projects, or thin integration layers with no significant business logic)

X) Other (please describe after [Answer]: tag below)

[Answer]: B

## Question 13
Should the resiliency baseline be applied to this project?

A) Yes — apply the resiliency baseline as directional best practices and design-time guidance

B) No — skip the resiliency baseline

X) Other (please describe after [Answer]: tag below)

[Answer]: B
