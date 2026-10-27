---
name: agent-memory-rag
description: Use when designing or changing character canon stores, episodic memory, belief storage, retrieval, reflection/summarization or context assembly.
---

# Agent Memory + RAG

Never collapse these layers:

1. immutable-ish character core;
2. canon evidence store;
3. static background knowledge;
4. current beliefs with confidence/provenance;
5. episodic memory from this simulation;
6. derived reflections/summaries;
7. current goals/plans.

Retrieval must be scoped by `character_id`, simulation timeline and visibility. Dynamic simulation memories outrank incompatible future-canon evidence.

When compressing memory, preserve event ids/provenance so summaries can be audited. A summary may be wrong; it is derived state, not world truth.

Design for deletion/rebuild of indexes. Canon/memory source records are authoritative; embeddings/vector indexes are derived artifacts.

## Mandatory CI billing/runner fallback

When CI does not allocate runners or cannot execute, follow `docs/agents/ci-billing-fallback.md` and `AGENTS.md`: check runner/steps before diagnosing financial cause; report `CI_NOT_RUN` separately from failed code/evals, run bounded local deterministic/replay/eval verification with real exit codes and protected corpus excluded, and preserve separate human phase and merge gates.
