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


@pytest.mark.parametrize(
    ("evidence_id", "case_id", "adaptation"),
    [
        ("ev.fixture.post-divergence.fact", "post-divergence-fact", "manga"),
        ("ev.fixture.secret.other-actor", "normal-kiyotaka-behavior", "anime"),
    ],
)
def test_default_retrieval_requires_actual_ln_backing_for_dual_labeled_records(
    evidence_id, case_id, adaptation
):
    suite, claims, registry, manifest = _inputs()
    claims = deepcopy(claims)
    registry = deepcopy(registry)
    evidence = _fixture_evidence(suite, evidence_id)
    work = next(item for item in registry["works"] if item["id"] == "fixture.work")
    work.update(
        {
            "medium": adaptation,
            "form": f"adaptation-{adaptation}",
            "tier": 3,
        }
    )
    evidence["provenance"]["continuity"] = ["ln", adaptation]
    for claim_id in evidence.get("supports_claims", []):
        claim = next(item for item in claims if item["id"] == claim_id)
        claim["provenance"]["continuity"] = ["ln", adaptation]

    records = compile_records([evidence], claims, registry)

    assert records[0]["compilation_errors"] == []
    assert records[0]["continuity"] == [adaptation]
    assert all(
        claim["continuity"] == [adaptation]
        for claim in records[0]["resolved_claims"]
    )

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


@pytest.mark.parametrize(
    "invalid_order",
    [
        {"Y1_M02": "20", "Y1_M03": "100"},
        {"Y1_M02": 20, "Y1_M03": "100"},
    ],
)
def test_retrieval_rejects_non_numeric_time_order_before_gating(invalid_order):
    suite, claims, registry, manifest = _inputs()
    evidence = _fixture_evidence(suite, "ev.fixture.post-divergence.fact")
    records = compile_records([evidence], claims, registry)
    request = _case(suite, "post-divergence-fact")

    with pytest.raises(ValueError, match="time_order"):
        retrieve(
            records,
            request,
            invalid_order,
            manifest["digest"],
            suite["seed_time"],
        )


@pytest.mark.parametrize("invalid_value", [True, float("nan"), float("inf")])
def test_retrieval_rejects_non_finite_or_boolean_time_ordinals(invalid_value):
    suite, claims, registry, manifest = _inputs()
    evidence = _fixture_evidence(suite, "ev.fixture.post-divergence.fact")
    records = compile_records([evidence], claims, registry)
    request = _case(suite, "post-divergence-fact")
    invalid_order = deepcopy(suite["time_order"])
    invalid_order["Y1_M02"] = invalid_value

    with pytest.raises(ValueError, match="time_order"):
        retrieve(
            records,
            request,
            invalid_order,
            manifest["digest"],
            suite["seed_time"],
        )


@pytest.mark.parametrize("verified_by", ["", "   "])
@pytest.mark.parametrize("target", ["evidence", "claim"])
def test_verified_records_require_a_non_blank_verifier_identity(verified_by, target):
    suite, claims, registry, _ = _inputs()
    claims = deepcopy(claims)
    evidence = _fixture_evidence(suite, "ev.fixture.post-divergence.fact")
    if target == "evidence":
        evidence["provenance"]["verified_by"] = verified_by
    else:
        claim = next(item for item in claims if item["id"] == "fixture.claim.future")
        claim["provenance"]["verified_by"] = verified_by

    records = compile_records([evidence], claims, registry)

    assert f"blank_verified_by:{target}" in records[0]["compilation_errors"]
    assert any(
        error.endswith(":pattern") for error in records[0]["compilation_errors"]
    )
