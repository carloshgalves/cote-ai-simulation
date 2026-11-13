"""CSFRV1-9A/A2: actual PSV1-1/2 seed-run fixtures, strict provenance and negatives."""

from __future__ import annotations

import copy
import hashlib
import json
from pathlib import Path

import jsonschema
import pytest
import yaml

from embodiment.cli import seed_cohort_command
from embodiment.snapshot import snapshot_hash
from tests.architecture.embodiment_a2_reference import (
    BUNDLE,
    POLICY,
    SCHEMA,
    SourceRejected,
    project_seed_source,
    reference_bytes,
)

HEX = lambda raw: hashlib.sha256(raw).hexdigest()


def _run(tmp_path, n=3):
    out = tmp_path / "run"
    seed_cohort_command(world_seed=42, count=n, out_dir=out)
    snap = (out / "snapshot.yaml").read_bytes()
    log = (out / "events.jsonl").read_bytes()
    return snap, log


def _project(snap, log):
    return project_seed_source(
        snap, log, snapshot_raw_sha256=HEX(snap), events_raw_sha256=HEX(log)
    )


def _yaml_document(snap):
    return yaml.safe_load(snap)


def _signed_yaml(data):
    clone = copy.deepcopy(data)
    clone["snapshot_hash"] = snapshot_hash(clone)
    return yaml.safe_dump(clone, sort_keys=True, allow_unicode=True).encode("utf-8")


def _log_records(log):
    return [json.loads(line) for line in log.decode().splitlines()]


def _relog(records):
    return b"".join(
        (json.dumps(item, sort_keys=True, ensure_ascii=False, allow_nan=False) + "\n").encode("utf-8")
        for item in records
    )


def _assert_rejected(snap, log, expected=None):
    with pytest.raises((SourceRejected, ValueError, TypeError, jsonschema.ValidationError)) as exc:
        _project(snap, log)
    if expected:
        assert expected in str(exc.value)


def test_positive_source_is_real_seeded_snapshot_and_event_log(tmp_path):
    snap, log = _run(tmp_path)
    result = _project(snap, log)
    schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
    jsonschema.Draft202012Validator.check_schema(schema)
    jsonschema.Draft202012Validator(schema).validate(result)
    assert result["admission"] == "PENDING_GO_ADMISSION"
    assert result["authority"] == "ENGINE_ONLY"
    assert result["source"]["local_origin"] == "Y1_START"
    assert result["source"]["snapshot_version"] == 1
    assert len(result["subjects"]) == 3
    assert [x["subject_id"] for x in result["subjects"]] == [
        "npc.0001", "npc.0002", "npc.0003"
    ]
    assert [x["source_local_sample_seq"] for x in result["subjects"]] == [1, 2, 3]
    assert all(x["engine_only"] for x in result["subjects"])
    assert all(x["source_baseline_pointer"].endswith("/capacity_baseline") for x in result["subjects"])
    assert all(x["source_body_pointer"].endswith("/body_state") for x in result["subjects"])


def test_repeated_identical_sealed_bytes_produce_identical_evidence_bytes(tmp_path):
    snap, log = _run(tmp_path)
    assert reference_bytes(_project(snap, log)) == reference_bytes(_project(snap, log))


def test_same_seed_on_independent_runs_has_equal_local_normalised_log_digest(tmp_path):
    a, b = tmp_path / "a", tmp_path / "b"
    a.mkdir()
    b.mkdir()
    s1, e1 = _run(a)
    s2, e2 = _run(b)
    r1, r2 = _project(s1, e1), _project(s2, e2)
    assert r1["source"]["normalised_log_sha256"] == r2["source"]["normalised_log_sha256"]
    assert [(s["subject_id"], s["baseline_sha256"], s["body_state_sha256"]) for s in r1["subjects"]] == [
        (s["subject_id"], s["baseline_sha256"], s["body_state_sha256"]) for s in r2["subjects"]
    ]


@pytest.mark.parametrize("which", ["snapshot", "events"])
def test_mismatched_a1_sealed_raw_source_hash_fails(tmp_path, which):
    snap, log = _run(tmp_path)
    args = {"snapshot_raw_sha256": HEX(snap), "events_raw_sha256": HEX(log)}
    args["snapshot_raw_sha256" if which == "snapshot" else "events_raw_sha256"] = "f" * 64
    with pytest.raises(SourceRejected, match="raw SHA mismatch"):
        project_seed_source(snap, log, **args)


@pytest.mark.parametrize("mutation", [
    "extra_root", "missing_body", "wrong_version", "wrong_origin", "wrong_seed",
    "wrong_unit", "missing_dimension", "wrong_body_id", "wrong_body_time",
    "uninstalled_estimator", "metadata_hash_mismatch", "wrong_baseline_hash",
    "invalid_body_domain", "wrong_prior", "unknown_baseline_field",
    "evidence_sufficiency_drift",
])
def test_signed_snapshot_mutations_fail_closed(tmp_path, mutation):
    snap, log = _run(tmp_path)
    document = _yaml_document(snap)
    subject = "npc.0001"
    section = document["characters"][subject]
    baseline = section["capacity_baseline"]
    meta = document["run_metadata"]
    if mutation == "extra_root":
        document["WorldRevision"] = 55
    elif mutation == "missing_body":
        del section["body_state"]
    elif mutation == "wrong_version":
        document["snapshot_version"] = 2
    elif mutation == "wrong_origin":
        document["sim_time"] = "Y1_START+00001.000h"
    elif mutation == "wrong_seed":
        document["world_seed"] = 43
    elif mutation == "wrong_unit":
        baseline["dimensions"]["sprint_speed"]["unit"] = "km/h"
    elif mutation == "missing_dimension":
        del baseline["dimensions"]["body_mass"]
    elif mutation == "wrong_body_id":
        section["body_state"]["character_id"] = "npc.9999"
    elif mutation == "wrong_body_time":
        section["body_state"]["t_hours"] = 12
    elif mutation == "uninstalled_estimator":
        meta["estimator_version"] = "0.1"
    elif mutation == "metadata_hash_mismatch":
        meta["posterior_hash_by_character"][subject] = "a" * 64
    elif mutation == "wrong_baseline_hash":
        baseline["sampled_from"]["posterior_hash"] = "a" * 64
    elif mutation == "invalid_body_domain":
        section["body_state"]["hydration"]["deficit_pct_body_mass"] = -1
    elif mutation == "wrong_prior":
        meta["prior_version"] = "unknown"
    elif mutation == "unknown_baseline_field":
        baseline["unsafe"] = "bypass"
    elif mutation == "evidence_sufficiency_drift":
        baseline["evidence_sufficiency"]["reaction_time"] = 0.4
    _assert_rejected(_signed_yaml(document), log)


@pytest.mark.parametrize("mutation", [
    "duplicate_seq", "sequence_gap", "reversed_records", "unknown_type",
    "unknown_payload", "new_global_field", "nonseed_sim_time",
    "posterior_mismatch", "capacity_mismatch", "missing_sample",
    "duplicated_sample", "bad_correlation", "bad_prior",
    "advanced_history", "additional_run_started",
    "nonzero_evidence", "invented_ess", "missing_terminal",
])
def test_jsonl_mutations_fail_closed(tmp_path, mutation):
    snap, log = _run(tmp_path)
    rec = _log_records(log)
    if mutation == "duplicate_seq":
        rec[2]["seq"] = rec[1]["seq"]
    elif mutation == "sequence_gap":
        rec[2]["seq"] = 99
    elif mutation == "reversed_records":
        rec[1], rec[2] = rec[2], rec[1]
    elif mutation == "unknown_type":
        rec[1]["event"] = "physical.commit"
    elif mutation == "unknown_payload":
        rec[1]["payload"]["EventOrderKey"] = 1
    elif mutation == "new_global_field":
        rec[1]["WorldRevision"] = 9
    elif mutation == "nonseed_sim_time":
        rec[1]["sim_time"] = "Y1_START+00001.000h"
    elif mutation == "posterior_mismatch":
        rec[1]["payload"]["posterior_hash"] = "f" * 64
    elif mutation == "capacity_mismatch":
        rec[1]["payload"]["dimensions"]["body_mass"]["value"] += 10
    elif mutation == "missing_sample":
        rec.pop(1)
        for i, item in enumerate(rec):
            item["seq"] = i
    elif mutation == "duplicated_sample":
        rec[2]["payload"] = copy.deepcopy(rec[1]["payload"])
    elif mutation == "bad_correlation":
        rec[-1]["payload"]["cohort_size"] = 900
    elif mutation == "bad_prior":
        rec[-1]["payload"]["prior_version"] = "bogus"
    elif mutation == "advanced_history":
        rec.insert(-1, {"seq": len(rec) - 1, "event": "body.advanced",
                        "sim_time": "Y1_START", "wall_time": "2026-01-01T00:00:00+00:00",
                        "payload": {"character_id": "npc.0001"}})
        for i, item in enumerate(rec):
            item["seq"] = i
    elif mutation == "additional_run_started":
        rec[2]["event"] = "run.started"
    elif mutation == "nonzero_evidence":
        rec[1]["payload"]["evidence_sufficiency"]["body_mass"] = 0.1
    elif mutation == "invented_ess":
        rec[1]["payload"]["estimator_ess"] = 42
    elif mutation == "missing_terminal":
        rec.pop()
    _assert_rejected(snap, _relog(rec))


def test_snapshot_local_hash_cannot_be_self_signed_with_bad_fields(tmp_path):
    snap, log = _run(tmp_path)
    data = _yaml_document(snap)
    data["characters"]["npc.0001"]["capacity_baseline"]["dimensions"]["agility"]["unit"] = "s"
    _assert_rejected(_signed_yaml(data), log, "unit mismatch")


@pytest.mark.parametrize("corruption", [
    b"", b"bad: [", b"null\n", b"snapshot_version: 1\nsnapshot_version: 1\n",
    b"key: &x [1,2]\nother: *x\n", b"unsafe: !!python/object/new:object {}",
    b"nan: .nan\n", b"snapshot_version: 1\ninvalid: 2026-01-01",
])
def test_untrusted_yaml_is_rejected(tmp_path, corruption):
    _, log = _run(tmp_path)
    _assert_rejected(corruption, log)


def test_yaml_duplicate_inner_field_is_rejected_even_if_snapshot_re_signed(tmp_path):
    snap, log = _run(tmp_path)
    # A duplicate root field must be caught before hash comparison.
    raw = snap + b"\nsnapshot_version: 1\n"
    _assert_rejected(raw, log, "duplicate YAML")


@pytest.mark.parametrize("malformed", [
    b"", b"{}", b"{}\n", b"\xff\n", b"{not-json}\n",
    b'{"seq":0,"seq":0}\n', b'{"value":NaN}\n',
    b'{"seq":0}\n\n',
])
def test_bad_jsonl_fails_closed(tmp_path, malformed):
    snap, _ = _run(tmp_path)
    _assert_rejected(snap, malformed)


def test_no_global_fields_or_causal_authority_in_prepared_projection(tmp_path):
    snap, log = _run(tmp_path)
    result = _project(snap, log)
    forbidden = json.loads((BUNDLE / "seed-evidence-registry-proposal-v1.json").read_text())["forbidden_pre_go_fields"]
    def walk(value):
        if isinstance(value, dict):
            for k, v in value.items():
                assert k not in forbidden
                walk(v)
        elif isinstance(value, list):
            for child in value:
                walk(child)
    walk(result)
    assert "event_id" not in reference_bytes(result).decode("utf-8")
    assert "CycleCommit" not in reference_bytes(result).decode("utf-8")


def test_source_seq_is_provenance_only_and_never_chooses_global_time(tmp_path):
    snap, log = _run(tmp_path)
    out = _project(snap, log)
    assert [x["source_local_sample_seq"] for x in out["subjects"]] == [1, 2, 3]
    assert not any(k in out for k in ("EventOrderKey", "SimulationInstant", "LogicalSequence"))
    records = _log_records(log)
    records[1]["sim_time"] = "Y1_START+00002.000h"
    _assert_rejected(snap, _relog(records), "sim_time")
    records = _log_records(log)
    records[2]["seq"] = records[1]["seq"]
    _assert_rejected(snap, _relog(records), "seq")


def test_provisional_extension_registry_must_not_claim_go_registration():
    registry = json.loads((BUNDLE / "seed-evidence-registry-proposal-v1.json").read_text())
    assert registry["runtime_extension_registered"] is False
    assert registry["status"] == "PROPOSED_NON_CANONICAL_NOT_REGISTERED"
    assert registry["source_events"]["body.advanced"] == "reject_unhandled_history"
    assert registry["source_events"]["cohort.correlation_report"] == "telemetry_only_never_world_event"
    assert registry["privacy"] == "ENGINE_ONLY_NO_EVIDENCE_CONTEXT_BEFORE_A3"


def test_schema_has_no_open_top_level_or_subject_writes():
    schema = json.loads(SCHEMA.read_text())
    assert schema["additionalProperties"] is False
    for key in ("source", "subject"):
        assert schema["$defs"][key]["additionalProperties"] is False
    assert set(schema["properties"]) == {
        "kind", "schema_version", "authority", "admission", "source", "subjects"
    }
    for forbidden in ("WorldRevision", "LogicalSequence", "SimulationInstant", "EventOrderKey"):
        assert forbidden not in json.dumps(schema)


def test_false_duplicate_key_guard_mutant_is_killed(tmp_path):
    snap, log = _run(tmp_path)
    data = _yaml_document(snap)
    # The real parser rejects duplicate mapping keys, including before hash checking.
    attack = _signed_yaml(data).replace(b"snapshot_version: 1\n",
                                          b"snapshot_version: 1\nsnapshot_version: 1\n", 1)
    _assert_rejected(attack, log, "duplicate YAML")
