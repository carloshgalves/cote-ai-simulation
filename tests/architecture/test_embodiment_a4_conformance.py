"""CSFRV1-9A/A4: executable gate before any physical CSF commit.

This tests the A1/A2/A3 contract and Go boundary against REAL physical runs.
It does not invoke the unmerged #110 Go adapter or claim authoritative admission.
"""
from __future__ import annotations

import copy
import json
from pathlib import Path

import jsonschema
import pytest

from embodiment.cli import advance_clock_command, seed_cohort_command
from embodiment.types import SleepQuality
from tests.architecture.embodiment_a2_transition_reference import (
    project_verified_transitions, reference_bytes, verify_projected_transitions,
)
from tests.architecture.embodiment_a3_disclosure import (
    DisclosureRejected, validate_policy, validate_source,
    validate_qualitative_candidate, refuse_agent_context,
)
from tests.architecture.test_embodiment_a2_transitions import _receipts
from tests.architecture.test_embodiment_ipc_contract import SCHEMA_PATH as IPC_SCHEMA

ROOT = Path(__file__).resolve().parents[2]
A2_SCHEMA = ROOT / "docs/architecture/embodiment-causal-adapter-v1/schemas/verified-transition-evidence-v1.schema.json"
GO_BOUNDARY = ROOT / "internal/csf/contextboundary/builder.go"
BUNDLE = ROOT / "docs/architecture/canonical-codec-v1-bundle/BUNDLE.sha256"
REGISTRY = ROOT / "docs/architecture/embodiment-causal-adapter-v1/verified-transition-registry-proposal-v1.json"


def _fixture(parent: Path) -> dict:
    run = parent / "run"
    seed_cohort_command(world_seed=42, count=3, out_dir=run)
    advance_clock_command(run_dir=run, character_id="npc.0001",
                          days=2, sleep="7.3h", quality=SleepQuality.FAIR)
    advance_clock_command(run_dir=run, character_id="npc.0002",
                          days=1, sleep="0.1h", quality=SleepQuality.POOR)
    snapshot = (run / "snapshot.yaml").read_bytes()
    log = (run / "events.jsonl").read_bytes()
    receipts = _receipts(run)
    evidence = project_verified_transitions(snapshot, log, **receipts)
    verify_projected_transitions(evidence, snapshot, log, **receipts)
    return evidence


@pytest.fixture(scope="module")
def corpus(tmp_path_factory):
    path = tmp_path_factory.mktemp("csfrv1-a4-real")
    a, b = path / "a", path / "b"
    a.mkdir()
    b.mkdir()
    return _fixture(a), _fixture(b)


def _inert_candidate(evidence, subject="npc.0001"):
    tip = next(x for x in evidence["subject_tips"] if x["subject_id"] == subject)
    return {
        "kind": "physical.qualitative-observation-candidate.v1",
        "schema_version": 1,
        "authority": "NOT_ADMITTED",
        "recipient_id": subject,
        "observer_id": subject,
        "subject_id": subject,
        "signal": "fatigue_low",
        "source_tip_sha256": tip["verified_body_sha256"],
    }


def test_two_real_runs_repeat_semantically_without_host_wall_time_authority(corpus):
    a, b = corpus
    for source in (a, b):
        validate_source(source)
        assert source["completion"] == "ATTESTED_COMPLETE"
        assert source["admission"] == "PENDING_GO_ADMISSION"
        assert source["authority"] == "ENGINE_ONLY"
        assert source["source"]["verified_transition_count"] == 6
        assert len(source["command_receipts"]) == 2
    assert a["source"]["normalised_log_sha256"] == b["source"]["normalised_log_sha256"]
    assert a["transitions"] == b["transitions"]
    assert a["command_receipts"] == b["command_receipts"]
    assert a["subject_tips"] == b["subject_tips"]
    for key in ("snapshot_document_sha256", "normalised_log_sha256",
                "verified_transition_count", "completed_phase_count",
                "completed_transition_count"):
        assert a["source"][key] == b["source"][key]
    assert reference_bytes(a) and reference_bytes(b)


@pytest.mark.parametrize("mutation", [
    "schema_version", "physical_time_as_global", "world_revision",
    "draft", "unknown_physical_channel", "body_numeric_domain",
    "tip_hash", "local_seq_float", "unknown_source_type",
])
def test_a4_rejects_v1_authority_or_schema_mutants(corpus, mutation):
    base = corpus[0]
    altered = copy.deepcopy(base)
    if mutation == "schema_version":
        altered["schema_version"] = 2
    elif mutation == "physical_time_as_global":
        altered["transitions"][0]["SimulationInstant"] = 77
    elif mutation == "world_revision":
        altered["WorldRevision"] = 99
    elif mutation == "draft":
        altered["EventDraft"] = {"attested": True}
    elif mutation == "unknown_physical_channel":
        altered["subject_tips"][0]["verified_body"]["new_fatigue_signal"] = 0.1
    elif mutation == "body_numeric_domain":
        altered["subject_tips"][0]["verified_body"]["central_fatigue"] = -1
    elif mutation == "tip_hash":
        altered["subject_tips"][0]["verified_body_sha256"] = "wrong"
    elif mutation == "local_seq_float":
        altered["transitions"][0]["source_local_seq"] = 7.5
    elif mutation == "unknown_source_type":
        altered["source"]["snapshot_version"] = 2
    with pytest.raises(DisclosureRejected):
        validate_source(altered)


@pytest.mark.parametrize("attack", [
    "other_recipient", "other_observer", "wrong_digest",
    "private_body_payload", "freeform_text", "pretend_causal",
    "delete_subject", "new_field",
])
def test_no_physical_candidate_can_bypass_recipient_or_privacy(corpus, attack):
    base = corpus[0]
    candidate = _inert_candidate(base)
    if attack == "other_recipient":
        candidate["recipient_id"] = "npc.0002"
    elif attack == "other_observer":
        candidate["observer_id"] = "npc.0002"
    elif attack == "wrong_digest":
        candidate["source_tip_sha256"] = "f" * 64
    elif attack == "private_body_payload":
        candidate["BodyState"] = base["subject_tips"][0]["verified_body"]
    elif attack == "freeform_text":
        candidate["signal"] = "fatigue_low; fatigue_value=0.8"
    elif attack == "pretend_causal":
        candidate["authority"] = "WorldRevision"
    elif attack == "delete_subject":
        del candidate["subject_id"]
    elif attack == "new_field":
        candidate["rng_state"] = 42
    with pytest.raises(DisclosureRejected):
        validate_qualitative_candidate(candidate, base)


def test_reference_candidate_validates_shape_but_never_reaches_context(corpus):
    base = corpus[0]
    candidate = validate_qualitative_candidate(_inert_candidate(base), base)
    assert candidate["authority"] == "NOT_ADMITTED"
    with pytest.raises(DisclosureRejected):
        refuse_agent_context(candidate)
    with pytest.raises(DisclosureRejected):
        refuse_agent_context(base)


def test_a1_control_plane_schema_excludes_causal_authority_and_body():
    schema = json.loads(IPC_SCHEMA.read_text(encoding="utf-8"))
    jsonschema.Draft202012Validator.check_schema(schema)
    request = schema["$defs"]["request"]
    response = schema["$defs"]["response"]
    assert request["additionalProperties"] is False
    assert response["additionalProperties"] is False
    assert request["properties"]["operation"] == {"const": "inspect_run"}
    for name in ("WorldRevision", "CycleCommit", "EventDraft", "BodyState",
                 "SimulationInstant", "LogicalSequence"):
        assert name not in request["properties"] and name not in response["properties"]


def test_csf_core_is_not_registered_for_physical_world_commit():
    proposal = json.loads(REGISTRY.read_text(encoding="utf-8"))
    assert proposal["status"] == "PROPOSED_NON_CANONICAL_NOT_REGISTERED"
    assert proposal["runtime_extension_registered"] is False
    assert proposal["canonical_cbor_bundle_registered"] is False
    assert proposal["runtime_adapter_implemented"] is False
    registry = (ROOT / "docs/architecture/canonical-codec-v1-bundle/registries.json").read_text()
    assert '"physical.verified-transition-evidence.v1"' not in registry
    assert BUNDLE.is_file()
    # A contract candidate cannot be treated as a causal revision.
    context = GO_BOUNDARY.read_text(encoding="utf-8")
    assert 'context rejected non-addressed percept (canon/physical/privileged)' in context
    assert 'case "physical.' not in context
    assert 'case "cote.physical.' not in context


def test_exhaustive_a3_rules_are_versioned_and_go_only():
    validate_policy()
    schema = json.loads(A2_SCHEMA.read_text(encoding="utf-8"))
    assert schema["additionalProperties"] is False
    assert schema["properties"]["authority"]["const"] == "ENGINE_ONLY"
    assert schema["properties"]["admission"]["const"] == "PENDING_GO_ADMISSION"
    assert schema["properties"]["completion"]["enum"] == [
        "UNATTESTED_PREFIX", "ATTESTED_COMPLETE",
    ]
