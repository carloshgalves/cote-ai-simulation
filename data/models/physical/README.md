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

**Open but not blocking** — `oq.capability.cohort-selectivity` asks whether the school's intake is
physically unlike the national cohort the prior is calibrated against. It does not stop the prior
being built; it fixes what the prior must assume while unanswered, which is the national cohort with
no shift, declared as such.

## Forbidden in every case

- A capacity number chosen by hand because we "know" a character is strong.
- Any record stating that one character exceeds another. Ordering is an output of estimation plus a
  seeded sample plus a contest, never an input. The only admissible comparison is a canon
  `COMPARATIVE` feat anchored to one observed event.
- Presenting any of these models, in a prompt or an output, as a rule of the school or as canon.

## State

Structure and gating are decided. What exists:

- [`fitness-test-score-table.yaml`](fitness-test-score-table.yaml) — `UNIT_ANCHORS`, transcribed from
  the 新体力テスト 実施要項 item score table for ages 12-19, both sexes, plus the age-specific overall
  rating bands. Real units on the right cohort, and an official equating between the endurance run
  and the 20 m shuttle run.
- [`SOURCING.md`](SOURCING.md) — the four sourcing gaps that still stand between this and a
  calibrated prior, with the e-Stat table ids and one verified blocker.

Not written: the prior itself, the estimator and the dynamics parameters. Their structure, chosen
alternatives and parameter sources are settled in
[`docs/research/physical-domain-v1.md`](../../../docs/research/physical-domain-v1.md); the numbers
wait on S1 and S2.

One correction from that research lands here rather than in a parameter file: **the acute:chronic
workload ratio must not be used as an injury-risk factor.** It is mathematically coupled, unstable at
low chronic load, unsupported as a causal factor, and the figure that popularised it is the subject of
a formal retraction request. Absolute recent load relative to the body's own available capability,
plus injury history, replaces it. See the research §7.1.
