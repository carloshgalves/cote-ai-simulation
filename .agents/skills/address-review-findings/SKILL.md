---
name: address-review-findings
description: Use only when a Draft PR already has review findings. Read the persisted findings, validate and fix the actionable ones, run regression coverage, then commit and push the remediation.
---

# Address Review Findings

Use this skill **only because review findings already exist**. Do not perform an open-ended review and do not re-run primary implementation from scratch.

The Draft PR is the handoff surface. If the expected PR or marked findings cannot be loaded, stop and report the broken handoff instead of guessing what the reviewer meant.

## Load the review contract

1. Identify the feature branch and its Draft PR.
2. Read the originating ticket/spec, relevant ADRs, `CONTEXT.md`, architecture docs, and the current PR diff.
3. Load every PR conversation comment marked `<!-- cote-review-finding:v1 -->` whose status is `OPEN`.
4. Process findings by severity, highest first. Work on one finding at a time unless two findings share the same root cause and one coherent correction closes both.

## For each finding

1. Verify the evidence against the current branch and accepted spec. A finding is not automatically correct merely because a reviewer wrote it.
2. If valid, reproduce it at the highest stable seam and add or tighten a regression test when practical.
3. Apply the smallest safe correction; do not expand the ticket's product/domain scope.
4. Run focused validation immediately.
5. If the finding is invalid, document why and mark it `DISMISSED` rather than changing correct code.
6. If an accepted plan explicitly defers it to another ticket, mark it `DEFERRED` and name that ticket. Do not use deferral to avoid an in-scope fix.

Use `tdd` when a deterministic finding benefits from red-green repair.

## Full validation

After actionable findings are addressed:

- run the broader relevant test/eval suite;
- run `git diff --check`;
- inspect the remediation diff for unrelated changes;
- preserve deterministic authority, knowledge boundaries, provenance, and reproducibility.

## Commit and push the remediation

When remediation is green:

1. commit the corrections on the **same feature branch** as a new review-fix checkpoint;
2. push the branch so the Draft PR updates;
3. do not squash away the original implementation checkpoint during review;
4. do not merge or mark the PR ready from this skill.

## Close the handoff loop

Update each original marked finding comment after the correction commit:

- `**Status:** RESOLVED` plus the correction commit SHA for fixed findings;
- `**Status:** DISMISSED` plus the evidence/rationale for invalid findings;
- `**Status:** DEFERRED` plus the explicit target ticket and accepted reason for deferred findings.

Prefer updating the original comment so machine-readable status remains authoritative. If tooling cannot edit that comment, reply to it with the resolution and leave a clear note that the original marker could not be updated.

After this skill finishes, run `code-review` again in an independent session against the complete PR diff.
