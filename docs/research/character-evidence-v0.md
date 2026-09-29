# Character Evidence V0 — research and horizon decision

Date: 2026-09-29
Branch: `feat/character-evidence-v0`

## Decision this research must inform

Choose a canon/knowledge horizon and evidence policy for the first six focal character packs without coupling Trilha D to the still-independent Exam Engine or turning later canon into initial memory.

## Alternatives considered

### A. `Y1_START` baseline — chosen

Build one reusable school-entry pack per character. Pre-enrollment self-knowledge may seed the initial layer when supported. Events after the horizon may only evidence a plausibly pre-existing behavioral tendency or capability and are marked `FUTURE_EVIDENCE_ONLY`.

Advantages:
- independent of which Special Exam Trilha C selects;
- matches ADR-0005, where canonical `known_by` is authoritative only for initial seeding;
- makes leakage auditing explicit;
- avoids hardcoding later rivalries, alliances and goals.

Cost:
- some behavioral evidence comes from later observations and must not be mistaken for episode memory.

### B. Horizon at the start of the first selected Special Exam — not chosen for V0

This would provide richer observed relationships and goals, but couples character data to Trilha C and risks importing the canonical path that the simulator is meant to diverge from.

### C. One pack per later canonical checkpoint — deferred

Useful eventually for offline fidelity analysis, but unsuitable as the initial simulation seed because each checkpoint mixes canonical history with state that may never occur after divergence.

## Evidence policy

The repository's current canon baseline is still explicitly unverified. Therefore this implementation creates no `VERIFIED` character assertion.

Candidate primary support is recorded through existing source ids:
- `ln.y1.v01`
- `ln.y1.v02`
- `ln.y1.v03`
- `ln.y1.v07`
- `ln.y1.v10`
- `ln.jp.y1.v00` where pre-enrollment background is relevant.

Community summaries and character pages are used only as `discovered_via` locator aids. They never make an assertion verified.

The official Seven Seas series page confirms the licensed English light-novel series and volume bibliography, but it is not used as behavioral evidence:
https://sevenseasentertainment.com/series/classroom-of-the-elite-light-novel/

Discovery aids used to locate candidate scenes:
- https://you-zitsu.fandom.com/wiki/Light_Novel_Volume_1/Summary
- https://you-zitsu.fandom.com/wiki/Light_Novel_Volume_2/Summary
- https://you-zitsu.fandom.com/wiki/Light_Novel_Volume_3/Summary
- https://you-zitsu.fandom.com/wiki/Light_Novel_Volume_10/Summary

No source text is copied into the repository; evidence records are original paraphrases.

## V0 coverage

Six packs:
- Kiyotaka Ayanokōji
- Suzune Horikita
- Kakeru Ryūen
- Kikyō Kushida
- Yōsuke Hirata
- Honami Ichinose

Passage-level evidence records: 32.

The packs cover:
- identity and aliases;
- private self-knowledge where support can be located;
- small behavioral tendencies;
- demonstrated/inferred/possible/future-only goals;
- multidimensional nonphysical capabilities;
- context → objective → behavior → consequence decision patterns;
- social interaction tendencies without prose imitation;
- blind spots and failure modes;
- explicit capability-versus-manifestation rules.

Physical capacity is deliberately excluded. It remains owned by the shared feat corpus and Embodiment domain.

## Important gaps

1. **Primary verification remains open.** None of the 32 records is human-verified against Tier 0–1 text yet.
2. **Exact Y1_START private-knowledge boundaries need direct reading.** The most sensitive records are Kiyotaka's pre-enrollment background, Suzune/Manabu family context, Kushida/Horikita prior history, and Hirata's pre-enrollment trauma.
3. **Later behavior is not proof of an unchanged earlier goal.** V0 uses later events conservatively for tendencies/capabilities; event-specific goals stay future-only (for example Ryūen's search for X).
4. **Ichinose has no distinctive private Y1_START knowledge encoded yet.** The registered V0 source set provides stronger behavioral than private-background evidence for her.
5. **Ryūen's exact active Y1_START priorities remain underdetermined.** V0 records later control/leverage behavior as evidence while refusing to seed the later X objective.
6. **Physical feats remain empty.** No character pack substitutes a guessed physical score for the missing feat corpus.
7. **First-exam roster is an integration dependency.** If Trilha C selects an exam requiring another indispensable participant, add that participant deliberately rather than expanding the roster wholesale.

## Canon conflicts

No new formal canon conflict is asserted in this V0. The main risk is methodological, not a resolved contradiction: later canon can show behavior after the divergence point while the simulator needs a school-entry disposition. The pack schema handles this with `evidence_temporality` and keeps later evidence out of initial knowledge.

## Recommendation

Use these packs only as an unverified evidence/eval layer until a human performs direct Tier 0–1 verification. Future RAG should retrieve evidence under character/timeline/visibility gates; future cognition should consume the pack as structured priors, not as a giant persona prompt.
