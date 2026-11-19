#!/usr/bin/env python3
"""Execute versioned CSF evidence, never promote a planning row by assertion."""
import argparse
import hashlib
import json
import os
import re
import subprocess
from pathlib import Path

REGISTRY = "docs/reference-run/csfrv1-10g/registry.json"
TRACE = "docs/reference-run/csfrv1-10a/traceability.json"
ADR = "docs/adr/0008-causal-simulation-foundation-v1.md"
PIN_TRACE = "8990c024f19ea5e520c1a0b14f03a6992249a037"
PIN_ADR = "ec101510e120f86a7bc870e50498a6dd68849ea4"
PREFIX = "github.com/carloshgalves/cote-ai-simulation/"
OWNERS = {"simulation": 161, "determinism": 162, "embodiment": 163, "knowledge": 164}
REQUIRED = {f"INV-{i:02}" for i in range(1, 42)} | {f"STRESS-{i:02}" for i in range(1, 69)}
TEST_NAME = re.compile(r"Test[A-Z][A-Za-z0-9_]*\Z")
PATH = re.compile(r"(?:internal|tests)/[A-Za-z0-9_/-]+_test\.go\Z")


def unique_pairs(pairs):
    out = {}
    for key, val in pairs:
        if key in out:
            raise ValueError("duplicate JSON key: " + key)
        out[key] = val
    return out


def load(raw):
    return json.loads(raw, object_pairs_hook=unique_pairs)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def git_blob(raw):
    return hashlib.sha1(b"blob " + str(len(raw)).encode() + b"\0" + raw).hexdigest()


def git(root, *args):
    return subprocess.check_output(["git", *args], cwd=root).decode().strip()


def norm_rows(trace):
    if trace["source_blob"] != PIN_ADR:
        raise ValueError("frozen ADR identity mismatch")
    out = {}
    for key, prefix, count in (("invariants", "INV", 41), ("stress_scenarios", "STRESS", 68)):
        group = trace[key]
        if len(group) != count:
            raise ValueError("wrong normative cardinality")
        for n, row in enumerate(group, 1):
            expected = f"{prefix}-{n:02}"
            if row["id"] != expected or row["number"] != n or expected in out:
                raise ValueError("normative roster mismatch")
            out[expected] = row
    return out


def validate(registry, trace, source):
    if set(registry) != {"schema", "baseline_traceability_blob", "baseline_adr_blob", "witnesses", "rows"} or registry["schema"] != "csfrv1-10g.registry.v1":
        raise ValueError("invalid registry schema")
    if registry["baseline_traceability_blob"] != PIN_TRACE or registry["baseline_adr_blob"] != PIN_ADR:
        raise ValueError("registry changed frozen identity")
    normative = norm_rows(trace)
    witnesses = registry["witnesses"]
    keys = set()
    for wid, w in witnesses.items():
        if not re.fullmatch(r"[a-z0-9-]+", wid) or set(w) != {"path", "name", "kind", "subtests"}:
            raise ValueError("invalid witness structure")
        if not PATH.fullmatch(w["path"]) or ".." in w["path"].split("/") or not TEST_NAME.fullmatch(w["name"]):
            raise ValueError("invalid witness path/name")
        if w["kind"] not in {"EXISTING", "INTEGRATED"} or not isinstance(w["subtests"], list):
            raise ValueError("invalid witness kind")
        if any(not isinstance(s, str) or not re.fullmatch(r"[A-Za-z0-9_./-]+", s) or s.startswith("/") or s.endswith("/") for s in w["subtests"]) or len(set(w["subtests"])) != len(w["subtests"]):
            raise ValueError("invalid subtest inventory")
        key = (str(Path(w["path"]).parent), w["name"])
        if key in keys:
            raise ValueError("duplicate test selector")
        keys.add(key)
        pattern = r"(?m)^func\s+" + re.escape(w["name"]) + r"\(\s*\w+\s+\*testing\.T\s*\)"
        if not re.search(pattern, source(w["path"])):
            raise ValueError("missing named test in registered tracked source")
    rows = {}
    used = set()
    for row in registry["rows"]:
        rid = row["id"]
        if rid not in normative or rid in rows:
            raise ValueError("duplicate/foreign normative row")
        required = {"id", "owner_issue", "eval", "state", "witnesses", "reason"}
        if row["state"] == "BLOCKED":
            required.add("blocker_issue")
        if set(row) != required or OWNERS.get(row["eval"]) != row["owner_issue"] or not isinstance(row["reason"], str) or not row["reason"].strip():
            raise ValueError("invalid row fields or ownership")
        refs = row["witnesses"]
        if not isinstance(refs, list) or len(set(refs)) != len(refs) or not set(refs) <= set(witnesses):
            raise ValueError("unregistered/duplicate witness")
        if row["state"] == "READY":
            if not refs:
                raise ValueError("ready row requires executable proof")
            used.update(refs)
        elif row["state"] in {"NOT_RUN", "BLOCKED"}:
            if refs or (row["state"] == "BLOCKED" and (type(row["blocker_issue"]) is not int or row["blocker_issue"] <= 0)):
                raise ValueError("non-executable row must not receive credit")
        else:
            raise ValueError("registry may not predeclare results")
        rows[rid] = row
    if set(rows) != REQUIRED or set(witnesses) != used:
        raise ValueError("missing row or unused witness")
    return normative


def verify_events(raw, package, names):
    """Exact starts, passes and package completion; no skip/cache/extra tests."""
    wanted = set(names)
    started, passed = set(), set()
    package_started = package_passed = False
    for line in raw.splitlines():
        event = load(line)
        action, pkg, name = event.get("Action"), event.get("Package"), event.get("Test")
        if action in {"fail", "skip", "build-fail"} or event.get("FailedBuild"):
            raise ValueError("test/package failed or skipped")
        if action == "build-output":
            continue
        if pkg != package:
            raise ValueError("unexpected package")
        if action == "start" and not name:
            if package_started:
                raise ValueError("duplicate package start")
            package_started = True
            continue
        if not package_started or package_passed:
            raise ValueError("event outside live package")
        if name and name not in wanted:
            raise ValueError("unregistered test/subtest")
        if action == "output" and not name and "(cached)" in event.get("Output", ""):
            raise ValueError("cached result")
        if name and action == "run":
            if name in started:
                raise ValueError("duplicate test start")
            started.add(name)
        elif name and action == "pass":
            if name not in started or name in passed:
                raise ValueError("pass without unique start")
            passed.add(name)
        elif not name and action == "pass":
            if passed != wanted:
                raise ValueError("premature package pass")
            package_passed = True
        elif action not in {"output", "pause", "cont"}:
            raise ValueError("unexpected test action")
    if not package_started or not package_passed or started != wanted or passed != wanted:
        raise ValueError("incomplete executed inventory")
    return sorted(passed)


def execute(root, directory, witnesses):
    results = {}
    env = dict(os.environ)
    env.update(GOFLAGS="", GOWORK="off", GOTOOLCHAIN="local", PHYSICAL_A1_PRODUCER_TEST="1")
    for wid, w in sorted(witnesses.items()):
        package = str(Path(w["path"]).parent)
        names = [w["name"], *(w["name"] + "/" + s for s in w["subtests"])]
        results[wid] = {}
        for mode in ("normal", "race"):
            args = ["go", "test", "-json", "-count=1", "-timeout=15m"]
            if mode == "race":
                args.append("-race")
            args.extend(["./" + package, "-run", "^" + w["name"] + "$"])
            proc = subprocess.run(args, cwd=root, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
            # Raw logs remain local private test material, not the CI certificate.
            for suffix, content in (("jsonl", proc.stdout), ("stderr", proc.stderr)):
                with (directory / f"{wid}-{mode}.{suffix}").open("xb") as f:
                    f.write(content)
            status, reason, tests = "FAIL", "nonzero exit", []
            if proc.returncode == 0:
                try:
                    tests = verify_events(proc.stdout, PREFIX + package, names)
                    status, reason = "PASS", ""
                except (ValueError, TypeError, AttributeError) as exc:
                    reason = str(exc)
            result = {"status": status, "command": args, "exit_status": proc.returncode,
                      "log_sha256": digest(proc.stdout), "stderr_sha256": digest(proc.stderr),
                      "test_names": tests, "reason": reason}
            results[wid][mode] = result
            print(f"{wid}/{mode}: {status} exit={proc.returncode} {reason}", flush=True)
    return results


def report(registry, normative, results, sha, registry_hash):
    rows = []
    for item in registry["rows"]:
        status = item["state"]
        refs = item["witnesses"]
        if status == "READY":
            ok = all(w in results and all(results[w].get(mode, {}).get("status") == "PASS" for mode in ("normal", "race")) for w in refs)
            status = ("PASS_INTEGRATED_TEST" if any(registry["witnesses"][w]["kind"] == "INTEGRATED" for w in refs) else "PASS_EXISTING_TEST") if ok else "FAIL"
        row = dict(item, status=status)
        norm = normative[item["id"]]
        row["normative_source"] = norm["normative_source"]
        row["normative_statement"] = norm.get("normative_text", norm.get("scenario"))
        if "required_proof" in norm:
            row["required_proof"] = norm["required_proof"]
        row["commit_sha"] = sha if refs else None
        rows.append(row)
    counts = {}
    for row in rows:
        group = row["id"].split("-")[0]
        tally = counts.setdefault(group, {})
        tally[row["status"]] = tally.get(row["status"], 0) + 1
    complete = all(row["status"].startswith("PASS_") for row in rows)
    return {"schema": "csfrv1-10g.evidence.v1", "sha": sha, "registry_sha256": registry_hash,
            "normative_traceability_blob": PIN_TRACE, "normative_adr_blob": PIN_ADR,
            "counts": counts, "rows": rows, "witnesses": results,
            "matrix_complete": complete, "dod_accepted": False, "release_authorized": False,
            "external_gates": {"93": "NOT_ATTESTED", "94": "NOT_ATTESTED", "113": "NOT_ATTESTED"},
            "disposition": "EVIDENCE_COMPLETE_REVIEW_REQUIRED" if complete else "NO_GO_INCOMPLETE_MATRIX"}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--sha", required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--require-complete", action="store_true")
    args = parser.parse_args()
    root = Path(git(Path.cwd(), "rev-parse", "--show-toplevel"))
    sha = git(root, "rev-parse", "HEAD")
    if args.sha != sha or not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise SystemExit("exact checkout SHA required")
    if git(root, "status", "--porcelain", "--untracked-files=all"):
        raise SystemExit("clean tracked and untracked checkout required")
    trace_raw, adr_raw, raw = ((root / p).read_bytes() for p in (TRACE, ADR, REGISTRY))
    if git_blob(trace_raw) != PIN_TRACE or git_blob(adr_raw) != PIN_ADR:
        raise SystemExit("frozen normative inputs changed")
    registry, trace = load(raw), load(trace_raw)
    def source(path):
        git(root, "ls-files", "--error-unmatch", "--", path)
        return (root / path).read_text(encoding="utf-8")
    normative = validate(registry, trace, source)
    # Exclusive output outside repository; do not accidentally overwrite evidence.
    directory = args.out.resolve()
    if directory == root or root in directory.parents:
        raise SystemExit("evidence directory must be outside checkout")
    directory.mkdir(mode=0o700, parents=False, exist_ok=False)
    os.umask(0o077)
    results = execute(root, directory, registry["witnesses"])
    if git(root, "rev-parse", "HEAD") != sha or git(root, "status", "--porcelain", "--untracked-files=all"):
        raise SystemExit("checkout changed during execution; no certificate")
    certificate = report(registry, normative, results, sha, digest(raw))
    with (directory / "evidence.json").open("x", encoding="utf-8") as f:
        json.dump(certificate, f, ensure_ascii=False, indent=2)
        f.write("\n")
    print(json.dumps(certificate["counts"], sort_keys=True), flush=True)
    if any(row["status"] == "FAIL" for row in certificate["rows"]):
        return 1
    if args.require_complete and not certificate["matrix_complete"]:
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
