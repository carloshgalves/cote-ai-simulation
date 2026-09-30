from __future__ import annotations

from copy import deepcopy
import unicodedata

from common import (
    BACKEND,
    compile_records,
    corpus_manifest,
    merge_source_registries,
    sha256_json,
)
from retrieval import retrieve


REQUEST_FIELDS = (
    "character_id",
    "simulation_time",
    "divergence_time",
    "knowledge_horizon",
    "query",
    "knowledge_scope",
    "filters",
    "top_k",
    "allow_unverified_behavioral",
)


def normalize_request(case):
    request = {field: deepcopy(case[field]) for field in REQUEST_FIELDS if field in case}
    request.setdefault("divergence_time", None)
    request.setdefault("knowledge_horizon", {})
    request.setdefault("knowledge_scope", {"mode": "SIMULATION", "allowed_claim_ids": []})
    request["knowledge_scope"].setdefault("allowed_claim_ids", [])
    request["knowledge_scope"]["allowed_claim_ids"] = sorted(
        set(request["knowledge_scope"]["allowed_claim_ids"])
    )
    request.setdefault("filters", {})
    for field in ("retrieval_roles", "topics", "evidence_ids"):
        request["filters"][field] = sorted(set(request["filters"].get(field, [])))
    request.setdefault("top_k", 5)
    request.setdefault("allow_unverified_behavioral", False)
    request["query"]["text"] = unicodedata.normalize("NFC", request["query"]["text"])
    return request


def evaluate_suite(corpus, claims, source_registry, suite):
    fixture_claims = suite.get("fixture_claims", [])
    fixture_registry = suite.get("fixture_source_registry", {"schema_version": 1, "works": []})
    records = compile_records(
        corpus + suite.get("fixture_records", []),
        claims + fixture_claims,
        merge_source_registries(source_registry, fixture_registry),
    )
    manifest = corpus_manifest(corpus, claims, source_registry)
    suite_identity = {
        "version": suite["suite_version"],
        "time_order": suite["time_order"],
        "seed_time": suite["seed_time"],
        "fixture_records": suite.get("fixture_records", []),
        "fixture_claims": fixture_claims,
        "fixture_source_registry": fixture_registry,
        "cases": suite["cases"],
    }
    suite_digest = sha256_json(suite_identity)
    configured_backend = {
        "id": suite.get("backend_id", BACKEND["id"]),
        "version": suite.get("backend_version", BACKEND["version"]),
    }
    if configured_backend != BACKEND:
        raise ValueError(f"suite backend {configured_backend!r} does not match reference backend {BACKEND!r}")
    backend = deepcopy(BACKEND)
    run_digest = sha256_json(
        {"manifest_digest": manifest["digest"], "suite_digest": suite_digest, "backend": backend}
    )

    total = hits = returned_n = irrelevant = provenance_bad = 0
    reciprocal_ranks, failures, case_reports = [], [], []
    wrong = timeline = post = 0
    for case in suite["cases"]:
        request = normalize_request(case)
        result = retrieve(
            records,
            request,
            suite["time_order"],
            manifest["digest"],
            suite["seed_time"],
        )
        returned = [item["evidence_id"] for item in result["items"]]
        expected = case.get("expected_evidence_ids", [])
        acceptable = set(case.get("acceptable_evidence_ids", []))
        total += len(expected)
        hits += sum(evidence_id in returned for evidence_id in expected)
        ranks = [returned.index(evidence_id) + 1 for evidence_id in expected if evidence_id in returned]
        reciprocal_ranks.append(
            1 / min(ranks) if ranks else (1.0 if not expected and result["abstained"] else 0.0)
        )
        returned_n += len(returned)
        irrelevant += sum(evidence_id not in acceptable for evidence_id in returned)
        for item in result["items"]:
            required = (
                "evidence_id",
                "work",
                "locator",
                "source_id",
                "provenance",
                "source_digest",
                "manifest_digest",
            )
            if any(item.get(field) in (None, "") for field in required):
                provenance_bad += 1
        excluded = {item["evidence_id"]: item["reason"] for item in result["excluded"]}
        for evidence_id, reason in case.get("forbidden_evidence", {}).items():
            if evidence_id in returned:
                failures.append(f"{case['id']} leaked {evidence_id} ({reason})")
                wrong += reason == "WRONG_ACTOR"
                timeline += reason in {"SPOILER", "NOT_EFFECTIVE"}
                post += reason == "POST_DIVERGENCE_FACT"
            if excluded.get(evidence_id) != reason:
                failures.append(
                    f"{case['id']} expected {evidence_id}={reason}, got {excluded.get(evidence_id)}"
                )
        if case.get("abstention") == "REQUIRED" and not result["abstained"]:
            failures.append(f"{case['id']} should abstain")
        if case.get("abstention") == "FORBIDDEN" and result["abstained"]:
            failures.append(f"{case['id']} should return evidence")
        case_reports.append(
            {
                "id": case["id"],
                "request": request,
                "result_evidence_ids": returned,
                "exclusions": [
                    {"evidence_id": item["evidence_id"], "reason": item["reason"]}
                    for item in result["excluded"]
                ],
            }
        )
    metrics = {
        "recall_at_k": 1.0 if total == 0 else hits / total,
        "mean_reciprocal_rank": sum(reciprocal_ranks) / len(reciprocal_ranks),
        "irrelevant_context_rate": 0.0 if returned_n == 0 else irrelevant / returned_n,
        "wrong_character_leakage": wrong,
        "forbidden_timeline_leakage": timeline,
        "post_divergence_canon_leakage": post,
        "provenance_incomplete_items": provenance_bad,
    }
    return {
        "manifest": manifest,
        "suite": {"version": suite["suite_version"], "digest": suite_digest},
        "backend": backend,
        "run_digest": run_digest,
        "cases": case_reports,
        "metrics": metrics,
        "critical_failures": failures,
    }
