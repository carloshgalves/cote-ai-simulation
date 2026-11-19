"""Fail on missing, skipped or duplicate F4 cases."""
import json
import sys
from pathlib import Path

prefix = "github.com/carloshgalves/cote-ai-simulation/"
expected = {
    prefix + "internal/csf/commit": {
        "TestReferenceF4ConcurrentReadOnly",
        "TestReferenceF4PathAndPagerControls",
        *("TestReferenceF4PathAndPagerControls/" + name for name in (
            "symlink", "hardlink", "parent-symlink", "group-write-file",
            "group-write-parent", "pager-link",
        )),
    },
    prefix + "internal/csf/operationalmetrics": {
        "TestF4MetricsPagerAndForeignSchema",
        "TestF4MetricsInjectedTriggerAndPathReplacement",
    },
    prefix + "internal/csf/persistence/sqlite": {
        "TestCSFRV19O1SQLiteReadOnlyModeAndIndependentQueryOnlyDefense",
    },
    prefix + "internal/localdbguard": {"TestF4ActualACLAndPinReplacement"},
    prefix + "tests/architecture": {"TestF4ReadCapabilityTransitiveDependencies"},
}
wanted = {(package, test) for package, tests in expected.items() for test in tests}
started, passed, packages = set(), set(), set()
for line in Path(sys.argv[1]).read_text().splitlines():
    event = json.loads(line)
    action = event["Action"]
    name = event.get("Test")
    package = event.get("Package")
    if action in {"fail", "skip"}:
        raise SystemExit(f"failed/skipped {package}/{name}")
    item = (package, name)
    if name and action == "run":
        if item in started:
            raise SystemExit(f"duplicate test start {item}")
        started.add(item)
    if name and action == "pass":
        if item in passed:
            raise SystemExit(f"duplicate test pass {item}")
        passed.add(item)
    if not name and action == "pass":
        packages.add(package)
if started != wanted or passed != wanted or packages != set(expected):
    raise SystemExit(f"incomplete F4 matrix: missing {wanted-passed}, unexpected {passed-wanted}")
print(f"Verified {len(wanted)} required F4 tests")
