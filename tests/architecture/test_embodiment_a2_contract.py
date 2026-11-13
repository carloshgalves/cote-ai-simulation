"""CSFRV1-9A/A2: actual PSV1-1/2 seed-run fixtures, strict provenance and negatives."""

from __future__ import annotations

import copy
import hashlib
import json
from pathlib import Path

import jsonschema
import pytest
import yaml

from embodiment.cli import seed_cohort_command, advance_clock_command
from embodiment.snapshot import snapshot_hash
from tests.architecture.embodiment_a2_reference import (
    BUNDLE,
    POLICY,
    SCHEMA,
    SourceRejected,
    project_seed_source,
    reference_bytes,
    verify_prepared_evidence,
)

HEX = lambda raw: hashlib.sha256(raw).hexdigest()


def _run(tmp_path, n=3):
    out = tmp_path / "run"
    seed_cohort_command(world_seed=42, count=n, out_dir=out)
    snap = (out / "snapshot.yaml").read_bytes()
    log = (out / "events.jsonl").read_bytes()
    return snap, log


def _a1_semantic_receipts(snap, log):
    """Reference receipt generator for tests, not an authenticating A1 process."""
    try:
        doc = yaml.safe_load(snap)
        doc_sha = doc.get("snapshot_hash", "0" * 64) if isinstance(doc, dict) else "0" * 64
    except (yaml.YAMLError, UnicodeDecodeError):
        doc_sha = "0" * 64
    try:
        lines = []
        for line in log.decode("utf-8").splitlines():
            item = json.loads(line)
            item["wall_time"] = "1970-01-01T00:00:00+00:00"
            lines.append(json.dumps(item, ensure_ascii=False, sort_keys=True))
        log_sha = HEX(("\n".join(lines) + "\n").encode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError, TypeError, AttributeError):
        log_sha = "0" * 64
    return {"snapshot_document_sha256": doc_sha,
            "normalised_log_sha256": log_sha}


def _project(snap, log):
    return project_seed_source(
        snap, log, snapshot_raw_sha256=HEX(snap), events_raw_sha256=HEX(log),
        **_a1_semantic_receipts(snap, log)
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
        project_seed_source(snap, log, **args, **_a1_semantic_receipts(snap, log))


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
    "correlation_unknown_field", "correlation_nested_unknown",
    "correlation_duplicate_pair", "correlation_invalid_sign",
    "correlation_wrong_status", "correlation_bad_threshold",
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
    elif mutation == "correlation_unknown_field":
        rec[-1]["payload"]["WorldRevision"] = 9
    elif mutation == "correlation_nested_unknown":
        rec[-1]["payload"]["pairs"][0]["EventOrderKey"] = "forged"
    elif mutation == "correlation_duplicate_pair":
        rec[-1]["payload"]["pairs"][1] = copy.deepcopy(rec[-1]["payload"]["pairs"][0])
    elif mutation == "correlation_invalid_sign":
        rec[-1]["payload"]["pairs"][0]["sign_agrees"] = 123
    elif mutation == "correlation_wrong_status":
        rec[-1]["payload"]["prior_status"] = "CALIBRATED"
    elif mutation == "correlation_bad_threshold":
        rec[-1]["payload"]["material_pairs_threshold"] = -10
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
    def prop_names(node):
        if isinstance(node, dict):
            names = set(node.get("properties", {}))
            for child in node.values():
                names |= prop_names(child)
            return names
        if isinstance(node, list):
            names = set()
            for child in node:
                names |= prop_names(child)
            return names
        return set()

    forbidden = {"WorldRevision", "LogicalSequence", "SimulationInstant", "EventOrderKey",
                 "CycleCommit", "producer_unit_key", "event_id", "candidate_id"}
    assert prop_names(schema).isdisjoint(forbidden)


def test_false_duplicate_key_guard_mutant_is_killed(tmp_path):
    snap, log = _run(tmp_path)
    data = _yaml_document(snap)
    # The real parser rejects duplicate mapping keys, including before hash checking.
    attack = _signed_yaml(data).replace(b"snapshot_version: 1\n",
                                          b"snapshot_version: 1\nsnapshot_version: 1\n", 1)
    _assert_rejected(attack, log, "duplicate YAML")


@pytest.mark.parametrize("digest", ["snapshot_document_sha256", "normalised_log_sha256"])
def test_a1_semantic_receipt_mismatch_is_rejected(tmp_path, digest):
    snap, log = _run(tmp_path)
    receipts = _a1_semantic_receipts(snap, log)
    receipts[digest] = "e" * 64
    with pytest.raises(SourceRejected, match="A1 .* digest mismatch"):
        project_seed_source(snap, log, snapshot_raw_sha256=HEX(snap),
                            events_raw_sha256=HEX(log), **receipts)


@pytest.mark.parametrize("mutation", [
    "forged_body_valid",
    "forged_capacity_and_matching_log",
    "forged_posterior_hash_everywhere",
    "forged_world_seed_everywhere",
    "forged_prior_hash",
    "future_prior_version_both_files",
    "future_dynamics_version_both_files",
])
def test_self_consistent_manipulated_seed_cannot_bypass_pinned_models(tmp_path, mutation):
    snap, log = _run(tmp_path)
    doc, records = _yaml_document(snap), _log_records(log)
    subject = "npc.0001"
    sec = doc["characters"][subject]
    baseline = sec["capacity_baseline"]
    meta = doc["run_metadata"]
    if mutation == "forged_body_valid":
        sec["body_state"]["hydration"]["deficit_pct_body_mass"] = 0.5
    elif mutation == "forged_capacity_and_matching_log":
        baseline["dimensions"]["body_mass"]["value"] += 5
        records[1]["payload"]["dimensions"]["body_mass"]["value"] += 5
    elif mutation == "forged_posterior_hash_everywhere":
        meta["posterior_hash_by_character"][subject] = "f" * 64
        baseline["sampled_from"]["posterior_hash"] = "f" * 64
        records[1]["payload"]["posterior_hash"] = "f" * 64
        records[0]["payload"]["run_metadata"] = copy.deepcopy(meta)
    elif mutation == "forged_world_seed_everywhere":
        doc["world_seed"] = 43
        meta["world_seed"] = 43
        records[0]["payload"]["run_metadata"] = copy.deepcopy(meta)
    elif mutation == "forged_prior_hash":
        records[1]["payload"]["prior_content_hash"] = "f" * 64
    elif mutation == "future_prior_version_both_files":
        meta["prior_version"] = "99.0"
        baseline["sampled_from"]["prior_version"] = "99.0"
        records[1]["payload"]["prior_version"] = "99.0"
        records[-1]["payload"]["prior_version"] = "99.0"
        records[0]["payload"]["run_metadata"] = copy.deepcopy(meta)
    elif mutation == "future_dynamics_version_both_files":
        meta["dynamics_version"] = "99.0"
        records[0]["payload"]["run_metadata"] = copy.deepcopy(meta)
    _assert_rejected(_signed_yaml(doc), _relog(records))


def test_jsonl_exponent_overflow_is_not_accepted_as_infinite_statistic(tmp_path):
    snap, log = _run(tmp_path)
    assert b'"cohort_size": 3' in log
    corrupted = log.replace(b'"cohort_size": 3', b'"cohort_size": 1e400')
    _assert_rejected(snap, corrupted, "non-finite")


def test_no_unreviewed_correlation_fields_even_when_result_is_telemetry(tmp_path):
    snap, log = _run(tmp_path)
    rec = _log_records(log)
    rec[-1]["payload"]["pairs"][0]["WorldRevision"] = 0
    _assert_rejected(snap, _relog(rec), "correlation pair fields")


@pytest.mark.parametrize("mutation", [
    "wrong_subject_pointer",
    "wrong_posterior_hash",
    "other_subject_seq",
    "reversed_subject_list",
    "missing_subject",
    "forged_source_normalised_hash",
    "extra_global_authority",
])
def test_schema_valid_but_unbound_prepared_evidence_is_rejected(tmp_path, mutation):
    snap, log = _run(tmp_path)
    expected = _project(snap, log)
    changed = copy.deepcopy(expected)
    a, b = changed["subjects"][:2]
    if mutation == "wrong_subject_pointer":
        a["source_body_pointer"] = b["source_body_pointer"]
    elif mutation == "wrong_posterior_hash":
        a["posterior_hash"] = "f" * 64
    elif mutation == "other_subject_seq":
        a["source_local_sample_seq"] = b["source_local_sample_seq"]
    elif mutation == "reversed_subject_list":
        changed["subjects"].reverse()
    elif mutation == "missing_subject":
        changed["subjects"].pop()
    elif mutation == "forged_source_normalised_hash":
        changed["source"]["normalised_log_sha256"] = "e" * 64
    elif mutation == "extra_global_authority":
        changed["WorldRevision"] = 1
    if mutation != "extra_global_authority":
        jsonschema.Draft202012Validator(json.loads(SCHEMA.read_text())).validate(changed)
    with pytest.raises(SourceRejected):
        verify_prepared_evidence(
            changed, snap, log,
            snapshot_raw_sha256=HEX(snap), events_raw_sha256=HEX(log),
            **_a1_semantic_receipts(snap, log),
        )


@pytest.mark.parametrize("value", [
    "not-a-timestamp",
    "2026-01-01T00:00:00+03:00",
    "2026-01-01T00:00:00Z",
    "2026-01-01",
])
def test_wall_time_requires_original_utc_timestamp_form(tmp_path, value):
    snap, log = _run(tmp_path)
    records = _log_records(log)
    records[1]["wall_time"] = value
    _assert_rejected(snap, _relog(records), "wall_time")


def test_correlation_pair_order_is_part_of_source_contract(tmp_path):
    snap, log = _run(tmp_path)
    records = _log_records(log)
    pairs = records[-1]["payload"]["pairs"]
    pairs[0], pairs[1] = pairs[1], pairs[0]
    _assert_rejected(snap, _relog(records), "correlation pair")


@pytest.mark.parametrize("where", ["record", "nested_payload"])
def test_duplicate_json_key_in_otherwise_valid_seeding_log_rejected(tmp_path, where):
    snap, log = _run(tmp_path)
    rows = log.splitlines(keepends=True)
    assert len(rows) == 5
    row = rows[1]
    if where == "record":
        original = b'"event": "capacity.sampled"'
    else:
        original = b'"posterior_hash": '
        # Use the first match in the nested payload and duplicate its entire scalar
        parsed = json.loads(row)
        digest = parsed["payload"]["posterior_hash"].encode()
        original += b'"' + digest + b'"'
    assert row.count(original) == 1
    rows[1] = row.replace(original, original + b", " + original, 1)
    corrupted = b"".join(rows)
    # Removal of the strict duplicate-key hook must make this regression fail.
    with pytest.raises(SourceRejected, match="duplicate JSON mapping key"):
        _project(snap, corrupted)


def test_real_advance_clock_cannot_be_promoted_from_frozen_seed_snapshot(tmp_path):
    snap, log = _run(tmp_path)
    run_dir = tmp_path / "run"
    advance_clock_command(run_dir=run_dir, character_id="npc.0001", days=1)
    new_snap = (run_dir / "snapshot.yaml").read_bytes()
    new_log = (run_dir / "events.jsonl").read_bytes()
    assert new_snap == snap
    assert new_log != log
    with pytest.raises(SourceRejected, match="unsupported history"):
        _project(new_snap, new_log)
