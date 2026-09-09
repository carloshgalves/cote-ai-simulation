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
from .types import CapacityBaselineRecord, Dimension, Y1_START, freeze_mapping

__all__ = [
    "Posterior",
    "CapacityBaselineStore",
    "AlreadySeededError",
    "posterior_for",
    "seed_character",
    "seed_cohort",
    "cohort_ids",
]


class AlreadySeededError(RuntimeError):
    """A body is sampled once. A second sample would be a different character."""


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
) -> Posterior:
    """The distribution a character's body is drawn from.

    The seam PSV1-3 enters through: it will run the SIR here, weighting prior
    particles by typed constraints, and report a real ESS and a real
    `evidence_sufficiency`. Until then, constraints are refused rather than
    ignored — silently dropping evidence would make a posterior that claims to
    have seen it.
    """
    if constraints:
        raise NotImplementedError(
            "constraints are not consumed yet: the capacity estimator is PSV1-3. "
            "Seeding refuses evidence it cannot use rather than dropping it, because a "
            "posterior that silently ignores a constraint is indistinguishable from one "
            "that weighed it and found it uninformative."
        )

    sex = prior.draw_sex(substream(world_seed, character_id, SEEDING_EVENT_ID, PURPOSE_COHORT_SEX))
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
    store: CapacityBaselineStore | None = None,
    log: EventLog | None = None,
) -> CapacityBaselineRecord:
    """Draw, freeze and record one body."""
    if store is not None and character_id in store:
        raise AlreadySeededError(
            f"{character_id} already has a capacity_baseline in this run (failure mode F5)"
        )
    posterior = posterior_for(world_seed, character_id, prior, constraints)
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
        evidence_sufficiency=posterior.evidence_sufficiency,
    )
    if store is not None:
        store.put(record)
    if log is not None:
        log.append(EVENT_CAPACITY_SAMPLED, capacity_sampled_payload(record, posterior))
    return record


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
        "sex": posterior.sex,
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
    store: CapacityBaselineStore | None = None,
    log: EventLog | None = None,
) -> CapacityBaselineStore:
    """Seed a whole cohort. Order of iteration changes nothing but the log order."""
    store = store if store is not None else CapacityBaselineStore()
    for character_id in character_ids:
        seed_character(world_seed, character_id, prior, store=store, log=log)
    return store
