# COTE Claude model orchestration: scoped phase routing

Invoke `/orchestrate #N PHASE` after this PR merges. **Does not turn on unattended delivery**. When no phase was authorized, agent may inspect GitHub/worktree and recommend a next phase read-only, but may not implement, review or remediate without a distinct human invocation.

The canonical `delivery-pipeline` skill still owns stage gates: PLAN → IMPLEMENT (checkpoint and STOP) → REVIEW (Draft PR, `cote-review-finding:v1`, STOP) → REMEDIATE (frozen OPEN findings, STOP) → REVIEW_AGAIN (independent validation, STOP). Separate merge authorization is always required.

Native Claude project subagents in `.claude/agents/cote-*.md` declare their own model, effort and tool allowlist:
- Haiku Medium: status, logs, handoff;
- Sonnet Medium: tickets, deterministic tests, evaluations and known-finding closure;
- Sonnet High: simulation implementation/regressions and initial independent review;
- Opus High: exceptional read-only ADR consultation across >=4 domains, once per ticket maximum; XHigh requires explicit human permission.

Agent delegation does not switch the parent's model and does not bypass provider account limits. Verify effective model/effort under `/tasks` in the supervised first pilot. Keep simulation deterministic world state, scoped character observations, replay/ledger/fork invariants, knowledge fences and canonical provenance intact. Do not publish copyrighted novels or protected private source material in model logs.

If CI jobs receive no runner, apply `docs/agents/ci-billing-fallback.md`; run offline deterministic and versioned evals only with actual exit codes and isolation, classify `CI_NOT_RUN` and `EVAL_NOT_RUN` explicitly and keep release/merge human gated.

Structural tests: `python3 -m unittest discover -s scripts/agents -p test_claude_orchestrate.py`. They check configuration, not effective Claude models or runtime correctness. First full pilot should be on a low-risk, fully approved synthetic issue without external LLM/production effects.
