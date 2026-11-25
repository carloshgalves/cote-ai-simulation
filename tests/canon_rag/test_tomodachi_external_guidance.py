import sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/"tools"))
import tomodachi_rag_v2 as tg
from retrieval import retrieve

def request(actor):
    return {"character_id":actor,"simulation_time":tg.ANCHOR,"divergence_time":tg.ANCHOR,
      "query":{"text":"strategy observation incentives cooperation","purpose":"BEHAVIORAL_GUIDANCE"},
      "knowledge_scope":{"mode":"SIMULATION","allowed_claim_ids":[]},"knowledge_horizon":{tg.WORK:0},
      "allow_unverified_behavioral":True,"top_k":100}

def test_native_default_rejects_external_collection():
    out=retrieve(tg.compile_guidance(),request("actor.yuuichi-katagiri"),{tg.ANCHOR:0},"test")
    assert out["abstained"] and all(x["reason"]=="FILTERED_OUT" for x in out["excluded"])

def test_native_world_and_actor_gate():
    r=tg.compile_guidance()
    q=request("actor.yuuichi-katagiri")
    out=retrieve(r,q,{tg.ANCHOR:0},"test",continuity="manga",world=tg.WORLD)
    assert out["items"] and all(x["evidence_id"].startswith("ev.tg.guidance.yuuichi-katagiri.") for x in out["items"])
    assert any(x["reason"]=="WRONG_ACTOR" for x in out["excluded"])
    q["allow_unverified_behavioral"]=False
    assert retrieve(r,q,{tg.ANCHOR:0},"test",continuity="manga",world=tg.WORLD)["abstained"]

def test_no_factual_lane_and_no_future_memory():
    q=request("actor.yuuichi-katagiri")
    q["query"]["purpose"]="KNOWN_FACTS"
    assert retrieve(tg.compile_guidance(),q,{tg.ANCHOR:0},"test",continuity="manga",world=tg.WORLD)["abstained"]
    out=tg.lookup("actor.yuuichi-katagiri","kokkuri-san","incentives observation")
    assert out["guidance"]
    assert all("Tenji" not in x and "Maria" not in x for x in out["guidance"])
    assert not tg.lookup("actor.unknown","kokkuri-san")["guidance"]
