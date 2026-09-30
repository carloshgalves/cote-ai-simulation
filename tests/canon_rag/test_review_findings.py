from copy import deepcopy
import json
from pathlib import Path
import sys

import jsonschema
import pytest
from referencing import Registry, Resource


HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
sys.path.insert(0, str(HERE))

from common import (  # noqa: E402
    compile_records,
    corpus_manifest,
    lexical_score,
    load_claims,
    load_corpus,
    load_source_registry,
    load_suite,
    merge_source_registries,
)
from eval_runner import evaluate_suite  # noqa: E402
from retrieval import retrieve  # noqa: E402


def _inputs():
    suite = load_suite(REPO)
    evidence = load_corpus(REPO)
    claims = load_claims(REPO)
    registry = load_source_registry(REPO)
    registry = merge_source_registries(registry, suite["fixture_source_registry"])
    claims = claims + suite["fixture_claims"]
    records = compile_records(evidence + suite["fixture_records"], claims, registry)
    manifest = corpus_manifest(evidence, load_claims(REPO), load_source_registry(REPO))
    return suite, evidence, claims, registry, records, manifest


def _case(suite, case_id):
    return deepcopy(next(case for case in suite["cases"] if case["id"] == case_id))


def test_role_drives_gates_and_cross_role_requests_are_filtered():
    suite, _, _, _, records, manifest = _inputs()

    behavioral_as_fact = _case(suite, "normal-kiyotaka-behavior")
    behavioral_as_fact["query"]["purpose"] = "KNOWN_FACTS"
    behavioral_as_fact["knowledge_scope"]["allowed_claim_ids"] = ["fixture.claim.future"]
    behavioral_as_fact["filters"] = {}
    output = retrieve(
        records, behavioral_as_fact, suite["time_order"], manifest["digest"], suite["seed_time"]
    )
    excluded = {item["evidence_id"]: item["reason"] for item in output["excluded"]}
    assert excluded["ev.kiyotaka.v01.reluctant-social-help"] == "FILTERED_OUT"

    fact_as_behavior = _case(suite, "post-divergence-fact")
    fact_as_behavior["query"]["purpose"] = "BEHAVIORAL_GUIDANCE"
    fact_as_behavior["character_id"] = "actor.kiyotaka-ayanokoji"
    fact_as_behavior["filters"] = {}
    output = retrieve(records, fact_as_behavior, suite["time_order"], manifest["digest"], suite["seed_time"])
    excluded = {item["evidence_id"]: item["reason"] for item in output["excluded"]}
    assert excluded["fixture.ev.post-divergence.fact"] == "FILTERED_OUT"


def test_factual_lane_requires_resolved_verified_temporally_valid_claim():
    suite, _, claims, registry, records, manifest = _inputs()
    request = _case(suite, "post-divergence-fact")
    request["divergence_time"] = None

    unverified_claims = deepcopy(claims)
    claim = next(item for item in unverified_claims if item["id"] == "fixture.claim.future")
    claim["provenance"]["epistemic_status"] = "UNVERIFIED"
    compiled = compile_records(suite["fixture_records"], unverified_claims, registry)
    output = retrieve(compiled, request, suite["time_order"], manifest["digest"], suite["seed_time"])
    assert {item["evidence_id"]: item["reason"] for item in output["excluded"]}[
        "fixture.ev.post-divergence.fact"
    ] == "UNVERIFIED_FACT"

    conflicted_claims = deepcopy(claims)
    claim = next(item for item in conflicted_claims if item["id"] == "fixture.claim.future")
    claim["provenance"]["conflicts"] = ["fixture.conflict.claim-only"]
    compiled = compile_records(suite["fixture_records"], conflicted_claims, registry)
    output = retrieve(compiled, request, suite["time_order"], manifest["digest"], suite["seed_time"])
    assert {item["evidence_id"]: item["reason"] for item in output["excluded"]}[
        "fixture.ev.post-divergence.fact"
    ] == "OPEN_CONFLICT"

    unresolved = deepcopy(suite["fixture_records"])
    future = next(item for item in unresolved if item["id"] == "fixture.ev.post-divergence.fact")
    future["supports_claims"] = ["fixture.claim.missing"]
    request["knowledge_scope"]["allowed_claim_ids"] = ["fixture.claim.missing"]
    compiled = compile_records(unresolved, claims, registry)
    output = retrieve(compiled, request, suite["time_order"], manifest["digest"], suite["seed_time"])
    assert {item["evidence_id"]: item["reason"] for item in output["excluded"]}[
        "fixture.ev.post-divergence.fact"
    ] == "MISSING_GATE_METADATA"

    future_claims = deepcopy(claims)
    claim = next(item for item in future_claims if item["id"] == "fixture.claim.future")
    claim["temporal"]["effective_from"] = {"mode": "EXACT", "exact": {"anchor": "Y1_M04"}}
    request["knowledge_scope"]["allowed_claim_ids"] = ["fixture.claim.future"]
    compiled = compile_records(suite["fixture_records"], future_claims, registry)
    output = retrieve(compiled, request, suite["time_order"], manifest["digest"], suite["seed_time"])
    assert {item["evidence_id"]: item["reason"] for item in output["excluded"]}[
        "fixture.ev.post-divergence.fact"
    ] == "NOT_EFFECTIVE"


def test_canon_seed_is_rejected_after_configured_seed_instant():
    suite, _, _, _, records, manifest = _inputs()
    request = _case(suite, "unverified-fact-abstains")
    request["simulation_time"] = "Y1_M03"
    with pytest.raises(ValueError, match="CANON_SEED"):
        retrieve(records, request, suite["time_order"], manifest["digest"], suite["seed_time"])


def test_topic_and_evidence_filters_apply_before_gates_with_empty_meaning_unrestricted():
    suite, _, _, _, records, manifest = _inputs()
    request = _case(suite, "normal-kiyotaka-behavior")
    request["filters"]["topics"] = ["not-this-topic"]
    request["filters"]["evidence_ids"] = ["fixture.ev.secret.other-actor"]
    output = retrieve(records, request, suite["time_order"], manifest["digest"], suite["seed_time"])
    assert output["abstained"]
    assert {item["evidence_id"]: item["reason"] for item in output["excluded"]}[
        "ev.kiyotaka.v01.reluctant-social-help"
    ] == "FILTERED_OUT"

    request["filters"]["topics"] = []
    request["filters"]["evidence_ids"] = []
    output = retrieve(records, request, suite["time_order"], manifest["digest"], suite["seed_time"])
    assert [item["evidence_id"] for item in output["items"]] == [
        "ev.kiyotaka.v01.reluctant-social-help"
    ]


def test_manifest_and_compiled_records_cover_all_authoritative_inputs():
    evidence = load_corpus(REPO)
    claims = load_claims(REPO)
    registry = load_source_registry(REPO)
    manifest = corpus_manifest(evidence, claims, registry)

    changed_evidence = deepcopy(evidence)
    changed_evidence[0]["text"] += " changed"
    assert corpus_manifest(changed_evidence, claims, registry)["digest"] != manifest["digest"]

    changed_claims = deepcopy(claims)
    claim = next(item for item in changed_claims if item["id"] == "rule.economy.initial-deposit")
    claim["provenance"]["epistemic_status"] = "VERIFIED"
    assert corpus_manifest(evidence, changed_claims, registry)["digest"] != manifest["digest"]
    before = compile_records(evidence, claims, registry)
    after = compile_records(evidence, changed_claims, registry)
    before_record = next(item for item in before if item["evidence_id"] == "ev.institution.v01.initial-deposit")
    after_record = next(item for item in after if item["evidence_id"] == "ev.institution.v01.initial-deposit")
    assert before_record != after_record

    changed_registry = deepcopy(registry)
    work = next(item for item in changed_registry["works"] if item["id"] == "ln.y1.v01")
    work["title"] += " changed"
    assert corpus_manifest(evidence, claims, changed_registry)["digest"] != manifest["digest"]
    changed_source_records = compile_records(evidence, claims, changed_registry)
    changed_source_record = next(
        item for item in changed_source_records if item["evidence_id"] == "ev.institution.v01.initial-deposit"
    )
    assert changed_source_record != before_record
    assert manifest["works"]
    assert all(item["id"] and item["version"] and item["source_digest"] for item in manifest["works"])


def test_eval_report_identifies_inputs_backend_requests_results_and_exclusions():
    suite, evidence, _, _, _, _ = _inputs()
    report = evaluate_suite(evidence, load_claims(REPO), load_source_registry(REPO), suite)
    assert report["suite"]["version"] == suite["suite_version"]
    assert len(report["suite"]["digest"]) == 64
    assert report["backend"] == {"id": "reference-lexical", "version": 1}
    assert len(report["run_digest"]) == 64
    assert all("request" in case and "result_evidence_ids" in case and "exclusions" in case for case in report["cases"])

    changed_fixture = deepcopy(suite)
    changed_fixture["fixture_records"][0]["text"] += " changed"
    changed_fixture_report = evaluate_suite(
        evidence, load_claims(REPO), load_source_registry(REPO), changed_fixture
    )
    assert changed_fixture_report["run_digest"] != report["run_digest"]

    changed_golden = deepcopy(suite)
    changed_golden["cases"][0]["query"]["text"] += " changed"
    changed_golden_report = evaluate_suite(
        evidence, load_claims(REPO), load_source_registry(REPO), changed_golden
    )
    assert changed_golden_report["run_digest"] != report["run_digest"]


def test_evidence_schema_requires_exactly_one_primary_support():
    schema = json.loads((REPO / "data/canon/schema/evidence.schema.json").read_text(encoding="utf-8"))
    common = json.loads((REPO / "data/canon/schema/common.schema.json").read_text(encoding="utf-8"))
    registry = Registry().with_resource(common["$id"], Resource.from_contents(common))
    validator = jsonschema.Draft202012Validator(schema, registry=registry)
    evidence = deepcopy(load_corpus(REPO)[0])
    assert not list(validator.iter_errors(evidence))

    evidence["provenance"]["supports"].append(deepcopy(evidence["provenance"]["supports"][0]))
    errors = list(validator.iter_errors(evidence))
    assert errors


def test_missing_or_inconsistent_primary_source_fails_closed():
    suite, _, claims, registry, _, manifest = _inputs()
    request = _case(suite, "post-divergence-fact")
    request["divergence_time"] = None
    source = next(item for item in suite["fixture_records"] if item["id"] == "fixture.ev.post-divergence.fact")

    for mutation in ("locator", "edition", "work_mismatch"):
        evidence = deepcopy(source)
        primary = evidence["provenance"]["supports"][0]
        if mutation == "work_mismatch":
            primary["work"] = "ln.y1.v01"
        else:
            primary.pop(mutation)
        records = compile_records([evidence], claims, registry)
        output = retrieve(records, request, suite["time_order"], manifest["digest"], suite["seed_time"])
        assert output["abstained"]
        assert output["excluded"][0]["reason"] == "MISSING_GATE_METADATA"


@pytest.mark.parametrize("top_k", [0, -1, True, 1.5, "1"])
def test_invalid_top_k_is_rejected(top_k):
    suite, _, _, _, records, manifest = _inputs()
    request = _case(suite, "normal-kiyotaka-behavior")
    request["top_k"] = top_k
    with pytest.raises(ValueError, match="top_k"):
        retrieve(records, request, suite["time_order"], manifest["digest"], suite["seed_time"])


def test_lexical_oracle_normalizes_unicode_and_handles_non_latin_words():
    evidence = {"text": "caf\u00e9 \u5354\u529b", "topics": [], "situation_tags": []}
    assert lexical_score("cafe\u0301", evidence) == 1.0
    assert lexical_score("\u5354\u529b", evidence) == 1.0
