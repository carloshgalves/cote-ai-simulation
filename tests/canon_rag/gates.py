from common import primary_support

def _anchor(st):
    if st.get("mode") == "EXACT":
        return st.get("exact", {}).get("anchor")
    if st.get("mode") in {"HOLDS_BY", "INTERVAL"}:
        return st.get("holds_by", {}).get("anchor")

def _missing(ev, purpose):
    p, v = ev.get("narrative_position"), ev.get("provenance")
    if not ev.get("story_time") or not p or p.get("work") is None or p.get("ordinal") is None:
        return True
    if not v or v.get("epistemic_status") is None:
        return True
    if purpose == "BEHAVIORAL_GUIDANCE" and not ev.get("actors"):
        return True
    if purpose == "KNOWN_FACTS" and not ev.get("supports_claims"):
        return True
    try:
        primary_support(ev)
    except ValueError:
        return True
    return False

def gate_reason(ev, req, order):
    t, purpose = [], req["query"]["purpose"]
    roles = set(req.get("filters", {}).get("retrieval_roles", []))
    if roles and ev.get("retrieval_role") not in roles:
        return "FILTERED_OUT", ["prefilter=deny"]
    if _missing(ev, purpose):
        return "MISSING_GATE_METADATA", ["metadata=deny"]
    story, now = order.get(_anchor(ev["story_time"])), order.get(req.get("simulation_time"))
    if story is None or now is None:
        return "MISSING_GATE_METADATA", ["effective=unknown"]
    if story > now:
        return "NOT_EFFECTIVE", ["effective=deny"]
    t.append("effective=allow")

    if purpose == "BEHAVIORAL_GUIDANCE":
        if req["character_id"] not in ev.get("actors", []):
            return "WRONG_ACTOR", t + ["actor=deny"]
    elif purpose == "KNOWN_FACTS":
        allowed = set(req.get("knowledge_scope", {}).get("allowed_claim_ids", []))
        if not allowed.intersection(ev.get("supports_claims", [])):
            return "CLAIM_NOT_ALLOWED", t + ["actor=deny"]
    else:
        return "MISSING_GATE_METADATA", t + ["purpose=unknown"]
    t.append("actor=allow")

    p, h = ev["narrative_position"], req.get("knowledge_horizon", {})
    if p["work"] not in h or p["ordinal"] > h[p["work"]]:
        return "SPOILER", t + ["spoiler=deny"]
    t.append("spoiler=allow")

    div = req.get("divergence_time")
    if div is not None:
        d = order.get(div)
        if d is None:
            return "MISSING_GATE_METADATA", t + ["divergence=unknown"]
        if story > d and purpose == "KNOWN_FACTS":
            return "POST_DIVERGENCE_FACT", t + ["divergence=deny"]
    t.append("divergence=allow")

    prov, status = ev["provenance"], ev["provenance"]["epistemic_status"]
    if purpose == "KNOWN_FACTS":
        if status != "VERIFIED":
            return "UNVERIFIED_FACT", t + ["epistemic=deny"]
        if prov.get("conflicts", []):
            return "OPEN_CONFLICT", t + ["epistemic=deny"]
    else:
        if status == "UNVERIFIED" and not req.get("allow_unverified_behavioral", False):
            return "UNVERIFIED_BEHAVIOR_DISABLED", t + ["epistemic=deny"]
        if status in {"INFERRED", "INTERPRETATION"}:
            return "NON_FACTUAL_STATUS", t + ["epistemic=deny"]
    return None, t + ["epistemic=allow"]
