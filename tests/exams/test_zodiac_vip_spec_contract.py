"""Contract checks for declarative Zodiac/VIP V1 inputs.

This deliberately does not claim the Examination Engine is implemented.
"""
from pathlib import Path
import hashlib
import itertools
import json

import yaml

ROOT = Path(__file__).resolve().parents[2]
MODEL = ROOT / "data/models/exams/zodiac-vip-v1.yaml"
VECTORS = ROOT / "data/models/exams/conformance/zodiac-vip-v1.yaml"
MICRO = ROOT / "data/models/exams/micro-secret-v1.yaml"
SPEC = ROOT / "docs/spec/zodiac-vip-exam-v1.md"


def read_yaml(path):
    return yaml.safe_load(path.read_text(encoding="utf-8"))


def test_provisional_model_never_impersonates_verified_canon():
    model = read_yaml(MODEL)
    assert model["schema_version"] == 1
    assert model["not_canon"] is True
    assert model["status"] == "PROVISIONAL"
    assert model["profile"] == "CANON_ADAPTED"
    assert model["provenance"]["canon_status"] == "UNVERIFIED"
    assert model["identity"]["canonical_work_dependency"] == "ln.y1.v04"
    assert model["groups"]["count"] == 12
    assert model["secret_roles"]["total_roles"] == 12


def test_conformance_vectors_and_setup_mutations_are_complete_and_unique():
    suite = read_yaml(VECTORS)
    assert suite["not_canon"] is True
    vectors = suite["vectors"]
    ids = [item["id"] for item in vectors]
    assert len(vectors) == 18
    assert len(set(ids)) == 18
    assert {f"V{n:02d}" for n in range(1, 19)} == {
        item.split("-")[0] for item in ids
    }
    assert len(suite["static_setup_generators"]["mutations"]) == 8
    mutations = suite["static_setup_generators"]["mutations"]
    assert len({item["id"] for item in mutations}) == len(mutations)
    assert len(suite["economic_properties"]) == 7
    assert {p["id"].split("-", 1)[0] for p in suite["economic_properties"]} == {
        f"P{n:02d}" for n in range(1, 8)
    }


def test_four_outcomes_are_distinct_and_scoring_fixture_closes():
    suite = read_yaml(VECTORS)
    by_id = {item["id"].split("-")[0]: item for item in suite["vectors"]}
    assert by_id["V01"]["expect"]["outcome"] == "OUTCOME_1_SHARED_SUCCESS"
    assert by_id["V02"]["expect"]["outcome"] == "OUTCOME_2_VIP_HIDDEN"
    assert by_id["V04"]["expect"]["outcome"] == "OUTCOME_3_TRAITOR_CORRECT"
    assert by_id["V05"]["expect"]["outcome"] == "OUTCOME_4_TRAITOR_WRONG"
    # 13-person group: the VIP receives 1M and the other 12 receive 500k.
    assert by_id["V01"]["expect"]["total_pp"] == 1_000_000 + 12 * 500_000
    for key in ("V04", "V05"):
        assert sum(by_id[key]["expect"]["cp_deltas"].values()) == 0


def test_simultaneous_inputs_and_audit_projection_are_specified():
    by_id = {
        item["id"].split("-")[0]: item
        for item in read_yaml(VECTORS)["vectors"]
    }
    simultaneous = by_id["V14"]
    assert len(simultaneous["execute_permutations"]) == 2
    assert simultaneous["expect_for_every_permutation"]["final_state_hash_equal_across_permutations"] is True
    assert simultaneous["expect_for_every_permutation"]["priority_winner"] == "s-v14-wrong"
    public = by_id["V15"]["expect_public"]
    assert public["vip_actor_id"] == "ABSENT"
    assert public["terminal_submitter_actor_id"] == "ABSENT"


def test_no_direct_expulsion_and_micro_exam_remain_declared_only():
    suite = read_yaml(VECTORS)
    assert any(item["id"] == "P07-no-direct-expulsion" for item in suite["economic_properties"])
    micro = read_yaml(MICRO)
    assert micro["not_canon"] is True
    assert micro["status"] == "TEST_FIXTURE"
    assert micro["participants"]["count"] == 8
    assert len(micro["groups"]) == 2
    assert "resolver_never_queries_llm" in micro["assertions"]
    spec = SPEC.read_text(encoding="utf-8")
    assert "### D2" in spec and "### D3" in spec
    assert "SIMULATION_AUTHORED" in spec

def _fixture_cohort_resolution(vector, permutation):
    """Small deterministic fixture oracle; NOT an executable Examination Engine.

    Input order may change; only the full closed cohort determines the receipt,
    quota and terminal outcome. This checks the new normative vectors.
    """
    messages = vector["action_set"]["submissions"]
    by_transport = {
        x.get("transport_attempt_id", x["submission_id"]): x for x in messages
    }
    seen_id = {}
    for arrival in permutation:
        msg = by_transport[arrival]
        canonical_fields = (msg["actor_id"], msg["guess"], msg["submission_id"])
        original = seen_id.setdefault(msg["submission_id"], canonical_fields)
        if original != canonical_fields:
            raise ValueError("causal identity reused with divergent payload")
    ranks = vector["identity_policy_fixture"]["derived_priority"]
    per_actor = {}
    for submission_id, fields in seen_id.items():
        actor_id, guess, _ = fields
        per_actor.setdefault(actor_id, []).append((int(ranks[submission_id], 16), submission_id, guess))
    selected = {
        actor_id: min(candidates) for actor_id, candidates in per_actor.items()
    }
    if set(selected) != {"b1"}:
        raise AssertionError("fixture only covers same-actor arbitration")
    priority, submission_id, guess = selected["b1"]
    right = guess == "d2"
    outcome = "OUTCOME_3_TRAITOR_CORRECT" if right else "OUTCOME_4_TRAITOR_WRONG"
    result = {
        "canonical_actor_submission": submission_id,
        "received_unique_count": 1,
        "receipt_ids": [submission_id],
        "quota_consumed_by_actor": {"b1": 1},
        "outcomes_count": 1,
        "outcome": outcome,
        "cp_deltas": {"B": 50 if right else -50, "D": -50 if right else 50},
        "pp": {"b1": 500000} if right else {"d2": 500000},
    }
    return result


def test_closed_cohort_quota_is_permutation_invariant_for_same_actor():
    suite = read_yaml(VECTORS)
    by_id = {item["id"].split("-")[0]: item for item in suite["vectors"]}
    for vector_id in ("V16", "V17"):
        vector = by_id[vector_id]
        output = [
            _fixture_cohort_resolution(vector, ordering)
            for ordering in vector["execute_permutations"]
        ]
        assert output[0] == output[1]
        expected = vector["expect_for_every_permutation"]
        for key in (
            "canonical_actor_submission", "received_unique_count",
            "quota_consumed_by_actor", "outcome", "cp_deltas", "pp", "outcomes_count"
        ):
            assert output[0][key] == expected[key], (vector_id, key)
        fingerprints = [
            hashlib.sha256(json.dumps(x, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
            for x in output
        ]
        assert len(set(fingerprints)) == 1
        assert expected["final_state_hash_equal_across_permutations"]
        assert expected["economic_ledger_hash_equal_across_permutations"]
        assert expected["receipt_ids_equal_across_permutations"]
    assert by_id["V16"]["expect_for_every_permutation"]["other_disposition"] == "SUPERSEDED_BY_SAME_ACTOR_COHORT"
    assert by_id["V17"]["expect_for_every_permutation"]["retransmission_disposition"] == "SAME_RECEIPT"


def test_reused_causal_identity_with_divergent_payload_fails_both_orders():
    suite = read_yaml(VECTORS)
    vector = next(item for item in suite["vectors"] if item["id"].startswith("V18-"))
    for ordering in vector["execute_permutations"]:
        try:
            _fixture_cohort_resolution(vector, ordering)
        except ValueError as exc:
            assert "divergent payload" in str(exc)
        else:
            raise AssertionError("same identity with two payloads did not fail closed")
    expected = vector["expect_for_every_permutation"]
    assert expected["cohort_rejected"] is True
    assert expected["received_unique_count"] == 0
    assert expected["quota_consumed_by_actor"] == {"b1": 0}
    assert expected["outcomes_count"] == 0
    assert expected["state_hash_equals_pre_cohort"] is True
    assert expected["economic_ledger_hash_equals_pre_cohort"] is True


def test_spec_and_model_make_actor_canonicalization_a_pre_quota_stage():
    model = read_yaml(MODEL)
    assert model["submission"]["admission_boundary"] == "CLOSED_CSF_COHORT"
    assert model["submission"]["per_actor_canonicalization_before_quota"] is True
    assert model["submission"]["identity_same_id_different_payload"] == "FAIL_CLOSED_NO_PARTIAL_EFFECTS"
    stages = model["simultaneity"]["cohort_preparation"]
    assert stages.index("SELECT_ONE_REPRESENTATIVE_PER_ACTOR_BY_PRIORITY") < stages.index(
        "APPLY_PRE_COHORT_QUOTA_AND_SEMANTIC_VALIDATION"
    )
    assert stages[-1] == "COMMIT_RECEIPTS_OUTCOME_AND_ECONOMY_ATOMICALLY"
    text = SPEC.read_text(encoding="utf-8")
    assert "SUPERSEDED_BY_SAME_ACTOR_COHORT" in text
    assert "mesmo estado pré-coorte" in text
    assert "payload diferente" in text
