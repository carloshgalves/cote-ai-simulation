#!/usr/bin/env python3
"""Transfer allowlisted source snapshots between *separate* unprivileged and trusted jobs.

Export runs after the LLM in a read-only-permissions worker. Import runs only
on a fresh publisher runner and validates the complete, untrusted artifact.
Never execute anything from the artifact or from an agent-controlled worktree.
"""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import shutil
import stat
import subprocess
import sys

MAX_FILES = 3500
MAX_FILE_BYTES = 2_000_000
MAX_BYTES = 30_000_000


def allowed_roots(value: str) -> list[str]:
    result = value.split()
    if not result or len(set(result)) != len(result):
        raise ValueError("Missing or duplicate source allowlist")
    for root in result:
        p = PurePosixPath(root)
        if p.is_absolute() or ".." in p.parts or "." in p.parts or not all(
            x.replace("-", "").replace("_", "").isalnum() for x in p.parts
        ):
            raise ValueError("Invalid allowlist path: " + root)
    return result


def validate_path(name: str, roots: list[str]) -> None:
    p = PurePosixPath(name)
    if p.is_absolute() or ".." in p.parts or "." in p.parts or not p.parts:
        raise ValueError("Unsafe artifact path")
    bad = {".git", ".github", ".claude", ".agents", "node_modules",
           "__pycache__", ".pytest_cache", "target", "build", "dist", "coverage",
           "credentials.json", "token.json", "id_rsa"}
    if any(x in bad or x == ".env" or x.startswith(".env.") or
           x.endswith((".pem", ".key", ".p12", ".pfx")) for x in p.parts):
        raise ValueError("Sensitive or generated content: " + name)
    if not any(name.startswith(root + "/") for root in roots):
        raise ValueError("Non-allowlisted content: " + name)


def hash_file(file: Path) -> str:
    digest = hashlib.sha256()
    with file.open("rb") as stream:
        while piece := stream.read(64_000):
            digest.update(piece)
    return digest.hexdigest()


def tracked_files(worktree: Path, roots: list[str]) -> set[str]:
    result = subprocess.run(["git", "-C", str(worktree), "ls-files", "--cached",
                             "--others", "--exclude-standard", "-z", "--", *roots],
                            check=True, capture_output=True)
    return {x.decode("utf-8") for x in result.stdout.split(b"\0") if x}


def export(worktree: Path, destination: Path, roots: list[str]) -> None:
    if destination.exists():
        raise ValueError("Snapshot destination must be fresh")
    destination.mkdir(parents=True)
    entries = []
    total = 0
    for name in sorted(tracked_files(worktree, roots)):
        validate_path(name, roots)
        source = worktree.joinpath(*PurePosixPath(name).parts)
        if not source.exists() and not source.is_symlink():
            continue  # tracked file deleted by the agent
        if source.is_symlink() or not source.is_file() or not source.resolve().is_relative_to(worktree):
            raise ValueError("Refusing symlink/non-file outside working tree: " + name)
        size = source.stat().st_size
        total += size
        if size > MAX_FILE_BYTES or total > MAX_BYTES or len(entries) >= MAX_FILES:
            raise ValueError("Snapshot size/file count limit exceeded")
        target = destination.joinpath(*PurePosixPath(name).parts)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
        entries.append({"path": name, "bytes": size, "sha256": hash_file(target)})
    (destination / "manifest.json").write_text(
        json.dumps({"schema_version": 1, "roots": roots, "files": entries},
                   sort_keys=True, separators=(",", ":")), encoding="utf-8"
    )
    print("Exported", len(entries), "allowlisted files,", total, "bytes; no repository credentials")


def restore(worktree: Path, destination: Path, roots: list[str]) -> None:
    raw = json.loads((destination / "manifest.json").read_text(encoding="utf-8"))
    if raw.get("schema_version") != 1 or raw.get("roots") != roots or not isinstance(raw.get("files"), list):
        raise ValueError("Invalid source snapshot manifest")
    rows = raw["files"]
    if len(rows) > MAX_FILES:
        raise ValueError("Too many files")
    expected = set()
    total = 0
    for entry in rows:
        if not isinstance(entry, dict) or set(entry) != {"path", "bytes", "sha256"}:
            raise ValueError("Invalid snapshot row")
        name, size, sha = entry["path"], entry["bytes"], entry["sha256"]
        if not isinstance(name, str) or not isinstance(size, int) or not isinstance(sha, str):
            raise ValueError("Invalid snapshot value")
        validate_path(name, roots)
        if name in expected or size < 0 or size > MAX_FILE_BYTES or len(sha) != 64:
            raise ValueError("Duplicate/oversized snapshot file")
        expected.add(name)
        source = destination.joinpath(*PurePosixPath(name).parts)
        if source.is_symlink() or not source.is_file() or not source.resolve().is_relative_to(destination):
            raise ValueError("Unsafe snapshot source")
        if source.stat().st_size != size or hash_file(source) != sha:
            raise ValueError("Snapshot payload does not match manifest: " + name)
        total += size
        if total > MAX_BYTES:
            raise ValueError("Snapshot too large")
    actual = {str(x.relative_to(destination)).replace("\\", "/")
              for x in destination.rglob("*") if x.is_file() or x.is_symlink()}
    if actual != expected | {"manifest.json"}:
        raise ValueError("Unexpected file/symlink in snapshot")
    # Preflight every destination path before writing any untrusted data.
    for name in expected:
        path = worktree.joinpath(*PurePosixPath(name).parts)
        if not path.resolve().is_relative_to(worktree):
            raise ValueError("Destination escapes worktree")
        for parent in list(path.parents):
            if parent == worktree:
                break
            if parent.is_symlink():
                raise ValueError("Symlink parent in destination")
    # Synchronize only allowlisted tracked paths; no shell and no source code execution.
    existing = tracked_files(worktree, roots)
    for name in sorted(existing - expected):
        path = worktree.joinpath(*PurePosixPath(name).parts)
        if path.is_file() or path.is_symlink():
            path.unlink()
    for name in sorted(expected):
        target = worktree.joinpath(*PurePosixPath(name).parts)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(destination.joinpath(*PurePosixPath(name).parts), target)
    print("Restored", len(expected), "validated files to trusted publishing worktree")


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("mode", choices=["export", "restore"])
    p.add_argument("--worktree", required=True)
    p.add_argument("--snapshot", required=True)
    p.add_argument("--allow", required=True)
    a = p.parse_args()
    try:
        worktree = Path(a.worktree).resolve(strict=True)
        snapshot = Path(a.snapshot).resolve()
        roots = allowed_roots(a.allow)
        if not worktree.is_dir() or (worktree / ".git").is_dir() is False and not (worktree / ".git").is_file():
            raise ValueError("Not a git worktree")
        if snapshot.is_relative_to(worktree):
            raise ValueError("Snapshot must be outside the source worktree")
        if a.mode == "export":
            export(worktree, snapshot, roots)
        else:
            restore(worktree, snapshot, roots)
        return 0
    except (ValueError, OSError, subprocess.CalledProcessError, UnicodeError) as error:
        print("NO-GO: unsafe/incomplete fleet artifact:", error, file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
