#!/usr/bin/env python3
"""Y1 V7.5 global pack integration gates (not external literary review)."""
import copy
import json
import unittest
from pathlib import Path
B=Path(__file__).resolve().parents[1]/"data/canon/consolidated"
J=lambda p: json.loads(p.read_text(encoding="utf-8"))
SRC="ln.y1.v075.sevenseas.en.operator-provided"
def load():
    ld=J(B.parents[2]/"docs/canon/audits/y1-v075-candidate-ledger.json")
    cases={r["id"]:J(B/r["path"].replace("data/canon/consolidated/","")) for r in J(B/"audits/y1-v075-case-manifest.json")["cases"]}
    return dict(ledger=ld,source=J(B/"sources/y1-v075.json"),evidence=J(B/"evidence/y1-v075.json"),
       manifest=J(B/"audits/y1-v075-case-manifest.json"),cases=cases,
       relationships=J(B/"relationships/y1-v075.json"),relation_index=J(B/"indexes/relationship-map.json"),
       actor_index=J(B/"indexes/character-topic-map.json"),evidence_index=J(B/"indexes/evidence-topic-map.json"),
       page_index=J(B/"indexes/y1-v075-page-map.json"),
       kiriyama=J(B/"characters/kiriyama.json"),kei=J(B/"characters/kei-karuizawa.json"),
       school=J(B/"institutions/anhs.json"),council=J(B/"institutions/student-council.json"),
       whiteroom=J(B/"institutions/white_room.json"),festival=J(B/"exams/y1-sports-festival.json"),
       island=J(B/"exams/y1-deserted-island.json"),zodiac=J(B/"exams/y1-zodiac-cruise.json"))
def errors(s):
    issues=[]
    def require(x,message):
        if not x:issues.append(message)
    ld=s["ledger"];src=s["source"];e=s["evidence"];m=s["manifest"];idx=s["actor_index"]
    require(src["source_id"]==SRC and src["original_pdf_committed"] is False,"private PDF/source ref")
    require(src["pdf_pages"]==172 and src["pdf_sha256"]==ld["pdf_sha256"],"PDF digest/size")
    require(src["claims_all_verified"] is False,"source review not completed")
    require(len(ld["records"])==77 and len(e["records"])==67 and len(m["cases"])==37,"candidate/evidence/case cardinality")
    require(sum(r["category"]=="FILTER" for r in ld["records"])==10,"exclusions improperly promoted")
    require({x["source_catalog_id"] for x in e["records"]}=={r["id"] for r in ld["records"] if r["category"]!="FILTER"},"missing or wrong candidate promotion")
    eid={x["id"]:x for x in e["records"]}
    require(len(eid)==67 and len(s["cases"])==37,"duplicate cases/evidence")
    for x in e["records"]:
        require(x["source"]["work_id"]==SRC and not x["verified"] and not x["engine_admission"],"evidence admitted")
        require(x["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR","mention promoted to feat")
        require(bool(x["source"]["pdf_pages"]) and all(1<=p<=172 for p in x["source"]["pdf_pages"]),"out-of-bounds source locator")
        for t in x["topics"]: require(x["id"] in s["evidence_index"]["evidence_by_topic"][t],"topic index incomplete")
        for p in x["source"]["pdf_pages"]:require(x["id"] in s["page_index"]["pages"][str(p)]["evidence"],"page evidence index incomplete")
    for item in m["cases"]:
        c=s["cases"].get(item["id"])
        if c is None:
            require(False,"missing registered behavioral case "+item["id"])
            continue
        require(c["id"]==item["id"] and c["primary_actor_id"]==item["actor_id"],"primary actor attribution")
        require(c["evidence_refs"]==item["evidence_refs"] and all(ref in eid for ref in c["evidence_refs"]),"case evidence link")
        require(not c["verified"] and c["behavioral_policy"]=="NON_EXECUTABLE","case prematurely promoted")
        require(c["counterfactual_test"].startswith("Do not force"),"divergent history overdetermined")
        require(idx["behavioral_case_paths"][c["id"]]==item["path"],"case global lookup missing")
        for p in c["source"]["pdf_pages"]:require(c["id"] in s["page_index"]["pages"][str(p)]["cases"],"case page index missing")
    require(s["kiriyama"]["character_id"]=="actor.kiriyama" and SRC in s["kiriyama"]["source_coverage"],"Kiriyama pack absent")
    require("y1v075.case.kiriyama.v075-c036" in s["kiriyama"]["domains"]["politics"]["behavioral_case_refs"],"Kiriyama case not searchable")
    require(len(s["kei"]["y1_v075_actor_state_deltas"])==4,"Kei staged internal development missing")
    rel=s["relationships"];ed={x["id"]:x for x in rel["edges"]}
    require(len(ed)==7 and not rel["engine_memory_admission"],"relationship edges not gated")
    require(ed["y1v075.rel.kei-kiyotaka"]["public_status"]=="NOT_DECLARED_COUPLE","unearned Kei-Kiyotaka relationship")
    require(ed["y1v075.rel.kei-hirata"]["actual_breakup_in_volume"] is False,"fake couple already broken")
    require(ed["y1v075.rel.sato-kiyotaka"]["official_couple"] is False,"Satou becomes girlfriend")
    require(ed["y1v075.rel.ike-shinohara"]["couple_declared"] is False,"Ike/Shinohara relationship certified")
    require(len(s["relation_index"]["edges"])==len(ed),"relationship retrieval index missing")
    require(s["council"]["y1_v075_roster_governance_and_nagumo_opposition"]["disciplinary_statistics"]["reported_total_second_year_expulsions"]==17,"Manabu expulsion report erased")
    require(s["council"]["y1_v075_roster_governance_and_nagumo_opposition"]["disciplinary_statistics"]["individually_certified_cases"]==0,"17 reports turned into independently verified cases")
    require(s["council"]["y1_v075_roster_governance_and_nagumo_opposition"]["nagumo_online_exam_tease"]["status"]=="PROVOCATION_NOT_A_CONFIRMED_EXAM_RULE","hypothetical game promoted to exam")
    transfer=s["school"]["y1_v075_advanced_notice_and_transfer_confirmation"]["individual_class_transfer"]
    require(transfer["v2_teacher_reported_price"]==20000000 and transfer["v7_ibuki_dialogue_discrepancy"]==2000000,"silently changed V2/V7 transfer disagreement")
    require(transfer["normative_formula_or_group_discount_verified"] is False,"invented 800M formula")
    require(s["school"]["y1_v075_advanced_notice_and_transfer_confirmation"]["future_exam"]["exam_spec_created"] is False,"future January exam pretended executable")
    require(s["whiteroom"]["white_room_model"]["y1_v075_external_knowledge_bridge"]["new_training_curriculum"]=="NONE","fabricated White Room curriculum")
    for x in ("festival","island","zodiac"):
        require(SRC in s[x]["source_coverage"],"exam source reference absent "+x)
        require(s[x]["authority"] in ("CANON_REFERENCE_NOT_EXECUTABLE","REFERENCE_ONLY","CANON_REFERENCE"),"exam promoted to runtime "+x)
    return issues
class TestY1V075(unittest.TestCase):
    @classmethod
    def setUpClass(cls): cls.base=load()
    def test_full_grounded_integration(self):self.assertEqual([],errors(self.base))
    def fail_if_mutated(self,mut):
        state=copy.deepcopy(self.base);mut(state);self.assertNotEqual([],errors(state))
    def test_negative_ascribe_verified(self):self.fail_if_mutated(lambda s:s["evidence"]["records"][0].update(verified=True))
    def test_negative_character_cured(self):self.fail_if_mutated(lambda s:s["kei"]["y1_v075_actor_state_deltas"].clear())
    def test_negative_force_romance(self):self.fail_if_mutated(lambda s:next(x for x in s["relationships"]["edges"] if x["id"]=="y1v075.rel.kei-kiyotaka").update(public_status="OFFICIAL_COUPLE"))
    def test_negative_set_sato_couple(self):self.fail_if_mutated(lambda s:next(x for x in s["relationships"]["edges"] if x["id"]=="y1v075.rel.sato-kiyotaka").update(official_couple=True))
    def test_negative_school_17_direct_kicks(self):self.fail_if_mutated(lambda s:s["council"]["y1_v075_roster_governance_and_nagumo_opposition"]["disciplinary_statistics"].update(individually_certified_cases=17))
    def test_negative_nagumo_game_spec(self):self.fail_if_mutated(lambda s:s["council"]["y1_v075_roster_governance_and_nagumo_opposition"]["nagumo_online_exam_tease"].update(status="APPROVED"))
    def test_negative_change_v7_transfer_quote(self):self.fail_if_mutated(lambda s:s["school"]["y1_v075_advanced_notice_and_transfer_confirmation"]["individual_class_transfer"].update(v7_ibuki_dialogue_discrepancy=20000000))
    def test_negative_auto_future_exam(self):self.fail_if_mutated(lambda s:s["school"]["y1_v075_advanced_notice_and_transfer_confirmation"]["future_exam"].update(exam_spec_created=True))
    def test_negative_case_removed(self):self.fail_if_mutated(lambda s:s["cases"].pop(next(iter(s["cases"]))))
    def test_negative_knowledge_broadcast(self):self.fail_if_mutated(lambda s:s["evidence"]["records"][0].update(subject_assertion="EVERYONE_KNOWS"))
    def test_negative_white_room_curriculum(self):self.fail_if_mutated(lambda s:s["whiteroom"]["white_room_model"]["y1_v075_external_knowledge_bridge"].update(new_training_curriculum="EXCLUSIVE_NEW_PROGRAM"))
if __name__=="__main__":unittest.main()
