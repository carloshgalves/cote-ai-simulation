---
name: code-review
description: Use to review a diff for spec fidelity, engineering quality and COTE simulation invariants before merge.
---

# Code Review

Review three axes:

## Spec
Does the change implement the accepted issue/spec without inventing scope?

## Engineering
Are interfaces small, state ownership clear, tests meaningful and adapters isolated?

## Simulation invariants
Check especially:
- world truth vs belief separation;
- secret information leakage;
- LLM deciding deterministic rules;
- canon events leaking after divergence;
- unreproducible model runs;
- raw copyrighted corpus entering the public repo.

Classify findings by severity and point to concrete lines/behaviors.
