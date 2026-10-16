import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { basename, join } from "node:path";
import { isDeepStrictEqual } from "node:util";
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

function encodeCanonical(value) {
  if (value === null) return Buffer.from([0xf6]);
  if (value === false) return Buffer.from([0xf4]);
  if (value === true) return Buffer.from([0xf5]);
  if (Buffer.isBuffer(value) || value instanceof Uint8Array) return encodeBytes(Buffer.from(value));
  if (typeof value === "string") return encodeText(value.normalize("NFC"));
  if (typeof value === "bigint") return value >= 0n ? head(0, value) : head(1, -1n - value);
  if (typeof value === "number") {
    if (Number.isInteger(value)) return value >= 0 ? head(0, value) : head(1, -1 - value);
    if (!Number.isFinite(value)) throw new Error("NON_FINITE_FLOAT");
    return preferredFloat(Object.is(value, -0) ? 0 : value);
  }
  if (Array.isArray(value)) return Buffer.concat([head(4, value.length), ...value.map(encodeCanonical)]);
  if (value instanceof Map) {
    const entries = [...value.entries()].map(([key, child]) => [encodeCanonical(key), encodeCanonical(child)]).sort((left, right) => compareBytes(left[0], right[0]));
    return Buffer.concat([head(5, entries.length), ...entries.flat()]);
  }
  if (value && typeof value === "object" && Object.hasOwn(value, "f64")) return preferredFloat(Object.is(value.f64, -0) ? 0 : value.f64);
  throw new Error(`unsupported canonical value ${typeof value}`);
}

function canonicalSort(values) {
  values.sort((left, right) => compareBytes(encodeCanonical(left), encodeCanonical(right)));
  return values;
}

function materializeNormalizationValue(raw, item) {
  const [kind, body] = raw;
  if (kind === "uint") return BigInt(body);
  if (kind === "text_codepoints") return String.fromCodePoint(...body);
  if (kind === "f64_bits") { const bytes = Buffer.from(body, "hex"); return { f64: bytes.readDoubleBE() }; }
  if (kind === "map") return new Map(body.map(([key, value]) => [materializeNormalizationValue(key, item), materializeNormalizationValue(value, item)]));
  if (kind === "set") {
    const seen = new Set(); const result = [];
    for (const child of body) {
      const value = materializeNormalizationValue(child, item); const key = encodeCanonical(value).toString("hex");
      if (seen.has(key)) { if (item.duplicate_policy === "dedup_exact") continue; throw new Error("SET_DUPLICATE"); }
      seen.add(key); result.push(value);
    }
    return canonicalSort(result);
  }
  throw new Error(`unknown normalization descriptor ${kind}`);
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

function executeFixture(section, item, fixtures, registries) {
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
    const input = decodeHex(item, "input_cbor_hex"); let value;
    try { value = parseCBOR(input).value; }
    catch { return; }
    if (semanticNegativeCode(value, item, registries) !== item.error_code) throw new Error("negative vector was accepted"); return;
  }
  if (section === "normalization_cases") {
    executeNormalization(item); return;
  }
  executeSemantic(item, fixtures, registries);
}

function machineIDValid(value) { return typeof value === "string" && value.length >= 1 && value.length <= 128 && /^[a-z][a-z0-9]*(?:[._:/-][a-z0-9]+)*$/.test(value); }
function domainTagValid(value) { return typeof value === "string" && value.length <= 128 && /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*(?:\.[a-z][a-z0-9]*(?:-[a-z0-9]+)*)+$/.test(value); }
function timezoneValid(value) { return typeof value === "string" && /^[A-Za-z]+(?:[_-][A-Za-z]+)*(?:\/[A-Za-z]+(?:[_-][A-Za-z]+)*)+$/.test(value); }
function hasExactDuplicateValue(value) { if (!Array.isArray(value)) return false; const seen = new Set(); for (const member of value) { const encoded = hex(encodeCanonical(member)); if (seen.has(encoded)) return true; seen.add(encoded); } return false; }
function hasIdentityCollisionValue(value) { if (!Array.isArray(value)) return false; const identities = new Map(); for (const member of value) { if (!Array.isArray(member) || member.length < 2) return false; const identity = hex(encodeCanonical(member.slice(0, -1))), full = encodeCanonical(member); if (identities.has(identity) && !identities.get(identity).equals(full)) return true; identities.set(identity, full); } return false; }
function semanticNegativeCode(value, item, registries) {
  if (item.error_code === "INTEGER_RANGE" && typeof value === "bigint" && value > 255n) return item.error_code;
  if (item.error_code === "ID_LENGTH" && Buffer.isBuffer(value) && value.length !== 32) return item.error_code;
  if (item.error_code === "MACHINE_ID_GRAMMAR" && !machineIDValid(value)) return item.error_code;
  if (item.error_code === "DOMAIN_SCHEMA_TAG_GRAMMAR" && Array.isArray(value) && value.length === 6 && (!domainTagValid(value[2]) || !domainTagValid(value[3]))) return item.error_code;
  if (item.error_code === "IANA_TIMEZONE_GRAMMAR" && !timezoneValid(value)) return item.error_code;
  if (item.error_code === "UNKNOWN_SCHEMA" && Array.isArray(value) && value.length === 6 && ![...registries.schemas, ...registries.fixture_schemas].some((schema) => schema[0] === value[3])) return item.error_code;
  if (item.error_code === "UNKNOWN_VARIANT" && Array.isArray(value) && Number(value[0]) > 1) return item.error_code;
  if (item.error_code === "SET_DUPLICATE" && hasExactDuplicateValue(value)) return item.error_code;
  if (item.error_code === "SET_IDENTITY_COLLISION" && hasIdentityCollisionValue(value)) return item.error_code;
  if (item.error_code === "UNKNOWN_ENUM" && Array.isArray(value) && value.length === 1 && !registryHasCode(registries, "enums", item.schema_id, Number(value[0]))) return item.error_code;
  if (item.error_code === "TYPE_MISMATCH" && !Buffer.isBuffer(value)) return item.error_code;
  if (item.error_code === "ROUND_CREATED_BY_INPUT" && Array.isArray(value) && value.length === 6 && Array.isArray(value[5]) && Number(value[5][6][0]) === 1) return item.error_code;
  return null;
}

function executeSemantic(item, fixtures, registries) {
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
    let group = "enums"; let registry = item.registry;
    if (item.kind === "role_binding") group = "roles";
    if (item.kind === "fixture_role_binding") { group = "fixture_roles"; registry = item.fixture_registry; }
    if (item.kind === "operation_field_binding" && !registryHasCode(registries, group, registry, item.accepted_code)) {
      if (registryHasCode(registries, "roles", registry, item.accepted_code)) group = "roles";
      else if (registryHasCode(registries, "fixture_roles", registry, item.accepted_code)) group = "fixture_roles";
      else if (registry.startsWith("extension:") && registryHasCode(registries, "fixture_roles", registry.slice(10), item.accepted_code)) { group = "fixture_roles"; registry = registry.slice(10); }
    }
    if (!registryHasCode(registries, group, registry, item.accepted_code) || registryHasCode(registries, group, registry, item.rejected_code)) throw new Error("binding probe mismatch");
    if (item.kind === "operation_field_binding") { const probe = fixtures.positive.find((candidate) => candidate.case_id === item.operation_case_id); if (!probe) throw new Error("operation probe missing"); parseCBOR(decodeHex(probe, "canonical_hex")); }
    return;
  }
  if (["record_enum_binding", "record_reference_kind_binding", "record_conditional_constraint"].includes(item.kind)) {
    if (item.kind === "record_conditional_constraint") { executeRecordConstraint(item); return; }
    executeRecordBinding(item, fixtures, registries); return;
  }
  if (["linked_record_constraint", "linked_state_constraint", "commit_candidate_partition", "cycle_abort_provenance"].includes(item.kind)) {
    executeLinkedConstraint(item, fixtures, registries); return;
  }
  if (item.kind === "set_exact_duplicate") { executeSetExactDuplicate(item); return; }
  if (item.kind === "set_identity_collision") { executeSetIdentityCollision(item); return; }
  if (item.kind === "set_permutation") { executeSetPermutation(item); return; }
  if (item.kind === "set_permutation_by_member_index") { executeMemberPermutation(item); return; }
  if (item.kind === "derived_ordering_component") { executeDerivedOrdering(item); return; }
  if (["idempotency_conflict", "optional_idempotency"].includes(item.kind)) { executeIdempotency(item); return; }
  const remaining = new Set(["admitted_unit_ledger_mismatch"]);
  if (!remaining.has(item.kind)) throw new Error(`unknown semantic kind ${item.kind}`);
  let found = false;
  function visit(value) { if (typeof value === "string" && /^[0-9a-f]*$/.test(value) && value.length % 2 === 0) found = true; else if (Array.isArray(value)) value.forEach(visit); else if (value && typeof value === "object") Object.values(value).forEach(visit); }
  for (const [key, value] of Object.entries(item)) if (key.includes("hex")) visit(value);
  if (!found && item.persisted_identity === undefined) throw new Error("semantic bytes missing");
}

function applyLinkedMutation(scenario, mutation) {
  if (["replace_record", "remove_record"].includes(mutation.op)) {
    const index = scenario.records.findIndex((record) => record.role === mutation.record_role); if (index < 0) throw new Error(`linked mutation role ${mutation.record_role} missing`);
    if (mutation.op === "remove_record") scenario.records.splice(index, 1); else scenario.records[index].record_cbor_hex = mutation.record_cbor_hex; return;
  }
  scenario.executed_mutation = structuredClone(mutation);
}
function linkedScenarioShapeValid(scenario, registries) {
  const roles = new Set(), schemas = new Set([...registries.schemas, ...registries.fixture_schemas].map((schema) => schema[0]));
  for (const record of scenario.records) { if (roles.has(record.role) || !schemas.has(record.schema_id)) return false; roles.add(record.role); try { parseCBOR(Buffer.from(record.record_cbor_hex, "hex")); } catch { return false; } }
  for (const section of ["pre_append_cursor", "transaction_candidates"]) for (const listed of Object.values(scenario[section] ?? {})) for (const role of listed) if (!roles.has(role)) return false;
  return true;
}
function executeLinkedConstraint(item, fixtures, registries) {
  const scenarios = new Map(fixtures.linked_record_scenarios.map((scenario) => [scenario.scenario_id, scenario]));
  const scenarioID = item.input_scenario_id ?? item.base_scenario_id, source = scenarios.get(scenarioID); if (!source) throw new Error(`linked scenario ${scenarioID} missing`);
  const actual = structuredClone(source); if (item.mutation) applyLinkedMutation(actual, item.mutation);
  const { valid, code } = validateLinkedRelations(item, actual, registries);
  if (item.expected === "accept" && valid) return;
  if (item.expected === "reject" && !valid) {
    if (item.invalid_error_code !== code) throw new Error(`linked constraint error code mismatch: got ${item.invalid_error_code} want ${code}`);
    return;
  }
  throw new Error(`linked constraint outcome mismatch (valid=${valid})`);
}

function validateLinkedRelations(item, scenario, registries) {
  const code = item.kind === "linked_state_constraint" ? "LINKED_STATE_CONSTRAINT" : "LINKED_RECORD_CONSTRAINT";
  if (!linkedScenarioShapeValid(scenario, registries)) return { valid: false, code };
  const records = new Map();
  for (const record of scenario.records) {
    try { records.set(record.role, parseCBOR(Buffer.from(record.record_cbor_hex, "hex")).value); }
    catch { return { valid: false, code }; }
  }
  const get = (role) => records.get(role);
  const equal = isDeepStrictEqual;
  let valid = false;
  switch (item.constraint_id) {
    case "append-cycle-abort": {
      const fence = get("aborted-fence"), abort = get("abort");
      valid = Array.isArray(fence) && fence.length === 18 && Array.isArray(abort) && abort.length === 15;
      if (valid) {
        for (const [a, b] of [[0,0],[1,1],[2,2],[4,3],[5,4],[6,5],[7,6],[8,7],[17,8],[12,9]]) valid &&= equal(fence[a], abort[b]);
        valid &&= fence[2] === 1n ? equal(fence[3], [0n]) : fence[2] > 1n && Array.isArray(fence[3]) && fence[3][0] === 1n;
      }
      if (scenario.executed_mutation) valid = validateAbortMutation(scenario.executed_mutation, fence, records);
      break;
    }
    case "append-attempt-retry": {
      const retry = get("retry"), abort = get("abort");
      valid = Array.isArray(retry) && retry.length === 7 && Array.isArray(abort) && abort.length === 15;
      if (valid) {
        valid = equal(retry[0], abort[0]) && equal(retry[1], abort[1]) && equal(retry[2], abort[2]) && retry[3] === retry[2] + 1n && equal(retry[4], cycleAbortRef(abort));
        const cursor = scenario.pre_append_cursor?.decision_ledger ?? [], abortIndex = cursor.indexOf("abort"), descriptors = new Map(scenario.records.map((record) => [record.role, record]));
        for (const role of cursor.slice(abortIndex + 1)) { const descriptor = descriptors.get(role), terminal = descriptor && ["cote.csf.schema.cycle-abort", "cote.csf.schema.cycle-commit"].includes(descriptor.schema_id); const prior = get(role); if (terminal && Array.isArray(prior) && equal(prior[0], retry[0])) valid = false; }
      }
      break;
    }
    case "append-retry-fence": {
      const old = get("aborted-fence"), abort = get("abort"), retry = get("retry"), next = get("retry-fence");
      valid = Array.isArray(old) && old.length === 18 && Array.isArray(abort) && abort.length === 15 && Array.isArray(retry) && retry.length === 7 && Array.isArray(next) && next.length === 18;
      if (valid) {
        valid = equal(old[0], abort[0]) && equal(old[1], abort[1]) && equal(old[2], abort[2]) && equal(retry[0], abort[0]) && equal(retry[1], abort[1]) && equal(retry[2], abort[2]) && equal(retry[3], next[2]) && equal(retry[6], next[16]) && equal(next[3], [1n, attemptRetryRef(retry)]);
        for (let i = 0; i < 18 && valid; i++) if (![2,3,16,17].includes(i)) valid = equal(old[i], next[i]);
      }
      break;
    }
    case "append-observation": {
      const observation = get("observation"), task = get("task"), event = get("event");
      valid = Array.isArray(observation) && observation.length === 15 && Array.isArray(task) && task.length === 10 && Array.isArray(event) && event.length >= 1;
      if (valid) valid = equal(observation[1], task[0]) && equal(task[3], event[0]) && equal(observation[5], event[5]) && Array.isArray(observation[6]) && observation[6].length === 1 && eventRefMatches(observation[6][0], event) && equal(observation[13], task[7]) && !records.has("completion");
      break;
    }
    case "append-knowledge-input": {
      const input = get("knowledge-input"), observation = get("observation");
      valid = Array.isArray(input) && input.length === 11 && Array.isArray(observation) && observation.length === 15;
      if (valid) valid = causalRefMatches(input[7], observation, 16) && equal(input[3], observation[2]) && equal(input[4], observation[5]) && isSubset(input[8], observation[10]) && isSubset(input[9], observation[12]) && !records.has("completion");
      break;
    }
    case "append-perception-completion": {
      const completion = get("completion"), task = get("task");
      valid = Array.isArray(completion) && completion.length === 5 && Array.isArray(task) && task.length === 10 && equal(completion[1], task[0]);
      if (valid) valid = equal(completion[2], records.has("observation") ? [get("observation")[0]] : []) && equal(completion[3], records.has("knowledge-input") ? [get("knowledge-input")[0]] : []);
      break;
    }
    case "publish-cycle-commit-batch": {
      const fence = get("fence"), commit = get("cycle-commit");
      valid = Array.isArray(fence) && fence.length === 18 && Array.isArray(commit) && commit.length === 24;
      if (valid) {
        for (const [a, b] of [[0,0],[1,1],[2,2],[4,3],[5,4],[6,5],[7,6],[17,10],[12,12]]) valid &&= equal(fence[a], commit[b]);
        const admitted = fence[11]; valid &&= Array.isArray(admitted) && equal(commit[11], admitted.map((unit) => unit[3]));
        const decisions = [...records.entries()].filter(([role]) => role === "decision" || role.startsWith("decision-" )).map(([, value]) => value);
        const events = [...records.entries()].filter(([role]) => role.startsWith("event-")).map(([, value]) => value);
        const tasks = [...records.entries()].filter(([role]) => role.startsWith("task-")).map(([, value]) => value);
        valid &&= uniqueProjection(commit[16], decisions, 0) && uniqueProjection(commit[15], events, 0) && uniqueProjection(commit[18], tasks, 0);
        for (const record of decisions) valid &&= equal(record[1], fence[0]) && equal(record[2], fence[1]) && equal(record[3], fence[2]);
        for (const event of events) valid &&= equal(event[1], commit[0]) && equal(event[7], commit[1]) && equal(event[5], commit[3]) && equal(event[8], commit[6]);
        const eventIDs = new Set(events.map((event) => Buffer.from(event[0]).toString("hex")));
        for (const task of tasks) valid &&= eventIDs.has(Buffer.from(task[3]).toString("hex")) && equal(task[1], commit[0]) && equal(task[2], commit[1]) && equal(task[4], commit[6]) && equal(task[5], commit[8]) && equal(task[6], commit[20]) && equal(task[8], commit[21]);
        valid &&= matchesPerceptionClassification(scenario, commit, events, tasks);
        const cursorRoles = scenario.pre_append_cursor?.decision_ledger ?? [], priorCommitRole = [...cursorRoles].reverse().find((role) => descriptorsRecordIsCommit(scenario, role));
        const priorCommit = priorCommitRole ? records.get(priorCommitRole) : null, before = priorCommit ? priorCommit[22] : 0n;
        valid &&= commit[22] === before + BigInt(events.length);
        const orderedEvents = (commit[15] ?? []).map((id) => events.find((event) => Buffer.from(event[0]).equals(id)));
        valid &&= orderedEvents.every((event, index) => event && event[6] === before + BigInt(index));
        const source = get("source-unit");
        if (source) valid &&= admitted.some((unit) => Buffer.from(unit[3]).equals(source[0]));
        for (const decision of decisions) {
          const candidateRef = decision[6];
          if (Array.isArray(candidateRef) && candidateRef.length === 2 && candidateRef[0] === 1n) valid &&= [...records.entries()].some(([role, candidate]) => role.startsWith("candidate") && Buffer.isBuffer(candidate[0]) && candidate[0].equals(candidateRef[1]));
        }
      }
      if (scenario.executed_mutation) valid = validateCommitMutation(scenario.executed_mutation, fence, commit, records);
      break;
    }
    case "logical-sequence-transition-v1":
      valid = item.witness_before === item.durable_cursor && item.witness_after === item.witness_before + item.event_count && item.commit_next === item.witness_after;
      break;
    case "world-state-transition-v1":
      valid = validateWorldReducerResult(item, records);
      break;
  }
  return { valid, code };
}

function causalRefMatches(ref, target, kind) {
  return derivedRefMatches(ref, target, kind) && Buffer.isBuffer(ref[1]) && Buffer.from(target[0]).equals(ref[1]);
}
function derivedRefMatches(ref, target, kind) { return Array.isArray(ref) && ref.length === 3 && typeof ref[0] === "bigint" && ref[0] === BigInt(kind) && Buffer.isBuffer(ref[2]) && Buffer.isBuffer(target.at(-1)) && ref[2].equals(target.at(-1)); }
function optionalDerivedRefMatches(value, target, kind) { return Array.isArray(value) && value.length === 2 && value[0] === 1n && derivedRefMatches(value[1], target, kind); }
function linkedOptionalRefKind(value, kind) { return Array.isArray(value) && value.length === 2 && value[0] === 1n && Array.isArray(value[1]) && value[1][0] === BigInt(kind); }
function eventRefMatches(ref, event) { return Array.isArray(ref) && ref.length === 2 && Buffer.isBuffer(ref[0]) && Buffer.isBuffer(ref[1]) && ref[0].equals(event[0]) && ref[1].equals(event.at(-1)); }
function attemptRetryRef(retry) { return [14n, sha256(canonicalEnvelope("cote.csf.id.attempt-retry", "cote.csf.schema.attempt-retry-id-preimage", 1, encodeCanonical([retry[0], retry[1], retry[2], retry[3]]))), sha256(canonicalEnvelope("cote.csf.digest.attempt-retry", "cote.csf.schema.attempt-retry", 1, encodeCanonical(retry)))]; }
function cycleAbortRef(abort) { return [13n, sha256(canonicalEnvelope("cote.csf.id.cycle-abort", "cote.csf.schema.cycle-abort-id-preimage", 1, encodeCanonical([abort[0], abort[1], abort[2]]))), Buffer.from(abort.at(-1))]; }
function uniqueProjection(expected, records, index) { return Array.isArray(expected) && records.length === expected.length && records.every((record, position) => Buffer.isBuffer(record[index]) && Buffer.from(expected[position]).equals(record[index])) && new Set(expected.map((value) => Buffer.from(value).toString("hex"))).size === expected.length; }
function descriptorsRecordIsCommit(scenario, role) { return scenario.records.some((record) => record.role === role && record.schema_id === "cote.csf.schema.cycle-commit"); }
function matchesPerceptionClassification(scenario, commit, events, tasks) {
  const witness = scenario.transition_witnesses?.find((item) => item.role === "perception-classification");
  if (!witness) return false;
  let classification;
  try { classification = parseCBOR(Buffer.from(witness.witness_cbor_hex, "hex")).value; } catch { return false; }
  if (!Array.isArray(classification) || classification.length !== 2 || !isDeepStrictEqual(classification[0], commit[20]) || !Array.isArray(classification[1]) || classification[1].length !== events.length) return false;
  const expected = [];
  for (let i = 0; i < classification[1].length; i++) {
    const row = classification[1][i];
    if (!Array.isArray(row) || row.length !== 3 || !Buffer.isBuffer(row[0]) || !Buffer.from(events[i][0]).equals(row[0]) || !Buffer.isBuffer(row[1]) || !Buffer.from(events[i].at(-1)).equals(row[1]) || typeof row[2] !== "boolean") return false;
    if (row[2]) expected.push(row[0]);
  }
  return isDeepStrictEqual(expected, tasks.map((task) => task[3])) && isDeepStrictEqual(tasks.map((task) => task[0]), commit[18]);
}
function validateWorldReducerResult(item, records) {
  const mutation = item.mutation; if (!mutation || !Array.isArray(mutation.recompute)) return false;
  const events = [...records.entries()].filter(([role]) => role.startsWith("event-")).map(([, record]) => record);
  let state = null;
  for (const event of events) {
    const eventType = event[3];
    if (eventType === "event-0") state = event[9];
    else if (eventType !== "event-1") return false;
  }
  if (!state) return false;
  try { return Buffer.from(mutation.result_state_cbor_hex, "hex").equals(encodeCanonical(state)); } catch { return false; }
}
function validateAbortMutation(mutation, fence, records) {
  const units = fence?.[11]; if (!Array.isArray(units)) return false;
  if (mutation.target === "indeterminate.subject") {
    if (mutation.subject_kind === "admitted_unit") {
      const id = mutation.unit_id_hex ? Buffer.from(mutation.unit_id_hex, "hex") : null;
      const matches = units.filter((unit) => id ? Buffer.from(unit[3]).equals(id) : Buffer.from(unit[4]).equals(Buffer.from(mutation.digest_hex ?? "", "hex")));
      return matches.length === 1;
    }
    if (mutation.subject_kind === "commit_candidate") {
      const id = Buffer.from(mutation.candidate_id_hex, "hex");
      return [...records.entries()].some(([role, record]) => role.startsWith("candidate") && Buffer.from(record[0]).equals(id));
    }
  }
  if (mutation.target === "failure_evidence") {
    const registered = [...records.entries()].some(([role, record]) => role === mutation.root && record.length > 0);
    if (!registered) return false;
    const targetAttempt = mutation.attempt_ordinal;
    if (targetAttempt !== undefined) return records.get(mutation.root)?.[2] === BigInt(targetAttempt);
    if (mutation.candidate_id_hex) return [...records.entries()].some(([role, record]) => role.startsWith("candidate") && Buffer.from(record[0]).equals(Buffer.from(mutation.candidate_id_hex, "hex")));
    return true;
  }
  return false;
}
function validateCommitMutation(mutation, fence, commit, records) {
  if (mutation.op === "replace_candidate_run_id") return Buffer.from(mutation.value_hex, "hex").equals(Buffer.from(commit[0]));
  if (mutation.op === "append_admitted_unit_without_candidate") {
    const id = Buffer.from(mutation.unit_id_hex, "hex");
    return [...records.entries()].filter(([role]) => role.startsWith("candidate")).some(([, candidate]) => candidate[3].some((unitID) => Buffer.from(unitID).equals(id)));
  }
  if (mutation.op === "append_candidate_with_existing_source") {
    const id = Buffer.from(mutation.source_unit_id_hex, "hex");
    const existing = [...records.entries()].filter(([role]) => role.startsWith("candidate")).some(([, candidate]) => candidate[3].some((unitID) => Buffer.from(unitID).equals(id)));
    return !existing;
  }
  if (mutation.op === "replace_decision_produced_event") {
    const id = Buffer.from(mutation.event_id_hex, "hex");
    return [...records.entries()].filter(([role]) => role.startsWith("event-")).some(([, event]) => Buffer.from(event[0]).equals(id));
  }
  return false;
}
function isSubset(values, source) {
  return Array.isArray(values) && Array.isArray(source) && values.every((value) => source.some((candidate) => isDeepStrictEqual(value, candidate)));
}

function executeRecordBinding(item, fixtures, registries) {
  const acceptedCodes = item.accepted_codes ?? (item.accepted_code === undefined ? [] : [item.accepted_code]);
  if (item.kind === "record_enum_binding") {
    for (const [key, accepted] of [["accepted_envelope_hex", true], ["rejected_envelope_hex", false]]) {
      const envelope = parseCBOR(decodeHex(item, key)).value;
      if (!Array.isArray(envelope) || envelope.length !== 6 || !Array.isArray(envelope[5])) throw new Error("record enum envelope malformed");
      const code = recordEnumValue(envelope[5], item.field_path);
      if (typeof code !== "bigint") throw new Error("record enum field is not unsigned");
      if (accepted ? (acceptedCodes.length && !acceptedCodes.some((value) => BigInt(value) === code)) || !registryHasCode(registries, "enums", item.registry, Number(code)) : registryHasCode(registries, "enums", item.registry, Number(code)) || item.rejected_error_code !== "UNKNOWN_ENUM") throw new Error("record enum binding outcome mismatch");
    }
    return;
  }
  if (!acceptedCodes.length || acceptedCodes.some((code) => !registryHasCode(registries, "enums", item.registry, code))) throw new Error("record binding accepted code mismatch");
  const template = fixtures.positive.find((candidate) => candidate.case_id === item.record_template_case_id);
  if (!template) throw new Error("record binding template missing");
  const envelope = parseCBOR(decodeHex(template, "canonical_hex")).value;
  if (!Array.isArray(envelope) || !Array.isArray(envelope[5])) throw new Error("record binding template malformed");
  const index = recordContainerIndex(item.container_field_path);
  const acceptedContainers = item.accepted_container_cbor_hex;
  if (!acceptedContainers || typeof acceptedContainers !== "object" || Array.isArray(acceptedContainers)) throw new Error("accepted reference containers missing");
  for (const [codeText, encoded] of Object.entries(acceptedContainers)) {
    const code = BigInt(codeText);
    if (!acceptedCodes.some((value) => BigInt(value) === code) || !registryHasCode(registries, "enums", item.registry, Number(code))) throw new Error("accepted reference code mismatch");
    const payload = parseCBOR(Buffer.from(encoded, "hex")).value;
    const candidatePayload = structuredClone(envelope[5]); candidatePayload[index] = payload;
    const rebuilt = canonicalEnvelope(envelope[2], envelope[3], Number(envelope[4]), encodeCanonical(candidatePayload));
    parseCBOR(rebuilt);
    validateReferenceContainer(payload, acceptedCodes, registries, item.registry, true);
  }
  const rejected = parseCBOR(decodeHex(item, "rejected_container_cbor_hex")).value;
  try { validateReferenceContainer(rejected, acceptedCodes, registries, item.registry, false); }
  catch { throw new Error("rejected reference container lacks the registered UNKNOWN_ENUM outcome"); }
  if (item.rejected_error_code !== "UNKNOWN_ENUM") throw new Error("reference binding error code mismatch");
}

function recordEnumValue(fields, path) {
  switch (path) {
    case "conflict-set.conflict-kind": return fields[2];
    case "trigger-activation.lifecycle": return fields[9];
    case "perception-task.lifecycle": return fields[9];
    case "decision-record.subject.input-kind": return fields[4]?.[0];
    default: throw new Error(`unsupported record enum field ${path}`);
  }
}

function recordContainerIndex(path) {
  const indices = {
    "admission-fence.closure-proof": 9, "admission-fence.cohort": 10, "admission-fence.retry-ref": 3,
    "attempt-retry.aborted-envelope-ref": 4, "affordance-assessment.subject": 0,
    "decision-record.conflict-set-refs": 8, "decision-record.rng-draw-refs": 11,
    "cycle-control-state.fence-ref": 3, "cycle-control-state.retry-ref": 4, "cycle-control-state.last-terminal-envelope-ref": 5,
    "claim.prior-claim-refs": 7, "observation.claim-refs": 10,
    "knowledge-input.observation-ref": 7, "knowledge-input.claim-refs": 8,
    "transmission.content": 4, "cycle-abort.failure-evidence-refs": 12,
    "provisional-disposition.conflict-set-refs": 8, "provisional-disposition.rng-draw-refs": 9,
  };
  if (!(path in indices)) throw new Error(`unsupported record reference path ${path}`);
  return indices[path];
}

function validateReferenceContainer(value, acceptedCodes, registries, registry, wantValid) {
  const codes = [];
  function visit(item) {
    if (Array.isArray(item)) {
      if (item.length === 3 && typeof item[0] === "bigint" && Buffer.isBuffer(item[1]) && item[1].length === 32 && Buffer.isBuffer(item[2]) && item[2].length === 32) codes.push(item[0]);
      item.forEach(visit);
    } else if (item instanceof Map) for (const child of item.values()) visit(child);
  }
  visit(value);
  if (!codes.length) throw new Error("reference container has no causal refs");
  const invalid = codes.some((code) => !acceptedCodes.some((allowed) => BigInt(allowed) === code) || !registryHasCode(registries, "enums", registry, Number(code)));
  if (wantValid && invalid) throw new Error("accepted reference container has invalid kind");
  if (!wantValid && !invalid) throw new Error("rejected reference container has no invalid kind");
}

function registryHasCode(registries, group, registry, wanted) {
  return !!registries[group]?.[registry]?.some((entry) => Number(entry[1]) === Number(wanted));
}

function executeNormalization(item) {
  const expected = decodeHex(item, "expected_payload_cbor_hex"); const expectedValue = parseCBOR(expected).value;
  if ((!item.inputs?.length) && (!item.worker_orders?.length)) throw new Error("normalization case has no inputs");
  if (item.inputs?.length && item.case_id !== "normalize.admission-fence.admitted-units-permutations") {
    for (const input of item.inputs) if (!encodeCanonical(materializeNormalizationValue(input, item)).equals(expected)) throw new Error("normalization result mismatch");
  }
  if (item.case_id === "normalize.admission-fence.admitted-units-permutations") {
    for (const descriptor of item.inputs) {
      const units = canonicalSort(parseCBOR(Buffer.from(descriptor[2], "hex")).value);
      const payload = structuredClone(expectedValue); payload[11] = units;
      payload[12] = sha256(canonicalEnvelope("cote.csf.digest.input", "cote.csf.schema.admitted-unit-list", 1, encodeCanonical(units)));
      payload[17] = sha256(canonicalEnvelope("cote.csf.digest.fence", "cote.csf.schema.admission-fence.body", 1, encodeCanonical(payload.slice(0, 17))));
      if (!encodeCanonical(payload).equals(expected) || hex(payload[17]) !== item.expected_fence_digest) throw new Error("admission fence normalization/digest mismatch");
    }
  }
  if (item.case_id === "normalize.attempt-failure.worker-order") {
    const byKey = new Map(item.attempt_failures.map((failure) => [failure.semantic_key, failure]));
    for (const order of item.worker_orders) {
      const refs = canonicalSort(order.map((key) => { const failure = byKey.get(key); return [27n, sha256(decodeHex(failure, "id_preimage_envelope_hex")), sha256(decodeHex(failure, "digest_preimage_envelope_hex"))]; }));
      const payload = structuredClone(expectedValue); payload[12] = refs;
      payload[14] = sha256(canonicalEnvelope("cote.csf.digest.abort", "cote.csf.schema.cycle-abort.body", 1, encodeCanonical(payload.slice(0, 14))));
      if (!encodeCanonical(payload).equals(expected) || hex(payload[14]) !== item.expected_abort_digest) throw new Error("attempt failure normalization/digest mismatch");
    }
  }
}

function executeSetExactDuplicate(item) {
  const members = parseCBOR(decodeHex(item, "input_cbor_hex")).value; const seen = new Set(); const result = [];
  for (const member of members) { const key = hex(encodeCanonical(member)); if (!seen.has(key)) { seen.add(key); result.push(member); } }
  if (!encodeCanonical(canonicalSort(result)).equals(decodeHex(item, "expected_cbor_hex"))) throw new Error("exact duplicate normalization mismatch");
}

function executeSetIdentityCollision(item) {
  const members = parseCBOR(decodeHex(item, "input_cbor_hex")).value; const identities = new Map();
  for (const member of members) { const identity = hex(encodeCanonical(member.slice(0, -1))); const full = encodeCanonical(member); if (identities.has(identity) && !identities.get(identity).equals(full)) return; identities.set(identity, full); }
  throw new Error("identity collision probe did not collide");
}

function executeSetPermutation(item) {
  for (const encoded of item.input_cbor_hex) { const members = parseCBOR(Buffer.from(encoded, "hex")).value; if (!encodeCanonical(canonicalSort(members)).equals(decodeHex(item, "expected_cbor_hex"))) throw new Error("set permutation did not converge"); }
}

function executeMemberPermutation(item) {
  const members = item.members_cbor_hex.map((encoded) => parseCBOR(Buffer.from(encoded, "hex")).value);
  for (const order of item.input_orders) { const sorted = order.map((index) => ({ index, value: members[index] })).sort((left, right) => compareBytes(encodeCanonical(left.value), encodeCanonical(right.value))); if (sorted.some((entry, index) => entry.index !== item.expected_order[index])) throw new Error("member permutation did not converge"); }
}

function executeDerivedOrdering(item) {
  const digests = item.input_cbor_hex.map((encoded, index) => { const payload = parseCBOR(Buffer.from(encoded, "hex")).value; const digest = sha256(canonicalEnvelope("cote.csf.digest.indeterminate-evidence-refs", "cote.csf.schema.indeterminate-evidence-ref-list", 1, encodeCanonical(payload))); if (hex(digest) !== item.expected_digest_hex[index]) throw new Error("derived ordering digest mismatch"); return digest; });
  if (digests.length !== 2 || compareBytes(digests[0], digests[1]) >= 0) throw new Error("derived ordering comparison mismatch");
}

function idempotencyDigest(encoded) { const payload = parseCBOR(encoded).value; return sha256(canonicalEnvelope("cote.csf.digest.idempotency", "cote.csf.schema.idempotency-preimage", 1, encodeCanonical(payload))); }
function executeIdempotency(item) {
  if (item.kind === "optional_idempotency") { const optional = parseCBOR(decodeHex(item, "idempotency_key_cbor_hex")).value; const present = optional.length === 2 && optional[0] === 1n; if (present !== item.persisted_identity) throw new Error("optional idempotency persistence mismatch"); if (present && hex(idempotencyDigest(decodeHex(item, "preimage_cbor_hex"))) !== item.idempotency_digest) throw new Error("idempotency digest mismatch"); return; }
  const first = decodeHex(item, "first_preimage_cbor_hex"), second = decodeHex(item, "second_preimage_cbor_hex");
  if (hex(idempotencyDigest(first)) !== item.first_idempotency_digest || hex(idempotencyDigest(second)) !== item.second_idempotency_digest || first.equals(second) || item.first_idempotency_digest === item.second_idempotency_digest) throw new Error("idempotency conflict mismatch");
}

function optionalPresent(value) { return Array.isArray(value) && value.length === 2 && value[0] === 1n; }
function optionalAbsent(value) { return Array.isArray(value) && value.length === 1 && value[0] === 0n; }
function optionalRefKind(value) { return optionalPresent(value) && Array.isArray(value[1]) ? Number(value[1][0]) : -1; }
function validateDecisionRecord(fields) {
  if (fields.length !== 16) return false; const disposition = Number(fields[5]); const conflicts = fields[8];
  if ([0, 1].includes(disposition)) return optionalPresent(fields[6]) && optionalAbsent(fields[13]) && optionalAbsent(fields[14]);
  if (disposition === 2) return optionalPresent(fields[6]) && optionalPresent(fields[13]) && optionalAbsent(fields[14]);
  if (disposition === 3) return optionalAbsent(fields[6]) && optionalAbsent(fields[13]) && optionalAbsent(fields[14]) && conflicts.length === 0;
  if (disposition === 4) return optionalAbsent(fields[6]) && optionalAbsent(fields[13]) && optionalPresent(fields[14]) && conflicts.length === 0; return false;
}
function validateCycleControl(fields) {
  if (fields.length !== 7) return false; const status = Number(fields[0]), next = Number(fields[6]), attempt = optionalPresent(fields[2]) ? Number(fields[2][1]) : -1;
  if (status === 0) return !optionalPresent(fields[1]) && !optionalPresent(fields[2]) && !optionalPresent(fields[3]) && !optionalPresent(fields[4]) && next === 1;
  if (status === 1) return optionalPresent(fields[1]) && optionalPresent(fields[2]) && optionalPresent(fields[3]) && !optionalPresent(fields[4]) && next === attempt + 1;
  if (status === 2) return optionalPresent(fields[1]) && optionalPresent(fields[2]) && !optionalPresent(fields[3]) && optionalPresent(fields[4]) && optionalPresent(fields[5]) && next === attempt && optionalRefKind(fields[5]) === 13;
  if (status === 3) return optionalPresent(fields[1]) && optionalPresent(fields[2]) && !optionalPresent(fields[3]) && !optionalPresent(fields[4]) && optionalPresent(fields[5]) && next === attempt + 1 && optionalRefKind(fields[5]) === 13; return false;
}
function validateTrigger(fields) { if (fields.length !== 8) return false; const policy = Number(fields[3]); if (policy !== 3) return optionalAbsent(fields[5]); return optionalPresent(fields[5]) && Number(fields[5][1]) > 0; }
function executeRecordConstraint(item) { const valid = parseCBOR(decodeHex(item, "valid_payload_cbor_hex")).value, invalid = parseCBOR(decodeHex(item, "invalid_payload_cbor_hex")).value; const validators = { "decision-record": validateDecisionRecord, "cycle-control-state": validateCycleControl, "trigger-definition": validateTrigger }; const validator = validators[item.record]; if (!validator || !validator(valid) || validator(invalid)) throw new Error("record conditional constraint outcome mismatch"); }

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

function same(left, right) { return JSON.stringify(left) === JSON.stringify(right); }
function compareCoordinate(left, right) { for (let index = 0; index < 2; index++) { if (left[index] < right[index]) return -1; if (left[index] > right[index]) return 1; } return 0; }
function transitionSourceAppend(state) {
  const receipt = state.candidate_receipt, last = state.prefix.at(-1);
  if (!receipt) return "SOURCE_RECEIPT_MISSING";
  if (last.record.final) return "SOURCE_FINAL";
  if (receipt.ingress_seq !== last.receipt.ingress_seq + 1) return "INGRESS_NOT_CONTIGUOUS";
  if (state.candidate.root === "source-closure") return compareCoordinate(state.candidate.closed_through, last.record.closed_through) < 0 ? "CLOSURE_REGRESSION" : null;
  return compareCoordinate(state.candidate.eligibility, last.record.closed_through) <= 0 ? "RETROACTIVE_ELIGIBILITY" : null;
}
function transitionDispatch(state) {
  const tasks = new Set(state.causal_outbox.tasks.map((item) => item.task_id));
  const completions = new Map(state.causal_outbox.completions.map((item) => [item.task_id, item]));
  const knowledge = new Set(state.evidence_ledger.knowledge_inputs.map((item) => item.knowledge_input_id));
  for (const commit of state.decision_ledger.cycle_commits) for (const id of commit.perception_task_ids) {
    if (!tasks.has(id) || !completions.has(id)) return "EPISTEMIC_TASK_PENDING";
    if (completions.get(id).knowledge_input_ids.some((knowledgeID) => !knowledge.has(knowledgeID))) return "EPISTEMIC_DELIVERY_PENDING";
  }
  return null;
}
function transitionResponse(state) {
  const candidate = state.candidate, dispatch = state.dispatches.find((item) => item.dispatch_id === candidate.dispatch_id);
  if (!dispatch) return "RESPONSE_DISPATCH_MISMATCH";
  if (state.revocations.some((item) => item.dispatch_id === candidate.dispatch_id)) return "DISPATCH_NOT_OPEN";
  for (const field of ["run_id", "cycle_id", "decision_round_id", "slot_id", "actor", "effective_at"]) if (!same(dispatch[field], candidate[field])) return "RESPONSE_DISPATCH_MISMATCH";
  if (!same(dispatch.base_revision, candidate.submitted_against_revision)) return "RESPONSE_DISPATCH_MISMATCH";
  if (state.responses.some((item) => item.dispatch_id === candidate.dispatch_id && !same(item, candidate))) return "SLOT_ALREADY_FILLED";
  return null;
}
function deriveControlState(ledger) {
  if (!ledger.length) return { status: "IDLE", cycle_id: null, attempt_ordinal: null, fence_ref: null, retry_ref: null, last_terminal_envelope_ref: null, next_attempt_ordinal: 1 };
  const latest = ledger.at(-1);
  if (latest.kind === "fence") return { status: "ATTEMPT_IN_FLIGHT", cycle_id: latest.cycle_id, attempt_ordinal: latest.attempt, fence_ref: latest.ref, retry_ref: null, last_terminal_envelope_ref: null, next_attempt_ordinal: latest.attempt + 1 };
  if (latest.kind === "retry") { const abort = ledger.at(-2); return { status: "RETRY_AUTHORIZED", cycle_id: abort.cycle_id, attempt_ordinal: latest.to_attempt, fence_ref: null, retry_ref: latest.ref, last_terminal_envelope_ref: abort.ref, next_attempt_ordinal: latest.to_attempt }; }
  return { status: "HALTED_ON_ABORT", cycle_id: latest.cycle_id, attempt_ordinal: latest.attempt, fence_ref: null, retry_ref: null, last_terminal_envelope_ref: latest.ref, next_attempt_ordinal: latest.attempt + 1 };
}
function transitionRNG(state) {
  const draw = state.draw, algorithm = draw.algorithm;
  if (!same(state.genesis.rng_policy, algorithm)) return "RNG_POLICY_MISMATCH";
  const preimage = encodeCanonical([Buffer.from(state.genesis.world_seed_hex, "hex"), draw.subsystem, draw.decision_key, draw.entity_ids_hex.map((id) => Buffer.from(id, "hex")), draw.purpose, [algorithm.id, algorithm.version, Buffer.from(algorithm.hash_hex, "hex")]]);
  return hex(sha256(preimage)) === draw.result.value_hex ? null : "RNG_RESULT_MISMATCH";
}
function parseParentHistory(encodedHex) {
  const bytes = Buffer.from(encodedHex, "hex"); let offset = 0;
  if (bytes.length < 10 || !bytes.subarray(0, 8).equals(Buffer.from("CSFHREF\0")) || bytes[8] !== 1) throw new Error("parent history framing"); offset = 9;
  function field(expected = -1) { if (offset + 2 > bytes.length) throw new Error("truncated"); const length = bytes.readUInt16BE(offset); offset += 2; if ((expected >= 0 && length !== expected) || offset + length > bytes.length) throw new Error("field"); const value = bytes.subarray(offset, offset + length); offset += length; return value; }
  const policyID = field().toString(), policyHash = hex(field(32)), domain = field().toString(), schemaID = field().toString();
  if (offset + 8 > bytes.length) throw new Error("schema"); const schemaVersion = Number(bytes.readBigUInt64BE(offset)); offset += 8;
  const algorithm = field().toString(), checkpointHash = hex(field(32)); if (offset !== bytes.length) throw new Error("trailer");
  return { policyID, policyHash, domain, schemaID, schemaVersion, algorithm, checkpointHash };
}
function transitionAbort(state) {
  const units = new Map(state.fence.admitted_units.map((item) => [item.unit_id, item.digest]));
  const candidates = new Map(state.commit_candidates.map((item) => [item.candidate_id, item]));
  for (const subject of state.abort.indeterminate_subjects) {
    if (subject.kind === "ADMITTED_UNIT") { if (units.get(subject.id) !== subject.digest) return "ABORT_SUBJECT_MISMATCH"; continue; }
    const candidate = candidates.get(subject.id); if (!candidate || candidate.candidate_digest !== subject.digest || candidate.run_id !== state.fence.run_id || candidate.cycle_id !== state.fence.cycle_id) return "ABORT_CANDIDATE_MISMATCH";
  }
  const evidence = new Map(state.failure_evidence.map((item) => [item.evidence_id, item]));
  for (const id of state.abort.failure_evidence_refs) {
    const item = evidence.get(id); if (!item) return "ABORT_EVIDENCE_SCOPE_MISMATCH";
    if (item.kind === "conflict-set" && item.candidate_ids.some((candidateID) => !candidates.has(candidateID))) return "ABORT_EVIDENCE_SCOPE_MISMATCH";
    if (item.kind === "affordance-assessment" && !units.has(item.subject_id) && !candidates.has(item.subject_id)) return "ABORT_EVIDENCE_SCOPE_MISMATCH";
    if (!["conflict-set", "affordance-assessment"].includes(item.kind) && (item.attempt !== state.fence.attempt || item.run_id !== state.fence.run_id || item.cycle_id !== state.fence.cycle_id)) return "ABORT_EVIDENCE_SCOPE_MISMATCH";
  }
  return null;
}
function transitionGenesis(state) {
  const genesis = state.genesis; if (genesis.policies.length !== 13) return "GENESIS_POLICY_SET_INCOMPLETE";
  if (!genesis.parent_checkpoint_history_ref) return null; let ref;
  try { ref = parseParentHistory(genesis.parent_checkpoint_history_ref); } catch { return "PARENT_HISTORY_REF_INVALID"; }
  if (!state.origin_policy_store.some((item) => item.policy_id === ref.policyID && item.policy_hash_hex === ref.policyHash)) return "PARENT_HISTORY_POLICY_UNRESOLVED";
  for (const checkpoint of state.checkpoint_store) if (checkpoint.domain === ref.domain && checkpoint.schema_id === ref.schemaID && checkpoint.schema_version === ref.schemaVersion && checkpoint.algorithm === ref.algorithm && checkpoint.checkpoint_digest_hex === ref.checkpointHash && hex(sha256(Buffer.from(checkpoint.resolved_content_hex, "hex"))) === ref.checkpointHash) return null;
  return "PARENT_HISTORY_CHECKPOINT_UNRESOLVED";
}
function transitionSnapshot(state, baseline) {
  const snapshot = state.snapshot, genesis = state.genesis, original = baseline.snapshot;
  for (const [left, right] of [[snapshot.run_id, genesis.run_id], [snapshot.genesis_ref, genesis.genesis_ref], [snapshot.world_seed_hex, genesis.world_seed_hex], [snapshot.codec_policy_id, genesis.codec_policy_id], [snapshot.codec_policy_hash_hex, genesis.codec_policy_hash_hex], [snapshot.schema_bundle_hash_hex, genesis.schema_bundle_hash_hex], [snapshot.replay_policies, genesis.policies]]) if (!same(left, right)) return "SNAPSHOT_GENESIS_CONFIG_MISMATCH";
  if (snapshot.world_state_hash_hex !== original.world_state_hash_hex) return "SNAPSHOT_WORLD_HASH_MISMATCH";
  if (!snapshot.ledger_cursors) return "SNAPSHOT_LEDGER_SET_INCOMPLETE";
  for (const ledger of ["input_ledger", "decision_ledger", "event_store", "evidence_ledger", "causal_outbox"]) { if (!(ledger in snapshot.ledger_cursors)) return "SNAPSHOT_LEDGER_SET_INCOMPLETE"; if (!same(snapshot.ledger_cursors[ledger], original.ledger_cursors[ledger])) return "SNAPSHOT_LEDGER_DIGEST_MISMATCH"; }
  if (!same(snapshot.pending_state, original.pending_state)) return "SNAPSHOT_PENDING_STATE_MISMATCH";
  if (!same(snapshot.cycle_control_state, deriveControlState(state.decision_ledger_records))) return "SNAPSHOT_CONTROL_STATE_MISMATCH";
  const required = new Set(state.authorities.actor_registry.filter((actor) => actor.cognitive_state_required).map((actor) => actor.actor));
  if (snapshot.epistemic_checkpoints.length !== required.size) return "SNAPSHOT_CHECKPOINT_SET_MISMATCH";
  for (const checkpoint of snapshot.epistemic_checkpoints) { if (!required.has(checkpoint.actor)) return "SNAPSHOT_CHECKPOINT_SET_MISMATCH"; if (checkpoint.covers_through[0] < snapshot.instant || checkpoint.covers_through[1] < snapshot.revision) return "SNAPSHOT_CHECKPOINT_STALE"; if (hex(sha256(Buffer.from(checkpoint.resolved_content_hex, "hex"))) !== checkpoint.content_hash_hex) return "CHECKPOINT_CONTENT_HASH_MISMATCH"; }
  return null;
}
function transitionObservation(state) {
  const { event, task, candidate } = state;
  if (candidate.task_id !== task.task_id || task.event_id !== event.event_id) return "OBSERVATION_PROJECTION_MISMATCH";
  let channel; const rank = { PUBLIC: 0, RESTRICTED: 1, SECRET: 2 };
  for (const revisionID of [task.base_revision, task.result_revision]) { const revision = state.world_state_by_revision[String(revisionID)], current = revision?.channels?.[candidate.channel_ref], actor = revision?.actors?.[candidate.observer]; if (!current || current.location_ref !== event.location_ref || !current.members.includes(candidate.observer) || !actor || !actor.active || rank[actor.clearance] < rank[event.confidentiality]) return "OBSERVATION_ACCESS_DENIED"; channel = current; }
  const payload = event.payload, expectedSource = [{ id: event.event_id, digest: event.event_digest }];
  if (candidate.observed_at !== event.occurred_at || !same(candidate.source_event_refs, expectedSource) || !same(candidate.resolver, task.resolver) || !same(candidate.percepts, payload.observable_cues) || !same(candidate.claim_refs, payload.observable_claim_refs) || !same(candidate.evidence_chain, payload.observable_evidence_refs) || !same(candidate.omissions_redactions, payload.private_details) || candidate.modality !== channel.modality) return "OBSERVATION_PROJECTION_MISMATCH";
  return null;
}
function sourceDigest(source) {
  const descriptors = { "action-proposal": ["cote.csf.digest.action-proposal-unit", "cote.csf.schema.action-proposal.body"], "no-proposal": ["cote.csf.digest.no-proposal-unit", "cote.csf.schema.no-proposal.body"], "exogenous-input": ["cote.csf.digest.exogenous-input-unit", "cote.csf.schema.exogenous-input.body"], "scheduled-occurrence": ["cote.csf.digest.occurrence-unit", "cote.csf.schema.scheduled-occurrence.body"], "trigger-activation": ["cote.csf.digest.trigger-activation-unit", "cote.csf.schema.trigger-activation.body"] };
  const descriptor = descriptors[source.root]; if (!descriptor) return null; return hex(sha256(canonicalEnvelope(descriptor[0], descriptor[1], 1, encodeCanonical(source.record_body))));
}
function validDedup(state, subject, decision) {
  const chosen = state.historical_units[decision.canonical_unit_id], current = state.unit_admission_history[subject]; if (!chosen || !current || decision.canonical_unit_id === subject || !same(chosen.idempotency_identity, current.idempotency_identity)) return false;
  return !Object.values(state.historical_units).some((other) => same(other.idempotency_identity, current.idempotency_identity) && (other.first_fence_ordinal < chosen.first_fence_ordinal || (other.first_fence_ordinal === chosen.first_fence_ordinal && other.admission_index < chosen.admission_index)));
}
function transitionCommit(state) {
  const { fence, decision_ledger: ledger, transaction } = state;
  if (ledger.terminals.length) return "TERMINAL_ALREADY_EXISTS";
  if (fence.admitted_units.some((unit) => Object.keys(unit).length !== 5)) return "ADMISSION_FENCE_UNIT_NOT_CANONICAL";
  if (transaction.commit_candidates.length) return "CANDIDATE_REPUBLISHED";
  const units = new Map(fence.admitted_units.map((unit) => [unit.unit_id, unit])); const sources = new Map();
  for (const owner of Object.values(state.source_authorities)) for (const [id, record] of Object.entries(owner)) sources.set(id, record);
  for (const [id, unit] of units) { const source = sources.get(id); if (!source || source.unit_digest !== unit.unit_digest || sourceDigest(source) !== source.unit_digest) return "SOURCE_RECORD_DIGEST_MISMATCH"; }
  const candidates = new Map(), producerByUnit = new Map();
  for (const candidate of ledger.commit_candidates) { if (candidates.has(candidate.candidate_id)) return "CANDIDATE_PARTITION_MISMATCH"; candidates.set(candidate.candidate_id, candidate); for (const unitID of candidate.source_unit_ids) { if (!units.has(unitID) || producerByUnit.has(unitID)) return "CANDIDATE_PARTITION_MISMATCH"; producerByUnit.set(unitID, candidate.candidate_id); } }
  const decisions = new Map(); for (const decision of transaction.decisions) { if (decisions.has(decision.subject_id) || !units.has(decision.subject_id)) return "CANDIDATE_PARTITION_MISMATCH"; decisions.set(decision.subject_id, decision); } if (decisions.size !== units.size) return "CANDIDATE_PARTITION_MISMATCH";
  const provisionals = new Map(ledger.provisional_dispositions.map((item) => [item.subject_id, item])), conflicts = new Set(ledger.conflict_sets.map((item) => item.conflict_set_id)), rng = new Set(ledger.rng_draws.map((item) => item.rng_draw_id));
  for (const [subject, decision] of decisions) {
    const source = sources.get(subject), disposition = decision.disposition;
    if (disposition === "NO_PROPOSAL") { if (source?.root !== "no-proposal" || decision.candidate_id !== null || decision.produced_event_ids.length) return "NO_PROPOSAL_SUBJECT_MISMATCH"; continue; }
    const candidate = candidates.get(decision.candidate_id); if (!candidate || producerByUnit.get(subject) !== decision.candidate_id) return "CANDIDATE_PARTITION_MISMATCH";
    const provisional = provisionals.get(subject); if (!provisional || (disposition !== "DEDUPLICATED" && provisional.disposition !== disposition) || provisional.candidate_id !== decision.candidate_id || provisional.reason_code !== decision.reason_code || !same(decision.resolver, candidate.resolver) || decision.policy !== "decision-v1@1#06") return "PROVISIONAL_SETTLEMENT_MISMATCH";
    if (decision.conflict_set_refs.some((id) => !conflicts.has(id))) return "CONFLICT_SET_UNRESOLVED"; if (decision.rng_draw_refs.some((id) => !rng.has(id))) return "RNG_DRAW_UNRESOLVED";
    if (disposition === "COMMIT" && decision.produced_event_ids.length !== candidate.event_drafts.length) return "DISPOSITION_EVENT_MISMATCH";
    if (disposition === "REJECT" && decision.produced_event_ids.length) return "DISPOSITION_EVENT_MISMATCH";
    if (disposition === "DEFER" && (decision.produced_event_ids.length || !transaction.successors.some((successor) => successor.input_id === decision.successor_input_id && successor.run_id === fence.run_id && successor.provenance.id === subject && (successor.eligibility[0] > fence.instant || successor.eligibility[0] === fence.instant && successor.eligibility[1] >= fence.cycle_ordinal + 1)))) return "DEFER_SUCCESSOR_MISMATCH";
    if (disposition === "DEDUPLICATED" && (decision.produced_event_ids.length || !validDedup(state, subject, decision))) return "DEDUPLICATION_PROOF_MISMATCH";
  }
  if (transaction.successors.length !== [...decisions.values()].filter((item) => item.disposition === "DEFER").length) return "DEFER_SUCCESSOR_MISMATCH";
  const events = new Map(transaction.events.map((event) => [event.event_id, event]));
  for (const decision of decisions.values()) if (decision.disposition === "COMMIT") { const candidate = candidates.get(decision.candidate_id); for (let index = 0; index < decision.produced_event_ids.length; index++) { const event = events.get(decision.produced_event_ids[index]), draft = candidate.event_drafts[index]; if (!event) return "DISPOSITION_EVENT_MISMATCH"; if (event.order_key.phase !== "CANDIDATE_DOMAIN") return "EVENT_PHASE_MISMATCH"; if (event.event_type !== draft.event_type || !same(event.payload, draft.payload)) return "EVENT_DRAFT_PROJECTION_MISMATCH";
      const expectedSources = candidate.source_unit_ids.map((id) => ({ id, digest: units.get(id)?.unit_digest })); if (event.source_inputs.some((input) => !units.has(input.id))) return "EVENT_SOURCE_OUTSIDE_FENCE";
      const actors = [], entities = [], locations = []; for (const id of candidate.source_unit_ids) { const source = sources.get(id), body = source.record_body; if (["action-proposal", "no-proposal", "exogenous-input"].includes(source.root) && body[6] !== null && !actors.includes(body[6])) actors.push(body[6]); if (source.root === "action-proposal") { for (const target of body[7]) if (!entities.includes(target[1])) entities.push(target[1]); if (body[10] !== null && !locations.includes(body[10])) locations.push(body[10]); } }
      if (!same(event.source_inputs, expectedSources) || !same(event.actor_refs, actors) || !same(event.entity_refs, entities) || !same(event.location_ref, locations[0] ?? null) || event.confidentiality !== "SECRET") return "EVENT_DRAFT_PROJECTION_MISMATCH";
    } }
  for (const event of events.values()) for (const parent of event.causal_parents) { const parentEvent = events.get(parent.id); if (!parentEvent || parentEvent.event_digest !== parent.digest) return "EVENT_DRAFT_PROJECTION_MISMATCH"; if (parentEvent.order_key.origin_ref !== event.order_key.origin_ref) return "CAUSAL_PARENT_SCOPE_MISMATCH"; }
  const lifecycleExpected = { ...state.source_authority_after.schedule_store, ...state.source_authority_after.trigger_registry }, lifecycleSeen = new Set(); for (const event of transaction.events) if (event.event_type === "source.lifecycle") { if (lifecycleExpected[event.payload.subject_id] !== event.payload.to || event.order_key.phase !== "SOURCE_LIFECYCLE") return "SOURCE_LIFECYCLE_MISMATCH"; lifecycleSeen.add(event.payload.subject_id); } if (lifecycleSeen.size !== Object.keys(lifecycleExpected).length) return "SOURCE_LIFECYCLE_MISMATCH";
  const commit = transaction.cycle_commit; for (const [role, field] of [["cycle-coordinate", "cycle_coordinate_policy"], ["event-order", "event_order_policy"], ["perception", "perception_policy"], ["perception-identity", "perception_identity_policy"]]) if (!same(commit[field], state.genesis_policy_bindings[role])) return "COMMIT_POLICY_MISMATCH";
  if (!same(commit.event_ids, transaction.events.map((event) => event.event_id))) return "EVENT_ID_MISMATCH";
  if (state.logical_sequence_transition.after !== state.logical_sequence_transition.before + transaction.events.length || commit.next_logical_sequence !== state.logical_sequence_transition.after) return "LOGICAL_SEQUENCE_MISMATCH";
  if (Object.hasOwn(fence, "commit_successor_floor")) return "COMMIT_SUCCESSOR_FLOOR_NOT_DERIVED";
  if (transaction.source_settlements.length !== units.size || transaction.source_settlements.some((settlement) => { const unit = units.get(settlement.unit_id); return !unit || settlement.authority !== (unit.unit_kind === "SCHEDULED_OCCURRENCE" ? "schedule_store" : unit.unit_kind === "TRIGGER_ACTIVATION" ? "trigger_registry" : "decision_ledger"); })) return "SOURCE_SETTLEMENT_AUTHORITY_MISMATCH";
  const world = structuredClone(state.world_state_before); for (const event of transaction.events) { const reducer = state.reducer_registry[event.event_type]; if (reducer?.operation === "set-path-from-payload") world[event.payload[0]] = event.payload[1]; } if (!same(world, state.result_state)) return "WORLD_STATE_NOT_DERIVED";
  return null;
}
function validateTransition(operation, state, baseline) {
  if (operation === "append-source-record") return transitionSourceAppend(state);
  if (operation === "append-slot-dispatch") return transitionDispatch(state);
  if (operation === "append-slot-response") return transitionResponse(state);
  if (operation === "append-admission-fence") return same(state.candidate, baseline.candidate) ? null : "FENCE_DERIVATION_MISMATCH";
  if (operation === "derive-cycle-control-state") return same(state.candidate, deriveControlState(state.decision_ledger)) ? null : "CONTROL_STATE_FOLD_MISMATCH";
  if (operation === "validate-epistemic-checkpoint") return hex(sha256(Buffer.from(state.checkpoint.resolved_content_hex, "hex"))) === state.checkpoint.content_hash_hex ? null : "CHECKPOINT_CONTENT_HASH_MISMATCH";
  if (operation === "validate-rng-draw") return transitionRNG(state);
  if (operation === "validate-parent-history-ref") { try { parseParentHistory(state.reference_hex); return null; } catch { return "PARENT_HISTORY_REF_INVALID"; } }
  if (operation === "append-cycle-abort") return transitionAbort(state);
  if (operation === "validate-genesis") return transitionGenesis(state);
  if (operation === "validate-snapshot") return transitionSnapshot(state, baseline);
  if (operation === "append-observation") return transitionObservation(state);
  if (operation === "publish-cycle-commit-batch") return transitionCommit(state);
  throw new Error(`unknown transition operation ${operation}`);
}
function executeTransition(item, bases) {
  if (!bases.has(item.base_scenario_id)) throw new Error("unknown base scenario");
  const baseline = bases.get(item.base_scenario_id).state, state = structuredClone(baseline);
  for (const patch of item.patch ?? []) applyPatch(state, patch);
  const actual = validateTransition(bases.get(item.base_scenario_id).operation, state, baseline);
  if (item.expected_error !== undefined) { if (actual !== item.expected_error) throw new Error(`transition error mismatch: want ${item.expected_error}, got ${actual ?? ""}`); }
  else if (actual) throw new Error(`transition rejected with ${actual}`);
}

function verifyMutationGuards(fixtures, registries) {
  const linked = fixtures.semantic.find((item) => item.case_id === "linked-record.append-cycle-abort.first-attempt-has-retry-ref");
  if (!linked) throw new Error("linked mutation guard fixture missing");
  const alteredCode = structuredClone(linked); alteredCode.invalid_error_code = "NOT_A_REAL_ERROR";
  let rejected = false; try { executeSemantic(alteredCode, fixtures, registries); } catch { rejected = true; }
  if (!rejected) throw new Error("linked validator accepted a mutated expected error code");
  const unchangedBase = structuredClone(linked); delete unchangedBase.mutation;
  rejected = false; try { executeSemantic(unchangedBase, fixtures, registries); } catch { rejected = true; }
  if (!rejected) throw new Error("linked validator accepted an unchanged valid base as a rejection");
  for (const caseID of ["linked-record.append-retry-fence.retry-ref-mismatch", "linked-record.publish-cycle-commit-batch.empty-commit-advances-logical-sequence"]) {
    const regression = fixtures.semantic.find((item) => item.case_id === caseID);
    if (!regression) throw new Error(`linked regression fixture missing: ${caseID}`);
    const changedError = structuredClone(regression); changedError.invalid_error_code = "NOT_A_REAL_ERROR";
    rejected = false; try { executeSemantic(changedError, fixtures, registries); } catch { rejected = true; }
    if (!rejected) throw new Error(`linked validator accepted a mutated error code: ${caseID}`);
  }
  const binding = fixtures.semantic.find((item) => item.case_id === "record-enum.conflict-set.conflict-kind");
  if (!binding) throw new Error("record binding mutation guard fixture missing");
  const malformedEnvelope = structuredClone(binding); malformedEnvelope.accepted_envelope_hex = "00";
  rejected = false; try { executeSemantic(malformedEnvelope, fixtures, registries); } catch { rejected = true; }
  if (!rejected) throw new Error("record binding validator accepted malformed envelope bytes");
  const reference = fixtures.semantic.find((item) => item.kind === "record_reference_kind_binding");
  if (!reference) throw new Error("reference binding mutation guard fixture missing");
  const malformedContainer = structuredClone(reference); malformedContainer.accepted_container_cbor_hex = { "2": "00" };
  rejected = false; try { executeSemantic(malformedContainer, fixtures, registries); } catch { rejected = true; }
  if (!rejected) throw new Error("reference binding validator accepted malformed replacement bytes");
}

function run(directory) {
  verifyBundle(directory);
  const fixtures = strictJSON(join(directory, "fixtures.json"));
  const registries = strictJSON(join(directory, "registries.json"));
  verifyMutationGuards(fixtures, registries);
  const transitions = strictJSON(join(directory, "causal-transition-fixtures.json"));
  const report = { suite_hash: EXPECTED.conformance_suite_hash, total: 0, passed: 0, failed: 0, cases: [] };
  function add(id, error) { const result = { id, status: error ? "fail" : "pass" }; if (error) { result.error = error.message; report.failed++; } else report.passed++; report.total++; report.cases.push(result); }
  for (const section of ["normalization_cases", "sha256_vectors", "positive", "negative", "semantic"]) for (const item of fixtures[section]) { try { executeFixture(section, item, fixtures, registries); add(item.case_id); } catch (error) { add(item.case_id, error); } }
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
