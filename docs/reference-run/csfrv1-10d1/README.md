# CSFRV1-10 / D1 — composable stock and physical WorldState

**Issue:** #134, first child of #118 / epic #114. **Status:** Go reducer + Genesis compatibility layer only. D2 #135 must actually execute supervised Python A1 on the same B+C causal run. D3 #136 owns integrated ENGINE_ONLY disclosure, Observatory/public POV, replay/fault matrices and issue #118 technical acceptance.

## Actual architectural blocker removed

The **B+C4 reference** uses one authenticated world-state typed envelope whose inner variant is `testresource.stock.v1`. The **existing A1 Go physical commit** uses a different inner variant, `physical.body-state.v1`. Both are valid separately, but no single state's decoder could previously preserve both. A second SQLite database or separate RunID would falsify the requirement of #118.

D1 introduces **`reference.stock-and-physical.v1`**, under the *existing registered typed-value schema and world-state domain tag*, with one fixed canonical nested value:

```text
outer typed-value: [schema typed-value, version 1, canonical envelope, digest]
inner payload: [
  "reference.stock-and-physical.v1",
  1,
  [ exact authenticated stock typed-value, exact authenticated body typed-value ]
]
```

The schema is closed to **exactly two** ordered components. Each nested component must be a standalone, independently authenticated `StockVariant` or `PhysicalVariant` with its own valid domain tag, schema, version, canonical digest, semantic constraints and no fields added or removed. Composite/nested recursive values, duplicate branch kinds, swapped stock/physical order, unknown versions and digest substitution all fail closed.

**Authority:** The pure Go `EncodeComposite` constructor is not an A1 admission capability or production schema install. A Genesis containing the composite variant is valid **only when the installed policy bundle already has a genuine `PhysicalRegistration`** (manifest hash, entrypoint, complete import-tree digest and producer signing public key pinned in Genesis). The legacy stock-only Genesis remains valid *without* a physical extension; a physical-only Genesis remains valid with it. No fallback converts stock-only into a physical run.

## Reducer consequences

- `DecodeStock` projects **only** the inventory from a verified composite; `DecodePhysical` projects **only** the body vector. Neither accepts a malformed composite.
- `Consume` deterministically modifies only stock while recreating the composed typed envelope with the identical authenticated physical substate. It never creates a BodyState observation or modifies any body value.
- `ApplyCompletedPhysical` enforces existing monotonic per-subject CAS, refuses stale/unknown subjects and recreates the composed state with the identical authenticated resource substate. Two physical epochs retain all committed body changes and cannot replenish a consumed inventory unit.
- Standalone reducer behavior remains exactly as before; no changes to the actual A1 signed cut, authority of Go CommitCoordinator, Event ENGINE_ONLY or global clock ownership are introduced in D1.

## Reproduce the true RED/GREEN sequence

```bash
go test -count=1 -v ./internal/csf/reducer ./internal/csf/genesis \
  -run '^(TestComposite|TestGenesisComposite)'
go test -count=1 ./internal/csf/reducer ./internal/csf/genesis ./internal/csf/policy
go test -count=1 ./internal/csf/commit -run '^(TestReference|TestPhysical)'
go test -race -count=1 ./internal/csf/reducer ./internal/csf/genesis \
  -run '^(TestComposite|TestGenesisComposite)'
```

[Recorded RED workflow #43934913498](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/43934913498) demonstrates undefined `EncodeComposite`, `DecodeComposite` and `CompositeVariant` **before** implementation. The GREEN CI on the exact PR HEAD must be checked separately.

## Security negatives

1. Missing/swapped/duplicate/extra nested branches or variants/versions: reject.
2. Malformed nested typed records, forged outer/inner content digest: reject.
3. Registered physical extension missing from Genesis, wrong manifest hash, missing physical Source ID, unregistered schema: reject.
4. Resource `Consume` must not change physical typed bytes, body update must not change resource typed bytes.
5. Resource double-spend and stale physical epoch fail closed.
6. Reversed subject order produces byte-identical canonical state.
7. Existing standalone B/C4 and physical test suites must remain green.

**NO-GO:** This is only a structural precondition to D2. It does NOT claim actual supervised Python execution, two epochs in the B+C RunID, complete dependency pinning, strong same-UID isolation, original physical producer completion, or public/POV ENGINE_ONLY guarantees. External #94/#113 gates remain open. PR merge is human-only.
