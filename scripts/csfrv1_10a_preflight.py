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
# Independent normative pins for this #115 snapshot. Do not derive these from
# the manifest under test; doing so makes a malicious coordinated re-pin pass.
FROZEN_BASELINE = "a20163b1d5124648192f3adff7c330ed5b4350fe"
FROZEN_TREE = "0ee27ed49a36a42feddd7afce87d29b74f184493"
FROZEN_NORMATIVE = {
    "adr": ("docs/adr/0008-causal-simulation-foundation-v1.md",
            "ec101510e120f86a7bc870e50498a6dd68849ea4"),
    "spec": ("docs/spec/causal-simulation-foundation-v1.md",
             "2f78f7dcaf797ce0ba32f469facb534f8e67b02d"),
    "ticket": ("docs/tickets/csf-runtime-v1/10-adapter-fisico-e-cenario.md",
               "24ff82e7eeae0a14a95032eb775a4274a3d166cd"),
}
FROZEN_BUNDLE = {
    "codec_id": "cote.csf.codec.cbor-det.v1",
    "codec_version": 1,
    "codec_policy_hash": "513111dc82a5e58c07aecdabf410633f5aa5418908d2461ef0dff0d9ae5d8203",
    "schema_bundle_hash": "ef7c1e4cec18f1491e4b72bd7dd35f49a1c28d77a5bdd89d435122074f5cba18",
    "conformance_suite_hash": "05a226340c4211045d6f3df8f034434f3ac336001158379833f7230df4038dfd",
    "manifest_raw_sha256": {
        "policy-manifest.json": "6c4db38cdec9e15a2f21195dd8f2110846ff86721b17c6b197b5d15dbe0ece68",
        "schema-manifest.json": "79e6b37d94fdd04a86a78f167f03c0a04e389e933ec959c06617dd1f934156d3",
        "conformance-manifest.json": "629a238b9367cb0fd14e5b1040816923797dd051a98bb09955b5c7dcfba31902",
    },
}
FROZEN_TOOLCHAIN = {"go": "1.27.2", "python": "3.13.5",
                    "sqlite_driver": "modernc.org/sqlite", "sqlite_driver_version": "v1.60.1"}
FROZEN_PLAN_BLOBS = {
    "manifest.json": "81267b3c88ec0c19e74ff09fea934054cb6926f5",
    "scenario.json": "76bbbe51db55704eb087d4594cb93ed900a25678",
    "traceability.json": "8990c024f19ea5e520c1a0b14f03a6992249a037",
}

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


def validate_frozen_authority(manifest: dict) -> None:
    require(manifest.get("baseline", {}).get("commit") == FROZEN_BASELINE and
            manifest["baseline"].get("tree") == FROZEN_TREE and
            manifest["baseline"].get("ref") == "main",
            "frozen baseline commit/tree cannot be re-pinned")
    for role, (path, blob) in FROZEN_NORMATIVE.items():
        entry = manifest.get("normative", {}).get(role, {})
        require(entry.get("path") == path and entry.get("git_blob") == blob,
                f"frozen normative blob cannot be re-pinned: {role}")
    b = manifest.get("policy_bundle", {})
    require(b.get("directory") == "docs/architecture/canonical-codec-v1-bundle",
            "frozen policy bundle directory cannot be re-pinned")
    for key, expected in FROZEN_BUNDLE.items():
        require(b.get(key) == expected, f"frozen policy cannot be re-pinned: {key}")
    tc = manifest.get("toolchain", {})
    require(tc.get("go") == FROZEN_TOOLCHAIN["go"] and
            tc.get("python") == FROZEN_TOOLCHAIN["python"] and
            tc.get("sqlite", {}).get("driver") == FROZEN_TOOLCHAIN["sqlite_driver"] and
            tc.get("sqlite", {}).get("driver_version") ==
                FROZEN_TOOLCHAIN["sqlite_driver_version"],
            "frozen toolchain cannot be re-pinned")


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
            # #115 is a plan, not the executor. Promoting rows to PASS
            # requires a future verifier that executes named tests and binds
            # exact outputs to SHA, never just truthy strings in JSON.
            require(status == "NEEDS_NEW_INTEGRATION_TEST",
                    f"{kind}-{i:02d} cannot claim PASS in the unexecuted #115 plan")
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
    if g.get("status") == "DECLARATIVE_ONLY_NOT_CANONICAL":
        require(g.get("run_id") is None and g.get("canonical_genesis_sha256") is None and
                g.get("canonical_path") is None,
                "unsigned/declarative Genesis must not masquerade as canonical bytes")
    elif g.get("status") == "CANONICAL_VERIFIED":
        require(isinstance(g.get("run_id"), str) and
                re.fullmatch(r"[0-9a-f]{64}", g["run_id"]) is not None and
                isinstance(g.get("canonical_genesis_sha256"), str) and
                re.fullmatch(r"[0-9a-f]{64}", g["canonical_genesis_sha256"]) is not None and
                isinstance(g.get("canonical_path"), str) and bool(g["canonical_path"]),
                "verified Genesis requires real run ID, raw SHA-256 and canonical path")
    else:
        raise PreflightError("unknown Genesis status")
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
    require(manifest["execution"]["causal_writers"] == ["go.commit-coordinator"] and
            manifest["execution"]["stores"] == [
                "input-ledger", "decision-ledger", "event-store",
                "evidence-ledger", "snapshot-store"],
            "causal authority/storage changed")
    require([s["id"] for s in src] == ["school-events", "physical-a1"] and
            [v["key"] for v in src[0]["inputs"]] == ["school.e1", "school.e2"] and
            src[1]["admission_status"] == "BLOCKED_UNTIL_VERIFIED_EXTENSION_AND_AUTHORIZED_SOURCE",
            "source topology or physical authority changed")
    require([s["closures"][0]["producer_key"] for s in src] ==
            ["school.partial.0", "physical.partial.0"] and
            [s["closures"][1]["producer_key"] for s in src] ==
            ["school.final.10", "physical.final.10"],
            "closure aliases/pins changed")
    require([(x["actor_id"], x["role"], x["pov"], x["slot_id"]) for x in rounds] ==
            [("actor.a", "candidate", "A", "act"),
             ("actor.b", "candidate", "B", "act")],
            "role/POV/slot topology changed")
    require(plan["dispatch"]["response_kind"] == "ActionProposal" and
            plan["dispatch"]["match"] == [
                "run", "cycle", "round", "slot", "actor", "base_revision", "instant"],
            "dispatch authority changed")
    require(c["resource_id"] == "supply.last-unit" and c["proposals"] == ["R1", "R2"] and
            c["winner_oracle"].startswith("derive exclusively from persisted canonical DecisionRecords"),
            "resource or runtime-derived winner oracle changed")
    require(plan["clock"]["initial_ordinal"] == 0 and plan["clock"]["advance_to"] == 10 and
            plan["clock"]["expected_target_ordinal"] == 0, "logical coordinate plan changed")
    require(plan["triggers"][0]["kind"] == "RISING_EDGE" and
            plan["triggers"][0]["activation"] ==
                "durably created with producing CycleCommit" and
            plan["triggers"][0]["due_coordinate"] == "commit_successor_floor",
            "trigger semantics changed")
    require(len(plan["claims"]) == 1 and plan["claims"][0]["truth_in_world"] is False and
            plan["claims"][0]["promote_to_world_truth"] is False and
            plan["claims"][0]["destination"] == "actor.b", "claim promoted to world truth")
    require(len(plan["secrets"]) == 1 and plan["secrets"][0]["holder"] == "actor.a" and
            plan["secrets"][0]["forbidden_to"] == ["actor.b", "public"] and
            plan["secrets"][0]["observatory_privileged_is_not_knowledge"] is True,
            "POV/secret boundary weakened")
    require(plan["physical"]["global_time_from"] == "Go SimulationInstant" and
            plan["physical"]["global_sequence_from"] == "Go CommitCoordinator" and
            set(plan["physical"]["source_fields_private"]) >= {
                "BodyState", "CapacityProfile", "posterior", "rng_state",
                "local_seq", "local_sim_time"},
            "physical telemetry promoted into world/knowledge authority")


def check_frozen_plan_blobs(root: Path = ROOT) -> None:
    # Git blob SHA authenticates the complete raw bytes of the declarative
    # fixture, including seed, claims, policy pins and all 109 GAP statuses.
    for name, expected in FROZEN_PLAN_BLOBS.items():
        path = FOLDER / name
        require(run("git", "hash-object", str(path), cwd=root) == expected,
                f"frozen #115 fixture edited without a contract revision: {name}")


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
    require(left.is_dir() and right.is_dir() and not left.is_symlink() and not right.is_symlink(),
            "comparison directories must be physical and independent")
    seen_inodes: set[tuple[int, int]] = set()
    out: dict[str, str] = {}
    for name in ORACLE_FILES:
        a, b = left / name, right / name
        for path in (a, b):
            require(not path.is_symlink() and path.is_file(),
                    f"missing, symlinked or non-regular authoritative bytes: {name}")
            meta = path.stat()
            require(meta.st_size > 0, f"empty authoritative bytes: {name}")
            inode = (meta.st_dev, meta.st_ino)
            require(inode not in seen_inodes, f"alias/same-inode artifact: {name}")
            seen_inodes.add(inode)
        digest = hashlib.sha256()
        with a.open("rb") as fa, b.open("rb") as fb:
            while True:
                av, bv = fa.read(1 << 20), fb.read(1 << 20)
                require(av == bv, f"bytewise mismatch: {name}")
                if not av:
                    break
                digest.update(av)
        out[name] = digest.hexdigest()
    return out


def check_canonical_genesis(manifest: dict, root: Path = ROOT) -> Path:
    genesis = manifest["genesis"]
    require(genesis["status"] == "CANONICAL_VERIFIED",
            "canonical Genesis not verified by the Go runtime")
    relative = Path(genesis["canonical_path"])
    require(not relative.is_absolute() and
            all(part not in ("..", ".") for part in relative.parts) and
            len(relative.parts) > 1, "unsafe canonical Genesis path")
    path = root / relative
    require(path.is_file() and not path.is_symlink() and
            path.resolve().is_relative_to(root.resolve()), "canonical Genesis not a local regular file")
    require(sha256(path.read_bytes()) == genesis["canonical_genesis_sha256"],
            "canonical Genesis raw bytes/digest mismatch")
    return path


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
        validate_frozen_authority(manifest)
        validate_traceability(matrix, adr_text)
        validate_scenario(manifest, plan)
        check_frozen_plan_blobs()
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
            genesis_path = check_canonical_genesis(manifest)
            actual = run("go", "run", "./tools/csfrv1_10a_sqlite_probe.go",
                         "--genesis", str(genesis_path), "--bundle",
                         manifest["policy_bundle"]["directory"])
            lines = actual.splitlines()
            require(len(lines) == 2 and lines[0] == engine and lines[1] == "GENESIS_VALIDATED:" + manifest["genesis"]["run_id"],
                    "Go-linked SQLite runtime or canonical Genesis rejected")
        if args.compare:
            d = compare_authoritative(*args.compare)
            print(json.dumps({"status": "BYTE_EQUALITY_ONLY_NOT_CAUSAL_ATTESTATION",
                              "sha256": d}, sort_keys=True))
        print("PLAN_PREFLIGHT_PASS: frozen baseline/policy/41 invariants/68 stresses/toolchain; E2E NOT_RUN")
        return 0
    except (PreflightError, OSError, KeyError, TypeError, IndexError) as exc:
        print(f"PRECHECK_FAIL: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
