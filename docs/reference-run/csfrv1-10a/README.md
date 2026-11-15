# CSFRV1-10/A — Reference Run preparation (issue #115)

**Status: PLAN_FROZEN / REFERENCE_RUN_NOT_RUN / GENESIS_NOT_ATTESTED.** This is **not**
a passing causal E2E run, a physical A1 authorization, or acceptance of CSF V1.
It does not resolve the separate review gates #93, #94, and #113.

## Authority and frozen baseline

The declared starting point is `main@a20163b1d5124648192f3adff7c330ed5b4350fe`,
tree `0ee27ed49a36a42feddd7afce87d29b74f184493`. ADR 0008 §14 and its
stress table, the spec §1 and §11.5, and ticket 10 are individually pinned to
their original Git blob IDs by `manifest.json`. These pins are read-only
checks against the **working repository**: a later branch may add tests, but
cannot silently rewrite the normative sources to make them pass.

The V1 codec/schema/conformance hashes are those authenticated by
`docs/architecture/canonical-codec-v1-bundle/BUNDLE.sha256`.
`scripts/csfrv1_10a_preflight.py` also checks every policy artifact by
byte size and SHA-256, and checks the domain-separated manifest digest. Do not
replace these with hashes of JSON after parsing it.

Go is frozen at `1.27.2`, the Python *reference machine* at `3.13.5`
(with project constraint `>=3.11`), and the Go SQLite driver at
`modernc.org/sqlite v1.60.1`. The actual SQLite **core** version linked by
the Go driver is **NOT_ATTESTED**; measure it with the read-only
`go run ./tools/csfrv1_10a_sqlite_probe.go` and pin that result before
`--execution` is authorized. `python -c 'import sqlite3; ...'` and the
system `sqlite3` program are not equivalent to the Go-linked engine.

## Frozen synthetic topology

The immutable *declarative* fixture is `scenario.json`. Use
`world_seed_hex = 000102...1f`, timezone `Asia/Tokyo`, instant 0, ordinal 0,
two exogenous sources `school-events` and `physical-a1`, two partial
closures through `(0,0)` and two final closures through `(10,0)`, and
producer-controlled E1/E2 keys. R1/actor A and R2/actor B share
`base_revision=0` and one slot `act` each; complete slots in the opposite
order to their declarations. One resource unit exists initially. Never
hardcode which proposal wins: infer all winners, events, global sequence,
revision, fence and state digests from the runtime's persisted canonical
records, then compare exact bytes across permutations.

A scheduled notification is due at instant 10. A resource-exhausted rising
edge must produce one durable trigger activation under the versioned policy.
A false accusation is a `Claim`, not world truth. `exam.vip` and A1 raw
physiological state remain private; POV B and public views may only consume
individually authorized evidence. The observer's privileged query is not a
causal write or a new `KnowledgeInput`. Python's local `seq` and `sim_time`
remain physical provenance; the Go coordinator exclusively derives global
logical sequence, instant and published world revision.

**Genesis limitation:** `manifest.json` defines the Genesis *parameters*, not
fabricated canonical CBOR. No `run_id` or digest is invented. Only the verified
Go Genesis builder/loader with registered source, extension, policies, schemas,
initial world, and per-actor epistemic checkpoints can seal that artifact
when #116/#118 implement the execution. The unapproved physical A1 source
must not be promoted by a matching filename, SHA supplied by the producer, or
legacy `seq` position. Until that gate, `--execution` MUST fail.

## Matrix and ownership

`traceability.json` contains one row for each of **41 invariants** and
**68 stress scenarios** copied from the exact ADR, with links to planned
named integration tests, primary child issue, and empty evidence/SHA slots.
Current statuses are all `NEEDS_NEW_INTEGRATION_TEST`. A row's presence is
not a test pass. In this #115 frozen planning artifact, the checker
**rejects** any `EXISTING_PASS_EXACT_SHA` claim, even a fake evidence path
and SHA; a later versioned executor/attestor must be implemented before such
a transition can be trusted. Baseline, normative blob IDs, toolchain and
bundle hashes are independently pinned in the checker, rather than trusted
solely from a mutable manifest. Invariants 1–16 are authority; 17–34 admission/cohorts;
35–40 triggers/events; 41 epistemic chronology. Child issues #116–#120
supply most integration behavior; #121 collects test/eval coverage.

Stress inventory includes E1/E2 ordering, partial/final closures, concurrent
rounds, reverse responses, idempotency aliases/conflicts, abort and explicit
retry, clock advancement, triggers, all crash points, intermediate
snapshots, fork and replay, POV visibility, world and knowledge hashes.
The plan's permutation IDs are **preconditions to test**, not results.

For the scenario acceptance oracle, require **exact** byte equality for
the five exported ledgers, agenda/trigger runtime, commit cursors, event IDs,
state hashes, and snapshot across continuous, permuted-worker, replay, and
resume executions. A JSON semantic diff is insufficient. Differing fork
tails are expected, but the parent prefix must be identical and tails
isolated. No order-of-arrival/wall-time/worker-ID branch may choose a winner.

## Commands and outcomes

At the repository root:

```bash
python3 -m unittest discover -s tests/reference_run -p 'test_*.py' -v
python3 scripts/csfrv1_10a_preflight.py --structure-only
python3 scripts/csfrv1_10a_preflight.py
go run ./tools/csfrv1_10a_sqlite_probe.go
python3 scripts/csfrv1_10a_preflight.py --execution
```

`--structure-only` is a reduced check of normative docs, Git baseline and
bundle integrity, **not** version validation. A normal preflight checks the
exact Go/Python runtime and SQLite driver dependency, and may fail on a
machine outside the pinned reference toolchain. `--execution` is
intentionally fail-closed until signed Genesis and Go SQLite core version
are attested, and subsequently requires a real harness run (issues #116–#122).
Do not weaken the check just to make the command green.

After the future harness exports two independent directories, compare:

```bash
python3 scripts/csfrv1_10a_preflight.py --compare artifacts/continuous artifacts/replay
```

Each export must supply `input-ledger.bin`, `decision-ledger.bin`,
`event-store.bin`, `evidence-ledger.bin`, `snapshot-store.bin`,
`agenda-runtime.bin`, `commit-cursors.bin`, `event-ids.bin`,
`world-state.sha256`, and `knowledge-state.sha256`. The preflight
compares bytes and prints SHA-256. Merely creating identical placeholder
files does **not** prove a valid run; each input must be exported and
validated against the ledger's authenticated commit chain by the future
harness. Any missing file, differing byte, unsafe alias, unpinned policy
or unexpected version fails closed.

## Review and release gates

A review of this preparation may validate completeness, stable source
extraction, negative controls and failure behavior. It must not certify an
integrated scenario, developer-independent audit, CI runner or external
production approval that has not occurred. CI failures with zero runner/steps
are `CI_NOT_RUN`, not `TEST_FAILED` or an assumed billing proof. Any V1
Definition-of-Done gap holds `NO_GO` even when a partial run exists. Merge
requires an explicit human decision, separate from a draft PR or clean
document review.
