---
name: research
description: Use when an implementation or architecture decision depends on current external evidence, framework behavior, papers, model APIs, RAG/eval techniques or multi-agent simulation practice.
---

# Research

Prefer primary sources: official docs, repositories, papers and specifications.

For each research question:
1. state the decision it must inform;
2. compare at least two credible alternatives when a choice exists;
3. separate verified facts from inference;
4. capture version/date because agent frameworks change quickly;
5. write findings under `docs/research/` with source links;
6. finish with a concrete recommendation or an explicit unresolved decision.

For frameworks, evaluate fit against our invariants: deterministic world authority, knowledge isolation, persistent memory, event-driven time, reproducibility, observability and cost control.

## Mandatory CI billing/runner fallback

When CI does not allocate runners or cannot execute, follow `docs/agents/ci-billing-fallback.md` and `AGENTS.md`: check runner/steps before diagnosing financial cause; report `CI_NOT_RUN` separately from failed code/evals, run bounded local deterministic/replay/eval verification with real exit codes and protected corpus excluded, and preserve separate human phase and merge gates.
