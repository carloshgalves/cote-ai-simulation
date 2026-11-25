# Tomodachi Game — Go integration checkpoint

The external reference runner passed the operator's local Node validation (60 packs, 59 cases, 126 evidence, 86 eval scenarios; 35 deterministic + 13 adversarial assertions; ten terminal/replay fixtures). This does not prove the new Go checkpoint.

## Implemented
- Explicit external-world scope in the existing RAG V2 retrieval and gate functions, default LN path unchanged. A pinned projection of 21 case-linked abstract lessons enters only the secondary behavioral lane. No raw narrative episode, unsupported factual claim, or cross-world record is sent to models.
- Pure Go Kokkuri reducer under internal/csf/reducer/tomodachi: simultaneous cohorts, actor-specific views, public claims, private votes/debt/card, deterministic rules and candidate encoding with native CSF codec.
- Go experimental runner: no host fixture outcomes; each question's discussion and ballot are resolved from decisions; real HTTP model adapter or test-only scripted model, native retrieval subprocess per actor, owner-only decision trace and deterministic candidate replay. Fixed question texts, speaking phase and final answer are explicit reconstructed model rules.
- Tests for invalid decisions, actor/continuity gates, replay and tampering.

## Reproduction (from root)
Use repository-pinned Go toolchain, Python 3 with pytest, jsonschema, PyYAML.
```sh
python3 -m pytest tests/canon_rag -q
go test ./internal/csf/reducer/tomodachi ./internal/tomodachi ./cmd/tomodachi-local
go run ./cmd/tomodachi-local --scripted --out /tmp/tg-go-owner.json
go run ./cmd/tomodachi-local --replay /tmp/tg-go-owner.json
```
For a real model, set TG_LLM_URL to the complete chat completions URL, TG_LLM_MODEL and optional TG_LLM_KEY, then omit --scripted. No provider calls or empirical strategy evaluations were performed in this checkpoint.

## Not yet native CSF publication
The currently pinned CSF codec/policy/Genesis/reducer and SQLite replay paths support closed operation/state variants; the new prospective candidate domain is NOT registered. A JSON owner journal is not a CSF CycleCommit. Do not route game effects through existing school stock or physical actions. Needed: versioned extension registration and pinned Genesis, typed action and event schemas, authenticated cohort/fence materialization, CommitCoordinator publication, transactional SQLite transition validation and replay, perception/KnowledgeInput routing, fork/snapshot handling, normative vectors and independently reviewed integration tests. No existing immutable bundle has been modified. Nine remaining game models remain in the JS reference runtime, not autonomous Go. No UI integration. The full environment is not yet ready for a scientifically meaningful all-game experiment.
