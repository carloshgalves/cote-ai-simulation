# CSFRV1-9 Observatory — O4 local acceptance and operational-metrics boundary

**Issue:** [#88](https://github.com/carloshgalves/cote-ai-simulation/issues/88), parent [#84](https://github.com/carloshgalves/cote-ai-simulation/issues/84).
**Branch:** `feat/88-observatory-operations-acceptance`; **base:** `main@964a4255c01db1d1bcbb4f74ee3a3598fde4d2f4`.
**Evidence gate:** [Draft PR #92](https://github.com/carloshgalves/cote-ai-simulation/pull/92), GitHub Actions **O4 Observatory operational metrics and integrated acceptance** on the exact final head SHA (inspect latest run, not an earlier attempt).

## Authority and permitted behavior

1. **Causal/epistemic database:** O1, O2, O3 adapters open a distinct SQLite `mode=ro`, `query_only(ON)` connection. Verified `ReadRun` and O2 replay prove clock/world/cycle. The actor POV is bound to a durable `SlotDispatch`, and only matched `Observation`/`KnowledgeInput` receipts reach A. B receives zero unauthorized entries. The public view returns `NO_REGISTERED_PUBLIC_ROUTE` until a distinct registered publisher is built.
2. **Operations data:** `internal/csf/operationalmetrics` uses a physically **separate** SQLite sidecar. The host-only `Recorder.Record` may append to that sidecar; `Reader.Read` opens it using `mode=ro` and `query_only(ON)`. The reader never creates an absent file. Both APIs require the causal path solely to reject path equality, symlink/alias and hardlink. **Neither interface imports or writes into a causal store.**
3. **Trust:** Every metric is a *host-supplied, untrusted observation*, **not** a normative clock, world revision, rejected-ingress audit, actor memory, token billing fact or evidence in the five causal ledgers. No automatic LLM instrumentation, real model cost metering or backfilled timestamps exists in this slice.
4. **Schema:** V1 sidecar stores bounded metric samples (sample ID, run correlation, wall nanos, stage, duration, host/worker IDs, queue depth, retries, worker calls, cost micros with currency, rejection count/code). Separate versioned schema, strict field allow-lists, maximum 4 KiB/sample, deterministic `ORDER BY wall_time,id`, limit 1–1000 **without silent truncation**. Repeated identical ID is idempotent; inconsistent ID bytes are rejected. Reader refuses malformed/unknown/noncanonical JSON and fails all-or-error.
5. **Path safety:** same-path, existing symlink/hardlink, **dangling symlink before causal bootstrap** and directory symlink alias before creation are denied. The trusted local operator must keep paths inside a controlled workspace; this is not an untrusted multi-user file-hosting sandbox. No web endpoint or SaaS capability is added.

## Acceptance matrix — requirement → executable proof

| Criterion | Test/evidence | Expected invariant |
|---|---|---|
| A01 Same-run O1/O2/POV A/POV B/public | `TestCSFRV19O4IntegratedOperatorPOVAPOVBPublicAndMetrics` | O1=O2 world/revision; delivered A=1, B=0, public=0, audit refs present |
| A02 Pending perception barrier | `TestCSFRV19O4IntegratedOperatorPOVAPOVBPublicAndMetrics`, `TestCSFRV19O2ReportsPendingPerceptionTasks` | pending task detected from durable outbox, not inferred from an absent event |
| A03 Abort + explicit retry | `TestCSFRV19O4AbortRetryPendingAndPublicRemainExtracausal`, O2 abort/retry fixture | AUTHORIZED retry and actual abort refs; world clock/hash unchanged |
| A04 No persistent read side effects | O4 same-run digest of **all** application tables; 6×4 concurrent query permutations; O1/O2/O3 suites | no new receipt, evidence, cursor, snapshot, clock, event or ledger bytes |
| A05 Sidecar isolation | `TestCSFRV19O4MetricsReadOnlyIsolatedSidecar`, `TestCSFRV19O4MetricsNeverOpenCausalDatabaseForWriting` | causal sentinel and schema unchanged; mode=ro, query_only and correct table targeting |
| A06 Dangling/parent symlink attacks | `TestCSFRV19O4RejectsMissingCausalTargetThroughPendingLinks` | no causal SQLite file created before bootstrap, legitimate independent sidecar succeeds |
| A07 Idempotence, bounded deterministic reads | metrics sidecar test, 25 repeated read queries and 20 concurrent records | exact-order read, collision rejection, no silent truncation |
| A08 Corruption/error atomicity | `TestCSFRV19O4RejectsCorruptAndNoncausalPayloads` | after first good record, a corrupt second record yields **nil report + error**, then restoration succeeds |
| A09 No extra authority in contracts | `TestCSFRV19O4MetricsArchitectureHasNoCausalWritePortDependency`, `TestCSFRV19O1QueryPortCannotImportWritableAuthority`, O3 method-set tests | only explicitly permitted imports/reader methods; no actor or core write ports |
| A10 RED/GREEN | [expected RED checkpoint](https://github.com/carloshgalves/cote-ai-simulation/commit/864d653cfa33d9a678a04788992bd07c031af347) plus latest O4 Actions | RED must compile-fail on missing O4 API, GREEN runs all assertions |
| A11 Race and full Go | O4 GitHub Actions `focused`, `all-go` | `go test -race -count=1`, `go test -count=1 ./...`, `go vet` |
| A12 V1/V2 and two-language 621-case conformance | O4 Actions `causal-continuity` and `canonical` | fork negatives + V1/V2 race; independent Go/TS reports exactly equal |
| A13 Formatting and patch integrity | O4 exact HEAD `gofmt -l`, `git diff --check` | test the **committed** bytes, never rewrite a source file within a validation job |
| A14 Review | PR #92 individual `cote-review-finding:v1` comments | every actionable finding persisted, corrected or explicitly justified, re-reviewed |

## Known exclusions and decision gates

- **No registered public publisher:** empty public projection is expected and secure; no visible world event becomes public through the confidentiality bit.
- **No active production metrics wiring:** Recorder accepts measurements **only when the trusted local host supplies them**. Closure/dispatch/task wall ages, real host latency, model cost, queue samples, rejected-response counts and digest alerts are `NOT_INSTRUMENTED` unless reported by the host. In particular, O2 rejected ingress remains `NOT_PERSISTED` and must never be guessed from samples.
- **No privileged→actor flow:** A/B projections derive from addressed receipts, never the privileged report or global event store.
- **Performance:** O1–O3 replay validates complete causal history per query; O4 does not introduce an index/cache or promise high-frequency polling of large runs. Sidecar sample query is indexed and bounded.
- **Review independence:** self-review plus 0 outstanding actionable findings is not an independent reviewer submission; treat review gate separately.
- **Release:** **GO for local Observatory only after all exact-SHA gates and human acceptance**, not an automatic merge; **First Real Run remains NO-GO** until character agents, executable exams and other remaining integration gates are delivered. No LLM, UX, web service, background workers or new command port is created by O4.

## Evidence integrity

Use CI **job-level** results on the same final SHA. Never label a queued/cancelled/in-progress run as PASS. When a runner never executes steps, use `CI_NOT_RUN`, not a test failure or billing inference. Confirm tests, negative controls, no-write proof and findings again after any new commit.
