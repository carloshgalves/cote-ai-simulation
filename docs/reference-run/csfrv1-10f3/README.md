# CSFRV1-10/F3 — Crossed Observatory disclosure views

Parent #120, slice #153. Base: human merge #156, `51ecd1c72f520e3fb79386db4cc23f9d74f2d4e1` (28/28 exact-head checks). F3 extends the pre-existing O3 readers using the F1 causal provenance graph and actual B–D3 signed physical reference run. No new publication policy or aggregate mixed-view interface.

## Explicit projection allowlists

| View | Permitted fields | Authority |
|---|---|---|
| Privileged human audit | F1 DTOs: authenticated genesis/run/clock, immutable causal attempts, source/candidate/assessment/decision/event/revision/task/completion/evidence refs and digests; operational metadata in F2 | Verified replay and exact persisted membership; never an actor's context |
| A POV | Run, Actor, ThroughRevision, Entries; each entry: ObservationID, KnowledgeInputID, ObservationCanonical, KnowledgeCanonical, ReceivedAt, Channel | Original durable A SlotDispatch, its immutable full bytes, sealed round/holder, CausalInbox cutoff, verified addressed completion |
| B POV | Same fields, separately authorized for B; no shared evidence buffer | Original durable B SlotDispatch and B-only route |
| Public | Run, PublicationStatus, Publications (PublicationID, Channel, PublishedAt, ContentHash; no registered publisher in V1 so **empty**) | Verified run and explicit registered publication authority; never a privacy-bit filter of EventStore |

Actor canonical records are recipient-bound Observation (15 fields) and KnowledgeInput (11 fields), not EventStore records. Only the two registered channels are admitted: redacted round lifecycle and delivered communication. Lifecycle notice has no source payload; communication preserves presented sender, unverified Claim and causal arrival time, not actual sender or world truth. This is the existing normative context boundary, not a redaction copy of privileged state. Unregistered DTO fields or evidence channels invalidate acceptance. Host-private owner content, world seed, physical input/Event IDs and BodyState are never actor/public payloads.

A POV reader reauthenticates its original SlotDispatch and verifies unchanged canonical-byte hash, run/holder/cutoff on every read. Mint and read use the existing data-version fence; a historical dispatch remains a *historical observation cutoff*, not permission to respond, act, restore an owner, or access current private state. This does not create a network authentication scheme or alter InputLedger dispatch lifecycle. Restricted errors return only ErrViewUnavailable (or caller cancellation); inner replay/SQL/holder details remain privileged. No registered publisher: public queries do not infer publications from any Events. Case-insensitive reserved publication table/view names without a registered resolver deny access.

## Integrated evidence and negative controls

TestReferenceF3CrossedViews runs two real supervised A1 cuts and a complete 9-commit D3, opening four separate adapters on the same causal database. E1 captures actual causal + A/B owner databases. An independent SQL roster joins evidence routes, ledger rows and tasks; all 12 addressed authority rows must be covered once in F1's provenance, and each POV must equal its authorized holder/cutoff subset (including raw bytes). A fresh genesis dispatch sees no later evidence; B alone receives the false Claim at t=35 with presented sender C, no factual resource.stolen event. The oracle checks the actual A private marker exists in owner storage before asserting exclusion and checks physical Events are ENGINE_ONLY and have no perception edge. No publisher means zero public publications. Separate exact DTO/method allowlists protect against accidental API broadening.

The same already-open readers must reject:
- Physical Event reclassified as public with recomputed Event digest; no partial projection or raw replay error.
- Actual B KnowledgeInput corrupted, or holder index relabeled to A.
- Swapped dispatches across A/B.
- Dispatch base revision and request digest changed after mint despite unchanged derived dispatch ID.
- Unregistered uppercase PUBLIC_ROUTES SQLite view, even when it selects existing evidence.
- Caller cancellation is distinguishable from restricted denial.

Malicious SQL is performed solely by the test, never by the Observatory. Capture damaged state, verify denial leaves it unchanged, restore exact original bytes and require baseline views. E1 also proves unaltered causal/owner transcript during clean sequential reads and after restoration. F4 will separately audit concurrent reads, connection lifetime, path/sidecar ACL and import boundary; F3 is not proof of those guarantees or of resistance to timing side channels.

## Reproduction

Pinned Go/Python and actual A1 fixture setup are in the dedicated workflow. With that environment installed:

```sh
export PHYSICAL_A1_PRODUCER_TEST=1
set -o pipefail
go test -json -count=1 -timeout=10m ./internal/csf/commit -run '^TestReferenceF3' | tee /tmp/f3-normal.jsonl
python docs/reference-run/csfrv1-10f3/verify_tests.py /tmp/f3-normal.jsonl
go test -race -json -count=1 -timeout=15m ./internal/csf/commit -run '^TestReferenceF3' | tee /tmp/f3-race.jsonl
python docs/reference-run/csfrv1-10f3/verify_tests.py /tmp/f3-race.jsonl
```

Exact expected case starts/passes are verified; no skip/missing case accepted. Prior F1/F2, D3, O3, architecture and vet regressions run on exact HEAD. No independent audit or release claim. F4/F5 not implemented; gates #93/#94/#113 remain unchanged. Human merge only.
