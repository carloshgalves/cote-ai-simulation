#!/usr/bin/env python3
"""G6: independently reconcile a completed G1–G5 run and Go/TS V1 conformance.

This program does NOT turn a locally runnable eval into external attestation,
and never reclassifies NOT_RUN/BLOCKED. Both reports are from one clean exact
commit, a single CI job and the immutable source bundle. Inputs are untrusted.
"""
import argparse
import hashlib
import json
import os
import platform
import re
from pathlib import Path

import run_evals as g

BUNDLE = "docs/architecture/canonical-codec-v1-bundle"
EXPECTED_GAP_ISSUES = {"simulation": 168, "determinism": 169, "embodiment": 172, "knowledge": 174}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha256(raw):
    return hashlib.sha256(raw).hexdigest()


def strict(raw):
    return g.load(raw)


def source_roster(root, registry, trace):
    require(g.git_blob((root / g.TRACE).read_bytes()) == g.PIN_TRACE, "frozen traceability changed")
    require(g.git_blob((root / g.ADR).read_bytes()) == g.PIN_ADR, "frozen ADR changed")

    def read_source(path):
        expected = g.git(root, "rev-parse", "HEAD:" + path)
        entry = g.git(root, "ls-files", "--stage", "--", path).split()
        require(len(entry) == 4 and entry[0] in {"100644", "100755"} and entry[2:] == ["0", path],
                "witness source is not a tracked committed regular file")
        file = root / path
        require(file.is_file() and not file.is_symlink() and file.resolve() == file,
                "witness source path not a direct regular file")
        raw = file.read_bytes()
        require(g.git_blob(raw) == expected, "witness source differs from HEAD")
        return raw.decode("utf-8")

    normative = g.validate(registry, trace, read_source)
    return normative, read_source


def replay_sanitized(events, package, names):
    require(type(events) is list and events, "missing sanitized Go events")
    formatted = []
    for event in events:
        require(type(event) is dict and set(event) in ({"action", "package"}, {"action", "package", "test"}),
                "unsafe sanitized event shape")
        require(event["action"] in {"start", "run", "pass"} and event["package"] == package,
                "unexpected sanitized event")
        obj = {"Action": event["action"], "Package": package}
        if "test" in event:
            require(type(event["test"]) is str and event["test"] in names, "foreign test in sanitized event")
            obj["Test"] = event["test"]
        formatted.append(json.dumps(obj).encode())
    return g.verify_events(b"\n".join(formatted) + b"\n", package, names)


def check_attested_evidence(root, raw, sha, registry, normative, source, logs_dir):
    doc = strict(raw)
    require(type(doc) is dict and doc.get("schema") == "csfrv1-10g.evidence.v1",
            "missing G1–G5 evidence schema")
    registry_raw = (root / g.REGISTRY).read_bytes()
    require(doc.get("sha") == sha and doc.get("registry_sha256") == g.digest(registry_raw),
            "stale SHA or registry evidence hash")
    require(doc.get("normative_traceability_blob") == g.PIN_TRACE and doc.get("normative_adr_blob") == g.PIN_ADR,
            "normative source identity mismatch")
    require(re.fullmatch(r"go version go1\.27\.2 linux/\w+", doc.get("go_version", "")) is not None,
            "unapproved Go toolchain")
    require(doc.get("python_version") == platform.python_version(),
            "evidence Python differs from G6 current Python")
    if os.getenv("GITHUB_RUN_ID"):
        require(doc.get("ci_run") == os.environ["GITHUB_RUN_ID"] and
                doc.get("ci_attempt") == os.getenv("GITHUB_RUN_ATTEMPT"),
                "G6 must consume evidence from the same CI invocation")

    original = doc.get("witnesses")
    require(type(original) is dict and set(original) == set(registry["witnesses"]),
            "missing, extra or duplicated witness")
    failures = []
    for wid, registered in registry["witnesses"].items():
        result = original[wid]
        require(type(result) is dict and set(result) == {
            "source", "name", "kind", "source_git_blob", "normal", "race"},
            "witness record shape differs from runner")
        require(result["source"] == registered["path"] and result["name"] == registered["name"] and
                result["kind"] == registered["kind"] and
                result["source_git_blob"] == g.git_blob(source(registered["path"]).encode("utf-8")),
                "mismatched registered witness source blob")
        package = g.PREFIX + str(Path(registered["path"]).parent)
        names = [registered["name"], *(registered["name"] + "/" + child for child in registered["subtests"])]
        for mode in ("normal", "race"):
            row = result[mode]
            require(type(row) is dict and set(row) == {
                "status", "command", "exit_status", "failed_registered_tests",
                "log_sha256", "stderr_sha256", "test_names", "reason",
                "sanitized_events", "sanitized_events_sha256"},
                "witness mode has incomplete or foreign fields")
            expected_command = ["go", "test", "-json", "-count=1",
                                "-timeout=" + g.witness_timeout(wid, mode)]
            if mode == "race":
                expected_command.append("-race")
            expected_command += ["./" + str(Path(registered["path"]).parent),
                                 "-run", "^" + registered["name"] + "$"]
            require(row["command"] == expected_command, "changed normal/race command or timeout")
            require(all(type(row[field]) is str and re.fullmatch(r"[0-9a-f]{64}", row[field])
                        for field in ("log_sha256", "stderr_sha256", "sanitized_events_sha256")),
                    "missing raw/sanitized transcript digest")
            require(row["sanitized_events_sha256"] == g.digest(json.dumps(
                row["sanitized_events"], sort_keys=True, separators=(",", ":")).encode()),
                "sanitized event digest changed")
            # Re-read PRIVATE raw Go -json stdout/stderr from this same CI
            # invocation. The G6 artifact never uploads them, but the
            # independent aggregator must not merely trust a green label or
            # a self-reported digest inside the G1-G5 certificate.
            raw_files = {}
            for suffix, key in (("jsonl", "log_sha256"), ("stderr", "stderr_sha256")):
                log = logs_dir / (wid + "-" + mode + "." + suffix)
                require(log.is_file() and not log.is_symlink(),
                        "private original witness log missing")
                payload = log.read_bytes()
                require(sha256(payload) == row[key], "private original witness digest mismatch")
                raw_files[suffix] = payload
            if row["status"] == "PASS":
                direct_history = []
                actual_tests = g.verify_events(raw_files["jsonl"], package, names,
                                               transcript=direct_history)
                require(actual_tests == row["test_names"] and
                        direct_history == row["sanitized_events"],
                        "sanitized execution diverges from private Go event source")
            if row["status"] == "PASS":
                require(row["exit_status"] == 0 and row["reason"] == "" and
                        row["failed_registered_tests"] == [] and row["test_names"] == sorted(names),
                        "green label without actual exact-test execution")
                require(replay_sanitized(row["sanitized_events"], package, names) == sorted(names),
                        "sanitized event history is incomplete or reordered")
            else:
                require(row["status"] == "FAIL" and row["exit_status"] is not None and
                        row["test_names"] == [] and row["sanitized_events"] == [],
                        "non-PASS witness not classified as failure")
                failures.append(wid + "/" + mode)

    again = g.report(registry, normative, original, sha, g.digest(registry_raw))
    for field in ("schema", "sha", "registry_sha256", "normative_traceability_blob",
                  "normative_adr_blob", "counts", "rows", "evals",
                  "matrix_complete", "evals_complete", "dod_accepted", "release_authorized",
                  "external_gates", "disposition"):
        require(doc.get(field) == again[field], "certificate disagrees with independently recomputed " + field)
    require(doc["dod_accepted"] is False and doc["release_authorized"] is False,
            "source claimed an unauthorized release")
    return again, failures


def conformance_case_ids(root):
    fixture = strict((root / BUNDLE / "fixtures.json").read_bytes())
    transitions = strict((root / BUNDLE / "causal-transition-fixtures.json").read_bytes())
    ids = []
    for key in ("normalization_cases", "sha256_vectors", "positive", "negative", "semantic"):
        require(type(fixture.get(key)) is list, "conformance bundle missing section")
        ids.extend(item["case_id"] for item in fixture[key])
    require(type(transitions.get("cases")) is list, "transition cases absent")
    ids.extend(item["case_id"] for item in transitions["cases"])
    require(len(ids) == 621 and len(set(ids)) == 621 and
            all(type(v) is str and v for v in ids), "canonical 621-case roster is incomplete")
    return ids


def conformance_bundle_hash(root):
    lines = (root / BUNDLE / "BUNDLE.sha256").read_text().splitlines()
    found = [line.split() for line in lines if line.startswith("conformance_suite_hash ")]
    require(len(found) == 1 and len(found[0]) == 2 and
            re.fullmatch(r"[0-9a-f]{64}", found[0][1]) is not None, "invalid pinned conformance receipt")
    return found[0][1]


def check_conformance(root, go_raw, ts_raw):
    require(go_raw == ts_raw, "Go and TypeScript report bytes differ")
    obj = strict(go_raw)
    require(type(obj) is dict and set(obj) == {"suite_hash", "total", "passed", "failed", "cases"},
            "conformance output schema invalid")
    require(obj["suite_hash"] == conformance_bundle_hash(root), "conformance suite hash untrusted")
    require(obj["total"] == 621 and obj["passed"] == 621 and obj["failed"] == 0,
            "conformance not 621/621")
    ids = conformance_case_ids(root)
    require(type(obj["cases"]) is list and len(obj["cases"]) == 621,
            "conformance case roster missing")
    for case, expected in zip(obj["cases"], ids):
        require(type(case) is dict and case == {"id": expected, "status": "pass"},
                "one case failed, changed order or imported an unregistered case")
    return {"status": "PASS_GO_TS_EXACT_BYTES", "total": 621, "passed": 621,
            "suite_hash": obj["suite_hash"], "identical_report_sha256": sha256(go_raw)}


def finalize(sha, evidence, failures, conformance, registry):
    row_states = [item["status"] for item in evidence["rows"]]
    gaps = []
    for item in evidence["rows"]:
        if item["status"] in {"NOT_RUN", "BLOCKED", "FAIL"}:
            gaps.append({
                "id": item["id"], "eval": item["eval"], "owner_issue": item["owner_issue"],
                "state": item["status"], "reason": item["reason"],
                "blocker_issue": item.get("blocker_issue"),
                "followup_issue": EXPECTED_GAP_ISSUES[item["eval"]],
                "normative_source": item["normative_source"],
            })
    require(len(evidence["rows"]) == 109 and
            sum(row["id"].startswith("INV-") for row in evidence["rows"]) == 41 and
            sum(row["id"].startswith("STRESS-") for row in evidence["rows"]) == 68,
            "incomplete normative inventory")
    require(all(group["scope"] in {"SCOPED_WITH_GAPS", "PILOT_ONLY"} and not group["complete"]
                for group in evidence["evals"].values()), "unreviewed complete eval scope")
    require(set(evidence["evals"]) == set(g.OWNERS), "four evals not reconciled")
    if failures or "FAIL" in row_states or any(v["status"] == "FAIL" for v in evidence["evals"].values()):
        status = "NO_GO_FAILED_EVIDENCE"
    elif gaps:
        status = "NO_GO_INCOMPLETE_MATRIX"
    else:
        status = "NO_GO_INCOMPLETE_EVAL_SCOPE"
    # This is a strong local CI certificate, never an externally signed release.
    decision = {
        "schema": "csfrv1-10g.g6-decision.v1", "sha": sha,
        "evidence": {"rows": 109, "counts": evidence["counts"], "evals": {
            k: {"scope": v["scope"], "status": v["status"], "complete": False}
            for k, v in sorted(evidence["evals"].items())},
            "witnesses": len(registry["witnesses"]), "failed_modes": sorted(failures)},
        "conformance": conformance, "gaps": {
            "total": len(gaps), "blocked": sum(x["state"] == "BLOCKED" for x in gaps),
            "not_run": sum(x["state"] == "NOT_RUN" for x in gaps),
            "failed": sum(x["state"] == "FAIL" for x in gaps),
            "by_owner_issue": {str(owner): sum(x["owner_issue"] == owner for x in gaps)
                               for owner in sorted(set(x["owner_issue"] for x in gaps))},
        },
        "external_gates": {"93": "NOT_ATTESTED", "94": "NOT_ATTESTED", "113": "NOT_ATTESTED"},
        "independent_review": "NOT_ATTESTED",
        "matrix_complete": bool(evidence["matrix_complete"]),
        "evals_complete": bool(evidence["evals_complete"]),
        "dod_accepted": False, "release_authorized": False,
        "disposition": status,
    }
    return decision, {
        "schema": "csfrv1-10g.g6-gaps.v1", "sha": sha, "total": len(gaps),
        "rows": gaps,
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--sha", required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument("--go-report", type=Path, required=True)
    parser.add_argument("--ts-report", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()

    root = Path(g.git(Path.cwd(), "rev-parse", "--show-toplevel"))
    require(re.fullmatch(r"[0-9a-f]{40}", args.sha) is not None and
            g.git(root, "rev-parse", "HEAD") == args.sha and g.clean_checkout(root),
            "G6 requires one clean exact committed SHA")
    registry = strict((root / g.REGISTRY).read_bytes())
    trace = strict((root / g.TRACE).read_bytes())
    normative, source = source_roster(root, registry, trace)
    evidence_raw = args.evidence.read_bytes()
    base, failures = check_attested_evidence(root, evidence_raw, args.sha, registry, normative, source, args.evidence.parent)
    go_raw, ts_raw = args.go_report.read_bytes(), args.ts_report.read_bytes()
    conf = check_conformance(root, go_raw, ts_raw)
    decision, gaps = finalize(args.sha, base, failures, conf, registry)
    decision["evidence_sha256"] = sha256(evidence_raw)
    decision["go_version"] = base.get("go_version", "go1.27.2")
    decision["python_version"] = platform.python_version()
    decision["node_version"] = os.getenv("G6_NODE_VERSION", "NOT_ATTESTED")
    decision["ci_run"] = os.getenv("GITHUB_RUN_ID")
    require(args.out.is_dir() and root not in args.out.resolve().parents and
            args.out.resolve() != root, "private output directory required")
    for name, doc in (("g6-decision.json", decision), ("g6-gaps.json", gaps)):
        target = args.out / name
        with target.open("x", encoding="utf-8") as handle:
            json.dump(doc, handle, ensure_ascii=False, indent=2)
            handle.write("\n")
    print(json.dumps({"sha": args.sha, "disposition": decision["disposition"],
                      "gaps": decision["gaps"], "conformance": conf["status"]},
                     sort_keys=True), flush=True)
    return 0 if not failures else 1


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, OSError, KeyError, TypeError) as exc:
        raise SystemExit("G6_NO_CERTIFICATE: " + str(exc)) from exc
