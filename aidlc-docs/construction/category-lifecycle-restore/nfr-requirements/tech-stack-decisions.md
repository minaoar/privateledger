# Technology Decisions — UOW-4 Category Lifecycle and Mapping-File Integrity

Generated 2026-09-07 alongside `nfr-requirements.md`, awaiting the same approval. UOW-1, UOW-2 and UOW-3
technology decisions are retained unchanged. **No dependency is added, removed, or upgraded**, and no
schema change is made.

| Concern | Decision | Requirement |
|---|---|---|
| Runtime | Existing Go module baseline; no new process | COMPAT-01 |
| HTTP and UI | Existing Gin, embedded Go templates, Bootstrap, HTMX, vanilla JavaScript | SEC-01 |
| Persistence | Untouched. No table, column, index or query is added | REL-01 |
| Header normalization | Standard library `strings` and `unicode/utf8` only | SEC-02 |
| Value sanitization | Standard library `unicode` and `unicode/utf8`; one shared helper | SEC-01 |
| Category ID index | An in-memory map built over the slice `categoryRepo.GetAll()` already returns | PERF-01 |
| Logging | Existing `log/slog`, carrying codes and counts only, never echoed names | SEC-01 |
| Verification | Go `testing`, temporary SQLite databases, the existing merge timing harness | TEST-03 through TEST-06 |
| Generated properties | Existing test-only `pgregory.net/rapid v1.1.0` | TEST-01, TEST-02, PBT-09 |
| External services / infrastructure | None; single local binary retained; Infrastructure Design skipped | SEC-03 |

## TD-U4-01 — Normalization happens at comparison time and is never persisted

The canonical column names remain the single definition of the contract. A normalized header form is
never stored, returned, or echoed, and export continues to emit the canonical spelling unchanged.

This keeps the tolerance one-directional. Input becomes more forgiving; output does not drift. A file
this application writes is still exactly the file it wrote before UOW-4, byte for byte.

## TD-U4-02 — One shared sanitization helper, not per-site formatting

Every diagnostic that echoes a name calls the same helper. Nothing formats a name into a message
directly.

This is a structural decision rather than a stylistic one. The realistic failure is not that today's
five diagnostics get the bound wrong — they will be written together, against NFR-U4-TEST-02. It is that
a sixth diagnostic added later interpolates a name directly and reintroduces the unbounded path, in a
change too small to attract review. Routing every site through one function makes that omission
something a reader can see.

## TD-U4-03 — Reuse the already-loaded category slice; add no query

`validateAndBuildSICMappings` already calls `categoryRepo.GetAll()` and passes the result to
`indexCategories`, which builds an exact-name map and a case-folded map. Naming the category behind
`Category_ID` needs a third index over that same slice.

No repository method is added, no query is issued, and the extra cost is one pass over categories per
upload — not per row.

## TD-U4-04 — No new diagnostic code

`category_not_found`, `category_ambiguous` and `invalid_header` are retained with their existing
spelling; only message text changes. This is what keeps the independent reviewers' UOW-2 tests, which
match on codes, unaffected by this unit.

It is also the reason Q5's UTF-16 detection was declined: it would require a code that does not exist,
which is a larger commitment than its line count suggests.

## TD-U4-05 — Standard library only for text handling

Truncation counts runes via `utf8.RuneCountInString` and rune iteration, not bytes. Control-character
detection uses `unicode.IsControl`. Case-insensitive comparison uses `strings.EqualFold`, and whitespace
trimming uses `strings.TrimSpace`, whose `unicode.IsSpace` definition already covers the non-breaking
space a spreadsheet may emit.

No text-processing, sanitization, or CSV library is introduced. Everything this unit needs is in the
standard library, and adding a dependency to a privacy-first single-binary application for five string
operations would be a poor trade.
