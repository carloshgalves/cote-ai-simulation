"""Snapshot serialisation — `capacity_baseline`, `body_state` and `run_metadata`.

Spec §5.3. `physical_beliefs` arrives with PSV1-6; what PSV1-1 fixed is the
**shape**, and one part of the shape is not stylistic: world truth lives under
`characters`, belief lives in its own tree with an explicit holder.

PSV1-2 adds the second half of world truth: `characters.<id>.body_state`, whole,
including `illnesses: []`, which is serialised and which nothing in V1 writes
(spec §13.5). A body that did not survive the save would make every consequence
in the model scene-local, which is the failure the persistence invariant exists
to prevent. The public writer accepts only the authoritative
seeding inputs and builds the document itself, so a caller cannot sign a raw
mapping that bypassed the world-truth and provenance checks.

`snapshot_version: 1` is introduced here. A snapshot is only loadable while the
parameter versions in its `run_metadata` are available: the bodies in it were
sampled under that prior and do not mean the same thing under another (spec
§10.2), which is also why a recalibrated prior re-runs from the seed rather than
migrating old snapshots.
"""

from __future__ import annotations

import hashlib
import json
from collections.abc import Mapping
from pathlib import Path
from typing import Any

import yaml

from .seeding import CapacityBaselineStore, Posterior
from .types import (
    DIMENSION_UNITS,
    BodyState,
    CapacityProfile,
    Dimension,
    DimensionValue,
    RunMetadata,
    SCHEMA_VERSION,
    SNAPSHOT_VERSION,
    Y1_START,
    as_document,
)

__all__ = [
    "build_snapshot",
    "write_snapshot",
    "read_snapshot",
    "snapshot_hash",
    "body_state_of",
    "capacity_profile_of",
    "SnapshotShapeError",
    "BELIEF_SECTIONS",
]

#: Sections that may never appear under `characters.<id>`. Belief has a holder
#: and world truth does not; putting the two in one tree is how they get fused.
BELIEF_SECTIONS = frozenset(
    {"physical_beliefs", "self_physical_model", "capacity_beliefs", "beliefs"}
)


class SnapshotShapeError(ValueError):
    """The snapshot is not shaped the way the invariants require."""


def build_snapshot(
    *,
    world_seed: int,
    store: CapacityBaselineStore,
    metadata: RunMetadata,
    posteriors: Mapping[str, Posterior],
    bodies: Mapping[str, BodyState] | None = None,
    sim_time: str = Y1_START,
) -> dict[str, Any]:
    validated_bodies = _validated_bodies(bodies)
    _assert_consistent_provenance(
        world_seed=world_seed,
        store=store,
        metadata=metadata,
        posteriors=posteriors,
        bodies=validated_bodies,
    )
    characters: dict[str, Any] = {}
    for character_id, record in sorted(store.items()):
        characters[character_id] = {
            "capacity_baseline": {
                "schema_version": SCHEMA_VERSION,
                "sim_time": record.sim_time,
                "sampled_from": {
                    "posterior_hash": record.posterior_hash,
                    "prior_version": record.prior_version,
                    # Which marginals the copula used. A cohort covariate, not a
                    # capacity dimension: without it the percentile *view* of
                    # this body cannot be recomputed from the snapshot alone.
                    # Read off the frozen record — the authoritative output of
                    # seeding — and not off the posterior argument, which a
                    # caller reconstructs and which hashes the same either way.
                    "cohort_sex": record.cohort_sex,
                    # Whether canon fixed that covariate or the cohort ratio
                    # drew it. Without it the snapshot cannot distinguish a
                    # sourced fact from an `[INT]` assumption of the prior file.
                    "cohort_sex_source": record.cohort_sex_source,
                    "substream": record.substream,
                },
                "evidence_sufficiency": {
                    dimension.value: sufficiency
                    for dimension, sufficiency in record.evidence_sufficiency.items()
                },
                "dimensions": {
                    dimension.value: {"value": entry.value, "unit": entry.unit}
                    for dimension, entry in record.profile.dimensions.items()
                },
            }
        }
        if validated_bodies is not None:
            #: World truth, in the same tree as the baseline it belongs to and in
            #: full — `t_hours` is hours since the snapshot's own `sim_time`
            #: origin, and `illnesses` is present and empty because nothing in V1
            #: writes it (spec §13.5), not because nobody thought about it.
            characters[character_id]["body_state"] = as_document(
                validated_bodies[character_id]
            )

    snapshot: dict[str, Any] = {
        "snapshot_version": SNAPSHOT_VERSION,
        "world_seed": world_seed,
        "sim_time": sim_time,
        "characters": characters,
        "run_metadata": metadata.as_dict(),
    }
    _assert_shape(snapshot)
    return snapshot


def _validated_bodies(
    bodies: Mapping[str, BodyState] | None,
) -> dict[str, BodyState] | None:
    """Reconstruct bodies from plain data before the snapshot attests them.

    A caller can bypass an instance method with Pydantic's low-level construction
    APIs. The persistence boundary therefore trusts neither the class label nor
    ``frozen=True`` and validates the complete aggregate again before hashing.
    """
    if bodies is None:
        return None
    return {
        character_id: BodyState.model_validate(as_document(body))
        for character_id, body in bodies.items()
    }


def _assert_consistent_provenance(
    *,
    world_seed: int,
    store: CapacityBaselineStore,
    metadata: RunMetadata,
    posteriors: Mapping[str, Posterior],
    bodies: Mapping[str, BodyState] | None = None,
) -> None:
    if metadata.world_seed != world_seed:
        raise SnapshotShapeError(
            f"snapshot world_seed {world_seed} disagrees with run_metadata world_seed "
            f"{metadata.world_seed}"
        )

    character_ids = set(store.ids())
    metadata_ids = set(metadata.posterior_hash_by_character)
    if metadata_ids != character_ids:
        raise SnapshotShapeError(
            "run_metadata posterior_hash characters must exactly match snapshot characters: "
            f"metadata_only={sorted(metadata_ids - character_ids)}, "
            f"snapshot_only={sorted(character_ids - metadata_ids)}"
        )

    if bodies is not None:
        body_ids = set(bodies)
        if body_ids != character_ids:
            raise SnapshotShapeError(
                "every character in the snapshot has exactly one body: "
                f"bodies_only={sorted(body_ids - character_ids)}, "
                f"characters_only={sorted(character_ids - body_ids)}"
            )
        for character_id, body in bodies.items():
            if body.character_id != character_id:
                raise SnapshotShapeError(
                    f"characters.{character_id} carries the body of {body.character_id}"
                )
            if not metadata.component_versions.get("dynamics"):
                raise SnapshotShapeError(
                    "a snapshot carrying body_state must declare dynamics_version in "
                    "run_metadata: a body means nothing without the parameter file it "
                    "was advanced under (spec §9.2)"
                )

    metadata_prior_version = metadata.component_versions.get("prior")
    if not metadata_prior_version:
        raise SnapshotShapeError("run_metadata is missing prior_version")

    for character_id, record in store.items():
        try:
            posterior = posteriors[character_id]
        except KeyError:
            raise SnapshotShapeError(
                f"characters.{character_id} has a capacity baseline but no posterior; "
                f"cohort_sex cannot be reconstructed"
            ) from None
        if posterior.character_id != character_id:
            raise SnapshotShapeError(
                f"characters.{character_id} points to posterior for {posterior.character_id}"
            )

        metadata_hash = metadata.posterior_hash_by_character[character_id]
        if len({record.posterior_hash, posterior.posterior_hash, metadata_hash}) != 1:
            raise SnapshotShapeError(
                f"characters.{character_id} posterior_hash disagrees across baseline, "
                f"posterior and run_metadata"
            )
        if len({record.prior_version, posterior.prior_version, metadata_prior_version}) != 1:
            raise SnapshotShapeError(
                f"characters.{character_id} prior_version disagrees across baseline, "
                f"posterior and run_metadata"
            )
        #: `posterior_hash` is deliberately blind to `sex_source`, so a posterior
        #: rebuilt with the other provenance hashes identically. Only the frozen
        #: record says which one actually ran, and a snapshot signed over the
        #: wrong one presents an `[INT]` assumption of the prior as canon.
        if (record.cohort_sex, record.cohort_sex_source) != (posterior.sex, posterior.sex_source):
            raise SnapshotShapeError(
                f"characters.{character_id} cohort_sex provenance disagrees between the "
                f"frozen baseline ({record.cohort_sex}/{record.cohort_sex_source}) and the "
                f"posterior supplied ({posterior.sex}/{posterior.sex_source})"
            )


def _assert_shape(snapshot: Mapping[str, Any]) -> None:
    for character_id, sections in (snapshot.get("characters") or {}).items():
        offending = BELIEF_SECTIONS & set(sections)
        if offending:
            raise SnapshotShapeError(
                f"characters.{character_id} carries belief section(s) {sorted(offending)}. "
                f"World truth and belief are distinct structures with distinct owners "
                f"(CONTEXT.md invariant 1); belief lives in its own tree, keyed by holder."
            )


def snapshot_hash(snapshot: Mapping[str, Any]) -> str:
    """sha256 over the snapshot without its own hash field."""
    body = {key: value for key, value in snapshot.items() if key != "snapshot_hash"}
    canonical = json.dumps(body, sort_keys=True, ensure_ascii=False, default=str)
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest()


def write_snapshot(
    path: str | Path,
    *,
    world_seed: int,
    store: CapacityBaselineStore,
    metadata: RunMetadata,
    posteriors: Mapping[str, Posterior],
    bodies: Mapping[str, BodyState] | None = None,
    sim_time: str = Y1_START,
) -> str:
    """Build, validate and exclusively persist one authoritative snapshot.

    Raw mappings are deliberately not accepted here. Signing a caller-authored
    mapping would create a second writer of physical world truth outside the
    seeding boundary and would make the hash attest bytes rather than validity.
    """
    snapshot = build_snapshot(
        world_seed=world_seed,
        store=store,
        metadata=metadata,
        posteriors=posteriors,
        bodies=bodies,
        sim_time=sim_time,
    )
    digest = snapshot_hash(snapshot)
    payload = {key: value for key, value in snapshot.items() if key != "snapshot_hash"}
    payload["snapshot_hash"] = digest
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("x", encoding="utf-8") as handle:
        handle.write(
            yaml.safe_dump(payload, sort_keys=True, allow_unicode=True, default_flow_style=False)
        )
    return digest


def read_snapshot(path: str | Path) -> dict[str, Any]:
    """Parse and verify a snapshot before returning its data as world truth.

    Deliberately does **not** reconstruct `CapacityBaselineRecord`: rebuilding a
    frozen sample outside `seeding.py` is the door failure mode F3 comes through.
    Rehydration of a run belongs to PSV1-8, and it will come through seeding.

    Hash verification is not rehydration or migration. It is the read-side half
    of the existing snapshot contract: a file whose contents no longer match the
    digest written with them is not authoritative input to any engine phase.
    """
    data = yaml.safe_load(Path(path).read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise SnapshotShapeError(f"{path}: a snapshot must be a mapping at the top level")
    stored_digest = data.get("snapshot_hash")
    if not isinstance(stored_digest, str):
        raise SnapshotShapeError(f"{path}: snapshot_hash is missing or is not a string")
    actual_digest = snapshot_hash(data)
    if stored_digest != actual_digest:
        raise SnapshotShapeError(
            f"{path}: snapshot_hash mismatch: stored {stored_digest}, computed {actual_digest}"
        )
    return data


def capacity_profile_of(section: Mapping[str, Any]) -> CapacityProfile:
    """Read a character's frozen baseline back as a profile — and only a profile.

    Deliberately **not** a `CapacityBaselineRecord`: the provenance of a sample
    is the seeding module's to state, and rebuilding one outside it is the door
    failure mode F3 comes through. What a later phase of a run needs in order to
    compose `capability_available` is the values and their units, which is what
    this returns and all it returns.
    """
    baseline = section.get("capacity_baseline")
    if not isinstance(baseline, Mapping):
        raise SnapshotShapeError("this character has no capacity_baseline in the snapshot")
    dimensions = baseline.get("dimensions")
    if not isinstance(dimensions, Mapping):
        raise SnapshotShapeError("capacity_baseline carries no dimensions")
    return CapacityProfile(
        dimensions={
            dimension: DimensionValue(
                value=float(dimensions[dimension.value]["value"]),
                unit=str(dimensions[dimension.value].get("unit", DIMENSION_UNITS[dimension].unit)),
            )
            for dimension in Dimension
            if dimension.value in dimensions
        }
    )


def body_state_of(section: Mapping[str, Any]) -> BodyState:
    """Read a character's body back out of a snapshot.

    Reading a persisted body is not writing one: this is the only place outside
    `dynamics.py` that may produce a `BodyState`, and it produces exactly the one
    that was written. Every path that *changes* a body still goes through
    `dynamics.advance` with an explicit interval.
    """
    body = section.get("body_state")
    if not isinstance(body, Mapping):
        raise SnapshotShapeError(
            "this character has no body_state in the snapshot: it was written before bodies "
            "were persisted, or by a run that did not seed one"
        )
    return BodyState.model_validate(dict(body))
