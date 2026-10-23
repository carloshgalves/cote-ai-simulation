"""Crosswalk QA: six existing YAMLs stay non-executable; future canon never seeded."""
import hashlib,json,re
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
B=ROOT/"data/canon/consolidated"
m=json.loads((B/"migrations/y1-legacy-six.json").read_text(encoding="utf-8"))
idx=json.loads((B/"indexes/character-topic-map.json").read_text(encoding="utf-8"))
assert len(m["actor_map"])==6
assert all(x["legacy_yaml_is_second_runtime_memory"] is False for x in m["actor_map"].values())
total=0
for actor,x in sorted(m["actor_map"].items()):
    raw=(ROOT/x["legacy_file"]).read_bytes()
    sha=hashlib.sha1(f"blob {len(raw)}\0".encode()+raw).hexdigest()
    assert sha==x["git_blob_sha1"],(actor,"source changed without review")
    ids=re.findall(r"(?m)^\s+- id:\s+(char\.[a-z0-9.-]+)\s*$",raw.decode("utf-8"))
    assert len(ids)==x["claim_count"]==len(x["claims"]),(actor,len(ids),len(x["claims"]))
    assert len(set(ids))==len(ids)
    assert set(ids)=={c["legacy_claim_id"] for c in x["claims"]}
    assert actor in idx["character_packs"]
    pack=json.loads((ROOT/x["consolidated_file"]).read_text(encoding="utf-8"))
    assert pack["character_id"]==actor
    assert pack["legacy_y1_crosswalk"]["admitted_initial_claim_ids"]==[]
    for c in x["claims"]:
        assert c["admitted_as_actor_memory"] is False
        assert c["requires_new_record_review"] is True
        assert c["disposition"] in ("FUTURE_BEHAVIORAL_PRIOR_ONLY","PRIVATE_HORIZON_CANDIDATE_REQUIRES_ACTOR_GATE","UNCLASSIFIED_NOT_ADMITTED")
        if c["disposition"]=="FUTURE_BEHAVIORAL_PRIOR_ONLY":
            assert c["admitted_as_actor_memory"] is False
    total+=len(ids)
assert total==119,total
ryuen=json.loads((B/"characters/kakeru-ryuen.json").read_text(encoding="utf-8"))
assert ryuen["domains"]=={}, "later-volume strategic feats cannot prepopulate Y1_START"
assert ryuen["source_coverage"]==[]
print("PASS legacy six actor crosswalk:",total,"claims, no future-memory admission")
