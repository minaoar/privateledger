# Personas — Issue #5 SIC Auto-Categorization

## Persona 1 — Local Personal Finance User

- **Role**: Primary user managing personal finances locally.
- **Goals**:
  - Import OFX/QFX files with minimal manual cleanup.
  - Have transactions categorized accurately and privately.
  - Configure categorization behavior when imported bank data includes useful SIC values.
- **Pain Points**:
  - Repeated manual categorization of similar transactions.
  - Bank transaction descriptions can be inconsistent or insufficient for text matching.
  - Wants financial data to remain local.
- **Relevant Feature Interactions**:
  - Imports OFX/QFX files.
  - Reviews transaction categories.
  - Opens SIC configuration page to add or update mappings.
  - Views transaction details when troubleshooting categorization.
- **Mapped Stories**: US-01, US-02, US-03, US-04, US-05, US-06, US-10, US-11, US-12

## Persona 2 — Existing User Upgrading from an Older Database

- **Role**: Current PrivateLedger user with an existing local `privateledger.db`.
- **Goals**:
  - Upgrade without losing existing accounts, transactions, categories, patterns, or import history.
  - Gain SIC categorization support without manually recreating data.
- **Pain Points**:
  - Local-only apps can lose trust if upgrades require deleting data.
  - Existing category rules should keep working exactly as before.
- **Relevant Feature Interactions**:
  - Starts the upgraded app against an existing database.
  - Imports new OFX/QFX files after upgrade.
  - Verifies previous categories and text patterns still work.
- **Mapped Stories**: US-07, US-08, US-09

## Persona 3 — Maintainer/Developer

- **Role**: Developer maintaining PrivateLedger’s clean architecture and tests.
- **Goals**:
  - Add SIC support without breaking import, deduplication, or categorization behavior.
  - Keep changes local-only and maintainable.
  - Verify behavior with repeatable tests.
- **Pain Points**:
  - Database schema changes can break existing local installations.
  - Categorization priority rules can regress silently without tests.
  - Mapping file import/export and overwrite behavior must be deterministic and safe.
- **Relevant Feature Interactions**:
  - Maintains parser/model/repository/service/handler layering.
  - Adds tests for parsing, mapping, migration, and categorization rules.
  - Ensures partial PBT expectations are addressed where applicable.
- **Mapped Stories**: US-08, US-09, US-13
