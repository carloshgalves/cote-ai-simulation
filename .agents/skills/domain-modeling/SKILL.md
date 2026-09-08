---
name: domain-modeling
description: Use when domain terms, invariants, boundaries or responsibilities in the COTE simulation are unclear or changing; update CONTEXT.md/ADRs before implementation.
---

# Domain Modeling

1. Read `CONTEXT.md` and relevant ADRs.
2. Identify ambiguous nouns/verbs: world truth, belief, memory, observation, exam rule, action, scene, event, relationship, snapshot.
3. Stress-test each term with concrete scenarios involving hidden information and divergent timelines.
4. Assign responsibility to one bounded context; avoid duplicate sources of truth.
5. Record invariants explicitly.
6. Update `CONTEXT.md` or propose an ADR when a decision changes architecture.

Do not solve ambiguity by adding a generic service or a bigger prompt. If two modules can both “decide what happened,” the domain is not yet sharp enough.
