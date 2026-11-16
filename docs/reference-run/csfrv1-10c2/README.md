# CSFRV1-10/C2 — addressed evidence, privacy and durable cognitive-owner recovery

**Issue:** #127, child of #117 (epic #114). **Dependency:** human-merged C1 #126. **Status:** experimental local proof only, no production trust attestation and no LLM.

## Real single-run command

```bash
d="$(mktemp -d)"
mkdir -m 700 "$d/private"
go run ./cmd/csf-reference-run \
  --db "$d/causal.db" \
  --owner-db "$d/private/owner-b.db" \
  --report "$d/report.json" \
  --epistemic-owner
```

The command requires fresh files and a private `0700` owner directory. It creates **two separate owner files**: `owner-b.db` and `owner-b.db.actor-a`. Each is a real `localowner` SQLite store with its **own signer**, separate from the authoritative `causal.db` SQLite. Original `--communication` / 10C1 mode remains supported. Owner DB contents are not returned in JSON; the report exposes only opaque identities/digests and boolean checks.

This is intentionally **the same six-cycle B+C1 RunID**: resource contest, clock advance/trigger, message send at t=10, clock advance to t=35, delayed delivery. The stock remains zero; `resource.stolen` remains an unverified `Claim`, never a World Truth event. There is no new causal publisher and no replay-generated fictional KnowledgeInput.

## Exact implementation sequence

1. **B-only perception:** Complete the two-source C1 run; use the actual causal `communication.delivered` PerceptionTask and its single authorized Observation/KnowledgeInput. `received_at` is strictly the causal delivered time `t=35`, not the wall clock. Pre-delivery checks from C1 verify no new message KnowledgeInput while `IN_TRANSIT`. `CheckPerceptionBarrier(run,6)` is mandatory before the next dispatch.
2. **Separate actor capabilities:** Open real next-round `RoundDeclaration` / `SlotDispatch` for both `school/a` and `school/b` at revision 6. `BindActorInbox` is a sealed, holder-only capability; `contextboundary.Build` accepts B's delivery but rejects A pretending to be B and denies public/unknown readers. No global raw `EventStore`, privileged Observatory state, future canon, engine-only resource event or physical state is placed in an agent context.
3. **Owner escrow:** Addressed evidence makes both A and B eligible snapshot holders. Register **both** actors with distinct public Ed25519 keys. Build one B checkpoint from the authenticated B task/input IDs and time; build an independent A checkpoint from A's addressed input IDs and a synthetic A-only private memento. No A-only memento appears in B's private context or the JSON report.
4. **Durable independent commit and publication:** Each `localowner.Store` (separate `0600` file in a `0700` directory) commits its own private bytes **before** signing. `ownercheckpoint.Gate.Stage` escrows each receipt in causal SQLite; one `CreateSnapshotV1` freezes both eligible holders plus all **five canonical ledger prefixes** at revision 6. `localdemo.Host.StageSnapshot` is not used because its single-checkpoint shortcut is insufficient for the two eligible actors.
5. **Real reopen and ACK:** Close the causal and *both* owner SQLite handles; reopen all independently. Neither new host may execute WORK before `RestoreAndAcknowledge`. A missing/forged snapshot, foreign run or actor, mismatched A/B receipt or different signer is rejected. Exact signed snapshot restore plus durable ACK permits the scripted B cognitive callback only in B's holder-only context, and independently permits the A callback only in A's.
6. **Idempotency:** A repeated registered `DeliverCommunicationPerception` changes no evidence counts; barrier, immutable snapshot verification, five-ledger prefixes and deterministic ReplayClock remain valid. No duplicated Claim, Observation, KnowledgeInput or CycleCommit is created.

**Determinism:** for this fully local synthetic reference **only**, per-actor Ed25519 seeds derive deterministically from the canonical RunID and different test labels. This makes signed snapshot IDs/receipts byte-comparable under source/worker permutations. **These deliberately reproducible demo keys are NOT production secrets or a release trust root.** Never use them for a deployed owner. Real external signer management, sandbox attestation and independent audits remain future gates.

## Tests and negative controls

```bash
go test -count=1 -v ./internal/csf/commit -run '^TestReferenceC2'
go test -count=1 ./internal/csf/ownercheckpoint/... ./internal/csf/contextboundary/...
```

- Same RunID/SQLite with six actual commits, accepted B KnowledgeInput at t=35, next-round dispatch guard and two signed owner receipts.
- Four worker/source permutations with exact EventStore, SnapshotID and both owner receipt IDs.
- Missing owner path and same-file owner/causal refusal before any causal mutation.
- Wrong actor and foreign run, swapped owner receipt, missing snapshot and unacknowledged callback rejection.
- After restart: externally tampered owner checkpoint content and wrong Ed25519 signer fail closed with zero cognitive calls.
- Deleted B Observation route and deleted PerceptionCompletion block the barrier/replay and next dispatch.
- Re-delivery of the same causal task is idempotent.
- Legacy C1, B and owner-checkpoint regression suites are executed in `.github/workflows/csfrv1-10c2-epistemic.yml`.

## Remaining boundaries

This proof does **not** demonstrate generalized inference/belief formation, true private SaaS tenant isolation, real LLM/RAG, signed A1 physical input (#118), INDETERMINATE/abort/retry (#128 C3), cross-scenario integration (#129 C4), full recovery/fork matrix (#119), Observatory (#120), all 41 normative invariants and stress cases (#121), First Reference Run release packet (#122) or independent approval (#123). No merge without explicit operator authorization.
