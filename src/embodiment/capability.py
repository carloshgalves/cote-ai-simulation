"""`capability_available(t)` — the baseline as this body is right now, per dimension.

    capacity_baseline ⊗ impairments(injuries) ⊗ state ⊗ environment

Composed **per dimension**, and that is the whole point of the module. An
impairment that applies equally to every dimension is failure mode F12, not a
simplification: a compromised wrist takes down grip strength and throwing and
leaves sprinting almost untouched, and that is what lets an exam of grip be
decided by an injury nobody narrated.

Two rules the composition has to keep straight:

* **Orientation.** `reaction_time` is in milliseconds and `recovery_rate` is a
  time constant in hours, so a degraded body produces a *larger* number in both.
  A retention factor is applied as a divisor there and as a multiplier
  everywhere else; `DIMENSION_UNITS[d].orientation` is the authority.
* **Traits are not performances.** Body mass, stature, injury resilience,
  recovery rate, thermoregulation and pain tolerance are not degraded by state:
  they are what *governs* the dynamics. Fatigue does not make a body shorter.
  They are named in the parameter file, so the composition stays total.

**Nothing here reaches an agent.** These are `BodyState` numbers by another
name, and spec §7.1 keeps them inside the engine: what a character gets is the
qualitative, biased interoception of PSV1-6. A function that returned one of
these numbers to a context builder would be the leak this module is most likely
to be the source of.
"""

from __future__ import annotations

from collections.abc import Mapping

from .dynamics import (
    BodyDynamicsParams,
    BodyTraits,
    Environment,
    NEUTRAL_ENVIRONMENT,
    load_params,
    sleep_pressure,
)
from .types import (
    DIMENSION_UNITS,
    BodyState,
    CapacityProfile,
    Dimension,
    FatigueRegion,
)

__all__ = [
    "CHANNEL_KEYS",
    "channel_losses",
    "channel_retention",
    "state_retention",
    "impairment_multipliers",
    "capability_available",
    "apply_retention",
]

#: The state channels that reach `capability_available`, in the order of the
#: model's §7 table. `injuries` is not among them: a lesion arrives as an
#: explicit per-dimension multiplier, not as a scalar the composition invents.
CHANNEL_KEYS = (
    "w_prime_balance",
    "peripheral_fatigue",
    "central_fatigue",
    "sleep",
    "energy",
    "hydration",
    "thermal",
    "soreness",
)


def _regional_exposure(
    dimension: Dimension,
    per_region: Mapping[FatigueRegion, float],
    params: BodyDynamicsParams,
) -> float:
    """How much of a per-region channel a dimension actually feels."""
    weights = params.degradation.dimension_regions.get(dimension, {})
    return sum(float(weights.get(region, 0.0)) * float(value) for region, value in per_region.items())


def channel_losses(
    state: BodyState,
    *,
    environment: Environment = NEUTRAL_ENVIRONMENT,
    traits: BodyTraits | None = None,
    params: BodyDynamicsParams | None = None,
) -> Mapping[str, Mapping[Dimension, float]]:
    """What each channel costs each dimension, as a fraction of the baseline.

    Returned per channel rather than pre-multiplied because the ordering between
    channels is the part of this the evidence actually supports — the sleep
    meta-analysis gives ratios between dimensions, not percentages — and a caller
    checking that ordering should not have to unpick a product to see it.
    """
    params = params or load_params()
    traits = traits or BodyTraits.reference(params)
    degradation = params.degradation
    modulated = tuple(
        dimension for dimension in Dimension if dimension not in degradation.unmodulated_dimensions
    )

    losses: dict[str, dict[Dimension, float]] = {key: {} for key in CHANNEL_KEYS}

    spent = 1.0 - state.w_prime_balance.fraction
    for dimension, loss in degradation.w_prime_loss_at_empty.items():
        losses["w_prime_balance"][dimension] = loss * spent

    for dimension, loss in degradation.peripheral_fatigue_loss_at_full.items():
        losses["peripheral_fatigue"][dimension] = loss * _regional_exposure(
            dimension, state.peripheral_fatigue, params
        )

    for dimension, loss in degradation.central_fatigue_loss_at_full.items():
        losses["central_fatigue"][dimension] = loss * state.central_fatigue

    pressure = sleep_pressure(state, params)
    gain = params.channels.sleep.performance_loss_gain
    for dimension, sensitivity in degradation.sleep_sensitivity.items():
        losses["sleep"][dimension] = gain * sensitivity * pressure

    depleted = 1.0 - state.energy.substrate_availability
    for dimension, loss in degradation.substrate_loss_at_empty.items():
        losses["energy"][dimension] = loss * depleted

    hydration = params.channels.hydration
    above_threshold = max(
        0.0, state.hydration.deficit_pct_body_mass - hydration.threshold_pct_body_mass
    )
    #: Amplified by heat, per the review: the same deficit costs more in the sun.
    heat_amplification = 1.0 + max(0.0, environment.wbgt_c - params.channels.thermal.neutral_wbgt_c) / 10.0
    for dimension in hydration.degrades:
        losses["hydration"][dimension] = (
            hydration.loss_per_pct_above_threshold * above_threshold * heat_amplification
        )

    thermal = params.channels.thermal.loss_per_degree_over_tolerance
    #: The one channel that reaches every dimension, because heat does. It is
    #: indexed to WBGT, to the work:rest ratio already folded into `core_offset_c`
    #: and to the body's own tolerated WBGT — never to age (failure mode F16).
    over_tolerance = max(0.0, environment.wbgt_c - traits.thermoregulation_c)
    thermal_loss = (
        thermal.per_degree_wbgt * over_tolerance
        + thermal.per_degree_core_offset * state.thermal.core_offset_c
    )
    for dimension in modulated:
        losses["thermal"][dimension] = thermal_loss

    expressed = {region: entry.expressed for region, entry in state.soreness.by_region.items()}
    for dimension, loss in degradation.soreness_loss_at_full.items():
        losses["soreness"][dimension] = loss * _regional_exposure(dimension, expressed, params)

    for channel, per_dimension in losses.items():
        for dimension in tuple(per_dimension):
            if dimension in degradation.unmodulated_dimensions:
                #: A trait that governs the dynamics is not worn down by them.
                del per_dimension[dimension]
            else:
                per_dimension[dimension] = max(0.0, min(1.0, per_dimension[dimension]))
    return losses


def channel_retention(
    state: BodyState,
    *,
    environment: Environment = NEUTRAL_ENVIRONMENT,
    traits: BodyTraits | None = None,
    params: BodyDynamicsParams | None = None,
) -> Mapping[str, Mapping[Dimension, float]]:
    """`1 - loss` per channel per dimension."""
    return {
        channel: {dimension: 1.0 - loss for dimension, loss in per_dimension.items()}
        for channel, per_dimension in channel_losses(
            state, environment=environment, traits=traits, params=params
        ).items()
    }


def state_retention(
    state: BodyState,
    *,
    environment: Environment = NEUTRAL_ENVIRONMENT,
    traits: BodyTraits | None = None,
    params: BodyDynamicsParams | None = None,
) -> Mapping[Dimension, float]:
    """The channels composed: what fraction of the baseline this body still has.

    Multiplicative, so two half-costs do not add up to a body that cannot stand,
    and floored, because a body at the floor is unable rather than negative.
    """
    params = params or load_params()
    losses = channel_losses(state, environment=environment, traits=traits, params=params)
    retention: dict[Dimension, float] = {dimension: 1.0 for dimension in Dimension}
    for per_dimension in losses.values():
        for dimension, loss in per_dimension.items():
            retention[dimension] *= 1.0 - loss
    floor = params.degradation.retention_floor
    return {dimension: max(floor, value) for dimension, value in retention.items()}


def impairment_multipliers(
    state: BodyState, *, params: BodyDynamicsParams | None = None
) -> Mapping[Dimension, float]:
    """Injury impairments, per dimension, fading as the lesion heals.

    Only the dimensions a lesion actually names are touched. Two open injuries
    compound, which is the honest reading of two damaged tissues.
    """
    params = params or load_params()
    multipliers: dict[Dimension, float] = {dimension: 1.0 for dimension in Dimension}
    for injury in state.open_injuries():
        remaining = 1.0 - injury.healing.progress
        for dimension, multiplier in injury.impairments.items():
            multipliers[dimension] *= 1.0 - (1.0 - multiplier) * remaining
    return multipliers


def apply_retention(dimension: Dimension, baseline: float, retention: float) -> float:
    """Apply a retention factor in the direction the dimension is read.

    A retention of 0.8 means a body producing four fifths of what it could — a
    shorter jump, and a *longer* reaction time. Getting this backwards would make
    a fatigued body quicker, which is the kind of bug that survives review
    because every number involved still looks reasonable.
    """
    if DIMENSION_UNITS[dimension].orientation == "LOWER_IS_BETTER":
        return baseline / retention
    return baseline * retention


def capability_available(
    baseline: CapacityProfile,
    state: BodyState,
    *,
    environment: Environment = NEUTRAL_ENVIRONMENT,
    traits: BodyTraits | None = None,
    params: BodyDynamicsParams | None = None,
) -> Mapping[Dimension, float]:
    """`capacity_baseline ⊗ impairments ⊗ state ⊗ environment`, per dimension.

    In the physical unit each dimension is stored in, never as a percentile and
    never as a single scalar. World truth: it stays in the engine.
    """
    params = params or load_params()
    traits = traits or BodyTraits.from_profile(baseline)
    retention = state_retention(state, environment=environment, traits=traits, params=params)
    impairments = impairment_multipliers(state, params=params)
    floor = params.degradation.retention_floor
    return {
        dimension: apply_retention(
            dimension,
            baseline.value(dimension),
            max(floor, retention[dimension] * impairments[dimension]),
        )
        for dimension in Dimension
    }
