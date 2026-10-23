# Canon reference layout — global consolidated packs

**Authoritative consolidated reference:** `data/canon/consolidated/`. There is **one read-model pack per character** and **one per institution**, regardless of the number of source volumes.

- `characters/<actor>.json`: cross-volume profile, topical summaries and stable references to behavioral cases and evidence.
- `institutions/<institution>.json`: cross-volume policies, curriculum, factions, personnel and other institutional knowledge.
- `behavioral_cases/<actor-slug>/<existing-case-name>.json`: demonstrations grouped by **canonical primary actor**, not by volume. Stable `case_id` stays unchanged. Find a case in O(1) indexed lookup via `indexes/character-topic-map.json` → `behavioral_case_paths[case_id]`; the behavioral topic index still stores stable IDs.
- `evidence/volume0.json` and future `evidence/<source>.json`: volume-scoped atomic paraphrases with source locators. Sources do not own or duplicate character packs.
- `indexes/`: deterministic direct actor/topic, evidence/topic and institution lookups; no linear scan of volumes.
- `audits/legacy-extraction/`: retained **non-authoritative** raw analysis, historical questions, timeline and relation records from the initial Volume 0 extraction. These use older identifiers, are not runtime/agent context and require reconciliation with global IDs before compilation.

**No new `data/canon/volumeN/characters/` or `data/canon/volumeN/institutions/` directories.** New volumes update the global consolidated packs and append source-local evidence. The old Volume 0 staging tree was retired in the PR branch; Git history retains its prior contents.

The existing `data/canon/characters/*.yaml` are legacy Y1-start prior packs with a different schema. They remain separate until an explicit canonical-schema migration, not a second V2 character source. Source approval by the operator does not auto-verify every individual claim; source text itself stays private and off the public repository.

Cases must never be saved at the root of `behavioral_cases/` or under a volume directory. New actors get a new subfolder; each case belongs to the primary actor demonstrating the behavior. Other participants are represented by references, not duplicate case files.

## Year 1 Volume 1 ingestion

Volume 1 evidence is at `evidence/y1-v01.json`, with 136 scene-relevant records, operator-supplied Seven Seas 2019 edition provenance and a chapter coverage ledger in `audits/y1-v01-coverage.json`. Fifty behavioral cases belong to their primary actor folders. Character packs are global and updated in place; `institutions/anhs.json` covers public versus undisclosed point rules. `institutions/student-council.json` registers the newly documented council. `exams/y1-first-midterm.json` is a **reference only** and must not be mistaken for executable scoring. Four indexes (actor/topic, case paths, institution, and volume-specific page maps) support direct retrieval without scanning books.

Facts revealed in May must not seed April character beliefs. Narrated impressions and actor claims remain separate from institutional truth, and acceptance of this edition does not auto-verify all extracted records.

## Year 1 Volume 1 semantic audit additions

A second content pass added swimming performances, seven short-test failure candidates, Kōenji/Yukimura's tied short-test result, late syllabus notices, parenthetical cost constraints, and secondary students. `assessments/y1-v01-swimming.json` is **a physical education assessment, not an ANHS special exam**. `institutions/anhs.json` distinguishes disclosed rules from school-only information and narrator inference, avoiding January-like automatic allowance assumptions. `institutions/koenji_conglomerate.json` contains *only* Kouenji's statement of prospective succession, not verified wealth or spending power. Yamawaki's Class C affiliation was corrected from an earlier Class A misclassification. All source interpretations have received assistant semantic review, but operator-level canon verification remains separately pending.

## Volume 0/1 closure (Draft PR #29)

`migrations/y1-legacy-six.json` enumerates all 119 source-locked legacy claims from the six Y1_START YAML packs. They are **not** a second actor memory; the single consolidated read model is authoritative. Ryūen exists in global characters without Volume 2+ imported knowledge.

`audits/source-review-register-v0-v1.json` lists all **268 evidence items and 75 retained behavioral cases** in an individualized automated source-window triage. **Individual assistant semantic review was completed on 2026-10-23**, with claim-level source modes and contextual restrictions recorded; one contextual Kijima case was retracted and source locators/actor attribution were repaired. All items remain `verified:false` until the separate operator release approval. Page-text overlap and known file digests alone never justify promotion to VERIFIED.

`exams/y1-first-midterm-executable.json` and `tools/canon_first_midterm_v1.py` expose a deterministic **partial** resolver for the documented English Class D cutoff, and the historically approved grade-mark transaction. No general purchase API, automatic expulsion or unknown-subject rules exist, and it is not connected to the CSF runtime.

### Semantic review completion boundary

The source-review register records 268 source-linked assistant-reviewed assertions and 75 actor-owned behavioral cases (after one Kijima context-only case was retired). This means the **source-to-paraphrase meaning was reviewed**, not that all first-person claims are necessarily world truth, nor that scene outcomes may be forced in counterfactual simulation. A human release/verification stage remains deliberately separate. Review fixes include separating Arisu's genetic argument from Sakayanagi father's decisions, restricting Naoe's waiting behavior to the observed episode, and extending precise page citations for bus introductions, exam grading and appeals.

## Year 1 Volume 2 (draft ingestion)

`evidence/y1-v02.json` preserves 91 scene-relevant paraphrases from seven chapters, together with page locators and speaker-sensitive source modes. The character and institution packs remain global. Twenty-seven actor-owned demonstrations are stored under `behavioral_cases/<primary-actor>/`; six additional minor roles have minimal packs rather than invented biographies. Direct indexes and per-record semantic review are separate gates, and the operator release gate remains pending. The incident involving Sudō is **not** a general disciplinary algorithm. Ryūen's alleged authorship remains an attribution by Ichinose, not independent proof. The source PDF is not stored in the repository.

The individual assistant semantic review is complete for **91 evidence items and 27 actor-owned behavioral cases** in `audits/source-review-register-y1-v02.json`. Each item has a page-grounded explanation, explicit attribution and epistemic limits. Source locators and seven primary-actor cases were corrected during review. All 118 entries remain `verified: false`: human operator approval is a distinct, unfulfilled release gate. CI does not override it.

## Volume 2 operator acceptance — 2026-10-23

The operator explicitly authorized merging the completed 118-item assistant semantic review into the canonical **reference corpus**. This is not a false declaration that the operator independently checked all 118 PDF windows: individual `verified` flags remain false and later events remain disallowed as agent starting knowledge or guaranteed simulation outcomes.

## Year 1 Volume 3 — draft extraction

The 262-page operator-provided Seven Seas 2019 source is **not committed**; `sources/y1-v03.json` records its SHA-256. The initial seven-chapter source pass contains **117 atomic contextual records** in `evidence/y1-v03.json` and **35 primary-actor behavioral cases**, all consolidated into global character packs. **Six newly attested actors** have minimal or bounded global packs. The independent island examination is described only in `exams/y1-deserted-island.json` with its disclosed 300-point allowance, resource and leader rules, 8-hour spot timers, penalties, explicit results, and unknowns; it is **not an executable or completed migration**. Indexes provide direct actor/topic/case/page lookup. 

**Assistant semantic review completed:** `audits/source-review-register-y1-v03.json` contains 117 evidence + 35 behavioral cases, each with source-window judgement and epistemic limits. All remain unverified, with runtime/agent-memory and operator-release gates closed. A retrospective reveal about Ayanokōji's leadership substitution is strictly later knowledge and cannot enter an earlier character memory or an alternate divergence.


**Volume 3 adversarial check:** `audits/adversarial-review-y1-v03.json` records corrected attribution, leader/roll-call chronology, documented A/C alliance and report-only claims. This second pass was performed by the same assistant; it is **not** an independent human or separately commissioned reviewer. External review and operator release remain gates before merge. All items remain `verified: false`.
