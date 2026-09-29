from __future__ import annotations
import hashlib, json, re
from pathlib import Path
from typing import Any

WORD_RE = re.compile(r"[A-Za-z0-9]+")

def canonical_json(value: Any) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode("utf-8")

def sha256_json(value: Any) -> str:
    return hashlib.sha256(canonical_json(value)).hexdigest()

def primary_support(ev: dict[str, Any]) -> dict[str, Any]:
    supports = ev.get("provenance", {}).get("supports", [])
    primaries = [x for x in supports if x.get("strength") == "primary"]
    if len(primaries) != 1:
        raise ValueError(f"{ev.get('id', '<missing>')}: exactly one primary support required")
    return primaries[0]

def source_id(ev: dict[str, Any]) -> str:
    primary = primary_support(ev)
    payload = {"work": primary.get("work"), "edition": primary.get("edition"), "locator": primary.get("locator")}
    return "src:" + sha256_json(payload)[:24]

def corpus_manifest(records: list[dict[str, Any]]) -> dict[str, Any]:
    ordered = sorted(records, key=lambda x: x["id"])
    manifest = {
        "manifest_version": 1,
        "compiler_id": "canon-rag-v0-reference",
        "chunking_id": "evidence-1to1-max1600-no-overlap",
        "backend_id": "reference-lexical-v0",
        "evidence_ids": [x["id"] for x in ordered],
        "sources": [
            {"evidence_id": x["id"], "work": x["narrative_position"]["work"], "source_id": source_id(x), "source_digest": sha256_json(x)}
            for x in ordered
        ],
    }
    manifest["digest"] = sha256_json(manifest)
    return manifest

def tokens(text: str) -> set[str]:
    return {x.casefold() for x in WORD_RE.findall(text)}

def lexical_score(query: str, ev: dict[str, Any]) -> float:
    q = tokens(query)
    if not q:
        return 0.0
    haystack = " ".join([ev.get("text", "")] + list(ev.get("topics", [])) + list(ev.get("situation_tags", [])))
    return len(q & tokens(haystack)) / len(q)

def load_corpus(repo: Path) -> list[dict[str, Any]]:
    out = []
    for path in sorted((repo / "data/canon/evidence/focal-v0").glob("*.json")):
        out.extend(json.loads(path.read_text(encoding="utf-8"))["evidence"])
    return out

def load_suite(repo: Path) -> dict[str, Any]:
    cfg = json.loads((repo / "data/canon/rag-evals/v0-config.json").read_text(encoding="utf-8"))
    fixtures = json.loads((repo / "data/canon/rag-evals/v0-fixtures.json").read_text(encoding="utf-8"))
    cases = []
    for path in sorted((repo / "data/canon/rag-evals/golden").glob("*.json")):
        cases.extend(json.loads(path.read_text(encoding="utf-8"))["cases"])
    return {**cfg, **fixtures, "cases": cases}
