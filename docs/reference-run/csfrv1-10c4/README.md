# CSFRV1-10/C4 — Unified C1+C2+C3 reference acceptance

**Issue:** #129, final child of #117 under epic #114. **Prerequisites merged:** #126, #127 and #128. The C4 command is a *single real SQLite / RunID*, not three independent simulations pasted into a report. This local scripted harness has no real LLM, physical A1 device, external owner trust, or production attestation.

## Reproduce

```bash
d="$(mktemp -d)"
mkdir -m 700 "$d/private"
go run ./cmd/csf-reference-run \
  --db "$d/causal.db" \
  --owner-db "$d/private/owner-b.db" \
  --report "$d/c4-public.json" \
  --cross-acceptance

go test -count=1 -v ./internal/csf/commit -run '^TestReferenceC4'
```

The CLI writes **only a privacy-reviewed allowlisted JSON schema** (status, RunID, commit count, opaque delivered Claim IDs, private owner receipt IDs, retry root IDs, five-ledger digests/counts and verified booleans). The raw engine-oriented `ReferenceResult` is **never serialized** by `--cross-acceptance`; it includes privileged facts for local Go test oracles. The JSON report is fail-closed: an incomplete combined run does not produce a successful report. The SQLite paths must not exist before the command; owner directory needs `0700` and houses two distinct owner databases.

### Actual joint trajectory

| Temporal coordinate | Native transition | Causal security guarantee |
|---|---|---|
| t=0 / attempt 0 | Simultaneous two-source conflict, real validator returns two `INDETERMINATE` results | **No winner, no WorldRevision or EventBatch.** Exact `CycleAbortRecord` and `HALTED_ON_ABORT` |
| t=0 / restart | SQLite reopened and source cohort frozen | Unauthorized attempt 1 fails; no partial publication |
| t=0 / authorized retry 1 | OS-random **local reference-only** Ed25519 operator, signed grant, native `AttemptRetryRecord` and new AdmissionFence | Existing cycle, source members, policy and input digest unchanged; no second retry |
| t=0 / resolution | Native resource contest | One winner, stock 1→0, one CycleCommit |
| t=10 | CLOCK_ADVANCE + trigger system WORK | Second and third CycleCommit; epistemic completions closed before next actor dispatch |
| t=10 / actor A | Genuine RoundDeclaration, SlotDispatch, response, `communication.sent` | **Claim remains allegation**, sender presented as C, recipient B; no change to WorldState or B KnowledgeInput while in transit |
| t=35 | CLOCK_ADVANCE + due WORK `communication.delivered` | Exactly one B-only Observation/KnowledgeInput at logical `received_at=35`, six total CycleCommits |
| post-delivery | Two independent actor A/B owner checkpoints and one joint SnapshotV1 | Owner SQLite files separated and signed with distinct synthetic keys, ACK required after closing/reopening owner+CSF connections; barrier before next round |
| final verify | ReplayClock + SnapshotV1 + source/outbox + five-prefix byte digests | Historical abort/retry authenticated after control IDLE; five ledger hashes tied byte-for-byte to canonical SnapshotV1 |

**Privacy:** A's opaque private memento never enters B's addressed KnowledgeInput or the public projection; B's Claim input is not eligible for A or the public reader. Canon future and engine state are not copied into model-owned contexts. Actor and owner gate implementation is inherited from C2, then **re-exercised on the very same C3+C1 run** in C4. The public schema intentionally omits `final_stock`, `contest_winner`, world hashes, event bytes, claims' plaintext, private owner content, operator signing key and full privileged grant.

## Verify four permutations / negative controls

```bash
go test -count=1 -v ./internal/csf/commit -run '^TestReferenceC4'
go test -count=1 ./internal/csf/commit -run '^(TestReferenceRun|TestReferenceCommunicationC1|TestReferenceC2|TestReferenceC3)'
```

- Four independent source/worker permutations; compare original RunID, all five immutable ledger prefix digests/counts, EventStore bytes, SnapshotID, both owner signed ReceiptIDs and Abort/RetryIDs. A new ephemeral operator signature per run is intentionally **not** expected to match; its protected private key is not durable or in output.
- Cross-tampering of delivered B Observation route, B KnowledgeInput route, durable PerceptionCompletion and historical AttemptRetryRecord. Replay fails closed.
- Cross-fault injection of two abort transaction stages, two retry stages, forged operator/sender and failed due delivery. Verify no partially committed WorldRevision, Claim/Transmission delivery or owner material when an earlier gate fails.
- Restore/reopen/barrier checks cover both actors with private `0700` owner root and staged snapshots; a disallowed owner path is rejected before any C4 causal mutation.
- Independently rehash the five actual ledgers' canonical CBOR bytes; verify they **match the five prefixes sealed inside the exact joint SnapshotV1**. Audit live pending clock, scheduled occurrence birth index, terminal Transmission, the due Event and exact B recipient task/evidence IDs. Compare all settled world revisions 3..6 for unchanged resource hash.

## Scope / no implicit approval

Only #117/C1–C4 local experimental acceptance. **No** signed production A1 (#118), broad crash/fork recovery (#119), complete Observatory (#120), 41×68 eval matrix (#121), full First Reference Run packet (#122), independent human acceptance (#123), production operator PKI or real cognitive model. `--cross-acceptance` enforces a test-only signer profile and does not turn a synthetic owner checkpoint into a deployed agent. **No merge without an explicit human decision.**
