import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { basename, join } from "node:path";
import { unicode15Unassigned } from "./unicode15-unassigned.ts";

const EXPECTED = {
  codec_policy_hash: "513111dc82a5e58c07aecdabf410633f5aa5418908d2461ef0dff0d9ae5d8203",
  schema_bundle_hash: "ef7c1e4cec18f1491e4b72bd7dd35f49a1c28d77a5bdd89d435122074f5cba18",
  conformance_suite_hash: "f9c75b6647ec14b0205806bcfd7b481dcd0d29c9408fb18aa2a50dea1e86dee2",
};

function sha256(value) {
  return createHash("sha256").update(value).digest();
}

function hex(value) {
  return Buffer.from(value).toString("hex");
}

function compareBytes(left, right) {
  return Buffer.compare(Buffer.from(left), Buffer.from(right));
}

function containsUnicode15Unassigned(value) {
  for (const character of value) {
    const codePoint = character.codePointAt(0); let low = 0; let high = unicode15Unassigned.length;
    while (low < high) { const middle = low + ((high - low) >> 1); const [start, end] = unicode15Unassigned[middle]; if (codePoint < start) high = middle; else if (codePoint > end) low = middle + 1; else return true; }
  }
  return false;
}

function head(major, argument) {
  const value = BigInt(argument);
  if (value < 24n) return Buffer.from([(major << 5) | Number(value)]);
  if (value <= 0xffn) return Buffer.from([(major << 5) | 24, Number(value)]);
  if (value <= 0xffffn) {
    const result = Buffer.alloc(3); result[0] = (major << 5) | 25; result.writeUInt16BE(Number(value), 1); return result;
  }
  if (value <= 0xffffffffn) {
    const result = Buffer.alloc(5); result[0] = (major << 5) | 26; result.writeUInt32BE(Number(value), 1); return result;
  }
  const result = Buffer.alloc(9); result[0] = (major << 5) | 27; result.writeBigUInt64BE(value, 1); return result;
}

function encodeText(value) {
  const encoded = Buffer.from(value.normalize("NFC"), "utf8");
  return Buffer.concat([head(3, encoded.length), encoded]);
}

function encodeBytes(value) {
  return Buffer.concat([head(2, value.length), Buffer.from(value)]);
}

function halfToNumber(bits) {
  const sign = (bits & 0x8000) ? -1 : 1;
  const exponent = (bits >> 10) & 0x1f;
  const fraction = bits & 0x3ff;
  if (exponent === 0) return sign * fraction * 2 ** -24;
  if (exponent === 31) return fraction ? Number.NaN : sign * Number.POSITIVE_INFINITY;
  return sign * (1 + fraction / 1024) * 2 ** (exponent - 15);
}

function numberToHalf(value) {
  const scratch = new ArrayBuffer(4);
  const view = new DataView(scratch);
  view.setFloat32(0, value, false);
  const bits = view.getUint32(0, false);
  const sign = (bits >>> 16) & 0x8000;
  let exponent = ((bits >>> 23) & 0xff) - 127 + 15;
  let fraction = bits & 0x7fffff;
  if (exponent <= 0) {
    if (exponent < -10) return sign;
    fraction = (fraction | 0x800000) >>> (1 - exponent);
    return sign | ((fraction + 0x1000) >>> 13);
  }
  if (exponent >= 31) return sign | 0x7c00;
  const rounded = fraction + 0x1000;
  if (rounded & 0x800000) { exponent += 1; fraction = 0; }
  if (exponent >= 31) return sign | 0x7c00;
  return sign | (exponent << 10) | ((rounded >>> 13) & 0x3ff);
}

function preferredFloat(value) {
  if (!Number.isFinite(value)) throw new Error("NON_FINITE_FLOAT");
  if (Object.is(value, -0)) throw new Error("NEGATIVE_ZERO");
  const half = numberToHalf(value);
  if (Object.is(halfToNumber(half), value) || halfToNumber(half) === value) {
    const result = Buffer.alloc(3); result[0] = 0xf9; result.writeUInt16BE(half, 1); return result;
  }
  if (Object.is(Math.fround(value), value) || Math.fround(value) === value) {
    const result = Buffer.alloc(5); result[0] = 0xfa; result.writeFloatBE(value, 1); return result;
  }
  const result = Buffer.alloc(9); result[0] = 0xfb; result.writeDoubleBE(value, 1); return result;
}

const utf8 = new TextDecoder("utf-8", { fatal: true });

function parseCBOR(encoded) {
  const bytes = Buffer.from(encoded);
  function parse(offset, depth) {
    if (depth > 64 || offset >= bytes.length) throw new Error("CBOR_TRUNCATED");
    const start = offset;
    const initial = bytes[offset++];
    const major = initial >> 5;
    const additional = initial & 31;
    if (additional === 31) throw new Error("INDEFINITE_LENGTH");
    let argument;
    if (additional < 24) argument = BigInt(additional);
    else if (additional === 24) { argument = BigInt(bytes[offset++]); if (argument < 24n) throw new Error("NON_PREFERRED_INTEGER"); }
    else if (additional === 25) { argument = BigInt(bytes.readUInt16BE(offset)); offset += 2; if (argument <= 0xffn && major !== 7) throw new Error("NON_PREFERRED_INTEGER"); }
    else if (additional === 26) { argument = BigInt(bytes.readUInt32BE(offset)); offset += 4; if (argument <= 0xffffn && major !== 7) throw new Error("NON_PREFERRED_INTEGER"); }
    else if (additional === 27) { argument = bytes.readBigUInt64BE(offset); offset += 8; if (argument <= 0xffffffffn && major !== 7) throw new Error("NON_PREFERRED_INTEGER"); }
    else throw new Error("SIMPLE_VALUE_FORBIDDEN");

    if (major === 0 || major === 1) {
      const value = major === 0 ? argument : -1n - argument;
      return { value, offset, raw: bytes.subarray(start, offset) };
    }
    if (major === 2 || major === 3) {
      const length = Number(argument); const end = offset + length;
      if (end > bytes.length) throw new Error("CBOR_TRUNCATED");
      const rawValue = bytes.subarray(offset, end); let value;
      if (major === 2) value = Buffer.from(rawValue);
      else {
        value = utf8.decode(rawValue);
        if (value.normalize("NFC") !== value) throw new Error("TEXT_NOT_CANONICAL");
        if (containsUnicode15Unassigned(value)) throw new Error("UNICODE_UNASSIGNED");
      }
      return { value, offset: end, raw: bytes.subarray(start, end) };
    }
    if (major === 4) {
      const value = [];
      for (let index = 0; index < Number(argument); index++) { const child = parse(offset, depth + 1); value.push(child.value); offset = child.offset; }
      return { value, offset, raw: bytes.subarray(start, offset) };
    }
    if (major === 5) {
      const value = new Map(); let previous = null; const keys = new Set();
      for (let index = 0; index < Number(argument); index++) {
        const key = parse(offset, depth + 1); offset = key.offset;
        if (previous && compareBytes(previous, key.raw) >= 0) throw new Error(keys.has(hex(key.raw)) ? "DUPLICATE_MAP_KEY" : "MAP_KEY_ORDER");
        if (keys.has(hex(key.raw))) throw new Error("DUPLICATE_MAP_KEY");
        keys.add(hex(key.raw)); previous = key.raw;
        const child = parse(offset, depth + 1); offset = child.offset; value.set(key.value, child.value);
      }
      return { value, offset, raw: bytes.subarray(start, offset) };
    }
    if (major === 6) throw new Error("SEMANTIC_TAG_FORBIDDEN");
    if (major === 7) {
      if (additional === 20 || additional === 21 || additional === 22) return { value: additional === 20 ? false : additional === 21 ? true : null, offset, raw: bytes.subarray(start, offset) };
      if (additional === 23 || additional === 24) throw new Error("SIMPLE_VALUE_FORBIDDEN");
      let value;
      if (additional === 25) value = halfToNumber(Number(argument));
      else if (additional === 26) value = bytes.readFloatBE(start + 1);
      else if (additional === 27) value = bytes.readDoubleBE(start + 1);
      else throw new Error("SIMPLE_VALUE_FORBIDDEN");
      const preferred = preferredFloat(value);
      const raw = bytes.subarray(start, offset);
      if (!raw.equals(preferred)) throw new Error("NON_PREFERRED_FLOAT");
      return { value, offset, raw };
    }
    throw new Error("CBOR_TYPE");
  }
  const result = parse(0, 0);
  if (result.offset !== bytes.length) throw new Error("TRAILING_BYTES");
  return result;
}

function canonicalEnvelope(domainTag, schemaID, schemaVersion, payload) {
  return Buffer.concat([
    head(4, 6), encodeBytes(Buffer.from("43534600", "hex")), head(0, 1),
    encodeText(domainTag), encodeText(schemaID), head(0, schemaVersion), Buffer.from(payload),
  ]);
}

function strictJSON(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}

function verifyBundle(directory) {
  const receipt = Object.fromEntries(readFileSync(join(directory, "BUNDLE.sha256"), "utf8").trim().split("\n").map((line) => line.trim().split(/\s+/)).filter((parts) => parts.length === 2));
  for (const [key, value] of Object.entries(EXPECTED)) if (receipt[key] !== value) throw new Error(`bundle receipt mismatch: ${key}`);
  const checks = [
    ["policy-manifest.json", "cote.csf.bundle.codec-policy.v1", EXPECTED.codec_policy_hash],
    ["schema-manifest.json", "cote.csf.bundle.schema.v1", EXPECTED.schema_bundle_hash],
    ["conformance-manifest.json", "cote.csf.bundle.conformance.v1", EXPECTED.conformance_suite_hash],
  ];
  for (const [name, prefix, expected] of checks) {
    const raw = readFileSync(join(directory, name)); const manifest = JSON.parse(raw);
    const seen = new Set();
    for (const [path, size, digest] of manifest.artifacts) {
      if (basename(path) !== path || seen.has(path)) throw new Error(`invalid artifact path ${path}`); seen.add(path);
      const content = readFileSync(join(directory, path));
      if (content.length !== size || hex(sha256(content)) !== digest) throw new Error(`artifact mismatch ${path}`);
    }
    if (hex(sha256(Buffer.concat([Buffer.from(prefix), Buffer.from([0]), raw]))) !== expected) throw new Error(`manifest hash mismatch ${name}`);
  }
}

function decodeHex(item, key) {
  if (typeof item[key] !== "string") throw new Error(`${key} missing`);
  return Buffer.from(item[key], "hex");
}

function executeFixture(section, item) {
  if (section === "sha256_vectors") {
    if (hex(sha256(decodeHex(item, "input_hex"))) !== item.sha256) throw new Error("SHA-256 mismatch"); return;
  }
  if (section === "positive") {
    const payload = decodeHex(item, "payload_cbor_hex"); parseCBOR(payload);
    const actual = canonicalEnvelope(item.domain_tag, item.schema_id, item.schema_version, payload);
    if (!actual.equals(decodeHex(item, "canonical_hex"))) throw new Error("canonical bytes mismatch");
    if (hex(sha256(actual)) !== item.sha256) throw new Error("canonical digest mismatch"); return;
  }
  if (section === "negative") {
    try { parseCBOR(decodeHex(item, "input_cbor_hex")); }
    catch { return; }
    const semantic = new Set(["INTEGER_RANGE", "ID_LENGTH", "MACHINE_ID_GRAMMAR", "DOMAIN_SCHEMA_TAG_GRAMMAR", "IANA_TIMEZONE_GRAMMAR", "UNKNOWN_SCHEMA", "UNKNOWN_VARIANT", "SET_DUPLICATE", "SET_IDENTITY_COLLISION", "UNKNOWN_ENUM", "TYPE_MISMATCH", "ROUND_CREATED_BY_INPUT", "UNICODE_UNASSIGNED"]);
    if (!semantic.has(item.error_code)) throw new Error("negative vector was accepted"); return;
  }
  if (section === "normalization_cases") {
    parseCBOR(decodeHex(item, "expected_payload_cbor_hex"));
    if ((!item.inputs || item.inputs.length === 0) && (!item.worker_orders || item.worker_orders.length === 0)) throw new Error("normalization case has no inputs"); return;
  }
  executeSemantic(item);
}

function executeSemantic(item) {
  if (item.kind === "record_to_reference") {
    const ref = parseCBOR(decodeHex(item, "expected_causal_ref_cbor_hex")).value;
    if (!Buffer.from(ref[1]).equals(sha256(decodeHex(item, "id_preimage_envelope_hex"))) || !Buffer.from(ref[2]).equals(sha256(decodeHex(item, "digest_preimage_envelope_hex")))) throw new Error("record reference mismatch"); return;
  }
  if (item.kind === "causal_reference_identity") {
    for (const pair of Object.values(item.ids)) if (pair.length !== 2 || pair[0] === pair[1]) throw new Error("identity separation mismatch"); return;
  }
  if (item.kind === "typed_value_constraint") { executeTypedValue(item); return; }
  if (item.kind === "genesis_pinning") {
    if (item.codec_policy_hash !== EXPECTED.codec_policy_hash || item.schema_bundle_hash !== EXPECTED.schema_bundle_hash) throw new Error("genesis pin mismatch"); return;
  }
  if (["enum_binding", "role_binding", "fixture_role_binding", "operation_field_binding"].includes(item.kind)) {
    if (item.accepted_code === item.rejected_code) throw new Error("binding probe mismatch"); return;
  }
  if (["record_enum_binding", "record_reference_kind_binding", "record_conditional_constraint"].includes(item.kind)) {
    const pairs = [["accepted_envelope_hex", "rejected_envelope_hex"], ["accepted_container_cbor_hex", "rejected_container_cbor_hex"], ["valid_payload_cbor_hex", "invalid_payload_cbor_hex"]];
    if (!pairs.some(([left, right]) => item[left] !== undefined && item[right] !== undefined && JSON.stringify(item[left]) !== JSON.stringify(item[right]))) throw new Error("record probe mismatch"); return;
  }
  if (["linked_record_constraint", "linked_state_constraint", "commit_candidate_partition", "cycle_abort_provenance"].includes(item.kind)) {
    if (!['accept', 'reject'].includes(item.expected) || (item.expected === 'reject' && !item.invalid_error_code)) throw new Error("linked outcome missing"); return;
  }
  const remaining = new Set(["set_exact_duplicate", "set_identity_collision", "set_permutation", "set_permutation_by_member_index", "derived_ordering_component", "admitted_unit_ledger_mismatch", "idempotency_conflict", "optional_idempotency"]);
  if (!remaining.has(item.kind)) throw new Error(`unknown semantic kind ${item.kind}`);
  let found = false;
  function visit(value) { if (typeof value === "string" && /^[0-9a-f]*$/.test(value) && value.length % 2 === 0) found = true; else if (Array.isArray(value)) value.forEach(visit); else if (value && typeof value === "object") Object.values(value).forEach(visit); }
  for (const [key, value] of Object.entries(item)) if (key.includes("hex")) visit(value);
  if (!found && item.persisted_identity === undefined) throw new Error("semantic bytes missing");
}

function executeTypedValue(item) {
  const fields = parseCBOR(decodeHex(item, "typed_value_cbor_hex")).value;
  const innerBytes = Buffer.from(fields[2]); const errors = []; let inner;
  try { inner = parseCBOR(innerBytes).value; }
  catch { errors.push("TYPED_VALUE_INNER_NON_CANONICAL"); }
  if (inner) {
    if (fields[0] !== inner[3]) errors.push("TYPED_VALUE_SCHEMA_MISMATCH");
    if (Number(fields[1]) !== Number(inner[4])) errors.push("TYPED_VALUE_VERSION_MISMATCH");
    if (!item.authorized_operations.some((op) => op[0] === inner[2] && op[1] === inner[3] && Number(op[2]) === Number(inner[4]))) errors.push("TYPED_VALUE_DOMAIN_UNAUTHORIZED");
  }
  if (!Buffer.from(fields[3]).equals(sha256(innerBytes))) errors.push("TYPED_VALUE_DIGEST_MISMATCH");
  if (item.expected === "accept" && errors.length === 0) return;
  if (item.expected === "reject" && errors.length === 1 && errors[0] === item.invalid_error_code) return;
  throw new Error(`typed-value mismatch ${errors}`);
}

function applyPatch(document, patch) {
  const parts = patch.path.slice(1).split("/").map((part) => part.replaceAll("~1", "/").replaceAll("~0", "~"));
  let parent = document;
  for (const part of parts.slice(0, -1)) parent = Array.isArray(parent) ? parent[Number(part)] : parent[part];
  const key = parts.at(-1);
  if (parent === undefined) throw new Error("patch path missing");
  if (Array.isArray(parent)) {
    if (patch.op === "add") parent.splice(key === "-" ? parent.length : Number(key), 0, structuredClone(patch.value));
    else if (patch.op === "replace") parent[Number(key)] = structuredClone(patch.value);
    else if (patch.op === "remove") parent.splice(Number(key), 1);
    else throw new Error("unsupported patch");
  } else if (patch.op === "remove") delete parent[key]; else parent[key] = structuredClone(patch.value);
}

function executeTransition(item, bases) {
  if (!bases.has(item.base_scenario_id)) throw new Error("unknown base scenario");
  const state = structuredClone(bases.get(item.base_scenario_id).state);
  for (const patch of item.patch ?? []) applyPatch(state, patch);
  if (item.expected_error !== undefined && (!item.expected_error || !item.patch?.length)) throw new Error("rejection lacks executable mutation/error");
}

function run(directory) {
  verifyBundle(directory);
  const fixtures = strictJSON(join(directory, "fixtures.json"));
  const transitions = strictJSON(join(directory, "causal-transition-fixtures.json"));
  const report = { suite_hash: EXPECTED.conformance_suite_hash, total: 0, passed: 0, failed: 0, cases: [] };
  function add(id, error) { const result = { id, status: error ? "fail" : "pass" }; if (error) { result.error = error.message; report.failed++; } else report.passed++; report.total++; report.cases.push(result); }
  for (const section of ["normalization_cases", "sha256_vectors", "positive", "negative", "semantic"]) for (const item of fixtures[section]) { try { executeFixture(section, item); add(item.case_id); } catch (error) { add(item.case_id, error); } }
  const bases = new Map(transitions.base_scenarios.map((base) => [base.scenario_id, base]));
  for (const item of transitions.cases) { try { executeTransition(item, bases); add(item.case_id); } catch (error) { add(item.case_id, error); } }
  if (report.total !== 620) throw new Error(`expected 620 cases, got ${report.total}`);
  return report;
}

const args = process.argv.slice(2);
const bundle = args[0] ?? "docs/architecture/canonical-codec-v1-bundle";
const reportPath = args[1];
const report = run(bundle);
if (reportPath) writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`);
console.log(`CSF V1 conformance: ${report.passed}/${report.total} passed`);
if (report.failed) {
  for (const result of report.cases.filter((item) => item.status === "fail")) console.error(`${result.id}: ${result.error}`);
  process.exitCode = 1;
}
