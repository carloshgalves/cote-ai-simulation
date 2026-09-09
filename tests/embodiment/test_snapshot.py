"""Snapshot, event log and the run metadata that must exist before either.

Spec §5.3, §5.4 and §9.2. The load-bearing test here is the boring one: a run
missing a mandatory metadatum fails **at the start**. A run that fails at the end
has already produced numbers nobody can interpret.
"""

from __future__ import annotations

import json

import pytest

from embodiment.eventlog import (
    EVENT_CAPACITY_SAMPLED,
    EVENT_RUN_STARTED,
    NORMALISED_WALL_TIME,
    EventLog,
    normalised_bytes,
)
from embodiment.prior import PopulationPrior
from embodiment.seeding import CapacityBaselineStore, cohort_ids, posterior_for, seed_cohort
from embodiment.snapshot import (
    BELIEF_SECTIONS,
    SnapshotShapeError,
    build_snapshot,
    read_snapshot,
    snapshot_hash,
    write_snapshot,
)
from embodiment.types import Dimension, MissingRunMetadataError, RunMetadata

WORLD_SEED = 42


def make_run(prior: PopulationPrior, size: int = 5):
    ids = cohort_ids(size)
    posteriors = {cid: posterior_for(WORLD_SEED, cid, prior) for cid in ids}
    metadata = RunMetadata.from_mapping(
        {
            "world_seed": WORLD_SEED,
            "component_versions": {"prior": prior.version},
            "posterior_hash_by_character": {
                cid: posterior.posterior_hash for cid, posterior in posteriors.items()
            },
        },
        required_components=("prior",),
    )
    store = seed_cohort(WORLD_SEED, ids, prior)
    return store, posteriors, metadata


# --------------------------------------------------------------------- metadata


@pytest.mark.parametrize(
    ("missing", "expected_field"),
    [
        ("world_seed", "world_seed"),
        ("posterior_hash_by_character", "posterior_hash_by_character"),
        ("component_versions", "prior_version"),
    ],
)
def test_a_run_without_a_mandatory_metadatum_fails_at_the_start(
    prior: PopulationPrior, missing: str, expected_field: str
) -> None:
    raw = {
        "world_seed": WORLD_SEED,
        "component_versions": {"prior": prior.version},
        "posterior_hash_by_character": {"npc.0001": "0" * 64},
    }
    raw.pop(missing)
    with pytest.raises(MissingRunMetadataError) as error:
        RunMetadata.from_mapping(raw, required_components=("prior",))
    assert expected_field in str(error.value)
    assert error.value.field == expected_field


def test_metadata_names_every_component_spec_9_2_requires() -> None:
    """The full §9.2 list is declared from the start, so a later ticket adding a
    component cannot quietly ship without its version."""
    from embodiment.types import SPEC_9_2_COMPONENTS

    assert set(SPEC_9_2_COMPONENTS.values()) == {
        "prior_version",
        "estimator_version",
        "dynamics_version",
        "contest_resolver_version",
        "observation_params_version",
    }


def test_an_unknown_parameter_component_is_refused() -> None:
    with pytest.raises(ValueError, match="unknown parameter component"):
        RunMetadata(world_seed=1, component_versions={"vibes": "1.0.0"})


def test_run_metadata_cannot_drift_after_validation(prior: PopulationPrior) -> None:
    _, _, metadata = make_run(prior)
    before = metadata.as_dict()

    with pytest.raises(TypeError):
        metadata.posterior_hash_by_character["npc.0001"] = "rewritten"  # type: ignore[index]
    with pytest.raises(TypeError):
        metadata.component_versions["prior"] = "other"  # type: ignore[index]

    assert metadata.as_dict() == before


# -------------------------------------------------------------------- event log


def test_the_log_opens_with_the_run_metadata(prior: PopulationPrior, tmp_path) -> None:
    _, _, metadata = make_run(prior)
    path = tmp_path / "events.jsonl"
    with EventLog(path, metadata=metadata, required_components=("prior",)) as log:
        log.append("capacity.sampled", {"character_id": "npc.0001"})
    records = [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()]
    assert records[0]["event"] == EVENT_RUN_STARTED
    assert records[0]["payload"]["run_metadata"]["world_seed"] == WORLD_SEED
    assert records[0]["payload"]["run_metadata"]["prior_version"] == prior.version
    assert [record["seq"] for record in records] == [0, 1]


def test_the_log_refuses_to_open_without_metadata(prior: PopulationPrior, tmp_path) -> None:
    metadata = RunMetadata(world_seed=WORLD_SEED, posterior_hash_by_character={"npc.0001": "x"})
    with pytest.raises(MissingRunMetadataError, match="prior_version"):
        EventLog(tmp_path / "events.jsonl", metadata=metadata, required_components=("prior",))


def test_normalisation_flattens_only_the_wall_clock(prior: PopulationPrior, tmp_path) -> None:
    _, _, metadata = make_run(prior)
    path = tmp_path / "events.jsonl"
    with EventLog(path, metadata=metadata, required_components=("prior",)) as log:
        log.append(EVENT_CAPACITY_SAMPLED, {"character_id": "npc.0001"})
    for record in (json.loads(line) for line in normalised_bytes(path).decode("utf-8").splitlines()):
        assert record["wall_time"] == NORMALISED_WALL_TIME
        assert record["sim_time"] == "Y1_START"


def test_an_existing_event_log_is_never_truncated(prior: PopulationPrior, tmp_path) -> None:
    _, _, metadata = make_run(prior)
    path = tmp_path / "events.jsonl"
    path.write_text("previous audit record\n", encoding="utf-8")

    with pytest.raises(FileExistsError):
        EventLog(path, metadata=metadata, required_components=("prior",))

    assert path.read_text(encoding="utf-8") == "previous audit record\n"


# --------------------------------------------------------------------- snapshot


def test_snapshot_carries_capacity_baseline_and_run_metadata(prior: PopulationPrior, tmp_path) -> None:
    store, posteriors, metadata = make_run(prior)
    snapshot = build_snapshot(
        world_seed=WORLD_SEED, store=store, metadata=metadata, posteriors=posteriors
    )
    digest = write_snapshot(tmp_path / "snapshot.yaml", snapshot)
    written = read_snapshot(tmp_path / "snapshot.yaml")

    assert written["snapshot_version"] == 1
    assert written["snapshot_hash"] == digest
    assert written["world_seed"] == WORLD_SEED
    assert written["run_metadata"]["prior_version"] == prior.version
    assert set(written["characters"]) == set(cohort_ids(5))

    baseline = written["characters"]["npc.0001"]["capacity_baseline"]
    assert set(baseline["dimensions"]) == {dimension.value for dimension in Dimension}
    assert all(entry["unit"] for entry in baseline["dimensions"].values())
    assert baseline["sampled_from"]["prior_version"] == prior.version
    assert len(baseline["sampled_from"]["posterior_hash"]) == 64
    assert set(baseline["evidence_sufficiency"].values()) == {0.0}


def test_body_state_and_beliefs_are_absent_until_their_tickets(prior: PopulationPrior) -> None:
    """PSV1-2 adds `body_state`; PSV1-6 adds `physical_beliefs`. Neither is faked here."""
    store, posteriors, metadata = make_run(prior)
    snapshot = build_snapshot(
        world_seed=WORLD_SEED, store=store, metadata=metadata, posteriors=posteriors
    )
    assert "physical_beliefs" not in snapshot
    for sections in snapshot["characters"].values():
        assert set(sections) == {"capacity_baseline"}


def test_snapshot_refuses_a_character_without_its_posterior(prior: PopulationPrior) -> None:
    store, posteriors, metadata = make_run(prior)
    posteriors.pop("npc.0001")

    with pytest.raises(SnapshotShapeError, match="npc.0001.*posterior"):
        build_snapshot(
            world_seed=WORLD_SEED,
            store=store,
            metadata=metadata,
            posteriors=posteriors,
        )


@pytest.mark.parametrize(
    ("case", "match"),
    [
        ("world_seed", "world_seed"),
        ("metadata_characters", "characters"),
        ("metadata_hash", "posterior_hash"),
        ("metadata_prior", "prior_version"),
        ("posterior_hash", "posterior_hash"),
        ("posterior_prior", "prior_version"),
        ("record_hash", "posterior_hash"),
        ("record_prior", "prior_version"),
    ],
)
def test_snapshot_refuses_contradictory_provenance(
    prior: PopulationPrior, case: str, match: str
) -> None:
    store, posteriors, metadata = make_run(prior)
    world_seed = WORLD_SEED

    if case == "world_seed":
        world_seed += 1
    elif case == "metadata_characters":
        metadata = metadata.model_copy(update={"posterior_hash_by_character": {"npc.other": "x"}})
    elif case == "metadata_hash":
        hashes = dict(metadata.posterior_hash_by_character)
        hashes["npc.0001"] = "wrong"
        metadata = metadata.model_copy(update={"posterior_hash_by_character": hashes})
    elif case == "metadata_prior":
        metadata = metadata.model_copy(update={"component_versions": {"prior": "wrong"}})
    elif case in {"posterior_hash", "posterior_prior"}:
        field = "posterior_hash" if case == "posterior_hash" else "prior_version"
        posteriors = dict(posteriors)
        posteriors["npc.0001"] = posteriors["npc.0001"].model_copy(update={field: "wrong"})
    else:
        field = "posterior_hash" if case == "record_hash" else "prior_version"
        changed = CapacityBaselineStore()
        for character_id, record in store.items():
            if character_id == "npc.0001":
                record = record.__class__.model_construct(**{**record.__dict__, field: "wrong"})
            changed.put(record)
        store = changed

    with pytest.raises(SnapshotShapeError, match=match):
        build_snapshot(
            world_seed=world_seed,
            store=store,
            metadata=metadata,
            posteriors=posteriors,
        )


def test_a_belief_under_characters_is_refused(prior: PopulationPrior, tmp_path) -> None:
    """Invariant 1: a serialiser able to write belief into world truth has lost it."""
    store, posteriors, metadata = make_run(prior)
    snapshot = build_snapshot(
        world_seed=WORLD_SEED, store=store, metadata=metadata, posteriors=posteriors
    )
    for section in sorted(BELIEF_SECTIONS):
        smuggled = json.loads(json.dumps(snapshot))
        smuggled["characters"]["npc.0001"][section] = {"max_strength": {"lower": 30}}
        with pytest.raises(SnapshotShapeError, match="belief"):
            write_snapshot(tmp_path / "snapshot.yaml", smuggled)


def test_the_hash_covers_the_body_and_not_itself(prior: PopulationPrior) -> None:
    store, posteriors, metadata = make_run(prior)
    snapshot = build_snapshot(
        world_seed=WORLD_SEED, store=store, metadata=metadata, posteriors=posteriors
    )
    digest = snapshot_hash(snapshot)
    assert snapshot_hash({**snapshot, "snapshot_hash": digest}) == digest

    moved = json.loads(json.dumps(snapshot))
    moved["characters"]["npc.0001"]["capacity_baseline"]["dimensions"]["max_strength"]["value"] += 1.0
    assert snapshot_hash(moved) != digest
