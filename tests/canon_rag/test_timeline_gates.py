from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
sys.path.insert(0,str(HERE))
from common import corpus_manifest,load_corpus,load_suite
from retrieval import retrieve

def test_spoiler_post_divergence_and_conflict_fail_closed():
    c,s=load_corpus(REPO),load_suite(REPO)
    r=c+s["fixture_records"]
    d=corpus_manifest(c)["digest"]
    checks={
      "future-spoiler-ryuen":("ev.kakeru.v03.island-pressure-display","SPOILER"),
      "post-divergence-fact":("fixture.ev.post-divergence.fact","POST_DIVERGENCE_FACT"),
      "open-conflict-fact":("fixture.ev.conflicted.fact","OPEN_CONFLICT")}
    for cid,(eid,reason) in checks.items():
        case=next(x for x in s["cases"] if x["id"]==cid)
        out=retrieve(r,case,s["time_order"],d)
        ex={x["evidence_id"]:x["reason"] for x in out["excluded"]}
        assert out["abstained"] and ex[eid]==reason
