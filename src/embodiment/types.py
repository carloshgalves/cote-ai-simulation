"""State types of the Embodiment subdomain — only what PSV1-1 uses.

This module grows one ticket at a time. `BodyState` arrives with PSV1-2,
`ExertionIntent`/`PerformanceOutcome` with PSV1-4, `SelfPhysicalModel` and
`CapacityBelief` with PSV1-6: a full type surface written before the dynamics
that exercise it is a layer without behaviour.

Two rules from `docs/architecture/physical-model.md` are structural here:

* **Every dimension carries a physical unit** (invariant 9). Cohort percentile is
  a *derived view* (`prior.percentile_of`), never storage. `pain_tolerance` is
  the single permitted exception, and it declares its reference cohort.
* **`CapacityProfile` is never derived from a performance** (invariant 1). There
  is no constructor, classmethod or coercion from an outcome to a profile, and
  none may be added.
"""

from __future__ import annotations

from collections.abc import Mapping
from enum import StrEnum
from types import MappingProxyType
from typing import Any, Self

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

__all__ = [
    "Dimension",
    "UnitSpec",
    "DIMENSION_UNITS",
    "DimensionValue",
    "CapacityProfile",
    "CapacityBaselineRecord",
    "RunMetadata",
    "MissingRunMetadataError",
    "SPEC_9_2_COMPONENTS",
    "Y1_START",
    "SCHEMA_VERSION",
    "SNAPSHOT_VERSION",
    "freeze_mapping",
]

SCHEMA_VERSION = 1
SNAPSHOT_VERSION = 1

#: The simulation instant `capacity_baseline` is sampled at (spec §5.3). The
#: logical clock of ADR 0003 is not in this ticket; the label is, because a
#: sample without an instant cannot be read later.
Y1_START = "Y1_START"


def freeze_mapping(value: Mapping[Any, Any]) -> Mapping[Any, Any]:
    """Copy a mapping into recursively immutable containers."""
    return MappingProxyType({key: _freeze_container(child) for key, child in value.items()})


def _freeze_container(value: Any) -> Any:
    if isinstance(value, Mapping):
        return freeze_mapping(value)
    if isinstance(value, (list, tuple)):
        return tuple(_freeze_container(child) for child in value)
    if isinstance(value, (set, frozenset)):
        return frozenset(_freeze_container(child) for child in value)
    return value


class Dimension(StrEnum):
    """The 15 capacity dimensions of `physical-model.md` §4.

    Kept in lockstep with `data/canon/schema/enums/capacity-dimensions.yaml`,
    which is the shared vocabulary and the authority; a test fails on drift.
    """

    AEROBIC_CAPACITY = "aerobic_capacity"
    ANAEROBIC_POWER = "anaerobic_power"
    SPRINT_SPEED = "sprint_speed"
    MAX_STRENGTH = "max_strength"
    STRENGTH_ENDURANCE = "strength_endurance"
    AGILITY = "agility"
    FLEXIBILITY = "flexibility"
    BODY_MASS = "body_mass"
    STATURE = "stature"
    REACTION_TIME = "reaction_time"
    COORDINATION = "coordination"
    INJURY_RESILIENCE = "injury_resilience"
    RECOVERY_RATE = "recovery_rate"
    THERMOREGULATION = "thermoregulation"
    PAIN_TOLERANCE = "pain_tolerance"


class UnitSpec(BaseModel):
    """The unit a dimension is stored in, and where that unit comes from.

    `battery_unit` differing from `unit` is allowed only where the enum file
    itself licenses it: an item scored in one unit that the engine consumes in
    another (`sprint_speed`), or an item offering two alternative instruments
    (`aerobic_capacity`, `anaerobic_power`), where we pick one and say which.
    """

    model_config = ConfigDict(frozen=True)

    unit: str
    battery_unit: str | None = None
    reference_cohort: str | None = None
    note: str = ""

    @property
    def is_percentile_only(self) -> bool:
        return self.unit == "percentile"

    @model_validator(mode="after")
    def _percentile_declares_cohort(self) -> UnitSpec:
        # Invariant 9: a dimension without a physical unit is permitted only when
        # no unit exists, and then it must declare the cohort it is a percentile
        # of. This is the door "0-100 arbitrary" would otherwise come back through.
        if self.is_percentile_only and not self.reference_cohort:
            raise ValueError("a percentile-only dimension must declare its reference cohort")
        return self


DIMENSION_UNITS: Mapping[Dimension, UnitSpec] = MappingProxyType(
    {
        Dimension.AEROBIC_CAPACITY: UnitSpec(
            unit="laps",
            battery_unit="laps | s",
            note="20mシャトルラン column chosen over 持久走: endurance-run distances per sex are unverified (SOURCING S3).",
        ),
        Dimension.ANAEROBIC_POWER: UnitSpec(
            unit="cm",
            battery_unit="cm | m",
            note="立ち幅とび (standing long jump) chosen as the stored indicator; ハンドボール投げ is the second indicator of the same dimension.",
        ),
        Dimension.SPRINT_SPEED: UnitSpec(
            unit="m_per_s",
            battery_unit="s",
            note="50m走 is scored in seconds; the enum file itself instructs conversion to m.s-1 for engine use, so higher is better here.",
        ),
        Dimension.MAX_STRENGTH: UnitSpec(unit="kg", battery_unit="kg", note="握力, grip dynamometer."),
        Dimension.STRENGTH_ENDURANCE: UnitSpec(unit="reps_30s", battery_unit="reps_30s", note="上体起こし."),
        Dimension.AGILITY: UnitSpec(unit="touches_20s", battery_unit="touches_20s", note="反復横とび: a count, not an elapsed time."),
        Dimension.FLEXIBILITY: UnitSpec(unit="cm", battery_unit="cm", note="長座体前屈."),
        Dimension.BODY_MASS: UnitSpec(unit="kg", battery_unit="kg", note="体格測定."),
        Dimension.STATURE: UnitSpec(unit="cm", battery_unit="cm", note="体格測定."),
        Dimension.REACTION_TIME: UnitSpec(unit="ms", note="No battery item. Response latency to a discrete cue; lower is better."),
        Dimension.COORDINATION: UnitSpec(unit="hit_rate", note="No battery item. Motor precision under time pressure, in [0, 1]."),
        Dimension.INJURY_RESILIENCE: UnitSpec(unit="risk_multiplier", note="No battery item. Dimensionless divisor of injury probability; higher is more robust."),
        Dimension.RECOVERY_RATE: UnitSpec(unit="h", note="No battery item. Time constant of fatigue dissipation; lower is faster recovery."),
        Dimension.THERMOREGULATION: UnitSpec(unit="degC", note="No battery item. Tolerated WBGT. Indexed to WBGT, never to age."),
        Dimension.PAIN_TOLERANCE: UnitSpec(
            unit="percentile",
            reference_cohort="same-age, same-sex secondary school cohort",
            note="No physical unit exists for this dimension. Percentile-only by exception, cohort declared.",
        ),
    }
)


class DimensionValue(BaseModel):
    """One capacity value in its physical unit. There is no percentile field."""

    model_config = ConfigDict(frozen=True)

    value: float
    unit: str

    @model_validator(mode="after")
    def _finite(self) -> DimensionValue:
        if self.value != self.value or self.value in (float("inf"), float("-inf")):
            raise ValueError("a capacity value must be finite")
        return self


class CapacityProfile(BaseModel):
    """What a rested body can do in neutral conditions at maximal effort.

    Structure 1 of the six of `physical-model.md` §2. Never observable directly,
    and never derived from structure 4 (`PerformanceOutcome`): reading backwards
    from performance to capacity is inference, and inference yields bounds.
    """

    model_config = ConfigDict(frozen=True)

    dimensions: Mapping[Dimension, DimensionValue]

    @field_validator("dimensions", mode="after")
    @classmethod
    def _complete_and_correctly_united(
        cls, dimensions: Mapping[Dimension, DimensionValue]
    ) -> Mapping[Dimension, DimensionValue]:
        missing = set(Dimension) - set(dimensions)
        if missing:
            raise ValueError(f"capacity profile missing dimensions: {sorted(d.value for d in missing)}")
        for dimension, entry in dimensions.items():
            expected = DIMENSION_UNITS[dimension].unit
            if entry.unit != expected:
                raise ValueError(
                    f"{dimension.value} must be stored in {expected!r}, got {entry.unit!r}"
                )
        return freeze_mapping(dimensions)

    def value(self, dimension: Dimension) -> float:
        return self.dimensions[dimension].value


class CapacityBaselineRecord(BaseModel):
    """The frozen sample: `capacity_baseline(Y1_START)` plus how it was drawn.

    Constructed in `seeding.py` and nowhere else (failure mode F3), immutable
    once drawn (F5). `evidence_sufficiency` travels with the sample because a
    posterior with sufficiency 0 is the prior under another name, and whoever
    reads the run must see that without reconstructing the inference.
    """

    model_config = ConfigDict(frozen=True)

    schema_version: int = SCHEMA_VERSION
    character_id: str
    sim_time: str = Y1_START
    profile: CapacityProfile
    posterior_hash: str
    prior_version: str
    substream: str
    evidence_sufficiency: Mapping[Dimension, float]

    def model_copy(
        self, *, update: Mapping[str, Any] | None = None, deep: bool = False
    ) -> Self:
        if update:
            raise TypeError(
                "CapacityBaselineRecord.model_copy does not accept update: "
                "capacity_baseline is created only by seeding.py"
            )
        return super().model_copy(deep=deep)

    @field_validator("evidence_sufficiency", mode="after")
    @classmethod
    def _sufficiency_per_dimension(cls, value: Mapping[Dimension, float]) -> Mapping[Dimension, float]:
        missing = set(Dimension) - set(value)
        if missing:
            raise ValueError(
                f"evidence_sufficiency missing dimensions: {sorted(d.value for d in missing)}"
            )
        for dimension, sufficiency in value.items():
            if not 0.0 <= sufficiency <= 1.0:
                raise ValueError(f"evidence_sufficiency[{dimension.value}] outside [0, 1]: {sufficiency}")
        return freeze_mapping(value)


class MissingRunMetadataError(RuntimeError):
    """Raised at run start when a mandatory §9.2 metadatum is absent.

    Names the missing field: a run that starts without it produces numbers whose
    meaning cannot be recovered afterwards.
    """

    def __init__(self, field: str) -> None:
        super().__init__(f"run metadata is missing a mandatory field: {field}")
        self.field = field


#: Components whose parameter version spec §9.2 makes mandatory, and the
#: metadata key each contributes. A run declares which components it engages and
#: `RunMetadata.require` refuses to start without their versions. The list is
#: complete from the start so a later ticket adding a component cannot quietly
#: skip its version: it appears here already, unengaged.
SPEC_9_2_COMPONENTS: Mapping[str, str] = MappingProxyType(
    {
        "prior": "prior_version",
        "estimator": "estimator_version",
        "dynamics": "dynamics_version",
        "contest_resolver": "contest_resolver_version",
        "observation_params": "observation_params_version",
    }
)


class RunMetadata(BaseModel):
    """Spec §9.2. Without these a snapshot is a pile of numbers with no meaning."""

    model_config = ConfigDict(frozen=True)

    world_seed: int
    component_versions: Mapping[str, str] = Field(default_factory=dict)
    posterior_hash_by_character: Mapping[str, str] = Field(default_factory=dict)
    estimator_ess_by_character: Mapping[str, float] = Field(default_factory=dict)
    evidence_sufficiency_by_character: Mapping[str, Mapping[Dimension, float]] = Field(default_factory=dict)

    @field_validator("component_versions", mode="after")
    @classmethod
    def _known_components(cls, value: Mapping[str, str]) -> Mapping[str, str]:
        unknown = set(value) - set(SPEC_9_2_COMPONENTS)
        if unknown:
            raise ValueError(f"unknown parameter component(s): {sorted(unknown)}")
        return freeze_mapping(value)

    @field_validator(
        "posterior_hash_by_character",
        "estimator_ess_by_character",
        "evidence_sufficiency_by_character",
        mode="after",
    )
    @classmethod
    def _freeze_provenance_mappings(cls, value: Mapping[Any, Any]) -> Mapping[Any, Any]:
        return freeze_mapping(value)

    @classmethod
    def from_mapping(cls, raw: Mapping[str, object], *, required_components: tuple[str, ...]) -> RunMetadata:
        """Build and validate at run start, failing on the first missing field."""
        for field in ("world_seed", "posterior_hash_by_character"):
            if raw.get(field) is None:
                raise MissingRunMetadataError(field)
        metadata = cls.model_validate(dict(raw))
        metadata.require(required_components)
        return metadata

    def require(self, components: tuple[str, ...]) -> None:
        for component in components:
            if component not in SPEC_9_2_COMPONENTS:
                raise ValueError(f"unknown parameter component: {component}")
            key = SPEC_9_2_COMPONENTS[component]
            if not self.component_versions.get(component):
                raise MissingRunMetadataError(key)
        if not self.posterior_hash_by_character:
            raise MissingRunMetadataError("posterior_hash_by_character")

    def as_dict(self) -> dict[str, object]:
        """Serialisable form, with §9.2's field names rather than component keys."""
        out: dict[str, object] = {"world_seed": self.world_seed}
        for component, key in SPEC_9_2_COMPONENTS.items():
            version = self.component_versions.get(component)
            if version:
                out[key] = version
        out["posterior_hash_by_character"] = dict(self.posterior_hash_by_character)
        if self.estimator_ess_by_character:
            out["estimator_ess_by_character"] = dict(self.estimator_ess_by_character)
        if self.evidence_sufficiency_by_character:
            out["evidence_sufficiency_by_character"] = {
                character: {dimension.value: sufficiency for dimension, sufficiency in per_dimension.items()}
                for character, per_dimension in self.evidence_sufficiency_by_character.items()
            }
        return out
