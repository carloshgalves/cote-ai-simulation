# Evidence

Units of RAG-facing canon evidence are our own paraphrases of passages, with locator, story time and
situation tags. Never store protected source text.

Character V0 records live under `data/canon/evidence/characters/` and follow
`evidence_collection.schema.json`. They may be consumed later by RAG, but this directory contains
no retrieval infrastructure or embeddings.

A later-volume passage may support a pre-existing tendency/capability while remaining
divergence-sensitive. Character packs must mark such use as `FUTURE_EVIDENCE_ONLY`; it must not
become initial memory or privileged knowledge.

See `docs/canon/provenance.md` and the corpus policy in `../README.md`.
