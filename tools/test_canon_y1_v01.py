#!/usr/bin/env python3
"""Y1V1 canonical data contract tests: behavioral attribution, factual scope, chronology."""
import json
from pathlib import Path

R=Path(__file__).resolve().parents[1]/"data/canon/consolidated"
def load(p): return json.loads(p.read_text(encoding="utf-8"))
index=load(R/"indexes/character-topic-map.json")
evidence=load(R/"evidence/y1-v01.json")["records"]
ev={e["id"]:e for e in evidence}
source=load(R/"sources/y1-v01.json")
ledger=load(R/"audits/y1-v01-coverage.json")
school=load(R/"institutions/anhs.json")
exam=load(R/"exams/y1-first-midterm.json")
assessment=load(R/"assessments/y1-v01-swimming.json")

assert len(evidence)==136 and len(ev)==136
assert set(ledger["chapter_evidence_counts"])==set(str(x) for x in range(1,13))
assert ledger["records"]==len(evidence)
assert all(e["source"]["pdf_pages"] and 1<=e["source"]["pdf_pages"][0]<=264 for e in evidence)
assert all(e["source"]["work_id"]==source["source_id"] for e in evidence)
assert all(e["source_mode"] in {"OBSERVED","NARRATED","CHARACTER_ASSERTION","INNER_MONOLOGUE"} for e in evidence)
assert all(e["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR" for e in evidence)
assert source["operator_accepted"] and not source["claims_all_verified"]

def get_case(case_id,actor):
    path=index["behavioral_case_paths"][case_id]
    c=load(R.parents[2]/path)
    assert c["id"]==case_id
    assert c["primary_actor_id"]=="actor."+actor
    assert Path(path).parent.name==actor
    assert all(ref in ev or ref.startswith("v0.") for ref in c["evidence_refs"])
    return c

for a,n in [
    ("onodera","fifty-meter-swim"),
    ("kikyo-kushida","fifty-meter-swim"),
    ("ken-sudo","fifty-meter-swim"),
    ("kiyotaka-ayanokoji","fifty-meter-swim-withheld-effort"),
    ("rokusuke-koenji","swim-final-over-sudo"),
    ("haruki-yamauchi","confuse-apec-and-toeic"),
    ("ken-sudo","prioritize-study-over-basketball"),
]:
    get_case("y1v1.case."+a+"."+n,a)

seen={p["actor"]:p for p in assessment["observed_feats"]}
assert seen["actor.onodera"]["time_seconds_approx"]==26
assert seen["actor.kikyo-kushida"]["time_seconds_approx"]==31
assert seen["actor.ken-sudo"]["time_seconds_approx"]==25
assert seen["actor.kiyotaka-ayanokoji"]["time_seconds_approx"]==36
assert seen["actor.rokusuke-koenji"]["time_seconds"]==23.22
assert seen["actor.yosuke-hirata"]["time_seconds"]==26.13
assert assessment["type"]=="PHYSICAL_EDUCATION_NOT_SPECIAL_EXAM"
assert "not maximal" in " ".join(assessment["simulation_cautions"])

yamawaki=load(R/"characters/yamawaki.json")
assert "Class C" in yamawaki["identity"]["role_summary"]
assert "Class A" not in yamawaki["identity"]["role_summary"]
assert any("Class C student Yamawaki" in e["summary"] for e in evidence)
assert not any("Class A student Yamawaki" in e["summary"] for e in evidence)

for a in ["airi-sakura","hasebe","matsushita","mori","kikuchi"]:
    actor=load(R/"characters"/(a+".json"))
    assert actor["modeling_tier"]=="MINIMAL_ENTITY"
    assert index["behavioral_by_actor_topic"]["actor."+a]=={}

assert school["s_system"]["class_points"]["evaluation_formula"]=="NOT_DISCLOSED"
assert school["s_system"]["private_points"]["initial_grant"]==100000
assert school["s_system"]["class_points"]["first_may_observed"]=={"A":940,"B":650,"C":490,"D":0}
assert "not disclosed" in school["public_disclosure_policy"]["Y1_START"].lower() or "not the real" in school["public_disclosure_policy"]["Y1_START"].lower()
assert any(e["source"]["pdf_pages"]==[225] and "changed" in e["summary"].lower() for e in evidence)

assert exam["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE"
assert exam["exceptional_transaction"]["cost_private_points"]==100000
assert exam["grading_observations"]["cutoff"]==40
assert "No universal" in " ".join(exam.get("contradictions_to_avoid",[])) or exam["simulation_policy"].startswith("Do not")
assert any(e["source"]["pdf_pages"]==[128] and "90" in e["summary"] for e in evidence)
assert all(not e["story_time"].startswith("Y3") for e in evidence)

print("PASS Y1V1 full-page-location, cases, fitness, actors, ANHS point/epistemic, grading and institutional constraints")
