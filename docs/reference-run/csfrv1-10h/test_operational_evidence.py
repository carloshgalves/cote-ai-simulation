"""H2-E structural and adversarial controls (real processes exercised in CI)."""
import copy
import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import operational_evidence as h


class EvidenceTests(unittest.TestCase):
    def receipt(self):
        sha = "a" * 40
        binary_sha = "b" * 64
        pins = {"index_sha256": "c" * 64, "manifest_sha256": "d" * 64,
                "run_id": "0" * 64, "genesis_sha256": "e" * 64,
                "registration_sha256": "f" * 64,
                "manifest_started_at_utc": "2026-01-01T00:00:01Z"}
        steps = []
        for operation, start, end, markers in (
            ("out", 0, 2, h.MARKERS), ("verify", 3, 4, h.VERIFY_REQUIRED)
        ):
            steps.append({"operation": operation, "command": h.COMMANDS[operation],
                          "started_at_utc": f"2026-01-01T00:00:0{start}Z",
                          "finished_at_utc": f"2026-01-01T00:00:0{end}Z",
                          "process_returncode": 0, "redacted_log": sorted(markers | h.SUCCESS_MARKERS),
                          "public_identity": {"source_sha": sha, "run_id": pins["run_id"],
                                              "registration_sha256": pins["registration_sha256"]}
                          if operation == "out" else {}})
        receipt = {"schema": h.SCHEMA, "source_sha": sha, "binary_sha256": binary_sha,
                   "disposition": "LOCAL_OBSERVED_NOT_EXTERNALLY_ATTESTED", "production_approval": False,
                   "archive": pins, "steps": steps}
        return receipt, sha, binary_sha, pins

    def test_real_zero_run_and_negative_receipt_semantics(self):
        receipt, sha, binary_sha, pins = self.receipt()
        h.validate(receipt, sha, binary_sha, pins)
        for mutate in (
            lambda r: r["steps"][0].update(process_returncode=18),
            lambda r: r["steps"].reverse(),
            lambda r: r["steps"][1].update(started_at_utc="2026-01-01T00:00:00Z"),
            lambda r: r["steps"][0]["redacted_log"].append("PRIVATE_PAYLOAD"),
            lambda r: r["steps"][0]["redacted_log"].remove("FIRST_REFERENCE_RUN_LOCAL=EXECUTED_AND_CHECKED"),
            lambda r: r["steps"][1]["redacted_log"].append("LOCAL_REFERENCE_EXIT_CODE=18"),
            lambda r: r["archive"].update(index_sha256="0" * 64),
            lambda r: r["steps"][0]["public_identity"].update(run_id="1" * 64),
            lambda r: r["steps"][1]["redacted_log"].append("FIRST_REFERENCE_RUN_LOCAL=EXECUTED_AND_CHECKED"),
            lambda r: r.update(production_approval=True),
            lambda r: r.update(extra="PRIVATE_PAYLOAD"),
        ):
            with self.subTest(mutate=mutate):
                bad = copy.deepcopy(receipt)
                mutate(bad)
                with self.assertRaises(ValueError):
                    h.validate(bad, sha, binary_sha, pins)

    def test_capture_retains_only_exact_allowlisted_lines_and_actual_exit(self):
        output = b"LOCAL_REFERENCE_STATE=CORRUPT_OR_UNVERIFIED\nLOCAL_REFERENCE_EXIT_CODE=18\nPRIVATE_PAYLOAD\nPRODUCTION_APPROVAL=NO extra\n"
        with patch.object(h.subprocess, "run", return_value=SimpleNamespace(stdout=output, returncode=18)):
            step = h.capture(Path("/binary"), Path("/archive"), "verify")
        self.assertEqual(step["process_returncode"], 18)
        self.assertEqual(step["redacted_log"], [
            "LOCAL_REFERENCE_STATE=CORRUPT_OR_UNVERIFIED", "LOCAL_REFERENCE_EXIT_CODE=18"])
        self.assertFalse(h.success(step))
        self.assertEqual(step["public_identity"], {})

    def test_only_valid_unambiguous_precausal_pins_preserved(self):
        output = ("LOCAL_REFERENCE_SOURCE_SHA=" + "a" * 40 + "\n"
                  "LOCAL_REFERENCE_RUN_ID=" + "0" * 64 + "\n"
                  "LOCAL_REFERENCE_REGISTRATION_SHA256=" + "b" * 64 + "\n"
                  "LOCAL_REFERENCE_REGISTRATION_SHA256=PRIVATE_PAYLOAD\n").encode()
        with patch.object(h.subprocess, "run", return_value=SimpleNamespace(stdout=output, returncode=11)):
            step = h.capture(Path("/binary"), Path("/archive"), "out")
        self.assertEqual(step["public_identity"], {"source_sha": "a" * 40, "run_id": "0" * 64})
        self.assertNotIn("PRIVATE_PAYLOAD", str(step))

    def test_snapshot_refuses_links_and_detects_byte_changes(self):
        with tempfile.TemporaryDirectory() as work:
            root = Path(work)
            for name in ("checksums.json", "run-manifest.json"):
                path = root / name
                path.write_bytes(b"one")
                path.chmod(0o600)
            before = h.snapshot(root)
            (root / "checksums.json").write_bytes(b"two")
            self.assertNotEqual(before, h.snapshot(root))
            (root / "alias").symlink_to(root / "checksums.json")
            with self.assertRaises(ValueError):
                h.snapshot(root)

    def test_receipt_is_exclusive(self):
        receipt, *_ = self.receipt()
        with tempfile.TemporaryDirectory() as work:
            path = Path(work)
            h.write_receipt(path, receipt)
            self.assertEqual((path / "operational-receipt.json").stat().st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError):
                h.write_receipt(path, receipt)

    @unittest.skipUnless(os.getenv("H2E_REAL_ARCHIVE"), "requires exact-head real H2 archive")
    def test_real_receipt_rejects_semantic_mutants_and_failed_process(self):
        archive = Path(os.environ["H2E_REAL_ARCHIVE"])
        binary = Path(os.environ["H2E_REAL_BINARY"])
        evidence = Path(os.environ["H2E_REAL_EVIDENCE"])
        receipt_path = evidence / "operational-receipt.json"
        raw = receipt_path.read_bytes()
        pin = hashlib.sha256(raw).hexdigest()
        original = json.loads(raw)
        before = h.snapshot(archive)

        def invoke(operation, target_archive, target_evidence, receipt_sha=None, env=None):
            args = [sys.executable, str(Path(h.__file__).resolve()), operation,
                    "--binary", str(binary), "--archive", str(target_archive),
                    "--evidence", str(target_evidence)]
            if receipt_sha is not None:
                args.extend(["--receipt-sha256", receipt_sha])
            return subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                  check=False, env=env)

        self.assertEqual(invoke("verify", archive, evidence, pin).returncode, 0)
        mutations = {
            "wrong-exit": lambda r: r["steps"][1].update(process_returncode=18),
            "reordered": lambda r: r["steps"].reverse(),
            "wrong-time": lambda r: r["steps"][0].update(finished_at_utc="2000-01-01T00:00:00Z"),
            "private-log": lambda r: r["steps"][0]["redacted_log"].append("PRIVATE_PAYLOAD"),
            "wrong-pin": lambda r: r["archive"].update(index_sha256="0" * 64),
            "foreign-run": lambda r: r["steps"][0]["public_identity"].update(run_id="1" * 64),
            "false-go": lambda r: r.update(production_approval=True),
        }
        for name, mutate in mutations.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as work:
                bad = copy.deepcopy(original)
                mutate(bad)
                target = Path(work) / "evidence"
                target.mkdir(mode=0o700)
                new_pin = h.write_receipt(target, bad)
                # Rehash every semantic mutant; checksum-only validation
                # would accept it. The archive remains the genuine original.
                result = invoke("verify", archive, target, new_pin)
                self.assertEqual(result.returncode, 1)
                self.assertNotIn(b"PRIVATE_PAYLOAD", result.stdout)
                self.assertNotIn(b"H2_E_COLD_RECEIPT=VERIFIED", result.stdout)
        self.assertEqual(invoke("verify", archive, evidence, "0" * 64).returncode, 1)
        with tempfile.TemporaryDirectory() as work:
            root = Path(work)
            fake = root / "python"
            fake.write_text("#!/bin/sh\ncase \"$1\" in\n--version) echo 'Python 0.0.0';;\n-S) printf '%s\\n' '" + work + "';;\n*) exit 42;;\nesac\n")
            fake.chmod(0o700)
            env = {**os.environ, "PATH": work + os.pathsep + os.environ["PATH"]}
            target = root / "evidence"
            failed = root / "failed-archive"
            result = invoke("record", failed, target, env=env)
            self.assertEqual(result.returncode, 1)
            receipt = json.loads((target / "operational-receipt.json").read_bytes())
            self.assertEqual(receipt["disposition"], "INCOMPLETE_NO_GO")
            self.assertIsNone(receipt["archive"])
            self.assertEqual(len(receipt["steps"]), 1)
            self.assertEqual(receipt["steps"][0]["process_returncode"], 17)
            self.assertFalse((failed / "checksums.json").exists())
            self.assertNotIn(b"H2_E_REAL_COMMANDS=OBSERVED", result.stdout)
        self.assertEqual(h.snapshot(archive), before)
        self.assertEqual(receipt_path.read_bytes(), raw)
        print("H2_E_REAL_MUTANTS=REHASHED_RECEIPTS_REJECTED_AND_FAILED_EXIT_RECORDED")


if __name__ == "__main__":
    unittest.main()
