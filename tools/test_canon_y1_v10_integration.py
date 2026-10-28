#!/usr/bin/env python3
"""Y1 Volume 10 source-pack contracts. Deterministic checks; NOT independent literary review."""
import copy
import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
B = ROOT / "data/canon/consolidated"
SRC = "ln.y1.v10.en.operator-provided"
DIGEST = "88831ad45cd7e0fdae3e5e1bdd4f288570ed4cef5bf807f196239fb4ae549fa7"
J = lambda p: json.loads(p.read_text(encoding="utf-8"))

def load():
    files = [
        "sources/y1-v10.json", "evidence/y1-v10.json",
        "audits/y1-v10-case-manifest.json", "audits/y1-v10-review-register.json",
        "relationships/y1-v10.json", "goal_transitions/y1-v10.json",
        "exams/y1-class-poll.json",
        "biographies/evt.pre-y1.hirata-bullied-friend-attempted-suicide.json",
        "indexes/character-topic-map.json", "indexes/evidence-topic-map.json",
        "indexes/y1-v10-page-map.json", "indexes/relationship-map.json",
        "indexes/goal-transition-map.json", "indexes/biography-event-map.json",
        "institutions/anhs.json", "institutions/student-council.json",
        "institutions/white_room.json",
    ]
    s = {x:J(B/x) for x in files}
    ci = s["indexes/character-topic-map.json"]
    m = s["audits/y1-v10-case-manifest.json"]
    e = s["evidence/y1-v10.json"]
    actors = {a for x in e["records"] for a in x["mention_actor_ids"]}
    s["actors"] = {a:J(ROOT/ci["character_packs"][a]["path"]) for a in actors}
    s["cases"] = {x["case_id"]:J(ROOT/ci["behavioral_case_paths"][x["case_id"]]) for x in m["cases"]}
    return s

def violations(s):
    bad = []
    def ck(c, m):
        if not c: bad.append(m)
    src = s["sources/y1-v10.json"]
    ev = s["evidence/y1-v10.json"]
    m = s["audits/y1-v10-case-manifest.json"]
    audit = s["audits/y1-v10-review-register.json"]
    rel = s["relationships/y1-v10.json"]
    goals = s["goal_transitions/y1-v10.json"]
    exam = s["exams/y1-class-poll.json"]
    bio = s["biographies/evt.pre-y1.hirata-bullied-friend-attempted-suicide.json"]
    ci = s["indexes/character-topic-map.json"]
    ei = s["indexes/evidence-topic-map.json"]
    pi = s["indexes/y1-v10-page-map.json"]
    ri = s["indexes/relationship-map.json"]
    gi = s["indexes/goal-transition-map.json"]
    bi = s["indexes/biography-event-map.json"]
    em = {x["id"]:x for x in ev["records"]}
    ck(src["source_id"] == SRC and src["pdf_pages"] == 376 and src["pdf_sha256"] == DIGEST, "source identity")
    ck(not src["claims_all_verified"] and not src["original_pdf_committed"], "source release gate")
    ck(len(ev["records"]) == 68 and len(em) == 68 and len(m["cases"]) == 45, "record counts")
    ck(len(rel["edges"]) == 16 and len(goals["transitions"]) == 11, "relation/goal counts")
    ck(audit["counts"]["total"] == 141 and len(audit["records"]) == 141, "audit item count")
    ck(audit["review_state"]["external_independent_reviewer"] == "NOT_PERFORMED" and
       audit["review_state"]["engine_memory_admission"] is False, "forged external review")
    ck(m["independent_review_status"] == "NOT_PERFORMED" and
       m["engine_memory_admission"] is False and m["exam_executable"] is False, "manifest gate")
    for e in ev["records"]:
        ck(e["source"]["work_id"] == SRC and e["verified"] is False and
           e["engine_admission"] is False, "evidence admitted")
        ck(e["subject_assertion"] == "MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR", "mention became feat")
        ck(all(1 <= n <= 376 for n in e["source"]["pdf_pages"]), "source page")
        for t in e["topics"]:
            ck(e["id"] in ei["evidence_by_topic"].get(t, []), "evidence topic index")
        for n in e["source"]["pdf_pages"]:
            ck(e["id"] in pi["pages"].get(str(n),{}).get("evidence",[]), "evidence page index")
        for a in e["mention_actor_ids"]:
            ck(a in s["actors"], "missing actor")
            ck(any(e["id"] in d.get("context_evidence_refs",[]) for d in s["actors"][a]["domains"].values()), "actor evidence not linked")
    for x in m["cases"]:
        c = s["cases"].get(x["case_id"])
        if not c: ck(False, "missing case"); continue
        ck(c["primary_actor_id"] == x["primary_actor_id"] and c["actor_id"] == x["primary_actor_id"], "case misattributed")
        ck(c["source"]["work_id"] == SRC and c["source"]["pdf_pages"] == x["pdf_pages"], "case source")
        ck(c["evidence_refs"] == [x["evidence_id"]], "case evidence")
        ck(c["verified"] is False and c["behavioral_policy"] == "NON_EXECUTABLE", "case executable")
        ck(c["episode_scope"] == ("PRE_ADMISSION_BIOGRAPHICAL_EVIDENCE" if x["candidate_id"]=="V10-C001"
             else "POST_ADMISSION_CANON_BEHAVIOR_ONLY"), "wrong biographical boundary")
        ck(x["case_id"] in ci["behavioral_case_paths"], "direct case index")
        a = x["primary_actor_id"]
        ck(any(x["case_id"] in d.get("behavioral_case_refs",[]) for d in s["actors"][a]["domains"].values()), "case not in owner pack")
        for n in x["pdf_pages"]:
            ck(x["case_id"] in pi["pages"].get(str(n),{}).get("cases",[]), "case page index")
    for x in rel["edges"]:
        ck(x["verified"] is False and not x["engine_memory_admission"], "relationship executable")
        ck(x["id"] in ri["edges"], "relationship direct index")
    for g in goals["transitions"]:
        a=g["actor_id"]
        ck(g["id"] in s["actors"][a].get("goal_transition_refs",[]), "actor goal link")
        ck(gi["transition_files"].get(g["id"]) == "data/canon/consolidated/goal_transitions/y1-v10.json", "goal source index")
        ck(not g["simulation_guard"].startswith("Every simulation"), "canon goal forced")
    ck(exam["authority"] == "CANON_REFERENCE_NOT_EXECUTABLE" and exam["executable"] is False and not exam["engine_memory_admission"], "exam execution leak")
    voters = exam["electorate"]
    ck(voters["internal_praise_votes_per_student"] == 3 and voters["internal_censure_votes_per_student"] == 3, "internal ballot counts")
    ck(voters["additional_cross_class_praise_votes_per_student"] == 1 and exam["cross_class_authority"]["praise_only"], "external negative vote invented")
    ck(voters["self_vote_prohibited"] and voters["duplicate_vote_for_same_person_prohibited"] and voters["abstention_or_blank_vote_prohibited"], "illegal ballot accepted")
    ck(exam["net_votes"]["formula"] == "TOTAL_PRAISE_INCLUDING_CROSS_CLASS_MINUS_INTERNAL_CENSURE", "vote calculation")
    ck(exam["net_votes"]["all_net_zero_force_revote"] and exam["net_votes"]["tie_implementation"] == "UNKNOWN_NOT_EXECUTABLE", "invented tiebreak")
    reward=exam["rewards_and_penalties"]
    ck(reward["protection_point"]["nontransferable"] and reward["optional_expulsion_revocation"]["private_points_cost"] == 20000000, "protection or revocation rule")
    ck(reward["class_point_change_caused_by_exam"] == 0, "invented class-point penalty")
    results = exam["observed_official_results"]
    ck(results["expulsions_by_class"] == {"A":"actor.yahiko","B":None,"C":"actor.haruki-yamauchi","D":"actor.manabe-shiho"}, "official expulsions")
    ck(results["protection_point_winners_by_class"] == {"A":"actor.arisu-sakayanagi","B":"actor.honami-ichinose","C":"actor.kiyotaka-ayanokoji","D":"actor.kaneda"}, "official protection winners")
    ck(results["private_point_transfer_details"]["ichinose_nagumo_loan_executed"] is False, "forced loan or romance")
    ck(exam["cross_class_authority"]["privately_reported_original_negative_vote_proposal"]["enacted"] is False, "nonenacted negative-vote proposal enacted")
    ck(bio["event_time"].startswith("BEFORE_Y1_START") and bio["baseline_gate"]["actor_initial_background"] == ["actor.yosuke-hirata"], "biography chronology")
    ck(bio["baseline_gate"]["not_runtime_episodic_memory"] and not bio["engine_memory_admission"], "biography mistaken for sim episode")
    ck("evt.pre-y1.hirata-bullied-friend-attempted-suicide" in bi["biographical_event_lookup"], "biography index")
    ck(s["actors"]["actor.yosuke-hirata"]["y1_v10_pre_admission_biography"]["not_public_at_entry"], "Hirata private background leak")
    ck(s["actors"]["actor.tsukishiro"]["y1_v10_intervention_boundaries"]["camera_tamper_allegation_is_self_report"], "camera forensic forgery")
    ck(s["institutions/anhs.json"]["y1_v10_class_poll"]["executable"] is False, "institution exam auto-patch")
    return sorted(set(bad))

class TestClassPollV10(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.baseline = load()
    def test_full_integrity(self):
        self.assertEqual([], violations(self.baseline))
    def mutation(self, callback):
        v=copy.deepcopy(self.baseline)
        callback(v)
        self.assertNotEqual([], violations(v))
    def test_negative_wrong_pdf(self):
        self.mutation(lambda s:s["sources/y1-v10.json"].update(pdf_pages=375))
    def test_negative_fabricated_review(self):
        self.mutation(lambda s:s["audits/y1-v10-review-register.json"]["review_state"].update(external_independent_reviewer="COMPLETE"))
    def test_negative_source_auto_verified(self):
        self.mutation(lambda s:s["evidence/y1-v10.json"]["records"][0].update(verified=True))
    def test_negative_wrong_actor(self):
        self.mutation(lambda s:next(iter(s["cases"].values())).update(primary_actor_id="actor.nagumo"))
    def test_negative_lost_index(self):
        self.mutation(lambda s:s["indexes/character-topic-map.json"]["behavioral_case_paths"].pop(next(iter(s["cases"]))))
    def test_negative_hirata_public_knowledge(self):
        self.mutation(lambda s:s["actors"]["actor.yosuke-hirata"]["y1_v10_pre_admission_biography"].update(not_public_at_entry=False))
    def test_negative_sim_memory_seed(self):
        self.mutation(lambda s:s["biographies/evt.pre-y1.hirata-bullied-friend-attempted-suicide.json"]["baseline_gate"].update(not_runtime_episodic_memory=False))
    def test_negative_external_censure(self):
        self.mutation(lambda s:s["exams/y1-class-poll.json"]["cross_class_authority"].update(praise_only=False))
    def test_negative_unrevealed_tiebreak(self):
        self.mutation(lambda s:s["exams/y1-class-poll.json"]["net_votes"].update(tie_implementation="COIN_FLIP"))
    def test_negative_nonmandatory_vote(self):
        self.mutation(lambda s:s["exams/y1-class-poll.json"]["electorate"].update(abstention_or_blank_vote_prohibited=False))
    def test_negative_class_b_expulsion(self):
        self.mutation(lambda s:s["exams/y1-class-poll.json"]["observed_official_results"]["expulsions_by_class"].update(B="actor.ryuji-kanzaki"))
    def test_negative_fabricated_nagumo_deal(self):
        self.mutation(lambda s:s["exams/y1-class-poll.json"]["observed_official_results"]["private_point_transfer_details"].update(ichinose_nagumo_loan_executed=True))
    def test_negative_fabricated_government_rule(self):
        self.mutation(lambda s:s["exams/y1-class-poll.json"]["cross_class_authority"]["privately_reported_original_negative_vote_proposal"].update(enacted=True))
    def test_negative_not_actual_cameras(self):
        self.mutation(lambda s:s["actors"]["actor.tsukishiro"]["y1_v10_intervention_boundaries"].update(camera_tamper_allegation_is_self_report=False))
    def test_negative_fictional_result_engine(self):
        self.mutation(lambda s:s["exams/y1-class-poll.json"].update(executable=True))

if __name__=="__main__":
    unittest.main()
