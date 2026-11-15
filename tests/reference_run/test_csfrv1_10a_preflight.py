"""Pure negative controls for the #115 normative plan; no causal runtime required."""
from __future__ import annotations

import copy
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("preflight115", ROOT / "scripts/csfrv1_10a_preflight.py")
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)
DATA = ROOT / "docs/reference-run/csfrv1-10a"


class PreflightStructureTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.manifest = MODULE.load_json(DATA / "manifest.json")
        cls.scenario = MODULE.load_json(DATA / "scenario.json")
        cls.matrix = MODULE.load_json(DATA / "traceability.json")
        cls.adr = (ROOT / "docs/adr/0008-causal-simulation-foundation-v1.md").read_text(encoding="utf-8")

    def test_original_frozen_authority(self) -> None:
        MODULE.validate_frozen_authority(self.manifest)

    def test_rebound_baseline_sha_and_tree_rejected(self) -> None:
        m = copy.deepcopy(self.manifest)
        m["baseline"]["commit"] = "0" * 40
        m["baseline"]["tree"] = "f" * 40
        with self.assertRaisesRegex(MODULE.PreflightError, "baseline"):
            MODULE.validate_frozen_authority(m)

    def test_policy_manifest_rebind_rejected(self) -> None:
        m = copy.deepcopy(self.manifest)
        m["policy_bundle"]["codec_policy_hash"] = "f" * 64
        with self.assertRaisesRegex(MODULE.PreflightError, "policy"):
            MODULE.validate_frozen_authority(m)

    def test_toolchain_rebind_rejected(self) -> None:
        m = copy.deepcopy(self.manifest)
        m["toolchain"]["go"] = "1.99.9"
        with self.assertRaisesRegex(MODULE.PreflightError, "toolchain"):
            MODULE.validate_frozen_authority(m)

    def test_normative_source_blob_rebind_rejected(self) -> None:
        m = copy.deepcopy(self.manifest)
        m["normative"]["adr"]["git_blob"] = "0" * 40
        with self.assertRaisesRegex(MODULE.PreflightError, "normative"):
            MODULE.validate_frozen_authority(m)

    def test_frozen_plan_blob_rejects_seed_tamper(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            target = Path(td) / MODULE.FOLDER
            target.mkdir(parents=True)
            for name in MODULE.FROZEN_PLAN_BLOBS:
                (target / name).write_bytes((DATA / name).read_bytes())
            raw = (target / "scenario.json").read_text(encoding="utf-8")
            (target / "scenario.json").write_text(
                raw.replace("0001020304050607", "f001020304050607"), encoding="utf-8")
            with self.assertRaisesRegex(MODULE.PreflightError, "frozen #115 fixture"):
                MODULE.check_frozen_plan_blobs(Path(td))

    def test_pass_claim_rejected_even_with_fabricated_evidence(self) -> None:
        t = copy.deepcopy(self.matrix)
        row = t["invariants"][0]
        row["status"] = "EXISTING_PASS_EXACT_SHA"
        row["test"]["state"] = "EXISTING"
        row["evidence"] = {"commit_sha": "a" * 40,
                           "artifact": "fake-evidence.bin",
                           "exact_bytes_digest": "b" * 64}
        with self.assertRaisesRegex(MODULE.PreflightError, "cannot claim PASS"):
            MODULE.validate_traceability(t, self.adr)

    def test_exact_41_and_68_from_normative_adr(self) -> None:
        MODULE.validate_traceability(self.matrix, self.adr)
        self.assertEqual(len(self.matrix["invariants"]), 41)
        self.assertEqual(len(self.matrix["stress_scenarios"]), 68)
        self.assertTrue(all(row["status"] != "EXISTING_PASS_EXACT_SHA"
                            for row in self.matrix["invariants"] + self.matrix["stress_scenarios"]))

    def test_declarative_genesis_and_synthetic_plan(self) -> None:
        MODULE.validate_scenario(self.manifest, self.scenario)

    def test_mutated_normative_rule_is_rejected(self) -> None:
        tampered = copy.deepcopy(self.matrix)
        tampered["invariants"][0]["normative_text"] += " altered"
        with self.assertRaisesRegex(MODULE.PreflightError, "diverges"):
            MODULE.validate_traceability(tampered, self.adr)

    def test_missing_stress_is_rejected(self) -> None:
        tampered = copy.deepcopy(self.matrix)
        tampered["stress_scenarios"].pop()
        with self.assertRaisesRegex(MODULE.PreflightError, "coverage"):
            MODULE.validate_traceability(tampered, self.adr)

    def test_false_existing_pass_without_sha_is_rejected(self) -> None:
        tampered = copy.deepcopy(self.matrix)
        tampered["invariants"][8]["status"] = "EXISTING_PASS_EXACT_SHA"
        with self.assertRaises(MODULE.PreflightError):
            MODULE.validate_traceability(tampered, self.adr)

    def test_test_name_alias_is_rejected(self) -> None:
        tampered = copy.deepcopy(self.matrix)
        tampered["invariants"][12]["test"]["name"] = "TestReferenceInvariant14"
        with self.assertRaisesRegex(MODULE.PreflightError, "test alias"):
            MODULE.validate_traceability(tampered, self.adr)

    def test_duplicate_json_keys_are_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            path = Path(td) / "collision.json"
            path.write_text('{"source":"A","source":"B"}', encoding="utf-8")
            with self.assertRaises(MODULE.PreflightError):
                MODULE.load_json(path)

    def test_duplicate_source_alias_is_rejected(self) -> None:
        p = copy.deepcopy(self.scenario)
        p["sources"][1]["id"] = p["sources"][0]["id"]
        with self.assertRaisesRegex(MODULE.PreflightError, "source alias"):
            MODULE.validate_scenario(self.manifest, p)

    def test_duplicate_producer_key_is_rejected(self) -> None:
        p = copy.deepcopy(self.scenario)
        p["sources"][0]["inputs"][1]["key"] = p["sources"][0]["inputs"][0]["key"]
        with self.assertRaisesRegex(MODULE.PreflightError, "producer_unit_key"):
            MODULE.validate_scenario(self.manifest, p)

    def test_hardcoded_winner_is_rejected(self) -> None:
        p = copy.deepcopy(self.scenario)
        p["conflict"]["winner"] = "actor.a"
        with self.assertRaisesRegex(MODULE.PreflightError, "winner"):
            MODULE.validate_scenario(self.manifest, p)

    def test_secret_to_public_is_rejected(self) -> None:
        p = copy.deepcopy(self.scenario)
        p["secrets"][0]["forbidden_to"] = ["actor.b"]
        with self.assertRaisesRegex(MODULE.PreflightError, "secret"):
            MODULE.validate_scenario(self.manifest, p)

    def test_python_local_seq_promoted_to_global_is_rejected(self) -> None:
        p = copy.deepcopy(self.scenario)
        p["physical"]["global_sequence_from"] = "Python local seq"
        with self.assertRaisesRegex(MODULE.PreflightError, "telemetry"):
            MODULE.validate_scenario(self.manifest, p)

    def test_closure_key_alias_change_rejected(self) -> None:
        p = copy.deepcopy(self.scenario)
        p["sources"][0]["closures"][1]["producer_key"] = "school.final.999"
        with self.assertRaisesRegex(MODULE.PreflightError, "closure aliases"):
            MODULE.validate_scenario(self.manifest, p)

    def test_promoted_unsigned_genesis_is_rejected(self) -> None:
        m = copy.deepcopy(self.manifest)
        m["genesis"]["status"] = "CANONICAL_VERIFIED"
        with self.assertRaisesRegex(MODULE.PreflightError, "Genesis"):
            MODULE.validate_scenario(m, self.scenario)


class ByteOracleTests(unittest.TestCase):
    def test_exact_bytes_and_all_required_artifacts(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            first, second = Path(td) / "A", Path(td) / "B"
            first.mkdir()
            second.mkdir()
            for name in MODULE.ORACLE_FILES:
                (first / name).write_bytes(b"\x00\x01\xfe")
                (second / name).write_bytes(b"\x00\x01\xfe")
            self.assertEqual(len(MODULE.compare_authoritative(first, second)), len(MODULE.ORACLE_FILES))
            (second / "event-store.bin").write_bytes(b"\x00\x01\xff")
            with self.assertRaisesRegex(MODULE.PreflightError, "bytewise mismatch"):
                MODULE.compare_authoritative(first, second)

    def test_symlink_alias_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            first, second = Path(td) / "A", Path(td) / "B"
            first.mkdir()
            second.mkdir()
            for name in MODULE.ORACLE_FILES:
                (first / name).write_bytes(b"real")
                (second / name).write_bytes(b"real")
            (second / "event-store.bin").unlink()
            (second / "event-store.bin").symlink_to(first / "event-store.bin")
            with self.assertRaisesRegex(MODULE.PreflightError, "symlinked"):
                MODULE.compare_authoritative(first, second)

    def test_hardlink_same_inode_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            first, second = Path(td) / "A", Path(td) / "B"
            first.mkdir()
            second.mkdir()
            for name in MODULE.ORACLE_FILES:
                (first / name).write_bytes(b"real")
                (second / name).write_bytes(b"real")
            (second / "event-store.bin").unlink()
            (second / "event-store.bin").hardlink_to(first / "event-store.bin")
            with self.assertRaisesRegex(MODULE.PreflightError, "same-inode"):
                MODULE.compare_authoritative(first, second)

    def test_missing_artifact_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as td:
            first, second = Path(td) / "A", Path(td) / "B"
            first.mkdir()
            second.mkdir()
            with self.assertRaisesRegex(MODULE.PreflightError, "missing, symlinked"):
                MODULE.compare_authoritative(first, second)


if __name__ == "__main__":
    unittest.main()
