# Unit of Work Plan — Issue #5 SIC Auto-Categorization

## Purpose

Decompose the approved SIC auto-categorization design into manageable development work while preserving PrivateLedger's brownfield, local-only, single-binary monolith architecture.

## Context Summary

- Project type: brownfield Go monolith with embedded HTMX/Bootstrap UI and SQLite.
- Approved scope: SIC parsing/storage, migration, mapping CRUD and CSV workflows, categorization priority, transaction-modal integration, and tests.
- Architectural boundary: `handler -> service -> repository -> SQLite`; no independently deployable service is introduced.
- Story set: US-01 through US-13.
- Infrastructure design is skipped by the approved execution plan.

## Part 1 — Decomposition Decisions

The categories required by the AI-DLC Units Generation stage were evaluated as follows:

- Story Grouping: applicable because the stories span import, categorization, configuration, UI, migration, and testing.
- Dependencies: applicable because persistence and parser changes enable categorization and UI workflows.
- Team Alignment: applicable because ownership affects whether logical modules should become separate handoff units.
- Technical Considerations: applicable to confirm the approved single-binary deployment boundary is retained during decomposition.
- Business Domain: applicable because import enrichment, categorization, and mapping administration could be treated as separate capabilities or one cohesive feature.
- Code Organization (greenfield multi-unit only): not applicable; this is a brownfield monolith and the existing directory/layer organization is retained.

### Question 1 — Story Grouping and Business Boundary

How should the SIC feature stories be grouped into units of work?

A) One SIC Auto-Categorization unit containing logical implementation modules for persistence/import, categorization, mapping management, UI, and tests (recommended for the existing monolith)

B) Three units: SIC Data Foundation, SIC Categorization, and SIC Management/UI

C) One unit per user-facing capability, producing more granular handoffs

X) Other (describe the desired grouping after the tag)

[Answer]:B

### Question 2 — Dependency and Delivery Sequence

Within the selected grouping, which delivery sequence should guide implementation planning?

A) Foundation first: schema/model/parser/repositories, then categorization/services, then API/UI, then cross-cutting verification (recommended)

B) Vertical slices: implement import, mapping management, and modal behavior end-to-end one at a time

C) UI/API contract first, then implement the underlying services and persistence

X) Other (describe the desired sequence after the tag)

[Answer]:B

### Question 3 — Team Alignment

What ownership model should the unit decomposition assume?

A) One maintainer/team owns the complete feature; logical modules are sequencing aids rather than separate handoffs (recommended)

B) Separate owners for backend/data and frontend/UI, with an explicit API handoff

C) Separate owners for each logical module, requiring independently reviewable module boundaries

X) Other (describe the team/ownership model after the tag)

[Answer]:A

### Question 4 — Technical and Deployment Boundary

Should every resulting unit/module remain part of the existing single binary and shared SQLite database?

A) Yes; retain the approved monolith, shared process, and shared database with no independent deployment boundary (recommended)

B) Keep one binary now, but define strict internal module interfaces intended for possible future extraction

C) Introduce an independently deployable component for part of the SIC feature

X) Other (describe the intended technical boundary after the tag)

[Answer]:A

## Answer Analysis

- Question 1 selects three units organized by technical/business capability: SIC Data Foundation, SIC Categorization, and SIC Management/UI.
- Question 2 selects end-to-end vertical slices organized by user workflow: import, mapping management, and modal behavior.
- Questions 3 and 4 are consistent: one team owns all units, which remain within the existing monolith and shared SQLite database.
- A follow-up is required because the selected unit grouping and delivery sequence imply two different primary boundaries. The story map and dependency matrix cannot unambiguously use both as the unit definition.

### Follow-up Question 1 — Primary Unit Boundary

Which interpretation should control the generated units of work?

A) Keep the three Q1 units as the primary units: SIC Data Foundation, SIC Categorization, and SIC Management/UI. Use vertical slices only as incremental delivery checkpoints within those units.

B) Use three end-to-end vertical units instead: SIC Import and Storage, SIC Mapping Management, and Transaction Categorization Integration. Each unit may touch model, repository, service, handler, and UI layers.

C) Keep the three Q1 units and replace Q2's vertical-slice sequence with their dependency order: Data Foundation, then Categorization, then Management/UI.

X) Other (describe the exact unit boundaries and sequencing rule after the tag)

[Answer]:B

### Follow-up Resolution

The follow-up selects end-to-end vertical capabilities as the controlling unit boundary. The generated decomposition will therefore use:

1. SIC Import and Storage
2. SIC Mapping Management
3. Transaction Categorization Integration

Each unit may cross the existing application layers while remaining inside the single binary and shared SQLite database. One maintainer/team owns the complete feature. This resolves the earlier conflict between capability-layer grouping and vertical delivery; no unanswered or ambiguous decomposition decisions remain.

## Part 2 — Generation Steps

These steps will be executed only after every answer above is complete, ambiguity analysis is complete, and the user explicitly approves this plan.

- [x] Confirm all answers are complete, specific, mutually consistent, and aligned with the approved application design.
- [x] Define unit boundaries, responsibilities, included logical modules, and delivery sequence.
- [x] Generate `aidlc-docs/inception/application-design/unit-of-work.md` with unit definitions and responsibilities.
- [x] Generate `aidlc-docs/inception/application-design/unit-of-work-dependency.md` with the dependency matrix and sequencing constraints.
- [x] Generate `aidlc-docs/inception/application-design/unit-of-work-story-map.md` mapping every story (US-01 through US-13) to a unit and logical module.
- [x] Validate unit boundaries against the monolith architecture and approved component/service dependencies.
- [x] Verify that every story is assigned exactly once at the unit level and that cross-module acceptance criteria remain traceable.
- [x] Verify the generated units are ready for per-unit Functional Design, NFR Requirements, and NFR Design.

## Required Generation Outputs

- `aidlc-docs/inception/application-design/unit-of-work.md`
- `aidlc-docs/inception/application-design/unit-of-work-dependency.md`
- `aidlc-docs/inception/application-design/unit-of-work-story-map.md`

## Approval Gate

After the answers have been reviewed and any follow-up questions resolved, explicit approval is required before Part 2 generation begins.
