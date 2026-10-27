#!/usr/bin/env python3
"""Adversarial source-bound followup for Student Council, Zodiac and ANHS/White Room.

This script validates data consistency and epistemic boundaries, NOT independent
literary review or permission to admit these assertions to agent memory.
"""
import copy
import json
import unittest
from pathlib import Path

B=Path(__file__).resolve().parents[1]/"data/canon/consolidated"
J=lambda rel:json.loads((B/rel).read_text(encoding="utf-8"))
SRC="ln.y1.v07.sevenseas.en.operator-provided"

def errors(s):
    a=[]
    def require(b,msg):
        if not b:a.append(msg)
    council=s["student-council"]
    zodiac=s["zodiac"]
    school=s["anhs"]
    wr=s["white_room"]
    actors=s["characters"]
    ev=s["evidence"]
    index=s["institution-index"]
    actorindex=s["actor-index"]
    require(council["structure"]["president"]=="actor.nagumo","Manabu incorrectly shown as current president")
    require(council["structure"]["former_president"]=="actor.manabu-horikita","Manabu succession lost")
    warn=council["y1_v07_succession_and_private_warning"]
    require(warn["offices"]["current_president"]=="actor.nagumo","new president continuity")
    require(warn["observed_new_rules"]=="NONE_CONFIRMED_BY_Y1_V07","Nagumo forecasts hardened into approved bylaws")
    require(warn["contingent_coordination"]["outcome"].startswith("KIYOTAKA_ACCEPTS_CONTACT_ONLY"),"Kiyotaka made to agree automatically")
    require(warn["rooftop_witness_boundary"]["did_not_directly_observe_fight"] is True,"former president made direct fight eyewitness")
    require(warn["rooftop_witness_boundary"]["punishment_or_official_hearing"]=="NONE_CONDUCTED_BY_MANABU","former president issued false sanctions")
    require(warn["actor_knowledge_gate"].find("NAGUMO_NOT_AUTOMATICALLY_AWARE")>=0,"Nagumo receives private warning without a transmission")
    require(warn["engine_memory_admission"] is False,"council admitted to agent engine")
    require(SRC in council["source_coverage"],"council source absent")
    require(index["institutions"]["org.student-council"]["source_volume_count"]==len(set(council["source_coverage"])),"institution index stale")
    require(actors["manabu"]["identity"]["role_summary"].startswith("Former ANHS"),"Manabu pack still uses old title")
    require(SRC in actors["nagumo"]["source_coverage"],"Nagumo context missing")
    require("y1v7.ev.047" in actors["nagumo"]["domains"]["strategy"]["context_evidence_refs"],"Nagumo missing from private-source context")
    require("y1v7.ev.047" in actorindex["context_mentions_by_actor_topic"]["actor.nagumo"]["strategy"],"Nagumo reverse index missing")
    evidence={x["id"]:x for x in ev["records"]}
    require("actor.nagumo" in evidence["y1v7.ev.047"]["mention_actor_ids"],"Nagumo attribution absent")
    require("org.student-council" in evidence["y1v7.ev.047"]["mention_organization_ids"],"council institution attribution absent")
    require(zodiac["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE","Zodiac promoted to executable")
    require(zodiac["group_count"]==12 and zodiac["observed_final_outcomes"]["Rabbit"]==4,"original exam canon overwritten")
    new=[x for x in zodiac["later_revelations"] if x["source_id"]==SRC]
    require(len(new)==2,"V7 Zodiac reconstruction incomplete")
    if len(new)==2:
        manabe=next((x for x in new if x.get("source_evidence_refs")),None)
        koenji=next((x for x in new if x.get("source_locator_pending_atomic_evidence")),None)
        require(manabe is not None and all(x in evidence for x in manabe["source_evidence_refs"]),"Manabe V7 anchors invalid")
        require(koenji is not None and 110 in koenji["pdf_pages"],"Koenji Monkey VIP later source missing")
        require(koenji is not None and koenji["source_evidence_refs"]==[],"invented atomic Koenji evidence")
        require(all(x["verified"] is False for x in new),"premature Zodiac validation")
        require(all(x["claim_promotion"].startswith(("RETRIEVAL_CONTEXT_ONLY","NO_GENERAL")) for x in new),"Zodiac reveal silently makes a rule")
    require("vip_selection_formula" not in zodiac,"Zodiac VIP algorithm fabricated")
    admission=school["y1_v07_school_board_admission_and_retention"]
    require(admission["office"]=="SCHOOL_BOARD_CHAIRMAN_NOT_STUDENT_COUNCIL_PRESIDENT","board and student government confused")
    require(admission["actor"]=="actor.sakayanagi-father","false chair identity")
    require(admission["admissions"]["class_d_placement_interpretation"]["status"]=="NARRATOR_INFERENCE_NOT_CONFIRMED_BY_CHAIRMAN","student placement motive falsely certified")
    require("only unexpected applicant" in admission["admissions"]["exception"].lower(),"Kiyotaka exception incorrectly universalized")
    require("genuine breach" in admission["student_retention"]["disciplinary_exception"].lower(),"absolute immunity invented")
    require(admission["engine_memory_admission"] is False,"board's private rules leaked to actors")
    require("org.anhs" in evidence["y1v7.ev.035"]["mention_organization_ids"],"board testimony institution attribution missing")
    boundary=wr["white_room_model"]["y1_v07_school_board_interface"]
    require(boundary["prior_source_id"]=="ln.y1.v00.legacyemtls.v1.0.1","White Room historical anchor lost")
    require(boundary["no_parental_control_inference"] is True,"Atsuomi given current ANHS rule power")
    require(boundary["no_private_funding_control_inference"] is True,"V0 observation made current White Room ownership of ANHS")
    require(boundary["engine_memory_admission"] is False,"private White Room/ANHS link made agent memory")
    require(wr["white_room_model"]["y1_v07_qualified_additions"]["engine_memory_admission"] is False,"old WR guard bypassed")
    return a

def load_data():
    return {
        "student-council":J("institutions/student-council.json"),
        "zodiac":J("exams/y1-zodiac-cruise.json"),
        "anhs":J("institutions/anhs.json"),
        "white_room":J("institutions/white_room.json"),
        "characters":{"nagumo":J("characters/nagumo.json"),"manabu":J("characters/manabu-horikita.json")},
        "evidence":J("evidence/y1-v07.json"),
        "institution-index":J("indexes/institution-map.json"),
        "actor-index":J("indexes/character-topic-map.json"),
    }

class V7InstitutionExamFollowup(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.original=load_data()
    def test_source_bounds_and_global_consistency(self):
        self.assertEqual([],errors(self.original))
    def negative(self,mutate):
        copy_of=copy.deepcopy(self.original)
        mutate(copy_of)
        self.assertNotEqual([],errors(copy_of))
    def test_negative_former_president_still_current(self):
        self.negative(lambda s:s["student-council"]["structure"].update(president="actor.manabu-horikita"))
    def test_negative_forecast_expulsion_as_rule(self):
        self.negative(lambda s:s["student-council"]["y1_v07_succession_and_private_warning"].update(observed_new_rules="MASS_EXPULSIONS_ENACTED"))
    def test_negative_former_president_sees_fight(self):
        self.negative(lambda s:s["student-council"]["y1_v07_succession_and_private_warning"]["rooftop_witness_boundary"].update(did_not_directly_observe_fight=False))
    def test_negative_zodiac_formula(self):
        self.negative(lambda s:s["zodiac"].update(vip_selection_formula="AUTOMATIC_ALPHABETIC_SEQUENCE"))
    def test_negative_false_koenji_atomic_anchor(self):
        self.negative(lambda s:next(x for x in s["zodiac"]["later_revelations"] if x["source_id"]==SRC and x.get("source_locator_pending_atomic_evidence")).update(source_evidence_refs=["y1v7.ev.055"]))
    def test_negative_universal_admission(self):
        self.negative(lambda s:s["anhs"]["y1_v07_school_board_admission_and_retention"]["admissions"].update(exception="All students may enter by board discretion"))
    def test_negative_kiyotaka_hypothesis_as_chairman_rule(self):
        self.negative(lambda s:s["anhs"]["y1_v07_school_board_admission_and_retention"]["admissions"]["class_d_placement_interpretation"].update(status="CONFIRMED_BY_CHAIRMAN"))
    def test_negative_parental_white_room_control(self):
        self.negative(lambda s:s["white_room"]["white_room_model"]["y1_v07_school_board_interface"].update(no_parental_control_inference=False))
    def test_negative_actor_knowledge_leak(self):
        self.negative(lambda s:s["student-council"]["y1_v07_succession_and_private_warning"].update(actor_knowledge_gate="ALL_ACTORS_KNOW"))
    def test_negative_wrong_historical_position(self):
        self.negative(lambda s:s["characters"]["manabu"]["identity"].update(role_summary="Current ANHS student council president"))

if __name__=="__main__":
    unittest.main()
