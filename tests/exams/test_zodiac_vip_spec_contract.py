"""Contract checks for declarative Zodiac/VIP V1 inputs.

This deliberately does not claim the Examination Engine is implemented.
"""
from pathlib import Path

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
    assert len(vectors) == 15
    assert len(set(ids)) == 15
    assert {f"V{n:02d}" for n in range(1, 16)} == {
        item.split("-")[0] for item in ids
    }
    assert len(suite["static_setup_generators"]["mutations"]) == 8
    mutations = suite["static_setup_generators"]["mutations"]
    assert len({item["id"] for item in mutations}) == len(mutations)
    assert len(suite["economic_properties"]) == 7
    assert {p["id"] for p in suite["economic_properties"]} == {
        f"P{n:02d}-" + p["id"].split("-", 1)[1]
        for n in range(1, 8)
        for p in suite["economic_properties"]
        if p["id"].startswith(f"P{n:02d}-")
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
