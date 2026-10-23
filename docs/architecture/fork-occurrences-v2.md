# R20 — pending occurrence births across V1 → V2

The V2 bootstrap retains each pending source occurrence exactly once. Existing
communication imports own their Claim/Transmission/due graph. Other occurrences
use the established Genesis rekey/digest path after authenticating the exact
source bytes against the snapshot's kind-6 ref. Event-owned recurrence births
additionally require their frozen Event and the publishing DEFER DecisionRecord
and CycleCommit. The source Event is historical; the child Genesis owns the new
live occurrence. Due instant, kind, payload, recurrence and idempotency metadata
are preserved. Same-instant eligibility is rebased to the child ordinal origin.

Communication's already-versioned reserved birth coordinates are allocated
first. Uncontested Genesis coordinates are then retained. Event births and any
Genesis birth displaced by a reserved communication coordinate use the same
lowest-free ordinal allocator as rounds, per role and in source-ID order.
Distinct parental origins with equal local coordinates cannot alias in the child.

`fork_occurrence_imports` stores exact source birth bytes in the same transaction
as Genesis, lineage, five ledger archives, rounds, communication, schedules and
integrity seals. Lineage replay rebuilds all expected initial occurrences and
rejects missing Event proofs, altered records or extra live Genesis births.
Earlier V2 Genesis-only forks can reconstruct their source birth by reversing
the established Genesis rekey and matching the exact signed parent ID/digest;
this compatibility path cannot synthesize an Event-owned source. It also cannot
recover a birth whose local coordinate required remapping: its source proof is
mandatory. These proofs never emit events or create actor knowledge.

R20 validation includes real DEFER commits, colliding Event origins, coexistence
with Genesis schedules and round responses, transactional rollback/retry,
reopen, replay, proof corruption and actual child settlement. OPEN-02 and
OPEN-05 remain open pending the other migration classes and final matrix.
