"""Y1 Volume 5: source-gated festival, physical comparisons and case indexes.

Structural tests, not independent literary-semantic signoff or executable simulation approval.
"""
import copy
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
BASE=ROOT/"data/canon/consolidated"
J=lambda p:json.loads(Path(p).read_text(encoding="utf-8"))
source=J(BASE/"sources/y1-v05.json")
records=J(BASE/"evidence/y1-v05.json")["records"]
evidence={e["id"]:e for e in records}
manifest=J(BASE/"audits/y1-v05-case-manifest.json")["cases"]
cases={row["id"]:row for row in (J(ROOT/m["path"]) for m in manifest)}
coverage=J(BASE/"audits/y1-v05-coverage.json")
audit=J(BASE/"audits/source-review-register-y1-v05.json")
index=J(BASE/"indexes/character-topic-map.json")
eindex=J(BASE/"indexes/evidence-topic-map.json")
page=J(BASE/"indexes/y1-v05-page-map.json")
retro=J(BASE/"indexes/retrospective-event-map.json")
fest=J(BASE/"exams/y1-sports-festival.json")
perf=J(BASE/"performance/y1-v05-event-results.json")

assert source["pdf_sha256"]=="0302c25d48f82c9a336b019c4d52045560fdbd91138381a1b24f6a960ae85914"
assert source["pdf_pages"]==207 and source["original_pdf_committed"] is False
assert source["claims_all_verified"] is False
assert len(records)==len(evidence)==coverage["records"]==80
assert len(cases)==len(manifest)==coverage["cases"]==39
assert coverage["chapter_evidence_counts"]=={"1":1,"2":10,"3":5,"4":7,"5":4,"6":21,"7":3,"8":9,"9":20}
assert len(audit["evidence"])==80 and len(audit["behavioral_cases"])==39
assert {x["record_id"] for x in audit["evidence"]}==set(evidence)
assert {x["case_id"] for x in audit["behavioral_cases"]}==set(cases)
assert audit["release_gate"]["external_independent_review"]=="NOT_PERFORMED"
assert audit["release_gate"]["operator_approval"]=="NOT_REQUESTED"
assert audit["release_gate"]["engine_memory_admission"] is False
assert audit["release_gate"]["exam_executable"] is False
assert all(x["verified"] is False for x in audit["evidence"]+audit["behavioral_cases"])
assert fest["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE"
assert "16 votes" in evidence["y1v5.ev.014"]["summary"]
assert "13" in evidence["y1v5.ev.014"]["summary"] and "abstain" in evidence["y1v5.ev.014"]["summary"]
assert fest["scoring"]["team_loss_rule_source_discrepancy"]["state"]=="UNRESOLVED_PRINTED_RULE_VS_CLOSING_NARRATION"
assert fest["scoring"]["team_loss_rule_source_discrepancy"]["printed_rule"].endswith("100 class points")
assert fest["outcome"]["first_year_net_class_points"]["B"]==0
assert any(f["id"]=="Y1V5-ADV-006" and f["state"].startswith("OPEN") for f in audit["findings"])
assert fest["scoring"]["last_individual_event_fallback_exam_penalty"]=={"points":1,"condition":"ONLY_IF_STUDENT_PRIVATE_POINTS_BELOW_1000","source_pdf_page":19}
assert "Last-place fallback is NOT an elective choice" in fest["scoring"]["individual_winner_reward_choice_note"]
assert "ONLY_IF_STUDENT_PRIVATE_POINTS_BELOW_1000" not in evidence["y1v5.ev.006"]["summary"] or "last place" in evidence["y1v5.ev.006"]["summary"].lower()
assert "fewer than 1000 private points" in evidence["y1v5.ev.006"]["summary"]
assert "82.4" in evidence["y1v5.ev.015"]["summary"]
assert "OFF" in evidence["y1v5.ev.063"]["summary"] and "No completed payment" in evidence["y1v5.ev.063"]["summary"]
assert "three paid substitutions" in evidence["y1v5.ev.051"]["summary"]
assert "Ibuki" in evidence["y1v5.ev.077"]["summary"] and "Kinoshita" in evidence["y1v5.ev.077"]["summary"]
assert fest["outcome"]["team_winner"]=="RED"
assert fest["outcome"]["first_year_ranking"]==["B","C","A","D"]
assert fest["outcome"]["first_year_mvp"]=="actor.shibata-so"
assert fest["outcome"]["first_year_net_class_points"]=={"A":-50,"B":0,"C":-100,"D":-100}
assert len(fest["events"])==13 and len(perf["events"])==13
assert sum(len(ev["observations"]) for ev in perf["events"])==35
assert len(perf["measures"])==2
assert "evt.y1.v04.zodiac-vip-selection-algorithm" in retro["unknown_not_resolved"]
assert retro["event_lookup"]["evt.y1.v04.zodiac-dragon-vip-disclosure"]["status"]=="PARTIALLY_EXPLAINED"

for e in records:
    assert e["source"]["work_id"]==source["source_id"]
    assert e["source"]["pdf_pages"] and all(1<=n<=207 for n in e["source"]["pdf_pages"])
    assert 1<=e["source"]["chapter"]<=9
    assert e["assertion_status"]=="ASSISTANT_SOURCE_EXTRACTED_INDIVIDUAL_SEMANTIC_REVIEW_PENDING"
    assert e["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR"
    assert e["related_canonical_event_ids"]==retro["events_by_source_evidence"].get(e["id"],[])
    for n in e["source"]["pdf_pages"]:
        assert e["id"] in page["pages"][str(n)]["evidence"]
    for topic in e["topics"]:
        assert e["id"] in eindex["evidence_by_topic"][topic]
    for actor in e["mention_actor_ids"]:
        assert actor in index["character_packs"],actor
        a=J(ROOT/index["character_packs"][actor]["path"])
        assert source["source_id"] in a["source_coverage"],actor
        for topic in e["topics"]:
            assert e["id"] in index["context_mentions_by_actor_topic"][actor][topic],(actor,topic,e["id"])
            assert e["id"] in a["domains"][topic]["context_evidence_refs"]

for m in manifest:
    c=cases[m["id"]]
    owner="actor."+Path(m["path"]).parent.name
    assert c["actor_id"]==owner==c["primary_actor_id"] and c["id"]==m["id"]
    assert index["behavioral_case_paths"][c["id"]]==m["path"]
    assert c["source"]["work_id"]==source["source_id"]
    assert c["evidence_refs"] and set(c["evidence_refs"])<=set(evidence)
    assert any(owner in evidence[r]["mention_actor_ids"] for r in c["evidence_refs"])
    assert c["epistemic_status"]=="ASSISTANT_SOURCE_EXTRACTED_INDIVIDUAL_SEMANTIC_REVIEW_PENDING"
    assert c["behavioral_policy"]=="DESCRIPTIVE_NOT_PRESCRIPTIVE"
    pack=J(ROOT/index["character_packs"][owner]["path"])
    for t in c["topics"]:
        assert c["id"] in index["behavioral_by_actor_topic"][owner][t]
        assert c["id"] in pack["domains"][t]["behavioral_case_refs"]
    for n in c["source"]["pdf_pages"]:
        assert c["id"] in page["pages"][str(n)]["cases"]

def violations(model):
    v=[]
    measured=model["measures"][0]["observations"]
    ay=next(x for x in measured if x["actor"]=="actor.kiyotaka-ayanokoji")
    sudo=next((x for x in measured if x["actor"]=="actor.ken-sudo"),None)
    if sudo is None or sudo["value"]!=82.4: v.append("Sudou source grip value missing")
    if ay["value"]!=60.6 or ay["source_mode"]!="INTENTIONALLY_SUBMAXIMAL":v.append("false maximum grip")
    if model["comparison_policy"]!="OBSERVED_EVENT_ONLY_NO_UNCONSTRAINED_GENERALIZATION":v.append("unbounded physical rankings")
    race=model["events"][0]["observations"]
    if not any(x.get("actor")=="actor.suzune-horikita" and x["result"]=="AHEAD_OF_IBUKI_BY_NARROW_MARGIN" for x in race):v.append("unearned 100m result")
    relay=model["events"][-1]["observations"]
    if not any(x.get("actor")=="actor.kiyotaka-ayanokoji" and x["result"]=="FAST_ANCHOR_OBSTRUCTED" for x in relay):v.append("clean race ranking invented")
    shibata=model["events"][5]["observations"]
    if not any(x.get("actor")=="actor.shibata-so" and x["result"]=="BEHIND_SUDO" and "Composite" not in x.get("context","") for x in shibata):
        v.append("obstacle event erased or mislabeled")
    if len(model["events"])!=13:v.append("event count")
    return v

# Event-local observations, never automatic combat or speed ratings.
assert not violations(perf),violations(perf)
mutants=[
    ("upgrade submaximal grip",lambda p:next(x for x in p["measures"][0]["observations"] if x["actor"]=="actor.kiyotaka-ayanokoji").update(source_mode="MAXIMUM")),
    ("delete Sudo measured 82.4",lambda p:next(x for x in p["measures"][0]["observations"] if x["actor"]=="actor.ken-sudo").update(value=78.0)),
    ("unbounded physical score",lambda p:p.update(comparison_policy="GLOBAL_RANK_BY_WINS")),
    ("erase Horikita-Ibuki nuance",lambda p:next(x for x in p["events"][0]["observations"] if x.get("actor")=="actor.suzune-horikita").update(result="FASTER_IN_ALL_CONTEXTS")),
    ("forge clear Ayanokoji-Manabu win",lambda p:next(x for x in p["events"][-1]["observations"] if x.get("actor")=="actor.kiyotaka-ayanokoji").update(result="UNCONTESTED_WIN")),
    ("erase obstacle comparison",lambda p:p["events"][5]["observations"].clear()),
    ("delete athletic event",lambda p:p["events"].pop()),
]
for name,change in mutants:
    altered=copy.deepcopy(perf)
    change(altered)
    assert violations(altered),("negative mutation escaped",name)

assert len(index["character_packs"])>=85
assert len(index["behavioral_case_paths"])>=241
print("PASS Year 1 V5 80 evidences, 39 actor cases, 13 events, 35 athletic observations, 6 sports negative controls; release blocked")
