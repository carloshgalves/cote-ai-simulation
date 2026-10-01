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


def _future_evidence(suite):
    return deepcopy(
        next(
            item
            for item in suite["fixture_records"]
            if item["id"] == "ev.fixture.post-divergence.fact"
        )
    )


def test_post_divergence_claim_is_excluded_even_when_evidence_predates_divergence():
    suite, claims, registry, manifest = _inputs()
    evidence = _future_evidence(suite)
    evidence["story_time"] = {"mode": "EXACT", "exact": {"anchor": "Y1_M01"}}
    records = compile_records([evidence], claims, registry)
    request = _case(suite, "post-divergence-fact")

    output = retrieve(
        records, request, suite["time_order"], manifest["digest"], suite["seed_time"]
    )

    assert output["abstained"]
    assert output["excluded"] == [
        {
            "evidence_id": "ev.fixture.post-divergence.fact",
            "reason": "POST_DIVERGENCE_FACT",
            "gate_trace": [
                "prefilter=allow",
                "effective=allow",
                "actor=allow",
                "spoiler=allow",
                "divergence=deny",
            ],
        }
    ]


def test_belief_claim_cannot_authorize_the_known_canon_facts_lane():
    suite, claims, registry, manifest = _inputs()
    belief_claims = deepcopy(claims)
    claim = next(item for item in belief_claims if item["id"] == "fixture.claim.future")
    claim.update(
        {
            "claim_kind": "BELIEF",
            "holder": "actor.kiyotaka-ayanokoji",
            "truth_value": False,
            "derivation_kind": "assumption",
        }
    )
    records = compile_records([_future_evidence(suite)], belief_claims, registry)
    assert records[0]["compilation_errors"] == []
    request = _case(suite, "post-divergence-fact")
    request["divergence_time"] = None

    output = retrieve(
        records, request, suite["time_order"], manifest["digest"], suite["seed_time"]
    )

    assert output["abstained"]
    assert output["excluded"][0]["reason"] == "NON_FACTUAL_STATUS"
    assert assemble_context(output)["KNOWN_CANON_FACTS"] == []


@pytest.mark.parametrize("invalid_opt_in", ["false", 1, 0, None])
def test_unverified_behavioral_opt_in_requires_a_real_boolean(invalid_opt_in):
    suite, claims, registry, manifest = _inputs()
    records = compile_records(suite["fixture_records"], claims, registry)
    request = _case(suite, "normal-kiyotaka-behavior")
    request["allow_unverified_behavioral"] = invalid_opt_in

    with pytest.raises(ValueError, match="allow_unverified_behavioral"):
        retrieve(records, request, suite["time_order"], manifest["digest"], suite["seed_time"])


@pytest.mark.parametrize(
    ("field", "invalid_value"),
    [
        ("knowledge_horizon", {"ln.y1.v01": "500"}),
        ("knowledge_horizon", {"ln.y1.v01": True}),
        ("allowed_claim_ids", "fixture.claim.future"),
        ("retrieval_roles", "KNOWLEDGE_EVIDENCE"),
        ("topics", ["private-points", 7]),
        ("evidence_ids", {"ev.fixture.post-divergence.fact": True}),
        ("character_id", 7),
        ("simulation_time", []),
        ("divergence_time", []),
        ("query", []),
        ("query.text", 7),
        ("query.purpose", []),
    ],
)
def test_safety_relevant_request_collections_reject_wrong_types(field, invalid_value):
    suite, claims, registry, manifest = _inputs()
    records = compile_records(suite["fixture_records"], claims, registry)
    request = _case(suite, "post-divergence-fact")
    if field == "knowledge_horizon":
        request[field] = invalid_value
    elif field == "allowed_claim_ids":
        request["knowledge_scope"][field] = invalid_value
    elif field in {"retrieval_roles", "topics", "evidence_ids"}:
        request["filters"][field] = invalid_value
    elif field.startswith("query."):
        request["query"][field.removeprefix("query.")] = invalid_value
    else:
        request[field] = invalid_value

    with pytest.raises(ValueError, match=field):
        retrieve(records, request, suite["time_order"], manifest["digest"], suite["seed_time"])
