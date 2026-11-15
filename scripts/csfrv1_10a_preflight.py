#!/usr/bin/env python3
"""CSFRV1-10A: read-only plan preflight. Does not run or mutate the causal world."""
from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
import tomllib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
FOLDER = Path("docs/reference-run/csfrv1-10a")
STATUSES = frozenset({"EXISTING_PASS_EXACT_SHA", "NEEDS_NEW_INTEGRATION_TEST", "BLOCKED_RULE", "NOT_RUN"})
ORACLE_FILES = (
    "input-ledger.bin", "decision-ledger.bin", "event-store.bin",
    "evidence-ledger.bin", "snapshot-store.bin", "agenda-runtime.bin",
    "commit-cursors.bin", "event-ids.bin", "world-state.sha256",
    "knowledge-state.sha256",
)


class PreflightError(ValueError):
    pass


def require(value: bool, message: str) -> None:
    if not value:
        raise PreflightError(message)


def sha256(raw: bytes) -> str:
    return hashlib.sha256(raw).hexdigest()


def strict_object(items: list[tuple[str, object]]) -> dict:
    result: dict = {}
    for key, value in items:
        require(key not in result, f"duplicate JSON key: {key}")
        result[key] = value
    return result


def load_json(path: Path) -> dict:
    try:
        return json.loads(path.read_bytes(), object_pairs_hook=strict_object)
    except (OSError, ValueError, UnicodeError) as exc:
        raise PreflightError(f"invalid JSON {path}: {exc}") from exc


def run(*argv: str, cwd: Path = ROOT) -> str:
    try:
        proc = subprocess.run(argv, cwd=cwd, text=True, capture_output=True, check=False, timeout=35)
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise PreflightError(f"command unavailable: {argv[0]}: {exc}") from exc
    require(proc.returncode == 0, f"command failed: {' '.join(argv)}: {proc.stderr.strip()[:250]}")
    return proc.stdout.strip()


def parse_adr(text: str) -> tuple[list[str], list[tuple[str, str]]]:
    a = text.split("### 14. Invariantes normativas", 1)
    require(len(a) == 2, "ADR 0008 §14 missing")
    b = a[1].split("## Cenários de stress e provas de aceite", 1)
    require(len(b) == 2, "ADR stress table missing")
    normative = b[0].strip()
    matches = list(re.finditer(r"(?m)^(\d{1,2})\. ", normative))
    invariants = []
    for i, match in enumerate(matches):
        require(int(match.group(1)) == i + 1, "ADR invariant numbering is non-contiguous")
        end = matches[i + 1].start() if i + 1 < len(matches) else len(normative)
        invariants.append(" ".join(normative[match.end():end].split()))
    rows = []
    for line in b[1].split("## Contratos/ports mínimos", 1)[0].splitlines():
        if not line.startswith("| ") or line.startswith("| Cenário") or line.startswith("|---"):
            continue
        cells = line.split("|")
        require(len(cells) == 4, "malformed ADR stress row")
        rows.append((cells[1].strip(), cells[2].strip()))
    require(len(invariants) == 41 and len(rows) == 68, "ADR expected 41 invariants and 68 stress rows")
    return invariants, rows


def validate_traceability(trace: dict, adr_text: str) -> None:
    inv, stress = parse_adr(adr_text)
    require(trace.get("schema") == "csfrv1-10a.traceability.v1", "wrong traceability schema")
    require(trace.get("measurement_state") == "NOT_RUN", "traceability may not claim an E2E run")
    require(trace.get("allowed_statuses") == sorted(STATUSES) or set(trace.get("allowed_statuses", [])) == STATUSES,
            "status vocabulary changed")
    require(len(trace.get("invariants", [])) == 41 and len(trace.get("stress_scenarios", [])) == 68,
            "normative coverage count is incomplete")
    for kind, entries, expected in (
        ("INV", trace["invariants"], inv),
        ("STRESS", trace["stress_scenarios"], stress),
    ):
        for i, entry in enumerate(entries, start=1):
            require(entry.get("number") == i and entry.get("id") == f"{kind}-{i:02d}",
                    f"{kind} {i} identity/ordering mismatch")
            if kind == "INV":
                require(entry.get("normative_text") == expected[i-1], f"INV-{i:02d} normative text diverges")
                require(entry.get("group") == (
                    "authority" if i <= 16 else "ingress-cohorts" if i <= 34
                    else "triggers-events" if i <= 40 else "epistemic-chronology"),
                    f"INV-{i:02d} normative group mismatch")
                require(entry.get("scenario_id") in {f"STRESS-{k:02d}" for k in range(1, 69)},
                        f"INV-{i:02d} missing stress link")
            else:
                require((entry.get("scenario"), entry.get("required_proof")) == expected[i-1],
                        f"STRESS-{i:02d} scenario/proof diverges")
            status = entry.get("status")
            require(status in STATUSES, f"{kind}-{i:02d} unknown status")
            test = entry.get("test", {})
            require(test.get("name") == f"TestReference{'Invariant' if kind=='INV' else 'Stress'}{i:02d}",
                    f"{kind}-{i:02d} test alias/name mismatch")
            require(test.get("path") == "tests/scenarios/reference_run_test.go", "test path alias changed")
            evidence = entry.get("evidence", {})
            if status == "EXISTING_PASS_EXACT_SHA":
                require(test.get("state") == "EXISTING",
                        f"{kind}-{i:02d} PASS has no existing test")
                require(isinstance(evidence.get("commit_sha"), str) and
                        bool(re.fullmatch(r"[0-9a-f]{40}", evidence["commit_sha"])) and
                        bool(evidence.get("artifact")) and bool(evidence.get("exact_bytes_digest")),
                        f"{kind}-{i:02d} PASS without immutable evidence")
            else:
                require(test.get("state") == "PLANNED_NOT_PRESENT" and
                        all(evidence.get(k) is None for k in ("commit_sha", "artifact", "exact_bytes_digest")),
                        f"{kind}-{i:02d} unproved evidence must remain absent")
            require(isinstance(entry.get("owner_issue"), int), f"{kind}-{i:02d} missing owner issue")


def validate_scenario(manifest: dict, plan: dict) -> None:
    require(manifest.get("schema") == "csfrv1-10a.reference-plan.v1", "manifest schema mismatch")
    require(manifest.get("run_status") == "NOT_RUN" and manifest.get("acceptance_status") == "NOT_APPROVED",
            "premature run/acceptance status")
    require(plan.get("schema") == "csfrv1-10a.scenario.v1" and plan.get("run_status") == "NOT_RUN",
            "scenario attempts to claim execution")
    g = manifest["genesis"]
    require(g["status"] == "DECLARATIVE_ONLY_NOT_CANONICAL" and
            g["run_id"] is None and g["canonical_genesis_sha256"] is None,
            "unsigned/declarative Genesis must not masquerade as signed canonical bytes")
    require(re.fullmatch(r"[0-9a-f]{64}", g["world_seed_hex"]) is not None and
            plan["seed_hex"] == g["world_seed_hex"], "world seed mismatch")
    require(g["timezone"] == "Asia/Tokyo" and g["initial_instant"] == 0, "Genesis time mismatch")
    require(plan["clock"]["basis"] == "logical" and not plan["clock"]["wall_time_authoritative"],
            "wall time cannot be causal")
    require(manifest["execution"]["topology"] == "local-single-operator" and
            not any(manifest["execution"][k] for k in ("network", "llm", "provider", "cloud", "protected_data", "physical_seq_global")),
            "nonlocal/provider or local seq authority prohibited")
    src = plan["sources"]
    ids = [s["id"] for s in src]
    require(len(src) == 2 and len(set(ids)) == 2 and ids == g["source_ids"], "source alias/Genesis mismatch")
    allkeys = [v["key"] for s in src for v in s["inputs"]]
    require(len(set(allkeys)) == len(allkeys), "ambiguous producer_unit_key aliases")
    for s in src:
        closures = s["closures"]
        require(len(closures) == 2 and len({c["producer_key"] for c in closures}) == 2,
                "missing or aliased closures")
        require(not closures[0]["final"] and closures[1]["final"], "partial/final closure order invalid")
        require(closures[0]["closed_through"] == [0, 0] and closures[1]["closed_through"] == [10, 0],
                "closure coordinate mismatch")
    rounds = plan["rounds"]
    require([x["alias"] for x in rounds] == ["R1", "R2"] and
            len({x["actor_id"] for x in rounds}) == 2 and
            all(x["base_revision"] == 0 and x["effective_at"] == 0 for x in rounds),
            "simultaneous rounds or actor aliases invalid")
    require(plan["dispatch"]["response_order"] == ["R2", "R1"] and
            plan["dispatch"]["forbid_second_response"], "dispatch permutation invalid")
    c = plan["conflict"]
    require(c["initial_available"] == 1 and c["expected_commits"] == 1 and
            c["expected_resource_available_after"] == 0 and c["same_base_revision"] == 0,
            "last-unit conflict must be runtime resolved")
    require("winner" not in c and "winner_actor_id" in plan["expected"]["derived_only"],
            "winner cannot be hardcoded in fixture")
    require(len(plan["permutations"]) >= 18 and
            len({v["id"] for v in plan["permutations"]}) == len(plan["permutations"]),
            "permutation or aliases incomplete")
    require(g["physical_extension"]["status"] == "NOT_ATTESTED" and
            plan["physical"]["status"] == "BLOCKED_UNTIL_ISSUE_118",
            "unapproved physical Genesis/A1 admission")


def check_git(manifest: dict, root: Path = ROOT) -> None:
    base = manifest["baseline"]["commit"]
    require(re.fullmatch(r"[0-9a-f]{40}", base) is not None, "invalid baseline SHA")
    require(run("git", "cat-file", "-t", base, cwd=root) == "commit", "missing baseline commit")
    require(run("git", "rev-parse", f"{base}^{{tree}}", cwd=root) ==
            manifest["baseline"]["tree"], "baseline tree mismatch")
    run("git", "merge-base", "--is-ancestor", base, "HEAD", cwd=root)
    for item in manifest["normative"].values():
        require(run("git", "hash-object", item["path"], cwd=root) == item["git_blob"],
                f"normative source edited: {item['path']}")


def check_policy(manifest: dict, root: Path = ROOT) -> None:
    p = manifest["policy_bundle"]
    bundle = root / p["directory"]
    pins = [("policy-manifest.json", "codec_policy_hash", "cote.csf.bundle.codec-policy.v1"),
            ("schema-manifest.json", "schema_bundle_hash", "cote.csf.bundle.schema.v1"),
            ("conformance-manifest.json", "conformance_suite_hash", "cote.csf.bundle.conformance.v1")]
    receipt = (bundle / "BUNDLE.sha256").read_text(encoding="utf-8")
    for filename, name, prefix in pins:
        raw = (bundle / filename).read_bytes()
        require(sha256(raw) == p["manifest_raw_sha256"][filename], f"policy manifest bytes changed: {filename}")
        digest = sha256(prefix.encode("ascii") + b"\x00" + raw)
        require(digest == p[name] and digest in receipt, f"policy hash/receipt mismatch: {filename}")
        doc = load_json(bundle / filename)
        seen = set()
        for path, size, hexhash in doc["artifacts"]:
            require(path not in seen and bool(re.fullmatch(r"[A-Za-z0-9_.-]+", path)),
                    f"duplicate/unsafe policy artifact: {path}")
            seen.add(path)
            filebytes = (bundle / path).read_bytes()
            require(len(filebytes) == size and sha256(filebytes) == hexhash,
                    f"policy artifact bytes mismatch: {path}")
    profile = load_json(bundle / "profile.json")
    require(profile["codec_policy_id"] == p["codec_id"] and profile["codec_version"] == p["codec_version"],
            "policy/profile alias mismatch")


def check_versions(manifest: dict, root: Path = ROOT) -> None:
    tc = manifest["toolchain"]
    mod = (root / "go.mod").read_text(encoding="utf-8")
    require(re.search(r"(?m)^go " + re.escape(tc["go"]) + r"$", mod) is not None, "go.mod version diverges")
    require(re.search(r"(?m)^\s*" + re.escape(tc["sqlite"]["driver"]) + r" " +
                      re.escape(tc["sqlite"]["driver_version"]) + r"\s*$", mod) is not None,
            "SQLite Go driver pin diverges")
    py = tomllib.loads((root / "pyproject.toml").read_text(encoding="utf-8"))
    require(py["project"]["requires-python"] == ">=3.11", "Python project constraint changed")
    require("go" + tc["go"] in run("go", "version", cwd=root), "Go runtime does not match frozen version")
    require(".".join(map(str, sys.version_info[:3])) == tc["python"], "Python runtime does not match frozen version")


def compare_authoritative(left: Path, right: Path) -> dict[str, str]:
    require(left.resolve() != right.resolve(), "comparison inputs must be independent directories")
    out = {}
    for name in ORACLE_FILES:
        a, b = left / name, right / name
        require(a.is_file() and b.is_file(), f"missing authoritative comparison bytes: {name}")
        av, bv = a.read_bytes(), b.read_bytes()
        require(av == bv, f"bytewise mismatch: {name}")
        out[name] = sha256(av)
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--structure-only", action="store_true",
                        help="validate frozen documents only; NOT a runtime preflight PASS")
    parser.add_argument("--execution", action="store_true",
                        help="require canonical Genesis and attested Go SQLite engine; intentionally fail until prepared")
    parser.add_argument("--compare", nargs=2, type=Path, metavar=("CONTINUOUS", "REPLAY"))
    args = parser.parse_args()
    try:
        manifest = load_json(ROOT / FOLDER / "manifest.json")
        plan = load_json(ROOT / FOLDER / "scenario.json")
        matrix = load_json(ROOT / FOLDER / "traceability.json")
        adr_text = (ROOT / manifest["normative"]["adr"]["path"]).read_text(encoding="utf-8")
        validate_traceability(matrix, adr_text)
        validate_scenario(manifest, plan)
        check_git(manifest)
        check_policy(manifest)
        if args.structure_only:
            print("STRUCTURE_ONLY: 41 invariants / 68 stresses; no toolchain or execution attestation")
            return 0
        check_versions(manifest)
        if args.execution:
            engine = manifest["toolchain"]["sqlite"]["engine_version"]
            require(bool(engine) and manifest["toolchain"]["sqlite"]["engine_status"] == "ATTESTED",
                    "execution blocked: Go-linked SQLite version not attested")
            require(manifest["genesis"]["canonical_genesis_sha256"] is not None and
                    manifest["genesis"]["status"] == "CANONICAL_VERIFIED",
                    "execution blocked: no canonical sealed Genesis")
        if args.compare:
            d = compare_authoritative(*args.compare)
            print(json.dumps({"status": "BYTE_ORACLE_PASS", "sha256": d}, sort_keys=True))
        print("PLAN_PREFLIGHT_PASS: frozen baseline/policy/41 invariants/68 stresses/toolchain; E2E NOT_RUN")
        return 0
    except (PreflightError, OSError, KeyError, TypeError, IndexError) as exc:
        print(f"PRECHECK_FAIL: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
