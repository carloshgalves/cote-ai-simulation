from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
sys.path.insert(0,str(HERE))
from common import compile_records,corpus_manifest,load_claims,load_corpus,load_source_registry,load_suite,merge_source_registries
from retrieval import retrieve

def test_spoiler_post_divergence_and_conflict_fail_closed():
    c,s=load_corpus(REPO),load_suite(REPO)
    claims,registry=load_claims(REPO),load_source_registry(REPO)
    r=compile_records(c+s["fixture_records"],claims+s["fixture_claims"],merge_source_registries(registry,s["fixture_source_registry"]))
    d=corpus_manifest(c,claims,registry)["digest"]
    checks={
      "future-spoiler-ryuen":("ev.kakeru.v03.island-pressure-display","SPOILER"),
      "post-divergence-fact":("ev.fixture.post-divergence.fact","POST_DIVERGENCE_FACT"),
      "open-conflict-fact":("ev.fixture.conflicted.fact","OPEN_CONFLICT")}
    for cid,(eid,reason) in checks.items():
        case=next(x for x in s["cases"] if x["id"]==cid)
        out=retrieve(r,case,s["time_order"],d,s["seed_time"])
        ex={x["evidence_id"]:x["reason"] for x in out["excluded"]}
        assert out["abstained"] and ex[eid]==reason
