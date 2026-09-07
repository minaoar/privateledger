# NFR Requirements Plan — UOW-4 Category Lifecycle and Mapping-File Integrity

## Stage Inputs

UOW-4 Functional Design approved 2026-09-07. UOW-1, UOW-2 and UOW-3 are complete with independent gates
PASS. Property-based testing remains configured **Partial**; security-baseline and resiliency-baseline
remain opted out.

Stage answers already binding on this unit: Q1 B (keep rejecting a stale name, with an actionable
message), Q2 A (normalize the header for BOM, whitespace and case; keep order and count required),
Q3 A (report case collisions rather than migrating or guessing), Q4 A (one story), FQ1 A (retitle US-14;
record U4-01 as mitigated).

Scope remains frozen at the 2026-09-06 admission of U4-01 and U4-02.

## The One Genuinely New NFR Surface

UOW-4 adds no table, endpoint, dependency or mutation, so most NFR surfaces are simply carried forward.
One thing is new, and it is a security surface rather than a performance one.

**A diagnostic message will now contain file-supplied text.** Every diagnostic this application produces
today carries only fixed text, a row number, a field name and a stable code. BR-U4-11 requires the
unresolved `Category_Name` — a value taken directly from an untrusted uploaded file — to appear in the
message.

That collides with NFR-U2-SEC-01, which says diagnostics must not echo file content, and with the
functional design's own BR-U4-13, which claims diagnostics "never echo arbitrary file content". Under
Q1 B they now do, by design, because that value is exactly what the user needs in order to repair the
file in one edit.

The collision is real and this is the stage to resolve it explicitly, rather than leaving it to be
discovered during code generation or by the reviewer.

### Verified magnitude

| Fact | Evidence |
|---|---|
| The CSV header has five columns | `sicMappingCSVHeader` in `internal/service/sic_mapping_service.go` |
| `Category_Name` is resolved, never stored, so no length validation applies to it | `resolveSICCategory`; no rune or byte cap exists on that field anywhere |
| A single CSV field may therefore be as large as the whole upload | `MaxSICMappingFileSize` = 10 MiB; no per-field bound |
| Up to 50 diagnostics are retained per report | `MaxSICMappingDiagnostics` = 50 |
| Categories are already fully loaded before row validation | `categoryRepo.GetAll()`, then `indexCategories` |

The last row matters: naming "the category `Category_ID` currently refers to" needs an index by ID over
a slice already in memory. It costs one extra pass over categories per upload and **no extra query**.

The other rows matter together: without a bound, a report could carry fifty copies of a multi-megabyte
field into an HTTP response and into the DOM. That is not a hypothesis about a malicious file — a
truncated or mis-delimited CSV produces one enormous field by accident.

## Carried-Forward Precedents

- UOW-2 NFR-U2-PERF-01: a 100,000-row merge within 10 s, including backup I/O, median of five
  uninstrumented runs after one warm-up.
- UOW-2 NFR-U2-SEC-01: local processing, escaped DOM text, no logging of upload contents.
- UOW-3 NFR-U3-PERF-02: import regression budget, two fixtures, ≤10 % median.
- Reference environment: Apple M1, 8 logical CPUs, 16 GiB RAM, macOS 15.5, go1.26.0, APPLE SSD AP1024Q
  NVMe.

## Open Questions

Answer each by replacing the `[Answer]:` tag. One option per question is marked **(Recommended)** with
the reason in italics below it. They are suggestions, not defaults — Q1 in the Functional Design stage
went against my recommendation and produced a better decision.

### Q1 — Bounding the echoed `Category_Name`

The value is user-controlled, unbounded, and now flows into a JSON response and the DOM.

- A. **(Recommended)** Cap the echoed value at a fixed rune length — 64 is enough to recognize a
  category name — appending an ellipsis when truncated, and replace control characters and line breaks
  before it is echoed. The value is never written to a log.
  *Preserves the entire point of Q1 B: 64 runes is far more than any real category name, so the user
  still gets the one edit they need. It bounds a 10 MiB × 50 worst case to a few KB, and stripping
  control characters means a crafted name cannot forge extra diagnostic lines in the rendered list.*
- B. Echo the value in full and rely on the existing 50-diagnostic cap.
  *The cap bounds the count, not the size. Fifty unbounded fields is not a bound.*
- C. Do not echo the file's value at all; report only the current name of the category `Category_ID`
  refers to.
  *Safest, but it removes the half of the message that identifies which row to fix, which is what makes
  the one-edit repair possible. It would weaken Q1 B to the point of not mitigating U4-01.*

[Answer]:

### Q2 — What the header diagnostic says

BR-U4-07 requires identifying the failing column without echoing header content. Two readings survive
that.

- A. **(Recommended)** Name the 1-based column position and the canonical name expected there, for
  example *"CSV header column 4 must be Category_Name"*. Nothing from the file is echoed.
  *Fully actionable — position plus expected name is everything needed to fix it — while keeping the
  header path free of the echo question entirely. Also survives the pathological case where the header
  itself is one enormous field.*
- B. Also echo the received column name, bounded and sanitized exactly as Q1 decides.
  *Marginally friendlier when the mismatch is a typo, at the cost of extending the echo surface to a
  second place for a case normalization already largely eliminates.*
- C. Keep the current single generic message.
  *Leaves U4-02's diagnostic half unfixed.*

[Answer]:

### Q3 — Whether column count and order failures are distinguished

A file with four columns and a file with five wrongly-ordered columns fail for different reasons, and
today both produce the same message.

- A. **(Recommended)** Distinguish them: a count mismatch reports the expected and received counts; an
  order or name mismatch reports the first failing position per Q2.
  *Both are one-line changes over the same normalized comparison, and they are the two distinct repairs
  a user makes. A count is a number, not file content, so it raises no echo question.*
- B. One message for any header failure, naming the first failing position only.
  *Simpler, but a file with a missing column gets a message pointing at a position that shifted, which
  reads as misleading rather than merely terse.*

[Answer]:

### Q4 — Performance requirement for this unit

Header normalization is a handful of string operations run once per upload. The ID index is one pass
over categories already in memory. Neither scales with row count.

- A. **(Recommended)** Set no new numeric performance target. Re-run UOW-2's NFR-U2-PERF-01 100,000-row
  merge as a non-regression check and record the figure.
  *A new target for work that is O(columns) once per upload would be ceremony. The merge benchmark is
  the one measurement that could plausibly move, because the ID index and the per-row resolution path
  sit inside it, and it already exists.*
- B. Add a numeric target for header normalization specifically.
  *Measures something that cannot realistically fail.*
- C. No performance verification for this unit at all.
  *Cheap to re-run an existing benchmark; skipping it forgoes the only evidence that the per-row path
  did not regress.*

[Answer]:

### Q5 — UTF-16 files

Stripping a UTF-8 BOM raises the neighbouring case. A UTF-16 file begins `FF FE` or `FE FF` and fails
today with a header diagnostic that gives no clue the encoding is the problem. Detecting those two byte
sequences is a few lines.

**This is out of the frozen scope.** U4-02 is the BOM/case/whitespace finding; UTF-16 was never
admitted. It also contradicts the approved `domain-entities.md`, which states no diagnostic code is
added. And the practical exposure is smaller than it first looks: Excel's "CSV UTF-8" writes UTF-8 with
a BOM, which Q2 A already covers; its UTF-16 output is tab-delimited `.txt`, not CSV.

- A. **(Recommended)** Decline it here. Record UTF-16 detection as a candidate finding for a future
  unit, alongside deferred findings F-04 and F-05.
  *I already made the scope error once in this unit — the restore capability came from my own adjacent
  observation rather than from the finding, and the user was right to strike it. Cheapness is not
  admission criteria. Recording it loses nothing.*
- B. Admit it into UOW-4 and add an `unsupported_encoding` diagnostic.
  *Turns an inscrutable failure into a clear one, but reopens a frozen scope and an approved artifact
  for a case Excel's CSV export does not actually produce.*

[Answer]:

## Execution Checklist

- [x] Confirm UOW-4 Functional Design approval and log it.
- [x] Re-verify the header, resolution and diagnostic code paths against the current tree.
- [x] Identify NFR surfaces genuinely new to this unit, and state the SEC-01 / BR-U4-11 collision.
- [ ] Receive answers to Q1 through Q5.
- [ ] Analyze answers for conflicts with approved requirements, prior NFRs and the frozen scope; raise
      follow-ups rather than resolving silently.
- [ ] Generate `nfr-requirements.md` and `tech-stack-decisions.md` under
      `aidlc-docs/construction/category-lifecycle-restore/nfr-requirements/`.
- [ ] If any answer amends an approved artifact, amend it explicitly and dated.
- [ ] Receive explicit NFR Requirements approval.

## Out of Scope

- Any finding not admitted before the 2026-09-06 scope freeze, including UTF-16 detection unless Q5
  admits it.
- A restore capability, restore semantics, and any deletion of mappings.
- Deferred independent findings F-04 and F-05.
- New production dependencies and schema changes. Infrastructure Design remains skipped.
- UOW-5, which remains registered and not started and requires an FR7/US-03 amendment first.
