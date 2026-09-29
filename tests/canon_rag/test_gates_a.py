from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
sys.path.insert(0,str(HERE))
from common import corpus_manifest,load_corpus,load_suite
from retrieval import retrieve

def setup():
    c,s=load_corpus(REPO),load_suite(REPO)
    return c,s,c+s["fixture_records"],corpus_manifest(c)["digest"]

def case(s,i):
    return next(x for x in s["cases"] if x["id"]==i)

def test_gate_before_rank_and_wrong_actor():
    _,s,r,d=setup()
    out=retrieve(r,case(s,"wrong-character-secret"),s["time_order"],d)
    assert [x["evidence_id"] for x in out["items"]]==["ev.kiyotaka.v01.reluctant-social-help"]
    ex={x["evidence_id"]:x["reason"] for x in out["excluded"]}
    assert ex["fixture.ev.secret.other-actor"]=="WRONG_ACTOR"

def test_same_query_same_time_differs_by_actor():
    _,s,r,d=setup()
    a=retrieve(r,case(s,"contrast-kiyotaka"),s["time_order"],d)
    b=retrieve(r,case(s,"contrast-kikyo"),s["time_order"],d)
    assert [x["evidence_id"] for x in a["items"]]==["ev.kiyotaka.v01.reluctant-social-help"]
    assert [x["evidence_id"] for x in b["items"]]==["ev.kikyo.v01.social-bridging"]

def test_unverified_fact_abstains():
    c,s,_,d=setup()
    out=retrieve(c,case(s,"unverified-fact-abstains"),s["time_order"],d)
    ex={x["evidence_id"]:x["reason"] for x in out["excluded"]}
    assert out["abstained"] and ex["ev.institution.v01.initial-deposit"]=="UNVERIFIED_FACT"
