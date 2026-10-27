---
name: orchestrate
description: Cost-aware Claude Code phase routing for authorized COTE Issue work and independent human-gated review; never auto-merge.
disable-model-invocation: true
---

# /orchestrate — COTE agent and model routing

Manual entry: `/orchestrate [#issue] [PHASE]`. This skill delegates the authorized **current phase** to native Claude Code subagents. It does not replace `.agents/skills/delivery-pipeline/SKILL.md`, `docs/agents/implementation-pipeline.md`, or accepted ADRs. No automatic phase transitions, autonomous fleet or scheduler are enabled by this change.

## Selection

1. If Issue/PR/phase is supplied, respect it, inspect existing GitHub checkpoint, branch/HEAD, current findings and worktree; do not substitute a new task. If no phase is given, perform read-only RESUME/status and recommend the next phase, without edits.
2. Without a named task, inspect open Issues/PRs and prefer ready approved already-in-progress work, excluding blocked work, unaccepted canonical choices and conflicting migration/replay contracts. An open Issue alone does not imply permission. Use `to-tickets` for approved large specs only and propose up to three vertical pieces without duplication.
3. Load `CONTEXT.md`, applicable accepted ADRs, `AGENTS.md`, domain skill for knowledge/memory/exam/fidelity and `docs/agents/ci-billing-fallback.md`. Keep literary canon claims `VERIFIED`/`INFERRED`/`INTERPRETATION`/`UNVERIFIED` distinct. Never send private full novels to providers.

## Native Claude delegation

The coordinator uses the Claude Code Agent tool to invoke the exact role in `.claude/agents/cote-*.md`, each with `model`, `effort` and tool allowlist. The current interactive parent model does not change; verify requested vs effective model/effort on supervised pilot with `/tasks`. Environment/account quota and permissions still apply. If delegation tool/model unavailable or substituted, STOP with BLOCKED; do not fake switch or evade quota.

| Authorized activity | Subagent | Model/effort |
|---|---|---|
| Issue/PR/log reconnaissance and handoff | `cote-route-status` | Haiku Medium |
| Tickets and structured planning | `cote-ticket-planner` | Sonnet Medium |
| Primary implementation or complex regressions | `cote-implementer` | Sonnet High |
| Deterministic tests, suites/evals, documentation | `cote-validator` | Sonnet Medium |
| Independent INITIAL security/semantic/spec review | `cote-review-initial` | Sonnet High |
| Fix frozen OPEN findings | `cote-remediator` | Sonnet High |
| POST-REMEDIATION known-finding closure | `cote-review-closure` | Sonnet Medium |
| Exceptional cross-domain normative architecture | `cote-architecture-escalation` | Opus High |

Opus may be invoked once for an evidenced unresolved ADR decision spanning >=4 independent domains and is strictly read-only. No automatic XHigh; get explicit human authorization. Never use Opus to poll logs, run tests, read routine PDFs or replace ordinary Sonnet review.

## Strict existing phase contract

Read `delivery-pipeline`. PLAN/TICKETS -> human authorized IMPLEMENT -> **stop** at code checkpoint; independent REVIEW -> Draft PR and `<!-- cote-review-finding:v1 -->` records, **stop**; independently authorized REMEDIATE -> frozen OPEN findings and focused RED/GREEN, **stop**; separately authorized REVIEW_AGAIN -> verify closure and regression. No more than four review cycles without explicit renewed user authorization, and never treat zero LLM findings as human release gate. Implementer and reviewer never share a context or execute writes concurrently.

World truth and exam scoring belong to deterministic engine; agent memories/beliefs must be causal and character-specific. Check deterministic replay, genesis/fork/ledger identities, no leaked future/canon information, tested exam trajectories, provenance and stochastic evaluation versioning. Stop on unresolved canonical ambiguity; do not hallucinate verified evidence.

If GitHub Actions lacks runner or billing, follow `docs/agents/ci-billing-fallback.md`: `CI_NOT_RUN` versus real failure, only trusted local tests with exit codes, no protected corpus sent outside, no fake green. On quota, save Git-first WIP and stop. End each invocation with Issue, phase, PR/branch/HEAD, tests/evals run and not run, remaining findings and **next phase requiring separate human authorization**. No merge, deploy or live simulation rollout.
