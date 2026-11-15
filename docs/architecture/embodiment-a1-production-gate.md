# A1 physical producer — production trust and First Real Run gate

**Status:** DEVELOPMENT VERIFIED, PRODUCTION NOT ATTESTED. This runbook is an
acceptance contract, not a certificate that any host has been hardened.
PR #110 must remain draft until an independent reviewer and the operator
supply concrete evidence. The signed A1 cut and a green CI do not prove
that the OS deployment is a trustworthy source of physical-world truth.

## Exact registered authority

1. Install the verified base policy bundle. Install one canonical
   `cote.embodiment.physical-registration.v1` manifest; persist a Genesis
   with its **exact extension SHA-256 and `physical.a1` source ID**.
2. The registered manifest binds the Ed25519 producer key, Python executable
   SHA-256, A1 entrypoint SHA-256, and the **producer import-tree SHA-256**.
   The tree digest covers all Python modules under `src/embodiment` and
   physical YAML/JSON parameters in `data/models/physical`, including
   `population-prior.yaml`, `body-dynamics.yaml` and its JSON schema.
   Non-Python modules, unregistered parameter-file types, symlink entries
   and preexisting `__pycache__`/compiled bytecode fail closed: Python `-B`
   does **not** prevent *reading* malicious existing pyc files. A1 must be
   deployed without caches, and the full source/model tree is verified
   before launch and again before signing.
3. The host executes the *authenticated binary bytes* from a sealed Linux
   memfd at `/proc/self/fd/3` (not the original executable pathname).
   The producer receives no signing private key, canonical SQLite handle or
   global CSF clock. A successful exit is followed by a process-group
   termination request and immutable snapshot/log capture.
4. Go independently verifies the A2 completion and physical typed projection.
   The signed cut is still **not a commit capability**. It must pass the
   `InputLedger → SourceClosure → AdmissionFence → CommitCoordinator →
   SQLite.CommitWork` atomic terminal. Events remain ENGINE_ONLY.

## P0 operating-system trust requirements — not discharged by CI

The preceding controls authenticate the bytes observed by the host, but
**do not prove** that `PYTHONHOME`, installed wheels, native extensions,
libc, dynamic linker, Python standard library or the filesystem remained
unaltered while the producer executed. Checking the import tree before and
after launch does **not** stop a same-UID attacker from swapping and restoring
files between those checks. `Setpgid` and `Pdeathsig` are not equivalent to
cgroup containment of descendants.

An operator must therefore provide, *before a production signature*:

- A signed, reproducible deployment image digest and SBOM covering the
  interpreter, its dynamic libraries, the Python standard library, every
  wheel and native extension, the A1 package, physical model data and the
  verified policy/Genesis manifest. Pin **transitive** package versions and
  artifact hashes; do not rely on version ranges or live network installs.
- A **read-only, root- or deployment-owner-controlled** image/mount that the
  A1 worker UID cannot modify, replace or remount. Check parent-directory
  ownership, write bits, symlinks, mount topology and package hash in the
  **actual** container/host, not solely in the CI checkout.
- A separately confined worker identity and kernel-enforced containment
  (for example a dedicated cgroup v2/systemd unit and restricted user/mount
  namespaces), without network or ptrace access to the signer, database or
  other processes. Demonstrate that forked children cannot escape termination
  and cannot write an artifact after the cut is declared quiescent.
- A confidential signing-key store available **only to the Go supervisor**;
  prove via OS permissions/credentials and negative tests that neither the
  Python worker nor any descendant can access it.
- A private ephemeral staging volume writable only as needed, with a
  bounded lifetime and deletion after seal/sign. Preserve enough immutable
  audit evidence to reproduce the source-root and complete cut verification
  without exposing physical data to the public/observer surface.

If any one requirement is unproved, mark **GOP-R03 / NO-GO**. Never add a
runtime fallback to a mutable executable path, a caller-selected producer
key, a noncanonical policy, an unsealed fd or a public physical Event.

## First Real Run / independent acceptance

Freeze the source bundle, canonical schema, Genesis hash, build image, A1
plan/epoch, code SHA, test protocol and decision thresholds **before** the
acceptance run. On the target OS capture:

1. The exact deployment digest, identity and filesystem/namespace/cgroup
   attestation, including a demonstration that a same-UID file mutation and
   a process-group-escaping child are denied by the *OS* boundary.
2. A real A1 multi-epoch run and its signed snapshot/log cut; independent Go
   A2 verification; complete SourceClosure/AdmissionFence provenance.
3. One atomic physical EventDraft, DecisionRecord, EventStore item,
   WorldRevision and CycleCommit, with seven crash-injection rollback points,
   duplicate rejection, altered-source/manifest tests and ReplayClock
   invariance after SQLite reopen.
4. Positive fork only from a finalized source cursor; negative fork while
   the source is still open. Verify no parent/child aliasing of run-bound IDs.
5. A3/observatory authorization review: physical engine-only values do not
   create observer PerceptionTasks or public reports, including after
   replay, snapshots and forks.

Record outcome per check, exact SHA and job URL. A green CI is **necessary,
not sufficient**. Obtain an independent human review of PR #110 and the A4
dependency (#112), then an explicit human release/merge authorization.

## Existing automated evidence

The Go physical workflow covers preflight, sealed fd/execute path swapping,
source and model-tree mutations, atomic commit/replay/fork, two consecutive
epochs, and the whole Go regression suite. Consult the actual **HEAD**
workflow run, not an earlier passing commit. Unattested OS and independent
human gates remain open in issue #108.
