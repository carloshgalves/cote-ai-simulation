#!/usr/bin/env python3
"""Y1 V7 draft candidate integrity and V0↔V7 anti-leakage checks.

Draft-only source checks. Passing them DOES NOT grant semantic, external,
operator, engine or goal-runtime authority. Execute from any working directory:
    python -m unittest discover -s tools -p 'test_canon_y1_v07_staging.py'
"""
from __future__ import annotations

import copy
import json
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
AUDITS = ROOT / "data/canon/consolidated/audits"
V0_FILE = ROOT / "data/canon/consolidated/evidence/volume0.json"
V7_FILE = AUDITS / "y1-v07-extraction-candidates.json"
BRIDGE_FILE = AUDITS / "y1-v07-v00-reconciliation.json"

EXPECTED = {
    "evidence_candidates": 113,
    "behavioral_cases": 44,
    "goal_transitions": 18,
    "retrospective_dossiers": 14,
    "negative_controls_proposed": 32,
}

def read(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))

def check(data: dict, bridges: dict, volume0: dict) -> list[str]:
    failures: list[str] = []
    def require(ok: bool, reason: str) -> None:
        if not ok:
            failures.append(reason)

    require(data.get("schema_version") == "cote-y1-v7-report-evidence-candidates-v1", "wrong V7 candidate schema")
    require(data.get("source", {}).get("pdf_sha256") == "b41f00143f483fa7704bcc6c4bfda03323bd835c77ea1e95e53d27bd859e461b", "wrong V7 PDF hash")
    require(data.get("source", {}).get("original_pdf_committed") is False, "source published in public repo")
    for field, count in EXPECTED.items():
        rows = data.get(field, [])
        require(isinstance(rows, list) and len(rows) == count, "wrong count: " + field)
        if not isinstance(rows, list):
            continue
        ids = [row.get("id") for row in rows if isinstance(row, dict)]
        require(len(ids) == len(rows) and len(set(ids)) == len(ids), "duplicate/missing IDs: " + field)
        for row in rows:
            if not isinstance(row, dict):
                continue
            pages = str(row.get("pages", ""))
            nums = [int(n) for n in re.findall(r"\d+", pages)]
            require(bool(nums) and all(1 <= n <= 207 for n in nums), "invalid V7 source pages: " + str(row.get("id")))
            if field != "negative_controls_proposed":
                require(row.get("verified") is False, "false record signoff: " + str(row.get("id")))
    for row in data.get("goal_transitions", []):
        require(row.get("engine_goal_update") is False, "goal scripts must remain inert")
    r = data.get("reviews", {})
    require(r.get("external_independent_review") == "NOT_PERFORMED", "fake independent review")
    require(r.get("external_independent_reviews_completed") == 0, "fake reviewed count")
    require(r.get("external_independent_reviews_required") == 175, "wrong independent review scope")
    require(r.get("engine_memory_admission") is False, "unsafe agent memory admission")
    require(r.get("goal_runtime_auto_update") is False, "unsafe executable goal transition")
    require(r.get("human_release_gate") == "NOT_GRANTED", "fake human release")
    require(bridges.get("schema_version") == "cote-cross-volume-v00-v07-bridges-v1", "wrong cross-volume schema")
    require(bridges.get("release_gate", {}).get("engine_memory_admission") is False, "unsafe cross-volume engine admission")
    require(bridges.get("release_gate", {}).get("external_independent_review") == "NOT_PERFORMED", "fake cross-volume reviewer")
    links = bridges.get("links", [])
    require(isinstance(links, list) and len(links) == 10, "ten source-bound links required")
    existing_v0 = {row["id"] for row in volume0.get("records", [])}
    existing_v7 = {row["id"] for row in data.get("evidence_candidates", [])}
    for link in links:
        lid = link.get("id", "<missing>")
        prev = link.get("source_before", {})
        late = link.get("source_after", {})
        require(prev.get("work_id") == "ln.y1.v00.legacyemtls.v1.0.1", f"{lid}: wrong V0 source")
        require(late.get("work_id") == "ln.y1.v07.sevenseas.en.operator-provided", f"{lid}: wrong V7 source")
        require(bool(prev.get("evidence_refs")) and all(x in existing_v0 for x in prev.get("evidence_refs", [])), f"{lid}: V0 references missing")
        require(bool(late.get("evidence_refs")) and all(x in existing_v7 for x in late.get("evidence_refs", [])), f"{lid}: V7 references missing")
        require(bool(prev.get("pdf_pages")) and all(isinstance(p, int) and 1 <= p <= 265 for p in prev.get("pdf_pages", [])), f"{lid}: bad V0 page")
        require(bool(late.get("pdf_pages")) and all(isinstance(p, int) and 1 <= p <= 207 for p in late.get("pdf_pages", [])), f"{lid}: bad V7 page")
        require(link.get("global_actor_memory_backdating") is False, f"{lid}: backdated information")
        require(link.get("engine_admission") is False, f"{lid}: engine admission")
        require(link.get("relationship") in {"SUPPLEMENTS", "CONTEXTUALIZES", "CHALLENGES_WITH_CONFLICT"}, f"{lid}: unearned resolution")
        require(bool(link.get("unresolved")), f"{lid}: lost uncertainty boundaries")
    match = {link.get("id"): link for link in links}
    plan = match.get("V00V07-B05", {})
    require(plan.get("relationship") == "CONTEXTUALIZES" and any("UNKNOWN" in s for s in plan.get("unresolved", [])), "Atsuomi plan must remain open")
    matsuo = match.get("V00V07-B09", {})
    require(any("CHARACTER_CLAIM_NOT_VERIFIED" in s for s in matsuo.get("unresolved", [])), "Matsuo death must not be certified")
    findings = {x.get("id"): x for x in bridges.get("proposed_findings", [])}
    require(set(findings) == {"V00V07-OPEN-01", "V00V07-OPEN-02", "V00V07-OPEN-03"}, "lost historical findings")
    require(all(f.get("status") == "UNRESOLVED" for f in findings.values()), "unearned historical closure")
    return failures

class V7StagingTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.v7 = read(V7_FILE)
        cls.v0 = read(V0_FILE)
        cls.bridges = read(BRIDGE_FILE)

    def test_source_and_candidates(self):
        self.assertEqual([], check(self.v7, self.bridges, self.v0))

    def test_negative_fake_reviewer(self):
        modified = copy.deepcopy(self.v7)
        modified["reviews"]["external_independent_review"] = "APPROVED"
        self.assertTrue(check(modified, self.bridges, self.v0))

    def test_negative_premature_memory(self):
        modified = copy.deepcopy(self.bridges)
        modified["links"][0]["global_actor_memory_backdating"] = True
        self.assertTrue(check(self.v7, modified, self.v0))

    def test_negative_matsuo_premature_closure(self):
        modified = copy.deepcopy(self.bridges)
        next(x for x in modified["links"] if x["id"] == "V00V07-B09")["unresolved"] = ["Confirmed official death"]
        self.assertTrue(check(self.v7, modified, self.v0))

    def test_negative_atsuomi_secret_plan_certified(self):
        modified = copy.deepcopy(self.bridges)
        target = next(x for x in modified["links"] if x["id"] == "V00V07-B05")
        target["relationship"] = "RESOLVES_AMBIGUITY"
        target["unresolved"] = []
        self.assertTrue(check(self.v7, modified, self.v0))

    def test_negative_ghost_source_reference(self):
        modified = copy.deepcopy(self.bridges)
        modified["links"][0]["source_before"]["evidence_refs"] = ["v0.fabricated"]
        self.assertTrue(check(self.v7, modified, self.v0))

    def test_negative_independent_goal_runtime(self):
        modified = copy.deepcopy(self.v7)
        modified["goal_transitions"][0]["engine_goal_update"] = True
        self.assertTrue(check(modified, self.bridges, self.v0))

if __name__ == "__main__":
    unittest.main()
