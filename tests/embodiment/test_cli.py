"""The observable result of PSV1-1, end to end.

    python -m embodiment seed-cohort --world-seed 42 --n 40 --out runs/demo/

Evidence of what happened is the **event log**, not the terminal output, so every
assertion here reads the log or the snapshot.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from embodiment.cli import main
from embodiment.eventlog import (
    EVENT_COHORT_CORRELATION_REPORT,
    EVENT_RUN_STARTED,
    normalised_bytes,
)
from embodiment.snapshot import read_snapshot
from embodiment.types import Dimension


def run(out_dir: Path, *, world_seed: int = 42, count: int = 40) -> Path:
    assert main(["seed-cohort", "--world-seed", str(world_seed), "--n", str(count), "--out", str(out_dir)]) == 0
    return out_dir


def events(out_dir: Path) -> list[dict]:
    return [json.loads(line) for line in (out_dir / "events.jsonl").read_text(encoding="utf-8").splitlines()]


def test_the_command_writes_a_log_and_a_snapshot(tmp_path: Path) -> None:
    out = run(tmp_path / "demo")
    assert (out / "events.jsonl").is_file()
    assert (out / "snapshot.yaml").is_file()


def test_the_log_holds_forty_sampled_bodies_with_no_evidence(tmp_path: Path) -> None:
    """Evidence 3 of the ticket: every `evidence_sufficiency` is 0.0, because
    there is no evidence — not because the field was left unfilled."""
    sampled = [record for record in events(run(tmp_path / "demo")) if record["event"] == "capacity.sampled"]
    assert len(sampled) == 40
    for record in sampled:
        payload = record["payload"]
        assert len(payload["posterior_hash"]) == 64
        assert payload["prior_version"] == "0.1.0-provisional"
        assert payload["posterior_kind"] == "PRIOR_ZERO_EVIDENCE"
        assert payload["substream"].endswith("|world.seeding|capacity.sample")
        sufficiency = payload["evidence_sufficiency"]
        assert set(sufficiency) == {dimension.value for dimension in Dimension}
        assert set(sufficiency.values()) == {0.0}


def test_the_run_starts_with_complete_metadata(tmp_path: Path) -> None:
    first = events(run(tmp_path / "demo"))[0]
    assert first["event"] == EVENT_RUN_STARTED
    metadata = first["payload"]["run_metadata"]
    assert metadata["world_seed"] == 42
    assert metadata["prior_version"]
    assert len(metadata["posterior_hash_by_character"]) == 40


def test_the_same_seed_produces_the_same_log(tmp_path: Path) -> None:
    """Evidence 2: byte-identical after wall clocks are normalised."""
    first = run(tmp_path / "one")
    second = run(tmp_path / "two")
    assert normalised_bytes(first / "events.jsonl") == normalised_bytes(second / "events.jsonl")
    assert read_snapshot(first / "snapshot.yaml") == read_snapshot(second / "snapshot.yaml")


def test_a_different_seed_produces_a_different_world(tmp_path: Path) -> None:
    first = run(tmp_path / "one", world_seed=42)
    second = run(tmp_path / "two", world_seed=43)
    assert normalised_bytes(first / "events.jsonl") != normalised_bytes(second / "events.jsonl")


def test_seeding_forty_one_keeps_the_forty(tmp_path: Path) -> None:
    forty = read_snapshot(run(tmp_path / "forty", count=40) / "snapshot.yaml")
    forty_one = read_snapshot(run(tmp_path / "forty-one", count=41) / "snapshot.yaml")
    assert len(forty_one["characters"]) == 41
    for character_id, sections in forty["characters"].items():
        assert forty_one["characters"][character_id] == sections


def test_the_run_reports_the_correlation_structure_of_the_cohort(tmp_path: Path) -> None:
    """Evidence 4: every run emits a complete report of the sampled structure."""
    reports = [
        record["payload"]
        for record in events(run(tmp_path / "demo"))
        if record["event"] == EVENT_COHORT_CORRELATION_REPORT
    ]
    assert len(reports) == 1
    report = reports[0]
    assert report["cohort_size"] == 40
    by_pair = {tuple(entry["dimensions"]): entry for entry in report["pairs"]}
    assert set(by_pair) == {
        ("max_strength", "body_mass"),
        ("body_mass", "aerobic_capacity"),
        ("sprint_speed", "aerobic_capacity"),
        ("stature", "body_mass"),
        ("max_strength", "aerobic_capacity"),
    }
    assert all(isinstance(entry["expected_from_loadings"], float) for entry in report["pairs"])
    assert all(isinstance(entry["observed"], float) for entry in report["pairs"])
    assert all(isinstance(entry["sign_agrees"], bool) for entry in report["pairs"])
    agreement = report["material_pairs_sign_agreement"]
    assert set(agreement) == {"agree", "disagree"}
    assert agreement["agree"] + agreement["disagree"] > 0


@pytest.mark.parametrize("existing_name", ["events.jsonl", "snapshot.yaml"])
def test_the_command_preserves_an_existing_run_artifact(
    tmp_path: Path, existing_name: str
) -> None:
    out = tmp_path / "demo"
    out.mkdir()
    artifact = out / existing_name
    artifact.write_text("previous audit artifact\n", encoding="utf-8")

    with pytest.raises(FileExistsError, match=existing_name):
        run(out)

    assert artifact.read_text(encoding="utf-8") == "previous audit artifact\n"
    other_name = "snapshot.yaml" if existing_name == "events.jsonl" else "events.jsonl"
    assert not (out / other_name).exists()


def test_the_snapshot_holds_the_frozen_bodies(tmp_path: Path) -> None:
    snapshot = read_snapshot(run(tmp_path / "demo") / "snapshot.yaml")
    assert snapshot["snapshot_version"] == 1
    assert snapshot["sim_time"] == "Y1_START"
    assert len(snapshot["characters"]) == 40
    baseline = snapshot["characters"]["npc.0001"]["capacity_baseline"]
    assert baseline["dimensions"]["max_strength"]["unit"] == "kg"
    assert baseline["sampled_from"]["cohort_sex"] in ("male", "female")
    assert snapshot["snapshot_hash"]


def test_an_empty_cohort_is_refused_clearly(tmp_path: Path) -> None:
    """Rather than failing later on empty run metadata, which names the wrong cause."""
    from embodiment.cli import seed_cohort_command

    with pytest.raises(ValueError, match="at least one student"):
        seed_cohort_command(world_seed=42, count=0, out_dir=tmp_path / "demo")
