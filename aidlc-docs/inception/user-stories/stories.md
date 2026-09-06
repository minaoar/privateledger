# User Stories — Issue #5 SIC Auto-Categorization

Stories are ordered using the approved hybrid Journey + Feature-Based approach. Each story is medium-sized and includes Given/When/Then acceptance criteria.

## US-01 — Import transactions with SIC data

**As a** Local Personal Finance User,
**I want** the app to read SIC values from OFX/QFX transaction files,
**so that** the imported transaction has enough structured data for improved auto-categorization.

**Personas**: Local Personal Finance User

**Acceptance Criteria**:
- **Given** an OFX/QFX transaction contains a `<SIC>` value, **when** I import the file, **then** the transaction stores that SIC value.
- **Given** an OFX/QFX transaction does not contain `<SIC>`, **when** I import the file, **then** the transaction imports successfully with no SIC value.
- **Given** a transaction contains SIC, **when** duplicate detection runs, **then** the existing deduplication key remains unchanged and does not include SIC.

**Requirement Mapping**: FR1, FR2, FR12

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-02 — Use existing text patterns before SIC mappings

**As a** Local Personal Finance User,
**I want** my existing text-pattern categorization rules to take priority over SIC mappings,
**so that** new SIC behavior does not override rules I already trust.

**Personas**: Local Personal Finance User

**Acceptance Criteria**:
- **Given** a transaction matches an existing text pattern and also has a mapped SIC code, **when** auto-categorization runs, **then** the text pattern category is applied.
- **Given** a transaction does not match a text pattern but has a mapped SIC code with a non-empty category, **when** auto-categorization runs, **then** the SIC mapping category is applied.
- **Given** a transaction does not match a text pattern but has a mapped SIC code with an empty category, **when** auto-categorization runs, **then** no SIC category is applied and the transaction remains uncategorized.
- **Given** a transaction has no matching text pattern and no mapped SIC code, **when** auto-categorization runs, **then** it remains uncategorized.

**Requirement Mapping**: FR5, FR6, FR12

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-03 — Preserve manual categorization

**As a** Local Personal Finance User,
**I want** manually assigned categories to remain unchanged,
**so that** the app never overwrites deliberate user decisions.

**Personas**: Local Personal Finance User

**Acceptance Criteria**:
- **Given** a transaction has a manual category, **when** SIC mappings are created or updated, **then** the manual category is not changed.
- **Given** a transaction has an existing rule-based category, **when** a new SIC mapping is created, **then** that existing rule-based category is not changed.
- **Given** an uncategorized transaction has a matching SIC code, **when** a relevant SIC mapping is created or updated, **then** the transaction may be categorized by the SIC mapping.

**Requirement Mapping**: FR7, FR14

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-04 — Manage SIC mappings in a separate configuration page

**As a** Local Personal Finance User,
**I want** a dedicated SIC mapping configuration page,
**so that** I can view and manage SIC-to-category rules without mixing them into text patterns.

**Personas**: Local Personal Finance User

**Acceptance Criteria**:
- **Given** I open the SIC configuration page, **when** mappings exist, **then** I can see each SIC code and its mapped category or see that its category is empty.
- **Given** I create a new SIC mapping with a valid code and category, **when** I save it, **then** the mapping is available for future categorization.
- **Given** I create a new SIC mapping with a valid code and an empty category, **when** I save it, **then** the mapping is available as an intentional "do not categorize by SIC" rule.
- **Given** I edit an existing SIC mapping, **when** I save it, **then** future categorization uses the updated code/category relationship, including an empty category if selected.
- **Given** I delete a SIC mapping, **when** the deletion succeeds, **then** the mapping is no longer used for future categorization.
- **Given** categories exist in the Categories page, **when** I create or edit a SIC mapping, **then** all current categories are available as mapping targets.
- **Given** I create or update a category in the Categories page, **when** I return to the SIC mapping page, **then** that category is available for mapping.

**Requirement Mapping**: FR8, FR9, FR11, FR13, FR14

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-05 — Prevent duplicate SIC mappings

**As a** Local Personal Finance User,
**I want** each SIC code to have only one mapping,
**so that** auto-categorization produces predictable results even when a mapping intentionally has no category.

**Personas**: Local Personal Finance User

**Acceptance Criteria**:
- **Given** a SIC code is already mapped, **when** I try to create another mapping with the same exact code, **then** the app rejects it.
- **Given** I enter a SIC code with leading or trailing whitespace, **when** the app validates it, **then** whitespace is trimmed before saving or duplicate checking.
- **Given** I enter an empty SIC code, **when** I save the mapping, **then** the app rejects it.
- **Given** I enter a SIC code containing a non-digit character, **when** the app validates it, **then** the app rejects it, because a SIC value parsed from OFX/QFX is always numeric and an alphanumeric mapping could never match a transaction.
- **Given** I save a mapping with an empty category, **when** validation runs, **then** the app accepts the empty category as intentional and does not reject the mapping solely because no category is selected.

**Requirement Mapping**: FR8, FR11

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-06 — Inspect SIC in transaction detail context

**As a** Local Personal Finance User,
**I want** to see a transaction’s SIC value in a secondary detail or edit context,
**so that** I can troubleshoot why a transaction was or was not SIC-categorized without cluttering the main table.

**Personas**: Local Personal Finance User

**Acceptance Criteria**:
- **Given** a transaction has a stored SIC value, **when** I open the Change Category modal, **then** the SIC code is visible.
- **Given** a transaction has a stored SIC value, **when** I open the Create Categorization Pattern modal, **then** the SIC code is visible.
- **Given** a SIC mapping has a `Description`, **when** either categorization modal displays SIC information, **then** the `Description` value is shown.
- **Given** a SIC mapping has no `Description` but has `Description_Detail`, **when** either categorization modal displays SIC information, **then** the `Description_Detail` value is shown.
- **Given** a transaction has no SIC value, **when** I open either categorization modal, **then** the UI handles the missing value cleanly.
- **Given** I view the main transactions table, **when** SIC support is enabled, **then** the table is not cluttered with a prominent new SIC column.

**Requirement Mapping**: FR10, FR14

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-07 — Upgrade existing local databases safely

**As an** Existing User Upgrading from an Older Database,
**I want** my existing local database upgraded in place,
**so that** I keep my financial history while gaining SIC support.

**Personas**: Existing User Upgrading from an Older Database

**Acceptance Criteria**:
- **Given** I have an existing `privateledger.db` without SIC columns/tables, **when** I start the upgraded app, **then** the app adds the required schema without deleting existing data.
- **Given** I have existing accounts, transactions, categories, text patterns, and import history, **when** the migration completes, **then** those records remain available.
- **Given** startup migration has already run once, **when** I restart the app, **then** migration runs safely without duplicate schema errors.

**Requirement Mapping**: FR3, NFR2, NFR4

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-08 — Initialize SIC mappings from an existing mapping file

**As an** Existing User Upgrading from an Older Database,
**I want** the app to import an existing SIC-to-category mapping file when available,
**so that** I can seed the database with mappings without configuring every code manually.

**Personas**: Existing User Upgrading from an Older Database, Maintainer/Developer

**Acceptance Criteria**:
- **Given** the app starts and `sic_mappings.csv` exists beside `config.json` and `privateledger.db`, **when** initialization runs, **then** mappings are copied into SQLite.
- **Given** the app starts and no `sic_mappings.csv` file exists in that location, **when** initialization runs, **then** startup succeeds without failing.
- **Given** mappings already exist in SQLite, **when** the app starts again with the file still on disk, **then** the import is skipped and my existing mappings — including any I edited in the UI — are left untouched.
- **Given** the CSV mapping file references categories by `Category_Name` with `Category_ID` as a tiebreaker, **when** import runs, **then** mappings are created for valid resolvable categories and rows where name and ID disagree are rejected.
- **Given** the CSV mapping file contains `SIC_Code`, `Description`, `Description_Detail`, `Category_Name`, and `Category_ID` columns, **when** import runs, **then** description values are stored for display.
- **Given** the mapping file contains a SIC entry with both category columns omitted, **when** import runs, **then** the mapping is imported and later matching transactions are not categorized by SIC.
- **Given** the mapping file contains invalid non-empty category references, **when** import runs, **then** those entries are reported or skipped according to the final import design.

**Requirement Mapping**: FR4, NFR4

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-09 — Preserve local-only privacy

**As an** Existing User Upgrading from an Older Database,
**I want** SIC categorization to remain fully local,
**so that** my financial data and merchant metadata are not sent to external services.

**Personas**: Existing User Upgrading from an Older Database, Maintainer/Developer

**Acceptance Criteria**:
- **Given** the app parses SIC values during import, **when** categorization runs, **then** no external SIC/MCC lookup service is called.
- **Given** mapping import/export is used, **when** the app reads or writes mapping data, **then** it uses local files and SQLite only.
- **Given** I manage SIC mappings, **when** I save changes, **then** they are stored locally.

**Requirement Mapping**: NFR1

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-10 — Download current SIC mappings

**As a** Local Personal Finance User,
**I want** to download the current SIC-to-category mapping file,
**so that** I can back up, inspect, or transfer my configured mappings.

**Personas**: Local Personal Finance User

**Acceptance Criteria**:
- **Given** I am on the SIC mapping page, **when** I choose download, **then** the app downloads `sic_mappings.csv` representing the current mappings in SQLite.
- **Given** the downloaded file is used later as an upload/import source, **when** it is validated, **then** it includes `SIC_Code`, `Description`, `Description_Detail`, `Category_Name`, and `Category_ID` columns and supports omitted category values.
- **Given** no mappings exist, **when** I choose download, **then** the app still returns a valid empty CSV mapping file with headers.

**Requirement Mapping**: FR9, FR13

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-11 — Upload SIC mappings and merge with existing mappings

**As a** Local Personal Finance User,
**I want** to upload a SIC-to-category mapping file that adds new mappings and updates matching mappings,
**so that** I can restore or bulk update mapping configuration efficiently.

**Personas**: Local Personal Finance User

**Acceptance Criteria**:
- **Given** I select a SIC-to-category CSV mapping file, **when** I choose upload, **then** I confirm the import/update before anything is sent, and the app then validates and merges in a single request.
- **Given** a valid upload, **when** merge runs, **then** new codes are inserted, matching codes are updated, and database codes omitted from the file remain unchanged.
- **Given** the uploaded file contains only the required header, **when** merge runs, **then** it succeeds as a no-op and does not delete mappings.
- **Given** the uploaded file is invalid, **when** validation runs, **then** existing mappings are not changed and I receive a per-row report of what failed.
- **Given** an upload is ready to merge, **when** backup succeeds, **then** its path is reported; if backup fails, **then** the merge may continue but the result prominently reports the backup warning.
- **Given** the uploaded file contains duplicate SIC codes, **when** validation runs, **then** the upload is rejected.
- **Given** the uploaded file references categories by `Category_Name` with `Category_ID` as a tiebreaker, **when** validation runs, **then** valid resolvable references and omitted category values are accepted, and rows where name and ID disagree are rejected.
- **Given** the uploaded file contains a SIC entry with an omitted category, **when** the upload succeeds, **then** matching transactions are not categorized by SIC for that entry.
- **Given** a mapping is absent from the uploaded file, **when** the upload succeeds, **then** that mapping remains until I explicitly delete it.

**Requirement Mapping**: FR8, FR9, FR11, FR13

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-12 — Create SIC mappings from transaction categorization modals

**As a** Local Personal Finance User,
**I want** to create SIC mappings from the existing transaction categorization modals,
**so that** I can quickly turn a categorized transaction’s SIC code into a reusable rule.

**Personas**: Local Personal Finance User

**Acceptance Criteria**:
- **Given** I open the Change Category modal for a transaction with a SIC code and select a category, **when** I agree to create/update the SIC mapping, **then** the SIC mapping is created or updated to that category.
- **Given** I agree to create/update the SIC mapping from the Change Category modal, **when** the current transaction is updated, **then** the selected category is applied with rule source instead of manual source.
- **Given** I open the Create Categorization Pattern modal for a transaction with a SIC code and selected category, **when** I agree to create/update the SIC mapping instead, **then** only the SIC mapping is created or updated and no text pattern is created.
- **Given** I use either modal to create/update a SIC mapping, **when** no category is selected, **then** the modal does not create an empty-category SIC mapping.

**Requirement Mapping**: FR10, FR14

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## US-13 — Maintain reliable implementation boundaries and tests

**As a** Maintainer/Developer,
**I want** SIC support implemented within the existing architecture with repeatable tests,
**so that** the feature is maintainable and safe to change.

**Personas**: Maintainer/Developer

**Acceptance Criteria**:
- **Given** the feature is implemented, **when** code is reviewed, **then** dependencies still follow handler → service → repository → SQLite layering.
- **Given** parser, repository, categorizer, migration, mapping file import/export, and merge/upsert behavior exist, **when** tests run, **then** key scenarios are covered by example-based tests.
- **Given** SIC validation/normalization and idempotent mapping file import have clear properties, **when** property-based tests are feasible, **then** partial PBT expectations are addressed with an appropriate Go PBT framework or documented rationale.
- **Given** implementation is complete, **when** `go test ./...` runs, **then** all tests pass.

**Requirement Mapping**: NFR3, NFR5, NFR6, Acceptance Criteria 17

**INVEST Check**: Independent, Negotiable, Valuable, Estimable, Small, Testable.

---

## Coverage Matrix

| Requirement | Covered By |
|---|---|
| FR1 | US-01 |
| FR2 | US-01 |
| FR3 | US-07 |
| FR4 | US-08 |
| FR5 | US-02 |
| FR6 | US-02 |
| FR7 | US-03 |
| FR8 | US-04, US-05, US-11 |
| FR9 | US-04, US-10, US-11 |
| FR10 | US-06, US-12 |
| FR11 | US-05, US-11 |
| FR12 | US-01, US-02 |
| FR13 | US-04, US-10, US-11 |
| FR14 | US-03, US-04, US-06, US-12 |
| NFR1 | US-09 |
| NFR2 | US-07 |
| NFR3 | US-13 |
| NFR4 | US-07, US-08 |
| NFR5 | US-13 |
| NFR6 | US-13 |
