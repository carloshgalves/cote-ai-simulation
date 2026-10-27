---
name: to-spec
description: Use when a conversation, design decision or feature idea is ready to become an implementable specification for this repository.
---

# To Spec

A spec must include:
- problem and user-visible goal;
- domain terms/invariants affected;
- in-scope / out-of-scope;
- deterministic behavior vs model-dependent behavior;
- data/state changes;
- knowledge/visibility implications;
- failure modes;
- observability and reproducibility requirements;
- tests and LLM/RAG/simulation evals;
- migration/backward compatibility if persisted snapshots change.

Do not hide unresolved decisions inside acceptance criteria. Link the relevant ADR or research task.

## Mandatory CI billing/runner fallback

When CI does not allocate runners or cannot execute, follow `docs/agents/ci-billing-fallback.md` and `AGENTS.md`: check runner/steps before diagnosing financial cause; report `CI_NOT_RUN` separately from failed code/evals, run bounded local deterministic/replay/eval verification with real exit codes and protected corpus excluded, and preserve separate human phase and merge gates.
