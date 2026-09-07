# NFR Requirements — UOW-4 Category Lifecycle and Mapping-File Integrity

## Status and Inputs

Generated 2026-09-07 from the approved UOW-4 functional design, project NFR1–NFR6, the UOW-1, UOW-2 and
UOW-3 NFR precedents, and stage answers Q1 A, Q2 A, Q3 A, Q4 A, Q5 A, NFR-FQ1 A. Awaiting explicit
approval.

UOW-4 introduces no persistence, no dependency, no endpoint and no mutation. Consequently most NFR
surfaces are carried forward unchanged and only one is genuinely new: a diagnostic message will now
contain user-controlled, unbounded text.

Reference environment carried forward from UOW-1, UOW-2 and UOW-3: Apple M1, 8 logical CPUs, 16 GiB RAM,
macOS 15.5, go1.26.0, APPLE SSD AP1024Q NVMe.

## Security and Safe Presentation

### NFR-U4-SEC-01 — Bounded, sanitized diagnostic values (blocking)

This is the requirement the whole stage exists for.

Three names may now appear inside a diagnostic message, and **all three are user-controlled text with no
length bound anywhere in the system**:

| Name | Source | Why it is unbounded |
|---|---|---|
| The unresolved `Category_Name` | The uploaded CSV | Resolved but never stored, so no validation applies; one CSV field may be as large as the 10 MiB upload |
| The current name of the category `Category_ID` refers to | The database | `category.name` is `TEXT NOT NULL UNIQUE` with no length constraint (`schema.sql:53`) and `CreateCategory` validates no length (`category_handler.go:96`) |
| Each colliding category name in the ambiguity diagnostic | The database | As above, and the diagnostic lists several of them |

Per NFR-FQ1 A, provenance does not decide the treatment. Every name echoed in any diagnostic, whatever
its source, is passed through **one shared sanitization helper** before it reaches a message:

1. Truncate to **64 runes**, counted in runes and never in bytes, appending U+2026 when truncation
   occurred. Sixty-four runes is far beyond any real category name, so the user still receives the value
   they need to perform the one-edit repair.
2. Replace every rune for which `unicode.IsControl` reports true — line feeds, carriage returns and tabs
   included — with U+FFFD.
3. Guarantee the result is valid UTF-8, so JSON encoding and DOM insertion cannot produce mojibake or
   silently-dropped bytes.

The ambiguity diagnostic additionally names **at most three** colliding categories, followed by an
"and N more" count. A per-name bound alone does not bound that message, because the number of colliding
names is itself unbounded.

Sanitization lives in one helper and every diagnostic that echoes a name calls it. This is a structural
requirement, not a stylistic one: the realistic failure is a diagnostic added months from now that
formats a name directly and bypasses the bound. A single choke point makes that omission visible.

**Echoed values are never written to a log.** `slog` continues to carry operation, row, field, stable
code and counts only, per NFR-U2-SEC-01.

Rendering remains DOM `textContent`, never assembled HTML. Escaping was already correct; it is restated
because the content flowing through it is newly attacker-influenced, and because replacing control
characters is what stops a crafted name from appearing to forge additional diagnostic lines in the
rendered list.

**Resulting ceiling.** With 50 retained diagnostics, three names per message at 64 runes, and bounded
fixed text, a worst-case report is on the order of tens of kilobytes rather than the 500 MB that
50 × 10 MiB would have permitted.

### NFR-U4-SEC-02 — Header diagnostics echo nothing (blocking)

Per Q2 A and Q3 A, header diagnostics are built entirely from fixed text and integers:

- An order or name mismatch reports the **1-based column position** and the **canonical name expected**
  there, for example *"CSV header column 4 must be Category_Name"*.
- A count mismatch reports the **expected and received column counts** as integers.

No received header text is echoed. This is not merely tidy: it means the pathological case of a header
that is one enormous field cannot inflate a response, and it keeps the header path outside the
NFR-U4-SEC-01 bound entirely rather than dependent on it.

### NFR-U4-SEC-03 — Locality unchanged

All processing remains local. No external service, telemetry, authentication system or cloud dependency
is introduced. Parameterized SQL and foreign keys are preserved. Backup path resolution is untouched by
this unit.

## Compatibility

### NFR-U4-COMPAT-01 — Stable codes, byte-identical export (blocking)

- No diagnostic code is added, renamed or removed. `category_not_found`, `category_ambiguous` and
  `invalid_header` retain their spelling; only their human-readable messages change.
- Export output is **byte-identical** to before this unit. Tolerance applies to input only; the canonical
  header spelling is still what gets written.
- Anything matching on codes — including the independent reviewers' existing UOW-2 tests — is therefore
  unaffected by design rather than by luck.

### NFR-U4-COMPAT-02 — Acceptance widens only for the header

Outside the header, this unit changes what the user is told and not what is accepted. A stale
`Category_Name` is still rejected; a name/ID disagreement is still rejected; an ID without a name is
still rejected; one invalid row still rejects the whole file. Failure ordering is unchanged: size
rejection precedes validation, validation precedes any backup or mutation.

## Performance

### NFR-U4-PERF-01 — Non-regression only (blocking)

Per Q4 A, **no new numeric target is set**. Header normalization is a handful of string operations run
once per upload, and the category ID index is one pass over a slice already loaded by
`categoryRepo.GetAll()` — it adds **no query**. Neither scales with row count, so a bespoke target would
measure something that cannot realistically fail.

Instead, re-run UOW-2's existing NFR-U2-PERF-01 100,000-row merge and record the median. It must remain
within the existing ten-second bound, under the same protocol: deterministic pre-state restored per run,
one warm-up excluded, median of at least five uninstrumented runs, backup I/O included.

That benchmark is the right instrument because the ID index and the per-row category resolution path sit
inside it. If anything in this unit costs measurable time, it shows up there.

## Reliability

### NFR-U4-REL-01 — No new mutation, no new failure mode

No mapping, category or transaction is written by any path this unit changes. A rejected upload mutates
nothing, exactly as before. Because nothing new is mutated, this unit introduces no new partial-failure
or rollback surface, and the UOW-2 admission gate, backup and merge semantics are untouched.

## Verification

Ownership follows the mandatory cross-provider rule: production authors none of the tests below, and the
independent provider owns them and the re-review.

### NFR-U4-TEST-01 — Header normalization property (blocking)

Property-based testing is configured **Partial**; header normalization is a natural fit because the
input space is small and precisely characterized. Using the existing test-only `pgregory.net/rapid`:

- For any canonical header decorated with arbitrary per-column case changes, arbitrary surrounding
  whitespace, and an optional leading UTF-8 BOM, matching **always succeeds**.
- For any header with a reordered column, an omitted column, an extra column, or an altered column name,
  matching **always fails**.

The two halves matter equally. The second is what proves Q2 A's promise that order and count remain
required — that tolerance did not quietly become permissiveness.

### NFR-U4-TEST-02 — Sanitization property (blocking)

For an arbitrary input string, including invalid UTF-8 and strings of control characters, the shared
helper's output must be at most 64 runes plus the ellipsis, contain no rune for which
`unicode.IsControl` reports true, and be valid UTF-8.

### NFR-U4-TEST-03 — The four observed failures (blocking)

Example-based coverage of every failure actually reproduced when U4-02 was admitted: a lowercase header,
a mixed-case header, a header with a space following a comma, and a header carrying a UTF-8 BOM. Each
must now import successfully. These are regression anchors for the specific complaints, distinct from
the generative property above.

### NFR-U4-TEST-04 — Diagnostic content (blocking)

- A renamed category yields a message naming both the unresolved value and the current name behind
  `Category_ID`, sufficient for a single find-and-replace.
- A case collision names the colliding categories, truncated at three with an accurate "and N more".
- Header failures name a position or a pair of counts, and contain no text from the file.
- No echoed name exceeds the bound, and no echoed name appears in any log record.

### NFR-U4-TEST-05 — Compatibility evidence (blocking)

Export output is byte-identical before and after this unit, and the existing UOW-2 and UOW-3 suites pass
unchanged, including the reviewer-owned tests that match on diagnostic codes.

### NFR-U4-TEST-06 — Race evidence

This unit adds no concurrency. The existing `-race -short` suite must continue to pass; no new
race-specific test is required.

## Scope

### NFR-U4-SCOPE-01 — Frozen scope holds

Per Q5 A, UTF-16 input detection is **declined for this unit** and recorded as a candidate finding
alongside deferred findings F-04 and F-05. It was never admitted before the 2026-09-06 freeze, it would
require a new diagnostic code that `domain-entities.md` forbids, and Excel's "CSV UTF-8" export writes
UTF-8 with a BOM — a case Q2 A already covers.

The cost of implementing it is not the criterion. This unit already had a restore capability struck from
it for arriving the same way, from an adjacent observation rather than from a finding.

## Amendments to Approved Artifacts

Q1 A and NFR-FQ1 A widen what a diagnostic may contain, which contradicts artifacts approved earlier the
same day. Each is amended explicitly and dated rather than reinterpreted.

| Artifact | Statement | Amendment |
|---|---|---|
| `business-rules.md` BR-U4-13 | Diagnostics "never echo arbitrary file content" | Amended: diagnostics may echo a file-supplied `Category_Name`, bounded and sanitized per NFR-U4-SEC-01. The word "arbitrary" now carries the weight — nothing unbounded or unsanitized is echoed |
| `domain-entities.md` invariant | "Diagnostics contain only values from this database and fixed text; never file content" | Amended identically, and corrected: database-sourced names are not inherently safe, because `category.name` has no length bound |
| UOW-2 `nfr-requirements.md` NFR-U2-SEC-01 | Diagnostics must not echo file content | Amended with a pointer to NFR-U4-SEC-01, which supplies the bounded exception. The logging and DOM-escaping clauses are unchanged and still binding |

## Traceability

| Source | Requirement |
|---|---|
| Q1 A, NFR-FQ1 A; BR-U4-11, BR-U4-12, BR-U4-13 | SEC-01 |
| Q2 A, Q3 A; BR-U4-07 | SEC-02 |
| NFR1–NFR4; project locality constraints | SEC-03 |
| BR-U4-06, BR-U4-19; `domain-entities.md` diagnostic codes | COMPAT-01 |
| Q1 B (Functional Design); BR-U4-08, BR-U4-10, BR-U4-17 | COMPAT-02 |
| Q4 A; UOW-2 NFR-U2-PERF-01 precedent | PERF-01 |
| BR-U4-17, BR-U4-18 | REL-01 |
| Extension: property-based testing (Partial); US-14 | TEST-01, TEST-02 |
| U4-02 reproduction evidence | TEST-03 |
| BR-U4-11, BR-U4-12, BR-U4-14 | TEST-04 |
| BR-U4-06, BR-U4-19 | TEST-05 |
| Cross-provider project convention | TEST-01 through TEST-06 ownership |
| Q5 A; 2026-09-06 scope freeze | SCOPE-01 |

## Extension Compliance

- **security-baseline**: opted out. Not enforced. NFR-U4-SEC-01 and SEC-02 are driven by the unit's own
  analysis, not by the extension.
- **resiliency-baseline**: opted out. Not enforced. N/A regardless — this unit adds no failure mode.
- **property-based testing (Partial)**: PBT-09 compliant, `pgregory.net/rapid v1.1.0` already in
  `go.mod`, no new dependency. PBT-02/03 are satisfied by NFR-U4-TEST-01 and TEST-02, which are the two
  places in this unit with a characterizable input space. PBT-07/08 carry into verification. PBT-01,
  04–06 and 10 remain advisory under Partial. No blocking PBT finding at this stage.

## Assigned to NFR Design

These are design decisions, not unanswered product questions:

- Where the shared sanitization helper lives, and how every diagnostic path is routed through it such
  that a future diagnostic cannot bypass it silently.
- The exact message wording for each of the five affected diagnostics.
- Whether the category ID index is built unconditionally or only when a row needs it.
- How the "and N more" count is computed when the diagnostic cap has already truncated the report.
