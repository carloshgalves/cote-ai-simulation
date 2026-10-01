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


def test_every_positive_vector_hashes_its_canonical_bytes() -> None:
    fixtures = _json("fixtures.json")
    mismatches = {}
    for case in fixtures["positive"]:
        actual = hashlib.sha256(bytes.fromhex(case["canonical_hex"])).hexdigest()
        if actual != case["sha256"]:
            mismatches[case["case_id"]] = {
                "expected": case["sha256"],
                "actual": actual,
            }

    assert mismatches == {}


def test_semantic_enum_matrix_matches_normative_bindings() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    expected = _enum_bindings(registries)
    covered: dict[str, set[str]] = {}

    for case in fixtures["semantic"]:
        field_path = case.get("field_path")
        if field_path not in expected or case["kind"] not in {
            "enum_binding",
            "operation_field_binding",
            "record_enum_binding",
            "record_reference_kind_binding",
        }:
            continue
        covered.setdefault(field_path, set()).add(case["registry"])

        if "accepted_code" in case:
            valid_codes = {code for _, code in registries["enums"][expected[field_path]]}
            assert case["accepted_code"] in valid_codes
            assert case["rejected_code"] not in valid_codes

    assert covered == {field_path: {registry} for field_path, registry in expected.items()}

    cases = {case["case_id"]: case for case in fixtures["semantic"]}
    perception = cases["enum-binding.perception-task.lifecycle"]
    assert perception["registry"] == "pending_lifecycle"
    assert perception["rejected_code"] == 2
    trigger = cases["enum-binding.trigger-activation.lifecycle"]
    assert trigger["registry"] == "pending_consumed_lifecycle"
    assert trigger["accepted_code"] == 2
    assert trigger["rejected_code"] == 1


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


def test_every_persisted_root_has_one_self_describing_reference() -> None:
    cddl = (BUNDLE / "foundation.cddl").read_text(encoding="utf-8")
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")

    identities = registries["reference_identities"]
    by_root = {entry["persisted_root"]: entry for entry in identities}
    assert len(by_root) == len(identities)
    assert all("dispatch_registry" not in entry for entry in identities)

    unit_roots = {
        entry["persisted_root"]
        for entry in registries["unit_reference_dispatch"]
        if "dispatch_registry" not in entry
    }
    response_roots = {
        entry["persisted_root"]
        for entry in registries["slot_response_reference_dispatch"]
    }
    assert unit_roots | response_roots <= by_root.keys()

    reference_cases = {
        case["persisted_root"]: case
        for case in fixtures["semantic"]
        if case["kind"] == "record_to_reference"
    }
    for persisted_root in unit_roots | response_roots:
        encoded = bytes.fromhex(
            reference_cases[persisted_root]["expected_causal_ref_cbor_hex"]
        )
        assert encoded[0] == 0x83
        assert encoded[1] == by_root[persisted_root]["code"]

    slot_dispatch = next(
        entry
        for entry in registries["unit_reference_dispatch"]
        if entry["unit_kind"] == "slot"
    )
    assert slot_dispatch["discriminant"] == "input-ref.slot-response-kind"
    assert "input-ref =\n  [input-kind: 0, slot-response-kind: u8" in cddl

    slot_cases = {
        case["persisted_root"]: bytes.fromhex(case["expected_input_ref_cbor_hex"])
        for case in reference_cases.values()
        if case["persisted_root"] in response_roots
    }
    assert {encoded[0] for encoded in slot_cases.values()} == {0x84}
    assert {encoded[1] for encoded in slot_cases.values()} == {0}
    assert {encoded[2] for encoded in slot_cases.values()} == {0, 1}


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


def test_typed_causal_refs_use_exact_reference_kind_subsets() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    bindings = _enum_bindings(registries)

    expected = {
        "closure-proof-item.closure-ref.kind": (
            "source_closure_reference_kind",
            {2},
        ),
        "cohort-slot.response-ref.kind": (
            "slot_response_reference_kind",
            {5, 23},
        ),
        "admission-fence.retry-ref.kind": (
            "attempt_retry_reference_kind",
            {14},
        ),
        "affordance-assessment.subject.kind": (
            "action_proposal_reference_kind",
            {5},
        ),
        "attempt-retry.aborted-envelope-ref.kind": (
            "cycle_abort_reference_kind",
            {13},
        ),
        "decision-record.conflict-set-refs[*].kind": (
            "conflict_set_reference_kind",
            {9},
        ),
        "decision-record.rng-draw-refs[*].kind": (
            "rng_draw_reference_kind",
            {24},
        ),
        "cycle-abort.failure-evidence-refs[*].ref-kind": (
            "failure_evidence_reference_kind",
            {9, 24, 25, 26, 27},
        ),
        "cycle-control-state.fence-ref.kind": (
            "admission_fence_reference_kind",
            {11},
        ),
        "cycle-control-state.retry-ref.kind": (
            "attempt_retry_reference_kind",
            {14},
        ),
        "cycle-control-state.last-terminal-envelope-ref.kind": (
            "terminal_envelope_reference_kind",
            {12, 13},
        ),
        "claim.prior-claim-refs[*].kind": ("claim_reference_kind", {18}),
        "observation.claim-refs[*].kind": ("claim_reference_kind", {18}),
        "knowledge-input.observation-ref.kind": (
            "observation_reference_kind",
            {16},
        ),
        "knowledge-input.claim-refs[*].kind": ("claim_reference_kind", {18}),
        "transmission.content.claims[*].kind": ("claim_reference_kind", {18}),
    }

    semantic = {
        case["field_path"]: case
        for case in fixtures["semantic"]
        if case["kind"] == "record_reference_kind_binding"
    }
    for field_path, (registry, allowed_codes) in expected.items():
        assert bindings[field_path] == registry
        assert {code for _, code in registries["enums"][registry]} == allowed_codes
        case = semantic[field_path]
        assert case["registry"] == registry
        assert set(case["accepted_codes"]) == allowed_codes
        assert case["rejected_code"] not in allowed_codes
        assert case["record_template_case_id"].startswith("root.")
        assert case["container_field_path"]
        assert set(map(int, case["accepted_container_cbor_hex"])) == allowed_codes
        assert case["rejected_container_cbor_hex"]
        assert case["rejected_error_code"] == "UNKNOWN_ENUM"


def test_decision_record_variants_close_all_conditional_fields() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")

    constraint = next(
        item
        for item in registries["record_constraints"]
        if item["record"] == "decision-record"
    )
    assert constraint["discriminant"] == "decision-record.disposition"
    assert constraint["variants"] == [
        {
            "code": 0,
            "required_present": ["candidate-id"],
            "required_empty": [],
            "required_absent": ["successor-input-id", "canonical-unit-id"],
        },
        {
            "code": 1,
            "required_present": ["candidate-id"],
            "required_empty": [],
            "required_absent": ["successor-input-id", "canonical-unit-id"],
        },
        {
            "code": 2,
            "required_present": ["candidate-id", "successor-input-id"],
            "required_empty": [],
            "required_absent": ["canonical-unit-id"],
        },
        {
            "code": 3,
            "required_present": [],
            "required_empty": ["conflict-set-refs"],
            "required_absent": [
                "candidate-id",
                "successor-input-id",
                "canonical-unit-id",
            ],
        },
        {
            "code": 4,
            "required_present": ["canonical-unit-id"],
            "required_empty": ["conflict-set-refs"],
            "required_absent": ["candidate-id", "successor-input-id"],
        },
    ]

    cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "record_conditional_constraint"
        and case["record"] == "decision-record"
    ]
    assert {case["disposition_code"] for case in cases} == set(range(5))
    for case in cases:
        assert case["valid_payload_cbor_hex"]
        assert case["invalid_payload_cbor_hex"]
        assert case["invalid_error_code"] == "RECORD_CONSTRAINT"


def test_attempt_evidence_and_rng_have_total_reference_contracts() -> None:
    cddl = (BUNDLE / "foundation.cddl").read_text(encoding="utf-8")
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    identities = {
        entry["reference_kind"]: entry for entry in registries["reference_identities"]
    }

    assert "rng-draw-fields = (" in cddl
    assert "provisional-disposition-fields = (" in cddl
    assert "attempt-failure-fields = (" in cddl
    assert "affordance-assessment-id-preimage = [" in cddl

    assert set(identities) >= {
        "rng_draw",
        "affordance_assessment",
        "provisional_disposition",
        "attempt_failure",
    }
    assert identities["rng_draw"]["persisted_root"] == "cote.csf.schema.rng-draw"
    assert identities["affordance_assessment"]["persisted_root"] == (
        "cote.csf.schema.affordance-assessment"
    )
    assert identities["provisional_disposition"]["persisted_root"] == (
        "cote.csf.schema.provisional-disposition"
    )
    assert identities["attempt_failure"]["persisted_root"] == (
        "cote.csf.schema.attempt-failure"
    )

    operations = {operation[0] for operation in registries["domain_operations"]}
    covered_operations = {
        case["domain_tag"]
        for case in fixtures["positive"]
        if case["case_id"].startswith("operation.")
    }
    assert covered_operations == operations

    reference_cases = {
        case["persisted_root"]: case
        for case in fixtures["semantic"]
        if case["kind"] == "record_to_reference"
    }
    for reference_kind in (
        "rng_draw",
        "affordance_assessment",
        "provisional_disposition",
        "attempt_failure",
    ):
        identity = identities[reference_kind]
        case = reference_cases[identity["persisted_root"]]
        derived_id = hashlib.sha256(
            bytes.fromhex(case["id_preimage_envelope_hex"])
        ).digest()
        derived_digest = hashlib.sha256(
            bytes.fromhex(case["digest_preimage_envelope_hex"])
        ).digest()
        expected_ref = (
            b"\x83\x18"
            + bytes([identity["code"]])
            + b"\x58\x20"
            + derived_id
            + b"\x58\x20"
            + derived_digest
        )
        assert bytes.fromhex(case["expected_causal_ref_cbor_hex"]) == expected_ref


def test_standalone_proposition_binds_polarity() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    bindings = _enum_bindings(registries)

    assert bindings["proposition.polarity"] == "polarity"
    case = next(
        item
        for item in fixtures["semantic"]
        if item["case_id"] == "enum-binding.proposition.polarity"
    )
    assert case["field_path"] == "proposition.polarity"
    assert case["registry"] == "polarity"
    assert case["accepted_code"] == 0
    assert case["rejected_code"] == 3
    assert case["rejected_error_code"] == "UNKNOWN_ENUM"


def test_fragmentary_negative_vectors_name_their_field_path() -> None:
    fixtures = _json("fixtures.json")
    cases = {case["case_id"]: case for case in fixtures["negative"]}

    assert cases["reject.set-exact-duplicate"]["field_path"] == (
        "conflict-set.candidate-ids"
    )
    assert cases["reject.set-identity-collision"]["field_path"] == (
        "event.source-inputs"
    )
    assert "field_path supplies a set-like field fragment" in fixtures["rules"][
        "negative"
    ]


def test_text_classification_and_specific_grammar_probes_are_exhaustive() -> None:
    contract = (BUNDLE.parent / "canonical-codec-v1.md").read_text(encoding="utf-8")
    readme = (BUNDLE / "README.md").read_text(encoding="utf-8")
    spec = (
        BUNDLE.parents[1] / "spec" / "causal-simulation-foundation-v1.md"
    ).read_text(encoding="utf-8")
    fixtures = _json("fixtures.json")
    cases = {case["case_id"]: case for case in fixtures["negative"]}

    for document in (contract, readme, spec):
        assert "`domain-tag`/`schema-id`" in document
        assert "`iana-timezone`" in document

    assert cases["reject.domain-schema-tag-grammar"]["error_code"] == (
        "DOMAIN_SCHEMA_TAG_GRAMMAR"
    )
    assert cases["reject.iana-timezone-grammar"]["error_code"] == (
        "IANA_TIMEZONE_GRAMMAR"
    )


def test_published_conformance_case_counts_match_the_bundle() -> None:
    fixtures = _json("fixtures.json")
    contract = (BUNDLE.parent / "canonical-codec-v1.md").read_text(encoding="utf-8")
    readme = (BUNDLE / "README.md").read_text(encoding="utf-8")

    assert len(fixtures["positive"]) == 152
    assert len(fixtures["negative"]) == 30
    assert len(fixtures["semantic"]) == 166
    assert "152 casos positivos" in contract
    assert "30 casos negativos" in contract
    assert "166 casos semânticos" in contract
    assert "152 vetores positivos, 30 negativos, 166 casos semânticos" in readme
