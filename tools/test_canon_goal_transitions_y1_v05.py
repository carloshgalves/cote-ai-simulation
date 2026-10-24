"""Validate evidence-grounded, non-deterministic actor goal transitions (Y1 V5).

Structural and source-boundary checks are not an independent semantic review.
"""
import copy
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
BASE=ROOT/"data/canon/consolidated"
J=lambda p:json.loads(Path(p).read_text(encoding="utf-8"))
goals=J(BASE/"goal_transitions/y1-v05.json")
idx=J(BASE/"indexes/goal-transition-map.json")
source=J(BASE/"sources/y1-v05.json")
evidence={x["id"]:x for x in J(BASE/"evidence/y1-v05.json")["records"]}
actor_idx=J(BASE/"indexes/character-topic-map.json")
review=J(BASE/"audits/source-review-register-y1-v05.json")

assert goals["authority"]=="HISTORICAL_EXAMPLES_NOT_EXECUTABLE_GOAL_POLICY"
assert idx["guard"]=="RETRIEVAL_ONLY_NOT_AUTOMATIC_RUNTIME_GOAL_UPDATE"
assert goals["source_id"]==source["source_id"]
assert len(goals["transitions"])==10
assert len({t["id"] for t in goals["transitions"]})==10
assert len(idx["by_actor"])==5
assert review["release_gate"]["external_independent_review"]=="NOT_PERFORMED"
assert review["release_gate"]["operator_approval"]=="NOT_REQUESTED"
assert review["release_gate"]["engine_memory_admission"] is False
assert review["release_gate"]["exam_executable"] is False
assert all(x["verified"] is False for x in review["evidence"]+review["behavioral_cases"])

def errors(dataset):
    out=[]
    trans=dataset["transitions"]
    by_id={t["id"]:t for t in trans}
    if len(by_id)!=10:out.append("IDs not unique or lost")
    for t in trans:
        if not t["prior_goal_or_means"] or not t["observed_goal_or_means_after"]:out.append("no before/after goal")
        if not t["trigger_or_new_information"] or not t["behavioral_evidence"]:out.append("no grounded event")
        if t["source"]["work_id"]!=source["source_id"]:out.append("wrong source")
        if not t["evidence_refs"] or not set(t["evidence_refs"])<=set(evidence):out.append("orphan goal evidence")
        if not t["contingent_alternatives"] or not t["simulation_guard"]:out.append("forced fixed path")
        if not t["event_time_ref"]:out.append("missing event time")
        if t["actor_id"] not in actor_idx["character_packs"]:out.append("untracked actor")
    expected={a:sorted([t["id"] for t in trans if t["actor_id"]==a]) for a in idx["by_actor"]}
    if expected!=idx["by_actor"]:out.append("stale actor goal index")
    ay=by_id["y1v5.goal.kiyotaka-ayanokoji.allow-local-risk-for-horikita-learning"]
    if "not a proved intention" not in ay["simulation_guard"].lower() and "never assert" not in ay["simulation_guard"].lower():out.append("miscast full class loss as designed")
    if "purposefully make class finish last" in ay["observed_goal_or_means_after"].lower():out.append("invented forced loss")
    sudo=by_id["y1v5.goal.ken-sudo.romantic-interest-shapes-mvp-motivation"]
    if "Never convert" not in sudo["simulation_guard"] or "reciprocation" not in sudo["simulation_guard"]:out.append("unearned romance")
    ho=by_id["y1v5.goal.suzune-horikita.individual-excellence-to-reciprocal-alliance"]
    if "Class A" not in ho["observed_goal_or_means_after"] or "Manabu" not in ho["observed_goal_or_means_after"]:out.append("false change of distant Horikita aim")
    kushida=by_id["y1v5.goal.kikyo-kushida.expulsion-prioritized-over-class-rank"]
    if "prioritize expelling Suzune" not in kushida["observed_goal_or_means_after"]:out.append("lost secret priority")
    ryuen=by_id["y1v5.goal.kakeru-ryuen.halt-coercion-and-investigate-anonymous-opponent"]
    if "unidentified" not in ryuen["behavioral_evidence"]:out.append("Ryuen falsely knows sender")
    if "Kiyotaka sent the audio" in ryuen["behavioral_evidence"]:out.append("sender identity fabricated")
    if dataset["future_divergence_gate"].startswith("Every simulation must repeat"):out.append("story arc predetermined")
    return sorted(set(out))

assert not errors(goals),errors(goals)
for actor,ids in idx["by_actor"].items():
    pack=J(ROOT/actor_idx["character_packs"][actor]["path"])
    assert set(pack["goal_transition_refs"])==set(ids)
    assert pack["goal_transition_model"]=="EVIDENCE_REFERENCE_ONLY_NOT_RUNTIME_UPDATE"
for evid,ids in idx["by_evidence"].items():
    assert evid in evidence
    assert set(ids)=={t["id"] for t in goals["transitions"] if evid in t["evidence_refs"]}
assert {x for ids in idx["by_actor"].values() for x in ids}==set(idx["transition_files"])
assert set(idx["source_to_transitions"][source["source_id"]])=={t["id"] for t in goals["transitions"]}

negative=[
 ("erase causal trigger",lambda v:v["transitions"][0].update(trigger_or_new_information="")),
 ("make universal future path",lambda v:v.update(future_divergence_gate="Every simulation must repeat the novel events")),
 ("fabricate Ryuen knowing sender",lambda v:next(t for t in v["transitions"] if t["id"].endswith("halt-coercion-and-investigate-anonymous-opponent")).update(behavioral_evidence="Kiyotaka sent the audio")),
 ("turn local risk into forced last place",lambda v:next(t for t in v["transitions"] if t["id"].endswith("allow-local-risk-for-horikita-learning")).update(observed_goal_or_means_after="purposefully make class finish last")),
 ("delete romance epistemic boundary",lambda v:next(t for t in v["transitions"] if t["id"].endswith("romantic-interest-shapes-mvp-motivation")).update(simulation_guard="True love")),
 ("declare Suzune abandoned distant goal",lambda v:next(t for t in v["transitions"] if t["id"].endswith("individual-excellence-to-reciprocal-alliance")).update(observed_goal_or_means_after="No longer cares about Manabu or the Class A goal")),
 ("silence Kushida secret priority",lambda v:next(t for t in v["transitions"] if t["id"].endswith("expulsion-prioritized-over-class-rank")).update(observed_goal_or_means_after="Always prioritize getting to Class A")),
 ("remove source proof",lambda v:v["transitions"][1].update(evidence_refs=[]))
]
for name,change in negative:
    modified=copy.deepcopy(goals)
    change(modified)
    assert errors(modified),("negative control escaped",name)
print(f"PASS Y1 V5: {len(goals['transitions'])} contingent goal transitions, {len(negative)} adversarial mutations rejected; no runtime promotion")
