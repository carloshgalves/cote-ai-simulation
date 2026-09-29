from common import corpus_manifest
from retrieval import retrieve

def evaluate_suite(corpus, suite):
    records = corpus + suite.get("fixture_records", [])
    manifest = corpus_manifest(corpus)
    total = hits = returned_n = irrelevant = prov_bad = 0
    rr, failures = [], []
    wrong = timeline = post = 0
    for case in suite["cases"]:
        result = retrieve(records, case, suite["time_order"], manifest["digest"])
        returned = [x["evidence_id"] for x in result["items"]]
        expected = case.get("expected_evidence_ids", [])
        allowed = set(case.get("acceptable_evidence_ids", []))
        total += len(expected)
        hits += sum(x in returned for x in expected)
        ranks = [returned.index(x) + 1 for x in expected if x in returned]
        rr.append(1 / min(ranks) if ranks else (1.0 if not expected and result["abstained"] else 0.0))
        returned_n += len(returned)
        irrelevant += sum(x not in allowed for x in returned)
        for item in result["items"]:
            if any(item.get(k) in (None, "") for k in ("evidence_id","work","locator","source_id","provenance","manifest_digest")):
                prov_bad += 1
        excluded = {x["evidence_id"]: x["reason"] for x in result["excluded"]}
        for ev_id, reason in case.get("forbidden_evidence", {}).items():
            if ev_id in returned:
                failures.append(f"{case['id']} leaked {ev_id} ({reason})")
                wrong += reason == "WRONG_ACTOR"
                timeline += reason in {"SPOILER", "NOT_EFFECTIVE"}
                post += reason == "POST_DIVERGENCE_FACT"
            if excluded.get(ev_id) != reason:
                failures.append(f"{case['id']} expected {ev_id}={reason}, got {excluded.get(ev_id)}")
        if case.get("abstention") == "REQUIRED" and not result["abstained"]:
            failures.append(f"{case['id']} should abstain")
        if case.get("abstention") == "FORBIDDEN" and result["abstained"]:
            failures.append(f"{case['id']} should return evidence")
    metrics = {
        "recall_at_k": 1.0 if total == 0 else hits / total,
        "mean_reciprocal_rank": sum(rr) / len(rr),
        "irrelevant_context_rate": 0.0 if returned_n == 0 else irrelevant / returned_n,
        "wrong_character_leakage": wrong,
        "forbidden_timeline_leakage": timeline,
        "post_divergence_canon_leakage": post,
        "provenance_incomplete_items": prov_bad,
    }
    return {"manifest": manifest, "metrics": metrics, "critical_failures": failures}
