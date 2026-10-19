# Evidence

Authoritative RAG evidence units are our own paraphrases with story time, narrative position and provenance. Never store full protected source text here.

CanonEvidence V2 separates:
- `BEHAVIORAL_CANON`: examples about how the named actor behaved; never current simulation memory or world truth.
- `KNOWLEDGE_EVIDENCE`: evidence tied to claims; factual context requires an already-authorized claim and all retrieval gates to pass.

The first vertical corpus is under `focal-v0/`. It covers Kiyotaka Ayanokoji, Suzune Horikita, Kikyo Kushida, Yosuke Hirata, Kakeru Ryuen and Honami Ichinose, plus two minimal institutional evidence records.

All records in this V0 corpus are `UNVERIFIED`. They were located with Tier 5 help and still require human Tier 0-1 verification. Behavioral retrieval therefore requires explicit opt-in for architecture/eval work, and factual retrieval abstains.

See `docs/spec/canon-rag-v0.md` and `data/canon/rag-evals/`.

## Character Evidence V0 (legacy, not a RAG V2 corpus)

The historical `data/canon/evidence/characters/*.yaml` collections are legacy character-pack
support records (schema V1). They are deliberately **not loaded or retrieved by Canon RAG V0**:
the RAG-authoritative `evidence_collection.schema.json` and `evidence.schema.json` remain V2.
The legacy records have their own namespaced schemas and require explicit private-content review,
source metadata and narrative ordinal before a gated V2 conversion can be authorized.
Never treat `FUTURE_EVIDENCE_ONLY` as memory or knowledge at `Y1_START`.
