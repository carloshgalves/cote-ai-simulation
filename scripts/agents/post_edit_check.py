#!/usr/bin/env python3
"""Claude PostToolUse hook: read-only, fast syntax/format feedback on edited files."""
import ast
import json
import os
from pathlib import Path
import subprocess
import sys


def main() -> int:
    try:
        event = json.load(sys.stdin)
        path = event.get("tool_input", {}).get("file_path", "")
        if not isinstance(path, str) or not path:
            return 0
        root = Path(os.environ.get("CLAUDE_PROJECT_DIR", os.getcwd())).resolve()
        candidate = (root / path).resolve()
        if not candidate.is_relative_to(root) or not candidate.is_file():
            return 0
        if candidate.stat().st_size > 512_000:
            print("post-edit: skipped large file")
            return 0
        suffix = candidate.suffix.lower()
        if suffix == ".py":
            ast.parse(candidate.read_text(encoding="utf-8"), filename=str(candidate))
        elif suffix == ".json":
            json.loads(candidate.read_text(encoding="utf-8"))
        elif suffix == ".go":
            result = subprocess.run(["gofmt", "-d", str(candidate)], capture_output=True,
                                    text=True, timeout=10, check=False)
            if result.returncode or result.stdout:
                print("post-edit: Go formatting differs from gofmt (no auto-fix):\n" + result.stdout[:3000],
                      file=sys.stderr)
                return 2
        elif suffix in {".js", ".mjs", ".cjs"}:
            result = subprocess.run(["node", "--check", str(candidate)], capture_output=True,
                                    text=True, timeout=10, check=False)
            if result.returncode:
                print("post-edit: JS syntax invalid:\n" + result.stderr[:3000], file=sys.stderr)
                return 2
        return 0
    except (SyntaxError, ValueError, UnicodeError, subprocess.TimeoutExpired, OSError) as exc:
        print(f"post-edit: validation failed: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
