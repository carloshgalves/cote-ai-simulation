# B1 — Owner cognitive checkpoint (local SQLite proof)

**Status:** B1 implementation under review in [#76](https://github.com/carloshgalves/cote-ai-simulation/issues/76). Not production-ready, not remote/host durability, not the B2 full CSF `Gate` integration.

`localowner.Open(ctx, dbPath, ed25519.PrivateKey)` returns a `DurableOwner` implementation. It owns an **independent SQLite file**, not the causal CSF file. Supply a pre-existing private directory (mode 0700) and keep the database (0600) outside the repository/corpus. The signer is provided externally; the private key is **never persisted** to the database or CSF. Losing it prevents valid restoration. No key rotation, tenant ACL, storage encryption, backup or remote availability is supplied by this package.

The owner pins one verified connection and requires `journal_mode=WAL`, `synchronous=FULL` and foreign keys, using the repository's pinned `modernc.org/sqlite`. A successful `PersistAndSign` means the SQLite commit returned **before** the owner signed. Retries of the exact actor/run/coverage/bytes return the same locator and signature. A conflicting checkpoint for the same actor/cut is rejected. The locator is an opaque derived `machine-id`, not a path.

`Restore` retrieves and authenticates the requested exact bytes using scope, locator, content hash and receipt identity. `Acknowledge` accepts the matching receipt/snapshot/hash only following a successful `Restore` **in this live process**; it durably records ACK with an idempotent insert. A new process has no remembered restoration, even if an old ACK exists. It must restore again. **ACK alone never authorizes WORK:** the outer `ownercheckpoint.Gate` verifies the actual CSF snapshot and its tip before granting a permit. B2 wires this path into a dedicated synthetic host.

## Tests

The tests in `sqlite_test.go` cover commit-then-sign and re-open, Ed25519 identity, idempotent retry, session-bound ACK, cross-run/actor/locator/receipt/hash refusal, foreign owner key, tampering, concurrent retries and insecure path refusal. B3 separately owns crash points between owner/CSF commits, process boundaries and V1→V2 forks. B4 owns consolidated release evidence.

**Not guaranteed by B1:** proof of OS-level power-loss durability, host loss, cloud storage flush, filesystem path race resistance against an attacker already executing on the host, multi-process distributed fencing, tenant authorization, envelope encryption, privileged CSF endpoint authorization or LLM exactly-once calls.

Do not use this package with real private memory without separately approving encryption, ACL, backup and key policy. See [ADR 0013](../../../../docs/adr/0013-cognitive-owner-durability-s1.md) and [S1a plan](../../../../docs/validation/cognitive-owner-s1a-2026-11-12.md).
