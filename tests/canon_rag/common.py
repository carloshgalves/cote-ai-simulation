from __future__ import annotations

from copy import deepcopy
from functools import lru_cache
import hashlib
import json
from pathlib import Path
import re
import unicodedata
from typing import Any

import jsonschema
from referencing import Registry, Resource
import yaml


WORD_RE = re.compile(r"\w+", re.UNICODE)
COMPILER_ID = "canon-rag-v0-reference"
CHUNKING_ID = "evidence-1to1-max1600-no-overlap"
BACKEND = {"id": "reference-lexical", "version": 1}
SCHEMA_DIR = Path(__file__).resolve().parents[2] / "data/canon/schema"


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode("utf-8")


def sha256_json(value: Any) -> str:
    return hashlib.sha256(canonical_json(value)).hexdigest()


def primary_support(ev: dict[str, Any]) -> dict[str, Any]:
    supports = ev.get("provenance", {}).get("supports", [])
    primaries = [item for item in supports if item.get("strength") == "primary"]
    if len(primaries) != 1:
        raise ValueError(f"{ev.get('id', '<missing>')}: exactly one primary support required")
    return primaries[0]


def source_id(ev: dict[str, Any]) -> str:
    primary = primary_support(ev)
    payload = {
        "work": primary.get("work"),
        "edition": primary.get("edition"),
        "locator": primary.get("locator"),
    }
    if not isinstance(payload["work"], str) or not payload["work"]:
        raise ValueError("primary support requires work")
    if not isinstance(payload["edition"], str) or not payload["edition"]:
        raise ValueError("primary support requires edition")
    if not isinstance(payload["locator"], dict) or not payload["locator"]:
        raise ValueError("primary support requires a non-empty locator")
    return "src:" + sha256_json(payload)[:24]


def _index_unique(items: list[dict[str, Any]], kind: str) -> dict[str, dict[str, Any]]:
    indexed: dict[str, dict[str, Any]] = {}
    for item in items:
        item_id = item.get("id")
        if not isinstance(item_id, str) or not item_id:
            raise ValueError(f"{kind} requires a non-empty id")
        if item_id in indexed:
            raise ValueError(f"duplicate {kind} id: {item_id}")
        indexed[item_id] = item
    return indexed


@lru_cache(maxsize=None)
def _schema_validator(schema_name: str) -> jsonschema.Draft202012Validator:
    schema = json.loads((SCHEMA_DIR / schema_name).read_text(encoding="utf-8"))
    common = json.loads((SCHEMA_DIR / "common.schema.json").read_text(encoding="utf-8"))
    registry = Registry().with_resource(common["$id"], Resource.from_contents(common))
    return jsonschema.Draft202012Validator(
        schema,
        registry=registry,
        format_checker=jsonschema.Draft202012Validator.FORMAT_CHECKER,
    )


def _schema_errors(kind: str, value: dict[str, Any], schema_name: str) -> list[str]:
    errors = sorted(_schema_validator(schema_name).iter_errors(value), key=lambda error: error.json_path)
    return [f"schema:{kind}:{error.json_path}:{error.validator}" for error in errors]


def merge_source_registries(*registries: dict[str, Any]) -> dict[str, Any]:
    merged = {"schema_version": 1, "works": []}
    seen: set[str] = set()
    for registry in registries:
        if registry.get("schema_version") != 1:
            raise ValueError("unsupported source registry version")
        for work in registry.get("works", []):
            work_id = work.get("id")
            if not isinstance(work_id, str) or not work_id:
                raise ValueError("source work requires a non-empty id")
            if work_id in seen:
                raise ValueError(f"duplicate source work id: {work_id}")
            seen.add(work_id)
            merged["works"].append(deepcopy(work))
    merged["works"].sort(key=lambda item: item["id"])
    return merged


def _work_continuity(work: dict[str, Any]) -> str:
    if work.get("medium") == "anime" or work.get("form") == "adaptation-anime":
        return "anime"
    if work.get("medium") == "manga" or work.get("form") == "adaptation-manga":
        return "manga"
    return "ln"


def _normalized_continuity(provenance: dict[str, Any]) -> list[str]:
    return sorted(
        {
            item
            for item in provenance.get("continuity", [])
            if isinstance(item, str) and item
        }
    )


def _resolve_supports(
    provenance: dict[str, Any], work_by_id: dict[str, dict[str, Any]]
) -> tuple[list[dict[str, Any]], list[str]]:
    resolved = []
    errors = []
    continuity = set(provenance.get("continuity", []))
    for support in provenance.get("supports", []):
        work_id = support.get("work")
        work = work_by_id.get(work_id)
        if work is None:
            errors.append(f"unresolved_support_work:{work_id}")
            continue
        tier = work.get("tier")
        if not isinstance(tier, int) or tier > 3:
            errors.append(f"support_tier_not_allowed:{work_id}")
        support_edition = support.get("edition")
        registry_edition = work.get("edition")
        if support_edition is not None and support_edition != registry_edition:
            errors.append(f"support_edition_mismatch:{work_id}")
        if _work_continuity(work) not in continuity:
            errors.append(f"support_continuity_mismatch:{work_id}")
        resolved.append({"reference": deepcopy(support), "work": deepcopy(work)})
    return resolved, errors


def _compiled_claim(
    claim: dict[str, Any], work_by_id: dict[str, dict[str, Any]]
) -> dict[str, Any]:
    provenance = claim.get("provenance", {})
    resolved_supports, errors = _resolve_supports(provenance, work_by_id)
    errors.extend(_schema_errors("claim", claim, "claim.schema.json"))
    status = provenance.get("epistemic_status")
    claim_kind = claim.get("claim_kind")
    verified_tier_limit = {
        "WORLD_TRUTH": 1,
        "INSTITUTIONAL_RULE": 1,
        "ENTITY": 2,
    }.get(claim_kind)
    if status == "VERIFIED" and verified_tier_limit is not None:
        if not any(
            isinstance(item["work"].get("tier"), int)
            and item["work"]["tier"] <= verified_tier_limit
            for item in resolved_supports
        ):
            errors.append(f"verified_claim_tier_not_allowed:{claim_kind}")
    return {
        "claim_id": claim["id"],
        "claim_kind": claim_kind,
        "effective_time": deepcopy(claim.get("temporal")),
        "epistemic_status": status,
        "open_conflicts": sorted(provenance.get("conflicts", [])),
        "verification": {
            "verified_by": provenance.get("verified_by"),
            "verified_at": provenance.get("verified_at"),
        },
        "continuity": _normalized_continuity(provenance),
        "resolved_supports": resolved_supports,
        "source_digest": sha256_json(claim),
        "compilation_errors": sorted(set(errors)),
    }


def compile_records(
    evidence: list[dict[str, Any]],
    claims: list[dict[str, Any]],
    source_registry: dict[str, Any],
) -> list[dict[str, Any]]:
    """Compile authoritative inputs while retaining invalid records for fail-closed traces."""

    _index_unique(evidence, "evidence")
    claim_by_id = _index_unique(claims, "claim")
    work_by_id = _index_unique(source_registry.get("works", []), "source work")
    registry_errors = _schema_errors("source_registry", source_registry, "source.schema.json")
    records = []
    for ev in sorted(evidence, key=lambda item: item.get("id", "")):
        errors = list(registry_errors)
        errors.extend(_schema_errors("evidence", ev, "evidence.schema.json"))
        primary: dict[str, Any] = {}
        resolved_source_id = None
        try:
            primary = primary_support(ev)
            resolved_source_id = source_id(ev)
        except (TypeError, ValueError):
            errors.append("invalid_primary_support")

        narrative = ev.get("narrative_position") or {}
        narrative_work = narrative.get("work")
        primary_work = primary.get("work")
        work = work_by_id.get(primary_work) if primary_work else None
        if narrative_work != primary_work:
            errors.append("narrative_source_work_mismatch")
        if work is None:
            errors.append("unresolved_source_work")
        elif work.get("edition") != primary.get("edition"):
            errors.append("source_edition_mismatch")

        _, support_errors = _resolve_supports(ev.get("provenance") or {}, work_by_id)
        errors.extend(support_errors)

        provenance = ev.get("provenance") or {}
        continuity = _normalized_continuity(provenance)
        resolved_claims = []
        for claim_id in ev.get("supports_claims", []):
            claim = claim_by_id.get(claim_id)
            if claim is None:
                errors.append(f"unresolved_claim:{claim_id}")
            else:
                compiled_claim = _compiled_claim(claim, work_by_id)
                resolved_claims.append(compiled_claim)
                errors.extend(compiled_claim["compilation_errors"])
                if not set(continuity).intersection(compiled_claim["continuity"]):
                    errors.append(f"claim_continuity_mismatch:{claim_id}")

        source_material = {"evidence": ev, "claims": resolved_claims, "work": work}
        records.append(
            {
                "record_id": f"record:{ev.get('id')}" if ev.get("id") else None,
                "evidence_id": ev.get("id"),
                "source_id": resolved_source_id,
                "work": narrative_work,
                "locator": deepcopy(primary.get("locator")),
                "narrative_ordinal": narrative.get("ordinal"),
                "actors": list(ev.get("actors", [])),
                "supported_claim_ids": list(ev.get("supports_claims", [])),
                "resolved_claims": resolved_claims,
                "effective_time": deepcopy(ev.get("story_time")),
                "retrieval_role": ev.get("retrieval_role"),
                "epistemic_status": provenance.get("epistemic_status"),
                "continuity": continuity,
                "open_conflicts": sorted(provenance.get("conflicts", [])),
                "topics": list(ev.get("topics", [])),
                "situation_tags": list(ev.get("situation_tags", [])),
                "text": ev.get("text"),
                "provenance": deepcopy(provenance),
                "source_digest": sha256_json(source_material),
                "compilation_errors": sorted(set(errors)),
            }
        )
    return records


def corpus_manifest(
    evidence: list[dict[str, Any]],
    claims: list[dict[str, Any]],
    source_registry: dict[str, Any],
) -> dict[str, Any]:
    evidence_by_id = _index_unique(evidence, "evidence")
    claim_by_id = _index_unique(claims, "claim")
    ordered_evidence = sorted(evidence_by_id.values(), key=lambda item: item["id"])
    ordered_claims = sorted(claim_by_id.values(), key=lambda item: item["id"])
    referenced_work_ids = sorted(
        {
            item.get("narrative_position", {}).get("work")
            for item in ordered_evidence
            if item.get("narrative_position", {}).get("work")
        }
        | {
            support.get("work")
            for item in [*ordered_evidence, *ordered_claims]
            for support in item.get("provenance", {}).get("supports", [])
            if support.get("work")
        }
    )
    work_by_id = _index_unique(source_registry.get("works", []), "source work")
    normalized_registry = deepcopy(source_registry)
    normalized_registry["works"] = sorted(
        normalized_registry.get("works", []), key=lambda item: item["id"]
    )
    works = []
    for work_id in referenced_work_ids:
        work = work_by_id.get(work_id)
        if work is None:
            works.append({"id": work_id, "version": "UNRESOLVED", "source_digest": None})
            continue
        works.append(
            {
                "id": work_id,
                "version": work.get("edition", f"registry-v{source_registry.get('schema_version', 'unknown')}"),
                "source_digest": sha256_json(work),
            }
        )

    manifest = {
        "manifest_version": 1,
        "compiler_id": COMPILER_ID,
        "chunking_id": CHUNKING_ID,
        "backend": BACKEND,
        "evidence_ids": [item["id"] for item in ordered_evidence],
        "claim_ids": [item["id"] for item in ordered_claims],
        "works": works,
        "source_digests": {
            "evidence": [{"id": item["id"], "digest": sha256_json(item)} for item in ordered_evidence],
            "claims": [{"id": item["id"], "digest": sha256_json(item)} for item in ordered_claims],
            "source_registry": sha256_json(normalized_registry),
        },
    }
    manifest["digest"] = sha256_json(manifest)
    return manifest


def tokens(text: str) -> set[str]:
    normalized = unicodedata.normalize("NFC", text).casefold()
    return set(WORD_RE.findall(normalized))


def lexical_score(query: str, record: dict[str, Any]) -> float:
    query_tokens = tokens(query)
    if not query_tokens:
        return 0.0
    haystack = " ".join(
        [record.get("text", "")] + list(record.get("topics", [])) + list(record.get("situation_tags", []))
    )
    return len(query_tokens & tokens(haystack)) / len(query_tokens)


def load_corpus(repo: Path) -> list[dict[str, Any]]:
    output = []
    for path in sorted((repo / "data/canon/evidence/focal-v0").glob("*.json")):
        output.extend(json.loads(path.read_text(encoding="utf-8"))["evidence"])
    return output


def load_claims(repo: Path) -> list[dict[str, Any]]:
    claims = []
    for path in sorted((repo / "data/canon").rglob("*.yaml")):
        document = yaml.safe_load(path.read_text(encoding="utf-8"))
        if isinstance(document, dict) and isinstance(document.get("claims"), list):
            claims.extend(document["claims"])
    _index_unique(claims, "claim")
    return sorted(claims, key=lambda item: item["id"])


def load_source_registry(repo: Path) -> dict[str, Any]:
    path = repo / "data/canon/sources/works.yaml"
    registry = yaml.safe_load(path.read_text(encoding="utf-8"))
    _index_unique(registry.get("works", []), "source work")
    return registry


def load_suite(repo: Path) -> dict[str, Any]:
    config = json.loads((repo / "data/canon/rag-evals/v0-config.json").read_text(encoding="utf-8"))
    fixtures = json.loads((repo / "data/canon/rag-evals/v0-fixtures.json").read_text(encoding="utf-8"))
    cases = []
    for path in sorted((repo / "data/canon/rag-evals/golden").glob("*.json")):
        cases.extend(json.loads(path.read_text(encoding="utf-8"))["cases"])
    return {**config, **fixtures, "cases": cases}
