"""Cross-contract tests for the quarantined V0 character evidence.

None of these tests grants behavioral access to private text. An explicit
approval fixture is only used to exercise validation of the conversion port.
"""
from __future__ import annotations

from copy import deepcopy
import json

import pytest
import yaml

from tools.character_evidence_v2_bridge import (
    EvidenceNotAdmitted,
    ROOT,
    convert_for_v2,
    eligible_records,
    load_legacy_records,
)


def _registry():
    return yaml.safe_load((ROOT / "data/canon/sources/works.yaml").read_text(encoding="utf-8"))


def _approved(evidence_id: str) -> dict:
    return {
        "evidence_id": evidence_id,
        "approved_behavioral_guidance": True,
        "private_content_reviewed": True,
        "reviewer": "TEST_FIXTURE_NOT_HUMAN_VERIFICATION",
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
    # A legacy collection cannot accidentally masquerade as the RAG V2 format.
    collection = yaml.safe_load(
        (ROOT / "data/canon/evidence/characters/kiyotaka-ayanokoji.yaml").read_text(encoding="utf-8")
    )
    assert "records" in collection and "evidence" not in collection


def test_no_legacy_evidence_is_automatically_admitted():
    records = load_legacy_records()
    assert eligible_records(records, approvals={}, source_registry=_registry()) == []
    for record in records.values():
        with pytest.raises(EvidenceNotAdmitted, match="approval required"):
            convert_for_v2(record, approval=None, source_registry=_registry())


def test_explicit_synthetic_v2_projection_preserves_identity_and_status():
    records = load_legacy_records()
    item = records["ev.kiyotaka.v01.avoid-unnecessary-involvement"]
    result = convert_for_v2(item, approval=_approved(item["id"]), source_registry=_registry())
    assert result["id"] == item["id"]
    assert result["schema_version"] == 2
    assert result["retrieval_role"] == "BEHAVIORAL_CANON"
    assert result["supports_claims"] == []
    assert result["provenance"]["epistemic_status"] == "UNVERIFIED"
    assert result["narrative_position"] == {"work": "ln.y1.v01", "ordinal": 999}
    assert result["provenance"]["discovered_via"] == item["provenance"]["discovered_via"]
    assert result["provenance"]["supports"][0]["edition"] == "seven-seas-en"
    assert item["schema_version"] == 1 and "retrieval_role" not in item


@pytest.mark.parametrize(
    "change",
    [
        {"evidence_id": "ev.unrelated"},
        {"approved_behavioral_guidance": False},
        {"private_content_reviewed": False},
        {"reviewer": ""},
        {"narrative_ordinal": None},
        {"narrative_ordinal": -1},
        {"narrative_ordinal": True},
        {"narrative_work": "ln.y1.v07"},
    ],
)
def test_missing_or_ambiguous_authorization_fails_closed(change):
    records = load_legacy_records()
    entry = records["ev.kiyotaka.v01.avoid-unnecessary-involvement"]
    approval = _approved(entry["id"])
    approval.update(change)
    with pytest.raises(EvidenceNotAdmitted):
        convert_for_v2(entry, approval=approval, source_registry=_registry())


def test_factual_claim_relabeling_cannot_enter_behavioral_lane():
    entry = deepcopy(load_legacy_records()["ev.kiyotaka.v01.avoid-unnecessary-involvement"])
    entry["supports_claims"] = ["claim.fixture.secret"]
    with pytest.raises(EvidenceNotAdmitted, match="factual claims"):
        convert_for_v2(entry, approval=_approved(entry["id"]), source_registry=_registry())


def test_no_unregistered_edition_or_unknown_evidence_is_accepted():
    records = load_legacy_records()
    entry = records["ev.kiyotaka.v01.avoid-unnecessary-involvement"]
    with pytest.raises(EvidenceNotAdmitted, match="edition"):
        convert_for_v2(entry, approval=_approved(entry["id"]), source_registry={"works": []})
    with pytest.raises(EvidenceNotAdmitted, match="unknown approved"):
        eligible_records(records, approvals={"ev.never.exists": _approved("ev.never.exists")}, source_registry=_registry())


def test_v2_rag_corpus_loader_does_not_scan_legacy_character_collections():
    # The active V2 corpus only includes validated focal-v0 JSON. Character
    # packs are offline evidence for fidelity, not a source of factual grants.
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
