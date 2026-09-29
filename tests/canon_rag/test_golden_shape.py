from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
sys.path.insert(0,str(HERE))
from common import load_suite

def test_golden_suite_has_required_cases():
    ids={x["id"] for x in load_suite(REPO)["cases"]}
    expected={
      "normal-kiyotaka-behavior","future-spoiler-ryuen","wrong-character-secret",
      "post-divergence-fact","open-conflict-fact","unverified-fact-abstains",
      "contrast-kiyotaka","contrast-kikyo","no-evidence-for-unknown-actor"}
    assert expected <= ids
