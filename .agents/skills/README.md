# Repo-scoped Agent Skills

Use the skill whose `description` matches the task. The canonical project context is `CONTEXT.md`; architectural decisions live in `docs/adr/`.

Project-specific AI skills are intentionally strict about knowledge isolation because accidental omniscience would invalidate strategic outcomes.


## Claude Code model routing

`/orchestrate #issue PHASE` delegates an **authorized current phase** to Claude Haiku/Sonnet/Opus agents under `.claude/agents/`; see `docs/agents/model-orchestration.md`. Does not override the manual `delivery-pipeline` phase gates. All skills follow `docs/agents/ci-billing-fallback.md` on unavailable runners.
