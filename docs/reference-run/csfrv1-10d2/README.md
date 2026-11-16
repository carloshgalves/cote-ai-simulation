# CSFRV1-10/D2 — real Python A1 in one C4 causal RunID

Issue #135 / parent #118 / epic #114. PR #138 stays Draft; merge exclusively by human decision.

## Causal sequence — one SQLite, one authenticated Genesis, one RunID

| World revision | Go instant / ordinal | Actual transition | Security proof |
|---|---|---|---|
| 0–6 | 0→35 | C3 abort/retry + B/C1/C2/C4 resource, false Claim, due delivery and private A/B checkpoint | SnapshotV1 with five sealed ledger cursors |
| 7 | 35 / 1 | Two outstanding C2 A/B dispatches answer signed NoProposal; real Go fence + one world-neutral CycleCommit | Two native round.consumed EventStore entries and completed PerceptionTasks, body and stock unchanged |
| 8 | 35 / 2 | Real Python A1 captured and signed epoch 1 → Go StagePhysicalInput → AdmissionFence → CommitPhysical | One authoritative physical WorldRevision and ENGINE_ONLY physical.advanced Event |
| 9 | 35 / 3 | Real Python A1 captured and signed epoch 2 → final physical SourceClosure before fence → Go physical commit | Second physical WorldRevision, same resource stock, monotonically advanced body |

The producer has no Go clock, EventStore, InputLedger or public actor authority. Python seed, days, source-local sim_time and source wall_time cannot change the global Go instant, which stays 35 throughout the physical terminal sequence.

## Real source security and limits

The installed physical manifest pins the executable hash, Python module hash, complete import-tree hash, signing public key, physical state/action schema, producer source and ENGINE_ONLY disclosure. The existing supervisor starts an authenticated sealed Python interpreter memfd with -S, a private 0700 staging root and no shell, collects PSV1-2 snapshot and events plus A2 evidence, validates the source and signs the exact ApprovedCut after the subprocess exits. Go independently authenticates that signature and bound Genesis before source ingress. All ordinary D2 executions actually run Python twice.

**Important limit:** the current A1 capture protocol reseeds from the same deterministic physical seed for each epoch, with a separate cumulative duration plan (for example, 2 days followed by 3 days). The Go world state persists and advances monotonically over both epochs. This does **not** demonstrate resuming a prior Python snapshot in place or source-chain attestation across Python processes; PSV1-8 recovery remains a separate issue.

**Time provenance:** raw Python events contain authentic wall_time. Repeated source captures naturally have different raw hashes even when the model result is equal. The deterministic permutation test must first capture and sign two genuine A1 cuts, then reuse those **exact source inputs** in two otherwise independent C4 runs. The package-private frozen-cut test fixture is not accessible as a production request parameter, and signed-cut/Genesis/fence validation still runs. The normal D2 path never uses frozen cuts and invokes the real A1 supervisor.

## Reproduce on Linux

Install the pinned test environment, produce the A2 evidence, and clear Python bytecode before import-tree attestation:

~~~bash
python -m pip install --disable-pip-version-check --no-input -e '.[dev]' \
  'numpy==2.5.4' 'pydantic==2.14.3' 'PyYAML==6.0.3' \
  'pytest==9.2.0' 'jsonschema==4.26.0' 'hypothesis==6.170.15'
python -m tests.architecture.emit_go_physical_fixture --out /tmp/physical-go-fixture-d2
find src/embodiment -type d -name __pycache__ -prune -exec rm -rf {} +
PHYSICAL_A1_PRODUCER_TEST=1 go test -count=1 -v ./internal/csf/commit -run '^TestReferenceD2'
PHYSICAL_A1_PRODUCER_TEST=1 go test -race -count=1 ./internal/csf/commit -run '^TestReferenceD2(RealTwoEpochsOnSameC4RunID|TwoSourceWorkerOrdersYieldIdenticalAuthenticatedPhysicalTail)$'
go test -count=1 ./internal/csf/commit -run '^(TestReferenceC4|TestPhysical)'
go test -count=1 ./internal/csf/reducer ./internal/csf/genesis ./internal/csf/policy ./internal/csf/adapter/embodimentv1 ./internal/physicalhost
~~~

PHYSICAL_A1_PRODUCER_TEST=1 is mandatory; otherwise the integration tests SKIP and cannot be counted as successful. CI uses Go 1.27.2, Python 3.13.18 and the pinned dependencies.

## Adversarial acceptance matrix

| Gate | Evidence |
|---|---|
| Open exogenous source | With LeaveSourceOpen, C4 admission fails before the first contested commit |
| Fake signing private key | Rejected before creating the causal database |
| Wrong Python executable or import root | Physical source fails validation without committing a physical WorldRevision |
| A1 cut identity and epoch | Reusing signed epoch 1 as epoch 2 is rejected, preserving only the first committed epoch |
| Exact closure window | Source is closed non-final through C2 NoProposal; final closure occurs after second signed ingress but before the second physical fence |
| Atomic failures | Injected at steps 1 and 7 of each epoch; no partial world revisions, CycleCommits or physical.advanced EventStore entries |
| Go-owned context | The inventory remains 0 after C4; physical Go reducer preserves the authenticated resource branch bytes |
| World continuity | Same RunID/Genesis/SQLite, two distinct signed A1 input roots and events, cumulative body state, nine committed revisions |
| Replay and snapshot | Native ReplayClock verifies nine commits; original two-owner C4 SnapshotV1 verifies exactly three tail commits |
| Worker/source order | Byte-identical five-ledger prefixes and Go WorldState when **exactly the same genuine A1 cuts** are supplied |
| Public privacy | Explicit allowlisted report only; no raw Claim, body values, future canon, stock, private signing key or world hashes |

**Remaining boundaries:** D3 #136 must prove integrated ENGINE_ONLY disclosure to actor/public/Observatory contexts. Full crash/fork recovery #119, independent production trust and gates #94/#113 are not proven by this test harness. No independent external security approval is claimed, and no merge is authorized by this document.
