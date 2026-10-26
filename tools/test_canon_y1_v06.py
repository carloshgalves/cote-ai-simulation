"""Year 1 Volume 6: paper-shuffle canon source and actor/goal safety regressions.

Covers structural integrity plus negative controls. This DOES NOT constitute
independent semantic validation of each literary scene or engine release.
"""
import copy
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
B=ROOT/"data/canon/consolidated"
J=lambda p:json.loads(Path(p).read_text(encoding="utf-8"))
source=J(B/"sources/y1-v06.json")
evidence=J(B/"evidence/y1-v06.json")["records"]
ev={e["id"]:e for e in evidence}
manifest=J(B/"audits/y1-v06-case-manifest.json")["cases"]
cases={m["id"]:J(ROOT/m["path"]) for m in manifest}
changes=J(B/"goal_transitions/y1-v06.json")
audit=J(B/"audits/source-review-register-y1-v06.json")
coverage=J(B/"audits/y1-v06-coverage.json")
exam=J(B/"exams/y1-paper-shuffle.json")
card=J(B/"assessments/y1-v06-illustrated-school-cards.json")
characters=J(B/"indexes/character-topic-map.json")
topics=J(B/"indexes/evidence-topic-map.json")
pages=J(B/"indexes/y1-v06-page-map.json")
goals=J(B/"indexes/goal-transition-map.json")
retro=J(B/"indexes/retrospective-event-map.json")
council=J(B/"institutions/student-council.json")
anhs=J(B/"institutions/anhs.json")
SRC="ln.y1.v06.sevenseas.en.operator-provided"
SHA="e85ebfe1efb68282f8067768b8d81b289492c295720d157f77afdd4e10ee706b"
assert source["source_id"]==SRC and source["pdf_sha256"]==SHA and source["pdf_pages"]==211
assert source["original_pdf_committed"] is False and source["claims_all_verified"] is False
assert source["analysis_report"]["candidate_records"]==80
assert len(evidence)==len(ev)==80 and len(cases)==len(manifest)==43
assert len(changes["transitions"])==15
assert coverage["records"]==80 and coverage["cases"]==43 and coverage["goal_transitions"]==15
assert coverage["chapter_evidence_counts"]=={"1":2,"2":7,"3":12,"4":5,"5":26,"6":12,"7":16}
assert len(audit["evidence"])==80 and len(audit["behavioral_cases"])==43 and len(audit["goal_transitions"])==15
assert {x["record_id"] for x in audit["evidence"]}==set(ev)
assert {x["case_id"] for x in audit["behavioral_cases"]}==set(cases)
assert {x["transition_id"] for x in audit["goal_transitions"]}=={g["id"] for g in changes["transitions"]}
assert sum(x["direct_pdf_spotchecked"] for x in audit["evidence"])==41
assert all(x["verified"] is False for x in audit["evidence"]+audit["behavioral_cases"]+audit["goal_transitions"])
assert audit["release_gate"]["external_independent_review"]=="NOT_PERFORMED"
assert audit["release_gate"]["engine_memory_admission"] is False
assert audit["release_gate"]["exam_executable"] is False
assert audit["release_gate"]["goal_runtime_auto_update"] is False
assert len(characters["character_packs"])==90
assert len(characters["behavioral_case_paths"])==284

for i in range(1,81):
    id=f"y1v6.ev.{i:03d}"
    e=ev[id]
    assert e["source_catalog_id"]==f"V6-E{i:03d}"
    assert e["source"]["work_id"]==SRC
    assert all(isinstance(p,int) and 10<=p<=209 for p in e["source"]["pdf_pages"])
    assert 1<=e["source"]["chapter"]<=7
    assert e["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR"
    assert e["related_canonical_event_ids"]==retro["events_by_source_evidence"].get(id,[])
    assert e["assertion_status"]=="REPORT_EXTRACTED_SOURCE_SEMANTIC_REVIEW_PENDING"
    for p in e["source"]["pdf_pages"]:
        assert id in pages["pages"][str(p)]["evidence"]
    for t in e["topics"]:
        assert id in topics["evidence_by_topic"][t]
        for actor in e["mention_actor_ids"]:
            assert actor in characters["character_packs"],actor
            assert id in characters["context_mentions_by_actor_topic"][actor][t],(actor,t,id)
            pack=J(ROOT/characters["character_packs"][actor]["path"])
            assert SRC in pack["source_coverage"]
            assert id in pack["domains"][t]["context_evidence_refs"]

for m in manifest:
    c=cases[m["id"]]
    actor=c["actor_id"]
    assert c["id"]==m["id"] and actor==c["primary_actor_id"]
    assert actor=="actor."+Path(m["path"]).parent.name
    assert characters["behavioral_case_paths"][c["id"]]==m["path"]
    assert c["source"]["work_id"]==SRC
    assert c["evidence_refs"] and set(c["evidence_refs"])<=set(ev)
    assert any(actor in ev[ref]["mention_actor_ids"] for ref in c["evidence_refs"])
    assert c["source"]["pdf_pages"]==m["pdf_pages"]
    assert c["behavioral_policy"]=="SOURCE_DESCRIPTION_NOT_PRESCRIPTIVE_OR_RUNTIME_ACTION"
    pack=J(ROOT/characters["character_packs"][actor]["path"])
    for t in c["topics"]:
        assert c["id"] in pack["domains"][t]["behavioral_case_refs"]
        assert c["id"] in characters["behavioral_by_actor_topic"][actor][t]
    for p in c["source"]["pdf_pages"]:
        assert c["id"] in pages["pages"][str(p)]["cases"]

assert changes["authority"]=="HISTORICAL_EXAMPLES_NOT_EXECUTABLE_GOAL_POLICY"
assert SRC in goals["source_to_transitions"] and len(goals["source_to_transitions"][SRC])==15
for g in changes["transitions"]:
    assert g["source"]["work_id"]==SRC
    assert set(g["evidence_refs"])<=set(ev)
    assert any(g["actor_id"] in ev[r]["mention_actor_ids"] for r in g["evidence_refs"])
    assert g["contingent_alternatives"] and g["simulation_guard"]
    assert g["id"] in goals["by_actor"][g["actor_id"]]
    assert goals["transition_files"][g["id"]]=="data/canon/consolidated/goal_transitions/y1-v06.json"
    pack=J(ROOT/characters["character_packs"][g["actor_id"]]["path"])
    assert g["id"] in pack["goal_transition_refs"]
    for ref in g["evidence_refs"]:
        assert g["id"] in goals["by_evidence"][ref]

rules=exam["rules"]
assert exam["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE"
assert rules["diagnostic_quiz"]["questions"]==100
assert rules["diagnostic_quiz"]["grade_credit"] is False
assert rules["diagnostic_quiz"]["pairing"]=="HIGHEST_WITH_LOWEST_CONTINUING_INWARD"
assert rules["diagnostic_quiz"]["tie_break"]=="RANDOM"
assert [g["score_target"] for g in rules["diagnostic_quiz"]["horikita_four_group_targets"]]==["0","1","MAXIMUM_80","AT_LEAST_85"]
assert rules["main_exam"]["subjects"]==8
assert rules["main_exam"]["per_subject_pair_minimum"]==60
assert rules["main_exam"]["combined_total_minimum"]==692
assert rules["main_exam"]["earlier_total_estimate_about"]==700
assert rules["main_exam"]["earlier_estimate_not_final"] is True
assert len(exam["paired_students"])==7
assert exam["end_of_volume_outcomes"]["final_class_results"]=="NOT_PUBLISHED"
assert exam["end_of_volume_outcomes"]["final_individual_scores"]=="NOT_PUBLISHED"
assert exam["end_of_volume_outcomes"]["math_wager"]["official_math_scores"] is None
assert exam["end_of_volume_outcomes"]["math_wager"]["horikita_score"]=="SELF_ASSESSED_AT_LEAST_98"
assert exam["end_of_volume_outcomes"]["math_wager"]["kushida_score"]=="SELF_ASSESSED_BELOW_80"
assert exam["end_of_volume_outcomes"]["math_wager"]["witness_privacy"]=="MANABU_DID_NOT_HEAR_DETAILED_JUNIOR_HIGH_CONFESSION"
assert exam["executable"] is False and exam["engine_memory_admission"] is False
assert len(exam["strategy_layers"])==5
assert exam["strategy_layers"][0]["id"]=="horikita"
assert exam["strategy_layers"][2]["id"]=="x_transaction"

assert card["numeric_conversion"]=="FORBIDDEN" and card["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE"
card_by={x["actor_id"]:x for x in card["cards"]}
assert set(card_by)=={"actor.yukimura","actor.maya-sato"}
assert card_by["actor.yukimura"]["attributes"]=={"academic":"A","intelligence":"A-","decision_making":"C","physical":"D-","cooperativeness":"D-"}
assert card_by["actor.maya-sato"]["attributes"]=={"academic":"D-","intelligence":"D-","decision_making":"D-","physical":"C","cooperativeness":"C-"}
assert anhs["paper_shuffle_y1_v06"]["combined_total_minimum"]==692
assert all(p["enacted"]=="NOT_CONFIRMED" for p in council["presidency_y1_v06"]["proposals"])
assert len(retro["event_lookup"])==18
assert retro["event_lookup"]["evt.y1.v06.unknown-x-transaction-and-karuizawa-photo"]["status"]=="PARTIALLY_EXPLAINED"
assert "evt.y1.v04.zodiac-vip-selection-algorithm" in retro["unknown_not_resolved"]

def forbidden(x):
    out=[]
    e=x["exam"];a=x["audit"];d=x["cards"];co=x["council"];r=x["retro"];g=x["changes"];f=x["source"];evs=x["ev"]
    if e["rules"]["main_exam"]["combined_total_minimum"]!=692:out.append("false 700 final threshold")
    if e["rules"]["diagnostic_quiz"]["pairing"]!="HIGHEST_WITH_LOWEST_CONTINUING_INWARD":out.append("pairing falsified")
    if [z["score_target"] for z in e["rules"]["diagnostic_quiz"]["horikita_four_group_targets"]]!=["0","1","MAXIMUM_80","AT_LEAST_85"]:out.append("four score bands lost")
    if e["end_of_volume_outcomes"]["final_class_results"]!="NOT_PUBLISHED":out.append("invented class result")
    if e["end_of_volume_outcomes"]["math_wager"]["official_math_scores"] is not None:out.append("invented certified math score")
    if e["end_of_volume_outcomes"]["math_wager"]["witness_privacy"]!="MANABU_DID_NOT_HEAR_DETAILED_JUNIOR_HIGH_CONFESSION":out.append("Manabu hears forbidden secret")
    if e["strategy_layers"][0]["id"]!="horikita":out.append("Suzune independent plan misattributed")
    if d["numeric_conversion"]!="FORBIDDEN":out.append("ordinal school cards numeric")
    if any(q["enacted"]!="NOT_CONFIRMED" for q in co["presidency_y1_v06"]["proposals"]):out.append("speech enacted without school authority")
    if r["event_lookup"]["evt.y1.v06.unknown-x-transaction-and-karuizawa-photo"]["status"]!="PARTIALLY_EXPLAINED":out.append("X identity prematurely solved")
    if "evt.y1.v04.zodiac-vip-selection-algorithm" not in r["unknown_not_resolved"]:out.append("invented VIP exam formula")
    if g["authority"]!="HISTORICAL_EXAMPLES_NOT_EXECUTABLE_GOAL_POLICY":out.append("goals automatic")
    if a["release_gate"]["engine_memory_admission"] is not False:out.append("engine leaked")
    if a["release_gate"]["external_independent_review"]!="NOT_PERFORMED":out.append("independent signoff invented")
    if any(q["verified"] is not False for q in a["evidence"]+a["behavioral_cases"]+a["goal_transitions"]):out.append("individual verification forged")
    if not f["analysis_report"]["sha256"]=="b637e9625e3287dd53a6c3f6bbfe5a4071751bec6fca3cf8b9be6f227223d320":out.append("report provenance changed")
    if not evs["y1v6.ev.078"]["summary"].find("Transaction")>=0:out.append("transaction lost")
    return out

baseline={"exam":exam,"audit":audit,"cards":card,"council":council,"retro":retro,"changes":changes,"source":source,"ev":ev}
assert forbidden(baseline)==[]
mutations=[
("promote about-700 estimate",lambda b:b["exam"]["rules"]["main_exam"].update(combined_total_minimum=700)),
("pair students by similar scores",lambda b:b["exam"]["rules"]["diagnostic_quiz"].update(pairing="NEAREST_SCORES")),
("erase zero/one/eighty/eightyfive design",lambda b:b["exam"]["rules"]["diagnostic_quiz"]["horikita_four_group_targets"][3].update(score_target="AT_LEAST_70")),
("forge final class victory",lambda b:b["exam"]["end_of_volume_outcomes"].update(final_class_results="CLASS_D_WON")),
("forge Horikita certified mathematics score",lambda b:b["exam"]["end_of_volume_outcomes"]["math_wager"].update(official_math_scores={"horikita":98})),
("give Manabu Kushida complete secret",lambda b:b["exam"]["end_of_volume_outcomes"]["math_wager"].update(witness_privacy="MANABU_HEARD_FULL_CONFESSION")),
("erase Horikita plan credit",lambda b:b["exam"]["strategy_layers"][0].update(id="x_transaction")),
("make grades scalar",lambda b:b["cards"].update(numeric_conversion="CONVERT_TO_IQ_SCORES")),
("pretend Nagumo speech enacted",lambda b:b["council"]["presidency_y1_v06"]["proposals"][0].update(enacted="YES")),
("pretend Ryuen knows X",lambda b:b["retro"]["event_lookup"]["evt.y1.v06.unknown-x-transaction-and-karuizawa-photo"].update(status="RESOLVED")),
("fake VIP selection algorithm",lambda b:b["retro"]["unknown_not_resolved"].remove("evt.y1.v04.zodiac-vip-selection-algorithm")),
("execute historical arc",lambda b:b["changes"].update(authority="RUN_AUTOMATIC_AGENT_GOAL_UPDATE")),
("allow unreviewed engine memory",lambda b:b["audit"]["release_gate"].update(engine_memory_admission=True)),
("fake independent review",lambda b:b["audit"]["release_gate"].update(external_independent_review="COMPLETE")),
("fake verified source record",lambda b:b["audit"]["evidence"][0].update(verified=True)),
("break report SHA",lambda b:b["source"]["analysis_report"].update(sha256="INVALID")),
("erase X's transactional demand",lambda b:b["ev"]["y1v6.ev.078"].update(summary="Ordinary visit without transaction"))
]
for name,change in mutations:
    b=copy.deepcopy(baseline);change(b)
    assert forbidden(b),("negative control escaped",name)

print(f"PASS Y1 V6: 80 evidence, 43 actor cases, 15 contingent goals, 18 event dossiers, 17 negative mutations; promotion blocked")
