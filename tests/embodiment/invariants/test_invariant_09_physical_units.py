"""Invariant 9 — dimensions carry physical units; percentile is a derived view.

`physical-model.md` §15.9 and §4. The exception is explicit and singular:
`pain_tolerance` has no physical unit and therefore declares its reference
cohort. Without that clause, "0-100 arbitrary" comes back through the exception.

This file also holds the drift check against the canon enum, because the enum is
the authority on which dimensions exist and the engine may not quietly disagree
with it.
"""

from __future__ import annotations

import yaml

from embodiment.types import DIMENSION_UNITS, CapacityProfile, Dimension, DimensionValue

CANON_ENUM = "data/canon/schema/enums/capacity-dimensions.yaml"


def test_invariant_09_every_dimension_declares_a_unit() -> None:
    for dimension in Dimension:
        spec = DIMENSION_UNITS[dimension]
        assert spec.unit, dimension.value


def test_invariant_09_the_only_unitless_dimension_declares_its_cohort() -> None:
    percentile_only = [
        dimension for dimension in Dimension if DIMENSION_UNITS[dimension].is_percentile_only
    ]
    assert percentile_only == [Dimension.PAIN_TOLERANCE]
    assert DIMENSION_UNITS[Dimension.PAIN_TOLERANCE].reference_cohort


def test_invariant_09_percentile_is_never_stored() -> None:
    assert "percentile" not in DimensionValue.model_fields
    assert set(DimensionValue.model_fields) == {"value", "unit"}
    assert set(CapacityProfile.model_fields) == {"dimensions"}


def test_invariant_09_a_value_in_the_wrong_unit_is_refused(prior) -> None:
    from embodiment.rng import substream

    profile = prior.sample_profile(substream(1, "npc.0001", "e", "p"), "male")
    dimensions = dict(profile.dimensions)
    dimensions[Dimension.MAX_STRENGTH] = DimensionValue(value=40.0, unit="newtons")
    try:
        CapacityProfile(dimensions=dimensions)
    except ValueError as error:
        assert "kg" in str(error)
    else:  # pragma: no cover
        raise AssertionError("a dimension in the wrong unit must be refused")


def test_invariant_09_the_engine_does_not_drift_from_the_canon_enum(repo_root) -> None:
    """The enum file is the shared vocabulary; `types.py` may not disagree with it."""
    enum = yaml.safe_load((repo_root / CANON_ENUM).read_text(encoding="utf-8"))
    declared = {entry["id"]: entry for entry in enum["dimensions"]}
    assert set(declared) == {dimension.value for dimension in Dimension}

    for dimension in Dimension:
        spec = DIMENSION_UNITS[dimension]
        canon_unit = str(declared[dimension.value]["unit"])
        alternatives = {part.strip() for part in canon_unit.split("|")}
        if spec.unit in alternatives:
            continue
        # A stored unit that differs from the instrument's is allowed only where
        # the enum file itself instructs the conversion, and only if the spec
        # records which instrument unit it came from.
        assert spec.battery_unit == canon_unit, dimension.value
        assert "convert" in str(declared[dimension.value].get("anchor", "")).lower(), dimension.value


def test_invariant_09_the_reference_cohort_matches_the_enum(repo_root) -> None:
    enum = yaml.safe_load((repo_root / CANON_ENUM).read_text(encoding="utf-8"))
    declared = {entry["id"]: entry for entry in enum["dimensions"]}
    assert (
        DIMENSION_UNITS[Dimension.PAIN_TOLERANCE].reference_cohort
        == declared["pain_tolerance"]["reference_cohort"]
    )
