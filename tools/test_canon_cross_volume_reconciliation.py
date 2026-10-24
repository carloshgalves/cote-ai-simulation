"""Cumulative revelations must refine a past event without leaking later knowledge.

This adversarial deterministic check does NOT represent independent human
or separate-model source review.
"""
import copy
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
B=ROOT/"data/canon/consolidated"
J=lambda p:json.loads(Path(p).read_text(encoding="utf-8"))
index=J(B/"indexes/retrospective-event-map.json")
source=J(B/"evidence/y1-v045.json")
source5=J(B/"evidence/y1-v05.json")
reg=J(B/"audits/source-review-register-y1-v045.json")
originals={r["id"] for r in J(B/"evidence/y1-v03.json")["records"]+J(B/"evidence/y1-v04.json")["records"]}
current={r["id"] for r in source["records"]+source5["records"]}
dossiers={key:J(ROOT/row["path"]) for key,row in index["event_lookup"].items()}
exam=J(B/"exams/y1-zodiac-cruise.json")
school=J(B/"institutions/anhs.json")
BASE={"index":index,"sources":current|originals,"current":current,"dossiers":dossiers,"exam":exam,"school":school,"reg":reg}

def problems(s):
    errors=[]
    idx=s["index"];doss=s["dossiers"];src=s["sources"];new=s["current"];exam=s["exam"];reg=s["reg"]
    def require(c,msg):
        if not c: errors.append(msg)
    require(len(doss)==11,"eleven cumulative event dossiers")
    require(set(idx["event_lookup"])==set(doss),"stable IDs present in index")
    for key,d in doss.items():
        require(d["id"]==key,"dossier id "+key)
        require(d["status"] in {"OPEN","PARTIALLY_EXPLAINED","RESOLVED","CONFLICT"},"dossier status "+key)
        require(all(x in src for x in d["canonical_event_sources"]),"prior anchor exists "+key)
        for rel in d["later_revelations"]:
            require(rel["source_id"] in {"ln.y1.v045.sevenseas.en.operator-provided","ln.y1.v05.sevenseas.en.operator-provided"},"true source locator "+key)
            require(all((rid.startswith("y1v45.") if "v045." in rel["source_id"] else rid.startswith("y1v5.")) for rid in rel["evidence_refs"]),"source volume matches evidence "+key)
            require(rel["chapter"] in (range(1,8) if "v045." in rel["source_id"] else range(1,10)) and bool(rel["pdf_pages"]),"chapter and physical locator "+key)
            require(rel["relationship"] in {"SUPPLEMENTS","RESOLVES_AMBIGUITY","CORRECTS_PRIOR_INTERPRETATION","CHALLENGES_WITH_CONFLICT","CONTEXTUALIZES"},"relation "+key)
            require(all(x in new for x in rel["evidence_refs"]),"later evidence belongs to its actual source "+key)
            require(all(item["since"] and item["status"] for item in rel["actor_knowledge"]),"nonempty actor knowledge gate "+key)
            require(rel["no_backdated_global_knowledge"] is True,"no global spoiler leak "+key)
            if rel["relationship"]=="RESOLVES_AMBIGUITY":
                require(bool(rel.get("resolves_assertion_refs")),"explicit resolved prior claim "+key)
            if rel["relationship"]=="CHALLENGES_WITH_CONFLICT":
                require(d["status"]=="CONFLICT","conflict remains open "+key)
    island=doss["evt.y1.v03.island-class-d-leader-replacement"]
    island_revelation=island["later_revelations"][1]
    require(any(x["actor"]=="actor.tachibana" and x["status"]=="HEARD_MANABU_DISCLOSE_SUBSTITUTED_LEADER" for x in island_revelation["actor_knowledge"]),"Tachibana hears island leader disclosure in V4.5")
    require("homeroom teachers" in island_revelation["explanation"],"committee knowledge not shared with ordinary teaching staff")
    pool=doss["evt.y1.v045.pool-invasion-and-suppression"]
    pool_revelation=pool["later_revelations"][0]
    require("y1v45.ev.051" in pool["canonical_event_sources"],"physical object participates in pool history")
    require("y1v45.ev.051" in pool_revelation.get("contextualizes_but_does_not_resolve_assertion_refs",[]),"unnamed card-shaped object only contextualized")
    require("y1v45.ev.051" not in pool_revelation["resolves_assertion_refs"],"do not pretend physical object identity is explicitly resolved")
    v5_trap=doss["evt.y1.v05.class-d-roster-leak-and-counterplay"]
    require(v5_trap["status"]=="PARTIALLY_EXPLAINED","V5 covert operation must remain partial")
    require(any("Ibuki/Kinoshita" in gap for gap in v5_trap["unresolved"]),"V5 anonymous audio name discrepancy not flattened")
    white=doss["evt.y1.v05.sakayanagi-white-room-recognition"]
    require("UNVERIFIED" not in white["event_time"] or white["status"]!="RESOLVED","White Room prior identity is not a proven full chronology")
    require(any(x["actor"]=="actor.arisu-sakayanagi" for rel in white["later_revelations"] for x in rel["actor_knowledge"]),"private Arisu knowledge gate")
    vip=doss["evt.y1.v04.zodiac-vip-surname-order-pattern"]
    require(vip["status"]=="PARTIALLY_EXPLAINED","surname pattern remains actor explanation, not formal certified algorithm")
    require("evt.y1.v04.zodiac-dragon-vip-disclosure" in doss and doss["evt.y1.v04.zodiac-dragon-vip-disclosure"]["status"]=="OPEN","Kushida VIP leak not retroactively proved")
    reverse={}
    for key,d in doss.items():
        for aid in set(d["canonical_event_sources"]+[ref for rel in d["later_revelations"] for ref in rel["evidence_refs"]]):
            reverse.setdefault(aid,set()).add(key)
    for eid,keys in reverse.items():
        require(sorted(keys)==idx["events_by_source_evidence"].get(eid),"bidirectional index "+eid)
    require("evt.y1.v03.ryuen-katsuragi-alliance-contract-terms" in idx["unknown_not_resolved"],"A/C contract remains unknown")
    require("evt.y1.v04.zodiac-vip-selection-algorithm" in idx["unknown_not_resolved"],"VIP algorithm remains unknown")
    require(exam["authority"]=="CANON_REFERENCE_NOT_EXECUTABLE","Zodiac engine not executable")
    require("allocation_formula" not in exam and "vip_selection_formula" not in exam,"no invented VIP formula")
    require(exam["group_assignment_discretion"]["status"]=="TEACHER_REPORTED_SUPPORTED_BY_MASHIMA","teacher vs VIP-selection distinction")
    require("cannot override" in s["school"]["outside_contact_restrictions_y1_v045"]["purchase_boundary"].lower(),"points cannot bypass prohibition")
    require(reg["release_gate"]["engine_memory_admission"] is False and reg["release_gate"]["operator_approval"]=="NOT_REQUESTED","human and memory gates")
    return sorted(set(errors))

assert not problems(BASE),problems(BASE)
mutants=[
 ("erase later source identity",lambda s:s["dossiers"]["evt.y1.v03.island-class-d-leader-replacement"]["later_revelations"][0].update(source_id="ln.y1.v03.sevenseas.en.2019.operator-provided")),
 ("promote a past spoiler",lambda s:s["dossiers"]["evt.y1.v045.pool-invasion-and-suppression"]["later_revelations"][0].update(no_backdated_global_knowledge=False)),
 ("remove resolution trace",lambda s:s["dossiers"]["evt.y1.v045.katsuragi-family-parcel-payment"]["later_revelations"][0].pop("resolves_assertion_refs")),
 ("invent new contract knowledge",lambda s:s["index"]["unknown_not_resolved"].remove("evt.y1.v03.ryuen-katsuragi-alliance-contract-terms")),
 ("claim VIP selection solved",lambda s:s["index"]["unknown_not_resolved"].remove("evt.y1.v04.zodiac-vip-selection-algorithm")),
 ("change school point legality",lambda s:s["school"]["outside_contact_restrictions_y1_v045"].update(purchase_boundary="Points may buy any unlawful act")),
 ("pass unrelated source locator",lambda s:s["dossiers"]["evt.y1.v04.zodiac-dragon-vip-disclosure"]["later_revelations"][0].update(evidence_refs=["y1v4.ev.125"])),
 ("detach event reverse link",lambda s:s["index"]["events_by_source_evidence"]["y1v45.ev.055"].clear()),
 ("fake external memory gate",lambda s:s["reg"]["release_gate"].update(engine_memory_admission=True)),
 ("promote executable VIP exam",lambda s:s["exam"].update(authority="EXECUTABLE")),
 ("forge VIP allocation",lambda s:s["exam"].update(vip_selection_formula="AUTHORITATIVE_DIGIT_SEQUENCE")),
 ("erase Tachibana disclosure",lambda s:s["dossiers"]["evt.y1.v03.island-class-d-leader-replacement"]["later_revelations"][1]["actor_knowledge"].pop()),
 ("promote inferential pool card to explicit proof",lambda s:s["dossiers"]["evt.y1.v045.pool-invasion-and-suppression"]["later_revelations"][0]["resolves_assertion_refs"].append("y1v45.ev.051")),
 ("move V5 report back into V4.5",lambda s:s["dossiers"]["evt.y1.v05.class-d-roster-leak-and-counterplay"]["later_revelations"][0].update(source_id="ln.y1.v045.sevenseas.en.operator-provided")),
 ("erase V5 undisclosed tape discrepancy",lambda s:s["dossiers"]["evt.y1.v05.class-d-roster-leak-and-counterplay"]["unresolved"].clear()),
 ("forge Kushida V4 leak proof",lambda s:s["dossiers"]["evt.y1.v04.zodiac-dragon-vip-disclosure"].update(status="RESOLVED")),
 ("erase Arisu knowledge speaker",lambda s:s["dossiers"]["evt.y1.v05.sakayanagi-white-room-recognition"]["later_revelations"][0]["actor_knowledge"].clear()),

]
for name,alter in mutants:
    mutant=copy.deepcopy(BASE);alter(mutant)
    assert problems(mutant),("negative control escaped",name)
print(f"PASS cumulative cross-volume revelation: {len(dossiers)} dossiers; {len(mutants)} forbidden mutations caught")
