"""A1 contract-only proof: strict framing and control-plane fixtures.

This is a reference fixture decoder, NOT the production subprocess/Go adapter.
Source mapping, physical reducers and canonical causal bytes belong to A2/A4/10.
"""

from __future__ import annotations

import copy
import json
from pathlib import Path

import jsonschema
import pytest

ROOT = Path(__file__).resolve().parents[2]
BUNDLE = ROOT / "docs" / "architecture" / "embodiment-causal-adapter-v1"
SCHEMA_PATH = BUNDLE / "schemas" / "ipc-v1.schema.json"
FIXTURE_PATH = BUNDLE / "fixtures" / "ipc-v1-cases.json"
MAX_FRAME_BYTES = 4 * 1024 * 1024

FORBIDDEN_AUTHORITY_KEYS = frozenset(
    {
        "CycleCommit", "CycleAbortRecord", "WorldRevision", "SimulationInstant",
        "LogicalSequence", "EventOrderKey", "EventDraft", "CommitCandidate",
        "EventStore", "DecisionLedger", "SnapshotStore", "store_handle",
        "global_time", "global_sequence", "commit", "priority",
    }
)


def _unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON object key: {key}")
        result[key] = value
    return result


def _reject_constant(value):
    raise ValueError(f"non-finite JSON constant: {value}")


def _reject_float(value):
    raise ValueError(f"forbidden JSON float token: {value[:24]}")


def _strict_json(raw: bytes):
    return json.loads(
        raw.decode("utf-8", errors="strict"),
        object_pairs_hook=_unique_pairs,
        parse_constant=_reject_constant,
        parse_float=_reject_float,
    )


def _schema():
    return _strict_json(SCHEMA_PATH.read_bytes())


def _cases():
    return _strict_json(FIXTURE_PATH.read_bytes())


def _validator():
    schema = _schema()
    jsonschema.Draft202012Validator.check_schema(schema)
    return jsonschema.Draft202012Validator(schema)


def _frame(value):
    """Reference encoding only; operational framing is normative in the doc."""
    encoded = json.dumps(
        value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False,
    ).encode("utf-8")
    if not 0 < len(encoded) <= MAX_FRAME_BYTES:
        raise ValueError("frame size outside protocol bounds")
    # Schema accepts 43.0 as mathematically integral; the A1 wire does not.
    _validator().validate(_strict_json(encoded))
    return len(encoded).to_bytes(4, "big") + encoded


def _unframe(data):
    """Reject partial, overlong, trailing, duplicate and malformed envelopes."""
    if len(data) < 4:
        raise ValueError("truncated frame prefix")
    declared = int.from_bytes(data[:4], "big")
    if declared == 0 or declared > MAX_FRAME_BYTES:
        raise ValueError("frame size outside protocol bounds")
    if len(data) != 4 + declared:
        raise ValueError("truncated or trailing frame bytes")
    parsed = _strict_json(data[4:])
    _validator().validate(parsed)
    return parsed


def _mutate(value, mutation):
    updated = copy.deepcopy(value)
    parts = mutation["path"].strip("/").split("/")
    current = updated
    for part in parts[:-1]:
        current = current[part]
    if mutation["op"] == "set":
        current[parts[-1]] = mutation["value"]
    elif mutation["op"] == "delete":
        del current[parts[-1]]
    else:
        raise AssertionError("unknown test mutation")
    return updated


def _properties(node):
    """Inspect declared properties, including recursive schema definitions."""
    if isinstance(node, dict):
        return set(node.get("properties", {})) | set().union(
            *(_properties(item) for item in node.values())
        )
    if isinstance(node, list):
        return set().union(*(_properties(item) for item in node))
    return set()


def test_schema_and_fixtures_are_strict_and_valid_json():
    validator = _validator()
    cases = _cases()
    assert cases["contract"] == "cote.embodiment.ipc.v1"
    assert set(cases["positive"]) == {"request", "ok", "error"}
    assert len(cases["negative"]) >= 20
    for payload in cases["positive"].values():
        assert not list(validator.iter_errors(payload))


def test_negative_fixtures_all_fail_schema():
    cases = _cases()
    validator = _validator()
    seen = set()
    for mutation in cases["negative"]:
        assert mutation["name"] not in seen
        seen.add(mutation["name"])
        assert mutation["base"] in cases["positive"]
        mutated = _mutate(cases["positive"][mutation["base"]], mutation)
        assert list(validator.iter_errors(mutated)), mutation["name"]


def test_control_plane_cannot_expose_any_causal_authority():
    properties = _properties(_schema())
    assert FORBIDDEN_AUTHORITY_KEYS.isdisjoint(properties)
    assert "payload" not in properties, "A2 must version physical payloads separately"
    assert "body_state" not in properties
    assert "capacity_baseline" not in properties
    assert "world_seed" not in properties
    assert "physical_beliefs" not in properties


def test_response_is_bound_to_the_operational_request_only():
    cases = _cases()["positive"]
    request = cases["request"]
    for response in (cases["ok"], cases["error"]):
        assert response["correlation_id"] == request["correlation_id"]
        assert response["operation"] == request["operation"]
        assert response["protocol"] == request["protocol"]
        _unframe(_frame(response))


def test_reference_encoding_is_deterministic_under_map_reordering():
    request = _cases()["positive"]["request"]
    reversed_order = dict(reversed(list(request.items())))
    assert _frame(request) == _frame(reversed_order)
    assert _unframe(_frame(request)) == request


def test_fixture_frames_have_4_byte_be_length_and_no_extra_bytes():
    for payload in _cases()["positive"].values():
        wire = _frame(payload)
        assert int.from_bytes(wire[:4], "big") == len(wire) - 4
        assert _unframe(wire) == payload


@pytest.mark.parametrize(
    "wire",
    [
        b"", b"\x00", b"\x00\x00\x00", b"\x00\x00\x00\x00",
        b"\xff\xff\xff\xff",
        b"\x00\x00\x00\x02\xff\xfe",
        b"\x00\x00\x00\x02{}extra",
        b"\x00\x00\x00\x06{}",
    ],
)
def test_reference_decoder_rejects_corrupted_framing(wire):
    with pytest.raises((ValueError, UnicodeDecodeError, json.JSONDecodeError)):
        _unframe(wire)


def test_reference_decoder_rejects_truncation_and_trailing_frames():
    wire = _frame(_cases()["positive"]["request"])
    for broken in (wire[:-1], wire + b"\x00", wire + wire):
        with pytest.raises(ValueError):
            _unframe(broken)


@pytest.mark.parametrize(
    "raw",
    [
        b'{"kind":"request","kind":"response"}',
        b'{"kind":"request","source":{"x":1,"x":2}}',
        b'{"kind":"request","nonfinite":NaN}',
        b'{"kind":"request","nonfinite":Infinity}',
    ],
)
def test_reference_decoder_rejects_duplicate_keys_and_nonfinite_json(raw):
    wire = len(raw).to_bytes(4, "big") + raw
    with pytest.raises((ValueError, jsonschema.ValidationError)):
        _unframe(wire)


def test_oversize_encoder_fails_before_a_frame_is_built():
    request = copy.deepcopy(_cases()["positive"]["request"])
    request["source"]["unspecified"] = "x" * MAX_FRAME_BYTES
    with pytest.raises(ValueError, match="size"):
        _frame(request)


def test_no_implicit_version_upgrade_or_source_path_parameter():
    schema = _schema()
    request = schema["$defs"]["request"]
    assert request["properties"]["protocol"]["const"] == "cote.embodiment.ipc.v1"
    assert request["properties"]["operation"]["const"] == "inspect_run"
    source = schema["$defs"]["source"]
    assert source["properties"]["snapshot_name"]["const"] == "snapshot.yaml"
    assert source["properties"]["events_name"]["const"] == "events.jsonl"
    assert source["additionalProperties"] is False
    assert request["additionalProperties"] is False
    assert schema["$defs"]["response"]["additionalProperties"] is False
    assert schema["$defs"]["result"]["additionalProperties"] is False
    assert schema["$defs"]["error"]["additionalProperties"] is False


def test_all_version_one_properties_are_an_explicit_closed_allowlist():
    """An added field requires a versioned decision, not just a new schema key."""
    defs = _schema()["$defs"]
    allowed = {
        "sha256": None,
        "source": {
            "snapshot_name", "events_name", "snapshot_raw_sha256", "events_raw_sha256",
        },
        "result": {
            "snapshot_raw_sha256", "events_raw_sha256",
            "snapshot_document_sha256", "normalised_log_sha256",
            "event_count", "snapshot_version",
        },
        "error": {"code"},
        "request": {"kind", "protocol", "correlation_id", "operation", "source"},
        "response": {
            "kind", "protocol", "correlation_id", "operation",
            "status", "result", "error",
        },
    }
    assert set(defs) == set(allowed)
    for name, keys in allowed.items():
        if keys is None:
            assert defs[name].get("properties") is None
            continue
        assert set(defs[name]["properties"]) == keys, name
        assert defs[name]["additionalProperties"] is False, name


def test_source_hashes_are_echoed_without_global_identity_fields():
    request = _cases()["positive"]["request"]
    result = _cases()["positive"]["ok"]["result"]
    assert request["source"]["snapshot_raw_sha256"] == result["snapshot_raw_sha256"]
    assert request["source"]["events_raw_sha256"] == result["events_raw_sha256"]
    assert "producer_unit_key" not in result
    assert "input_id" not in result
    assert "event_id" not in result
    assert "wall_time" not in result


def _verify_exchange(request, response):
    """Reference transaction guard: schema-valid is not the same as source-bound."""
    validator = _validator()
    validator.validate(request)
    validator.validate(response)
    if request["kind"] != "request" or response["kind"] != "response":
        raise ValueError("invalid exchange roles")
    for field in ("protocol", "correlation_id", "operation"):
        if request[field] != response[field]:
            raise ValueError(f"unbound response {field}")
    if response["status"] == "ok":
        for field in ("snapshot_raw_sha256", "events_raw_sha256"):
            if response["result"][field] != request["source"][field]:
                raise ValueError(f"unbound response {field}")
    return response


def test_reference_exchange_requires_matching_raw_source_hashes():
    cases = _cases()["positive"]
    assert _verify_exchange(cases["request"], cases["ok"]) == cases["ok"]
    assert _verify_exchange(cases["request"], cases["error"]) == cases["error"]
    for field in ("snapshot_raw_sha256", "events_raw_sha256"):
        response = copy.deepcopy(cases["ok"])
        response["result"][field] = "e" * 64  # syntactically valid, wrong source
        assert not list(_validator().iter_errors(response))
        with pytest.raises(ValueError, match="unbound response"):
            _verify_exchange(cases["request"], response)


def test_reference_exchange_rejects_a_response_from_another_request():
    cases = _cases()["positive"]
    response = copy.deepcopy(cases["ok"])
    response["correlation_id"] = "f" * 32  # schema-valid, different invocation
    assert not list(_validator().iter_errors(response))
    with pytest.raises(ValueError, match="unbound response correlation_id"):
        _verify_exchange(cases["request"], response)

# A1-R02/R03/R04/R05: adversarial RED controls. The policy file is a
# versioned implementation contract, not an executable process adapter.
POLICY_PATH = BUNDLE / "ipc-v1-operational-policy.json"


def _policy():
    return _strict_json(POLICY_PATH.read_bytes())


def _validate_policy(policy):
    """Reference contract guard: reject privilege/availability regressions."""
    allowed = {
        "process": {"single_flight", "direct_exec_no_shell", "no_remote_transport",
                    "operational_failure_precedence"},
        "source_view": {
            "mode", "child_cannot_modify", "host_pins_absolute_root",
            "reject_symlinks_and_non_regular_files", "no_pre_post_hash_fallback",
            "source_paths_are_not_protocol_fields", "must_verify_raw_sha256_before_spawning",
            "fail_if_os_read_only_isolation_missing",
            "producer_quiescence_required", "coherent_snapshot_and_eventlog_cut",
            "fail_if_coherence_unprovable",
        },
        "io": {
            "drain_stdout_and_stderr_concurrently", "read_stdout_exactly_one_framed_message_then_eof",
            "bounded_reads_before_allocating", "kill_on_stdout_or_stderr_limit",
            "kill_on_timeout_or_cancellation", "redact_or_discard_stderr",
            "nonzero_exit_always_fails",
        },
        "limits": {
            "frame_payload_bytes", "source_snapshot_bytes", "source_events_bytes",
            "source_event_line_bytes", "source_event_count", "stderr_retained_bytes",
            "end_to_end_process_budget_ms", "termination_grace_ms",
        },
    }
    assert set(policy) == {"protocol", *allowed}
    for group, fields in allowed.items():
        assert set(policy[group]) == fields, group
    limits = policy["limits"]
    process = policy["process"]
    source = policy["source_view"]
    io = policy["io"]
    assert policy["protocol"] == "cote.embodiment.ipc.v1"
    assert process["single_flight"] is True
    assert process["direct_exec_no_shell"] is True
    assert process["no_remote_transport"] is True
    assert process["operational_failure_precedence"] == [
        "CANCELLED", "TIMED_OUT", "STDERR_LIMIT", "FRAME_INVALID", "PROCESS_FAILURE",
    ]
    assert source["mode"] == "immutable_host_owned_view"
    assert source["child_cannot_modify"] is True
    assert source["host_pins_absolute_root"] is True
    assert source["reject_symlinks_and_non_regular_files"] is True
    assert source["no_pre_post_hash_fallback"] is True
    assert source["source_paths_are_not_protocol_fields"] is True
    assert source["must_verify_raw_sha256_before_spawning"] is True
    assert source["fail_if_os_read_only_isolation_missing"] is True
    assert source["producer_quiescence_required"] is True
    assert source["coherent_snapshot_and_eventlog_cut"] is True
    assert source["fail_if_coherence_unprovable"] is True
    assert io["drain_stdout_and_stderr_concurrently"] is True
    assert io["read_stdout_exactly_one_framed_message_then_eof"] is True
    assert io["bounded_reads_before_allocating"] is True
    assert io["kill_on_stdout_or_stderr_limit"] is True
    assert io["kill_on_timeout_or_cancellation"] is True
    assert io["redact_or_discard_stderr"] is True
    assert io["nonzero_exit_always_fails"] is True
    assert limits["frame_payload_bytes"] == MAX_FRAME_BYTES
    assert 0 < limits["source_snapshot_bytes"] <= 16 * 1024 * 1024
    assert 0 < limits["source_events_bytes"] <= 64 * 1024 * 1024
    assert 0 < limits["source_event_line_bytes"] <= 1024 * 1024
    assert 0 < limits["source_event_count"] <= 100000
    assert 0 < limits["stderr_retained_bytes"] <= 16384
    assert 0 < limits["termination_grace_ms"] < limits["end_to_end_process_budget_ms"]
    assert limits["end_to_end_process_budget_ms"] <= 30000
    assert limits["source_event_line_bytes"] <= limits["source_events_bytes"]


def test_encoder_rejects_invalid_envelopes_before_writing_any_frame():
    cases = _cases()
    for negative in cases["negative"]:
        invalid = _mutate(cases["positive"][negative["base"]], negative)
        with pytest.raises((ValueError, jsonschema.ValidationError)):
            _frame(invalid)


def test_encoder_rejects_invalid_operation_even_when_below_byte_limit():
    request = copy.deepcopy(_cases()["positive"]["request"])
    request["operation"] = "commit_world"
    with pytest.raises(jsonschema.ValidationError):
        _frame(request)


def test_policy_requires_immutable_child_inaccessible_source_view():
    _validate_policy(_policy())


@pytest.mark.parametrize(
    ("group", "key", "weak_value"),
    [
        ("source_view", "child_cannot_modify", False),
        ("source_view", "no_pre_post_hash_fallback", False),
        ("source_view", "reject_symlinks_and_non_regular_files", False),
        ("source_view", "fail_if_os_read_only_isolation_missing", False),
        ("source_view", "producer_quiescence_required", False),
        ("source_view", "coherent_snapshot_and_eventlog_cut", False),
        ("source_view", "fail_if_coherence_unprovable", False),
        ("process", "operational_failure_precedence", ["PROCESS_FAILURE", "TIMED_OUT"]),
        ("io", "drain_stdout_and_stderr_concurrently", False),
        ("io", "kill_on_timeout_or_cancellation", False),
        ("io", "bounded_reads_before_allocating", False),
        ("limits", "source_events_bytes", 2**40),
        ("limits", "source_snapshot_bytes", 2**40),
        ("limits", "source_event_line_bytes", 2**40),
        ("limits", "termination_grace_ms", 32000),
        ("limits", "end_to_end_process_budget_ms", 60000),
    ],
)
def test_policy_mutants_are_rejected(group, key, weak_value):
    config = copy.deepcopy(_policy())
    config[group][key] = weak_value
    with pytest.raises(AssertionError):
        _validate_policy(config)


def test_decoder_fails_closed_on_oversized_declared_payload_before_json():
    for frame in ((MAX_FRAME_BYTES + 1).to_bytes(4, "big"), b"\xff\xff\xff\xff"):
        with pytest.raises(ValueError, match="size"):
            _unframe(frame)


@pytest.mark.parametrize("numeric_token", ["43.0", "43e0", "1e400", "4.3E1"])
def test_decoder_rejects_lexical_float_even_when_integral(numeric_token):
    payload = _cases()["positive"]["ok"]
    raw = json.dumps(payload, separators=(",", ":"), allow_nan=False).encode("utf-8")
    assert b'"event_count":43' in raw
    raw = raw.replace(b'"event_count":43', f'"event_count":{numeric_token}'.encode())
    with pytest.raises(ValueError, match="forbidden JSON float"):
        _unframe(len(raw).to_bytes(4, "big") + raw)


def test_sender_rejects_float_lexeme_with_integral_value():
    payload = copy.deepcopy(_cases()["positive"]["ok"])
    payload["result"]["event_count"] = 43.0
    with pytest.raises(ValueError, match="forbidden JSON float"):
        _frame(payload)


@pytest.mark.parametrize("group", ["process", "source_view", "io", "limits"])
def test_policy_rejects_added_unreviewed_fields(group):
    policy = copy.deepcopy(_policy())
    policy[group]["arbitrary_bypass"] = True
    with pytest.raises(AssertionError):
        _validate_policy(policy)


def test_negative_fixtures_rejected_by_actual_frame_decoder():
    cases = _cases()
    for case in cases["negative"]:
        invalid = _mutate(cases["positive"][case["base"]], case)
        raw = json.dumps(invalid, separators=(",", ":"), allow_nan=False).encode()
        wire = len(raw).to_bytes(4, "big") + raw
        with pytest.raises((ValueError, jsonschema.ValidationError)):
            _unframe(wire)


@pytest.mark.parametrize("field", ["kind", "protocol"])
def test_valid_envelope_with_duplicate_top_level_name_is_rejected(field):
    request = _cases()["positive"]["request"]
    raw = json.dumps(request, separators=(",", ":")).encode()
    existing = f'"{field}":"{request[field]}"'.encode()
    assert raw.count(existing) == 1
    raw = raw.replace(existing, existing + b"," + existing)
    with pytest.raises(ValueError, match="duplicate JSON object key"):
        _unframe(len(raw).to_bytes(4, "big") + raw)


def test_valid_envelope_with_nested_duplicate_name_is_rejected():
    req = _cases()["positive"]["request"]
    raw = json.dumps(req, separators=(",", ":")).encode()
    existing = b'"snapshot_name":"snapshot.yaml"'
    assert raw.count(existing) == 1
    raw = raw.replace(existing, existing + b"," + existing)
    with pytest.raises(ValueError, match="duplicate JSON object key"):
        _unframe(len(raw).to_bytes(4, "big") + raw)


@pytest.mark.parametrize("token", ["NaN", "Infinity", "-Infinity"])
def test_valid_envelope_with_nonfinite_event_count_is_rejected(token):
    valid = _cases()["positive"]["ok"]
    raw = json.dumps(valid, separators=(",", ":")).encode()
    assert b'"event_count":43' in raw
    raw = raw.replace(b'"event_count":43', f'"event_count":{token}'.encode())
    with pytest.raises(ValueError, match="non-finite JSON constant"):
        _unframe(len(raw).to_bytes(4, "big") + raw)


def test_policy_requires_a_coherent_source_cut_and_fixed_failure_precedence():
    policy = _policy()
    source = policy["source_view"]
    process = policy["process"]
    assert source["producer_quiescence_required"] is True
    assert source["coherent_snapshot_and_eventlog_cut"] is True
    assert source["fail_if_coherence_unprovable"] is True
    assert process["operational_failure_precedence"] == [
        "CANCELLED", "TIMED_OUT", "STDERR_LIMIT", "FRAME_INVALID", "PROCESS_FAILURE"
    ]
