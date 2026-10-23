"""Source-window audit registry covers every claim and case, without promoting them."""
import json
from pathlib import Path
B=Path(__file__).resolve().parents[1]/"data/canon/consolidated"
def J(p):return json.loads(p.read_text(encoding="utf-8"))
r=J(B/"audits/source-review-register-v0-v1.json")
e=[x for p in (B/"evidence").glob("*.json") for x in J(p)["records"]]
idx=J(B/"indexes/character-topic-map.json")
a={x["id"]:x for x in r["records"]}
assert len(e)==len(a)==268
assert set(a)=={x["id"] for x in e}
assert len(r["cases"])==len(idx["behavioral_case_paths"])==74
assert {c["case_id"] for c in r["cases"]}==set(idx["behavioral_case_paths"])
assert r["global_promotion"]=="BLOCKED_PENDING_OPERATOR_RELEASE_APPROVAL"
assert all(x["adjudication"]=="ASSISTANT_DIRECT_SOURCE_REVIEW_PASS" and x["verified"] is False for x in r["records"])
assert all(c["semantic_adjudication"]=="ASSISTANT_DIRECT_CASE_REVIEW_PASS" and c["verified"] is False for c in r["cases"])
sources={x["file"]:x["pages"] for x in r["sources"]}
for x in e:
    row=a[x["id"]]
    assert row["pdf_pages"]==x["source"]["pdf_pages"]
    assert row["source_mode"]==x["source_mode"]
    assert row["work_id"]==x["source"]["work_id"]
    n=sources["volume0" if x["id"].startswith("v0.") else "y1-v01"]
    assert all(type(p) is int and 1<=p<=n for p in row["pdf_pages"])
for x in r["cases"]:
    assert x["case_file"]==idx["behavioral_case_paths"][x["case_id"]]
    assert x["source_case_owner_id"]=="actor."+Path(x["case_file"]).parent.name
# An inactive legacy staging path must not be a live source.
for folder in ("characters", "institutions"):
    for packfile in (B/folder).glob("*.json"):
        record=J(packfile)
        assert all("/volume0/" not in path for path in record.get("source_documents", [])), packfile
assert "v0.case.kijima.public-profile" not in idx["behavioral_case_paths"]
assert "v0.case.kijima.public-profile" not in str(idx["behavioral_by_actor_topic"]["actor.kijima"])
assert J(B/"sources/volume0.json")["record_audit"]=="ASSISTANT_SEMANTIC_REVIEW_COMPLETE_OPERATOR_RELEASE_PENDING"
assert J(B/"sources/y1-v01.json")["record_audit"]=="ASSISTANT_SEMANTIC_REVIEW_COMPLETE_OPERATOR_RELEASE_PENDING"
father=J(B/"behavioral_cases/sakayanagi-father/sakayanagi--ethical-observation.json")
assert "Arisu" in father["epistemic_limits"] and "innate" not in father["observed_actions"]
print("PASS 268 reviewed evidence / 74 reviewed cases, attributed actor boundaries and zero unsupported Kijima feats.")
