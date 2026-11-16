# CSFRV1-10/B — real SQLite causal tracer bullet (#116)

**Status:** local synthetic execution only. This is not the full First Reference Run, a production physical A1 run, or independent validation of all 41/68 normative items. No LLM/network/cloud provider is called.

## Execute on a fresh directory

```bash
go run ./cmd/csf-reference-run --db /tmp/csf-116-first.db --report /tmp/csf-116-first.json
# A second independent run with source and worker completion orders inverted:
go run ./cmd/csf-reference-run --db /tmp/csf-116-reversed.db --report /tmp/csf-116-reversed.json --reverse-sources --reverse-responses
# Focused integrated tests, including exact canonical-byte comparisons and the 7 commit fault-injection points:
go test -count=1 ./internal/csf/commit -run '^TestReferenceRun' -v
```

Use **fresh filenames**, not existing production SQLite databases; the CLI refuses overwrite. Run only in a disposable working directory under the pinned Go 1.27.2, Python 3.13.5 reference toolchain and exact V1 policy bundle.

## Proven causal sequence

The command authenticates a deterministic canonical Genesis with one stock unit and an immutable schema/policy bundle, and creates a fresh SQLite. Two declared sources (`school-events`, `physical.a1`) each supply authoritative partial `(0,0)` and final `(10,0)` closures. The name `physical.a1` reserves the canonical ID but **does not** authorize an A1 Python producer or insert an A1 physical input: signed A1 integration belongs to #118. Two simultaneous rounds `R1/R2` at base revision 0 submit real `ActionProposal` responses, optionally in reversed dispatch-completion order. `AdmissionFence` uses both closure proofs and both slot records; the deterministic conflict resolver persists exactly one `COMMIT` and one `REJECT`, with no hardcoded winning actor.

The *same* database then receives a committed clock advance from 0 to 10, creating exactly one durable clock-predicate `RISING_EDGE` trigger activation. At `(10,0)` that trigger and a nonrecurring scheduled occurrence enter one system `AdmissionFence` and are terminally settled in a second WORK commit. This is a **clock trigger**, because the current registry supports clock-only predicates; a resource-depletion predicate from the declarative #115 plan is **BLOCKED_RULE** pending a versioned, approved domain trigger policy. No new rule is invented here.

The result JSON exposes only run metadata, canonical fence IDs/digests, journal hashes, revision and logical coordinate progression, resolved actor winner/status, final world hash, replay counts and table names for the existing five ledgers. SHA-256 values over raw exported bytes are diagnostics, *not* substitutes for normative CBOR identity. Raw event bytes are compared in Go tests and omitted from public JSON. All SQLite commit/trigger/decision/event writes go through the existing coordinator, not a shadow harness store.

## Evidence / limits

- `RunReference` verifies world availability falls from one to zero, exactly one winner and one loser read back from the persisted `DecisionRecord`, trigger activation + occurrence materialization and settlement, and a `ReplayClock` rebuilt entirely from SQLite authority.
- Negative cases prove that an unclosed source blocks the fence; final closure cannot accept new closure; duplicate destination path is rejected; seven internal `CommitWork` failpoints do not publish partial world/event/commit state; permuting source and slot response ordering leaves fence, journal and event bytes equal.
- Phase #117 owns false claims/secret provenance and abort/retry. #118 owns physical A1 attestation; #119 owns full crash/perception, snapshot/resume/fork equivalence; #120 Observatory; #121 integration matrix; #122 one-command complete reference release; #123 independent review. None of those gates are silently marked PASS.

Only a draft PR and human-authorized merge are permitted.