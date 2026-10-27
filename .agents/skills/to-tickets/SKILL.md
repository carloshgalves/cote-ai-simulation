---
name: to-tickets
description: Use to split an accepted spec into small tracer-bullet implementation tickets with explicit dependencies and validation.
---

# To Tickets

Break work into vertical slices that produce observable behavior.

Each ticket states:
- outcome;
- files/modules likely touched;
- dependency/blocking edges;
- deterministic tests;
- model/RAG evals when applicable;
- knowledge-boundary checks when applicable;
- completion evidence.

Prefer a slice like “run one turn of one exam with isolated knowledge and deterministic scoring” over separate tickets for every repository layer.

## Mandatory CI billing/runner fallback

When CI does not allocate runners or cannot execute, follow `docs/agents/ci-billing-fallback.md` and `AGENTS.md`: check runner/steps before diagnosing financial cause; report `CI_NOT_RUN` separately from failed code/evals, run bounded local deterministic/replay/eval verification with real exit codes and protected corpus excluded, and preserve separate human phase and merge gates.
