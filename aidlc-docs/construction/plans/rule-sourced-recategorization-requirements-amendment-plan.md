# Requirements and Story Amendment Plan — UOW-5 Rule-Sourced Recategorization

## Stage Inputs

UOW-4 complete with independent gate PASS, 2026-09-07. UOW-1 through UOW-4 all complete.

This is the first stage of UOW-5 and it is unusual: it amends **approved Inception artifacts** before any
design work. That is deliberate. FR7 says in terms what this unit would reverse, so proceeding to design
first would mean designing against a requirement that still forbids the outcome.

## What This Unit Reverses

FR7 — *Preserve manual categorization* — currently reads:

> - Transactions with `category_source = 2` are excluded from automatic categorization changes.
> - Creating or updating SIC mappings only re-categorizes currently uncategorized transactions.
> - **Existing rule-based categorizations are not changed by SIC mapping creation.**

The third bullet is the sentence this unit would strike. The first is not in question and will not
change: **manual stays manual, always.**

US-03's second acceptance criterion says the same thing and would change with it.

## The Constraint That Shapes Every Answer

**A transaction does not record which rule categorized it.** `category_source` is `0=none, 1=rule,
2=manual` and no column identifies the responsible pattern or mapping. So "follow the rule that
categorized it" is not directly implementable as stated.

This is why R1 is the question it is. "Follow the rule that categorized it" and "re-examine against the
rules as they now stand" produce the same answer in almost every case, and they are not the same
promise. Only the second is achievable without recording something the system has never recorded, and
only the first is what the sentence literally says.

R1 asks which promise to make. **How** it is built is Functional Design's decision.

## Verified Starting State

Re-confirmed against the current tree on 2026-09-07.

| Observation | Evidence |
|---|---|
| Rule-sourced rows are excluded from recategorization | `GetUncategorizedBySICCodes` requires `category_source = 0 AND category_id IS NULL` |
| The same exclusion exists in the decision function | `Categorizer.decide` stops on a manual source or an existing category |
| UI edit and CSV import behave identically | Verified during UOW-3; both route through the same collaborator |
| No column identifies the responsible rule | `schema.sql` `ledger_transaction`; `category_source` CHECK constraint |
| The rule is encoded in five approved places | FR7, US-03, BR-U3-03, BR-U2-29 through BR-U2-31, NFR-U3-REL-01 |

## Scope Correction — 2026-09-07

The first draft of this plan asked, as Q1, whether to re-run the decision function or add a column
recording which rule categorized each row. **That is a mechanism question, and it does not belong in a
requirements stage.** It also overlapped Q2 and Q3: one of its options was "SIC mappings only", which
Q3 already asked.

The questions below are restated as **observable behaviour** — what a user can see happen. How it is
built is Functional Design's decision, made against whatever these answers settle.

Where a promise has an unavoidable structural cost, that cost is stated as a consequence rather than
offered as a choice, because it changes what you are agreeing to.

## Open Questions

Answer each by replacing the `[Answer]:` tag. One option per question is marked **(Recommended)** with
the reason in italics. They are suggestions — UOW-4's Q1 went against my recommendation and produced the
better decision.

### R1 — When a rule changes, which rule-sourced transactions are re-examined?

- A. **(Recommended)** All of them. Any rule change re-examines every rule-sourced transaction, and each
  keeps, changes or loses its category according to what the rules now say. A transaction categorized by
  a text pattern may therefore be re-examined when a SIC mapping changes — and keeps its category,
  because its pattern still matches.
  *One rule for everything, and no transaction can sit in a category the current rules do not support.
  The strangeness is bounded: being re-examined is not the same as being changed, and a row whose rule
  still matches is untouched.*
- B. Only transactions the changed rule was responsible for.
  *Exactly what "follow the rule that categorized it" says, and it never re-examines an unrelated row.
  **Unavoidable consequence:** nothing records which rule categorized a transaction today, so this
  requires storing rule identity going forward and deciding what to do about every transaction already
  categorized, whose responsible rule is now unknowable. That decision cannot be avoided by
  implementation cleverness — the information does not exist.*
- C. Only transactions whose SIC code appears in the changed mapping.
  *Cheaper and never re-examines a pattern-matched row. But a transaction categorized by a pattern that
  no longer exists stays wrong permanently, and the promise becomes "mapping changes propagate, pattern
  changes do not", which is harder to explain than either A or B.*

[Answer]:I will go with A. Also what will happen when a new rule is created (through CSV mapping or through pattern matching)? According to A, will a new rule also trigger re-examination of all existing categorized and uncategorized transactions?

### R1a — Follow-up: does *creating* a rule also trigger re-examination?

Raised by the user when answering R1: *"what will happen when a new rule is created (through CSV mapping
or through pattern matching)? According to A, will a new rule also trigger re-examination of all existing
categorized and uncategorized transactions?"*

**Under R1 A as written, yes — a creation is a rule change.** The question found something R1 A's own
wording softened, so the full picture is set out here before it is settled.

**Uncategorized transactions: nothing changes.** They are already re-examined today when a mapping is
created and when a pattern is added (`RecategorizeByCategory`). This unit does not alter that.

**Rule-sourced transactions: the effect is sharply asymmetric**, because verified against the tree,
`Categorizer.decide` evaluates text patterns **before** SIC mappings, and `sic_mapping.sic_code` is
`UNIQUE`.

| New rule | Effect on already-categorized rule-sourced transactions |
|---|---|
| **New SIC mapping** | Small. A new mapping necessarily covers a code no mapping covered before, since the code is unique. Pattern-categorized transactions keep their category, because patterns outrank mappings. The only ones that move are those whose pattern has since been deleted — which is the staleness this unit exists to fix |
| **New text pattern** | **Significant.** Patterns outrank mappings, so a new pattern can pull transactions *away* from categories a SIC mapping assigned. Today a new pattern only claims uncategorized transactions |

The second row is the real content of the question, and R1 A's blurb gave only the reassuring direction —
that a pattern-categorized transaction keeps its category when a mapping changes. The reverse is the
sharper edge and was not stated.

- A. **(Recommended)** Yes. Creating a rule triggers re-examination exactly as changing or deleting one
  does.
  *The alternative reintroduces precisely the history-dependence this unit exists to remove: whether a
  transaction ends up in a pattern's category would depend on whether the pattern existed before or
  after the mapping categorized it. Two users with identical rules would see different categories
  because of the order they happened to create them in. Under A, the current rules always determine the
  current categories, and that is a sentence that can be explained in full.*
- B. No. Creation applies only to transactions nothing else has claimed; only modification and deletion
  re-examine rule-sourced transactions.
  *Much smaller blast radius for what is a common, casual action — adding a pattern would never disturb
  existing categorizations. The cost is the order-dependence above, and a rule set that no longer fully
  determines the outcome.*
- C. Yes for mappings, no for patterns.
  *Targets exactly the asymmetric case. But it makes the two rule types behave differently for no reason
  a user could infer, which is the objection that ruled out R3 B.*

**Interaction worth noting:** if R4 is answered B — an explicit "Reapply rules" action rather than
automatic — the blast-radius concern behind B and C largely dissolves, because no re-examination happens
until you ask for it. These two questions are worth deciding together.

[Answer]:A

**The principle behind the answer, in the user's words, 2026-09-07:**

> I want the same rule book to create the same transaction categorization, irrespective of when the
> rules were created.

This is stronger and more useful than the reasoning offered with option A, which argued from the
awkwardness of order-dependence. Stated positively it is a **determinism principle**, and it settles
questions this plan never asked:

**Categorization is a function of the current rule set and the user's manual choices. It is not a
function of rule history.** Two databases with identical transactions, identical rules and identical
manual assignments must categorize identically, whatever order the rules were created in, and whatever
order the transactions arrived in.

Recorded in `requirements.md` as **FR15**, so it governs future work rather than living only in this
unit's plan.

### R2 — When a re-examined transaction matches no rule at all

Under R1 A or C this transaction has a category only because of a rule that no longer says so.

- A. **(Recommended)** It becomes uncategorized, and the result reports that count separately from
  transactions that moved to a different category.
  *The category is unsupported by any current rule and was never chosen by a human. Keeping it preserves
  exactly the staleness this unit exists to end. These transactions are not lost — the uncategorized
  dashboard is where they surface, and that is the screen built for deciding what to do with them.*
- B. It keeps its current category.
  *Never removes a category, which matters if you would rather see a stale category than none. The cost
  is that the unit half-solves the problem: rules propagate when they have somewhere to move a
  transaction, and silently do not when they do not.*
- C. It keeps its category and is marked manual, protecting it from then on.
  *Records you as having chosen a category you never chose, which corrupts the one signal the system
  treats as authoritative.*

**Worth your scrutiny.** This is the only decision here that can take a category away from a transaction,
and you have said before that you dislike deletion in principle. I recommend A, and B is a coherent
position rather than a wrong one — it trades completeness for never removing anything.

[Answer]:A. Because the prior rule is changing and everything which is rule based, should remain rule based. Keeping it in the old category means it becomes almost like a manual categorization. 

### R3 — Which kinds of rule change trigger this?

- A. **(Recommended)** Both text-pattern changes and SIC mapping changes.
  *A user editing a pattern and a user editing a mapping are doing the same thing — changing how
  categorization works — and there is no principle that would justify different behaviour. Splitting
  them means explaining an asymmetry that has no reason behind it.*
- B. SIC mapping changes only.
  *Matches the observation that prompted this unit and touches less. The asymmetry above remains, and
  closing it later is another unit.*

[Answer]:A

### R4 — Does it happen automatically, or when you ask for it?

- A. **(Recommended)** Automatically, whenever a rule changes — the same way a mapping change already
  recategorizes uncategorized transactions today.
  *Consistent with what the application already does. A rule edit already rewrites transactions; this
  widens which ones. Nothing goes stale by inaction.*
- B. An explicit "Reapply rules" action you trigger.
  *Makes a bulk rewrite deliberate, which is defensible now that more rows can change. The cost is a
  split model — rule edits take effect immediately for uncategorized rows and only on request for
  rule-sourced ones — and a user who never presses it keeps stale categories indefinitely.*
- C. Automatic, with a confirmation when more than N transactions would change.
  *N is arbitrary, and the prompt appears at the least predictable moment.*

[Answer]:A

### R5 — What the result tells you afterwards

Today's result reports what changed and nothing about what did not. That gap exists regardless of this
unit.

- A. **(Recommended)** Three counts: moved to a different category, became uncategorized, and left alone
  because they are manual.
  *The manual count answers the question anyone actually has after a bulk rewrite — "did it touch the
  ones I set by hand?" — and answering it with a number requires no list.*
- B. The two changed counts only.
  *Leaves the manual-protection question unanswered precisely when it matters most.*
- C. Per-transaction detail.
  *A bulk operation can affect thousands of rows, and the uncategorized dashboard already lists the ones
  that became uncategorized.*

[Answer]:A

### R6 — Transactions already categorized before this ships

Every transaction currently marked as rule-sourced was categorized under the old behaviour. The first
rule change after this ships re-examines all of them at once.

- A. **(Recommended)** Accept it. The first run is simply the new behaviour applying for the first time,
  and R5's counts make its size visible in the result.
  *Any alternative needs a marker separating "categorized before UOW-5" from after — which is the same
  missing information R1 B runs into.*
- B. Treat pre-existing rule-sourced transactions as manual, freezing them permanently.
  *Records choices you never made, and the stale categories this unit targets are mostly in exactly
  those transactions.*

[Answer]:A

## Execution Checklist

- [x] Confirm UOW-4 is complete with gate PASS before starting UOW-5.
- [x] Re-verify the exclusion behaviour and the missing-rule-identity constraint against the tree.
- [x] Restate the questions as observable behaviour after the user asked whether the requirement should
      be finalized first. The original Q1 asked for a mechanism and overlapped Q2 and Q3.
- [ ] Receive answers to R1 through R6, and to follow-up R1a raised by the user at R1.
- [ ] Analyze answers for conflicts with approved artifacts; raise follow-ups rather than resolving
      silently.
- [ ] Amend `requirements.md` FR7 and `stories.md` US-03, dated and marked as UOW-5 amendments.
- [ ] Define UOW-5 stories and promote them into `unit-of-work.md` and `unit-of-work-story-map.md`.
- [ ] Identify every downstream artifact the amendment invalidates — BR-U3-03, BR-U2-29 through
      BR-U2-31, NFR-U3-REL-01 at minimum — and amend each explicitly.
- [ ] Receive explicit approval before Functional Design.

## Out of Scope

- Weakening manual protection. `category_source = 2` stays excluded under every option above.
- Candidate findings C4-01, C4-02 and C4-03.
- Deferred independent findings F-04 and F-05.
- New dependencies. A schema change is in scope only if Q1 B is chosen.
