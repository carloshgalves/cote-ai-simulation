"""Year 1 Volume 3 initial source pass: explicit pending gates, actor attribution, exam reference."""
import json
from pathlib import Path
B=Path(__file__).resolve().parents[1]/"data/canon/consolidated"
J=lambda p:json.loads(p.read_text(encoding="utf-8"))
source=J(B/"sources/y1-v03.json")
data=J(B/"evidence/y1-v03.json")
E=data["records"]
case_idx=J(B/"indexes/character-topic-map.json")
topics=J(B/"indexes/evidence-topic-map.json")
inst=J(B/"indexes/institution-map.json")
page=J(B/"indexes/y1-v03-page-map.json")
coverage=J(B/"audits/y1-v03-coverage.json")
review=J(B/"audits/source-review-register-y1-v03.json")
exam=J(B/"exams/y1-deserted-island.json")
ids={e["id"] for e in E}
assert len(E)==len(ids)==117==coverage["records"]
assert source["pdf_pages"]==262
assert source["pdf_sha256"]=="22b13fabb5aa2b93e355a0cd621c7e5d9dd63c33d775eda58e27067541c5387e"
assert source["operator_accepted"] and not source["original_pdf_committed"] and not source["claims_all_verified"]
assert data["source_id"]==source["source_id"]==review["source_id"]
assert set(e["source"]["chapter"] for e in E)==set(range(1,8))
assert all(e["source"]["work_id"]==source["source_id"] for e in E)
assert all(e["assertion_status"]=="OPERATOR_ACCEPTED_RECORD_AUDIT_PENDING" for e in E)
assert all(e["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR" for e in E)
assert all(e["source_mode"] in {"OBSERVED","NARRATED","SELF_DESCRIPTION","REPORTED_BY_CHARACTER","INNER_MONOLOGUE","INTERPRETATION","DOCUMENT_IMAGE"} for e in E)
case_paths={id:path for id,path in case_idx["behavioral_case_paths"].items() if id.startswith("y1v3.case.")}
assert len(case_paths)==35
cases=[J(B.parent.parent.parent/path) for path in case_paths.values()]
assert len({c["id"] for c in cases})==35
for e in E:
    assert e["source"]["pdf_pages"] and all(1<=int(n)<=262 for n in e["source"]["pdf_pages"])
    for aid in e["mention_actor_ids"]:
        assert aid in case_idx["character_packs"],aid
        for topic in e["topics"]:
            assert e["id"] in case_idx["context_mentions_by_actor_topic"][aid][topic]
    for topic in e["topics"]:
        assert e["id"] in topics["evidence_by_topic"][topic]
    for n in e["source"]["pdf_pages"]:
        assert e["id"] in page["pages"][str(n)]["evidence"]
for c in cases:
    owner=c["primary_actor_id"]
    assert owner==c["actor_id"]=="actor."+Path(case_paths[c["id"]]).parent.name
    assert c["epistemic_status"]=="OPERATOR_ACCEPTED_RECORD_AUDIT_PENDING"
    assert c["evidence_refs"] and set(c["evidence_refs"])<=ids
    for topic in c["topics"]:
        assert c["id"] in case_idx["behavioral_by_actor_topic"][owner][topic]
    for n in c["source"]["pdf_pages"]:
        assert c["id"] in page["pages"][str(n)]["cases"]
assert review["evidence_count"]==117
assert review["behavioral_case_count"]==35
assert {x["record_id"] for x in review["evidence"]}==ids
assert {x["case_id"] for x in review["behavioral_cases"]}==set(case_paths)
assert all(not x["verified"] and not x["reviewed_by_assistant"] for x in review["evidence"]+review["behavioral_cases"])
assert review["global_promotion"].startswith("BLOCKED")
assert review["release_gate"]["operator_approval"]=="PENDING"
assert exam["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE"
assert exam["initial_class_test_points"]==300
assert exam["scoring"]["observed_final_scores"]=={"A":120,"B":140,"C":0,"D":225}
assert exam["leader"]["replacement"]=="ONLY_WITH_SUITABLE_JUSTIFICATION"
assert exam["leader"]["actor_knowledge_gate"]=="AFTER_EXAM_RESULT"
assert exam["spots"]["exclusive_duration_hours"]==8
assert exam["spots"]["correct_guess_gain"]==50
assert any(p["kind"]=="POLLUTE_ENVIRONMENT" and p["class_exam_points"]==20 for p in exam["penalties"])
assert any(p["kind"]=="ABSENT_ROLL_CALL" and p["class_exam_points"]==5 for p in exam["penalties"])
assert exam["scoring"]["observed_A_initial_adjustment"]["points"]==270
assert inst["institutions"]["org.anhs"]["source_volume_count"]==4
assert "exam.y1.deserted-island-1" in J(B/"institutions/anhs.json")["domains"]["exams"]["reference_exam_ids"]
assert any(e["source_mode"]=="DOCUMENT_IMAGE" and e["source"]["pdf_pages"]==[12] for e in E)
assert any("Kaneda" in e["summary"] and e["source_mode"]=="REPORTED_BY_CHARACTER" for e in E)
assert any("intuition" in e["summary"] and e["source_mode"]=="INTERPRETATION" for e in E)
assert any(e["source"]["chapter"]==5 and e["story_time"].endswith("FLASHBACK") for e in E)
assert all(c["source"]["work_id"]==source["source_id"] for c in cases)
print("PASS Y1 V3 initial extraction: 117 source records, 35 actor cases, exam reference, pending release gates")
