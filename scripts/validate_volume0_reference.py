#!/usr/bin/env python3
"""Validate the operator-authorized Volume 0 reference layer without external dependencies.

This check deliberately does NOT mark extracted records as VERIFIED and does NOT
compile the files into the engine: these are separate audited human gates.
"""
from __future__ import annotations

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "data/canon/volume0"
SOURCE_ID = "ln.y1.v00.legacyemtls.v1.0.1"
MAX_PAGE = 265

def load_all(folder: str):
    path = ROOT / folder
    assert path.is_dir(), f"missing directory: {path}"
    return [(p, json.loads(p.read_text(encoding="utf-8"))) for p in sorted(path.glob("*.json")) if p.name not in {"deep-profile-index.json", "faction-history.json"}]

def validate_pages(item, label):
    pages = item.get("source_pages", item.get("reference_pages", []))
    assert pages and isinstance(pages, list), f"{label}: no page provenance"
    assert all(isinstance(p, int) and 1 <= p <= MAX_PAGE for p in pages), f"{label}: invalid page references"

def validate_source(item, label):
    source = item.get("source")
    assert isinstance(source, dict) and source.get("work_id") == SOURCE_ID, f"{label}: wrong source id"
    assert source.get("kind") == "fan_translation", f"{label}: do not misrepresent source edition"
    assert source.get("project_source_authority") == "OPERATOR_ACCEPTED", f"{label}: unauthorized source"
    assert source.get("record_status") == "SOURCE_ACCEPTED_RECORD_REVIEW_REQUIRED", f"{label}: source approval is not individual verification"

def main():
    characters, organizations, cases = load_all("characters"), load_all("organizations"), load_all("behavioral_cases")
    assert len(characters) == 35, f"expected 35 actor references, got {len(characters)}"
    assert len(organizations) == 13, f"expected 13 organization references, got {len(organizations)}"
    assert len(cases) == 20, f"expected 20 behavioral cases, got {len(cases)}"
    actor_ids = set()
    case_ids = set()
    for path, obj in characters:
        validate_source(obj, str(path))
        validate_pages(obj, str(path))
        assert obj["character_id"] == "actor." + path.stem
        assert obj.get("scope", "").startswith("REFERENCE_ONLY"), str(path)
        assert obj["character_id"] not in actor_ids
        actor_ids.add(obj["character_id"])
    for path, obj in organizations:
        validate_source(obj, str(path))
        validate_pages(obj, str(path))
        assert obj["organization_id"] == "org." + path.stem, str(path)
        assert obj.get("operational_rights") == "NOT_INFERRED", str(path)
    for path, obj in cases:
        validate_source(obj, str(path))
        validate_pages(obj, str(path))
        assert obj["case_id"] == "v0.case." + path.stem, str(path)
        assert obj["actor_id"] in actor_ids, str(path)
        assert obj.get("observed_outcome") and obj.get("limits"), str(path)
        assert obj.get("counterfactual_required") is True, str(path)
        assert obj["case_id"] not in case_ids
        case_ids.add(obj["case_id"])
    for path, obj in characters:
        for case_ref in obj["behavioral_case_refs"]:
            assert case_ref in case_ids, f"{path}: missing behavioral reference {case_ref}"
    for file in (ROOT / "institutions").glob("*.json"):
        obj = json.loads(file.read_text(encoding="utf-8"))
        assert obj["schema_version"] in ("volume0-institution-evidence-v1", "institution-pack-template-v1")
        if file.name == "white-room.json":
            validate_source(obj, str(file))
            assert len(obj["cohort_policy"]) == 8, "cohort chronology truncated"
    print(f"Volume 0 reference validation passed: {len(characters)} characters, "
          f"{len(organizations)} organizations, {len(cases)} behavioral cases.")
    print("This is structural validation, NOT a claim-level verification or a release approval.")

if __name__ == "__main__":
    main()
