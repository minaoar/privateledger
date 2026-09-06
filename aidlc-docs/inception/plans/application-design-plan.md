# Application Design Plan — Issue #5 SIC Auto-Categorization

## Purpose

Define high-level application components, responsibilities, interfaces, services, and dependencies for SIC auto-categorization before detailed Functional Design.

## Context

- Requirements: `aidlc-docs/inception/requirements/requirements.md`
- User stories: `aidlc-docs/inception/user-stories/stories.md`
- Execution plan: `aidlc-docs/inception/plans/execution-plan.md`
- Existing architecture: clean layered Go application using Gin, services, repositories, SQLite, embedded templates/static assets.

## Design Scope

Application Design will cover:

- Transaction SIC storage and parser integration.
- SIC mapping domain model and repository.
- SIC mapping service for CRUD, validation, recategorization orchestration, import/export, and startup mapping-file import.
- Categorizer extension for text-pattern-first, SIC-second matching.
- SIC mapping API handler and page route.
- Mapping file format boundary at a high level.
- Component dependency relationships.

Detailed business rules, validation edge cases, PBT design, and implementation sequencing will be handled in later AI-DLC stages.

## Application Design Checklist

- [x] Review requirements, stories, and execution plan.
- [x] Confirm application design questions are answered and unambiguous.
- [x] Identify new and modified components.
- [x] Define component responsibilities and high-level interfaces.
- [x] Define service orchestration responsibilities.
- [x] Define dependency and communication patterns.
- [x] Generate `aidlc-docs/inception/application-design/components.md`.
- [x] Generate `aidlc-docs/inception/application-design/component-methods.md`.
- [x] Generate `aidlc-docs/inception/application-design/services.md`.
- [x] Generate `aidlc-docs/inception/application-design/component-dependency.md`.
- [x] Generate consolidated `aidlc-docs/inception/application-design/application-design.md`.
- [x] Validate design completeness and consistency against FR1–FR14 and US-01–US-13.
- [x] Review design against software engineering best practices, implementation simplicity, and maintainability; verify every design claim against the existing codebase and `ofxgo` v0.1.3.
- [x] Apply review resolutions to all five application design artifacts (see `application-design.md` → Design Review Resolutions).
- [x] Amend `requirements.md` for FR4 (x2) and FR11 to match the reviewed design; align the affected `stories.md` acceptance criteria. FR13 needed no change; it already specified the single upload endpoint.

## Design Questions

Please answer each question by filling in the letter choice after the `[Answer]:` tag. If none of the options match, choose `X` and describe your preference after the tag.

### Question 1
Where should the SIC mapping page appear in navigation?

A) Add a top-level navigation item named “SIC Mappings”

B) Add it as a secondary link from the existing Categories page

C) Add both a top-level navigation item and a Categories page link

X) Other (please describe after [Answer]: tag below)

[Answer]: B.

### Question 2
How should the SIC mapping file identify target categories?

A) By category name only, because names are already unique and user-readable

B) By category ID only, because IDs are stable inside one SQLite database

C) By both category ID and category name, using name as fallback/validation

X) Other (please describe after [Answer]: tag below)

[Answer]: B

### Question 3
How should a mapping file represent an empty category?

A) Omit the category field for that SIC mapping

B) Include the category field with an empty string value

C) Include an explicit boolean such as `uncategorized: true`

X) Other (please describe after [Answer]: tag below)

[Answer]: A

### Question 4
Where should the optional startup SIC mapping file be located?

A) Beside `config.json` and `privateledger.db` in the application data directory

B) Embedded in the binary as a default file only

C) User-configurable path in `config.json`

X) Other (please describe after [Answer]: tag below)

[Answer]: A

### Question 5
Which component should own SIC mapping import/export and upload merge/update orchestration?

A) A dedicated `SICMappingService` in `internal/service`, used by startup, handler, and categorizer coordination

B) The existing `Categorizer`, because SIC mappings are categorization rules

C) The handler layer, because upload/download are UI/API concerns

X) Other (please describe after [Answer]: tag below)

[Answer]: B

**Design Adjustment**: After review, SIC mapping management is split out of the core `Categorizer`. The final design uses `SICMappingCategorizer` as the categorizer extension for SIC matching and `SICMappingService` for CSV/import/export/CRUD workflows.

### Question 6
How should upload import/update be exposed to users at the application-design level?

A) Upload immediately overwrites after server-side validation succeeds

B) Upload validates and returns a preview/summary; a second confirm action overwrites

C) Upload overwrites but creates a downloadable backup first

X) Other (please describe after [Answer]: tag below)

[Answer]: X. Both B and C.

### Question 7
How should transaction SIC visibility be handled if no transaction detail modal currently exists?

A) Add a lightweight transaction detail modal that includes SIC

B) Add SIC to the existing change-category modal only

C) Defer UI display and expose SIC through API/model only for now

X) Other (please describe after [Answer]: tag below)

[Answer]: X. "Create Categorization Pattern" modal and "change category" modal. In both cases, it should show the SIC code and it's default description if there is a SIC code. If user sets a category value, it should prompt if the SIC code to category mapping should be done and in that case, the SIC-category mapping should be applied, instead of text pattern to category. Update user stories and requirements if needed.

### Question 8
How should SIC mappings with empty categories be represented in the database model?

A) `category_id` nullable; NULL means intentionally no SIC category

B) Use a sentinel category ID such as 0

C) Store empty-category mappings in a separate table from categorized mappings

X) Other (please describe after [Answer]: tag below)

[Answer]: A

## Planned Artifacts

### `components.md`
Will define:
- Modified transaction model/parser/repository components.
- New SIC mapping model/repository/service/handler/page components.
- Modified categorizer and startup wiring components.

### `component-methods.md`
Will define:
- High-level method signatures and input/output types.
- No detailed business logic beyond method purpose.

### `services.md`
Will define:
- Service orchestration patterns for import-time categorization, mapping CRUD, startup import, download, upload merge/update, and recategorization.

### `component-dependency.md`
Will define:
- Dependency matrix.
- Data flow diagrams for import, mapping UI, upload/download, and startup import.

### `application-design.md`
Will consolidate all application design decisions into one reference document.

## Approval Gate

After answers are provided and validated, the design artifacts will be generated and presented for review.
