"""Strict F5 acceptance certificate: exact test/package inventory, two uncached runs."""
import argparse
import hashlib
import json
import os
import re
import subprocess
from pathlib import Path

PREFIX = "github.com/carloshgalves/cote-ai-simulation/"
COMMIT = PREFIX + "internal/csf/commit"
SQLITE = PREFIX + "internal/csf/persistence/sqlite"
MATRIX = {
    COMMIT: {
        "TestReferenceF1IntegratedProvenance",
        "TestReferenceF2ColdOperationalCuts",
        "TestReferenceF2FinalAndCorruptedAuthority",
        *("TestReferenceF2ColdOperationalCuts/" + n for n in (
            "closure-gap", "in-flight", "halted", "retry-authorized", "trigger-pending",
        )),
        "TestReferenceF3CrossedViews",
        "TestReferenceF3RestrictedAllowlists",
        *("TestReferenceF3CrossedViews/" + n for n in (
            "physical-event", "knowledge-input", "foreign-holder", "swapped-dispatch",
            "changed-live-dispatch", "unsupported-public-view", "cancellation",
        )),
        "TestReferenceF4ConcurrentReadOnly",
        "TestReferenceF4PathAndPagerControls",
        *("TestReferenceF4PathAndPagerControls/" + n for n in (
            "symlink", "hardlink", "parent-symlink", "group-write-file",
            "group-write-parent", "pager-link",
        )),
        "TestReferenceF5EquivalentIntegratedViews",
        *("TestReferenceF5EquivalentIntegratedViews/" + n for n in (
            "normal", "reverse", "recovered",
        )),
        "TestReferenceF5LiveWALFailClosed",
        "TestReferenceF5HumanAllowlists",
        "TestReferenceF5OracleIncludesNearReservedNames",
    },
    PREFIX + "internal/csf/operationalmetrics": {
        "TestF4MetricsPagerAndForeignSchema",
        "TestF4MetricsInjectedTriggerAndPathReplacement",
    },
    SQLITE: {
        "TestCSFRV19O1SQLiteReadOnlyModeAndIndependentQueryOnlyDefense",
        "TestF4SegregatedReadCapabilities",
        "TestF5LiveWriterVersionFence",
    },
    PREFIX + "internal/readonlysql": {"TestF4ConnectorCannotReopen"},
    PREFIX + "internal/localdbguard": {"TestF4ActualACLAndPinReplacement"},
    PREFIX + "tests/architecture": {"TestF4ReadCapabilityTransitiveDependencies"},
}
REGRESSIONS = {
    COMMIT: {
        "TestCSFRV19O1PristineAndCommittedRunReadonly",
        "TestCSFRV19O1InFlightAbortAndExplicitRetry",
        "TestCSFRV19O1RejectsForeignRunCorruptClockAndControl",
        "TestCSFRV19O1RejectsAbsentSQLiteWithoutCreatingFiles",
        "TestCSFRV19O1PublicReaderExposesNoWriteMethods",
        "TestCSFRV19O2CommittedCausalChainAndNoSideEffects",
        "TestCSFRV19O2AbortRetryLifecycleWithoutWorldMutation",
        "TestCSFRV19O2RejectsTamperedCycleAndEventAuthorities",
        "TestCSFRV19O2RejectsTamperedCycleAndEventAuthorities/decision_ledger",
        "TestCSFRV19O2RejectsTamperedCycleAndEventAuthorities/event_store",
        "TestCSFRV19O2RejectsForgedAbortAndRetryWithoutLeakingRows",
        "TestCSFRV19O2RejectsForgedAbortAndRetryWithoutLeakingRows/abort",
        "TestCSFRV19O2RejectsForgedAbortAndRetryWithoutLeakingRows/retry",
        "TestCSFRV19O2RejectsOrphanTerminalJournal",
        "TestCSFRV19O2ReportsExplicitSourceClosureGap",
        "TestCSFRV19O2ReportsPendingPerceptionTasks",
        "TestCSFRV19O2RejectsForeignFenceAuthority",
        "TestCSFRV19O2RejectsFalseIdleAfterUnrepairedAbort",
        "TestCSFRV19O2AddressedEvidenceAndWorldRevisionAuthority",
        "TestCSFRV19O2RejectsOrphanDecisionRecord",
        "TestCSFRV19O3POVStrictlyRecipientScopedAndReadonly",
        "TestCSFRV19O3StaleDispatchCannotReadFutureEvidence",
        "TestCSFRV19O3PublicRequiresExplicitPublicationRoute",
        "TestCSFRV19O3RejectsForgedRecipientRoute",
        "TestCSFRV19O3ActorDoesNotInheritPrivilegedEventTruth",
        "TestCSFRV19O3UnknownPublicationSchemaFailsClosed",
        "TestCSFRV19O4IntegratedOperatorPOVAPOVBPublicAndMetrics",
        "TestCSFRV19O4AbortRetryPendingAndPublicRemainExtracausal",
        "TestReferenceD3RealPythonPrivateEventAndLiveObservatory",
        "TestReferenceD3AdversarialEventTaskAndHolderMutationsFailClosed",
        "TestReferenceD3ImmutablePrefixesUnderSourceWorkerPermutation",
        "TestReferenceE1SameSignedSourcesWholeCausalAndOwnerByteTranscript",
        "TestReferenceE1MismatchAndMissingAuthorityFailClosed",
        "TestReferenceE1RejectsCorruptCanonicalAndReadOnlyCannotWrite",
    }
}


def verify(raw, expected):
    """Reject absent, duplicate, fabricated, skipped, cached or failed cases."""
    wanted = {(pkg, test) for pkg, tests in expected.items() for test in tests}
    starts, passes, pkg_starts, pkg_passes = set(), set(), set(), set()
    for lineno, line in enumerate(raw.splitlines(), 1):
        try:
            event = json.loads(line)
        except (ValueError, UnicodeError) as exc:
            raise ValueError(f"invalid JSONL line {lineno}") from exc
        if not isinstance(event, dict):
            raise ValueError(f"invalid event object {lineno}")
        action, package, name = event.get("Action"), event.get("Package"), event.get("Test")
        if action in {"fail", "skip", "build-fail"} or event.get("FailedBuild"):
            raise ValueError(f"failed/skipped case {package}/{name}")
        if action == "output" and not name and "(cached)" in event.get("Output", ""):
            raise ValueError(f"cached package {package}")
        # Successful build telemetry can be emitted before package start.
        if action == "build-output":
            continue
        if package not in expected:
            raise ValueError(f"unexpected package {package}")
        item = (package, name)
        if action == "start" and not name:
            if package in pkg_starts:
                raise ValueError(f"duplicate package start {package}")
            pkg_starts.add(package)
            continue
        if package not in pkg_starts or package in pkg_passes:
            raise ValueError(f"event outside live package {package}")
        if name and item not in wanted:
            raise ValueError(f"unexpected case {item}")
        if name and action == "run":
            if item in starts:
                raise ValueError(f"duplicate start {item}")
            starts.add(item)
        elif name and action == "pass":
            if item not in starts or item in passes:
                raise ValueError(f"missing start or duplicate pass {item}")
            passes.add(item)
        elif not name and action == "pass":
            if not all((package, test) in passes for test in expected[package]):
                raise ValueError(f"premature package pass {package}")
            pkg_passes.add(package)
        elif action not in {"output", "pause", "cont"}:
            raise ValueError(f"unexpected action {action}")
    if starts != wanted or passes != wanted or pkg_starts != set(expected) or pkg_passes != set(expected):
        raise ValueError(f"incomplete inventory: missing={sorted(wanted-passes)}")
    return {
        "status": "PASS",
        "test_count": len(passes),
        "package_count": len(pkg_passes),
        "jsonl_sha256": hashlib.sha256(raw).hexdigest(),
        "tests": {pkg: sorted(expected[pkg]) for pkg in sorted(expected)},
    }


def verify_evidence(raw):
    """Require actual per-case E1 digest/count summaries, with no private data."""
    names = {"normal", "reverse", "recovered"}
    rows = {}
    fields = {"case", "commits", "attempts", "databases", "tables",
              "before_sha256", "after_sha256"}
    for line in raw.splitlines():
        event = json.loads(line)
        output = event.get("Output", "")
        if event.get("Action") != "output" or "F5_EVIDENCE " not in output:
            continue
        row = json.loads(output.split("F5_EVIDENCE ", 1)[1].strip())
        name = row.get("case")
        if (set(row) != fields or name not in names or name in rows
                or event.get("Package") != COMMIT
                or event.get("Test") != "TestReferenceF5EquivalentIntegratedViews/" + name
                or row["commits"] != 9 or row["attempts"] != 10 or row["databases"] != 3
                or type(row["tables"]) is not int or row["tables"] < 20
                or not re.fullmatch(r"[0-9a-f]{64}", row["before_sha256"])
                or row["before_sha256"] != row["after_sha256"]):
            raise ValueError("invalid F5 E1 evidence")
        rows[name] = row
    if set(rows) != names:
        raise ValueError("missing F5 E1 evidence")
    if len({row["before_sha256"] for row in rows.values()}) != 1:
        raise ValueError("equivalent executions have different E1 transcript digests")
    if len({row["tables"] for row in rows.values()}) != 1:
        raise ValueError("equivalent executions have different table counts")
    return [rows[n] for n in sorted(names)]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--normal", type=Path, required=True)
    parser.add_argument("--race", type=Path, required=True)
    parser.add_argument("--regressions", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--sha", required=True)
    args = parser.parse_args()
    checkout = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    if not re.fullmatch(r"[0-9a-f]{40}", args.sha) or args.sha != checkout:
        raise SystemExit("certificate requires exact current checkout SHA")
    paths = [args.normal, args.race, args.regressions]
    if len({p.resolve(strict=True) for p in paths}) != len(paths):
        raise SystemExit("normal, race, regressions require distinct input files")
    normal, race, regressions = (p.read_bytes() for p in paths)
    if len({hashlib.sha256(raw).digest() for raw in (normal, race, regressions)}) != 3:
        raise SystemExit("certificate requires distinct execution logs")
    try:
        passes = {
            "normal": verify(normal, MATRIX),
            "race": verify(race, MATRIX),
            "regressions": verify(regressions, REGRESSIONS),
        }
        passes["normal"]["integrated_e1"] = verify_evidence(normal)
        passes["race"]["integrated_e1"] = verify_evidence(race)
    except ValueError as exc:
        raise SystemExit(str(exc)) from exc
    report = {
        "schema": "csfrv1-10f5.acceptance.v1",
        "sha": checkout,
        "status": "LOCAL_TEST_MATRIX_PASS",
        "run_id": os.environ.get("GITHUB_RUN_ID", "local"),
        "run_attempt": os.environ.get("GITHUB_RUN_ATTEMPT", "local"),
        "matrix": passes,
        "external_gates": {"93": "NOT_ATTESTED", "94": "NOT_ATTESTED", "113": "NOT_ATTESTED"},
        "release_authorized": False,
        "human_merge_required": True,
        "scope": "local experimental acceptance; not an independent audit or production homologation",
    }
    with args.out.open("x", encoding="utf-8") as output:
        json.dump(report, output, indent=2)
        output.write("\n")
    print(f"F5 exact SHA {checkout}: {passes['normal']['test_count']} normal, "
          f"{passes['race']['test_count']} race, {passes['regressions']['test_count']} regressions")


if __name__ == "__main__":
    main()
