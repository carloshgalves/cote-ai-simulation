#!/usr/bin/env python3
"""V8 canon pack integrity, provenance and adversarial controls.

A passing suite is not an independent review of the original PDF and cannot
authorize an automatic historical replay or engine memory import.
"""
import copy
import json
import unittest
from pathlib import Path

BASE=Path(__file__).resolve().parents[1]/"data/canon/consolidated"
ROOT=BASE.parents[2]
J=lambda path: json.loads(path.read_text(encoding="utf-8"))
SRC="ln.y1.v08.sevenseas.en.operator-provided"

def load():
    files=["sources/y1-v08.json","evidence/y1-v08.json","audits/y1-v08-review-register.json",
           "audits/y1-v08-case-manifest.json","relationships/y1-v08.json",
           "indexes/character-topic-map.json","indexes/evidence-topic-map.json",
           "indexes/y1-v08-page-map.json","indexes/relationship-map.json",
           "exams/y1-mixed-training-camp.json","institutions/anhs.json",
           "institutions/student-council.json"]
    obj={p:J(BASE/p) for p in files}
    obj["cases"]={x["id"]:J(ROOT/x["path"]) for x in obj["audits/y1-v08-case-manifest.json"]["cases"]}
    obj["characters"]={p.stem:J(p) for p in (BASE/"characters").glob("*.json")}
    return obj

def violations(s):
    bad=[]
    def ck(cond,why):
        if not cond:bad.append(why)
    source=s["sources/y1-v08.json"]
    E=s["evidence/y1-v08.json"]
    A=s["audits/y1-v08-review-register.json"]
    M=s["audits/y1-v08-case-manifest.json"]
    R=s["relationships/y1-v08.json"]
    EI=s["indexes/evidence-topic-map.json"]
    CI=s["indexes/character-topic-map.json"]
    PI=s["indexes/y1-v08-page-map.json"]
    RI=s["indexes/relationship-map.json"]
    exam=s["exams/y1-mixed-training-camp.json"]
    ck(source["source_id"]==SRC and source["pdf_pages"]==244 and not source["original_pdf_committed"],"wrong source")
    ck(source["pdf_sha256"]==A["pdf_sha256"] and not source["claims_all_verified"],"PDF hash and review gate")
    ck(len(A["records"])==88 and A["individual_independent_reviews_completed"]==0,"independent review forged")
    ck(len(E["records"])==80 and len(M["cases"])==38 and len(R["edges"])==14,"unexpected item cardinality")
    ck(sum(x["category"]=="FILTER" for x in A["records"])==8,"filter decisions lost")
    ck(all(x["verified"] is False and x["engine_memory_admission"] is False for x in A["records"]),"candidate automatically approved")
    ck(len({x["id"] for x in E["records"]})==80,"duplicate evidence ID")
    ck(len(s["cases"])==38,"case missing")
    ck(R["engine_memory_admission"] is False,"relationships executable")
    evidence={x["id"]:x for x in E["records"]}
    for e in E["records"]:
        ck(e["source"]["work_id"]==SRC and not e["verified"] and not e["engine_admission"],"evidence promoted to runtime")
        ck(e["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR","mention turned to behavior")
        ck(all(1<=p<=244 for p in e["source"]["pdf_pages"]),"evidence PDF locator")
        for p in e["source"]["pdf_pages"]:
            ck(e["id"] in PI["pages"][str(p)]["evidence"],"evidence page index incomplete")
        for topic in e["topics"]:ck(e["id"] in EI["evidence_by_topic"].get(topic,[]),"evidence topic index missing")
    for c in M["cases"]:
        case=s["cases"].get(c["id"])
        if case is None:
            ck(False,"missing case "+c["id"]);continue
        ck(case["primary_actor_id"]==c["actor_id"],"case actor mismatch")
        ck(case["evidence_refs"]==c["evidence_refs"] and set(case["evidence_refs"]).issubset(evidence),"case evidence mismatch")
        ck(not case["verified"] and case["behavioral_policy"]=="NON_EXECUTABLE","case runnable")
        ck(CI["behavioral_case_paths"].get(case["id"])==c["path"],"case path index missing")
        for p in c["pdf_pages"]:ck(case["id"] in PI["pages"][str(p)]["cases"],"case page index missing")
    new=["hashimoto","nazuna-asahina","ishikura","ikari-momoko","ninomiya","ayase-natsu"]
    for slug in new:
        c=s["characters"].get(slug)
        ck(c is not None and SRC in c["source_coverage"],"missing new character "+slug)
        ck("actor."+slug in CI["character_packs"],"new actor reverse index missing "+slug)
    for name in ["manabu-horikita","ishizaki","suzune-horikita","kikyo-kushida"]:
        ck(bool(s["characters"][name]["y1_v08_actor_state_deltas"]),"private state delta not represented "+name)
    for name in ["kei-karuizawa","kiyotaka-ayanokoji"]:
        ck("y1v08.rel.043" in s["characters"][name]["y1_v08_relationship_refs"],"Kei Kiyotaka relationship missing")
    ck(set(x["id"] for x in R["edges"]).issubset(RI["edges"]),"relationship search index incomplete")
    ck(RI["engine_memory_admission"] is False,"relationship-memory gate")
    ck(exam["exam_id"]=="exam.y1.mixed-training-camp" and exam["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE","duplicated or runnable exam")
    ck(len(exam["v08_evidence_refs"])==17,"exam source claims missing")
    ck(exam["scope"]["cohorts"]==["YEAR_1","YEAR_2","YEAR_3"],"three-year exam regressed")
    ck(exam["representative"]["representative_expulsion_rule"]["solidarity_requires_school_to_deem_person_contributory"],"unlimited solidarity killing")
    ck(exam["revocation_of_expulsion"]["private_points"]==20000000 and exam["revocation_of_expulsion"]["class_points"]==300,"revocation price")
    ck(not exam["engine_memory_admission"] and not exam["executable"],"exam engine policy bypass")
    ck(exam["observed_results"]["male"]["no_expulsions"] is True,"male expulsion falsely documented")
    ck(exam["observed_results"]["female"]["solidarity_named"]=="Tachibana Akane","wrong target")
    ck("EXACT_FINAL_SCORES" in exam["open_fields"],"invented individual score ledger")
    ck("actor.nagumo" in evidence["y1v08.ev.040"]["mention_actor_ids"],"Nagumo source inference detached")
    ck(evidence["y1v08.ev.040"]["source_mode"].startswith("KIYOTAKA_RECONSTRUCTION"),"secret collusion made official fact")
    return bad

class TestV8(unittest.TestCase):
    @classmethod
    def setUpClass(cls):cls.canonical=load()
    def test_full_integrity(self):self.assertEqual([],violations(self.canonical))
    def mutation(self,change):
        test=copy.deepcopy(self.canonical);change(test);self.assertNotEqual([],violations(test))
    def test_negative_literary_review_forged(self):self.mutation(lambda s:s["audits/y1-v08-review-register.json"].update(individual_independent_reviews_completed=88))
    def test_negative_four_years(self):self.mutation(lambda s:s["exams/y1-mixed-training-camp.json"]["scope"].update(cohorts=["YEAR_1","YEAR_2","YEAR_3","YEAR_4"]))
    def test_negative_unlimited_solidarity(self):self.mutation(lambda s:s["exams/y1-mixed-training-camp.json"]["representative"]["representative_expulsion_rule"].update(solidarity_requires_school_to_deem_person_contributory=False))
    def test_negative_price(self):self.mutation(lambda s:s["exams/y1-mixed-training-camp.json"]["revocation_of_expulsion"].update(private_points=2000000))
    def test_negative_keis_relationship(self):self.mutation(lambda s:s["characters"]["kei-karuizawa"].update(y1_v08_relationship_refs=[]))
    def test_negative_spurious_private_transfer(self):self.mutation(lambda s:s["exams/y1-mixed-training-camp.json"].update(executable=True))
    def test_negative_invented_exam_results(self):self.mutation(lambda s:s["exams/y1-mixed-training-camp.json"]["open_fields"].remove("EXACT_FINAL_SCORES"))
    def test_negative_unverified_promotion(self):self.mutation(lambda s:s["evidence/y1-v08.json"]["records"][0].update(verified=True))
    def test_negative_case_deleted(self):self.mutation(lambda s:s["cases"].pop(next(iter(s["cases"]))))
    def test_negative_prior_knowledge(self):self.mutation(lambda s:s["evidence/y1-v08.json"]["records"][39].update(source_mode="OFFICIAL_SIGNED_CONSPIRACY_RECORD"))
    def test_negative_no_ishizaki_change(self):self.mutation(lambda s:s["characters"]["ishizaki"].update(y1_v08_actor_state_deltas=[]))

if __name__=="__main__":unittest.main()
