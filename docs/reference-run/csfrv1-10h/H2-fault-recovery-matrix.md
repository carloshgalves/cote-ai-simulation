# H2-F — real signed A1 C4/D3 atomic crash and restart matrix

**Issue:** [H2 #177](https://github.com/carloshgalves/cote-ai-simulation/issues/177), parent H #122. Based on human-merged [H2-I PR #181](https://github.com/carloshgalves/cote-ai-simulation/pull/181), `main@efbcb3c51e4d883585eaacc3b81c39004d584574` (16/16 last-head checks passing). **This is a bounded adversarial fault-matrix slice, not the full H2 acceptance package.**

## Two authentic A1 cuts, fourteen real Go atomic fault positions

The existing integrated E2 oracle `TestReferenceE2IntegratedC4D3CrashResumeByteExactAfterBothEpochs` originally checked only SQL positions 1, 4 and 7 for each epoch. H2-F upgrades the real combined C4+D3 experiment to the **complete Cartesian product of two physical epochs × seven atomic SQL steps = 14 cases**, using **the exact same two signed physical cuts originally captured once by supervised Python A1**. Every case independently:

- Runs the actual full C4+D2 path until its named `CommitPhysical` failure injection, with a genuine Genesis, policy bundle, authenticated signed A1 input, durable InputLedger and frozen `AdmissionFence`.
- Requires the failed transaction to publish **zero partial** WorldRevision, CycleCommit or `physical.advanced` events beyond the prior durable prefix.
- Reopens the **same** causal SQLite authority in a distinct child Go process, restoring the frozen fence and committing exactly once using the pre-existing signed ingress and its persisted source references; the original Ed25519 secret and Python producer are not passed to the recovery process.
- Reopens again after the child exits and rejects second commit/reentry attempts, and (if the first physical epoch failed) commits the already-signed second epoch **without rerunning the Python producer**.
- Verifies the final nine committed cycles, original six-commit snapshot plus three causal tail commits, `ReplayClock`, and full exact canonical causal/owner database equality with a separate continuous baseline built from those **same original two signed A1 cuts**.

The private E2 report, written **only on test success** to `CSF_E2_EVIDENCE_PATH`, has a canonical per-table digest/count manifest and counters: `VerifiedBaselineCommits=9`, `AuthenticatedA1Cuts=2`, `IndependentCrashResumeCases=14`, `PhysicalSQLRollbackSteps=7`, `SignerReexecution=false`. The GitHub Actions runner checks this JSON using Python's standard library and prints **only** the nonsecret counter `H2_F_2x7_RESTART_MATRIX=14_OF_14_SAME_SIGNED_A1`. It never uploads a causal database, actor-owner store, original capture or signed physical payload.

## Other exact-HEAD H2-F gates

In the same `private-local` job, additional real E2 tests must prove:

1. A fresh coordinator restarted **before the fence** sees the identical persisted cut; retrying the same ingress is idempotent, and the later terminal is unique.
2. All seven standalone physical atomic faultpoints recover in a **new Go process**, without duplicate events/knowledge, and a tampered saved source ingress is refused before any commit.
3. A recovered terminal-only signed cut cannot become new physical ingress or cross to a foreign RunID.
4. Historical snapshot/barrier tests preserve a frozen semantic fence, process a partially completed perception after crash **exactly once**, never auto-resume a halted abort, and drain pending perception tasks only after authenticated completion.

**Important scope limit:** the integrated fourteen-case fault oracle is a distinct local real-A1 test run whose two signed cuts are frozen in-memory by the test, not read from the separate H2-I command's `physical-cuts/` archive. It exercises the same real C4+D3 implementations and source authentication, but is not a new replay of that specific earlier archive. Its private test report is not itself an immutable external signature.

## Still missing before #177/#122 can close

- Pending barrier **across the specific two H2-I archived A1 cuts** and full causal/owner receipt trace in the one-command archive; real stop-state/exit-code runbook rather than test-only assertions.
- Deliberate WAL/SHM truncation and disruption where sidecars are live, with no silent SQLite repair; genesis/signer and receipt/rollback mutants attached to the same full archive profile.
- Formal independent adversarial review and external immutable auditor attestation #123, complete auxiliary chronological export and remaining G6 normative decisions.

`NO_GO_INCOMPLETE_MATRIX` remains the global decision: G6 has 50 NOT_RUN and 8 BLOCKED; #22, #168, #169, #172, #174, #93, #94, #113, #123 are separate open gates. #177, #122, #121, #114 remain OPEN until **all** criteria, not merely the H2-F CI, are verified. Human-only merge; private experimental test data cannot be released publicly.
