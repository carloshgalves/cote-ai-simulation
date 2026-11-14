"""Adversarial CSFRV1-9A/A3 contract tests over real PSV1-2/A2 evidence."""
from __future__ import annotations

import copy
import json
from pathlib import Path

import pytest

from embodiment.cli import advance_clock_command, seed_cohort_command
from embodiment.types import SleepQuality
from tests.architecture.embodiment_a2_transition_reference import (
    project_verified_transitions,
)
from tests.architecture.embodiment_a3_disclosure import (
    BASE, POLICY, SCHEMA, DisclosureRejected,
    all_terminal_schema_paths, refuse_agent_context,
    validate_policy, validate_qualitative_candidate, validate_source,
)
from tests.architecture.test_embodiment_a2_transitions import _receipts


@pytest.fixture(scope="module")
def physical_run(tmp_path_factory):
    out = tmp_path_factory.mktemp("a3-physical") / "run"
    seed_cohort_command(world_seed=42, count=3, out_dir=out)
    advance_clock_command(
        run_dir=out, character_id="npc.0001", days=2,
        sleep="4h", quality=SleepQuality.POOR,
    )
    advance_clock_command(
        run_dir=out, character_id="npc.0002", days=1,
        sleep="8h", quality=SleepQuality.GOOD,
    )
    evidence = project_verified_transitions(
        (out / "snapshot.yaml").read_bytes(),
        (out / "events.jsonl").read_bytes(),
        **_receipts(out),
    )
    return evidence, out


def _policy():
    return json.loads(POLICY.read_text(encoding="utf-8"))


def _schema():
    return json.loads(SCHEMA.read_text(encoding="utf-8"))


def _candidate(evidence, subject="npc.0001", signal="fatigue_moderate"):
    tip = next(x for x in evidence["subject_tips"] if x["subject_id"] == subject)
    return {
        "kind": "physical.qualitative-observation-candidate.v1",
        "schema_version": 1,
        "authority": "NOT_ADMITTED",
        "recipient_id": subject,
        "observer_id": subject,
        "subject_id": subject,
        "signal": signal,
        "source_tip_sha256": tip["verified_body_sha256"],
    }


def test_every_a2_terminal_field_has_closed_explicit_disclosure_rule():
    policy, schema = _policy(), _schema()
    validate_policy()
    paths = all_terminal_schema_paths(schema)
    assert len(paths) == 88
    assert set(policy["fields"]) == paths
    assert all(rule["direct_disclosure"] is False for rule in policy["fields"].values())
    assert {r["classification"] for r in policy["fields"].values()} == {
        "ENGINE_ONLY", "QUALITATIVE_DERIVATION_ONLY", "FORBIDDEN_IN_CONTEXT"
    }
    assert all(
        rule["classification"] != "ENGINE_ONLY"
        for path, rule in policy["fields"].items()
        if path.startswith("subject_tips[].verified_body.")
    )


def test_actual_two_actor_a2_evidence_is_valid_but_not_agent_context(physical_run):
    evidence, out = physical_run
    before = [(out / p).read_bytes() for p in ("snapshot.yaml", "events.jsonl")]
    validate_source(evidence)
    assert evidence["completion"] == "ATTESTED_COMPLETE"
    assert len(evidence["subject_tips"]) == 2
    assert evidence["subject_tips"][0]["verified_body"]["character_id"] == "npc.0001"
    with pytest.raises(DisclosureRejected, match="contract rejected"):
        refuse_agent_context(evidence)
    assert before == [(out / p).read_bytes() for p in ("snapshot.yaml", "events.jsonl")]


@pytest.mark.parametrize("field", [
    "new_body_channel", "unknown_sampler_value", "unreviewed_nested_percept"
])
def test_schema_drift_requires_new_explicit_field_policy(field):
    schema = _schema()
    schema["$defs"]["body_state_psv1_2"]["properties"][field] = {
        "type": "number", "minimum": 0, "maximum": 1,
    }
    schema["$defs"]["body_state_psv1_2"]["required"].append(field)
    with pytest.raises(DisclosureRejected):
        validate_policy(schema=schema)


@pytest.mark.parametrize("operation", [
    "delete_rule", "add_rule", "grant_direct", "classify_body_as_engine_only",
    "wrong_scope", "skip_go_auth", "wrong_policy_version", "disable_fail_closed",
    "replace_values", "unregistered_signal", "wrong_schema_id",
])
def test_policy_tamper_is_rejected_without_implicit_permissions(operation):
    p = _policy()
    leaf = "subject_tips[].verified_body.central_fatigue"
    if operation == "delete_rule":
        del p["fields"][leaf]
    elif operation == "add_rule":
        p["fields"]["subject_tips[].verified_body.future_channel"] = {
            "classification": "ENGINE_ONLY", "direct_disclosure": False,
            "requires_go_authentication": False, "recipient_scope": "NONE"
        }
    elif operation == "grant_direct":
        p["fields"][leaf]["direct_disclosure"] = True
    elif operation == "classify_body_as_engine_only":
        p["fields"][leaf] = {
            "classification": "ENGINE_ONLY", "direct_disclosure": False,
            "requires_go_authentication": False, "recipient_scope": "NONE"
        }
    elif operation == "wrong_scope":
        p["fields"][leaf]["recipient_scope"] = "ALL_ACTORS"
    elif operation == "skip_go_auth":
        p["fields"][leaf]["requires_go_authentication"] = False
    elif operation == "wrong_policy_version":
        p["policy_version"] = "csfrv1-9a-a3.v999"
    elif operation == "disable_fail_closed":
        p["default_classification"] = "PUBLIC"
    elif operation == "replace_values":
        p["future_qualitative_candidate"]["values"] = []
    elif operation == "unregistered_signal":
        p["future_qualitative_candidate"]["authority"] = "ADMITTED"
    elif operation == "wrong_schema_id":
        p["source_schema_id"] = "fake"
    with pytest.raises(DisclosureRejected):
        validate_policy(policy=p)


def test_candidate_is_only_shape_checked_and_never_admitted(physical_run):
    evidence, _ = physical_run
    proposal = _candidate(evidence)
    result = validate_qualitative_candidate(proposal, evidence)
    assert result == proposal
    assert result["authority"] == "NOT_ADMITTED"
    assert "WorldRevision" not in result and "BodyState" not in result
    with pytest.raises(DisclosureRejected):
        refuse_agent_context(result)


@pytest.mark.parametrize("mutation", [
    "foreign_recipient", "foreign_observer", "foreign_subject",
    "missing_provenance", "other_actor_provenance", "unreviewed_signal",
    "direct_body", "numeric_leak", "source_seed", "go_id",
    "world_revision", "wrong_version", "pretend_admitted",
    "remove_subject", "unknown_optional", "arbitrary_text",
])
def test_untrusted_candidate_never_leaks_body_or_crosses_recipients(physical_run, mutation):
    evidence, _ = physical_run
    c = _candidate(evidence)
    if mutation == "foreign_recipient":
        c["recipient_id"] = "npc.0002"
    elif mutation == "foreign_observer":
        c["observer_id"] = "npc.0002"
    elif mutation == "foreign_subject":
        c["subject_id"] = "npc.0002"
    elif mutation == "missing_provenance":
        c["source_tip_sha256"] = "f" * 64
    elif mutation == "other_actor_provenance":
        c["source_tip_sha256"] = evidence["subject_tips"][1]["verified_body_sha256"]
    elif mutation == "unreviewed_signal":
        c["signal"] = "cognitive_thoughts"
    elif mutation == "direct_body":
        c["verified_body"] = evidence["subject_tips"][0]["verified_body"]
    elif mutation == "numeric_leak":
        c["central_fatigue"] = 0.94
    elif mutation == "source_seed":
        c["world_seed"] = 42
    elif mutation == "go_id":
        c["EventOrderKey"] = ["physical", 99]
    elif mutation == "world_revision":
        c["WorldRevision"] = 2
    elif mutation == "wrong_version":
        c["schema_version"] = 2
    elif mutation == "pretend_admitted":
        c["authority"] = "GO"
    elif mutation == "remove_subject":
        del c["subject_id"]
    elif mutation == "unknown_optional":
        c["notes"] = "feels tired"
    elif mutation == "arbitrary_text":
        c["signal"] = "fatigue_moderate; private: " + str(evidence["subject_tips"][0]["verified_body"])
    with pytest.raises(DisclosureRejected, match="contract rejected"):
        validate_qualitative_candidate(c, evidence)


@pytest.mark.parametrize("mutate", [
    "body_unknown_field", "version", "hash", "source_truncated",
    "unregistered_private", "wrong_body_actor", "bad_unit",
])
def test_invalid_physical_envelope_never_reaches_qualitative_proposal(
    physical_run, mutate
):
    evidence, _ = physical_run
    e = copy.deepcopy(evidence)
    if mutate == "body_unknown_field":
        e["subject_tips"][0]["verified_body"]["hidden_adversarial"] = 10
    elif mutate == "version":
        e["schema_version"] = 10
    elif mutate == "hash":
        e["subject_tips"][0]["verified_body_sha256"] = "not_sha256"
    elif mutate == "source_truncated":
        del e["source"]["normalised_log_sha256"]
    elif mutate == "unregistered_private":
        e["posterior"] = {"secret": True}
    elif mutate == "wrong_body_actor":
        e["subject_tips"][0]["verified_body"]["character_id"] = "../admin"
    elif mutate == "bad_unit":
        e["subject_tips"][0]["verified_body"]["central_fatigue"] = -10.0
    with pytest.raises(DisclosureRejected, match="contract rejected"):
        validate_qualitative_candidate(_candidate(evidence), e)


def test_all_private_paths_are_non_disclosable_without_go_attestation():
    p = _policy()
    for name, rule in p["fields"].items():
        if name.startswith("subject_tips[].verified_body."):
            assert rule["direct_disclosure"] is False
        if rule["classification"] == "QUALITATIVE_DERIVATION_ONLY":
            assert rule["requires_go_authentication"]
            assert rule["recipient_scope"] == "SELF_OBSERVER_ONLY"
    assert p["future_qualitative_candidate"]["authority"] == "NOT_ADMITTED"
    for token in ["BodyState", "CapacityProfile", "rng_state", "third_party_body"]:
        assert token in p["forbidden_artifact_classes"]


def test_contextboundary_go_has_no_physical_typed_tag_allowlist():
    source = (Path(__file__).resolve().parents[2] /
              "internal/csf/contextboundary/builder.go").read_text(encoding="utf-8")
    assert 'default:' in source
    assert 'context rejected non-addressed percept (canon/physical/privileged)' in source
    assert '"cote.csf.round.consumed.v1"' in source
    assert '"cote.csf.communication.delivered.v1"' in source
    assert 'case "physical.' not in source
    assert 'case "cote.physical.' not in source


def test_negative_contract_errors_never_echo_secret_payload(physical_run):
    evidence, _ = physical_run
    c = _candidate(evidence)
    secret = str(evidence["subject_tips"][0]["verified_body"])
    c["signal"] = secret
    with pytest.raises(DisclosureRejected) as exc:
        validate_qualitative_candidate(c, evidence)
    assert secret not in str(exc.value)
    assert "central_fatigue" not in str(exc.value)
    assert "world_seed" not in str(exc.value)


def test_trailing_schema_field_and_unknown_ref_are_both_denied():
    schema = _schema()
    schema["properties"]["physical_context"] = {"type": "object"}
    schema["required"].append("physical_context")
    with pytest.raises(DisclosureRejected):
        validate_policy(schema=schema)
    schema = _schema()
    schema["properties"]["source"] = {"$ref": "#/$defs/unknown_physical_ref"}
    with pytest.raises(DisclosureRejected):
        validate_policy(schema=schema)



@pytest.mark.parametrize(("path", "field", "bad"), [
    ("central_fatigue", "maximum", 2.0),
    ("central_fatigue", "minimum", -2.0),
    ("central_fatigue", "type", "string"),
])
def test_same_schema_paths_but_changed_numeric_semantics_are_rejected(path, field, bad):
    schema = _schema()
    schema["$defs"]["body_state_psv1_2"]["properties"][path][field] = bad
    assert all_terminal_schema_paths(schema) == all_terminal_schema_paths(_schema())
    with pytest.raises(DisclosureRejected):
        validate_policy(schema=schema)


def test_same_schema_with_forged_blob_pin_is_rejected():
    p = _policy()
    p["source_schema_git_blob_sha"] = "0" * 40
    with pytest.raises(DisclosureRejected):
        validate_policy(policy=p)
