# Character Evidence V0 -> Canon RAG V2: controlled compatibility boundary

**Integration source:** historical `feat/character-evidence-v0` (four commits).
**Reference contract:** Canon RAG V0 now integrated in `main` (PR #7); `CanonEvidence` V2.

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

## Export conditions — fail closed (remediated Finding 1)

Public conversion functions do **not** accept caller-supplied approval dictionaries
or reviewer strings. The *only* authority source is the versioned file
`data/canon/reviews/character-evidence-v2-approvals.json`, read at a fixed path
and checked against a SHA-256 digest pinned in the bridge implementation.
The current file has **zero approved records**, so exporting any of the 32
legacy records is denied. Changing the manifest AND its code pin requires a
separate, independently reviewed repository PR: the repository's reviewed
release is the trust boundary. This scheme does **not** cryptographically
authenticate a human GitHub reviewer or independently verify review URLs;
repository access controls and human review are required for new grants.

Every future per-record grant must include evidence ID, complete canonical
record SHA-256 (including text, actors, story time and provenance), authorized
behavioral role, private-content review, named reviewer, review PR reference,
and verified narrative work/ordinal. A changed record invalidates the grant.
A source locator alone cannot invent an ordinal. The public adapter must still
validate source edition/locator, one primary source, factual-claim exclusion,
V2 schema, role, horizon and provenance. `UNVERIFIED` stays `UNVERIFIED`
and later plot facts remain prohibited from initial agent memory.

The private helper `_project_reviewed_record` is solely a schema projection
seam and accepts synthetic review fixtures for tests; it **is not an authority
or an API to be called by the RAG importer**. A direct call to it bypasses
the release authority and must never be used as a runtime export entrypoint.
Only `convert_for_v2` / `eligible_records` have authorization semantics,
and neither currently exports a record.

The default V2 export is empty and 32/32 records remain quarantined; the
positive synthetic test is not evidence that a human authorized export.

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

PR #7 is integrated into `main` (merge `f2339acf`). This PR #18 now
targets `main`, with the historical four character-evidence commits intact.
No data is exported before a separately reviewed release-authority update,
source verification and semantic deduplication against `focal-v0`.
