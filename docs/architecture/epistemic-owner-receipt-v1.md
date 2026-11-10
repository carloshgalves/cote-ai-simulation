# Epistemic Owner Receipt V1 — R22

**Status:** Proposed integration on CSFRV1-8 / PR #25. The canonical V1/V2 bundle and frozen digest operations are unchanged. This note is not an independent approval.

## Ownership and trust

- Causal Simulation Foundation (CSF) owns the journal, world truth, snapshot seal, eligibility roster, and five canonical ledger prefixes.
- The cognitive-memory owner alone owns the meaning, forgetting policy and persistence of private belief, memory and reflection.
- CSF treats memory content as opaque bytes. Ed25519 verifies an attestation by a registered owner; it **does not independently establish that a remote provider physically flushed its storage**. Owners must commit their own state durably before signing, and the host must enforce this operation order.

## Durable-before-reference protocol

1. Register a cognitive actor under the authenticated Genesis. Before the first causal snapshot of that run, pin exactly one Ed25519 public key with RegisterCognitiveOwnerKeyV1. Key changes or late trust retrofits are rejected.
2. The owner commits opaque state durably in its own store, then signs the output of CognitiveCheckpointSigningBytesV1. The canonical payload binds a fixed domain/version, origin run ID, actor namespace/id, store ID + schema, producer + version, coverage instant/revision/evidence cursor, SHA-256 of exact owner bytes via the registered identity adapter, and an opaque stable locator ID. A receipt from another run or another actor cannot be replayed.
3. PersistCognitiveCheckpointV1 verifies the owner signature and keeps the exact content bytes, digest, receipt ID, locator and signature in an immutable SQLite escrow. This happens **before** CreateSnapshotV1. Identical retries are idempotent; conflicting bytes for the same actor and coverage fail closed. SQLite runs under a verified WAL + synchronous FULL runtime.
4. CreateSnapshotV1 rechecks the escrow/signature, eligible actor roster and coverage **under the same SQLite writer transaction** that publishes the new snapshot and prefix seals. It does not regenerate memory.
5. VerifySnapshotV1 performs the same check on reopen, causal replay and historical resume. LoadCognitiveCheckpointV1 returns only the addressed actor's exact verified bytes and locator, without decoding them or calling an LLM. Caller/owner application must restore those bytes before any new model invocation.
6. A V1→V2 fork imports the source owner's public key and immutable signed SOURCE receipt into fork_cognitive_owner_proofs as part of the atomic child bootstrap. Child lineage replay verifies signature, binding to archived parent memory bytes, parent run, and the copied child signer key. Any new child snapshot must carry **a separate owner-signed receipt for the new child run**. The imported signer roster is sealed independently; **newly enrolled child-native actors and their signer keys remain allowed** and require their own eligible checkpoints, while deletion of any imported signer/proof remains a replay error.

## Trust and compatibility limitations

The signed path is mandatory for actors that enroll an owner signing key. Historical V1 synthetic/legacy inline checkpoints without a pinned key remain compatible; this fallback is **not production authentication**. Production orchestration must explicitly require signer enrollment for every externally persisted cognitive actor and prohibit dispatch before its owner has been rehydrated and acknowledged.

Signer attestation is not a network availability guarantee, a remote durability oracle, or authorization for a recipient to view private memory. The CSF adapter's load method is privileged infrastructure; a user-facing API must provide authorization before exposing those bytes. The owner signature/private key must never be stored in CSF or in this repository.

The V2 fork archive and owner key data live in SQLite migration authorities, not the canonical V1 record body. Current lineage binds the signature and original parent snapshot bytes but cannot independently prove a remote service's durability after its source store has vanished. Any production deployment must record external ownership/backup guarantees and signer key trust outside this database.

## Fault and regression cases

- Missing owner receipt, missing actor, forged signature, wrong signer/actor/locator/run.
- Byte-for-byte signed escrow verified after close/reopen and repeated restoration.
- Corrupted escrow rejected on resume; exact restoration recovers.
- Conflicting state at the same coverage rejected.
- Crash after owner escrow but before snapshot seal: incomplete snapshot transaction rolls back, escrow survives, retry publishes exactly one snapshot.
- Fork V1→V2 with signed parent, copied proof, no implicit child trust downgrade; child must re-attest under its new run.
- Tampered fork signature or copied signing key fails lineage/replay.
- R21/R23 integration must keep all five ledgers isolated; no model-generated replacement bytes.

**Release gates outstanding:** human-approved owner durability obligations, final differential/failure matrix, independent review, and exactly-HEAD full-suite/race evidence. A green CI alone does not approve PR #25.

## Snapshot signer-trust sealing (R22 security hardening)

Each newly created snapshot transaction additionally seals the **exact owner public-key roster**, including the empty roster, in `snapshot_owner_key_seals`. Both `CreateSnapshotV1` on idempotent retry and `VerifySnapshotV1` on replay compare the current run's owner key set to the snapshotted count/hash. Removing a key and its signed receipt can no longer silently convert a historically authenticated snapshot into the compatibility fallback. Missing seal or corrupted key rejects replay. V2 fork destinations must likewise be empty of owner-trust seals before bootstrap.

**Migration caveat:** SQLite database files containing snapshots created **before this trust-seal storage contract** have no `snapshot_owner_key_seals` records. They deliberately fail closed on the newer verifier; operator-reviewed migration or a compatible export must establish their prior unsigned/signed provenance. Do not silently synthesize trust seals for an existing snapshot.

## R24-01 — Late actor enrollment on later revisions

Owner-key enrollment is tied to the actor's durable `registered_revision`, not a global run-wide first snapshot. A new actor registered after advancing from snapshot revision R to R+1 may register a new immutable Ed25519 owner key and publish a signed checkpoint at R+1. A key cannot be introduced after any snapshot at or beyond that actor's registered revision. Snapshot trust roster verification compares the subset of signed owners registered **by the historical revision**, so older snapshots keep their original count/hash even after subsequent agents enroll. No change to the canonical snapshot bytes or V1 codec is required.
