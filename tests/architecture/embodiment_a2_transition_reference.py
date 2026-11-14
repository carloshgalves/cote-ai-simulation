"""CSFRV1-9A/A2: test-side typed view of *verified* PSV1-2 body transitions.

No second seed parser/replayer: PSV1-2 continuity (#105, merged PR #106)
is the sole physical verifier. This module defines a reference transport
projection, NOT a Go adapter, CSF event draft, world reducer or transaction.
"""
from __future__ import annotations

import hashlib
import json
from collections.abc import Mapping
from pathlib import Path

import jsonschema
import yaml

from embodiment.continuity import PhysicalHistoryError, replay_body_history
from embodiment.eventlog import EVENT_BODY_ADVANCED, NORMALISED_WALL_TIME
from embodiment.types import as_document

ROOT = Path(__file__).resolve().parents[2]
SCHEMA = (ROOT / "docs/architecture/embodiment-causal-adapter-v1/schemas"
          / "verified-transition-evidence-v1.schema.json")
MAX_FRAME_BYTES = 4_194_304


class TransitionEvidenceRejected(ValueError):
    """A1 source receipts or the prepared A2 transition evidence do not match."""


def _require(ok: bool, message: str) -> None:
    if not ok:
        raise TransitionEvidenceRejected(message)


def _sha(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def _doc_bytes(value: object) -> bytes:
    """Stable physical reference JSON; deliberately not canonical CSF CBOR."""
    return json.dumps(
        value, sort_keys=True, ensure_ascii=False,
        separators=(",", ":"), allow_nan=False,
    ).encode("utf-8")


def _doc_sha(value: object) -> str:
    return _sha(_doc_bytes(value))


def reference_bytes(value: Mapping) -> bytes:
    raw = _doc_bytes(value)
    _require(len(raw) <= MAX_FRAME_BYTES, "prepared A2 evidence exceeds A1 frame limit")
    return raw


def _schema_validate(value: dict) -> None:
    schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
    try:
        jsonschema.Draft202012Validator(schema).validate(value)
    except jsonschema.ValidationError as exc:
        raise TransitionEvidenceRejected("prepared transition evidence violates closed schema") from exc


def project_verified_transitions(
    snapshot_bytes: bytes, events_bytes: bytes, *,
    snapshot_raw_sha256: str,
    events_raw_sha256: str,
    snapshot_document_sha256: str,
    normalised_log_sha256: str,
) -> dict:
    """Map a PSV1-2 verified *local prefix* to an ENGINE_ONLY reference envelope.

    Caller supplies authenticated source provenance from A1. All source bytes
    must already be a coherent, immutable view; hashes prove only consistency
    with that view, not authorization. This function cannot grant Go admission.
    """
    _require(type(snapshot_bytes) is bytes and type(events_bytes) is bytes,
             "A1 source must be sealed bytes")
    receipts = {
        "snapshot_raw_sha256": snapshot_raw_sha256,
        "events_raw_sha256": events_raw_sha256,
        "snapshot_document_sha256": snapshot_document_sha256,
        "normalised_log_sha256": normalised_log_sha256,
    }
    for field, digest in receipts.items():
        _require(type(digest) is str and len(digest) == 64 and
                 all(char in "0123456789abcdef" for char in digest),
                 f"invalid A1 receipt: {field}")
    _require(_sha(snapshot_bytes) == snapshot_raw_sha256,
             "A1 snapshot raw digest mismatch")
    _require(_sha(events_bytes) == events_raw_sha256,
             "A1 events raw digest mismatch")

    # This is the *one* source/seed/transition semantic validator. It validates
    # every physical prefix, including the model versions and exact channel diffs.
    try:
        prefix = replay_body_history(snapshot_bytes, events_bytes)
    except PhysicalHistoryError as exc:
        raise TransitionEvidenceRejected("PSV1-2 physical continuity rejected the source") from exc
    _require(prefix.snapshot_document_sha256 == snapshot_document_sha256,
             "A1 physical snapshot document digest mismatch")

    # After PSV1-2 has validated the exact bytes, read them only to project
    # fields that already passed its grammar/typed/physical checks.
    records = [json.loads(row) for row in events_bytes.decode("utf-8").splitlines()]
    _require(len(records) == prefix.log_record_count, "verified source count drift")
    normalized = []
    transitions = []
    last_seq: dict[str, int] = {}
    counts: dict[str, int] = {}
    for record in records:
        semantic = dict(record, wall_time=NORMALISED_WALL_TIME)
        normalized.append(json.dumps(semantic, ensure_ascii=False, sort_keys=True))
        if record["event"] != EVENT_BODY_ADVANCED:
            continue
        payload = record["payload"]
        subject = payload["character_id"]
        seq = record["seq"]
        counts[subject] = counts.get(subject, 0) + 1
        last_seq[subject] = seq
        transitions.append({
            "subject_id": subject,
            "source_local_seq": seq,
            "source_local_sim_time": record["sim_time"],
            "source_local_after_hours": payload["t_hours_after"],
            "source_local_delta_hours": payload["dt_hours"],
            "source_segment": payload["segment"],
            "semantic_record_sha256": _doc_sha(semantic),
            "channel_diff_sha256": _doc_sha(payload["channels_changed"]),
        })
    _require(len(transitions) == prefix.body_advance_count and len(transitions) > 0,
             "no verified body transitions to project")
    _require(counts == {k: v for k, v in prefix.advances_by_character.items() if v > 0},
             "subject counts diverge from PSV1-2 verification")
    normalized_hash = _sha(("\n".join(normalized) + "\n").encode("utf-8"))
    _require(normalized_hash == normalised_log_sha256,
             "A1 normalised physical log digest mismatch")

    # The source seed is *used to reconstruct previous state*, but not exposed
    # as an additional seed evidence schema or a second seed validator.
    snapshot = yaml.safe_load(snapshot_bytes)
    meta = snapshot["run_metadata"]
    tips = [{
        "subject_id": subject,
        "last_local_seq": last_seq[subject],
        # The only numeric body materialization is ENGINE_ONLY, never an actor
        # observation; A3 must classify every field before any disclosure.
        "verified_body_sha256": _doc_sha(as_document(prefix.bodies[subject])),
        "verified_body": as_document(prefix.bodies[subject]),
        "verified_advance_count": counts[subject],
        "visibility": "ENGINE_ONLY",
    } for subject in sorted(counts)]
    evidence = {
        "kind": "physical.verified-transition-evidence.v1",
        "schema_version": 1,
        "authority": "ENGINE_ONLY",
        "admission": "PENDING_GO_ADMISSION",
        "completion": "UNATTESTED_PREFIX",
        "source": {
            "snapshot_raw_sha256": snapshot_raw_sha256,
            "events_raw_sha256": events_raw_sha256,
            "snapshot_document_sha256": snapshot_document_sha256,
            "normalised_log_sha256": normalised_log_sha256,
            "snapshot_version": 1,
            "local_origin": "Y1_START",
            "prior_version": meta["prior_version"],
            "dynamics_version": meta["dynamics_version"],
            "log_record_count": prefix.log_record_count,
            "verified_transition_count": len(transitions),
        },
        "transitions": transitions,
        "subject_tips": tips,
    }
    _schema_validate(evidence)
    reference_bytes(evidence)
    return evidence


def verify_projected_transitions(
    evidence: Mapping,
    snapshot_bytes: bytes, events_bytes: bytes, *,
    snapshot_raw_sha256: str,
    events_raw_sha256: str,
    snapshot_document_sha256: str,
    normalised_log_sha256: str,
) -> None:
    """Reference check of every pointer/digest/count vs. verified PSV1-2 bytes.

    Schema validity ALONE cannot bind a transition to an actor or source.
    Go must independently enforce this when building a real CSF adapter.
    """
    _require(type(evidence) is dict, "prepared A2 envelope not a dict")
    _schema_validate(evidence)
    expected = project_verified_transitions(
        snapshot_bytes, events_bytes,
        snapshot_raw_sha256=snapshot_raw_sha256,
        events_raw_sha256=events_raw_sha256,
        snapshot_document_sha256=snapshot_document_sha256,
        normalised_log_sha256=normalised_log_sha256,
    )
    _require(reference_bytes(evidence) == reference_bytes(expected),
             "prepared evidence is unbound to PSV1-2 verified source")
