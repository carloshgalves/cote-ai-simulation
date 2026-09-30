from __future__ import annotations

from common import lexical_score
from gates import gate_reason


def _validate_request(request, seed_time):
    top_k = request.get("top_k", 5)
    if isinstance(top_k, bool) or not isinstance(top_k, int) or top_k < 1:
        raise ValueError("top_k must be a non-boolean integer >= 1")

    mode = request.get("knowledge_scope", {}).get("mode")
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
