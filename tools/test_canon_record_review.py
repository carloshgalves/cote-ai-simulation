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
assert len(r["cases"])==len(idx["behavioral_case_paths"])==75
assert {c["case_id"] for c in r["cases"]}==set(idx["behavioral_case_paths"])
assert r["global_promotion"]=="BLOCKED_UNTIL_RECORD_LEVEL_SEMANTIC_SIGNOFF"
assert all(x["adjudication"]=="PENDING_INDIVIDUAL_SEMANTIC_SIGNOFF" and x["verified"] is False for x in r["records"])
assert all(c["verified"] is False for c in r["cases"])
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
print("PASS 268 individually indexed evidence / 75 cases; no unsanctioned semantic promotions.")
