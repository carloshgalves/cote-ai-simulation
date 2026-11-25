# Pilot V9 — autonomy and behavioral evaluation protocol V1

Delivery 3/9. This is an evaluation harness, not a change in simulation semantics.

## Evidence classes

1. **Deterministic integrity:** bounded full-population, fixed-seed trajectories with a wait-only provider substitute; validate causal ordering, balances against ledger deltas, CP conservation, body bounds, expiration of activities, social symmetry, private memory and chronology. Replay full final state; check source-forgery negative control.
2. **Behavioral proxies:** controlled positive/contrast/anti-cases for routine selection, need priority, independence of resolution tier and private identity. These assess policy behavior, not model reasoning.
3. **Model fidelity:** real model outputs under a fixed scenario/configuration, multiple independent runs and blinded human ratings. Not equivalent to the synthetic checks; no score without recorded outputs and source review.

## Reproduction

```sh
go test -count=1 ./internal/pilot -run 'TestAutonomy(BehavioralContrast|NegativeControlForgedRoutine)$'
COTE_AUTONOMY_EVAL=1 go test -v -count=1 -timeout=8m ./internal/pilot -run '^TestAutonomyLongitudinalEvaluation$'
# Optional, local only. Never put a real journal in CI artifacts or logs.
COTE_AUTONOMY_JOURNAL=/private/path/save-v9.json go test -v -count=1 ./internal/pilot -run '^TestAutonomyEvaluateLocalJournal$'
```

Longitudinal fixture: two seeds (64 repetitions of a and b), admission 08:00 to at least next day 08:00, 67 individually represented actors, maximum 600 cycles per trajectory. No provider access. Pass gates: horizon, routine and deliberative decisions both present, routine >= deliberative, focal actors complete ordinary tasks, Horikita and Kushida have endogenous opportunities, per-transition integrity and exact replay. These are minimum engineering gates, not target fidelity scores. Each seed reports actual counts independently; no average conceals a failed run. Counters measure committed decisions, not HTTP requests, retries, tokens or spend. Real cost comparison belongs to delivery 9.

## Evidence-linked rubric for subsequent real-model evaluations

The following references are existing repository case records, not newly verified source pages. Their source audit status remains pending as indicated in the records. Later canon cases inform reviewer hypotheses only, never runtime memory or compulsory outcomes.

| Case | Positive signal | Anti-case / failure signal | Evidence record |
|---|---|---|---|
| Ayanokoji, admission, possible friendship | Attempts, hesitation or a reasoned choice to remain discreet, depending on context | Automatically dominates everyone or acquires hidden facts; never has ordinary goals | `y1v1.case.k.social-entry-freeze` (`y1v1.ev.007`) |
| Horikita, unsolicited social pressure | Evaluates value of interaction; autonomy preserved | Forced trust solely from co-presence; rejection regardless of new causal evidence | `y1v1.case.s.refuse-forced-friendship` (`y1v1.ev.043`) |
| Kushida, social opportunity | Appropriate initiative and attention to interpersonal acceptance, without guaranteed success | Assumes all secrets are already available or trust is guaranteed | `y1v6.case.kikyo-kushida.construct-popularity-and-collect-trusted-secrets` (`y1v6.ev.001`, `y1v6.ev.002`), retrospective subjective report |
| Sudo, ordinary free time / sport | Sports interest balanced with actual constraints and competing needs | Infinite exercise despite fatigue, athletic skill treated as universal leadership | `y1v5.case.ken-sudo.practice-coaching-and-temper` (`y1v5.ev.019`) |

Review the same opportunity for at least two actors and vary goals, information, needs and interlocutor. Score 0 (contradiction), 1 (plausible but generic), 2 (differentiated and evidence-grounded), or N/A (opportunity not observed); keep per-case denominators and reviewer disagreement. Never count absence of opportunity as success. Hard privacy/rule failures are separate from behavioral scores. Use an independently reviewed rubric, model ID/version and parameters, journal digest, run horizon, seed and repeated samples before comparing model fidelity; the stub cannot supply that evidence. Do not expose private journals, corpus text or credentials publicly.

## Current limitations

- The CI synthetic fixture is a 24-hour trajectory rather than the whole first month or VIP exam. Separate existing exam tests remain required.
- Actual LLM reasoning, strategic initiative quality, emergence of friendships and long-term canonical fidelity are **NOT_RUN** without real-model trajectories and independent review. Co-presence is not dialogue or trust. The routine priors are provisional.
- The real-journal command is a replay and aggregate decision counter, not an independent human fidelity evaluator. No provider call is performed.
- Tomodachi Game has its own corpus branch; no changes to that branch or content.
