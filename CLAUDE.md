# Claude Code project instructions

Read `CONTEXT.md`, `AGENTS.md`, relevant ADRs, and architecture docs before substantial work.

Project skills for Claude Code live in `.claude/skills/`. The same skill content is mirrored in `.agents/skills/` for other coding agents.

Use the skill whose frontmatter description best matches the task. Do not jump to implementation while important domain questions remain unresolved.

Core invariants:
- the simulation engine owns world truth, exam rules, scoring, time, and legal state transitions;
- separate world truth from each character's observations, beliefs, memories, inferences, and reports;
- character intelligence never grants hidden information;
- canon evidence is scoped by simulation time and divergence;
- RAG retrieves evidence for context construction and is not the character's mind;
- deterministic rules use ordinary tests; model-dependent behavior uses versioned evals;
- preserve enough metadata to reproduce stochastic runs.

For implementation work, keep the lifecycle explicit:
- `implement` is only for the first implementation of an accepted ticket/spec and must finish by committing the validated checkpoint on its feature branch; it does not inspect review findings;
- `code-review` is an independent pass over the committed implementation, owns the Draft PR review surface, and publishes each actionable finding separately without changing code;
- `address-review-findings` is only for an existing Draft PR with findings, fixes/justifies those findings, validates regressions, then commits and pushes the remediation on the same branch;
- run `code-review` again after remediation. Merge remains a separate explicit decision.

Current phase: architectural and domain discovery plus the first implemented Embodiment slices. Prefer `research`, `domain-modeling`, `character-fidelity`, `agent-memory-rag`, `knowledge-boundary-audit`, and `exam-modeling` before `implement` unless an accepted spec or ticket already exists.

For canon research, treat the light novel as the primary authority. Separate VERIFIED, INFERRED, INTERPRETATION, and UNVERIFIED claims. Do not silently fill gaps from model memory, and keep the simulation timestamp explicit to avoid future information contaminating earlier character states.
