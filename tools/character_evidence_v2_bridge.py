"""Fail-closed adapter for legacy Character Evidence V0 -> CanonEvidence V2.

Legacy records remain pack/eval evidence, never automatic RAG input. No record
can be exported without an explicit human-reviewed, per-record authorization
and a confirmed narrative position. This module does not index anything.
"""
from __future__ import annotations

from copy import deepcopy
import json
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator
from referencing import Registry, Resource
import yaml

ROOT = Path(__file__).resolve().parents[1]
SCHEMAS = ROOT / "data/canon/schema"
LEGACY_DIR = ROOT / "data/canon/evidence/characters"


class EvidenceNotAdmitted(ValueError):
    """The record cannot be represented as retrievable CanonEvidence V2."""


def _validator(schema_file: str) -> Draft202012Validator:
    names = (
        "common.schema.json",
        "evidence.schema.json",
        "evidence_collection.schema.json",
        "character_evidence_legacy_record.schema.json",
        "character_evidence_legacy_collection.schema.json",
    )
    resources = [
        json.loads((SCHEMAS / name).read_text(encoding="utf-8"))
        for name in names
    ]
    registry = Registry()
    for schema in resources:
        Draft202012Validator.check_schema(schema)
        registry = registry.with_resource(schema["$id"], Resource.from_contents(schema))
    selected = next(schema for schema in resources if schema["$id"].endswith("/" + schema_file))
    return Draft202012Validator(
        selected, registry=registry, format_checker=Draft202012Validator.FORMAT_CHECKER
    )


def load_legacy_records() -> dict[str, dict[str, Any]]:
    """Validate all six *legacy* collections under namespaced, non-RAG schemas."""
    validator = _validator("character_evidence_legacy_collection.schema.json")
    records: dict[str, dict[str, Any]] = {}
    for path in sorted(LEGACY_DIR.glob("*.yaml")):
        collection = yaml.safe_load(path.read_text(encoding="utf-8"))
        validator.validate(collection)
        for entry in collection["records"]:
            if entry["id"] in records:
                raise ValueError(f"duplicate legacy evidence ID: {entry['id']}")
            records[entry["id"]] = entry
    return records


def convert_for_v2(
    record: dict[str, Any],
    *,
    approval: dict[str, Any] | None,
    source_registry: dict[str, Any],
) -> dict[str, Any]:
    """Build one schema-valid V2 BEHAVIORAL_CANON item, or refuse to export.

    Approval must be a separately maintained human-reviewed decision. A
    locator alone must never generate an invented narrative ordinal, and a
    behavioral classification must not smuggle factual/private claims.
    """
    if record.get("schema_version") != 1:
        raise EvidenceNotAdmitted("expected legacy evidence schema V1")
    if not isinstance(approval, dict):
        raise EvidenceNotAdmitted("explicit per-record reviewer approval required")
    if (
        approval.get("evidence_id") != record.get("id")
        or approval.get("approved_behavioral_guidance") is not True
        or approval.get("private_content_reviewed") is not True
        or not isinstance(approval.get("reviewer"), str)
        or not approval["reviewer"].strip()
    ):
        raise EvidenceNotAdmitted("behavioral authorization/private-content review missing")
    ordinal = approval.get("narrative_ordinal")
    if isinstance(ordinal, bool) or not isinstance(ordinal, int) or ordinal < 0:
        raise EvidenceNotAdmitted("confirmed nonnegative narrative ordinal required")
    if record.get("supports_claims"):
        raise EvidenceNotAdmitted("behavioral lane cannot carry factual claims")
    if record.get("provenance", {}).get("epistemic_status") != "UNVERIFIED":
        raise EvidenceNotAdmitted("epistemic promotion prohibited in legacy bridge")
    actors = record.get("actors")
    if not isinstance(actors, list) or not actors:
        raise EvidenceNotAdmitted("actor binding required")
    supports = deepcopy(record.get("provenance", {}).get("supports", []))
    primary = [s for s in supports if s.get("strength") == "primary"]
    if len(primary) != 1:
        raise EvidenceNotAdmitted("exactly one primary source is required")
    work = primary[0].get("work")
    if work != approval.get("narrative_work"):
        raise EvidenceNotAdmitted("reviewed narrative work does not match primary source")
    works = {w["id"]: w for w in source_registry.get("works", [])}
    for support in supports:
        bibliographic = works.get(support.get("work"))
        if bibliographic is None or not bibliographic.get("edition"):
            raise EvidenceNotAdmitted("source registry lacks a known edition")
        if support.get("edition") not in (None, bibliographic["edition"]):
            raise EvidenceNotAdmitted("source edition mismatch")
        if not isinstance(support.get("locator"), dict) or not support["locator"]:
            raise EvidenceNotAdmitted("source locator required")
        support["edition"] = bibliographic["edition"]
    evidence = {
        "id": record["id"],
        "schema_version": 2,
        "retrieval_role": "BEHAVIORAL_CANON",
        "text": record["text"],
        "story_time": deepcopy(record["story_time"]),
        "narrative_position": {"work": work, "ordinal": ordinal},
        "actors": deepcopy(actors),
        "topics": sorted(set(record.get("situation_tags", []))),
        "situation_tags": sorted(set(record.get("situation_tags", []))),
        "supports_claims": [],
        "provenance": deepcopy(record["provenance"]),
    }
    evidence["provenance"]["supports"] = supports
    try:
        _validator("evidence.schema.json").validate(evidence)
    except Exception as exc:
        raise EvidenceNotAdmitted(f"V2 validation rejected record: {exc}") from exc
    return evidence


def eligible_records(
    records: dict[str, dict[str, Any]],
    *,
    approvals: dict[str, dict[str, Any]],
    source_registry: dict[str, Any],
) -> list[dict[str, Any]]:
    """Only explicitly reviewed records may enter the V2 export; fail closed."""
    unknown = set(approvals) - set(records)
    if unknown:
        raise EvidenceNotAdmitted(f"unknown approved evidence IDs: {sorted(unknown)}")
    return [
        convert_for_v2(records[key], approval=approvals[key], source_registry=source_registry)
        for key in sorted(approvals)
    ]
