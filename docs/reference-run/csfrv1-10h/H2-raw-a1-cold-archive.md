# H2-R — private raw supervised Python A1 source archive

**Scope:** H2 issue #177, parent H #122. Base is the human-merged H2-S PR #179. This slice records the actual *original* supervised A1 source bytes; it is **not** a second full independent Go simulation yet.

## Origin and authority

The Linux A1 supervisor's `ProducerHost.PrivateCaptureRoot` is an **optional, trusted host-owned absolute 0700 directory**, never a model/actor-supplied path. The ordinary producer remains unchanged when this setting is empty. For the one-command experimental reference run, the directory `physical-cuts/` is created privately *before* starting the C4/D2/D3 run.

After the Python producer terminates, its process group is ended and the original files are opened with the existing A1 `openat2` protections. The supervisor seals its source descriptors in Linux memfd, runs the Go A2 completion verifier, signs the exact source digests with the installed Genesis-pinned producer key and validates the signed source against the seal. **Only if all gates pass** does it persist the original `snapshot.yaml`, `events.jsonl` and `evidence.json` bytes, the canonical `cut.cbor` and the original `signature.bin` for each of epochs 1 and 2. Files are exclusive-create, owner-only 0600; directories owner-only 0700; each file and directory is sync'd. Reusing an epoch, symlinked archive root, exposed root, or failed write is a hard refusal before a new source is staged. The private signing key is never archived.

Directory layout (actual files contain ENGINE_ONLY source and must stay secret):

```text
private-run/
  physical-cuts/
    epoch-1/
      snapshot.yaml
      events.jsonl
      evidence.json
      cut.cbor
      signature.bin
    epoch-2/
      snapshot.yaml
      events.jsonl
      evidence.json
      cut.cbor
      signature.bin
    raw-a1-proof.json
```

## Independent cold verification

The mandatory new `run-manifest.json.evidence_profile = H2_RAW_SOURCE_V1` binds the archive layout; with this evidence profile retained, deleting `physical-cuts/`, either epoch, any of its ten files, or the raw proof is a hard failure, **even if the local checksum file is recalculated**. A coordinated attacker who can rewrite the *manifest profile itself* and the whole local archive could roll back to a valid older evidence profile; preventing that requires a separately retained, immutable external auditor attestation and remains an unresolved #177/#123 requirement. All files are indexed by length and SHA256, alongside H2's original five-ledger and owner/receipt evidence. Older profiles remain verifiable under their original, narrower claims, but are not silently promoted to H2-R.

The fresh detached read-only verifier:

1. Revalidates the installed private physical registration and persisted canonical Genesis.
2. Independently authenticates the 8 durable `physical.a1` ingress receipts, two signed epochs, six ordered source closures, signatures and completed bodies from the original InputLedger (H2-S).
3. Reads the **actual original** three Python source files per epoch, enforces their size limits and compares their original bytes' hashes to the signed canonical cut.
4. Places the raw snapshot and eventlog bytes in *fresh ephemeral F_SEAL_WRITE/SHRINK/GROW/SEAL Linux memfds* and invokes the original Go `InspectSignedSealedUnadmitted` with the installed Genesis and raw A2 evidence. This rechecks physical event ordering, transition completion, subject tips and verified completed-state hash with the authentic verifier.
5. Compares the resulting typed completed physical state and original signed cut/signature **byte for byte** with the exact persisted InputLedger source. It independently recomputes `raw-a1-proof.json` (epochs, lengths, source digests, signature, run/Genesis/commit) instead of trusting a self-declared success flag.

The detached verifier **does not invoke Python** and never reintroduces the archive as a new causal input. It only proves that two real supervised, signed, original source pairs are now retained and can be cold-validated. It does not make a fresh Go causal execution from them.

### Negative controls

Run `go run ./cmd/csf-reference-run-local --out /tmp/FRESH_PRIVATE_RUN` and `go run ./cmd/csf-reference-run-local --verify /tmp/FRESH_PRIVATE_RUN` under the pinned local dependencies. The exact-HEAD Actions workflow runs genuine Python A1+Go C4/D2/D3, the complete cold verification and adversarial mutated archives with a **fully recalculated SHA256 index**: raw snapshot, raw eventlog, raw evidence, signed cut, signature and raw proof. Host tests reject symlink/overexposed roots and a second write to an existing private epoch. The GitHub Actions job must never upload the archive, physical source, actor databases or keys to a public artifact or log.

## What is still missing

**An independent second D3 execution using these very same raw bytes** remains necessary. The runner's `RunReferenceD2` does not accept a raw A1 archive as an ordinary source yet, and H2-R deliberately does not create a new ingress authority or a mechanism to bypass A1 supervision. A controlled same-signed-source replay with exact table/owner/snapshot comparisons, crash/retry/pending-barrier fault matrix, complete auxiliary authority export and formal external review are future H2 work. The operator-owned SHA256 index is **not an external auditor signature**. No release GO.

G6 remains `NO_GO_INCOMPLETE_MATRIX` with 50 `NOT_RUN` and 8 `BLOCKED`; residual #22/#168/#169/#172/#174 and independent audits #93/#94/#113/#123 are open. Parent #122, H2 #177 and epic #114 remain open. Human-only merge after final exact-SHA CI.
