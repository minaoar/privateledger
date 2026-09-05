# Project Guidelines — PrivateLedger

This is the canonical provider-neutral source for durable project conventions. All planning, production-code, review, and test roles must read it before acting. Feature-specific approved AI-DLC artifacts take precedence when they intentionally amend these rules for their scoped feature.

## Product Boundary

PrivateLedger is a privacy-first personal finance application that imports OFX/QFX bank transactions, categorizes them, and produces local spending insights.

- Keep financial data and categorization logic local to the user's machine.
- Preserve the single-binary architecture and avoid cloud dependencies unless explicitly approved.
- Maintain compatibility with supported OFX/QFX variations and existing local databases.

## Technology and Structure

- Language: Go.
- HTTP/UI: Gin, Bootstrap, HTMX, and vanilla JavaScript.
- Persistence: SQLite through `modernc.org/sqlite`; no CGO dependency.
- Dependency wiring belongs in `cmd/privateledger/main.go`; do not introduce global service state or singletons.
- Application layers flow in one direction:

  `handler -> service -> repository -> SQLite`

- Parsers produce domain models consumed by services; repositories must not depend on services or handlers.
- Templates and static assets are embedded so releases remain a single binary.

## Durable Domain Invariants

### Transactions

- The SQLite table is `ledger_transaction`, not the reserved word `transaction`.
- Transaction deduplication uses exactly:

  `account_id, trn_type, fit_id, date_posted`

- Do not add fields such as SIC to the deduplication key unless an approved requirement explicitly changes it.
- Derive debit/credit transaction type from OFX type and amount; do not trust an unvalidated direct mapping.

### Categorization

- `category_source = 0`: none.
- `category_source = 1`: rule.
- `category_source = 2`: manual.
- Automatic categorization and recategorization must never overwrite manual categorization.
- Text category patterns use case-insensitive contains matching; first match wins unless an approved feature design changes priority.
- Adding patterns may recategorize only eligible uncategorized transactions.
- Deleting a category clears affected transaction categories through the approved foreign-key/service behavior.
- Category patterns are globally unique.

### Database

- Use parameterized SQL for all values.
- Enable and respect SQLite foreign keys.
- Schema changes must work for fresh databases and upgrade existing databases without data loss.
- Migrations and startup initialization must be idempotent.
- Preserve documented `ON DELETE` behavior and verify changes that affect referential integrity.

## Implementation Conventions

- Put business logic in services, persistence in repositories, transport behavior in handlers, and domain data/behavior in models.
- Prefer constructor injection and explicit dependencies.
- Use `slog` for runtime logging; do not use `fmt.Println` or the standard `log` package for application logging.
- Include useful context in errors and logs without exposing sensitive financial data unnecessarily.
- Use `filepath.Join` and cross-platform APIs for filesystem behavior.
- Keep changes focused on the approved unit/plan; avoid unrelated refactoring and duplicate replacement files.
- Application code belongs in the workspace source tree. AI-DLC documentation belongs under `aidlc-docs/`.

## Build and Verification

Common commands:

```bash
make build
make test
go test ./...
go vet ./...
gofmt -w <changed-go-files>
```

- Add tests beside the relevant Go package using `_test.go` files.
- Use temporary databases/files for tests; never mutate the user's real `privateledger.db`, `config.json`, or financial exports.
- Store reusable OFX/QFX fixtures under package `testdata/` where appropriate.
- Verify fresh-database and existing-database paths for schema changes.
- Preserve the independent ownership boundary defined by AI-DLC: the production provider does not author or weaken verification tests, and the independent provider does not modify production code.

## AI-DLC Context Loading

Before unit work, read as applicable:

1. `aidlc-docs/aidlc-state.md`
2. `aidlc-docs/inception/requirements/requirements.md`
3. Assigned stories and unit-of-work artifacts
4. The current unit's approved functional design and NFR artifacts
5. The approved code-generation plan
6. The independent review artifact when fixing or re-reviewing production code

When these artifacts conflict with unapproved assumptions, follow the approved artifacts and record the discrepancy.
