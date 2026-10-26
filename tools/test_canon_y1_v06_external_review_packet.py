"""Negative controls for a FUTURE external reviewer packet.

Synthetic test identities and APPROVED flags exist only in RAM while the
unit test runs. This test never creates an independent actual signoff and
never changes the canonical verification/release gates.
"""
import copy
import importlib.util
from pathlib import Path

MODULE_PATH=Path(__file__).with_name("canon_y1_v06_external_review_packet.py")
spec=importlib.util.spec_from_file_location("review_packet",MODULE_PATH)
m=importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
HEAD="a"*40

pkt=m.prepare(HEAD)
assert len(pkt["reviews"])==138
assert pkt["reviewer"]["identity"]==""
assert all(item["review"]["verdict"]=="PENDING" for item in pkt["reviews"])
assert all(pkt[x] is False for x in ("engine_memory_admission","exam_executable","goal_runtime_auto_update"))
assert m.validate(pkt,HEAD), "Blank reviewer worksheet must never pass"

# Build an in-memory specimen ONLY to test structural-validation behavior.
synthetic=copy.deepcopy(pkt)
synthetic["reviewer"]={
    "identity":"TEST_FIXTURE_OTHER_EVALUATOR_NOT_REAL",
    "affiliation_or_model_provider":"TEST_FIXTURE_ONLY",
    "method":"SYNTHETIC_TEST_ONLY_NOT_A_SOURCE_REVIEW",
    "independent_of_pr_author":True,
    "pdf_sha256_actually_verified":m.DIGEST,
    "signed_at_utc":"2026-10-26T15:00:00Z",
    "attestation":"TEST FIXTURE ONLY: NO ACTUAL REVIEWER APPROVAL IS ASSERTED",
    "review_submission_url":"https://github.com/example/fake-synthetic-unit-test"
}
anchors={x["record_id"]:x["pdf_page"] for x in m.read(m.DATA/"audits/y1-v06-verbatim-source-anchors.json")["records"]}
for row in synthetic["reviews"]:
    review=row["review"]
    review.update({
       "verdict":"APPROVED","source_pdf_read":True,
       "pages_checked":list({anchors[ref] for ref in row["record"]["evidence_refs"]}),
       "evidence_refs_checked":row["record"]["evidence_refs"].copy(),
       "short_source_excerpt":"SOURCE FIXTURE QUOTE ONLY",
       "reasoning":"Synthetic test explanation containing enough characters to exercise the data validator, not a real source reading.",
       "finding_severity":None,"finding_description":""
    })
assert m.validate(synthetic,HEAD)==[],m.validate(synthetic,HEAD)

negatives=[
("stale PR",lambda x:x.update(reviewed_head_sha="b"*40)),
("source changed",lambda x:x.update(original_pdf_sha256="WRONG")),
("fake independence",lambda x:x["reviewer"].update(independent_of_pr_author=False)),
("same assistant",lambda x:x["reviewer"].update(identity="same_assistant")),
("missing reviewer SHA",lambda x:x["reviewer"].update(pdf_sha256_actually_verified="WRONG")),
("invalid GitHub evidence link",lambda x:x["reviewer"].update(review_submission_url="NOT_AN_URL")),
("missing item",lambda x:x["reviews"].pop()),
("duplicate item",lambda x:x["reviews"].append(copy.deepcopy(x["reviews"][0]))),
("record changed",lambda x:x["reviews"][0]["record"].update(claim="Fabricated alternative claim")),
("fingerprint changed",lambda x:x["reviews"][1].update(record_fingerprint="FORGED")),
("unread PDF",lambda x:x["reviews"][2]["review"].update(source_pdf_read=False)),
("missing pages",lambda x:x["reviews"][3]["review"].update(pages_checked=[])),
("wrong cited source page",lambda x:x["reviews"][4]["review"].update(pages_checked=[1])),
("missing cited evidence",lambda x:x["reviews"][5]["review"].update(evidence_refs_checked=[])),
("fabricated quotation omitted",lambda x:x["reviews"][6]["review"].update(short_source_excerpt="")),
("copyright overquote",lambda x:x["reviews"][7]["review"].update(short_source_excerpt=" ".join(["word"]*21))),
("missing reasoning",lambda x:x["reviews"][8]["review"].update(reasoning="ok")),
("empty verdict",lambda x:x["reviews"][9]["review"].update(verdict="PENDING")),
("unresolved finding without rationale",lambda x:x["reviews"][10]["review"].update(verdict="UNRESOLVED")),
("unsourced finding severity",lambda x:x["reviews"][11]["review"].update(verdict="FINDING",finding_description="Claim error that remains material to source modeling")),
("approved with severity",lambda x:x["reviews"][12]["review"].update(finding_severity="HIGH")),
("unauthorized actor memory",lambda x:x.update(engine_memory_admission=True)),
("unauthorized executable exam",lambda x:x.update(exam_executable=True)),
("unauthorized goal runtime",lambda x:x.update(goal_runtime_auto_update=True)),
]
for name,change in negatives:
    test=copy.deepcopy(synthetic)
    change(test)
    assert m.validate(test,HEAD),("Review packet mutation escaped",name)
print(f"PASS external-review readiness: 138 mandatory unpublished slots, {len(negatives)} adversarial packet mutations rejected; no actual independent review")
