# CSFRV1-10/F4 — Concurrent read-only Observatory and sidecar isolation

Parent #120, slice #154. Baseline: human merge of #157 at 242faa93b20f0d3a24d7723378a68e69e748d4fd (24/24 checks). No UI or remote access. All inputs and replay remain owned by CSF.

## Architectural boundary

The actual projection algorithms (run-state, privileged provenance, operational status, POV, public) now live in `internal/csf/observatory/queryhost`. Its **transitive import graph** must exclude the engine store, write ports, input ledger, commit coordinator, metrics sink, owner checkpoints and physical host. This is stronger than checking only DTO imports. The existing SQLite package is the trusted composition root: inject separately typed read-only capabilities into the queryhost and reuse the normative engine verifier. Read-only replay explicitly uses a read transaction and restores the input prefix through `RestoreReadPrefix`, which never returns an append-capable Ledger. No second replay implementation. A/B and public receive nominal distinct wrappers: type assertion cannot recover the human audit/raw-query interface. The engine composition root itself necessarily also hosts the existing writers; it is **not** claimed to be a separate writer-free executable or sandbox. The projection package cannot import them.

SQLite's `mode=ro` and `query_only=ON` are independent protections; the new `readonlysql` connector additionally denies Exec/DDL/DML/ATTACH/writable pragmas, prepared statements, extra statements and non-read-only transactions. It admits only fixed internal SELECTs and a closed read-PRAGMA list. It cannot transparently reconnect (data_version is connection-relative), and exposes no raw driver. This is not an arbitrary SQL API. The existing version/policy/replay and data_version fences still apply; failure returns no partial projection.

## Local filesystem contract

Linux local operator-owned SQLite families only. Every ancestor is validated without following links; root-owned sticky ancestors such as /tmp are allowed, but the actual parent must be operator-owned and non-group/other-writable and within a private (0700) ancestor. File, WAL, SHM and rollback journal must be regular, single-link, owner-owned and not group/other writable. Default parent POSIX ACLs are refused before opening/creating pager files. Recheck the original main and parent inode and current file-family permissions at each fenced query. ACL test exercises real named-user and default ACL xattrs, not only simulated modes.

The guard assumes a trusted local operator/UID and filesystem; root or a malicious process running under that same UID can still replace files concurrently. This is not an OS sandbox or a claim of TOCTOU resistance to privileged adversaries. Do not expose path arguments to an untrusted remote caller. Use a private workspace; existing files outside such a workspace must be migrated by the operator. Live WAL is never ignored with immutable=1. SQLite SHM is transient lock/index coordination; it may change during read-only operation, is not a durable causal log and is not included in durable byte equality. Main, WAL and rollback-journal bytes, and every E1 row of causal/A/B stores, must remain equal in the quiescent concurrent experiment.

## Operational metrics

The metrics file and its entire reserved pager family must be physically distinct from the causal file and its family. Reject ordinary or dangling links, hardlinks, unsafe ACLs, recovery companions, reserved pager names, renamed path identity, and foreign schema (even matching user_version). New files use exclusive 0600 reservation, not mode=rwc on arbitrary paths. Existing files are preflighted read-only and checked for rollback-journal header before any write-capable open. Exact table/index schema and user_version are reverified inside each transaction. No schema mutation on existing files, no unregistered triggers, no reading/writing causal authority, and no implicit recovery of a dirty metrics file. A failed first creation may leave a reserved empty file; remove it only through explicit operator recovery, not an automatic deletion of an unknown path. Existing metrics stores use DELETE journal mode. The recorder remains an extracausal sink, never injected into the queryhost.

## Acceptance tests

- Integrated real D3 with authentic supervised A1 cuts: independent E1 full transcript before/after concurrent 12 workers x 6 surfaces (privileged, operational, A, B, public, metrics). Check complete baseline/equality, and literal main/WAL/journal bytes of all three authority DBs plus metrics file.
- Causal paths: symlink, hardlink, ancestor symlink, group-write permissions, pager link to immutable log. Actual Linux named-user/default ACL and byte-identical inode replacement. Verify no causal mutation and safe restoration.
- Metrics: complete reserved pager family, both symlink and hardlink aliases to log, orphan regular journal, foreign version-spoofed DB, live version change/trigger/inode replacement. Preserve protected bytes.
- Driver independent write denial including SQL-query path, plus unwrapped pager mode=ro test with query_only disabled. Explicitly prove no transparent connection replacement. Nominal actor/public capability type-assertion denial and transitive import test of the real queryhost.
- Exact-head normal and race inventory; F1–F3, D3, O1–O4, input ledger, persistence and architecture regressions; static vet. No skipped required test. All evidence refers to the exact current HEAD. Human merge only, F5 and #93/#94/#113 remain untouched.

The read-only operational projection and runtime checks do not constitute an independent external audit or production deployment release.
