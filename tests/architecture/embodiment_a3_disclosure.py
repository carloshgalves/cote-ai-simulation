"""CSFRV1-9A/A3: fail-closed disclosure contract for *unadmitted* A2 evidence.

This reference is neither a perception engine nor an authorization service.
It classifies every terminal path of the closed A2 JSON Schema and accepts
only an inert, recipient-addressed qualitative CANDIDATE shape. It never
grants access to KnowledgeInput, Context, Observation or Go WorldRevision.
"""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any, Mapping

import jsonschema


ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / "docs/architecture/embodiment-causal-adapter-v1"
SCHEMA = BASE / "schemas/verified-transition-evidence-v1.schema.json"
POLICY = BASE / "physical-disclosure-matrix-a3.json"
MAX_CANDIDATE_BYTES = 4096
CLASSIFICATIONS = frozenset({
    "ENGINE_ONLY", "FORBIDDEN_IN_CONTEXT", "QUALITATIVE_DERIVATION_ONLY",
})



# Review-pinned approved provenance channels: editing JSON policy alone cannot
# broaden what physiology may serve as a qualitative signal.
APPROVED_QUALITATIVE_PATHS = frozenset({
    "subject_tips[].verified_body.central_fatigue",
    "subject_tips[].verified_body.energy.substrate_availability",
    "subject_tips[].verified_body.hydration.deficit_pct_body_mass",
    "subject_tips[].verified_body.pain.analgesia",
    "subject_tips[].verified_body.pain.by_region.arms",
    "subject_tips[].verified_body.pain.by_region.core",
    "subject_tips[].verified_body.pain.by_region.grip",
    "subject_tips[].verified_body.pain.by_region.legs",
    "subject_tips[].verified_body.pain.global_intensity",
    "subject_tips[].verified_body.peripheral_fatigue.arms",
    "subject_tips[].verified_body.peripheral_fatigue.core",
    "subject_tips[].verified_body.peripheral_fatigue.grip",
    "subject_tips[].verified_body.peripheral_fatigue.legs",
    "subject_tips[].verified_body.sleep.circadian_phase",
    "subject_tips[].verified_body.sleep.debt_hours",
    "subject_tips[].verified_body.sleep.hours_since_wake",
    "subject_tips[].verified_body.sleep.last_sleep",
    "subject_tips[].verified_body.sleep.last_sleep.end_h",
    "subject_tips[].verified_body.sleep.last_sleep.quality",
    "subject_tips[].verified_body.sleep.last_sleep.start_h",
    "subject_tips[].verified_body.soreness.by_region.arms.expressed",
    "subject_tips[].verified_body.soreness.by_region.arms.latency[]",
    "subject_tips[].verified_body.soreness.by_region.arms.pending_onset",
    "subject_tips[].verified_body.soreness.by_region.core.expressed",
    "subject_tips[].verified_body.soreness.by_region.core.latency[]",
    "subject_tips[].verified_body.soreness.by_region.core.pending_onset",
    "subject_tips[].verified_body.soreness.by_region.grip.expressed",
    "subject_tips[].verified_body.soreness.by_region.grip.latency[]",
    "subject_tips[].verified_body.soreness.by_region.grip.pending_onset",
    "subject_tips[].verified_body.soreness.by_region.legs.expressed",
    "subject_tips[].verified_body.soreness.by_region.legs.latency[]",
    "subject_tips[].verified_body.soreness.by_region.legs.pending_onset",
    "subject_tips[].verified_body.soreness.repeated_bout_adaptation",
    "subject_tips[].verified_body.thermal.core_offset_c",
})
APPROVED_QUALITATIVE_LABELS = frozenset({
    "fatigue_low", "fatigue_moderate", "fatigue_high",
    "thirst_low", "thirst_high", "pain_low", "pain_high",
    "sleepiness_low", "sleepiness_high",
})
POLICY_FIELDS = frozenset({
    "policy_version", "source_schema", "source_schema_id",
    "schema_sha256_state", "source_schema_git_blob_sha",
    "exhaustive_schema_paths", "default_classification",
    "agent_context_direct_disclosure", "future_qualitative_candidate",
    "forbidden_artifact_classes", "fields",
})
REQUIRED_FORBIDDEN_CLASSES = frozenset({
    "BodyState", "CapacityProfile", "posterior", "capacity.sampled",
    "cohort.correlation_report", "random_seed", "rng_state",
    "third_party_body", "source_raw_bytes", "command_receipts",
    "local_physical_clock_as_global",
})


class DisclosureRejected(ValueError):
    """Sanitized denial; never include source values, agent text or secrets."""


def _deny() -> None:
    raise DisclosureRejected("physical disclosure contract rejected")


def _load_json(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def all_terminal_schema_paths(schema: Mapping[str, Any]) -> frozenset[str]:
    """Walk strict JSON-schema object/array/oneOf/$ref, with unknown forms denied."""
    defs = schema.get("$defs", {})
    leaves: set[str] = set()

    def visit(node: Any, path: str, depth: int, refs: frozenset[str]) -> None:
        if depth > 40 or not isinstance(node, dict):
            _deny()
        if "$ref" in node:
            ref = node["$ref"]
            if (not isinstance(ref, str) or not ref.startswith("#/$defs/")
                    or ref in refs or ref.split("/")[-1] not in defs):
                _deny()
            visit(defs[ref.split("/")[-1]], path, depth + 1, refs | {ref})
            return
        if "oneOf" in node:
            branches = node["oneOf"]
            if not isinstance(branches, list) or not branches:
                _deny()
            for branch in branches:
                visit(branch, path, depth + 1, refs)
            return
        if node.get("type") == "object":
            props = node.get("properties")
            if (node.get("additionalProperties") is not False
                    or not isinstance(props, dict) or not props
                    or set(node.get("required", [])) != set(props)):
                _deny()
            for name, child in props.items():
                if not isinstance(name, str) or not name or "." in name or "[]" in name:
                    _deny()
                visit(child, f"{path}.{name}" if path else name, depth + 1, refs)
            return
        if node.get("type") == "array":
            if "items" not in node or "prefixItems" in node:
                _deny()
            visit(node["items"], path + "[]", depth + 1, refs)
            return
        if "type" not in node and "const" not in node and "enum" not in node:
            _deny()
        if not path:
            _deny()
        leaves.add(path)

    visit(schema, "", 0, frozenset())
    return frozenset(leaves)


def validate_policy(*, schema: dict | None = None, policy: dict | None = None) -> None:
    stored_schema = _load_json(SCHEMA)
    schema = stored_schema if schema is None else schema
    policy = _load_json(POLICY) if policy is None else policy
    # A field-only inventory misses changed numerical constraints, units,
    # enums or hidden additionalProperties. Pin ALL bytes of the A2 schema.
    raw = SCHEMA.read_bytes()
    blob_sha = hashlib.sha1(b"blob " + str(len(raw)).encode("ascii") + b"\x00" + raw).hexdigest()
    if policy.get("source_schema_git_blob_sha") != blob_sha or schema != stored_schema:
        _deny()
    try:
        jsonschema.Draft202012Validator.check_schema(schema)
    except jsonschema.SchemaError:
        _deny()
    if set(policy) != POLICY_FIELDS:
        _deny()
    if policy.get("source_schema") != "verified-transition-evidence-v1.schema.json":
        _deny()
    if policy.get("schema_sha256_state") != "source_git_blob_sha_pinned_and_verified":
        _deny()
    forbidden = policy.get("forbidden_artifact_classes")
    if (type(forbidden) is not list or
            not all(type(x) is str for x in forbidden) or
            len(forbidden) != len(REQUIRED_FORBIDDEN_CLASSES) or
            set(forbidden) != REQUIRED_FORBIDDEN_CLASSES):
        _deny()
    if policy.get("policy_version") != "csfrv1-9a-a3.v1":
        _deny()
    if policy.get("source_schema_id") != schema.get("$id"):
        _deny()
    if policy.get("default_classification") != "REJECT_UNKNOWN":
        _deny()
    if policy.get("agent_context_direct_disclosure") is not False:
        _deny()
    if policy.get("exhaustive_schema_paths") is not True:
        _deny()
    listed = policy.get("fields")
    paths = all_terminal_schema_paths(schema)
    if not isinstance(listed, dict) or set(listed) != paths:
        _deny()
    for path, rule in listed.items():
        if not isinstance(rule, dict) or set(rule) != {
            "classification", "direct_disclosure",
            "requires_go_authentication", "recipient_scope",
        }:
            _deny()
        classification = rule["classification"]
        if classification not in CLASSIFICATIONS or rule["direct_disclosure"] is not False:
            _deny()
        expected_scope = "SELF_OBSERVER_ONLY" if classification == "QUALITATIVE_DERIVATION_ONLY" else "NONE"
        if rule["recipient_scope"] != expected_scope:
            _deny()
        if rule["requires_go_authentication"] is not (classification == "QUALITATIVE_DERIVATION_ONLY"):
            _deny()
        if classification == "QUALITATIVE_DERIVATION_ONLY" and path not in APPROVED_QUALITATIVE_PATHS:
            _deny()
        if path in APPROVED_QUALITATIVE_PATHS and classification != "QUALITATIVE_DERIVATION_ONLY":
            _deny()
        if path.startswith("subject_tips[].verified_body.") and classification == "ENGINE_ONLY":
            _deny()
    proposal = policy.get("future_qualitative_candidate")
    if not isinstance(proposal, dict) or proposal.get("authority") != "NOT_ADMITTED":
        _deny()
    if proposal.get("go_authorization_required") is not True:
        _deny()
    if proposal.get("provenance_requires_verified_source") is not True:
        _deny()
    if proposal.get("subject_scope") != "SELF_ONLY":
        _deny()
    signals = proposal.get("values")
    if (type(signals) is not list or not all(type(x) is str for x in signals)
            or len(signals) != len(APPROVED_QUALITATIVE_LABELS)
            or set(signals) != APPROVED_QUALITATIVE_LABELS):
        _deny()
    if not all(isinstance(x, str) and x.isascii() and x.replace("_", "").islower() for x in signals):
        _deny()


def validate_source(evidence: Any, *, policy: dict | None = None) -> None:
    """Schema and classification guard only; source authenticity remains A1/Go."""
    schema = _load_json(SCHEMA)
    validate_policy(schema=schema, policy=policy)
    if type(evidence) is not dict:
        _deny()
    try:
        jsonschema.Draft202012Validator(schema).validate(evidence)
    except jsonschema.ValidationError:
        _deny()
    if (evidence.get("authority") != "ENGINE_ONLY"
            or evidence.get("admission") != "PENDING_GO_ADMISSION"):
        _deny()


def validate_qualitative_candidate(candidate: Any, evidence: Mapping[str, Any]) -> dict:
    """Validate a non-authoritative candidate, never approve an Observation.

    Caller-supplied IDs and labels cannot authenticate Go; this function only
    proves a narrow, closed, recipient-addressed shape, not perceptual truth.
    """
    validate_source(evidence)
    if type(candidate) is not dict:
        _deny()
    fields = {
        "kind", "schema_version", "authority", "recipient_id",
        "observer_id", "subject_id", "signal", "source_tip_sha256",
    }
    if set(candidate) != fields:
        _deny()
    if (candidate["kind"] != "physical.qualitative-observation-candidate.v1"
            or type(candidate["schema_version"]) is not int
            or candidate["schema_version"] != 1
            or candidate["authority"] != "NOT_ADMITTED"):
        _deny()
    policy = _load_json(POLICY)
    if candidate["signal"] not in policy["future_qualitative_candidate"]["values"]:
        _deny()
    subject = candidate["subject_id"]
    if type(subject) is not str or not subject or len(subject) > 128:
        _deny()
    if candidate["observer_id"] != subject or candidate["recipient_id"] != subject:
        _deny()
    matching = [tip for tip in evidence["subject_tips"] if tip["subject_id"] == subject]
    if len(matching) != 1 or matching[0]["visibility"] != "ENGINE_ONLY":
        _deny()
    if candidate["source_tip_sha256"] != matching[0]["verified_body_sha256"]:
        _deny()
    try:
        encoded = json.dumps(candidate, sort_keys=True, ensure_ascii=False,
                             separators=(",", ":"), allow_nan=False).encode("utf-8")
    except (TypeError, ValueError, OverflowError):
        _deny()
    if len(encoded) > MAX_CANDIDATE_BYTES:
        _deny()
    return json.loads(encoded)


def refuse_agent_context(_: Any) -> None:
    """No A3 physical data or qualitative candidate is admitted to actor context."""
    _deny()
