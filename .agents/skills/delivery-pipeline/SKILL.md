---
name: delivery-pipeline
description: Coordinate planning and handoffs for the COTE implementation/review/remediation pipeline without crossing explicitly separated skill phase boundaries.
disable-model-invocation: true
---

# Delivery Pipeline — coordination and recovery

Read `AGENTS.md`, `CONTEXT.md`, the relevant Issue/spec and Accepted ADRs, plus `docs/agents/implementation-pipeline.md` in full before acting. This is a **coordinator**, not a replacement for accepted project skills or a license to merge.

## At invocation

1. Identify Issue, base branch, feature branch/HEAD and open Draft PR if one exists; use GitHub as durable state, never reconstruct work from a chat transcript.
2. Classify exactly one requested phase: `PLAN`, `IMPLEMENT`, `REVIEW`, `REMEDIATE`, `REVIEW_AGAIN`, `RELEASE_GATE`, `RESUME`, or `PARALLEL_PLAN`. If missing, inspect persisted state and **report** the next allowed phase; never silently broaden the user's request.
3. Read acceptance criteria and unresolved decisions; map layer/seam/tests/owners, including negative cases. Do not invent decisions or mark work done from stale docs.
4. Consult the canonical skill owning the chosen phase. Follow its stop condition even if the coordinator's longer plan includes later phases.

## Stage contracts

- `PLAN`: route unresolved questions through `to-spec` / `to-tickets` / domain research; return issue-level vertical slices and dependencies.
- `IMPLEMENT`: use canonical `implement` (and `tdd` where applicable); capture real RED when practical, immediate focused GREEN, comprehensive relevant checks, `git diff --check`, committed/pushed checkpoint and exact exit codes.
- `REVIEW`: independent inspection of *persisted* full PR diff against fixed base + spec; persist findings in the repository's already-established format, one actionable finding per record.
- `REMEDIATE`: load and freeze OPEN findings at start; reproduce, add RED regression, fix, run targeted + broader tests; create a separate fix commit and record resolution SHA/status for each.
- `REVIEW_AGAIN`: independent check of closures, changed delta and real regressions; do not infer zero findings from passing CI.
- `RELEASE_GATE`: differentiate test green, operational proof and explicit human acceptance. Absence of required evidence is `NO-GO`.
- `RESUME`: load current branch/PR/Issue and last persisted checkpoint. Do not reimplement committed slices after disconnection or quota failure. Publish a fresh, honest handoff.
- `PARALLEL_PLAN`: select only disjoint ready Issues, show file/contract/DB overlap and conflict risk; one worktree + branch per Issue, never concurrent changes to shared governance files. Parallel execution requires user authorization and available agent capability.

## Bounded convergence

Suggest up to four independent review/remediation cycles only **when requested and permitted by the repository**. Do not hide open findings to claim convergence; escalate contract/ADR changes, privacy/data-loss concerns, architectural redesign, unsupported environment and unresolved risk. Never merge, mark ready or deploy without explicit authority.

## Safety and evidence

- Always treat negative-control mutation as a temporary operation in a separate disposable worktree/copy; never mutate the dirty checkout or run destructive `git checkout --` / stash-based automatic restore.
- Publish GitHub comment bodies from a UTF-8 file via `gh ... --body-file`, verify round-trip; protect original contents before editing an existing comment. Avoid fragile heredocs and Windows path conversion.
- A check is `PASSOU`, `FALHOU`, `NÃO EXECUTADO`, or `MANUAL PENDENTE`; never equate adapter tests with real GUI/production/user approval.
- Never claim a Draft PR exists, a test passed, a hook executed, an external effect occurred or a file was created without current-session evidence.

## Repository-specific limits

MANDATORY: This coordinator does NOT invoke `implement`, `code-review`, `address-review-findings` or their next phases automatically. Each one is a separate USER-TRIGGERED invocation, and each stops before the next. In particular, `implement` commits/pushes but does NOT open a Draft PR; `code-review` creates/reuses it. `address-review-findings` freezes previously persisted OPEN findings and stops after pushing remediation. Preserve `<!-- cote-review-finding:v1 -->` on every finding. For deterministic engine state/commit/replay/knowledge boundaries, require actual tests and stochastic/versioned evals as specified; do not let LLM fill missing world truth.

## Required end-of-invocation report

Return: Issue; phase; base/branch/HEAD/PR; accepted spec; completed slice(s); RED evidence or N/A; focused/full test commands and exit codes; mutation evidence if used; finding counts/status IDs; manual/operational gaps; next explicitly permitted stage. STOP at the canonical skill's boundary.

See `docs/agents/implementation-pipeline.md` for gates, parallel worktree selection, crash-safe negative controls and handoff schema.

## Mandatory CI billing/runner fallback

When CI does not allocate runners or cannot execute, follow `docs/agents/ci-billing-fallback.md` and `AGENTS.md`: check runner/steps before diagnosing financial cause; report `CI_NOT_RUN` separately from failed code/evals, run bounded local deterministic/replay/eval verification with real exit codes and protected corpus excluded, and preserve separate human phase and merge gates.
