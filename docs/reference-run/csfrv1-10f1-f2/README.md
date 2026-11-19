# CSFRV1-10/F1–F2 — Observatory integrado

Parent #120, slices #151 and #152. Base `cb8459e7921f8ae6ef78b0febdbc9739a515aebb`
(E5 human merge, 16 workflows passed). Increment O1–O4 rather than
introducing another causal engine or a publication policy.

## F1 — privileged provenance

`PrivilegedReader.ReadPrivilegedReport` uses the existing read-only connection,
verified Genesis / ReplayClock, immutable attempt and commit rosters, and
connection-relative data-version fence. Its privileged contract adds:

- Genesis: actual RunID SQL key, derived GenesisID, normative digest and
  canonical-content hash (not seed or manifest bytes).
- Every terminal DecisionRecord: admitted source ref, source/decision digest,
  status, optional candidate, referenced conflict and draw, contested
  assessment, and exact produced Event refs. Lifecycle Events without an
  individual decision owner are retained in the commit roster. NoProposal
  retains an absent candidate; no fabricated candidate for ClockAdvance.
- Each task is bound to its Event and optional completion; Observation and
  KnowledgeInput refs remain separated by the verified completion roster.
- Historical aborted and operator-retried attempts remain separate from
  the committed successor. Missing or foreign roots, unreferenced
  candidate rows and invalid proof digests cannot yield a partial report.

The report is for local privileged human diagnosis only, never actor
context or a public route. The only fields are explicit DTO fields in
`internal/csf/observatory/privileged.go`; no raw canonical payload or owner
data is returned. ContentHash is the opaque hash of stored canonical bytes,
not a substitute for registered domain-specific digest validation.

## F2 — operational status

`PrivilegedReader.ReadOperationalReport` is a distinct human-only query.
It starts from the existing fully verified privileged report, then uses
the normative pending clock membership and exact latest ingressed closure
per source. It returns the persisted control, minimum pending coordinate
(if any), pending Round / Exogenous / Occurrence / Trigger / Trigger Repeat
refs and coverage, pending perception count and the explicit
`NOT_PERSISTED` rejected-ingress marker. It never counts immutable
lifecycle=0 rows as live merely because their historical flag is zero.

Source closure distinguishes FINAL (irreversible), COVERED (not final but
covers next target), GAP (target not covered), NO_TARGET (nothing pending;
not an inferred final closure). NextPendingEligibility is explanatory,
not a license to open a new fence during an abort/retry/in-flight cut.
The outer read-only data-version check rejects changing authority across
the extra reads. An invalid/mismatched installed codec/extension is
ErrInvalidPolicy, replay/projection corruption ErrInvalidReplay, observed
concurrent source change ErrSourceChanged. Invalid status is never an
apparently successful report. Cancellation remains an error.

## Reproducible evidence

Pinned Go/Python A1 dependencies and real signed source cuts as in E5.
Execute from the repository root:

```sh
export PHYSICAL_A1_PRODUCER_TEST=1
set -o pipefail
go test -json -count=1 -timeout=10m ./internal/csf/commit -run '^TestReferenceF[12]' | tee /tmp/f-normal.jsonl
python docs/reference-run/csfrv1-10f1-f2/verify_tests.py /tmp/f-normal.jsonl
go test -race -json -count=1 -timeout=15m ./internal/csf/commit -run '^TestReferenceF[12]' | tee /tmp/f-race.jsonl
python docs/reference-run/csfrv1-10f1-f2/verify_tests.py /tmp/f-race.jsonl
```

F1 runs the genuine 9-commit D3 on the real causal and two owner stores.
Independent SQL reads check reported refs/hashes and the E1 oracle checks
every stored row before/after queries, mutations and restoration.
Candidate corruption and orphan candidate controls must fail closed.

F2 stops the real reference harness at closure gap, fenced in-flight,
halted abort, authorized retry after genuine reopen, and post-clock
advance before actual trigger settlement. No fake lifecycle rows are
written. The completed D3 control checks final closures and rejects
mutated physical Event authority and the wrong installed policy. Intermediate
cuts use the causal transcript when owner stores have not yet been created.
The workflow verifies the named non-skipped inventory on the exact PR head,
under both normal and race instrumentation, with O1–O4 and D3 regressions.

## Scope and review

F1/F2 are one integration branch, with changes reviewed by slice. F3 four
views cross-comparison, F4 concurrent read-only/sidecars/hardlinks/import
boundary and F5 final acceptance are deliberately later work. Existing
read-only path/SQLite guarantees are reused here; F1/F2 do not claim
complete F4 isolation. F2 reference harness stop flags are unexported;
neither a new public run-control API nor a new normative transition.

No UI, HTTP endpoint, expanded public access, release attestation or
production claim. Gates #93/#94/#113 unchanged. E4 still documents the
post-D3 positive fork limitation. Human merge only after exact-head
green evidence; no independent audit claim.
