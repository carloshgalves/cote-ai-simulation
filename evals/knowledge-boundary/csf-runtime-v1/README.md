# CSFRV1-6 knowledge-boundary audit

## Scope and authority

The only positive observer route registered in this slice is the terminal
`round.consumed` event. The **sealed RoundDeclaration** determines the one
eligible actor; the worker has no permission to supply observers. The resolver
loads the committed task/event, Genesis-pinned policy refs, and the adjacent
pre/post WorldState revisions. It emits a deliberately redacted *round closed*
notice, not the event's private source receipts, resource outcome, or world
truth. A second internal `resource.consumed` task has no eligible observer and
must end with an empty completion.

`ActorInbox` is bound to the run, actor and base revision of a durable,
authenticated SlotDispatch. It reads only the actor-addressed EvidenceLedger
partition and refuses pending/mismatched terminal receipts. The context
builder consumes this scoped interface; it has no EventStore/WorldState or
unrestricted RAG access.

## Expected addressed payload report

| Producer / datum | Recipient A | Recipient B | Secret / global data in either agent context |
| --- | --- | --- | --- |
| Closed round notice, role 0 / ordinal 0 | 1 Observation + 1 KnowledgeInput, after causal completion | 0 | No |
| Internal `resource.consumed` event | 0 | 0 | No |
| `source_event_refs` | Event ID and digest only, not raw event | None | No raw source payload |
| Private slot response roots | Redacted | None | No |
| Future event after dispatch base revision | Not accessible through stale dispatch | Not accessible | No |
| Unrecognized claim / missing gated background | Rejected | Rejected | No |
| Cross-run addressed evidence | Rejected by run-bound inbox | Rejected | No |

The test fixture supplies A as the owner of the settled round. B is not
automatically eligible merely because the event exists. No LLM judgment or
RAG lookup is involved in deciding eligibility.

## Negative audits

- VIP/role disclosure prior to a source event: reject;
- private observation of A under B's actor capability: reject;
- White Room/background fact without an explicit gate: reject;
- canon future after divergence: reject;
- synthetic physical telemetry without perception event: reject;
- privileged Observatory query path: unavailable;
- unredacted source IDs/receipt list in percept: reject;
- worker-supplied semantic latency: reject;
- missing input, altered holder, missing completion, forged/extra evidence
  or orphan route: fail closed on context/replay;
- worker permutation, task completion order and retries: exact canonical
  evidence/outbox bytes, no duplicate identities.

## Executable verification

```sh
go test -count=1 -v ./internal/csf/contextboundary -run '^TestKnowledgeBoundary'
go test -count=1 -v ./internal/csf/commit -run '^TestCSFRV16'
go test -count=1 ./internal/csf/perception/... ./internal/csf/persistence/sqlite/...
go test ./...
git diff --check origin/main HEAD
```

CI workflow: `.github/workflows/csfrv1-6-implementation.yml`. The independent
review must cite the **current HEAD** and corresponding passing jobs; an older
green run does not certify newer code.

## Not included

Inference, beliefs, memory summaries, claims as first-class records, generic
communication handlers, and RAG retrieval (there are no retrieval chunks to
score in this version). These cannot be silently treated as available context.
