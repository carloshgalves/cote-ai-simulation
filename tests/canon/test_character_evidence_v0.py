from __future__ import annotations

import json
from pathlib import Path

import yaml
from jsonschema import Draft202012Validator

ROOT = Path(__file__).resolve().parents[2]
CHAR_DIR = ROOT / "data" / "canon" / "characters"
ACTOR_DIR = ROOT / "data" / "canon" / "actors"
EVIDENCE_DIR = ROOT / "data" / "canon" / "evidence" / "characters"
EVAL_DIR = ROOT / "evals" / "character-fidelity"
SCHEMA_DIR = ROOT / "data" / "canon" / "schema"

SLUGS = {
    "kiyotaka-ayanokoji": "actor.kiyotaka-ayanokoji",
    "suzune-horikita": "actor.suzune-horikita",
    "kakeru-ryuen": "actor.kakeru-ryuen",
    "kikyo-kushida": "actor.kikyo-kushida",
    "yosuke-hirata": "actor.yosuke-hirata",
    "honami-ichinose": "actor.honami-ichinose",
}


def load_yaml(path: Path):
    return yaml.safe_load(path.read_text(encoding="utf-8"))


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def evidence_records() -> dict[str, dict]:
    records: dict[str, dict] = {}
    for path in sorted(EVIDENCE_DIR.glob("*.yaml")):
        collection = load_yaml(path)
        assert collection["schema_version"] == 1
        for record in collection["records"]:
            assert record["id"] not in records
            records[record["id"]] = record
    return records


def pack_assertions(pack: dict):
    yield from pack["identity"]["relationships_at_horizon"]
    yield from pack["initial_knowledge"]
    yield from pack["initial_beliefs"]
    yield from pack["stable_tendencies"]
    for bucket in ("demonstrated", "inferred", "possible", "future_only"):
        yield from pack["goals"][bucket]
    yield from pack["capabilities"]
    yield from pack["risk_tolerance"]
    yield from pack["social_linguistic_tendencies"]
    yield from pack["blind_spots"]
    yield from pack["capability_manifestation"]


def all_evidence_refs(pack: dict):
    for assertion in pack_assertions(pack):
        yield from assertion["evidence_refs"]
    for pattern in pack["decision_patterns"]:
        yield from pattern["evidence_refs"]


def test_v0_has_exactly_six_focal_packs_and_actor_stubs():
    assert {p.stem for p in CHAR_DIR.glob("*.yaml")} == set(SLUGS)
    assert {p.stem for p in ACTOR_DIR.glob("*.yaml")} == set(SLUGS)

    for slug, actor_id in SLUGS.items():
        pack = load_yaml(CHAR_DIR / f"{slug}.yaml")
        actor = load_yaml(ACTOR_DIR / f"{slug}.yaml")
        assert pack["character_id"] == actor_id
        assert actor["id"] == actor_id
        assert pack["knowledge_horizon"]["anchor"] == "Y1_START"
        assert "initial_beliefs" in pack
        assert pack["risk_tolerance"]


def test_v0_contains_32_unique_paraphrased_evidence_records():
    records = evidence_records()
    assert len(records) == 32

    for record in records.values():
        assert record["provenance"]["epistemic_status"] == "UNVERIFIED"
        assert record["text"].strip()
        assert len(record["text"]) < 1200
        for support in record["provenance"].get("supports", []):
            assert support.get("quote_policy", "paraphrase-only") == "paraphrase-only"


def test_pack_and_eval_schemas_are_valid_and_packs_conform():
    pack_schema = load_json(SCHEMA_DIR / "character_evidence_pack.schema.json")
    eval_schema = load_json(SCHEMA_DIR / "character_fidelity_eval.schema.json")
    evidence_collection_schema = load_json(SCHEMA_DIR / "evidence_collection.schema.json")

    for schema in (pack_schema, eval_schema, evidence_collection_schema):
        Draft202012Validator.check_schema(schema)

    pack_validator = Draft202012Validator(pack_schema)
    eval_validator = Draft202012Validator(eval_schema)

    for slug in SLUGS:
        pack_validator.validate(load_yaml(CHAR_DIR / f"{slug}.yaml"))
        eval_validator.validate(load_yaml(EVAL_DIR / f"{slug}.yaml"))


def test_every_pack_evidence_reference_resolves():
    records = evidence_records()
    for slug in SLUGS:
        pack = load_yaml(CHAR_DIR / f"{slug}.yaml")
        missing = sorted(set(all_evidence_refs(pack)) - set(records))
        assert missing == [], f"{slug} has missing evidence refs: {missing}"


def test_future_evidence_never_becomes_initial_known_information():
    for slug in SLUGS:
        pack = load_yaml(CHAR_DIR / f"{slug}.yaml")
        for assertion in pack_assertions(pack):
            if assertion["evidence_temporality"] == "FUTURE_EVIDENCE_ONLY":
                assert assertion["horizon_relation"] != "KNOWN_AT_HORIZON"

        for assertion in pack["initial_knowledge"]:
            if assertion["horizon_relation"] == "KNOWN_AT_HORIZON":
                assert assertion["evidence_temporality"] in {"AT_OR_BEFORE_HORIZON", "MIXED"}


def test_no_character_assertion_is_promoted_to_verified():
    for slug in SLUGS:
        actor = load_yaml(ACTOR_DIR / f"{slug}.yaml")
        pack = load_yaml(CHAR_DIR / f"{slug}.yaml")
        assert actor["provenance"]["epistemic_status"] == "UNVERIFIED"
        for assertion in pack_assertions(pack):
            assert assertion["status"] != "VERIFIED"
        for pattern in pack["decision_patterns"]:
            assert pattern["status"] != "VERIFIED"


def test_physical_capacity_is_not_duplicated_in_character_packs():
    for slug in SLUGS:
        pack = load_yaml(CHAR_DIR / f"{slug}.yaml")
        assert "Embodiment" in pack["physical_capacity_boundary"]
        dimensions = {capability["dimension"] for capability in pack["capabilities"]}
        assert "physical" not in dimensions
        for capability in pack["capabilities"]:
            assert not isinstance(capability["can_do"], (int, float))
            assert not isinstance(capability["manifestation_at_horizon"], (int, float))


def test_eval_suites_cover_positive_contrast_anti_and_leakage():
    records = evidence_records()
    for slug, actor_id in SLUGS.items():
        suite = load_yaml(EVAL_DIR / f"{slug}.yaml")
        assert suite["character_id"] == actor_id
        assert suite["knowledge_horizon"] == "Y1_START"
        assert len(suite["positive_cases"]) >= 2
        assert len(suite["contrast_cases"]) >= 1
        assert len(suite["anti_cases"]) >= 2
        assert len(suite["knowledge_leakage_cases"]) >= 2

        for group in ("positive_cases", "contrast_cases", "anti_cases", "knowledge_leakage_cases"):
            for case in suite[group]:
                assert set(case["evidence_refs"]) <= set(records)
                assert case["expected"]
                assert case["forbidden"]

        for case in suite["knowledge_leakage_cases"]:
            assert case["withheld_world_truth"]
            assert case["available_information"]


def test_priority_contrast_matrix_has_required_pairs():
    matrix = load_yaml(EVAL_DIR / "contrast-matrix.yaml")
    pair_ids = {pair["id"] for pair in matrix["priority_pairs"]}
    assert {
        "contrast.horikita-hirata.weak-teammate",
        "contrast.ryuen-ichinose.rival-dispute",
        "contrast.kiyotaka-ryuen.hidden-advantage",
        "contrast.kushida-horikita.recruitment",
        "contrast.hirata-ryuen.group-compliance",
    } <= pair_ids


def test_eval_evidence_references_resolve():
    records = evidence_records()
    matrix = load_yaml(EVAL_DIR / "contrast-matrix.yaml")
    for pair in matrix["priority_pairs"]:
        assert set(pair["evidence_refs"]) <= set(records)

    for slug in SLUGS:
        suite = load_yaml(EVAL_DIR / f"{slug}.yaml")
        for group in ("positive_cases", "contrast_cases", "anti_cases", "knowledge_leakage_cases"):
            for case in suite[group]:
                assert set(case["evidence_refs"]) <= set(records)
