from copy import deepcopy
from pathlib import Path
import sys

import pytest


HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
sys.path.insert(0, str(HERE))

from common import (  # noqa: E402
    compile_records,
    corpus_manifest,
    load_claims,
    load_corpus,
    load_source_registry,
    load_suite,
    merge_source_registries,
)
from retrieval import retrieve  # noqa: E402


def _inputs():
    suite = load_suite(REPO)
    claims = load_claims(REPO) + suite["fixture_claims"]
    registry = merge_source_registries(
        load_source_registry(REPO), suite["fixture_source_registry"]
    )
    manifest = corpus_manifest(
        load_corpus(REPO), load_claims(REPO), load_source_registry(REPO)
    )
    return suite, claims, registry, manifest


def _case(suite, case_id):
    return deepcopy(next(case for case in suite["cases"] if case["id"] == case_id))


def _fixture_evidence(suite, evidence_id):
    return deepcopy(
        next(item for item in suite["fixture_records"] if item["id"] == evidence_id)
    )


def _set_fixture_continuity(claims, registry, evidence, continuity):
    work = next(item for item in registry["works"] if item["id"] == "fixture.work")
    work.update(
        {
            "medium": continuity,
            "form": f"adaptation-{continuity}",
            "tier": 3,
        }
    )
    evidence["provenance"]["continuity"] = [continuity]
    for claim_id in evidence.get("supports_claims", []):
        claim = next(item for item in claims if item["id"] == claim_id)
        claim["provenance"]["continuity"] = [continuity]


@pytest.mark.parametrize(
    ("evidence_id", "case_id", "continuity"),
    [
        ("ev.fixture.post-divergence.fact", "post-divergence-fact", "manga"),
        ("ev.fixture.secret.other-actor", "normal-kiyotaka-behavior", "anime"),
    ],
)
def test_default_retrieval_excludes_adaptation_only_continuities(
    evidence_id, case_id, continuity
):
    suite, claims, registry, manifest = _inputs()
    claims = deepcopy(claims)
    registry = deepcopy(registry)
    evidence = _fixture_evidence(suite, evidence_id)
    _set_fixture_continuity(claims, registry, evidence, continuity)
    records = compile_records([evidence], claims, registry)
    assert records[0]["compilation_errors"] == []
    assert records[0]["continuity"] == [continuity]

    request = _case(suite, case_id)
    request["divergence_time"] = None
    request["knowledge_horizon"]["fixture.work"] = 100
    request["filters"] = {}
    if evidence["retrieval_role"] == "BEHAVIORAL_CANON":
        request["character_id"] = evidence["actors"][0]
        request["query"]["text"] = evidence["text"]

    output = retrieve(
        records, request, suite["time_order"], manifest["digest"], suite["seed_time"]
    )

    assert output["abstained"]
    assert output["excluded"][0]["reason"] == "FILTERED_OUT"
    assert "continuity=deny" in output["excluded"][0]["gate_trace"]


def test_compiler_rejects_incompatible_evidence_and_claim_continuities():
    suite, claims, registry, _ = _inputs()
    registry = deepcopy(registry)
    registry["works"].append(
        {
            "id": "fixture.manga",
            "title": "Fixture manga",
            "medium": "manga",
            "form": "adaptation-manga",
            "tier": 3,
            "edition": "fixture-manga-v1",
        }
    )
    evidence = _fixture_evidence(suite, "ev.fixture.post-divergence.fact")
    evidence["narrative_position"]["work"] = "fixture.manga"
    evidence["provenance"]["continuity"] = ["manga"]
    evidence["provenance"]["supports"][0].update(
        {"work": "fixture.manga", "edition": "fixture-manga-v1"}
    )

    records = compile_records([evidence], claims, registry)

    assert "claim_continuity_mismatch:fixture.claim.future" in records[0][
        "compilation_errors"
    ]


@pytest.mark.parametrize("field", ["verified_at", "retrieved_at"])
def test_schema_formats_fail_closed_during_compilation(field):
    suite, claims, registry, _ = _inputs()
    claims = deepcopy(claims)
    evidence = _fixture_evidence(suite, "ev.fixture.post-divergence.fact")
    if field == "verified_at":
        claim = next(item for item in claims if item["id"] == "fixture.claim.future")
        claim["provenance"]["verified_at"] = "not-a-date"
    else:
        evidence["provenance"]["discovered_via"] = [
            {"tier": 5, "ref": "fixture.discovery", "retrieved_at": "not-a-date"}
        ]

    records = compile_records([evidence], claims, registry)

    assert any(error.endswith(":format") for error in records[0]["compilation_errors"])


def test_manifest_lists_every_work_referenced_by_evidence_and_claim_supports():
    evidence = load_corpus(REPO)
    claims = load_claims(REPO)
    registry = load_source_registry(REPO)

    manifest = corpus_manifest(evidence, claims, registry)
    work_ids = {work["id"] for work in manifest["works"]}

    assert {
        "ln.jp.y1.v00",
        "ln.y1.v07",
        "ln.y1.v10",
        "para.school-database",
    } <= work_ids
    assert all(work["version"] and work["source_digest"] for work in manifest["works"])


def test_manifest_rejects_duplicate_claim_ids_like_the_compiler():
    evidence = load_corpus(REPO)
    claims = load_claims(REPO)
    registry = load_source_registry(REPO)
    duplicate = claims + [deepcopy(claims[0])]

    with pytest.raises(ValueError, match="duplicate claim id"):
        corpus_manifest(evidence, duplicate, registry)
