# Physical simulation models — NOT CANON

Our reconstructions of how bodies behave. Every record here carries `not_canon: true`.
Dependency stays unidirectional: canon → models. No record here may appear in
`provenance.supports` of a canon claim.

Decision: [ADR 0006](../../../docs/adr/0006-physical-domain-model.md).
Model: [`docs/architecture/physical-model.md`](../../../docs/architecture/physical-model.md).

## The four models

| Model | `model_kind` | What it holds | Calibrated against |
|---|---|---|---|
| `POPULATION_PRIORS` | prior | capacity distributions per cohort (year, sex, club membership, build) | real-world age-cohort fitness norms |
| `UNIT_ANCHORS` | normalisation | mapping from feat observations to physical units; condition and body-state de-normalisation | measurement literature, internal consistency |
| `CAPACITY_ESTIMATOR` | estimator | constraints → posterior; hierarchical shrinkage toward cohort mean when evidence is thin; `evidence_sufficiency` | canon feats as fixtures, never as inputs to the formula |
| `BODY_DYNAMICS` | dynamics | fatigue, recovery, sleep debt, substrate, hydration, thermal load, healing, injury risk, contest resolution | physiology, plus deterministic unit tests |

## Population priors: absence of evidence is a prior, never a guess

A student with no feats draws from a cohort distribution conditioned on what canon actually states.
No hand-set attributes anywhere, for anyone.

The anchoring source is the Japanese national physical fitness test battery (50 m dash, handball
throw, standing long jump, sit-ups, sit-and-reach, grip strength, shuttle run / endurance run), which
publishes norms by age and sex. Two consequences:

1. the test items determine how capacity dimensions are anchored to units;
2. **the numeric norms must be transcribed from the publication, not from model memory.** Until
   transcribed, the prior is explicitly provisional and any run using it says so in its metadata.

The same prior serves focal characters: a focal character is this prior with many constraints, an
anonymous NPC is the same prior with none.

## What is gated and what is not

**May be built now** — priors and body dynamics calibrate against real physiology, not against canon
reading, so no open question blocks them.

**Blocked** — per-character posteriors may not be committed, and no named capacity profile compiles
to the engine, while these are open:

`oq.capability.fitness-test-records` · `oq.capability.club-and-training-background` ·
`oq.capability.effort-attestation` · `oq.capability.physical-exam-tasks`

## Forbidden in every case

- A capacity number chosen by hand because we "know" a character is strong.
- Any record stating that one character exceeds another. Ordering is an output of estimation plus a
  seeded sample plus a contest, never an input. The only admissible comparison is a canon
  `COMPARATIVE` feat anchored to one observed event.
- Presenting any of these models, in a prompt or an output, as a rule of the school or as canon.

## Empty for now

Structure and gating are decided; parameter files are not written. Building `POPULATION_PRIORS`
starts with transcribing cohort norms, which is a sourcing task, not a modelling one.
