# Requirement Clarification Questions — Issue #5 SIC Auto-Categorization

I detected two requirements that need clarification before the AI-DLC Requirements Analysis can be completed.

Please answer each question by filling in the letter choice after the `[Answer]:` tag. If none of the options match, choose `X` and describe your preference after the tag.

## Clarification 1: SIC code mapping target
Your answer to Question 2 says: “Existing text patterns first. Then SIC/MCC code should be mapped to the existing text patterns only ... Finally, the transaction should be categorized to the text pattern.”

The current application model maps text patterns directly to categories (`category_pattern.pattern_name -> category_id`). To implement your answer precisely, the system needs to know whether SIC codes map to an existing text pattern row, a pattern-like named rule, or directly to the category associated with a selected pattern.

### Question 1
What should a SIC code mapping point to?

A) SIC code maps to an existing text pattern row; the transaction receives that pattern's category

B) SIC code maps to a category directly, but the UI labels this as using the category's pattern/rule system

C) SIC code maps to a pattern name string; if the named pattern exists, use its category, otherwise keep the SIC code unmapped

X) Other (please describe after [Answer]: tag below)

[Answer]: B

## Clarification 2: Prepopulated mapping source and mutability
Your answer to Question 2 says SIC/MCC mappings “should be prepopulated in a config file” and editable in the UI when new codes are found.

### Question 2
How should prepopulated SIC mappings and user edits be stored?

A) Ship a default JSON config file embedded in the binary; copy/import defaults into SQLite on startup, then all user edits live in SQLite

B) Keep mappings only in an external JSON config file beside `config.json`; the UI edits that file directly

C) Ship defaults in code, store user overrides in SQLite, and merge both at runtime

X) Other (please describe after [Answer]: tag below)

[Answer]: A

## Clarification 3: Existing local database compatibility
Your answer to Question 10 asks why migration is needed because this is a new feature. The current app creates `privateledger.db` locally and existing users may already have data. Adding a new transaction field/table requires deciding whether existing DB files should keep working automatically.

### Question 3
Should existing `privateledger.db` files be upgraded in place when this feature is added?

A) Yes, add lightweight startup migration so existing local data is preserved automatically

B) No, it is acceptable to require users to delete/recreate their local database for this feature

C) Defer existing DB compatibility; implement schema for fresh installs only and document the limitation

X) Other (please describe after [Answer]: tag below)

[Answer]: A
