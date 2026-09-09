---
name: code-review
description: Use after primary implementation is committed to independently review a COTE simulation branch/PR for spec fidelity, engineering quality, and simulation invariants. Publish each finding separately; do not fix code.
---

# Code Review

Use this skill **after `implement` has produced a committed implementation checkpoint**. This skill reviews; it does not modify production code.

This skill is one workflow phase. It may inspect the complete PR and persist findings, but it must not start remediation on its own.

## Establish the review surface

1. Identify the feature branch, fixed comparison base, originating ticket/spec, relevant ADRs, `CONTEXT.md`, architecture docs, and matching domain skills.
2. Require the implementation to be committed. Do not review an accidental mixture of unrelated uncommitted changes.
3. Ensure the feature branch is available remotely.
4. Reuse the Draft PR for this branch if one exists; otherwise create a Draft PR against the intended base branch. The PR is the persistent handoff surface between implementation, review, and remediation.
5. Review the complete PR diff, not only the latest commit.
6. Read existing marked findings before publishing new ones. Do not duplicate a materially identical finding that is already `OPEN`.

## Axis A — Spec fidelity

Check for:
- missing or partially implemented acceptance criteria;
- behavior contradicting the accepted ticket/spec;
- invented scope or accidental inclusion of explicit out-of-scope work;
- violated dependency assumptions;
- tests that claim behavior the spec does not authorize.

## Axis B — Engineering quality

Check for:
- unclear state ownership or duplicated/scattered domain logic;
- abstractions without leverage;
- provider/framework coupling that should sit behind adapters;
- implementation details leaking through interfaces;
- false-green, tautological, brittle, or statistically invalid tests;
- silent overwrite/data-loss paths;
- hidden nondeterminism or unreproducible RNG;
- unsafe parameter/schema guards whose prose promises more than the code enforces;
- unnecessary speculative generality or unrelated refactors.

## Axis C — Simulation invariants

Check especially:
- world truth vs belief/perception separation;
- secret information leakage;
- LLM deciding deterministic rules or legal state transitions;
- canon events or future knowledge leaking after divergence;
- stochastic runs lacking reproducible substreams or provenance;
- observer visibility becoming character knowledge;
- character intelligence granting hidden information;
- raw copyrighted corpus entering the public repository.

Repository ADRs and accepted specs override generic preferences.

## Severity

Use: `Blocker`, `High`, `Medium`, `Low`.

## Persist every finding in the Draft PR

Publish **each actionable finding as its own top-level PR conversation comment**. Do not bundle multiple findings into one message.

Each finding comment must use this machine-readable marker and structure:

```markdown
<!-- cote-review-finding:v1 -->
### Finding N — <Severity> — <short title>
**Status:** OPEN
**Ticket:** <ticket id>
**Location:** <file:line or behavior>
**Evidence:** <reproduction/spec evidence>
**Impact:** <why it matters>
**Smallest safe correction:** <minimal correction>
```

If a review axis is clean, say so in the review summary. If there are no findings at all, publish one clean-review summary comment; do not invent work merely to populate the PR.

## Stop condition

After all findings and the review summary are persisted:

1. **STOP.**
2. Do not edit production code or tests to remediate findings.
3. Do not invoke `address-review-findings` or `implement`.
4. Do not spawn a remediation subagent or start another review/remediation cycle.
5. Do not approve, mark ready, or merge the PR.
6. Report the review result and tell the user that `$address-review-findings <ticket>` is the separate next phase when actionable findings exist.

A re-review is also a separate user-triggered invocation of this skill. Phase transitions are never automatic.