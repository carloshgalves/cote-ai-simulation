from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
sys.path.insert(0,str(HERE))
from common import corpus_manifest,load_corpus,load_suite
from retrieval import assemble_context,retrieve

def test_context_lanes_stay_separate():
    c,s=load_corpus(REPO),load_suite(REPO)
    d=corpus_manifest(c)["digest"]
    case=next(x for x in s["cases"] if x["id"]=="normal-kiyotaka-behavior")
    out=retrieve(c,case,s["time_order"],d)
    lanes=assemble_context(out)
    assert lanes["CANON_BEHAVIORAL_EVIDENCE"]
    assert lanes["KNOWN_CANON_FACTS"]==[]
