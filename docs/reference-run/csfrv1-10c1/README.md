# CSFRV1-10/C1 — false Claim and delayed communication (#126)

**Scope:** child C1 of #117 (epic #114), extending the real **#116 SQLite run**. Experimental synthetic execution, not a new Genesis and not an independently approved First Reference Run. No LLM, RAG, host-time scheduler or signed A1 subprocess.

## Reproduce (fresh files only)

```bash
go run ./cmd/csf-reference-run --db /tmp/csf-c1-a.db --report /tmp/csf-c1-a.json --communication
go run ./cmd/csf-reference-run --db /tmp/csf-c1-b.db --report /tmp/csf-c1-b.json --communication --reverse-sources --reverse-responses
go test -count=1 -v ./internal/csf/commit -run '^TestReferenceCommunicationC1'
```

Use a new database/report filename for each run. The command rejects existing SQLite files; the Go runtime is pinned to 1.27.2 by the project's workflows. The `--communication` flag is opt-in to preserve the original #116 three-cycle behavior.

## Expected causal sequence

1. Original #116: resource contest (1 → 0), CLOCK_ADVANCE to t=10 and trigger+scheduled occurrence terminal WORK; **three commits** and one verified trigger activation.
2. Registered prior PerceptionTasks are completed through authenticated SQLite projection (internal secret events do **not** become actor KnowledgeInput); `CheckPerceptionBarrier` must approve the committed prefix. No synthetic completion bytes are fabricated.
3. At t=10, ordinal=1, actor A's **new, real** `RoundDeclaration → SlotDispatch → SlotResponse → AdmissionFence` produces an authenticated `communication.send` ActionProposal. Its proposition is the *unverified* `resource.stolen` allegation, reported as presented by C and addressed to B; A is the authenticated author. The atomic `CommitCommunicationSend` publishes Claim, IN_TRANSIT Transmission, due ScheduledOccurrence, CycleCommit, events, WorldRevision and outbox. Stock stays **0**.
4. `CheckPerceptionBarrier` and a query to the B-only evidence routes prove that B has received **no new KnowledgeInput about this in-transit Claim**.
5. CLOCK_ADVANCE from t=10 to t=35 is a real committed cycle; the due occurrence is admitted by a new fence and `CommitCommunicationDue` produces exactly one `communication.delivered`. A duplicate attempt must fail.
6. The registered `DeliverCommunicationPerception` projects the authoritative delivery to exactly one B Observation and one B KnowledgeInput, and nothing to another actor. The delivery-task identity is joined to those private evidence routes; `ReplayClock` and the through-revision barrier must verify the complete durable history.

**Total:** six real committed cycles in one RunID and SQLite; final revision 6, t=35, stock 0. The JSON report includes opaque Claim/Transmission/delivery/task/input IDs and clocks, **never the proposition payload or private KnowledgeInput bytes**. `RunID`, Genesis, policy hashes and event-stream SHA are inherited from the original run.

## RED/GREEN, negative and adversarial acceptance

- `TestReferenceCommunicationC1SameRunAndDueDelivery`: six commits, one Claim/Transmission, one eventual delivery, no `resource.stolen` factual Event, exact stock.
- `TestReferenceCommunicationC1SourceWorkerPermutationByteExact`: four independent runs; compare **all** EventStore bytes and the C1 AdmissionFence bytes across source/worker order inversions.
- `TestReferenceCommunicationC1AtomicFailures`: seven injected before-commit failure points for each of send and due, with checks against partial claims/transmissions and terminal publication.
- `TestReferenceCommunicationC1ForgedAuthorRejected`: a re-digested mismatch between proposed author and authorized slot must fail before Claim publication.
- `TestReferenceCommunicationC1EncodedReadOnlyURIPath`: SQLite audit uses an escaped `mode=ro` URI even when a filename contains `?`/`#`.
- `TestReferenceCommunicationC1TamperedDurableEvidenceFailsClosed`: independent mutation of stored Claim, Transmission, delivery task and B KnowledgeInput; reconstructed causal replay **must reject** tampering.
- `TestReferenceCommunicationC1ForeignRecipientRouteFailsClosed`: a forged evidence route from B to A is denied by the barrier and replay.

Live CI: `.github/workflows/csfrv1-10c1-communication.yml`. A green workflow proves the **C1 experimental scope only**; the 41 normative invariants and 68 stress scenarios are **not** certified by C1.

## Explicit downstream gates

C2 #127 will audit POV A/B/public boundaries, owner checkpoint, general private secret and KnowledgeInput barrier. C3 #128 must prove `INDETERMINATE → CycleAbortRecord → HALTED_ON_ABORT → AttemptRetryRecord` independently. C4 #129 will combine C1/C2/C3, including cross-branch disruptions. A1 attested Python/Go belongs to #118; crash/snapshot/fork matrix #119; Observatory #120, evals #121, final command/evidence #122 and independent acceptance #123 remain open. **Do not merge without explicit human authorization.**
