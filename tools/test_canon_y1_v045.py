"""Y1 Volume 4.5: selective-source provenance, actor ownership and release gates.

Structural checks are not individual semantic or independent human approval.
"""
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
B=ROOT/"data/canon/consolidated"
J=lambda p:json.loads(Path(p).read_text(encoding="utf-8"))
src=J(B/"sources/y1-v045.json")
E=J(B/"evidence/y1-v045.json")["records"]
idx=J(B/"indexes/character-topic-map.json")
topic=J(B/"indexes/evidence-topic-map.json")
page=J(B/"indexes/y1-v045-page-map.json")
institutions=J(B/"indexes/institution-map.json")
cover=J(B/"audits/y1-v045-coverage.json")
audit=J(B/"audits/source-review-register-y1-v045.json")
ret=J(B/"indexes/retrospective-event-map.json")
cases={id:J(ROOT/path) for id,path in idx["behavioral_case_paths"].items() if id.startswith("y1v45.case.")}
eid={e["id"]:e for e in E}
assert src["source_id"]=="ln.y1.v045.sevenseas.en.operator-provided"
assert src["pdf_sha256"]=="b01441252214bc44e9cee75f3a27640e25ff266609ac7386ac83d4d588f72d22"
assert src["pdf_pages"]==227
assert src["original_pdf_committed"] is False
assert src["claims_all_verified"] is False
assert len(E)==len(eid)==57==cover["records"]
assert len(cases)==21==cover["cases"]
assert cover["chapter_evidence_counts"]=={"1":1,"2":13,"3":17,"4":5,"5":5,"6":11,"7":5}
assert set(x["source"]["chapter"] for x in E)==set(range(1,8))
assert len(audit["evidence"])==57 and len(audit["behavioral_cases"])==21
assert {x["record_id"] for x in audit["evidence"]}==set(eid)
assert {x["case_id"] for x in audit["behavioral_cases"]}==set(cases)
assert audit["release_gate"]["operator_approval"]=="NOT_REQUESTED"
assert audit["release_gate"]["engine_memory_admission"] is False
assert audit["release_gate"]["exam_executable"] is False
assert audit["global_promotion"].startswith("BLOCKED_")
assert all(x["verified"] is False for x in audit["evidence"]+audit["behavioral_cases"])
assert all(x["reviewed_by_assistant"] is True for x in audit["evidence"]+audit["behavioral_cases"])
assert all(x["semantic_review"]=="ASSISTANT_SECOND_PASS_PDF_MATCHED_INDEPENDENT_REVIEW_PENDING" for x in audit["evidence"]+audit["behavioral_cases"])
assert audit["release_gate"]["assistant_individual_semantic_review"]=="COMPLETED_PDF_SECOND_PASS"
assert audit["release_gate"]["assistant_second_pass_count"]==78
assert audit["release_gate"]["independent_review_pending_count"]==78
assert audit["release_gate"]["individual_verified_count"]==0
assert audit["release_gate"]["external_independent_review"]=="NOT_PERFORMED"
assert src["record_audit"]=="ASSISTANT_SECOND_PASS_78_ITEMS_COMPLETE_INDEPENDENT_ADVERSARIAL_REVIEW_PENDING"
assert cover["pending_semantic_items"]==0 and cover["pending_independent_review_items"]==78
assert len(audit["findings"])==5 and all(x["status"].startswith("REMEDIATED") for x in audit["findings"])
assert src["source_id"]==audit["source_id"]==cover["source"]
assert len(idx["character_packs"])>=80
assert len(idx["behavioral_case_paths"])>=202
assert institutions["institutions"]["org.anhs"]["source_volume_count"]==6
assert institutions["institutions"]["org.student-council"]["source_volume_count"]==3
for e in E:
    assert e["source"]["work_id"]==src["source_id"]
    assert e["source"]["pdf_pages"] and all(1<=n<=227 for n in e["source"]["pdf_pages"])
    assert e["assertion_status"]=="ASSISTANT_SECOND_PDF_PASS_COMPLETE_INDEPENDENT_AUDIT_PENDING"
    assert e["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR"
    assert e["actor_knowledge_gate"]=="LOCAL_PARTICIPANT_ONLY_NOT_AUTOMATIC_GLOBAL_MEMORY"
    assert e["related_canonical_event_ids"]==ret["events_by_source_evidence"].get(e["id"],[])
    for actor in e["mention_actor_ids"]:
        assert actor in idx["character_packs"],actor
        assert src["source_id"] in J(ROOT/idx["character_packs"][actor]["path"])["source_coverage"],actor
        for t in e["topics"]:
            assert e["id"] in idx["context_mentions_by_actor_topic"][actor][t],(e["id"],actor,t)
    for t in e["topics"]:
        assert e["id"] in topic["evidence_by_topic"][t]
    for n in e["source"]["pdf_pages"]:
        assert e["id"] in page["pages"][str(n)]["evidence"],e["id"]
for id,c in cases.items():
    owner="actor."+Path(idx["behavioral_case_paths"][id]).parent.name
    assert owner==c["actor_id"]==c["primary_actor_id"]
    assert c["id"]==id and c["source"]["work_id"]==src["source_id"]
    assert c["epistemic_status"]=="ASSISTANT_SECOND_PDF_PASS_COMPLETE_INDEPENDENT_AUDIT_PENDING"
    assert c["source"]["source_status"]=="ASSISTANT_SECOND_PDF_PASS_COMPLETE_INDEPENDENT_AUDIT_PENDING"
    assert c["behavioral_policy"]=="DESCRIPTIVE_NOT_PRESCRIPTIVE"
    assert c["evidence_refs"] and set(c["evidence_refs"])<=set(eid)
    support=[eid[r] for r in c["evidence_refs"]]
    assert any(owner in x["mention_actor_ids"] for x in support)
    for t in c["topics"]:
        assert id in idx["behavioral_by_actor_topic"][owner][t]
        assert id in J(ROOT/idx["character_packs"][owner]["path"])["domains"][t]["behavioral_case_refs"]
    for n in c["source"]["pdf_pages"]:
        assert id in page["pages"][str(n)]["cases"]
        assert any(n in x["source"]["pdf_pages"] for x in support),(id,n)
for x in audit["evidence"]:
    e=eid[x["record_id"]]
    assert x["pdf_pages"]==e["source"]["pdf_pages"]
    assert x["primary_claim"]==e["summary"]
    assert x["actor_ids"]==e["mention_actor_ids"]
for x in audit["behavioral_cases"]:
    c=cases[x["case_id"]]
    assert x["pdf_pages"]==c["source"]["pdf_pages"]
    assert x["primary_actor_id"]==c["primary_actor_id"]
assert src["source_id"] in J(B/"exams/y1-zodiac-cruise.json")["source_coverage"]
assert src["source_id"] in J(B/"exams/y1-deserted-island.json")["source_coverage"]
assert J(B/"exams/y1-zodiac-cruise.json")["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE"
assert "not" in J(B/"exams/y1-zodiac-cruise.json")["later_revelations"][-1]["comment"].lower() or "no" in J(B/"exams/y1-zodiac-cruise.json")["later_revelations"][-1]["comment"].lower()
assert "actor.nagumo" in idx["character_packs"]
assert J(B/"characters/katsuragi.json")["display_name"]=="Kouhei Katsuragi"
assert J(B/"institutions/anhs.json")["outside_contact_restrictions_y1_v045"]["engine_authority"]=="REFERENCE_ONLY_NOT_COMPILED"
assert "remotely moved" in eid["y1v45.ev.014"]["summary"]
assert "recruits Sudō" in eid["y1v45.ev.027"]["summary"]
assert "never names it" in eid["y1v45.ev.051"]["summary"]
assert eid["y1v45.ev.051"]["related_canonical_event_ids"]==["evt.y1.v045.pool-invasion-and-suppression"]
assert "actor.manabu-horikita" not in cases["y1v45.case.nagumo.lead-older-students-volleyball"]["other_actors"]
assert "actor.manabu-horikita" not in cases["y1v45.case.honami-ichinose.initiate-friendly-volleyball-challenge"]["other_actors"]
assert "actor.nagumo" not in cases["y1v45.case.honami-ichinose.initiate-friendly-volleyball-challenge"]["other_actors"]
assert "remotely operate" in cases["y1v45.case.katsuragi.help-stranded-elevator-rivals"]["observed_result"]
print("PASS Y1 V4.5 assistant PDF second pass: 57 evidence, 21 cases, five remediations, independent gates blocked")
