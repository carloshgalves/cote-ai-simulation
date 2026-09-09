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
