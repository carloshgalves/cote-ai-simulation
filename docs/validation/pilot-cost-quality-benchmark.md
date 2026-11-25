# Cost and quality benchmark — delivery 9/9

Compare V7 (cognition), V8 (selective activation) and V9 (autonomous life) on two matched deterministic seeds, the same admission population, same wait-only decision substitute, and a target of 600 simulated minutes. Report each trajectory separately. No provider calls are made. This is a structural comparison of workload and ordinary-life activity, not a measurement of actual model fidelity.

```sh
COTE_COST_BENCHMARK=1 go test -v -count=1 -timeout=12m ./internal/pilot -run '^TestPilotCostQualityBenchmark$'
go test -count=1 ./internal/pilot -run '^TestPilotBenchmarkTelemetryMissingUsage$'
```

Rows contain simulated minutes, cycles, committed routine/deliberative decisions, endogenous-only opportunities (V8/V9), completed task records, academic tasks, actors with task records, pair-days of co-presence, serialized context bytes and wall time. Compare decisions per simulated hour, not per cycle; report horizon differences, never equate bytes with provider tokens. No threshold is calibrated against canon. Independent integrity checks from delivery 3 remain active. A failed trajectory is a failure, not an omitted observation.

## Actual provider measurements (local, optional)

Export the operator report from the Observatory, extract operador.json privately, then:

```sh
COTE_BENCHMARK_REPORT=/private/operador.json go test -v -count=1 ./internal/pilot -run '^TestPilotBenchmarkLocalTelemetry$'
```

The telemetry is current-process only, includes route tests, failures, repairs and possible dropped records. The evaluator groups by provider/model and prints anonymous group indices, usage coverage and sums of reported token counts. Missing usage is not zero consumption. To compare actual model runs, reset telemetry by restarting between runs, use distinct saves, retain model IDs/configuration privately, record all attempts, run several seeds and score blind using the delivery-3 rubric. For monetary estimates only, multiply observed input and output tokens by the applicable provider-specific prices per million tokens; cached tokens, subscription limits, missing records and non-token fees need separate accounting. No price or cost is fabricated.

## Acceptance boundary

Synthetic benchmark results become evidence only after successful execution; actual HTTP cost, token savings, and canonical fidelity remain NOT_RUN until real trajectories are evaluated. Do not merge a failing engineering gate. This change does not modify runtime semantics or the Tomodachi Game corpus.
