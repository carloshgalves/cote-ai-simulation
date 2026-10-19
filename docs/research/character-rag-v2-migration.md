# Character Evidence V0 -> Canon RAG V2: controlled compatibility boundary

**Integration source:** historical `feat/character-evidence-v0` (four commits).
**Reference contract:** Canon RAG V0 `feat/canon-rag-v0`, `CanonEvidence` V2.

## What is preserved

Six declarative character packs, six actor stubs, 32 paraphrased evidence records in
`data/canon/evidence/characters/*.yaml`, fidelity evals and contrast tests.

These historical YAML collections are **not Canon RAG V2 collections**: they use
`records`, V1 evidence fields, no `retrieval_role`, no supported `narrative_position`
and no per-record private-content clearance. Renamed schemas
`character_evidence_legacy_{record,collection}.schema.json` validate them without
overwriting RAG's authoritative `evidence.schema.json` (V2) or
`evidence_collection.schema.json` (V2 container).

The Canon RAG loader indexes only `data/canon/evidence/focal-v0/*.json`.
It **does not** scan the legacy character YAML collections.

## Export conditions — fail closed

`tools/character_evidence_v2_bridge.py` supports projection of an individual
historical record to schema-valid `BEHAVIORAL_CANON` V2 only after:
1. explicit individual evidence-ID approval by a named reviewer;
2. explicit review of private/secret/factual content before behavioral classification;
3. confirmed (not automatically invented) narrative *work and ordinal*;
4. one resolvable primary source, plus edition and locator for each support;
5. no factual `supports_claims` piggybacking on a behavioral record;
6. source epistemic status stays `UNVERIFIED`; Tier 5 discovery is not primary proof.

No approvals or narrative ordinals are supplied in the repository. Therefore the
**default V2 export is empty** and 32/32 records remain quarantined. The positive
synthetic test does not authorize any real evidence for runtime retrieval.

Approval is necessary but does not promote a source to `VERIFIED`, convert
`FUTURE_EVIDENCE_ONLY` to initial memory, or authorize use beyond the RAG gate.
Any exported item must still pass the RAG's time, character, horizon, continuity,
divergence and provenance gates.

## Before this Draft PR can be integrated

- Review primary locators/editions and map real ordinal values per approved item.
- Detect overlap with existing `focal-v0` evidence and resolve semantically without
  duplicating IDs or double-counting apparent behavioral support.
- Explicitly decide which of the 32 records may be V2 behavioral and which remain
  offline fidelity evidence. Some contain facts about private pre-enrollment
  experiences: these must never enter an unauthorized character context merely
  because their role is `BEHAVIORAL_CANON`.
- Verify all six pack↔evidence references, leak checks, JSON-schema V2 validation,
  corpus/golden tests, and false-authority tests.
- Keep character cognition, special exams and CSF runtime outside this PR.

## Delivery ordering

This PR is **stacked on PR #7** to verify V2 compatibility against the actual V2
schemas and gates. It is independent from the Zodiac/VIP spec PR and the CSF PR.
When PR #7 merges to `main`, retarget this Draft PR to `main` and validate it
again. Never merge the stack directly into `main` before that dependency.
