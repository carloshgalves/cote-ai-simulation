"""Structural regression checks for the language-neutral codec bundle.

These tests lint the normative metadata.  They are deliberately not a Python
implementation of the causal codec or Simulation Engine.
"""

from __future__ import annotations

import hashlib
import json
import re
from copy import deepcopy
from pathlib import Path


BUNDLE = (
    Path(__file__).parents[2]
    / "docs"
    / "architecture"
    / "canonical-codec-v1-bundle"
)


def _strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON object name: {key}")
        result[key] = value
    return result


def _json(name: str):
    return json.loads(
        (BUNDLE / name).read_text(encoding="utf-8"),
        object_pairs_hook=_strict_object,
    )


def test_bundle_json_objects_have_unique_member_names() -> None:
    for path in sorted(BUNDLE.glob("*.json")):
        json.loads(path.read_text(encoding="utf-8"), object_pairs_hook=_strict_object)


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


def _domain_digest(domain_tag: str, schema_id: str, value) -> bytes:
    return hashlib.sha256(
        _encode_cbor_item([b"CSF\x00", 1, domain_tag, schema_id, 1, value])
    ).digest()


def _event_envelope(event) -> bytes:
    return _encode_cbor_item(
        [
            b"CSF\x00",
            1,
            "cote.csf.test.event",
            "cote.csf.schema.event",
            1,
            event,
        ]
    )


def _batch_digest(commit, events) -> bytes:
    return _domain_digest(
        "cote.csf.digest.batch",
        "cote.csf.schema.batch-preimage",
        [commit[:-1], [_event_envelope(event) for event in events]],
    )


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
        "eligibility",
        "unit-kind",
        "unit-id",
        "source-id",
        "unit-digest",
    }
    authoritative_records = {
        case["case_id"]: case
        for case in fixtures["semantic"]
        if case["kind"] == "record_to_reference"
    }
    for case in mismatch_cases:
        authoritative = authoritative_records[case["referenced_record_case_id"]]
        record = _decode_cbor_hex(authoritative["record_envelope_hex"])[5]
        expected_ref = _decode_cbor_hex(authoritative["expected_input_ref_cbor_hex"])
        projection = _decode_cbor_hex(case["authoritative_admitted_unit_cbor_hex"])
        assert projection == [record[9], 1, record[2], expected_ref[-2], expected_ref[-1]]

        mutated = list(projection)
        field = case["mutate"][0]
        index = {
            "eligibility": 0,
            "unit-kind": 1,
            "source-id": 2,
            "unit-id": 3,
            "unit-digest": 4,
        }[field]
        replacement = case["mutate"][1]
        if replacement == "flip-low-bit":
            value = bytearray(mutated[index])
            value[-1] ^= 1
            mutated[index] = bytes(value)
        else:
            mutated[index] = replacement
        assert mutated != projection


def test_genesis_explicitly_pins_profile_and_schema_bundle_without_hash_cycle() -> None:
    cddl = (BUNDLE / "foundation.cddl").read_text(encoding="utf-8")
    fixtures = _json("fixtures.json")
    policy_manifest = _json("policy-manifest.json")
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
        "causal-transition-contracts.json",
    }
    assert {entry[0] for entry in conformance_manifest["artifacts"]} == {
        "fixtures.json",
        "causal-transition-fixtures.json",
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
    genesis = _decode_cbor_hex(pinning["genesis_envelope_hex"])
    assert genesis[5][11] == bytes.fromhex(schema_bundle_hash)

    for manifest in (policy_manifest, schema_manifest, conformance_manifest):
        for name, size, expected_sha256 in manifest["artifacts"]:
            artifact = (BUNDLE / name).read_bytes()
            assert len(artifact) == size
            assert hashlib.sha256(artifact).hexdigest() == expected_sha256

    published_hashes = {
        parts[0]: parts[1]
        for line in (BUNDLE / "BUNDLE.sha256").read_text(encoding="utf-8").splitlines()
        if len(parts := line.split()) == 2
    }
    codec_policy_hash = hashlib.sha256(
        b"cote.csf.bundle.codec-policy.v1\x00"
        + (BUNDLE / "policy-manifest.json").read_bytes()
    ).hexdigest()
    assert pinning["codec_policy_hash"] == codec_policy_hash
    assert published_hashes["codec_policy_hash"] == codec_policy_hash
    assert published_hashes["schema_bundle_hash"] == schema_bundle_hash
    assert published_hashes["conformance_suite_hash"] == hashlib.sha256(
        b"cote.csf.bundle.conformance.v1\x00"
        + (BUNDLE / "conformance-manifest.json").read_bytes()
    ).hexdigest()


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
        "cursor": "decision-ledger pre-append prefix",
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
        "abort_instant_mismatch",
        "abort_cycle_ordinal_mismatch",
        "abort_base_revision_mismatch",
        "abort_base_state_hash_mismatch",
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
    assert all(
        isinstance(case.get("mutation"), dict) or case.get("input_scenario_id")
        for case in cases
    )
    assert all(case["invalid_error_code"] == "LINKED_RECORD_CONSTRAINT" for case in cases)

    accepted = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "linked_record_constraint"
        and case["constraint_id"] in retry_ids
        and case["expected"] == "accept"
    ]
    assert {case["constraint_id"] for case in accepted} == retry_ids


def test_abort_immutable_coordinate_negatives_are_internally_hash_valid() -> None:
    fixtures = _json("fixtures.json")
    scenario = next(
        item
        for item in fixtures["linked_record_scenarios"]
        if item["scenario_id"] == "retry.abort.valid"
    )
    records = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in scenario["records"]
    }
    fence = records["aborted-fence"]
    cases = {
        case["violation"]: _decode_cbor_hex(case["mutation"]["record_cbor_hex"])
        for case in fixtures["semantic"]
        if case.get("violation")
        in {
            "abort_instant_mismatch",
            "abort_cycle_ordinal_mismatch",
            "abort_base_revision_mismatch",
            "abort_base_state_hash_mismatch",
        }
    }
    assert cases.keys() == {
        "abort_instant_mismatch",
        "abort_cycle_ordinal_mismatch",
        "abort_base_revision_mismatch",
        "abort_base_state_hash_mismatch",
    }
    immutable_pairs = ((3, 4), (4, 5), (6, 7), (7, 8))
    for abort in cases.values():
        assert abort[-1] == _domain_digest(
            "cote.csf.digest.cycle-abort",
            "cote.csf.schema.cycle-abort.body",
            abort[:-1],
        )
        assert any(abort[abort_index] != fence[fence_index] for abort_index, fence_index in immutable_pairs)


def test_abort_provenance_is_resolved_against_the_aborted_fence() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    constraint = next(
        item
        for item in registries["linked_record_constraints"]
        if item["constraint_id"] == "append-cycle-abort"
    )
    roles = {item["role"] for item in constraint["record_roles"]}
    assert {
        "commit-candidates",
        "conflict-sets",
        "rng-draws",
        "affordance-assessments",
        "provisional-dispositions",
        "attempt-failures",
    } <= roles
    assert {
        "every ADMITTED_UNIT indeterminate subject resolves one admitted unit in aborted-fence and repeats its unit-id and unit-digest",
        "every COMMIT_CANDIDATE indeterminate subject resolves one commit-candidate whose run-id and cycle-id match abort, whose source-unit-ids belong to the aborted-fence partition, and whose subject-digest equals candidate-digest",
        "every failure-evidence ref resolves its registered root; rng-draw, provisional-disposition, and attempt-failure match abort run-id, cycle-id, and attempt-ordinal",
        "every affordance-assessment subject resolves an admitted unit or commit-candidate of aborted-fence",
        "every conflict-set candidate-id resolves a commit-candidate in the aborted-fence partition",
    } <= set(constraint["relations"])

    cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "cycle_abort_provenance"
    ]
    assert {case["violation"] for case in cases} == {
        "admitted_unit_subject_foreign",
        "admitted_unit_subject_digest_mismatch",
        "commit_candidate_subject_foreign",
        "conflict_set_foreign_candidate",
        "rng_draw_foreign_attempt",
        "affordance_assessment_foreign_subject",
        "provisional_disposition_foreign_attempt",
        "attempt_failure_foreign_attempt",
    }
    assert all(case["base_scenario_id"] == "retry.abort.valid" for case in cases)
    assert all(case["mutation"] for case in cases)


def test_epistemic_constraints_are_scoped_to_each_append_boundary() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    constraints_by_id = {
        item["constraint_id"]: item
        for item in registries["linked_record_constraints"]
    }
    epistemic_ids = {
        "publish-cycle-commit-batch",
        "append-observation",
        "append-knowledge-input",
        "append-perception-completion",
    }
    assert epistemic_ids <= constraints_by_id.keys()
    assert {
        constraint_id: constraints_by_id[constraint_id]["applies_on"]["root"]
        for constraint_id in epistemic_ids
    } == {
        "publish-cycle-commit-batch": "cycle-commit",
        "append-observation": "observation",
        "append-knowledge-input": "knowledge-input",
        "append-perception-completion": "perception-task-completion",
    }
    atomic = constraints_by_id["publish-cycle-commit-batch"]
    assert atomic["applies_on"] == {
        "operation": "atomic-publish",
        "candidate_role": "commit-batch",
        "root": "cycle-commit",
        "cursor": "decision-ledger/event-store/source-authority/trigger-registry pre-publication prefixes plus unpublished transaction working set",
        "publication": "source settlement + decision settlement + successors + source lifecycle + events + trigger runtime/activations + cycle commit + tasks + world revision indivisible",
    }
    assert {role["role"] for role in atomic["record_roles"]} == {
        "fence",
        "commit-candidates",
        "decision-records",
        "cycle-commit",
        "events",
        "tasks",
    }
    assert {role["role"] for role in atomic["state_roles"]} == {
        "source-consumption",
        "world-state-transition",
        "logical-sequence-transition",
        "perception-classification",
    }
    required_relation_fragments = {
        "no CycleCommit or CycleAbortRecord exists",
        "immutable commit-candidate in the decision-ledger pre-publication prefix",
        "NO_PROPOSAL resolves a NoProposal subject",
        "terminal decision equals its resolved ProvisionalDisposition",
        "REJECT and DEFER produced-event-ids are empty",
        "DEFER publishes exactly one new successor",
        "canonical full-envelope projection",
        "event-id is recomputed",
        "SOURCE_LIFECYCLE event is the exact policy-derived projection",
        "same-cycle parent is earlier within the same candidate",
        "authority-specific proof for every admitted",
        "policy refs equal the corresponding AdmissionFence.rule-versions/genesis pins",
        "exact TriggerRuntimeState successors and TriggerActivations",
        "publish all-or-none",
    }
    relations = "\n".join(atomic["relations"])
    assert all(fragment in relations for fragment in required_relation_fragments)
    assert registries["perception_policy_contracts"] == [
        {
            "policy_id": "perception-visible-first-v1",
            "version": 1,
            "policy_hash_hex": hashlib.sha256(
                b"perception-visible-first-v1"
            ).hexdigest(),
            "input": "full canonical event record and event digest",
            "predicate": "event.event-order-key.event-local-ordinal == 0",
            "output": "potentially-perceptible boolean",
            "ordering": "evaluate events in cycle-commit.event-ids order",
            "observer_projection_input": "PerceptionTask + full Event + base/result state loaded by task revisions + authenticated channel/access authorities; no supplied allow-list/output",
            "observer_access": "derive the exact observer allow-list from channel membership and actor clearance; confidentiality never grants fallback access",
            "observer_output": "derive exact modality/channel from the channel authority and percepts, claim refs, evidence chain and omissions from the registered Event payload projection",
            "append_rule": "reject an unauthorized observer or any extra, omitted or substituted observer output",
        }
    ]
    assert all(
        constraints_by_id[constraint_id]["applies_on"]["operation"] == "append"
        for constraint_id in epistemic_ids - {"publish-cycle-commit-batch"}
    )
    completion = constraints_by_id["append-perception-completion"]
    assert any(
        role["role"] == "observations"
        and role["source"] == "all evidence_ledger observations for task.task-id at pre-append cursor"
        for role in completion["record_roles"]
    )
    assert any(
        role["role"] == "knowledge-inputs"
        and role["source"] == "all evidence_ledger knowledge-inputs for task.task-id at pre-append cursor"
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
        "published_event_without_task",
        "decision_record_missing",
        "decision_record_extra",
        "decision_record_foreign",
        "fence_consumption_missing",
        "source_state_missing",
        "source_already_consumed",
        "source_unit_foreign",
        "source_post_state_not_consumed",
        "world_base_hash_missing",
        "world_base_hash_mismatch",
        "world_result_hash_mismatch",
        "world_revision_not_successor",
        "world_post_state_missing",
        "commit_run_id_mismatch",
        "commit_cycle_id_mismatch",
        "commit_attempt_ordinal_mismatch",
        "commit_instant_mismatch",
        "commit_cycle_ordinal_mismatch",
        "commit_cycle_plan_mismatch",
        "commit_base_revision_mismatch",
        "commit_base_state_hash_mismatch",
        "decision_attempt_scope_mismatch",
        "event_coordinate_mismatch",
        "task_coordinate_mismatch",
        "event_sequence_permuted",
        "event_sequence_duplicate",
        "event_sequence_gap",
        "next_logical_sequence_ahead",
        "next_logical_sequence_behind",
        "empty_commit_advances_logical_sequence",
        "perceptible_event_task_omitted",
        "duplicate_task_for_event",
        "task_for_nonperceptible_event",
        "task_perception_identity_policy_mismatch",
        "observation_resolver_mismatch",
        "observation_after_completion",
        "knowledge_input_after_completion",
    }
    assert all(case["expected"] == "reject" for case in cases)
    assert all(
        isinstance(case.get("mutation"), dict) or case.get("input_scenario_id")
        for case in cases
    )
    assert all(case["invalid_error_code"] == "LINKED_RECORD_CONSTRAINT" for case in cases)

    accepted = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "linked_record_constraint"
        and case["constraint_id"] in epistemic_ids
        and case["expected"] == "accept"
    ]
    assert {case["constraint_id"] for case in accepted} == epistemic_ids


def test_record_authorities_exhaustively_cover_persisted_roots() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    authorities = {
        item["root"]: item["authority"]
        for item in registries["record_authorities"]
    }
    assert authorities == {
        "source-ingress-receipt": "input_ledger",
        "exogenous-input": "input_ledger",
        "source-closure": "input_ledger",
        "slot-dispatch": "input_ledger",
        "slot-dispatch-revocation": "input_ledger",
        "action-proposal": "input_ledger",
        "no-proposal": "input_ledger",
        "idempotency-identity": "input_ledger",
        "round-declaration": "schedule_store",
        "scheduled-occurrence": "schedule_store",
        "trigger-definition": "trigger_registry",
        "trigger-runtime-state": "trigger_registry",
        "trigger-activation": "trigger_registry",
        "transmission": "world_state",
        "admission-fence": "decision_ledger",
        "affordance-assessment": "decision_ledger",
        "commit-candidate": "decision_ledger",
        "conflict-set": "decision_ledger",
        "decision-record": "decision_ledger",
        "cycle-abort": "decision_ledger",
        "attempt-retry": "decision_ledger",
        "cycle-commit": "decision_ledger",
        "rng-draw": "decision_ledger",
        "provisional-disposition": "decision_ledger",
        "attempt-failure": "decision_ledger",
        "cycle-control-state": "decision_ledger",
        "event": "event_store",
        "claim": "evidence_ledger",
        "observation": "evidence_ledger",
        "knowledge-input": "evidence_ledger",
        "perception-task": "causal_outbox",
        "perception-task-completion": "causal_outbox",
        "snapshot": "snapshot_store",
        "epistemic-checkpoint-ref": "snapshot_store",
        "genesis-manifest": "genesis_store",
    }
    persisted_roots = {item["root"] for item in registries["persisted_roots"]}
    assert len(persisted_roots) == len(registries["persisted_roots"])
    assert authorities.keys() == persisted_roots
    assert {
        item["persisted_root"].removeprefix("cote.csf.schema.")
        for item in registries["reference_identities"]
    } <= persisted_roots
    assert {
        "slot-dispatch-revocation",
        "trigger-definition",
        "trigger-runtime-state",
        "cycle-control-state",
        "idempotency-identity",
    } <= persisted_roots
    assert "input_ledger" in authorities.values()

    forbidden_authorities = {"fence_ledger", "event_ledger"}
    for constraint in registries["linked_record_constraints"]:
        encoded = json.dumps(constraint)
        assert not any(authority in encoded for authority in forbidden_authorities)

    for scenario in fixtures["linked_record_scenarios"]:
        records = {record["role"]: record for record in scenario["records"]}
        for authority, roles in scenario.get("pre_append_cursor", {}).items():
            assert authority not in forbidden_authorities
            for role in roles:
                schema = records[role]["schema_id"].removeprefix("cote.csf.schema.")
                assert authorities[schema] == authority, (scenario["scenario_id"], role)
        for authority, roles in scenario.get("transaction_candidates", {}).items():
            for role in roles:
                schema = records[role]["schema_id"].removeprefix("cote.csf.schema.")
                assert authorities[schema] == authority, (scenario["scenario_id"], role)

    scenarios = {
        scenario["scenario_id"]: scenario
        for scenario in fixtures["linked_record_scenarios"]
    }
    atomic = scenarios["epistemic.commit-batch.valid"]
    assert atomic["pre_append_cursor"] == {
        "input_ledger": ["source-unit"],
        "decision_ledger": ["fence", "candidate"],
        "event_store": [],
        "causal_outbox": [],
    }
    assert atomic["transaction_candidates"] == {
        "decision_ledger": ["decision", "cycle-commit"],
        "event_store": ["event-0", "event-1"],
        "causal_outbox": ["task-0"],
    }
    atomic_records = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in atomic["records"]
    }
    commit = atomic_records["cycle-commit"]
    fence = atomic_records["fence"]
    candidate = atomic_records["candidate"]
    decision = atomic_records["decision"]
    source_unit = atomic_records["source-unit"]
    events = [atomic_records["event-0"], atomic_records["event-1"]]
    task = atomic_records["task-0"]
    witnesses = {
        witness["role"]: _decode_cbor_hex(witness["witness_cbor_hex"])
        for witness in atomic["transition_witnesses"]
    }
    assert {
        witness["role"]: witness["cddl_type"]
        for witness in atomic["transition_witnesses"]
    } == {
        "source-consumption": "source-unit-consumption-transition",
        "world-state-transition": "world-state-transition",
        "logical-sequence-transition": "logical-sequence-transition",
        "perception-classification": "perception-classification",
    }
    admitted_unit_ids = [unit[3] for unit in fence[11]]
    assert admitted_unit_ids
    expected_input_digest = hashlib.sha256(
        _encode_cbor_item(
            [
                b"CSF\x00",
                1,
                "cote.csf.digest.input",
                "cote.csf.schema.admitted-unit-list",
                1,
                fence[11],
            ]
        )
    ).digest()
    assert fence[12] == expected_input_digest
    expected_fence_digest = hashlib.sha256(
        _encode_cbor_item(
            [
                b"CSF\x00",
                1,
                "cote.csf.digest.fence",
                "cote.csf.schema.admission-fence.body",
                1,
                fence[:-1],
            ]
        )
    ).digest()
    assert fence[-1] == expected_fence_digest
    assert [commit[index] for index in (0, 1, 2)] == [
        fence[index] for index in (0, 1, 2)
    ]
    assert [commit[index] for index in (3, 4, 5, 6, 7)] == [
        fence[index] for index in (4, 5, 6, 7, 8)
    ]
    assert commit[10] == fence[-1]
    assert commit[11] == admitted_unit_ids
    assert commit[12] == fence[12]
    assert commit[16] == [decision[0]]
    assert decision[4][-2:] == [fence[11][0][3], fence[11][0][4]]
    expected_decision_digest = hashlib.sha256(
        _encode_cbor_item(
            [
                b"CSF\x00",
                1,
                "cote.csf.digest.decision-record",
                "cote.csf.schema.decision-record.body",
                1,
                decision[:-1],
            ]
        )
    ).digest()
    assert decision[-1] == expected_decision_digest
    assert decision[1:4] == fence[0:3]
    assert decision[6] == [1, candidate[0]]
    assert candidate[1:3] == fence[0:2]
    assert candidate[3] == admitted_unit_ids
    assert candidate[-1] == _domain_digest(
        "cote.csf.digest.commit-candidate",
        "cote.csf.schema.commit-candidate.body",
        candidate[:-1],
    )
    expected_settlement_digest = hashlib.sha256(
        _encode_cbor_item(
            [
                b"CSF\x00",
                1,
                "cote.csf.digest.decision",
                "cote.csf.schema.decision-pair-list",
                1,
                [[admitted_unit_ids[0], decision[0], decision[-1]]],
            ]
        )
    ).digest()
    assert commit[17] == expected_settlement_digest
    assert source_unit[0] == fence[11][0][3]
    assert source_unit[-1] == fence[11][0][4]
    source_transition = witnesses["source-consumption"]
    assert source_transition == [
        "cote.csf.schema.action-proposal",
        "input_ledger",
        source_unit[0],
        source_unit[-1],
        False,
        True,
        fence[-1],
    ]

    world_transition = witnesses["world-state-transition"]
    assert world_transition[0] == fence[7] == commit[6]
    assert world_transition[2] == fence[8] == commit[7]
    assert world_transition[3] == commit[8] == world_transition[0] + 1
    assert world_transition[5] == commit[9]
    for state_value, expected_hash in (
        (world_transition[1], world_transition[2]),
        (world_transition[4], world_transition[5]),
    ):
        assert expected_hash == hashlib.sha256(
            _encode_cbor_item(
                [
                    b"CSF\x00",
                    1,
                    "cote.csf.hash.world-state",
                    "cote.csf.schema.typed-value",
                    1,
                    state_value,
                ]
            )
        ).digest()

    assert commit[15] == [event[0] for event in events]
    assert events == sorted(events, key=lambda event: _encode_cbor_item(event[2]))
    assert [event[6] for event in events] == [0, 1]
    assert all(
        (event[1], event[7], event[5], event[8])
        == (commit[0], commit[1], commit[3], commit[6])
        for event in events
    )
    assert witnesses["logical-sequence-transition"] == [0, 2]
    assert commit[22] == 2

    perception = witnesses["perception-classification"]
    assert perception[0] == commit[20]
    policy_contract = registries["perception_policy_contracts"][0]
    assert commit[20] == [
        policy_contract["policy_id"],
        policy_contract["version"],
        bytes.fromhex(policy_contract["policy_hash_hex"]),
    ]
    assert perception[1] == [
        [events[0][0], events[0][-1], True],
        [events[1][0], events[1][-1], False],
    ]
    assert [item[2] for item in perception[1]] == [
        event[2][4] == 0 for event in events
    ]
    assert commit[18] == [task[0]]
    assert task[3] == events[0][0]
    assert (task[1], task[2], task[4], task[5], task[6]) == (
        commit[0],
        commit[1],
        commit[6],
        commit[8],
        commit[20],
    )
    assert task[4] == commit[6]
    assert task[5] == commit[8]
    assert task[8] == commit[21]

    empty = scenarios["epistemic.commit-batch.empty.valid"]
    empty_records = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in empty["records"]
    }
    empty_commit = empty_records["cycle-commit"]
    empty_witnesses = {
        witness["role"]: _decode_cbor_hex(witness["witness_cbor_hex"])
        for witness in empty["transition_witnesses"]
    }
    assert empty_commit[15] == []
    assert empty_commit[18] == []
    assert empty_witnesses["logical-sequence-transition"] == [2, 2]
    assert empty_commit[22] == 2
    prior_commit = empty_records["logical-cursor-anchor"]
    assert empty["pre_append_cursor"]["decision_ledger"] == [
        "logical-cursor-anchor",
        "fence",
    ]
    assert prior_commit[22] == empty_witnesses["logical-sequence-transition"][0]

    invalid = scenarios["epistemic.commit-batch.event-without-task.invalid"]
    assert invalid["pre_append_cursor"]["event_store"] == ["event-0"]
    assert invalid["pre_append_cursor"]["causal_outbox"] == []
    invalid_records = {record["role"] for record in invalid["records"]}
    assert "cycle-commit" in invalid_records
    assert "task-0" not in invalid_records
    invalid_commit = next(
        _decode_cbor_hex(record["record_cbor_hex"])
        for record in invalid["records"]
        if record["role"] == "cycle-commit"
    )
    assert invalid_commit[18]

    settlement_violations = {
        "decision_record_missing",
        "decision_record_extra",
        "decision_record_foreign",
        "fence_consumption_missing",
    }
    settlement_cases = {
        case["violation"]: scenarios[case["input_scenario_id"]]
        for case in fixtures["semantic"]
        if case.get("violation") in settlement_violations
    }
    assert settlement_cases.keys() == settlement_violations
    assert "decision" not in {
        record["role"]
        for record in settlement_cases["decision_record_missing"]["records"]
    }
    assert {
        record["role"]
        for record in settlement_cases["decision_record_extra"]["records"]
        if record["role"].startswith("decision")
    } == {"decision", "decision-extra"}
    foreign = settlement_cases["decision_record_foreign"]
    foreign_records = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in foreign["records"]
    }
    assert foreign_records["decision"][4][-2] not in [
        unit[3] for unit in foreign_records["fence"][11]
    ]
    assert foreign_records["cycle-commit"][16] == [foreign_records["decision"][0]]
    assert not any(
        witness["role"] == "source-consumption"
        for witness in settlement_cases["fence_consumption_missing"].get(
            "transition_witnesses", []
        )
    )


def test_atomic_commit_negative_scenarios_recompute_cryptographic_fields() -> None:
    fixtures = _json("fixtures.json")
    scenarios = {
        scenario["scenario_id"]: scenario
        for scenario in fixtures["linked_record_scenarios"]
    }
    by_violation = {
        case["violation"]: scenarios[case["input_scenario_id"]]
        for case in fixtures["semantic"]
        if case.get("constraint_id") == "publish-cycle-commit-batch"
        and case.get("input_scenario_id")
    }

    coordinate_violations = {
        "commit_run_id_mismatch",
        "commit_cycle_id_mismatch",
        "commit_attempt_ordinal_mismatch",
        "commit_instant_mismatch",
        "commit_cycle_ordinal_mismatch",
        "commit_cycle_plan_mismatch",
        "commit_base_revision_mismatch",
        "commit_base_state_hash_mismatch",
    }
    for violation in coordinate_violations:
        records = {
            record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
            for record in by_violation[violation]["records"]
        }
        commit = records["cycle-commit"]
        fence = records["fence"]
        events = [records[role] for role in ("event-0", "event-1")]
        assert commit[-1] == _batch_digest(commit, events)
        assert (
            commit[0],
            commit[1],
            commit[2],
            commit[3],
            commit[4],
            commit[5],
            commit[6],
            commit[7],
        ) != (
            fence[0],
            fence[1],
            fence[2],
            fence[4],
            fence[5],
            fence[6],
            fence[7],
            fence[8],
        )

    decision_scope = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in by_violation["decision_attempt_scope_mismatch"]["records"]
    }
    decision = decision_scope["decision"]
    assert decision[-1] == _domain_digest(
        "cote.csf.digest.decision-record",
        "cote.csf.schema.decision-record.body",
        decision[:-1],
    )
    assert decision[1:4] != decision_scope["fence"][0:3]

    event_scope = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in by_violation["event_coordinate_mismatch"]["records"]
    }
    event = event_scope["event-0"]
    assert event[0] == _domain_digest(
        "cote.csf.id.event",
        "cote.csf.schema.event-id-preimage",
        [event[1], event[7], event[2]],
    )
    assert event[-1] == _domain_digest(
        "cote.csf.digest.event", "cote.csf.schema.event.body", event[:-1]
    )
    assert (event[1], event[7], event[5], event[8]) != (
        event_scope["cycle-commit"][0],
        event_scope["cycle-commit"][1],
        event_scope["cycle-commit"][3],
        event_scope["cycle-commit"][6],
    )

    task_scope = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in by_violation["task_coordinate_mismatch"]["records"]
    }
    task = task_scope["task-0"]
    assert task[0] == _domain_digest(
        "cote.csf.id.perception-task",
        "cote.csf.schema.perception-task-id-preimage",
        [task[1], task[3], task[6], task[7], task[8]],
    )
    assert (task[1], task[2], task[4], task[5], task[6]) != (
        task_scope["cycle-commit"][0],
        task_scope["cycle-commit"][1],
        task_scope["cycle-commit"][6],
        task_scope["cycle-commit"][8],
        task_scope["cycle-commit"][20],
    )

    identity_scope = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in by_violation[
            "task_perception_identity_policy_mismatch"
        ]["records"]
    }
    identity_task = identity_scope["task-0"]
    identity_commit = identity_scope["cycle-commit"]
    identity_events = [
        identity_scope[role] for role in ("event-0", "event-1")
    ]
    assert identity_task[0] == _domain_digest(
        "cote.csf.id.perception-task",
        "cote.csf.schema.perception-task-id-preimage",
        [
            identity_task[1],
            identity_task[3],
            identity_task[6],
            identity_task[7],
            identity_task[8],
        ],
    )
    assert identity_commit[18] == [identity_task[0]]
    assert identity_commit[-1] == _batch_digest(identity_commit, identity_events)
    assert identity_task[8] != identity_commit[21]

    source_violations = {
        "source_state_missing",
        "source_already_consumed",
        "source_unit_foreign",
        "source_post_state_not_consumed",
    }
    world_violations = {
        "world_base_hash_missing",
        "world_base_hash_mismatch",
        "world_result_hash_mismatch",
        "world_revision_not_successor",
        "world_post_state_missing",
    }
    logical_violations = {
        "event_sequence_permuted",
        "event_sequence_duplicate",
        "event_sequence_gap",
        "next_logical_sequence_ahead",
        "next_logical_sequence_behind",
        "empty_commit_advances_logical_sequence",
    }
    perception_violations = {
        "perceptible_event_task_omitted",
        "duplicate_task_for_event",
        "task_for_nonperceptible_event",
    }
    assert source_violations | world_violations | logical_violations | perception_violations <= (
        by_violation.keys()
    )
    for violation in source_violations | world_violations | logical_violations | perception_violations:
        scenario = by_violation[violation]
        assert scenario["records"]
        assert scenario.get("transition_witnesses") is not None


def test_commit_candidate_partition_cases_are_executable() -> None:
    fixtures = _json("fixtures.json")
    scenarios = {
        scenario["scenario_id"]: scenario
        for scenario in fixtures["linked_record_scenarios"]
    }
    cases = [
        case
        for case in fixtures["semantic"]
        if case["kind"] == "commit_candidate_partition"
    ]
    assert {case["violation"] for case in cases} == {
        "candidate_missing",
        "candidate_foreign",
        "candidate_partition_gap",
        "candidate_partition_overlap",
        "candidate_produced_event_mismatch",
    }
    base = scenarios["epistemic.commit-batch.valid"]
    records = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in base["records"]
    }
    admitted_ids = {unit[3] for unit in records["fence"][11]}
    candidate = records["candidate"]
    decision = records["decision"]
    assert set(candidate[3]) == admitted_ids
    assert decision[5] == 0  # COMMIT; REJECT/DEFER never project candidate-domain events.
    assert decision[6] == [1, candidate[0]]
    assert decision[12] == records["cycle-commit"][15]
    assert {records[role][2][0] for role in ("event-0", "event-1")} == {1}
    mutations = {case["violation"]: case["mutation"] for case in cases}
    assert mutations["candidate_missing"] == {
        "op": "remove_record",
        "record_role": "candidate",
    }
    assert bytes.fromhex(mutations["candidate_foreign"]["value_hex"]) != candidate[1]
    assert bytes.fromhex(
        mutations["candidate_partition_gap"]["unit_id_hex"]
    ) not in admitted_ids
    assert bytes.fromhex(
        mutations["candidate_partition_overlap"]["source_unit_id_hex"]
    ) in admitted_ids
    assert bytes.fromhex(
        mutations["candidate_produced_event_mismatch"]["event_id_hex"]
    ) not in {records["event-0"][0], records["event-1"][0]}


def test_world_state_transition_executes_the_pinned_fixture_reducers() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    contract = registries["fixture_reducer_contracts"][0]
    assert contract["policy_hash_hex"] == hashlib.sha256(
        b"fixture-world-reducer-v1"
    ).hexdigest()
    selectors = {
        (
            item["event_type"],
            item["schema_version"],
            item["producer"],
            item["producer_version"],
        ): item["operation"]
        for item in contract["selectors"]
    }
    scenario = next(
        item
        for item in fixtures["linked_record_scenarios"]
        if item["scenario_id"] == "epistemic.commit-batch.valid"
    )
    records = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in scenario["records"]
    }
    witnesses = {
        witness["role"]: _decode_cbor_hex(witness["witness_cbor_hex"])
        for witness in scenario["transition_witnesses"]
    }
    transition = witnesses["world-state-transition"]
    state = transition[1]
    for event in (records["event-0"], records["event-1"]):
        operation = selectors[(event[3], event[4], event[16], event[17])]
        if operation == "replace-state-with-event-payload":
            state = event[9]
        elif operation != "identity":
            raise AssertionError(operation)
    assert state == transition[4]

    case = next(
        item
        for item in fixtures["semantic"]
        if item["case_id"] == "linked-state.world-state.reducer-result-mismatch"
    )
    replacement = _decode_cbor_hex(case["mutation"]["result_state_cbor_hex"])
    replacement_hash = _domain_digest(
        "cote.csf.hash.world-state", "cote.csf.schema.typed-value", replacement
    )
    mutated_commit = list(records["cycle-commit"])
    mutated_commit[9] = replacement_hash
    mutated_commit[-1] = _batch_digest(
        mutated_commit, [records["event-0"], records["event-1"]]
    )
    assert mutated_commit[9] == replacement_hash
    assert mutated_commit[-1] == _batch_digest(
        mutated_commit, [records["event-0"], records["event-1"]]
    )
    assert replacement != state


def test_logical_cursor_anchor_rejects_a_locally_consistent_jump() -> None:
    fixtures = _json("fixtures.json")
    case = next(
        item
        for item in fixtures["semantic"]
        if item["case_id"] == "linked-state.logical-sequence.cursor-anchor-mismatch"
    )
    assert case["durable_cursor"] == 2
    assert case["witness_before"] == case["witness_after"] == case["commit_next"]
    assert case["event_count"] == 0
    assert case["witness_before"] != case["durable_cursor"]


def test_epistemic_completion_is_terminal_and_policies_are_pinned() -> None:
    registries = _json("registries.json")
    fixtures = _json("fixtures.json")
    constraints = {
        item["constraint_id"]: item
        for item in registries["linked_record_constraints"]
    }
    observation = constraints["append-observation"]
    knowledge = constraints["append-knowledge-input"]
    assert "observation.resolver == task.resolver byte-for-byte" in observation["relations"]
    assert any("exact observer allow-list" in relation for relation in observation["relations"])
    assert any("observer-specific projection" in relation for relation in observation["relations"])
    assert any("supplied allow-list/output is ignored" in relation for relation in observation["relations"])
    assert "no perception-task-completion for task.task-id exists at pre-append cursor" in observation["relations"]
    assert "no perception-task-completion for task.task-id exists at pre-append cursor" in knowledge["relations"]
    assert "causal-outbox" in knowledge["applies_on"]["cursor"]

    scenarios = {
        scenario["scenario_id"]: scenario
        for scenario in fixtures["linked_record_scenarios"]
    }
    for scenario_id, candidate_role in (
        ("epistemic.observation.after-completion.invalid", "observation"),
        ("epistemic.knowledge-input.after-completion.invalid", "knowledge-input"),
    ):
        scenario = scenarios[scenario_id]
        assert "completion" in scenario["pre_append_cursor"]["causal_outbox"]
        assert candidate_role in {record["role"] for record in scenario["records"]}

    resolver_case = next(
        item
        for item in fixtures["semantic"]
        if item.get("violation") == "observation_resolver_mismatch"
    )
    mutated_observation = _decode_cbor_hex(
        resolver_case["mutation"]["record_cbor_hex"]
    )
    valid = scenarios[resolver_case["base_scenario_id"]]
    records = {
        record["role"]: _decode_cbor_hex(record["record_cbor_hex"])
        for record in valid["records"]
    }
    assert mutated_observation[-1] == _domain_digest(
        "cote.csf.digest.observation",
        "cote.csf.schema.observation.body",
        mutated_observation[:-1],
    )
    assert mutated_observation[1] == records["task"][0]
    assert mutated_observation[13] != records["task"][7]


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
        if "base_scenario_id" in case:
            assert case["base_scenario_id"] in scenarios
        if case["expected"] == "reject" and "mutation" in case:
            mutation = case["mutation"]
            assert mutation["op"] == "replace_record"
            assert _decode_cbor_hex(mutation["record_cbor_hex"])
            assert case["invalid_error_code"] == "LINKED_RECORD_CONSTRAINT"
        elif case["expected"] == "reject":
            assert case["input_scenario_id"] in scenarios
            assert case["invalid_error_code"] == "LINKED_RECORD_CONSTRAINT"
        else:
            assert "mutation" not in case

    durable_terminal_cases = {
        case["violation"]: scenarios[case["input_scenario_id"]]
        for case in linked_cases
        if case.get("violation") in {"not_latest_abort", "later_terminal_other_cycle"}
    }
    assert durable_terminal_cases.keys() == {
        "not_latest_abort",
        "later_terminal_other_cycle",
    }
    for violation, scenario in durable_terminal_cases.items():
        records = {record["role"]: record for record in scenario["records"]}
        assert scenario["pre_append_cursor"]["decision_ledger"][-1] == "later-terminal"
        assert "later-terminal" in records
        later = _decode_cbor_hex(records["later-terminal"]["record_cbor_hex"])
        assert records["later-terminal"]["schema_id"] == "cote.csf.schema.cycle-abort"
        expected_digest = hashlib.sha256(
            _encode_cbor_item(
                [
                    b"CSF\x00",
                    1,
                    "cote.csf.digest.abort",
                    "cote.csf.schema.cycle-abort.body",
                    1,
                    later[:-1],
                ]
            )
        ).digest()
        assert later[-1] == expected_digest
        original = _decode_cbor_hex(records["abort"]["record_cbor_hex"])
        assert later[0] == original[0]
        assert (later[1] == original[1]) is (violation == "not_latest_abort")
        retry = _decode_cbor_hex(records["retry"]["record_cbor_hex"])
        expected_abort_id = hashlib.sha256(
            _encode_cbor_item(
                [
                    b"CSF\x00",
                    1,
                    "cote.csf.id.cycle-abort",
                    "cote.csf.schema.cycle-abort-id-preimage",
                    1,
                    original[:3],
                ]
            )
        ).digest()
        assert retry[4] == [13, expected_abort_id, original[-1]]
        fence_role = "later-fence" if violation == "not_latest_abort" else "other-cycle-fence"
        fence = _decode_cbor_hex(records[fence_role]["record_cbor_hex"])
        assert later[8] == fence[-1]

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
    transitions = _json("causal-transition-fixtures.json")
    contract = (BUNDLE.parent / "canonical-codec-v1.md").read_text(encoding="utf-8")
    readme = (BUNDLE / "README.md").read_text(encoding="utf-8")

    assert len(fixtures["positive"]) == 156
    assert len(fixtures["negative"]) == 30
    assert len(fixtures["semantic"]) == 306
    assert len(fixtures["normalization_cases"]) == 7
    assert len(transitions["cases"]) == 114
    assert "156 casos positivos" in contract
    assert "30 casos negativos" in contract
    assert "306 casos semânticos" in contract
    assert "114 casos executáveis de transição causal" in contract
    assert "156 vetores positivos, 30 negativos, 306 casos semânticos" in readme
    assert "totalizando 615 vetores/casos de conformidade" in readme


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


def _json_pointer_parent(document, pointer: str):
    parts = [part.replace("~1", "/").replace("~0", "~") for part in pointer.split("/")[1:]]
    target = document
    for part in parts[:-1]:
        target = target[int(part)] if isinstance(target, list) else target[part]
    return target, parts[-1]


def _apply_fixture_patch(document, operations: list[dict]):
    import copy

    result = copy.deepcopy(document)
    for operation in operations:
        parent, key = _json_pointer_parent(result, operation["path"])
        if operation["op"] == "replace":
            if isinstance(parent, list):
                parent[int(key)] = operation["value"]
            else:
                parent[key] = operation["value"]
        elif operation["op"] == "remove":
            parent.pop(int(key)) if isinstance(parent, list) else parent.pop(key)
        elif operation["op"] == "add":
            if isinstance(parent, list):
                parent.append(operation["value"]) if key == "-" else parent.insert(int(key), operation["value"])
            else:
                parent[key] = operation["value"]
        else:
            raise AssertionError(operation)
    return result


def _coordinate(value: list[int]) -> tuple[int, int]:
    return value[0], value[1]


_EVENT_PHASE_ORDER = {
    "TEMPORAL_ADVANCE": 0,
    "CANDIDATE_DOMAIN": 1,
    "SOURCE_LIFECYCLE": 2,
    "TRIGGER_RUNTIME": 3,
    "TRIGGER_ACTIVATION": 4,
}


def _event_order_components(order_key: dict) -> list:
    return [
        _EVENT_PHASE_ORDER[order_key["phase"]],
        order_key["origin_ref"],
        order_key["producer"],
        order_key["producer_version"],
        order_key["event_local_ordinal"],
        order_key["event_role"],
    ]


def _derived_event_id(fence: dict, order_key: dict) -> str:
    return _domain_digest(
        "cote.csf.id.event",
        "cote.csf.schema.event-id-preimage",
        [fence["run_id"], fence["cycle_id"], _event_order_components(order_key)],
    ).hex()


def _ledger_prefix_digest(ledger_id: str, prefix: list[dict]) -> str:
    return _domain_digest(
        "cote.csf.digest.ledger-prefix",
        "cote.csf.schema.ledger-prefix-digest-preimage",
        [ledger_id, [bytes.fromhex(item["canonical_envelope_hex"]) for item in prefix]],
    ).hex()


def _canonical_typed_fixture(domain_tag: str, schema_id: str, value) -> dict:
    envelope = _encode_cbor_item([b"CSF\x00", 1, domain_tag, schema_id, 1, value])
    return {
        "schema_id": schema_id,
        "schema_version": 1,
        "canonical_envelope_hex": envelope.hex(),
        "envelope_digest_hex": hashlib.sha256(envelope).hexdigest(),
        "value": value,
    }


def _derive_snapshot_pending_state(authorities: dict) -> dict:
    schedule = authorities["schedule_store"]
    trigger = authorities["trigger_registry"]
    input_ledger = authorities["input_ledger"]
    outbox = authorities["causal_outbox"]
    completed = {item["task_id"] for item in outbox["completions"]}
    responded_dispatches = {item["dispatch_id"] for item in input_ledger["responses"]}
    revoked_dispatches = {item["dispatch_id"] for item in input_ledger["revocations"]}
    return {
        "occurrences": sorted(item["id"] for item in schedule["occurrences"] if item["lifecycle"] == "PENDING"),
        "trigger_runtime": sorted((item["record"] for item in trigger["runtime_states"]), key=lambda item: (item[0], item[1])),
        "trigger_activations": sorted(item["id"] for item in trigger["activations"] if item["lifecycle"] == "PENDING"),
        "source_closures": sorted(item["id"] for item in input_ledger["source_closures"] if item["is_latest"]),
        "rounds": sorted(item["id"] for item in schedule["rounds"] if item["lifecycle"] == "PENDING"),
        "dispatches": sorted(item["dispatch_id"] for item in input_ledger["dispatches"] if item["dispatch_id"] not in responded_dispatches | revoked_dispatches),
        "responses": sorted(item["id"] for item in input_ledger["responses"]),
        "pending_units": sorted(item["id"] for item in input_ledger["units"] if not item["settled"]),
        "perception_tasks": sorted(item["task_id"] for item in outbox["tasks"] if item["task_id"] not in completed),
        "perception_completions": sorted((item["record"] for item in outbox["completions"]), key=lambda item: item[1]),
    }


def _commit_successor_floor(fence: dict) -> list[int]:
    if fence["cycle_plan"][0] == "CLOCK_ADVANCE":
        return [fence["cycle_plan"][2], 0]
    return [fence["instant"], fence["cycle_ordinal"] + 1]


def _defer_successor_error(successor: dict, predecessor: dict, fence: dict) -> bool:
    successor_id = successor.get("input_id", successor.get("occurrence_id"))
    if successor_id == predecessor["unit_id"] or _coordinate(successor["eligibility"]) < _coordinate(_commit_successor_floor(fence)):
        return True
    if successor["run_id"] != fence["run_id"]:
        return True
    if successor["root"] == "exogenous-input":
        provenance = successor["provenance"]
        if provenance != {
            "kind": predecessor["root"],
            "id": predecessor["unit_id"],
            "digest": predecessor["unit_digest"],
        }:
            return True
        expected_id = _domain_digest(
            "cote.csf.id.exogenous-input",
            "cote.csf.schema.exogenous-input-id-preimage",
            [successor["run_id"], successor["source_id"], successor["producer_unit_key"]],
        ).hex()
        body = [
            expected_id,
            successor["run_id"],
            successor["source_id"],
            successor["producer_unit_key"],
            successor["input_kind"],
            successor["input_schema_version"],
            successor["actor"],
            successor["payload"],
            successor["declared_effective_at"],
            successor["eligibility"],
            [provenance["kind"], provenance["id"], provenance["digest"]],
            successor["idempotency_key"],
        ]
        expected_digest = _domain_digest(
            "cote.csf.digest.exogenous-input-unit",
            "cote.csf.schema.exogenous-input.body",
            body,
        ).hex()
        return successor["input_id"] != expected_id or successor["unit_digest"] != expected_digest
    if successor["root"] == "scheduled-occurrence":
        created_by = successor["created_by"]
        if created_by != {
            "kind": predecessor["root"],
            "id": predecessor["unit_id"],
            "digest": predecessor["unit_digest"],
        }:
            return True
        expected_id = _domain_digest(
            "cote.csf.id.occurrence",
            "cote.csf.schema.occurrence-id-preimage",
            [[created_by["kind"], created_by["id"], created_by["digest"]], successor["occurrence_role"], successor["occurrence_local_ordinal"]],
        ).hex()
        body = [
            expected_id,
            successor["run_id"],
            successor["source_id"],
            successor["due_at"],
            successor["eligibility"],
            successor["occurrence_kind"],
            successor["occurrence_schema_version"],
            successor["payload"],
            successor["lifecycle"],
            [created_by["kind"], created_by["id"], created_by["digest"]],
            successor["occurrence_role"],
            successor["occurrence_local_ordinal"],
            successor["recurrence"],
            successor["idempotency_key"],
        ]
        expected_digest = _domain_digest(
            "cote.csf.digest.occurrence-unit",
            "cote.csf.schema.scheduled-occurrence.body",
            body,
        ).hex()
        return successor["occurrence_id"] != expected_id or successor["unit_digest"] != expected_digest
    return True


def _derive_result_state(state: dict, events: list[dict]) -> tuple[dict | None, str | None]:
    import copy

    result = copy.deepcopy(state["world_state_before"])
    for event in events:
        if _EVENT_PHASE_ORDER[event["order_key"]["phase"]] > 2:
            continue
        reducer = state["reducer_registry"].get(event["event_type"])
        if reducer is None:
            return None, "REDUCER_UNRESOLVED"
        if reducer["operation"] == "noop":
            continue
        if reducer["operation"] == "set-path-from-payload":
            path, value = event["payload"]
            result[path] = value
            continue
        return None, "REDUCER_UNRESOLVED"
    return result, None


def _derive_trigger_outputs(state: dict, fence: dict, tx: dict, result_state: dict) -> tuple[list[dict], list[dict]]:
    definitions = state["trigger_registry"]["definitions"]
    runtime_by_id = {
        item["trigger_id"]: item for item in state["trigger_registry"]["runtime_states"]
    }
    runtime_outputs, activations = [], []
    pre_trigger_events = [
        event
        for event in tx["events"]
        if _EVENT_PHASE_ORDER[event["order_key"]["phase"]] <= 2
    ]
    successor_floor = _commit_successor_floor(fence)
    now = successor_floor[0]
    for definition in definitions:
        before = runtime_by_id[definition["trigger_id"]]
        touched = [
            event["event_id"]
            for event in pre_trigger_events
            if set(state["reducer_registry"][event["event_type"]]["dependency_footprint"])
            & set(definition["dependencies"])
        ]
        if not touched:
            continue
        current = result_state.get(definition["predicate"]["path"]) == definition["predicate"]["equals"]
        policy = definition["activation_policy"]
        fire = False
        armed = before["armed"]
        exhausted = before["exhausted"]
        next_repeat_at = before["next_repeat_at"]
        if policy == "RISING_EDGE":
            fire = armed and current and (
                before["last_value"] is False
                or (before["last_value"] is None and definition["fire_on_initial_true"])
            )
            armed = not current
        elif policy == "FALLING_EDGE":
            fire = armed and before["last_value"] is True and not current
            armed = current
        elif policy == "ONCE_WHEN_TRUE":
            fire = current and not exhausted
            exhausted = exhausted or fire
        elif policy == "REPEAT_WHILE_TRUE":
            fire = current and (next_repeat_at is None or now >= next_repeat_at)
            next_repeat_at = now + definition["repeat_every"] if current else None
        count = before["activation_count"] + (1 if fire else 0)
        runtime = {
            "trigger_id": definition["trigger_id"],
            "last_value": current,
            "armed": armed,
            "exhausted": exhausted,
            "activation_count": count,
            "next_repeat_at": next_repeat_at,
            "evaluated_through_revision": state["result_revision"],
        }
        runtime_outputs.append(runtime)
        if fire:
            activations.append(
                {
                    "trigger_id": definition["trigger_id"],
                    "activation_count": count,
                    "causing_event_ids": touched,
                    "eligibility": successor_floor,
                }
            )
    return runtime_outputs, activations


_CANONICAL_ADMITTED_UNIT_FIELDS = {
    "eligibility",
    "unit_kind",
    "source_id",
    "unit_id",
    "unit_digest",
}


def _resolve_source_units(
    state: dict, fence: dict, contracts: dict
) -> tuple[dict[str, dict], str | None]:
    resolved: dict[str, dict] = {}
    authorities = state["source_authorities"]
    admission_history = state["unit_admission_history"]
    owner_by_root = contracts["source_record_authorities"]
    projections = contracts["source_record_projection_contracts"]
    for admitted in fence["admitted_units"]:
        if set(admitted) != _CANONICAL_ADMITTED_UNIT_FIELDS:
            return {}, "ADMISSION_FENCE_UNIT_NOT_CANONICAL"
        matches = [
            (owner, records[admitted["unit_id"]])
            for owner, records in authorities.items()
            if admitted["unit_id"] in records
            and records[admitted["unit_id"]]["unit_digest"]
            == admitted["unit_digest"]
        ]
        if len(matches) != 1:
            return {}, "SOURCE_RECORD_UNRESOLVED"
        owner, record = matches[0]
        if owner_by_root.get(record["root"]) != owner:
            return {}, "SOURCE_RECORD_AUTHORITY_MISMATCH"
        projection = projections.get(record["root"])
        if projection is not None:
            body = record.get("record_body")
            if (
                body is None
                or body[projection["unit_id_index"]] != admitted["unit_id"]
                or _domain_digest(
                    projection["digest_operation"], projection["digest_schema"], body
                ).hex()
                != admitted["unit_digest"]
            ):
                return {}, "SOURCE_RECORD_DIGEST_MISMATCH"
        resolved[admitted["unit_id"]] = (
            admitted | record | admission_history.get(admitted["unit_id"], {})
        )
    return resolved, None


def _candidate_event_projection(
    candidate: dict,
    draft: dict,
    units: dict[str, dict],
    contracts: dict,
) -> dict | None:
    event_contract = contracts["event_projection_contracts"].get(draft["event_type"])
    if event_contract is None:
        return None
    sources = [units.get(unit_id) for unit_id in candidate["source_unit_ids"]]
    if any(source is None for source in sources):
        return None
    source_contracts = contracts["source_record_projection_contracts"]
    if any(source["root"] not in source_contracts for source in sources):
        return None
    actor_refs: set[str] = set()
    entity_refs: set[str] = set()
    locations: set[str] = set()
    for source in sources:
        source_contract = source_contracts[source["root"]]
        body = source["record_body"]
        actor_refs.add(body[source_contract["actor_index"]])
        if "targets_index" in source_contract:
            entity_refs.update(target[1] for target in body[source_contract["targets_index"]])
        if "location_index" in source_contract:
            location = body[source_contract["location_index"]]
            if location is not None:
                locations.add(location)
    if len(locations) > 1:
        return None
    return {
        "event_schema_version": event_contract["schema_version"],
        "source_inputs": [
            {"id": unit_id, "digest": units[unit_id]["unit_digest"]}
            for unit_id in candidate["source_unit_ids"]
        ],
        "actor_refs": sorted(actor_refs),
        "entity_refs": sorted(entity_refs),
        "location_ref": next(iter(locations), None),
        "confidentiality": event_contract["default_confidentiality"],
    }


def _observation_transition_error(state: dict, contracts: dict) -> str | None:
    observation = state["candidate"]
    task = state["task"]
    event = state["event"]
    if observation["task_id"] != task["task_id"] or task["event_id"] != event["event_id"]:
        return "OBSERVATION_TASK_MISMATCH"
    if state["completions"]:
        return "PERCEPTION_TASK_ALREADY_COMPLETE"
    if observation["observed_at"] != event["occurred_at"]:
        return "OBSERVATION_INSTANT_MISMATCH"
    if observation["source_event_refs"] != [
        {"id": event["event_id"], "digest": event["event_digest"]}
    ]:
        return "OBSERVATION_SOURCE_MISMATCH"
    if observation["resolver"] != task["resolver"]:
        return "OBSERVATION_RESOLVER_MISMATCH"
    policy = next(
        (
            item
            for item in contracts["observer_projection_contracts"]
            if item["policy_ref"] == task["perception_policy"]
            and item["resolver_ref"] == task["resolver"]
        ),
        None,
    )
    if policy is None:
        return "OBSERVATION_POLICY_UNRESOLVED"
    revisions = state["world_state_by_revision"]
    before = revisions.get(str(task["base_revision"]))
    after = revisions.get(str(task["result_revision"]))
    channels = [
        (channel_ref, channel)
        for channel_ref, channel in state["channel_authority"].items()
        if channel["location_ref"] == event["location_ref"]
    ]
    if before is None or after is None or len(channels) != 1:
        return "OBSERVATION_AUTHORITY_UNRESOLVED"
    channel_ref, channel = channels[0]
    before_members = set(before["channel_members"].get(channel_ref, []))
    after_members = set(after["channel_members"].get(channel_ref, []))
    confidentiality_rank = {"PUBLIC": 0, "RESTRICTED": 1, "SECRET": 2}
    allowed_observers = {
        actor
        for actor in before_members & after_members
        if state["actor_authority"].get(actor, {}).get("active")
        and confidentiality_rank[
            state["actor_authority"][actor]["clearance"]
        ]
        >= confidentiality_rank[event["confidentiality"]]
    }
    if observation["observer"] not in allowed_observers:
        return "OBSERVATION_ACCESS_DENIED"
    payload = event["payload"]
    fields = policy["payload_projection"]
    projection = {
        field: payload[payload_field] for field, payload_field in fields.items()
    } | {"modality": channel["modality"], "channel_ref": channel_ref}
    projected_fields = (
        "percepts",
        "claim_refs",
        "evidence_chain",
        "omissions_redactions",
        "modality",
        "channel_ref",
    )
    if any(observation[field] != projection[field] for field in projected_fields):
        return "OBSERVATION_PROJECTION_MISMATCH"
    return None


def _derive_perception_tasks(state: dict, fence: dict, events: list[dict]) -> list[dict]:
    policy = state["perception_policy"]
    tasks = []
    for event in events:
        if event["event_type"] not in policy["perceptible_event_types"]:
            continue
        task_id = _domain_digest(
            "cote.csf.id.perception-task",
            "cote.csf.schema.perception-task-id-preimage",
            [
                fence["run_id"],
                event["event_id"],
                policy["policy_ref"],
                policy["resolver_ref"],
                policy["identity_policy_ref"],
            ],
        ).hex()
        tasks.append(
            {
                "task_id": task_id,
                "event_id": event["event_id"],
                "event_digest": event["event_digest"],
                "base_revision": state["base_revision"],
                "result_revision": state["result_revision"],
                "perception_policy": policy["policy_ref"],
                "resolver": policy["resolver_ref"],
                "perception_identity_policy": policy["identity_policy_ref"],
            }
        )
    return tasks


def _derive_control_state(prefix: list[dict]) -> dict:
    latest_fence = next((item for item in reversed(prefix) if item["kind"] == "fence"), None)
    terminals = [item for item in prefix if item["kind"] in {"abort", "commit"}]
    last_terminal = terminals[-1] if terminals else None
    matching_terminal = next((item for item in reversed(terminals) if latest_fence and item["cycle_id"] == latest_fence["cycle_id"] and item["attempt"] == latest_fence["attempt"]), None)
    retry = next((item for item in reversed(prefix) if item["kind"] == "retry" and last_terminal and item["abort_id"] == last_terminal.get("abort_id")), None)
    attempts = [item["attempt"] for item in prefix if item["kind"] == "fence" and (latest_fence is None or item["cycle_id"] == latest_fence["cycle_id"])]
    next_attempt = max(attempts, default=0) + 1
    if latest_fence and matching_terminal is None:
        return {"status": "ATTEMPT_IN_FLIGHT", "cycle_id": latest_fence["cycle_id"], "attempt_ordinal": latest_fence["attempt"], "fence_ref": latest_fence["ref"], "retry_ref": None, "last_terminal_envelope_ref": last_terminal["ref"] if last_terminal else None, "next_attempt_ordinal": next_attempt}
    if last_terminal and last_terminal["kind"] == "abort" and retry:
        return {"status": "RETRY_AUTHORIZED", "cycle_id": last_terminal["cycle_id"], "attempt_ordinal": retry["to_attempt"], "fence_ref": None, "retry_ref": retry["ref"], "last_terminal_envelope_ref": last_terminal["ref"], "next_attempt_ordinal": next_attempt}
    if last_terminal and last_terminal["kind"] == "abort":
        return {"status": "HALTED_ON_ABORT", "cycle_id": last_terminal["cycle_id"], "attempt_ordinal": last_terminal["attempt"], "fence_ref": None, "retry_ref": None, "last_terminal_envelope_ref": last_terminal["ref"], "next_attempt_ordinal": next_attempt}
    return {"status": "IDLE", "cycle_id": None, "attempt_ordinal": None, "fence_ref": None, "retry_ref": None, "last_terminal_envelope_ref": last_terminal["ref"] if last_terminal else None, "next_attempt_ordinal": 1}


def _transition_error(operation: str, state: dict, contracts: dict) -> str | None:
    if operation == "append-source-record":
        configured = state["configured_sources"]
        candidate = state["candidate"]
        receipt = state.get("candidate_receipt")
        if candidate["source_id"] not in configured:
            return "SOURCE_NOT_DECLARED"
        prefix = [item for item in state["prefix"] if item["record"]["source_id"] == candidate["source_id"]]
        if receipt is None:
            return "SOURCE_RECEIPT_MISSING"
        if receipt.get("root") != "source-ingress-receipt":
            return "SOURCE_RECEIPT_NOT_CANONICAL"
        if any(item["record"].get("final", False) for item in prefix):
            return "SOURCE_FINAL"
        expected_seq = (prefix[-1]["receipt"]["ingress_seq"] + 1) if prefix else 1
        record_id = candidate.get("closure_id", candidate.get("input_id"))
        record_ref = receipt.get("record_ref", {})
        record_digest = candidate.get("unit_digest", record_ref.get("digest"))
        if (receipt["source_id"], record_ref.get("id"), record_ref.get("digest")) != (candidate["source_id"], record_id, record_digest):
            return "SOURCE_RECEIPT_MISMATCH"
        if receipt["ingress_seq"] != expected_seq:
            return "INGRESS_NOT_CONTIGUOUS"
        closures = [item["record"] for item in prefix if item["record"]["root"] == "source-closure"]
        if candidate["root"] == "source-closure":
            if candidate["ingress_seq"] != receipt["ingress_seq"]:
                return "SOURCE_RECEIPT_MISMATCH"
            if closures and _coordinate(candidate["closed_through"]) < _coordinate(closures[-1]["closed_through"]):
                return "CLOSURE_REGRESSION"
        elif closures:
            open_coordinate = [closures[-1]["closed_through"][0], closures[-1]["closed_through"][1] + 1]
            if _coordinate(candidate["eligibility"]) < _coordinate(open_coordinate):
                return "RETROACTIVE_ELIGIBILITY"
        return None

    if operation == "append-slot-dispatch":
        dispatch = state["candidate"]
        round_ = state["round"]
        cycle = state["cycle"]
        inherited = ("run_id", "decision_round_id", "actor", "eligibility")
        if round_["lifecycle"] != "PENDING":
            return "DISPATCH_ROUND_NOT_PENDING"
        if round_["decision_round_id"] not in cycle["cohort_round_ids"]:
            return "DISPATCH_ROUND_OUTSIDE_COHORT"
        if dispatch["slot_id"] not in round_["slot_ids"] or any(dispatch[name] != round_[name] for name in inherited) or dispatch["cycle_id"] != cycle["cycle_id"] or dispatch["base_revision"] != cycle["base_revision"] or dispatch["effective_at"] != cycle["instant"]:
            return "DISPATCH_ROUND_MISMATCH"
        same_slot = [item for item in state["dispatches"] if item["decision_round_id"] == dispatch["decision_round_id"] and item["slot_id"] == dispatch["slot_id"]]
        revoked = {item["dispatch_id"] for item in state["revocations"]}
        if any(item["dispatch_id"] not in revoked for item in same_slot) or any(item["decision_round_id"] == dispatch["decision_round_id"] and item["slot_id"] == dispatch["slot_id"] for item in state["responses"]):
            return "DISPATCH_ALREADY_OPEN"
        if dispatch["dispatch_ordinal"] != len(same_slot):
            return "DISPATCH_ORDINAL_MISMATCH"
        task_by_id = {item["task_id"]: item for item in state["causal_outbox"]["tasks"]}
        completion_by_task = {item["task_id"]: item for item in state["causal_outbox"]["completions"]}
        prior_task_ids = {
            task_id
            for commit in state["decision_ledger"]["cycle_commits"]
            if commit["run_id"] == dispatch["run_id"]
            and commit["result_revision"] <= dispatch["base_revision"]
            for task_id in commit["perception_task_ids"]
        }
        if any(task_id not in task_by_id or task_id not in completion_by_task for task_id in prior_task_ids):
            return "EPISTEMIC_TASK_PENDING"
        knowledge_by_id = {item["knowledge_input_id"]: item for item in state["evidence_ledger"]["knowledge_inputs"]}
        addressed = {
            knowledge_input_id
            for task_id in prior_task_ids
            for knowledge_input_id in completion_by_task[task_id]["knowledge_input_ids"]
            if knowledge_by_id.get(knowledge_input_id, {}).get("recipient") == dispatch["actor"]
        }
        expected_addressed = {
            knowledge_input_id
            for task_id in prior_task_ids
            for knowledge_input_id in completion_by_task[task_id]["knowledge_input_ids"]
            if knowledge_input_id in knowledge_by_id
            and knowledge_by_id[knowledge_input_id].get("recipient") == dispatch["actor"]
        }
        if addressed != expected_addressed or any(
            knowledge_input_id not in knowledge_by_id
            for task_id in prior_task_ids
            for knowledge_input_id in completion_by_task[task_id]["knowledge_input_ids"]
        ):
            return "EPISTEMIC_DELIVERY_PENDING"
        return None

    if operation == "append-observation":
        return _observation_transition_error(state, contracts)

    if operation == "append-slot-response":
        response = state["candidate"]
        dispatch = next((item for item in state["dispatches"] if item["dispatch_id"] == response["dispatch_id"]), None)
        if dispatch is None or response["dispatch_id"] in {item["dispatch_id"] for item in state["revocations"]}:
            return "DISPATCH_NOT_OPEN"
        inherited = ("run_id", "cycle_id", "decision_round_id", "slot_id", "actor", "effective_at")
        if any(response[name] != dispatch[name] for name in inherited) or response["submitted_against_revision"] != dispatch["base_revision"]:
            return "RESPONSE_DISPATCH_MISMATCH"
        existing = next((item for item in state["responses"] if item["decision_round_id"] == response["decision_round_id"] and item["slot_id"] == response["slot_id"]), None)
        if existing == response:
            return None
        if existing is not None:
            return "SLOT_ALREADY_FILLED"
        return None

    if operation == "append-admission-fence":
        current = (state["clock"]["current_instant"], state["clock"]["cycle_ordinal_at_instant"])
        consumed = {unit_id for commit in state["prior_cycle_commits"] for unit_id in commit["admitted_input_ids"]}
        pending = [item for name in ("inputs", "occurrences", "activations") for item in state[name] if item["unit_id"] not in consumed]
        pending.extend(
            {"eligibility": item["eligibility"], "round_id": item["round_id"]}
            for item in state["rounds"]
            if item["lifecycle"] == "PENDING"
        )
        minimum = min((_coordinate(item["eligibility"]) for item in pending), default=None)
        if minimum is None:
            return "NO_PENDING_WORK"
        if minimum <= current:
            return "PENDING_COORDINATE_STALE"
        if minimum[0] == current[0]:
            coordinate = minimum
            derived_plan = ["WORK"]
        else:
            coordinate = (current[0], current[1] + 1)
            derived_plan = ["CLOCK_ADVANCE", current[0], minimum[0], list(minimum)]
        if _coordinate(state["coordinate"]) != coordinate:
            return "NEXT_CYCLE_COORDINATE_MISMATCH"
        closure_coordinate = _coordinate(derived_plan[3]) if derived_plan[0] == "CLOCK_ADVANCE" else coordinate
        closures = []
        for source_id in state["exogenous_sources"]:
            covering = [item for item in state["closures"] if item["source_id"] == source_id and (item["final"] or _coordinate(item["closed_through"]) >= closure_coordinate)]
            if not covering:
                return "SOURCE_NOT_CLOSED"
            item = min(covering, key=lambda value: value["ingress_seq"])
            closures.append({key: item[key] for key in ("source_id", "closure_id", "ingress_seq", "closed_through", "final")})
        rounds = sorted(
            [item for item in state["rounds"] if item["lifecycle"] == "PENDING" and _coordinate(item["eligibility"]) <= coordinate],
            key=lambda item: item["round_id"],
        )
        cohort = []
        units = []
        for round_ in rounds:
            slots = []
            for slot_id in sorted(round_["slot_ids"]):
                response = next((item for item in state["responses"] if item["round_id"] == round_["round_id"] and item["slot_id"] == slot_id), None)
                if response is None:
                    return "COHORT_SLOT_UNFILLED"
                slots.append([slot_id, response["unit_id"]])
                if response["unit_id"] not in consumed:
                    units.append([round_["eligibility"], "SLOT", round_["source_id"], response["unit_id"], response["unit_digest"]])
            cohort.append([round_["round_id"], slots])
        for collection, kind in ((state["inputs"], "EXOGENOUS_INPUT"), (state["occurrences"], "SCHEDULED_OCCURRENCE"), (state["activations"], "TRIGGER_ACTIVATION")):
            for item in collection:
                if item["unit_id"] not in consumed and _coordinate(item["eligibility"]) <= coordinate:
                    units.append([item["eligibility"], kind, item["source_id"], item["unit_id"], item["unit_digest"]])
        units.sort(key=lambda item: (_coordinate(item[0]), contracts["unit_kind_order"].index(item[1]), item[2], item[4], item[3]))
        derived = {"closure_proof": closures, "cohort": cohort, "admitted_units": units, "cycle_plan": derived_plan}
        return None if state["candidate"] == derived else "FENCE_DERIVATION_MISMATCH"

    if operation == "publish-cycle-commit-batch":
        return _commit_transition_error(state, contracts)

    if operation == "append-cycle-abort":
        fence = state["fence"]
        abort = state["abort"]
        if any(
            (item["run_id"], item["cycle_id"], item["attempt"])
            == (fence["run_id"], fence["cycle_id"], fence["attempt"])
            for item in state["terminals"]
        ):
            return "TERMINAL_ALREADY_EXISTS"
        units = {item["unit_id"]: item for item in fence["admitted_units"]}
        candidates = {item["candidate_id"]: item for item in state["commit_candidates"]}
        for subject in abort["indeterminate_subjects"]:
            if subject["kind"] == "ADMITTED_UNIT":
                unit = units.get(subject["id"])
                if unit is None or unit["digest"] != subject["digest"]:
                    return "ABORT_SUBJECT_MISMATCH"
            elif subject["kind"] == "COMMIT_CANDIDATE":
                candidate = candidates.get(subject["id"])
                if (
                    candidate is None
                    or candidate["candidate_digest"] != subject["digest"]
                    or (candidate["run_id"], candidate["cycle_id"])
                    != (fence["run_id"], fence["cycle_id"])
                    or not set(candidate["source_unit_ids"]) <= units.keys()
                ):
                    return "ABORT_CANDIDATE_MISMATCH"
        evidence = {item["evidence_id"]: item for item in state["failure_evidence"]}
        for evidence_id in abort["failure_evidence_refs"]:
            item = evidence.get(evidence_id)
            if item is None:
                return "ABORT_EVIDENCE_UNRESOLVED"
            if item["kind"] == "conflict-set" and not set(item["candidate_ids"]) <= candidates.keys():
                return "ABORT_EVIDENCE_SCOPE_MISMATCH"
            if item["kind"] in {"rng-draw", "provisional-disposition", "attempt-failure"} and (item["run_id"], item["cycle_id"], item["attempt"]) != (fence["run_id"], fence["cycle_id"], fence["attempt"]):
                return "ABORT_EVIDENCE_SCOPE_MISMATCH"
            if item["kind"] == "affordance-assessment" and item["subject_id"] not in units | candidates:
                return "ABORT_EVIDENCE_SCOPE_MISMATCH"
        return None

    if operation == "derive-cycle-control-state":
        return None if state["candidate"] == _derive_control_state(state["decision_ledger"]) else "CONTROL_STATE_FOLD_MISMATCH"

    if operation == "validate-genesis":
        refs = state["genesis"]["policies"]
        registry = contracts["genesis_policy_bindings"]
        bindings = [item for item in registry if item["ref"] in refs]
        roles = [item["role"] for item in bindings]
        required = set(contracts["required_genesis_policy_roles"])
        if set(roles) != required or len(roles) != len(set(roles)) or len(refs) != len(bindings):
            return "GENESIS_POLICY_SET_INCOMPLETE"
        if state["genesis"]["parent_checkpoint_history_ref"] is not None:
            parent_state = {
                "reference_hex": state["genesis"]["parent_checkpoint_history_ref"],
                "origin_policy_store": state["origin_policy_store"],
                "checkpoint_store": state["checkpoint_store"],
            }
            error = _parent_history_ref_error(parent_state, resolve=True)
            if error:
                return error
        return None

    if operation == "validate-rng-draw":
        draw = state["draw"]
        pinned = state["genesis"]["rng_policy"]
        contract = next((item for item in contracts["rng_policy_contracts"] if (item["policy_id"], item["version"], item["policy_hash_hex"]) == (draw["algorithm"]["id"], draw["algorithm"]["version"], draw["algorithm"]["hash_hex"])), None)
        if draw["algorithm"] != pinned or contract is None or (state["mode"] == "production" and not contract["production_use"]):
            return "RNG_POLICY_MISMATCH"
        preimage = [bytes.fromhex(state["genesis"]["world_seed_hex"]), draw["subsystem"], draw["decision_key"], [bytes.fromhex(item) for item in draw["entity_ids_hex"]], draw["purpose"], [draw["algorithm"]["id"], draw["algorithm"]["version"], bytes.fromhex(draw["algorithm"]["hash_hex"])]]
        derived = hashlib.sha256(_encode_cbor_item(preimage)).hexdigest()
        expected_result = {
            "schema_id": "cote.csf.test.bytes32",
            "schema_version": 1,
            "canonical_envelope_hex": _encode_cbor_item(
                [
                    b"CSF\x00",
                    1,
                    "cote.csf.test.rng-result",
                    "cote.csf.test.bytes32",
                    1,
                    bytes.fromhex(derived),
                ]
            ).hex(),
            "value_hex": derived,
        }
        return None if draw["result"] == expected_result else "RNG_RESULT_MISMATCH"

    if operation == "validate-epistemic-checkpoint":
        checkpoint = state["checkpoint"]
        content = bytes.fromhex(checkpoint["resolved_content_hex"])
        if hashlib.sha256(content).hexdigest() != checkpoint["content_hash_hex"]:
            return "CHECKPOINT_CONTENT_HASH_MISMATCH"
        if checkpoint["mode"] == "inline" and checkpoint["inline_content_hex"] != checkpoint["resolved_content_hex"]:
            return "CHECKPOINT_INLINE_CONTENT_MISMATCH"
        return None

    if operation == "validate-parent-history-ref":
        return _parent_history_ref_error(state)

    if operation == "validate-snapshot":
        snapshot = state["snapshot"]
        world = snapshot["world_state"]
        typed_value = [world["schema_id"], world["schema_version"], bytes.fromhex(world["canonical_envelope_hex"]), bytes.fromhex(world["envelope_digest_hex"])]
        if _domain_digest("cote.csf.hash.world-state", "cote.csf.schema.typed-value", typed_value).hex() != snapshot["world_state_hash_hex"]:
            return "SNAPSHOT_WORLD_HASH_MISMATCH"
        if set(snapshot["ledger_cursors"]) != set(contracts["required_snapshot_ledgers"]):
            return "SNAPSHOT_LEDGER_SET_INCOMPLETE"
        if set(state["ledgers"]) != set(contracts["required_snapshot_ledgers"]):
            return "SNAPSHOT_AUTHORITY_SET_INCOMPLETE"
        genesis = state["genesis"]
        if any((snapshot["run_id"] != genesis["run_id"], snapshot["genesis_ref"] != genesis["genesis_ref"], snapshot["world_seed_hex"] != genesis["world_seed_hex"], snapshot["replay_policies"] != genesis["policies"], snapshot["codec_policy_id"] != genesis["codec_policy_id"], snapshot["codec_policy_hash_hex"] != genesis["codec_policy_hash_hex"], snapshot["schema_bundle_hash_hex"] != genesis["schema_bundle_hash_hex"])):
            return "SNAPSHOT_GENESIS_CONFIG_MISMATCH"
        for ledger_id, prefix in state["ledgers"].items():
            cursor, digest = snapshot["ledger_cursors"][ledger_id]
            if cursor != len(prefix) or digest != _ledger_prefix_digest(ledger_id, prefix):
                return "SNAPSHOT_LEDGER_DIGEST_MISMATCH"
        derived_pending_state = _derive_snapshot_pending_state(state["authorities"])
        expected_pending_state = _canonical_typed_fixture(
            "cote.csf.snapshot.pending-state",
            "cote.csf.schema.snapshot-pending-state",
            [
                derived_pending_state[name]
                for name in (
                    "occurrences",
                    "trigger_runtime",
                    "trigger_activations",
                    "source_closures",
                    "rounds",
                    "dispatches",
                    "responses",
                    "pending_units",
                    "perception_tasks",
                    "perception_completions",
                )
            ],
        )
        expected_pending_state["value"] = derived_pending_state
        if snapshot["pending_state"] != expected_pending_state:
            return "SNAPSHOT_PENDING_STATE_MISMATCH"
        if snapshot["cycle_control_state"] != _derive_control_state(state["decision_ledger_records"]):
            return "SNAPSHOT_CONTROL_STATE_MISMATCH"
        checkpoints = {item["actor"]: item for item in snapshot["epistemic_checkpoints"]}
        eligible_actors = {
            item["actor"]
            for item in state["authorities"]["actor_registry"]
            if item["cognitive_state_required"] and item["active_at_revision"] <= snapshot["revision"]
        }
        if set(checkpoints) != eligible_actors:
            return "SNAPSHOT_CHECKPOINT_SET_MISMATCH"
        required_cover = (snapshot["instant"], snapshot["revision"], snapshot["ledger_cursors"]["evidence_ledger"][0])
        if any(any(actual < required for actual, required in zip(item["covers_through"], required_cover)) for item in checkpoints.values()):
            return "SNAPSHOT_CHECKPOINT_STALE"
        if any(hashlib.sha256(bytes.fromhex(item["resolved_content_hex"])).hexdigest() != item["content_hash_hex"] for item in checkpoints.values()):
            return "CHECKPOINT_CONTENT_HASH_MISMATCH"
        return None

    raise AssertionError(operation)


def _commit_transition_error(state: dict, contracts: dict) -> str | None:
    fence = state["fence"]
    tx = state["transaction"]
    ledger = state["decision_ledger"]
    if "commit_successor_floor" in fence:
        return "COMMIT_SUCCESSOR_FLOOR_NOT_DERIVED"
    if any(item["run_id"] == fence["run_id"] and item["cycle_id"] == fence["cycle_id"] and item["attempt"] == fence["attempt"] for item in ledger["terminals"]):
        return "TERMINAL_ALREADY_EXISTS"
    if tx["commit_candidates"]:
        return "CANDIDATE_REPUBLISHED"
    candidates = {item["candidate_id"]: item for item in ledger["commit_candidates"]}
    if any(event["order_key"]["origin_ref"] in candidates and event["order_key"]["phase"] != "CANDIDATE_DOMAIN" for event in tx["events"]):
        return "EVENT_PHASE_MISMATCH"
    units, source_error = _resolve_source_units(state, fence, contracts)
    if source_error:
        return source_error
    decisions = {item["subject_id"]: item for item in tx["decisions"]}
    if decisions.keys() != units.keys():
        return "SETTLEMENT_NOT_BIJECTIVE"
    candidate_units: list[str] = []
    distinct_candidates: set[str] = set()
    for unit_id, decision in decisions.items():
        unit = units[unit_id]
        disposition = decision["disposition"]
        if disposition == "NO_PROPOSAL" and unit["root"] != "no-proposal":
            return "NO_PROPOSAL_SUBJECT_MISMATCH"
        if disposition == "DEDUPLICATED":
            identity = unit.get("idempotency_identity")
            canonical = units.get(decision.get("canonical_unit_id")) or state["historical_units"].get(decision.get("canonical_unit_id"))
            if (
                identity is None
                or decision.get("canonical_unit_id") == unit_id
                or canonical is None
                or canonical.get("idempotency_identity") != identity
            ):
                return "DEDUPLICATION_PROOF_MISMATCH"
            matching_units = {
                candidate_id: candidate_unit
                for candidate_id, candidate_unit in (units | state["historical_units"]).items()
                if candidate_unit.get("idempotency_identity") == identity
            }
            survivor = min(
                matching_units,
                key=lambda candidate_id: (
                    matching_units[candidate_id]["first_fence_ordinal"],
                    matching_units[candidate_id]["admission_index"],
                    candidate_id,
                ),
            )
            if decision["canonical_unit_id"] != survivor:
                return "DEDUPLICATION_PROOF_MISMATCH"
            continue
        if disposition in {"COMMIT", "REJECT", "DEFER"}:
            candidate = candidates.get(decision.get("candidate_id"))
            if candidate is None or unit_id not in candidate["source_unit_ids"]:
                return "CANDIDATE_PARTITION_MISMATCH"
            if candidate["candidate_id"] not in distinct_candidates:
                distinct_candidates.add(candidate["candidate_id"])
                candidate_units.extend(candidate["source_unit_ids"])
            projected = [event["event_id"] for event in tx["events"] if event["order_key"]["phase"] == "CANDIDATE_DOMAIN" and event["order_key"]["origin_ref"] == candidate["candidate_id"]]
            expected_events = projected if disposition == "COMMIT" else []
            if decision["produced_event_ids"] != expected_events:
                return "DISPOSITION_EVENT_MISMATCH"
            if disposition == "DEFER":
                successor = next((item for item in tx["successors"] if item.get("input_id", item.get("occurrence_id")) == decision.get("successor_input_id")), None)
                if successor is None or _defer_successor_error(successor, unit, fence):
                    return "DEFER_SUCCESSOR_MISMATCH"
    decisions_by_candidate: dict[str, list[dict]] = {}
    for decision in tx["decisions"]:
        if decision.get("candidate_id") is not None:
            decisions_by_candidate.setdefault(decision["candidate_id"], []).append(decision)
    shared_outcome_fields = (
        "disposition",
        "reason_code",
        "conflict_set_refs",
        "rng_draw_refs",
        "produced_event_ids",
        "successor_input_id",
    )
    if any(
        any(
            tuple(item[field] for field in shared_outcome_fields)
            != tuple(group[0][field] for field in shared_outcome_fields)
            for item in group[1:]
        )
        for group in decisions_by_candidate.values()
    ):
        return "CANDIDATE_OUTCOME_NOT_ATOMIC"
    expected_candidate_units = sorted(unit_id for unit_id, decision in decisions.items() if decision["disposition"] in {"COMMIT", "REJECT", "DEFER"})
    if sorted(candidate_units) != expected_candidate_units:
        return "CANDIDATE_PARTITION_MISMATCH"
    expected_successor_ids = {
        decision["successor_input_id"]
        for decision in tx["decisions"]
        if decision["disposition"] == "DEFER"
    }
    actual_successor_ids = [
        item.get("input_id", item.get("occurrence_id")) for item in tx["successors"]
    ]
    if set(actual_successor_ids) != expected_successor_ids or len(actual_successor_ids) != len(set(actual_successor_ids)):
        return "DEFER_SUCCESSOR_SET_MISMATCH"
    provisional = {item["subject_id"]: item for item in ledger["provisional_dispositions"]}
    for unit_id, decision in decisions.items():
        if decision["disposition"] in {"COMMIT", "REJECT", "DEFER"}:
            material = provisional.get(unit_id)
            compared = ("run_id", "cycle_id", "attempt", "subject_id", "disposition", "candidate_id", "reason_code", "conflict_set_refs", "rng_draw_refs")
            if material is None or any(material[name] != decision[name] for name in compared):
                return "PROVISIONAL_SETTLEMENT_MISMATCH"
            candidate = candidates[decision["candidate_id"]]
            if decision["policy"] not in state["genesis_policies"] or decision["resolver"] != candidate["resolver"]:
                return "PROVISIONAL_SETTLEMENT_MISMATCH"
            if any(ref not in {item["conflict_set_id"] for item in ledger["conflict_sets"]} for ref in decision["conflict_set_refs"]):
                return "CONFLICT_SET_UNRESOLVED"
            if any(ref not in {item["draw_id"] for item in ledger["rng_draws"]} for ref in decision["rng_draw_refs"]):
                return "RNG_DRAW_UNRESOLVED"
    if any(event["order_key"]["phase"] not in _EVENT_PHASE_ORDER for event in tx["events"]):
        return "EVENT_PHASE_MISMATCH"
    ordered_events = sorted(tx["events"], key=lambda event: tuple(_event_order_components(event["order_key"])))
    if ordered_events != tx["events"]:
        return "EVENT_ORDER_MISMATCH"
    order_keys = [tuple(_event_order_components(item["order_key"])) for item in tx["events"]]
    if len(order_keys) != len(set(order_keys)):
        return "EVENT_ORDER_KEY_DUPLICATE"
    prior = {item["event_id"]: item["event_digest"] for item in state["prior_events"]}
    current = {item["event_id"]: item for item in tx["events"]}
    for event in tx["events"]:
        phase = event["order_key"]["phase"]
        if event["event_id"] != _derived_event_id(fence, event["order_key"]):
            return "EVENT_ID_MISMATCH"
        if any(
            source["id"] not in units
            or units[source["id"]]["unit_digest"] != source["digest"]
            for source in event["source_inputs"]
        ):
            return "EVENT_SOURCE_OUTSIDE_FENCE"
        if phase == "CANDIDATE_DOMAIN":
            candidate = candidates.get(event["order_key"]["origin_ref"])
            if candidate is None:
                return "EVENT_DRAFT_PROJECTION_MISMATCH"
            draft = next((item for item in candidate["event_drafts"] if (item["event_role"], item["event_local_ordinal"]) == (event["order_key"]["event_role"], event["order_key"]["event_local_ordinal"])), None)
            expected_key = {"phase": "CANDIDATE_DOMAIN", "origin_ref": candidate["candidate_id"], "producer": candidate["producer"], "producer_version": candidate["producer_version"], "event_local_ordinal": draft["event_local_ordinal"] if draft else -1, "event_role": draft["event_role"] if draft else -1}
            expected_id = _derived_event_id(fence, expected_key)
            candidate_events_by_role = {
                item["order_key"]["event_role"]: item
                for item in tx["events"]
                if item["order_key"]["phase"] == "CANDIDATE_DOMAIN"
                and item["order_key"]["origin_ref"] == candidate["candidate_id"]
            }
            expected_parents = []
            for role in draft["causal_parent_roles"] if draft else []:
                parent = candidate_events_by_role.get(role)
                if parent is None or tuple(_event_order_components(parent["order_key"])) >= tuple(_event_order_components(event["order_key"])):
                    return "EVENT_CAUSAL_PARENT_ROLE_MISMATCH"
                expected_parents.append({"id": parent["event_id"], "digest": parent["event_digest"]})
            projection = (
                _candidate_event_projection(candidate, draft, units, contracts)
                if draft is not None
                else None
            )
            if draft is None or projection is None or event["order_key"] != expected_key or event["event_id"] != expected_id or event["event_type"] != draft["event_type"] or event["payload"] != draft["payload"] or event["causal_parents"] != expected_parents or any(event.get(field) != value for field, value in projection.items()):
                return "EVENT_DRAFT_PROJECTION_MISMATCH"
        elif phase == "TEMPORAL_ADVANCE":
            if fence["cycle_plan"][0] != "CLOCK_ADVANCE" or event["order_key"]["origin_ref"] != state["clock_advance_origin"]:
                return "TEMPORAL_ADVANCE_MISMATCH"
        elif phase in {"TRIGGER_RUNTIME", "TRIGGER_ACTIVATION"}:
            if event["event_type"] not in {"trigger.runtime.changed", "trigger.activation.created"}:
                return "TRIGGER_EVENT_MISMATCH"
        for parent in event["causal_parents"]:
            if parent["id"] in prior and prior[parent["id"]] == parent["digest"]:
                continue
            parent_event = current.get(parent["id"])
            if parent_event is None or parent_event["event_digest"] != parent["digest"]:
                return "CAUSAL_PARENT_UNRESOLVED"
            if (
                parent_event["order_key"]["origin_ref"] != event["order_key"]["origin_ref"]
                or tuple(_event_order_components(parent_event["order_key"]))
                >= tuple(_event_order_components(event["order_key"]))
            ):
                return "CAUSAL_PARENT_SCOPE_MISMATCH"
    lifecycle_subjects = {}
    for cohort_round in fence["cohort"]:
        round_record = state["round_authority"].get(cohort_round["round_id"])
        if round_record is None or set(cohort_round["unit_ids"]) - units.keys():
            return "SOURCE_LIFECYCLE_AUTHORITY_MISMATCH"
        lifecycle_subjects[("round-declaration", cohort_round["round_id"])] = {
            "lifecycle": round_record["lifecycle"],
            "terminal": "CONSUMED",
            "source_units": cohort_round["unit_ids"],
        }
    for unit_id, unit in units.items():
        if unit["root"] in {"scheduled-occurrence", "trigger-activation"}:
            terminal = "CONSUMED"
            if unit["root"] == "scheduled-occurrence":
                policy = next(
                    (
                        item
                        for item in state["lifecycle_policy_registry"]
                        if item["root"] == unit["root"]
                        and item["policy_ref"] == unit["lifecycle_policy"]
                    ),
                    None,
                )
                if policy is None:
                    return "SOURCE_LIFECYCLE_POLICY_UNRESOLVED"
                terminal = policy["terminal_by_disposition"].get(
                    decisions[unit_id]["disposition"]
                )
                if terminal not in {"CONSUMED", "CANCELLED"}:
                    return "SOURCE_LIFECYCLE_POLICY_UNRESOLVED"
            lifecycle_subjects[(unit["root"], unit_id)] = {
                "lifecycle": unit["lifecycle"],
                "terminal": terminal,
                "source_units": [unit_id],
            }
    lifecycle_events = [
        item for item in tx["events"] if item["order_key"]["phase"] == "SOURCE_LIFECYCLE"
    ]
    actual_lifecycle = {
        (item["payload"]["root"], item["payload"]["subject_id"]): item
        for item in lifecycle_events
    }
    if len(actual_lifecycle) != len(lifecycle_events):
        return "SOURCE_LIFECYCLE_DUPLICATE"
    if actual_lifecycle.keys() != lifecycle_subjects.keys():
        return "SOURCE_LIFECYCLE_MISMATCH"
    reduced_authorities = {owner: {} for owner in contracts["lifecycle_reducer_owners"].values()}
    for key, subject in lifecycle_subjects.items():
        root, subject_id = key
        event = actual_lifecycle[key]
        source_refs = {item["id"]: item["digest"] for item in event["source_inputs"]}
        expected_refs = {unit_id: units[unit_id]["unit_digest"] for unit_id in subject["source_units"]}
        order_key = event["order_key"]
        expected_event_id = _derived_event_id(fence, order_key)
        if (
            event["event_type"] != "source.lifecycle"
            or event["event_id"] != expected_event_id
            or event["order_key"]["origin_ref"] != subject_id
            or event["payload"] != {
                "root": root,
                "subject_id": subject_id,
                "from": subject["lifecycle"],
                "to": subject["terminal"],
            }
            or source_refs != expected_refs
        ):
            return "SOURCE_LIFECYCLE_MISMATCH"
        reduced_authorities[contracts["lifecycle_reducer_owners"][root]][subject_id] = subject["terminal"]
    if reduced_authorities != state["source_authority_after"]:
        return "SOURCE_LIFECYCLE_MISMATCH"
    settlement_by_unit = {item["unit_id"]: item for item in tx["source_settlements"]}
    if settlement_by_unit.keys() != units.keys():
        return "SOURCE_SETTLEMENT_MISSING"
    for unit_id, unit in units.items():
        required = contracts["source_settlement_authority"][unit["root"]]
        if settlement_by_unit[unit_id]["authority"] != required["authority"] or settlement_by_unit[unit_id]["proof"] != required["proof"]:
            return "SOURCE_SETTLEMENT_AUTHORITY_MISMATCH"
    role_refs = state["genesis_policy_bindings"]
    commit_refs = {"cycle-coordinate": tx["cycle_commit"]["cycle_coordinate_policy"], "event-order": tx["cycle_commit"]["event_order_policy"], "perception": tx["cycle_commit"]["perception_policy"], "perception-identity": tx["cycle_commit"]["perception_identity_policy"]}
    if commit_refs != {role: role_refs[role] for role in commit_refs}:
        return "COMMIT_POLICY_MISMATCH"
    result_state, reducer_error = _derive_result_state(state, tx["events"])
    if reducer_error:
        return reducer_error
    if result_state != state["result_state"]:
        return "WORLD_STATE_NOT_DERIVED"
    expected_runtime, expected_activations = _derive_trigger_outputs(state, fence, tx, result_state)
    if tx["trigger_runtime_states"] != expected_runtime or tx["trigger_activations"] != expected_activations:
        return "TRIGGER_TRANSITION_MISMATCH"
    runtime_event_payloads = [
        event["payload"]
        for event in tx["events"]
        if event["order_key"]["phase"] == "TRIGGER_RUNTIME"
    ]
    activation_event_payloads = [
        event["payload"]
        for event in tx["events"]
        if event["order_key"]["phase"] == "TRIGGER_ACTIVATION"
    ]
    if runtime_event_payloads != expected_runtime or activation_event_payloads != expected_activations:
        return "TRIGGER_EVENT_MISMATCH"
    before = state["prior_cycle_commits"][-1]["next_logical_sequence"] if state["prior_cycle_commits"] else contracts["initial_logical_sequence"]
    if state["logical_sequence_transition"] != {"before": before, "after": before + len(tx["events"])} or tx["cycle_commit"]["next_logical_sequence"] != before + len(tx["events"]):
        return "LOGICAL_SEQUENCE_MISMATCH"
    expected_event_ids = [event["event_id"] for event in ordered_events]
    if tx["cycle_commit"]["event_ids"] != expected_event_ids:
        return "CYCLE_COMMIT_EVENT_RECEIPT_MISMATCH"
    unit_order = list(units)
    expected_decision_ids = [decisions[unit_id]["decision_id"] for unit_id in unit_order]
    if tx["cycle_commit"]["admitted_input_ids"] != unit_order or tx["cycle_commit"]["decision_record_ids"] != expected_decision_ids:
        return "CYCLE_COMMIT_DECISION_RECEIPT_MISMATCH"
    decision_pairs = [
        [unit_id, decisions[unit_id]["decision_id"], decisions[unit_id]["record_digest"]]
        for unit_id in unit_order
    ]
    expected_decision_digest = _domain_digest(
        "cote.csf.digest.decision",
        "cote.csf.schema.decision-pair-list",
        decision_pairs,
    ).hex()
    if tx["cycle_commit"]["decision_digest"] != expected_decision_digest:
        return "CYCLE_COMMIT_DECISION_DIGEST_MISMATCH"
    expected_tasks = _derive_perception_tasks(state, fence, ordered_events)
    if tx["perception_tasks"] != expected_tasks:
        return "PERCEPTION_TASK_PROJECTION_MISMATCH"
    expected_task_ids = [item["task_id"] for item in expected_tasks]
    expected_task_digest = _domain_digest(
        "cote.csf.digest.perception-task-list",
        "cote.csf.schema.perception-task-id-list",
        expected_task_ids,
    ).hex()
    if (
        tx["cycle_commit"]["perception_task_ids"] != expected_task_ids
        or tx["cycle_commit"]["perception_task_digest"] != expected_task_digest
    ):
        return "CYCLE_COMMIT_PERCEPTION_RECEIPT_MISMATCH"
    return None


def _parse_parent_history_ref(reference_hex: str) -> dict | None:
    try:
        data = bytes.fromhex(reference_hex)
        if data[:8] != b"CSFHREF\x00" or data[8] != 1:
            raise ValueError
        offset = 9
        values = []
        for _ in range(4):
            size = int.from_bytes(data[offset:offset + 2], "big"); offset += 2
            values.append(data[offset:offset + size]); offset += size
        schema_version = int.from_bytes(data[offset:offset + 8], "big"); offset += 8
        for _ in range(2):
            size = int.from_bytes(data[offset:offset + 2], "big"); offset += 2
            values.append(data[offset:offset + size]); offset += size
        policy_id, policy_hash, domain, schema_id, algorithm, digest = values
        if offset != len(data) or len(policy_hash) != 32 or len(digest) != 32 or algorithm != b"sha-256" or schema_version < 1:
            raise ValueError
        decoded_policy_id, decoded_domain, decoded_schema_id = (
            value.decode("ascii") for value in (policy_id, domain, schema_id)
        )
    except (ValueError, UnicodeDecodeError, IndexError):
        return None
    return {
        "origin_policy_id": decoded_policy_id,
        "origin_policy_hash_hex": policy_hash.hex(),
        "domain": decoded_domain,
        "schema_id": decoded_schema_id,
        "schema_version": schema_version,
        "algorithm": algorithm.decode("ascii"),
        "checkpoint_digest_hex": digest.hex(),
    }


def _parent_history_ref_error(state: dict, *, resolve: bool = False) -> str | None:
    parsed = _parse_parent_history_ref(state["reference_hex"])
    if parsed is None:
        return "PARENT_HISTORY_REF_INVALID"
    if not resolve:
        return None
    policy = next(
        (
            item
            for item in state["origin_policy_store"]
            if item["policy_id"] == parsed["origin_policy_id"]
            and item["policy_hash_hex"] == parsed["origin_policy_hash_hex"]
        ),
        None,
    )
    if policy is None:
        return "PARENT_HISTORY_POLICY_UNRESOLVED"
    machine_id = re.compile(r"[a-z][a-z0-9]*(?:[._:/-][a-z0-9]+)*\Z")
    domain_tag = re.compile(
        r"[a-z][a-z0-9]*(?:-[a-z0-9]+)*(?:\.[a-z][a-z0-9]*(?:-[a-z0-9]+)*)+\Z"
    )
    if (
        machine_id.fullmatch(parsed["origin_policy_id"]) is None
        or domain_tag.fullmatch(parsed["domain"]) is None
        or domain_tag.fullmatch(parsed["schema_id"]) is None
    ):
        return "PARENT_HISTORY_REF_INVALID"
    checkpoint = next(
        (
            item
            for item in state["checkpoint_store"]
            if all(
                item[name] == parsed[name]
                for name in (
                    "domain",
                    "schema_id",
                    "schema_version",
                    "algorithm",
                    "checkpoint_digest_hex",
                )
            )
        ),
        None,
    )
    if checkpoint is None:
        return "PARENT_HISTORY_CHECKPOINT_UNRESOLVED"
    content = bytes.fromhex(checkpoint["resolved_content_hex"])
    if hashlib.sha256(content).hexdigest() != parsed["checkpoint_digest_hex"]:
        return "PARENT_HISTORY_CHECKPOINT_UNRESOLVED"
    try:
        envelope, offset = _decode_cbor_item(content)
    except (AssertionError, IndexError, UnicodeDecodeError):
        return "PARENT_HISTORY_CHECKPOINT_SCHEMA_INVALID"
    if (
        offset != len(content)
        or _encode_cbor_item(envelope) != content
        or len(envelope) != 6
        or envelope[:2] != [b"CSF\x00", 1]
        or envelope[2:5] != [parsed["domain"], parsed["schema_id"], parsed["schema_version"]]
        or [parsed["domain"], parsed["schema_id"], parsed["schema_version"]]
        not in policy["domain_operations"]
    ):
        return "PARENT_HISTORY_CHECKPOINT_SCHEMA_INVALID"
    payload = envelope[5]
    if policy["checkpoint_validator"] == "snapshot-v1-fixture":
        if (
            not isinstance(payload, list)
            or len(payload) != 2
            or not isinstance(payload[0], bytes)
            or not isinstance(payload[1], bytes)
            or hashlib.sha256(payload[0]).digest() != payload[1]
        ):
            return "PARENT_HISTORY_CHECKPOINT_SCHEMA_INVALID"
    return None


def test_causal_transition_contracts_execute_all_review_regressions() -> None:
    contracts = _json("causal-transition-contracts.json")
    fixtures = _json("causal-transition-fixtures.json")
    bases = {item["scenario_id"]: item for item in fixtures["base_scenarios"]}
    findings = set()
    for case in fixtures["cases"]:
        findings.update(case["findings"])
        state = _apply_fixture_patch(bases[case["base_scenario_id"]]["state"], case.get("patch", []))
        actual = _transition_error(bases[case["base_scenario_id"]]["operation"], state, contracts)
        assert actual == case.get("expected_error"), case["case_id"]
    assert findings == set(range(68, 153))


def test_transition_fixtures_use_canonical_authorities_not_parallel_oracles() -> None:
    fixtures = _json("causal-transition-fixtures.json")
    bases = {item["scenario_id"]: item["state"] for item in fixtures["base_scenarios"]}

    source = bases["source-input.valid"]
    assert "ingress_seq" not in source["candidate"]
    assert source["candidate_receipt"]["root"] == "source-ingress-receipt"
    assert source["candidate_receipt"]["record_ref"]["id"] == source["candidate"]["input_id"]

    dispatch = bases["slot-dispatch.valid"]
    assert "prior_perception_tasks" not in dispatch
    assert "required_knowledge_inputs" not in dispatch
    assert "confirmed_knowledge_input_ids" not in dispatch["evidence_ledger"]
    assert "recipient" not in dispatch["causal_outbox"]["tasks"][0]
    assert "causal_ordinal" not in dispatch["causal_outbox"]["tasks"][0]
    assert {item["task_id"] for item in dispatch["causal_outbox"]["tasks"]} == {
        item["task_id"] for item in dispatch["causal_outbox"]["completions"]
    }

    commit = bases["commit.valid"]
    candidate = commit["decision_ledger"]["commit_candidates"][0]
    assert "event_ids" not in candidate
    assert candidate["event_drafts"]
    assert "trigger_expected" not in commit
    assert "eligible_actors" not in commit
    assert "rounds" not in commit["transaction"]
    assert "occurrences" not in commit["transaction"]
    assert "trigger_activations_source" not in commit["transaction"]
    assert "policies" not in commit["fence"]
    assert "policies" not in commit["transaction"]["cycle_commit"]
    assert all(
        set(unit) == _CANONICAL_ADMITTED_UNIT_FIELDS
        for unit in commit["fence"]["admitted_units"]
    )
    assert (
        commit["source_authorities"]["input_ledger"]["unit-action"]["root"]
        == "action-proposal"
    )

    observation = bases["observation.valid"]
    assert "channel_state" not in observation
    assert set(observation["world_state_by_revision"]) == {"7", "8"}
    assert observation["actor_authority"]["actor-2"]["clearance"] == "PUBLIC"

    control = bases["control.in-flight.valid"]
    assert "candidate_status" not in control
    assert set(control["candidate"]) == {
        "status",
        "cycle_id",
        "attempt_ordinal",
        "fence_ref",
        "retry_ref",
        "last_terminal_envelope_ref",
        "next_attempt_ordinal",
    }

    genesis = bases["genesis.valid"]
    assert "policy_roles" not in genesis
    assert "policy_store" not in genesis
    assert genesis["genesis"]["policies"]

    rng = bases["rng.valid"]
    assert "result_hex" not in rng["draw"]
    assert rng["draw"]["result"]["schema_id"] == "cote.csf.test.bytes32"

    commit = bases["commit.valid"]
    assert commit["source_authority_after"]["schedule_store"]["round-1"] == "CONSUMED"

    snapshot = bases["snapshot.valid"]
    assert "eligible_actors" not in snapshot
    assert set(snapshot["ledgers"]) == {
        "input_ledger",
        "decision_ledger",
        "event_store",
        "evidence_ledger",
        "causal_outbox",
    }
    assert "policy_refs" not in snapshot["snapshot"]
    assert "world_state_hex" not in snapshot["snapshot"]
    assert snapshot["snapshot"]["pending_state"]["schema_id"] == (
        "cote.csf.schema.snapshot-pending-state"
    )
    assert "knowledge_inboxes" not in snapshot["snapshot"]["pending_state"]["value"]


def test_adr_0009_followup_rejects_reintroduced_parallel_or_stale_state() -> None:
    contracts = _json("causal-transition-contracts.json")
    fixtures = _json("causal-transition-fixtures.json")
    bases = {item["scenario_id"]: item["state"] for item in fixtures["base_scenarios"]}

    fence = deepcopy(bases["admission-fence.valid"])
    fence["clock"]["cycle_ordinal_at_instant"] = 1
    assert _transition_error("append-admission-fence", fence, contracts) == (
        "PENDING_COORDINATE_STALE"
    )
    fence = deepcopy(bases["admission-fence.valid"])
    for name in ("inputs", "occurrences", "activations", "rounds"):
        fence[name] = []
    assert _transition_error("append-admission-fence", fence, contracts) == "NO_PENDING_WORK"

    dispatch = deepcopy(bases["slot-dispatch.valid"])
    dispatch["round"]["lifecycle"] = "CONSUMED"
    assert _transition_error("append-slot-dispatch", dispatch, contracts) == (
        "DISPATCH_ROUND_NOT_PENDING"
    )
    dispatch = deepcopy(bases["slot-dispatch.valid"])
    dispatch["round"]["lifecycle"] = "CANCELLED"
    assert _transition_error("append-slot-dispatch", dispatch, contracts) == (
        "DISPATCH_ROUND_NOT_PENDING"
    )

    abort = deepcopy(bases["cycle-abort.valid"])
    abort["terminals"].append({"run_id": "run-1", "cycle_id": "cycle-1", "attempt": 1})
    assert _transition_error("append-cycle-abort", abort, contracts) == (
        "TERMINAL_ALREADY_EXISTS"
    )
    abort = deepcopy(bases["cycle-abort.valid"])
    abort["abort"]["indeterminate_subjects"][1]["digest"] = "foreign-digest"
    assert _transition_error("append-cycle-abort", abort, contracts) == (
        "ABORT_CANDIDATE_MISMATCH"
    )

    snapshot = deepcopy(bases["snapshot.valid"])
    snapshot["ledgers"].pop("event_store")
    assert _transition_error("validate-snapshot", snapshot, contracts) == (
        "SNAPSHOT_AUTHORITY_SET_INCOMPLETE"
    )
    snapshot = deepcopy(bases["snapshot.valid"])
    snapshot["authorities"]["actor_registry"].append(
        {
            "actor": "actor-2",
            "cognitive_state_required": True,
            "active_at_revision": 0,
        }
    )
    assert _transition_error("validate-snapshot", snapshot, contracts) == (
        "SNAPSHOT_CHECKPOINT_SET_MISMATCH"
    )


def test_adr_0009_followup_commit_projection_is_closed_and_derived() -> None:
    contracts = _json("causal-transition-contracts.json")
    fixtures = _json("causal-transition-fixtures.json")
    base = next(
        item["state"]
        for item in fixtures["base_scenarios"]
        if item["scenario_id"] == "commit.valid"
    )

    state = deepcopy(base)
    state["decision_ledger"]["terminals"].append(
        {"run_id": "run-1", "cycle_id": "cycle-1", "attempt": 1}
    )
    assert _commit_transition_error(state, contracts) == "TERMINAL_ALREADY_EXISTS"

    state = deepcopy(base)
    state["transaction"]["decisions"][0]["disposition"] = "DEDUPLICATED"
    state["transaction"]["decisions"][0]["canonical_unit_id"] = "unit-action"
    assert _commit_transition_error(state, contracts) == "DEDUPLICATION_PROOF_MISMATCH"

    state = deepcopy(base)
    state["decision_ledger"]["commit_candidates"][0]["source_unit_ids"].append(
        "unit-occurrence"
    )
    state["transaction"]["decisions"][2]["candidate_id"] = "candidate-1"
    assert _commit_transition_error(state, contracts) == "CANDIDATE_OUTCOME_NOT_ATOMIC"

    state = deepcopy(base)
    state["transaction"]["events"][0]["event_id"] = hashlib.sha256(
        _encode_cbor_item(_event_order_components(state["transaction"]["events"][0]["order_key"]))
    ).hexdigest()
    state["transaction"]["decisions"][0]["produced_event_ids"] = [
        state["transaction"]["events"][0]["event_id"]
    ]
    assert _commit_transition_error(state, contracts) == "EVENT_ID_MISMATCH"

    state = deepcopy(base)
    state["decision_ledger"]["commit_candidates"][0]["event_drafts"][0][
        "causal_parent_roles"
    ] = [1]
    assert _commit_transition_error(state, contracts) == (
        "EVENT_CAUSAL_PARENT_ROLE_MISMATCH"
    )

    state = deepcopy(base)
    state["transaction"]["successors"].append(
        {"root": "exogenous-input", "input_id": "unreceipted-extra"}
    )
    assert _commit_transition_error(state, contracts) == "DEFER_SUCCESSOR_SET_MISMATCH"

    state = deepcopy(base)
    duplicate = deepcopy(state["transaction"]["events"][1])
    duplicate["order_key"]["event_local_ordinal"] = 1
    duplicate["event_id"] = _derived_event_id(state["fence"], duplicate["order_key"])
    state["transaction"]["events"].insert(2, duplicate)
    assert _commit_transition_error(state, contracts) == "SOURCE_LIFECYCLE_DUPLICATE"

    state = deepcopy(base)
    state["round_authority"].clear()
    assert _commit_transition_error(state, contracts) == (
        "SOURCE_LIFECYCLE_AUTHORITY_MISMATCH"
    )

    state = deepcopy(base)
    state["transaction"]["events"][2]["payload"]["to"] = "CONSUMED"
    state["source_authority_after"]["schedule_store"]["unit-occurrence"] = "CONSUMED"
    assert _commit_transition_error(state, contracts) == "SOURCE_LIFECYCLE_MISMATCH"

    state = deepcopy(base)
    state["transaction"]["cycle_commit"]["cycle_coordinate_policy"] = [
        "caller-selected",
        1,
        "ff",
    ]
    assert _commit_transition_error(state, contracts) == "COMMIT_POLICY_MISMATCH"

    state = deepcopy(base)
    state["transaction"]["cycle_commit"]["event_ids"].pop()
    assert _commit_transition_error(state, contracts) == (
        "CYCLE_COMMIT_EVENT_RECEIPT_MISMATCH"
    )

    state = deepcopy(base)
    state["transaction"]["cycle_commit"]["decision_record_ids"].reverse()
    assert _commit_transition_error(state, contracts) == (
        "CYCLE_COMMIT_DECISION_RECEIPT_MISMATCH"
    )

    state = deepcopy(base)
    state["perception_policy"]["perceptible_event_types"] = ["domain.changed"]
    assert _commit_transition_error(state, contracts) == (
        "PERCEPTION_TASK_PROJECTION_MISMATCH"
    )

    state = deepcopy(base)
    state["result_state"]["resource.balance"] = 99
    assert _commit_transition_error(state, contracts) == "WORLD_STATE_NOT_DERIVED"

    state = deepcopy(base)
    state["decision_ledger"]["commit_candidates"][0]["event_drafts"][0][
        "payload"
    ] = ["resource.balance", 2]
    state["transaction"]["events"][0]["payload"] = ["resource.balance", 2]
    state["reducer_registry"]["domain.changed"] = {
        "operation": "set-path-from-payload",
        "dependency_footprint": ["resource.balance"],
    }
    state["result_state"]["resource.balance"] = 2
    assert _commit_transition_error(state, contracts) == "TRIGGER_TRANSITION_MISMATCH"


def test_findings_146_to_150_fail_closed_at_the_authoritative_seams() -> None:
    contracts = _json("causal-transition-contracts.json")
    fixtures = _json("causal-transition-fixtures.json")
    bases = {item["scenario_id"]: item["state"] for item in fixtures["base_scenarios"]}

    commit = bases["commit.valid"]

    state = deepcopy(commit)
    state["transaction"]["events"][0]["source_inputs"] = [
        {
            "id": "unit-none",
            "digest": "524bcb79e731c5d0a3b8be7762351757442eaacc16a09619e97141463878911e",
        }
    ]
    assert _commit_transition_error(state, contracts) == "EVENT_DRAFT_PROJECTION_MISMATCH"

    state = deepcopy(commit)
    state["transaction"]["events"][0]["actor_refs"] = ["actor-2"]
    assert _commit_transition_error(state, contracts) == "EVENT_DRAFT_PROJECTION_MISMATCH"

    state = deepcopy(commit)
    state["transaction"]["events"][0]["confidentiality"] = "PUBLIC"
    assert _commit_transition_error(state, contracts) == "EVENT_DRAFT_PROJECTION_MISMATCH"

    observation = deepcopy(bases["observation.valid"])
    observation["candidate"]["observer"] = "actor-2"
    assert _transition_error("append-observation", observation, contracts) == (
        "OBSERVATION_ACCESS_DENIED"
    )

    observation = deepcopy(bases["observation.valid"])
    observation["candidate"]["percepts"] = ["secret-detail"]
    assert _transition_error("append-observation", observation, contracts) == (
        "OBSERVATION_PROJECTION_MISMATCH"
    )

    dispatch = deepcopy(bases["slot-dispatch.valid"])
    assert "confirmed_knowledge_input_ids" not in dispatch["evidence_ledger"]
    assert _transition_error("append-slot-dispatch", dispatch, contracts) is None

    state = deepcopy(commit)
    state["transaction"]["events"][1]["event_id"] = "arbitrary-lifecycle-id"
    assert _commit_transition_error(state, contracts) == "EVENT_ID_MISMATCH"

    state = deepcopy(commit)
    state["fence"]["commit_successor_floor"] = [1000, 99]
    assert _commit_transition_error(state, contracts) == (
        "COMMIT_SUCCESSOR_FLOOR_NOT_DERIVED"
    )


def test_findings_151_and_152_reject_coordinated_authority_oracle_tampering() -> None:
    contracts = _json("causal-transition-contracts.json")
    fixtures = _json("causal-transition-fixtures.json")
    bases = {item["scenario_id"]: item["state"] for item in fixtures["base_scenarios"]}

    commit = deepcopy(bases["commit.valid"])
    commit["fence"]["admitted_units"][0]["confidentiality"] = "PUBLIC"
    commit["transaction"]["events"][0]["confidentiality"] = "PUBLIC"
    assert _commit_transition_error(commit, contracts) == (
        "ADMISSION_FENCE_UNIT_NOT_CANONICAL"
    )

    commit = deepcopy(bases["commit.valid"])
    commit["source_authorities"]["input_ledger"]["unit-action"]["record_body"][6] = (
        "actor-2"
    )
    commit["transaction"]["events"][0]["actor_refs"] = ["actor-2"]
    assert _commit_transition_error(commit, contracts) == (
        "SOURCE_RECORD_DIGEST_MISMATCH"
    )

    observation = deepcopy(bases["observation.valid"])
    observation["channel_state"] = {
        "event_access": {"event-secret-1": ["actor-1", "actor-2"]},
        "observer_outputs": {
            "event-secret-1": {
                "actor-2": {
                    "percepts": ["secret-detail"],
                    "claim_refs": ["claim-secret"],
                    "evidence_chain": ["evidence-secret"],
                    "omissions_redactions": [],
                    "modality": "VISION",
                    "channel_ref": "room-channel",
                }
            }
        },
    }
    observation["candidate"].update(
        {
            "observer": "actor-2",
            "percepts": ["secret-detail"],
            "claim_refs": ["claim-secret"],
            "evidence_chain": ["evidence-secret"],
            "omissions_redactions": [],
        }
    )
    assert _transition_error("append-observation", observation, contracts) == (
        "OBSERVATION_ACCESS_DENIED"
    )


def test_clock_advance_derives_destination_floor_and_validates_trigger_event_ids() -> None:
    contracts = _json("causal-transition-contracts.json")
    fixtures = _json("causal-transition-fixtures.json")
    base = next(
        item["state"]
        for item in fixtures["base_scenarios"]
        if item["scenario_id"] == "commit.valid"
    )
    state = deepcopy(base)
    fence = state["fence"]
    fence["cycle_plan"] = ["CLOCK_ADVANCE", 1000, 2000, [2000, 0]]
    fence["admitted_units"] = []
    fence["cohort"] = []
    assert _commit_successor_floor(fence) == [2000, 0]

    state["clock_advance_origin"] = "clock-advance-1"
    state["decision_ledger"]["commit_candidates"] = []
    state["decision_ledger"]["provisional_dispositions"] = []
    state["round_authority"] = {}
    state["source_authority_after"] = {
        "schedule_store": {},
        "trigger_registry": {},
    }
    state["world_state_before"] = {"clock.current": 1000}
    state["result_state"] = {"clock.current": 2000}
    state["reducer_registry"] = {
        "clock.advanced": {
            "operation": "set-path-from-payload",
            "dependency_footprint": ["clock.current"],
        },
        "trigger.runtime.changed": {
            "operation": "noop",
            "dependency_footprint": ["trigger.runtime"],
        },
        "trigger.activation.created": {
            "operation": "noop",
            "dependency_footprint": ["trigger.activation"],
        },
    }
    state["trigger_registry"] = {
        "definitions": [
            {
                "trigger_id": "trigger-clock",
                "dependencies": ["clock.current"],
                "predicate": {"path": "clock.current", "equals": 2000},
                "activation_policy": "RISING_EDGE",
                "fire_on_initial_true": False,
                "repeat_every": None,
            }
        ],
        "runtime_states": [
            {
                "trigger_id": "trigger-clock",
                "last_value": False,
                "armed": True,
                "exhausted": False,
                "activation_count": 0,
                "next_repeat_at": None,
                "evaluated_through_revision": 7,
            }
        ],
    }

    temporal_key = {
        "phase": "TEMPORAL_ADVANCE",
        "origin_ref": "clock-advance-1",
        "producer": "clock",
        "producer_version": 1,
        "event_local_ordinal": 0,
        "event_role": 0,
    }
    temporal = {
        "event_id": _derived_event_id(fence, temporal_key),
        "event_digest": "digest-clock",
        "order_key": temporal_key,
        "event_type": "clock.advanced",
        "payload": ["clock.current", 2000],
        "causal_parents": [],
        "source_inputs": [],
    }
    transaction = state["transaction"]
    transaction["decisions"] = []
    transaction["successors"] = []
    transaction["source_settlements"] = []
    transaction["events"] = [temporal]
    runtime, activations = _derive_trigger_outputs(
        state, fence, transaction, state["result_state"]
    )
    assert activations[0]["eligibility"] == [2000, 0]

    runtime_key = {
        "phase": "TRIGGER_RUNTIME",
        "origin_ref": "trigger-clock-runtime-1",
        "producer": "trigger-engine",
        "producer_version": 1,
        "event_local_ordinal": 0,
        "event_role": 0,
    }
    activation_key = {
        "phase": "TRIGGER_ACTIVATION",
        "origin_ref": "trigger-clock-activation-1",
        "producer": "trigger-engine",
        "producer_version": 1,
        "event_local_ordinal": 0,
        "event_role": 0,
    }
    transaction["events"].extend(
        [
            {
                "event_id": _derived_event_id(fence, runtime_key),
                "event_digest": "digest-trigger-runtime",
                "order_key": runtime_key,
                "event_type": "trigger.runtime.changed",
                "payload": runtime[0],
                "causal_parents": [],
                "source_inputs": [],
            },
            {
                "event_id": _derived_event_id(fence, activation_key),
                "event_digest": "digest-trigger-activation",
                "order_key": activation_key,
                "event_type": "trigger.activation.created",
                "payload": activations[0],
                "causal_parents": [],
                "source_inputs": [],
            },
        ]
    )
    transaction["trigger_runtime_states"] = runtime
    transaction["trigger_activations"] = activations
    commit = transaction["cycle_commit"]
    commit["admitted_input_ids"] = []
    commit["decision_record_ids"] = []
    commit["decision_digest"] = _domain_digest(
        "cote.csf.digest.decision", "cote.csf.schema.decision-pair-list", []
    ).hex()
    commit["event_ids"] = [event["event_id"] for event in transaction["events"]]
    commit["next_logical_sequence"] = 3
    state["logical_sequence_transition"] = {"before": 0, "after": 3}

    assert _commit_transition_error(state, contracts) is None
    state["transaction"]["events"][1]["event_id"] = "arbitrary-trigger-id"
    assert _commit_transition_error(state, contracts) == "EVENT_ID_MISMATCH"


def test_adr_0009_trigger_policies_are_derived_from_authoritative_state() -> None:
    fixtures = _json("causal-transition-fixtures.json")
    base = next(
        item["state"]
        for item in fixtures["base_scenarios"]
        if item["scenario_id"] == "commit.valid"
    )
    fence = base["fence"]
    tx = deepcopy(base["transaction"])
    tx["events"][0]["payload"] = ["resource.balance", 2]
    state = deepcopy(base)
    state["reducer_registry"]["domain.changed"] = {
        "operation": "set-path-from-payload",
        "dependency_footprint": ["resource.balance"],
    }

    cases = [
        ("RISING_EDGE", False, True, False, None, True),
        ("FALLING_EDGE", True, True, False, None, True),
        ("ONCE_WHEN_TRUE", False, True, False, None, True),
        ("REPEAT_WHILE_TRUE", True, True, False, 1000, True),
    ]
    for policy, last_value, armed, exhausted, next_repeat_at, expected_fire in cases:
        definition = state["trigger_registry"]["definitions"][0]
        definition["activation_policy"] = policy
        definition["fire_on_initial_true"] = policy == "RISING_EDGE"
        definition["repeat_every"] = 10 if policy == "REPEAT_WHILE_TRUE" else None
        runtime = state["trigger_registry"]["runtime_states"][0]
        runtime.update(
            {
                "last_value": last_value,
                "armed": armed,
                "exhausted": exhausted,
                "next_repeat_at": next_repeat_at,
            }
        )
        result = {"resource.balance": 1 if policy == "FALLING_EDGE" else 2}
        _, activations = _derive_trigger_outputs(state, fence, tx, result)
        assert bool(activations) is expected_fire, policy


def test_adr_0009_contracts_register_every_new_authority_and_derivation() -> None:
    contracts = _json("causal-transition-contracts.json")
    registries = _json("registries.json")
    operations = {tuple(item) for item in registries["domain_operations"]}
    fixture_schemas = {(item[0], item[1]) for item in registries["fixture_schemas"]}
    persisted = {item["root"] for item in registries["persisted_roots"]}

    assert contracts["initial_logical_sequence"] == 0
    assert len(contracts["genesis_policy_bindings"]) == len(
        contracts["required_genesis_policy_roles"]
    )
    assert {item["role"] for item in contracts["genesis_policy_bindings"]} == set(
        contracts["required_genesis_policy_roles"]
    )
    assert (
        "cote.csf.record.source-ingress-receipt",
        "cote.csf.schema.source-ingress-receipt",
        1,
    ) in operations
    assert (
        "cote.csf.digest.ledger-prefix",
        "cote.csf.schema.ledger-prefix-digest-preimage",
        1,
    ) in operations
    assert ("cote.csf.test.bytes32", 1) in fixture_schemas
    assert "source-ingress-receipt" in persisted


def test_adr_0009_parent_checkpoint_is_strictly_decoded_after_hashing() -> None:
    fixtures = _json("causal-transition-fixtures.json")
    case = next(
        item for item in fixtures["cases"] if item["case_id"] == "genesis.parent-resolved"
    )
    base = next(
        item["state"]
        for item in fixtures["base_scenarios"]
        if item["scenario_id"] == "genesis.valid"
    )
    state = _apply_fixture_patch(base, case["patch"])
    invalid = _encode_cbor_item(
        [
            b"CSF\x00",
            1,
            "cote.csf.hash.snapshot",
            "cote.csf.schema.snapshot",
            1,
            [b"origin-state", b"\x00" * 32],
        ]
    )
    digest = hashlib.sha256(invalid).digest()
    reference = bytearray(bytes.fromhex(state["genesis"]["parent_checkpoint_history_ref"]))
    reference[-32:] = digest
    state["genesis"]["parent_checkpoint_history_ref"] = reference.hex()
    checkpoint = state["checkpoint_store"][0]
    checkpoint["checkpoint_digest_hex"] = digest.hex()
    checkpoint["resolved_content_hex"] = invalid.hex()
    assert _transition_error("validate-genesis", state, _json("causal-transition-contracts.json")) == (
        "PARENT_HISTORY_CHECKPOINT_SCHEMA_INVALID"
    )


def test_transition_bundle_declares_normative_ownership_and_integrity_algorithms() -> None:
    contracts = _json("causal-transition-contracts.json")
    assert contracts["checkpoint_content_integrity"] == {
        "algorithm": "sha-256",
        "preimage": "exact resolved opaque content bytes",
        "inline": "hash inline bytes before accepting the checkpoint",
        "locator": "resolve bytes then hash before accepting the checkpoint",
    }
    assert contracts["commit_candidate_lifecycle"] == "persisted-provisional-before-terminal"
    assert contracts["parent_history_reference"]["parser"] == "cross-policy-checkpoint-reference-v1"
    assert set(contracts["required_snapshot_ledgers"]) == {
        "input_ledger",
        "decision_ledger",
        "event_store",
        "evidence_ledger",
        "causal_outbox",
    }
