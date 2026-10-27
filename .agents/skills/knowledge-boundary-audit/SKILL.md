---
name: knowledge-boundary-audit
description: Use when changing prompts, context builders, observations, RAG, messages, exams or memories; audit that each agent sees only causally available information.
---

# Knowledge Boundary Audit

Treat information flow as a security boundary.

For every datum entering an agent context, answer:
- what is the source event/document?
- when did the agent gain access?
- by which channel?
- is the datum fact, report, inference or rumor?
- may this character know it at this simulation timestamp?

Mandatory tests:
- secret VIP/role not visible before disclosure;
- one agent's private observation absent from others;
- hidden White Room/background facts stay hidden;
- canon future events absent after divergence;
- retrieved chunks respect character/timeline scope;
- summaries/reflections do not promote guesses into world truth.

Prefer allow-list construction of context over generating a global prompt and trying to redact secrets afterward.

## Mandatory CI billing/runner fallback

When CI does not allocate runners or cannot execute, follow `docs/agents/ci-billing-fallback.md` and `AGENTS.md`: check runner/steps before diagnosing financial cause; report `CI_NOT_RUN` separately from failed code/evals, run bounded local deterministic/replay/eval verification with real exit codes and protected corpus excluded, and preserve separate human phase and merge gates.
