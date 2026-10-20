# CSFRV1-4 — Decision Kernel implementation checkpoint

**Branch:** `feat/csfrv1-4-minimal-decision-kernel`
**Base:** `main@43574722e3f22eb629da47e17bf573dad4a68369` (CSFRV1-3 merged)
**State:** implementation checkpoint validated in CI; Draft PR #20 remains unmerged.
**Validated remediation code head:** `cda9a291be311371a374a982a1f27bcac40db487`

## Accepted boundary

CSFRV1-4 produces **provisional, non-authoritative** decisions for two
`resource.consume` actions competing for a single remaining inventory unit
against one `base_revision`.

- Pure inputs/outputs: no direct `WorldState` write, `Event`,
  `CycleCommit`, terminal settlement, or revision publication.
- Identities and digests derive from the pinned canonical codec/policy bundle.
- A persisted `AdmissionFence` can be authenticated against the immutable
  input ledger before evaluating; the synthetic fixture also supports
  isolated direct execution.
- CSFRV1-5 owns authoritative terminal transaction, abort/retry, and
  ingestion of provisional material into the transactional decision ledger.

## Implemented and exercised

1. Registered versioned synthetic action schema, fully typed proposal and
   `NoProposal` parsing, same frozen base revision and canonical ID/digest
   validation. Unknown action/validator never silently grants permission.
2. Six separate affordance facets: physical possibility, explicitly proven access, resource
   availability, temporal applicability, institutional prohibition, and
   legality. A typed prohibition does not rewrite physical possibility.
3. Exact source-unit partition with no gaps, overlaps or external sources;
   candidate source IDs must form a strictly canonical ordered set.
   Candidate IDs bind source units, producer, version, role and ordinal.
4. Conservative claim detector for resource, read/write and invariant claims.
   The resource resolver consumes the detector's actual `ConflictSet` and
   cannot mint its own alternative conflict identity. The already-enforced
   `resource.nonnegative` invariant does not trigger a duplicate lottery.
5. Named RNG substream with versioned/pinned policy reference, canonical
   `RngDraw`, and stable provisional `COMMIT`/`REJECT` disposition.
6. Idempotency logical digest/namespace, alias in the same cycle, historical
   canonical alias, and mismatch as `IDEMPOTENCY_CONFLICT`.
7. Subject-attributed `INDETERMINATE` evidence on missing rule/data,
   unsupported resolver or arithmetic overflow, with **no** terminal
   dispositions. Invalid provenance fails closed before resource resolution.
8. Property/regression scenarios for worker-order permutation, registration
   permutation/irrelevant handlers, last-unit conservation, ample stock
   without lottery, bad source partitions and alias collision. The
   architecture test rejects mutable persistence/commit dependencies in
   handlers and the pure decision kernel.
9. A synthetic, persisted CBOR scenario and a persisted-fence-backed
   test using the in-memory store. The command writes an independently
   reopenable **provisional-only** artifact, not an authoritative world
   record.

## Executed CI evidence

Code head `b010b0dafeb8e0a1094f0a017f728f7bb6e1028a`:

- [Decision kernel checkpoint run 39435966005](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39435966005):
  successful `git diff --check`; targeted decision and synthetic extension
  tests; full `go test ./...`; Go canonical conformance **621/621**;
  TypeScript conformance **621/621**; byte-identical receipts; produced
  `/tmp/csfrv1-4/provisional.cbor` (2,870 bytes) as a workflow artifact.
- [Independent canonical conformance run 39435966449](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39435966449):
  successful.
- Scenario assertion: two authenticated candidate records, one resource
  conflict, one named draw, one provisional winner, post-copy stock zero,
  original snapshot unmodified. Reversing delivered worker responses yields
  byte-identical provisional material.

## Deliberate limits / handoff

- The scenario's CBOR file proves **provisional artifact persistence**, not
  transactional publication to the canonical `DecisionLedger`. The
  authoritatively durable provisional records and their lifecycle need
  CSFRV1-5 integration with the SQLite transaction/fence/replay contracts.
- Historical alias bindings are explicit immutable inputs in this kernel;
  loading and authenticating bindings from the authoritative history is a
  coordinator/persistence integration step, not permission for a caller to
  forge historical state.
- An `INDETERMINATE` result is not yet a terminal `CycleAbortRecord`; ADR
  0008 invariant 7 remains preparatory until CSFRV1-5.
- No actor receives conflicts, RNG, provisional stock or affordance facets
  through this ticket; agent knowledge/perception is outside the kernel.
- Implementation has been tested, **not independently code-reviewed or
  approved for merge**. Keep PR #20 Draft until a separate `$code-review
  CSFRV1-4` phase and any requested remediation.

## Review/remediation rounds (2026-10-20)

- [Finding 1](https://github.com/carloshgalves/cote-ai-simulation/pull/20#issuecomment-6293157136):
  **RESOLVED**. Access is actor-scoped evidence, not a silent grant.
  Missing institutional rule also fails closed; denied access is filtered
  before resource contention.
- [Finding 2](https://github.com/carloshgalves/cote-ai-simulation/pull/20#issuecomment-6293158376):
  **RESOLVED**. DetectClaims verifies candidate identity preimage,
  canonical source IDs and cycle binding before grouping conflict evidence.
- [Finding 3](https://github.com/carloshgalves/cote-ai-simulation/pull/20#issuecomment-6293159352):
  **RESOLVED**. NoProposal reason must be one of the three registered
  uint8 values, including when source digest/bytes are self-consistent.
- [Finding 4](https://github.com/carloshgalves/cote-ai-simulation/pull/20#issuecomment-6293348676):
  **RESOLVED**. Provisional audit CBOR now binds the exact fence-ordered
  (admitted unit ID, admitted unit digest) pairs, including NoProposal
  and idempotent alias units; worker completion remains irrelevant.

CI: [decision run 39440500500](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39440500500)
and [cross-language canonical conformance 39440500486](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39440500486)
both succeeded after the remediation. All four review findings have
terminal RESOLVED markers, with correction SHAs in their original PR
conversation comments.

The clean-review result is scoped to **CSFRV1-4 provisional semantics**;
it does not claim that CSFRV1-5's atomic terminal publisher, authenticated
historical alias retrieval or authoritative replay already exists.
