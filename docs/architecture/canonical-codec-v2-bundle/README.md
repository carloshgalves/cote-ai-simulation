# Canonical CSF Policy V2 — cross-policy fork

**Normative policy:** `cote.csf.codec.cbor-det.v2`; migration contract `cote.csf.migration.fork.v2`.

This version preserves the deterministic CBOR V1 binary algorithm, envelope schema and Unicode rules. It **changes the causal policy** by pinning an explicit versioned fork-migration protocol, and is therefore a distinct signed policy even where the byte encoder is unchanged. V1 artifacts preserved here are rechecked as compatibility vectors, not interpreted as new V2 Genesis fixtures. `fork-v2-vectors.json` is the V2 migration conformance contract.

The origin V1 checkpoint is always verified by its installed V1 bundle under the exact CSFHREF record. All five parent ledgers remain immutable historical archive, never live child authority. The child must have a V2 Genesis with the V2 codec-policy ID/hash and the signed schema manifest. Every carried pending Claim, Transmission and delivery ScheduledOccurrence must be rebuilt under new causal identifiers and linked to an authenticated migration birth. There is no receipt of a parent send event as though it occurred in the child. No worker-selected migration projection may bypass the child terminal proof.

**Safety gate:** a migration is executable only after the corresponding child replay verifier and transaction-wide rollback tests accept the new migration-birth authority. A signed bundle alone does not make an incomplete migrator safe. Unsupported states (revoked dispatches, non-Genesis triggers, cognition, in-flight fences, unknown remote policy) fail closed.

Hash bootstrap uses SHA-256 of `ASCII(domain)||0x00||manifest_bytes`, under V2 domains `cote.csf.bundle.codec-policy.v2`, `cote.csf.bundle.schema.v2`, and `cote.csf.bundle.conformance.v2`. All artifacts must match the signed size/hash tuple; receipt lines bind the resulting three manifest digests.
