from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
sys.path.insert(0,str(HERE))
from common import compile_records,corpus_manifest,load_claims,load_corpus,load_source_registry,load_suite,merge_source_registries
from retrieval import retrieve

def setup():
    c,s=load_corpus(REPO),load_suite(REPO)
    claims=load_claims(REPO)
    registry=load_source_registry(REPO)
    records=compile_records(c+s["fixture_records"],claims+s["fixture_claims"],merge_source_registries(registry,s["fixture_source_registry"]))
    return c,s,records,corpus_manifest(c,claims,registry)["digest"]

def case(s,i):
    return next(x for x in s["cases"] if x["id"]==i)

def test_gate_before_rank_and_wrong_actor():
    _,s,r,d=setup()
    out=retrieve(r,case(s,"wrong-character-secret"),s["time_order"],d,s["seed_time"])
    assert [x["evidence_id"] for x in out["items"]]==["ev.kiyotaka.v01.reluctant-social-help"]
    ex={x["evidence_id"]:x["reason"] for x in out["excluded"]}
    assert ex["ev.fixture.secret.other-actor"]=="WRONG_ACTOR"

def test_same_query_same_time_differs_by_actor():
    _,s,r,d=setup()
    a=retrieve(r,case(s,"contrast-kiyotaka"),s["time_order"],d,s["seed_time"])
    b=retrieve(r,case(s,"contrast-kikyo"),s["time_order"],d,s["seed_time"])
    assert [x["evidence_id"] for x in a["items"]]==["ev.kiyotaka.v01.reluctant-social-help"]
    assert [x["evidence_id"] for x in b["items"]]==["ev.kikyo.v01.social-bridging"]

def test_unverified_fact_abstains():
    c,s,_,d=setup()
    claims=load_claims(REPO)
    registry=load_source_registry(REPO)
    out=retrieve(compile_records(c,claims,registry),case(s,"unverified-fact-abstains"),s["time_order"],d,s["seed_time"])
    ex={x["evidence_id"]:x["reason"] for x in out["excluded"]}
    assert out["abstained"] and ex["ev.institution.v01.initial-deposit"]=="UNVERIFIED_FACT"
