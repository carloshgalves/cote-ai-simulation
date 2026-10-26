"""Y1 Volume 6 source-centric semantic regressions and hostile mutations.

Checks source-supported identity, goal persistence, exam uncertainty and
superseded V5 details. Neither this test nor its assistant review is an
independent literary peer review or approval for executable memory.
"""
from pathlib import Path
import copy
import json

ROOT=Path(__file__).resolve().parents[1]
BASE=ROOT/"data/canon/consolidated"
J=lambda x:json.loads(Path(x).read_text(encoding="utf-8"))

actors={name:J(BASE/f"characters/{name}.json") for name in ["nagumo","hasebe","miyake","yukimura"]}
schoolcards=J(BASE/"assessments/y1-v06-illustrated-school-cards.json")
v6={r["id"]:r for r in J(BASE/"evidence/y1-v06.json")["records"]}
v5={r["id"]:r for r in J(BASE/"evidence/y1-v05.json")["records"]}
v5exam=J(BASE/"exams/y1-sports-festival.json")
v6exam=J(BASE/"exams/y1-paper-shuffle.json")
goals=J(BASE/"goal_transitions/y1-v06.json")
cases=J(BASE/"audits/y1-v06-case-manifest.json")
case_ryuen=J(BASE/"behavioral_cases/kakeru-ryuen/choose-false-kushida-answers-after-x-transaction.json")
audit=J(BASE/"audits/source-review-register-y1-v06.json")
source=J(BASE/"sources/y1-v06.json")
council=J(BASE/"institutions/student-council.json")
events=J(BASE/"indexes/retrospective-event-map.json")

dataset={"actors":actors,"schoolcards":schoolcards,"v6":v6,"v5":v5,"v5exam":v5exam,"v6exam":v6exam,"goals":goals,"cases":cases,"case_ryuen":case_ryuen,"audit":audit,"source":source,"council":council,"events":events}

def violations(x):
    err=[]
    a=x["actors"];b=x["schoolcards"];e=x["v6"];older=x["v5"];ex5=x["v5exam"];ex=x["v6exam"];g=x["goals"];re=x["audit"]
    expected={
      "nagumo":("actor.nagumo","Miyabi Nagumo",["Nagumo Miyabi"]),
      "hasebe":("actor.hasebe","Haruka Hasebe",["Hasebe Haruka"]),
      "miyake":("actor.miyake","Akito Miyake",["Miyake Akito"]),
      "yukimura":("actor.yukimura","Teruhiko Yukimura (preferred name: Keisei)",["Keisei","Yukimura Teruhiko"])
    }
    for who,(stable,display,aliases) in expected.items():
        o=a[who]
        if o["character_id"]!=stable or o["display_name"]!=display:err.append("actor naming identity "+who)
        if not all(name in o["identity"]["aliases"] for name in aliases):err.append("actor aliases "+who)
        if not o["identity"].get("temporal_updates"):err.append("actor temporal source anchor "+who)
    if a["nagumo"]["uncertainties"]["given_name_unknown_in_source"] is not False:err.append("Nagumo name still unknown")
    if a["nagumo"]["identity"]["temporal_updates"][0]["office"]!="STUDENT_COUNCIL_PRESIDENT":err.append("Nagumo publicly installed role")
    if a["yukimura"]["identity"]["temporal_updates"][0]["preferred_given_name"]!="Keisei":err.append("Yukimura name conflation")
    dates={c["actor_id"]:c["date_of_birth_month_day"] for c in b["cards"]}
    if dates!={"actor.yukimura":"07-11","actor.maya-sato":"02-28"}:err.append("school-card birthday falsified")
    if b["numeric_conversion"]!="FORBIDDEN":err.append("subjective school grades to numeric AI scores")
    if not all(t["enacted"]=="NOT_CONFIRMED" for t in x["council"]["presidency_y1_v06"]["proposals"]):err.append("promised rule treated as enacted")
    if "Miyabi" not in e["y1v6.ev.003"]["summary"]:err.append("Miyabi missing in source evidence")
    if not all(n in e["y1v6.ev.056"]["summary"] for n in ["Haruka","Akito","Teruhiko"]):err.append("friend-name disclosure missing")
    if "Keisei" not in e["y1v6.ev.058"]["summary"] or "nome oficial" not in e["y1v6.ev.058"]["summary"]:err.append("Keisei nickname made legal name")
    ry=e["y1v6.ev.079"]["summary"]
    if "objetivo de longo prazo" not in ry or "Classe A" not in ry or "continua inalterado" not in ry:err.append("Ryuen long-term Class A goal erased")
    if "X permanece anônimo" not in ry:err.append("Ryuen falsely knows X")
    ryGoals=[y for y in g["transitions"] if y["actor_id"]=="actor.kakeru-ryuen"]
    if len(ryGoals)!=2 or any("goal to reach Class A is explicitly unchanged" not in y["simulation_guard"] for y in ryGoals):err.append("local tactical change becomes permanent goal replacement")
    if "enduring ambition to reach Class A" not in x["case_ryuen"]["observed_result"]:err.append("Ryuen case loses goal")
    if "16 votes" not in older["y1v5.ev.014"]["summary"] or "13" not in older["y1v5.ev.014"]["summary"]:err.append("old PR vote lost")
    if ex5["scoring"]["team_loss_rule_source_discrepancy"]["state"]!="UNRESOLVED_PRINTED_RULE_VS_CLOSING_NARRATION":err.append("old PR unresolved source contradiction falsely reconciled")
    if ex5["outcome"]["first_year_net_class_points"]["B"]!=0:err.append("narrated Class B score replaced by inferred rule")
    if ex["rules"]["main_exam"]["combined_total_minimum"]!=692:err.append("estimated instead of final minimum")
    if ex["end_of_volume_outcomes"]["final_class_results"]!="NOT_PUBLISHED":err.append("final exam winner invented")
    if ex["end_of_volume_outcomes"]["math_wager"]["official_math_scores"] is not None:err.append("unpublished math scores invented")
    if ex["strategy_layers"][0]["id"]!="horikita" or ex["strategy_layers"][0]["authority"]!="SCHOOL_DIALOGUE_CONFIRMED":err.append("Horikita counterplan credited to X")
    if ex["end_of_volume_outcomes"]["math_wager"]["witness_privacy"]!="MANABU_DID_NOT_HEAR_DETAILED_JUNIOR_HIGH_CONFESSION":err.append("Manabu secret leak")
    if x["events"]["event_lookup"]["evt.y1.v06.unknown-x-transaction-and-karuizawa-photo"]["status"]!="PARTIALLY_EXPLAINED":err.append("X secret prematurely resolved")
    if source["claims_all_verified"] is not False or source["original_pdf_committed"] is not False:err.append("false source complete")
    if re["release_gate"]["external_independent_review"]!="NOT_PERFORMED":err.append("independent peer review fraud")
    if any(x["verified"] for k in ["evidence","behavioral_cases","goal_transitions"] for x in re[k]):err.append("source individual verification fraud")
    if re["release_gate"]["engine_memory_admission"] is not False or re["release_gate"]["goal_runtime_auto_update"] is not False:err.append("runtime promotion without gate")
    if len(re["evidence"])!=80 or len(re["behavioral_cases"])!=43 or len(re["goal_transitions"])!=15:err.append("source audit coverage lost")
    if len(x["cases"]["cases"])!=43:err.append("actor cases missing")
    return sorted(set(err))

assert not violations(dataset),violations(dataset)
mutations=[
("rename Nagumo again to unknown",lambda a:a["actors"]["nagumo"].update(display_name="Nagumo (given name unknown)")),
("wrong named actor",lambda a:a["actors"]["miyake"].update(character_id="actor.akito")),
("remove Hasebe real given name",lambda a:a["actors"]["hasebe"]["identity"].update(aliases=[])),
("defer student council president identity",lambda a:a["actors"]["nagumo"]["identity"]["temporal_updates"][0].update(office="VICE_PRESIDENT")),
("turn Teruhiko into unknown",lambda a:a["actors"]["yukimura"]["identity"]["temporal_updates"][0].update(preferred_given_name="Teruhiko")),
("swap birthdays",lambda a:a["schoolcards"]["cards"][0].update(date_of_birth_month_day="02-28")),
("convert subjective grade to IQ",lambda a:a["schoolcards"].update(numeric_conversion="0_TO_100_IQ")),
("deem council proposal implemented",lambda a:a["council"]["presidency_y1_v06"]["proposals"][0].update(enacted="YES")),
("erase true Miyabi source",lambda a:a["v6"]["y1v6.ev.003"].update(summary="Nagumo assumes presidency")),
("erase first-name social disclosure",lambda a:a["v6"]["y1v6.ev.056"].update(summary="A group was formed")),
("misstate preferred alias legal",lambda a:a["v6"]["y1v6.ev.058"].update(summary="Keisei is his original legal given name")),
("erase Ryuen persistent long goal",lambda a:a["v6"]["y1v6.ev.079"].update(summary="Ryuuen abandons Class A ambition")),
("claim Ryuen knows X",lambda a:a["v6"]["y1v6.ev.079"].update(summary="Ryuen knows who X is")),
("force temporary plan into permanent goal",lambda a:next(t for t in a["goals"]["transitions"] if t["actor_id"]=="actor.kakeru-ryuen").update(simulation_guard="Class A no longer matters")),
("erase Ryuen case persistence",lambda a:a["case_ryuen"].update(observed_result="Ryuen hates Suzune")),
("erase 16–13 majority",lambda a:a["v5"]["y1v5.ev.014"].update(summary="A class vote happened")),
("invent festival exception to printed scoring",lambda a:a["v5exam"]["scoring"]["team_loss_rule_source_discrepancy"].update(state="FULLY_EXPLAINED")),
("forge festival B net",lambda a:a["v5exam"]["outcome"]["first_year_net_class_points"].update(B=-50)),
("fabricate 700 cutoff",lambda a:a["v6exam"]["rules"]["main_exam"].update(combined_total_minimum=700)),
("forge school result",lambda a:a["v6exam"]["end_of_volume_outcomes"].update(final_class_results="CLASS_D_WON")),
("forge official math score",lambda a:a["v6exam"]["end_of_volume_outcomes"]["math_wager"].update(official_math_scores={"suzune":98})),
("erase Horikita's independently authorized counterplan",lambda a:a["v6exam"]["strategy_layers"][0].update(id="x_transaction")),
("give Manabu Kushida's private secret",lambda a:a["v6exam"]["end_of_volume_outcomes"]["math_wager"].update(witness_privacy="MANABU_KNOWS_JUNIOR_HIGH_SECRETS")),
("pretend X publicly identified",lambda a:a["events"]["event_lookup"]["evt.y1.v06.unknown-x-transaction-and-karuizawa-photo"].update(status="RESOLVED")),
("false independent review",lambda a:a["audit"]["release_gate"].update(external_independent_review="COMPLETE")),
("enable unreviewed agent goals",lambda a:a["audit"]["release_gate"].update(goal_runtime_auto_update=True)),
("pretend all records verified",lambda a:a["audit"]["evidence"][0].update(verified=True)),
("erase case coverage",lambda a:a["cases"]["cases"].pop())
]
for name,change in mutations:
    altered=copy.deepcopy(dataset)
    change(altered)
    assert violations(altered),("negative mutation escaped",name)

print(f"PASS Volume 5 closure + Volume 6 final semantic gates: {len(mutations)} adversarial mutations rejected, source & engine gates preserved")
