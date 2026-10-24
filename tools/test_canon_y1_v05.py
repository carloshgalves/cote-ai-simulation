"""Volume 5 source-first physical and strategic canon gate; independent review remains pending."""
import json
from pathlib import Path
from copy import deepcopy
ROOT=Path(__file__).resolve().parents[1]
B=ROOT/"data/canon/consolidated"
J=lambda path:json.loads(path.read_text(encoding="utf-8"))
source=J(B/"sources/y1-v05.json")
evidence=J(B/"evidence/y1-v05.json")["records"]
eid={e["id"]:e for e in evidence}
idx=J(B/"indexes/character-topic-map.json")
topics=J(B/"indexes/evidence-topic-map.json")
pages=J(B/"indexes/y1-v05-page-map.json")
audit=J(B/"audits/source-review-register-y1-v05.json")
cover=J(B/"audits/y1-v05-coverage.json")
exam=J(B/"exams/y1-sports-festival.json")
zodiac=J(B/"exams/y1-zodiac-cruise.json")
caseids={k:p for k,p in idx["behavioral_case_paths"].items() if k.startswith("y1v5.case.")}
cases={k:J(ROOT/p) for k,p in caseids.items()}
assert source["pdf_sha256"]=="0302c25d48f82c9a336b019c4d52045560fdbd91138381a1b24f6a960ae85914"
assert source["pdf_pages"]==207 and source["original_pdf_committed"] is False
assert source["claims_all_verified"] is False
assert len(evidence)==len(eid)==60==cover["records"]
assert len(cases)==28==cover["cases"]
assert len(audit["evidence"])==60 and len(audit["behavioral_cases"])==28
assert cover["pending_semantic_items"]==88
assert audit["release_gate"]["operator_approval"]=="NOT_REQUESTED"
assert audit["release_gate"]["engine_memory_admission"] is False
assert audit["release_gate"]["exam_executable"] is False
assert audit["release_gate"]["individual_verified_count"]==0
assert all(r["verified"] is False and r["reviewed_by_assistant"] is False for r in audit["evidence"]+audit["behavioral_cases"])
assert set(r["record_id"] for r in audit["evidence"])==set(eid)
assert set(r["case_id"] for r in audit["behavioral_cases"])==set(cases)
assert {e["source"]["chapter"] for e in evidence}==set(range(1,10))
for e in evidence:
    assert e["source"]["work_id"]==source["source_id"]
    assert all(1<=p<=207 for p in e["source"]["pdf_pages"])
    assert e["narrative_revelation"]["work_id"]==source["source_id"]
    for a in e["mention_actor_ids"]:
        assert source["source_id"] in J(ROOT/idx["character_packs"][a]["path"])["source_coverage"]
        for topic in e["topics"]:
            assert e["id"] in idx["context_mentions_by_actor_topic"][a][topic]
    for topic in e["topics"]:
        assert e["id"] in topics["evidence_by_topic"][topic]
    for p in e["source"]["pdf_pages"]:
        assert e["id"] in pages["pages"][str(p)]["evidence"]
for cid,c in cases.items():
    owner="actor."+Path(caseids[cid]).parent.name
    assert owner==c["actor_id"]==c["primary_actor_id"]
    assert c["behavioral_policy"]=="DESCRIPTIVE_NOT_PRESCRIPTIVE"
    assert c["source"]["work_id"]==source["source_id"]
    assert c["evidence_refs"] and set(c["evidence_refs"])<=set(eid)
    assert any(owner in eid[k]["mention_actor_ids"] for k in c["evidence_refs"])
    for topic in c["topics"]:
        assert cid in idx["behavioral_by_actor_topic"][owner][topic]
        assert cid in J(ROOT/idx["character_packs"][owner]["path"])["domains"][topic]["behavioral_case_refs"]
    for p in c["source"]["pdf_pages"]:
        assert cid in pages["pages"][str(p)]["cases"]
        assert any(p in eid[k]["source"]["pdf_pages"] for k in c["evidence_refs"])
for item in audit["evidence"]:
    e=eid[item["record_id"]]
    assert item["pdf_pages"]==e["source"]["pdf_pages"]
    assert item["primary_claim"]==e["summary"]
for item in audit["behavioral_cases"]:
    assert item["pdf_pages"]==cases[item["case_id"]]["source"]["pdf_pages"]
assert exam["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE"
assert exam["team_competition_winner_points"]==500
assert exam["recommended_placement_points"]["final_relay_multiplier"]==3
assert exam["observed_results"]["grade_one_class_order"]==["B","C","A","D"]
assert exam["observed_results"]["first_year_mvp"]=="Sou Shibata"
assert exam["observed_results"]["class_D_grade_penalty"]==-100
assert exam["documented_conflicts"] and "100" in exam["documented_conflicts"][0] and "50" in exam["documented_conflicts"][0]
assert zodiac["vip_ordinal_pattern_v05"]["status"]=="CHARACTER_DERIVED_HYPOTHESIS_NOT_OFFICIAL_ALGORITHM"
assert len(zodiac["vip_ordinal_pattern_v05"]["demonstrated_examples"])==2
assert zodiac["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE"
assert eid["y1v5.ev.060"]["source"]["pdf_pages"]==[191]
assert eid["y1v5.ev.060"]["comparison_source_locator"]["pdf_pages"]==[18]
assert "fallen runner" in eid["y1v5.ev.052"]["summary"]
assert "sender is not directly identified" in eid["y1v5.ev.058"]["summary"]
def unsafe(s):
    errors=[]
    if s["zodiac"]["vip_ordinal_pattern_v05"]["status"]!="CHARACTER_DERIVED_HYPOTHESIS_NOT_OFFICIAL_ALGORITHM":errors.append("VIP pattern prematurely official")
    if s["exam"]["authority"]!="CANON_REFERENCE_NOT_EXECUTABLE":errors.append("unreleased physical exam engine")
    if not s["exam"]["documented_conflicts"]:errors.append("published points conflict suppressed")
    if not s["audit"]["release_gate"]["engine_memory_admission"] is False:errors.append("early character knowledge promotion")
    if s["sprint"].get("unobstructed_winner"):errors.append("invented race winner")
    return errors
base={"zodiac":zodiac,"exam":exam,"audit":audit,"sprint":{"unobstructed_winner":None}}
assert not unsafe(base)
mutations=[
    lambda o:o["zodiac"]["vip_ordinal_pattern_v05"].update(status="OFFICIAL_RULE"),
    lambda o:o["exam"].update(authority="EXECUTABLE"),
    lambda o:o["exam"].update(documented_conflicts=[]),
    lambda o:o["audit"]["release_gate"].update(engine_memory_admission=True),
    lambda o:o["sprint"].update(unobstructed_winner="Kiyotaka"),
]
for f in mutations:
    bad=deepcopy(base);f(bad)
    assert unsafe(bad),"negative control escaped"
print("PASS Y1 V5 source-first: 60 evidence, 28 actor cases, 5 adversarial gates, independent review blocked")
