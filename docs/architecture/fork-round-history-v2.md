# R19 — pending round history at a V1 → V2 boundary

The V2 child reconstructs pending Genesis-owned or Event-owned RoundDeclarations and their
SlotDispatch, SlotResponse (ActionProposal or NoProposal), and revocation history.
The source is the exact InputLedger prefix sealed by the parent snapshot, including
revoked requests which do not appear in the active-dispatch partition. Later
parent responses are excluded. Parent ledgers remain historical archive.

An unrevoked source dispatch must belong to the current source cut: run, cycle at
the next clock ordinal, effective instant and base revision are authenticated. A
durably revoked request may come from an earlier revision. Its cycle/instant are
reconstructed from the exact frozen CycleCommit journal; it is imported only as
an atomically revoked receipt. All child receipts bind to the child Genesis cut,
and their original coordinates remain in the immutable parental archive. Child run,
Genesis, round, cycle, dispatch and response identities are derived with the
installed V2 policy. The child starts at revision/ordinal zero. Actors, slots,
request digest, response kind, action content, revocation reason/policy and dispatch
ordinals are preserved. Same-instant round eligibility ordinals are rebased relative
to the parent's next ordinal, consistently with the child clock and dispatch.
An originating intention reference to an authenticated parent root remains an
exact historical, opaque reference. Bootstrap and lineage replay resolve its
registered kind, complete ID preimage and digest against the five archived
ledgers, parent Genesis, parent snapshot or snapshot epistemic refs. ConflictSet
identities additionally require the cycle witness from an archived DecisionRecord.
No child event, evidence or knowledge is inferred from the pointer. Roots outside
these archives require their own historical proof and fail closed while unavailable.
An earlier unrevoked request cannot
be retimed or granted a new authorization by this migrator.

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

Event-owned source rounds additionally require their exact source Event in the
sealed parent EventStore prefix. Its run, causal ID, registered digest and the
round's EventRef are checked under V1. No event is inserted into the child to
simulate that old cause. The child Genesis owns the migrated declaration, and
lineage replay rederives its historical birth from the V1 input/event archive.

Genesis-owned rounds retain their role/local ordinal. Event-owned rounds retain
their role and receive the lowest unused uint32 ordinal in that role, after
reserving all migrated Genesis round coordinates, in lexicographic parent Round
ID order. This named migration output order prevents distinct Event/Genesis
origins with the same local coordinates from aliasing in the child. The source
coordinates and EventRef remain immutable archived provenance.
