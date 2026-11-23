# H2-S — signed physical ingress provenance (scoped)

**Issue:** #177, H parent #122. **Base:** human-merged H2-L [PR #178](https://github.com/carloshgalves/cote-ai-simulation/pull/178), `main@4c525bc9ffdb577c58e866af5bf7cf0106ee779d`. This is a private local provenance audit, **not** an independent compliance approval.

## What the new proof actually demonstrates

The existing real `go run ./cmd/csf-reference-run-local --out NEW_PRIVATE_DIR` runs Python A1 twice under the OS supervisor and produces its original real C4/D2/D3 causal authority and actor-owner DBs. After H2's five-ledger and two-owner exports, H2-S independently opens a **private detached** SQLite copy in `mode=ro`/`query_only=ON` and reconstructs the registered `physical.a1` ingress prefix.

The private `ledger-export/signed-ingress-proof.json` is a **digest-only index**, not another source or capability. It binds source commit SHA, RunID, installed Genesis digest, exact registered manifest digest, the final source closure, five ordered durable source ingress receipts (two signed inputs plus three closures), and exactly one Ed25519-attested physical source for each epoch 1 and 2. Each epoch has the InputLedger record ID, receipt sequence, cut canonical SHA256, signature SHA256 and completed physical state content digest. The validator independently:

- Decodes original authoritative InputLedger CBOR, checks identity and domain-separated input/closure digests, and checks the receipt's exact `source_id`/ordinal/type/record ID/digest.
- Validates registered physical manifest against installed policy; checks Genesis canonical bytes/digest against the persistent root and installed source public key.
- Decodes signed cut v1 and verifies **Ed25519 signatures over the actual persisted declaration**, including epoch/run/Genesis/source/executable/snapshot/events/evidence/completed-state digests.
- Invokes the existing terminal-only `RecoverApprovedCutFromDurableIngress` to validate normalized completed state, while requiring that capability be `IsDurablyRecovered()` — a cut **not eligible to create another source ingress**.
- Requires exactly five gap-free receipts, two distinct physical epochs and three closures, with the terminal source closure last. No post-final ingress can be accepted.

The signed original declaration, its signature and completed state are already present in the private `input_ledger.bin` from H2-L; H2-S proves those bytes correspond to the authoritative SQLite ingress. The new JSON carries only hashes and IDs. It **does not** contain the full Python `snapshot.yaml`, `events.jsonl`, or `evidence.json` captured by the A1 supervisor. Their absence matters: the signed hash declarations cannot be elevated to a replayable original Python source merely by preserving this proof.

## Backward compatibility and anti-downgrade

New runs declare `run-manifest.json.evidence_profile = "H2_SIGNED_INGRESS_V1"`. A matching signed-ingress proof is mandatory and must pass independent revalidation; removing it while recalculating `checksums.json` is a **hard failure**. Pre-existing H1/H2-L archives without that profile can be verified under their older, narrower guarantees, but must not be reported as H2-S or production approved.

The complete private H2-S file is covered by H2-L's fixed paths, length and SHA256 archive index. Adversarial real-A1 CI copies the genuine archive, tampers with the signed-ingress proof or removes it, recalculates the **whole** checksum index, and still requires the semantic/profile validator to reject the counterfeit, in addition to the existing owner/snapshot/ledger mutants. No private physical or cognitive material is uploaded as an Actions artifact.

## Status and remaining issue #177 obligations

This slice is **H2-S signed ingress provenance**, NOT a second complete physical replay. It does not rehydrate raw Python source bytes into sealed memfds or re-run C4/D3 with a second independent authority from the *identical* signed source pair. `RecoverApprovedCutFromDurableIngress` is terminal-only, not an authorized new `StagePhysicalInput`. Further work must include secure cold archive of actual supervised raw source bytes, controlled replay, cross-run byte transcripts, auxiliary authorities and fault/abort/restart injections. No second Go write authority is introduced by this slice.

G6 decision remains `NO_GO_INCOMPLETE_MATRIX` (50 NOT_RUN and 8 BLOCKED), #22, #168/#169/#172/#174, and independent #93/#94/#113/#123 remain open. The H parent #122 and epic #114 are still open; this internal experimental proof does **not** approve a real product run or waive independent review. Merge requires human authorization after exact-SHA tests and correction/re-review.
