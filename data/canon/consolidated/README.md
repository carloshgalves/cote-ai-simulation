# Canon reference layout — global consolidated packs

**Authoritative consolidated reference:** `data/canon/consolidated/`. There is **one read-model pack per character** and **one per institution**, regardless of the number of source volumes.

- `characters/<actor>.json`: cross-volume profile, topical summaries and stable references to behavioral cases and evidence.
- `institutions/<institution>.json`: cross-volume policies, curriculum, factions, personnel and other institutional knowledge.
- `behavioral_cases/`: independently referenced demonstrations attributed to their actual primary actors.
- `evidence/volume0.json` and future `evidence/<source>.json`: volume-scoped atomic paraphrases with source locators. Sources do not own or duplicate character packs.
- `indexes/`: deterministic direct actor/topic, evidence/topic and institution lookups; no linear scan of volumes.
- `audits/legacy-extraction/`: retained **non-authoritative** raw analysis, historical questions, timeline and relation records from the initial Volume 0 extraction. These use older identifiers, are not runtime/agent context and require reconciliation with global IDs before compilation.

**No new `data/canon/volumeN/characters/` or `data/canon/volumeN/institutions/` directories.** New volumes update the global consolidated packs and append source-local evidence. The old Volume 0 staging tree was retired in the PR branch; Git history retains its prior contents.

The existing `data/canon/characters/*.yaml` are legacy Y1-start prior packs with a different schema. They remain separate until an explicit canonical-schema migration, not a second V2 character source. Source approval by the operator does not auto-verify every individual claim; source text itself stays private and off the public repository.
