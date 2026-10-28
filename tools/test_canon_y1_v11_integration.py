#!/usr/bin/env python3
"""V11 source-pack consistency and mutation guards; NOT a substitute for independent source review."""
import copy
import json
import unittest
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
B=ROOT/"data/canon/consolidated"
SRC="ln.y1.v11.en.operator-provided"
HASH="cd2c30f42da87a0a19c0666ba87777bc7242adf66a395e5aa9a426b720e7bec0"
J=lambda p:json.loads(Path(p).read_text(encoding="utf-8"))

def load():
    names=[
        "sources/y1-v11.json","evidence/y1-v11.json",
        "audits/y1-v11-case-manifest.json","audits/y1-v11-review-register.json",
        "relationships/y1-v11.json","goal_transitions/y1-v11.json",
        "exams/y1-event-selection.json",
        "biographies/evt.pre-y1.arisu-white-room-observation.json",
        "biographies/evt.pre-y1.hirata-bullied-friend-attempted-suicide.json",
        "indexes/character-topic-map.json","indexes/evidence-topic-map.json",
        "indexes/y1-v11-page-map.json","indexes/relationship-map.json",
        "indexes/goal-transition-map.json","indexes/biography-event-map.json",
        "institutions/anhs.json","institutions/white_room.json"]
    v={n:J(B/n) for n in names}
    ev=v["evidence/y1-v11.json"]
    ix=v["indexes/character-topic-map.json"]
    a={a for r in ev["records"] for a in r["mention_actor_ids"]}
    v["actors"]={a:J(ROOT/ix["character_packs"][a]["path"]) for a in a}
    v["cases"]={r["case_id"]:J(ROOT/ix["behavioral_case_paths"][r["case_id"]])
                for r in v["audits/y1-v11-case-manifest.json"]["cases"]}
    return v

def errors(v):
    bad=[]
    def require(pred,msg):
        if not pred:bad.append(msg)
    src=v["sources/y1-v11.json"]
    e=v["evidence/y1-v11.json"]
    m=v["audits/y1-v11-case-manifest.json"]
    audit=v["audits/y1-v11-review-register.json"]
    rel=v["relationships/y1-v11.json"]
    g=v["goal_transitions/y1-v11.json"]
    ex=v["exams/y1-event-selection.json"]
    bi=v["biographies/evt.pre-y1.arisu-white-room-observation.json"]
    hir=v["biographies/evt.pre-y1.hirata-bullied-friend-attempted-suicide.json"]
    ix=v["indexes/character-topic-map.json"]
    tx=v["indexes/evidence-topic-map.json"]
    px=v["indexes/y1-v11-page-map.json"]
    rx=v["indexes/relationship-map.json"]
    gx=v["indexes/goal-transition-map.json"]
    bx=v["indexes/biography-event-map.json"]
    records={x["id"]:x for x in e["records"]}
    require(src["source_id"]==SRC and src["pdf_sha256"]==HASH and src["pdf_pages"]==476,"source PDF provenance")
    require(not src["original_pdf_committed"] and not src["external_independent_review_completed"],"source gate")
    require(m["case_count"]==56 and m["evidence_count"]==74 and m["context_evidence_count"]==18,"manifest counts")
    require(len(m["cases"])==56 and len(e["records"])==74 and len(records)==74,"evidence/case counts")
    require(len(v["actors"])==26 and len(v["cases"])==56,"actor/case direct index")
    require(len(rel["edges"])==15 and len(g["transitions"])==12,"rel/goal counts")
    require(len(audit["records"])==160 and audit["counts"]["total"]==160,"audit totals")
    require(audit["review_state"]["external_independent_reviewer"]=="NOT_PERFORMED" and audit["review_state"]["engine_memory_admission"] is False,"external review fake")
    require(m["independent_review_status"]=="NOT_PERFORMED" and not m["engine_memory_admission"] and not m["exam_executable"],"manifest promoted")
    for x in e["records"]:
        require(x["source"]["work_id"]==SRC and x["verified"] is False and x["engine_admission"] is False,"evidence promoted")
        require(all(1<=n<=476 for n in x["source"]["pdf_pages"]),"page out of range")
        require(x["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR","mention mistaken for behavior")
        for topic in x["topics"]:require(x["id"] in tx["evidence_by_topic"].get(topic,[]),"missing evidence topic")
        for pg in x["source"]["pdf_pages"]:require(x["id"] in px["pages"].get(str(pg),{}).get("evidence",[]),"missing evidence page")
        for a in x["mention_actor_ids"]:
            p=v["actors"].get(a)
            require(p is not None,"actor pack absent")
            if p is not None:
                require(any(x["id"] in d.get("context_evidence_refs",[]) for d in p["domains"].values()),"actor context missing")
    for r in m["cases"]:
        c=v["cases"].get(r["case_id"])
        require(c is not None,"missing case")
        if not c:continue
        require(c["primary_actor_id"]==r["primary_actor_id"] and c["actor_id"]==r["primary_actor_id"],"wrong primary actor")
        require(c["source"]["work_id"]==SRC and c["source"]["pdf_pages"]==r["pdf_pages"],"case provenance")
        require(c["evidence_refs"]==[r["evidence_id"]],"case evidence mismatch")
        require(c["verified"] is False and c["behavioral_policy"]=="NON_EXECUTABLE","case executable")
        require(c["episode_scope"]==("PRE_ADMISSION_BIOGRAPHICAL_EVIDENCE" if r["candidate_id"] in ["V11-C001","V11-C028"] else "POST_ADMISSION_CANON_BEHAVIOR_ONLY"),"biography / post school boundary")
        p=v["actors"][r["primary_actor_id"]]
        require(any(c["id"] in d.get("behavioral_case_refs",[]) for d in p["domains"].values()),"case not in owner")
        for page in r["pdf_pages"]:require(c["id"] in px["pages"].get(str(page),{}).get("cases",[]),"case not indexed per page")
    for x in rel["edges"]:
        require(x["id"] in rx["edges"] and not x["verified"] and not x["engine_memory_admission"],"relation gate")
        require(all(a in v["actors"] for a in x["actors"]),"invented relation pack")
    require(rel["excluded_unmodeled_relation"]["reason"].startswith("Mii-chan actor pack"),"unmodeled witness lost")
    for z in g["transitions"]:
        require(z["id"] in v["actors"][z["actor_id"]].get("goal_transition_refs",[]),"missing actor goal")
        require(gx["transition_files"].get(z["id"])=="data/canon/consolidated/goal_transitions/y1-v11.json","goal path")
        require(not z["simulation_guard"].startswith("Every simulation"),"future deterministic")
    require(ex["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE" and ex["executable"] is False and ex["engine_memory_admission"] is False,"exam promoted")
    ev=ex["event_proposals"]
    require(ev["proposed_per_class"]==10 and ev["shortlisted_per_class_on_exam_day"]==5 and ev["school_draws"]==7,"selection counts")
    require(ev["number_participants_different_for_own_ten_proposals"] and ev["participants_per_event_min"]==1 and ev["participants_per_event_max"]==20,"participants")
    require(ev["each_student_one_event_until_all_classmates_have_participated"],"participant exhaustion")
    require(ev["tiebreaker_defined_for_every_event"] and ev["school_checks_prior_to_final_submission"],"event preflight")
    cmd=ex["command_runtime_reference"]
    require(cmd["commander_not_event_player"] and cmd["one_headset_receives_instructions_per_class_per_event"],"commander authority")
    require(cmd["text_to_speech_messages_only"] and cmd["require_incoming_vs_played_message_audit"],"command integrity")
    score=ex["scoring"]
    require(score["event_winner_transfer_class_points"]==30 and score["overall_winner_bonus_class_points_from_school"]==100,"points formula")
    require(score["seven_events_always_run_even_when_winner_decided"],"early stop")
    require(ex["commander_penalty"]["protection_point_if_present_consumed_to_prevent_expulsion"],"protection not consumed")
    a=ex["observed_events"]["class_a_vs_c"]
    require(a["event_wins"]=={"CLASS_A":4,"CLASS_C":3} and a["official_winner"]=="CLASS_A","official result")
    require([z["name"] for z in a["known_sequence"]]==["Basketball","Typing","English Test","Math Test","Flash Mental Arithmetic","Archery","Chess"],"event order")
    require(a["class_point_delta"]=={"CLASS_A":130,"CLASS_C":-30},"points official")
    chess=a["chess"]
    require(chess["official_result"]=="CLASS_A_WIN" and chess["official_result"]!=chess["private_rematch"]["winner"],"official/private conflation")
    require(chess["authoritative_message_discrepancy"]["command_changed_before_speech_output"] and chess["authoritative_message_discrepancy"]["channel_integrity_compromised"],"tampering ignored")
    require(chess["private_rematch"]["winner"]=="actor.kiyotaka-ayanokoji" and chess["private_rematch"]["official_exam_overturned"] is False,"private result changes official")
    b=ex["observed_events"]["class_b_vs_d"]
    require(b["official_winner"]=="CLASS_D" and b["reported_class_d_delta"]==190 and b["inferred_wins_from_delta"]=={"CLASS_D":5,"CLASS_B":2},"D/B outcome")
    require(b["fifth_event_karate"]["result"].startswith("CLASS_B_FORFEIT"),"karate loss altered")
    require(bi["initial_known_by"]==["actor.arisu-sakayanagi"] and bi["belief_record"]["assertion_status"]=="CHARACTER_BELIEF_NOT_SCIENTIFIC_PROOF","Arisu background/ideology leak")
    require(bi["baseline_gate"]["not_runtime_episodic_memory"] and not bi["engine_memory_admission"],"biography imported to runtime")
    require(hir["friend_identity"].startswith("Sugimura") and "coma" in hir["friend_condition_at_source_time"],"Hirata past erased")
    require(hir["y1_v11_source_extension"]["does_not_give_every_student_initial_knowledge"],"Hirata backdated knowledge")
    require("evt.pre-y1.arisu-white-room-observation" in bx["biographical_event_lookup"],"biography index absent")
    require(v["institutions/anhs.json"]["y1_v11_event_selection"]["executable"] is False,"institution runtime mutation")
    require(v["institutions/white_room.json"]["y1_v11_arisu_observation"]["one_way_visual_observation"],"two way White Room false")
    return sorted(set(bad))

class V11Regression(unittest.TestCase):
    @classmethod
    def setUpClass(cls):cls.base=load()
    def test_integrity(self):self.assertEqual([],errors(self.base))
    def mutation(self,f):
        d=copy.deepcopy(self.base);f(d)
        self.assertNotEqual([],errors(d))
    def test_negative_fake_external_review(self):self.mutation(lambda x:x["audits/y1-v11-review-register.json"]["review_state"].update(external_independent_reviewer="COMPLETE"))
    def test_negative_source_sha(self):self.mutation(lambda x:x["sources/y1-v11.json"].update(pdf_sha256="fake"))
    def test_negative_fake_actor(self):self.mutation(lambda x:next(iter(x["cases"].values())).update(primary_actor_id="actor.tsukishiro"))
    def test_negative_promote_behavior(self):self.mutation(lambda x:next(iter(x["cases"].values())).update(behavioral_policy="EXECUTABLE"))
    def test_negative_memory_premature(self):self.mutation(lambda x:x["biographies/evt.pre-y1.arisu-white-room-observation.json"]["baseline_gate"].update(not_runtime_episodic_memory=False))
    def test_negative_arisu_science(self):self.mutation(lambda x:x["biographies/evt.pre-y1.arisu-white-room-observation.json"]["belief_record"].update(assertion_status="SCIENTIFIC_PROVEN_FACT"))
    def test_negative_hirata_public(self):self.mutation(lambda x:x["biographies/evt.pre-y1.hirata-bullied-friend-attempted-suicide.json"]["y1_v11_source_extension"].update(does_not_give_every_student_initial_knowledge=False))
    def test_negative_event_count(self):self.mutation(lambda x:x["exams/y1-event-selection.json"]["event_proposals"].update(school_draws=10))
    def test_negative_reuse_participant(self):self.mutation(lambda x:x["exams/y1-event-selection.json"]["event_proposals"].update(each_student_one_event_until_all_classmates_have_participated=False))
    def test_negative_commander_in_event(self):self.mutation(lambda x:x["exams/y1-event-selection.json"]["command_runtime_reference"].update(commander_not_event_player=False))
    def test_negative_skip_unfinished(self):self.mutation(lambda x:x["exams/y1-event-selection.json"]["scoring"].update(seven_events_always_run_even_when_winner_decided=False))
    def test_negative_math_english_swap(self):self.mutation(lambda x:x["exams/y1-event-selection.json"]["observed_events"]["class_a_vs_c"]["known_sequence"][2].update(name="Math Test"))
    def test_negative_change_official_winner(self):self.mutation(lambda x:x["exams/y1-event-selection.json"]["observed_events"]["class_a_vs_c"].update(official_winner="CLASS_C"))
    def test_negative_ignore_tampering(self):self.mutation(lambda x:x["exams/y1-event-selection.json"]["observed_events"]["class_a_vs_c"]["chess"]["authoritative_message_discrepancy"].update(channel_integrity_compromised=False))
    def test_negative_private_changes_official(self):self.mutation(lambda x:x["exams/y1-event-selection.json"]["observed_events"]["class_a_vs_c"]["chess"]["private_rematch"].update(official_exam_overturned=True))
    def test_negative_d_wins_absent(self):self.mutation(lambda x:x["exams/y1-event-selection.json"]["observed_events"]["class_b_vs_d"].update(reported_class_d_delta=-90))
    def test_negative_topic_index(self):self.mutation(lambda x:x["indexes/evidence-topic-map.json"]["evidence_by_topic"]["y1_v11"].clear())
    def test_negative_classroom_knowledge(self):self.mutation(lambda x:x["institutions/white_room.json"]["y1_v11_arisu_observation"].update(one_way_visual_observation=False))
    def test_negative_overstep_exam(self):self.mutation(lambda x:x["exams/y1-event-selection.json"].update(executable=True))

if __name__=="__main__":unittest.main()
