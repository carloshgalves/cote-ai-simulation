"""Prepare/validate a *separate* Y1 V6 independent literary review.

This does NOT solicit an evaluator or confer independence. The reviewer must
actually have authorized access to the original operator PDF and must supply
their own evidence-based verdict on each of the 138 canon records.

Examples (working copy outside git):
  python tools/canon_y1_v06_external_review_packet.py prepare --head "$(git rev-parse HEAD)" --output /tmp/y1-v06-review.json
  python tools/canon_y1_v06_external_review_packet.py validate --head HEAD_SHA --input /tmp/y1-v06-review.json

Release promotion always requires manual reviewer provenance verification and
an explicit operator gate after any required corrections/re-review.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
DATA=ROOT/"data/canon/consolidated"
SOURCE_ID="ln.y1.v06.sevenseas.en.operator-provided"
DIGEST="e85ebfe1efb68282f8067768b8d81b289492c295720d157f77afdd4e10ee706b"
SHA_RE=re.compile(r"^[0-9a-f]{40}$")
VERDICTS={"APPROVED","FINDING","UNRESOLVED"}

def read(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))

def schema_records():
    evidence=read(DATA/"evidence/y1-v06.json")["records"]
    manifest=read(DATA/"audits/y1-v06-case-manifest.json")["cases"]
    goals=read(DATA/"goal_transitions/y1-v06.json")["transitions"]
    anchors={x["record_id"]:x for x in read(DATA/"audits/y1-v06-verbatim-source-anchors.json")["records"]}
    rows=[]
    for e in evidence:
        a=anchors[e["id"]]
        rows.append({"id":e["id"],"kind":"evidence","actor_id":None,
                     "pdf_pages":e["source"]["pdf_pages"],"evidence_refs":[e["id"]],
                     "source_anchor":{"pdf_page":a["pdf_page"],"short_fragment":a["normalized_source_fragment"]},
                     "claim":e["summary"],"epistemic_status":e["source_mode"]})
    for c in manifest:
        details=read(ROOT/c["path"])
        rows.append({"id":details["id"],"kind":"behavioral_case","actor_id":details["actor_id"],
                     "pdf_pages":details["source"]["pdf_pages"],
                     "evidence_refs":details["evidence_refs"],
                     "claim":details["observed_actions"]+" / "+details["observed_result"],
                     "epistemic_status":details["epistemic_limits"]})
    for g in goals:
        rows.append({"id":g["id"],"kind":"goal_transition","actor_id":g["actor_id"],
                     "pdf_pages":g["source"]["pdf_pages"],"evidence_refs":g["evidence_refs"],
                     "claim":g["prior_goal_or_means"]+" -> "+g["observed_goal_or_means_after"],
                     "epistemic_status":g["simulation_guard"]})
    if len(rows)!=138 or len({r["id"] for r in rows})!=138:
        raise ValueError("Expected 138 distinct claims: 80 evidence, 43 cases, 15 goals")
    return rows

def row_fingerprint(row):
    return hashlib.sha256(json.dumps(row,sort_keys=True,ensure_ascii=False,
          separators=(",",":")).encode("utf-8")).hexdigest()

def prepare(head):
    if not SHA_RE.fullmatch(head):raise ValueError("head must be an exact Git commit SHA")
    src=schema_records()
    records=[]
    for x in src:
        records.append({"record":x,"record_fingerprint":row_fingerprint(x),
                        "review":{"verdict":"PENDING","source_pdf_read":False,
                                  "pages_checked":[],"evidence_refs_checked":[],
                                  "short_source_excerpt":"","reasoning":"",
                                  "finding_severity":None,"finding_description":"",
                                  "linked_github_review_url":""}})
    return {"schema_version":"canon-independent-literary-review-v1",
            "source_id":SOURCE_ID,"original_pdf_sha256":DIGEST,
            "reviewed_pr":37,"reviewed_head_sha":head,
            "expected_record_counts":{"evidence":80,"behavioral_case":43,"goal_transition":15},
            "reviewer":{"identity":"","affiliation_or_model_provider":"",
                        "method":"","independent_of_pr_author":False,
                        "pdf_sha256_actually_verified":"","signed_at_utc":"",
                        "attestation":"","review_submission_url":""},
            "reviews":records,
            "release_note":"This is an UNFILLED worksheet, not a review. A third party must read original PDF, review each item and supply a verifiable external attestation. The assistant cannot self-sign.",
            "engine_memory_admission":False,"exam_executable":False,
            "goal_runtime_auto_update":False}

def validate(packet,head,allow_findings=False):
    issues=[]
    source=schema_records()
    expected={x["id"]:x for x in source}
    if packet.get("schema_version")!="canon-independent-literary-review-v1":issues.append("schema mismatch")
    if packet.get("source_id")!=SOURCE_ID or packet.get("original_pdf_sha256")!=DIGEST:issues.append("source identity mismatch")
    if not SHA_RE.fullmatch(head) or packet.get("reviewed_head_sha")!=head:issues.append("stale or invalid PR head")
    if packet.get("reviewed_pr")!=37:issues.append("wrong PR")
    if packet.get("expected_record_counts")!={"evidence":80,"behavioral_case":43,"goal_transition":15}:issues.append("wrong volume count")
    reviewer=packet.get("reviewer",{})
    if not reviewer.get("identity") or len(reviewer["identity"].strip())<4:issues.append("reviewer identity absent")
    for key in ("affiliation_or_model_provider","method","signed_at_utc","attestation","review_submission_url"):
        if len(str(reviewer.get(key,"")).strip())<8:issues.append("reviewer "+key+" missing")
    if reviewer.get("independent_of_pr_author") is not True:issues.append("independence not attested")
    if reviewer.get("pdf_sha256_actually_verified")!=DIGEST:issues.append("reviewer has not confirmed primary PDF checksum")
    if str(reviewer.get("review_submission_url","")).startswith("https://github.com/") is False:
        issues.append("review not linked to GitHub")
    if str(reviewer.get("identity","")).strip().lower() in ("same_assistant","chatgpt_same_assistant","sAME_assistant_two_methods".lower()):
        issues.append("same assistant not an independent reviewer")
    anchor_pages={x["record_id"]:x["pdf_page"] for x in read(DATA/"audits/y1-v06-verbatim-source-anchors.json")["records"]}
    rows=packet.get("reviews",[])
    if len(rows)!=138:issues.append("missing review rows")
    seen=set()
    findings=0
    for item in rows:
        claim=item.get("record",{})
        key=claim.get("id")
        if key in seen:issues.append("duplicate record "+str(key))
        seen.add(key)
        if key not in expected:issues.append("unknown record "+str(key));continue
        if claim!=expected[key] or item.get("record_fingerprint")!=row_fingerprint(expected[key]):
            issues.append("edited/stale source claim "+key)
        entry=item.get("review",{})
        verdict=entry.get("verdict")
        if verdict not in VERDICTS:issues.append("missing verdict "+key)
        if verdict!="APPROVED":findings+=1
        if entry.get("source_pdf_read") is not True:issues.append("PDF not actually read "+key)
        if not set(entry.get("pages_checked",[])).issubset(set(expected[key]["pdf_pages"])) or not entry.get("pages_checked"):
            issues.append("invalid page review "+key)
        refs=set(expected[key]["evidence_refs"])
        if set(entry.get("evidence_refs_checked",[]))!=refs:issues.append("missing per-claim citations "+key)
        for ref in refs:
            if anchor_pages.get(ref) not in entry.get("pages_checked",[]):
                issues.append("source page for referenced evidence not read "+key+" -> "+ref)
        excerpt=str(entry.get("short_source_excerpt","")).strip()
        if len(excerpt)<6:issues.append("supporting excerpt absent "+key)
        if len(excerpt.split())>20:issues.append("excessive quotation from copyrighted source "+key)
        if len(str(entry.get("reasoning","")).strip())<45:issues.append("insufficient reasoning "+key)
        if verdict in ("FINDING","UNRESOLVED"):
            if entry.get("finding_severity") not in ("LOW","MEDIUM","HIGH","CRITICAL"):issues.append("finding severity absent "+key)
            if len(str(entry.get("finding_description","")).strip())<30:issues.append("finding description absent "+key)
        if verdict=="APPROVED" and entry.get("finding_severity") is not None:
            issues.append("approved item has finding severity "+key)
    if set(expected)!=seen:issues.append("not all 138 claim IDs covered")
    for field in ("engine_memory_admission","exam_executable","goal_runtime_auto_update"):
        if packet.get(field) is not False:issues.append("unauthorized engine promotion "+field)
    if findings and not allow_findings:
        issues.append(str(findings)+" unremediated findings/unresolved decisions")
    # These checks verify the submitted document's shape, not the real
    # identity, independence or accuracy of an external evaluator.
    return sorted(set(issues))

def main():
    cli=argparse.ArgumentParser(description=__doc__)
    command=cli.add_subparsers(dest="command",required=True)
    p=command.add_parser("prepare")
    p.add_argument("--head",required=True)
    p.add_argument("--output",required=True)
    v=command.add_parser("validate")
    v.add_argument("--head",required=True)
    v.add_argument("--input",required=True)
    v.add_argument("--allow-findings",action="store_true")
    args=cli.parse_args()
    if args.command=="prepare":
        out=Path(args.output);out.parent.mkdir(parents=True,exist_ok=True)
        out.write_text(json.dumps(prepare(args.head),indent=2,ensure_ascii=False)+"\n",encoding="utf-8")
        print("Prepared 138 UNREVIEWED slots at",out)
    else:
        findings=validate(read(args.input),args.head,args.allow_findings)
        if findings:
            print(json.dumps({"status":"BLOCKED","issues":findings},ensure_ascii=False,indent=2))
            raise SystemExit(1)
        print("STRUCTURAL VALIDATION PASS only: human/source-independent identity still requires operator verification")

if __name__=="__main__":main()
