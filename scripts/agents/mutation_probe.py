#!/usr/bin/env python3
"""Crash-isolated mutation probe: never mutates the caller's checkout.

Example:
 python3 scripts/agents/mutation_probe.py --file src/rules.py \
   --old 'return allowed' --new 'return True' \
   --must-contain 'FAILED' -- python3 -m pytest -q tests/test_rules.py

An exit code of 0 means the baseline passed and the injected defect was detected.
The worktree is removed in finally, even after ordinary exceptions/SIGTERM.
An abrupt runner/OS death may leave a disposable worktree, not modified source.
"""
from __future__ import annotations

import argparse
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile


def execute(argv: list[str], cwd: Path, timeout: int) -> subprocess.CompletedProcess[str]:
    return subprocess.run(argv, cwd=cwd, capture_output=True, text=True,
                          errors="replace", timeout=timeout, check=False)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--file", required=True, help="Tracked relative text file to mutate")
    parser.add_argument("--old", required=True, help="Exact text to replace once")
    parser.add_argument("--new", required=True, help="Replacement text")
    parser.add_argument("--must-contain", default="", help="Expected text from failing test output")
    parser.add_argument("--timeout", type=int, default=120, help="Seconds per test run (1..900)")
    parser.add_argument("test_command", nargs=argparse.REMAINDER)
    args = parser.parse_args()

    if not 1 <= args.timeout <= 900 or args.old == args.new:
        parser.error("Require a positive timeout <=900 and a real semantic mutation")
    command = args.test_command
    if command and command[0] == "--":
        command = command[1:]
    if not command:
        parser.error("Test command required after --")
    base = execute(["git", "rev-parse", "--show-toplevel"], Path.cwd(), 15)
    if base.returncode:
        print("Not a Git checkout", file=sys.stderr)
        return 2
    repo = Path(base.stdout.strip()).resolve()
    candidate = Path(args.file)
    if candidate.is_absolute() or ".." in candidate.parts or not candidate.parts:
        print("Mutation file must be a relative path inside the repository", file=sys.stderr)
        return 2
    tracked = execute(["git", "ls-files", "--error-unmatch", "--", args.file], repo, 15)
    if tracked.returncode:
        print("Mutation file must be tracked at the source HEAD", file=sys.stderr)
        return 2

    def terminate(_signum: int, _frame: object) -> None:
        raise KeyboardInterrupt("Termination received; cleaning isolated worktree")

    old_term = signal.signal(signal.SIGTERM, terminate)
    old_int = signal.signal(signal.SIGINT, terminate)
    try:
        with tempfile.TemporaryDirectory(prefix="agent-mutation-") as temp:
            isolated = Path(temp) / "worktree"
            added = False
            try:
                created = execute(["git", "worktree", "add", "--detach", str(isolated), "HEAD"], repo, 30)
                if created.returncode:
                    print(created.stderr, file=sys.stderr)
                    return 2
                added = True
                target = isolated.joinpath(*candidate.parts)
                if not target.is_file() or target.is_symlink() or not target.resolve().is_relative_to(isolated.resolve()):
                    print("Refusing non-file/symlink target", file=sys.stderr)
                    return 2
                baseline = execute(command, isolated, args.timeout)
                print(f"BASELINE exit={baseline.returncode}")
                if baseline.returncode != 0:
                    print((baseline.stdout + baseline.stderr)[-2500:], file=sys.stderr)
                    print("NO-GO: baseline was already failing", file=sys.stderr)
                    return 1
                body = target.read_text(encoding="utf-8")
                if body.count(args.old) != 1:
                    print("Expected the mutation anchor to occur exactly once", file=sys.stderr)
                    return 2
                target.write_text(body.replace(args.old, args.new, 1), encoding="utf-8")
                mutant = execute(command, isolated, args.timeout)
                combined = mutant.stdout + mutant.stderr
                detected = mutant.returncode != 0 and (not args.must_contain or args.must_contain in combined)
                print(f"MUTANT exit={mutant.returncode} detected={detected}")
                print(combined[-2500:])
                if not detected:
                    print("SURVIVING MUTANT: the selected test did not prove the contract", file=sys.stderr)
                    return 1
                print("DETECTED: baseline GREEN, mutated behavior RED, original checkout untouched")
                return 0
            finally:
                if added:
                    cleanup = execute(["git", "worktree", "remove", "--force", str(isolated)], repo, 30)
                    if cleanup.returncode:
                        print("WARNING: isolated worktree cleanup requires inspection: " + str(isolated), file=sys.stderr)
    except (KeyboardInterrupt, subprocess.TimeoutExpired, UnicodeError, OSError) as exc:
        print(f"NO-GO: interrupted or invalid probe: {exc}", file=sys.stderr)
        return 2
    finally:
        signal.signal(signal.SIGTERM, old_term)
        signal.signal(signal.SIGINT, old_int)


if __name__ == "__main__":
    raise SystemExit(main())
