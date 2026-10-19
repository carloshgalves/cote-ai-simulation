# Evidence

Authoritative RAG evidence units are our own paraphrases with story time, narrative position and provenance. Never store full protected source text here.

CanonEvidence V2 separates:
- `BEHAVIORAL_CANON`: examples about how the named actor behaved; never current simulation memory or world truth.
- `KNOWLEDGE_EVIDENCE`: evidence tied to claims; factual context requires an already-authorized claim and all retrieval gates to pass.

The first vertical corpus is under `focal-v0/`. It covers Kiyotaka Ayanokoji, Suzune Horikita, Kikyo Kushida, Yosuke Hirata, Kakeru Ryuen and Honami Ichinose, plus two minimal institutional evidence records.

All records in this V0 corpus are `UNVERIFIED`. They were located with Tier 5 help and still require human Tier 0-1 verification. Behavioral retrieval therefore requires explicit opt-in for architecture/eval work, and factual retrieval abstains.

See `docs/spec/canon-rag-v0.md` and `data/canon/rag-evals/`.
