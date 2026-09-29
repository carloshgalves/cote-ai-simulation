from common import lexical_score, primary_support, source_id
from gates import gate_reason

def retrieve(records, req, order, manifest_digest):
    eligible, excluded = [], []
    for ev in records:
        reason, trace = gate_reason(ev, req, order)
        if reason is not None:
            excluded.append({"evidence_id": ev.get("id"), "reason": reason, "gate_trace": trace})
            continue
        score = lexical_score(req["query"]["text"], ev)
        if score <= 0:
            excluded.append({"evidence_id": ev["id"], "reason": "FILTERED_OUT", "gate_trace": trace + ["rank=zero"]})
            continue
        eligible.append((score, ev, trace))
    eligible.sort(key=lambda x: (-x[0], x[1]["id"]))

    items = []
    allowed = set(req.get("knowledge_scope", {}).get("allowed_claim_ids", []))
    for score, ev, trace in eligible[:req.get("top_k", 5)]:
        primary = primary_support(ev)
        items.append({
            "evidence_id": ev["id"],
            "score": score,
            "work": ev["narrative_position"]["work"],
            "locator": primary.get("locator"),
            "source_id": source_id(ev),
            "supported_claim_ids": list(ev.get("supports_claims", [])),
            "retrieval_role": ev["retrieval_role"],
            "epistemic_status": ev["provenance"]["epistemic_status"],
            "authorization_claim_ids": sorted(set(ev.get("supports_claims", [])) & allowed),
            "gate_trace": trace,
            "provenance": ev["provenance"],
            "manifest_digest": manifest_digest,
            "text": ev["text"],
        })
    return {"items": items, "excluded": excluded, "abstained": not items, "manifest_digest": manifest_digest}

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
