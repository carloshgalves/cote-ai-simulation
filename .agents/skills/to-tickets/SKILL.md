---
name: to-tickets
description: Use to split an accepted spec into small tracer-bullet implementation tickets with explicit dependencies and validation.
---

# To Tickets

Break work into vertical slices that produce observable behavior.

Each ticket states:
- outcome;
- files/modules likely touched;
- dependency/blocking edges;
- deterministic tests;
- model/RAG evals when applicable;
- knowledge-boundary checks when applicable;
- completion evidence.

Prefer a slice like “run one turn of one exam with isolated knowledge and deterministic scoring” over separate tickets for every repository layer.
