# Unit of Work Dependencies — Issue #5 SIC Auto-Categorization

## Dependency Matrix

| Unit | Depends On | Dependency Type | Required Contract |
|---|---|---|---|
| UOW-1 — SIC Import and Storage | Existing PrivateLedger architecture | Foundation | Existing transaction import, startup migration, repository, and local-data conventions |
| UOW-2 — SIC Mapping Management | UOW-1 | Hard | `sic_mapping` schema/model/repository, normalization, category foreign-key semantics, application data directory |
| UOW-3 — Transaction Categorization Integration | UOW-1 | Hard | Stored transaction `sic_code`, SIC-aware transaction reads, `GetUncategorizedBySICCodes`, migration/index support |
| UOW-3 — Transaction Categorization Integration | UOW-2 | Hard | Mapping CRUD/read contracts, CSV-overwrite affected-code handoff, reloadable mapping source, modal upsert service contract |

## Dependency Direction

```text
Existing PrivateLedger
        |
        v
UOW-1: SIC Import and Storage
        |
        +----------------------+
        |                      |
        v                      |
UOW-2: SIC Mapping Management |
        |                      |
        +-----------+----------+
                    |
                    v
UOW-3: Transaction Categorization Integration
```

UOW-2 and UOW-3 both depend on UOW-1. UOW-3 additionally depends on UOW-2's stable mapping-management contracts. There is no reverse runtime dependency from repositories to services or from lower layers to handlers.

## Contract Details

### UOW-1 -> UOW-2

UOW-1 provides:

- The `SICMapping` model and normalized digits-only SIC representation.
- The `sic_mapping` table with global `UNIQUE(sic_code)` and nullable `category_id`.
- Repository primitives for list, get, create, update, delete, count, and atomic replace.
- The application data-directory convention used for startup import, upload backup, and download naming.
- Category foreign-key semantics needed for CSV category resolution.

UOW-2 must not redefine normalization, uniqueness, persistence, or data-directory rules.

### UOW-1 -> UOW-3

UOW-1 provides:

- `Transaction.SICCode` and persisted `ledger_transaction.sic_code`.
- Parser extraction and import propagation.
- `idx_txn_sic` and `TransactionRepository.GetUncategorizedBySICCodes`.
- SIC-aware transaction reads capable of exposing the joined display description.

UOW-3 consumes these contracts without changing the established deduplication key.

### UOW-2 -> UOW-3

UOW-2 provides:

- Authoritative SQLite mappings and service-mediated CRUD.
- Mapping list/load behavior used by `SICMappingCategorizer`.
- Affected-code output from create/update/upload workflows.
- `CreateOrUpdateFromTransaction` behavior used by the transaction modals.
- Page and API contracts for mappings and current categories.

UOW-3 provides the recategorization implementation invoked after eligible UOW-2 changes. To avoid a service cycle, orchestration follows the approved dependency injection design: `SICMappingService` receives the categorization collaborator/interface; repositories never call services, and `Categorizer` does not call `SICMappingService`.

## Shared Components and Coordination

| Shared Component | UOW-1 Responsibility | UOW-2 Responsibility | UOW-3 Responsibility |
|---|---|---|---|
| `model.SICMapping` / normalization | Define data and normalization contract | Validate management and CSV input using it | Match transaction codes using it |
| `SICMappingRepository` | Define schema-backed persistence primitives | Use CRUD and atomic replacement | Supply cached mapping reads |
| `TransactionRepository` | Persist/select SIC and add SIC-scoped query | No direct handler access | Consume scoped query and joined description |
| `SICMappingService` | Startup import portion | CRUD, page data, CSV import/export/backup | Recategorization and modal integration collaboration |
| Application wiring | Initialize schema/repositories/startup import | Add page/API handler and routes | Connect categorizer extension and transaction modal routes |
| Tests | Parser/migration/repository/startup tests | CRUD/CSV/API tests | Priority/preservation/modal/integration/PBT tests |

Shared files require sequential integration or coordinated commits, but they do not collapse the three behavioral units into layer-based units.

## Sequencing Constraints

1. Apply fresh-schema and existing-database migration changes before exercising repository or service behavior.
2. Stabilize normalization and repository contracts before CSV validation or categorizer caching uses them.
3. Complete mapping CRUD/read paths before wiring mapping reload and affected-SIC recategorization.
4. Establish transaction SIC persistence before testing import-time SIC categorization.
5. Complete handler/service contracts before finalizing page and modal JavaScript behavior.
6. Run cross-unit regression tests only after all three units are integrated; unit-specific tests run throughout.

## Parallel Work Constraints

- After UOW-1 contracts stabilize, most UOW-2 UI/CSV work and portions of UOW-3 categorizer logic can proceed in parallel.
- Changes to `transaction_repo.go`, `sic_mapping_service.go`, application wiring, and shared templates/routes require explicit coordination.
- UOW-3 cannot complete until UOW-2 exposes stable mapping and modal-upsert contracts.
- No unit may bypass the handler -> service -> repository direction to avoid waiting for another unit.

## External and Infrastructure Dependencies

- No external service, cloud lookup, network dependency, new process, or new database is introduced.
- The existing `ofxgo` SIC representation, Go standard-library CSV support, Gin, and SQLite remain the relevant technical dependencies.
- Infrastructure Design remains skipped because all units share the current deployment topology.

## Dependency Validation

- The unit graph is acyclic: UOW-1 -> UOW-2 -> UOW-3, with an additional UOW-1 -> UOW-3 edge.
- Every hard dependency corresponds to an approved component or service contract.
- Cross-unit callbacks are resolved through service-level dependency injection rather than repository-to-service calls.
- Deleting or replacing SIC mappings does not orphan transaction rows because transactions store the SIC value, not a mapping foreign key.
