---
name: codebase-design
description: Use when introducing or changing module boundaries, interfaces, state ownership or dependencies in the simulation codebase.
---

# Codebase Design

Design deep modules with small interfaces.

Required seams:
- `WorldState` / simulation authority;
- perception/disclosure boundary;
- agent context builder;
- model adapter;
- memory/retrieval ports;
- exam rules/scoring;
- event log/snapshot;
- eval harness.

The domain must not depend directly on a specific LLM provider, vector database or agent framework. Keep adapters outside core rules.

Favor pure/deterministic functions for exam validation, scoring, time transitions and visibility filters. Those should be testable without an LLM call.
