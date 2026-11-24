# CSFRV1-10/H — first private one-command local reference run (#122)

**Scope:** H1 experimental Linux harness, built on the existing B+C1+C2+C3+C4+D2+D3 real Python/Go pipeline. It is not the acceptance gate in #123 and does not close parent #121. Exact base: G6 #175 human merge `d3e9251f2779ba0d65a0546451d9cae711762018`, which attested **621/621 Go↔TypeScript V1** but explicitly concluded `NO_GO_INCOMPLETE_MATRIX` (50 NOT_RUN, 8 BLOCKED). External #93/#94/#113 are still open.

## Execute locally

Use a clean, committed **Linux/amd64** checkout, Go 1.27.2, Python 3.13.18 and the real A1 pinned dependency installation described in `.github/workflows/csfrv1-10h-local.yml`. The command itself takes no network credentials, remote services, LLM, RAG, real protected canon or paid license. Run from the repository checkout and specify a **fresh** directory outside the checkout:

```bash
go run ./cmd/csf-reference-run-local --out /tmp/csf-h-new-01
```

The invocation creates exactly one new local RunID using the real supervised Python A1 twice, registered authenticated source key/policy, and Go InputLedger/Fence/Commit. The original synthetic contest, two actor owner stores, closed/pending sources, C3 authorized retry, claim delivery, temporal advancement, snapshots/replay and D3 disclosure audit all run in the **same causal DB**. The two physical epochs result in nine committed revisions. Failure at any boundary is fatal and cannot produce a success certificate.

On success the command prints `FIRST_REFERENCE_RUN_LOCAL=EXECUTED_AND_CHECKED`, `CSFRV1_10_V1_DOD_ACCEPTED=NO` and `FIRST_REAL_RUN_PRODUCTION_GO=NO`. Those are three different decisions; the first is experimental success, not production approval.

## Private archive format

The directory and owner subdirectory have 0700 permissions. Files are 0600 and never overwritten. The expected file names below are required; if SQLite has left a legitimate private `-wal` or `-shm` sidecar for any database, the archive also records and hashes that sidecar rather than deleting it or ignoring uncheckpointed data:

| Artifact | Meaning |
| --- | --- |
| `causal.db` | Original committed SQLite authority, nine revisions, five canonical ledgers, Genesis, snapshots, triggers, source closures, disclosure and provenance |
| `owner/actor-b.db`, `owner/actor-b.db.actor-a` | Two private, distinct actor cognition owner stores; never publish |
| `registration.json` | Installed A1 public extension manifest, sealed executable/module/import-tree hashes and verifier public key; **no private signing key** |
| `report.json` | Strictly allowlisted D3 public report: authenticated local status, RunID and privacy/disclosure booleans; no raw bodies, stock, actor memory or claims |
| `run-manifest.json` | Source SHA, seed, Genesis, codec/schema policy and A1 registration digests, Go/Python/SQLite versions, Linux CPU/RAM metadata and operational timestamp; marked `NO_PRODUCTION_GO` |
| `checksums.json` | Exact permitted archive paths, byte lengths and SHA-256 for every item except the checksum file itself |

The A1 signer is **ephemeral and in host memory only**, never passed to Python. The producer uses a pinned executable copy and supervised process group with memfd; the recorded manifest contains only public authorization details. Archive files are *private local material*. No CI artifact or public PR comment may contain the raw DB, owner stores, event CBOR, physical captures, signatures, cognitive state, or actor-specific contents.

## Independent verification, without rerunning Python or writing to the archive

```bash
go run ./cmd/csf-reference-run-local --verify /tmp/csf-h-new-01
```

The verifier refuses extra, missing, hardlinked, symlinked or world-readable evidence files; checks the canonical closed JSON shape (rejecting duplicate/foreign fields), fixed index ordering (including any present SQLite WAL/SHM sidecars), file sizes, SHA-256, source SHA, pinned Go/Python and codec/schema identities, no-release classification, public disclosure matrix and physical registration. It copies the original causal database **and any original WAL/SHM sidecars** to a fresh private scratch directory, then verifies the installed policy, exact Genesis hash, linked SQLite core/runtime, nine-commit `ReplayClock`, persisted clock equality and final world revision on the **detached copy** only. It **does not call Python again** and does not modify the original causal SQLite. Permission, path and checksum negative controls, overwrite refusal, forged archive data and a separate real CI run are part of the H1 workflow.

The per-file hash index is **not a trusted external signature**. An adversary able to replace the entire archive and checksum file or compromise the same-UID host is outside this local single-operator check. No completed audit package can be issued solely because the JSON says PASS.

## Independent-source repeatability nuance

Two fresh Python captures legitimately contain independent `wall_time` in their authentic signed source; therefore **two independently executed H commands are not expected to yield bit-identical Event/Input ledgers** even with seed 42. E5 already tests equivalent worker/source/recovery permutations by reusing the *same authenticated A1 signed pair*, not by disguising separate physical evidence as equal. H1 does not claim literal equality between newly generated signed pairs. The reproducible **input identity** requirement for a full H acceptance needs a persistable, replay-only authenticated-cut mechanism with source/creator trust and no new write authority. It is deferred as a separate scope, not patched by removing physical timestamps.

## Runbook — interpret fail-closed states

- `WAIT_FOR_SOURCE_CLOSURE`: the upstream source has not closed the needed causal prefix. Inspect durable closure receipts; do **not** synthesize a closure to force a clock advance.
- `REJECTED_SOURCE`: manifest, Python executable, import tree, key, signature or A1 cut failed admission. Stop and inspect source pins; do **not** relax hashes or substitute an unsigned file.
- `ATTEMPT_IN_FLIGHT`: an existing cohort attempt owns a live fence. Resume/revalidate the persisted attempt; never issue a parallel terminal writer.
- `HALTED_ON_ABORT`: an indeterminate validator aborted the atomic transaction. Review the stored abort/cause and never infer a successful terminal.
- `RETRY_AUTHORIZED`: a specific durable operator retry resumes **the same** fenced membership. Do not silently reselect candidates.
- `CRASH_BETWEEN_FENCE_AND_COMMIT`: cold-reopen the authoritative ledger and validate the frozen fence before any resumed settlement; no producer re-execution.
- `PENDING_EPISTEMIC_BARRIER`: wait for all verified owner ACKs and completions before admitting a later WORK cut. Never query the privileged Observatory to fill cognition.
- Runtime/schema mismatch: the installed Go/SQLite/policy/Python pins differ; restore the approved pinned environment, do not rewrite the Genesis.
- Archive hash/replay failure: quarantine the output directory and diagnose on a detached copy. Do **not** overwrite, repair silently or publish a green manifest.

## Explicit incomplete acceptance / H2 follow-up

H1 verifies that the CLI can execute and package one **genuine** A1/D3 run, and that a detached replay detects unsound temporal or Genesis state. It does **not yet** independently export and compare every canonical ledger, snapshot/owner trust root and signed cut from *outside* the original D3 proof. It does not give a signed WORM archive, durable source-cut reuse across independent commands or comprehensive stop-state fault injection in this CLI. H1 CI must run before claiming even this scoped pass. Additional H tests and complete owner/ledger export or recorded-source replay are required before #122 can close.

The normative G6 decision remains `NO_GO_INCOMPLETE_MATRIX`, with #168/#169/#172/#174 and #22 unresolved; #93/#94/#113 and independent final reviewer #123 are separate gates. Merge remains human-only after adverse review. This H1 tool never claims DoD accepted, independent review or A1 operational/production GO.

## H2-O executable operational runbook

See [H2-operational-runbook.md](H2-operational-runbook.md) for the direct executable contract, typed stop states, operator-confirmed crash semantics, quiescent-only diagnosis and validation requirements. Do not use `go run` to inspect the actual process exit number: the Go wrapper returns its own status.

## H2-E detached operational receipt

See [H2-operational-evidence.md](H2-operational-evidence.md) for the exact binary command/exit chronology, redacted marker receipt, external receipt hash pin and cold verification. The receipt is stored separately from causal authority and remains private and non-attested.

## Final H2 causal report

`ledger-export/causal-report.json` is a private, closed, reference-only projection of the authenticated privileged Observatory. It contains Genesis, committed cycles, fences, decisions, attempts including abort/retry, event and perception authority references, world revision hashes and logical coordinates; no raw canonical payload. It is not a write port or actor context. The full H2 same-A1 profile requires the report, and cold verification recomputes it on detached SQLite from both primary and independently replayed authorities, rejecting rehashed edits or omission. The H2-E receipt also requires its verified marker. Earlier SHA archives must use their own exact source checkout. The report is private and never uploaded. Neither H2 nor the human merge replaces the independent review in #123 or the unresolved G matrix.
