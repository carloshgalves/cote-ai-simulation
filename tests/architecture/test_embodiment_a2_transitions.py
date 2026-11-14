"""CSFRV1-9A/A2: real PSV1-2 transition receipts, no duplicate seed parser."""

from __future__ import annotations

import copy
import hashlib
import json
from pathlib import Path

import jsonschema
import pytest
import yaml

from embodiment.cli import advance_clock_command, seed_cohort_command
from embodiment.continuity import replay_body_history
from embodiment.eventlog import normalised_bytes
from embodiment.types import SleepQuality
from tests.architecture.embodiment_a2_transition_reference import (
    MAX_FRAME_BYTES, ROOT, SCHEMA, TransitionEvidenceRejected,
    project_verified_transitions, reference_bytes, verify_projected_transitions,
)

HEX = lambda raw: hashlib.sha256(raw).hexdigest()


def _run(root: Path, *, count=3, days=2, sleep="4h", extra=False):
    out = root / "run"
    seed_cohort_command(world_seed=42, count=count, out_dir=out)
    if days:
        advance_clock_command(
            run_dir=out, character_id="npc.0001", days=days,
            sleep=sleep, quality=SleepQuality.POOR,
        )
    if extra:
        advance_clock_command(
            run_dir=out, character_id="npc.0002", days=1, sleep="8h",
        )
    return out


def _receipts(out: Path):
    snap = (out / "snapshot.yaml").read_bytes()
    events = (out / "events.jsonl").read_bytes()
    return {
        "snapshot_raw_sha256": HEX(snap),
        "events_raw_sha256": HEX(events),
        "snapshot_document_sha256": yaml.safe_load(snap)["snapshot_hash"],
        "normalised_log_sha256": HEX(normalised_bytes(out / "events.jsonl")),
    }


def _project(out: Path):
    receipts = _receipts(out)
    return project_verified_transitions(
        (out / "snapshot.yaml").read_bytes(),
        (out / "events.jsonl").read_bytes(),
        **receipts,
    )


def _verify(out, evidence):
    return verify_projected_transitions(
        evidence,
        (out / "snapshot.yaml").read_bytes(),
        (out / "events.jsonl").read_bytes(),
        **_receipts(out),
    )


def _records(raw):
    return [json.loads(line) for line in raw.decode().splitlines()]


def _jsonl(rows):
    return ("\n".join(json.dumps(x, sort_keys=True, ensure_ascii=False) for x in rows) + "\n").encode()


def _has_no_csf_authority(value):
    forbidden = {
        "LogicalSequence", "SimulationInstant", "EventOrderKey", "WorldRevision",
        "CycleCommit", "EventDraft", "CommitCandidate", "source_id", "producer_unit_key",
        "input_id", "candidate_id", "event_id", "global_sequence", "priority",
        "actor_context", "knowledge_input",
    }
    if isinstance(value, dict):
        assert forbidden.isdisjoint(value)
        for child in value.values():
            _has_no_csf_authority(child)
    elif isinstance(value, list):
        for child in value:
            _has_no_csf_authority(child)


def test_multi_day_transition_evidence_follows_real_verified_prefix(tmp_path):
    out = _run(tmp_path)
    snap = (out / "snapshot.yaml").read_bytes()
    log = (out / "events.jsonl").read_bytes()
    before = (snap, log)
    prefix = replay_body_history(snap, log)
    result = _project(out)
    schema = json.loads(SCHEMA.read_text())
    jsonschema.Draft202012Validator.check_schema(schema)
    jsonschema.Draft202012Validator(schema).validate(result)
    assert result["kind"] == "physical.verified-transition-evidence.v1"
    assert result["authority"] == "ENGINE_ONLY"
    assert result["admission"] == "PENDING_GO_ADMISSION"
    assert result["completion"] == "UNATTESTED_PREFIX"
    assert result["source"]["verified_transition_count"] == prefix.body_advance_count == 4
    assert len(result["subject_tips"]) == 1
    assert result["subject_tips"][0]["verified_advance_count"] == 4
    assert [x["source_local_seq"] for x in result["transitions"]] == [5, 6, 7, 8]
    assert [x["source_segment"] for x in result["transitions"]] == [
        "awake", "asleep", "awake", "asleep"
    ]
    assert result["source"]["normalised_log_sha256"] == _receipts(out)["normalised_log_sha256"]
    assert (out / "snapshot.yaml").read_bytes() == snap
    assert (out / "events.jsonl").read_bytes() == log
    assert reference_bytes(result) == reference_bytes(_project(out))
    _verify(out, result)
    _has_no_csf_authority(result)


@pytest.mark.parametrize(("sleep","days"), [
    ("0h", 1), ("24h", 1), ("7.3h", 2), ("0.1h", 2), ("8h", 3),
])
def test_supported_real_physical_day_patterns_and_numeric_receipts(tmp_path, sleep, days):
    out = _run(tmp_path, sleep=sleep, days=days)
    result = _project(out)
    expected = days * (1 if sleep in ("0h", "24h") else 2)
    assert len(result["transitions"]) == expected
    assert result["subject_tips"][0]["verified_advance_count"] == expected
    _verify(out, result)


def test_multiple_independent_subjects_have_distinct_tips(tmp_path):
    out = _run(tmp_path, extra=True)
    result = _project(out)
    tips = result["subject_tips"]
    assert [x["subject_id"] for x in tips] == ["npc.0001", "npc.0002"]
    assert [x["verified_advance_count"] for x in tips] == [4, 2]
    assert [x["last_local_seq"] for x in tips] == [8, 11]
    assert result["source"]["verified_transition_count"] == 6
    _verify(out, result)


def test_identical_run_repetitions_match_semantically_not_raw_wall_time(tmp_path):
    a, b = tmp_path / "one", tmp_path / "two"
    a.mkdir(); b.mkdir()
    ra, rb = _project(_run(a)), _project(_run(b))
    assert ra["source"]["normalised_log_sha256"] == rb["source"]["normalised_log_sha256"]
    assert ra["transitions"] == rb["transitions"]
    assert ra["subject_tips"] == rb["subject_tips"]


def test_unadvanced_seed_cannot_be_passed_off_as_transition(tmp_path):
    out = _run(tmp_path, days=0)
    with pytest.raises(TransitionEvidenceRejected, match="no verified body transitions"):
        _project(out)


@pytest.mark.parametrize("which", [
    "snapshot_raw_sha256", "events_raw_sha256",
    "snapshot_document_sha256", "normalised_log_sha256",
])
def test_wrong_a1_receipt_is_fatal_even_if_source_physically_valid(tmp_path, which):
    out = _run(tmp_path)
    receipts = _receipts(out)
    receipts[which] = "e" * 64
    with pytest.raises(TransitionEvidenceRejected, match="digest mismatch"):
        project_verified_transitions(
            (out / "snapshot.yaml").read_bytes(),
            (out / "events.jsonl").read_bytes(),
            **receipts,
        )


@pytest.mark.parametrize("invalid", [
    "bad", "A" * 64, "a" * 63, "a" * 64 + "\n", None, 0, True,
])
def test_untrusted_a1_digest_types_and_widths_are_rejected(tmp_path, invalid):
    out = _run(tmp_path)
    receipts = _receipts(out)
    receipts["events_raw_sha256"] = invalid
    with pytest.raises(TransitionEvidenceRejected, match="invalid A1 receipt"):
        project_verified_transitions(
            (out / "snapshot.yaml").read_bytes(),
            (out / "events.jsonl").read_bytes(),
            **receipts,
        )


@pytest.mark.parametrize("mutation", [
    "subject_id", "source_local_seq", "source_local_sim_time",
    "source_local_after_hours", "source_local_delta_hours", "source_segment",
    "semantic_record_sha256", "channel_diff_sha256", "source_raw",
    "source_normalised", "tip_actor", "tip_hash", "tip_count", "tip_seq",
    "reverse_order", "drop_transition", "duplicate_transition", "drop_tip",
])
def test_schema_valid_but_forged_evidence_is_rejected_by_binding(tmp_path, mutation):
    out = _run(tmp_path, extra=True)
    changed = copy.deepcopy(_project(out))
    first = changed["transitions"][0]
    tip = changed["subject_tips"][0]
    if mutation == "subject_id":
        first["subject_id"] = "npc.0002"
    elif mutation == "source_local_seq":
        first["source_local_seq"] += 1
    elif mutation == "source_local_sim_time":
        first["source_local_sim_time"] = "Y1_START+00021.000h"
    elif mutation == "source_local_after_hours":
        first["source_local_after_hours"] += 1
    elif mutation == "source_local_delta_hours":
        first["source_local_delta_hours"] = 1.5
    elif mutation == "source_segment":
        first["source_segment"] = "asleep"
    elif mutation == "semantic_record_sha256":
        first["semantic_record_sha256"] = "f" * 64
    elif mutation == "channel_diff_sha256":
        first["channel_diff_sha256"] = "f" * 64
    elif mutation == "source_raw":
        changed["source"]["events_raw_sha256"] = "f" * 64
    elif mutation == "source_normalised":
        changed["source"]["normalised_log_sha256"] = "f" * 64
    elif mutation == "tip_actor":
        tip["subject_id"] = "npc.0002"
    elif mutation == "tip_hash":
        tip["verified_body_sha256"] = "f" * 64
    elif mutation == "tip_count":
        tip["verified_advance_count"] += 1
    elif mutation == "tip_seq":
        tip["last_local_seq"] += 1
    elif mutation == "reverse_order":
        changed["transitions"].reverse()
    elif mutation == "drop_transition":
        changed["transitions"].pop()
    elif mutation == "duplicate_transition":
        changed["transitions"].append(copy.deepcopy(first))
    elif mutation == "drop_tip":
        changed["subject_tips"].pop()
    schema = json.loads(SCHEMA.read_text())
    jsonschema.Draft202012Validator(schema).validate(changed)
    with pytest.raises(TransitionEvidenceRejected, match="unbound"):
        _verify(out, changed)


def test_go_authority_and_unknown_evidence_fields_are_rejected_by_closed_schema(tmp_path):
    out = _run(tmp_path)
    evidence = _project(out)
    schema = json.loads(SCHEMA.read_text())
    for field in ("WorldRevision", "CycleCommit", "EventDraft", "candidate_id"):
        fake = copy.deepcopy(evidence)
        fake[field] = 1
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.Draft202012Validator(schema).validate(fake)
    for field in ("priority", "EventOrderKey", "SimulationInstant"):
        fake = copy.deepcopy(evidence)
        fake["transitions"][0][field] = 5
        with pytest.raises(jsonschema.ValidationError):
            jsonschema.Draft202012Validator(schema).validate(fake)
    assert schema["additionalProperties"] is False
    for name in ("source", "transition", "tip"):
        assert schema["$defs"][name]["additionalProperties"] is False


@pytest.mark.parametrize("attack", [
    "change_dt", "change_diff", "change_local_seq",
    "truncate_log", "unsupported_event", "duplicate_log_key",
])
def test_corrupted_physical_source_fails_before_any_projection(tmp_path, attack):
    out = _run(tmp_path)
    snap = (out / "snapshot.yaml").read_bytes()
    events = (out / "events.jsonl").read_bytes()
    records = _records(events)
    row = next(x for x in records if x["event"] == "body.advanced")
    if attack == "change_dt":
        row["payload"]["dt_hours"] = 6.0
    elif attack == "change_diff":
        row["payload"]["channels_changed"]["sleep"]["after"]["debt_hours"] = 0.0
    elif attack == "change_local_seq":
        row["seq"] = 999
    elif attack == "unsupported_event":
        row["event"] = "physical.commit"
    elif attack == "truncate_log":
        records.pop()
    if attack == "duplicate_log_key":
        target = b'"event": "body.advanced"'
        events = events.replace(target, target + b", " + target, 1)
    else:
        events = _jsonl(records)
    receipts = _receipts(out)
    receipts["events_raw_sha256"] = HEX(events)
    # A1 receipt deliberately recomputed for corrupted artifact to prove
    # the physical continuity validator, not just raw SHA, kills the mutant.
    normalized = []
    for r in _records(events) if attack != "duplicate_log_key" else records:
        r["wall_time"] = "1970-01-01T00:00:00+00:00"
        normalized.append(json.dumps(r, sort_keys=True, ensure_ascii=False))
    receipts["normalised_log_sha256"] = HEX(("\n".join(normalized)+"\n").encode())
    with pytest.raises(TransitionEvidenceRejected, match="PSV1-2 physical continuity"):
        project_verified_transitions(snap, events, **receipts)


def test_reference_projection_never_duplicates_seed_source_validator():
    file = (ROOT / "tests/architecture/embodiment_a2_transition_reference.py").read_text()
    assert "replay_body_history" in file
    for prohibited in ("seed_character", "posterior_for", "CapacityBaselineStore",
                       "parse_float=", "yaml.SafeLoader", "initial_body_state("):
        assert prohibited not in file


def test_reference_transport_fails_closed_on_4mib_limit(tmp_path):
    out = _run(tmp_path)
    evidence = _project(out)
    huge = copy.deepcopy(evidence)
    huge["transitions"] *= 13000
    with pytest.raises(TransitionEvidenceRejected, match="frame limit"):
        reference_bytes(huge)
    _verify(out, evidence)
