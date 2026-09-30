from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
sys.path.insert(0,str(HERE))
from common import compile_records,corpus_manifest,load_claims,load_corpus,load_source_registry,load_suite
from retrieval import assemble_context,retrieve

def test_context_lanes_stay_separate():
    c,s=load_corpus(REPO),load_suite(REPO)
    claims,registry=load_claims(REPO),load_source_registry(REPO)
    d=corpus_manifest(c,claims,registry)["digest"]
    records=compile_records(c,claims,registry)
    case=next(x for x in s["cases"] if x["id"]=="normal-kiyotaka-behavior")
    out=retrieve(records,case,s["time_order"],d,s["seed_time"])
    lanes=assemble_context(out)
    assert lanes["CANON_BEHAVIORAL_EVIDENCE"]
    assert lanes["KNOWN_CANON_FACTS"]==[]
