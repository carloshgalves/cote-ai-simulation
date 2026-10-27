---
name: tdd
description: Use for deterministic simulation rules, exam scoring, visibility filters, event scheduling, snapshots and other behavior that can be tested without an LLM.
---

# TDD

Use red → green → refactor on deterministic seams.

Prioritize tests for:
- exam scoring and deadlines;
- action validation;
- secret/role disclosure;
- event ordering and logical time;
- snapshot restore/replay;
- provenance/visibility filters;
- promotion/demotion of NPC simulation tiers.

Do not demand exact string outputs from stochastic LLM calls. Those belong in eval suites with rubrics/thresholds.

## Mandatory CI billing/runner fallback

When CI does not allocate runners or cannot execute, follow `docs/agents/ci-billing-fallback.md` and `AGENTS.md`: check runner/steps before diagnosing financial cause; report `CI_NOT_RUN` separately from failed code/evals, run bounded local deterministic/replay/eval verification with real exit codes and protected corpus excluded, and preserve separate human phase and merge gates.
