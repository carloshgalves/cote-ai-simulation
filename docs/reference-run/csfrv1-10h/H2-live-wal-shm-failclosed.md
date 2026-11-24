# H2-W — fail-closed WAL/SHM inspection before private detached replay

**Issue:** [H2 #177](https://github.com/carloshgalves/cote-ai-simulation/issues/177), parent #122. Based on human-merged H2-F [PR #182](https://github.com/carloshgalves/cote-ai-simulation/pull/182), `main@e9fb4ad4d03b256d26943cd90994aa1416d5ce2f`, 17/17 exact-HEAD CI passed.

## Threat and trust boundary

SQLite is allowed by its documented WAL recovery algorithm to disregard corrupt, incomplete or stale frames and reconstruct the transient `-shm` index. That is acceptable for its database-recovery role, but a **cold audit of a supposedly complete, operator-private causal archive** must not mistake silently discarded bytes for verified archived evidence. SHA256 indexes alone are mutable by the same local operator, so simply rehashing a corrupted sidecar must not bypass semantics.

The new `verifySQLiteSidecarsH2` runs **BEFORE** both detached database-opening entry points: `replayAndGenesis` (Genesis/ReplayClock) and `detachedDBH2` (canonical ledger export, owners, snapshots, second-run typed table oracle). It reads only the original private regular files under the existing 0600/no-hardlink/no-symlink rules and applies a complete independent, bounded WAL parser:

- Verify SQLite main file magic, page size and WAL mode.
- Accept a **zero-byte WAL after a real SQLite checkpoint** as an empty durable sidecar (the full signed nine-commit replay must still succeed). Reject nonempty incomplete/zero-frame WAL, unknown 0x377f0682/0x377f0683 header magic, incompatible WAL version/page size, and invalid header checksum.
- Validate **all frames** (24-byte frame header + SQLite page contents), page number, exact header/frame salt pair and cumulative SQLite rolling two-word checksums with the correct checksum byte order. At least one durable commit marker is required. Reject any trailing partial, corrupt or stale frame — do not let SQLite silently accept an earlier valid prefix.
- A present `-shm` must be a complete 32768-byte shared-memory mapping region (or multiple). Reject an orphan SHM when WAL is missing, or a truncated region. **Do not** treat SHM as durable causality: genuine WAL-only capture is allowed, because SQLite may reconstruct an absent transient WAL index.
- On acceptance, the existing detached SQLite recovery plus `ReplayClock`, signed receipts, five canonical ledgers, sealed snapshot and both private owner DBs remain mandatory. No new authoring/admission API; no write to original archive.

## Tests

`TestH2LiveSQLiteWALProvesCommittedFramesBeforeDetachedOpen` uses genuine active SQLite with a baseline checkpoint and **new committed, uncheckpointed WAL transaction** while the writer remains connected. It verifies full WAL frame checksum parsing, that detached read-only SQLite sees the pending committed row, original WAL bytes remain identical, and absence of a disposable SHM does not invalidate a valid WAL.

`TestH2LiveSQLiteWALTruncationsAndTamperedFramesFailBeforeRecovery` makes private copies of that genuine active WAL/SHM, then corrupts header/trailer/checksum/salt/page or truncates SHM, refusing each **before the detached SQLite copy can open**. The test also rejects an orphan SHM when the WAL disappears. Whole local-index rehash cannot help because parsing is independent of SHA256.

The initial exact-HEAD CI found a legitimate post-checkpoint WAL sidecar that the new strict parser wrongly treated as a truncated WAL. The correction permits precisely the **zero-length** form while keeping 1..31 nonzero header lengths and incomplete frames invalid; a new real SQLite checkpoint fixture checks that both committed rows remain recoverable. This does not exempt the archive from the nine-commit Genesis/replay, ledger, owner and receipt checks.\n\nThe existing integrated real two-epoch A1 + D3 H2 archive's semantic mutant matrix gains two variants: an injected corrupt causal WAL and a truncated SHM mapping, **with freshly recomputed complete `checksums.json`**. The real cold verifier must reject both at the WAL semantic gate, bringing the matrix from 15 to 17 if CI passes. Go normal/`-race`/vet/gofmt and genuine Python A1/second Go D3 remain required in the H2 private job. No original database, raw source or cognitive material is uploaded.

## Deliberate limits

This is a **strict, fail-closed archival format**, not a general-purpose SQLite crash-recovery implementation. SQLite can legitimately ignore trailing invalid frames after a crash and rebuild SHM; this H2 audit deliberately refuses such an archive instead of silently repairing it. An isolated, valid WAL with no SHM is supported, and clean closed databases with neither or a checkpointed zero-byte WAL are still valid. The transient SHM contents are not claimed as signed causal authority. The standalone validator does not detect deletion of BOTH WAL and SHM; the full H2 signed-receipt, 9-commit ReplayClock and archive inventory remains responsible for identifying missing authoritative state.

**Still open:** injecting a stop between fence/commit in the **specific** H2-I archive pair; complete runbook and exit-status taxonomy; independent external attestation/immutable anchor; auxiliary chronological evidence and G6 normative coverage. #177/#122/#121/#114, #93/#94/#113/#123 remain open. G6 remains `NO_GO_INCOMPLETE_MATRIX` (50 NOT_RUN and 8 BLOCKED). Human-only merge; no production approval.
