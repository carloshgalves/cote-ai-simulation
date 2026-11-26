"""Explicit external-world adapter reusing native RAG V2 retrieval and gates.

Secondary behavioral abstractions are NOT verified CanonEvidence or factual
claims. They enter an isolated, pinned collection. Native LN factual
compilation and legacy approvals remain unchanged.
"""
import argparse
import hashlib
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests/canon_rag"))
from retrieval import retrieve
from common import sha256_json

BASE = ROOT / "data/external-worlds/tomodachi-game"
WORLD = "tomodachi-game"
MANIFEST_BLOB_SHA1 = "b3c8745909a27f7e3580acd8ba71f91f80c019d1"
WORK = "tg.secondary.abstract-guidance.v1"
ANCHOR = "TG_GUIDANCE_AVAILABLE"
GAMES = frozenset(("kokkuri-san", "backbiting-sugoroku", "hide-and-seek",
    "friend-sin-trial", "friends-prison", "all-bet", "friend-murder",
    "friendless", "friendship-janken", "fake-game"))

def _read(path):
    return json.loads(path.read_text(encoding="utf-8"))

def compile_guidance():
    raw = (BASE / "rag/abstract-guidance-v1.json").read_bytes()
    if hashlib.sha1(b"blob " + str(len(raw)).encode() + b"\0" + raw).hexdigest() != MANIFEST_BLOB_SHA1:
        raise ValueError("guidance manifest changed; explicit release required")
    manifest = json.loads(raw)
    if manifest["world"] != WORLD or manifest["schema_version"] != 1:
        raise ValueError("invalid world guidance")
    idx = _read(BASE / "consolidated/indexes/character-topic-map.json")
    result, seen = [], set()
    for item in manifest["records"]:
        if set(item) != {"id", "actor", "game", "lesson", "case_ref"}:
            raise ValueError("unexpected guidance fields")
        if item["id"] in seen or item["game"] not in GAMES | {"*"}:
            raise ValueError("duplicate or invalid guidance")
        seen.add(item["id"])
        case_path = idx["behavioral_case_paths"][item["case_ref"]]
        expected_dir = BASE / "consolidated/behavioral_cases" / item["actor"].removeprefix("actor.")
        path = (ROOT / case_path).resolve()
        if path.parent != expected_dir.resolve():
            raise ValueError("foreign case path")
        case = _read(path)
        if case["id"] != item["case_ref"] or case["primary_actor_id"] != item["actor"] or case["runtime_memory"] is not False:
            raise ValueError("foreign or inadmissible case")
        if not isinstance(item["lesson"], str) or not 0 < len(item["lesson"]) <= 1000:
            raise ValueError("invalid abstract lesson")
        result.append({
            "record_id":"record:"+item["id"], "evidence_id":item["id"],
            "source_id":"src:"+sha256_json(item)[:24], "work":WORK,
            "locator":{"case_ref":case["id"]}, "narrative_ordinal":0,
            "actors":[item["actor"]], "supported_claim_ids":[], "resolved_claims":[],
            "effective_time":{"mode":"EXACT","exact":{"anchor":ANCHOR}},
            "retrieval_role":"BEHAVIORAL_CANON","epistemic_status":"UNVERIFIED",
            "continuity":["manga"],"external_world":WORLD,"open_conflicts":[],
            "topics":sorted(set(case["topics"])),"situation_tags":[item["game"]],
            "text":item["lesson"],"provenance":{"epistemic_status":"UNVERIFIED",
                "continuity":["manga"],"discovered_via":[{"tier":4,"ref":case["source"]["url"]}]},
            "source_digest":sha256_json({"projection":item,"case":case}),
            "compilation_errors":[],"game":item["game"],
        })
    return result

def lookup(actor, game, query="", *, top_k=5):
    if game not in GAMES or not isinstance(actor,str) or not actor.startswith("actor."):
        raise ValueError("invalid actor or game")
    records=compile_guidance()
    selected=[r for r in records if r["game"] in (game,"*")]
    terms=query+" "+" ".join(sorted({t for r in selected if actor in r["actors"] for t in r["topics"]}))
    req={"character_id":actor,"simulation_time":ANCHOR,"divergence_time":ANCHOR,
        "query":{"text":terms or game,"purpose":"BEHAVIORAL_GUIDANCE"},
        "top_k":top_k,"allow_unverified_behavioral":True,"knowledge_horizon":{WORK:0},
        "knowledge_scope":{"mode":"SIMULATION","allowed_claim_ids":[]}}
    digest=sha256_json(records)
    result=retrieve(selected,req,{ANCHOR:0},digest,continuity="manga",world=WORLD)
    return {"guidance":[x["text"] for x in result["items"]],
        "evidence_ids":[x["evidence_id"] for x in result["items"]],"manifest_digest":digest}

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--actor",required=True)
    p.add_argument("--game",required=True,choices=sorted(GAMES))
    p.add_argument("--query",default="")
    args=p.parse_args()
    print(json.dumps(lookup(args.actor,args.game,args.query),ensure_ascii=False))
if __name__=="__main__":
    main()
