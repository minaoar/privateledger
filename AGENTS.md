# Repository Agent Instructions

Before planning, editing, reviewing, or testing:

1. Read `PROJECT_GUIDELINES.md` completely.
2. Read `aidlc-docs/aidlc-state.md` and the approved artifacts for the current stage/unit.
3. Follow the relevant `.aidlc-rule-details/` stage rules when the request uses AI-DLC.

`PROJECT_GUIDELINES.md` is the canonical source for durable project-specific conventions. Do not copy those conventions into reusable role skills; load them from that file.

For Code Generation, preserve the cross-provider ownership boundary:

- The production role modifies production files only.
- The independent review/test role modifies test files and its review artifact only.
- Production findings return to the production role.
- A unit does not pass its independent gate while required tests fail or Blocking/High findings remain.
