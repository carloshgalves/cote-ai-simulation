from __future__ import annotations


PURPOSE_ROLE = {
    "BEHAVIORAL_GUIDANCE": "BEHAVIORAL_CANON",
    "KNOWN_FACTS": "KNOWLEDGE_EVIDENCE",
}
DEFAULT_CONTINUITY = "ln"


def _anchor(time_bound):
    if not isinstance(time_bound, dict):
        return None
    if time_bound.get("mode") == "EXACT":
        return time_bound.get("exact", {}).get("anchor")
    if time_bound.get("mode") in {"HOLDS_BY", "INTERVAL"}:
        return time_bound.get("holds_by", {}).get("anchor")
    return None


def _record_missing(record):
    required = (
        "record_id",
        "evidence_id",
        "source_id",
        "work",
        "locator",
        "narrative_ordinal",
        "effective_time",
        "retrieval_role",
        "epistemic_status",
        "provenance",
        "source_digest",
    )
    if record.get("compilation_errors"):
        return True
    if any(record.get(field) in (None, "", {}) for field in required):
        return True
    if not record.get("continuity"):
        return True
    role = record.get("retrieval_role")
    if role == "BEHAVIORAL_CANON" and not record.get("actors"):
        return True
    if role == "KNOWLEDGE_EVIDENCE":
        if not record.get("supported_claim_ids"):
            return True
        resolved = {claim.get("claim_id") for claim in record.get("resolved_claims", [])}
        if set(record["supported_claim_ids"]) != resolved:
            return True
    return role not in set(PURPOSE_ROLE.values())


def _filter_denies(record, request):
    filters = request.get("filters", {})
    roles = set(filters.get("retrieval_roles", []))
    evidence_ids = set(filters.get("evidence_ids", []))
    topics = set(filters.get("topics", []))
    if roles and record.get("retrieval_role") not in roles:
        return True
    if evidence_ids and record.get("evidence_id") not in evidence_ids:
        return True
    if topics and not topics.intersection(record.get("topics", [])):
        return True
    return False


def _claim_time_reason(claim, now, order):
    temporal = claim.get("effective_time")
    if not isinstance(temporal, dict):
        return "MISSING_GATE_METADATA"
    start = order.get(_anchor(temporal.get("effective_from")))
    if start is None:
        return "MISSING_GATE_METADATA"
    if start > now:
        return "NOT_EFFECTIVE"
    effective_to = temporal.get("effective_to")
    if effective_to is not None:
        end = order.get(_anchor(effective_to))
        if end is None:
            return "MISSING_GATE_METADATA"
        if now > end:
            return "NOT_EFFECTIVE"
    return None


def _claim_divergence_reason(claim, divergence, order):
    temporal = claim.get("effective_time")
    if not isinstance(temporal, dict):
        return "MISSING_GATE_METADATA"
    start = order.get(_anchor(temporal.get("effective_from")))
    if start is None:
        return "MISSING_GATE_METADATA"
    if start > divergence:
        return "POST_DIVERGENCE_FACT"
    return None


def gate_reason(record, request, order):
    trace = []
    purpose = request.get("query", {}).get("purpose")
    expected_role = PURPOSE_ROLE.get(purpose)
    if expected_role is None:
        return "MISSING_GATE_METADATA", ["purpose=unknown"]
    if _filter_denies(record, request) or record.get("retrieval_role") != expected_role:
        return "FILTERED_OUT", ["prefilter=deny"]
    trace.append("prefilter=allow")

    if _record_missing(record):
        return "MISSING_GATE_METADATA", trace + ["metadata=deny"]

    if DEFAULT_CONTINUITY not in record["continuity"]:
        return "FILTERED_OUT", trace + ["continuity=deny"]

    story = order.get(_anchor(record["effective_time"]))
    now = order.get(request.get("simulation_time"))
    if story is None or now is None:
        return "MISSING_GATE_METADATA", trace + ["effective=unknown"]
    if story > now:
        return "NOT_EFFECTIVE", trace + ["effective=deny"]

    allowed = set(request.get("knowledge_scope", {}).get("allowed_claim_ids", []))
    supported_ids = set(record.get("supported_claim_ids", []))
    authorizing_ids = allowed.intersection(supported_ids)
    claims_by_id = {claim["claim_id"]: claim for claim in record.get("resolved_claims", [])}
    if record["retrieval_role"] == "KNOWLEDGE_EVIDENCE":
        if any(
            DEFAULT_CONTINUITY not in claims_by_id[claim_id].get("continuity", [])
            for claim_id in sorted(authorizing_ids)
        ):
            return "FILTERED_OUT", trace + ["continuity=deny"]
        for claim_id in sorted(authorizing_ids):
            reason = _claim_time_reason(claims_by_id[claim_id], now, order)
            if reason is not None:
                return reason, trace + ["effective=deny"]
    trace.append("effective=allow")

    if record["retrieval_role"] == "BEHAVIORAL_CANON":
        if request.get("character_id") not in record["actors"]:
            return "WRONG_ACTOR", trace + ["actor=deny"]
    elif authorizing_ids != supported_ids:
        return "CLAIM_NOT_ALLOWED", trace + ["actor=deny"]
    trace.append("actor=allow")

    horizon = request.get("knowledge_horizon", {})
    if record["work"] not in horizon or record["narrative_ordinal"] > horizon[record["work"]]:
        return "SPOILER", trace + ["spoiler=deny"]
    trace.append("spoiler=allow")

    divergence = request.get("divergence_time")
    if divergence is not None:
        divergence_value = order.get(divergence)
        if divergence_value is None:
            return "MISSING_GATE_METADATA", trace + ["divergence=unknown"]
        if record["retrieval_role"] == "KNOWLEDGE_EVIDENCE":
            if story > divergence_value:
                return "POST_DIVERGENCE_FACT", trace + ["divergence=deny"]
            for claim_id in sorted(authorizing_ids):
                reason = _claim_divergence_reason(
                    claims_by_id[claim_id], divergence_value, order
                )
                if reason is not None:
                    return reason, trace + ["divergence=deny"]
    trace.append("divergence=allow")

    status = record["epistemic_status"]
    if record["retrieval_role"] == "KNOWLEDGE_EVIDENCE":
        if status != "VERIFIED":
            return "UNVERIFIED_FACT", trace + ["epistemic=deny"]
        if record.get("open_conflicts"):
            return "OPEN_CONFLICT", trace + ["epistemic=deny"]
        for claim_id in sorted(authorizing_ids):
            claim = claims_by_id[claim_id]
            if claim.get("claim_kind") == "BELIEF":
                return "NON_FACTUAL_STATUS", trace + ["epistemic=deny"]
            if claim.get("epistemic_status") != "VERIFIED":
                return "UNVERIFIED_FACT", trace + ["epistemic=deny"]
            if claim.get("open_conflicts"):
                return "OPEN_CONFLICT", trace + ["epistemic=deny"]
    else:
        if status == "UNVERIFIED" and not request.get("allow_unverified_behavioral", False):
            return "UNVERIFIED_BEHAVIOR_DISABLED", trace + ["epistemic=deny"]
        if status in {"INFERRED", "INTERPRETATION"}:
            return "NON_FACTUAL_STATUS", trace + ["epistemic=deny"]
    return None, trace + ["epistemic=allow"]
