# CSFRV1-8 / #58 — S0 external cognitive owner handshake

**State:** integration contract and fail-closed in-process work gate. **NOT a production integration, NOT remote-durability certification, NOT a new source of world truth.** The CSF V1/V2 canonical codec, hash contracts, owner-signer registration and SQLite escrow remain unchanged.

## Who owns what

The owner application exclusively controls private memory, beliefs, persistence, backup, keys and interpretation. CSF owns its causal run, immutable signed receipt escrow, actor registry, snapshot seal, replay and fork lineage. The new `internal/csf/ownercheckpoint` package is a **host-side orchestration boundary**, not the owner itself. The owner must supply a concrete `DurableOwner` implementation; this PR supplies only an injected fake for fault tests. A successful Ed25519 signature is evidence of the key owner's attestation, not cryptographic proof that remote hardware flushed bytes.

### Ordered protocol (normal and crash recovery)

1. Host registers an actor and its immutable Ed25519 public key in CSF via `RegisterCognitiveActor` and `RegisterCognitiveOwnerKeyV1`; no private key is sent to CSF.
2. Host calls `Gate.Stage` with actor/run scope and a `SnapshotCheckpoint` of **opaque** bytes, before publishing a snapshot. This revokes older in-process work permits.
3. `DurableOwner.PersistAndSign` must **commit to external durable storage before signing** `CognitiveCheckpointSigningBytesV1(run,checkpoint,locator)`. The owner may only return after the durable commit; a retry of the same identity and bytes must be idempotent. The external owner alone controls signing.
4. CSF `PersistCognitiveCheckpointV1` verifies the registered public key and escrows bytes, signature, digest and locator durably in SQLite WAL/FULL. An invalid signature, run, actor, locator, or conflicting same-cut state is rejected.
5. Host publishes the snapshot with the same checkpoint via `CreateSnapshotV1`; snapshot publication internally verifies receipt, actor roster and coverage. **Stage never authorizes WORK or publishes a snapshot**.
6. After a restart/fork, `Gate.RestoreAndAcknowledge` first calls `VerifySnapshotV1`, which authenticates the sealed historical cut and tails. It loads the actor's signed escrow via `LoadCognitiveCheckpointV1` **and proves that the exact actor/store/schema/content/hash/producer/coverage reference appears in the sealed snapshot's canonical epistemic roster**, refusing individually valid but unrelated receipts. Only then does it request external `Restore` using actor/run/locator/receipt/hash, compare the externally returned bytes to immutable CSF escrow, and require external durable `Acknowledge`.
7. Only then does `Gate.RunWork` permit the host callback, scoped to the **exact run, actor, snapshot ID and receipt ID**. Every restart begins with no permits; a failed retry revokes the existing permit, and a newer Stage/Restore supersedes an older one, with a generation fence held across SQLite escrow to prevent a delayed old persist. The gate holds a read lock across the callback, so Stage/Restore cannot revoke mid-dispatch. Callbacks must not re-enter Gate.

**Constraint:** a host that calls the LLM, worker or `Coordinator` directly can bypass this new wrapper. Therefore this PR does **not** claim integration across all runtime entry points. A production adapter/orchestrator integration is a later S1/S2 slice of #58, with independent audit and tests proving that no unguarded dispatch path remains. Integration must also ensure owner-side private-memory access control, tenant isolation and authenticated channels.

## External owner API responsibilities (required for future adapter)

| Action | Security and durability contract |
| --- | --- |
| `PersistAndSign` | Authenticated actor/run; atomic or recoverable durable write; monotonic immutable locator; explicit idempotent same-cut retry; never sign before flush; signing key stays with owner/KMS |
| `Restore` | Authenticated actor and locator, no anonymous reads; load the persisted checkpoint (not current volatile state), rehydrate into owner runtime, return exact bytes; on hash mismatch fail closed |
| `Acknowledge` | Durable/idempotent evidence the addressed actor has restored exact receipt/snapshot/hash; no model invocation before successful ACK |
| Owner key change/revocation | Not supported by current immutable owner roster: explicit versioned trust migration and operator approval are required; do not swap keys silently |
| Backup and disaster recovery | External backup retention, replication/restore test, loss policy, recovery window/RPO/RTO and owner-of-record must be declared for production; not simulated by SQLite escrow |

### Crash and negative matrix

The fake owner/escrow tests intentionally inject errors before persist, after durable write before signing, forged signature, wrong locator, escrow failure, missing sealed snapshot, external restore failure, corrupted provider bytes, missing escrow, lost ACK, cross-actor/run/snapshot/receipt reuse, competing/concurrent stage (including delayed owner persist), and a valid receipt presented against the wrong canonical snapshot checkpoint. This includes a snapshot with missing, substituted or duplicated actor references. In every negative, the guarded model callback count must stay zero. Successful retry of the same signed receipt performs **restore/ACK only**, not another memory generation or owner persist. The existing R22/R24/R25/R26 tests continue to assert real SQLite receipt, snapshot, fork and replay semantics.

No HTTP endpoint, credential name, storage service, cloud deployment or latency guarantee is invented in this S0. The actual owner adapter, application work dispatch wiring and infrastructure recovery drill remain separate acceptances before marking #58 complete.

## Merge gates

- New `internal/csf/ownercheckpoint` package compiles and its fault matrix passes.
- Five existing CSFRV1-5/6/7/8+conformance workflows pass on the **exact PR HEAD**, including `go test ./...` and `-race` where configured.
- `gofmt`, `git diff --check`, backward compatibility for existing V1/V2 receipts and golden bytes.
- Adversarial review, fixes and re-review until no actionable findings in S0; no silent claim of external provider E2E.
