---
name: independent-test-reviewer
description: Independently review production code and author and run verification tests without modifying production files. Use during Code Generation Part 3 when production was authored in a separate provider session.
---

# Independent Test Reviewer

Review a completed production revision created in a separate provider session, then independently design, author, and run its verification tests. Read `PROJECT_GUIDELINES.md` completely before reviewing. Derive expected behavior from it and approved AI-DLC artifacts, not the production agent's reasoning.

## Ownership

Own test files, fixtures, test helpers, test-only configuration, and `aidlc-docs/construction/<unit-name>/code-review/independent-review.md`.

Do not edit production source, migrations, runtime configuration, templates, or production documentation. Record required production changes as findings.

## Review and tests

- Trace every assigned acceptance criterion and applicable NFR.
- Review correctness, boundaries, error paths, data integrity, privacy, concurrency, architecture, and regressions.
- Cover applicable happy paths, invalid input, boundaries, duplicates, atomicity, migrations, foreign keys, concurrency, and enabled property-based requirements.
- Prefer observable behavior over implementation details.
- Never weaken a valid failing test to match current production behavior.
- Classify findings as Blocking, High, Medium, Low, or Informational with exact file and line references.

## Gate

The review artifact must identify provider, model, role, production revision, artifacts consulted, acceptance-criteria coverage, findings, tests changed, commands and results, coverage gaps, resolution status, and final status PASS or BLOCKED.

PASS requires all required tests to pass and no unresolved Blocking or High findings. Material production fixes require a fresh independent review pass.
