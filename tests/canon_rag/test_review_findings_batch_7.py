from copy import deepcopy
from pathlib import Path
import sys

import pytest
from jsonschema import ValidationError

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
sys.path.insert(0, str(HERE))

from common import (  # noqa: E402
    COMPILER_VERSION,
    _schema_errors,
    compile_records,
    corpus_manifest,
    load_claims,
    load_conflicts,
    load_corpus,
    load_source_registry,
    load_suite,
    merge_source_registries,
)
from eval_runner import evaluate_suite  # noqa: E402
from retrieval import retrieve  # noqa: E402


def _inputs():
    suite = load_suite(REPO)
    claims = load_claims(REPO) + suite["fixture_claims"]
    registry = merge_source_registries(
        load_source_registry(REPO), suite["fixture_source_registry"]
    )
    evidence = next(
        item for item in suite["fixture_records"]
        if item["id"] == "ev.fixture.post-divergence.fact"
    )
    return suite, deepcopy(evidence), claims, registry


def test_behavioral_record_cannot_transport_a_secret_fact_claim():
    suite, evidence, claims, registry = _inputs()
    evidence["retrieval_role"] = "BEHAVIORAL_CANON"
    evidence["actors"] = ["actor.kiyotaka"]
    evidence["text"] = "A secret fact disguised as a behavioral example."
    records = compile_records([evidence], claims, registry)

    assert any(error.endswith(":maxItems") for error in records[0]["compilation_errors"])
    request = next(case for case in suite["cases"] if case["id"] == "post-divergence-fact")
    request = deepcopy(request)
    request["query"]["purpose"] = "BEHAVIORAL_GUIDANCE"
    request["query"]["text"] = evidence["text"]
    request["character_id"] = "actor.kiyotaka"
    request["filters"] = {}
    result = retrieve(records, request, suite["time_order"], "manifest", suite["seed_time"])
    assert result["abstained"]
    assert result["excluded"][0]["reason"] == "MISSING_GATE_METADATA"


@pytest.mark.parametrize(
    "point",
    [
        {"anchor": "Y1_M01", "date": "Y1-01-31"},
        {"anchor": "Y1_M01", "years_before_y1": 1},
        {"date": "Y1-01-31", "years_before_y1": 1},
    ],
)
def test_timepoint_rejects_multiple_causal_coordinates(point):
    value = {
        "id": "ev.fixture.timepoint",
        "schema_version": 2,
        "retrieval_role": "BEHAVIORAL_CANON",
        "text": "Example.",
        "story_time": {"mode": "EXACT", "exact": point},
        "narrative_position": {"work": "fixture.work", "ordinal": 1},
        "actors": ["actor.kiyotaka"],
        "topics": [],
        "provenance": {
            "epistemic_status": "UNVERIFIED",
            "continuity": ["ln"],
            "supports": [{"work": "fixture.work", "edition": "fixture-v1", "locator": {"chapter": 1}, "strength": "primary"}],
        },
    }
    assert _schema_errors("evidence", value, "evidence.schema.json")


@pytest.mark.parametrize(
    "bound",
    [
        {"mode": "EXACT", "exact": {"anchor": "Y1_M01"}, "holds_by": {"anchor": "Y1_M02"}},
        {"mode": "HOLDS_BY", "holds_by": {"anchor": "Y1_M01"}, "origin": "UNKNOWN", "basis": "x", "exact": {"anchor": "Y1_M02"}},
        {"mode": "UNKNOWN", "basis": "x", "holds_by": {"anchor": "Y1_M01"}},
    ],
)
def test_timebound_rejects_fields_from_other_modes(bound):
    assert _schema_errors("timebound", bound, "evidence.schema.json")


def test_compiler_derives_open_conflicts_and_rejects_missing_bidirectional_link():
    suite, evidence, claims, registry = _inputs()
    claim = next(item for item in claims if item["id"] == "fixture.claim.future")
    claim["provenance"]["conflicts"] = []
    conflicts = [{
        "id": "conflict.fixture.blocker",
        "schema_version": 1,
        "question": "Is the fixture claim established?",
        "positions": [{"label": "a", "statement": "a"}, {"label": "b", "statement": "b"}],
        "status": "open",
        "blocks_compilation_of": [claim["id"]],
    }]

    records = compile_records([evidence], claims, registry, conflicts)
    assert "conflict_provenance_mismatch" in records[0]["compilation_errors"]

    claim["provenance"]["conflicts"] = [conflicts[0]["id"]]
    records = compile_records([evidence], claims, registry, conflicts)
    assert records[0]["resolved_claims"][0]["open_conflicts"] == [conflicts[0]["id"]]

    resolved = deepcopy(conflicts)
    resolved[0]["status"] = "resolved"
    resolved_records = compile_records([evidence], claims, registry, resolved)
    assert resolved_records[0]["resolved_claims"][0]["open_conflicts"] == []


def test_manifest_identity_includes_conflict_registry_and_compiler_version():
    corpus = load_corpus(REPO)
    claims = load_claims(REPO)
    registry = load_source_registry(REPO)
    conflicts = load_conflicts(REPO)
    before = corpus_manifest(corpus, claims, registry, conflicts)
    changed = deepcopy(conflicts)
    changed[0]["status"] = "resolved" if changed[0]["status"] == "open" else "open"
    after = corpus_manifest(corpus, claims, registry, changed)

    assert before["digest"] != after["digest"]
    assert before["compiler_version"] == COMPILER_VERSION == 2
    assert before["conflict_ids"] == sorted(conflict["id"] for conflict in conflicts)
    assert before["evidence_collections"]


def test_collection_loader_fails_closed_on_unknown_container_version():
    # The loader validates each collection before flattening; its direct schema contract
    # is also exercised here so this stays a deterministic test without filesystem mutation.
    from common import _schema_validator  # noqa: E402

    invalid = {"schema_version": 999, "evidence": []}
    with pytest.raises(ValidationError):
        _schema_validator("evidence_collection.schema.json").validate(invalid)


@pytest.mark.parametrize("filters", [{"evidence_id": ["ev.safe"]}, {"topic": ["mediation"]}])
def test_unknown_filter_keys_are_rejected_instead_of_ignored(filters):
    suite, _, _, _ = _inputs()
    request = deepcopy(suite["cases"][0])
    request["filters"] = filters
    with pytest.raises(ValueError, match="unknown keys"):
        retrieve([], request, suite["time_order"], "manifest", suite["seed_time"])


def test_collection_file_digest_and_schema_identity_change_manifest():
    corpus = load_corpus(REPO)
    claims = load_claims(REPO)
    registry = load_source_registry(REPO)
    base = corpus_manifest(corpus, claims, registry)
    corpus.collection_sources = [dict(corpus.collection_sources[0], digest="changed")]
    changed = corpus_manifest(corpus, claims, registry)
    assert base["digest"] != changed["digest"]


def test_default_compiler_and_manifest_load_repository_conflicts():
    suite = load_suite(REPO)
    claim = next(
        item for item in load_claims(REPO)
        if item["id"] == "y1.initial.class-labels"
    )
    conflict_id = "conflict.governance.class-label-reassignment"
    evidence = deepcopy(next(
        item for item in suite["fixture_records"]
        if item["id"] == "ev.fixture.post-divergence.fact"
    ))
    evidence["supports_claims"] = [claim["id"]]
    registry = merge_source_registries(
        load_source_registry(REPO), suite["fixture_source_registry"]
    )

    record = compile_records([evidence], [claim], registry)[0]
    assert record["resolved_claims"][0]["open_conflicts"] == [conflict_id]

    manifest = corpus_manifest([], [claim], load_source_registry(REPO))
    assert conflict_id in manifest["conflict_ids"]


def test_fixture_conflict_changes_suite_and_run_identity():
    suite = load_suite(REPO)
    evidence = load_corpus(REPO)
    claims = load_claims(REPO)
    registry = load_source_registry(REPO)

    open_report = evaluate_suite(evidence, claims, registry, suite)
    resolved_suite = deepcopy(suite)
    resolved_suite["fixture_conflicts"][0]["status"] = "resolved"
    resolved_report = evaluate_suite(evidence, claims, registry, resolved_suite)

    assert open_report["suite"]["digest"] != resolved_report["suite"]["digest"]
    assert open_report["run_digest"] != resolved_report["run_digest"]
