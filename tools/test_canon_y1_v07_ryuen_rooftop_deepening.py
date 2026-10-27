#!/usr/bin/env python3
"""Regression and hostile-mutation gates for V7 Ryuen 800M/rooftop source dossier.

These checks cannot replace a distinct independent human/model primary-PDF
review and do not authorize canon release to executable agent memories.
"""
import copy
import json
import unittest
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
BASE=ROOT/"data/canon/consolidated"
DOSSIER=BASE/"audits/y1-v07-ryuen-plan-rooftop-deepening.json"

def read(path):return json.loads(path.read_text(encoding="utf-8"))
def load():
    d=read(DOSSIER)
    packs={}
    for key in d["case_ids"]:
        actor,stem=key.rsplit(".",1)
        p=BASE/"behavioral_cases"/actor/(stem+".json")
        packs[key]=read(p)
    return {"d":d,"cases":packs,"anhs":read(BASE/"institutions/anhs.json"),
            "source":read(BASE/"sources/y1-v07.json"),
            "evidence":read(BASE/"evidence/y1-v07.json")}

def violations(state):
    d=state["d"];packs=state["cases"];errors=[]
    def require(test,label):
        if not test:errors.append(label)
    sid="ln.y1.v07.sevenseas.en.operator-provided"
    require(d["source"]["work_id"]==sid,"wrong source")
    require(d["source"]["physical_pdf_pages"]==207 and d["source"]["sha256"]==state["source"]["pdf_sha256"],"wrong source locator/hash")
    require(d["status"]["claims_all_verified"] is False and d["status"]["simulation_runtime_admission"] is False,"faked verification or runtime")
    require(d["status"]["independent_second_reviewer"]=="NOT_PERFORMED","external reviewer forged")
    require(d["release_gates"]["external_independent_review_required"] and d["release_gates"]["retroactive_actor_memory_forbidden"],"release bypass")
    require(len(d["case_ids"])==18 and len(set(d["case_ids"]))==18,"case coverage")
    require(set(d["case_ids"])==set(packs),"unlinked primary actor cases")
    evidence={x["id"] for x in state["evidence"]["records"]}
    for key,c in packs.items():
        slug,stem=key.rsplit(".",1)
        require(c["id"]=="y1v7.case."+key,"case identity "+key)
        require(c["actor_id"]=="actor."+slug and c["primary_actor_id"]=="actor."+slug,"actor attribution "+key)
        require(c["verified"] is False and c["epistemic_status"]=="UNVERIFIED","unverified case promoted "+key)
        a=c.get("episode_deepening",{})
        require(a.get("dossier_ref")=="data/canon/consolidated/audits/y1-v07-ryuen-plan-rooftop-deepening.json","dossier link "+key)
        require(a.get("runtime_admission") is False and a.get("simulator_replay")=="NOT_DETERMINISTIC","replay/rating leak "+key)
        require(a.get("independent_review")=="NOT_PERFORMED","pretend external reviewer "+key)
        require(bool(a.get("observed_mechanism")) and bool(a.get("epistemic_and_capability_limits")),"empty actor reasoning "+key)
        require(bool(c["source"]["pdf_pages"]) and all(1<=p<=207 for p in c["source"]["pdf_pages"]),"source page outside PDF "+key)
        require(set(c["evidence_refs"]).issubset(evidence),"unknown source ref "+key)
    combat=d["combat"]
    opponents=combat["engagement_records"]
    require(not combat["simultaneous_four_person_attack"],"false four-at-once combat")
    require([e["actor_id"] for e in opponents]==["actor.ishizaki","actor.yamada-albert","actor.mio-ibuki","actor.kakeru-ryuen"],"wrong fight order")
    require(opponents[1]["limits"][0].startswith("Kiyotaka is not invulnerable"),"pain erased")
    require("Kiyotaka explicitly experiences pain" in opponents[3]["limits"],"pain denied")
    require("actor.kei-karuizawa" in combat["present_on_roof"],"Karuizawa witnessed rooftop")
    require(len(combat["contextual_witnesses_outside_roof"])==2,"context witnesses missing")
    witnesses=d["kiyotaka_counterplan"]["witnesses"]
    require({x["actor_id"] for x in witnesses}=={"actor.manabu-horikita","actor.sae-chiyabashira"},"witness identity")
    for w in witnesses:
        require(w["direct_eyewitness_of_strikes"] is False and w["official_hearing"] is False,"imagined fight eyewitness or hearing")
        require("TESTEMUNHA" in w["classification"],"witness classification missing")
    man=next(x for x in witnesses if x["actor_id"]=="actor.manabu-horikita")
    ch=next(x for x in witnesses if x["actor_id"]=="actor.sae-chiyabashira")
    require(man["acted_as_student_council_president"] is False,"Manabu held former, not current, office")
    require("ESCADARIA" in man["position"],"stairwell position erased")
    require(ch["intervened_in_fight"] is False,"teacher invented intervention")
    for case_id in ["kiyotaka-ayanokoji.v7-c014","kiyotaka-ayanokoji.v7-c019","manabu-horikita.v7-c036","sae-chiyabashira.v7-c038"]:
        case=packs[case_id]
        require("testemunha" in json.dumps(case["episode_deepening"],ensure_ascii=False).lower(),"witness absent from case "+case_id)
    plan=d["eight_hundred_million"]
    require(plan["unit"]=="PRIVATE_POINTS" and "800,000,000" in plan["observed_document"],"misstated money units")
    require(plan["eventual_goal_execution"] is False,"800M executable transfer")
    require(any(v["source_mode"]=="IBUKI_INFERENCE" for v in plan["distinction"]),"Ibuki conjecture as fact")
    require(any(v["source_mode"]=="RYUEN_SELF_REPORT_OR_DEFLECTION" for v in plan["distinction"]),"Ryuen denial absent")
    require("A confession of Ryuen's true enduring collective objective" in plan["not_established"],"unproven intent certified")
    numeric=plan["unresolved_numeric_discrepancy"]
    require(numeric["v7_p199_ibuki_single_person_price"]==2000000,"V7 price silently changed")
    require(numeric["v2_teacher_reported_transfer_price"]==20000000,"V2 older rule silently changed")
    require(numeric["status"]=="OPEN_NO_SILENT_CORRECTION","price conflict falsely resolved")
    require(state["anhs"]["s_system"]["individual_class_transfer_y1_v02"]["price_private_points"]==20000000,"older school source changed")
    require(d["ryuen_operation"]["initial_epistemic_state"]["true_X_identity"]=="UNCONFIRMED_TO_RYUEN","X identity backdated")
    return errors

class TestV7Rooftop(unittest.TestCase):
    @classmethod
    def setUpClass(cls):cls.fixture=load()
    def test_record_and_source_integrity(self):self.assertEqual([],violations(self.fixture))
    def mutation(self,change):
        clone=copy.deepcopy(self.fixture)
        change(clone)
        self.assertTrue(violations(clone),"mutant slipped through")
    def test_negative_forged_external_review(self):self.mutation(lambda s:s["d"]["status"].update(independent_second_reviewer="APPROVED"))
    def test_negative_witness_observed_punches(self):self.mutation(lambda s:s["d"]["kiyotaka_counterplan"]["witnesses"][0].update(direct_eyewitness_of_strikes=True))
    def test_negative_manabu_still_president(self):self.mutation(lambda s:s["d"]["kiyotaka_counterplan"]["witnesses"][0].update(acted_as_student_council_president=True))
    def test_negative_teacher_intervened(self):self.mutation(lambda s:s["d"]["kiyotaka_counterplan"]["witnesses"][1].update(intervened_in_fight=True))
    def test_negative_four_at_once(self):self.mutation(lambda s:s["d"]["combat"].update(simultaneous_four_person_attack=True))
    def test_negative_fight_order(self):self.mutation(lambda s:s["d"]["combat"]["engagement_records"].reverse())
    def test_negative_kotaka_unhurt(self):self.mutation(lambda s:s["d"]["combat"]["engagement_records"][1]["limits"].__setitem__(0,"Kiyotaka felt no pain"))
    def test_negative_price_conflict_erased(self):self.mutation(lambda s:s["d"]["eight_hundred_million"]["unresolved_numeric_discrepancy"].update(v7_p199_ibuki_single_person_price=20000000))
    def test_negative_800m_agent_script(self):self.mutation(lambda s:s["d"]["eight_hundred_million"].update(eventual_goal_execution=True))
    def test_negative_ryuen_knows_x_early(self):self.mutation(lambda s:s["d"]["ryuen_operation"]["initial_epistemic_state"].update(true_X_identity="KIYOTAKA_CONFIRMED"))
    def test_negative_case_live_runtime(self):self.mutation(lambda s:s["cases"]["kiyotaka-ayanokoji.v7-c016"]["episode_deepening"].update(runtime_admission=True))
    def test_negative_case_witness_removed(self):self.mutation(lambda s:s["cases"]["manabu-horikita.v7-c036"]["episode_deepening"].pop("testemunha"))

if __name__=="__main__":unittest.main()
