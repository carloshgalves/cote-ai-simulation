"""Falsification controls: a green certificate cannot be fabricated by missing cases."""
import copy
import json
import subprocess
import tempfile
import unittest
from pathlib import Path

import run_evals as g

ROOT = Path(__file__).resolve().parents[3]


def encoded(events):
    return ("\n".join(json.dumps(x) for x in events) + "\n").encode()


class RegistryTests(unittest.TestCase):
    def setUp(self):
        self.registry = g.load((ROOT / g.REGISTRY).read_bytes())
        self.trace = g.load((ROOT / g.TRACE).read_bytes())

    def validate(self, manifest=None, trace=None):
        return g.validate(manifest if manifest is not None else self.registry,
                          trace if trace is not None else self.trace,
                          lambda path: (ROOT / path).read_text())

    def test_frozen_roster_and_pins(self):
        self.assertEqual(g.git_blob((ROOT / g.ADR).read_bytes()), g.PIN_ADR)
        self.assertEqual(g.git_blob((ROOT / g.TRACE).read_bytes()), g.PIN_TRACE)
        self.assertEqual(len(self.validate()), 109)
        self.assertEqual(sum(r["state"] == "READY" for r in self.registry["rows"]), 23)
        self.assertEqual(sum(r["state"] == "BLOCKED" for r in self.registry["rows"]), 8)

    def test_invalid_manifest(self):
        def row(m, rid):
            return next(r for r in m["rows"] if r["id"] == rid)

        cases = {
            "missing-row": lambda m: m["rows"].pop(),
            "duplicate-row": lambda m: m["rows"].append(m["rows"][0]),
            "fake-pass": lambda m: row(m, "INV-01").update(state="PASS_INTEGRATED_TEST"),
            "ready-empty": lambda m: row(m, "INV-01").update(state="READY"),
            "blocked-credit": lambda m: row(m, "INV-23")["witnesses"].append("contest"),
            "not-run-credit": lambda m: row(m, "INV-01")["witnesses"].append("contest"),
            "no-owner": lambda m: row(m, "INV-01").update(owner_issue=0),
            "unknown-witness": lambda m: row(m, "STRESS-41")["witnesses"].append("absent"),
            "wrong-source": lambda m: m["witnesses"]["contest"].update(path="../../foo_test.go"),
            "fake-test": lambda m: m["witnesses"]["contest"].update(name="TestNonexistent"),
            "duplicate-selector": lambda m: m["witnesses"].update(dup=m["witnesses"]["contest"]),
            "changed-pin": lambda m: m.update(baseline_adr_blob="0" * 40),
            "missing-eval": lambda m: m["evals"].pop("embodiment"),
            "fake-complete-eval": lambda m: m["evals"]["embodiment"].update(scope="COMPLETE"),
            "row-witness-unlisted": lambda m: m["evals"]["simulation"]["witnesses"].remove("contest"),
        }
        for name, mutate in cases.items():
            with self.subTest(name=name), self.assertRaises((ValueError, KeyError)):
                bad = copy.deepcopy(self.registry)
                mutate(bad)
                self.validate(bad)
        with self.assertRaises(ValueError):
            g.load(b'{"x":1,"x":2}')
        trace = copy.deepcopy(self.trace)
        trace["invariants"].pop()
        with self.assertRaises(ValueError):
            self.validate(trace=trace)

    def test_report_no_fabricated_coverage(self):
        normative = self.validate()
        no_runs = g.report(self.registry, normative, {}, "a" * 40, "b" * 64)
        self.assertFalse(no_runs["matrix_complete"])
        self.assertEqual(no_runs["counts"]["INV"]["FAIL"], 10)
        self.assertEqual(no_runs["counts"]["STRESS"]["FAIL"], 13)
        # Test certificate aggregation only: synthetic results are NEVER
        # written as simulation evidence, only run by this verifier unit test.
        successes = {w: {mode: {"status": "PASS"} for mode in ("normal", "race")}
                     for w in self.registry["witnesses"]}
        report = g.report(self.registry, normative, successes, "a" * 40, "b" * 64)
        self.assertEqual(report["counts"]["INV"]["PASS_INTEGRATED_TEST"], 10)
        self.assertEqual(report["counts"]["STRESS"]["PASS_INTEGRATED_TEST"], 13)
        self.assertFalse(report["matrix_complete"])
        self.assertFalse(report["evals_complete"])
        self.assertEqual(next(r for r in report["rows"] if r["id"] == "STRESS-61")["status"], "NOT_RUN")
        self.assertEqual(set(report["evals"]), set(g.OWNERS))
        self.assertTrue(all(x["status"] in {"PASS_PILOT_ONLY", "PASS_SCOPED_WITH_GAPS"} and not x["complete"] for x in report["evals"].values()))
        self.assertFalse(report["release_authorized"])
        self.assertEqual(report["disposition"], "NO_GO_INCOMPLETE_MATRIX")
        # A supplemental embodiment witness cannot promote STRESS-39
        # even if both of its runs pass. Its failure gates its eval even
        # though it is not credited to any normative row.
        self.assertEqual(next(r for r in report["rows"] if r["id"] == "STRESS-39")["status"], "NOT_RUN")
        del successes["physical-d3"]["normal"]
        bad_eval = g.report(self.registry, normative, successes, "a" * 40, "b" * 64)
        self.assertEqual(bad_eval["evals"]["embodiment"]["status"], "FAIL")
        self.assertEqual(bad_eval["counts"], report["counts"])
        del successes["contest"]["race"]
        report = g.report(self.registry, normative, successes, "a" * 40, "b" * 64)
        self.assertEqual(report["counts"]["STRESS"]["FAIL"], 1)


class CheckoutTests(unittest.TestCase):
    def test_ignored_go_source_is_not_clean(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            def git(*args):
                subprocess.run(["git", *args], cwd=root, check=True,
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            git("init")
            (root / ".gitignore").write_text("*.go\n")
            git("add", ".gitignore")
            git("-c", "user.name=Test", "-c", "user.email=test@example.invalid",
                "commit", "-m", "fixture")
            self.assertTrue(g.clean_checkout(root))
            (root / "injected_test.go").write_text("package fixture\n")
            self.assertEqual(g.git(root, "status", "--porcelain", "--untracked-files=all"), "")
            self.assertFalse(g.clean_checkout(root))


class EventTests(unittest.TestCase):
    package = "pkg"
    names = ["TestA", "TestA/child"]

    def valid(self):
        p = self.package
        return [
            {"Action": "start", "Package": p},
            {"Action": "run", "Package": p, "Test": "TestA"},
            {"Action": "run", "Package": p, "Test": "TestA/child"},
            {"Action": "pass", "Package": p, "Test": "TestA/child"},
            {"Action": "pass", "Package": p, "Test": "TestA"},
            {"Action": "pass", "Package": p},
        ]

    def test_valid(self):
        safe = []
        data = self.valid()
        data.insert(3, {"Action": "output", "Package": self.package, "Test": "TestA/child", "Output": "private payload"})
        self.assertEqual(g.verify_events(encoded(data), self.package, self.names, transcript=safe), self.names)
        self.assertEqual(len(safe), 6)
        self.assertNotIn("private payload", json.dumps(safe))

    def test_negative(self):
        f = self.valid()
        bad = {
            "empty": [],
            "no-start": f[1:],
            "no-child": [*f[:2], *f[4:]],
            "missing-pass": [*f[:3], *f[4:]],
            "no-package-pass": f[:-1],
            "duplicate-start": [*f[:2], *f[1:]],
            "duplicate-pass": [*f[:4], *f[3:]],
            "duplicate-package": [*f, f[-1]],
            "foreign": [{**x, "Package": "foreign"} for x in f],
            "skipped": [*f[:3], {**f[3], "Action": "skip"}, *f[4:]],
            "failed-build": [*f, {"Action": "build-fail", "ImportPath": "pkg"}],
            "cached": [*f[:-1], {"Action": "output", "Package": "pkg", "Output": "ok (cached)"}, f[-1]],
            "extra-test": [*f[:-1], {"Action": "run", "Package": "pkg", "Test": "TestOther"}, f[-1]],
            "child-before-parent": [f[0], f[2], f[1], *f[3:]],
            "parent-before-child-pass": [*f[:3], f[4], f[3], f[5]],
        }
        for name, events in bad.items():
            with self.subTest(name=name), self.assertRaises(ValueError):
                g.verify_events(encoded(events), self.package, self.names)


if __name__ == "__main__":
    unittest.main()
