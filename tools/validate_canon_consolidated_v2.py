#!/usr/bin/env python3
"""Validate Volume 0 consolidated packs, index and primary-agent role gates."""
import json
from pathlib import Path
B=Path(__file__).resolve().parents[1]/"data/canon/consolidated"
J=lambda x:json.loads(x.read_text(encoding="utf-8"))
idx=J(B/"indexes/character-topic-map.json")
actors={p.stem:J(p) for p in (B/"characters").glob("*.json")}
inst={p.stem:J(p) for p in (B/"institutions").glob("*.json")}
ev={x["id"]:x for x in J(B/"evidence/volume0.json")["records"]}
case_files=sorted((B/"behavioral_cases").glob("*/*.json"))
cases={x["id"]:x for x in (J(p) for p in case_files)}
assert len(case_files)==len(cases), "duplicate behavioral case IDs"
assert not list((B/"behavioral_cases").glob("*.json")), "flat behavioral cases are forbidden"
assert sorted((B/"behavioral_cases").rglob("*.json"))==case_files, "behavioral cases must live exactly one folder under their actor"
case_path_index=idx["behavioral_case_paths"]
assert set(case_path_index)==set(cases), "behavioral case path index incomplete or stale"
for p in case_files:
  c=J(p)
  expected_slug=c["primary_actor_id"].removeprefix("actor.")
  assert p.parent.name==expected_slug, (p,c["primary_actor_id"])
  canonical_path=p.relative_to(B.parents[2]).as_posix()
  assert case_path_index[c["id"]]==canonical_path, (c["id"], canonical_path)
  assert c["primary_actor_id"] in idx["character_packs"], c["id"]
for case_id, relative_path in case_path_index.items():
  resolved=B.parents[2]/relative_path
  assert resolved.is_file() and J(resolved)["id"]==case_id, case_id
assert (len(actors),len(inst),len(ev),len(cases))==(35,13,132,25)
assert len({x["character_id"] for x in actors.values()})==35
for p in actors.values():
  actor=p["character_id"]
  assert actor in idx["topics_by_actor"]
  for d in p["domains"].values():
    assert "evidence_refs" not in d
    for e in d.get("context_evidence_refs",[]):assert e in ev and actor in ev[e]["mention_actor_ids"]
    for c in d.get("behavioral_case_refs",[]):assert c in cases and cases[c]["primary_actor_id"]==actor
for actor,ts in idx["topics_by_actor"].items():
  for topic,refs in ts.items():
    b=idx["behavioral_by_actor_topic"][actor][topic]
    context=idx["context_mentions_by_actor_topic"][actor][topic]
    assert set(refs)==set(b)|set(context) and not set(b)&set(context)
    for c in b:assert c in cases and cases[c]["primary_actor_id"]==actor
    for e in context:assert e in ev and actor in ev[e]["mention_actor_ids"]
assert any("v0.case.shiro.freedom-choice" in ids for ids in idx["behavioral_by_actor_topic"]["actor.shiro"].values())
assert idx["behavioral_by_actor_topic"]["actor.jinnosuke-naoe"]["politics"]
assert idx["behavioral_by_actor_topic"]["actor.kiyotaka-ayanokoji"]["combat"]
assert all(e["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR" for e in ev.values())
assert J(B/"sources/volume0.json")["claims_all_verified"]==False
assert not list((B/"exams").glob("*.json"))

# Never regress to per-volume character/institution packs or stale pack paths.
CANON=B.parent
assert not (CANON/"volume0").exists(), "retired volume0 staging tree reintroduced"
pack_paths=idx["character_packs"]
assert set(pack_paths)=={x["character_id"] for x in actors.values()}
for actor, metadata in pack_paths.items():
    expected="data/canon/consolidated/characters/"+actor.removeprefix("actor.")+".json"
    assert metadata["path"]==expected, (actor, metadata["path"], expected)
    assert (B/"characters"/(actor.removeprefix("actor.")+".json")).is_file(), actor
assert len(actors)==35 and len(inst)==13
print("PASS Volume0 V2: 35 actors, 13 institutions, 25 cases, 132 evidence, explicit behavioral/context gates.")
