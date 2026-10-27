#!/usr/bin/env python3
"""Y1 V9 candidate-pack integrity and negative controls. NOT an external literary review."""
import copy
import json
import unittest
from pathlib import Path

B = Path(__file__).resolve().parents[1] / "data/canon/consolidated"
ROOT = B.parents[2]
SRC = "ln.y1.v09.en.operator-provided"
J = lambda p: json.loads(p.read_text(encoding="utf-8"))

def load():
    names = ["sources/y1-v09.json", "evidence/y1-v09.json",
             "audits/y1-v09-case-manifest.json", "relationships/y1-v09.json",
             "indexes/character-topic-map.json", "indexes/evidence-topic-map.json",
             "indexes/y1-v09-page-map.json", "indexes/relationship-map.json",
             "indexes/retrospective-event-map.json",
             "exams/y1-v09-academic-context.json", "institutions/anhs.json",
             "institutions/student-council.json"]
    s = {name:J(B/name) for name in names}
    m = s["audits/y1-v09-case-manifest.json"]
    s["cases"] = {x["case_id"]:J(ROOT / s["indexes/character-topic-map.json"]["behavioral_case_paths"][x["case_id"]])
                  for x in m["cases"]}
    s["actors"] = {a:J(B/"characters"/(a+".json")) for a in
                   ["honami-ichinose", "nagumo", "kiyotaka-ayanokoji", "masumi-kamuro"]}
    return s

def violations(s):
    bad = []
    def ck(test, reason):
        if not test:bad.append(reason)
    source=s["sources/y1-v09.json"]
    evidence=s["evidence/y1-v09.json"]
    m=s["audits/y1-v09-case-manifest.json"]
    r=s["relationships/y1-v09.json"]
    ci=s["indexes/character-topic-map.json"]
    ei=s["indexes/evidence-topic-map.json"]
    pi=s["indexes/y1-v09-page-map.json"]
    ri=s["indexes/relationship-map.json"]
    retro=s["indexes/retrospective-event-map.json"]
    e_by_id={e["id"]:e for e in evidence["records"]}
    ck(source["source_id"]==SRC and source["pdf_sha256"]=="51c4bdcea2cfa0389617a56de52970a7db878c4e37d4e07a02b7863df9c1d6db", "source digest")
    ck(source["pdf_pages"]==238 and source["original_pdf_committed"] is False, "source provenance")
    ck(not source["claims_all_verified"] and not source["external_independent_review_completed"], "source falsely reviewed")
    ck(len(m["cases"])==24 and len(evidence["records"])==34 and len(r["edges"])==11, "item counts")
    ck(len(set(e_by_id))==34 and len(s["cases"])==24, "duplicate/missing evidence/cases")
    ck(m["engine_memory_admission"] is False and m["goal_runtime_auto_update"] is False and
       m["independent_review_status"]=="NOT_PERFORMED", "release gate")
    for e in evidence["records"]:
        ck(e["source"]["work_id"]==SRC and e["verified"] is False and not e["engine_admission"], "evidence auto-promoted")
        ck(e["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR", "context wrongly promoted")
        ck(all(1<=p<=238 for p in e["source"]["pdf_pages"]), "source page bounds")
        for t in e["topics"]:ck(e["id"] in ei["evidence_by_topic"].get(t,[]), "missing topic evidence")
        for p in e["source"]["pdf_pages"]:ck(e["id"] in pi["pages"][str(p)]["evidence"], "missing page evidence")
    for rec in m["cases"]:
        c=s["cases"].get(rec["case_id"])
        if not c:ck(False, "case missing");continue
        ck(c["primary_actor_id"]==rec["primary_actor_id"] and c["source"]["work_id"]==SRC, "wrong case ownership")
        ck(c["verified"] is False and c["behavioral_policy"]=="NON_EXECUTABLE", "case executable")
        ck(c["evidence_refs"]==[rec["evidence_id"]], "case evidence refs")
        ck(c["source"]["pdf_pages"]==rec["pdf_pages"], "case source pages")
        ck(c["id"] in ci["behavioral_case_paths"], "case direct index")
        for p in rec["pdf_pages"]:ck(c["id"] in pi["pages"][str(p)]["cases"], "case page index")
        owners=ci["behavioral_by_actor_topic"].get(c["primary_actor_id"],{})
        ck(any(c["id"] in ids for ids in owners.values()), "case not assigned to owner pack")
    ck(r["verified"] is False and r["engine_memory_admission"] is False, "relationships memory leak")
    ck(all(x["id"] in ri["edges"] and x["verified"] is False for x in r["edges"]), "relation index/gate")
    ck("y1v09.ev.023" in next(x for x in r["edges"] if x["id"]=="y1v09.rel.003")["source_refs"], "wrong confidant evidence")
    ex=s["exams/y1-v09-academic-context.json"]
    ck(ex["authority"]=="CANON_CONTEXT_ONLY_NOT_EXAM_SPEC" and ex["exam_executable"] is False, "invented special exam")
    anhs=s["institutions/anhs.json"]["y1_v09_rumor_and_academic_context"]
    ck(anhs["rumors"]["prospective_only"] and anhs["academic_assessments"]["new_special_exam_mechanic_observed"] is False, "backdated rumor rule or invented exam")
    ich=s["actors"]["honami-ichinose"]["y1_v09_pre_admission_biography"]
    ck(ich["scope"]=="BIOGRAPHY_NOT_SIMULATION_EPISODIC_MEMORY" and ich["known_by_at_entry"]==["actor.honami-ichinose"] and ich["not_public_at_entry"], "pre-enrollment leak")
    ck(s["actors"]["nagumo"]["y1_v09_ichinose_secret_acquisition"]["known_at_y1_start"] is False, "Nagumo premature knowledge")
    ck(s["actors"]["kiyotaka-ayanokoji"]["y1_v09_ichinose_secret_acquisition"]["known_at_y1_start"] is False, "Kiyotaka premature knowledge")
    ck(s["actors"]["masumi-kamuro"]["y1_v09_school_furto_temporal_guard"]["time"]=="AFTER_Y1_START", "Kamuro event backdating")
    ck("evt.pre-y1.ichinose-hairclip-shoplifting" in retro["event_lookup"], "missing retrospective index")
    return bad

class TestV9(unittest.TestCase):
    @classmethod
    def setUpClass(cls):cls.canonical=load()
    def test_full_integrity(self):self.assertEqual([],violations(self.canonical))
    def mutated(self, func):
        sample=copy.deepcopy(self.canonical);func(sample)
        self.assertNotEqual([],violations(sample))
    def test_negative_source_promotion(self):self.mutated(lambda x:x["sources/y1-v09.json"].update(claims_all_verified=True))
    def test_negative_future_memory(self):self.mutated(lambda x:x["actors"]["honami-ichinose"]["y1_v09_pre_admission_biography"].update(scope="SIMULATION_EPISODIC_MEMORY"))
    def test_negative_private_secret_public(self):self.mutated(lambda x:x["actors"]["honami-ichinose"]["y1_v09_pre_admission_biography"].update(known_by_at_entry=["actor.honami-ichinose","actor.nagumo"]))
    def test_negative_nagumo_knows_early(self):self.mutated(lambda x:x["actors"]["nagumo"]["y1_v09_ichinose_secret_acquisition"].update(known_at_y1_start=True))
    def test_negative_kiyotaka_knows_early(self):self.mutated(lambda x:x["actors"]["kiyotaka-ayanokoji"]["y1_v09_ichinose_secret_acquisition"].update(known_at_y1_start=True))
    def test_negative_kamuro_time(self):self.mutated(lambda x:x["actors"]["masumi-kamuro"]["y1_v09_school_furto_temporal_guard"].update(time="PRE_Y1"))
    def test_negative_special_exam(self):self.mutated(lambda x:x["exams/y1-v09-academic-context.json"].update(exam_executable=True))
    def test_negative_backdated_rumor(self):self.mutated(lambda x:x["institutions/anhs.json"]["y1_v09_rumor_and_academic_context"]["rumors"].update(prospective_only=False))
    def test_negative_fake_independent_review(self):self.mutated(lambda x:x["audits/y1-v09-case-manifest.json"].update(independent_review_status="COMPLETE"))
    def test_negative_wrong_actor(self):self.mutated(lambda x:next(iter(x["cases"].values())).update(primary_actor_id="actor.nagumo"))
    def test_negative_confidant_wrong_evidence(self):self.mutated(lambda x:next(r for r in x["relationships/y1-v09.json"]["edges"] if r["id"]=="y1v09.rel.003").update(source_refs=["y1v09.ev.005"]))

if __name__=="__main__":unittest.main()
