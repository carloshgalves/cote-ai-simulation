---
name: exam-modeling
description: Use when converting an existing COTE special exam into an executable, deterministic ExamSpec and test suite.
---

# Exam Modeling

Do not implement an exam from narrative prose directly.

Extract and formalize:
- participants/classes/groups;
- public rules;
- secret roles/information;
- setup/randomization;
- timeline/deadlines;
- legal actions and validation;
- communication constraints;
- state transitions;
- scoring and penalties;
- terminal conditions.

Write examples and edge cases before agent strategy is involved. Verify the engine can run the exam with scripted/puppet agents first.

Only after deterministic correctness is proven should LLM agents enter. This prevents strategic model behavior from masking a broken exam implementation.
