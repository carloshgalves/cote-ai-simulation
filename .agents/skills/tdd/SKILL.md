---
name: tdd
description: Use for deterministic simulation rules, exam scoring, visibility filters, event scheduling, snapshots and other behavior that can be tested without an LLM.
---

# TDD

Use red → green → refactor on deterministic seams.

Prioritize tests for:
- exam scoring and deadlines;
- action validation;
- secret/role disclosure;
- event ordering and logical time;
- snapshot restore/replay;
- provenance/visibility filters;
- promotion/demotion of NPC simulation tiers.

Do not demand exact string outputs from stochastic LLM calls. Those belong in eval suites with rubrics/thresholds.
