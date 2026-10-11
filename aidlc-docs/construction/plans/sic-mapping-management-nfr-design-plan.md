# NFR Design Plan — UOW-2 SIC Mapping Management

## Inputs and Decision Authority

NFR Requirements and technology decisions approved on 2026-09-06 ("approved.").
Functional Design remains approved. The approved NFRs explicitly assign concrete timeout,
cancellation, multipart, and backup mechanics to NFR Design. Resolve these implementation
choices here for review; no additional product preference is needed.

## Category Assessment

| Category | Applicability and design response |
|---|---|
| Resilience | Applicable: bounded admission, SQLite rollback, truthful post-commit warnings, best-effort backups. Use no automatic mutation retry. |
| Scalability | Applicable within one local process: bounded HTTP intake and diagnostics, 1,000-row page fixture. Distributed scaling is excluded by approved local-only architecture. |
| Performance | Applicable: one prepared upsert transaction, one pre-state snapshot, indexed category lookup, ten-second merge benchmark. No page/export timing gate. |
| Security | Applicable: local paths, exclusive backup creation, escaped output, bounded bytes and safe diagnostics. No new authentication or compliance system under approved SEC-01. |
| Logical components | Applicable: handler intake, service admission/orchestration, repository transaction, injected collaborator. Queues, background jobs, new runtime frameworks and external infrastructure are excluded by approved technology decisions. |

## Checklist

- [x] Read project guidelines, state, approved NFRs and existing functional design context.
- [x] Inspect current configuration, validation, persistence, and application wiring.
- [x] Assess all five categories and distinguish implementation choices from missing preferences.
- [x] Define admission timeout/configuration, cancellation, and collaborator behavior.
- [x] Define HTTP outcomes, upload limits, diagnostics and backup mechanics.
- [x] Map responsibilities and verification seams to existing layers.
- [x] Generate nfr-design-patterns.md and logical-components.md.
- [x] Check requirement traceability and Partial PBT scope.
- [x] Receive explicit NFR Design approval. (2026-09-06, user: "approved the NFR.")

## Outputs

- `aidlc-docs/construction/sic-mapping-management/nfr-design/nfr-design-patterns.md`
- `aidlc-docs/construction/sic-mapping-management/nfr-design/logical-components.md`

No unanswered question tags. This stage proposes concrete mechanisms within the approved
requirements; approval of this design remains separate from the NFR Requirements approval.

## Simplification — 2026-09-06

User requested "simplify" after reviewing the complexity. Replaced configuration-file settings
with one constructor-injected five-second admission default; deferred the real collaborator's
processing budget to UOW-3; limited context changes to admission and new transaction operations;
selected standard bounded multipart parsing with temporary-file cleanup; reduced backup testing
seams to those actually needed. Atomicity, lock scope, byte/diagnostic bounds, truthful warnings,
benchmark targets and independent ownership remain intact. NFR Design still awaits approval.
