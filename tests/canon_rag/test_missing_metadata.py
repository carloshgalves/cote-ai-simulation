from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
sys.path.insert(0,str(HERE))
from common import corpus_manifest,load_corpus,load_suite
from retrieval import retrieve

def test_missing_metadata_fails_closed():
    c,s=load_corpus(REPO),load_suite(REPO)
    d=corpus_manifest(c)["digest"]
    case=next(x for x in s["cases"] if x["id"]=="normal-kiyotaka-behavior")
    src=next(x for x in c if x["id"]=="ev.kiyotaka.v01.reluctant-social-help")
    ev=dict(src)
    ev.pop("narrative_position")
    out=retrieve([ev],case,s["time_order"],d)
    assert out["abstained"]
    assert out["excluded"][0]["reason"]=="MISSING_GATE_METADATA"
