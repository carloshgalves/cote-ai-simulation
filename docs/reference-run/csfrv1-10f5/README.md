# CSFRV1-10/F5 — Integrated Observatory acceptance

Parent #120 / slice #155. Baseline is human merge #158
`ecf8f2f7d15b1469b95035970d42d4a112168657`, with 26/26 green
checks at F4 HEAD `7c9af57f7534bed8484aa9539a77466d7cdc9ff8`.
F1 #151, F2 #152, F3 #153 and F4 #154 are merged. F5 is not an
independent external audit, production homologation, deployment or release.

## Acceptance criteria and evidence

| #120 criterion | Exact executable witness |
|---|---|
| Causal provenance, signed inputs, commitments and abort/retry | F1 integrated provenance; F5 all fields of terminal report across same-source executions |
| Persisted diagnostic cuts and fail-closed replay/version | F2 closure-gap, in-flight, halted, authorized retry, pending trigger, final and corrupt; F5 genuine overlapping authorized writer |
| Explicit per-view allowlists and foreign/physical/private denials | F3 crossed views and restricted DTOs; F5 human-only recursive DTO allowlists |
| No query write port / sidecar and OS guards | F4 actual queryhost transitive imports, independent SQL pager and connector, capability segregation, links, real Linux ACL and metrics guards |
| No durable mutation, including concurrent/repeated reads | F4 72 queries on real D3, E1 full transcript and main/WAL/journal files; F5 all E1 rows and durable files across three executions and metrics perturbations |
| Fail-closed live WAL, not an immutable stale copy | F5 live uncheckpointed cursor tampering, main file equal, denied warm/cold readers and exact restoration |
| Scheduling/recovery invariance outside actor cognition | F5 normal, reversed realized scheduling, epoch-two step-seven fault/recovery with actual signed pair held constant |
| No silent regression or skipped acceptance | F5 exact test+package inventories, normal/race and O1–O4/D3/E1 regression inventory; scoped packages and vet |

The composed F5 test reads run-state, privileged causal report, operational
report, POV A, POV B, and public report from each completed D3 run. Every
field is compared, not just a count or a digest. Independent real supervised
Python A1 cuts are obtained once per matrix invocation and reused unchanged.
Each of the three executions has nine commits, ten attempts, both owner DBs
and one causal DB. Positive and recovery proofs must be non-vacuous; the
reverse ordering is checked against the realized eight scheduling phases.
Metrics vary host, worker, wall-time, duration, retries and call counts,
then are actually read; they must not influence any of the causal projections
or mutate authority. The metrics recorder is not a simulation input.

The live WAL negative case keeps a writer open with autocheckpoint disabled,
changes an authoritative source cursor without changing the main file, and
proves the WAL contains frames. Warm and cold queries must see invalid
authority, return no partial view, leave the damaged transcript and bytes
untouched, then read the exact original projection after explicit restoration.
A separate SQLite test wraps the actual read authority with deterministic
test-only post-replay/post-prefix hooks. An independent writer commits legal
closures while the query is open; logical state remains identical but the
connection's data_version changes. Both the run and outer operational fences
must reject and permit the next stable query.

## Evidence integrity

F5 discovered a blind spot in E1/O1: SQL LIKE with an unescaped underscore
could omit a legitimate near-reserved table. The oracle now uses GLOB with
a literal underscore and is tested against a genuine SQLite table whose
name begins with sqliteX. This does not change the causal engine.

The workflow runs the exact checked-out PR head (not a synthetic merge ref),
using Go 1.27.2, Python 3.13.18 and real supervised physical fixtures. It
executes 40 named F1–F5 tests/subtests in each uncached normal and race matrix,
plus the 34 named O1–O4/D3/E1 regressions, dependency/owner/persistence
regressions and vet. The certificate verifier rejects missing starts/passes,
duplicates, skips, cached or failed packages, unexpected tests, incomplete
package runs, an incorrect checkout SHA or reused input logs. Its own
negative controls execute before the real integration matrix.

On success the CI publishes only `acceptance.json`:
- exact verified SHA, CI run/attempt, explicit test/package inventories;
- SHA-256 of each complete JSONL execution log;
- per-case E1 before/after manifest digests, counts and equality across the
  three real executions, independently in normal and race mode;
- external gates NOT_ATTESTED, release_authorized=false and human merge
  required.

Raw observations, private owner checkpoint bytes, world state/seed, A1 signed
payload, absolute paths and combined actor reports are not included in the
certificate. Job logs remain the execution source, not a substitute for
review. It is not a cryptographic third-party signature or proof of
independent review. The runner and test code are trusted inputs; the exact
SHA and workflow identify what was executed.

## Threat model and disposition

F4 local operator workspace and trusted-UID constraints remain: no promise
of a sandbox against malicious same-UID/root processes or a separate
writer-free executable. The real query implementation's import graph excludes
writer/sink packages; the trusted SQLite composition root still reuses the
normative replay. SHM coordinates SQLite locks and indexes, not durable
causal authority; literal durable-file equality concerns main/WAL/journal
bytes, and E1 compares exact logical rows and schema.

Keep #93/#94/#113 untouched. Keep #120 and #155 open until the human merge
and verification of successful exact-head CI. Technical closure of F does
not close #114 or authorize external acceptance.
