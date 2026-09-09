---
name: simulation-evals
description: Use to evaluate end-to-end multi-agent runs, trajectories and world outcomes for causal correctness, fidelity, reproducibility and emergent behavior.
---

# Simulation Evals

Single-turn response evals are insufficient. Evaluate trajectories and environment state.

## Deterministic integrity
- same seed/config reproduces deterministic setup;
- rules/scoring match ExamSpec;
- impossible actions are rejected;
- event ordering/time remain valid.

## Agent integrity
- no secret knowledge leakage;
- memory/beliefs evolve from observed events;
- character behavior passes fidelity rubrics;
- strategic actions are grounded in available evidence.

## Trajectory quality
- inspect intermediate steps, not only winner/result;
- record major decisions and causal links;
- detect compounding orchestration errors;
- compare repeated stochastic runs rather than trusting one story.

## Regression suite
Maintain small fast scenarios plus a slower end-to-end exam benchmark. A visually entertaining run is not evidence that the simulator is correct.
