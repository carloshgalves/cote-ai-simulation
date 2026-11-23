"""RED/negative controls for G6 certification; synthetic data is never CI evidence."""
import copy
import json
import unittest
from pathlib import Path

import g6_finalize as g6
import run_evals as g

ROOT = Path(__file__).resolve().parents[3]


class G6DecisionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.registry = g.load((ROOT / g.REGISTRY).read_bytes())
        cls.trace = g.load((ROOT / g.TRACE).read_bytes())
        cls.normative = g.norm_rows(cls.trace)
        cls.ids = g6.conformance_case_ids(ROOT)
        cls.suite_hash = g6.conformance_bundle_hash(ROOT)

    def fake_conformance(self):
        # This is a schema-mutant fixture, never a report produced by Go/TS.
        return {
            "suite_hash": self.suite_hash, "total": 621,
            "passed": 621, "failed": 0,
            "cases": [{"id": ident, "status": "pass"} for ident in self.ids],
        }

    def test_pinned_node_major_is_required(self):
        self.assertEqual(g6.verify_node_version("v22.14.0"), "v22.14.0")
        for invalid in (None, "", "NOT_ATTESTED", "v20.13.1", "v24.0.0",
                        "v22x14x0", "v22.1.1-dev", 22):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                g6.verify_node_version(invalid)

    def test_621_roster_and_byte_agreement_are_all_required(self):
        valid = json.dumps(self.fake_conformance()).encode()
        actual = g6.check_conformance(ROOT, valid, valid)
        self.assertEqual(actual["status"], "PASS_GO_TS_EXACT_BYTES")
        self.assertEqual(actual["total"], 621)
        mutations = {
            "one divergent TS byte": (valid, valid + b" "),
            "forged suite hash": None,
            "missing case": None,
            "forged positive count": None,
            "one case marked failed": None,
            "case order swapped": None,
            "duplicate case": None,
        }
        for name in list(mutations)[1:]:
            altered = self.fake_conformance()
            if name == "forged suite hash":
                altered["suite_hash"] = "0" * 64
            elif name == "missing case":
                altered["cases"].pop()
            elif name == "forged positive count":
                altered["passed"] = 620
            elif name == "one case marked failed":
                altered["cases"][0]["status"] = "fail"
            elif name == "case order swapped":
                altered["cases"][0], altered["cases"][1] = altered["cases"][1], altered["cases"][0]
            elif name == "duplicate case":
                altered["cases"][0] = altered["cases"][1]
            payload = json.dumps(altered).encode()
            mutations[name] = (payload, payload)
        for name, (go_raw, ts_raw) in mutations.items():
            with self.subTest(name=name), self.assertRaises(ValueError):
                g6.check_conformance(ROOT, go_raw, ts_raw)

    def test_sanitized_execution_history_cannot_omit_a_child(self):
        pkg = "github.com/carloshgalves/cote-ai-simulation/internal/csf/commit"
        names = ["TestA", "TestA/child"]
        transcript = [
            {"action": "start", "package": pkg},
            {"action": "run", "package": pkg, "test": "TestA"},
            {"action": "run", "package": pkg, "test": "TestA/child"},
            {"action": "pass", "package": pkg, "test": "TestA/child"},
            {"action": "pass", "package": pkg, "test": "TestA"},
            {"action": "pass", "package": pkg},
        ]
        self.assertEqual(g6.replay_sanitized(transcript, pkg, names), names)
        mutations = {
            "no child": transcript[:2] + transcript[4:],
            "failed order": transcript[:3] + [transcript[4], transcript[3]] + transcript[5:],
            "no package": transcript[:-1],
            "duplicate pass": transcript[:4] + [transcript[3]] + transcript[4:],
            "extraneous actor payload": [dict(transcript[0], content="private"), *transcript[1:]],
        }
        for name, bad in mutations.items():
            with self.subTest(name=name), self.assertRaises(ValueError):
                g6.replay_sanitized(bad, pkg, names)

    def test_four_evals_aggregate_to_no_go_with_all_sources_synthetically_green(self):
        synthetic_results = {
            wid: {"normal": {"status": "PASS"}, "race": {"status": "PASS"}}
            for wid in self.registry["witnesses"]
        }
        # Arithmetic fixture ONLY. Real G6 requires the complete original
        # exact-SHA per-test record and never accepts this fake as evidence.
        report = g.report(self.registry, self.normative, synthetic_results,
                          "a" * 40, "b" * 64)
        decision, gaps = g6.finalize("a" * 40, report, [], {
            "status": "PASS_GO_TS_EXACT_BYTES", "total": 621, "passed": 621,
            "suite_hash": self.suite_hash, "identical_report_sha256": "b" * 64,
        }, self.registry)
        self.assertEqual(decision["disposition"], "NO_GO_INCOMPLETE_MATRIX")
        self.assertFalse(decision["dod_accepted"])
        self.assertFalse(decision["release_authorized"])
        self.assertEqual(decision["evidence"]["rows"], 109)
        self.assertEqual(decision["gaps"]["total"], 58)
        self.assertEqual(decision["gaps"]["blocked"], 8)
        self.assertEqual(decision["gaps"]["not_run"], 50)
        self.assertEqual(len(gaps["rows"]), 58)
        self.assertEqual(len(decision["evidence"]["evals"]), 4)
        self.assertEqual(decision["external_gates"]["93"], "NOT_ATTESTED")
        self.assertEqual(next(x for x in gaps["rows"] if x["id"] == "STRESS-39")["followup_issue"], 172)
        self.assertEqual(next(x for x in gaps["rows"] if x["id"] == "STRESS-50")["followup_issue"], 174)
        failed = copy.deepcopy(report)
        failed["rows"][0]["status"] = "FAIL"
        fdecision, _ = g6.finalize("a" * 40, failed, ["witness/race"], {
            "status": "PASS_GO_TS_EXACT_BYTES"}, self.registry)
        self.assertEqual(fdecision["disposition"], "NO_GO_FAILED_EVIDENCE")


if __name__ == "__main__":
    unittest.main()
