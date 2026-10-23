"""Year 1 Volume 2: direct index, owner attribution, provenance and institutional gates."""
import json
from pathlib import Path
B=Path(__file__).resolve().parents[1]/"data/canon/consolidated"
J=lambda p:json.loads(p.read_text(encoding="utf-8"))
src=J(B/"sources/y1-v02.json")
e=J(B/"evidence/y1-v02.json")["records"]
i=J(B/"indexes/character-topic-map.json")
page=J(B/"indexes/y1-v02-page-map.json")
aud=J(B/"audits/y1-v02-coverage.json")
eids={x["id"] for x in e}
assert len(e)==len(eids)==aud["record_count"]==91
assert src["pdf_pages"]==261 and src["pdf_sha256"]=="11da5801d717d2d072ba52619ea60987e0d581b8de6f477d33e01e8b1211b073"
assert src["operator_accepted"] and not src["claims_all_verified"] and not src["original_pdf_committed"]
assert set(x["source"]["chapter"] for x in e)==set(range(1,8))
assert all(x["source"]["work_id"]==src["source_id"] for x in e)
assert all(x["source_mode"] in {"OBSERVED","NARRATED","SELF_DESCRIPTION","REPORTED_BY_CHARACTER","INNER_MONOLOGUE"} for x in e)
assert all(x["assertion_status"]=="OPERATOR_ACCEPTED_RECORD_AUDIT_PENDING" for x in e)
assert all(x["subject_assertion"]=="MENTION_IS_NOT_PROOF_OF_ACTOR_BEHAVIOR" for x in e)
v2_cases={k:p for k,p in i["behavioral_case_paths"].items() if k.startswith("y1v2.case.")}
assert len(v2_cases)==27
cases=[J(B.parent.parent.parent/p) for p in v2_cases.values()]
assert len({c["id"] for c in cases})==27
assert all(c["id"] in v2_cases and c["primary_actor_id"]==c["actor_id"] for c in cases)
assert all(c["epistemic_status"]=="OPERATOR_ACCEPTED_RECORD_AUDIT_PENDING" for c in cases)
assert all(c["evidence_refs"] and set(c["evidence_refs"])<=eids for c in cases)
for c in cases:
    aid=c["primary_actor_id"]
    assert aid in i["character_packs"]
    assert Path(v2_cases[c["id"]]).parent.name==aid.removeprefix("actor.")
    for topic in c["topics"]:
        assert c["id"] in i["behavioral_by_actor_topic"][aid][topic]
for x in e:
    for actor in x["mention_actor_ids"]:
        assert actor in i["character_packs"]
        for topic in x["topics"]:
            assert x["id"] in i["context_mentions_by_actor_topic"][actor][topic]
    for n in x["source"]["pdf_pages"]:
        assert 1<=n<=261 and x["id"] in page["pages"][str(n)]["evidence"]
for c in cases:
    for n in c["source"]["pdf_pages"]:
        assert 1<=n<=261 and c["id"] in page["pages"][str(n)]["cases"]
ryuen=J(B/"characters/kakeru-ryuen.json")
assert not any(v["behavioral_case_refs"] for v in ryuen["domains"].values())
assert any("Ryūen" in x["summary"] and x["source_mode"]=="REPORTED_BY_CHARACTER" for x in e)
school=J(B/"institutions/anhs.json")
transfer=school["s_system"]["individual_class_transfer_y1_v02"]
assert transfer["price_private_points"]==20000000 and transfer["observed_successful_transfer"] is False
assert school["disciplinary_case_y1_v02"]["charge_amount"]=="UNKNOWN"
assert school["s_system"]["club_reward_y1_v02"]["unrevealed_reward_algorithm"]
assert aud["operator_release"]=="PENDING"

review=J(B/"audits/source-review-register-y1-v02.json")
assert review["evidence_count"]==len(e)==91
assert review["behavioral_case_count"]==len(cases)==27
assert {x["record_id"] for x in review["evidence"]}==eids
assert {x["case_id"] for x in review["behavioral_cases"]}==set(v2_cases)
assert all(x["verified"] is False and x["reviewed_by_assistant"] is False for x in review["evidence"]+review["behavioral_cases"])
assert review["release_gate"]["individual_verified_count"]==0

print("PASS Y1 V2: 91 evidence, 27 actor-owned cases, canonical index and source/knowledge gates")
