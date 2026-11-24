# H2-C — archived-source recovery and pending perception

Issue #177, parent #122. Based on H2-W #183, `main@8cc5f149`. The initial H2-C head `2feb8f8284c24ac69ce9278fcc795dfdfddcbb9f` passed 18/18 checks (private-local run [45313165880](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/45313165880)). That historical pass is not evidence for later commits.

## Same archive, not another capture

The H private workflow creates and cold-verifies one real H2-I archive. Its checksum-index digest is pinned across all H2-C processes. The archived A1 source bytes are revalidated by sealed A2/Genesis/Ed25519 before execution; the frozen offline path has no physical signing key. The original authority and SQLite sidecars are read only through private copies. The same E2 matrix reaches both epochs × all seven selected SQL rollback points, exits the fault process and resumes the existing registered work in a separate process. All fourteen actually completed cases and all three authority transcripts must agree with the original archive. This is atomic rollback + process restart, not a SIGKILL inside a SQLite transaction or a power-loss test.

## Pending epistemic barrier on the same signed inputs

A separate fresh run of the exact same archived inputs stops at the real C1 committed delivery (revision 6), before perception completion. A package-private reference harness hook pauses the driver; the first worker opens the persisted cut, checks the specific missing-completion error, attempts both real C2 actor dispatches, verifies no authoritative mutation and exits without completing perception. A separate worker repeats the denial on cold-open, performs native registered C1 perception, checks exactly one new B KnowledgeInput, verifies the barrier opens, and repeats completion with full canonical equality. The driver proceeds normally through C2 owners/ACK, C4 and both physical epochs. D3 disclosure and complete logical causal/owner transcripts must match the archived baseline. This models a perception-worker interruption, not a complete simulation-driver restart at revision 6.

## Evidence

The same archive must pass all 14 recovery cases plus one barrier-worker case, normal and race, with identical private digest reports. The final cold verifier checks that the original package remains unchanged. No private archive contents or actor material uploaded. The H2-C report is local evidence, not an immutable external signature.

The full CLI stop-state/exit-code taxonomy, auxiliary chronology, true abrupt driver/host loss, external attestation and normative G gaps are separate tasks. H2-C does not claim #177/#122/#121/#114 closure, independent review, DoD approval or production GO.
