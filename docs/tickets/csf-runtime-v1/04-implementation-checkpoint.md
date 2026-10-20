# CSFRV1-4 — Decision Kernel implementation checkpoint

**Branch:** `feat/csfrv1-4-minimal-decision-kernel`
**Base:** `main@43574722e3f22eb629da47e17bf573dad4a68369`
**Status:** IN PROGRESS — Draft PR; no claim of acceptance.

## Accepted contract

Deliver a deterministic provisional, **non-authoritative** decision
kernel for two competing `resource.consume` proposals sharing one
`base_revision`: typed action/affordance facets, exact candidate partition,
conservative conflict detection, versioned resource policy, named RNG
draws, idempotency alias handling and per-subject INDETERMINATE evidence.

- No world-state mutation, event, CycleCommit, settlement or terminal
  publication in this ticket; that boundary belongs to CSFRV1-5.
- Every deterministic identity must be derived from the pinned canonical
  codec bundle, not a new schema or process-order allocation.
- Keep the input, handler, validator, resolver and RNG seams pure;
  no mutable store is exposed to those components.
- Follow red/green tests and document executed CI at each code checkpoint.

## Work log

- Initial branch/checkpoint created before feature work to preserve the
  implementation handoff and open a Draft PR early.
