#!/usr/bin/env python3
"""Y1 V7 reference integration tests; not a literary independent review."""
import copy
import json
import unittest
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
B=ROOT/"data/canon/consolidated"
load=lambda path:json.loads((B/path).read_text(encoding="utf-8"))

def violations(fixture):
    E,C,G,idx,goals,anhs,wr,exams,rev,links,src=fixture
    eids={x["id"] for x in E["records"]}
    cs={x["id"] for x in C["cases"]}
    gids={x["id"] for x in G["transitions"]}
    bad=[]
    def check(pred,msg):
        if not pred:bad.append(msg)
    check((len(eids),len(cs),len(gids))==(113,44,18),"source volumes incomplete")
    check(len(rev["rows"])==175 and rev["external_independent_review_completed_count"]==0,"fake individual external review")
    check(set(x["id"] for x in rev["rows"])==eids|cs|gids,"audit register orphan")
    check(rev["release_gate"]["engine_memory_admission"] is False and rev["release_gate"]["merge_to_main"]=="BLOCKED","premature release")
    check(src["claims_all_verified"] is False and src["original_pdf_committed"] is False,"source verification/copyright breach")
    check(all(e["verified"] is False and e["engine_admission"] is False for e in E["records"]),"source evidence incorrectly admitted")
    check(all(x["verified"] is False for x in G["transitions"]),"unverified goals marked verified")
    check(all(x["engine_goal_update"] is False for x in G["transitions"]),"scripted historical goals")
    for c in C["cases"]:
        path=ROOT/c["path"]
        if not path.is_file():bad.append("missing case "+c["id"]);continue
        val=json.loads(path.read_text(encoding="utf-8"))
        check(c["actor_id"]==val["primary_actor_id"] and idx["behavioral_case_paths"].get(c["id"])==c["path"],"wrong case actor index "+c["id"])
    check(all(g["id"] in goals["by_actor"].get(g["actor_id"],[]) for g in G["transitions"]),"goal actor index")
    check(all(g["id"] in goals["transition_files"] for g in G["transitions"]),"goal file index")
    for x in E["records"]:
        check(x["id"] in idx["context_mentions_by_actor_topic"].get(x["mention_actor_ids"][0],{}).get(x["domain"],[]) if x["mention_actor_ids"] else True,"evidence actor index")
    check("exam.y1.sports-festival" in anhs["domains"]["exams"]["reference_exam_ids"],"sports festival missing from ANHS exam list")
    check(set(["exam.y1.midterm-1","exam.y1.deserted-island-1","exam.y1.zodiac-cruise-1","anhs.y1.paper-shuffle","exam.y1.sports-festival"]).issubset(set(anhs["domains"]["exams"]["reference_exam_ids"])),"missing historical exam reference")
    check(exams["paper"]["later_volume_7_outcomes"]["december_1_snapshot"]=={"A":874,"B":753,"C":542,"D":262},"wrong school card snapshot")
    check(exams["paper"]["later_volume_7_outcomes"]["official_final_point_totals"]=="NOT_ESTABLISHED","snapshot promoted to finals")
    check(exams["sports"]["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE","sports declared executable")
    check(exams["island"]["later_revelations"][-1]["payment_ledger_verified"] is False,"V3 contract payment ledger fabricated")
    extra=wr["white_room_model"]["y1_v07_qualified_additions"]
    check(extra["engine_memory_admission"] is False and extra["executable_institutional_behavior"] is False,"White Room engine promotion")
    check(extra["assertions"][1]["status"].startswith("CHARACTER_REPORT"),"Matsuo death no longer attributed")
    check(len(links["links"])==14 and all(x["verified"] is False for x in links["links"]),"retrospective register promotion")
    check(all(x["verified"] is False for x in E["records"]),"verified evidence without second reviewer")
    return bad

class V7Integration(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.fixture=(
          load("evidence/y1-v07.json"),load("audits/y1-v07-case-manifest.json"),
          load("goal_transitions/y1-v07.json"),load("indexes/character-topic-map.json"),
          load("indexes/goal-transition-map.json"),load("institutions/anhs.json"),
          load("institutions/white_room.json"),
          {"paper":load("exams/y1-paper-shuffle.json"),"sports":load("exams/y1-sports-festival.json"),
           "island":load("exams/y1-deserted-island.json")},
          load("audits/source-review-register-y1-v07.json"),
          load("retrospective/y1-v07-revelation-links.json"),load("sources/y1-v07.json"))
    def test_consistent(self):self.assertEqual([],violations(self.fixture))
    def mutate(self,index,fun):
        t=list(self.fixture);t[index]=copy.deepcopy(t[index]);fun(t[index])
        self.assertTrue(violations(t))
    def test_false_review(self):self.mutate(8,lambda x:x.update(external_independent_review_completed_count=175))
    def test_fake_school_snapshot(self):self.mutate(7,lambda x:x["paper"]["later_volume_7_outcomes"].update(official_final_point_totals={"A":874,"B":753,"C":542,"D":262}))
    def test_destroy_exam_pack_reference(self):self.mutate(5,lambda x:x["domains"]["exams"]["reference_exam_ids"].remove("exam.y1.sports-festival"))
    def test_false_matsuo_verification(self):self.mutate(6,lambda x:x["white_room_model"]["y1_v07_qualified_additions"]["assertions"][1].update(status="INSTITUTIONALLY_CERTIFIED_DEATH"))
    def test_autoupdate(self):self.mutate(2,lambda x:x["transitions"][0].update(engine_goal_update=True))
    def test_retro_future_truth(self):self.mutate(9,lambda x:x["links"][0].update(verified=True))

if __name__=="__main__":
    unittest.main()
