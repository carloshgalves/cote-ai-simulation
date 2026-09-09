"""Snapshot serialisation — first half: `capacity_baseline` and `run_metadata`.

Spec §5.3. `body_state` arrives with PSV1-2 and `physical_beliefs` with PSV1-6;
what this ticket fixes is the **shape**, and one part of the shape is not
stylistic: world truth lives under `characters`, belief lives in its own tree
with an explicit holder. The public writer accepts only the authoritative
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
from .types import RunMetadata, SCHEMA_VERSION, SNAPSHOT_VERSION, Y1_START

__all__ = [
    "build_snapshot",
    "write_snapshot",
    "read_snapshot",
    "snapshot_hash",
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
    sim_time: str = Y1_START,
) -> dict[str, Any]:
    _assert_consistent_provenance(
        world_seed=world_seed,
        store=store,
        metadata=metadata,
        posteriors=posteriors,
    )
    characters: dict[str, Any] = {}
    for character_id, record in sorted(store.items()):
        posterior = posteriors[character_id]
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
                    "cohort_sex": posterior.sex,
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

    snapshot: dict[str, Any] = {
        "snapshot_version": SNAPSHOT_VERSION,
        "world_seed": world_seed,
        "sim_time": sim_time,
        "characters": characters,
        "run_metadata": metadata.as_dict(),
    }
    _assert_shape(snapshot)
    return snapshot


def _assert_consistent_provenance(
    *,
    world_seed: int,
    store: CapacityBaselineStore,
    metadata: RunMetadata,
    posteriors: Mapping[str, Posterior],
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
    """Parse a snapshot back as plain data.

    Deliberately does **not** reconstruct `CapacityBaselineRecord`: rebuilding a
    frozen sample outside `seeding.py` is the door failure mode F3 comes through.
    Rehydration of a run belongs to PSV1-8, and it will come through seeding.
    """
    data = yaml.safe_load(Path(path).read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise SnapshotShapeError(f"{path}: a snapshot must be a mapping at the top level")
    return data
