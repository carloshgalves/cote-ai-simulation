"""Y1 Volume 5: direct-source semantic adversarial controls.

These are deterministic controls based on the operator's source PDF. This run
is NOT an independent peer or separate-model literary-semantic review.
"""
import copy
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
BASE=ROOT/"data/canon/consolidated"
J=lambda p:json.loads(Path(p).read_text(encoding="utf-8"))

E={x["id"]:x for x in J(BASE/"evidence/y1-v05.json")["records"]}
C={x["id"]:x for x in (J(ROOT/x["path"]) for x in J(BASE/"audits/y1-v05-case-manifest.json")["cases"])}
X=J(BASE/"exams/y1-sports-festival.json")
P=J(BASE/"performance/y1-v05-event-results.json")
A=J(BASE/"audits/source-review-register-y1-v05.json")
G=J(BASE/"goal_transitions/y1-v05.json")
MAP=J(BASE/"indexes/retrospective-event-map.json")
S=J(BASE/"sources/y1-v05.json")
REVIEW=J(BASE/"audits/y1-v05-semantic-adversarial-review-2026-10-25.json")
BASELINE={"e":E,"c":C,"exam":X,"perf":P,"audit":A,"goal":G,"events":MAP,"src":S,"review":REVIEW}

def violations(bundle):
    issues=[]
    evid=bundle["e"];cases=bundle["c"];exam=bundle["exam"];perf=bundle["perf"]
    review=bundle["audit"];goals=bundle["goal"];events=bundle["events"];source=bundle["src"]
    m=perf["measures"][0]["observations"]
    grip={x["actor"]:x for x in m}
    if grip.get("actor.ken-sudo",{}).get("value")!=82.4:
        issues.append("unfaithful Sudo grip")
    ay=grip.get("actor.kiyotaka-ayanokoji",{})
    if ay.get("value")!=60.6 or ay.get("source_mode")!="INTENTIONALLY_SUBMAXIMAL":
        issues.append("Ayanokoji submaximal treated as actual maximum")
    fallback=exam["scoring"].get("last_individual_event_fallback_exam_penalty",{})
    if fallback!={"points":1,"condition":"ONLY_IF_STUDENT_PRIVATE_POINTS_BELOW_1000","source_pdf_page":19}:
        issues.append("last-place exam fallback not conditional")
    if "Last-place fallback is NOT an elective choice" not in exam["scoring"].get("individual_winner_reward_choice_note",""):
        issues.append("last-place falsely given elective choice")
    if "fewer than 1000 private points" not in evid["y1v5.ev.006"]["summary"]:
        issues.append("literary last-place rule lost")
    if "82.4" not in evid["y1v5.ev.015"]["summary"]:
        issues.append("Sudo comparison source erased")
    payment=evid["y1v5.ev.063"]["summary"]
    if "OFFER" not in payment or "No completed payment is shown" not in payment or "accepted payment" in payment:
        issues.append("invented Kinoshita payment")
    if "three paid substitutions" not in evid["y1v5.ev.051"]["summary"]:
        issues.append("Hirata cost and participant funding erased")
    if evid["y1v5.ev.007"]["source"]["pdf_pages"]!=[21,22,23]:
        issues.append("festival enumerated event page missing")
    if 203 not in evid["y1v5.ev.067"]["source"]["pdf_pages"]:
        issues.append("White Room utterance source unlinked")
    if 203 not in cases["y1v5.case.arisu-sakayanagi.reveal-white-room-knowledge-in-private"]["source"]["pdf_pages"]:
        issues.append("Arisu case misses source term")
    tape=evid["y1v5.ev.077"]["summary"]
    if not all(t in tape for t in ("Ibuki","Kinoshita","does not identify the sender")):
        issues.append("source-quoted tape mismatch or anonymous attribution erased")
    if "begins kneeling" not in evid["y1v5.ev.076"]["summary"]:
        issues.append("forced kneeling treated as complete")
    if "Kushida explicitly admits" not in evid["y1v5.ev.074"]["summary"]:
        issues.append("direct Kushida confession denied")
    if evid["y1v5.ev.074"]["source_mode"]!="DIRECT_CHARACTER_ADMISSION":
        issues.append("private admission attribution altered")
    if events["event_lookup"]["evt.y1.v04.zodiac-dragon-vip-disclosure"]["status"]!="PARTIALLY_EXPLAINED":
        issues.append("VIP selection declared either unknown or fully certified")
    if "evt.y1.v04.zodiac-vip-selection-algorithm" not in events["unknown_not_resolved"]:
        issues.append("official VIP selection formula invented")
    boy=next(x for x in perf["events"] if x["code"]=="tug_of_war")
    if boy["observations"][0]["result"]!="RED_A_D_BOYS_WIN_AFTER_THREE_ROUNDS":
        issues.append("boys tug result rewritten")
    if exam["outcome"]["first_year_ranking"]!=["B","C","A","D"]:
        issues.append("festival official ranking forged")
    if goals["authority"]!="HISTORICAL_EXAMPLES_NOT_EXECUTABLE_GOAL_POLICY":
        issues.append("historical goals made executable")
    if goals["future_divergence_gate"].startswith("Every simulation"):
        issues.append("mandatory canon script for all simulated futures")
    if review["release_gate"]["engine_memory_admission"] is not False or review["release_gate"]["exam_executable"] is not False:
        issues.append("unauthorized engine promotion")
    if review["release_gate"]["external_independent_review"]!="NOT_PERFORMED":
        issues.append("invented independent review")
    if not all(x["verified"] is False for x in review["evidence"]+review["behavioral_cases"]):
        issues.append("invented independent record verification")
    if not all(x["reviewed_by_assistant"] is True for x in review["evidence"]+review["behavioral_cases"]):
        issues.append("assistant source review missing")
    if len(review["evidence"])!=80 or len(review["behavioral_cases"])!=39 or len(review["goal_transitions"])!=10:
        issues.append("source review coverage incomplete")
    conflicting=exam["scoring"].get("team_loss_rule_source_discrepancy",{})
    if conflicting.get("state")!="UNRESOLVED_PRINTED_RULE_VS_CLOSING_NARRATION":issues.append("fabricated scoring reconciliation")
    if "16 votes" not in evid["y1v5.ev.014"]["summary"] or "13" not in evid["y1v5.ev.014"]["summary"]:issues.append("vote counts erased")
    details={
      "y1v5.ev.021":["ties legs","adapting tempo"],
      "y1v5.ev.028":["catches the fist","palm"],
      "y1v5.ev.035":["height order","deciding round"],
      "y1v5.ev.049":["third draw","last in his heat"],
      "y1v5.ev.057":["Manabu finishes ahead","obstructs"]
    }
    for item,terms in details.items():
        if not all(term in evid[item]["summary"] for term in terms):issues.append("lost event-local V5 source "+item)
    if source["claims_all_verified"] is not False:
        issues.append("unproven global canon verification")
    if bundle["review"]["review_actor"]!="ASSISTANT_DIRECT_SOURCE_AND_ADVERSARIAL_NO_INDEPENDENT_REVIEW":
        issues.append("reviewer origin mislabeled")
    return sorted(set(issues))

assert not violations(BASELINE),violations(BASELINE)
def mutant_case(n):
    b=copy.deepcopy(BASELINE)
    e=b["e"]; p=b["perf"]; ex=b["exam"]; a=b["audit"]; g=b["goal"]
    cases={
    0:lambda: ex["scoring"]["last_individual_event_fallback_exam_penalty"].update(condition="UNCONDITIONAL"),
    1:lambda: ex["scoring"].update(individual_winner_reward_choice_note="Everyone may choose test points or cash"),
    2:lambda: p["measures"][0]["observations"].__setitem__(next(i for i,x in enumerate(p["measures"][0]["observations"]) if x["actor"]=="actor.ken-sudo"),{"actor":"actor.ken-sudo","value":50.0}),
    3:lambda: next(x for x in p["measures"][0]["observations"] if x["actor"]=="actor.kiyotaka-ayanokoji").update(source_mode="MAXIMUM"),
    4:lambda: e["y1v5.ev.063"].update(summary="Kinoshita accepted payment and confirmed medical proof"),
    5:lambda: e["y1v5.ev.051"].update(summary="Kushida is substitute; no costs reported"),
    6:lambda: e["y1v5.ev.007"]["source"].update(pdf_pages=[20,23]),
    7:lambda: e["y1v5.ev.067"]["source"].update(pdf_pages=[202,204]),
    8:lambda: b["c"]["y1v5.case.arisu-sakayanagi.reveal-white-room-knowledge-in-private"]["source"].update(pdf_pages=[202,204]),
    9:lambda: e["y1v5.ev.077"].update(summary="Ayanokoji is confirmed by Ryuen as the sender of the tape"),
    10:lambda: e["y1v5.ev.076"].update(summary="Horikita completes the kneeling and pays the requested amount"),
    11:lambda: e["y1v5.ev.074"].update(source_mode="UNCONFIRMED_HEARSAY"),
    12:lambda: b["events"]["event_lookup"]["evt.y1.v04.zodiac-dragon-vip-disclosure"].update(status="RESOLVED"),
    13:lambda: b["events"]["unknown_not_resolved"].remove("evt.y1.v04.zodiac-vip-selection-algorithm"),
    14:lambda: next(x for x in p["events"] if x["code"]=="tug_of_war")["observations"][0].update(result="WHITE_BOYS_WON"),
    15:lambda: ex["outcome"].update(first_year_ranking=["D","A","B","C"]),
    16:lambda: g.update(authority="EXECUTABLE_AUTO_GOAL_POLICY"),
    17:lambda: g.update(future_divergence_gate="Every simulation must match each historical goal arc"),
    18:lambda: a["release_gate"].update(engine_memory_admission=True),
    19:lambda: a["release_gate"].update(external_independent_review="COMPLETE"),
    20:lambda: a["evidence"][0].update(verified=True),
    21:lambda: a["behavioral_cases"][0].update(reviewed_by_assistant=False),
    22:lambda: a["evidence"].pop(),
    23:lambda: b["src"].update(claims_all_verified=True),
    24:lambda: b["review"].update(review_actor="INDEPENDENT_PEER_VERIFIED"),
    25:lambda: b["exam"]["scoring"]["team_loss_rule_source_discrepancy"].update(state="RECONCILED_BY_NEW_EXCEPTIONS"),
    26:lambda: b["e"]["y1v5.ev.014"].update(summary="Majority vote held with no reported tally"),
    27:lambda: b["e"]["y1v5.ev.021"].update(summary="All athletes fully coordinated"),
    28:lambda: b["e"]["y1v5.ev.028"].update(summary="A prolonged clean fight establishes Sudou superiority"),
    29:lambda: b["e"]["y1v5.ev.035"].update(summary="The tug winner is not identified"),
    30:lambda: b["e"]["y1v5.ev.049"].update(summary="Ayanokoji definitely won the scavenger race"),
    31:lambda: b["e"]["y1v5.ev.057"].update(summary="Ayanokoji won a clean and timed sprint"),
    }
    cases[n]()
    return b

for i in range(32):
    assert violations(mutant_case(i)),("negative mutation escaped",i)
print("PASS Y1 V5 direct-source adversarial: 129 source/actor/goal audit items, 32 negative mutations rejected, no fabricated peer approval")
