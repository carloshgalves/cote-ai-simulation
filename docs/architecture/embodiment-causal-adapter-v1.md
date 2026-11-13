# Embodiment → CSF adapter contract V1 — A1 transport and process boundary

**State:** A1 protocol specified with schema, fixtures and structural tests; **not** an implemented subprocess, physical reducer, canonical mapping or approved CSFRV1-9A integration.
**Owner:** CSF Go host/supervisor; Embodiment Python is the read-only physical computation/source process.
**Governing decisions:** ADR 0006, 0007, 0008, 0009, 0010, 0011; CSF spec §§12.2, 14.3; ticket CSFRV1-9A.
**Issue:** #98 (A1), parent #94. **A0:** #97, merged as PR #102.
**Bundle:** [A1 schema](embodiment-causal-adapter-v1/schemas/ipc-v1.schema.json), [A1 fixtures](embodiment-causal-adapter-v1/fixtures/ipc-v1-cases.json); [A1 structural regression](../../tests/architecture/test_embodiment_ipc_contract.py).

## 1. Normative scope, security and authority

A1 defines a **local, single-flight, read-only control plane** by which a Go host can inspect an already produced Embodiment run. A1 does **not** introduce physical domain message or candidate schemas. The only permitted operation in the initial protocol is **inspect_run**: inspect source artifacts without changing them, return declared hashes/count/version, and exit. An inspection result is not permission to generate or commit a CSF Event. A2 owns source-to-typed mapping; A3 owns field disclosure; A4 owns overall contract conformance; CSFRV1-10 owns Go/Python runtime integration.

The core Go remains the sole owner of admission, producer-unit identity, LogicalSequence, SimulationInstant, EventOrderKey, candidate authorization, CycleCommit, WorldRevision, WorldState and all persistence/clock/revision ports. A Python process does not receive any of these handles or an interface to write causal ledgers, take a causal snapshot, emit a KnowledgeInput or advance the global clock. No LLM, RAG, transport queue, remote service, or network call is part of this protocol.

A1 is deliberately narrower than the full target integration: no BodyState or capacity profile is transported in the A1 control envelope. Supporting physical payload transfer requires a **separate, closed schema/operation** under A2 with corresponding pinned policy and A3 disclosure; it is **not** an undocumented JSON extension. This prevents the IPC channel from becoming a generic authority or an implicit JSON-to-CBOR converter.

## 2. Process lifecycle and launch parameters

- **Parent Go** creates and supervises a single **local** child process per inspection operation; no shell, dynamic command, pipe to a second command, socket, port or listener. The future runtime entrypoint is proposed to be a dedicated module, such as python -m embodiment.ipc, **which does not exist yet**. The current CLI supports only seed-cohort and advance-clock, and must not be silently reused as an IPC server.
- A trusted host configuration selects the Python executable, import root, absolute **source_run_directory**, execution environment and pinned executable/module version. These are operational launch parameters **not copied into canonical causal data**. The source directory cannot be chosen from a request frame. The launcher must refuse symlink escapes, path traversal, missing files, unexpected file types and source roots outside its allowed directory.
- Go opens/binds the source files to an immutable source view before trusting output. The fixed file names are snapshot.yaml and events.jsonl. If the implementation cannot guarantee a stable read (e.g., by file descriptors or verified immutable copy), it must compare the full bytes and SHA-256 both before and after the child invocation; a change => SOURCE_CHANGED. The child must not open append/write handles for existing physical run data.
- Go starts the child, writes exactly **one** request frame to stdin, flushes and closes stdin, reads exactly **one** response frame and then EOF from stdout, waits for process exit, and either accepts the full matching tuple or reports a transport failure. No streaming candidates, multi-request process reuse or unsolicited stdout. A valid success response with a nonzero exit code still fails.
- **Operational limits (V1 default, pinned in deployment config):** 4,194,304 payload bytes per request or response; a four-byte big-endian unsigned length prefix; no zero-length frames; 30,000 ms end-to-end subprocess budget; 2,000 ms termination grace then force kill; at most 16,384 stderr bytes retained in **privileged, redacted operational diagnostics**, never in any actor context, evidence or world-state. Limits apply before parsing or buffering unbounded content. Exceeding them is a failure, never an invitation to truncate/repair an input.
- The host must close FDs and reap the child after success, error, cancellation or timeout. If cancellation races with process exit, return one deterministic terminal **classification** in host operational telemetry; never publish a partially inspected candidate. No use of wall clock, process PID, randomized correlation ID or duration in causal identity/order/revision.

The budgets are host-level safety limits, **not** simulation time. A1 fixtures cannot prove OS process isolation, path authorization or real cancellation; these require runtime tests in CSFRV1-10.

## 3. Framing, JSON subset and schema gate

Transport encoding is **UTF-8 JSON (RFC 8259)**, not the CSF canonical CBOR codec. The 4-byte unsigned network-order prefix declares the number of UTF-8 payload bytes, excluding the prefix. An EOF, interrupted header or body, zero or oversized length, invalid UTF-8, duplicate JSON object key **at any depth**, NaN/Infinity, trailing data, multiple frames, or unrecognized member anywhere is rejected fail-closed. No newline delimiters, implicit path normalization, indefinite streams or permissive JSON comments.

The **sender** uses the deterministic reference serialization for the closed control envelope: UTF-8, non-ASCII preserved, object keys sorted lexicographically, no insignificant whitespace; integer counters remain integers, no floats. The **receiver** accepts any strict JSON encoding of a schema-valid envelope, with duplicate detection, but **never** hashes transport bytes as canonical CSF bytes. Neither the exact JSON order nor the 4-byte framing is part of world-state provenance. Any future numeric physical schema needs its own precision/unit/canonicalization rules in A2.

An envelope with protocol=cote.embodiment.ipc.v1 uses the exact [JSON schema](embodiment-causal-adapter-v1/schemas/ipc-v1.schema.json), Draft 2020-12. **All object levels have additionalProperties=false.** The schema has exactly two variants:

- **request**: kind=request, protocol, correlation_id (32 lowercase hex digits, operational only), operation=inspect_run, source (fixed snapshot_name/events_name, plus snapshot_raw_sha256/events_raw_sha256, both 64 lowercase hex).
- **response on success**: kind=response, same protocol/correlation_id/operation, status=ok, result with exact source raw SHA-256 hashes, snapshot_document_sha256 (the Embodiment document hash), normalised_log_sha256 (wall-time-normalized JSONL hash), event_count (positive integer), snapshot_version=1.
- **response on source/inspection error**: kind=response with matching fields, status=error, error.code from the closed set UNSUPPORTED_VERSION, SOURCE_NOT_FOUND, SOURCE_CHANGED, SOURCE_INVALID, LIMIT_EXCEEDED, INTERNAL_FAILURE. An error response has no result; an ok response has no error. Error strings, traces, absolute paths and telemetry are **not** serialized on stdout.
  
The request source hashes are **raw bytes** SHA-256 of the two named files, independently computed by the Go supervisor before invocation. The Python worker recomputes them from the **same stable view** and echoes them in success. The Go supervisor verifies the echoed hashes, and the physical snapshot/log semantics are independently validated by Go/A2 before using any output. The physical document and normalized-log hashes are derived by local Embodiment functions; they **are not authenticated signatures and are not causal IDs**. A SHA-256 match proves consistency with the claimed bytes, not origin trust or authorization.

## 4. Negotiation, errors, deterministic failure and retries

There is **no opportunistic negotiation, fallback or automatic upgrade** in V1. The host pins the protocol to cote.embodiment.ipc.v1; an unknown version is rejected before admission. A future A2 projection operation must be deliberately versioned and shipped with schema, fixtures, negative tests and policy pinning. Unknown schema ids, fields, units, non-finite numbers and snapshot versions must fail closed, not be ignored.

Once the full request has been parsed and a correlation id can be safely echoed, the child may return an error response with a registered code and exit normally; malformed or truncated framing may terminate without a response. The Go host normalizes the latter to **FRAME_INVALID** or **PROCESS_FAILURE** in its own diagnostic channel, a timeout to **TIMED_OUT**, cancellation to **CANCELLED**, and a stderr limit breach to **STDERR_LIMIT**; these operational classifications are deliberately **not JSON response codes** and never become a canonical failure reason without a registered mapping owned by Go.

Retry may occur only for an *unchanged*, immutable physical source view and the same operation/schema/policy, **before** any causal admission/commit. Correlation IDs can differ across retries because they do not allocate producer_unit_key, input_id, candidate_id, event_id or commit identity. A repeated identical inspection response cannot increment any physical or global clock, rewrite local artifacts or mutate the world. Two identical raw hashes with different semantic artifacts must not be promoted to one causal identity without A2's full source authority/lineage checks.

Crash cases and their ownership:
1. **Python exits or Go crashes before a complete verified response:** no causal state change; host may rerun the same read-only inspection with the same immutable artifacts.
2. **Complete response received, but Go has not committed any causal event:** still no causal state change. Only Go may later admit a typed A2 output.
3. **Go has committed then loses an acknowledgement:** Python must **not** re-commit or issue a replacement event. Go recovers the durable admission fence, journal and revision, uses existing idempotency and re-verifies provenance against the pinned source. Duplicate delivery with different bytes must fail under the existing CSF collision rules.
4. **Source changes during read, output/truncated frame, unknown schema/version, invalid sha, invalid unit or suspicious stderr:** fail closed; do not fall back to partial snapshot/latest-by-timestamp or a reduced allow-list.

In particular, the A0 finding F01 remains blocking for **advanced** physical runs: the current advance-clock appends events but leaves the initial snapshot unchanged, and PSV1-8 rehydration is not implemented. A1's read-only inspection may describe such input but **must not** infer that the snapshot is latest. A2/CSFRV1-10 must prove the correct cut, reject unreconstructible source and keep local sim_time/seq out of global time/order.

## 5. Threat model, confidentiality and telemetry

- Treat subprocess output as untrusted external input, even for locally installed Python. Verify schema first, bind to actual files and pinned run policy, then allow only a Go-originated typed projection in A2; no direct event write or global revision output.
- A1 source artifacts may contain private numerical BodyState, capacity/posterior/RNG and wall_time; they never become a prompt/RAG document, evidence record, KnowledgeInput, public Observatory view, communication payload or actor context. A3 classifies every nested field and recipient.
- Never pass caller-controlled source paths, keys, command line fragments or environment secrets through JSON or log them in user-facing messages. Stderr may contain untrusted secrets/paths; use a bounded, privileged diagnostic bucket with redaction or discard entirely.
- The schema contains no global authority keys, generic payload or arbitrary extension map. Structural tests verify this property, but **do not** claim to inspect opaque physical file content or authorize new schemas. A2/A3 negative fixtures still mandatory.

## 6. Artifact evidence and gaps

| Requirement | Executable/observable artifact | Interpretation |
| --- | --- | --- |
| Strict request/response envelopes | schemas/ipc-v1.schema.json and positive/negative fixtures | Design contract; no runtime handler |
| Unknown field/version/authority/sha failure | tests/architecture/test_embodiment_ipc_contract.py | Structural verification only |
| Framing, UTF-8, duplicate-key, corrupt/truncate, 4 MiB rejection | Reference framing tests | Test harness proof, **not** Go/Python IPC production conformance |
| Version/hash and source permission | Host rules above | Requires executable adapter and OS-level tests at #10 |
| No global authority and no telemetry in context | Closed schema; A3 disclosure contract pending | No actor output exists at A1 |
| Repeatable source-to-CBOR conversion | A2/A4 pending | **Not proven by A1** |
| In-process physical replay and cut equivalence | PSV1-8/CSFRV1-10 pending | Not supplied or silently emulated |

**A1 exit gate:** reviewed schema and fixtures, structural tests with recorded results/SHA, no unexplained compatibility choice, no claim that A0-F01–F09 are all remediated, and explicit review/merge authorization. A1 cannot close CSFRV1-9A without A2, A3, A4 and a separate human release gate.
