"""World seeding — the only place in the repository that writes `capacity_baseline`.

`physical-model.md` §5 and invariant 3: absence of evidence produces a versioned
cohort prior, never a hand-picked attribute. A character with no constraints is
not a special case — it is the posterior of zero evidence, which is the prior,
with `evidence_sufficiency = 0` in every dimension. A focal character is the
same prior with many constraints, and PSV1-3 arrives **behind this signature**,
replacing what `posterior_for` computes without changing what seeding does with
it.

Three failure modes are structural here rather than hoped for:

* **F3, hand-set attribute** — `CapacityBaselineRecord` is constructed in this
  module and nowhere else, and a test of architecture fails if that changes.
* **F5, re-rolled capacity** — the sample is frozen. `CapacityBaselineStore`
  refuses a second write for a character, and the record itself is immutable.
* **F6, creation order** — a character's substream is derived from its id and the
  world seed, never from a counter. An NPC created on the last day of the run
  draws the body it would have drawn on the first, and displaces nobody.
"""

from __future__ import annotations

import hashlib
import json
from collections.abc import Iterable, Mapping, Sequence
from typing import Any

from pydantic import BaseModel, ConfigDict, field_validator

from .eventlog import EVENT_CAPACITY_SAMPLED, EventLog
from .prior import PopulationPrior, Sex
from .rng import (
    PURPOSE_CAPACITY_SAMPLE,
    PURPOSE_COHORT_SEX,
    SEEDING_EVENT_ID,
    substream,
    substream_name,
)
from .types import (
    CapacityBaselineRecord,
    Dimension,
    SexSource,
    Y1_START,
    freeze_mapping,
)

__all__ = [
    "CohortCovariates",
    "NO_COVARIATES",
    "SexSource",
    "Posterior",
    "CapacityBaselineStore",
    "AlreadySeededError",
    "UnknownCovariateSubjectError",
    "RunContextMismatchError",
    "posterior_for",
    "seed_character",
    "seed_cohort",
    "cohort_ids",
]


class AlreadySeededError(RuntimeError):
    """A body is sampled once. A second sample would be a different character."""


class UnknownCovariateSubjectError(KeyError):
    """Covariates were supplied for a character the cohort does not contain."""


class RunContextMismatchError(RuntimeError):
    """A sample was about to be appended to a log that declares other inputs.

    `run.started` carries the world seed, the prior version and the posterior
    hash per character of spec §9.2, and a replay reads those to reproduce the
    bodies. A `capacity.sampled` drawn under different inputs would make the
    audit artefact contradict itself while every field in it stayed well formed.
    """


class CohortCovariates(BaseModel):
    """What canon already fixes about a character, before any feat is weighed.

    ADR 0006 decision 4 does not say "draw everything": absence of evidence draws
    from a cohort *conditioned on what canon states* — year, sex, club or the lack
    of one, build. `population-prior.yaml` says the same of the sex ratio it
    carries, in as many words: the split is an `[INT]` assumption, and "a cohort
    seeded for a specific class should pass its composition in explicitly". A
    covariate the source establishes is not an unknown, and drawing it anyway
    writes world truth that contradicts the canon it claims to model.

    This is not a second door for a hand-set attribute (F3). The model forbids
    unknown fields, and a covariate is by construction *not* a capacity: no value,
    bound or percentile in any of the 15 dimensions may be named here. What a feat
    constrains arrives as a typed constraint through the estimator (PSV1-3), never
    as a covariate.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    #: `None` means canon does not establish it, which is the ordinary case for an
    #: anonymous cohort member and the reason the draw exists at all.
    sex: Sex | None = None


#: The covariates of a character canon says nothing about. Shared rather than
#: rebuilt so that "no covariates" is one object with one meaning.
NO_COVARIATES = CohortCovariates()


class Posterior(BaseModel):
    """What the sample is drawn from, and how much evidence shaped it.

    In V1 this is always the prior: `data/canon/feats/` is empty and stays empty
    (spec §3.2), so `constraint_count` is 0 and `evidence_sufficiency` is 0.0
    everywhere. The type carries the fields anyway because PSV1-3 fills them, and
    because a run must be able to say "this posterior is the prior under another
    name" without the reader reconstructing the inference.
    """

    model_config = ConfigDict(frozen=True)

    character_id: str
    sex: Sex
    #: Whether `sex` came from canon or from the cohort ratio. It is provenance,
    #: not identity: conditioning on a known male and drawing a male produce the
    #: *same* distribution, so `posterior_hash` is deliberately equal for the two.
    #: Which of them happened still has to survive into the log and the snapshot,
    #: or a reader cannot tell a modelled covariate from a sourced one.
    sex_source: SexSource
    prior_version: str
    prior_content_hash: str
    constraint_count: int
    posterior_hash: str
    evidence_sufficiency: Mapping[Dimension, float]
    effective_sample_size: float | None = None

    @field_validator("evidence_sufficiency", mode="after")
    @classmethod
    def _freeze_evidence_sufficiency(
        cls, value: Mapping[Dimension, float]
    ) -> Mapping[Dimension, float]:
        return freeze_mapping(value)


class CapacityBaselineStore:
    """The frozen samples of one run, write-once per character."""

    def __init__(self) -> None:
        self._records: dict[str, CapacityBaselineRecord] = {}

    def put(self, record: CapacityBaselineRecord) -> None:
        existing = self._records.get(record.character_id)
        if existing is not None:
            raise AlreadySeededError(
                f"{record.character_id} already has a capacity_baseline in this run. "
                f"A body is sampled once, at seeding, and frozen (physical-model.md §5.3)."
            )
        self._records[record.character_id] = record

    def get(self, character_id: str) -> CapacityBaselineRecord:
        try:
            return self._records[character_id]
        except KeyError:
            raise KeyError(f"{character_id} has not been seeded in this run") from None

    def __contains__(self, character_id: object) -> bool:
        return character_id in self._records

    def __len__(self) -> int:
        return len(self._records)

    def items(self) -> Iterable[tuple[str, CapacityBaselineRecord]]:
        return tuple(self._records.items())

    def ids(self) -> tuple[str, ...]:
        return tuple(self._records)


def cohort_ids(count: int, *, prefix: str = "npc", start: int = 1, width: int = 4) -> tuple[str, ...]:
    """Anonymous cohort ids. Deterministic, and stable when the cohort grows.

    Seeding `--n 41` gives the same forty bodies as `--n 40` plus one more,
    because the id fixes the substream and the position in the list does not.
    """
    if count < 0:
        raise ValueError("cohort size cannot be negative")
    return tuple(f"{prefix}.{index:0{width}d}" for index in range(start, start + count))


def posterior_for(
    world_seed: int,
    character_id: str,
    prior: PopulationPrior,
    constraints: Sequence[Any] = (),
    covariates: CohortCovariates | None = None,
) -> Posterior:
    """The distribution a character's body is drawn from.

    The seam PSV1-3 enters through: it will run the SIR here, weighting prior
    particles by typed constraints, and report a real ESS and a real
    `evidence_sufficiency`. Until then, constraints are refused rather than
    ignored — silently dropping evidence would make a posterior that claims to
    have seen it.

    `covariates` is the other half of ADR 0006 decision 4, and it is not the same
    door: a constraint is evidence about a *capacity* and needs the estimator to
    be weighed, while a covariate is a cohort fact canon already fixes and only
    ever selects which marginals the copula reads. Drawing a covariate the source
    states would be inventing world truth, so it is conditioned on, not sampled.
    """
    if constraints:
        raise NotImplementedError(
            "constraints are not consumed yet: the capacity estimator is PSV1-3. "
            "Seeding refuses evidence it cannot use rather than dropping it, because a "
            "posterior that silently ignores a constraint is indistinguishable from one "
            "that weighed it and found it uninformative."
        )

    covariates = NO_COVARIATES if covariates is None else covariates
    if covariates.sex is not None:
        #: The substream for `cohort.sex` is simply not consumed. Nothing else
        #: moves: `capacity.sample` is named from the character id, so a known
        #: covariate changes this character's marginals and nobody else's draw.
        sex: Sex = covariates.sex
        sex_source: SexSource = "KNOWN"
    else:
        sex = prior.draw_sex(
            substream(world_seed, character_id, SEEDING_EVENT_ID, PURPOSE_COHORT_SEX)
        )
        sex_source = "DRAWN"
    #: Zero evidence: sd(posterior) == sd(prior), so 1 - sd(posterior)/sd(prior)
    #: is exactly 0 in every dimension (physical-model.md §5.6).
    sufficiency = {dimension: 0.0 for dimension in Dimension}
    identity = {
        "posterior_kind": "PRIOR_ZERO_EVIDENCE",
        "prior_version": prior.version,
        "prior_content_hash": prior.content_hash,
        "sex": sex,
        "constraints": [],
    }
    digest = hashlib.sha256(
        json.dumps(identity, sort_keys=True, ensure_ascii=False).encode("utf-8")
    ).hexdigest()
    return Posterior(
        character_id=character_id,
        sex=sex,
        sex_source=sex_source,
        prior_version=prior.version,
        prior_content_hash=prior.content_hash,
        constraint_count=0,
        posterior_hash=digest,
        evidence_sufficiency=sufficiency,
        effective_sample_size=None,  # no particle filter ran: there was nothing to weigh
    )


def seed_character(
    world_seed: int,
    character_id: str,
    prior: PopulationPrior,
    *,
    constraints: Sequence[Any] = (),
    covariates: CohortCovariates | None = None,
    store: CapacityBaselineStore | None = None,
    log: EventLog | None = None,
) -> CapacityBaselineRecord:
    """Draw, freeze and record one body."""
    if store is not None and character_id in store:
        raise AlreadySeededError(
            f"{character_id} already has a capacity_baseline in this run (failure mode F5)"
        )
    posterior = posterior_for(world_seed, character_id, prior, constraints, covariates)
    if log is not None:
        _assert_matches_run(log, world_seed=world_seed, posterior=posterior)
    name = substream_name(character_id, SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE)
    generator = substream(world_seed, character_id, SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE)
    profile = prior.sample_profile(generator, posterior.sex)

    record = CapacityBaselineRecord(
        character_id=character_id,
        sim_time=Y1_START,
        profile=profile,
        posterior_hash=posterior.posterior_hash,
        prior_version=posterior.prior_version,
        substream=name,
        #: Frozen with the body, not looked up afterwards: `posterior_hash` is
        #: equal for a known male and a drawn male on purpose, so provenance that
        #: is not carried here cannot be recovered from the hash later.
        cohort_sex=posterior.sex,
        cohort_sex_source=posterior.sex_source,
        evidence_sufficiency=posterior.evidence_sufficiency,
    )
    if store is not None:
        store.put(record)
    if log is not None:
        log.append(EVENT_CAPACITY_SAMPLED, capacity_sampled_payload(record, posterior))
    return record


def _assert_matches_run(log: EventLog, *, world_seed: int, posterior: Posterior) -> None:
    """Refuse to append a sample the log's own `run.started` cannot account for.

    Checked **before** the draw is written, and before it reaches the store: the
    log is the audit artefact, and the moment it holds one entry that disagrees
    with its metadata, no reader can tell which of the two is the run that
    happened. The three fields are exactly the ones a replay depends on.
    """
    metadata = log.metadata
    if metadata.world_seed != world_seed:
        raise RunContextMismatchError(
            f"{posterior.character_id} was seeded under world_seed {world_seed}, but this log "
            f"declares world_seed {metadata.world_seed} in run.started"
        )

    declared_prior = metadata.component_versions.get("prior")
    if declared_prior != posterior.prior_version:
        raise RunContextMismatchError(
            f"{posterior.character_id} was seeded from prior {posterior.prior_version}, but "
            f"this log declares prior_version {declared_prior!r} in run.started"
        )

    declared_hash = metadata.posterior_hash_by_character.get(posterior.character_id)
    if declared_hash != posterior.posterior_hash:
        raise RunContextMismatchError(
            f"{posterior.character_id} has posterior_hash {posterior.posterior_hash}, but this "
            f"log declares {declared_hash!r} for it in run.started. The metadata of spec §9.2 "
            f"is complete before the log opens precisely so that this cannot drift."
        )


def capacity_sampled_payload(record: CapacityBaselineRecord, posterior: Posterior) -> dict[str, Any]:
    """The `capacity.sampled` record of spec §5.4.

    It carries the sampled values as well as the required fields: the log is the
    audit artefact, and a sample nobody can inspect cannot be audited. These
    numbers live here and in the snapshot, and reach no prompt (spec §7.1).
    """
    return {
        "character_id": record.character_id,
        "posterior_hash": record.posterior_hash,
        "prior_version": record.prior_version,
        "prior_content_hash": posterior.prior_content_hash,
        "posterior_kind": "PRIOR_ZERO_EVIDENCE" if posterior.constraint_count == 0 else "POSTERIOR",
        "substream": record.substream,
        "sex": record.cohort_sex,
        "sex_source": record.cohort_sex_source,
        "constraint_count": posterior.constraint_count,
        "estimator_ess": posterior.effective_sample_size,
        "evidence_sufficiency": {
            dimension.value: sufficiency
            for dimension, sufficiency in record.evidence_sufficiency.items()
        },
        "dimensions": {
            dimension.value: {"value": entry.value, "unit": entry.unit}
            for dimension, entry in record.profile.dimensions.items()
        },
    }


def seed_cohort(
    world_seed: int,
    character_ids: Sequence[str],
    prior: PopulationPrior,
    *,
    covariates: Mapping[str, CohortCovariates] | None = None,
    store: CapacityBaselineStore | None = None,
    log: EventLog | None = None,
) -> CapacityBaselineStore:
    """Seed a whole cohort. Order of iteration changes nothing but the log order.

    `covariates` is the explicit-composition path `population-prior.yaml` asks
    for: the file's sex ratio is an `[INT]` equal split, and a cohort standing in
    for a specific class states its composition here instead of letting that
    assumption decide it. A character the cohort does not contain is an error
    rather than a no-op, for the same reason `posterior_for` refuses constraints
    it cannot consume: silently dropping a stated composition is indistinguishable
    from having honoured it.
    """
    store = store if store is not None else CapacityBaselineStore()
    covariates = {} if covariates is None else dict(covariates)
    unknown = sorted(set(covariates) - set(character_ids))
    if unknown:
        raise UnknownCovariateSubjectError(
            f"covariates supplied for characters outside the cohort: {unknown}. "
            f"A composition that names nobody in the cohort was not applied, and a "
            f"seeding that ignored it would look identical to one that used it."
        )
    for character_id in character_ids:
        seed_character(
            world_seed,
            character_id,
            prior,
            covariates=covariates.get(character_id),
            store=store,
            log=log,
        )
    return store
