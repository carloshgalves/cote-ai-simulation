# CSFRV1-5 — Atomic terminal commit / retry implementation checkpoint

**Status:** implementation in progress, dedicated branch opened from the latest main after PR #20 merge.

## Contract
- Ticket: `docs/tickets/csf-runtime-v1/05-commit-atomico-e-retry.md`.
- Reuse CSFRV1-3 SQLite transaction boundary; no second authoritative writer.
- Promote CSFRV1-4 provisional decisions only within a terminal commit or persist fail-closed abort.
- Keep events, terminal decisions, lifecycle, tasks, journal envelope and world revision transactionally inseparable.
- Stop before autonomous review/merge; Draft PR remains open.

## Implementation ledger
Checkpoints and executed CI evidence will be appended as work progresses.
