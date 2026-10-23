# R19 — pending round history at a V1 → V2 boundary

The V2 child reconstructs pending Genesis-owned RoundDeclarations and their
SlotDispatch, SlotResponse (ActionProposal or NoProposal), and revocation history.
The source is the exact InputLedger prefix sealed by the parent snapshot, including
revoked requests which do not appear in the active-dispatch partition. Later
parent responses are excluded. Parent ledgers remain historical archive.

The source dispatch must belong to the current source cut: run, cycle at the next
clock ordinal, effective instant and base revision are authenticated. Child run,
Genesis, round, cycle, dispatch and response identities are derived with the
installed V2 policy. The child starts at revision/ordinal zero. Actors, slots,
request digest, response kind, action content, revocation reason/policy and dispatch
ordinals are preserved. Same-instant round eligibility ordinals are rebased relative
to the parent's next ordinal, consistently with the child clock and dispatch. A response with an originating intention reference still
requires that reference's own causal migrator; it is rejected rather than copied
as an unauthenticated child reference. Stale requests from earlier source cuts
and Event-owned rounds also remain unsupported.

A slot's first dispatch has ordinal zero; each subsequent dispatch requires the
preceding durable revocation. Revoked requests cannot also have a response. A
response fills a slot once. Its child conditional-storage key is installed in the
same SQLite transaction as Genesis, lineage/archive, declarations, integrity seals
and request history. Transaction failure leaves an empty destination and a retry
can publish the complete child.

Both bootstrap and subsequent lineage validation derive the expected child
records again from the archived source prefix and parent snapshot. Missing or
altered declarations, requests, responses, revocations or conditional keys fail
closed even when no child snapshot has yet pinned those rows. New child tail
records are allowed through the existing InputLedger admission rules. Migration
does not call a model, reinterpret an action or publish any action as world truth.

Acceptance evidence: `TestCSFRV18R19*` in `internal/csf/commit`, including real WORK
settlement, snapshot plus tail, reopen, source-tail isolation, both response kinds,
duplicate prevention, corruption and transactional rollback. This slice does not
close OPEN-02 or approve the R23 differential matrix required for OPEN-05.
