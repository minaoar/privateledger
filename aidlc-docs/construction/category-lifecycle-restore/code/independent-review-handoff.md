# Independent Review Handoff — UOW-4 Category Lifecycle and Mapping-File Integrity

Production code is complete and authored by a different provider. This document carries everything
needed to review it against the **approved decisions**, not against a fresh reading of what the code
ought to do. That distinction matters: UOW-2 shipped a navigation defect to the user precisely because
the design-decisions record was never carried into a handoff.

## Scope of the Change

**Three production files.** No test, fixture, template, handler, repository, schema, route or dependency
was touched.

- `internal/model/sic_mapping.go`
- `internal/service/sic_mapping_service.go`
- `cmd/privateledger/main.go`

## Two Failing Tests — Deliberate, Not Regressions

Both are yours. Neither was edited. Both fail because UOW-4 deliberately reverses the behaviour they
pin.

### HF-01 — `TestImportFileIfPresent_InvalidSeedIsNonFatalAndAtomic/utf8_BOM_before_the_header`

`internal/service/sic_mapping_seed_test.go` lists a UTF-8 BOM among the cases asserting an invalid seed
imports zero mappings. It now imports one.

That reversal **is** finding U4-02. A spreadsheet on Windows writes a BOM by default, and the file was
being rejected with a diagnostic that gave no hint an invisible byte was the cause. Q2 A approved
stripping it.

### HF-02 — `TestValidateCSV_DiagnosticsAreSafe`

The fixture places `ACCOUNT-4111111111111111-SECRET` in three columns and asserts no diagnostic contains
it. The `Category_Name` occurrence now appears, bounded to 64 runes:

```
Category_Name|category_not_found|Category_Name "Nonexistent Category ACCOUNT-4111111111111111-…" does not exist
```

Two of the three columns carrying the string — `Description` and `Description_Detail` — are still not
echoed anywhere.

**The rules the test cites do not forbid this, and the boundary has been drawn here once before.**
BR-ERR-01 forbids dumping "transaction data or entire financial files". NFR-U1-SEC-02 forbids "OFX/QFX
payloads, complete CSV contents, transaction descriptions, account identifiers, or other unnecessary
financial data". A bounded category name is none of those. The test's name generalizes past the rules
beneath it.

There is direct precedent: during UOW-1 you narrowed `TestProperty_RejectionErrorsAreSafe` for exactly
this reason, recording that "the approved redaction boundary protects OFX/CSV payloads, transaction
descriptions and account identifiers — not the single numeric SIC field". The same reasoning applies to
a bounded `Category_Name`.

**A real residual risk is attached, and it is yours to weigh — see "Open Concern" below.**

## One Deviation From the Plan, and Why

The plan's Step 3 said "check count before names, so a four-column file is diagnosed as a count
problem". Implemented literally, that branch was **unreachable**: `csvReader.FieldsPerRecord` was set to
the canonical count before the header read, so a wrong-count header failed inside `csv.Reader` as
`malformed_csv` and never reached it. Q3 A would have shipped as dead code.

The header is now read with `FieldsPerRecord = -1` and the canonical count is set immediately after the
header matches. Row behaviour is unchanged — a ragged row still reports `CSV row is malformed`, verified
against the running binary.

Worth your attention as the one place where row-level parsing behaviour could have been altered by
accident.

## The Design-Decisions Record

Review against these. Anything the code does that these do not authorize is a finding; anything these
authorize is not, however it reads.

### Functional Design (approved 2026-09-07)

| Q | Answer | Meaning |
|---|---|---|
| Q1 | **B** | A stale `Category_Name` is still **rejected**. Resolving by `Category_ID` was declined |
| Q2 | A | Normalize the header for BOM, whitespace and case; order and count stay required |
| Q3 | A | Report case collisions; do not migrate the schema and do not pick one silently |
| Q4 | A | One story, US-14 |
| FQ1 | A | Retitle US-14; record U4-01 as **mitigated, not resolved** |

**U4-01 is mitigated, not resolved.** A renamed category still makes a file fail to import. That is the
approved outcome, not a defect. The fix makes the failure repairable in one find-and-replace.

The `Category_ID` fallback was declined because nothing in a file distinguishes a rename, where the ID is
correct, from a delete-and-recreate, where it may point at a different category — and because the
startup seed file is read on databases where `Category_ID` values are meaningless.

### NFR Requirements (approved 2026-09-07)

Q1 A bound and sanitize echoed values; Q2 A header diagnostics echo nothing; Q3 A distinguish count from
order failures; Q4 A no new performance target; Q5 A decline UTF-16; **NFR-FQ1 A** the bound applies to
every echoed name regardless of source, because `category.name` has no length constraint either.

### NFR Design (approved 2026-09-07)

Q1 A the seed path logs the bounded `Message`; Q2 A a constructed type plus a behavioural ceiling test;
Q3 A the ID index is built unconditionally; Q4 A the message wording as specified in DP-U4-05.

## Five Dated Amendments to Previously Approved Artifacts

Each narrows a rule that would otherwise read as violated. Please check the amendments are honest, not
just that the code matches them.

1. `category-lifecycle-restore/functional-design/business-rules.md` — **BR-U4-13**
2. `category-lifecycle-restore/functional-design/domain-entities.md` — invariants
3. `sic-mapping-management/nfr-requirements/nfr-requirements.md` — **NFR-U2-SEC-01**
4. `sic-import-and-storage/nfr-requirements/nfr-requirements.md` — **NFR-U1-SEC-02**
5. `sic-import-and-storage/nfr-design/nfr-design-patterns.md` — **NFRP-U1-08**

The last two exist because your `TestLogSICMappingImportOutcome_NoSeedPayloadInDiagnostics` cites them
by name. NFRP-U1-08 said logs "exclude file contents", and a category name from the seed CSV is file
content by any honest reading, so it was narrowed explicitly rather than argued around.

## An Honest Limit You Should Test Against

`DiagValue` makes the bounded path the path of least resistance. **It does not make bypass impossible.**
`fmt.Sprintf` into the constant-message `AddError` still compiles. That is why two further mechanisms
exist, and why the second is the one that matters:

- **NFR-U4-SEC-04**: `AddError` truncates any assembled message at 512 runes, bounding an undetected
  bypass.
- **The behavioural ceiling test in DP-U4-06**: upload a CSV carrying a multi-megabyte category name and
  assert total response size stays under a fixed bound. A source-level check for `fmt.Sprintf` would be
  brittle; a response-size assertion holds however a future site is written.

Please treat the ceiling test as the real enforcement, not the type.

## Open Concern — Production's Own, Raised Rather Than Buried

HF-02 is authorized by the approved decisions. The residual risk it exposes was not named precisely by
any stage question, so it is raised here rather than closed.

A `Category_Name` field contains whatever column 4 of the user's file holds. Normally that is a category
name. In a **mis-delimited CSV** — an unquoted comma inside a description shifts every later field —
column 4 could hold part of a transaction description.

On the upload path that is ephemeral: shown once, on the user's own screen, about the user's own file.
On the **seed path it is written to `server.log` and persists on disk.** That is a step beyond showing
it, and it is the part Q1 A added.

Bounds already in place: 64 runes, control characters replaced, 512-rune message cap, and only column 4
is ever echoed. Whether that is sufficient is a judgement the user should make with your assessment in
front of them. Production's view is that the risk is real but small and the mitigation it buys is
large — the seed file is the one path with no screen. **Report it as a finding if you disagree; do not
treat production's framing as settling it.**

## Verification Obligations — DP-U4-06

1. **NFR-U4-TEST-01** — `rapid` property, both directions: any canonical header decorated with arbitrary
   case, surrounding whitespace and an optional BOM always matches; any reordering, omission, extra
   column or altered name never matches. The second half is what proves tolerance did not become
   permissiveness.
2. **NFR-U4-TEST-02** — `rapid` property over arbitrary strings including invalid UTF-8: output is
   ≤ 64 runes plus ellipsis, carries no `unicode.IsControl` rune, and is valid UTF-8.
3. **Ceiling test** — multi-megabyte category name, bounded total response size.
4. **NFR-U4-TEST-03** — the four failures reproduced when U4-02 was admitted: lowercase header,
   mixed-case header, space after a comma, UTF-8 BOM. All four must now import.
5. **NFR-U4-TEST-04** — message content per DP-U4-05, plus the seed log carrying the bounded message.
6. **NFR-U4-TEST-05 / PERF-01** — export byte-identical; re-run UOW-2's 100,000-row merge and record the
   median against the existing ten-second bound.

## Ownership

You own review, all tests, and
`aidlc-docs/construction/category-lifecycle-restore/code-review/independent-review.md`. Production owns
production fixes. **Production does not close this gate; your PASS does.**
