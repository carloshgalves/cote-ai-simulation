from __future__ import annotations

from common import lexical_score
from gates import gate_reason


def _validate_string_list(value, field):
    if not isinstance(value, list) or any(
        not isinstance(item, str) or not item for item in value
    ):
        raise ValueError(f"{field} must be a list of non-empty strings")


def _validate_request(request, seed_time):
    if not isinstance(request, dict):
        raise ValueError("request must be an object")

    for field in ("character_id", "simulation_time"):
        value = request.get(field)
        if not isinstance(value, str) or not value:
            raise ValueError(f"{field} must be a non-empty string")

    divergence = request.get("divergence_time")
    if divergence is not None and (not isinstance(divergence, str) or not divergence):
        raise ValueError("divergence_time must be null or a non-empty string")

    query = request.get("query")
    if not isinstance(query, dict):
        raise ValueError("query must be an object")
    if not isinstance(query.get("text"), str):
        raise ValueError("query.text must be a string")
    purpose = query.get("purpose")
    if not isinstance(purpose, str) or purpose not in {
        "BEHAVIORAL_GUIDANCE",
        "KNOWN_FACTS",
    }:
        raise ValueError(
            "query.purpose must be BEHAVIORAL_GUIDANCE or KNOWN_FACTS"
        )

    top_k = request.get("top_k", 5)
    if isinstance(top_k, bool) or not isinstance(top_k, int) or top_k < 1:
        raise ValueError("top_k must be a non-boolean integer >= 1")

    allow_unverified = request.get("allow_unverified_behavioral", False)
    if not isinstance(allow_unverified, bool):
        raise ValueError("allow_unverified_behavioral must be a boolean")

    horizon = request.get("knowledge_horizon", {})
    if not isinstance(horizon, dict) or any(
        not isinstance(work, str)
        or not work
        or isinstance(cap, bool)
        or not isinstance(cap, int)
        or cap < 0
        for work, cap in horizon.items()
    ):
        raise ValueError(
            "knowledge_horizon must map non-empty work ids to non-negative integer caps"
        )

    filters = request.get("filters", {})
    if not isinstance(filters, dict):
        raise ValueError("filters must be an object")
    for field in ("retrieval_roles", "topics", "evidence_ids"):
        if field in filters:
            _validate_string_list(filters[field], field)

    knowledge_scope = request.get("knowledge_scope")
    if not isinstance(knowledge_scope, dict):
        raise ValueError("knowledge_scope must be an object")
    if "allowed_claim_ids" in knowledge_scope:
        _validate_string_list(knowledge_scope["allowed_claim_ids"], "allowed_claim_ids")

    mode = knowledge_scope.get("mode")
    if mode not in {"CANON_SEED", "SIMULATION"}:
        raise ValueError("knowledge_scope.mode must be CANON_SEED or SIMULATION")
    if mode == "CANON_SEED":
        if seed_time is None:
            raise ValueError("CANON_SEED requires an explicit seed_time contract")
        if request.get("simulation_time") != seed_time:
            raise ValueError("CANON_SEED is valid only at the configured seed_time")
    return top_k


def retrieve(records, request, order, manifest_digest, seed_time=None):
    top_k = _validate_request(request, seed_time)
    eligible, excluded = [], []
    for record in records:
        reason, trace = gate_reason(record, request, order)
        if reason is not None:
            excluded.append(
                {"evidence_id": record.get("evidence_id"), "reason": reason, "gate_trace": trace}
            )
            continue
        score = lexical_score(request["query"]["text"], record)
        if score <= 0:
            excluded.append(
                {
                    "evidence_id": record["evidence_id"],
                    "reason": "FILTERED_OUT",
                    "gate_trace": trace + ["rank=zero"],
                }
            )
            continue
        eligible.append((score, record, trace))
    eligible.sort(key=lambda item: (-item[0], item[1]["evidence_id"]))

    items = []
    allowed = set(request.get("knowledge_scope", {}).get("allowed_claim_ids", []))
    for score, record, trace in eligible[:top_k]:
        items.append(
            {
                "evidence_id": record["evidence_id"],
                "score": score,
                "score_components": {"lexical_overlap": score},
                "work": record["work"],
                "locator": record["locator"],
                "source_id": record["source_id"],
                "supported_claim_ids": list(record["supported_claim_ids"]),
                "retrieval_role": record["retrieval_role"],
                "epistemic_status": record["epistemic_status"],
                "authorization_claim_ids": sorted(set(record["supported_claim_ids"]) & allowed),
                "gate_trace": trace,
                "provenance": record["provenance"],
                "source_digest": record["source_digest"],
                "manifest_digest": manifest_digest,
                "text": record["text"],
            }
        )
    return {
        "items": items,
        "excluded": excluded,
        "abstained": not items,
        "manifest_digest": manifest_digest,
    }


def assemble_context(result):
    lanes = {"CANON_BEHAVIORAL_EVIDENCE": [], "KNOWN_CANON_FACTS": []}
    for item in result["items"]:
        if item["retrieval_role"] == "BEHAVIORAL_CANON":
            lanes["CANON_BEHAVIORAL_EVIDENCE"].append(item)
        elif item["retrieval_role"] == "KNOWLEDGE_EVIDENCE":
            lanes["KNOWN_CANON_FACTS"].append(item)
        else:
            raise AssertionError(f"unknown retrieval role: {item['retrieval_role']}")
    return lanes
