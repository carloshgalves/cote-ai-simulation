"""Source-specific adversarial review for Year 1 Volume 4 Zodiac canon.

Separate negative-control method performed by the extraction assistant.
Never claim this is an independent human or different-model review.
"""
from __future__ import annotations
import copy
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
BASE=ROOT/"data/canon/consolidated"
J=lambda p:json.loads(Path(p).read_text(encoding="utf-8"))
E=J(BASE/"evidence/y1-v04.json")["records"]
INDEX=J(BASE/"indexes/character-topic-map.json")
PAGE=J(BASE/"indexes/y1-v04-page-map.json")
REG=J(BASE/"audits/source-review-register-y1-v04.json")
EXAM=J(BASE/"exams/y1-zodiac-cruise.json")
SOURCE=J(BASE/"sources/y1-v04.json")
COVERAGE=J(BASE/"audits/y1-v04-coverage.json")
CASES={k:J(ROOT/p) for k,p in INDEX["behavioral_case_paths"].items() if k.startswith("y1v4.case.")}
STATE={"evidence":{r["id"]:r for r in E},"index":INDEX,"page":PAGE,"register":REG,"exam":EXAM,"source":SOURCE,"cases":CASES}

def findings(s):
    bad=[]
    e=s["evidence"]; ix=s["index"]; p=s["page"]; reg=s["register"]; exam=s["exam"]; cases=s["cases"]; src=s["source"]
    def fail(condition,msg):
        if not condition: bad.append(msg)
    fail(len(e)==130 and len(cases)==43,"count")
    fail(src["pdf_sha256"]=="1d587b01242cd576fa2b0627222329d1bf1c23b12d206ad63f7a590f79fe9cc8","source hash")
    fail(src["original_pdf_committed"] is False and src["claims_all_verified"] is False,"source certification")
    fail(len(reg["evidence"])==130 and len(reg["behavioral_cases"])==43,"register count")
    fail(reg["release_gate"]["operator_approval"]=="PENDING","human authorization")
    fail(reg["release_gate"]["external_independent_review"]=="NOT_PERFORMED" or reg["release_gate"]["external_independent_review"]=="ADVERSARIAL_SAME_ASSISTANT_COMPLETE_EXTERNAL_REVIEWER_PENDING","external reviewer separation")
    fail(reg["release_gate"]["engine_memory_admission"] is False and reg["release_gate"]["exam_executable"] is False,"runtime gates")
    fail(reg["global_promotion"].startswith("BLOCKED"),"release remains blocked")
    for entry in reg["evidence"]+reg["behavioral_cases"]:
        fail(entry.get("reviewed_by_assistant") is True and entry.get("verified") is False,"record review certification")
        fail(len(entry.get("review_note",""))>=65,"individual source judgement")
        fail(entry.get("semantic_adjudication") in {"ASSISTANT_DIRECT_SOURCE_REVIEW_PASS","ASSISTANT_DIRECT_CASE_REVIEW_PASS"},"review type")
    for entry in reg["evidence"]:
        v=e.get(entry["record_id"])
        fail(v is not None,"register orphan")
        if v:
            fail(entry["pdf_pages"]==v["source"]["pdf_pages"],"review pages "+entry["record_id"])
            fail(entry["primary_claim"]==v["summary"],"review paraphrase "+entry["record_id"])
            fail(entry["actor_ids"]==v["mention_actor_ids"],"review actors "+entry["record_id"])
            fail(entry["source_mode"]==v["source_mode"],"review mode "+entry["record_id"])
    for record in e.values():
        aid=record["id"]
        fail(record["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR","mention gate "+aid)
        for actor in record["mention_actor_ids"]:
            fail(actor in ix["character_packs"],"actor profile "+aid)
            for topic in record["topics"]:
                fail(aid in ix["context_mentions_by_actor_topic"].get(actor,{}).get(topic,[]),"actor context "+aid)
        for n in record["source"]["pdf_pages"]:
            fail(aid in p["pages"].get(str(n),{}).get("evidence",[]),"source page "+aid)
    for caseid,c in cases.items():
        owner="actor."+Path(ix["behavioral_case_paths"][caseid]).parent.name
        fail(c["primary_actor_id"]==c["actor_id"]==owner,"case owner "+caseid)
        fail(c["source"]["work_id"]==src["source_id"],"case source "+caseid)
        fail(c["behavioral_policy"] in ("DESCRIPTIVE_NOT_PRESCRIPTIVE","DESCRIPTIVE_ONLY_NOT_PRESCRIPTIVE"),"nonprescriptive episode "+caseid)
        refs=[e.get(a) for a in c["evidence_refs"]]
        fail(bool(refs) and all(x is not None for x in refs),"case source refs "+caseid)
        refs=[x for x in refs if x]
        fail(any(owner in x["mention_actor_ids"] for x in refs),"primary actor demonstrated "+caseid)
        for pageid in c["source"]["pdf_pages"]:
            fail(caseid in p["pages"].get(str(pageid),{}).get("cases",[]),"case page index "+caseid)
            fail(any(pageid in x["source"]["pdf_pages"] for x in refs),"case source page supported "+caseid+":"+str(pageid))
        for topic in c["topics"]:
            fail(caseid in ix["behavioral_by_actor_topic"].get(owner,{}).get(topic,[]),"owner behavior topic "+caseid)
    for entry in reg["behavioral_cases"]:
        c=cases.get(entry["case_id"])
        fail(c is not None,"case register orphan")
        if c:
            fail(entry["primary_actor_id"]==c["primary_actor_id"],"case audit owner")
            fail(entry["pdf_pages"]==c["source"]["pdf_pages"],"case audit pages")
    def ev(n):return e.get("y1v4.ev."+str(n).zfill(3),{})
    fail("actor.haruki-yamauchi" in ev(60).get("mention_actor_ids",[]) and "actor.kakeru-ryuen" not in ev(60).get("mention_actor_ids",[]),"Yamauchi PDF p103")
    fail("actor.yosuke-hirata" in ev(36).get("mention_actor_ids",[]) and "actor.kei-karuizawa" not in ev(36).get("mention_actor_ids",[]),"Hirata PDF p52")
    fail("earlier emergency-stairway" in ev(106).get("summary",""),"photo provenance")
    fail("another round of cards" in ev(108).get("summary",""),"Rabbit cards")
    fail(ev(127).get("source",{}).get("pdf_pages")==[240,241],"outcome pages")
    fail(ev(129).get("source",{}).get("pdf_pages")==[241],"VIP screen pages")
    fail(ev(61).get("source_mode")=="INNER_MONOLOGUE","alliance hypothesis")
    fail(ev(125).get("source_mode")=="REPORTED_BY_CHARACTER","Ryūen knowledge claim")
    fail(ev(122).get("source_mode")=="REPORTED_BY_CHARACTER","Morishige claim")
    fail("before she reveals" in cases["y1v4.case.honami-ichinose.stage-open-email-visibility"]["observed_actions"],"Ichinose never shows her message")
    fail("consent" in cases["y1v4.case.kei-karuizawa.accept-protection-under-duress"]["epistemic_limits"].lower(),"coercion not consent")
    fail("NOT_EXECUTABLE" in exam["authority"],"engine allowed")
    fail(len(exam["group_names"])==12 and exam["group_count"]==12,"Zodiac count")
    fail([x["number"] for x in exam["outcomes"]]==[1,2,3,4],"exam outcomes")
    fail(exam["outcomes"][0]["awards"]["vip_private_points"]==1000000,"correct group VIP award")
    fail(exam["outcomes"][1]["awards"]["vip_private_points"]==500000,"hidden VIP award")
    fail(exam["outcomes"][2]["penalties"]["vip_class_points"]==-50,"early correct target")
    fail(exam["outcomes"][3]["penalties"]["submitter_class_points"]==-50,"early false guess cost")
    fail(exam["observed_net_class_points"]=={"A":-200,"B":0,"C":150,"D":50},"published class results")
    fail(exam["observed_final_outcomes"]["Rabbit"]==4 and exam["observed_final_outcomes"]["Dragon"]==1,"key results")
    fail("fourth" in exam["timing"]["schedule_ambiguity"] and "third" in exam["timing"]["schedule_ambiguity"],"source's calendar ambiguity")
    fail(exam["individual_known_secret_outcomes"][0]["public_group_knowledge"]=="NOT_SHARED_UNTIL_AFTER_EXAM","VIP disclosure leak")
    fail(not any("deterministic_formula" in str(k) for k in exam.keys()),"invented VIP allocation formula")
    return sorted(set(bad))

errors=findings(STATE)
assert not errors,errors
mutations=[
 ("wrong physical attacker",lambda s:s["evidence"]["y1v4.ev.060"].update(mention_actor_ids=["actor.kakeru-ryuen","actor.kiyotaka-ayanokoji"])),
 ("misassign briefing",lambda s:s["evidence"]["y1v4.ev.036"].update(mention_actor_ids=["actor.kei-karuizawa"])),
 ("wrong abuse photograph",lambda s:s["evidence"]["y1v4.ev.106"].update(summary="Photographed the later secluded-room assault.")),
 ("remove cross-page result",lambda s:s["evidence"]["y1v4.ev.127"]["source"].update(pdf_pages=[240])),
 ("invent class A alliance",lambda s:s["evidence"]["y1v4.ev.061"].update(source_mode="OBSERVED")),
 ("change correct guess award",lambda s:s["exam"]["outcomes"][2]["penalties"].update(vip_class_points=0)),
 ("change VIP own class point",lambda s:s["exam"]["outcomes"][3]["penalties"].update(submitter_class_points=0)),
 ("forge school results",lambda s:s["exam"]["observed_net_class_points"].update({"C":-5})),
 ("backdate Rabbit VIP",lambda s:s["exam"]["individual_known_secret_outcomes"][0].update(public_group_knowledge="KNOWN_BEFORE_BRIEFING")),
 ("clear teacher calendar conflict",lambda s:s["exam"]["timing"].update(schedule_ambiguity="No difference, exactly four days.")),
 ("promote verified",lambda s:s["register"]["evidence"][0].update(verified=True)),
 ("invent external reviewer",lambda s:s["register"]["release_gate"].update(external_independent_review="APPROVED_BY_INDEPENDENT_REVIEWER")),
 ("falsify operator acceptance",lambda s:s["register"]["release_gate"].update(operator_approval="APPROVED")),
 ("move case ownership",lambda s:s["cases"]["y1v4.case.machida.stop-forced-photography"].update(primary_actor_id="actor.manabe-shiho")),
 ("attribute disclosed email to Ichinose",lambda s:s["cases"]["y1v4.case.honami-ichinose.stage-open-email-visibility"].update(observed_actions="She showed everyone her school email.")),
 ("erase assault nonconsent",lambda s:s["cases"]["y1v4.case.kei-karuizawa.accept-protection-under-duress"].update(epistemic_limits="It was a free agreement.")),
 ("break case page provenance",lambda s:s["cases"]["y1v4.case.kiyotaka-ayanokoji.orchestrate-double-phone-swap"]["source"].update(pdf_pages=[220,245])),
 ("promote executable model",lambda s:s["exam"].update(authority="SCHOOL_EXECUTABLE_ENGINE")),
]
for name,change in mutations:
    mutant=copy.deepcopy(STATE)
    change(mutant)
    assert findings(mutant),("Mutation escaped detection",name)
print(f"PASS Y1V4 adversarial: 173 individually reviewed records and {len(mutations)} negative mutations rejected, no human/external signoff")
