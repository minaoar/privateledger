# Code Generation Plan — UOW-4 Category Lifecycle and Mapping-File Integrity

## Authority and Scope

This plan is the single source of truth for UOW-4 Code Generation. Production generation executes these
steps in order and may not add behaviour outside the approved requirements, functional design and NFR
design.

UOW-4 implements **US-14** — "Accept cosmetic mapping-file variation and explain what must be fixed" —
addressing admitted findings **U4-01** (mitigated, not resolved) and **U4-02**.

It adds **no** table, column, index, dependency, endpoint, page, route, template change or schema
change, and mutates nothing new.

## Mandatory Ownership Boundary

### Production role (this session)

- May modify the production Go files and production documentation listed below.
- **Must not** create or modify `_test.go` files, `testdata/`, fixtures, benchmarks, property tests,
  test-only dependencies, or the independent review artifact.
- **Must not** modify the independent role's existing UOW-1, UOW-2 or UOW-3 tests. A test failing
  against deliberately changed behaviour is reported as a handoff finding, never edited.

### Independent review/test role (separate provider session)

- Owns review, every verification test in DP-U4-06, and
  `aidlc-docs/construction/category-lifecycle-restore/code-review/independent-review.md`.
- Must report **PASS** before UOW-4 Code Generation completes.

## Verified Starting State

| Observation | Location |
|---|---|
| Eleven diagnostic sites, all in one file; three of them change | `sic_mapping_service.go:258,264,274,299,310,312,397,406,411,419,423` |
| No existing test references `matchesSICMappingHeader`, `indexCategories`, `resolveSICCategory` or `sicRowValidator` | verified across `internal/` and `cmd/` — signature changes cannot break test compilation |
| One existing test pins a message string, as a fixture it constructs itself | `startup_sic_outcome_test.go:244` |
| Categories already loaded before row validation | `sic_mapping_service.go:283-287` |
| `GetAll` orders by name, so folded-match order is deterministic | `category_repo.go:87` |
| Seed logging emits row, field, code and omits message | `main.go:271-288` |
| Upload diagnostics already render `e.message` through `textContent` | `sic_mappings.html:370`, `:187` |

## A Reviewer-Facing Conflict, Resolved Before Handoff

`cmd/privateledger/startup_sic_outcome_test.go:236` is a reviewer-owned test named
`TestLogSICMappingImportOutcome_NoSeedPayloadInDiagnostics`, whose comment cites **NFRP-U1-08 /
NFR-U1-SEC-02: startup logs must not contain file contents**.

Q1 A adds a category name from the seed CSV to a startup log. That is file content by any honest
reading. The test itself still passes — its fixture never puts the secret in a `Message` — but the rule
its name encodes is one this unit changes.

Both artifacts were therefore amended and dated **before** this plan, so the reviewer meets a documented
narrowing rather than an apparent violation:

- `sic-import-and-storage/nfr-requirements/nfr-requirements.md` NFR-U1-SEC-02
- `sic-import-and-storage/nfr-design/nfr-design-patterns.md` NFRP-U1-08

This is the U2-USER-02 lesson applied: that defect reached the user because the design decision was
never carried into any handoff I wrote. Both amendments are repeated in the handoff document at Step 8.

## Expected Production Files

**Modify**: `internal/model/sic_mapping.go`, `internal/service/sic_mapping_service.go`,
`cmd/privateledger/main.go`.

**Create**: `aidlc-docs/construction/category-lifecycle-restore/code/production-summary.md`,
`aidlc-docs/construction/category-lifecycle-restore/code/independent-review-handoff.md`.

**Explicitly not modified**: any template, any handler, any repository, `schema.sql`, `go.mod`,
`API_ROUTES.md`. Three production files change in total.

---

## Step 1 — `DiagValue` and the sanitizing constructor

File: `internal/model/sic_mapping.go`

- [ ] Add `maxDiagValueRunes = 64` and `maxDiagMessageRunes = 512` as named constants with comments
      citing NFR-U4-SEC-01 and NFR-U4-SEC-04.
- [ ] Add `DiagValue` as a struct wrapping an unexported string, so it cannot be constructed by
      conversion from `string`.
- [ ] Add `NewDiagValue(raw string) DiagValue`: replace every `unicode.IsControl` rune with U+FFFD,
      truncate to 64 runes counted by rune iteration, append U+2026 only when truncation occurred, and
      return valid UTF-8.
- [ ] Add `func (v DiagValue) String() string` so `%s` formatting works.
- [ ] Order matters: sanitize before truncating, so a truncation boundary cannot split a replaced rune.

## Step 2 — `AddErrorf` and the message backstop

File: `internal/model/sic_mapping.go`

- [ ] Add `AddErrorf(row int, field, code, format string, values ...DiagValue)`. Convert to `any` for
      `fmt.Sprintf` and delegate to `AddError`.
- [ ] In `AddError`, truncate any assembled message beyond 512 runes (DP-U4-03). Apply **after** the
      existing diagnostics-cap check so cap and truncation behaviour are unchanged.
- [ ] Do not change `MaxSICMappingDiagnostics`, `DiagnosticsTruncated`, or the append semantics.

## Step 3 — Header normalization

File: `internal/service/sic_mapping_service.go`

- [ ] Replace `matchesSICMappingHeader(header []string) bool` with a form returning why it failed —
      a small result carrying "ok", "wrong count" with the received count, or "wrong column" with the
      1-based position.
- [ ] Normalize per column: strip a leading UTF-8 BOM from the **first column only**, `strings.TrimSpace`,
      then compare with `strings.EqualFold`.
- [ ] Check count **before** names, so a four-column file is diagnosed as a count problem.
- [ ] Leave `sicMappingCSVHeader` untouched. Export continues to emit it unchanged.

## Step 4 — Header diagnostic messages

File: `internal/service/sic_mapping_service.go:274`

- [ ] Wrong column: `CSV header column %d must be %s` — position and canonical name, integers and a
      package constant only.
- [ ] Wrong count: `CSV header must have %d columns; this file has %d`.
- [ ] Keep the `invalid_header` code and the `(1, "header", ...)` row and field arguments unchanged.
- [ ] No received header text is echoed (NFR-U4-SEC-02).

## Step 5 — Three category indexes

File: `internal/service/sic_mapping_service.go:373`

- [ ] `indexCategories` returns exact, folded and **by-ID** maps from one pass.
- [ ] Update the single call site at line 287.
- [ ] No repository change and no new query.

## Step 6 — Category resolution diagnostics

File: `internal/service/sic_mapping_service.go:384`

- [ ] Add `sicRowValidator.rejectf(field, code, format string, values ...model.DiagValue)` wrapping
      `AddErrorf`, setting `invalid = true` exactly as `reject` does.
- [ ] `resolveSICCategory` takes the ID index.
- [ ] `category_not_found` with a resolving `Category_ID`:
      `Category_Name "%s" does not exist; Category_ID %d is currently "%s"`.
- [ ] `category_not_found` otherwise: `Category_Name "%s" does not exist`.
- [ ] `category_ambiguous`: `Category_Name "%s" matches %d categories: %s`, listing at most three names
      quoted and comma-separated, followed by ` and %d more` when more exist.
- [ ] Every name passes through `model.NewDiagValue`. No name is interpolated directly.
- [ ] The ID form is used only when the ID parses, is positive, and resolves. A malformed ID keeps the
      short form — this diagnostic must not double as ID validation, which `invalid_category_id` owns.
- [ ] Leave `id_without_name`, `invalid_category_id` and `category_conflict` messages unchanged.

## Step 7 — Seed path logs the bounded message

File: `cmd/privateledger/main.go:276-281`

- [ ] Add `slog.String("message", validationErr.Message)` to the existing per-row warning.
- [ ] Change nothing else: record count, `maxSICSeedDiagnostics`, the summary record and the
      omitted-diagnostics count all stay as they are, so the reviewer's record-count assertions hold.

## Step 8 — Production documentation

- [ ] Write `code/production-summary.md`: what changed, per file, with the requirement each satisfies.
- [ ] Write `code/independent-review-handoff.md` containing, at minimum:
      - the three changed production files and the diff scope;
      - **the design-decisions record** — Q1 B and FQ1 A from Functional Design, Q1–Q5 A and NFR-FQ1 A
        from NFR Requirements, Q1–Q4 A from NFR Design — so the reviewer can check the build against the
        decisions rather than against their own reading;
      - **the four dated amendments**: BR-U4-13, `domain-entities.md` invariants, UOW-2 NFR-U2-SEC-01,
        UOW-1 NFR-U1-SEC-02 and NFRP-U1-08;
      - the explicit statement that U4-01 is **mitigated, not resolved**, so a renamed category still
        fails to import and that is the approved outcome, not a defect to report;
      - the honest limit of DP-U4-02: `fmt.Sprintf` into `AddError` still compiles, which is why
        NFR-U4-SEC-04 and the ceiling test exist;
      - the six verification obligations of DP-U4-06, including the behavioural response-size ceiling
        test, which is the one that actually enforces the bound.

## Step 9 — Build verification (production role)

- [ ] `gofmt`, `go vet`, `go build ./...`.
- [ ] `go test -count=1 ./...` — existing suites must pass **unmodified**. Any failure is reported as a
      handoff finding, never fixed by editing a test.
- [ ] Confirm `git status` shows only the three production files and the two new documents.
- [ ] Confirm `go.mod` and `go.sum` are unchanged.

## Step 10 — Handoff

- [ ] Stage explicit paths only — never `git add -A` or `git add .` — and verify the staged set before
      committing.
- [ ] Commit and push.
- [ ] Hand off to the independent provider session. **Production does not close this gate.**

---

## Out of Scope

- Any finding not admitted before the 2026-09-06 scope freeze, including C4-01 (UTF-16) and C4-02
  (unbounded category names at creation).
- A restore capability, restore semantics, or any deletion of mappings.
- Deferred independent findings F-04 and F-05.
- New dependencies, schema changes, template changes, handler changes and repository changes.
- UOW-5, which remains registered and not started.
