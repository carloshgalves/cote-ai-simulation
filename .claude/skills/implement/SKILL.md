---
name: implement
description: Use for the first implementation of an accepted COTE simulation spec/ticket; build and validate the change, then checkpoint it in Git. Do not use for review-finding remediation.
---

# Implement

Use this skill for **primary implementation only**. Do not inspect pull-request review comments or try to repair prior review findings here; `address-review-findings` owns that phase.

This skill is one workflow phase. It must not start the next phase on its own.

## Before editing

1. Read the originating spec/ticket, `CONTEXT.md`, relevant ADRs, architecture docs, and matching domain skills.
2. Work on a dedicated feature branch for the ticket. Never implement directly on the default branch.
3. Inspect the affected code and tests and identify deterministic seams, simulation invariants, and required evals.
4. If the ticket still requires a product/domain decision that is not already accepted, stop that branch of work instead of guessing.

## Implementation loop

1. Establish a focused failing or verification signal when practical.
2. Implement the smallest coherent vertical slice.
3. Use `tdd` for deterministic behavior that benefits from red-green development.
4. Keep provider/framework code behind adapters.
5. Preserve world-truth/belief separation, knowledge boundaries, deterministic authority, provenance, and reproducible stochastic substreams.
6. Add traces/metadata required to replay or audit model-dependent runs.
7. Run focused tests as the slice grows, then the broader relevant suite.
8. Run applicable `llm-evals`, `rag-evals`, `knowledge-boundary-audit`, `character-fidelity`, or `simulation-evals` checks.
9. Inspect the final diff for unrelated changes and run `git diff --check`.

Do not “fix” failing deterministic tests by relaxing rules into prompts.

## Git checkpoint is mandatory

When the primary implementation and its validation are complete:

1. commit the implementation on the feature branch **before ending the task**;
2. use a ticket-scoped commit message;
3. treat the commit as the review checkpoint, not as approval to merge;
4. push the feature branch when remote credentials are available so an independent reviewer can inspect the exact commit.

Do not wait for code review before committing. Review findings, if any, belong in later commits produced by `address-review-findings`.

## Stop condition

After the validated implementation is committed and, when possible, pushed:

1. **STOP.**
2. Do not invoke `code-review` or `address-review-findings`.
3. Do not spawn a reviewer/subagent whose purpose is to perform the next workflow phase.
4. Do not inspect PR review comments or open a review cycle.
5. Report the implementation checkpoint and tell the user that `$code-review <ticket>` is the separate next phase if they want to run it.

Do not open, approve, mark ready, or merge a pull request from this skill. `code-review` owns the review surface and `address-review-findings` owns remediation. A phase transition requires a separate user-triggered skill invocation.