# Physical feats — shared corpus

Observed physical performances in canon. One collection for the whole cast, indexed by actor,
modality and capacity dimension. **There is no per-character feat store.**

Three reasons the corpus is shared:

- `COMPARATIVE` constraints involve two or more actors and have no single owner;
- de-normalising conditions (same race, same day, same heat) requires seeing the participants together;
- a per-character store invites duplicating one event with divergent numbers.

Two distinct consumers read it, and the distinction matters:

| Consumer | Reads | When | For |
|---|---|---|---|
| capacity estimator | numbers, constraints, conditions | world seeding | `capacity_posterior` per character |
| fidelity RAG | the paraphrase in the linked evidence record | runtime | how that person approaches a physical challenge |

The numeric posterior is a seeding input, never a RAG-retrievable document.

## A feat constrains capacity; it does not measure it

Lower bounds are cheap, upper bounds are expensive. `inference.constraint` says what a record
authorises, and the schema enforces the asymmetry: `UPPER_BOUND` is rejected unless
`effort.attestation` is `NARRATED_MAXIMAL`, `SELF_REPORTED_MAXIMAL` or `VERIFICATION_BOUT`. Same
discipline as `not_before` requiring `not_before_support` in ADR 0005 — the record cannot quietly
assume that what someone did is the most they could do.

`STRAIN_CUES_PRESENT` is deliberately *not* one of them. Panting, staggering, visible effort: in the
real world the equivalent secondary criteria are satisfied at intensities as low as 61% of maximum,
and trained professionals with a dynamometer misjudge sincerity of effort 47-69% of the time. Visible
strain is the fiction equivalent of a secondary criterion, and it licenses nothing.

`VERIFICATION_BOUT` is the structural exception: a second independent performance under conditions
where withholding was not viable — the fiction equivalent of the VO2max verification phase. It is the
only way to attest maximum without relying on narration or self-report.

Never fill `effort.attestation` with a maximal value for convenience. `UNKNOWN` is the default and
the majority case.

## Shape of a record

Illustrative template with placeholder values. **Not data** — nothing here is a canon claim.

```yaml
schema_version: 1
collection: feats.<event-or-modality>
feats:
  - id: feat.<actor>.<slug>
    schema_version: 1
    actors: [actor.<id>]
    modality: endurance-run
    story_time: {mode: HOLDS_BY, holds_by: {anchor: Y1_M0X}, origin: UNKNOWN, basis: "..."}
    measurement:
      dimensions: [aerobic_capacity]
      form: ordinal
      ordinal: {rank: 15, field_size: 40, field_composition: "..."}
    conditions:
      environment: {temperature_c: null, surface: "..."}
      body_state_at_the_time: {prior_exertion: "..."}
      unknowns: ["course length not stated"]
    effort:
      attestation: UNKNOWN            # never assume maximal
      note: "..."
    inference:
      constraint: LOWER_BOUND
      posterior_use: BOUND
      normalization_note: "..."
    observers:
      - {actor: group.class-d, channel: observation}
    divergence_sensitive: true
    evidence_refs: [ev.<slug>]
    provenance:
      epistemic_status: UNVERIFIED
      supports: [{work: ln.y1.vXX, locator: {chapter: X}, strength: primary}]
      continuity: [ln]
```

Numbers live here; prose lives in the linked `evidence` record. The corpus policy in
[`../README.md`](../README.md) applies unchanged: paraphrase only, never protected text.

## Empty for now

Registering feats requires direct Tier 0–1 reading: dimension, unit, conditions and effort attestation
cannot be recovered from a wiki summary, and inventing them from model memory is exactly the failure
this repository is built to prevent. Blocked by `oq.capability.*`.

Schema: [`../schema/feat.schema.json`](../schema/feat.schema.json) ·
dimensions: [`../schema/enums/capacity-dimensions.yaml`](../schema/enums/capacity-dimensions.yaml) ·
model: [`docs/architecture/physical-model.md`](../../../docs/architecture/physical-model.md) ·
decision: [ADR 0006](../../../docs/adr/0006-physical-domain-model.md).
