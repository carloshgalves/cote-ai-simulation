// Independent Node.js verifier for the rectified V1 record-to-reference fixtures.
// Uses only Node built-ins and decodes the record, ID preimage and digest preimage.
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import assert from "node:assert/strict";

const directory = "docs/architecture/canonical-codec-v1-bundle";
const fixtures = JSON.parse(readFileSync(`${directory}/fixtures.json`, "utf8"));
const registries = JSON.parse(readFileSync(`${directory}/registries.json`, "utf8"));
const sha256 = (bytes) => createHash("sha256").update(bytes).digest();

function head(major, value) {
  const n = BigInt(value);
  if (n < 24n) return Buffer.from([(major << 5) | Number(n)]);
  if (n <= 255n) return Buffer.from([(major << 5) | 24, Number(n)]);
  if (n <= 65535n) return Buffer.from([(major << 5) | 25, Number(n >> 8n), Number(n & 255n)]);
  if (n <= 4294967295n) {
    const out = Buffer.alloc(5);
    out[0] = (major << 5) | 26;
    out.writeUInt32BE(Number(n), 1);
    return out;
  }
  const out = Buffer.alloc(9);
  out[0] = (major << 5) | 27;
  out.writeBigUInt64BE(n, 1);
  return out;
}

function encode(value) {
  if (Buffer.isBuffer(value)) return Buffer.concat([head(2, value.length), value]);
  if (typeof value === "bigint") return head(value >= 0n ? 0 : 1, value >= 0n ? value : -1n - value);
  if (typeof value === "number" && Number.isSafeInteger(value)) return encode(BigInt(value));
  if (typeof value === "string") { const b = Buffer.from(value, "utf8"); return Buffer.concat([head(3, b.length), b]); }
  if (Array.isArray(value)) return Buffer.concat([head(4, value.length), ...value.map(encode)]);
  if (value === false) return Buffer.from([0xf4]);
  if (value === true) return Buffer.from([0xf5]);
  if (value === null) return Buffer.from([0xf6]);
  throw new Error("unsupported canonical CBOR AST");
}

function decode(bytes) {
  let offset = 0;
  function read() {
    if (offset >= bytes.length) throw new Error("CBOR truncated");
    const first = bytes[offset++], major = first >> 5, ai = first & 31;
    let n = BigInt(ai);
    if (ai >= 24 && ai <= 27) {
      const widths = { 24: 1, 25: 2, 26: 4, 27: 8 }, width = widths[ai];
      if (offset + width > bytes.length) throw new Error("truncated CBOR argument");
      n = 0n;
      for (let i = 0; i < width; i++) n = (n << 8n) + BigInt(bytes[offset++]);
    } else if (ai >= 28) throw new Error("unsupported CBOR additional information");
    if (major === 0) return n;
    if (major === 1) return -1n - n;
    if (n > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error("CBOR container length too large");
    const len = Number(n);
    if (major === 2 || major === 3) {
      if (offset + len > bytes.length) throw new Error("CBOR string truncated");
      const data = bytes.subarray(offset, offset + len);
      offset += len;
      return major === 2 ? Buffer.from(data) : Buffer.from(data).toString("utf8");
    }
    if (major === 4) return Array.from({ length: len }, () => read());
    if (major === 7 && ai === 20) return false;
    if (major === 7 && ai === 21) return true;
    if (major === 7 && ai === 22) return null;
    throw new Error(`unsupported CBOR major ${major}, ai ${ai}`);
  }
  const value = read();
  if (offset !== bytes.length || !encode(value).equals(bytes)) throw new Error("noncanonical CBOR envelope");
  return value;
}

const selectors = {
  "event": [1, 7, 2], "exogenous-input": [1, 2, 3], "source-closure": [1, 2, 3],
  "round-declaration": [6, 7, 8], "slot-dispatch": [1, 2, 3, 4, 5],
  "action-proposal": [1, "action"], "scheduled-occurrence": [9, 10, 11],
  "trigger-activation": [1, 3, 4, "activation-revision", 7],
  "commit-candidate": [1, 2, 3, 4, 5, 6, 7],
  "conflict-set": ["external-cycle", 1, 2, 3],
  "decision-record": [2, 3, "decision-input-id"],
  "admission-fence": [0, 1, 2], "cycle-commit": [0, 1, 2],
  "cycle-abort": [0, 1, 2], "attempt-retry": [0, 1, 2, 3],
  "perception-task": [1, 3, 6, 7, 8], "observation": [1, 2, 3, 4],
  "knowledge-input": ["observation-id", 3, 5, 1, 2],
  "claim": [1, 2, 3], "transmission": [1, 2, 3],
  "snapshot": [0, 1, 3], "genesis-manifest": [0],
  "epistemic-checkpoint-ref": [0, 1, 2, 7],
  "no-proposal": [1, "no-proposal"],
  "rng-draw": [1, 2, 3, 4, 5, 6, 7, 8],
  "affordance-assessment": [0, 2],
  "provisional-disposition": [1, 2, 3, 4],
  "attempt-failure": [1, 2, 3, 5, 6, 4],
};
const idStored = new Set([
  "event", "exogenous-input", "source-closure", "round-declaration",
  "slot-dispatch", "action-proposal", "scheduled-occurrence",
  "trigger-activation", "commit-candidate", "conflict-set", "decision-record",
  "perception-task", "observation", "knowledge-input", "claim", "transmission",
  "no-proposal", "rng-draw", "provisional-disposition", "attempt-failure"
]);
const bodyDigest = new Set([
  "event", "exogenous-input", "action-proposal", "scheduled-occurrence",
  "trigger-activation", "commit-candidate", "conflict-set", "decision-record",
  "admission-fence", "cycle-abort", "observation", "knowledge-input",
  "claim", "transmission", "snapshot", "no-proposal", "rng-draw",
  "affordance-assessment", "provisional-disposition", "attempt-failure"
]);
const eq = (a, b) => encode(a).equals(encode(b));

function project(root, record, prior) {
  return selectors[root].map((selector) => {
    if (Number.isInteger(selector)) return record[selector];
    if (selector === "action") return 0n;
    if (selector === "no-proposal") return 1n;
    if (selector === "external-cycle") return prior[0]; // needs candidate-ledger witness; tracked separately
    if (selector === "decision-input-id") return record[4][2];
    if (selector === "observation-id") return record[7][1];
    if (selector === "activation-revision") return record[5][0] === 0n ? record[5][2] : 0n;
    throw new Error(`unrecognized selector ${selector}`);
  });
}

function verifyConflictWitness(item, conflictFields) {
  const witnesses = item.candidate_record_envelopes_hex;
  assert(Array.isArray(witnesses) && witnesses.length === 2, "conflict-set requires two authoritative candidate records");
  const candidateIds = [];
  let cycle = null;
  for (const raw of witnesses) {
    const record = decode(Buffer.from(raw, "hex"));
    assert.equal(record[3], "cote.csf.schema.commit-candidate");
    const fields = record[5];
    const idPayload = [fields[1], fields[2], fields[3], fields[4], fields[5], fields[6], fields[7]];
    const idEnvelope = [Buffer.from([67, 83, 70, 0]), 1n, "cote.csf.id.commit-candidate", "cote.csf.schema.candidate-id-preimage", 1n, idPayload];
    const id = sha256(encode(idEnvelope));
    assert(fields[0].equals(id), "candidate ID does not derive from candidate's record");
    const digestEnvelope = [Buffer.from([67, 83, 70, 0]), 1n, "cote.csf.digest.commit-candidate", "cote.csf.schema.commit-candidate.body", 1n, fields.slice(0, -1)];
    assert(fields.at(-1).equals(sha256(encode(digestEnvelope))), "candidate digest mismatch");
    if (cycle !== null) assert(fields[2].equals(cycle), "conflict candidates belong to different cycles");
    cycle = fields[2];
    candidateIds.push(id);
  }
  assert(candidateIds[0].equals(conflictFields[1][0]) && candidateIds[1].equals(conflictFields[1][1]),
    "conflict candidates and recorded candidate IDs disagree");
  return cycle;
}

function verify(item) {
  const root = item.persisted_root.split(".").at(-1), registered = registries.reference_identities.find((r) => r.persisted_root === item.persisted_root);
  assert(registered, `reference root missing registry: ${root}`);
  assert.equal(registered.id.operation, item.id_operation);
  assert.equal(registered.digest.operation, item.digest_operation);
  const record = decode(Buffer.from(item.record_envelope_hex, "hex"));
  const identity = decode(Buffer.from(item.id_preimage_envelope_hex, "hex"));
  const digest = decode(Buffer.from(item.digest_preimage_envelope_hex, "hex"));
  assert.equal(record[3], item.persisted_root);
  assert.equal(identity[2], registered.id.operation);
  assert.equal(identity[3], registered.id.preimage_schema);
  assert.equal(digest[2], registered.digest.operation);
  assert.equal(digest[3], registered.digest.preimage_schema);
  const prior = root === "conflict-set" ? [verifyConflictWitness(item, record[5])] : identity[5];
  assert(eq(identity[5], project(root, record[5], prior)), `record→ID mismatch for ${root}`);
  const recordFields = record[5];
  if (root === "cycle-commit") {
    const eventEnvelopes = (item.event_record_envelopes_hex ?? []).map((raw) => Buffer.from(raw, "hex"));
    assert(eq(digest[5], [recordFields.slice(0, -1), eventEnvelopes]), "cycle-commit batch body mismatch");
    assert.equal(eventEnvelopes.length, recordFields[15].length, "cycle-commit event witnesses not resolved");
    const events = eventEnvelopes.map((raw) => decode(raw));
    assert(events.every((event) => event[3] === "cote.csf.schema.event"), "cycle-commit witness is not Event");
    assert(eq(events.map((event) => event[5][0]), recordFields[15]), "cycle-commit event IDs do not match witnesses");
    for (const event of events) {
      const fields = event[5];
      assert(fields[1].equals(recordFields[0]) && fields[7].equals(recordFields[1]), "event owner run/cycle mismatch");
      const idEnvelope = [Buffer.from([67, 83, 70, 0]), 1n, "cote.csf.id.event", "cote.csf.schema.event-id-preimage", 1n, [fields[1], fields[7], fields[2]]];
      const digestEnvelope = [Buffer.from([67, 83, 70, 0]), 1n, "cote.csf.digest.event", "cote.csf.schema.event.body", 1n, fields.slice(0, -1)];
      assert(fields[0].equals(sha256(encode(idEnvelope))), "cycle-commit Event ID mismatch");
      assert(fields.at(-1).equals(sha256(encode(digestEnvelope))), "cycle-commit Event digest mismatch");
    }
    const nextLogicalSequence = events.length === 0
      ? 0n
      : events.reduce((next, event) => event[5][6] >= next ? event[5][6] + 1n : next, 0n);
    assert.equal(recordFields[22], nextLogicalSequence, "cycle-commit next logical sequence mismatch");
  } else assert(eq(digest[5], bodyDigest.has(root) ? recordFields.slice(0, -1) : recordFields), `record→digest mismatch for ${root}`);
  const computedID = sha256(Buffer.from(item.id_preimage_envelope_hex, "hex"));
  const computedDigest = sha256(Buffer.from(item.digest_preimage_envelope_hex, "hex"));
  if (idStored.has(root)) assert(recordFields[0].equals(computedID), `record ID mismatch for ${root}`);
  if (bodyDigest.has(root)) assert(recordFields.at(-1).equals(computedDigest), `record digest mismatch for ${root}`);
  const ref = decode(Buffer.from(item.expected_causal_ref_cbor_hex, "hex"));
  assert.equal(ref[0], BigInt(registered.code));
  assert(ref[1].equals(computedID) && ref[2].equals(computedDigest), `derived reference mismatch for ${root}`);
  if (item.expected_input_ref_cbor_hex) {
    const inputRef = decode(Buffer.from(item.expected_input_ref_cbor_hex, "hex"));
    assert(inputRef.at(-2).equals(computedID) && inputRef.at(-1).equals(computedDigest), `input reference mismatch for ${root}`);
  }
  return { root, external: false };
}

const cases = fixtures.semantic.filter((item) => item.kind === "record_to_reference");
assert.equal(cases.length, 29, "28 roots plus the non-empty CycleCommit batch witness required");
assert.equal(new Set(cases.map((item) => item.persisted_root)).size, 28, "reference schemas must be unique");
const checked = cases.map(verify);
const mutationIndex = {
  "event":1, "exogenous-input":3, "source-closure":1,
  "round-declaration":7, "slot-dispatch":4, "action-proposal":1,
  "scheduled-occurrence":10, "trigger-activation":3, "commit-candidate":4,
  "conflict-set":2, "decision-record":3, "admission-fence":2,
  "cycle-commit":2, "cycle-abort":2, "attempt-retry":3,
  "perception-task":3, "observation":3, "knowledge-input":1,
  "claim":2, "transmission":2, "snapshot":3, "genesis-manifest":0,
  "epistemic-checkpoint-ref":1, "no-proposal":1, "rng-draw":3,
  "affordance-assessment":2, "provisional-disposition":3, "attempt-failure":3,
};
for (const item of cases) {
  const root = item.persisted_root.split(".").at(-1);
  const parsed = decode(Buffer.from(item.record_envelope_hex, "hex"));
  const fields = parsed[5], index = mutationIndex[root], oldValue = fields[index];
  assert(index !== undefined, `mutation guard missing for ${root}`);
  if (Buffer.isBuffer(oldValue)) {
    const modified = Buffer.from(oldValue); modified[0] ^= 1; fields[index] = modified;
  } else if (typeof oldValue === "string") fields[index] = oldValue + "-2";
  else if (typeof oldValue === "bigint") fields[index] = oldValue + 1n;
  else throw new Error(`mutation field not scalar: ${root}`);
  const mutated = { ...item, record_envelope_hex: encode(parsed).toString("hex") };
  assert.throws(() => verify(mutated), `record→reference mutation escaped verification: ${root}`);
}
const dispatchCase = cases.find((item) => item.persisted_root === "cote.csf.schema.slot-dispatch");
const dispatchRecord = decode(Buffer.from(dispatchCase.record_envelope_hex, "hex"));
dispatchRecord[5][4] = "slot-2";
assert.throws(() => verify({ ...dispatchCase, record_envelope_hex: encode(dispatchRecord).toString("hex") }),
  "slot-2 record-only mutation escaped ID verification");
const digestOnlyRecord = decode(Buffer.from(dispatchCase.record_envelope_hex, "hex"));
digestOnlyRecord[5][6][1] = "actor-2";
assert.throws(() => verify({ ...dispatchCase, record_envelope_hex: encode(digestOnlyRecord).toString("hex") }),
  "digest-only record mutation escaped digest verification");
const roundCase = cases.find((item) => item.persisted_root === "cote.csf.schema.round-declaration");
const nestedOriginRecord = decode(Buffer.from(roundCase.record_envelope_hex, "hex"));
nestedOriginRecord[5][6][0] = nestedOriginRecord[5][6][0] === 0n ? 1n : 0n;
assert.throws(() => verify({ ...roundCase, record_envelope_hex: encode(nestedOriginRecord).toString("hex") }),
  "created-by discriminant record-only mutation escaped ID verification");
const source = cases.find((item) => item.case_id === "reference.record-to-ref.exogenous-input");
assert(source, "exogenous fixture absent");
const inputRef = decode(Buffer.from(source.expected_input_ref_cbor_hex, "hex"));
for (const item of fixtures.semantic.filter((item) => item.kind === "admitted_unit_ledger_mismatch")) {
  assert.equal(item.referenced_record_case_id, source.case_id);
  const authority = decode(Buffer.from(item.authoritative_admitted_unit_cbor_hex, "hex"));
  assert(authority[3].equals(inputRef.at(-2)) && authority[4].equals(inputRef.at(-1)), "admitted source identity mismatch");
}
assert.equal(fixtures.semantic.filter((item) => item.kind === "admitted_unit_ledger_mismatch").length, 5);
console.log(JSON.stringify({
  cases: checked.length,
  roots: new Set(checked.map((item) => item.root)).size,
  admitted_authorities: fixtures.semantic.filter((item) => item.kind === "admitted_unit_ledger_mismatch").length,
  conflict_candidates: 2,
  record_mutations_rejected: cases.length + 3,
}));
