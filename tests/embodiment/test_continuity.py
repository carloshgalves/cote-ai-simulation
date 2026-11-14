"""PSV1-2 verified *prefix* replay against the actual Embodiment writer."""

from __future__ import annotations

import copy
import json
from pathlib import Path

import pytest
import yaml

from embodiment.cli import advance_clock_command, main, seed_cohort_command
from embodiment.continuity import PhysicalHistoryError, replay_body_history, verify_body_history
from embodiment.eventlog import (EVENT_BODY_ADVANCED, EVENT_RUN_EXTENDED, EventLog, logical_sim_time)
from embodiment.dynamics import BodyTraits, Environment, Exposure, advance, channel_diff, load_params
from embodiment.snapshot import body_state_of, capacity_profile_of, read_snapshot
from embodiment.types import RunMetadata
from embodiment.snapshot import snapshot_hash
from embodiment.types import SleepQuality, as_document


def _seed(root: Path, count: int = 3) -> Path:
    out = root / "run"
    seed_cohort_command(world_seed=42, count=count, out_dir=out)
    return out


def _files(out: Path) -> tuple[bytes, bytes]:
    return (out / "snapshot.yaml").read_bytes(), (out / "events.jsonl").read_bytes()


def _write_records(records: list[dict]) -> bytes:
    return b"".join(
        (json.dumps(record, ensure_ascii=False, sort_keys=True, allow_nan=False) + "\n").encode()
        for record in records
    )


def _records(log: bytes) -> list[dict]:
    return [json.loads(row) for row in log.decode().splitlines()]


def _signed(doc: dict) -> bytes:
    data = copy.deepcopy(doc)
    data["snapshot_hash"] = snapshot_hash(data)
    return yaml.safe_dump(data, sort_keys=True, allow_unicode=True).encode()


def _mutated(root: Path, name: str) -> tuple[bytes, bytes]:
    out = _seed(root)
    advance_clock_command(run_dir=out, character_id="npc.0001", days=2, sleep="4h",
                          quality=SleepQuality.POOR)
    snap, log = _files(out)
    data, rec = yaml.safe_load(snap), _records(log)
    first = next(i for i, r in enumerate(rec) if r["event"] == EVENT_BODY_ADVANCED)
    advance = rec[first]
    phase = rec[first - 1]
    p = advance["payload"]
    if name == "seq_gap":
        advance["seq"] = 900
    elif name == "seq_duplicate":
        advance["seq"] = phase["seq"]
    elif name == "wrong_local_time":
        advance["sim_time"] = "Y1_START+00100.000h"
    elif name == "wrong_before":
        p["t_hours_before"] += 1
    elif name == "wrong_after":
        p["t_hours_after"] += 1
    elif name == "wrong_diff":
        p["channels_changed"]["sleep"]["after"]["debt_hours"] = 0.0
    elif name == "wrong_duration":
        p["dt_hours"] = 3
    elif name == "negative_duration":
        p["dt_hours"] = -1
    elif name == "nonfinite_duration":
        p["dt_hours"] = 1234567
        # Leave the log JSON-valid but overflow the decoder's float during parsing.
    elif name == "wrong_load":
        p["load"]["intensity"] = 0.7
    elif name == "wrong_environment":
        p["environment"]["wbgt_c"] += 5
    elif name == "environment_extra":
        p["environment"]["WorldRevision"] = 3
    elif name == "wrong_version":
        p["dynamics_version"] = "forged"
    elif name == "wrong_subject":
        p["character_id"] = "npc.9999"
    elif name == "switch_subject_mid_phase":
        rec[first + 1]["payload"]["character_id"] = "npc.0002"
    elif name == "unknown_payload":
        p["EventOrderKey"] = "local"
    elif name == "wrong_ext_metadata":
        phase["payload"]["run_metadata"]["prior_version"] = "forged"
    elif name == "wrong_ext_time":
        phase["sim_time"] = "Y1_START+00001.000h"
    elif name == "unknown_event":
        advance["event"] = "world.commit"
    elif name == "missing_seed_sample":
        rec.pop(1)
        for index, r in enumerate(rec):
            r["seq"] = index
    elif name == "tampered_initial_body":
        data["characters"]["npc.0001"]["body_state"]["hydration"]["deficit_pct_body_mass"] = 0.4
        snap = _signed(data)
    elif name == "tampered_baseline":
        data["characters"]["npc.0001"]["capacity_baseline"]["dimensions"]["body_mass"]["value"] += 5
        snap = _signed(data)
    elif name == "unknown_snapshot_version":
        data["snapshot_version"] = 2
        snap = _signed(data)
    elif name == "wrong_seed_provenance":
        data["world_seed"] = 43
        data["run_metadata"]["world_seed"] = 43
        rec[0]["payload"]["run_metadata"]["world_seed"] = 43
        for record in rec:
            if record["event"] == EVENT_RUN_EXTENDED:
                record["payload"]["run_metadata"]["world_seed"] = 43
        snap = _signed(data)
    elif name == "snapshot_extra":
        data["WorldRevision"] = 2
        snap = _signed(data)
    elif name == "incomplete_phase":
        rec = rec[:first]
    else:
        raise AssertionError(f"unknown mutation {name}")
    if name == "nonfinite_duration":
        raw = _write_records(rec).replace(b'"dt_hours": 1234567', b'"dt_hours": 1e400')
    else:
        raw = _write_records(rec)
    return snap, raw


def test_seed_only_replays_to_original_body_without_mutating_files(tmp_path):
    out = _seed(tmp_path)
    before = _files(out)
    result = verify_body_history(out)
    assert _files(out) == before
    assert result.status == "VERIFIED_LOCAL_PREFIX_ONLY"
    assert result.body_advance_count == 0
    assert result.log_record_count == 5
    assert set(result.bodies) == {"npc.0001", "npc.0002", "npc.0003"}
    assert all(body.t_hours == 0 for body in result.bodies.values())
    assert all(number == 0 for number in result.advances_by_character.values())
    with pytest.raises(TypeError):
        result.bodies["npc.0001"] = result.bodies["npc.0002"]  # type: ignore[index]


@pytest.mark.parametrize(("days", "sleep", "quality"), [
    (1, "4h", SleepQuality.POOR),
    (2, "8h", SleepQuality.GOOD),
    (3, "0h", SleepQuality.FAIR),
    (2, "24h", SleepQuality.FAIR),
])
def test_real_advance_replays_exact_original_body_and_preserves_bytes(
    tmp_path, days, sleep, quality
):
    out = _seed(tmp_path)
    advanced = advance_clock_command(run_dir=out, character_id="npc.0001", days=days,
                                     sleep=sleep, quality=quality)
    before = _files(out)
    result = replay_body_history(*before)
    assert _files(out) == before
    assert as_document(result.bodies["npc.0001"]) == as_document(advanced["after"])
    assert result.body_advance_count == len(advanced["advances"])
    assert result.advances_by_character["npc.0001"] == len(advanced["advances"])
    assert result.bodies["npc.0002"].t_hours == 0
    assert result.bodies["npc.0003"].t_hours == 0


def test_multiple_characters_and_extensions_use_independent_previous_state(tmp_path):
    out = _seed(tmp_path)
    first = advance_clock_command(run_dir=out, character_id="npc.0003", days=3, sleep="4h",
                                  quality=SleepQuality.POOR)
    second = advance_clock_command(run_dir=out, character_id="npc.0001", days=2, sleep="8h")
    third = advance_clock_command(run_dir=out, character_id="npc.0002", days=1, sleep="0h")
    result = verify_body_history(out)
    assert as_document(result.bodies["npc.0003"]) == as_document(first["after"])
    assert as_document(result.bodies["npc.0001"]) == as_document(second["after"])
    assert as_document(result.bodies["npc.0002"]) == as_document(third["after"])
    assert result.body_advance_count == 6 + 4 + 1
    assert result.advances_by_character == {"npc.0001": 4, "npc.0002": 1, "npc.0003": 6}


def test_verified_prefix_is_not_a_claim_of_finished_command(tmp_path):
    out = _seed(tmp_path)
    advance_clock_command(run_dir=out, character_id="npc.0001", days=3, sleep="4h")
    snap, log = _files(out)
    rec = _records(log)
    first = next(i for i, r in enumerate(rec) if r["event"] == EVENT_BODY_ADVANCED)
    # An append-only crash could happen here; that prefix is mathematically
    # checkable, but the log cannot certify the command's intended last event.
    prefix = _write_records(rec[:first + 1])
    result = replay_body_history(snap, prefix)
    assert result.body_advance_count == 1
    assert result.status == "VERIFIED_LOCAL_PREFIX_ONLY"
    assert result.bodies["npc.0001"].t_hours == rec[first]["payload"]["t_hours_after"]


@pytest.mark.parametrize("mutation", [
    "seq_gap", "seq_duplicate", "wrong_local_time", "wrong_before", "wrong_after",
    "wrong_diff", "wrong_duration", "negative_duration", "nonfinite_duration",
    "wrong_load", "wrong_environment", "environment_extra", "wrong_version",
    "wrong_subject", "switch_subject_mid_phase", "unknown_payload",
    "wrong_ext_metadata", "wrong_ext_time", "unknown_event", "missing_seed_sample",
    "tampered_initial_body", "tampered_baseline", "unknown_snapshot_version",
    "wrong_seed_provenance", "snapshot_extra", "incomplete_phase",
])
def test_tampered_real_history_is_rejected_without_side_effects(tmp_path, mutation):
    snap, log = _mutated(tmp_path, mutation)
    original = (snap, log)
    with pytest.raises((PhysicalHistoryError, ValueError)):
        replay_body_history(snap, log)
    assert (snap, log) == original


@pytest.mark.parametrize("corruption", [
    b"", b"bad\n", b"{}", b"{}\n", b"{not json}\n",
    b'\xff\n', b'{"seq":0,"seq":0}\n', b'{"seq":0,"bad":NaN}\n',
])
def test_malformed_or_missing_jsonl_rejected(tmp_path, corruption):
    out = _seed(tmp_path)
    snap, _ = _files(out)
    with pytest.raises((PhysicalHistoryError, ValueError)):
        replay_body_history(snap, corruption)


def test_duplicate_key_in_complete_valid_body_advanced_record_rejected(tmp_path):
    out = _seed(tmp_path)
    advance_clock_command(run_dir=out, character_id="npc.0001", days=1)
    snap, log = _files(out)
    rows = log.splitlines(keepends=True)
    index = next(i for i, r in enumerate(rows) if b'"event": "body.advanced"' in r)
    row = rows[index]
    value = b'"event": "body.advanced"'
    assert row.count(value) == 1
    rows[index] = row.replace(value, value + b', ' + value, 1)
    with pytest.raises(PhysicalHistoryError, match="duplicate JSON mapping key"):
        replay_body_history(snap, b"".join(rows))


def test_terminal_partial_line_and_duplicate_yaml_are_refused(tmp_path):
    out = _seed(tmp_path)
    snap, log = _files(out)
    with pytest.raises(PhysicalHistoryError, match="terminal LF"):
        replay_body_history(snap, log.rstrip(b"\n"))
    with pytest.raises(PhysicalHistoryError, match="duplicate YAML"):
        replay_body_history(snap + b"\nsnapshot_version: 1\n", log)


def test_cli_inspector_is_read_only_and_does_not_print_body_truth(tmp_path, capsys):
    out = _seed(tmp_path)
    advance_clock_command(run_dir=out, character_id="npc.0001", days=1)
    original = _files(out)
    assert main(["verify-history", "--run", str(out)]) == 0
    output = capsys.readouterr().out
    assert "VERIFIED_LOCAL_PREFIX_ONLY" in output
    assert "body.advanced" in output
    assert "central_fatigue" not in output
    assert "capacity_baseline" not in output
    assert _files(out) == original



@pytest.mark.parametrize("mutation", ["bool_sample_count", "bool_body_zero", "int_environment", "bool_diff_zero"])
def test_type_coercion_mutants_are_rejected_when_semantically_equal(tmp_path, mutation):
    out = _seed(tmp_path)
    advance_clock_command(run_dir=out, character_id="npc.0001", days=1, sleep="4h")
    snap, log = _files(out)
    doc, rec = yaml.safe_load(snap), _records(log)
    index = next(i for i, item in enumerate(rec) if item["event"] == EVENT_BODY_ADVANCED)
    if mutation == "bool_sample_count":
        rec[1]["payload"]["constraint_count"] = False  # False == 0 in Python
    elif mutation == "bool_body_zero":
        doc["characters"]["npc.0001"]["body_state"]["central_fatigue"] = False
        snap = _signed(doc)  # A valid self-signed YAML remains a forged original.
    elif mutation == "int_environment":
        rec[index]["payload"]["environment"]["wbgt_c"] = 21  # 21 == 21.0
    elif mutation == "bool_diff_zero":
        rec[index]["payload"]["channels_changed"]["sleep"]["before"]["debt_hours"] = False
    with pytest.raises(PhysicalHistoryError):
        replay_body_history(snap, _write_records(rec))


def _non_cli_schedule(out, intervals):
    snapshot = read_snapshot(out / "snapshot.yaml")
    metadata = RunMetadata.from_document(snapshot["run_metadata"])
    section = snapshot["characters"]["npc.0001"]
    profile = capacity_profile_of(section)
    state = body_state_of(section)
    traits = BodyTraits.from_profile(profile)
    params = load_params()
    with EventLog.extend(
        out / "events.jsonl",
        metadata=metadata,
        required_components=("prior", "dynamics"),
    ) as event_log:
        for hours, wbgt in intervals:
            environment = Environment(wbgt_c=wbgt)
            exposure = Exposure.of(environment=environment, traits=traits, params=params)
            next_state = advance(state, hours, exposure)
            event_log.append(
                EVENT_BODY_ADVANCED,
                {
                    "character_id": "npc.0001",
                    "dt_hours": hours,
                    "segment": "awake",
                    "t_hours_before": state.t_hours,
                    "t_hours_after": next_state.t_hours,
                    "environment": as_document(environment),
                    "load": as_document(exposure.load),
                    "dynamics_version": params.model_version,
                    "channels_changed": channel_diff(state, next_state),
                },
                sim_time=logical_sim_time("Y1_START", next_state.t_hours),
            )
            state = next_state


@pytest.mark.parametrize(
    ("intervals", "pattern"),
    [
        ([(12.0, 21.0), (12.0, 21.0)], "day plan"),
        ([(24.0, 21.0), (24.0, 25.0)], "24-hour physical segment"),
        ([(23.0, 21.0), (24.0, 21.0)], "day"),
    ],
)
def test_self_consistent_physical_receipts_cannot_forge_cli_day_plan(tmp_path, intervals, pattern):
    out = _seed(tmp_path)
    _non_cli_schedule(out, intervals)
    with pytest.raises(PhysicalHistoryError, match=pattern):
        verify_body_history(out)


def test_large_json_integer_duration_fails_as_physical_error_not_overflow(tmp_path):
    out = _seed(tmp_path)
    advance_clock_command(run_dir=out, character_id="npc.0001", days=1)
    snap, log = _files(out)
    records = _records(log)
    first = next(item for item in records if item["event"] == EVENT_BODY_ADVANCED)
    first["payload"]["dt_hours"] = 10 ** 400
    with pytest.raises(PhysicalHistoryError, match="dt_hours"):
        replay_body_history(snap, _write_records(records))


def test_domain_invalid_but_self_signed_body_has_sanitized_error(tmp_path):
    out = _seed(tmp_path)
    snap, log = _files(out)
    doc = yaml.safe_load(snap)
    doc["characters"]["npc.0001"]["body_state"]["hydration"]["deficit_pct_body_mass"] = -10.0
    with pytest.raises(PhysicalHistoryError, match="invalid stored physical body/capacity"):
        replay_body_history(_signed(doc), log)
