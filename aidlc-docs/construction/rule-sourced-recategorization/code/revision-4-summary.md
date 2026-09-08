# Production Revision 4 — UOW-5, Response to the Revision 2/3 Re-review

All three open findings are fixed. **The full suite passes**, under `go test -count=1` and
`go test -race -short`, with no failing or hanging test in any package.

Production code only; no test file was created or modified.

## U5-R-F01 — the mapping snapshot was assembled across generations

**Fixed, and the reviewer unblocked it.**

Revision 3 built `sicByCode` with one `LookupCategory` call per distinct code. Each call takes and
releases the mapping cache's own lock, so a reload landing between two codes yields a map assembled from
two generations. The strengthened two-code test caught it 10 of 10.

Production had reported this as unfixable given the probes, because an atomic implementation reads the
index through `prepareMappings` and never calls `LookupCategory` — which made both generation tests hang
on channels they closed inside the lookup. **The reviewer moved the probes** to pause after either the
live lookup or the second prepared snapshot, which removed the obstacle without weakening either test.

`snapshotRules` now takes one atomic read of the whole mapping index when the source can stage one, and
keeps the per-code path only as a fallback for a lookup that cannot. Both generation tests pass at
`-count=10`.

Worth recording plainly: the previous revision shipped a design production knew could not satisfy
BR-U5-10, in order to keep one more test green. That was the wrong trade — a design that cannot meet an
approved rule should not be shipped to protect a passing test.

## U5-R-F03 — mapping warning branches hid the counts

**Fixed.** Mapping save and delete showed `describeRuleChangeCounts` only on full success; the warning
branches returned after rendering the warning and the durable-state line.

Both branches now lead with the counts, then the warnings, then the do-not-retry line. A warning branch
is exactly when the counts matter most: the user needs to know what the change did before the follow-up
failed, and reporting only the failure leaves them unable to tell whether anything moved.

The factual, non-retry wording is unchanged.

## U5-R3-F01 — Categories invented a zero source breakdown

**Fixed.** `describeReexamination` printed the pattern/SIC split unconditionally, defaulting missing
fields to zero. Rule-change responses carry the three FR16 counts without the split, so the page showed
`1 moved to a different category (0 by pattern, 0 by SIC mapping)` — an explanation contradicting the
number it explains.

The split now renders only when the response actually contains both fields. Nothing is synthesized.

## Verification

`gofmt`, `go vet` and `go build ./...` clean.

`go test -count=1 ./...` and `go test -race -short -count=1 ./...` both pass in **every** package. The
two generation tests were additionally run at `-count=10`.
