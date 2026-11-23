# H2-L — five authenticated private canonical ledger streams

H2 issue #177; parent #122; H1 [PR #176](https://github.com/carloshgalves/cote-ai-simulation/pull/176) is merged into `main@535df9354c327b94626193509a4eb5c23f6a8ca1`.

## Bounded scope of this slice

The original one-command A1 two-epoch D3 execution now creates `ledger-export/` inside its **0700 private archive**. Five stream files contain the exact persisted CBOR from `input_ledger`, `decision_ledger`, `event_store`, `evidence_ledger` and `causal_outbox`, in authoritative `ORDER BY id` order. Each stream is framed by an explicit H2 domain marker and ledger name; records carry 32-byte IDs, unsigned big-endian lengths and the **unmodified** raw canonical bytes. The file is *evidence only*, not a second storage authority. Invalid CBOR, empty ledgers, duplicate/unsorted IDs and oversized records/streams fail closed.

`ledger-export/ledger-proof.json` binds the registered five ledgers to the exact source commit, RunID, installed codec policy hash, record counts, byte-level SHA256 of private export frames, and the original **domain-separated CSF ledger-prefix digest** `cote.csf.digest.ledger-prefix / cote.csf.schema.ledger-prefix-digest-preimage`. Do not mislabel a plain SHA256 of a file as a canonical CSF prefix proof. All seven new files are added to the H archive's fixed inventory, protected by the original per-file sha256, size, private directory constraints and no-hardlink/no-symlink checks.

`--verify` reconstructs a *fresh detached* SQLite copy, including any WAL/SHM, opens it with `mode=ro` and `query_only=ON`, then independently reselects and revalidates the five ledger streams and compares **all original private bytes** plus counts and both digest families against the H2 proof. It also retains H1's replay, Genesis, registered producer, source SHA, disclosure and private owner-file hash requirements. It never calls Python again, interprets private memory, uploads the records, or grants a production pass.

The `ledger-proof.json` also records the actual C4 snapshot identity, sealed revision 6 and three D2 tail commits. On each new detached DB, H2 invokes the original `VerifySnapshotV1` over the snapshot body, source/seal, prefix members, registered owner roster and causal replay — **not** a self-reported `snapshot_verified=true` field.

`ledger-export/owner-audit.json` binds the same RunID and source SHA to a separately recomputed **two-owner check**: read the causal `cognitive_owner_receipts` and independently open both private owner SQLite databases on fresh detached copies; verify the original opaque checkpoint content/hash, owner key equality, Ed25519 signature over `CognitiveCheckpointSigningBytesV1`, exact signed receipt ID, persisted owner restore ACKs and corresponding sealed snapshot IDs. The private cognitive payload is used only in memory for validation; the report carries **counts and booleans**, never private content, actor names or signing secrets. The existing C2 reference uses **synthetic deterministic owner signing keys** for local repeatability; verified signatures demonstrate the reference trust boundary, **not** the possession or security of production cognitive-owner keys.

An adversarial CI test opens a **real captured** archive, forges a different owner checkpoint, the sealed snapshot digest and a canonical ledger frame in three private copies, rebuilds the whole top-level `checksums.json` to simulate a compromised local hash list, and requires rejection by the **semantic owner/snapshot/ledger validators**, not merely by mismatching SHA-256. All three mutant archives are discarded.

Older H1 archives without a `ledger-export/` directory retain their H1 verification semantics. **They are not upgraded to H2**. New real commands must create and verify H2-L evidence before printing the experimental success marker.

## Execution

```bash
go run ./cmd/csf-reference-run-local --out /tmp/csf-h2-unique-private-dir
go run ./cmd/csf-reference-run-local --verify /tmp/csf-h2-unique-private-dir
```

The archive is private. **Never** upload its full contents to GitHub Actions artifacts, issue comments or a public PR; only sanitized CI status can be shared.

### Red / negative controls

The H2-L tests reject unregistered tables, malformed CBOR, nonunique/unsorted IDs, symlink exports, absent roster entries and modified privately stored evidence. The H2 CI actually generates all five ledgers from a fresh supervised A1+D3 run, runs Go normal/`-race`, and mutates a private ledger export; the resumed verifier must reject it.

### H2 issue #177 remains open

This slice **does not** yet export a standalone full historical snapshot/member/trigger/source dataset, persist the two A1 signed cut source bytes, or replay the **same** signed A1 cuts through a second independent RunID. The two owner checkpoint contents/signatures and sealed C4 snapshot have been independently verified on detached private copies, but their proof does not constitute a portable externally signed attestation. These requirements remain H2 work, not implicit PASS. No clean equivalent can be inferred from comparing two separately captured Python sources with distinct genuine `wall_time` values.

G6 is still `NO_GO_INCOMPLETE_MATRIX`: 50 normative `NOT_RUN`, 8 `BLOCKED`, #22, #168/#169/#172/#174 and independent audit gates #93/#94/#113/#123. Merge is human-only after full exact-SHA CI and independent review. The H2-L implementation neither closes parent #122 nor authorizes the engine in production.
