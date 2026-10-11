# Requirements — Issue #5 SIC Auto-Categorization

## Intent Analysis

| Field | Value |
|---|---|
| **User Request** | Plan and implement GitHub issue #5: auto-categorize transactions using SIC values found in OFX/QFX `<SIC>` tags. |
| **Request Type** | New Feature / Enhancement |
| **Scope** | Multiple Components: parser, model, database schema/migration, repositories, categorizer service, handlers/API, UI, tests |
| **Complexity** | Moderate |
| **Project Type** | Brownfield Go application |

## Terminology Decision

The application will treat the OFX/QFX `<SIC>` tag strictly as **SIC** for this feature, while leaving room for a future separate MCC field. UI and API naming should prefer **SIC** rather than “SIC/MCC”.

## Functional Requirements

### FR1 — Parse SIC from OFX/QFX transactions
The OFX parser must extract the `<SIC>` value from each transaction when present.

- Missing SIC remains unset/null.
- SIC values must be preserved as strings in the application model.
- Existing OFX/QFX import behavior must remain unchanged for transactions without SIC.

### FR2 — Store SIC on transactions
Transactions must store the parsed SIC value in SQLite.

- Add nullable transaction field, e.g. `sic_code`.
- Existing duplicate detection remains unchanged: `account_id`, `trn_type`, `fit_id`, `date_posted`.
- SIC must not participate in deduplication.

### FR3 — Preserve existing local databases with startup migration
Existing `privateledger.db` files must be upgraded in place.

- Add lightweight startup migration for the new transaction SIC column.
- Add required SIC mapping table(s).
- Existing local financial data must be preserved automatically.

### FR4 — Initialize SIC mappings from a local mapping file when available
On first startup with the SIC feature, the application must check for an existing SIC-to-category mapping file named `sic_mappings.csv` beside `config.json` and `privateledger.db`.

- If `sic_mappings.csv` exists **and the SIC mapping table is empty**, import its mappings into SQLite.
- If `sic_mappings.csv` exists but SIC mappings already exist in SQLite, skip the import, log the skip, and leave existing mappings untouched.
- If `sic_mappings.csv` does not exist, startup must continue successfully without SIC mappings.
- The import must be idempotent and safe to run repeatedly: repeated startups must never duplicate, revert, or overwrite existing mappings.
- User edits after import live in SQLite and are authoritative. The file on disk goes stale as soon as the user edits mappings in the UI, so it must never be re-applied over those edits on a later startup.
- The mapping file acts as an import/export interchange format, not the only runtime source of truth.
- The mapping file format is CSV with columns: `SIC_Code`, `Description`, `Description_Detail`, `Category_Name`, and `Category_ID`. There is no internal/import ID column — normalized SIC code is the interchange identity and internal primary keys serve no purpose in the file.
- Category references resolve by `Category_Name` first, with `Category_ID` as a tiebreaker/hint. Rows where the two disagree must be rejected. Resolving by ID alone is unsafe: a category that was deleted and recreated leaves a stale ID that still resolves to a valid but wrong category, so an upload would silently mis-map every affected row.
- `Description` and `Description_Detail` are stored with SIC mappings for display/help text.

**Amended 2026-09-07 (UOW-4).** Header matching and diagnostic quality:

- The header is matched after normalization: a leading UTF-8 byte-order mark is stripped, surrounding
  whitespace is trimmed per column, and column names are compared case-insensitively. Column order and
  column count remain required, and export continues to emit the canonical spelling. A file saved by a
  spreadsheet, which commonly writes a byte-order mark, must import.
- `Category_ID` still never resolves a row on its own; the amendment below changes only what the user is
  told, not what is accepted.
- When `Category_Name` does not resolve, the row is still rejected, and the diagnostic must name the
  unresolved value and, when `Category_ID` refers to an existing category, that category's current name.
  This makes a file stale after a category rename repairable in one edit. It does not make it import
  unaided; that residual friction is accepted deliberately rather than resolved by trusting the ID.
- When `Category_Name` matches more than one category case-insensitively, the diagnostic must name the
  colliding categories. Category names remain unique case-sensitively; no migration is introduced, and
  the ambiguity is reported rather than resolved by guessing.

### FR5 — SIC mappings may map to categories or remain intentionally unmapped
A SIC mapping may point directly to a category, or it may intentionally have an empty category.

- Although the user described categorization through the existing pattern/rule system, the implementation maps SIC directly to the category associated with the intended rule/category when a category is present.
- If a SIC mapping has an empty category, matching transactions must remain uncategorized by SIC; the app must not assign any category from that mapping.
- UI wording may describe SIC mappings as categorization rules, but the persistence model should avoid unnecessary indirection through `category_pattern` rows.

### FR6 — Categorization priority
Auto-categorization priority must be:

1. Existing text patterns first.
2. SIC mappings second, only if no text pattern matched.

If a SIC mapping matches and has a non-empty category, the transaction is categorized to the mapped category with `category_source = rule`. If the matching SIC mapping has an empty category, no category is assigned by SIC and the transaction remains uncategorized unless already categorized by a text pattern.

### FR7 — Preserve manual categorization
Manual categorizations must never be overwritten.

- Transactions with `category_source = 2` are excluded from automatic categorization changes.
- ~~Creating or updating SIC mappings only re-categorizes currently uncategorized transactions.~~
- ~~Existing rule-based categorizations are not changed by SIC mapping creation.~~
- Rule-sourced categorizations follow the rules. When any rule changes, is created or is deleted, every
  rule-sourced transaction is re-examined against the current rules and takes the category those rules
  now give it, or none if no rule matches.

**Amended 2026-09-07 by UOW-5** (answers R1 A, R1a A, R2 A, R3 A, R4 A, R5 A, R6 A). The two struck
bullets are superseded, and shown struck rather than deleted because they were the approved behaviour
through UOW-1 to UOW-4 and explain why the shipped code looks as it does.

**The first bullet is unchanged and is not weakened by this amendment.** Manual stays manual. That was
never in question at any point in this unit.

What changed is the second and third. They made a rule-sourced categorization permanent, so a
transaction could sit in a category assigned by a rule that no longer said so, with nothing reporting
the divergence. In the user's framing: keeping it there makes it "almost like a manual categorization",
which corrupts the one signal the system treats as authoritative.

Consequences, all deliberate:

- Re-examination is triggered by **creating** a rule as well as changing or deleting one (R1a A), and by
  **text-pattern** changes as well as SIC mapping changes (R3 A).
- Because text patterns outrank SIC mappings, a **new pattern can move transactions away from a
  category a mapping assigned**. This is the sharpest edge of the amendment and is intended, not
  incidental.
- A re-examined transaction matching no rule **becomes uncategorized** (R2 A) rather than keeping an
  unsupported category. It surfaces on the uncategorized dashboard.
- Re-examination is **automatic** (R4 A), consistent with how rule changes already recategorize
  uncategorized transactions.
- The first rule change after this ships re-examines every pre-existing rule-sourced transaction at once
  (R6 A). Its size is visible in the FR16 counts.

### FR8 — Enforce globally unique SIC mappings
Each SIC code may have only one mapping globally.

- Duplicate exact SIC mappings must be rejected.
- A mapping may have an empty category to mean "do not categorize transactions with this SIC by SIC mapping".
- Account-specific SIC mappings are out of scope.
- Multiple categories for the same SIC are out of scope.

### FR9 — Manage SIC mappings in a separate configuration page
Users must manage SIC-to-category mappings in a separate UI page, not inside the existing Categories page.

The page should support at minimum:
- Viewing existing SIC mappings.
- Creating a new SIC mapping.
- Updating or deleting an existing SIC mapping.
- Selecting the target user-defined category, or leaving the category empty to intentionally avoid SIC-based categorization for that code.
- Showing all current categories from the Categories page as mapping targets, including categories created or updated after application startup.
- Downloading the current SIC-to-category mappings as `sic_mappings.csv`.
- Uploading a CSV SIC-to-category mapping file that atomically adds new mappings and updates matching mappings in SQLite without deleting codes omitted from the file.

### FR10 — Show SIC in transaction categorization modals
The Transactions table should not add a prominent new SIC column.

- SIC should be visible in the existing Change Category modal and Create Categorization Pattern modal when the transaction has a SIC code.
- The modal must show the SIC code and a display description.
- The display description is the mapping `Description` value when present; otherwise use `Description_Detail` when present.
- If no description values exist, the modal should still show the SIC code cleanly.

### FR11 — SIC code validation
User-entered SIC codes must be numeric (digits only).

**Rationale**: `ofxgo` parses the OFX/QFX `<SIC>` tag as `int64`, so a SIC value stored on a
transaction is always numeric. A non-numeric `<SIC>` fails the parse of the entire file, which is
pre-existing behavior independent of this feature. An alphanumeric mapping code could therefore never
match any imported transaction — it would validate, store, and occupy a unique-code slot while being
permanently dead. Restricting to digits also removes the need to decide letter-case equivalence and
the matching database collation.

Minimum validation:
- Trim whitespace.
- Reject empty values.
- Reject any value containing a non-digit character.
- Enforce reasonable max length to prevent accidental large input.
- Preserve the canonical normalized value consistently for exact matching.

### FR12 — Import result count remains unchanged
Import results continue to expose the single existing `total_auto_categorized` count.

- Text-pattern and SIC-based matches both count as auto-categorized.
- No separate user-facing counts are required.

### FR13 — API support for SIC mappings
The application needs API endpoints for the separate SIC configuration UI.

Recommended minimum endpoints:
- `GET /api/sic-mappings`
- `POST /api/sic-mappings`
- `PUT /api/sic-mappings/:id`
- `DELETE /api/sic-mappings/:id`
- `GET /api/sic-mappings/download`
- `POST /api/sic-mappings/upload`

API responses should include joined category display fields where useful. Upload must validate the entire mapping file before atomically merging it into existing mappings.

Upload merge rules:
- New normalized SIC codes are inserted.
- Existing normalized SIC codes are overwritten with the uploaded row values.
- Existing database codes omitted from the CSV remain unchanged; deletion requires an explicit delete action.
- A header-only CSV is a valid no-op.
- The service attempts a timestamped pre-upload backup. Backup failure does not block the merge, but the response must prominently report the warning and omit the backup path.

### FR14 — Re-categorization after SIC mapping changes
When a SIC mapping is created or updated from the dedicated SIC mapping page or file upload:

- Reload categorization rules/mappings.
- Re-categorize only currently uncategorized transactions that match the SIC code.
- Preserve manual and existing rule-based categorizations.

When a SIC mapping is created or updated from the Change Category modal:

- Prompt the user whether to create/update the SIC-to-category mapping when the transaction has a SIC code and a category is selected.
- If the user agrees, create/update the SIC mapping, then apply that SIC rule to the current transaction with rule source instead of manual source.

When a SIC mapping is created from the Create Categorization Pattern modal:

- Prompt the user to create/update the SIC mapping instead of the text pattern when the transaction has a SIC code and a category is selected.
- If the user agrees, create/update only the SIC mapping and do not create a text pattern.
- Modal-created SIC mappings require a selected category; empty-category mappings are managed only through the dedicated SIC mapping page or mapping file upload.

When a SIC mapping is deleted:

- Reload categorization rules/mappings.
- ~~Do not automatically clear categories already assigned by that mapping unless explicitly requested in a future feature.~~
- Re-examine rule-sourced transactions that carried that mapping's code; each takes whatever the current
  rules give it, or becomes uncategorized if nothing matches.

**Amended 2026-09-07 by UOW-5.** Three statements above are superseded by FR7 as amended and by FR15.
"Re-categorize only currently uncategorized transactions" and "preserve existing rule-based
categorizations" now read as: re-examine every rule-sourced transaction against the current rules.
Manual preservation is unchanged. The deletion bullet is struck because leaving a category assigned by a
mapping that no longer exists is precisely the history-dependence FR15 forbids — the "future feature" it
anticipated is UOW-5.

**Amended 2026-09-07 by UOW-5, after independent finding U5-R-F05.** Deleting a category clears it from
every transaction it held, **including manually assigned ones**, which then become uncategorized and are
re-examined like any other uncategorized transaction.

This does not weaken the first bullet, and the distinction is the whole point. FR7 protects a manual
assignment from being revised by a **rule**. Deleting the category is not a rule acting; it is the user
removing the very thing they chose. Once the category no longer exists the choice cannot be honoured in
any form, and the transaction is simply uncategorized.

The alternative was considered and declined: keeping `category_source = manual` on a transaction with no
category preserves a marker for a choice that can no longer be applied, and it creates a **second
representation of "uncategorized"** — one query counts such a row, another does not. A single meaning of
uncategorized is worth more than a marker for an unhonourable choice.

Manual assignments in categories that still exist are untouched by this, under every trigger.

### FR15 — Categorization is determined by the current rules, not by rule history

**Added 2026-09-07 by UOW-5**, from the user's stated principle:

> I want the same rule book to create the same transaction categorization, irrespective of when the
> rules were created.

Categorization is a function of the current rule set and the user's manual choices. It is **not** a
function of the order in which rules were created, changed or deleted, nor of the order in which
transactions were imported.

Two databases holding identical transactions, identical patterns, identical SIC mappings and identical
manual assignments must categorize identically.

This is why FR7's rule-permanence bullets could not stand: they made the outcome depend on whether a
rule existed before or after a transaction was categorized. It is also why rule *creation* triggers
re-examination — a rule set that only applies to transactions it happens to encounter first is not a
rule set that determines an outcome.

FR15 governs all future categorization work, not only UOW-5. Any proposal whose result depends on when
a rule was created contradicts it and needs an explicit amendment rather than an exception.

### FR16 — Report what a rule change did

**Added 2026-09-07 by UOW-5** (answer R5 A).

The result of a rule change reports three counts: transactions moved to a different category,
transactions that became uncategorized, and transactions left unchanged because they are manual.

The third exists because it answers the question a user actually has after a bulk rewrite — whether the
categories they set by hand were touched — and answering it needs a number, not a list. Reporting only
what changed leaves that question open exactly when it matters most.

## Non-Functional Requirements

### NFR1 — Privacy-first local operation
All SIC data, mappings, and categorization logic remain local-only. No external lookups or cloud services are introduced.

### NFR2 — Backward compatibility
Existing imports and existing local databases continue to work after upgrade.

### NFR3 — Maintain clean architecture
Implementation must preserve the existing layering:

`handler → service → repository → SQLite`

No global state or singleton should be introduced.

### NFR4 — Idempotent mapping file import and migrations
Startup migration and optional SIC mapping file import must be safe to run repeatedly. Missing mapping files must not prevent application startup.

### NFR5 — Testability
The feature must include example-based tests for parser extraction, repository behavior, categorization priority, migration, mapping file import, mapping file download, and atomic merge/upsert behavior where practical.

### NFR6 — Property-based testing partial enforcement
Property-based testing is partially enabled. Enforced PBT rules are:

- PBT-02 Round-trip Properties
- PBT-03 Invariant Properties
- PBT-07 Generator Quality
- PBT-08 Shrinking and Reproducibility
- PBT-09 Framework Selection

Potential PBT candidates include SIC normalization/validation invariants and idempotent mapping file import behavior, subject to design feasibility.

## Out of Scope

- Separate MCC field distinct from SIC.
- Account-specific SIC mappings.
- Multiple categories per SIC code.
- Changing transaction deduplication.
- Separate import history counts for text-pattern vs SIC auto-categorization.
- Cloud-based SIC/MCC lookup.
- Non-numeric SIC values from OFX/QFX. Supporting them would require extracting `<SIC>` before `ofxgo` parses the file, since `ofxgo` rejects a non-numeric `<SIC>` for the whole file.
- Server-side upload preview state. Upload validates and merges in one request; the browser confirms beforehand.
- Automatically clearing categories when SIC mappings are deleted.

## Acceptance Criteria

1. OFX/QFX transactions with `<SIC>` import successfully and store SIC on the transaction.
2. Transactions without `<SIC>` continue to import successfully.
3. Existing text patterns categorize before SIC mappings.
4. SIC mapping categorizes uncategorized transactions when no text pattern matches.
5. Manual categorizations are never overwritten.
6. A duplicate SIC mapping cannot be created.
7. Existing local databases are upgraded in place without data loss.
8. Existing SIC mapping files are loaded into SQLite when present and the mapping table is empty, skipped when mappings already exist, and ignored safely when absent — so repeated startups never duplicate or revert mappings.
9. Users can view and manage SIC mappings from a separate configuration page.
10. Users can download the current CSV mapping file as `sic_mappings.csv`.
11. Users can upload a CSV mapping file that atomically adds new mappings and updates matching mappings after validation, without deleting omitted mappings.
12. All existing and newly created/updated categories are available as mapping targets.
13. Change Category and Create Categorization Pattern modals show SIC code and description when available.
14. Modal-created SIC mappings use a selected non-empty category and follow the requested modal behavior.
15. A SIC code containing a non-digit character is rejected by validation, on the mapping page and during CSV import alike.
16. A CSV row whose `Category_Name` and `Category_ID` resolve to different categories is rejected rather than silently resolved.
17. `go test ./...` passes.
