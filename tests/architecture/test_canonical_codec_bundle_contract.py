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


def _decode_cbor_item(encoded: bytes, offset: int = 0):
    """Decode the small, definite-length CBOR subset used by these fixtures."""
    initial = encoded[offset]
    major, additional = initial >> 5, initial & 0x1F
    offset += 1
    if additional < 24:
        argument = additional
    elif additional == 24:
        argument, offset = encoded[offset], offset + 1
    elif additional == 25:
        argument, offset = int.from_bytes(encoded[offset : offset + 2], "big"), offset + 2
    elif additional == 26:
        argument, offset = int.from_bytes(encoded[offset : offset + 4], "big"), offset + 4
    elif additional == 27:
        argument, offset = int.from_bytes(encoded[offset : offset + 8], "big"), offset + 8
    else:
        raise AssertionError("fixtures must use definite-length canonical CBOR")

    if major == 0:
        return argument, offset
    if major == 1:
        return -1 - argument, offset
    if major in {2, 3}:
        end = offset + argument
        value = encoded[offset:end]
        return (value if major == 2 else value.decode("utf-8")), end
    if major == 4:
        values = []
        for _ in range(argument):
            value, offset = _decode_cbor_item(encoded, offset)
            values.append(value)
        return values, offset
    if major == 7 and argument in {20, 21, 22}:
        return {20: False, 21: True, 22: None}[argument], offset
    raise AssertionError(f"unsupported fixture CBOR major type {major}")


def _decode_cbor_hex(encoded_hex: str):
    encoded = bytes.fromhex(encoded_hex)
    value, offset = _decode_cbor_item(encoded)
    assert offset == len(encoded)
    return value


def _encode_cbor_item(value) -> bytes:
    """Encode the deterministic CBOR subset needed by semantic fixtures."""

    def head(major: int, argument: int) -> bytes:
        if argument < 24:
            return bytes([(major << 5) | argument])
        if argument <= 0xFF:
            return bytes([(major << 5) | 24, argument])
        if argument <= 0xFFFF:
            return bytes([(major << 5) | 25]) + argument.to_bytes(2, "big")
        if argument <= 0xFFFFFFFF:
            return bytes([(major << 5) | 26]) + argument.to_bytes(4, "big")
        return bytes([(major << 5) | 27]) + argument.to_bytes(8, "big")

    if value is None:
        return b"\xf6"
    if value is False:
        return b"\xf4"
    if value is True:
        return b"\xf5"
    if isinstance(value, int):
        return head(0, value) if value >= 0 else head(1, -1 - value)
    if isinstance(value, bytes):
        return head(2, len(value)) + value
    if isinstance(value, str):
        encoded = value.encode("utf-8")
        return head(3, len(encoded)) + encoded
    if isinstance(value, list):
        return head(4, len(value)) + b"".join(_encode_cbor_item(item) for item in value)
    raise AssertionError(f"unsupported semantic fixture value {type(value)!r}")


def _values_at_index_path(value, path: tuple[int | None, ...]):
    if not path:
        return [value]
    index, *tail = path
    if index is None:
        return [
            nested
            for item in value
            for nested in _values_at_index_path(item, tuple(tail))
        ]
    if not isinstance(value, list) or index >= len(value):
        return []
    return _values_at_index_path(value[index], tuple(tail))


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


def test_every_positive_root_obeys_its_narrow_reference_kind_bindings() -> None:
    fixtures = _json("fixtures.json")
    reference_index_paths = {
        "closure-proof-item.closure-ref.kind": (9, None, 1, 0),
        "cohort-slot.response-ref.kind": (10, 1, None, 1, None, 1, 0),
        "admission-fence.retry-ref.kind": (3, 1, 0),
        "attempt-retry.aborted-envelope-ref.kind": (4, 0),
        "affordance-assessment.subject.kind": (0, 0),
        "decision-record.conflict-set-refs[*].kind": (8, None, 0),
        "cycle-control-state.fence-ref.kind": (3, 1, 0),
        "cycle-control-state.retry-ref.kind": (4, 1, 0),
        "cycle-control-state.last-terminal-envelope-ref.kind": (5, 1, 0),
        "claim.prior-claim-refs[*].kind": (7, None, 0),
        "observation.claim-refs[*].kind": (10, None, 0),
        "knowledge-input.observation-ref.kind": (7, 0),
        "knowledge-input.claim-refs[*].kind": (8, None, 0),
        "transmission.content.claims[*].kind": (4, 1, None, 0),
        "cycle-abort.failure-evidence-refs[*].ref-kind": (12, None, 0),
        "decision-record.rng-draw-refs[*].kind": (11, None, 0),
        "provisional-disposition.conflict-set-refs[*].kind": (8, None, 0),
        "provisional-disposition.rng-draw-refs[*].kind": (9, None, 0),
    }
    roots = {
        case["case_id"]: _decode_cbor_hex(case["payload_cbor_hex"])
        for case in fixtures["positive"]
        if case["case_id"].startswith("root.")
    }
    cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "record_reference_kind_binding"
    ]
    assert {case["field_path"] for case in cases} == reference_index_paths.keys()
    for case in cases:
        codes = _values_at_index_path(
            roots[case["record_template_case_id"]],
            reference_index_paths[case["field_path"]],
        )
        assert set(codes) <= set(case["accepted_codes"]), case["field_path"]

    persisted = {
        case["persisted_root"]: (
            _decode_cbor_hex(case["record_envelope_hex"])[5],
            _decode_cbor_hex(case["digest_preimage_envelope_hex"])[5],
        )
        for case in fixtures["semantic"]
        if case["kind"] == "record_to_reference"
    }
    assert persisted["cote.csf.schema.affordance-assessment"][0][0][0] == 5
    assert persisted["cote.csf.schema.affordance-assessment"][1][0][0] == 5
    assert persisted["cote.csf.schema.attempt-retry"][0][4][0] == 13
    assert persisted["cote.csf.schema.attempt-retry"][1][4][0] == 13
    assert persisted["cote.csf.schema.transmission"][0][4][1][0][0] == 18
    assert persisted["cote.csf.schema.transmission"][1][4][1][0][0] == 18
    assert persisted["cote.csf.schema.knowledge-input"][0][7][0] == 16
    assert persisted["cote.csf.schema.knowledge-input"][1][7][0] == 16


def test_typed_value_inner_envelope_constraint_is_hashed_and_executable() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")

    assert registries["typed_value_constraints"] == [
        {
            "constraint_id": "typed-value-inner-envelope",
            "applies_on": "strict-decode every typed-value",
            "inner_envelope": {
                "magic_hex": "43534600",
                "codec_version": 1,
                "array_items": 6,
            },
            "relations": [
                {
                    "left": "typed-value.schema-id",
                    "operator": "equals",
                    "right": "inner-envelope.schema-id",
                },
                {
                    "left": "typed-value.schema-version",
                    "operator": "equals",
                    "right": "inner-envelope.schema-version",
                },
            ],
            "domain_authorization": {
                "operation": [
                    "inner-envelope.domain-tag",
                    "inner-envelope.schema-id",
                    "inner-envelope.schema-version",
                ],
                "registries": [
                    "domain_operations",
                    "genesis-pinned extension domain_operations",
                ],
            },
            "digest": {
                "algorithm": "sha-256",
                "input": "typed-value.canonical-envelope exact bytes",
                "expected": "typed-value.envelope-digest",
            },
            "canonicality": "strict-decode and byte-for-byte deterministic re-encode",
        }
    ]

    cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "typed_value_constraint"
    ]
    assert len(cases) == 6
    assert {case["expected"] for case in cases} == {"accept", "reject"}

    for case in cases:
        schema_id, schema_version, inner_bytes, envelope_digest = _decode_cbor_hex(
            case["typed_value_cbor_hex"]
        )
        inner = _decode_cbor_hex(inner_bytes.hex())
        errors = []
        if schema_id != inner[3]:
            errors.append("TYPED_VALUE_SCHEMA_MISMATCH")
        if schema_version != inner[4]:
            errors.append("TYPED_VALUE_VERSION_MISMATCH")
        if [inner[2], inner[3], inner[4]] not in case["authorized_operations"]:
            errors.append("TYPED_VALUE_DOMAIN_UNAUTHORIZED")
        if hashlib.sha256(inner_bytes).digest() != envelope_digest:
            errors.append("TYPED_VALUE_DIGEST_MISMATCH")
        if _encode_cbor_item(inner) != inner_bytes:
            errors.append("TYPED_VALUE_INNER_NON_CANONICAL")

        if case["expected"] == "accept":
            assert errors == [], case["case_id"]
        else:
            assert errors == [case["invalid_error_code"]], case["case_id"]


def test_retry_constraints_are_scoped_to_durable_append_transitions() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    constraints = {
        item["record"]: item for item in registries["record_constraints"]
    }

    assert constraints["admission-fence"] == {
        "record": "admission-fence",
        "discriminant": "admission-fence.attempt-ordinal",
        "variants": [
            {"value": 1, "required_absent": ["retry-ref"]},
            {"exclusive_minimum": 1, "required_present": ["retry-ref"]},
        ],
    }
    assert constraints["attempt-retry"] == {
        "record": "attempt-retry",
        "relations": ["to-attempt-ordinal == from-attempt-ordinal + 1"],
    }

    constraints_by_id = {
        item["constraint_id"]: item
        for item in registries["linked_record_constraints"]
    }
    retry_ids = {
        "append-cycle-abort",
        "append-attempt-retry",
        "append-retry-fence",
    }
    assert retry_ids <= constraints_by_id.keys()
    assert constraints_by_id["append-cycle-abort"]["applies_on"] == {
        "operation": "append",
        "candidate_role": "abort",
        "root": "cycle-abort",
        "cursor": "decision-ledger pre-append prefix",
    }
    assert constraints_by_id["append-attempt-retry"]["applies_on"] == {
        "operation": "append",
        "candidate_role": "retry",
        "root": "attempt-retry",
        "cursor": "decision-ledger pre-append prefix",
    }
    retry_fence = constraints_by_id["append-retry-fence"]
    assert retry_fence["applies_on"] == {
        "operation": "append",
        "candidate_role": "retry-fence",
        "root": "admission-fence",
        "when": "attempt-ordinal > 1",
        "cursor": "fence and decision ledgers pre-append prefix",
    }
    assert (
        "retry.rule-versions == retry-fence.rule-versions after canonical set normalization"
        in retry_fence["relations"]
    )
    assert all(
        "at pre-append cursor" in relation
        for relation in constraints_by_id["append-attempt-retry"]["relations"]
        if "latest terminal" in relation
    )

    cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "linked_record_constraint"
        and case["constraint_id"] in retry_ids
        and case["expected"] == "reject"
    ]
    assert {case["violation"] for case in cases} == {
        "first_attempt_has_retry_ref",
        "later_attempt_missing_retry_ref",
        "non_successor_attempt",
        "run_id_mismatch",
        "cycle_id_mismatch",
        "from_attempt_mismatch",
        "to_attempt_mismatch",
        "aborted_envelope_ref_mismatch",
        "retry_ref_mismatch",
        "not_latest_abort",
        "abort_fence_digest_mismatch",
        "abort_input_digest_mismatch",
        "abort_cycle_plan_mismatch",
        "retry_instant_mismatch",
        "retry_cycle_ordinal_mismatch",
        "retry_cycle_plan_mismatch",
        "retry_base_revision_mismatch",
        "retry_base_state_hash_mismatch",
        "retry_closure_proof_mismatch",
        "retry_cohort_mismatch",
        "retry_admitted_units_mismatch",
        "retry_input_digest_mismatch",
        "retry_immutable_policy_mismatch",
        "retry_rule_versions_mismatch",
        "later_terminal_other_cycle",
    }
    assert all(case["expected"] == "reject" for case in cases)
    assert all(isinstance(case["mutation"], dict) for case in cases)
    assert all(case["invalid_error_code"] == "LINKED_RECORD_CONSTRAINT" for case in cases)

    accepted = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "linked_record_constraint"
        and case["constraint_id"] in retry_ids
        and case["expected"] == "accept"
    ]
    assert {case["constraint_id"] for case in accepted} == retry_ids


def test_epistemic_constraints_are_scoped_to_each_append_boundary() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    constraints_by_id = {
        item["constraint_id"]: item
        for item in registries["linked_record_constraints"]
    }
    epistemic_ids = {
        "append-perception-task",
        "append-observation",
        "append-knowledge-input",
        "append-perception-completion",
    }
    assert epistemic_ids <= constraints_by_id.keys()
    assert {
        constraint_id: constraints_by_id[constraint_id]["applies_on"]["root"]
        for constraint_id in epistemic_ids
    } == {
        "append-perception-task": "perception-task",
        "append-observation": "observation",
        "append-knowledge-input": "knowledge-input",
        "append-perception-completion": "perception-task-completion",
    }
    assert all(
        constraints_by_id[constraint_id]["applies_on"]["operation"] == "append"
        for constraint_id in epistemic_ids
    )
    completion = constraints_by_id["append-perception-completion"]
    assert any(
        role["role"] == "observations"
        and role["source"] == "all evidence-ledger observations for task.task-id at pre-append cursor"
        for role in completion["record_roles"]
    )
    assert any(
        role["role"] == "knowledge-inputs"
        and role["source"] == "all evidence-ledger knowledge-inputs for task.task-id at pre-append cursor"
        for role in completion["record_roles"]
    )

    cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "linked_record_constraint"
        and case["constraint_id"] in epistemic_ids
        and case["expected"] == "reject"
    ]
    assert {case["violation"] for case in cases} == {
        "task_event_id_mismatch",
        "observation_task_id_mismatch",
        "source_event_missing",
        "observed_at_mismatch",
        "observation_ref_mismatch",
        "recipient_mismatch",
        "received_at_mismatch",
        "undisclosed_claim",
        "completion_task_id_mismatch",
        "completion_observation_missing",
        "completion_knowledge_input_missing",
        "source_event_extra",
        "undisclosed_evidence",
        "completion_foreign_observation",
        "completion_foreign_knowledge_input",
        "completion_duplicate_observation",
        "completion_duplicate_knowledge_input",
    }
    assert all(case["expected"] == "reject" for case in cases)
    assert all(isinstance(case["mutation"], dict) for case in cases)
    assert all(case["invalid_error_code"] == "LINKED_RECORD_CONSTRAINT" for case in cases)

    accepted = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "linked_record_constraint"
        and case["constraint_id"] in epistemic_ids
        and case["expected"] == "accept"
    ]
    assert {case["constraint_id"] for case in accepted} == epistemic_ids


def test_linked_semantic_cases_are_self_contained_and_all_case_refs_resolve() -> None:
    fixtures = _json("fixtures.json")
    registries = _json("registries.json")
    constraints = {
        item["constraint_id"] for item in registries["linked_record_constraints"]
    }
    scenarios = {
        scenario["scenario_id"]: scenario
        for scenario in fixtures["linked_record_scenarios"]
    }
    assert len(scenarios) == len(fixtures["linked_record_scenarios"])

    for scenario in scenarios.values():
        assert scenario["records"]
        for record in scenario["records"]:
            assert _decode_cbor_hex(record["record_cbor_hex"])
            assert record["schema_id"].startswith("cote.csf.schema.")

    linked_cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "linked_record_constraint"
    ]
    for case in linked_cases:
        assert case["constraint_id"] in constraints
        assert case["base_scenario_id"] in scenarios
        if case["expected"] == "reject":
            mutation = case["mutation"]
            assert mutation["op"] in {"replace_record", "replace_ledger_field"}
            if mutation["op"] == "replace_record":
                assert _decode_cbor_hex(mutation["record_cbor_hex"])
            assert case["invalid_error_code"] == "LINKED_RECORD_CONSTRAINT"
        else:
            assert "mutation" not in case

    all_case_ids = {
        case["case_id"]
        for value in fixtures.values()
        if isinstance(value, list)
        for case in value
        if isinstance(case, dict) and "case_id" in case
    }
    for value in fixtures.values():
        if not isinstance(value, list):
            continue
        for case in value:
            if not isinstance(case, dict):
                continue
            for field, referenced in case.items():
                if field.endswith("_case_id") and field != "case_id":
                    assert referenced in all_case_ids, (case.get("case_id"), field)
                if field.endswith("_case_ids"):
                    assert set(referenced) <= all_case_ids, (case.get("case_id"), field)


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
    assert len(fixtures["semantic"]) == 245
    assert len(fixtures["normalization_cases"]) == 7
    assert "152 casos positivos" in contract
    assert "30 casos negativos" in contract
    assert "245 casos semânticos" in contract
    assert "152 vetores positivos, 30 negativos, 245 casos semânticos" in readme


def test_cycle_control_state_variants_are_closed_by_status() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    constraints = {
        item["record"]: item for item in registries["record_constraints"]
    }

    constraint = constraints["cycle-control-state"]
    assert constraint["discriminant"] == "cycle-control-state.status"
    assert constraint["variants"] == [
        {
            "code": 0,
            "required_present": [],
            "required_absent": [
                "cycle-id",
                "attempt-ordinal",
                "fence-ref",
                "retry-ref",
            ],
            "reference_kind_constraints": [
                {
                    "field": "last-terminal-envelope-ref",
                    "when_present": "cycle_commit_reference_kind",
                }
            ],
            "relations": ["next-attempt-ordinal == 1"],
        },
        {
            "code": 1,
            "required_present": ["cycle-id", "attempt-ordinal", "fence-ref"],
            "required_absent": ["retry-ref"],
            "reference_kind_constraints": [],
            "relations": ["next-attempt-ordinal == attempt-ordinal + 1"],
        },
        {
            "code": 2,
            "required_present": [
                "cycle-id",
                "attempt-ordinal",
                "retry-ref",
                "last-terminal-envelope-ref",
            ],
            "required_absent": ["fence-ref"],
            "reference_kind_constraints": [
                {
                    "field": "last-terminal-envelope-ref",
                    "when_present": "cycle_abort_reference_kind",
                }
            ],
            "relations": ["next-attempt-ordinal == attempt-ordinal"],
        },
        {
            "code": 3,
            "required_present": [
                "cycle-id",
                "attempt-ordinal",
                "last-terminal-envelope-ref",
            ],
            "required_absent": ["fence-ref", "retry-ref"],
            "reference_kind_constraints": [
                {
                    "field": "last-terminal-envelope-ref",
                    "when_present": "cycle_abort_reference_kind",
                }
            ],
            "relations": ["next-attempt-ordinal == attempt-ordinal + 1"],
        },
    ]

    cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "record_conditional_constraint"
        and case["record"] == "cycle-control-state"
    ]
    assert {case["status_code"] for case in cases} == set(range(4))
    assert all(case["valid_payload_cbor_hex"] for case in cases)
    assert all(case["invalid_payload_cbor_hex"] for case in cases)
    assert all(case["invalid_error_code"] == "RECORD_CONSTRAINT" for case in cases)


def test_repeat_while_true_requires_a_positive_explicit_cadence() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    constraints = {
        item["record"]: item for item in registries["record_constraints"]
    }

    constraint = constraints["trigger-definition"]
    assert constraint["discriminant"] == "trigger-definition.activation-policy"
    assert constraint["variants"] == [
        {"codes": [0, 1, 2], "required_absent": ["repeat-every"]},
        {
            "code": 3,
            "required_present": ["repeat-every"],
            "field_ranges": [
                {"field": "repeat-every", "exclusive_minimum": 0}
            ],
        },
    ]

    cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "record_conditional_constraint"
        and case["record"] == "trigger-definition"
    ]
    assert {case["violation"] for case in cases} == {
        "non_repeat_present",
        "repeat_absent",
        "repeat_zero",
        "repeat_negative",
    }
    assert all(case["invalid_error_code"] == "RECORD_CONSTRAINT" for case in cases)


def test_attempt_failure_identity_is_stable_under_worker_permutation() -> None:
    cddl = (BUNDLE / "foundation.cddl").read_text(encoding="utf-8")
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    bindings = _enum_bindings(registries)
    identity = next(
        item
        for item in registries["reference_identities"]
        if item["reference_kind"] == "attempt_failure"
    )

    assert (
        "attempt-failure-id-preimage = [run-id: id32, cycle-id: id32, "
        "attempt-ordinal: u32,\n                               stage: u8, "
        "component: policy-ref,\n                               "
        "failure-local-ordinal: u32]"
        in cddl
    )
    assert identity["id"]["components"] == [
        "attempt-failure.run-id",
        "attempt-failure.cycle-id",
        "attempt-failure.attempt-ordinal",
        "attempt-failure.stage",
        "attempt-failure.component",
        "attempt-failure.failure-local-ordinal",
    ]
    assert bindings["attempt-failure-id-preimage.stage"] == "attempt_failure_stage"

    case = next(
        item
        for item in fixtures["normalization_cases"]
        if item["case_id"] == "normalize.attempt-failure.worker-order"
    )
    failures = {item["semantic_key"]: item for item in case["attempt_failures"]}
    assert len(failures) == 2
    assert case["worker_orders"][0] == list(reversed(case["worker_orders"][1]))
    assert case["component_policy_order"] == [
        {"semantic_key": "admission", "failure-local-ordinal": 0},
        {"semantic_key": "validation", "failure-local-ordinal": 1},
    ]

    normalized_by_order = []
    for worker_order in case["worker_orders"]:
        refs = []
        for semantic_key in worker_order:
            failure = failures[semantic_key]
            record_envelope = _decode_cbor_hex(failure["record_envelope_hex"])
            record = record_envelope[5]
            id_preimage_envelope = _decode_cbor_hex(
                failure["id_preimage_envelope_hex"]
            )
            digest_preimage_envelope = _decode_cbor_hex(
                failure["digest_preimage_envelope_hex"]
            )
            assert failure["failure_local_ordinal"] == next(
                item["failure-local-ordinal"]
                for item in case["component_policy_order"]
                if item["semantic_key"] == semantic_key
            )
            assert record[4] == failure["failure_local_ordinal"]
            assert id_preimage_envelope[5] == [
                record[1],
                record[2],
                record[3],
                record[5],
                record[6],
                record[4],
            ]
            assert digest_preimage_envelope[5] == record[:-1]
            failure_id = hashlib.sha256(
                bytes.fromhex(failure["id_preimage_envelope_hex"])
            ).digest()
            failure_digest = hashlib.sha256(
                bytes.fromhex(failure["digest_preimage_envelope_hex"])
            ).digest()
            assert record[0] == failure_id
            assert record[-1] == failure_digest
            expected_ref = b"\x83\x18\x1b\x58\x20" + failure_id + b"\x58\x20" + failure_digest
            assert bytes.fromhex(failure["expected_causal_ref_cbor_hex"]) == expected_ref
            refs.append(expected_ref)
        normalized_by_order.append(sorted(refs, key=lambda ref: (ref[2], ref[5:37], ref[39:71])))

    expected_refs = [
        bytes.fromhex(item) for item in case["expected_normalized_refs_cbor_hex"]
    ]
    assert normalized_by_order == [expected_refs, expected_refs]
    assert case["recompute"] == ["abort-digest"]
    assert case["expected_payload_cbor_hex"]
    assert hashlib.sha256(
        bytes.fromhex(case["abort_digest_preimage_envelope_hex"])
    ).hexdigest() == case["expected_abort_digest"]


def test_every_reference_identity_has_a_linked_record_to_reference_case() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")

    expected = {
        entry["persisted_root"] for entry in registries["reference_identities"]
    }
    cases = {
        case["persisted_root"]: case
        for case in fixtures["semantic"]
        if case["kind"] == "record_to_reference"
    }

    assert cases.keys() == expected
    assert len(cases) == 28
    for root, case in cases.items():
        identity = next(
            entry
            for entry in registries["reference_identities"]
            if entry["persisted_root"] == root
        )
        assert case["id_operation"] == identity["id"]["operation"]
        assert case["digest_operation"] == identity["digest"]["operation"]
        derived_id = hashlib.sha256(
            bytes.fromhex(case["id_preimage_envelope_hex"])
        ).digest()
        derived_digest = hashlib.sha256(
            bytes.fromhex(case["digest_preimage_envelope_hex"])
        ).digest()
        expected_ref = bytes.fromhex(case["expected_causal_ref_cbor_hex"])
        encoded_code = (
            bytes([identity["code"]])
            if identity["code"] < 24
            else b"\x18" + bytes([identity["code"]])
        )
        assert expected_ref == (
            b"\x83" + encoded_code + b"\x58\x20" + derived_id
            + b"\x58\x20" + derived_digest
        )
