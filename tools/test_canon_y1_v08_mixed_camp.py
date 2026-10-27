#!/usr/bin/env python3
"""Y1 V8 reference-exam gates; not primary-PDF independent literary review."""
import copy
import json
import unittest
from pathlib import Path

B=Path(__file__).resolve().parents[1]/"data/canon/consolidated"
def J(p):return json.loads((B/p).read_text(encoding="utf-8"))
def load():
    return {"pack":J("exams/y1-mixed-training-camp.json"),"source":J("sources/y1-v08.json"),
            "school":J("institutions/anhs.json"),"council":J("institutions/student-council.json"),
            "index":J("indexes/institution-map.json")}
def errs(s):
    e=[];p=s["pack"];source=s["source"]
    def ch(t,m):
        if not t:e.append(m)
    sid="ln.y1.v08.sevenseas.en.operator-provided"
    ch(p["exam_id"]=="exam.y1.mixed-training-camp","exam id changed")
    ch(p["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE" and p["executable"] is False and not p["engine_memory_admission"],"runtime promotion")
    ch(not p["verified"] and p["external_independent_review"]=="NOT_PERFORMED","fake external review")
    ch(source["source_id"]==sid and source["pdf_pages"]==244 and not source["original_pdf_committed"] and not source["claims_all_verified"],"source gate")
    ch(p["source_id"]==source["source_id"],"exam source mismatch")
    ch(p["scope"]["cohorts"]==["YEAR_1","YEAR_2","YEAR_3"] and p["scope"]["duration_days"]==8,"three-year camp")
    ch(p["scope"]["gender_segregated_small_groups"] and p["scope"]["final_large_groups_per_gender"]==6,"gender/groups")
    ch(p["grouping"]["minimum_classes_per_small_group"]==2 and p["grouping"]["formation_requires_unanimous_agreement"],"group safeguards")
    ch(p["grouping"]["final_membership_fixed"] and p["grouping"]["absent_ill_student_not_replaced"],"composition loophole")
    ch(p["grouping"]["caveat"].startswith("Source supplies separate table"),"invented under-60 rubric")
    ch(p["pedagogical_principles"]==["ETHICS","FORTITUDE","DISCIPLINE","INITIATIVE"],"four pillars")
    ch(set(p["final_assessment_parts"])=={"ZAZEN","WRITTEN_TEST","LONG_DISTANCE_RELAY","SPEECH"},"final tests")
    ch(p["final_score_weighting"]=="UNPUBLISHED" and p["scoring_threshold_numeric"]=="UNPUBLISHED","invented published score weights")
    table=p["base_rank_rewards_per_student"]
    ch([(x["private_points"],x["class_points"]) for x in table]==[(10000,3),(5000,1),(3000,0),(-5000,0),(-10000,-3),(-20000,-5)],"rank payoffs")
    mult=p["award_multipliers"]
    ch(mult["apply_only_to_positive_placements"]==[1,2,3] and mult["do_not_multiply_negative_rewards"],"negative rewards multiplied")
    ch(mult["confirmed_max_exemplar"]["private_points_total_for_principal_class"]==1080000 and mult["confirmed_max_exemplar"]["class_points_total_for_principal_class"]==336,"published rounding example")
    rep=p["representative"]
    ch(rep["failure_to_select"]=="ENTIRE_SMALL_GROUP_DISQUALIFIED_AND_ALL_MEMBERS_EXPELLED","missing representative rule")
    ch(rep["representative_expulsion_rule"]["solidarity_requires_school_to_deem_person_contributory"] is True,"solidarity unrestricted")
    ch(rep["representative_expulsion_rule"]["unrestricted_choice"] is False,"free targeting invented")
    ch(rep["additional_class_penalty_on_expulsion"]==-100,"expulsion class point deduction")
    rev=p["revocation_of_expulsion"]
    ch(rev["private_points"]==20000000 and rev["class_points"]==300,"revocation amounts")
    ch(rev["distinct_from_individual_class_transfer"] and rev["does_not_reverse_existing_class_penalties"],"class transfer confused")
    outcome=p["observed_results"]
    ch(outcome["male"]["no_expulsions"] is True and outcome["male"]["large_group_first"]["third_year_representative"]=="Ninomiya Kuranosuke","male results")
    ch("Miyabi Nagumo" in outcome["male"]["large_group_second"]["contains"],"Nagumo actually ranked second")
    ch(outcome["female"]["large_group_first"]["third_year_representative"]=="Ayase Natsu" and outcome["female"]["large_group_last"]["third_year_representative"]=="Ikari Momoko","female ranks")
    ch(outcome["permanence"]=="BOTH_EXPULSIONS_REVOKED_IN_STORY","Tachibana/Ikari falsely expelled")
    ch(outcome["actual_personal_point_ledgers"]=="NOT_IN_SOURCE","unearned bank ledger")
    ch(outcome["male"]["exact_scores"]=="NOT_DISCLOSED","fabricated overall ranking score")
    ch("Specific advance payment of 20 million from Nagumo to Ishikura" in p["adversarial_episode"]["actor_inferences_not_elevated_to_official_truth"],"unverified payment certified")
    ch(p["school_constraints"]["phones_collected"] and p["school_constraints"]["meal_cross_gender_contact_minutes"]==60,"cross-gender info rules")
    an=s["school"];sc=s["council"];ix=s["index"]
    ch(p["exam_id"] in an["domains"]["exams"]["reference_exam_ids"],"school exam ref")
    ch(sid in an["source_coverage"] and sid in sc["source_coverage"],"institution source missing")
    ch(ix["institutions"]["org.anhs"]["source_volume_count"]==len(set(an["source_coverage"])),"institution index stale ANHS")
    ch(ix["institutions"]["org.student-council"]["source_volume_count"]==len(set(sc["source_coverage"])),"institution index stale student council")
    ch(sc["y1_v08_interyear_political_exam_context"]["exam_influence"].startswith("ATTRIBUTED"),"council legal control fabricated")
    return e

class MixedCampGates(unittest.TestCase):
    @classmethod
    def setUpClass(cls):cls.base=load()
    def test_reference_integrity(self):self.assertEqual([],errs(self.base))
    def neg(self,mut):
        obj=copy.deepcopy(self.base)
        mut(obj);self.assertNotEqual([],errs(obj))
    def test_negative_missing_year(self):self.neg(lambda s:s["pack"]["scope"].update(cohorts=["YEAR_1","YEAR_2"]))
    def test_negative_same_class_team(self):self.neg(lambda s:s["pack"]["grouping"].update(minimum_classes_per_small_group=1))
    def test_negative_unknown_weights_made_exact(self):self.neg(lambda s:s["pack"].update(final_score_weighting={"ZAZEN":25}))
    def test_negative_deduction_multiplier(self):self.neg(lambda s:s["pack"]["award_multipliers"].update(do_not_multiply_negative_rewards=False))
    def test_negative_any_solidarity_target(self):self.neg(lambda s:s["pack"]["representative"]["representative_expulsion_rule"].update(unrestricted_choice=True))
    def test_negative_banned_revocation(self):self.neg(lambda s:s["pack"]["revocation_of_expulsion"].update(private_points=0))
    def test_negative_nagumo_declared_first(self):self.neg(lambda s:s["pack"]["observed_results"]["male"]["large_group_first"].update(third_year_representative="Miyabi Nagumo"))
    def test_negative_permanent_tachibana_expulsion(self):self.neg(lambda s:s["pack"]["observed_results"].update(permanence="TACHIBANA_EXPELLED"))
    def test_negative_fabricated_score(self):self.neg(lambda s:s["pack"]["observed_results"]["male"].update(exact_scores={"NINOMIYA":91}))
    def test_negative_bank_ledger(self):self.neg(lambda s:s["pack"]["observed_results"].update(actual_personal_point_ledgers={"Nagumo":20000000}))
    def test_negative_not_draft(self):self.neg(lambda s:s["pack"].update(executable=True))
    def test_negative_student_council_became_authority(self):self.neg(lambda s:s["council"]["y1_v08_interyear_political_exam_context"].update(exam_influence="LEGAL_UNLIMITED_POWER"))

if __name__=="__main__":unittest.main()
