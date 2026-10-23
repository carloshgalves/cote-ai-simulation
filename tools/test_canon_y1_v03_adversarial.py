"""Adversarial and mutation-style cross-check of Volume 3 canonical provenance.

This is an independent validation *method*, not an independent reviewer.
No primary copyrighted PDF is stored or read by CI.
"""
from __future__ import annotations
import copy
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
B=ROOT/"data/canon/consolidated"
J=lambda p:json.loads(Path(p).read_text(encoding="utf-8"))

evidence=J(B/"evidence/y1-v03.json")["records"]
by_id={e["id"]:e for e in evidence}
ix=J(B/"indexes/character-topic-map.json")
page=J(B/"indexes/y1-v03-page-map.json")
exam=J(B/"exams/y1-deserted-island.json")
register=J(B/"audits/source-review-register-y1-v03.json")
cases={id:J(ROOT/path) for id,path in ix["behavioral_case_paths"].items() if id.startswith("y1v3.case.")}
state={"evidence":by_id,"index":ix,"page":page,"exam":exam,"register":register,"cases":cases}

def issues(s):
    problems=[]
    e=s["evidence"]; ix=s["index"]; p=s["page"]; c=s["cases"]; x=s["exam"]; r=s["register"]
    if len(e)!=117 or len(c)!=35: problems.append("unexpected source/case count")
    if len(r["evidence"])!=117 or len(r["behavioral_cases"])!=35: problems.append("audit coverage")
    if r["release_gate"]["operator_approval"]!="PENDING": problems.append("operator gate promoted")
    if r["release_gate"]["external_independent_reviewer"]!="PENDING": problems.append("independent review falsely approved")
    if not all(z["reviewed_by_assistant"] and z["verified"] is False for z in r["evidence"]+r["behavioral_cases"]): problems.append("individual semantic review status")
    for rec in e.values():
        for a in rec["mention_actor_ids"]:
            if a not in ix["character_packs"]: problems.append("unknown actor "+a)
            for topic in rec["topics"]:
                if rec["id"] not in ix["context_mentions_by_actor_topic"].get(a,{}).get(topic,[]): problems.append("context index "+rec["id"])
        for n in rec["source"]["pdf_pages"]:
            if rec["id"] not in p["pages"].get(str(n),{}).get("evidence",[]): problems.append("page index "+rec["id"])
    for cid,case in c.items():
        aid=case["primary_actor_id"]
        if aid!=case["actor_id"] or aid!="actor."+Path(ix["behavioral_case_paths"][cid]).parent.name: problems.append("owner "+cid)
        if not set(case["evidence_refs"])<=e.keys(): problems.append("evidence ref "+cid)
        for topic in case["topics"]:
            if cid not in ix["behavioral_by_actor_topic"].get(aid,{}).get(topic,[]): problems.append("actor case index "+cid)
        for n in case["source"]["pdf_pages"]:
            if cid not in p["pages"].get(str(n),{}).get("cases",[]): problems.append("case page index "+cid)
    g=lambda n:e["y1v3.ev."+str(n).zfill(3)]
    if "actor.katsuragi" not in g(35)["mention_actor_ids"]: problems.append("deduction attribution")
    if "y1v3.case.katsuragi.infer-cave-route-from-cruise" not in c: problems.append("Katsuragi's behavioral case")
    if {"actor.komiya","actor.kondou"}-set(g(53)["mention_actor_ids"]) or "actor.ishizaki" in g(53)["mention_actor_ids"]: problems.append("wrong Class C messengers")
    if g(61)["source"]["pdf_pages"]!=[120,122]: problems.append("Class B-D limited pact source")
    if "alliance" not in g(97)["summary"].lower(): problems.append("observed A/C alliance erased")
    if g(113)["source_mode"]!="INTERPRETATION": problems.append("secret contract inference promoted")
    if "7:58" not in g(101)["summary"] or "roll call" not in g(101)["summary"]: problems.append("roll call chronology")
    if g(117)["source_mode"]!="REPORTED_BY_CHARACTER": problems.append("late Kaneda account promoted")
    if x["leader"]["replacement"]!="ONLY_WITH_SUITABLE_JUSTIFICATION": problems.append("leader replacement authority expanded")
    if x["leader"]["actor_knowledge_gate"]!="AFTER_EXAM_RESULT": problems.append("post-exam secret leaked")
    if x["scoring"]["observed_final_scores"]!={"A":120,"B":140,"C":0,"D":225}: problems.append("score tampered")
    if not any(z["kind"]=="POLLUTE_ENVIRONMENT" and z["class_exam_points"]==20 for z in x["penalties"]): problems.append("environment penalty tampered")
    if not any(z["kind"]=="ABSENT_ROLL_CALL" and z["class_exam_points"]==5 for z in x["penalties"]): problems.append("absence penalty tampered")
    if x["authority"]!="CANON_REFERENCE_NOT_EXECUTABLE": problems.append("unapproved executable engine")
    return sorted(set(problems))

assert not issues(state),issues(state)
mutations=[
 ("misassign_card_deduction",lambda s:s["cases"]["y1v3.case.katsuragi.infer-cave-route-from-cruise"].update(primary_actor_id="actor.kiyotaka-ayanokoji")),
 ("invent_messenger",lambda s:s["evidence"]["y1v3.ev.053"].update(mention_actor_ids=["actor.ishizaki"])),
 ("erase_page_provenance",lambda s:s["page"]["pages"]["237"]["evidence"].remove("y1v3.ev.101")),
 ("erase_a_c_alliance",lambda s:s["evidence"]["y1v3.ev.097"].update(summary="No alliance exists.")),
 ("promote_inference",lambda s:s["evidence"]["y1v3.ev.113"].update(source_mode="OBSERVED")),
 ("expand_leader_authority",lambda s:s["exam"]["leader"].update(replacement="UNRESTRICTED")),
 ("invent_penalty",lambda s:next(p for p in s["exam"]["penalties"] if p["kind"]=="POLLUTE_ENVIRONMENT").update(class_exam_points=99)),
 ("approve_without_operator",lambda s:s["register"]["release_gate"].update(operator_approval="APPROVED")),
 ("approve_without_independent",lambda s:s["register"]["release_gate"].update(external_independent_reviewer="APPROVED")),
]
for name,mutate in mutations:
    mutant=copy.deepcopy(state)
    mutate(mutant)
    assert issues(mutant),("mutation escaped detection",name)
print(f"PASS V3 adversarial gates: 152 source claims/cases, {len(mutations)} negative mutations detected, release locked")
