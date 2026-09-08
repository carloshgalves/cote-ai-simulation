---
name: implement
description: Use when implementing an accepted spec/ticket; enforce deterministic core rules, tests and project-specific AI evals before completion.
---

# Implement

1. Read the originating spec/ticket, `CONTEXT.md`, relevant ADRs and matching domain skill.
2. Identify deterministic seams and write tests first where practical.
3. Implement the smallest vertical slice.
4. Keep provider/framework code behind adapters.
5. Add traces/metadata needed to reproduce model-dependent runs.
6. Run unit/integration tests.
7. Run applicable `llm-evals`, `rag-evals`, `knowledge-boundary-audit`, `character-fidelity` or `simulation-evals` checks.
8. Review the diff against both coding standards and spec.

Do not “fix” failing deterministic tests by relaxing rules into prompts.
