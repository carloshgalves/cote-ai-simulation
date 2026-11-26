# Zodiac day-two model comparison (V29 experiment)

This is an opt-in laboratory chronology, not the normal school calendar or a canonical event. The journal records scenario `zodiac-day2`. Never mutate a previous journal to change its scenario or seed.

- Day 1: ordinary admission and school routine. No advance notice, roster, private role or guessing option for the first exam in actor context.
- Day 2, 08:00 (minute 1920): announce Zodiac/VIP for the whole first year, 160 students, 40 per class, 12 mixed-class groups of 13–14. One private VIP per group. Named participants receive the exam stimulus and are eligible for cognitive activation; generic students retain the existing deterministic policy.
- Days 2–4: existing three-day adapted Zodiac windows, including the final 21:30–22:00 window on day 4 and result publication at 23:00. Existing deterministic scoring, private roles, and school CP/PP rules apply. The regular graduation/benefit disclosure remains separate; the exam itself explains its own CP/PP rewards.
- Remaining calendar: retain 18 annual slots and six per grade, swapping the displaced first-grade allocation if needed; no overlap.
- Ordinary-life prompt baseline: permit friendship, hobbies, curiosity, routine, and waiting without framing every contact as an investigation; allow grounded strategic agency. This policy is scoped to the experimental journal and does not include any future exam information.
- No claim of NPC strategic equivalence, full canonical Zodiac mechanics, LLM fidelity, annual qualification, or accelerated routine execution.

## Run locally

```sh
git fetch origin
git switch --track origin/experiment/zodiac-day2
go test -count=1 ./internal/pilot -run '^TestZodiacDay2'
go run ./cmd/cote-local -provider ui -scenario zodiac-day2 -seed comparison-01 -save cote-zodiac-weak.json
```

Open http://127.0.0.1:8765. In Modelos e APIs, select and enable only the intended model route, disable fallbacks, and keep the same seed and starting configuration across model comparisons. Stop the process with Ctrl+C before using CLI on that save. Use a different new save per model and per repeated trajectory. Do not delete a save or its sidecar; resume by reusing exactly the same filename and configuration.

After selecting the first model, run to the result via UI, or, with routes already configured and the UI stopped:

```sh
go run ./cmd/cote-local -provider ui -scenario zodiac-day2 -seed comparison-01 -save cote-zodiac-weak.json -until first-result > zodiac-weak-result.json
```

For the next model, configure only that route and repeat with a new save, for example `cote-zodiac-strong.json`, using the same seed label. The deterministic seed is generated from the label; matching labels give the same initial world, exam allocation and VIP draw. They do not guarantee identical model decisions or provider randomness. Keep exact model ID and inference parameters in the run record. Repeat with additional labels after the matched comparison. A provider failure is not a model-quality score.

## Assessment

Compare concrete negotiation, use of legitimate evidence, deduction of VIP identity, bluffing, cooperation, premature guesses, adaptation to other agents, preservation of secrecy, and ordinary-life behavior before disclosure. Track points and results separately from the quality of decisions. Report provider attempts and time, but do not treat either as strategic quality. No real-provider experiment has been executed by this implementation.

Current limitations: existing Zodiac adapter, existing event/decision persistence and routing, and existing deterministic generic-agent behavior remain unchanged. This experiment does not implement cognitive sparsification or dialogue/event compression. The independent review and Go CI are separate gates.
