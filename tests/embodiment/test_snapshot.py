"""Snapshot, event log and the run metadata that must exist before either.

Spec §5.3, §5.4 and §9.2. The load-bearing test here is the boring one: a run
missing a mandatory metadatum fails **at the start**. A run that fails at the end
has already produced numbers nobody can interpret.
"""

from __future__ import annotations

import json

import pytest
import yaml
from pydantic import BaseModel

from embodiment.eventlog import (
    EVENT_CAPACITY_SAMPLED,
    EVENT_RUN_STARTED,
    NORMALISED_WALL_TIME,
    EventLog,
    normalised_bytes,
)
from embodiment.prior import PopulationPrior
from embodiment.seeding import (
    CapacityBaselineStore,
    CohortCovariates,
    RunContextMismatchError,
    cohort_ids,
    posterior_for,
    seed_character,
    seed_cohort,
)
from embodiment.snapshot import (
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


@pytest.mark.parametrize(
    "posterior_hashes",
    [
        {"npc.0001": ""},
        {"npc.0001": "not-a-sha256"},
        {"npc.0001": "G" * 64},
        {"": "0" * 64},
        {"   ": "0" * 64},
    ],
)
def test_invalid_posterior_identity_stops_before_the_log_opens(
    prior: PopulationPrior, tmp_path, posterior_hashes: dict[str, str]
) -> None:
    path = tmp_path / "events.jsonl"
    with pytest.raises(ValueError, match="posterior_hash_by_character"):
        metadata = RunMetadata.from_mapping(
            {
                "world_seed": WORLD_SEED,
                "component_versions": {"prior": prior.version},
                "posterior_hash_by_character": posterior_hashes,
            },
            required_components=("prior",),
        )
        EventLog(path, metadata=metadata, required_components=("prior",))

    assert not path.exists()


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
    metadata = RunMetadata(
        world_seed=WORLD_SEED, posterior_hash_by_character={"npc.0001": "0" * 64}
    )
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
    digest = write_snapshot(
        tmp_path / "snapshot.yaml",
        world_seed=WORLD_SEED,
        store=store,
        metadata=metadata,
        posteriors=posteriors,
    )
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


def test_raw_mapping_cannot_be_signed_as_a_snapshot(tmp_path) -> None:
    forged = {
        "snapshot_version": 1,
        "world_seed": WORLD_SEED,
        "sim_time": "Y1_START",
        "characters": {
            "actor.ayanokouji": {
                "capacity_baseline": {
                    "dimensions": {"max_strength": {"value": 999, "unit": "kg"}}
                }
            }
        },
        "run_metadata": {},
    }
    path = tmp_path / "snapshot.yaml"

    with pytest.raises(TypeError):
        write_snapshot(path, forged)

    assert not path.exists()


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
        ("prior_version", "prior_version"),
        ("character", "posterior_hash"),
    ],
)
def test_a_sample_the_log_cannot_account_for_is_refused(
    prior: PopulationPrior, tmp_path, case: str, match: str
) -> None:
    """The audit artefact may not contradict its own `run.started`.

    `seed_character` takes the seed and the prior as arguments and the log as
    another, so nothing but this check ties the three together. A log whose
    metadata says one seed and whose `capacity.sampled` was drawn under another
    is well formed in every field and reproduces nothing.
    """
    _, _, metadata = make_run(prior, size=1)
    path = tmp_path / "events.jsonl"
    world_seed = WORLD_SEED + 1 if case == "world_seed" else WORLD_SEED
    character_id = "npc.9999" if case == "character" else "npc.0001"
    if case == "prior_version":
        prior = prior.model_copy(update={"version": "9.9.9-other"})

    with EventLog(path, metadata=metadata, required_components=("prior",)) as log:
        with pytest.raises(RunContextMismatchError, match=match):
            seed_character(world_seed, character_id, prior, log=log)

    events = [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()]
    assert [event["event"] for event in events] == [EVENT_RUN_STARTED]


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


def test_a_drawn_covariate_cannot_be_relabelled_as_canon(prior: PopulationPrior) -> None:
    """`posterior_hash` is blind to `sex_source` on purpose; the record is not.

    Conditioning on a known male and drawing a male are the same distribution, so
    a posterior rebuilt from `CohortCovariates` hashes identically and passes
    every version check while claiming the other provenance. What actually
    happened is frozen on the baseline at seeding, and the snapshot is signed
    over that — otherwise it can present an `[INT]` cohort ratio as canon.
    """
    store, posteriors, metadata = make_run(prior, size=1)
    drawn = posteriors["npc.0001"]
    assert drawn.sex_source == "DRAWN"
    assert store.get("npc.0001").cohort_sex_source == "DRAWN"

    relabelled = posterior_for(
        WORLD_SEED, "npc.0001", prior, covariates=CohortCovariates(sex=drawn.sex)
    )
    assert relabelled.posterior_hash == drawn.posterior_hash
    assert relabelled.sex_source == "KNOWN"

    with pytest.raises(SnapshotShapeError, match="cohort_sex provenance"):
        build_snapshot(
            world_seed=WORLD_SEED,
            store=store,
            metadata=metadata,
            posteriors={"npc.0001": relabelled},
        )


def test_the_snapshot_reports_the_provenance_that_actually_ran(prior: PopulationPrior) -> None:
    """A covariate canon fixes reaches the snapshot as `KNOWN`, and says so."""
    ids = cohort_ids(1)
    covariates = {"npc.0001": CohortCovariates(sex="male")}
    posteriors = {
        cid: posterior_for(WORLD_SEED, cid, prior, covariates=covariates.get(cid)) for cid in ids
    }
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
    store = seed_cohort(WORLD_SEED, ids, prior, covariates=covariates)
    snapshot = build_snapshot(
        world_seed=WORLD_SEED, store=store, metadata=metadata, posteriors=posteriors
    )
    sampled_from = snapshot["characters"]["npc.0001"]["capacity_baseline"]["sampled_from"]
    assert sampled_from["cohort_sex"] == "male"
    assert sampled_from["cohort_sex_source"] == "KNOWN"

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


def test_reading_a_snapshot_requires_its_hash(prior: PopulationPrior, tmp_path) -> None:
    store, posteriors, metadata = make_run(prior)
    path = tmp_path / "snapshot.yaml"
    write_snapshot(
        path,
        world_seed=WORLD_SEED,
        store=store,
        metadata=metadata,
        posteriors=posteriors,
    )
    snapshot = yaml.safe_load(path.read_text(encoding="utf-8"))
    snapshot.pop("snapshot_hash")
    path.write_text(
        yaml.safe_dump(snapshot, sort_keys=True, allow_unicode=True), encoding="utf-8"
    )

    with pytest.raises(SnapshotShapeError, match="snapshot_hash"):
        read_snapshot(path)


# --------------------------------------------------------------------------
# PSV1-2 — the body in the snapshot
# --------------------------------------------------------------------------


def make_bodies(store, params=None):
    from embodiment.dynamics import initial_body_state

    return {
        character_id: initial_body_state(character_id, record.profile, params=params)
        for character_id, record in store.items()
    }


def with_dynamics(metadata: RunMetadata, params) -> RunMetadata:
    versions = {**metadata.component_versions, "dynamics": params.model_version}
    return metadata.model_copy(update={"component_versions": versions})


def test_the_snapshot_carries_the_body_whole(prior: PopulationPrior, dynamics_params) -> None:
    """Spec §5.3: `characters.<id>.body_state`, integral, next to its baseline."""
    store, posteriors, metadata = make_run(prior)
    bodies = make_bodies(store, dynamics_params)
    snapshot = build_snapshot(
        world_seed=WORLD_SEED,
        store=store,
        metadata=with_dynamics(metadata, dynamics_params),
        posteriors=posteriors,
        bodies=bodies,
    )
    section = snapshot["characters"]["npc.0001"]
    assert set(section) == {"capacity_baseline", "body_state"}
    body = section["body_state"]
    from embodiment.types import BodyState

    assert set(body) == set(BodyState.model_fields)
    #: Decision 13.5: present, empty, and nothing in V1 writes it.
    assert body["illnesses"] == []
    assert body["injuries"] == []
    assert body["cumulative_load"] == {"acute_7d": 0.0, "chronic_28d": 0.0}


def test_a_body_can_be_read_back_out_of_the_snapshot(prior: PopulationPrior, dynamics_params) -> None:
    from embodiment.snapshot import body_state_of, capacity_profile_of
    from embodiment.types import as_document

    store, posteriors, metadata = make_run(prior)
    bodies = make_bodies(store, dynamics_params)
    snapshot = build_snapshot(
        world_seed=WORLD_SEED,
        store=store,
        metadata=with_dynamics(metadata, dynamics_params),
        posteriors=posteriors,
        bodies=bodies,
    )
    section = snapshot["characters"]["npc.0001"]
    assert as_document(body_state_of(section)) == as_document(bodies["npc.0001"])

    profile = capacity_profile_of(section)
    assert profile == store.get("npc.0001").profile


def test_snapshot_revalidates_a_body_before_signing_world_truth(
    prior: PopulationPrior, dynamics_params
) -> None:
    """Even an object forged around the public copy guard cannot be attested."""
    store, posteriors, metadata = make_run(prior)
    bodies = make_bodies(store, dynamics_params)
    valid = bodies["npc.0001"]
    bodies["npc.0001"] = BaseModel.model_copy(
        valid, update={"central_fatigue": 9.0, "t_hours": -3.0}
    )

    with pytest.raises(ValueError):
        build_snapshot(
            world_seed=WORLD_SEED,
            store=store,
            metadata=with_dynamics(metadata, dynamics_params),
            posteriors=posteriors,
            bodies=bodies,
        )


def test_a_snapshot_with_bodies_must_say_which_dynamics_advanced_them(
    prior: PopulationPrior, dynamics_params
) -> None:
    """A body means nothing without the parameter file it was advanced under."""
    store, posteriors, metadata = make_run(prior)
    with pytest.raises(SnapshotShapeError, match="dynamics_version"):
        build_snapshot(
            world_seed=WORLD_SEED,
            store=store,
            metadata=metadata,  # prior only
            posteriors=posteriors,
            bodies=make_bodies(store, dynamics_params),
        )


def test_every_character_in_the_snapshot_has_exactly_one_body(
    prior: PopulationPrior, dynamics_params
) -> None:
    store, posteriors, metadata = make_run(prior)
    bodies = make_bodies(store, dynamics_params)
    bodies.pop("npc.0002")
    with pytest.raises(SnapshotShapeError, match="exactly one body"):
        build_snapshot(
            world_seed=WORLD_SEED,
            store=store,
            metadata=with_dynamics(metadata, dynamics_params),
            posteriors=posteriors,
            bodies=bodies,
        )


def test_a_body_filed_under_the_wrong_character_is_refused(
    prior: PopulationPrior, dynamics_params
) -> None:
    """The body and the baseline under one key have to be the same person."""
    store, posteriors, metadata = make_run(prior)
    bodies = make_bodies(store, dynamics_params)
    bodies["npc.0002"] = bodies["npc.0001"]
    with pytest.raises(SnapshotShapeError, match="carries the body of"):
        build_snapshot(
            world_seed=WORLD_SEED,
            store=store,
            metadata=with_dynamics(metadata, dynamics_params),
            posteriors=posteriors,
            bodies=bodies,
        )


def test_reading_a_body_from_a_snapshot_that_has_none_says_so(prior: PopulationPrior) -> None:
    from embodiment.snapshot import body_state_of

    store, posteriors, metadata = make_run(prior)
    snapshot = build_snapshot(
        world_seed=WORLD_SEED, store=store, metadata=metadata, posteriors=posteriors
    )
    with pytest.raises(SnapshotShapeError, match="no body_state"):
        body_state_of(snapshot["characters"]["npc.0001"])


# --------------------------------------------------------------------------
# A run is one log: a later phase appends to it, or it is another run
# --------------------------------------------------------------------------


def test_a_later_phase_appends_to_the_run_it_belongs_to(prior, dynamics_params, tmp_path) -> None:
    from embodiment.eventlog import EVENT_RUN_EXTENDED

    store, posteriors, metadata = make_run(prior)
    path = tmp_path / "events.jsonl"
    with EventLog(path, metadata=metadata, required_components=("prior",)) as log:
        seed_character(WORLD_SEED, "npc.0001", prior, log=log)

    extended = with_dynamics(metadata, dynamics_params)
    with EventLog.extend(path, metadata=extended, required_components=("prior", "dynamics")) as log:
        log.append("body.advanced", {"character_id": "npc.0001"})

    records = [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines()]
    assert [record["seq"] for record in records] == list(range(len(records)))
    assert records[0]["event"] == EVENT_RUN_STARTED
    assert any(record["event"] == EVENT_RUN_EXTENDED for record in records)


def test_a_phase_that_declares_another_world_is_refused(prior, tmp_path) -> None:
    """Two phases that disagree about the seed leave a log nobody can read."""
    from embodiment.eventlog import RunContinuityError

    store, posteriors, metadata = make_run(prior)
    path = tmp_path / "events.jsonl"
    with EventLog(path, metadata=metadata, required_components=("prior",)):
        pass

    other = metadata.model_copy(update={"world_seed": WORLD_SEED + 1})
    with pytest.raises(RunContinuityError, match="world_seed"):
        EventLog.extend(path, metadata=other, required_components=("prior",))


def test_a_phase_under_another_parameter_version_is_refused(prior, tmp_path) -> None:
    """Bodies advanced under one version of a file do not mean the same thing
    under another (spec §10.2), so a run may not change one halfway through."""
    from embodiment.eventlog import RunContinuityError

    store, posteriors, metadata = make_run(prior)
    path = tmp_path / "events.jsonl"
    with EventLog(path, metadata=metadata, required_components=("prior",)):
        pass

    other = metadata.model_copy(
        update={"component_versions": {"prior": "9.9.9"}}
    )
    with pytest.raises(RunContinuityError, match="prior_version"):
        EventLog.extend(path, metadata=other, required_components=("prior",))


def test_a_phase_over_another_cohort_is_refused(prior, tmp_path) -> None:
    from embodiment.eventlog import RunContinuityError

    store, posteriors, metadata = make_run(prior)
    path = tmp_path / "events.jsonl"
    with EventLog(path, metadata=metadata, required_components=("prior",)):
        pass

    hashes = {**metadata.posterior_hash_by_character, "npc.9999": "b" * 64}
    with pytest.raises(RunContinuityError, match="cohort"):
        EventLog.extend(
            path,
            metadata=metadata.model_copy(update={"posterior_hash_by_character": hashes}),
            required_components=("prior",),
        )


def test_a_run_metadata_document_reads_back_the_way_it_was_written(prior, dynamics_params) -> None:
    """`as_dict` and `from_document` are inverses, or a later phase cannot
    declare what the first one did."""
    store, posteriors, metadata = make_run(prior)
    extended = with_dynamics(metadata, dynamics_params)
    assert RunMetadata.from_document(extended.as_dict()).as_dict() == extended.as_dict()


def test_event_log_refuses_a_non_finite_payload_without_appending(prior, tmp_path) -> None:
    """Strict JSON is the last guard even when a caller bypasses domain models."""
    store, posteriors, metadata = make_run(prior)
    path = tmp_path / "events.jsonl"
    with EventLog(path, metadata=metadata, required_components=("prior",)) as log:
        before = path.read_bytes()
        with pytest.raises(ValueError):
            log.append("invalid.numeric_payload", {"value": float("nan")})
        assert path.read_bytes() == before
