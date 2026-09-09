---
name: diagnosing-bugs
description: Use for hard bugs, non-deterministic failures, agent regressions, performance/cost spikes or simulations that cannot be reproduced.
---

# Diagnosing Bugs

1. Capture seed, model/provider/version, prompt/config version, exam spec version and snapshot/event-log position.
2. Reduce to the smallest failing trajectory.
3. Classify the fault: deterministic engine, visibility leak, retrieval, memory, prompt/model behavior, orchestration or persistence.
4. Instrument before guessing.
5. Fix the root cause.
6. Add deterministic regression test or eval case.

For “the character acted weird,” inspect retrieved canon evidence and current beliefs/memories before rewriting the entire persona prompt.
