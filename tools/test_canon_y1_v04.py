"""Y1 Volume 4 provenance, behavioral attribution and Zodiac/VIP rule-reference QA.
Only verifies structure and source-gating; semantic PDF comparison needs individual audit.
"""
from pathlib import Path
import json

ROOT=Path(__file__).resolve().parents[1]
B=ROOT/"data/canon/consolidated"
J=lambda p:json.loads(Path(p).read_text(encoding="utf-8"))
source=J(B/"sources/y1-v04.json")
evidence=J(B/"evidence/y1-v04.json")
E=evidence["records"]
index=J(B/"indexes/character-topic-map.json")
eindex=J(B/"indexes/evidence-topic-map.json")
iindex=J(B/"indexes/institution-map.json")
pindex=J(B/"indexes/y1-v04-page-map.json")
coverage=J(B/"audits/y1-v04-coverage.json")
register=J(B/"audits/source-review-register-y1-v04.json")
exam=J(B/"exams/y1-zodiac-cruise.json")
cases={k:J(ROOT/path) for k,path in index["behavioral_case_paths"].items() if k.startswith("y1v4.case.")}
sids={e["id"] for e in E}
assert len(sids)==len(E)==130
assert len(cases)==43
assert source["pdf_pages"]==248 and source["pdf_sha256"]=="1d587b01242cd576fa2b0627222329d1bf1c23b12d206ad63f7a590f79fe9cc8"
assert source["original_pdf_committed"] is False and source["claims_all_verified"] is False
assert coverage["records"]==130 and coverage["cases"]==43
assert set(x["source"]["chapter"] for x in E)==set(range(1,6))
assert coverage["chapter_evidence_counts"]=={"1":2,"2":37,"3":27,"4":41,"5":23}
assert evidence["source_id"]==source["source_id"]==register["source_id"]==exam["source_coverage"][0]
assert len(register["evidence"])==130 and len(register["behavioral_cases"])==43
assert {v["record_id"] for v in register["evidence"]}==sids
assert {v["case_id"] for v in register["behavioral_cases"]}==set(cases)
assert register["release_gate"]["operator_approval"]=="PENDING"
assert register["release_gate"]["engine_memory_admission"] is False
assert register["release_gate"]["exam_executable"] is False
assert all(x["verified"] is False and x["reviewed_by_assistant"] is False for x in register["evidence"]+register["behavioral_cases"])
assert register["global_promotion"].startswith("BLOCKED")
for e in E:
    assert e["source"]["work_id"]==source["source_id"]
    assert e["source"]["pdf_pages"] and all(1<=p<=source["pdf_pages"] for p in e["source"]["pdf_pages"])
    assert e["assertion_status"]=="EXTRACTED_PENDING_INDIVIDUAL_SEMANTIC_REVIEW"
    assert e["subject_assertion"]=="ACTOR_MENTION_NOT_AUTOMATIC_BEHAVIOR"
    assert e["source_mode"] in {"OBSERVED","NARRATED","SELF_DESCRIPTION","REPORTED_BY_CHARACTER","INNER_MONOLOGUE","INTERPRETATION","DOCUMENT_IMAGE"}
    for aid in e["mention_actor_ids"]:
        assert aid in index["character_packs"],aid
        assert source["source_id"] in J(ROOT/index["character_packs"][aid]["path"])["source_coverage"],aid
        for t in e["topics"]:
            assert e["id"] in index["context_mentions_by_actor_topic"][aid][t],(aid,t,e["id"])
    for topic in e["topics"]:
        assert e["id"] in eindex["evidence_by_topic"][topic]
    for p in e["source"]["pdf_pages"]:
        assert e["id"] in pindex["pages"][str(p)]["evidence"]
for id,c in cases.items():
    owner="actor."+index["behavioral_case_paths"][id].split("/")[-2]
    assert c["id"]==id and c["actor_id"]==c["primary_actor_id"]==owner
    assert c["source"]["work_id"]==source["source_id"]
    assert c["evidence_refs"] and set(c["evidence_refs"])<=sids
    assert c["epistemic_status"]=="PENDING_INDIVIDUAL_SEMANTIC_REVIEW"
    assert c["behavioral_policy"]=="DESCRIPTIVE_NOT_PRESCRIPTIVE"
    for t in c["topics"]:
        assert id in index["behavioral_by_actor_topic"][owner][t]
        assert id in J(ROOT/index["character_packs"][owner]["path"])["domains"][t]["behavioral_case_refs"]
    for n in c["source"]["pdf_pages"]:
        assert id in pindex["pages"][str(n)]["cases"]
for r in register["evidence"]:
    e=next(e for e in E if e["id"]==r["record_id"])
    assert r["pdf_pages"]==e["source"]["pdf_pages"]
    assert r["actor_ids"]==e["mention_actor_ids"]
for r in register["behavioral_cases"]:
    c=cases[r["case_id"]]
    assert r["pdf_pages"]==c["source"]["pdf_pages"]
assert exam["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE"
assert exam["group_count"]==12 and len(exam["group_names"])==12
assert len(exam["outcomes"])==4
assert [o["number"] for o in exam["outcomes"]]==[1,2,3,4]
assert exam["outcomes"][0]["awards"]["vip_private_points"]==1000000
assert exam["outcomes"][0]["awards"]["each_non_vip_member_private_points"]==500000
assert exam["outcomes"][2]["awards"]["submitter_class_points"]==50
assert exam["outcomes"][2]["penalties"]["vip_class_points"]==-50
assert exam["outcomes"][3]["penalties"]["submitter_class_points"]==-50
assert exam["outcomes"][3]["awards"]["vip_class_points"]==50
assert exam["timing"]["normal_answer_window"]==["21:30","22:00"]
assert exam["timing"]["outcomes_announcement"]=="23:00 final day"
assert "fourth" in exam["timing"]["schedule_ambiguity"] and "three" in exam["timing"]["schedule_ambiguity"]
assert exam["observed_net_class_points"]=={"A":-200,"B":0,"C":150,"D":50}
assert exam["observed_net_private_points"]=={"A":2000000,"B":2500000,"C":5500000,"D":3000000}
assert exam["observed_final_outcomes"]["Rabbit"]==4
assert exam["observed_final_outcomes"]["Dragon"]==1
assert exam["observed_final_outcomes"]["Monkey"]==3
assert len(exam["group_rosters_reference"]["Rabbit"]["C"])==4
assert len(exam["group_rosters_reference"]["Dragon"]["D"])==3
assert exam["individual_known_secret_outcomes"][0]["actor"]=="actor.kei-karuizawa"
assert exam["individual_known_secret_outcomes"][1]["actor"]=="actor.kikyo-kushida"
assert "NOT_" in exam["individual_known_secret_outcomes"][0]["public_group_knowledge"]
assert "exam.y1.zodiac-cruise-1" in J(B/"institutions/anhs.json")["domains"]["exams"]["reference_exam_ids"]
assert iindex["institutions"]["org.anhs"]["source_volume_count"]==5
assert len(index["character_packs"])==79
assert len(index["behavioral_case_paths"])==180
assert J(B/"institutions/anhs.json")["zodiac_special_exam_y1_v04"]["normative_scope"]=="REFERENCE_ONLY"
assert any(x["source_mode"]=="DOCUMENT_IMAGE" and x["source"]["pdf_pages"]==[11] for x in E)
assert any("sexually harasses" in x["summary"] for x in E)
assert any("not an endorsed" in x["summary"] for x in E)
print("PASS Y1V4 first source pass: 130 evidence, 43 cases, 12-group VIP reference; all review/release gates locked")
