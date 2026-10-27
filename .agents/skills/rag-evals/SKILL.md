---
name: rag-evals
description: Use whenever canon ingestion, chunking, embeddings, metadata filters, retrieval, reranking or grounding behavior changes.
---

# RAG Evals

Evaluate retrieval separately from generation.

For each golden query record:
- character and simulation phase;
- expected source/evidence ids;
- evidence that must be excluded by timeline or character access;
- acceptable abstention.

Retrieval metrics/checks:
- hit/recall@K;
- ranking quality;
- irrelevant-context rate;
- wrong-character leakage;
- post-divergence canon leakage;
- private-corpus boundary violations.

Generation checks:
- factual support by retrieved evidence;
- unsupported behavioral claims;
- source provenance;
- correct uncertainty when evidence is missing/conflicting.

Any forbidden-timeline or forbidden-character retrieval is a critical failure even if the final answer looks good.

## Mandatory CI billing/runner fallback

When CI does not allocate runners or cannot execute, follow `docs/agents/ci-billing-fallback.md` and `AGENTS.md`: check runner/steps before diagnosing financial cause; report `CI_NOT_RUN` separately from failed code/evals, run bounded local deterministic/replay/eval verification with real exit codes and protected corpus excluded, and preserve separate human phase and merge gates.
