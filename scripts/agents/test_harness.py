#!/usr/bin/env python3
"""Offline regression tests for the agent harness: no provider credentials required."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
from headless_review import check_input
from fleet_snapshot import export as export_snapshot, restore as restore_snapshot

HERE = Path(__file__).resolve().parent


def command(args, cwd):
    return subprocess.run(args, cwd=cwd, text=True, capture_output=True, check=False, timeout=60)


class HarnessTests(unittest.TestCase):
    def test_review_context_fail_closed(self):
        good = {"diff": "diff --git a/x b/x\n+changed", "issue_body": "Accepted criteria",
                "pr_head": "a" * 40}
        check_input(good)
        for bad in (dict(good, diff="truncated"), dict(good, diff=good["diff"] * 10000),
                    dict(good, pr_head="unknown")):
            with self.assertRaises(ValueError):
                check_input(bad)

    def test_hook_no_changes_and_detects_python_syntax(self):
        with tempfile.TemporaryDirectory() as d:
            f = Path(d) / "sample.py"
            f.write_text("value = 1\n")
            env = {**os.environ, "CLAUDE_PROJECT_DIR": d}
            hook = str(HERE / "post_edit_check.py")
            def check():
                return subprocess.run([sys.executable, hook], input=json.dumps(
                    {"tool_input": {"file_path": str(f)}}), text=True,
                    capture_output=True, env=env, timeout=15)
            self.assertEqual(check().returncode, 0)
            self.assertEqual(f.read_text(), "value = 1\n")
            f.write_text("def f(:\n")
            self.assertEqual(check().returncode, 2)

    def test_mutation_isolated_red_and_survivor(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            for args in (["git", "init", "-q"], ["git", "config", "user.name", "Fixture"],
                         ["git", "config", "user.email", "fixture@example.invalid"]):
                self.assertEqual(command(args, root).returncode, 0)
            source = root / "logic.py"
            source.write_text("def allowed():\n    return False\n")
            test = root / "verify.py"
            test.write_text("from logic import allowed\nassert allowed() is False\n")
            self.assertEqual(command(["git", "add", "."], root).returncode, 0)
            self.assertEqual(command(["git", "commit", "-qm", "fixture"], root).returncode, 0)
            base = [sys.executable, str(HERE / "mutation_probe.py"),
                    "--file", "logic.py", "--old", "return False", "--new", "return True",
                    "--", sys.executable, "verify.py"]
            red = command(base, root)
            self.assertEqual(red.returncode, 0, red.stdout + red.stderr)
            self.assertIn("detected=True", red.stdout)
            self.assertIn("return False", source.read_text())
            self.assertEqual(command(["git", "status", "--porcelain"], root).stdout.strip(), "")
            survivor = command(base[:-3] + ["--", sys.executable, "-c", "print('irrelevant')"], root)
            self.assertEqual(survivor.returncode, 1, survivor.stdout + survivor.stderr)
            self.assertIn("return False", source.read_text())


    def test_snapshot_cross_job_manifest_and_untrusted_tampering(self):
        with tempfile.TemporaryDirectory() as d:
            temp = Path(d)
            repository = temp / "repo"
            repository.mkdir()
            for args in (["git", "init", "-q"], ["git", "config", "user.name", "Fixture"],
                         ["git", "config", "user.email", "fixture@example.invalid"]):
                self.assertEqual(command(args, repository).returncode, 0)
            (repository / "src").mkdir()
            file = repository / "src" / "feature.py"
            file.write_text("value = 1\\n")
            self.assertEqual(command(["git", "add", "."], repository).returncode, 0)
            self.assertEqual(command(["git", "commit", "-qm", "fixture"], repository).returncode, 0)
            snapshot = temp / "snapshot"
            export_snapshot(repository, snapshot, ["src"])
            file.write_text("value = 2\\n")
            restore_snapshot(repository, snapshot, ["src"])
            self.assertEqual(file.read_text(), "value = 1\\n")
            # Tampering never reaches source in the privileged publisher job.
            (snapshot / "src" / "feature.py").write_text("value = 99\\n")
            with self.assertRaises(ValueError):
                restore_snapshot(repository, snapshot, ["src"])
            self.assertEqual(file.read_text(), "value = 1\\n")
            # An unlisted file must also be rejected as injected artifact data.
            (snapshot / "src" / "rogue.py").write_text("evil\\n")
            with self.assertRaises(ValueError):
                restore_snapshot(repository, snapshot, ["src"])
            self.assertEqual(file.read_text(), "value = 1\\n")

    def test_snapshot_rejects_symlink_overwrite_and_secret_paths(self):
        with tempfile.TemporaryDirectory() as d:
            temp = Path(d)
            repo = temp / "repo"
            repo.mkdir()
            for args in (["git", "init", "-q"], ["git", "config", "user.name", "Fixture"],
                         ["git", "config", "user.email", "fixture@example.invalid"]):
                self.assertEqual(command(args, repo).returncode, 0)
            (repo / "src").mkdir()
            f = repo / "src" / "ok.py"
            f.write_text("ready = True\\n")
            self.assertEqual(command(["git", "add", "."], repo).returncode, 0)
            self.assertEqual(command(["git", "commit", "-qm", "fixture"], repo).returncode, 0)
            snapshot = temp / "snapshot"
            export_snapshot(repo, snapshot, ["src"])
            f.unlink()
            f.symlink_to(repo / ".git" / "config")
            with self.assertRaises(ValueError):
                restore_snapshot(repo, snapshot, ["src"])
            self.assertTrue(f.is_symlink())


    def test_snapshot_rejects_stale_branch_head(self):
        with tempfile.TemporaryDirectory() as d:
            temp = Path(d)
            repo = temp / "repo"
            repo.mkdir()
            for args in (["git", "init", "-q"], ["git", "config", "user.name", "Fixture"],
                         ["git", "config", "user.email", "fixture@example.invalid"]):
                self.assertEqual(command(args, repo).returncode, 0)
            (repo / "src").mkdir()
            file = repo / "src" / "feature.py"
            file.write_text("ready = True\\n")
            self.assertEqual(command(["git", "add", "."], repo).returncode, 0)
            self.assertEqual(command(["git", "commit", "-qm", "baseline"], repo).returncode, 0)
            snapshot = temp / "snapshot"
            export_snapshot(repo, snapshot, ["src"])
            file.write_text("ready = False\\n")
            self.assertEqual(command(["git", "commit", "-qam", "parallel change"], repo).returncode, 0)
            with self.assertRaisesRegex(ValueError, "Concurrent branch change"):
                restore_snapshot(repo, snapshot, ["src"])
            self.assertEqual(file.read_text(), "ready = False\\n")


if __name__ == "__main__":
    unittest.main()
