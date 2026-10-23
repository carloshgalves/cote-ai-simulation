# Canon reading and consolidation V2

1. Register source, edition, digest and operator acceptance.
2. Read all scenes and keep a volume coverage ledger; select durable facts relevant to a simulated institution, actor, capability, relation, resource or disclosure, not trivial narrative filler.
3. Extract atomic paraphrases with page/scene and label narrated fact, character assertion, conjecture or retrospective commentary. A mentioned actor is not necessarily the actor whose capability was demonstrated.
4. Capture behavioral case actor, conditions, evidence available, objective, actions, result, limits and counterfactual. Do not conflate success with general capability.
5. Merge by topic into ONE global stable Character Pack per actor and ONE Institution Pack per organization under `data/canon/consolidated/`; do not create a `volumeN/characters/` or `volumeN/institutions/` tree. Preserve raw evidence in independent atomic source records.
6. Store each behavioral case in `data/canon/consolidated/behavioral_cases/<canonical-actor-slug>/` (one subfolder per primary actor, no volumes). Keep immutable case IDs, references and other-actor mentions distinct. Recompile actor+topic, case-ID-to-path index and institution map; context retrieval uses these maps rather than opening volumes in sequence.
7. Apply distinct narrative time, factual validity time, actor knowledge-acquisition time and repository transaction time. Future canon must never become an actor memory after divergence.
8. Include full curriculum references for White Room; do not invent an ANHS special exam from training and clinical assessments.
9. Validate indices, primary actors, cases, unknowns and epistemic gates. Promote any extracted claim only after explicit record-level review.
10. Publish paraphrases and references only, never the source copyrighted PDF. Release gating is human-controlled.

## Location invariants

- Profiles and institutions are globally keyed and updated across volumes, not duplicated per volume.
- Original works can scope atomic evidence, case origin, coverage and audit records only.
- The retired Volume 0 working extraction is available via Git history and a small non-authoritative audit subset; older actor aliases must not enter retrieval indexes.
- All indexes point to canonical consolidated files; the validator fails if volume-scoped character/institution staging is reintroduced.

- The index `behavioral_case_paths` must cover every behavioral case, matching the canonical owner's folder; build and validation fail on missing, duplicate or orphan cases.
