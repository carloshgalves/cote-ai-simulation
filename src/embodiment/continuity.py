"""PSV1-2 continuity slice: verify a *local physical prefix* by deterministic replay.

This module is read-only. It does not certify the completion of an interrupted
advance-clock command, rehydrate a PSV1-8 physical world, provide a saved resume
checkpoint, or admit an event to the Go CSF. Input bytes must come from a
quiescent, authorized source when used beyond offline diagnostics.
"""

from __future__ import annotations

import hashlib
import json
import math
import re
from collections.abc import Mapping
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from types import MappingProxyType
from typing import Any

import yaml
from pydantic import ValidationError
from yaml.tokens import AliasToken, AnchorToken, TagToken

from .dynamics import (
    BodyTraits, Environment, Exposure, advance, channel_diff, initial_body_state, load_params
)
from .eventlog import (
    EVENT_BODY_ADVANCED, EVENT_CAPACITY_SAMPLED,
    EVENT_COHORT_CORRELATION_REPORT, EVENT_RUN_EXTENDED, EVENT_RUN_STARTED,
    logical_sim_time,
)
from .prior import POPULATION_PRIOR_PATH, PopulationPrior
from .seeding import (
    CapacityBaselineStore, CohortCovariates, capacity_sampled_payload,
    posterior_for, seed_character,
)
from .snapshot import body_state_of, capacity_profile_of, snapshot_hash
from .types import BodyState, Dimension, SNAPSHOT_VERSION, Y1_START, as_document

__all__ = [
    "PhysicalHistoryError", "VerifiedPhysicalPrefix",
    "replay_body_history", "verify_body_history",
]

_MAX_SNAPSHOT_BYTES = 16 * 1024 * 1024
_MAX_LOG_BYTES = 64 * 1024 * 1024
_MAX_LINE_BYTES = 1024 * 1024
_MAX_RECORDS = 100_000
_MAX_SUBJECTS = 10_000
_SUBJECT_ID = re.compile(r"[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*\Z")
_SHA256 = re.compile(r"[0-9a-f]{64}\Z")
_SNAPSHOT_KEYS = {
    "snapshot_version", "world_seed", "sim_time", "characters", "run_metadata",
    "snapshot_hash",
}
_METADATA_KEYS = {
    "world_seed", "prior_version", "dynamics_version",
    "posterior_hash_by_character", "evidence_sufficiency_by_character",
}
_BASELINE_KEYS = {
    "schema_version", "sim_time", "sampled_from",
    "evidence_sufficiency", "dimensions",
}
_ORIGIN_KEYS = {
    "posterior_hash", "prior_version", "cohort_sex", "cohort_sex_source", "substream",
}
_RECORD_KEYS = {"seq", "event", "sim_time", "wall_time", "payload"}
_ADVANCE_KEYS = {
    "character_id", "dt_hours", "segment", "t_hours_before", "t_hours_after",
    "environment", "load", "dynamics_version", "channels_changed",
}


class PhysicalHistoryError(ValueError):
    """Physical source is incomplete, incompatible, corrupt or unverifiable."""


@dataclass(frozen=True)
class VerifiedPhysicalPrefix:
    """Verified *local* state, never an admitted causal world revision."""

    bodies: Mapping[str, BodyState]
    advances_by_character: Mapping[str, int]
    snapshot_raw_sha256: str
    events_raw_sha256: str
    snapshot_document_sha256: str
    log_record_count: int
    body_advance_count: int
    status: str = "VERIFIED_LOCAL_PREFIX_ONLY"


class _UniqueLoader(yaml.SafeLoader):
    pass


def _valid(ok: bool, message: str) -> None:
    if not ok:
        raise PhysicalHistoryError(message)


def _yaml_mapping(loader: yaml.SafeLoader, node: yaml.MappingNode) -> dict:
    result: dict[str, Any] = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=True)
        _valid(type(key) is str and key not in result, "invalid/duplicate YAML mapping key")
        result[key] = loader.construct_object(value_node, deep=True)
    return result


_UniqueLoader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, _yaml_mapping)


def _unique_pairs(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    obj: dict[str, Any] = {}
    for key, value in pairs:
        _valid(key not in obj, "duplicate JSON mapping key")
        obj[key] = value
    return obj


def _reject_constant(token: str) -> None:
    raise PhysicalHistoryError("non-finite JSON literal")


def _parse_float(token: str) -> float:
    value = float(token)
    _valid(math.isfinite(value), "non-finite JSON float")
    return value


def _check_scalars(value: Any) -> None:
    if type(value) is dict:
        for key, child in value.items():
            _valid(type(key) is str, "non-string mapping key")
            _check_scalars(child)
    elif type(value) is list:
        for child in value:
            _check_scalars(child)
    elif type(value) is float:
        _valid(math.isfinite(value), "non-finite physical number")
    else:
        _valid(type(value) in (str, int, bool, type(None)), "unsupported YAML scalar")


def _closed(value: Any, names: set[str], message: str) -> dict:
    _valid(type(value) is dict and set(value) == names, message)
    return value


def _sha(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def _same_typed(actual: Any, expected: Any) -> bool:
    """Never accept Python's True == 1 or 1 == 1.0 for source-contract fields."""
    if type(actual) is not type(expected):
        return False
    if type(actual) is dict:
        return set(actual) == set(expected) and all(
            _same_typed(actual[k], expected[k]) for k in expected
        )
    if type(actual) in (list, tuple):
        return len(actual) == len(expected) and all(
            _same_typed(left, right) for left, right in zip(actual, expected)
        )
    return actual == expected


def _finite_number(value: Any) -> bool:
    if type(value) not in (int, float):
        return False
    try:
        return math.isfinite(value)
    except (OverflowError, ValueError):
        return False


def _snapshot(raw: bytes) -> dict:
    _valid(type(raw) is bytes and 0 < len(raw) <= _MAX_SNAPSHOT_BYTES,
           "snapshot bytes missing or over limit")
    try:
        text = raw.decode("utf-8", errors="strict")
        _valid(not any(isinstance(token, (AliasToken, AnchorToken, TagToken))
                       for token in yaml.scan(text)), "YAML aliases/anchors/tags forbidden")
        value = yaml.load(text, Loader=_UniqueLoader)
    except (UnicodeDecodeError, yaml.YAMLError) as exc:
        raise PhysicalHistoryError("invalid snapshot YAML") from exc
    _check_scalars(value)
    snap = _closed(value, _SNAPSHOT_KEYS, "snapshot root shape incompatible")
    _valid(type(snap["snapshot_version"]) is int and
           snap["snapshot_version"] == SNAPSHOT_VERSION, "unsupported physical snapshot version")
    _valid(type(snap["world_seed"]) is int, "invalid physical seed type")
    _valid(snap["sim_time"] == Y1_START, "source snapshot is not initial Y1_START")
    _valid(type(snap["snapshot_hash"]) is str and
           _SHA256.fullmatch(snap["snapshot_hash"]) is not None,
           "invalid snapshot document digest")
    _valid(snapshot_hash(snap) == snap["snapshot_hash"], "physical snapshot hash mismatch")
    return snap


def _records(raw: bytes) -> list[dict]:
    _valid(type(raw) is bytes and 0 < len(raw) <= _MAX_LOG_BYTES,
           "event log bytes missing or over limit")
    _valid(raw.endswith(b"\n"), "event log truncated: missing terminal LF")
    lines = raw.split(b"\n")[:-1]
    _valid(3 <= len(lines) <= _MAX_RECORDS, "event log count outside bounds")
    records = []
    for index, line in enumerate(lines):
        _valid(0 < len(line) <= _MAX_LINE_BYTES, "event log line outside bounds")
        try:
            rec = json.loads(
                line.decode("utf-8", errors="strict"),
                object_pairs_hook=_unique_pairs,
                parse_constant=_reject_constant,
                parse_float=_parse_float,
            )
        except (UnicodeDecodeError, json.JSONDecodeError, OverflowError) as exc:
            raise PhysicalHistoryError("invalid JSONL record") from exc
        _check_scalars(rec)
        record = _closed(rec, _RECORD_KEYS, "unknown physical event fields")
        _valid(type(record["seq"]) is int and record["seq"] == index,
               "non-contiguous local sequence")
        _valid(type(record["event"]) is str, "invalid physical event type")
        _valid(type(record["sim_time"]) is str, "invalid local sim_time type")
        _valid(type(record["payload"]) is dict, "invalid event payload type")
        wall = record["wall_time"]
        _valid(type(wall) is str and len(wall) <= 64 and wall.endswith("+00:00"),
               "invalid wall_time format")
        try:
            stamp = datetime.fromisoformat(wall)
            _valid(stamp.utcoffset() == timedelta(0), "wall_time must be UTC")
        except ValueError as exc:
            raise PhysicalHistoryError("invalid wall_time timestamp") from exc
        records.append(record)
    return records


def _seed_reference(snapshot: dict) -> tuple[dict[str, BodyState], dict[str, dict],
                                            dict[str, Any], PopulationPrior, Any]:
    """Rebuild seed as a *reference*, not a writer of a new physical snapshot."""
    meta = _closed(snapshot["run_metadata"], _METADATA_KEYS, "run metadata not closed")
    _valid(type(meta["world_seed"]) is int and
           meta["world_seed"] == snapshot["world_seed"], "seed metadata mismatch")
    prior = PopulationPrior.load(POPULATION_PRIOR_PATH)
    params = load_params()
    _valid(meta["prior_version"] == prior.version and
           meta["dynamics_version"] == params.model_version,
           "installed physical prior/dynamics version mismatch")
    entries = snapshot["characters"]
    _valid(type(entries) is dict and 0 < len(entries) <= _MAX_SUBJECTS,
           "invalid snapshot cohort size")
    ids = set(entries)
    _valid(all(type(k) is str and len(k) <= 128 and _SUBJECT_ID.fullmatch(k) is not None
               for k in ids), "invalid subject identifier")
    declared = _closed(meta["posterior_hash_by_character"], ids,
                       "posterior roster mismatch")
    suff = _closed(meta["evidence_sufficiency_by_character"], ids,
                   "evidence sufficiency roster mismatch")
    store = CapacityBaselineStore()
    bodies: dict[str, BodyState] = {}
    expected_samples: dict[str, dict] = {}
    posteriors: dict[str, Any] = {}
    for subject in sorted(ids):
        section = _closed(entries[subject], {"capacity_baseline", "body_state"},
                          "incomplete physical character")
        baseline = _closed(section["capacity_baseline"], _BASELINE_KEYS,
                           "incomplete capacity baseline")
        origin = _closed(baseline["sampled_from"], _ORIGIN_KEYS,
                         "baseline origin shape mismatch")
        _valid(origin["cohort_sex"] in ("male", "female") and
               origin["cohort_sex_source"] in ("KNOWN", "DRAWN"),
               "unsupported cohort covariate")
        covariates = (CohortCovariates(sex=origin["cohort_sex"])
                      if origin["cohort_sex_source"] == "KNOWN" else None)
        posterior = posterior_for(snapshot["world_seed"], subject, prior, covariates=covariates)
        record = seed_character(snapshot["world_seed"], subject, prior,
                                covariates=covariates, store=store)
        posteriors[subject] = posterior
        expected_origin = {
            "posterior_hash": record.posterior_hash,
            "prior_version": record.prior_version,
            "cohort_sex": record.cohort_sex,
            "cohort_sex_source": record.cohort_sex_source,
            "substream": record.substream,
        }
        expected_dims = {
            dim.value: {"value": entry.value, "unit": entry.unit}
            for dim, entry in record.profile.dimensions.items()
        }
        expected_suff = {d.value: val for d, val in record.evidence_sufficiency.items()}
        _valid(type(baseline["schema_version"]) is int and
               baseline["schema_version"] == 1 and
               baseline["sim_time"] == Y1_START and
               _same_typed(origin, expected_origin) and
               _same_typed(baseline["dimensions"], expected_dims) and
               _same_typed(baseline["evidence_sufficiency"], expected_suff),
               "seed baseline does not reproduce pinned physical model")
        _valid(type(declared[subject]) is str and
               declared[subject] == record.posterior_hash and
               _same_typed(suff[subject], expected_suff),
               "run provenance disagrees with seed")
        try:
            profile = capacity_profile_of(section)
            _valid(all(profile.value(d) == record.profile.value(d) for d in Dimension),
                   "stored capacity not equivalent to reconstructed capacity")
            body = body_state_of(section)
        except (ValueError, TypeError, KeyError, OverflowError) as exc:
            raise PhysicalHistoryError("invalid stored physical body/capacity") from exc
        initial = initial_body_state(subject, profile, params=params)
        _valid(_same_typed(section["body_state"], as_document(initial)),
               "initial physical body differs from pinned model")
        bodies[subject] = body
        expected_samples[subject] = capacity_sampled_payload(record, posterior)
    return bodies, expected_samples, posteriors, prior, (params, store)


def replay_body_history(snapshot_bytes: bytes, events_bytes: bytes) -> VerifiedPhysicalPrefix:
    """Read-only deterministic replay of *supported* PSV1-2 seed + body.advanced.

    A caller must pin a quiescent source pair before calling. Without a
    transaction/completion marker in the existing producer, a verified prefix
    is NOT proof that an advance-clock command finished successfully.
    """
    snapshot = _snapshot(snapshot_bytes)
    records = _records(events_bytes)
    bodies, samples, posteriors, prior, (params, store) = _seed_reference(snapshot)
    opening = records[0]
    _valid(opening["event"] == EVENT_RUN_STARTED and opening["sim_time"] == Y1_START,
           "local log missing run.started")
    _valid(_same_typed(opening["payload"], {
        "run_metadata": snapshot["run_metadata"],
        "required_components": ["prior", "dynamics"],
    }), "opening event provenance mismatch")

    # capacity.sampled order is local provenance only, not a causal ordering key.
    sampled: set[str] = set()
    i = 1
    while i < len(records) and records[i]["event"] == EVENT_CAPACITY_SAMPLED:
        event = records[i]
        _valid(event["sim_time"] == Y1_START, "seed sample local time mismatch")
        p = event["payload"]
        _valid(type(p) is dict and type(p.get("character_id")) is str,
               "invalid capacity.sampled subject")
        subject = p["character_id"]
        _valid(subject in samples and subject not in sampled and
               _same_typed(p, samples[subject]),
               "capacity.sampled differs from independently reproduced seed")
        sampled.add(subject)
        i += 1
    _valid(sampled == set(samples), "seed sample roster incomplete")
    _valid(i < len(records) and records[i]["event"] == EVENT_COHORT_CORRELATION_REPORT and
           records[i]["sim_time"] == Y1_START,
           "missing terminal seed correlation report")
    # Verify the complete recorded correlation payload rather than accepting
    # an arbitrary telemetry record with a matching cohort size.
    from .cli import correlation_report  # lazy import: CLI imports this module for the command
    _valid(_same_typed(records[i]["payload"], correlation_report(prior, store, posteriors)),
           "cohort correlation report mismatch")
    i += 1

    counts = {subject: 0 for subject in sorted(bodies)}
    active_subject: str | None = None
    active_count = 0
    used_subjects: set[str] = set()
    awaiting_extension = True
    phase_records: list[dict] = []
    while i < len(records):
        item = records[i]
        kind = item["event"]
        if kind == EVENT_RUN_EXTENDED:
            _valid(awaiting_extension or active_count > 0,
                   "empty run.extended phase in physical log")
            if phase_records:
                _check_phase_schedule(phase_records)
            _valid(item["sim_time"] == Y1_START and _same_typed(item["payload"], {
                "run_metadata": snapshot["run_metadata"],
                "required_components": ["prior", "dynamics"],
            }), "run.extended metadata mismatch")
            active_subject = None
            active_count = 0
            phase_records = []
            awaiting_extension = False
        elif kind == EVENT_BODY_ADVANCED:
            _valid(not awaiting_extension, "body.advanced without run.extended phase")
            subject = item["payload"].get("character_id")
            _valid(type(subject) is str and subject in bodies,
                   "unknown body.advanced character")
            if active_subject is None:
                _valid(subject not in used_subjects, "same body advanced in multiple phases")
                active_subject = subject
                used_subjects.add(subject)
            else:
                _valid(active_subject == subject, "run.extended phase mixes character IDs")
            # Pass profiles explicitly per call; avoid reliance on process-global context.
            p = capacity_profile_of(snapshot["characters"][subject])
            traits = BodyTraits.from_profile(p)
            _validate_transition(item, bodies, params, traits)
            counts[subject] += 1
            active_count += 1
            phase_records.append(item)
        else:
            raise PhysicalHistoryError("unsupported physical history event kind")
        i += 1
    _valid(awaiting_extension or active_count > 0,
           "trailing run.extended phase has no verified transition")
    if phase_records:
        _check_phase_schedule(phase_records)
    return VerifiedPhysicalPrefix(
        bodies=MappingProxyType(dict(sorted(bodies.items()))),
        advances_by_character=MappingProxyType(counts),
        snapshot_raw_sha256=_sha(snapshot_bytes),
        events_raw_sha256=_sha(events_bytes),
        snapshot_document_sha256=snapshot["snapshot_hash"],
        log_record_count=len(records),
        body_advance_count=sum(counts.values()),
    )


def _validate_transition(
    record: dict,
    bodies: dict[str, BodyState],
    params: Any,
    traits: BodyTraits,
) -> None:
    """Replay one record with a profile-derived exposure and validate its receipt."""
    payload = _closed(record["payload"], _ADVANCE_KEYS, "body.advanced payload not closed")
    subject = payload["character_id"]
    _valid(type(subject) is str and subject in bodies, "body.advanced subject unknown")
    _valid(payload["dynamics_version"] == params.model_version,
           "body.advanced dynamics version mismatch")
    dt = payload["dt_hours"]
    _valid(_finite_number(dt) and
           0 < dt <= params.integration.max_interval_hours,
           "invalid physical dt_hours")
    segment = payload["segment"]
    _valid(type(segment) is str and segment in ("awake", "asleep"),
           "unsupported physical interval segment")
    data = payload["environment"]
    _valid(type(data) is dict and type(data.get("wbgt_c")) in (int, float),
           "environment shape invalid")
    try:
        stored_env = Environment.model_validate(data)
        expected_env = Environment(
            wbgt_c=stored_env.wbgt_c,
            asleep=(segment == "asleep"),
            sleep_quality=stored_env.sleep_quality if segment == "asleep" else "good",
        )
    except (ValueError, ValidationError) as exc:
        raise PhysicalHistoryError("recorded environment invalid") from exc
    _valid(_same_typed(as_document(expected_env), data),
           "recorded environment not supported by advance-clock")
    old = bodies[subject]
    _valid(type(payload["t_hours_before"]) in (float, int) and
           payload["t_hours_before"] == old.t_hours,
           "physical time continuity mismatch")
    exposure = Exposure.of(environment=expected_env, traits=traits, params=params)
    _valid(_same_typed(payload["load"], as_document(exposure.load)),
           "unsupported PSV1-2 load")
    try:
        new = advance(old, dt, exposure)
    except ValueError as exc:
        raise PhysicalHistoryError("invalid replay interval") from exc
    _valid(type(payload["t_hours_after"]) in (float, int) and
           payload["t_hours_after"] == new.t_hours,
           "replayed end time mismatch")
    _valid(record["sim_time"] == logical_sim_time(Y1_START, new.t_hours),
           "replayed local sim_time mismatch")
    _valid(_same_typed(payload["channels_changed"], channel_diff(old, new)),
           "replayed physical channel_diff mismatch")
    bodies[subject] = new


def _check_phase_schedule(records: list[dict]) -> None:
    """Each PSV1-2 CLI phase is a prefix of repeated identical 24h day segments.

    This validates *the possible producer shape*, not that its intended day
    count completed. A crash after the awake segment is a valid local prefix.
    """
    _valid(bool(records), "empty physical phase")
    first = records[0]["payload"]
    first_hours = first["dt_hours"]
    first_kind = first["segment"]
    first_env = first["environment"]
    if first_kind == "asleep" or first_hours == 24:
        _valid(first_hours == 24, "single-segment PSV1-2 days require 24 hours")
        for record in records:
            part = record["payload"]
            _valid(part["segment"] == first_kind and part["dt_hours"] == 24 and
                   _same_typed(part["environment"], first_env),
                   "inconsistent repeated 24-hour physical segment")
    else:
        _valid(first_kind == "awake" and 0 < first_hours < 24,
               "invalid first day-segment for PSV1-2 CLI")
        sleep_env: dict | None = None
        for index, record in enumerate(records):
            part = record["payload"]
            if index % 2 == 0:
                _valid(part["segment"] == "awake" and part["dt_hours"] == first_hours and
                       _same_typed(part["environment"], first_env),
                       "awake segment differs from fixed PSV1-2 day plan")
            else:
                _valid(part["segment"] == "asleep" and
                       part["dt_hours"] == 24 - first_hours and
                       part["environment"]["wbgt_c"] == first_env["wbgt_c"],
                       "asleep segment does not close PSV1-2 day")
                if sleep_env is None:
                    sleep_env = part["environment"]
                _valid(_same_typed(part["environment"], sleep_env),
                       "sleep conditions changed within PSV1-2 command")


def verify_body_history(run_dir: str | Path) -> VerifiedPhysicalPrefix:
    """Convenience offline inspector; do not use against a concurrently writing run.

    File reads here do not establish A1's atomic/quiescent two-file source cut.
    Production integrations must pass a separately sealed/authorized byte pair.
    """
    directory = Path(run_dir)
    snapshot_path = directory / "snapshot.yaml"
    events_path = directory / "events.jsonl"
    if not snapshot_path.is_file() or not events_path.is_file():
        raise PhysicalHistoryError("seed snapshot and event log must both exist")
    if (snapshot_path.stat().st_size > _MAX_SNAPSHOT_BYTES or
            events_path.stat().st_size > _MAX_LOG_BYTES):
        raise PhysicalHistoryError("physical source exceeds continuity size limits")
    return replay_body_history(snapshot_path.read_bytes(), events_path.read_bytes())
