# CSFRV1-10/C3 — real INDETERMINATE, atomic abort and authorized retry

**Issue:** #128; child of #117 under epic #114. **Dependency:** merged #116. This is an **experimental, local, same-RunID** extension of the genuine B reference run (optional combined C1), not a second toy database or a substitute validator.

## Reproduce

```bash
go run ./cmd/csf-reference-run --db /tmp/csf-c3-a.db --report /tmp/csf-c3-a.json --abort-retry
go run ./cmd/csf-reference-run --db /tmp/csf-c3-b.db --report /tmp/csf-c3-b.json --abort-retry --communication --reverse-sources --reverse-responses
go test -count=1 -v ./internal/csf/commit -run '^TestReferenceC3'
```

All CLI `--db` and `--report` paths must be fresh. `--communication` continues the **same** post-retry run through C1 Claim publication and delayed delivery: **six real commits**, one RunID, zero final resource stock. Without it the authentic B runtime makes three actual commits. **Do not combine `--epistemic-owner` with `--abort-retry`: joint C2+C3 acceptance belongs to C4 #129.**

## Execution and trust boundaries

1. Build the exact verified B Genesis with one last physical stock, two registered exogenous source closures, two simultaneous school A/B RoundDeclarations, actual SlotDispatch/SlotResponses and the original committed **AdmissionFence attempt 0**.
2. The real `decision.FromPersistedFence` + `decision.Run` evaluates both source-backed `resource.consume` proposals against an **unknown validator accessibility facet** (`Snapshot.Known=false`). It must return `ErrIndeterminate` and two authenticated per-subject proofs, with **zero final Disposition**. This is simulated epistemic uncertainty, *not* database corruption or arbitrary exception injection.
3. The actual `Coordinator.Abort` and SQLite `CommitCycleAbort` atomically append **CycleAbortRecord** and `HALTED_ON_ABORT` to the frozen fence. No final DecisionRecord, CycleCommit, EventBatch or WorldRevision succeeds. The original world revision stays 0 and stock stays 1. An interrupted abort transaction is **not** treated as an authorized retry.
4. Close the writer SQLite handle and independently reopen the same database and InputLedger. `ReplayClock` must reconstruct zero committed cycles and the `HALTED_ON_ABORT` control state. An attempt-1 AdmissionFence with no `AttemptRetryRecord` is rejected without changing control state.
5. The harness enrolls a fresh Ed25519 operator public key **before** the validator and stores its private key only in protected process memory for the demo. After the SQLite reopen a scoped canonical grant covering **run, cycle, abort ID + digest, original fence ID + digest, pinned codec/schema policy hashes, retry reason and next attempt=1** must verify against that *pre-enrolled local operator public key*. Wrong/fabricated signature fails, leaving the run halted. The trusted coordinator then emits a normative, durable `AttemptRetryRecord` in SQLite, transitioning to `RETRY_AUTHORIZED`. A second retry is rejected. The grant's public key, signed canonical bytes, signature and digests are included in the report so the authorization is independently verifiable without revealing the secret signing key.
6. Close and reopen SQLite **again**; verify `RETRY_AUTHORIZED` survived. The new attempt 1 follows the original run/cycle/instant/ordinal/base revision/world hash, canonical source cohort, input digest and pinned rules verbatim. Only attempt ordinal and authenticated `retry_ref` differ. A fresh `AdmissionFence` commits and the normal resource contest resolves one winner and one loser, followed by the original triggered system flow. The source, operator, and worker order may not change causal records.

**Operator security qualification:** The Ed25519 **test** operator is generated with OS randomness rather than derived from the public RunID; an actor cannot provide arbitrary “operator.retry” to authorize itself through the exposed CLI. The key is **ephemeral, pre-enrolled only in this reference harness**, not durably enrolled in a production trust registry. The signed *grant* is independently checkable, but the normative `AttemptRetryRecord` does not include that grant (it is authorized by the trusted Coordinator). This is not an external operator-authentication, OS-process crash/recovery, or production security certification. Those require separate authority/policy and #119/#123 release gates.

## Adversarial regressions

```bash
go test -count=1 -v ./internal/csf/commit -run '^TestReferenceC3'
```

- Real C3 + C1 continuation (six commits); initial unknown facet, exact abort ledger, no provisional output, stock unaffected until explicitly retried, one authorized retry.
- Four source/worker order permutations, comparing precise EventStore bytes and normative abort/retry IDs.
- Four SQLite failures: abort stages 1–2 and retry stages 1–2. Assert no commits/events/revisions; abort rollback leaves `ATTEMPT_IN_FLIGHT`, retry rollback leaves `HALTED_ON_ABORT`. Never turn a failed SQL call into a legitimate `AttemptRetryRecord`.
- Signature forgery and unauthenticated attempt 1 block; public grant signature and canonical abort digest verified from stored records.
- Mutated committed CycleAbortRecord and AttemptRetryRecord each cause replay to fail closed. Missing/changed cause and frozen cohort cannot bypass the native fence/retry guard.
- Existing B/C1/C2, abort/retry and checkpoint/context boundary regression suites run on the same PR head.

C4 #129 still owns combined C1+C2+C3 scenario acceptance and cross-fault matrix. First Reference Run, A1 attestation, full 41/68 invariants, and external release gates remain **NO-GO**. Draft PR; merge strictly by human approval.
