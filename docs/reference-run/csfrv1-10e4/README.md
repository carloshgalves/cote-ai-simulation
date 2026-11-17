# CSFRV1-10/E4 — closed reference fork

Issue #143; functional parent #119. Baseline: E3 merge `6ac3ec0348f4`.
This slice does not change external gates #93/#94/#113 or implement E5.

## Authority boundary

`Store.ForkClosedReferenceV1ToV2` uses the existing authenticated V1-origin /
V2-child migration and transactional child publisher, but requires a **current
settled reference cut**. The historic-fork entrypoints retain their documented
ability to fork a verified older cut. V2 must resolve the exact pinned V1 origin;
the parent and child remain separate stores and RunIDs.

`VerifySnapshotV1` now computes `ReferenceForkReady` separately from
`OwnerWorkReady`, checking the pending-state projection and all five live
ledger prefixes against the sealed snapshot in the same read transaction.
A same-revision response need not revoke an owner's cognition permit, but it
invalidates a current reference fork. Preparation explicitly re-verifies the
cut before publishing the child. Existing closure and snapshot authentication,
child migration validation, and exact prefix archival are not bypassed.

This API is for a cooperative local reference host: stop parent writers while
creating the forks. The recheck detects observed advancement; it is not a
distributed lock spanning parent and child SQLite files. Once published, each
child has its own causal tail and does not depend on later parent input.

## Tests (Linux)

The CI executes:

```sh
PHYSICAL_A1_PRODUCER_TEST=1 go test -count=1 -v ./internal/csf/commit -run '^TestReferenceE4'
PHYSICAL_A1_PRODUCER_TEST=1 go test -race -count=1 ./internal/csf/commit -run '^(TestReferenceE4|TestCSFRV18R58B3ForkChild)'
PHYSICAL_A1_PRODUCER_TEST=1 go test -count=1 ./internal/csf/commit -run '^(TestCSFRV18|TestReferenceE3|TestReferenceD3RealPythonPrivateEventAndLiveObservatory)'
go test -count=1 ./internal/csf/persistence/sqlite ./internal/csf/ownercheckpoint/...
go vet ./internal/csf/commit ./internal/csf/persistence/sqlite
```

The workflow runs the real Python A1 producer fixture with the same pinned
Python and Go toolchains as E3; an absent or skipped E4 test is not a pass.
Historical RED is run from `b4ca52efa0ef351e7b3b3cf795429c8249ee26e1`,
where the new test reports a stale same-revision fork. The current run must
both reproduce that specific failure and pass the corrected test; compilation
errors and unrelated failures are not accepted as RED evidence.

### Matrix

- C4: two children with independent RunIDs/Genesis/snapshots and one new
  native CycleCommit each, rather than a direct SQL write or copied commit.
  E1 compares all five ordered archived memberships to the actual parent cut
  (ordinal, ID and canonical bytes). Check that no historical Event, Evidence,
  outbox or source-ingress authority is activated in the child.
- Opaque owner memory: re-attest the exact synthetic parent bytes with child
  scope/coverage, fresh receipts and isolated owner databases; reject parent,
  other actor and same-actor sibling-run receipt/ACKs. Historical actor public
  keys are retained by V2 migration, so isolation is not merely key mismatch.
  Compare owner stores and complete causal transcripts across child commits,
  later parent commit, cold reopen and attempted wrong historical recovery.
- Frozen tip and wrong policy: genuine same-revision failed terminal, no
  destination rows; wrong policy and altered sealed snapshot fail closed.
- Same-revision response: a legitimate new input with an otherwise ready
  owner must not allow reference forking from the stale cut.
- D3: actual two supervised signed physical epochs, old cut rejected on
  historical non-final physical SourceClosure even after the live final
  closure. At R9, stage both signed owner checkpoints and create the final
  snapshot. V1 same-policy fork rejects opaque cognitive checkpoints; V2
  without the physical extension registration is rejected. Both rejection
  paths leave the destination empty and the parent's E1 transcript intact.

## Scope / blockers

The positive two-tail demonstration is the integrated nonphysical C4
scenario. D3's positive post-physical fork remains unsupported by the
installed migrators. Do not strip the physical state, omit checkpoint owners,
forge extension pins, promote parent knowledge, or silently change V1
semantics to make D3 succeed. A separately versioned migration for the
physical extension and owner state would be needed for that positive case.
This slice proves a bounded fail-closed outcome, not post-D3 fork support.

The signed owner fixture uses synthetic test keys and opaque bytes, not a
model's cognitive reconstruction. CI evidence is attached to the exact PR
head; no release/independent merge is implied.
