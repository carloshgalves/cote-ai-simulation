"""Pure offline validation of COTE's Claude agent routing; no LLM calls."""
import re
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
AGENTS = ROOT / ".claude" / "agents"
ROUTES = {
    "cote-route-status": ("haiku", "medium", True),
    "cote-ticket-planner": ("sonnet", "medium", True),
    "cote-implementer": ("sonnet", "high", False),
    "cote-validator": ("sonnet", "medium", False),
    "cote-review-initial": ("sonnet", "high", True),
    "cote-review-closure": ("sonnet", "medium", True),
    "cote-remediator": ("sonnet", "high", False),
    "cote-architecture-escalation": ("opus", "high", True),
}
def front(file):
    text = file.read_text(encoding="utf-8")
    m = re.match(r"\A---\n([\s\S]*?)\n---\n", text)
    if not m:
        raise AssertionError(f"Missing frontmatter: {file}")
    return {k.strip(): v.strip() for k, v in (line.split(":", 1) for line in m.group(1).splitlines())}

class ClaudeRoutingContract(unittest.TestCase):
    def test_models_tools_and_bounded_sessions(self):
        for name, (model, effort, readonly) in ROUTES.items():
            with self.subTest(name=name):
                f = front(AGENTS / (name + ".md"))
                self.assertEqual(f["name"], name)
                self.assertEqual(f["model"], model)
                self.assertEqual(f["effort"], effort)
                self.assertTrue(0 < int(f["maxTurns"]) <= 35)
                tools = set(f["tools"].replace(" ", "").split(","))
                self.assertTrue({"Read", "Grep", "Glob"} <= tools)
                if readonly:
                    self.assertFalse({"Edit", "Write", "Bash", "Agent"} & tools)
        self.assertEqual(set(ROUTES), {f.stem for f in AGENTS.glob("cote-*.md")})

    def test_orchestrate_is_explicit_and_respects_phase_gates(self):
        skill = ROOT / ".agents/skills/orchestrate/SKILL.md"
        bridge = ROOT / ".claude/skills/orchestrate/SKILL.md"
        for p in (skill, bridge):
            f = front(p)
            self.assertEqual(f["name"], "orchestrate")
            self.assertEqual(f["disable-model-invocation"], "true")
        text = skill.read_text(encoding="utf-8")
        for term in ("delivery-pipeline", "cote-review-finding:v1", "CI_NOT_RUN",
                     "world truth", "human", "/tasks", "Opus High"):
            self.assertIn(term, text)

    def test_ci_policy_all_canonical_skills(self):
        paths = list((ROOT / ".agents/skills").glob("*/SKILL.md"))
        self.assertGreaterEqual(len(paths), 20)
        for p in paths:
            with self.subTest(p=str(p)):
                self.assertIn("docs/agents/ci-billing-fallback.md", p.read_text(encoding="utf-8"))

if __name__ == "__main__":
    unittest.main()
