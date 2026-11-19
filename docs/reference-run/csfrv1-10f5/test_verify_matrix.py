"""Negative controls for the certificate validator (not actual simulation evidence)."""
import json
import unittest

from verify_matrix import MATRIX, REGRESSIONS, verify


def encode(events):
    return ("\n".join(json.dumps(e) for e in events) + "\n").encode()


def fixture():
    return [
        {"Action": "start", "Package": "p"},
        {"Action": "run", "Package": "p", "Test": "TestActual"},
        {"Action": "pass", "Package": "p", "Test": "TestActual"},
        {"Action": "pass", "Package": "p"},
    ]


class ValidatorTests(unittest.TestCase):
    expected = {"p": {"TestActual"}}

    def test_inventory_is_pinned(self):
        self.assertEqual(sum(map(len, MATRIX.values())), 40)
        self.assertEqual(sum(map(len, REGRESSIONS.values())), 34)

    def test_valid(self):
        self.assertEqual(verify(encode(fixture()), self.expected)["test_count"], 1)

    def test_invalid(self):
        cases = {
            "empty": [],
            "missing-test-start": [fixture()[0], *fixture()[2:]],
            "missing-test-pass": [*fixture()[:2], fixture()[3]],
            "missing-package-pass": fixture()[:3],
            "wrong-package": [{**e, "Package": "other"} for e in fixture()],
            "wrong-test": [{**e, "Test": "TestOther"} if "Test" in e else e for e in fixture()],
            "duplicate-test-start": [fixture()[0], fixture()[1], *fixture()[1:]],
            "duplicate-test-pass": [*fixture()[:3], fixture()[2], fixture()[3]],
            "duplicate-package-start": [fixture()[0], *fixture()],
            "duplicate-package-pass": [*fixture(), fixture()[3]],
            "failed-test": [*fixture()[:2], {**fixture()[2], "Action": "fail"}, fixture()[3]],
            "skipped-test": [*fixture()[:2], {**fixture()[2], "Action": "skip"}, fixture()[3]],
            "build-failure": [*fixture(), {"Action": "build-fail", "ImportPath": "p"}],
            "cached": [*fixture()[:3], {"Action": "output", "Package": "p", "Output": "ok p (cached)"}, fixture()[3]],
            "premature-package-pass": [*fixture()[:2], fixture()[3], fixture()[2]],
            "extra-test": [*fixture()[:3], {"Action": "run", "Package": "p", "Test": "TestUnexpected"}, fixture()[3]],
        }
        for name, events in cases.items():
            with self.subTest(name=name), self.assertRaises(ValueError):
                verify(encode(events), self.expected)
        with self.assertRaises(ValueError):
            verify(b"not-json\n", self.expected)


if __name__ == "__main__":
    unittest.main()
