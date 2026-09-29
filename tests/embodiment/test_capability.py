"""`capability_available`, composed per dimension — never as one scalar.

The tests that matter here are the negative ones. A composition that degrades
everything a little whenever anything is wrong would pass a naive "does fatigue
reduce capability" check and would still be the failure mode the model exists to
prevent: an impairment that applies equally to every dimension (F12), a thermal
penalty attached to the cohort rather than to the conditions (F16), a fatigued
body with a *quicker* reaction because a multiplier went the wrong way.
"""

from __future__ import annotations

import pytest

from embodiment.capability import (
    capability_available,
    channel_losses,
    impairment_multipliers,
    state_retention,
)
from embodiment.dynamics import (
    BodyTraits,
    Environment,
    Exposure,
    IntervalLoad,
    advance,
    initial_body_state,
)
from embodiment.types import (
    BodyState,
    Dimension,
    FatigueRegion,
    Healing,
    Injury,
    InjuryMechanism,
    InjuryRegion,
    InjurySeverity,
    InjuryTissue,
    PainProfile,
    SleepQuality,
)

#: The five categories the sleep-deprivation meta-analysis actually measured,
#: with the dimension each is stored as. The numbers are standardised mean
#: differences: skill control -0.87 down to maximal strength -0.35.
SLEEP_ORDERING = (
    Dimension.COORDINATION,
    Dimension.AEROBIC_CAPACITY,
    Dimension.ANAEROBIC_POWER,
    Dimension.SPRINT_SPEED,
    Dimension.MAX_STRENGTH,
)


def replace_channel(state: BodyState, **sections) -> BodyState:
    fields = {name: getattr(state, name) for name in BodyState.model_fields}
    fields.update(sections)
    return BodyState(**fields)


def wrist_injury() -> Injury:
    """The lesion of the model's own example: grip and throwing, not running."""
    return Injury(
        region=InjuryRegion.WRIST_RIGHT,
        tissue=InjuryTissue.LIGAMENT,
        mechanism=InjuryMechanism.TWIST,
        severity=InjurySeverity.MODERATE,
        onset_h=0.0,
        healing=Healing(expected_days=21.0, progress=0.0, setback_on_load=0.05),
        impairments={Dimension.MAX_STRENGTH: 0.45, Dimension.ANAEROBIC_POWER: 0.6},
        pain_profile=PainProfile(at_rest=0.3, on_use=0.9),
        observable_cues=("guarding", "bandage"),
    )


def test_a_rested_body_in_a_neutral_room_is_its_own_baseline(reference_profile, rested_body) -> None:
    available = capability_available(reference_profile, rested_body)
    for dimension in Dimension:
        assert available[dimension] == pytest.approx(reference_profile.value(dimension))


def test_a_wrist_injury_takes_grip_and_throwing_and_leaves_sprinting_alone(
    reference_profile, rested_body
) -> None:
    """Failure mode F12, as the model states it.

    The test fails if **any** unrelated dimension moves, which is the only way to
    check that a per-dimension impairment is per-dimension and not a global
    penalty with a per-dimension table in front of it.
    """
    hurt = replace_channel(rested_body, injuries=(wrist_injury(),))
    available = capability_available(reference_profile, hurt)
    baseline = {dimension: reference_profile.value(dimension) for dimension in Dimension}

    assert available[Dimension.MAX_STRENGTH] < baseline[Dimension.MAX_STRENGTH]
    assert available[Dimension.ANAEROBIC_POWER] < baseline[Dimension.ANAEROBIC_POWER]
    for dimension in Dimension:
        if dimension in (Dimension.MAX_STRENGTH, Dimension.ANAEROBIC_POWER):
            continue
        assert available[dimension] == pytest.approx(baseline[dimension]), (
            f"{dimension.value} moved on a wrist injury that never named it"
        )


def test_the_exempt_dimensions_are_exactly_the_ones_the_file_justifies(dynamics_params) -> None:
    """The list is what keeps the composition total, so it is pinned here.

    Six constitutional traits plus `flexibility`, which is on the list for the
    other reason the parameter file gives: it is a real performance with a
    battery item, exempt only because no channel has a sourced effect size for
    range of motion. Adding a dimension without adding its reason is how a
    declared blank turns into a silent one.
    """
    assert set(dynamics_params.degradation.unmodulated_dimensions) == {
        Dimension.BODY_MASS,
        Dimension.STATURE,
        Dimension.INJURY_RESILIENCE,
        Dimension.RECOVERY_RATE,
        Dimension.THERMOREGULATION,
        Dimension.PAIN_TOLERANCE,
        Dimension.FLEXIBILITY,
    }


def test_no_channel_names_a_dimension_it_is_also_exempt_from(dynamics_params) -> None:
    """A dimension cannot be both degraded by a named channel and exempt."""
    degradation = dynamics_params.degradation
    exempt = set(degradation.unmodulated_dimensions)
    for table in (
        degradation.peripheral_fatigue_loss_at_full,
        degradation.w_prime_loss_at_empty,
        degradation.central_fatigue_loss_at_full,
        degradation.soreness_loss_at_full,
        degradation.substrate_loss_at_empty,
        degradation.sleep_sensitivity,
    ):
        assert not (set(table) & exempt), sorted(d.value for d in set(table) & exempt)


def test_an_exempt_dimension_is_still_reachable_by_a_lesion(rested_body) -> None:
    """Exempt from *state*, not from injury: the two are different mechanisms."""
    stiff = replace_channel(
        rested_body,
        injuries=(
            Injury(
                region=InjuryRegion.LOWER_BACK,
                tissue=InjuryTissue.MUSCLE,
                mechanism=InjuryMechanism.STRAIN,
                severity=InjurySeverity.MINOR,
                onset_h=0.0,
                healing=Healing(expected_days=14.0, progress=0.0),
                impairments={Dimension.FLEXIBILITY: 0.6},
            ),
        ),
    )
    assert impairment_multipliers(stiff)[Dimension.FLEXIBILITY] < 1.0
    assert impairment_multipliers(stiff)[Dimension.SPRINT_SPEED] == 1.0


def test_an_impairment_that_touches_every_dimension_equally_is_refused() -> None:
    """It is failure mode F12 by definition, so the type refuses to hold it."""
    with pytest.raises(ValueError, match="F12"):
        Injury(
            region=InjuryRegion.KNEE_LEFT,
            tissue=InjuryTissue.JOINT,
            mechanism=InjuryMechanism.IMPACT,
            severity=InjurySeverity.MINOR,
            onset_h=0.0,
            healing=Healing(expected_days=7.0, progress=0.0),
            impairments={dimension: 0.8 for dimension in Dimension},
        )


def test_an_impairment_fades_as_the_lesion_heals(rested_body) -> None:
    fresh = replace_channel(rested_body, injuries=(wrist_injury(),))
    mending = replace_channel(
        rested_body,
        injuries=(
            wrist_injury().model_copy(
                update={"healing": Healing(expected_days=21.0, progress=0.7, setback_on_load=0.05)}
            ),
        ),
    )
    assert (
        impairment_multipliers(mending)[Dimension.MAX_STRENGTH]
        > impairment_multipliers(fresh)[Dimension.MAX_STRENGTH]
    )


def test_a_degraded_body_reacts_more_slowly_rather_than_faster(reference_profile, rested_body, dynamics_params) -> None:
    """Orientation: `reaction_time` is in ms, so degradation makes it larger.

    Getting this backwards makes a fatigued body quicker, and every number
    involved still looks reasonable.
    """
    tired = replace_channel(rested_body, central_fatigue=0.8)
    available = capability_available(reference_profile, tired)
    assert available[Dimension.REACTION_TIME] > reference_profile.value(Dimension.REACTION_TIME)
    assert available[Dimension.COORDINATION] < reference_profile.value(Dimension.COORDINATION)


def test_sleep_loss_keeps_the_ordering_the_meta_analysis_supports(rested_body, dynamics_params) -> None:
    """Skill ≫ aerobic ≳ power > speed ≫ strength — as **ratios**, never values.

    The absolute size of the effect is `[INT]`; the ordering and the ratios
    between the dimensions are what the meta-analysis carries, so that is what is
    asserted.
    """
    deprived = advance(rested_body, 20.0, Exposure.of(params=dynamics_params))
    losses = channel_losses(deprived, params=dynamics_params)["sleep"]

    ordered = [losses[dimension] for dimension in SLEEP_ORDERING]
    assert ordered == sorted(ordered, reverse=True)

    sensitivity = dynamics_params.degradation.sleep_sensitivity
    expected = sensitivity[Dimension.COORDINATION] / sensitivity[Dimension.MAX_STRENGTH]
    assert losses[Dimension.COORDINATION] / losses[Dimension.MAX_STRENGTH] == pytest.approx(expected)
    assert expected == pytest.approx(0.87 / 0.35)


def test_hydration_below_the_threshold_costs_nothing_and_above_it_degrades(
    rested_body, dynamics_params
) -> None:
    """The 2 % of body mass is measured; what a point above it costs is not."""
    threshold = dynamics_params.channels.hydration.threshold_pct_body_mass

    def aerobic_loss(deficit: float) -> float:
        body = replace_channel(
            rested_body,
            hydration=rested_body.hydration.model_copy(update={"deficit_pct_body_mass": deficit}),
        )
        return channel_losses(body, params=dynamics_params)["hydration"][Dimension.AEROBIC_CAPACITY]

    assert aerobic_loss(threshold * 0.5) == 0.0
    assert aerobic_loss(threshold) == 0.0
    assert 0.0 < aerobic_loss(threshold + 1.0) < aerobic_loss(threshold + 3.0)


def test_the_same_deficit_costs_more_in_the_heat(rested_body, dynamics_params) -> None:
    dry = replace_channel(
        rested_body,
        hydration=rested_body.hydration.model_copy(update={"deficit_pct_body_mass": 4.0}),
    )
    cool = channel_losses(dry, environment=Environment(wbgt_c=18.0), params=dynamics_params)
    hot = channel_losses(dry, environment=Environment(wbgt_c=32.0), params=dynamics_params)
    assert hot["hydration"][Dimension.AEROBIC_CAPACITY] > cool["hydration"][Dimension.AEROBIC_CAPACITY]


def test_heat_is_indexed_to_conditions_and_to_tolerance_never_to_a_cohort(
    rested_body, dynamics_params
) -> None:
    """Failure mode F16: no thermal penalty for being this age.

    Two bodies in the same heat differ only by their own `thermoregulation`, and
    the room is what decides the rest. There is no cohort term to test for
    because there is no cohort term to have.
    """
    hardy = BodyTraits.reference(dynamics_params).model_copy(update={"thermoregulation_c": 31.0})
    delicate = BodyTraits.reference(dynamics_params).model_copy(update={"thermoregulation_c": 26.0})
    hot = Environment(wbgt_c=30.0)

    hardy_loss = channel_losses(rested_body, environment=hot, traits=hardy, params=dynamics_params)
    delicate_loss = channel_losses(
        rested_body, environment=hot, traits=delicate, params=dynamics_params
    )
    assert hardy_loss["thermal"][Dimension.AEROBIC_CAPACITY] == 0.0
    assert delicate_loss["thermal"][Dimension.AEROBIC_CAPACITY] > 0.0

    mild = channel_losses(
        rested_body, environment=Environment(wbgt_c=20.0), traits=delicate, params=dynamics_params
    )
    assert mild["thermal"][Dimension.AEROBIC_CAPACITY] == 0.0


def test_heat_is_the_one_channel_that_reaches_every_modulated_dimension(
    rested_body, dynamics_params
) -> None:
    delicate = BodyTraits.reference(dynamics_params).model_copy(update={"thermoregulation_c": 24.0})
    losses = channel_losses(
        rested_body, environment=Environment(wbgt_c=33.0), traits=delicate, params=dynamics_params
    )["thermal"]
    modulated = set(Dimension) - set(dynamics_params.degradation.unmodulated_dimensions)
    assert set(losses) == modulated
    assert all(loss > 0.0 for loss in losses.values())


def test_traits_that_govern_the_dynamics_are_not_worn_down_by_them(
    reference_profile, rested_body, dynamics_params
) -> None:
    """Fatigue does not make a body shorter, and heat does not lower its pain tolerance."""
    wrecked = replace_channel(
        rested_body,
        central_fatigue=1.0,
        peripheral_fatigue={region: 1.0 for region in FatigueRegion},
        energy=rested_body.energy.model_copy(update={"substrate_availability": 0.0}),
        hydration=rested_body.hydration.model_copy(update={"deficit_pct_body_mass": 8.0}),
    )
    available = capability_available(
        reference_profile, wrecked, environment=Environment(wbgt_c=35.0)
    )
    for dimension in dynamics_params.degradation.unmodulated_dimensions:
        assert available[dimension] == pytest.approx(reference_profile.value(dimension))


def test_every_dimension_is_either_degraded_by_a_channel_or_declared_untouched(
    dynamics_params,
) -> None:
    """The composition is total, so nothing is left out by omission."""
    reached = set(dynamics_params.degradation.unmodulated_dimensions)
    degradation = dynamics_params.degradation
    for table in (
        degradation.peripheral_fatigue_loss_at_full,
        degradation.w_prime_loss_at_empty,
        degradation.central_fatigue_loss_at_full,
        degradation.soreness_loss_at_full,
        degradation.substrate_loss_at_empty,
        degradation.sleep_sensitivity,
    ):
        reached |= set(table)
    reached |= set(dynamics_params.channels.hydration.degrades)
    assert reached == set(Dimension)


def test_the_composition_is_floored_rather_than_allowed_to_reach_zero(
    reference_profile, rested_body, dynamics_params
) -> None:
    """A body at the floor is unable, not negative."""
    wrecked = replace_channel(
        rested_body,
        central_fatigue=1.0,
        peripheral_fatigue={region: 1.0 for region in FatigueRegion},
        energy=rested_body.energy.model_copy(update={"substrate_availability": 0.0}),
        hydration=rested_body.hydration.model_copy(update={"deficit_pct_body_mass": 12.0}),
        sleep=rested_body.sleep.model_copy(update={"debt_hours": 40.0}),
        injuries=(wrist_injury(),),
    )
    retention = state_retention(
        wrecked, environment=Environment(wbgt_c=38.0), params=dynamics_params
    )
    floor = dynamics_params.degradation.retention_floor
    assert all(floor <= value <= 1.0 for value in retention.values())
    available = capability_available(
        reference_profile, wrecked, environment=Environment(wbgt_c=38.0)
    )
    assert all(value > 0.0 for value in available.values())


def test_capability_is_returned_in_the_unit_each_dimension_is_stored_in(
    reference_profile, rested_body
) -> None:
    """Invariant 9: a physical unit, never a percentile and never a scalar."""
    available = capability_available(reference_profile, rested_body)
    assert set(available) == set(Dimension)
    assert available[Dimension.MAX_STRENGTH] > 5.0  # kilograms of grip, not a fraction
    assert available[Dimension.REACTION_TIME] > 50.0  # milliseconds
    assert 0.0 < available[Dimension.COORDINATION] <= 1.0  # a hit rate


def test_work_that_empties_the_reserve_costs_power_and_speed_and_nothing_else(
    reference_profile, rested_body, dynamics_params
) -> None:
    spent = advance(
        rested_body,
        0.2,
        Exposure.of(
            load=IntervalLoad(intensity=1.4, by_region={FatigueRegion.LEGS: 1.0}, activity="sprint"),
            params=dynamics_params,
        ),
    )
    losses = channel_losses(spent, params=dynamics_params)["w_prime_balance"]
    assert set(losses) == {Dimension.ANAEROBIC_POWER, Dimension.SPRINT_SPEED}
    assert all(loss > 0.0 for loss in losses.values())
