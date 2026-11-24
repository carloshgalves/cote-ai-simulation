# H2-O — executable stop-state runbook (#177 / #122)

Local experimental Linux-only operator tooling. H2-C baseline: #184 human merge `7e2d0b2`, exact-head 24/24, 14/14 archived-source recovery normal/race, and pending-perception worker restart. Neither that proof nor H2-O approves production or the full G/I gates.

## Build once, execute the actual binary

From a clean exact-SHA checkout with the pinned Go/Python dependencies already installed, build outside the checkout:

```sh
umask 077
work=$(mktemp -d)
go build -o "$work/csf-reference-run-local" ./cmd/csf-reference-run-local
"$work/csf-reference-run-local" --out "$work/archive" >"$work/run.log" 2>&1
rc=$?
printf 'run_exit=%s\n' "$rc"
"$work/csf-reference-run-local" --verify "$work/archive"
```

Use the direct executable, not the `go run` wrapper, for exact exit codes. Keep logs private. The run emits the source SHA, expected RunID and public registration digest before the first causal write; preserve those separately as operator-trusted pins. A failed run may contain an incomplete directory; never treat it as a completed archive, delete a WAL, overwrite it or re-run `--out` there. Missing pre-causal identity/registration means diagnosis is unavailable.

## Diagnose a preserved, quiescent causal cut

First establish that the writer is stopped, or obtain a safely captured consistent **detached** copy through an authorized operator procedure. Do not copy a live SQLite database with ordinary cp. Preserve original causal.db, any WAL/SHM and registration.json. Use a private real directory (0700, files 0600). Record the exact source commit and pins emitted by the original trusted run, rather than inventing them from a damaged archive.

```sh
"$work/csf-reference-run-local" --diagnose "$work/archive" --quiescent \
  --source-sha "$SOURCE_SHA" --run-id "$RUN_ID" \
  --registration-sha256 "$REGISTRATION_SHA256"
rc=$?
printf 'diagnostic_exit=%s\n' "$rc"
```

Use `--crash-confirmed` only if an operator has independently established process interruption. A persisted in-flight fence alone cannot prove that its writer died. Neither flag is a lock or a liveness detector. The tool verifies the current checkout, registration pin, expected reference Genesis RunID, preflights WAL/SHM and reads the detached SQLite copy through the privileged read-only Observatory. It rechecks the input hashes before returning. This is causal-state diagnosis, **not** full archive/owner/cut verification, a recovery command, a proof of safe live copying, or a substitute for external authentication. No raw error text, actor payload or key is emitted.

| Exit | State | Required operator action |
| --- | --- | --- |
| 0 | LOCAL_REFERENCE_OK | Only successful out/verify; still local NO-GO for production |
| 1 | UNCLASSIFIED_FAILURE | Preserve evidence; investigate typed failure before retry |
| 2 | INVALID_ARGUMENTS | Correct invocation; no operation selected |
| 10 | WAIT_FOR_SOURCE_CLOSURE | Inspect verified source gap; never fabricate closure |
| 11 | REJECTED_SOURCE | Supervisor rejected physical A1 execution/admission; inspect source pins and runtime privately; never substitute unsigned input |
| 12 | ATTEMPT_IN_FLIGHT | Preserve existing fence; do not start a parallel terminal writer |
| 13 | HALTED_ON_ABORT | No automatic retry; inspect abort and obtain authorized retry if appropriate |
| 14 | RETRY_AUTHORIZED | Existing retry authority only; use native recovery under operator control |
| 15 | CRASH_BETWEEN_FENCE_AND_COMMIT | In-flight fence plus external confirmed interruption; recover frozen attempt, not a new cohort |
| 16 | PENDING_EPISTEMIC_BARRIER | Complete registered perception exactly once; do not admit later rounds |
| 17 | INCOMPATIBLE_VERSION_OR_POLICY | Restore pinned source and runtime/policy; do not reinterpret records |
| 18 | CORRUPT_OR_UNVERIFIED | Preserve originals; investigate failed integrity/replay or unavailable evidence; no repair |
| 19 | NO_CLASSIFIED_STOP | Verified causal projection with no listed stop; not a D3 certificate |
| 20 | SOURCE_CHANGED_DURING_DIAGNOSIS | Reject mixed observation; obtain quiescent cut |

The halted state takes precedence over the barrier; otherwise pending perception takes precedence over admission/retry. A closure gap at the next eligibility coordinate is meaningful only for idle selection, never for a frozen attempt. Rejected source is an immediate typed supervisor failure, not an inferred persisted rejection: the Observatory still reports rejected ingress as NOT_PERSISTED.

## Evidence and tests

The private H job builds the real command, runs it on actual durable cuts of the same archived signed A1 inputs (source gap, in-flight, halted, authorized retry, physical fault, pending perception), checks direct process exit codes and original bytes, verifies the complete archive separately, and rejects wrong source version, registration digest, corrupt DB and invalid arguments. Real supervisor rejection and closure wait are tested at their typed boundaries. Unknown failures never masquerade as one of these states. Normal/race H2-C remains intact; exact-HEAD H2-O evidence must come from the CI, not from this prose.

Remaining: durable chronological operator reports with source/commands/exit codes and redacted logs (H2-E), external independent review and outstanding normative/production gates. Full driver death, power-loss and untrusted same-UID filesystem isolation are not established by this harness.
