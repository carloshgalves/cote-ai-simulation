"""Reject skipped, missing, duplicate or failed cases in the F3 matrix."""
import json
import sys
from pathlib import Path

expected = {
    "TestReferenceF3CrossedViews",
    "TestReferenceF3RestrictedAllowlists",
    *("TestReferenceF3CrossedViews/" + name for name in (
        "physical-event",
        "knowledge-input",
        "foreign-holder",
        "swapped-dispatch",
        "changed-live-dispatch",
        "unsupported-public-view",
        "cancellation",
    )),
}
started = set()
passed = set()
packages = set()
for line in Path(sys.argv[1]).read_text().splitlines():
    event = json.loads(line)
    action = event["Action"]
    name = event.get("Test")
    if action in ("fail", "skip"):
        raise SystemExit(f"failed/skipped {name or event['Package']}")
    if name and action == "run":
        if name in started:
            raise SystemExit(f"duplicate start {name}")
        started.add(name)
    if name and action == "pass":
        if name in passed:
            raise SystemExit(f"duplicate pass {name}")
        passed.add(name)
    if not name and action == "pass":
        packages.add(event["Package"])
if started != expected or passed != expected or packages != {
    "github.com/carloshgalves/cote-ai-simulation/internal/csf/commit"
}:
    raise SystemExit(f"incomplete execution: missing starts={expected-started}, passes={expected-passed}")
print(f"Verified {len(expected)} real F3 test passes")
