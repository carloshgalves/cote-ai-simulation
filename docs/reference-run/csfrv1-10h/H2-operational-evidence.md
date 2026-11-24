# H2-E — detached operational evidence (#177 / #122)

The receipt records two **actual child process exits** and operational UTC chronology, not simulation time, causal ordering or actor knowledge. It is a private sibling of the existing H2 archive, not part of its recursive checksum index: the cold verifier can finish before its exit is recorded without mutating the archive. The exact source SHA, executable SHA-256, original archive index and manifest SHA-256, RunID, Genesis and registration pins bind the sibling to the same verified run. A zero RunID is valid.

From the exact clean Linux checkout, with pinned dependencies installed:

```sh
umask 077
work=$(mktemp -d)
go build -buildvcs=true -o "$work/csf-reference-run-local" ./cmd/csf-reference-run-local
python docs/reference-run/csfrv1-10h/operational_evidence.py record \
  --binary "$work/csf-reference-run-local" --archive "$work/archive" \
  --evidence "$work/operational" | tee "$work/summary.txt"
```

Use Bash with `set -o pipefail` when piping the command. The receipt is written exclusively to `operational/operational-receipt.json`, mode 0600, in a fresh 0700 directory. The two symbolic commands replace only local binary/archive path arguments; the executable bytes are pinned independently. Fixed allowlisted markers are the redacted log, not a transcript of stderr or raw runtime output. The pre-causal source SHA, RunID and registration digest are captured only in their closed hex formats, separately from logs, and checked against the completed archive. A failed command can preserve the pins it actually emitted; absent or ambiguous pins never become authority. Each child returncode is recorded directly (negative means termination by signal); wrapper exit 1 denotes incomplete/unverified evidence. Failure after a child has returned preserves its observed exit in an incomplete receipt. An externally interrupted recorder need not leave a receipt; never infer success from its absence.

Save the emitted `H2_E_RECEIPT_SHA256` independently of the directory. Verify the sibling with the same exact executable and checkout:

```sh
python docs/reference-run/csfrv1-10h/operational_evidence.py verify \
  --binary "$work/csf-reference-run-local" --archive "$work/archive" \
  --evidence "$work/operational" --receipt-sha256 "$RECEIPT_SHA256"
```

Verification checks the pinned canonical receipt, strict command order, actual recorded exit zero and success markers, temporal containment of the original manifest start time, exact source/executable/manifest/index/registration/Genesis/RunID pins, and invokes the existing full cold verifier without executing A1. The original archive file and directory roster, digests and receipt bytes are checked before/after. The private CI also verifies a genuine receipt, rejects rehashed semantic mutants, and records a real incompatible interpreter exit (17) without calling the cold verifier or minting a completed package. Do not use this wrapper on a live writer. The receipt proves no external clock trust, no external signature, no process death or power-loss recovery, and no malicious same-UID isolation. Local SHA-256 alone cannot attest a rewritten archive plus rewritten receipt; the separately retained pin is necessary.

H2-O remains the stop-state runbook and its real six-state normal/race regression. H2-E does not replace that matrix or claim that unsuccessful invocations have completed archives. It is not an independent auditor signature or production GO. The full G matrix, #93/#94/#113, #123 and final human review remain open.
