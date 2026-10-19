"""Negative authorization and schema tests for quarantined Character Evidence V0.

Synthetic approvals are exercised exclusively through the INTERNAL projector.
No synthetic fixture is loaded into the pinned public review authority.
"""
from __future__ import annotations

from copy import deepcopy
import hashlib
import json

import pytest
import yaml

import tools.character_evidence_v2_bridge as bridge
from tools.character_evidence_v2_bridge import (
    EvidenceNotAdmitted,
    ROOT,
    _project_reviewed_record,
    convert_for_v2,
    eligible_records,
    evidence_digest,
    load_legacy_records,
)


def _registry():
    return yaml.safe_load((ROOT / "data/canon/sources/works.yaml").read_text(encoding="utf-8"))


def _synthetic_approval(record: dict) -> dict:
    return {
        "evidence_id": record["id"],
        "approved_behavioral_guidance": True,
        "private_content_reviewed": True,
        "reviewer": "TEST_FIXTURE_NOT_HUMAN_VERIFICATION",
        "review_reference": "https://github.com/carloshgalves/cote-ai-simulation/pull/18",
        "reviewed_record_sha256": evidence_digest(record),
        "narrative_work": "ln.y1.v01",
        "narrative_ordinal": 999,
    }


def test_legacy_collections_are_namespaced_and_not_rag_v2():
    records = load_legacy_records()
    assert len(records) == 32
    assert set(records) == {r["id"] for r in records.values()}
    rag_schema = json.loads((ROOT / "data/canon/schema/evidence.schema.json").read_text(encoding="utf-8"))
    collection_schema = json.loads(
        (ROOT / "data/canon/schema/evidence_collection.schema.json").read_text(encoding="utf-8")
    )
    assert rag_schema["properties"]["schema_version"]["const"] == 2
    assert "evidence" in collection_schema["required"]
    assert "records" not in collection_schema["properties"]
    collection = yaml.safe_load(
        (ROOT / "data/canon/evidence/characters/kiyotaka-ayanokoji.yaml").read_text(encoding="utf-8")
    )
    assert "records" in collection and "evidence" not in collection


def test_zero_approved_records_and_no_caller_supplied_reviewers():
    records = load_legacy_records()
    assert bridge._trusted_review_registry() == {}
    assert eligible_records(records, source_registry=_registry()) == []
    for record in records.values():
        with pytest.raises(EvidenceNotAdmitted, match="approval required"):
            convert_for_v2(record, source_registry=_registry())
    record = next(iter(records.values()))
    with pytest.raises(TypeError):
        convert_for_v2(
            record,
            approval=_synthetic_approval(record),
            source_registry=_registry(),
        )
    with pytest.raises(TypeError):
        eligible_records(
            records,
            approvals={record["id"]: _synthetic_approval(record)},
            source_registry=_registry(),
        )


def test_synthetic_internal_projection_is_schema_valid_but_not_authority():
    records = load_legacy_records()
    item = records["ev.kiyotaka.v01.avoid-unnecessary-involvement"]
    result = _project_reviewed_record(
        item, approval=_synthetic_approval(item), source_registry=_registry()
    )
    assert result["id"] == item["id"]
    assert result["schema_version"] == 2
    assert result["retrieval_role"] == "BEHAVIORAL_CANON"
    assert result["supports_claims"] == []
    assert result["provenance"]["epistemic_status"] == "UNVERIFIED"
    assert result["narrative_position"] == {"work": "ln.y1.v01", "ordinal": 999}
    assert result["provenance"]["discovered_via"] == item["provenance"]["discovered_via"]
    assert result["provenance"]["supports"][0]["edition"] == "seven-seas-en"
    # Even the test-approved item remains forbidden by the public adapter.
    with pytest.raises(EvidenceNotAdmitted):
        convert_for_v2(item, source_registry=_registry())


@pytest.mark.parametrize(
    "change",
    [
        {"evidence_id": "ev.unrelated"},
        {"approved_behavioral_guidance": False},
        {"private_content_reviewed": False},
        {"reviewer": ""},
        {"review_reference": "https://example.invalid/forged"},
        {"reviewed_record_sha256": "0" * 64},
        {"narrative_ordinal": None},
        {"narrative_ordinal": -1},
        {"narrative_ordinal": True},
        {"narrative_work": "ln.y1.v07"},
    ],
)
def test_internal_projection_rejects_invalid_approval_fields(change):
    entry = load_legacy_records()["ev.kiyotaka.v01.avoid-unnecessary-involvement"]
    approval = _synthetic_approval(entry)
    approval.update(change)
    with pytest.raises(EvidenceNotAdmitted):
        _project_reviewed_record(entry, approval=approval, source_registry=_registry())


@pytest.mark.parametrize("change", [
    {"text": "Changed content with the same evidence ID"},
    {"actors": ["actor.kikyo-kushida"]},
    {"situation_tags": ["fabricated", "unreviewed"]},
    {"story_time": {"mode": "UNKNOWN", "basis": "unreviewed"}},
    {"provenance": {"epistemic_status": "UNVERIFIED", "supports": []}},
])
def test_authorization_of_original_cannot_be_reused_after_evidence_mutation(change):
    original = load_legacy_records()["ev.kiyotaka.v01.avoid-unnecessary-involvement"]
    approval = _synthetic_approval(original)
    modified = deepcopy(original)
    modified.update(change)
    assert evidence_digest(modified) != approval["reviewed_record_sha256"]
    with pytest.raises(EvidenceNotAdmitted, match="changed since human review"):
        _project_reviewed_record(modified, approval=approval, source_registry=_registry())


def test_factual_claim_relabeling_cannot_enter_behavioral_lane():
    entry = deepcopy(load_legacy_records()["ev.kiyotaka.v01.avoid-unnecessary-involvement"])
    entry["supports_claims"] = ["claim.fixture.secret"]
    with pytest.raises(EvidenceNotAdmitted, match="factual claims"):
        _project_reviewed_record(entry, approval=_synthetic_approval(entry), source_registry=_registry())


def test_no_unregistered_edition_can_be_accepted():
    entry = load_legacy_records()["ev.kiyotaka.v01.avoid-unnecessary-involvement"]
    with pytest.raises(EvidenceNotAdmitted, match="edition"):
        _project_reviewed_record(entry, approval=_synthetic_approval(entry), source_registry={"works": []})


def test_registry_path_cannot_be_replaced_by_unsigned_caller_manifest(tmp_path, monkeypatch):
    record = load_legacy_records()["ev.kiyotaka.v01.avoid-unnecessary-involvement"]
    forged = {
        "schema_version": 1,
        "reviewed_records": {record["id"]: _synthetic_approval(record)},
    }
    path = tmp_path / "forged-reviews.json"
    path.write_text(json.dumps(forged), encoding="utf-8")
    monkeypatch.setattr(bridge, "REVIEW_MANIFEST_PATH", path)
    with pytest.raises(EvidenceNotAdmitted, match="digest mismatch"):
        eligible_records({record["id"]: record}, source_registry=_registry())
    with pytest.raises(EvidenceNotAdmitted, match="digest mismatch"):
        convert_for_v2(record, source_registry=_registry())


def test_v2_rag_corpus_loader_does_not_scan_legacy_character_collections():
    import sys

    rag_tests = ROOT / "tests/canon_rag"
    sys.path.insert(0, str(rag_tests))
    try:
        from common import load_corpus

        corpus = load_corpus(ROOT)
        ids = {entry["id"] for entry in corpus}
        assert ids.isdisjoint(load_legacy_records())
    finally:
        sys.path.remove(str(rag_tests))
