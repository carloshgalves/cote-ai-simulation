"""CSFRV1-9A/A2: reference-only physical source validator and seed-evidence projector.

This test-side transformer is deliberately *not* an Embodiment subprocess,
Go adapter, causal extension, EventDraft or authoritative reducer. It accepts
already-sealed bytes from the A1 boundary; Go alone can later admit/commit.
"""

from __future__ import annotations

import hashlib
import json
import math
import re
from collections.abc import Mapping
from datetime import datetime, timedelta
from pathlib import Path

import jsonschema
import yaml
from yaml.events import AliasEvent
from yaml.tokens import AnchorToken, TagToken

from embodiment.snapshot import snapshot_hash
from embodiment.prior import PopulationPrior, POPULATION_PRIOR_PATH
from embodiment.dynamics import initial_body_state, load_params
from embodiment.seeding import seed_character, CohortCovariates
from embodiment.types import BodyState, CapacityProfile, DIMENSION_UNITS, Dimension, DimensionValue, Y1_START, as_document

ROOT = Path(__file__).resolve().parents[2]
BUNDLE = ROOT / "docs" / "architecture" / "embodiment-causal-adapter-v1"
SCHEMA = BUNDLE / "schemas" / "seed-evidence-v1.schema.json"
POLICY = BUNDLE / "ipc-v1-operational-policy.json"
_HEX64 = re.compile(r"[0-9a-f]{64}\Z")
_SUBJECT = re.compile(r"[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*\Z")
_WALL_TIME = "1970-01-01T00:00:00+00:00"
_SAMPLE_KEYS = {
    "character_id", "posterior_hash", "prior_version", "prior_content_hash",
    "posterior_kind", "substream", "sex", "sex_source", "constraint_count",
    "estimator_ess", "evidence_sufficiency", "dimensions",
}
_BASELINE_KEYS = {"schema_version", "sim_time", "sampled_from", "evidence_sufficiency", "dimensions"}
_BODY_KEYS = set(BodyState.model_fields)
_META_KEYS = {
    "world_seed", "prior_version", "dynamics_version",
    "posterior_hash_by_character", "evidence_sufficiency_by_character",
}


class SourceRejected(ValueError):
    """An untrusted physical source failed the A2 contract before Go admission."""


def _require(test: bool, diagnostic: str) -> None:
    if not test:
        raise SourceRejected(diagnostic)


def _strict_pairs(pairs):
    found = {}
    for key, value in pairs:
        if key in found:
            raise SourceRejected("duplicate JSON mapping key")
        found[key] = value
    return found


def _bad_constant(value):
    raise SourceRejected("non-finite JSON number")


def _reject_non_json(value):
    if isinstance(value, dict):
        _require(all(type(k) is str for k in value), "non-string YAML key")
        for child in value.values():
            _reject_non_json(child)
    elif isinstance(value, list):
        for child in value:
            _reject_non_json(child)
    else:
        _require(value is None or type(value) in (str, int, float, bool), "non-JSON YAML scalar")
        if type(value) is float:
            _require(math.isfinite(value), "non-finite YAML number")


class _UniqueSafeLoader(yaml.SafeLoader):
    pass


def _unique_mapping(loader, node):
    result = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=True)
        _require(type(key) is str, "non-string YAML key")
        _require(key not in result, "duplicate YAML mapping key")
        result[key] = loader.construct_object(value_node, deep=True)
    return result


_UniqueSafeLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, _unique_mapping
)


def _json_line(raw):
    try:
        return json.loads(
            raw.decode("utf-8", errors="strict"),
            object_pairs_hook=_strict_pairs,
            parse_constant=_bad_constant,
        )
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise SourceRejected("invalid UTF-8/JSONL record") from exc


def _object(value, fields, diagnostic):
    _require(type(value) is dict and set(value) == set(fields), diagnostic)
    return value


def _sha256(raw):
    return hashlib.sha256(raw).hexdigest()


def _finite_number(value) -> bool:
    if type(value) not in (int, float):
        return False
    try:
        return math.isfinite(value)
    except (OverflowError, ValueError):
        return False


def _document_hash(value):
    """Local *reference* hash; not an ADR 0009 canonical CBOR digest."""
    encoded = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False)
    return _sha256(encoded.encode("utf-8"))


def _source_snapshot(data: bytes):
    try:
        # Prevent aliases (billion laughs), anchors and tags, not merely unsafe constructors.
        tokens = list(yaml.scan(data.decode("utf-8", errors="strict")))
        _require(not any(isinstance(token, (AnchorToken, TagToken)) for token in tokens),
                 "YAML anchor/tag forbidden")
        events = yaml.parse(data)
        _require(not any(isinstance(event, AliasEvent) for event in events),
                 "YAML alias forbidden")
        document = yaml.load(data, Loader=_UniqueSafeLoader)
    except (yaml.YAMLError, UnicodeDecodeError) as exc:
        raise SourceRejected("untrusted YAML parse failed") from exc
    _reject_non_json(document)
    _object(document, {
        "snapshot_version", "world_seed", "sim_time", "characters",
        "run_metadata", "snapshot_hash",
    }, "snapshot root not closed")
    _require(type(document["snapshot_version"]) is int and document["snapshot_version"] == 1,
             "snapshot_version unsupported")
    _require(document["sim_time"] == Y1_START, "seed snapshot local clock mismatch")
    _require(type(document["world_seed"]) is int, "world_seed invalid")
    _require(type(document["snapshot_hash"]) is str and bool(_HEX64.fullmatch(document["snapshot_hash"])),
             "snapshot_hash invalid")
    _require(snapshot_hash(document) == document["snapshot_hash"],
             "snapshot_hash disagrees with document")
    return document


def _source_log(raw: bytes, *, max_line: int, max_events: int):
    _require(raw.endswith(b"\n"), "truncated JSONL: missing terminal LF")
    lines = raw.split(b"\n")
    _require(lines[-1] == b"", "invalid JSONL termination")
    lines = lines[:-1]
    _require(2 <= len(lines) <= max_events, "source event count invalid")
    records = []
    normalized = []
    for position, line in enumerate(lines):
        _require(0 < len(line) <= max_line, "source event line size invalid")
        record = _json_line(line)
        _reject_non_json(record)  # rejects 1e400 -> Infinity, not just NaN literals
        _object(record, {"seq", "event", "sim_time", "wall_time", "payload"}, "event keys not closed")
        _require(type(record["seq"]) is int and record["seq"] == position,
                 "source seq not contiguous zero-based")
        _require(type(record["event"]) is str and
                 record["event"] in {"run.started", "capacity.sampled", "cohort.correlation_report"},
                 "unsupported history: physical event kind not yet admitted")
        _require(record["sim_time"] == Y1_START, "seed event sim_time mismatch")
        _require(type(record["wall_time"]) is str and len(record["wall_time"]) <= 64 and
                 record["wall_time"].endswith("+00:00"), "wall_time invalid")
        try:
            utc_instant = datetime.fromisoformat(record["wall_time"])
        except ValueError as exc:
            raise SourceRejected("wall_time ISO instant invalid") from exc
        _require(utc_instant.utcoffset() == timedelta(0), "wall_time must be UTC")
        _require(type(record["payload"]) is dict, "event payload invalid")
        normalized_record = dict(record, wall_time=_WALL_TIME)
        normalized.append(json.dumps(normalized_record, ensure_ascii=False, sort_keys=True))
        records.append(record)
    digest = _sha256(("\n".join(normalized) + "\n").encode("utf-8"))
    return records, digest


def project_seed_source(
    snapshot_bytes: bytes,
    events_bytes: bytes,
    *,
    snapshot_raw_sha256: str,
    events_raw_sha256: str,
    snapshot_document_sha256: str,
    normalised_log_sha256: str,
) -> dict:
    """Fail closed; return a typed *evidence reference*, not an EventDraft.

    Caller guarantees A1 quiescence, immutable source view and source authorization.
    This function validates source shape/lineage as a second, non-authoritative gate.
    """
    settings = json.loads(POLICY.read_text(encoding="utf-8"))
    lim = settings["limits"]
    _require(type(snapshot_bytes) is bytes and type(events_bytes) is bytes, "unsealed source bytes")
    _require(0 < len(snapshot_bytes) <= lim["source_snapshot_bytes"], "snapshot raw size limit")
    _require(0 < len(events_bytes) <= lim["source_events_bytes"], "events raw size limit")
    _require(type(snapshot_raw_sha256) is str and bool(_HEX64.fullmatch(snapshot_raw_sha256)),
             "snapshot expected SHA invalid")
    _require(type(events_raw_sha256) is str and bool(_HEX64.fullmatch(events_raw_sha256)),
             "events expected SHA invalid")
    _require(_sha256(snapshot_bytes) == snapshot_raw_sha256, "snapshot raw SHA mismatch")
    _require(_sha256(events_bytes) == events_raw_sha256, "events raw SHA mismatch")
    _require(type(snapshot_document_sha256) is str and bool(_HEX64.fullmatch(snapshot_document_sha256)),
             "expected snapshot document digest invalid")
    _require(type(normalised_log_sha256) is str and bool(_HEX64.fullmatch(normalised_log_sha256)),
             "expected normalised log digest invalid")
    snapshot = _source_snapshot(snapshot_bytes)
    _require(snapshot["snapshot_hash"] == snapshot_document_sha256,
             "A1 snapshot document digest mismatch")
    records, normalized_hash = _source_log(
        events_bytes,
        max_line=lim["source_event_line_bytes"],
        max_events=lim["source_event_count"],
    )

    _require(normalized_hash == normalised_log_sha256, "A1 normalised log digest mismatch")
    meta = _object(snapshot["run_metadata"], _META_KEYS, "run_metadata keys not closed")
    _require(type(meta["world_seed"]) is int and
             meta["world_seed"] == snapshot["world_seed"], "world_seed provenance mismatch")
    for name in ("prior_version", "dynamics_version"):
        _require(type(meta[name]) is str and 0 < len(meta[name]) <= 96,
                 "missing/unbounded component version")
    # The reference test version pins both physical parameter files as read-only
    # sources, rather than treating self-declared run metadata as trusted proof.
    prior = PopulationPrior.load(POPULATION_PRIOR_PATH)
    params = load_params()
    _require(meta["prior_version"] == prior.version, "unsupported installed prior version")
    _require(meta["dynamics_version"] == params.model_version, "unsupported installed dynamics version")
    characters = snapshot["characters"]
    _require(type(characters) is dict and 0 < len(characters) <= 10000,
             "snapshot character count invalid")
    ids = set(characters)
    _require(all(type(k) is str and len(k) <= 128 and _SUBJECT.fullmatch(k) for k in ids),
             "invalid subject ID")
    hashes = _object(meta["posterior_hash_by_character"], ids,
                     "posterior provenance does not equal subjects")
    suff = _object(meta["evidence_sufficiency_by_character"], ids,
                   "evidence sufficiency does not equal subjects")
    for digest in hashes.values():
        _require(type(digest) is str and bool(_HEX64.fullmatch(digest)), "posterior hash invalid")

    _require(records[0]["event"] == "run.started", "missing opening event")
    opening = _object(records[0]["payload"], {"run_metadata", "required_components"}, "opening payload")
    _require(opening["run_metadata"] == meta, "log/snapshot metadata mismatch")
    _require(opening["required_components"] == ["prior", "dynamics"],
             "unexpected seeding component versions")

    _require(records[-1]["event"] == "cohort.correlation_report",
             "missing terminal correlation report")
    corr = _object(records[-1]["payload"], {
        "cohort_size", "scale", "method", "prior_version", "prior_status",
        "pairs", "material_pairs_threshold", "material_pairs_sign_agreement", "note",
    }, "correlation report fields not closed")
    _require(type(corr["cohort_size"]) is int and corr["cohort_size"] == len(ids),
             "correlation report cohort mismatch")
    _require(corr["prior_version"] == meta["prior_version"] and
             corr["prior_status"] == prior.status,
             "correlation report prior mismatch")
    _require(type(corr["scale"]) is str and type(corr["method"]) is str and
             type(corr["note"]) is str and len(corr["note"]) > 0,
             "correlation report descriptive fields invalid")
    _require(_finite_number(corr["material_pairs_threshold"]) and
             0 <= corr["material_pairs_threshold"] <= 1,
             "correlation report threshold invalid")
    counts = _object(corr["material_pairs_sign_agreement"], {"agree", "disagree"},
                     "correlation report sign count keys invalid")
    _require(all(type(value) is int and value >= 0 for value in counts.values()),
             "correlation sign counts invalid")
    pairs = corr["pairs"]
    _require(type(pairs) is list and len(pairs) == 5, "correlation pair count invalid")
    from embodiment.cli import REPORTED_PAIRS
    expected_pairs = [tuple(dim.value for dim in pair) for pair in REPORTED_PAIRS]
    _require(len(expected_pairs) == 5, "reference correlation pair policy drift")
    seen_pairs = set()
    for offset, pair in enumerate(pairs):
        pair = _object(pair, {
            "dimensions", "expected_from_loadings", "observed", "sign_agrees",
        }, "correlation pair fields not closed")
        dims = pair["dimensions"]
        _require(type(dims) is list and len(dims) == 2 and
                 all(type(d) is str and d in {dim.value for dim in Dimension} for d in dims) and
                 dims[0] != dims[1] and tuple(dims) not in seen_pairs,
                 "correlation pair dimension names invalid")
        seen_pairs.add(tuple(dims))
        _require(tuple(dims) == expected_pairs[offset],
                 "correlation pair not in the declared source ordering")
        expected = pair["expected_from_loadings"]
        observed = pair["observed"]
        agree = pair["sign_agrees"]
        _require(_finite_number(expected) and
                 -1 <= expected <= 1, "correlation expected value invalid")
        _require(observed is None or (_finite_number(observed) and -1 <= observed <= 1),
                 "correlation observed value invalid")
        _require((observed is None and agree is None) or
                 (observed is not None and type(agree) is bool and
                  agree == (observed * expected > 0)),
                 "correlation sign inference invalid")
    _require(len(records) == len(ids) + 2, "unsupported history: only seed records are admissible")

    samples = {}
    for record in records[1:-1]:
        _require(record["event"] == "capacity.sampled", "unsupported event kind or advanced history")
        payload = _object(record["payload"], _SAMPLE_KEYS, "capacity.sampled payload keys not closed")
        subject = payload["character_id"]
        _require(type(subject) is str and subject in ids and subject not in samples,
                 "unknown or duplicate capacity.sampled subject")
        samples[subject] = (record["seq"], payload)

    _require(set(samples) == ids, "missing capacity sample")
    subjects = []
    dimension_ids = {dim.value for dim in Dimension}
    for subject in sorted(ids):
        section = _object(characters[subject], {"capacity_baseline", "body_state"},
                          "snapshot character sections incomplete")
        baseline = _object(section["capacity_baseline"], _BASELINE_KEYS, "baseline shape incomplete")
        _require(baseline["schema_version"] == 1 and type(baseline["schema_version"]) is int,
                 "baseline schema_version invalid")
        _require(baseline["sim_time"] == Y1_START, "baseline local time invalid")
        origin = _object(baseline["sampled_from"], {
            "posterior_hash", "prior_version", "cohort_sex", "cohort_sex_source", "substream",
        }, "baseline provenance shape invalid")
        sample_seq, sample = samples[subject]
        _require(sample["character_id"] == subject, "sample character mismatch")
        _require(origin["posterior_hash"] == hashes[subject] == sample["posterior_hash"],
                 "sample posterior mismatch")
        _require(origin["prior_version"] == meta["prior_version"] == sample["prior_version"],
                 "sample prior mismatch")
        _require(origin["substream"] == sample["substream"] and
                 origin["cohort_sex"] == sample["sex"] and
                 origin["cohort_sex_source"] == sample["sex_source"],
                 "sample covariate provenance mismatch")
        _require(origin["cohort_sex"] in ("male", "female") and
                 origin["cohort_sex_source"] in ("KNOWN", "DRAWN"),
                 "covariate provenance invalid")
        _require(type(sample["prior_content_hash"]) is str and
                 sample["prior_content_hash"] == prior.content_hash,
                 "prior model content hash mismatch")
        _require(sample["posterior_kind"] == "PRIOR_ZERO_EVIDENCE" and
                 type(sample["constraint_count"]) is int and sample["constraint_count"] == 0 and
                 sample["estimator_ess"] is None,
                 "unsupported capacity estimator: PSV1-3 not installed")
        evidence = _object(suff[subject], dimension_ids, "run evidence dimension set invalid")
        _require(_object(baseline["evidence_sufficiency"], dimension_ids,
                         "baseline evidence dimensions invalid") == evidence ==
                 _object(sample["evidence_sufficiency"], dimension_ids,
                         "sample evidence dimensions invalid"),
                 "evidence sufficiency mismatch")
        _require(all(type(x) in (float, int) and x == 0 for x in evidence.values()),
                 "nonzero evidence requires estimator PSV1-3")

        dims = _object(baseline["dimensions"], dimension_ids, "baseline dimensions not closed")
        sample_dims = _object(sample["dimensions"], dimension_ids, "sample dimensions not closed")
        profile_values = {}
        for dim in Dimension:
            value = _object(dims[dim.value], {"value", "unit"}, "dimension value/unit keys invalid")
            _require(type(value["unit"]) is str and value["unit"] == DIMENSION_UNITS[dim].unit,
                     "capacity dimension unit mismatch")
            _require(type(value["value"]) in (int, float), "capacity dimension value invalid")
            try:
                numeric_value = float(value["value"])
            except (OverflowError, ValueError) as exc:
                raise SourceRejected("capacity dimension value overflow") from exc
            _require(math.isfinite(numeric_value), "capacity dimension value invalid")
            _require(value == sample_dims[dim.value], "sample/snapshot capacity mismatch")
            try:
                profile_values[dim] = DimensionValue(value=numeric_value, unit=value["unit"])
            except ValueError as exc:
                raise SourceRejected("invalid physical dimension value") from exc
        try:
            profile = CapacityProfile(dimensions=profile_values)  # read-side shape validation
        except ValueError as exc:
            raise SourceRejected("invalid physical capacity profile") from exc
        # A seed sample is a deterministic physical draw; verifying that the
        # installed prior and substream reproduce it catches self-signed edits.
        covariates = (CohortCovariates(sex=origin["cohort_sex"])
                      if origin["cohort_sex_source"] == "KNOWN" else None)
        reference_seed = seed_character(snapshot["world_seed"], subject, prior,
                                        covariates=covariates, store=None, log=None)
        _require(reference_seed.posterior_hash == hashes[subject] and
                 reference_seed.substream == origin["substream"] and
                 reference_seed.cohort_sex == origin["cohort_sex"] and
                 reference_seed.cohort_sex_source == origin["cohort_sex_source"],
                 "deterministic seed provenance mismatch")
        for dim in Dimension:
            _require(profile.value(dim) == reference_seed.profile.value(dim),
                     "seed capacity cannot be reproduced from pinned model")

        body = _object(section["body_state"], _BODY_KEYS, "body_state keys not closed")
        _require(body["character_id"] == subject and type(body["t_hours"]) in (float, int) and
                 body["t_hours"] == 0, "seed body id/time invalid")
        _require(body["illnesses"] == [], "PSV1-2 cannot write illness")
        try:
            BodyState.model_validate(body)  # read-side validation
        except ValueError as exc:
            raise SourceRejected("invalid physical BodyState") from exc
        expected_body = as_document(initial_body_state(subject, profile, params=params))
        _require(body == expected_body, "seed body cannot be reproduced from pinned dynamics")
        subjects.append({
            "subject_id": subject,
            "engine_only": True,
            "source_baseline_pointer": f"/characters/{subject}/capacity_baseline",
            "source_body_pointer": f"/characters/{subject}/body_state",
            "source_local_sample_seq": sample_seq,
            "posterior_hash": hashes[subject],
            "baseline_sha256": _document_hash(baseline),
            "body_state_sha256": _document_hash(body),
        })

    result = {
        "kind": "physical.seed-evidence.v1",
        "schema_version": 1,
        "authority": "ENGINE_ONLY",
        "admission": "PENDING_GO_ADMISSION",
        "source": {
            "snapshot_raw_sha256": snapshot_raw_sha256,
            "events_raw_sha256": events_raw_sha256,
            "snapshot_document_sha256": snapshot["snapshot_hash"],
            "normalised_log_sha256": normalized_hash,
            "snapshot_version": 1,
            "local_origin": Y1_START,
            "prior_version": meta["prior_version"],
            "dynamics_version": meta["dynamics_version"],
            "source_kind": "sealed-physical-seed-only",
        },
        "subjects": subjects,
    }
    try:
        jsonschema.Draft202012Validator(json.loads(SCHEMA.read_text(encoding="utf-8"))).validate(result)
    except jsonschema.ValidationError as exc:
        raise SourceRejected("output schema refused source projection") from exc
    return result


def reference_bytes(value: Mapping) -> bytes:
    """A2 stable *transport* JSON, explicitly NOT canonical CSF CBOR/digest."""
    return json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(",", ":"), allow_nan=False).encode("utf-8")


def verify_prepared_evidence(
    evidence: Mapping,
    snapshot_bytes: bytes,
    events_bytes: bytes,
    *,
    snapshot_raw_sha256: str,
    events_raw_sha256: str,
    snapshot_document_sha256: str,
    normalised_log_sha256: str,
) -> None:
    """Reference re-verification for a possibly tampered engine-only projection.

    A2 is not an authority for Go. Go must independently rederive/validate this
    relationship, under a pinned source and runtime policy, before its own admission.
    """
    schema = json.loads(SCHEMA.read_text(encoding="utf-8"))
    try:
        jsonschema.Draft202012Validator(schema).validate(evidence)
    except jsonschema.ValidationError as exc:
        raise SourceRejected("prepared evidence violates closed schema") from exc
    expected = project_seed_source(
        snapshot_bytes,
        events_bytes,
        snapshot_raw_sha256=snapshot_raw_sha256,
        events_raw_sha256=events_raw_sha256,
        snapshot_document_sha256=snapshot_document_sha256,
        normalised_log_sha256=normalised_log_sha256,
    )
    _require(reference_bytes(evidence) == reference_bytes(expected),
             "prepared evidence does not bind to the sealed A1 source")
