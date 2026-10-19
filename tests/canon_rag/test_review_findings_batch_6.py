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
import eval_runner  # noqa: E402
from eval_runner import evaluate_suite  # noqa: E402
from retrieval import assemble_context, retrieve  # noqa: E402


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


def test_factual_record_requires_every_supported_claim_to_be_authorized():
    suite, claims, registry, manifest = _inputs()
    evidence = _fixture_evidence(suite, "ev.fixture.post-divergence.fact")
    secret_claim = deepcopy(
        next(item for item in claims if item["id"] == "fixture.claim.future")
    )
    secret_claim["id"] = "fixture.claim.partial-secret"
    secret_claim["statement"] = "A second fact not authorized for the requesting actor."
    claims.append(secret_claim)
    evidence["supports_claims"].append(secret_claim["id"])
    evidence["text"] = "An allowed fact and a secret fact share this evidence record."
    records = compile_records([evidence], claims, registry)
    assert records[0]["compilation_errors"] == []

    request = _case(suite, "post-divergence-fact")
    request["divergence_time"] = None
    request["query"]["text"] = evidence["text"]
    output = retrieve(
        records, request, suite["time_order"], manifest["digest"], suite["seed_time"]
    )

    assert output["abstained"]
    assert output["excluded"][0]["reason"] == "CLAIM_NOT_ALLOWED"
    assert assemble_context(output)["KNOWN_CANON_FACTS"] == []


@pytest.mark.parametrize(
    ("evidence_id", "case_id"),
    [
        ("ev.fixture.post-divergence.fact", "post-divergence-fact"),
        ("ev.fixture.secret.other-actor", "normal-kiyotaka-behavior"),
    ],
)
def test_adaptation_primary_evidence_stays_out_with_ln_corroboration(
    evidence_id, case_id
):
    suite, claims, registry, manifest = _inputs()
    claims = deepcopy(claims)
    registry = deepcopy(registry)
    evidence = _fixture_evidence(suite, evidence_id)
    work = next(item for item in registry["works"] if item["id"] == "fixture.work")
    work.update(
        {
            "medium": "manga",
            "form": "adaptation-manga",
            "tier": 3,
        }
    )
    evidence["provenance"]["continuity"] = ["ln", "manga"]
    evidence["provenance"]["supports"].append(
        {
            "work": "ln.y1.v01",
            "edition": "seven-seas-en",
            "locator": {"chapter": 1, "scene": "fixture corroboration"},
            "quote_policy": "paraphrase-only",
            "strength": "corroborating",
        }
    )
    for claim_id in evidence.get("supports_claims", []):
        claim = next(item for item in claims if item["id"] == claim_id)
        claim["provenance"]["continuity"] = ["ln", "manga"]
        claim["provenance"]["supports"].append(
            {
                "work": "ln.y1.v01",
                "edition": "seven-seas-en",
                "locator": {"chapter": 1, "scene": "fixture corroboration"},
                "quote_policy": "paraphrase-only",
                "strength": "corroborating",
            }
        )

    records = compile_records([evidence], claims, registry)
    assert records[0]["compilation_errors"] == []
    assert records[0]["continuity"] == ["manga"]

    request = _case(suite, case_id)
    request["divergence_time"] = None
    request["knowledge_horizon"]["fixture.work"] = 100
    request["filters"] = {}
    request["query"]["text"] = evidence["text"]
    if evidence["retrieval_role"] == "BEHAVIORAL_CANON":
        request["character_id"] = evidence["actors"][0]

    output = retrieve(
        records, request, suite["time_order"], manifest["digest"], suite["seed_time"]
    )

    assert output["abstained"]
    assert output["excluded"][0]["reason"] == "FILTERED_OUT"
    assert "continuity=deny" in output["excluded"][0]["gate_trace"]


def test_golden_suite_exercises_factual_claim_authorization():
    suite = load_suite(REPO)
    case = _case(suite, "factual-secret-not-allowed")
    assert case["query"]["purpose"] == "KNOWN_FACTS"
    assert case["forbidden_evidence"] == {
        "ev.fixture.secret.fact": "CLAIM_NOT_ALLOWED"
    }

    report = evaluate_suite(
        load_corpus(REPO), load_claims(REPO), load_source_registry(REPO), suite
    )
    case_report = next(
        item for item in report["cases"] if item["id"] == "factual-secret-not-allowed"
    )
    assert case_report["result_evidence_ids"] == []
    assert {
        (item["evidence_id"], item["reason"])
        for item in case_report["exclusions"]
    } >= {("ev.fixture.secret.fact", "CLAIM_NOT_ALLOWED")}


def test_claim_authorization_leak_counts_as_wrong_character_leakage(monkeypatch):
    suite = load_suite(REPO)
    suite["cases"] = [_case(suite, "factual-secret-not-allowed")]

    def leaking_retrieve(records, request, order, manifest_digest, seed_time):
        return {
            "items": [
                {
                    "evidence_id": "ev.fixture.secret.fact",
                    "work": "fixture.work",
                    "locator": {"scene": "fixture only"},
                    "source_id": "src:fixture",
                    "provenance": {"epistemic_status": "VERIFIED"},
                    "source_digest": "fixture-source-digest",
                    "manifest_digest": manifest_digest,
                }
            ],
            "excluded": [],
            "abstained": False,
            "manifest_digest": manifest_digest,
        }

    monkeypatch.setattr(eval_runner, "retrieve", leaking_retrieve)
    report = evaluate_suite(
        load_corpus(REPO), load_claims(REPO), load_source_registry(REPO), suite
    )

    assert report["metrics"]["wrong_character_leakage"] == 1
    assert any(
        "leaked ev.fixture.secret.fact" in failure
        for failure in report["critical_failures"]
    )
