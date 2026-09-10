"""The observable results of PSV1-1 and PSV1-2, end to end.

    python -m embodiment seed-cohort --world-seed 42 --n 40 --out runs/demo/
    python -m embodiment advance-clock --run runs/demo/ --character npc.0017 \
        --days 3 --sleep 4h --quality poor

Evidence of what happened is the **event log**, not the terminal output, so every
assertion here reads the log or the snapshot.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from embodiment.capability import capability_available
from embodiment.cli import advance_clock_command, main
from embodiment.dynamics import BodyTraits, Environment
from embodiment.eventlog import (
    EVENT_COHORT_CORRELATION_REPORT,
    EVENT_RUN_STARTED,
    normalised_bytes,
)
from embodiment.snapshot import capacity_profile_of, read_snapshot
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


# --------------------------------------------------------------------------
# PSV1-2 — `advance-clock`
# --------------------------------------------------------------------------


def advance(out_dir: Path, *, character: str = "npc.0017", days: int = 3, sleep: str = "4h", quality: str = "poor") -> int:
    return main(
        [
            "advance-clock",
            "--run", str(out_dir),
            "--character", character,
            "--days", str(days),
            "--sleep", sleep,
            "--quality", quality,
        ]
    )


def test_a_seeded_character_has_a_rested_body_in_the_snapshot(tmp_path: Path) -> None:
    snapshot = read_snapshot(run(tmp_path / "demo") / "snapshot.yaml")
    body = snapshot["characters"]["npc.0001"]["body_state"]
    assert body["character_id"] == "npc.0001"
    assert body["t_hours"] == 0.0
    assert body["central_fatigue"] == 0.0
    assert body["sleep"]["debt_hours"] == 0.0
    assert body["energy"]["substrate_availability"] == 1.0
    assert set(body["peripheral_fatigue"]) == {"legs", "arms", "grip", "core"}
    #: Serialised and empty, by decision 13.5 rather than by omission.
    assert body["illnesses"] == []
    assert snapshot["run_metadata"]["dynamics_version"]


def test_the_command_advances_three_days_of_bad_sleep(tmp_path: Path, capsys) -> None:
    """The ticket's observable result, from the terminal the reader will use."""
    out = run(tmp_path / "demo")
    assert advance(out) == 0
    printed = capsys.readouterr().out
    assert "sleep.debt_hours" in printed
    assert "capability_available" in printed
    assert "coordination" in printed


def test_the_log_gains_one_body_advanced_per_interval(tmp_path: Path) -> None:
    """Evidence is the log: six intervals over three days, with both sides."""
    out = run(tmp_path / "demo")
    advance(out)
    advances = [record for record in events(out) if record["event"] == "body.advanced"]
    assert len(advances) == 6
    payload = advances[0]["payload"]
    assert payload["character_id"] == "npc.0017"
    assert payload["dt_hours"] == 20.0
    assert payload["environment"]["asleep"] is False
    assert payload["channels_changed"]["sleep"]["before"]["debt_hours"] == 0.0
    assert payload["channels_changed"]["sleep"]["after"]["debt_hours"] > 0.0


def test_advancing_zero_days_leaves_the_run_exactly_as_it_was(tmp_path: Path) -> None:
    """The other half of the ticket, and as much its point as the first.

    A command that looked at a body and wrote nothing changed nothing — not the
    body, and not the audit artefact either.
    """
    out = run(tmp_path / "demo")
    before = (out / "events.jsonl").read_bytes()
    snapshot_before = (out / "snapshot.yaml").read_bytes()

    assert advance(out, days=0) == 0

    assert (out / "events.jsonl").read_bytes() == before
    assert (out / "snapshot.yaml").read_bytes() == snapshot_before


def test_advancing_the_same_run_twice_appends_rather_than_forking_it(tmp_path: Path) -> None:
    out = run(tmp_path / "demo")
    advance(out, days=1)
    advance(out, character="npc.0002", days=1)
    records = events(out)
    assert records[0]["event"] == "run.started"
    assert [record["seq"] for record in records] == list(range(len(records)))
    assert len({record["payload"]["character_id"] for record in records if record["event"] == "body.advanced"}) == 2


def test_advancing_the_same_character_twice_is_refused_without_forking_its_history(
    tmp_path: Path,
) -> None:
    out = run(tmp_path / "demo")
    assert advance(out, character="npc.0001", days=1) == 0
    before = (out / "events.jsonl").read_bytes()

    with pytest.raises(RuntimeError, match="npc.0001.*already been advanced.*PSV1-8"):
        advance(out, character="npc.0001", days=1)

    assert (out / "events.jsonl").read_bytes() == before


def test_body_advanced_events_carry_the_logical_instant_the_body_reached(tmp_path: Path) -> None:
    out = run(tmp_path / "demo")
    assert advance(out, character="npc.0001", days=3) == 0
    advances = [record for record in events(out) if record["event"] == "body.advanced"]

    assert [record["sim_time"] for record in advances] == [
        "Y1_START+00020.000h",
        "Y1_START+00024.000h",
        "Y1_START+00044.000h",
        "Y1_START+00048.000h",
        "Y1_START+00068.000h",
        "Y1_START+00072.000h",
    ]
    assert [record["payload"]["t_hours_after"] for record in advances] == [
        20.0,
        24.0,
        44.0,
        48.0,
        68.0,
        72.0,
    ]


def test_event_instants_order_as_written_past_a_hundred_hours(tmp_path: Path) -> None:
    """The property, not six literals: sorting the log by `sim_time` is the log.

    An unpadded offset reads as ordered and is not — `Y1_START+100h` sorts before
    `Y1_START+20h` — and the ticket's own exam week already runs past 100 h. A
    consumer that compares timestamps has to get a wrong answer loudly or not at
    all, so this asserts the whole log's order rather than one run's spelling.
    """
    out = run(tmp_path / "demo")
    assert advance(out, character="npc.0001", days=9) == 0
    records = events(out)

    instants = [record["sim_time"] for record in records]
    assert instants == sorted(instants), instants
    assert any(record["payload"].get("t_hours_after", 0.0) > 100.0 for record in records)

    advances = [record for record in records if record["event"] == "body.advanced"]
    for record in advances:
        rendered = f"Y1_START+{record['payload']['t_hours_after']:09.3f}h"
        assert record["sim_time"] == rendered


def test_capability_is_reported_in_the_environment_the_body_was_advanced_through(
    tmp_path: Path,
) -> None:
    """`--wbgt` has to reach the number the ticket asks the command to print.

    Reading capability in a neutral environment while the body is advanced through
    a hot one prints what this body could do somewhere it is not. Both sides are
    read in the run's own ambient WBGT, so the reported change is what the passage
    of time did and not what walking into the sun did.
    """
    neutral = advance_clock_command(
        run_dir=run(tmp_path / "cool"), character_id="npc.0001", days=1, wbgt_c=21.0
    )
    hot = advance_clock_command(
        run_dir=run(tmp_path / "hot"), character_id="npc.0001", days=1, wbgt_c=34.0
    )

    for dimension in (Dimension.AEROBIC_CAPACITY, Dimension.MAX_STRENGTH, Dimension.COORDINATION):
        assert hot["capability_after"][dimension] < neutral["capability_after"][dimension], dimension
        assert hot["capability_before"][dimension] < neutral["capability_before"][dimension], dimension
    #: Milliseconds: the heat makes this one bigger, not smaller.
    assert (
        hot["capability_after"][Dimension.REACTION_TIME]
        > neutral["capability_after"][Dimension.REACTION_TIME]
    )
    #: A trait the dynamics govern rather than wear down is the same in both.
    assert (
        hot["capability_after"][Dimension.STATURE]
        == neutral["capability_after"][Dimension.STATURE]
    )


def test_the_reported_capability_is_the_one_the_composition_computes(tmp_path: Path) -> None:
    """No second composition: the command reports what `capability.py` returns."""
    out = run(tmp_path / "demo")
    result = advance_clock_command(run_dir=out, character_id="npc.0001", days=1, wbgt_c=34.0)
    section = read_snapshot(out / "snapshot.yaml")["characters"]["npc.0001"]
    profile = capacity_profile_of(section)

    expected = capability_available(
        profile,
        result["after"],
        environment=Environment(wbgt_c=34.0),
        traits=BodyTraits.from_profile(profile),
    )
    assert dict(result["capability_after"]) == pytest.approx(dict(expected))


def test_a_character_outside_the_run_is_named_rather_than_guessed(tmp_path: Path) -> None:
    out = run(tmp_path / "demo")
    with pytest.raises(KeyError, match="npc.9999"):
        advance(out, character="npc.9999")


def test_the_same_seed_and_the_same_days_produce_the_same_advance(tmp_path: Path) -> None:
    first = run(tmp_path / "one")
    second = run(tmp_path / "two")
    advance(first)
    advance(second)
    assert normalised_bytes(first / "events.jsonl") == normalised_bytes(second / "events.jsonl")


@pytest.mark.parametrize(
    ("text", "hours"), [("4h", 4.0), ("30m", 0.5), ("90min", 1.5), ("1.5h", 1.5), ("8", 8.0)]
)
def test_durations_are_read_the_way_the_ticket_writes_them(text: str, hours: float) -> None:
    from embodiment.cli import parse_hours

    assert parse_hours(text) == hours


def test_an_unreadable_duration_is_refused(tmp_path: Path) -> None:
    from embodiment.cli import parse_hours

    with pytest.raises(ValueError, match="duration"):
        parse_hours("a whole night")
