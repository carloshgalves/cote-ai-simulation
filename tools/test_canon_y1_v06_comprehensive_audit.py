"""Check 138 individual source/adversarial reviews without inventing external signoff.

This test verifies provenance, source anchors, actor ownership, epistemic
fences, 138 distinct counterclaims and mutation-sensitive audit wiring.
Original 211-page PDF can be checked locally with
tools/verify_canon_y1_v06_primary_pdf.py. CI cannot certify semantics without
that private copyrighted source. No human/other-model independence is claimed.
"""
import copy
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
BASE=ROOT/"data/canon/consolidated"
J=lambda path:json.loads(Path(path).read_text(encoding="utf-8"))
anchors=J(BASE/"audits/y1-v06-verbatim-source-anchors.json")
review=J(BASE/"audits/y1-v06-source-and-adversarial-138-evaluations.json")
registry=J(BASE/"audits/source-review-register-y1-v06.json")
coverage=J(BASE/"audits/y1-v06-coverage.json")
source=J(BASE/"sources/y1-v06.json")
claims={e["id"]:e for e in J(BASE/"evidence/y1-v06.json")["records"]}
manifest=J(BASE/"audits/y1-v06-case-manifest.json")["cases"]
case_files={m["id"]:J(ROOT/m["path"]) for m in manifest}
transitions={g["id"]:g for g in J(BASE/"goal_transitions/y1-v06.json")["transitions"]}
audit={x["id"]:x for x in review["individual_evaluations"]}
anchor_by={x["record_id"]:x for x in anchors["records"]}

def problems():
    bad=[]
    if len(anchor_by)!=80 or anchors["record_count"]!=80:bad.append("anchor coverage")
    if len(claims)!=80 or len(case_files)!=43 or len(transitions)!=15:bad.append("source coverage")
    if len(audit)!=138 or len(review["individual_evaluations"])!=138:bad.append("audit identity")
    if review["counts"]!={"evidence":80,"cases":43,"goals":15,"total":138,"independent_external_evaluations":0}:bad.append("false audit numbers")
    if review["reviewer_identity"]!="SAME_ASSISTANT_SOURCE_PASS_PLUS_ADVERSARIAL_PASS_NOT_AN_INDEPENDENT_PEER":bad.append("false reviewer")
    if review["external_independent_reviewer"]!="NOT_PERFORMED":bad.append("fabricated external reviewer")
    if source["pdf_sha256"]!=anchors["pdf_sha256"] or review["pdf_sha256"]!=source["pdf_sha256"]:bad.append("source hash mismatch")
    if registry["release_gate"]["assistant_individual_semantic_review"]!="COMPLETED_ALL_138_PRIMARY_SOURCE_AND_ADVERSARIAL_TWO_METHODS":bad.append("assistant audit incomplete")
    if registry["release_gate"]["external_independent_review"]!="NOT_PERFORMED":bad.append("external reviewer falsely complete")
    if registry["release_gate"]["individual_verified_count"]!=0:bad.append("uncorroborated record verified")
    if any(registry["release_gate"][k] is not False for k in ("engine_memory_admission","exam_executable","goal_runtime_auto_update")):bad.append("premature engine admission")
    if coverage["pending_semantic_items"]!=0 or coverage["pending_independent_review_items"]!=138:bad.append("open gate counts wrong")
    if coverage["independent_review_done"] is not False:bad.append("independent status forged")
    reg_e={x["record_id"]:x for x in registry["evidence"]}
    reg_c={x["case_id"]:x for x in registry["behavioral_cases"]}
    reg_g={x["transition_id"]:x for x in registry["goal_transitions"]}
    if set(reg_e)!=set(claims) or set(reg_c)!=set(case_files) or set(reg_g)!=set(transitions):bad.append("registry record IDs mismatch")
    if set(audit)!=set(claims)|set(case_files)|set(transitions):bad.append("audit record IDs mismatch")
    for record_id,e in claims.items():
        a=anchor_by.get(record_id,{})
        rr=reg_e.get(record_id,{})
        ar=audit.get(record_id,{})
        if not a or a.get("pdf_page") not in e["source"]["pdf_pages"]:bad.append("evidence page anchor "+record_id)
        if a.get("source_sha256")!=source["pdf_sha256"]:bad.append("evidence source identity "+record_id)
        if a.get("source_check")!="VERBATIM_SUBSTRING_MATCH_CONFIRMED_IN_OPERATOR_PDF":bad.append("evidence anchor attestation "+record_id)
        if len(a.get("normalized_source_fragment",""))<3:bad.append("evidence anchor phrase "+record_id)
        if rr.get("verified") is not False or not rr.get("reviewed_by_assistant"):bad.append("evidence status "+record_id)
        if rr.get("primary_pdf_anchor",{}).get("pdf_page")!=a.get("pdf_page"):bad.append("evidence review anchor "+record_id)
        if ar.get("kind")!="evidence" or ar.get("reviewer")!="SAME_ASSISTANT_TWO_METHODS" or ar.get("third_party_signoff") is not False:bad.append("evidence reviewer "+record_id)
        if not ar.get("adversarial_counterclaim"):bad.append("evidence adversarial challenge "+record_id)
    for id,c in case_files.items():
        rr=reg_c.get(id,{})
        ar=audit.get(id,{})
        if not c["evidence_refs"] or not set(c["evidence_refs"])<=set(claims):bad.append("case evidence "+id)
        if c["actor_id"]!=c["primary_actor_id"]:bad.append("wrong primary actor "+id)
        if not any(c["actor_id"] in claims[e]["mention_actor_ids"] for e in c["evidence_refs"]):bad.append("unearned actor attribution "+id)
        if not c["observed_actions"] or not c["observed_result"] or not c["epistemic_limits"]:bad.append("case no deep detail "+id)
        if rr.get("verified") is not False or not rr.get("reviewed_by_assistant"):bad.append("case review status "+id)
        if len(rr.get("primary_pdf_anchor_refs",[]))!=len(c["evidence_refs"]):bad.append("case insufficient primary anchor refs "+id)
        if ar.get("kind")!="behavioral_case" or ar.get("third_party_signoff") is not False or not ar.get("adversarial_counterclaim"):bad.append("case adversarial reviewer "+id)
        if len(ar.get("primary_pdf_anchor_refs",[]))!=len(c["evidence_refs"]):bad.append("case adversarial anchors "+id)
        if any(x["id"] not in anchor_by for x in ar["primary_pdf_anchor_refs"]):bad.append("case false primary refs "+id)
    for id,g in transitions.items():
        rr=reg_g.get(id,{})
        ar=audit.get(id,{})
        if not set(g["evidence_refs"])<=set(claims):bad.append("goal evidence "+id)
        if not (g["prior_goal_or_means"] and g["observed_goal_or_means_after"] and g["trigger_or_new_information"] and g["behavioral_evidence"]):bad.append("goal chronology "+id)
        if not (g["contingent_alternatives"] and g["simulation_guard"]):bad.append("scripted goal path "+id)
        if rr.get("verified") is not False or not rr.get("reviewed_by_assistant"):bad.append("goal review status "+id)
        if len(rr.get("primary_pdf_anchor_refs",[]))!=len(g["evidence_refs"]):bad.append("goal primary refs "+id)
        if ar.get("kind")!="goal_transition" or ar.get("third_party_signoff") is not False or not ar.get("adversarial_counterclaim"):bad.append("goal adversarial reviewer "+id)
        if len(ar.get("primary_pdf_anchor_refs",[]))!=len(g["evidence_refs"]):bad.append("goal adversarial anchors "+id)
    if "deliberadamente" not in claims["y1v6.ev.062"]["summary"] and "conscientemente" not in claims["y1v6.ev.062"]["summary"]:bad.append("juice wrongly treated as accidental")
    if "não prova quem planejou" not in claims["y1v6.ev.062"]["summary"]:bad.append("unproven orchestrator attributed")
    if "Classe A" not in claims["y1v6.ev.079"]["summary"]:bad.append("Ryuen final goal lost")
    return sorted(set(bad))

assert not problems(),problems()

# One deliberately corrupted individual report for each of 138 records.
# Mutation assertions test unique review wiring, not an independent reader.
entries=review["individual_evaluations"]
for item in entries:
    original=item["adversarial_counterclaim"]
    item["adversarial_counterclaim"]=""
    assert problems(),("adversarial record mutation escaped",item["id"])
    item["adversarial_counterclaim"]=original
assert not problems(),problems()

# Eight additional structural attempts to forge completion/temporal semantics.
original=registry["release_gate"]["external_independent_review"]
registry["release_gate"]["external_independent_review"]="COMPLETE"
assert problems()
registry["release_gate"]["external_independent_review"]=original
for path,item,field,mutant in [
    ("evidence",next(iter(registry["evidence"])),"verified",True),
    ("behavioral_cases",next(iter(registry["behavioral_cases"])),"reviewed_by_assistant",False),
    ("goal_transitions",next(iter(registry["goal_transitions"])),"verified",True),
]:
    old=item[field]
    item[field]=mutant
    assert problems()
    item[field]=old
assert not problems(),problems()
print("PASS Y1 V6: 138 per-record source/adversarial assistant reviews and 138 isolated mutations; independent third-party gate remains pending")
