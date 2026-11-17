"""Strict verifier for E5 internal hash-only evidence and Go test JSON events."""

import json
import re
import sys
from pathlib import Path

POSITIVE = {
    "normal", "reverse", "seed-1", "seed-42", "seed-2026",
    "seed-144-fault-1", "seed-999-fault-2",
}
NEGATIVE = {
    "signature-cryptographic", "corrupt-private-owner",
    *("corrupt-" + x for x in (
        "input_ledger", "decision_ledger", "event_store", "evidence_ledger",
        "causal_outbox", "genesis-policy", "snapshot", "owner-receipt",
        "source-cursor", "source-closure", "physical-source",
        "physical-signature", "event-visibility",
    )),
}
PHASES = {
    "ingress-declarations", "worker-responses", "abort-validator-inputs",
    "contest-validator-inputs", "candidate-completions", "validator-assessments",
}
TOP = "TestReferenceE5CrossMatrix"


def need(condition, message):
    if not condition:
        raise ValueError(message)


def hash64(value):
    return isinstance(value, str) and re.fullmatch(r"[0-9a-f]{64}", value) is not None


def manifest(payload):
    need(isinstance(payload, list) and len(payload) >= 30, "incomplete manifest")
    entries = {}
    for item in payload:
        need(set(item) == {"Database", "Table", "Rows", "SHA256"}, "manifest shape")
        key = item["Database"], item["Table"]
        need(key not in entries and hash64(item["SHA256"]), "invalid duplicate/hash")
        need(type(item["Rows"]) is int and item["Rows"] >= 0, "invalid row count")
        entries[key] = item
    for db in ("causal", "owner-a", "owner-b"):
        need((db, "__schema__") in entries, "missing DB")
    need(entries["causal", "committed_cycles"]["Rows"] == 9, "wrong commit count")
    need(entries["causal", "world_revisions"]["Rows"] == 10, "wrong world count")
    return entries


def verify(report, events, revision):
    need(report["status"] == "MATRIX_PASS" and report["sha"] == revision, "matrix/SHA")
    need(re.fullmatch(r"[0-9a-f]{40}", revision), "invalid checkout SHA")
    for key in ("run_id", "genesis_hash", "policy_hash"):
        need(hash64(report[key]), "missing " + key)
    cuts = report["signed_cut_and_signature_sha256"]
    need(len(cuts) == 2 and each_pair(cuts) and cuts[0] != cuts[1], "signed cuts")
    rows = report["experiments"]
    names = [row["name"] for row in rows]
    need(len(names) == len(set(names)) and set(names) == POSITIVE | NEGATIVE, "matrix roster differs")
    by_name = {row["name"]: row for row in rows}
    baseline = manifest(by_name["normal"]["after"])
    for name in POSITIVE:
        row = by_name[name]
        need(row["status"] == "BYTE_EQUAL" and not row.get("literal_difference"), name)
        need(row["commits"] == 9, name + " commit count")
        need(manifest(row["before"]) == baseline == manifest(row["after"]), name + " literal manifest")
        if name == "normal":
            continue
        trace = row["realized_order"]
        need(set(trace) == PHASES, name + " phase coverage")
        for values in trace.values():
            need(sorted(values) == [0, 1], name + " permutation")
            if name == "reverse":
                need(values == [1, 0], "reverse phase")
        if "fault" in name:
            need(row["fault_epoch"] in (1, 2) and 1 <= row["fault_step"] <= 7, "fault coordinates")
    for name in NEGATIVE:
        row = by_name[name]
        need(row["status"] == "FAIL_CLOSED" and row.get("guard"), name)
        if name == "signature-cryptographic":
            continue
        need(row["changed_rows"] == 1 and row["commits"] == 9, name + " mutation count")
        need(row.get("literal_difference"), name + " no measured mutation")
        need(manifest(row["before"]) == manifest(row["after"]), name + " mutated after denial")
    expected = {TOP, "TestReferenceE5ScheduleDeterministic"}
    expected |= {TOP + "/" + n for n in (POSITIVE - {"normal"}) | NEGATIVE}
    passes = set()
    for item in events:
        test = item.get("Test")
        if test is None:
            continue
        need(item["Action"] not in ("fail", "skip"), "failed/skipped test: " + test)
        if item["Action"] == "pass":
            need(test not in passes, "duplicate execution: " + test)
            passes.add(test)
    need(passes == expected, "Go test inventory does not match matrix")
    return {"sha": revision, "experiments": len(rows), "passed_tests": sorted(passes),
            "byte_equal": len(POSITIVE), "fail_closed": len(NEGATIVE)}


def each_pair(cuts):
    return all(len(pair) == 2 and all(hash64(value) for value in pair) for pair in cuts)


def main():
    report_path, events_path, revision, out = sys.argv[1:]
    report = json.loads(Path(report_path).read_text())
    events = [json.loads(line) for line in Path(events_path).read_text().splitlines()]
    result = verify(report, events, revision)
    Path(out).write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    main()
