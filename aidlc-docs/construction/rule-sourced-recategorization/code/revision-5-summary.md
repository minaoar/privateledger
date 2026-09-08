# Production Revision 5 — UOW-5, Response to the Revision 4 Re-review

One finding, fixed. The full suite passes under `go test -count=1` and `go test -race -short`, and the
three generation tests pass at `-count=10`.

Production code only; one file changed, `internal/service/categorizer.go`.

## U5-R4-F01 — a failed atomic snapshot fell back to mixed-generation reads

**Fixed by deleting the fallback.**

Revision 4 replaced per-code resolution with one atomic `prepareMappings` read, and kept the per-code
path as a defensive fallback if that read failed. The finding is that the fallback reintroduces exactly
the defect the snapshot exists to remove: each `LookupCategory` call takes and releases the mapping
cache's lock independently, so a reload landing between two codes rebuilds the old/new mixture.

The reviewer's test drives it deterministically — initial staging succeeds, the pass-level staging read
fails, the first fallback lookup samples the old cache, the new cache is published, the second lookup
reads it — and the two transactions land in different categories 10 of 10.

**The comment production wrote above that fallback was the whole error:** *"A staging failure is not
fatal to the pass; fall through to the per-code path rather than categorizing against nothing."* That
reasoning treated "some answer" as better than "no answer". It is not, when the some-answer is silently
inconsistent and the operation reports success: the user sees two transactions governed by one mapping
change land in different categories, with nothing indicating the categories came from different rule
versions.

A source that can stage now either produces one generation or the pass does not run. `snapshotRules`
returns an error, `Reexamine` returns it before writing anything, and no transaction changes.

The per-code path remains only for a source that cannot stage at all, which has no atomic read to
offer. Nothing in production takes that path.

## Verification

`gofmt`, `go vet` and `go build ./...` clean.

- `go test -count=1 ./...` — every package passes.
- `go test -race -short -count=1 ./...` — every package passes.
- `TestReviewU5FailedPassSnapshotNeverFallsBackToSplitGeneration`,
  `TestReviewU5OnePassSeesOneRuleGeneration` and
  `TestReviewU5ShippedMappingReloadCannotSplitOnePass` at `-count=10`.
