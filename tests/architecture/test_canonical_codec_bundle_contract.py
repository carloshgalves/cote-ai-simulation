"""Structural regression checks for the language-neutral codec bundle.

These tests lint the normative metadata.  They are deliberately not a Python
implementation of the causal codec or Simulation Engine.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path


BUNDLE = (
    Path(__file__).parents[2]
    / "docs"
    / "architecture"
    / "canonical-codec-v1-bundle"
)


def _json(name: str):
    return json.loads((BUNDLE / name).read_text(encoding="utf-8"))


def _enum_bindings(registries: dict) -> dict[str, str]:
    return dict(registries["enum_bindings"])


def test_persisted_records_contain_every_identity_preimage_component() -> None:
    cddl = (BUNDLE / "foundation.cddl").read_text(encoding="utf-8")
    registries = _json("registries.json")

    assert "source-closure = [\n  closure-id: id32,\n  run-id: id32," in cddl
    assert "event-fields = (\n  event-id: id32,\n  run-id: id32," in cddl

    identities = {
        entry["reference_kind"]: entry for entry in registries["reference_identities"]
    }
    assert identities["source_closure"]["id"]["components"][0] == (
        "source-closure.run-id"
    )
    assert identities["event"]["id"]["components"][0] == "event.run-id"


def test_reference_dispatch_is_total_for_ids_digests_and_roots() -> None:
    registries = _json("registries.json")
    reference_codes = {code for _, code in registries["enums"]["reference_kind"]}
    identities = registries["reference_identities"]
    operations = {operation[0] for operation in registries["domain_operations"]}

    assert {entry["code"] for entry in identities} == reference_codes
    for entry in identities:
        assert entry["persisted_root"]
        if "dispatch_registry" not in entry:
            assert entry["id"]["operation"] in operations
            assert entry["digest"]["operation"] in operations

    unit_codes = {code for _, code in registries["enums"]["unit_kind"]}
    unit_dispatch = registries["unit_reference_dispatch"]
    assert {entry["code"] for entry in unit_dispatch} == unit_codes
    response_codes = {code for _, code in registries["enums"]["slot_response_kind"]}
    response_dispatch = registries["slot_response_reference_dispatch"]
    assert {entry["code"] for entry in response_dispatch} == response_codes
    for entry in unit_dispatch + response_dispatch:
        assert entry["persisted_root"]
        if "dispatch_registry" not in entry:
            assert entry["id_operation"] in operations
            assert entry["digest_operation"] in operations


def test_field_domains_are_exact_and_use_real_record_paths() -> None:
    registries = _json("registries.json")
    bindings = _enum_bindings(registries)
    cddl = (BUNDLE / "foundation.cddl").read_text(encoding="utf-8")

    assert bindings["conflict-set.conflict-kind"] == "conflict_kind"
    assert bindings["trigger-activation.lifecycle"] == "pending_consumed_lifecycle"
    assert bindings["perception-task.lifecycle"] == "pending_lifecycle"
    assert bindings["decision-record.subject.input-kind"] == "admitted_unit_kind"
    assert bindings["test-enum-u8.value"] == "fixture_test_enum_u8"
    assert "test-enum-u8 = [value: u8]" in cddl

    assert registries["enums"]["pending_consumed_lifecycle"] == [
        ["pending", 0],
        ["consumed", 2],
    ]
    assert registries["enums"]["pending_lifecycle"] == [["pending", 0]]
    assert registries["enums"]["admitted_unit_kind"] == [
        ["slot", 0],
        ["exogenous_input", 1],
        ["scheduled_occurrence", 2],
        ["trigger_activation", 3],
    ]


def test_admitted_unit_ordering_is_self_contained_and_covered() -> None:
    cddl = (BUNDLE / "foundation.cddl").read_text(encoding="utf-8")
    fixtures = _json("fixtures.json")

    assert (
        "admitted-unit = [eligibility: eligibility-coordinate, unit-kind: u8, "
        "source-id: machine-id,\n                   unit-id: id32, unit-digest: digest32]"
        in cddl
    )
    cases = {case["case_id"]: case for case in fixtures["normalization_cases"]}
    fence_case = cases["normalize.admission-fence.admitted-units-permutations"]
    assert len(fence_case["inputs"]) == 2
    assert fence_case["inputs"][0] != fence_case["inputs"][1]
    assert fence_case["expected_payload_cbor_hex"]
    mismatch_cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "admitted_unit_ledger_mismatch"
    ]
    assert {case["mutate"][0] for case in mismatch_cases} == {
        "source-id",
        "unit-digest",
    }


def test_genesis_explicitly_pins_profile_and_schema_bundle_without_hash_cycle() -> None:
    cddl = (BUNDLE / "foundation.cddl").read_text(encoding="utf-8")
    fixtures = _json("fixtures.json")
    schema_manifest = _json("schema-manifest.json")
    conformance_manifest = _json("conformance-manifest.json")

    assert "schema-ref = [schema-id, schema-version]" in cddl
    assert "codec-version: u32" in cddl
    assert "identity-algorithm-version: machine-id" in cddl
    assert "unicode-version: machine-id" in cddl
    assert "normalization-profile-id: machine-id" in cddl

    assert {entry[0] for entry in schema_manifest["artifacts"]} == {
        "foundation.cddl",
        "registries.json",
    }
    assert {entry[0] for entry in conformance_manifest["artifacts"]} == {
        "fixtures.json"
    }

    schema_manifest_bytes = (BUNDLE / "schema-manifest.json").read_bytes()
    schema_bundle_hash = hashlib.sha256(
        b"cote.csf.bundle.schema.v1\x00" + schema_manifest_bytes
    ).hexdigest()
    pinning = next(
        case for case in fixtures["semantic"] if case["case_id"] == "genesis.pinning"
    )
    assert pinning["codec_version"] == 1
    assert pinning["identity_algorithm_version"] == "cote.csf.sha256.v1"
    assert pinning["unicode_version"] == "unicode-15.1.0"
    assert pinning["normalization_profile_id"] == "unicode.npss.nfc.15.1.0"
    assert pinning["schema_bundle_hash"] == schema_bundle_hash


def test_indeterminate_evidence_ordering_uses_a_registered_digest_operation() -> None:
    cddl = (BUNDLE / "foundation.cddl").read_text(encoding="utf-8")
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")

    operation = [
        "cote.csf.digest.indeterminate-evidence-refs",
        "cote.csf.schema.indeterminate-evidence-ref-list",
        1,
    ]
    assert operation in registries["domain_operations"]
    assert (
        "indeterminate-evidence-ref-list = [0*1024 evidence-ref]" in cddl
    )

    derived = {
        item[0]: item[1:] for item in registries["derived_ordering_components"]
    }
    assert derived["indeterminate-evidence-refs-digest"] == [
        "cycle-abort.indeterminate-records[*].evidence-refs",
        "cote.csf.digest.indeterminate-evidence-refs",
        "cote.csf.schema.indeterminate-evidence-ref-list",
        1,
    ]

    cases = {case["case_id"]: case for case in fixtures["semantic"]}
    ordering_case = cases[
        "normalize.cycle-abort-indeterminate-records-by-evidence-digest"
    ]
    assert len(ordering_case["input_cbor_hex"]) == 2
    assert ordering_case["input_cbor_hex"][0] != ordering_case["input_cbor_hex"][1]
    assert ordering_case["ordering_component_operation"] == operation[0]


def test_registered_preimages_bind_all_causal_enum_and_role_fields() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    enum_bindings = dict(registries["enum_bindings"])
    role_bindings = dict(registries["role_bindings"])

    expected_enum_bindings = {
        "conflict-set-id-preimage.conflict-kind": "conflict_kind",
        "event-id-preimage.event-order-key.phase": "phase",
        "knowledge-input-id-preimage.kind": "knowledge_kind",
        "occurrence-id-preimage.origin[0]": "created_by_variant",
        "decision-round-id-preimage.origin[0]": "round_created_by_variant",
        "claim-id-preimage.origin.kind": "reference_kind",
        "transmission-id-preimage.origin.kind": "reference_kind",
    }
    expected_role_bindings = {
        "occurrence-id-preimage.occurrence-role": "occurrence_creation_role",
        "decision-round-id-preimage.round-role": "decision_round_creation_role",
        "claim-id-preimage.claim-role": "claim_creation_role",
        "transmission-id-preimage.transmission-role": "transmission_creation_role",
        "candidate-id-preimage.candidate-role": "extension:candidate_role",
        "event-id-preimage.event-order-key.event-role": "extension:event_role",
        "observation-id-preimage.observation-role": "extension:observation_role",
        "knowledge-input-id-preimage.input-role": "extension:knowledge_input_role",
    }
    for path, registry in expected_enum_bindings.items():
        assert enum_bindings[path] == registry
    for path, registry in expected_role_bindings.items():
        assert role_bindings[path] == registry

    covered_paths = {
        case["field_path"]
        for case in fixtures["semantic"]
        if case["kind"] == "operation_field_binding"
    }
    assert covered_paths >= expected_enum_bindings.keys() | expected_role_bindings.keys()


def test_documentation_treats_two_language_conformance_as_future_gate() -> None:
    contract = (BUNDLE.parent / "canonical-codec-v1.md").read_text(encoding="utf-8")
    readme = (BUNDLE / "README.md").read_text(encoding="utf-8")

    assert "O CI de uma\nimplementação candidata deve executar" in contract
    assert "**Execução de conformidade neste checkpoint:** pendente" in readme
