"""The nine dynamic channels of `physical-model.md` §7, as pure functions.

This module is the whole of the persistence invariant. Two rules shape every
signature in it:

**A scene heals nothing.** Only the clock moving changes a body. There is no
function here that alters `BodyState` without an explicit `dt_hours`, and
`advance(state, 0.0, ...)` returns the state it was given, unchanged, object for
object. A badly slept Monday night is still in the body on Wednesday because
nothing but Tuesday can take it out.

**One writer.** `advance` is the only way a body changes, and only the engine
calls it. A test of architecture fails if any other module in the package writes
a body.

The nine channels have separate time scales because a single "stamina" reservoir
represents none of the stress scenarios: sleep deprivation degrades precision
without degrading strength, dehydration takes the aerobic dimension first, and
DOMS arrives a day after the effort that caused it. They are composed in a fixed
order over a fixed integration step, so an interval that is a whole number of
steps decomposes exactly: 24 h in one call equals 12 h twice.

`load` is the exogenous input — what the body did during the interval. Computing
it from an `ExertionIntent` is the effort resolver's job (PSV1-4); this module
defines only how a body responds to it over time. Everything here is
deterministic: no draw, no RNG, no substream. The injury roll is PSV1-4's, and
its absence here is the reason this module needs no randomness at all.

Parameters live in `data/models/physical/body-dynamics.yaml`, which carries the
honesty note this module must not restate as physics: the two-exponential form
for between-day fatigue is consistent state accounting, not a validated predictor
of performance.
"""

from __future__ import annotations

import math
from collections.abc import Callable, Mapping
from functools import lru_cache, wraps
from pathlib import Path
from typing import Annotated, Any

from pydantic import BaseModel, BeforeValidator, ConfigDict, Field, field_validator, model_validator

from .modelfile import PHYSICAL_MODELS_DIR, ModelFileError, load_model_file
from .types import (
    BodyState,
    CapacityProfile,
    CumulativeLoad,
    Dimension,
    Energy,
    FatigueRegion,
    Healing,
    Hydration,
    Injury,
    InjuryRegion,
    LastSleep,
    Pain,
    RegionSoreness,
    Sleep,
    SleepQuality,
    Soreness,
    SorenessOnset,
    Thermal,
    WPrimeBalance,
    as_document,
    freeze_mapping,
)

__all__ = [
    "BODY_DYNAMICS_PATH",
    "BodyDynamicsError",
    "BodyDynamicsParams",
    "load_params",
    "Environment",
    "NEUTRAL_ENVIRONMENT",
    "IntervalLoad",
    "NO_LOAD",
    "BodyTraits",
    "Exposure",
    "CHANNELS",
    "CHANNEL_NAMES",
    "only_with_a_clock",
    "initial_body_state",
    "advance",
    "advance_w_prime_balance",
    "advance_peripheral_fatigue",
    "advance_central_fatigue",
    "advance_sleep",
    "advance_energy",
    "advance_hydration",
    "advance_thermal",
    "advance_soreness",
    "advance_injuries",
    "advance_cumulative_load",
    "advance_pain",
    "sleep_pressure",
    "reconstitution_tau_s",
    "sweat_rate_pct_per_hour",
    "channel_diff",
    "CHANNEL_SECTIONS",
    "Y1_START_CIRCADIAN_PHASE",
]

BODY_DYNAMICS_PATH = PHYSICAL_MODELS_DIR / "body-dynamics.yaml"


class BodyDynamicsError(ValueError):
    """The dynamics were asked for something the model does not define."""


# --------------------------------------------------------------------------
# Parameters
# --------------------------------------------------------------------------
#
# The YAML wraps numbers in `{source: "[MARKING] …", value: …}` blocks so that
# every number carries its provenance marking. The models below ignore `source`
# and unwrap `value`: the marking is validated by `modelfile.py`, which is the
# gate every file in `data/models/physical/` passes through.


def _unwrap(value: Any) -> Any:
    if isinstance(value, Mapping) and "value" in value:
        return value["value"]
    return value


Scalar = Annotated[float, BeforeValidator(_unwrap)]


class _Block(BaseModel):
    model_config = ConfigDict(frozen=True, extra="ignore")


class _Reconstitution(_Block):
    at_low_intensity: float
    at_moderate_intensity: float
    at_heavy_intensity: float


class _Interpolation(_Block):
    moderate_intensity_fraction: float


class _WPrimeCapacity(_Block):
    reference_j: float


class WPrimeParams(_Block):
    reconstitution_tau_s: _Reconstitution
    reconstitution_interpolation: _Interpolation
    critical_power_fraction: Scalar
    depletion_rate_per_hour_per_unit_excess: Scalar
    capacity_j: _WPrimeCapacity


class PeripheralFatigueParams(_Block):
    recovery_tau_h: Mapping[FatigueRegion, float]
    accrual_per_hour_at_full_load: Scalar

    @field_validator("recovery_tau_h", mode="before")
    @classmethod
    def _drop_source(cls, value: Any) -> Any:
        if isinstance(value, Mapping):
            return {key: item for key, item in value.items() if key != "source"}
        return value


class _CentralRecovery(_Block):
    awake: float
    asleep: float


class CentralFatigueParams(_Block):
    recovery_tau_h: _CentralRecovery
    accrual_per_hour_at_full_load: Scalar
    accrual_per_hour_at_full_sleep_pressure: Scalar


class _QualityEfficiency(_Block):
    good: float
    fair: float
    poor: float


class _ProcessS(_Block):
    half_saturation_h: float
    normal_waking_hours: float


class _ProcessC(_Block):
    trough_hour: float
    gain: float


class SleepParams(_Block):
    need_hours_per_day: Scalar
    quality_efficiency: _QualityEfficiency
    debt_cap_hours: Scalar
    process_s: _ProcessS
    process_c: _ProcessC
    performance_loss_gain: Scalar


class _WorkBurn(_Block):
    coefficient: float
    intensity_exponent: float


class _Dehydration(_Block):
    per_pct_above_threshold: float


class _Kcal(_Block):
    resting_expenditure_per_day: float
    work_expenditure_per_hour_at_full: float
    intake_per_day_at_full_ration: float
    window_tau_h: float


class EnergyParams(_Block):
    resting_burn_per_hour: Scalar
    work_burn: _WorkBurn
    repletion_per_hour_at_full_diet: Scalar
    dehydration_coupling: _Dehydration
    kcal_ledger: _Kcal


class _SweatRate(_Block):
    per_degree_above_neutral: float
    work_equivalent_degrees: float


class HydrationParams(_Block):
    #: Which dimensions a deficit above the threshold reaches. Read from the
    #: file rather than hard-coded so the reach of a channel is reviewable next
    #: to its constants.
    degrades: tuple[Dimension, ...]
    threshold_pct_body_mass: Scalar
    sweat_rate_pct_body_mass_per_hour: _SweatRate
    intake_pct_body_mass_per_hour: Scalar
    max_deficit_pct_body_mass: Scalar
    loss_per_pct_above_threshold: Scalar


class _CoreOffset(_Block):
    gain_c_per_hour: float
    dissipation_tau_h: float
    resting_fraction: float
    max_offset_c: float
    dehydration_amplification_per_pct: float


class _ThermalLoss(_Block):
    per_degree_wbgt: float
    per_degree_core_offset: float


class ThermalParams(_Block):
    neutral_wbgt_c: Scalar
    core_offset: _CoreOffset
    loss_per_degree_over_tolerance: _ThermalLoss


class _SorenessKernel(_Block):
    onset_delay_h: float
    onset_resolution_h: Scalar
    latency_stages: int
    latency_tau_h: float
    resolution_tau_h: float


class _RepeatedBout(_Block):
    gain_per_unit_dose: float
    max_damage_reduction: float
    decay_tau_h: float


class SorenessParams(_Block):
    kernel: _SorenessKernel
    damage_per_hour_at_full_eccentric_load: Scalar
    repeated_bout: _RepeatedBout


class _InjuryHealing(_Block):
    recovery_scaling_exponent: float
    min_expected_days: float


class _Setback(_Block):
    intensity_threshold: float


class InjuryParams(_Block):
    healing: _InjuryHealing
    setback_on_load: _Setback


class ChannelParams(_Block):
    w_prime_balance: WPrimeParams
    peripheral_fatigue: PeripheralFatigueParams
    central_fatigue: CentralFatigueParams
    sleep: SleepParams
    energy: EnergyParams
    hydration: HydrationParams
    thermal: ThermalParams
    soreness: SorenessParams
    injuries: InjuryParams


class CumulativeLoadParams(_Block):
    acute_tau_h: float
    chronic_tau_h: float


class PainParams(_Block):
    from_soreness: float
    from_injury_at_rest: float
    analgesia_decay_tau_h: float
    response_tau_h: float


class IntegrationParams(_Block):
    step_hours: float
    max_interval_hours: float


class ReferenceBodyParams(_Block):
    body_mass_kg: float
    recovery_rate_h: float
    thermoregulation_c: float
    injury_resilience: float


class DegradationParams(_Block):
    """How far each channel reaches, per dimension.

    Every entry is a loss at the channel's full extent. A channel that reached
    every dimension equally would be failure mode F12 with a table in front of it,
    which is why `unmodulated_dimensions` is written out: the composition is
    total, and every dimension is either named by a channel or named there.
    """

    dimension_regions: Mapping[Dimension, Mapping[FatigueRegion, float]]
    peripheral_fatigue_loss_at_full: Mapping[Dimension, float]
    w_prime_loss_at_empty: Mapping[Dimension, float]
    central_fatigue_loss_at_full: Mapping[Dimension, float]
    soreness_loss_at_full: Mapping[Dimension, float]
    substrate_loss_at_empty: Mapping[Dimension, float]
    sleep_sensitivity: Mapping[Dimension, float]
    retention_floor: Scalar
    unmodulated_dimensions: tuple[Dimension, ...]

    @model_validator(mode="after")
    def _per_region_channels_know_where_to_read(self) -> DegradationParams:
        """A channel stored per region needs a region map for what it degrades.

        Without this the file can name a dimension in a loss table, have no entry
        for it in `dimension_regions`, and quietly degrade nothing — a channel
        switched off by omission rather than by decision.
        """
        for table, name in (
            (self.peripheral_fatigue_loss_at_full, "peripheral_fatigue_loss_at_full"),
            (self.soreness_loss_at_full, "soreness_loss_at_full"),
        ):
            missing = sorted(
                dimension.value for dimension in table if dimension not in self.dimension_regions
            )
            if missing:
                raise ValueError(
                    f"{name} names {missing}, which have no entry in dimension_regions: a "
                    f"per-region channel with no region to read would degrade nothing"
                )
        return self

    @field_validator(
        "dimension_regions",
        "peripheral_fatigue_loss_at_full",
        "w_prime_loss_at_empty",
        "central_fatigue_loss_at_full",
        "soreness_loss_at_full",
        "substrate_loss_at_empty",
        "sleep_sensitivity",
        mode="before",
    )
    @classmethod
    def _drop_source(cls, value: Any) -> Any:
        if isinstance(value, Mapping):
            return {key: item for key, item in value.items() if key != "source"}
        return value

    @field_validator("unmodulated_dimensions", mode="before")
    @classmethod
    def _unwrap_dimension_list(cls, value: Any) -> Any:
        if isinstance(value, Mapping):
            return value.get("dimensions", ())
        return value


class Taxonomies(_Block):
    """The two region taxonomies of decision 13.2, and the total map between them."""

    fatigue_regions: tuple[FatigueRegion, ...]
    injury_regions: tuple[InjuryRegion, ...]
    injury_region_to_fatigue_region: Mapping[InjuryRegion, FatigueRegion]


class BodyDynamicsParams(_Block):
    """A loaded, validated `body-dynamics.yaml`."""

    model_version: str
    status: str
    taxonomies: Taxonomies
    integration: IntegrationParams
    reference_body: ReferenceBodyParams
    channels: ChannelParams
    cumulative_load: CumulativeLoadParams
    pain: PainParams
    degradation: DegradationParams

    @classmethod
    def load(cls, path: str | Path = BODY_DYNAMICS_PATH) -> BodyDynamicsParams:
        path = Path(path)
        try:
            data = load_model_file(path)
        except ModelFileError as error:
            raise BodyDynamicsError(str(error)) from error
        if data.get("model_kind") != "BODY_DYNAMICS":
            raise BodyDynamicsError(f"{path}: not a BODY_DYNAMICS file")
        params = cls.model_validate(data)
        params._assert_taxonomy_map_is_total(path)
        return params

    def _assert_taxonomy_map_is_total(self, path: Path) -> None:
        """Decision 13.2 is only safe while every injury region has an image.

        The asymmetry between the four fatigue regions and the fine injury
        regions is deliberate and documented; a *gap* in the map between them is
        not, and would be discovered as a `KeyError` on the day a lesion first
        needed to be located in a fatigue reservoir.
        """
        missing = sorted(
            region.value
            for region in InjuryRegion
            if region not in self.taxonomies.injury_region_to_fatigue_region
        )
        if missing:
            raise BodyDynamicsError(
                f"{path}: injury_region_to_fatigue_region is not total; no fatigue region "
                f"for {missing}"
            )
        if set(self.taxonomies.injury_regions) != set(InjuryRegion):
            raise BodyDynamicsError(
                f"{path}: taxonomies.injury_regions has drifted from types.InjuryRegion"
            )
        if set(self.taxonomies.fatigue_regions) != set(FatigueRegion):
            raise BodyDynamicsError(
                f"{path}: taxonomies.fatigue_regions has drifted from types.FatigueRegion"
            )

    def fatigue_region_of(self, region: InjuryRegion) -> FatigueRegion:
        return self.taxonomies.injury_region_to_fatigue_region[region]


@lru_cache(maxsize=4)
def load_params(path: str | Path = BODY_DYNAMICS_PATH) -> BodyDynamicsParams:
    """The parameter file, loaded once. Immutable, so sharing it is safe."""
    return BodyDynamicsParams.load(path)


# --------------------------------------------------------------------------
# What the interval was like
# --------------------------------------------------------------------------


class Environment(BaseModel):
    """Ambient world truth for one interval: what the world was doing to the body.

    Heat is environment; thirst is interoception (PSV1-6). Nothing here is a
    perception, and nothing here reaches an agent.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    wbgt_c: float = 21.0
    work_rest_ratio: float = Field(default=1.0, gt=0.0)
    clothing_insulation: float = Field(default=1.0, ge=0.0)
    asleep: bool = False
    sleep_quality: SleepQuality = SleepQuality.GOOD
    #: Fraction of ad-libitum fluid the interval actually offered. Rationing is a
    #: state that persists, not a modifier of the day.
    fluid_access: float = Field(default=1.0, ge=0.0, le=1.0)
    #: Carbohydrate intake as a fraction of requirement. Low here is what makes
    #: the third day of a rationed exam start worse than the first.
    carbohydrate_intake: float = Field(default=1.0, ge=0.0, le=1.0)


NEUTRAL_ENVIRONMENT = Environment()


class IntervalLoad(BaseModel):
    """What the body did during the interval — the exogenous input of §7.

    Deciding this from an `ExertionIntent` is the effort resolver's job (PSV1-4,
    spec §6 step 6). PSV1-2 owns only the response: given that this much work
    happened over this many hours, what does the body look like afterwards.

    `intensity` is a fraction of `capability_available`, which is what the whole
    model indexes load to — never an absolute wattage, and never a fraction of
    latent capacity, because a tired body working flat out is at intensity 1.0.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    intensity: float = Field(default=0.0, ge=0.0, le=2.0)
    #: Share of the work each fatigue region carried, `0..1` each.
    by_region: Mapping[FatigueRegion, float] = Field(default_factory=dict)
    #: How much of the work was eccentric. DOMS comes from this, not from volume.
    eccentric_fraction: float = Field(default=0.0, ge=0.0, le=1.0)
    #: What the body was doing. The repeated-bout effect is specific to the
    #: activity as well as to the region: squats do not protect a forearm.
    activity: str = "unspecified"

    @field_validator("by_region", mode="after")
    @classmethod
    def _shares_are_fractions(
        cls, value: Mapping[FatigueRegion, float]
    ) -> Mapping[FatigueRegion, float]:
        for region, share in value.items():
            if not 0.0 <= share <= 1.0:
                raise ValueError(f"by_region[{region.value}] outside [0, 1]: {share}")
        return freeze_mapping(value)

    def regional_intensity(self, region: FatigueRegion) -> float:
        return self.intensity * float(self.by_region.get(region, 0.0))


NO_LOAD = IntervalLoad()


class BodyTraits(BaseModel):
    """The constitutional dimensions that *govern* the dynamics.

    Derived from the frozen `capacity_baseline` at call time and never stored on
    the body: duplicating `body_mass` into `BodyState` would create a second
    source of truth for a capacity dimension, which is exactly the fusion the
    six structures exist to prevent.
    """

    model_config = ConfigDict(frozen=True, extra="forbid")

    body_mass_kg: float = Field(gt=0.0)
    recovery_rate_h: float = Field(gt=0.0)
    thermoregulation_c: float
    injury_resilience: float = Field(gt=0.0)

    @classmethod
    def from_profile(cls, profile: CapacityProfile) -> BodyTraits:
        return cls(
            body_mass_kg=profile.value(Dimension.BODY_MASS),
            recovery_rate_h=profile.value(Dimension.RECOVERY_RATE),
            thermoregulation_c=profile.value(Dimension.THERMOREGULATION),
            injury_resilience=profile.value(Dimension.INJURY_RESILIENCE),
        )

    @classmethod
    def reference(cls, params: BodyDynamicsParams | None = None) -> BodyTraits:
        reference = (params or load_params()).reference_body
        return cls(
            body_mass_kg=reference.body_mass_kg,
            recovery_rate_h=reference.recovery_rate_h,
            thermoregulation_c=reference.thermoregulation_c,
            injury_resilience=reference.injury_resilience,
        )


class Exposure(BaseModel):
    """Everything a channel needs besides the body and the interval.

    This is the `env` of the model's `(state, Δt, env) -> state`: the ambient
    environment, the work performed, the body's own traits, and the versioned
    parameters the channels read. Bundled so that no channel reaches for a
    module-level global — the functions are pure, and two runs of the same inputs
    are the same run.
    """

    model_config = ConfigDict(frozen=True, arbitrary_types_allowed=True)

    environment: Environment = NEUTRAL_ENVIRONMENT
    load: IntervalLoad = NO_LOAD
    traits: BodyTraits | None = None
    params: BodyDynamicsParams | None = None

    @classmethod
    def of(
        cls,
        *,
        environment: Environment = NEUTRAL_ENVIRONMENT,
        load: IntervalLoad = NO_LOAD,
        traits: BodyTraits | None = None,
        params: BodyDynamicsParams | None = None,
    ) -> Exposure:
        params = params or load_params()
        return cls(
            environment=environment,
            load=load,
            traits=traits or BodyTraits.reference(params),
            params=params,
        )

    @property
    def resolved_params(self) -> BodyDynamicsParams:
        return self.params or load_params()

    @property
    def resolved_traits(self) -> BodyTraits:
        return self.traits or BodyTraits.reference(self.resolved_params)

    @property
    def intensity(self) -> float:
        """Work intensity, forced to zero while asleep. Nobody trains asleep."""
        return 0.0 if self.environment.asleep else self.load.intensity

    def regional_intensity(self, region: FatigueRegion) -> float:
        return 0.0 if self.environment.asleep else self.load.regional_intensity(region)


# --------------------------------------------------------------------------
# Small helpers
# --------------------------------------------------------------------------


def _clamp(value: float, low: float, high: float) -> float:
    return low if value < low else high if value > high else value


def _onset_instant(release_at_h: float, resolution_h: float) -> float:
    """The onset-grid instant at or after `release_at_h`.

    Always rounded **up**, so quantising the release cannot bring damage forward:
    the measured 12 h onset stays a floor rather than becoming an average, and
    failure mode F11 keeps the same guarantee it had before the grid existed.
    """
    if resolution_h <= 0.0:
        return round(release_at_h, 9)
    return round(math.ceil(release_at_h / resolution_h - 1e-9) * resolution_h, 9)


def _decay(dt_hours: float, tau_hours: float) -> float:
    """`exp(-Δt/τ)`. Exactly 1 at `Δt = 0`, which is how a scene heals nothing."""
    return math.exp(-dt_hours / tau_hours)


def _approach(current: float, target: float, dt_hours: float, tau_hours: float) -> float:
    """First-order approach to `target`. Composes exactly over subdivided intervals."""
    return target + (current - target) * _decay(dt_hours, tau_hours)


def only_with_a_clock(channel: Callable[..., BodyState]) -> Callable[..., BodyState]:
    """Make a channel the identity when no time passes — P1, per channel.

    `advance` already refuses to change a body over a zero interval, but a channel
    is callable on its own, and a channel that recomputed anything at `Δt = 0`
    would be a scene healing a body through a side door. The fields at risk are
    the ones that are recorded rather than integrated — the environment a body
    was last exposed to, the sleep episode it is inside, when it last ate — and
    none of those happened either, because no interval did.

    Applied by decoration rather than written into each channel so that a channel
    added later cannot quietly not have it.
    """

    @wraps(channel)
    def guarded(state: BodyState, dt_hours: float, *args: Any, **kwargs: Any) -> BodyState:
        if dt_hours == 0.0:
            return state
        return channel(state, dt_hours, *args, **kwargs)

    return guarded


def _replace(state: BodyState, **sections: Any) -> BodyState:
    """Swap one channel of a body, leaving the rest identical.

    Rebuilt through the constructor because Pydantic's default copy update is
    unchecked. It is not a second writer: this module is the only one allowed to
    call it on a body, and a test of architecture enforces that.
    """
    fields = {name: getattr(state, name) for name in BodyState.model_fields}
    fields.update(sections)
    return BodyState(**fields)


# --------------------------------------------------------------------------
# Channel 1 — `w_prime_balance`: seconds to minutes
# --------------------------------------------------------------------------


def reconstitution_tau_s(recovery_intensity: float, params: BodyDynamicsParams) -> float:
    """τ of W' reconstitution, **larger** the harder the recovery intensity.

    Three measured anchors — ~377 s recovering at 20 W, ~452 s in the moderate
    domain, ~580 s in the heavy domain — with a declared linear interpolation
    between them. The counter-intuitive direction is the whole finding: resting
    harder does not refill the reserve faster.
    """
    channel = params.channels.w_prime_balance
    anchors = channel.reconstitution_tau_s
    moderate_at = _clamp(channel.reconstitution_interpolation.moderate_intensity_fraction, 1e-6, 1.0 - 1e-6)
    r = _clamp(recovery_intensity, 0.0, 1.0)
    if r <= moderate_at:
        span = r / moderate_at
        return anchors.at_low_intensity + span * (anchors.at_moderate_intensity - anchors.at_low_intensity)
    span = (r - moderate_at) / (1.0 - moderate_at)
    return anchors.at_moderate_intensity + span * (anchors.at_heavy_intensity - anchors.at_moderate_intensity)


@only_with_a_clock
def advance_w_prime_balance(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """Work above critical power drains; work below it reconstitutes.

    What is spent above critical power does not come back inside the same event,
    which is what gives step 4 of the effort resolver (PSV1-4) a material price:
    following an acceleration to hold a display ceiling *costs reserve*.
    """
    params = exposure.resolved_params
    channel = params.channels.w_prime_balance
    reserve = state.w_prime_balance
    intensity = exposure.intensity
    critical = channel.critical_power_fraction

    tau_s = reconstitution_tau_s(min(1.0, intensity / critical) if critical > 0 else 1.0, params)
    if intensity > critical:
        drained = (
            reserve.capacity_j
            * channel.depletion_rate_per_hour_per_unit_excess
            * (intensity - critical)
            * dt_hours
        )
        remaining = _clamp(reserve.remaining_j - drained, 0.0, reserve.capacity_j)
    else:
        remaining = _clamp(
            _approach(reserve.remaining_j, reserve.capacity_j, dt_hours * 3600.0, tau_s),
            0.0,
            reserve.capacity_j,
        )
    return _replace(
        state,
        w_prime_balance=WPrimeBalance(
            remaining_j=remaining, capacity_j=reserve.capacity_j, tau_s=tau_s
        ),
    )


# --------------------------------------------------------------------------
# Channel 2 — `peripheral_fatigue`: hours, per region
# --------------------------------------------------------------------------


@only_with_a_clock
def advance_peripheral_fatigue(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """Four reservoirs, each recovering on the body's own `recovery_rate`.

    Four and not thirty-seven: decision 13.2 keeps fatigue coarse and leaves fine
    granularity to `Injury`, and what decides a grip exam is the per-dimension
    `impairments` of a lesion, not the name of a fatigue reservoir.
    """
    params = exposure.resolved_params
    channel = params.channels.peripheral_fatigue
    traits = exposure.resolved_traits

    fatigue: dict[FatigueRegion, float] = {}
    for region in FatigueRegion:
        current = float(state.peripheral_fatigue[region])
        tau = max(1e-6, traits.recovery_rate_h * channel.recovery_tau_h[region])
        recovered = current * _decay(dt_hours, tau)
        drive = channel.accrual_per_hour_at_full_load * exposure.regional_intensity(region)
        accrued = (1.0 - recovered) * (1.0 - _decay(dt_hours, 1.0 / drive)) if drive > 0 else 0.0
        fatigue[region] = _clamp(recovered + accrued, 0.0, 1.0)
    return _replace(state, peripheral_fatigue=freeze_mapping(fatigue))


# --------------------------------------------------------------------------
# Channel 3 — `central_fatigue`: hours to days
# --------------------------------------------------------------------------


@only_with_a_clock
def advance_central_fatigue(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """The slow trace: alertness, executive control, expression of technique.

    Sleep pressure feeds it as well as degrading performance directly, because
    the model's channel table carries both rows. The coupling is deliberate and
    its gain deliberately small, so the fast path stays the dominant one.
    """
    params = exposure.resolved_params
    channel = params.channels.central_fatigue
    tau = channel.recovery_tau_h.asleep if exposure.environment.asleep else channel.recovery_tau_h.awake

    recovered = state.central_fatigue * _decay(dt_hours, max(1e-6, tau))
    drive = (
        channel.accrual_per_hour_at_full_load * exposure.intensity
        + channel.accrual_per_hour_at_full_sleep_pressure * sleep_pressure(state, params)
    )
    accrued = (1.0 - recovered) * (1.0 - _decay(dt_hours, 1.0 / drive)) if drive > 0 else 0.0
    return _replace(state, central_fatigue=_clamp(recovered + accrued, 0.0, 1.0))


# --------------------------------------------------------------------------
# Channel 4 — `sleep`: days
# --------------------------------------------------------------------------


def sleep_pressure(state: BodyState, params: BodyDynamicsParams | None = None) -> float:
    """Two-process sleep pressure in `[0, 1]`: homeostatic × circadian.

    Process S saturates in accumulated debt plus the hours awake beyond a normal
    waking day; process C modulates it around the circadian trough, which this
    cohort runs phase-delayed into. The framework is standard; the numbers that
    turn it into a fraction are `[INT]` and say so in the parameter file.
    """
    channel = (params or load_params()).channels.sleep
    excess_awake = max(0.0, state.sleep.hours_since_wake - channel.process_s.normal_waking_hours)
    homeostatic = state.sleep.debt_hours + excess_awake
    process_s = homeostatic / (homeostatic + channel.process_s.half_saturation_h)

    phase_angle = 2.0 * math.pi * (state.sleep.circadian_phase - channel.process_c.trough_hour) / 24.0
    #: 1 at the trough, 0 at the peak — the circadian contribution to pressure.
    process_c = 0.5 * (1.0 + math.cos(phase_angle))
    modulated = process_s * (1.0 + channel.process_c.gain * (2.0 * process_c - 1.0))
    return _clamp(modulated, 0.0, 1.0)


@only_with_a_clock
def advance_sleep(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """Debt accrues while awake and is repaid, at the night's quality, while asleep.

    The circadian phase advances whatever the body does — it is the one part of
    this channel the body does not get a vote on.
    """
    params = exposure.resolved_params
    channel = params.channels.sleep
    environment = exposure.environment
    current = state.sleep

    need_rate = channel.need_hours_per_day / 24.0
    if environment.asleep:
        efficiency = getattr(channel.quality_efficiency, environment.sleep_quality.value)
        debt = current.debt_hours + need_rate * dt_hours - dt_hours * efficiency
        hours_since_wake = 0.0
        previous = current.last_sleep
        continuing = (
            previous is not None
            and previous.quality == environment.sleep_quality
            and abs(previous.end_h - state.t_hours) < 1e-9
        )
        last_sleep = LastSleep(
            start_h=previous.start_h if continuing and previous is not None else state.t_hours,
            end_h=state.t_hours + dt_hours,
            quality=environment.sleep_quality,
        )
    else:
        debt = current.debt_hours + need_rate * dt_hours
        hours_since_wake = current.hours_since_wake + dt_hours
        last_sleep = current.last_sleep

    return _replace(
        state,
        sleep=Sleep(
            debt_hours=_clamp(debt, 0.0, channel.debt_cap_hours),
            hours_since_wake=hours_since_wake,
            circadian_phase=(current.circadian_phase + dt_hours) % 24.0,
            last_sleep=last_sleep,
        ),
    )


# --------------------------------------------------------------------------
# Channel 5 — `energy`: hours to days
# --------------------------------------------------------------------------


@only_with_a_clock
def advance_energy(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """Substrate is what makes a multi-day exam a multi-day exam.

    Glycogen degradation rises exponentially with intensity, and on a
    low-carbohydrate diet the store stays low for days: rationing is not a
    modifier of the day, it is a state that persists into the next one.
    Dehydration accelerates the burn, which is the coupling the review names.
    """
    params = exposure.resolved_params
    channel = params.channels.energy
    environment = exposure.environment
    intensity = exposure.intensity
    threshold = params.channels.hydration.threshold_pct_body_mass

    burn = channel.resting_burn_per_hour + channel.work_burn.coefficient * (
        math.exp(channel.work_burn.intensity_exponent * intensity) - 1.0
    )
    above_threshold = max(0.0, state.hydration.deficit_pct_body_mass - threshold)
    burn *= 1.0 + channel.dehydration_coupling.per_pct_above_threshold * above_threshold
    repletion = channel.repletion_per_hour_at_full_diet * environment.carbohydrate_intake
    substrate = _clamp(
        state.energy.substrate_availability + (repletion - burn) * dt_hours, 0.0, 1.0
    )

    ledger = channel.kcal_ledger
    mass_scale = exposure.resolved_traits.body_mass_kg / params.reference_body.body_mass_kg
    expenditure = (
        ledger.resting_expenditure_per_day / 24.0 + ledger.work_expenditure_per_hour_at_full * intensity
    ) * mass_scale
    intake = ledger.intake_per_day_at_full_ration / 24.0 * environment.carbohydrate_intake * mass_scale
    balance = _approach(state.energy.balance_kcal_24h, 0.0, dt_hours, ledger.window_tau_h) + (
        intake - expenditure
    ) * dt_hours

    return _replace(
        state,
        energy=Energy(
            substrate_availability=substrate,
            balance_kcal_24h=balance,
            last_meal_at_h=(
                state.t_hours + dt_hours
                if environment.carbohydrate_intake > 0.0
                else state.energy.last_meal_at_h
            ),
        ),
    )


# --------------------------------------------------------------------------
# Channel 6 — `hydration`: hours
# --------------------------------------------------------------------------


def sweat_rate_pct_per_hour(state: BodyState, exposure: Exposure) -> float:
    """Loss per hour as a percentage of body mass. No age term (F16)."""
    params = exposure.resolved_params
    channel = params.channels.hydration.sweat_rate_pct_body_mass_per_hour
    environment = exposure.environment
    index = max(0.0, environment.wbgt_c - params.channels.thermal.neutral_wbgt_c)
    index += channel.work_equivalent_degrees * exposure.intensity
    return (
        channel.per_degree_above_neutral
        * index
        * environment.work_rest_ratio
        * environment.clothing_insulation
    )


@only_with_a_clock
def advance_hydration(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """Below ~2 % of body mass the deficit costs nothing; above it, progressively.

    The threshold is measured; what a percentage point above it costs is not.
    """
    params = exposure.resolved_params
    channel = params.channels.hydration
    intake = channel.intake_pct_body_mass_per_hour * exposure.environment.fluid_access
    deficit = state.hydration.deficit_pct_body_mass + (
        sweat_rate_pct_per_hour(state, exposure) - intake
    ) * dt_hours
    return _replace(
        state,
        hydration=Hydration(
            deficit_pct_body_mass=_clamp(deficit, 0.0, channel.max_deficit_pct_body_mass)
        ),
    )


# --------------------------------------------------------------------------
# Channel 7 — `thermal`: minutes to hours
# --------------------------------------------------------------------------


@only_with_a_clock
def advance_thermal(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """Heat load indexed to WBGT and to modifiable factors, never to age.

    With adequate hydration there is no demonstrated difference in heat
    accumulation, core temperature or tolerance for this cohort, so no term here
    reads an age and none may be added (failure mode F16). Individual variation
    lives in the `thermoregulation` dimension of the body's own baseline.
    """
    params = exposure.resolved_params
    channel = params.channels.thermal
    environment = exposure.environment
    core = channel.core_offset

    index = max(0.0, environment.wbgt_c - channel.neutral_wbgt_c) / 10.0
    effort = core.resting_fraction + exposure.intensity
    above_threshold = max(
        0.0,
        state.hydration.deficit_pct_body_mass - params.channels.hydration.threshold_pct_body_mass,
    )
    heat_load = (
        index
        * effort
        * environment.work_rest_ratio
        * environment.clothing_insulation
        * (1.0 + core.dehydration_amplification_per_pct * above_threshold)
    )
    equilibrium = core.gain_c_per_hour * heat_load * core.dissipation_tau_h
    offset = _approach(
        state.thermal.core_offset_c, equilibrium, dt_hours, max(1e-6, core.dissipation_tau_h)
    )
    return _replace(
        state,
        thermal=Thermal(
            core_offset_c=_clamp(offset, 0.0, core.max_offset_c),
            wbgt=environment.wbgt_c,
            work_rest_ratio=environment.work_rest_ratio,
            clothing_insulation=environment.clothing_insulation,
        ),
    )


# --------------------------------------------------------------------------
# Channel 8 — `soreness`: onset 12-24 h, peak 24-72 h, resolution ~7 d
# --------------------------------------------------------------------------


@only_with_a_clock
def advance_soreness(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """DOMS enters late, by construction.

    Damage goes into a latency chain and only reaches the expressed pool after
    walking it, which is why soreness cannot appear during the effort that caused
    it (failure mode F11). The repeated-bout flag is per region **and** per
    activity: without it, someone who trains every week suffers as if it were the
    first time, forever.
    """
    params = exposure.resolved_params
    channel = params.channels.soreness
    kernel = channel.kernel
    stages = max(1, kernel.latency_stages)
    load = exposure.load

    by_region = dict(state.soreness.by_region)
    adaptation = dict(state.soreness.repeated_bout_adaptation)

    for region in FatigueRegion:
        current = by_region.get(region, RegionSoreness())
        latency = list(current.latency) + [0.0] * (stages - len(current.latency))
        latency = latency[:stages]

        dose = (
            channel.damage_per_hour_at_full_eccentric_load
            * exposure.regional_intensity(region)
            * load.eccentric_fraction
            * dt_hours
        )
        key = f"{region.value}|{load.activity}"
        adapted = float(adaptation.get(key, 0.0))
        interval_end_h = state.t_hours + dt_hours
        due = 0.0
        #: Held damage keyed by the instant it is released on. Keying rather than
        #: appending is what keeps this queue a function of the onset window and
        #: not of `integration.step_hours`: a step half the size admits the same
        #: damage into the same bucket instead of into a second entry.
        held: dict[float, float] = {}
        for item in current.pending_onset:
            if item.release_at_h <= interval_end_h + 1e-9:
                due += item.amount
            else:
                held[item.release_at_h] = held.get(item.release_at_h, 0.0) + item.amount

        if dose > 0.0:
            admitted_dose = dose * (1.0 - channel.repeated_bout.max_damage_reduction * adapted)
            release_at_h = _onset_instant(
                interval_end_h + kernel.onset_delay_h, kernel.onset_resolution_h
            )
            held[release_at_h] = held.get(release_at_h, 0.0) + admitted_dose
            adaptation[key] = _clamp(
                adapted + (1.0 - adapted) * channel.repeated_bout.gain_per_unit_dose * dose,
                0.0,
                1.0,
            )
        pending = tuple(
            SorenessOnset(release_at_h=instant, amount=amount)
            for instant, amount in sorted(held.items())
            if amount > 0.0
        )

        #: Flow every stage from the values present at the *start* of the step.
        #: In-place flow would let one dose traverse the entire latency chain in
        #: a single integration step. Newly due damage enters stage zero only
        #: after these flows and therefore cannot be expressed immediately.
        previous_latency = tuple(latency)
        transferred = 0.0
        for index, value in enumerate(previous_latency):
            flow = min(value * (dt_hours / kernel.latency_tau_h), value)
            latency[index] -= flow
            if index + 1 < stages:
                latency[index + 1] += flow
            else:
                transferred += flow
        latency[0] += due
        expressed = _clamp(
            current.expressed * _decay(dt_hours, kernel.resolution_tau_h) + transferred, 0.0, 1.0
        )
        by_region[region] = RegionSoreness(
            latency=tuple(latency), pending_onset=pending, expressed=expressed
        )

    decay = _decay(dt_hours, channel.repeated_bout.decay_tau_h)
    adaptation = {key: value * decay for key, value in adaptation.items()}

    return _replace(
        state,
        soreness=Soreness(by_region=by_region, repeated_bout_adaptation=adaptation),
    )


# --------------------------------------------------------------------------
# Channel 9 — `injuries`: days to months
# --------------------------------------------------------------------------


@only_with_a_clock
def advance_injuries(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """Healing progresses with the clock, and loading an open lesion costs progress.

    The framing is dynamic-recursive: each exposure changes the next risk, by
    adaptation or maladaptation, and a lesion is not caused only by the incident
    that opened it. Opening one is the effort resolver's roll (PSV1-4); closing
    one is time.
    """
    params = exposure.resolved_params
    channel = params.channels.injuries
    traits = exposure.resolved_traits
    reference_recovery = params.reference_body.recovery_rate_h
    #: `recovery_rate` is a time constant in hours, so a *lower* value is a body
    #: that recovers faster and therefore heals faster.
    healing_scale = (reference_recovery / traits.recovery_rate_h) ** channel.healing.recovery_scaling_exponent

    healed: list[Injury] = []
    for injury in state.injuries:
        expected_days = max(channel.healing.min_expected_days, injury.healing.expected_days)
        progress = injury.healing.progress + healing_scale * dt_hours / (expected_days * 24.0)
        region_intensity = exposure.regional_intensity(params.fatigue_region_of(injury.region))
        if region_intensity > channel.setback_on_load.intensity_threshold:
            progress -= injury.healing.setback_on_load * region_intensity * dt_hours
        progress = _clamp(progress, 0.0, 1.0)
        if progress >= 1.0:
            #: A healed lesion leaves the body. The `body.advanced` record of the
            #: interval carries both sides of the channel, so the disappearance is
            #: in the audit artefact rather than only in the absence of a field.
            continue
        injury_fields = {name: getattr(injury, name) for name in Injury.model_fields}
        injury_fields["healing"] = Healing(
            expected_days=injury.healing.expected_days,
            progress=progress,
            setback_on_load=injury.healing.setback_on_load,
        )
        healed.append(Injury(**injury_fields))
    return _replace(state, injuries=tuple(healed))


# --------------------------------------------------------------------------
# Accounting and signal — not channels, and one of them is read by nobody
# --------------------------------------------------------------------------


@only_with_a_clock
def advance_cumulative_load(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """Exponentially weighted load over 7 and 28 days. **Read by nobody in V1.**

    The field exists, is updated, and nothing depends on it (spec §3.2). The
    ratio between the two windows is not a risk factor and must not become one:
    it is mathematically coupled, unstable at low chronic load, unsupported as a
    causal factor, and the figure that popularised it is the subject of a formal
    retraction request (failure mode F14).
    """
    params = exposure.resolved_params.cumulative_load
    dose = exposure.intensity * dt_hours
    return _replace(
        state,
        cumulative_load=CumulativeLoad(
            acute_7d=max(
                0.0, state.cumulative_load.acute_7d * _decay(dt_hours, params.acute_tau_h) + dose
            ),
            chronic_28d=max(
                0.0,
                state.cumulative_load.chronic_28d * _decay(dt_hours, params.chronic_tau_h) + dose,
            ),
        ),
    )


@only_with_a_clock
def advance_pain(state: BodyState, dt_hours: float, exposure: Exposure) -> BodyState:
    """Pain follows soreness and lesion, and analgesia suppresses it without healing.

    Pain and injury dissociate in both directions, which is why this is computed
    from both and stored apart from either. Analgesia decays on its own; the
    tissue underneath it does not care.
    """
    params = exposure.resolved_params
    channel = params.pain
    analgesia = state.pain.analgesia * _decay(dt_hours, max(1e-6, channel.analgesia_decay_tau_h))

    targets: dict[FatigueRegion, float] = {}
    for region in FatigueRegion:
        soreness = state.soreness.by_region.get(region)
        target = channel.from_soreness * (soreness.expressed if soreness else 0.0)
        for injury in state.injuries:
            if params.fatigue_region_of(injury.region) is region:
                target = max(
                    target,
                    channel.from_injury_at_rest
                    * injury.pain_profile.at_rest
                    * (1.0 - injury.healing.progress),
                )
        targets[region] = _clamp(target * (1.0 - analgesia), 0.0, 1.0)

    by_region = {
        region: _clamp(
            _approach(
                float(state.pain.by_region.get(region, 0.0)),
                targets[region],
                dt_hours,
                max(1e-6, channel.response_tau_h),
            ),
            0.0,
            1.0,
        )
        for region in FatigueRegion
    }
    return _replace(
        state,
        pain=Pain(
            by_region=by_region,
            global_intensity=max(by_region.values(), default=0.0),
            analgesia=_clamp(analgesia, 0.0, 1.0),
        ),
    )


#: The nine dynamic channels of the model's §7 table, in the order they resolve.
#: The tenth row of that table — drift of `capacity_baseline` — is out of scope
#: in V1 (spec §3.2), which is why there are nine functions here and not ten.
CHANNELS = (
    advance_w_prime_balance,
    advance_peripheral_fatigue,
    advance_central_fatigue,
    advance_sleep,
    advance_energy,
    advance_hydration,
    advance_thermal,
    advance_soreness,
    advance_injuries,
)

CHANNEL_NAMES = (
    "w_prime_balance",
    "peripheral_fatigue",
    "central_fatigue",
    "sleep",
    "energy",
    "hydration",
    "thermal",
    "soreness",
    "injuries",
)

#: Sections of `BodyState` a `body.advanced` record reports both sides of.
#: `illnesses` is here so that a run which somehow wrote one would show it in the
#: audit artefact; nothing in V1 writes it (spec §13.5).
CHANNEL_SECTIONS = (*CHANNEL_NAMES, "cumulative_load", "pain", "illnesses")


# --------------------------------------------------------------------------
# Creating a body, and the single write API
# --------------------------------------------------------------------------

#: The hour of the local day `Y1_START` is defined to begin at. A body has to
#: start somewhere on the circadian cycle, and starting it mid-morning, rested,
#: is a modelling choice rather than a fact — it is recorded in the run log with
#: the rest of the initial state so nobody has to infer it later.
Y1_START_CIRCADIAN_PHASE = 7.0


def initial_body_state(
    character_id: str,
    profile: CapacityProfile,
    *,
    t_hours: float = 0.0,
    environment: Environment = NEUTRAL_ENVIRONMENT,
    params: BodyDynamicsParams | None = None,
    circadian_phase: float = Y1_START_CIRCADIAN_PHASE,
) -> BodyState:
    """A rested body at the instant the world starts.

    Not a second writer: this **creates** a body, it does not alter one. Every
    path that changes an existing body goes through `advance` with an explicit
    `dt_hours`, and there is no signature here that heals anything.

    The reserve is scaled by the body's own mass off its frozen
    `capacity_baseline`; nothing else about the initial state is drawn, because
    nothing about it is uncertain — a rested body is rested.
    """
    params = params or load_params()
    traits = BodyTraits.from_profile(profile)
    capacity_j = params.channels.w_prime_balance.capacity_j.reference_j * (
        traits.body_mass_kg / params.reference_body.body_mass_kg
    )
    return BodyState(
        character_id=character_id,
        t_hours=t_hours,
        w_prime_balance=WPrimeBalance(
            remaining_j=capacity_j,
            capacity_j=capacity_j,
            tau_s=reconstitution_tau_s(0.0, params),
        ),
        peripheral_fatigue={region: 0.0 for region in FatigueRegion},
        central_fatigue=0.0,
        cumulative_load=CumulativeLoad(),
        sleep=Sleep(debt_hours=0.0, hours_since_wake=0.0, circadian_phase=circadian_phase % 24.0),
        energy=Energy(substrate_availability=1.0, balance_kcal_24h=0.0, last_meal_at_h=None),
        hydration=Hydration(deficit_pct_body_mass=0.0),
        thermal=Thermal(
            core_offset_c=0.0,
            wbgt=environment.wbgt_c,
            work_rest_ratio=environment.work_rest_ratio,
            clothing_insulation=environment.clothing_insulation,
        ),
        #: Every region present from the start, at zero. A body whose regions
        #: appear only once something happens to them makes the first advance
        #: look like a change in shape rather than a change in state, and the
        #: `body.advanced` diff is read by people looking for the difference.
        soreness=Soreness(
            by_region={
                region: RegionSoreness(
                    latency=(0.0,) * max(1, params.channels.soreness.kernel.latency_stages),
                    expressed=0.0,
                )
                for region in FatigueRegion
            }
        ),
        injuries=(),
        #: Never written in V1 (spec §13.5). Serialised so the survival exam that
        #: needs it does not cost a snapshot version bump.
        illnesses=(),
        pain=Pain(by_region={region: 0.0 for region in FatigueRegion}),
    )


def advance(
    state: BodyState,
    dt_hours: float,
    exposure: Exposure | None = None,
) -> BodyState:
    """Move a body forward by `dt_hours`. **The only way a body changes.**

    `dt_hours == 0` returns the state it was given, unchanged: a scene heals
    nothing (property P1, failure mode F2). Anything that wants a body to be
    different has to spend time on it, and this signature is the reason there is
    no way to ask for the difference without the time.

    Integration is at the fixed step of the parameter file, so an interval that
    is a whole number of steps decomposes exactly: advancing 24 h once equals
    advancing 12 h twice. The body is re-validated on the way out, which is where
    P3 (state validity) is enforced rather than assumed.
    """
    if dt_hours < 0.0:
        raise BodyDynamicsError(
            "the clock does not run backwards: a body cannot be advanced by a negative interval"
        )
    if dt_hours == 0.0:
        #: Object for object, not merely field for field. A caller that asks for
        #: no time is asking for no change.
        return state

    exposure = exposure if exposure is not None else Exposure.of()
    params = exposure.resolved_params
    if dt_hours > params.integration.max_interval_hours:
        raise BodyDynamicsError(
            f"an interval of {dt_hours} h exceeds the {params.integration.max_interval_hours} h "
            f"cap: advance the clock in shorter intervals, so that the fixed integration step "
            f"stays a small step"
        )
    if exposure.environment.asleep and exposure.load.intensity > 0.0:
        raise BodyDynamicsError(
            "a sleeping body cannot be carrying a load: an interval is either work or sleep, "
            "and an interval that claims to be both is a schedule the engine never built"
        )

    steps = max(1, math.ceil(dt_hours / params.integration.step_hours - 1e-9))
    step = dt_hours / steps
    for _ in range(steps):
        for channel in CHANNELS:
            state = channel(state, step, exposure)
        state = advance_cumulative_load(state, step, exposure)
        state = advance_pain(state, step, exposure)
        state = _replace(state, t_hours=state.t_hours + step)

    #: Rebuild through the constructor: `model_copy` does not validate, and the
    #: domains of `BodyState` are where P3 lives. Every channel clamps, and this
    #: is what makes that a checked claim rather than a promise.
    return BodyState(**{field: getattr(state, field) for field in BodyState.model_fields})


def channel_diff(before: BodyState, after: BodyState) -> dict[str, dict[str, Any]]:
    """The channels that moved, with both sides — the payload of `body.advanced`.

    Only what changed: a record that repeated every field of every channel would
    make the one that moved harder to find, and the log is read by people looking
    for exactly that.
    """
    changed: dict[str, dict[str, Any]] = {}
    for section in CHANNEL_SECTIONS:
        old = getattr(before, section)
        new = getattr(after, section)
        old_value = as_document(old)
        new_value = as_document(new)
        if old_value != new_value:
            changed[section] = {"before": old_value, "after": new_value}
    return changed
