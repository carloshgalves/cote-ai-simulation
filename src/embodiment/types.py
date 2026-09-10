"""State types of the Embodiment subdomain — only what PSV1-1 uses.

This module grows one ticket at a time. `BodyState` and `Injury` arrive with
PSV1-2, `ExertionIntent`/`PerformanceOutcome` with PSV1-4, `SelfPhysicalModel`
and `CapacityBelief` with PSV1-6: a full type surface written before the dynamics
that exercise it is a layer without behaviour.

Two more rules arrive with `BodyState`:

* **The body is world truth and carries no belief** (invariant 1). Nothing here
  is what a character thinks about their body; interoception is PSV1-6 and lives
  in another tree.
* **Domains are enforced by the type, not by the caller** (P3, failure mode
  F15). Hydration cannot go negative, `w_prime` cannot exceed its capacity,
  healing progress cannot leave `0..1` and severity cannot leave its enum,
  because the model refuses to be constructed that way.

Two rules from `docs/architecture/physical-model.md` are structural here:

* **Every dimension carries a physical unit** (invariant 9). Cohort percentile is
  a *derived view* (`prior.percentile_of`), never storage. `pain_tolerance` is
  the single permitted exception, and it declares its reference cohort.
* **`CapacityProfile` is never derived from a performance** (invariant 1). There
  is no constructor, classmethod or coercion from an outcome to a profile, and
  none may be added.
"""

from __future__ import annotations

import re
from collections.abc import Mapping
from enum import Enum, IntEnum, StrEnum
from types import MappingProxyType
from typing import Any, Literal, Self

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

__all__ = [
    "Dimension",
    "DimensionOrientation",
    "Sex",
    "SexSource",
    "UnitSpec",
    "DIMENSION_UNITS",
    "DimensionValue",
    "CapacityProfile",
    "CapacityBaselineRecord",
    "RunMetadata",
    "MissingRunMetadataError",
    "SPEC_9_2_COMPONENTS",
    "FatigueRegion",
    "InjuryRegion",
    "InjuryTissue",
    "InjuryMechanism",
    "InjurySeverity",
    "SleepQuality",
    "SorenessOnset",
    "LastSleep",
    "WPrimeBalance",
    "Sleep",
    "Energy",
    "Hydration",
    "Thermal",
    "RegionSoreness",
    "Soreness",
    "Healing",
    "PainProfile",
    "Concealment",
    "Injury",
    "Illness",
    "Pain",
    "CumulativeLoad",
    "BodyState",
    "Y1_START",
    "SCHEMA_VERSION",
    "SNAPSHOT_VERSION",
    "freeze_mapping",
    "as_document",
]

SCHEMA_VERSION = 1
SNAPSHOT_VERSION = 1
_SHA256_RE = re.compile(r"^[0-9a-f]{64}$")

#: The simulation instant `capacity_baseline` is sampled at (spec §5.3). The
#: logical clock of ADR 0003 is not in this ticket; the label is, because a
#: sample without an instant cannot be read later.
Y1_START = "Y1_START"

#: Which cohort marginals a body was drawn from. A covariate, never a capacity:
#: it selects the distribution, and no dimension of the profile is named by it.
Sex = Literal["male", "female"]

#: Where that covariate came from. `KNOWN` is canon establishing it; `DRAWN` is
#: the prior file's `[INT]` cohort ratio standing in for a source that is silent.
#: The two produce the same distribution, so nothing but this field distinguishes
#: a sourced fact from an assumption once the body is sampled.
SexSource = Literal["KNOWN", "DRAWN"]


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


#: Which way a dimension reads. `reaction_time` is in ms and `recovery_rate` is
#: a time constant in hours, so for both a *smaller* number is a better body;
#: everything else stored here reads upwards. It is structural rather than a
#: note because `capability.py` composes degradation multiplicatively, and a
#: degraded `reaction_time` has to get *longer*: an orientation kept only in
#: prose is an orientation that gets read backwards exactly once.
DimensionOrientation = Literal["HIGHER_IS_BETTER", "LOWER_IS_BETTER"]


class UnitSpec(BaseModel):
    """The unit a dimension is stored in, and where that unit comes from.

    `battery_unit` differing from `unit` is allowed only where the enum file
    itself licenses it: an item scored in one unit that the engine consumes in
    another (`sprint_speed`), or an item offering two alternative instruments
    (`aerobic_capacity`, `anaerobic_power`), where we pick one and say which.
    """

    model_config = ConfigDict(frozen=True)

    unit: str
    orientation: DimensionOrientation = "HIGHER_IS_BETTER"
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
        Dimension.REACTION_TIME: UnitSpec(unit="ms", orientation="LOWER_IS_BETTER", note="No battery item. Response latency to a discrete cue; lower is better."),
        Dimension.COORDINATION: UnitSpec(unit="hit_rate", note="No battery item. Motor precision under time pressure, in [0, 1]."),
        Dimension.INJURY_RESILIENCE: UnitSpec(unit="risk_multiplier", note="No battery item. Dimensionless divisor of injury probability; higher is more robust."),
        Dimension.RECOVERY_RATE: UnitSpec(unit="h", orientation="LOWER_IS_BETTER", note="No battery item. Time constant of fatigue dissipation; lower is faster recovery."),
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
    #: The covariate the copula read, and whether canon fixed it or the cohort
    #: ratio drew it. It travels on the frozen record rather than being looked up
    #: later because `posterior_hash` deliberately does not distinguish the two:
    #: conditioning on a known male and drawing a male are the same distribution.
    #: Without it here, a posterior rebuilt with the same hash but the other
    #: provenance is indistinguishable from the one that actually ran, and a
    #: snapshot can present an `[INT]` assumption as a sourced fact.
    cohort_sex: Sex
    cohort_sex_source: SexSource
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

    def copy(self, **kwargs: Any) -> Self:
        """Pydantic 1's `copy`, held to the same rule as `model_copy`.

        Pydantic 2 still exposes it, and deprecating a method is not the same as
        closing it: it emits a warning and returns the altered record anyway. The
        single-writer rule is about the type, not about one spelling of it, so
        every copy API the type offers refuses `update`.
        """
        if kwargs.get("update"):
            raise TypeError(
                "CapacityBaselineRecord.copy does not accept update: "
                "capacity_baseline is created only by seeding.py"
            )
        return super().copy(**kwargs)  # type: ignore[deprecated]

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

    @field_validator("posterior_hash_by_character", mode="after")
    @classmethod
    def _valid_posterior_identities(cls, value: Mapping[str, str]) -> Mapping[str, str]:
        for character_id, digest in value.items():
            if not character_id.strip():
                raise ValueError(
                    "posterior_hash_by_character contains an empty character id"
                )
            if not _SHA256_RE.fullmatch(digest):
                raise ValueError(
                    f"posterior_hash_by_character[{character_id!r}] must be a lowercase "
                    "sha256 digest"
                )
        return value

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

    @classmethod
    def from_document(cls, document: Mapping[str, Any]) -> RunMetadata:
        """The inverse of `as_dict`: read run metadata back off a snapshot or log.

        `as_dict` writes spec §9.2's field names (`prior_version`), while the
        model keys versions by component (`prior`). A later phase of a run has to
        declare the same metadata the run opened with, so the mapping has to go
        both ways or the two ends drift.
        """
        by_key = {key: component for component, key in SPEC_9_2_COMPONENTS.items()}
        raw: dict[str, Any] = {
            "world_seed": document.get("world_seed"),
            "component_versions": {
                by_key[key]: value for key, value in document.items() if key in by_key
            },
        }
        for field in (
            "posterior_hash_by_character",
            "estimator_ess_by_character",
            "evidence_sufficiency_by_character",
        ):
            if document.get(field) is not None:
                raw[field] = document[field]
        if raw["world_seed"] is None:
            raise MissingRunMetadataError("world_seed")
        return cls.model_validate(raw)

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


# --------------------------------------------------------------------------
# `BodyState` — physical-model.md §7. World truth, written only by the engine.
# --------------------------------------------------------------------------
#
# Everything below is frozen and validated at construction, which is where P3
# (state validity, failure mode F15) is enforced: a body outside its domains
# cannot be built, so no sequence of operations can produce one. The dynamics
# clamp before they construct, and the type is what makes that a rule rather
# than a habit.


class FatigueRegion(StrEnum):
    """The four reservoirs `peripheral_fatigue` is kept in (spec §13.2 (a)).

    Coarser than `InjuryRegion` on purpose, and the asymmetry is documented in
    `data/models/physical/body-dynamics.yaml`, which also carries the total map
    between the two.
    """

    LEGS = "legs"
    ARMS = "arms"
    GRIP = "grip"
    CORE = "core"


class InjuryRegion(StrEnum):
    """Where a lesion is. Fine, because a wrist and a hand decide different exams."""

    HEAD = "head"
    NECK = "neck"
    SHOULDER_LEFT = "shoulder_left"
    SHOULDER_RIGHT = "shoulder_right"
    UPPER_ARM_LEFT = "upper_arm_left"
    UPPER_ARM_RIGHT = "upper_arm_right"
    ELBOW_LEFT = "elbow_left"
    ELBOW_RIGHT = "elbow_right"
    FOREARM_LEFT = "forearm_left"
    FOREARM_RIGHT = "forearm_right"
    WRIST_LEFT = "wrist_left"
    WRIST_RIGHT = "wrist_right"
    HAND_LEFT = "hand_left"
    HAND_RIGHT = "hand_right"
    FINGER_LEFT = "finger_left"
    FINGER_RIGHT = "finger_right"
    CHEST = "chest"
    UPPER_BACK = "upper_back"
    LOWER_BACK = "lower_back"
    ABDOMEN = "abdomen"
    GROIN = "groin"
    HIP_LEFT = "hip_left"
    HIP_RIGHT = "hip_right"
    THIGH_LEFT = "thigh_left"
    THIGH_RIGHT = "thigh_right"
    KNEE_LEFT = "knee_left"
    KNEE_RIGHT = "knee_right"
    SHIN_LEFT = "shin_left"
    SHIN_RIGHT = "shin_right"
    CALF_LEFT = "calf_left"
    CALF_RIGHT = "calf_right"
    ANKLE_LEFT = "ankle_left"
    ANKLE_RIGHT = "ankle_right"
    FOOT_LEFT = "foot_left"
    FOOT_RIGHT = "foot_right"
    TOE_LEFT = "toe_left"
    TOE_RIGHT = "toe_right"


class InjuryTissue(StrEnum):
    MUSCLE = "muscle"
    LIGAMENT = "ligament"
    TENDON = "tendon"
    BONE = "bone"
    JOINT = "joint"
    SKIN = "skin"
    HEAD = "head"


class InjuryMechanism(StrEnum):
    OVERUSE = "overuse"
    IMPACT = "impact"
    TWIST = "twist"
    LACERATION = "laceration"
    STRAIN = "strain"


class InjurySeverity(IntEnum):
    """`physical-model.md` §7 writes severity as `0..3`; the enum names them.

    A severity outside the enum is refused by construction, which is half of P3.
    """

    NEGLIGIBLE = 0
    MINOR = 1
    MODERATE = 2
    SEVERE = 3


class SleepQuality(StrEnum):
    GOOD = "good"
    FAIR = "fair"
    POOR = "poor"


class _ValidatedStateModel(BaseModel):
    """A physical-state value whose copy APIs preserve its declared domains.

    Pydantic's default ``model_copy(update=...)`` deliberately skips validation.
    That is useful for ordinary data-transfer models and unsafe for authoritative
    world truth: ``frozen=True`` alone would still permit a caller to manufacture
    an invalid body. Rebuild from plain data so nested forged models are checked
    again as well as the field being updated.
    """

    def model_copy(
        self, *, update: Mapping[str, Any] | None = None, deep: bool = False
    ) -> Self:
        if update is None:
            return super().model_copy(deep=deep)
        document = as_document(self)
        document.update({name: as_document(value) for name, value in update.items()})
        return type(self).model_validate(document)

    def copy(
        self,
        *,
        include: Any = None,
        exclude: Any = None,
        update: Mapping[str, Any] | None = None,
        deep: bool = False,
    ) -> Self:
        """Close Pydantic 1's unchecked compatibility copy path too."""
        if include is not None or exclude is not None:
            raise TypeError(
                "physical state cannot be copied with fields omitted; rebuild a valid model"
            )
        return self.model_copy(update=update, deep=deep)


class LastSleep(_ValidatedStateModel):
    """The sleep episode that just ended, in hours since `Y1_START`."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    start_h: float
    end_h: float
    quality: SleepQuality

    @model_validator(mode="after")
    def _ends_after_it_starts(self) -> LastSleep:
        if self.end_h < self.start_h:
            raise ValueError("a sleep episode cannot end before it starts")
        return self


class WPrimeBalance(_ValidatedStateModel):
    """Work available above critical power, and how fast it comes back.

    `tau_s` is recorded rather than assumed because it is not a constant: the
    reconstitution time constant is *larger* the harder the recovery intensity
    (~377 s at 20 W, ~580 s in the heavy domain), and a body that recorded a
    single tau would have lost the one counter-intuitive fact about this channel.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    remaining_j: float = Field(ge=0.0)
    capacity_j: float = Field(gt=0.0)
    tau_s: float = Field(gt=0.0)

    @model_validator(mode="after")
    def _reserve_within_capacity(self) -> WPrimeBalance:
        if self.remaining_j > self.capacity_j:
            raise ValueError(
                f"w_prime remaining_j {self.remaining_j} exceeds capacity_j {self.capacity_j}"
            )
        return self

    @property
    def fraction(self) -> float:
        return self.remaining_j / self.capacity_j


class Sleep(_ValidatedStateModel):
    """The two-process model's state: homeostatic pressure and circadian phase."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    debt_hours: float = Field(ge=0.0)
    hours_since_wake: float = Field(ge=0.0)
    #: Hour of the local day, `[0, 24)`. It advances with the clock whatever the
    #: body does, which is why it is state and not a parameter.
    circadian_phase: float = Field(ge=0.0, lt=24.0)
    last_sleep: LastSleep | None = None


class Energy(_ValidatedStateModel):
    """Substrate is the channel that makes a multi-day exam a multi-day exam.

    `balance_kcal_24h` is accounting: it is written and, in V1, read by nobody.
    It is not the dead accounting the tests police, though — that is
    `cumulative_load`, which has a named failure mode attached (F14).
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    substrate_availability: float = Field(ge=0.0, le=1.0)
    balance_kcal_24h: float = 0.0
    last_meal_at_h: float | None = None


class Hydration(_ValidatedStateModel):
    """Deficit as a percentage of body mass. Below ~2 % it costs nothing."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    deficit_pct_body_mass: float = Field(ge=0.0)


class Thermal(_ValidatedStateModel):
    """Heat load, indexed to WBGT and to modifiable factors — never to age (F16).

    `wbgt`, `work_rest_ratio` and `clothing_insulation` are the environment the
    last advance applied, recorded so the log and the snapshot can explain the
    `core_offset_c` next to them. They are a receipt, not an input: the
    authoritative environment is the one handed to `dynamics.advance`.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    core_offset_c: float = Field(ge=0.0)
    wbgt: float
    work_rest_ratio: float = Field(gt=0.0)
    clothing_insulation: float = Field(ge=0.0)


class SorenessOnset(_ValidatedStateModel):
    """Damage held until the causal DOMS onset window opens."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    release_at_h: float = Field(ge=0.0)
    amount: float = Field(gt=0.0)


class RegionSoreness(_ValidatedStateModel):
    """One region's DOMS: damage still on its way, and damage being felt.

    The latency stages are why soreness cannot appear during the effort that
    caused it (failure mode F11). Damage enters the first stage, walks the chain,
    and only then becomes `expressed`.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    latency: tuple[float, ...] = ()
    pending_onset: tuple[SorenessOnset, ...] = ()
    expressed: float = Field(default=0.0, ge=0.0, le=1.0)

    @field_validator("latency", mode="after")
    @classmethod
    def _stages_are_non_negative(cls, value: tuple[float, ...]) -> tuple[float, ...]:
        for stage in value:
            if stage < 0.0:
                raise ValueError("a soreness latency stage cannot hold negative damage")
        return value


class Soreness(_ValidatedStateModel):
    """DOMS by region, plus the repeated-bout flag that stops it repeating forever."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    by_region: Mapping[FatigueRegion, RegionSoreness] = Field(default_factory=dict)
    #: Keyed `"<region>|<activity>"`. Per region *and* per activity, because the
    #: adaptation is specific to both: squats do not protect a forearm.
    repeated_bout_adaptation: Mapping[str, float] = Field(default_factory=dict)

    @field_validator("by_region", mode="after")
    @classmethod
    def _freeze_regions(
        cls, value: Mapping[FatigueRegion, RegionSoreness]
    ) -> Mapping[FatigueRegion, RegionSoreness]:
        return freeze_mapping(value)

    @field_validator("repeated_bout_adaptation", mode="after")
    @classmethod
    def _adaptation_is_a_fraction(cls, value: Mapping[str, float]) -> Mapping[str, float]:
        for key, adaptation in value.items():
            if not 0.0 <= adaptation <= 1.0:
                raise ValueError(f"repeated_bout_adaptation[{key}] outside [0, 1]: {adaptation}")
        return freeze_mapping(value)


class Healing(_ValidatedStateModel):
    """How far along an injury is, and what loading it costs."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    expected_days: float = Field(gt=0.0)
    progress: float = Field(ge=0.0, le=1.0)
    #: Fraction of progress lost per unit of load dose on the region while the
    #: lesion is open. `physical-model.md` §7 calls this `setback_on_load`.
    setback_on_load: float = Field(default=0.0, ge=0.0, le=1.0)


class PainProfile(_ValidatedStateModel):
    model_config = ConfigDict(frozen=True, extra="forbid")

    at_rest: float = Field(default=0.0, ge=0.0, le=1.0)
    on_use: float = Field(default=0.0, ge=0.0, le=1.0)


class Concealment(_ValidatedStateModel):
    """Hiding an injury is an intention with a price, not an adjective.

    Nothing in PSV1-2 writes this: concealment is resolved in PSV1-4 and leaks
    in PSV1-5. The field exists because the lesion it belongs to is persisted
    here, and a body that healed while its concealment lived elsewhere would be
    two structures pretending to be one.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    attempted: bool = False
    central_fatigue_cost: float = Field(default=0.0, ge=0.0)
    leak_probability: float = Field(default=0.0, ge=0.0, le=1.0)


class Injury(_ValidatedStateModel):
    """A lesion, with impairments **per dimension** (failure mode F12).

    An `impairments` map that touches every dimension by the same factor is the
    bug this type exists to make visible: a compromised wrist takes down grip and
    throwing and leaves sprinting almost untouched, and that is what lets an exam
    of grip strength be decided by an injury nobody narrated.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    region: InjuryRegion
    tissue: InjuryTissue
    mechanism: InjuryMechanism
    severity: InjurySeverity
    onset_h: float
    healing: Healing
    #: Multiplier per dimension, in `(0, 1]`. Absent means untouched.
    impairments: Mapping[Dimension, float] = Field(default_factory=dict)
    pain_profile: PainProfile = PainProfile()
    observable_cues: tuple[str, ...] = ()
    concealment: Concealment = Concealment()
    treated: bool = False

    @field_validator("impairments", mode="after")
    @classmethod
    def _impairments_are_multipliers(
        cls, value: Mapping[Dimension, float]
    ) -> Mapping[Dimension, float]:
        for dimension, multiplier in value.items():
            if not 0.0 < multiplier <= 1.0:
                raise ValueError(
                    f"impairments[{dimension.value}] must lie in (0, 1]: {multiplier}"
                )
        if len(value) == len(Dimension) and len(set(value.values())) == 1:
            raise ValueError(
                "an impairment that applies equally to every dimension is failure mode F12, "
                "not a simplification: a lesion restricts specific dimensions"
            )
        return freeze_mapping(value)


class Illness(_ValidatedStateModel):
    """Serialised, and never written in V1 (spec §13.5, decided 2026-09-09).

    The field exists so that `snapshot_version: 1` already has room for it when a
    survival exam needs it; no V1 ticket updates it and a test fails if any module
    writes one. Its absence of dynamics is a decision, not a bug to be found later.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    label: str
    onset_h: float
    severity: InjurySeverity = InjurySeverity.MINOR


class Pain(_ValidatedStateModel):
    """Pain is a signal. It dissociates from lesion in both directions."""

    model_config = ConfigDict(frozen=True, extra="forbid")

    by_region: Mapping[FatigueRegion, float] = Field(default_factory=dict)
    global_intensity: float = Field(default=0.0, ge=0.0, le=1.0)
    #: Adrenaline or medication: suppresses the signal without healing tissue.
    analgesia: float = Field(default=0.0, ge=0.0, le=1.0)

    @field_validator("by_region", mode="after")
    @classmethod
    def _regional_pain_is_a_fraction(
        cls, value: Mapping[FatigueRegion, float]
    ) -> Mapping[FatigueRegion, float]:
        for region, intensity in value.items():
            if not 0.0 <= intensity <= 1.0:
                raise ValueError(f"pain.by_region[{region.value}] outside [0, 1]: {intensity}")
        return freeze_mapping(value)


class CumulativeLoad(_ValidatedStateModel):
    """Load accounting for capacity drift, which V1 does not implement.

    Both fields are updated and **read by nobody** (spec §3.2). The ratio between
    them is not a risk factor and must not become one: it is mathematically
    coupled, unstable at low chronic load, unsupported as a causal factor, and the
    figure that popularised it is the subject of a formal retraction request
    (failure mode F14). A test fails if any module reads either field.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    acute_7d: float = Field(default=0.0, ge=0.0)
    chronic_28d: float = Field(default=0.0, ge=0.0)


class BodyState(_ValidatedStateModel):
    """The condition a body is in right now. Structure 2 of the six of §2.

    World truth, owned by the engine, indexed by simulation time, and present in
    the snapshot. It is **not** what the character believes about their body:
    that is `SelfPhysicalModel`, it is belief, it is biased, and it arrives with
    PSV1-6 in a different tree.

    The persistence invariant is the whole point of the type: **a scene heals
    nothing**. Only the clock moving, under the recovery rules, changes any of
    these fields. A badly slept Monday night is still in the body on Wednesday.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    character_id: str
    #: Hours since `Y1_START`. The logical clock of ADR 0003 is not in this
    #: subdomain; what the body needs is an ordered, additive instant, and hours
    #: since the world's origin is the smallest thing that is one.
    t_hours: float = Field(ge=0.0)

    w_prime_balance: WPrimeBalance
    peripheral_fatigue: Mapping[FatigueRegion, float]
    central_fatigue: float = Field(ge=0.0, le=1.0)
    cumulative_load: CumulativeLoad = CumulativeLoad()
    sleep: Sleep
    energy: Energy
    hydration: Hydration
    thermal: Thermal
    soreness: Soreness = Soreness()
    injuries: tuple[Injury, ...] = ()
    #: Serialised, never written in V1. See `Illness`.
    illnesses: tuple[Illness, ...] = ()
    pain: Pain = Pain()

    @field_validator("peripheral_fatigue", mode="after")
    @classmethod
    def _every_region_present_and_bounded(
        cls, value: Mapping[FatigueRegion, float]
    ) -> Mapping[FatigueRegion, float]:
        missing = set(FatigueRegion) - set(value)
        if missing:
            raise ValueError(
                f"peripheral_fatigue missing region(s): {sorted(r.value for r in missing)}"
            )
        for region, fatigue in value.items():
            if not 0.0 <= fatigue <= 1.0:
                raise ValueError(
                    f"peripheral_fatigue[{region.value}] outside [0, 1]: {fatigue}"
                )
        return freeze_mapping(value)

    def open_injuries(self) -> tuple[Injury, ...]:
        """Injuries still impairing something. Healed ones leave the list."""
        return tuple(injury for injury in self.injuries if injury.healing.progress < 1.0)


def as_document(value: Any) -> Any:
    """Plain JSON-compatible data from a state model, walked by hand.

    `model_dump` is not used because these models freeze their mappings into
    `MappingProxyType` — world truth that cannot be edited in place — and
    pydantic's serialiser does not know that type. Walking the declared fields
    also keeps the float rounding in one place: a value that differs in the
    sixteenth decimal is not a change anyone wants to read about in a log.
    """
    if isinstance(value, BaseModel):
        return {name: as_document(getattr(value, name)) for name in type(value).model_fields}
    if isinstance(value, Enum):
        return as_document(value.value)
    if isinstance(value, Mapping):
        return {str(key): as_document(item) for key, item in value.items()}
    if isinstance(value, (list, tuple)):
        return [as_document(item) for item in value]
    if isinstance(value, bool):
        return value
    if isinstance(value, float):
        return round(value, 9)
    return value
