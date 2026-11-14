"""Versioned PSV1-2 physical operation intent + durable completion receipt."""

from __future__ import annotations

import copy
import hashlib
import json
from pathlib import Path

import pytest
import yaml

from embodiment.cli import advance_clock_command, seed_cohort_command
from embodiment.continuity import PhysicalHistoryError, replay_body_history, verify_body_history
from embodiment.eventlog import (
    EVENT_BODY_ADVANCED, EVENT_BODY_ADVANCE_INTENT,
    EVENT_BODY_ADVANCE_COMPLETE, EVENT_RUN_EXTENDED,
    RunContinuityError,
)
from embodiment.types import SleepQuality, as_document


def _seed(root: Path) -> Path:
    out = root / "run"
    seed_cohort_command(world_seed=42, count=3, out_dir=out)
    return out


def _files(out: Path) -> tuple[bytes, bytes]:
    return (out / "snapshot.yaml").read_bytes(), (out / "events.jsonl").read_bytes()


def _records(out: Path) -> list[dict]:
    return [json.loads(line) for line in _files(out)[1].splitlines()]


def _log(rows: list[dict]) -> bytes:
    return b"".join(
        (json.dumps(r, sort_keys=True, ensure_ascii=False, allow_nan=False) + "\n").encode()
        for r in rows
    )


def _advance(out: Path, *, days=2, sleep="4h", character="npc.0001", quality=SleepQuality.POOR):
    return advance_clock_command(
        run_dir=out, character_id=character, days=days,
        sleep=sleep, quality=quality,
    )


@pytest.mark.parametrize(("days", "sleep"), [
    (1, "0h"), (1, "24h"), (2, "4h"), (3, "8h"),
    (2, "0.1h"), (2, "7.3h"),
])
def test_real_command_carries_intent_terminal_and_exact_body_receipt(tmp_path, days, sleep):
    out = _seed(tmp_path)
    result = _advance(out, days=days, sleep=sleep)
    prior = _files(out)
    records = _records(out)
    intents = [r for r in records if r["event"] == EVENT_BODY_ADVANCE_INTENT]
    terminal = [r for r in records if r["event"] == EVENT_BODY_ADVANCE_COMPLETE]
    advances = [r for r in records if r["event"] == EVENT_BODY_ADVANCED]
    assert len(intents) == len(terminal) == 1
    assert len(advances) == len(result["advances"])
    assert terminal[0]["seq"] == advances[-1]["seq"] + 1
    assert terminal[0]["payload"]["intent_seq"] == intents[0]["seq"]
    assert terminal[0]["payload"]["emitted_event_count"] == len(advances)
    state = as_document(result["after"])
    expected = hashlib.sha256(
        json.dumps(state, sort_keys=True, ensure_ascii=False, separators=(",", ":")).encode()
    ).hexdigest()
    assert terminal[0]["payload"]["final_body_sha256"] == expected
    prefix = replay_body_history(*prior)
    assert prefix.all_phases_complete is True
    assert prefix.completed_phase_count == 1
    assert prefix.completed_transition_count == len(advances)
    assert prefix.body_advance_count == len(advances)
    assert as_document(prefix.bodies["npc.0001"]) == state
    assert _files(out) == prior


def test_two_completed_characters_share_source_without_foreign_state_mix(tmp_path):
    out = _seed(tmp_path)
    one = _advance(out, days=2, character="npc.0001")
    two = _advance(out, days=3, character="npc.0002", sleep="8h")
    result = verify_body_history(out)
    assert result.all_phases_complete
    assert result.completed_phase_count == 2
    assert result.completed_transition_count == 10
    assert as_document(result.bodies["npc.0001"]) == as_document(one["after"])
    assert as_document(result.bodies["npc.0002"]) == as_document(two["after"])


@pytest.mark.parametrize("cut", ["intent", "first_transition", "all_but_terminal"])
def test_interrupted_command_prefix_never_attests_completion(tmp_path, cut):
    out = _seed(tmp_path)
    _advance(out, days=2)
    snapshot, _ = _files(out)
    records = _records(out)
    intent = next(i for i,r in enumerate(records) if r["event"] == EVENT_BODY_ADVANCE_INTENT)
    body = next(i for i,r in enumerate(records) if r["event"] == EVENT_BODY_ADVANCED)
    if cut == "intent":
        prefix_bytes = _log(records[:intent + 1])
    elif cut == "first_transition":
        prefix_bytes = _log(records[:body + 1])
    else:
        prefix_bytes = _log(records[:-1])
    if cut == "intent":
        with pytest.raises(PhysicalHistoryError):
            replay_body_history(snapshot, prefix_bytes)
    else:
        result = replay_body_history(snapshot, prefix_bytes)
        assert not result.all_phases_complete
        assert result.completed_transition_count == 0
        assert result.completed_phase_count == 0


@pytest.mark.parametrize("mutation", [
    "intent_kind", "intent_count", "intent_days", "intent_sleep",
    "intent_quality", "intent_wbgt", "intent_actor", "intent_origin",
    "intent_dup_field", "term_digest", "term_type_bool", "term_count",
    "term_plan", "term_version", "term_subject", "term_instant",
    "term_first_seq", "term_last_seq", "term_intent_seq",
    "term_extra", "duplicate_terminal", "duplicate_intent",
    "remove_mid_transition", "append_after_terminal",
])
def test_corrupted_or_forged_receipts_are_never_accepted(tmp_path, mutation):
    out = _seed(tmp_path)
    _advance(out, days=2)
    snap, raw = _files(out)
    records = _records(out)
    first = next(i for i,r in enumerate(records) if r["event"] == EVENT_BODY_ADVANCE_INTENT)
    end = next(i for i,r in enumerate(records) if r["event"] == EVENT_BODY_ADVANCE_COMPLETE)
    intent, terminal = records[first], records[end]
    p, finish = intent["payload"], terminal["payload"]
    if mutation == "intent_kind":
        p["protocol"] = "psv1-2.advance.v2"
    elif mutation == "intent_count":
        p["expected_event_count"] += 1
    elif mutation == "intent_days":
        p["planned_days"] += 1
    elif mutation == "intent_sleep":
        p["sleep_hours"] = 8.0
    elif mutation == "intent_quality":
        p["quality"] = "good"
    elif mutation == "intent_wbgt":
        p["wbgt_c"] = 30.0
    elif mutation == "intent_actor":
        p["character_id"] = "npc.0002"
    elif mutation == "intent_origin":
        p["t_hours_before"] += 3
    elif mutation == "intent_dup_field":
        p["extra"] = "fake"
    elif mutation == "term_digest":
        finish["final_body_sha256"] = "f" * 64
    elif mutation == "term_type_bool":
        finish["emitted_event_count"] = True
    elif mutation == "term_count":
        finish["emitted_event_count"] -= 1
    elif mutation == "term_plan":
        finish["planned_days"] += 1
    elif mutation == "term_version":
        finish["dynamics_version"] = "forged"
    elif mutation == "term_subject":
        finish["character_id"] = "npc.0002"
    elif mutation == "term_instant":
        terminal["sim_time"] = "Y1_START+00100.000h"
    elif mutation == "term_first_seq":
        finish["first_advance_seq"] += 1
    elif mutation == "term_last_seq":
        finish["last_advance_seq"] += 1
    elif mutation == "term_intent_seq":
        finish["intent_seq"] -= 1
    elif mutation == "term_extra":
        finish["WorldRevision"] = 99
    elif mutation == "duplicate_terminal":
        records.append(copy.deepcopy(terminal))
        records[-1]["seq"] = len(records) - 1
    elif mutation == "duplicate_intent":
        records.insert(first + 1, copy.deepcopy(intent))
        for index, row in enumerate(records):
            row["seq"] = index
    elif mutation == "remove_mid_transition":
        records.pop(first+2)
        for index, row in enumerate(records):
            row["seq"] = index
    elif mutation == "append_after_terminal":
        records.append(copy.deepcopy(records[end-1]))
        records[-1]["seq"] = len(records) - 1
    with pytest.raises(PhysicalHistoryError):
        replay_body_history(snap, _log(records))
    assert _files(out) == (snap, raw)


def test_no_subsequent_phase_can_cover_a_crashed_prior_phase(tmp_path):
    out = _seed(tmp_path)
    _advance(out, days=2)
    records = _records(out)
    (out / "events.jsonl").write_bytes(_log(records[:-1]))
    before = _files(out)
    with pytest.raises(RunContinuityError, match="incomplete"):
        _advance(out, character="npc.0002", days=1)
    assert _files(out) == before


def test_terminal_sync_is_executed_after_last_append(tmp_path, monkeypatch):
    from embodiment import eventlog

    out = _seed(tmp_path)
    invoked = []
    native = eventlog.os.fsync

    def spy(fd):
        invoked.append(fd)
        native(fd)

    monkeypatch.setattr(eventlog.os, "fsync", spy)
    _advance(out)
    assert len(invoked) == 1
    assert _records(out)[-1]["event"] == EVENT_BODY_ADVANCE_COMPLETE


def test_seed_only_and_zero_days_have_no_completion_false_claim(tmp_path):
    out = _seed(tmp_path)
    original = _files(out)
    p = verify_body_history(out)
    assert not p.all_phases_complete and p.completed_phase_count == 0
    _advance(out, days=0)
    assert _files(out) == original



@pytest.mark.parametrize("bad_days", [True, False, -1, 50001, 2.5, "2"])
def test_invalid_day_plan_is_rejected_before_any_file_is_changed(tmp_path, bad_days):
    out = _seed(tmp_path)
    before = _files(out)
    with pytest.raises(ValueError, match="days"):
        _advance(out, days=bad_days)
    assert _files(out) == before


@pytest.mark.parametrize("bad_wbgt", [101.0, -101.0, float("nan"), float("inf")])
def test_unsupported_temperature_cannot_create_unreplayable_footer(tmp_path, bad_wbgt):
    out = _seed(tmp_path)
    before = _files(out)
    with pytest.raises(ValueError, match="wbgt"):
        advance_clock_command(run_dir=out, character_id="npc.0001",
                              days=1, wbgt_c=bad_wbgt)
    assert _files(out) == before
