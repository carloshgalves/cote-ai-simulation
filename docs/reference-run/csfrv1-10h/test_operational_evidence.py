"""H2-E structural and adversarial controls (real processes exercised in CI)."""
import copy
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
                          "process_returncode": 0, "redacted_log": sorted(markers | h.SUCCESS_MARKERS)})
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
            with self.assertRaises(FileExistsError):
                h.write_receipt(path, receipt)


if __name__ == "__main__":
    unittest.main()
