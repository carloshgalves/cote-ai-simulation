# CSFRV1-7 — communication and forged-sender knowledge audit

## Proof boundary

The only registered first-run communication is `communication.send` from one
admitted ActionProposal, with one Claim and one scheduled Transmission. The
frozen admission + `CommitWork` transaction atomically publishes
`communication.sent`, private Claim and Transmission, a future
ScheduledOccurrence, the round closure and the CycleCommit. Sending does
**not** deliver knowledge.

The terminal due cycle consumes exactly that occurrence and publishes either
`communication.delivered` (one B-addressed notice) or
`communication.delivery_failed` (zero recipients), based on the pinned
closed channel registration, never on a worker-supplied success flag. The
normal CSFRV1-6 perception pipeline derives B's `Observation`,
`KnowledgeInput` and `PerceptionTaskCompletion` from independently
persisted roots in one CAS transaction. The actual sender remains private in
the Claim/Transmission authority.

The agent sees the observation **only** through a recipient-bound
`ActorInbox` at a dispatch causal revision later than delivery. The
context builder has no permission to read world state, private
`communication_transmissions`, global Claim, raw EventStore or Observatory.

## Recipient matrix

| Causal phase | Actor A | Actor B | World truth |
| --- | --- | --- | --- |
| Sent, `IN_TRANSIT` | private author only | 0 KnowledgeInput | no `resource.stolen` Event |
| Before scheduled due | no new receipt | 0 | no theft Event |
| Due / `school.message.v1` delivered | actual sender not disclosed through B inbox | exactly one `Observation` + `KnowledgeInput`, presented sender `C`, allegation unverified | no theft Event |
| Due / `school.drop.v1` failed | no B evidence | 0 | delivery-failed event only |
| Retry / replay | no duplicate | no duplicate | exact canonical terminal closure |

## Adversarial requirements

- Reject a worker-provided observer, channel success, semantic clock, source
  identity, forged due reference or terminal status not implied by the
  authoritative communication roots.
- Reject typed `canon.future`, `Observatory`, privileged background,
  physical telemetry or other unregistered scope/content envelopes **before
  publication**. An envelope hash alone does not authorize disclosure.
- The allowed delivered payload contains recipient B, presented sender C,
  a Claim reference and reported proposition, semantic receive timestamp and
  registered channel; it has no actual sender identity nor truth flag.
- The `KnowledgeInput` modality/channel, claims/evidence lists, causal
  timestamp and input digest must agree exactly with the addressed
  `Observation`. Other actors and stale dispatches must fail closed.
- The due cycle and replay must reject missing/modified Claim/Transmission,
  duplicate terminal delivery, partial SQLite writes, replay orphans and an
  invented world-event claim.
- No belief update, content attention, reputation, omniscient retrieval, or
  model judgement is permitted in this slice.

## Executable gates

```sh
go test -count=1 -v ./internal/csf/communication/...
go test -count=1 -v ./internal/csf/contextboundary -run '^TestCSFRV17KnowledgeBoundary'
go test -count=1 -v ./internal/csf/commit -run '^TestCSFRV17'
go test -count=1 ./...
git diff --check origin/main HEAD
```

CI source: `.github/workflows/csfrv1-7-communication.yml`.
Acceptance is contingent on all workflows passing on the actual PR HEAD and
the final adversarial review confirming **zero OPEN findings**. Pending review
issues are recorded on Draft PR #24; this audit file does not itself imply
a completed implementation.
