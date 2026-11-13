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


def _strict_json(raw: bytes):
    return json.loads(
        raw.decode("utf-8", errors="strict"),
        object_pairs_hook=_unique_pairs,
        parse_constant=_reject_constant,
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
