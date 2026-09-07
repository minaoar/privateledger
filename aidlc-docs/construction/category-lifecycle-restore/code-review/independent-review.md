# Independent Review — UOW-4 Category Lifecycle and Mapping-File Integrity

## Gate Result

**PASS**

Production revision `b7d0e1f2c2fd279c2116b25cf783a7ca197800d2` satisfies the approved UOW-4
behavior and verification obligations. All required tests pass, including both generated properties,
the behavioral response ceiling, the four reported header variants, row-parser compatibility, exact
diagnostics, seed logging, export compatibility, the 100,000-row merge measurement, and the short race
suite. No Blocking or High production finding remains.

Two non-blocking documentation/privacy findings are recorded below. Neither changes the production gate
result because the final approved UOW-4 decision is unambiguous, the implementation follows it, and the
remaining exposure is local, bounded and limited to a malformed seed that still parses as exactly five
columns.

## Reviewer and Scope

| Item | Value |
|---|---|
| Independent provider | OpenAI |
| Model | GPT-6 Codex |
| Role | Independent review and test author |
| Production provider | Anthropic / Claude Opus 5, as recorded in the commit |
| Branch | `support-mcc-for-category` |
| Production revision | `b7d0e1f2c2fd279c2116b25cf783a7ca197800d2` |
| Production files reviewed | `internal/model/sic_mapping.go`; `internal/service/sic_mapping_service.go`; `cmd/privateledger/main.go` |
| Ownership boundary | Reviewer changed tests and this review artifact only; production files were not modified |

`git diff-tree --no-commit-id --name-status -r b7d0e1f` also showed AI-DLC state, audit, plan,
handoff and production-summary changes in the production commit. Runtime scope was exactly the three
files named in the handoff. `go.mod`, `go.sum`, handlers, repositories, templates and schema were
unchanged.

## Artifacts Consulted

- `PROJECT_GUIDELINES.md`, `.claude/CLAUDE.md`, `.claude/agents/independent-test-reviewer.md`,
  `.codex/skills/independent-test-reviewer/SKILL.md`
- `aidlc-docs/aidlc-state.md` and `.aidlc-rule-details/construction/code-generation.md`
- `aidlc-docs/construction/category-lifecycle-restore/code/independent-review-handoff.md`
- All files under UOW-4 `functional-design/`, `nfr-requirements/` and `nfr-design/`
- `aidlc-docs/construction/plans/category-lifecycle-restore-code-generation-plan.md`
- FR4, US-14, the UOW story map, unit definition/dependency material, and the frozen UOW-4 findings register
- The amended UOW-1 NFR-U1-SEC-02 and NFRP-U1-08, and UOW-2 NFR-U2-SEC-01
- Production commit `git show b7d0e1f`, its parent implementation, relevant surrounding source, and
  the existing UOW-1/UOW-2 reviewer tests and performance harness

## Acceptance and NFR Traceability

| Requirement | Independent evidence | Result |
|---|---|---|
| US-14; BR-U4-01 through BR-U4-06 | `TestReviewU4HeaderNormalizationProperty` generates arbitrary per-letter case, surrounding Unicode whitespace and optional BOM. `TestReviewU4HeaderStrictnessProperty` generates reorder, omission, extra-column and altered-name cases. | PASS |
| U4-02; NFR-U4-TEST-03 | `TestReviewU4ObservedHeaderVariantsImport` commits one mapping for lowercase, mixed-case, post-comma whitespace and UTF-8 BOM headers. | PASS |
| BR-U4-05; NFR-U4-COMPAT-02 | The rejecting property proves count, order and names remain strict. Wrong count and wrong-column examples remain rejected. | PASS |
| FieldsPerRecord compatibility concern | `TestReviewU4RowCSVParsingCompatibility` preserves `malformed_csv` for short and long ragged rows, and accepts quoted commas, escaped quotes and embedded newlines without changing stored field content. | PASS |
| BR-U4-11; DP-U4-05 | `TestReviewU4DiagnosticMessages/renamed_category_names_stale_and_current_values` pins the one-edit repair message with unresolved and current names. | PASS |
| ID-message gating | Unknown, malformed, zero and negative IDs retain the short `category_not_found` message; a malformed ID paired with a resolved name remains owned by `invalid_category_id`. | PASS |
| BR-U4-12/16; DP-U4-04/05 | Four case-colliding categories are listed deterministically, capped at three, with an accurate `and 1 more`; format-like `%s` text remains data. | PASS |
| NFR-U4-SEC-01; NFR-U4-TEST-02 | `TestReviewU4DiagValueProperty` generates arbitrary byte strings, including invalid UTF-8. Output is valid UTF-8, control-free, and at most 64 runes plus ellipsis. Explicit multi-byte/control boundaries prove sanitization precedes truncation. | PASS |
| NFR-U4-SEC-02 | Exact wrong-column and wrong-count diagnostics contain only canonical fixed text, positions and counts; received header text is absent. | PASS |
| NFR-U4-SEC-04 | `TestReviewU4DiagnosticMessageBackstop` pins 512 runes plus ellipsis without splitting a multi-byte rune. | PASS |
| DP-U4-06 behavioral ceiling | A 3,145,822-byte HTTP upload with a multi-megabyte `Category_Name` returned 422 and a 430-byte JSON response, below the fixed 65,536-byte ceiling. The far-tail marker was absent. | PASS |
| DP-U4-07; NFR-U4-TEST-04 | `TestReviewU4SeedLogCarriesBoundedRepairMessage` drives a real rejected seed through validation and startup logging. The row log carries the exact bounded repair message plus stable field/code, followed by the unchanged summary. Static inspection confirms the HTTP upload path logs no diagnostic message. | PASS |
| BR-U4-06/19; NFR-U4-COMPAT-01 | `TestReviewU4ExportRemainsCanonicalAndByteIdentical` pins the canonical populated CSV bytes, including comma/newline quoting. Existing UOW-2 and UOW-3 suites pass. Stable codes are unchanged. | PASS |
| BR-U4-17/18; NFR-U4-REL-01 | Rejected validation yields no candidates; handler ceiling test returns invalid before merge. The production diff adds no mutation path. | PASS |
| NFR-U4-PERF-01 | Existing 100,000-pre-state/100,000-upload merge harness: one warm-up excluded, five measurements, median 1.410713833 s against 10 s. | PASS |
| NFR-U4-TEST-06 | `go test -race -short -count=1 ./...` passes all packages. | PASS |
| Frozen scope | No UTF-16 support, ID fallback, case-insensitive uniqueness migration, or category-creation length bound was added or required. U4-01 remains explicitly mitigated rather than resolved. | PASS |

## Production Review

### Diagnostic construction and bounds

`NewDiagValue` at `internal/model/sic_mapping.go:234` iterates runes, replaces every control rune before
counting/writing it, and appends the ellipsis only after a 65th input rune proves truncation. Invalid
UTF-8 bytes become U+FFFD through Go rune iteration, so the result is valid UTF-8. `AddError` applies the
512-rune backstop at `internal/model/sic_mapping.go:269-282`; `AddErrorf` accepts only `DiagValue`
arguments and delegates through that backstop at `:288-296`.

Every current name-bearing service diagnostic calls `model.NewDiagValue` at
`internal/service/sic_mapping_service.go:437-480`. `rejectAmbiguousCategory` constructs its format only
from a fixed prefix, a placeholder slice, and an integer-derived suffix at `:460-480`; no user string
can become format syntax. The tests include `%s` in category text and pin it as literal output.

### Header and CSV parsing

The header is read with flexible record length at `internal/service/sic_mapping_service.go:253-280`, then
`FieldsPerRecord` is restored to five immediately after a successful match at `:282-283`. This makes
the approved count diagnostic reachable while retaining row-level ragged-record rejection. BOM removal
is first-column-only, followed by `TrimSpace` and `EqualFold` at `:375-394`. The canonical export header
is unchanged.

### Category diagnostics and lookup

The exact, folded and ID indexes are built in one pass over the already-loaded category slice at
`internal/service/sic_mapping_service.go:493-515`; no query or repository method was added. The detailed
`category_not_found` form is selected only after integer parsing, positivity and a successful ID lookup
at `:437-447`. Unknown or malformed IDs use the short form, while a malformed ID paired with a resolved
name reaches the existing `invalid_category_id` branch at `:554-558`.

### Seed logging

`cmd/privateledger/main.go:286-291` adds the bounded report message to the existing capped per-row seed
warning. The record cap and aggregate summary are unchanged. The implementation matches the later,
specific NFR-U4-SEC-01 amendment and DP-U4-07 decision.

## Dated-Amendment Adjudication

| Amendment | Judgment |
|---|---|
| BR-U4-13 | Honest narrowing: it expressly admits bounded file-supplied `Category_Name` and corrects the assumption that database names are inherently safe. |
| UOW-4 domain invariant | Honest narrowing for the same reason; it clearly supersedes the former no-file-content invariant. |
| UOW-2 NFR-U2-SEC-01 | Incomplete after the later seed-log decision. Its bounded diagnostic exception is honest, but its retained absolute no-log sentence conflicts with DP-U4-07; see U4-R-F01. |
| UOW-1 NFR-U1-SEC-02 | The necessity and bounds are stated, but its assertion that a 64-rune category value cannot be financial data overstates what a mis-delimited file can guarantee; see U4-R-F02. |
| UOW-1 NFRP-U1-08 | Candidly admits the value is file content, but its remaining absolute exclusion of descriptions/account identifiers has the same positional-trust limit; see U4-R-F02. |

The amendments therefore establish a clear final product decision, but two earlier absolute statements
need documentation correction. Production follows the approved final decision rather than using the
amendments as a rationale for broader logging.

## Findings

### U4-R-F01 — Medium — Earlier logging artifacts contradict the final seed-message decision

**References:**

- `aidlc-docs/construction/category-lifecycle-restore/nfr-requirements/tech-stack-decisions.md:15`
- `aidlc-docs/construction/sic-mapping-management/nfr-requirements/nfr-requirements.md:109-116`
- `aidlc-docs/construction/category-lifecycle-restore/nfr-requirements/nfr-requirements.md:53-69`
- `aidlc-docs/construction/category-lifecycle-restore/nfr-design/nfr-design-patterns.md:134-154`
- `cmd/privateledger/main.go:286-291`

The UOW-4 technology table still says logs carry codes and counts only and never echoed names. The
dated UOW-2 amendment likewise says echoed names are never logged and that this remains binding. Both
conflict with the later approved UOW-4 amendment and DP-U4-07, which explicitly require the bounded
message on the startup seed path. Production follows the later and more specific decision.

**Acceptance condition:** amend the stale technology row and UOW-2 clause to state the seed-only,
bounded-message exception while retaining the no-message rule for uploads. This is an AI-DLC artifact
consistency correction; no production change is requested.

**Status:** Open, non-blocking for this production gate.

### U4-R-F02 — Low — A syntactically five-field mis-delimited seed can log bounded sensitive text from column 4

**References:**

- `internal/service/sic_mapping_service.go:282-283,323-327`
- `cmd/privateledger/main.go:286-291`
- `aidlc-docs/construction/sic-import-and-storage/nfr-requirements/nfr-requirements.md:93-106`
- `aidlc-docs/construction/sic-import-and-storage/nfr-design/nfr-design-patterns.md:158-168`

The service necessarily treats logical CSV column 4 as `Category_Name`. A hand-edited, mis-delimited
row that still parses as exactly five fields can therefore place part of a description or account text
there. The startup seed path will persist that value in its diagnostic log after bounding it. The UOW-1
amendments overstate the result when they say a category name cannot be financial data and that no
transaction description or account identifier can be logged.

An ordinary extra unquoted comma produces six fields and is rejected as `malformed_csv` before semantic
validation. Exposure requires an offsetting omission or another malformed layout that still produces
five fields. Processing is local, file logging is disabled by default, every value is control-sanitized
and limited to 64 runes, the message is limited to 512 runes, and only the first capped diagnostics are
logged. Those controls reduce this to a bounded residual privacy risk and do not justify failing the
approved UOW-4 production behavior.

**Acceptance condition:** record explicit product acceptance of this positional-trust residual, or in a
future scoped change add a seed-specific policy that avoids persisting ambiguous user text. Amend the
absolute UOW-1 wording so it describes the actual bounded guarantee.

**Status:** Open residual/candidate; non-blocking for this production gate.

## Reviewer-Owned Test Changes

- Added `internal/model/uow4_diagnostic_review_test.go`: arbitrary-byte Rapid property, explicit
  invalid-UTF-8/control/multi-byte boundary cases, and assembled-message backstop.
- Added `internal/service/uow4_review_test.go`: both header properties, four observed imports, ragged and
  quoted CSV behavior, exact category/header diagnostics, format-string isolation, and export bytes.
- Added `internal/handler/uow4_diagnostic_ceiling_review_test.go`: multi-megabyte upload response ceiling.
- Added `cmd/privateledger/uow4_seed_log_review_test.go`: real seed validation through bounded startup log.
- Corrected `internal/service/sic_mapping_seed_test.go`:
  - removed UTF-8 BOM from the invalid-seed table because US-14 now requires successful import;
  - separated protected description content from the now-permitted `Category_Name` marker in
    `TestValidateCSV_DiagnosticsAreSafe`, retaining the original description/account privacy assertion.

These corrections resolve handoff items HF-01 and HF-02. They change tests only where the prior
expectation contradicted approved UOW-4 artifacts; they do not weaken a still-applicable assertion.

## Commands and Results

| Command | Result |
|---|---|
| `go test -count=1 ./...` before reviewer edits | Expected baseline FAIL only in HF-01 and HF-02; all other packages passed. Service completed in 84.893 s. |
| Focused UOW-4 tests across model, service, handler and main | PASS |
| `go test -count=1 ./...` after test changes | PASS; service 84.795 s and every other tested package passed |
| `go test ./internal/handler -run '^TestReviewU4UploadDiagnosticResponseHasFixedCeiling$' -count=1 -v` | PASS; 3,145,822-byte CSV, 430-byte response, 65,536-byte ceiling |
| `go test ./internal/service -run '^TestReviewU2MergePerformance$' -count=1 -v` | PASS; warm-up 1.401259125 s; measured 1.390665541, 1.400273250, 1.426106417, 1.417743542, 1.410713833 s; median 1.410713833 s |
| `go test -race -short -count=1 ./...` | PASS across all packages |
| `go vet ./...` | PASS |
| `go build ./...` | PASS |
| `git diff --check` | PASS |

## Coverage Limits and Deliberate Exclusions

- No browser test was added because UOW-4 changes no template, JavaScript, handler behavior or route.
  The response ceiling exercises the real Gin upload boundary and JSON body; existing DOM rendering
  remains `textContent` and its UOW-2 tests remain green.
- UTF-16 detection is candidate C4-01 and is outside the frozen scope.
- A category-name creation bound is candidate C4-02 and is outside the frozen scope.
- U4-01 remains mitigated: stale names still reject and require the documented one-edit repair.
- Export compatibility is pinned against the established canonical bytes rather than by executing a
  second binary built from the parent revision.

## Final Status

**PASS** — required behavior and tests pass; no unresolved Blocking or High finding.
