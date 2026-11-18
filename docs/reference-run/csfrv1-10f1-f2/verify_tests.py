"""Require real completed F1/F2 cases, not empty regex passes or skips."""
import json
import sys
from pathlib import Path

expected = {
    "TestReferenceF1IntegratedProvenance",
    "TestReferenceF2ColdOperationalCuts",
    "TestReferenceF2FinalAndCorruptedAuthority",
    *("TestReferenceF2ColdOperationalCuts/" + n for n in (
        "closure-gap", "in-flight", "halted", "retry-authorized", "trigger-pending",
    )),
}
passed = set()
for line in Path(sys.argv[1]).read_text().splitlines():
    event = json.loads(line)
    name = event.get("Test")
    if not name:
        if event["Action"] == "fail":
            raise SystemExit("package failed")
        continue
    if event["Action"] in ("fail", "skip"):
        raise SystemExit("failed or skipped case: " + name)
    if event["Action"] == "pass":
        if name in passed:
            raise SystemExit("duplicate case: " + name)
        passed.add(name)
if passed != expected:
    raise SystemExit(f"Wrong inventory: missing {expected-passed}, unexpected {passed-expected}")
print(f"Verified {len(passed)} actual F1/F2 test passes")
