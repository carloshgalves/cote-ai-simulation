# CSFRV1-10/E5 — crossed adversarial matrix

Issue #144, functional parent #119. Base: human E4 merge
`8c564e91c9841ecb6469e6ac629641f0a788f5cc`. This is local integrated
acceptance, not the independent audit, release approval or First Reference Run.

## Actual experiment

The Linux test `TestReferenceE5CrossMatrix` captures two real A1 Python
supervised signed cuts, then runs the full B+C4+D2+D3 pipeline using those
same signed bytes, not regenerated or synthetic physical outputs. It creates
independent causal and A/B owner stores and compares complete E1 ordered
canonical rows before and after registered recovery/no-op/denial.

Seven positive cases: normal plus no-op; explicit reverse; seeds 1, 42,
2026; seed 144 with epoch 1 step 3 rollback/recovery; seed 999 with epoch 2
step 7 rollback/recovery. Normal and reverse are controls. Seeded runs must
collectively realize both orders at all eight phases of two-element work.
The local deterministic order stream is **not** the world RNG. The
experiment changes arrival of source declarations/closures and responses,
validated unit input order, actual independent validator and candidate
evaluation order for C3 and the resource contest, and collected candidate
and assessment proof order. Fence order, alias arbitration, conflict
resolution, named RNG, clock and sealed authority remain normative. The
physical epochs retain their causal dependency; no claim that epoch 2 can
legally commit before epoch 1. Worker ordering means deterministic sequential
scheduling perturbation, not a claim about arbitrary distributed workers.

Fifteen negative cases: all five canonical ledgers individually, Genesis
policy pin, snapshot, signed causal owner receipt, source cursor, final
source closure, persisted physical source and signature, physical event
visibility, real cryptographic signature verification, and a corrupted
private B-owner checkpoint at a newly signed current R9 cut. Each DB case
changes one genuine row and must fail actual snapshot/replay and registered
recovery guards (or owner restore/WORK gate, as appropriate); a fresh read
of the complete E1 transcript must show no secondary changes. For negative
cases, the clean, attacked and post-denial digests/counts are recorded.
The comparator checks literal bytes in memory, reporting only the first
database/table/ordinal diagnostic, never sensitive row contents.

Normal and race runs each verify the exact 22 experiments, exact Go test
inventory, non-skip status, seed trace, mutation scope, SHA and row counts.
`TestReferenceE5ScheduleDeterministic` proves phase-stream independence;
`TestWorkerOrderCanonicalFencePreserved` verifies full provisional bytes,
explicit rejection of invalid permutations, and both successful/indeterminate
outcomes. The workflow also executes E1–E4/D3 predecessor suites, including
the larger physical fault matrix and C4 two-tail fork. No success is inferred
solely from this documented inventory.

## Reproduction

Use Go 1.27.2 and Python 3.13.18 with the pinned CI dependencies from
`.github/workflows/csfrv1-10e5-cross-matrix.yml`. From the repo root:

```sh
mkdir -m 700 /tmp/e5-evidence
export PHYSICAL_A1_PRODUCER_TEST=1
export CSF_E5_EVIDENCE_PATH=/tmp/e5-evidence/normal.json
set -o pipefail
go test -json -count=1 ./internal/csf/commit -run '^TestReferenceE5' | tee /tmp/e5-normal.jsonl
python docs/reference-run/csfrv1-10e5/verify_matrix.py /tmp/e5-evidence/normal.json /tmp/e5-normal.jsonl "$(git rev-parse HEAD)" /tmp/e5-evidence/normal-verified.json
```

Do not reuse an existing evidence path. The race run uses a distinct path
and separate authenticated A1 pair. It compares identical source bytes
**within** that run, not between separately generated signed pairs.

The report contains checkout SHA, RunID, Genesis and policy digests, hashes
of both real cut/signature pairs, realized scheduling indices, named faults
or mutation, result status, exact table counts and hash/diff diagnostics.
Only these internal hash-only JSON reports and strict verification summaries
are uploaded, not raw SQL databases, owner cognition, physical BodyState,
signature bytes or engine audit logs. Host-only metadata includes scheduling
seed and order, machine/process timing and C3 demonstration operator
enrollment; the wall_time already included in a signed A1 cut is deliberately
held byte-identical, **not** stripped from authority.

## Known limits and gates

E4 demonstrates a positive C4 fork and fail-closed post-D3 refusal: a
registered physical/owner migration remains required for a positive
post-D3 fork. E5 does not bypass that contract. Separate Python producer
checkpoint resume is not demonstrated by replaying authenticated Go ingress.
Do not open #120 or recommend closure of #119 until human merge and the
complete exact-head matrix+regression evidence are confirmed. Do not change
#93/#94/#113 or claim independent review.

A matrix status and the PR's merge decision are separate. On failed or
skipped prerequisites the workflow must not certify a PASS.
