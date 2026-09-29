"""One test per channel, with the time scales of `physical-model.md` §7.

Four questions per channel, because they are the four ways a channel is wrong:
does it evolve under `Δt`, does it sit still at rest, does it move in the right
direction, and does it saturate instead of running off the end. On top of those,
the numbers the model actually sources — the W' reconstitution constants, the
2 % hydration threshold, the DOMS window, the ordering of sleep loss — are
asserted here rather than trusted, because they are the part of the parameter
file that claims to come from somewhere.

Everything here is deterministic. There is no seed in this module because there
is no draw in the code it tests: the injury roll is PSV1-4's.
"""

from __future__ import annotations

import pytest

from embodiment.dynamics import (
    BodyDynamicsError,
    BodyTraits,
    Environment,
    Exposure,
    IntegrationParams,
    IntervalLoad,
    advance,
    advance_central_fatigue,
    advance_energy,
    advance_hydration,
    advance_peripheral_fatigue,
    advance_sleep,
    advance_thermal,
    advance_w_prime_balance,
    initial_body_state,
    reconstitution_tau_s,
    sleep_pressure,
)
from embodiment.types import (
    BodyState,
    FatigueRegion,
    Healing,
    Injury,
    InjuryMechanism,
    InjuryRegion,
    InjurySeverity,
    InjuryTissue,
    PainProfile,
    SleepQuality,
    WPrimeBalance,
)


def exposure(dynamics_params, traits=None, **kwargs) -> Exposure:
    environment = kwargs.pop("environment", Environment())
    load = kwargs.pop("load", IntervalLoad())
    return Exposure.of(
        environment=environment,
        load=load,
        traits=traits or BodyTraits.reference(dynamics_params),
        params=dynamics_params,
    )


def work(intensity: float, *regions: FatigueRegion, eccentric: float = 0.0, activity: str = "drill") -> IntervalLoad:
    return IntervalLoad(
        intensity=intensity,
        by_region={region: 1.0 for region in regions} or {region: 1.0 for region in FatigueRegion},
        eccentric_fraction=eccentric,
        activity=activity,
    )


def replace_channel(state: BodyState, **sections) -> BodyState:
    """Build a variant body through the constructor, so the domains are checked."""
    fields = {name: getattr(state, name) for name in BodyState.model_fields}
    fields.update(sections)
    return BodyState(**fields)


# --------------------------------------------------------------------------
# Channel 1 — w_prime_balance
# --------------------------------------------------------------------------


def test_w_prime_tau_is_within_the_measured_band_and_rises_with_recovery_intensity(dynamics_params) -> None:
    """τ ≈ 380-580 s, and **larger** the harder the recovery (research §4).

    The direction is the counter-intuitive part: recovering harder does not
    refill the reserve faster. A file that had this backwards would still produce
    plausible numbers, which is why the direction is asserted and not read.
    """
    taus = [reconstitution_tau_s(fraction / 10.0, dynamics_params) for fraction in range(11)]
    assert taus == sorted(taus)
    assert taus[0] == pytest.approx(377.0)
    assert taus[-1] == pytest.approx(580.0)
    assert all(377.0 <= tau <= 580.0 for tau in taus)


def test_w_prime_reconstitutes_below_critical_power_and_is_a_fixed_point_when_full(
    rested_body, dynamics_params
) -> None:
    half = replace_channel(
        rested_body,
        w_prime_balance=WPrimeBalance(
            remaining_j=rested_body.w_prime_balance.capacity_j / 2.0,
            capacity_j=rested_body.w_prime_balance.capacity_j,
            tau_s=rested_body.w_prime_balance.tau_s,
        ),
    )
    recovered = advance(half, 0.25, exposure(dynamics_params))
    assert recovered.w_prime_balance.remaining_j > half.w_prime_balance.remaining_j

    full = advance(rested_body, 1.0, exposure(dynamics_params))
    assert full.w_prime_balance.remaining_j == pytest.approx(
        rested_body.w_prime_balance.capacity_j
    )


def test_w_prime_drains_above_critical_power_and_saturates_at_empty(rested_body, dynamics_params) -> None:
    hard = exposure(dynamics_params, load=work(1.0, FatigueRegion.LEGS))
    after_a_minute = advance_w_prime_balance(rested_body, 1.0 / 60.0, hard)
    assert after_a_minute.w_prime_balance.remaining_j < rested_body.w_prime_balance.capacity_j

    emptied = advance_w_prime_balance(rested_body, 2.0, hard)
    assert emptied.w_prime_balance.remaining_j == 0.0


def test_w_prime_recovers_less_when_the_recovery_intensity_is_higher(rested_body, dynamics_params) -> None:
    """The measured direction, as behaviour rather than as a constant."""
    critical = dynamics_params.channels.w_prime_balance.critical_power_fraction
    half = replace_channel(
        rested_body,
        w_prime_balance=WPrimeBalance(
            remaining_j=rested_body.w_prime_balance.capacity_j / 2.0,
            capacity_j=rested_body.w_prime_balance.capacity_j,
            tau_s=rested_body.w_prime_balance.tau_s,
        ),
    )
    idle = advance_w_prime_balance(half, 0.1, exposure(dynamics_params, load=work(0.0)))
    brisk = advance_w_prime_balance(
        half, 0.1, exposure(dynamics_params, load=work(critical * 0.95, FatigueRegion.LEGS))
    )
    assert idle.w_prime_balance.remaining_j > brisk.w_prime_balance.remaining_j
    assert brisk.w_prime_balance.tau_s > idle.w_prime_balance.tau_s


# --------------------------------------------------------------------------
# Channel 2 — peripheral_fatigue
# --------------------------------------------------------------------------


def test_peripheral_fatigue_accrues_only_where_the_work_was(rested_body, dynamics_params) -> None:
    worked = advance_peripheral_fatigue(
        rested_body, 1.0, exposure(dynamics_params, load=work(0.8, FatigueRegion.GRIP))
    )
    assert worked.peripheral_fatigue[FatigueRegion.GRIP] > 0.0
    for region in (FatigueRegion.LEGS, FatigueRegion.ARMS, FatigueRegion.CORE):
        assert worked.peripheral_fatigue[region] == 0.0


def test_peripheral_fatigue_decays_to_zero_at_rest_and_zero_is_a_fixed_point(
    rested_body, dynamics_params
) -> None:
    tired = replace_channel(
        rested_body, peripheral_fatigue={region: 0.8 for region in FatigueRegion}
    )
    rested = advance(tired, 12.0, exposure(dynamics_params))
    for region in FatigueRegion:
        assert 0.0 < rested.peripheral_fatigue[region] < 0.8

    still_fresh = advance(rested_body, 12.0, exposure(dynamics_params))
    assert all(value == 0.0 for value in still_fresh.peripheral_fatigue.values())


def test_peripheral_fatigue_saturates_below_one(rested_body, dynamics_params) -> None:
    hammered = advance_peripheral_fatigue(
        rested_body, 48.0, exposure(dynamics_params, load=work(2.0, *FatigueRegion))
    )
    assert all(0.0 < value <= 1.0 for value in hammered.peripheral_fatigue.values())


def test_a_body_that_recovers_faster_sheds_fatigue_faster(rested_body, dynamics_params) -> None:
    """`recovery_rate` is a time constant, so a lower value is a faster body."""
    tired = replace_channel(
        rested_body, peripheral_fatigue={region: 0.6 for region in FatigueRegion}
    )
    quick = BodyTraits.reference(dynamics_params).model_copy(update={"recovery_rate_h": 12.0})
    slow = BodyTraits.reference(dynamics_params).model_copy(update={"recovery_rate_h": 36.0})
    assert (
        advance(tired, 8.0, exposure(dynamics_params, traits=quick)).peripheral_fatigue[FatigueRegion.LEGS]
        < advance(tired, 8.0, exposure(dynamics_params, traits=slow)).peripheral_fatigue[FatigueRegion.LEGS]
    )


# --------------------------------------------------------------------------
# Channel 3 — central_fatigue
# --------------------------------------------------------------------------


def test_central_fatigue_rises_with_work_and_falls_faster_asleep(rested_body, dynamics_params) -> None:
    worked = advance(rested_body, 2.0, exposure(dynamics_params, load=work(0.9, *FatigueRegion)))
    assert worked.central_fatigue > rested_body.central_fatigue

    awake = advance_central_fatigue(worked, 6.0, exposure(dynamics_params))
    asleep = advance_central_fatigue(
        worked, 6.0, exposure(dynamics_params, environment=Environment(asleep=True))
    )
    assert asleep.central_fatigue < awake.central_fatigue < worked.central_fatigue


def test_central_fatigue_is_a_fixed_point_at_zero_with_no_load_and_no_sleep_pressure(
    rested_body, dynamics_params
) -> None:
    """Zero is a fixed point — but only for a body that owes the night nothing.

    A rested body kept awake accrues a little of it anyway, because sleep
    pressure feeds this channel by design (both rows are in the model's table).
    That is behaviour, not drift, so the fixed point is stated where it actually
    holds and the cycle is checked separately below.
    """
    asleep = exposure(dynamics_params, environment=Environment(asleep=True))
    assert advance(rested_body, 6.0, asleep).central_fatigue == pytest.approx(0.0)


def test_central_fatigue_does_not_ratchet_across_well_slept_days(rested_body, dynamics_params) -> None:
    """A day costs something, a full night pays most of it back, and the rest
    settles at a small standing level instead of climbing week after week.

    The steady state is not zero, and should not be: a body awake fifteen hours a
    day carries a little central fatigue all the time. What would be wrong is a
    ratchet, so what is asserted is convergence.
    """
    need = dynamics_params.channels.sleep.need_hours_per_day
    night = exposure(
        dynamics_params, environment=Environment(asleep=True, sleep_quality=SleepQuality.GOOD)
    )
    body = rested_body
    mornings = []
    for _ in range(6):
        body = advance(advance(body, 24.0 - need, exposure(dynamics_params)), need, night)
        mornings.append(body.central_fatigue)
    assert body.sleep.debt_hours == pytest.approx(0.0)
    steps = [later - earlier for earlier, later in zip(mornings, mornings[1:])]
    assert all(step > 0 for step in steps)
    assert steps[-1] < steps[0] / 10.0
    assert mornings[-1] < 0.1


def test_central_fatigue_saturates_below_one(rested_body, dynamics_params) -> None:
    for hours in (24.0, 240.0):
        driven = advance_central_fatigue(
            rested_body, hours, exposure(dynamics_params, load=work(2.0, *FatigueRegion))
        )
        assert 0.0 < driven.central_fatigue <= 1.0


# --------------------------------------------------------------------------
# Channel 4 — sleep
# --------------------------------------------------------------------------


def test_a_full_night_leaves_the_debt_where_it_was(rested_body, dynamics_params) -> None:
    """The fixed point of the channel: sleeping what the day costs owes nothing."""
    need = dynamics_params.channels.sleep.need_hours_per_day
    awake = advance(rested_body, 24.0 - need, exposure(dynamics_params))
    slept = advance(
        awake,
        need,
        exposure(dynamics_params, environment=Environment(asleep=True, sleep_quality=SleepQuality.GOOD)),
    )
    assert slept.sleep.debt_hours == pytest.approx(0.0, abs=1e-9)
    assert slept.sleep.hours_since_wake == 0.0
    assert slept.sleep.last_sleep is not None
    assert slept.sleep.last_sleep.quality is SleepQuality.GOOD


def test_a_short_bad_night_leaves_a_debt_that_the_next_day_inherits(rested_body, dynamics_params) -> None:
    night = exposure(
        dynamics_params, environment=Environment(asleep=True, sleep_quality=SleepQuality.POOR)
    )
    day_one = advance(advance(rested_body, 20.0, exposure(dynamics_params)), 4.0, night)
    day_two = advance(advance(day_one, 20.0, exposure(dynamics_params)), 4.0, night)
    assert day_two.sleep.debt_hours > day_one.sleep.debt_hours > 0.0


def test_worse_sleep_repays_less_of_the_same_night(rested_body, dynamics_params) -> None:
    indebted = advance(rested_body, 20.0, exposure(dynamics_params))
    repaid = {
        quality: advance_sleep(
            indebted, 8.0, exposure(dynamics_params, environment=Environment(asleep=True, sleep_quality=quality))
        ).sleep.debt_hours
        for quality in SleepQuality
    }
    assert repaid[SleepQuality.GOOD] < repaid[SleepQuality.FAIR] < repaid[SleepQuality.POOR]


def test_the_debt_saturates_rather_than_diverging(rested_body, dynamics_params) -> None:
    unslept = advance(rested_body, 30.0 * 24.0, exposure(dynamics_params))
    assert unslept.sleep.debt_hours == pytest.approx(dynamics_params.channels.sleep.debt_cap_hours)


def test_the_circadian_phase_advances_whatever_the_body_does(rested_body, dynamics_params) -> None:
    awake = advance(rested_body, 30.0, exposure(dynamics_params))
    asleep = advance(
        rested_body, 30.0, exposure(dynamics_params, environment=Environment(asleep=True))
    )
    assert awake.sleep.circadian_phase == pytest.approx(asleep.sleep.circadian_phase)
    assert 0.0 <= awake.sleep.circadian_phase < 24.0


def test_sleep_pressure_rises_with_debt_and_peaks_at_the_circadian_trough(
    rested_body, dynamics_params
) -> None:
    trough = dynamics_params.channels.sleep.process_c.trough_hour
    indebted = advance(rested_body, 20.0, exposure(dynamics_params))
    assert sleep_pressure(indebted, dynamics_params) > sleep_pressure(rested_body, dynamics_params)

    at_trough = replace_channel(
        indebted, sleep=indebted.sleep.model_copy(update={"circadian_phase": trough})
    )
    at_peak = replace_channel(
        indebted, sleep=indebted.sleep.model_copy(update={"circadian_phase": (trough + 12.0) % 24.0})
    )
    assert sleep_pressure(at_trough, dynamics_params) > sleep_pressure(at_peak, dynamics_params)


# --------------------------------------------------------------------------
# Channel 5 — energy / substrate
# --------------------------------------------------------------------------


def test_substrate_is_full_and_stays_full_on_a_normal_diet_at_rest(rested_body, dynamics_params) -> None:
    assert advance(rested_body, 24.0, exposure(dynamics_params)).energy.substrate_availability == pytest.approx(1.0)


def test_substrate_burns_exponentially_faster_with_intensity(rested_body, dynamics_params) -> None:
    """The measured shape: the rate rises exponentially with intensity."""
    def burnt(intensity: float) -> float:
        #: Measured with nothing coming in, so what is seen is the burn and not
        #: the balance between burn and repletion.
        after = advance_energy(
            rested_body,
            1.0,
            exposure(
                dynamics_params,
                environment=Environment(carbohydrate_intake=0.0),
                load=work(intensity, *FatigueRegion),
            ),
        )
        return 1.0 - after.energy.substrate_availability

    low, middle, high = burnt(0.0), burnt(0.5), burnt(1.0)
    assert low < middle < high
    assert (high - middle) > (middle - low)


def test_a_poor_carbohydrate_diet_keeps_substrate_low_for_days(rested_body, dynamics_params) -> None:
    """The channel that makes a multi-day exam a multi-day exam.

    Three days of work on a quarter ration: each morning starts lower than the
    last, which is the whole difference between a state that persists and a
    modifier of the day.
    """
    rationed = Environment(carbohydrate_intake=0.25)
    day = exposure(
        dynamics_params, environment=rationed, load=work(0.35, *FatigueRegion, activity="march")
    )
    night = exposure(
        dynamics_params,
        environment=Environment(carbohydrate_intake=0.25, asleep=True, sleep_quality=SleepQuality.FAIR),
    )
    mornings = []
    body = rested_body
    for _ in range(3):
        mornings.append(body.energy.substrate_availability)
        body = advance(advance(body, 16.0, day), 8.0, night)
    assert mornings[2] < mornings[1] < mornings[0]
    assert mornings[2] < 0.7


def test_dehydration_accelerates_the_burn(rested_body, dynamics_params) -> None:
    """The coupling the review names: dehydration increases glycogen use."""
    threshold = dynamics_params.channels.hydration.threshold_pct_body_mass
    dry = replace_channel(
        rested_body,
        hydration=rested_body.hydration.model_copy(
            update={"deficit_pct_body_mass": threshold + 3.0}
        ),
    )
    load = work(0.6, *FatigueRegion)
    wet_burn = 1.0 - advance_energy(rested_body, 2.0, exposure(dynamics_params, load=load)).energy.substrate_availability
    dry_burn = 1.0 - advance_energy(dry, 2.0, exposure(dynamics_params, load=load)).energy.substrate_availability
    assert dry_burn > wet_burn


def test_substrate_saturates_at_both_ends(rested_body, dynamics_params) -> None:
    starved = advance_energy(
        rested_body,
        72.0,
        exposure(dynamics_params, environment=Environment(carbohydrate_intake=0.0), load=work(1.5, *FatigueRegion)),
    )
    assert starved.energy.substrate_availability == 0.0
    assert advance(starved, 72.0, exposure(dynamics_params)).energy.substrate_availability == pytest.approx(1.0)


# --------------------------------------------------------------------------
# Channel 6 — hydration
# --------------------------------------------------------------------------


def test_a_deficit_accrues_in_heat_and_is_replaced_at_rest(rested_body, dynamics_params) -> None:
    hot = exposure(
        dynamics_params, environment=Environment(wbgt_c=31.0), load=work(0.8, *FatigueRegion)
    )
    sweating = advance(rested_body, 3.0, hot)
    assert sweating.hydration.deficit_pct_body_mass > 0.0

    drinking = advance(sweating, 6.0, exposure(dynamics_params))
    assert drinking.hydration.deficit_pct_body_mass == 0.0


def test_hydration_is_a_fixed_point_at_zero_in_a_neutral_room(rested_body, dynamics_params) -> None:
    assert advance(rested_body, 12.0, exposure(dynamics_params)).hydration.deficit_pct_body_mass == 0.0


def test_the_deficit_is_capped(rested_body, dynamics_params) -> None:
    parched = advance_hydration(
        rested_body,
        48.0,
        exposure(
            dynamics_params,
            environment=Environment(wbgt_c=34.0, fluid_access=0.0),
            load=work(1.0, *FatigueRegion),
        ),
    )
    assert parched.hydration.deficit_pct_body_mass == pytest.approx(
        dynamics_params.channels.hydration.max_deficit_pct_body_mass
    )


def test_rationed_fluid_leaves_a_deficit_that_full_access_would_have_cleared(
    rested_body, dynamics_params
) -> None:
    hot = Environment(wbgt_c=30.0)
    load = work(0.7, *FatigueRegion)
    rationed = advance(
        rested_body, 4.0, exposure(dynamics_params, environment=hot.model_copy(update={"fluid_access": 0.2}), load=load)
    )
    supplied = advance(rested_body, 4.0, exposure(dynamics_params, environment=hot, load=load))
    assert rationed.hydration.deficit_pct_body_mass > supplied.hydration.deficit_pct_body_mass


# --------------------------------------------------------------------------
# Channel 7 — thermal
# --------------------------------------------------------------------------


def test_core_offset_rises_with_heat_and_returns_to_neutral(rested_body, dynamics_params) -> None:
    hot = advance(
        rested_body,
        2.0,
        exposure(dynamics_params, environment=Environment(wbgt_c=32.0), load=work(0.8, *FatigueRegion)),
    )
    assert hot.thermal.core_offset_c > 0.0
    assert hot.thermal.wbgt == 32.0

    cooled = advance(hot, 4.0, exposure(dynamics_params))
    assert cooled.thermal.core_offset_c < 0.01 * hot.thermal.core_offset_c
    assert advance(hot, 12.0, exposure(dynamics_params)).thermal.core_offset_c == pytest.approx(
        0.0, abs=1e-6
    )


def test_core_offset_is_a_fixed_point_at_zero_in_a_neutral_room(rested_body, dynamics_params) -> None:
    assert advance(rested_body, 8.0, exposure(dynamics_params)).thermal.core_offset_c == 0.0


def test_core_offset_saturates(rested_body, dynamics_params) -> None:
    baked = advance_thermal(
        rested_body,
        24.0,
        exposure(
            dynamics_params,
            environment=Environment(wbgt_c=40.0, work_rest_ratio=3.0, clothing_insulation=2.0),
            load=work(1.5, *FatigueRegion),
        ),
    )
    assert 0.0 < baked.thermal.core_offset_c <= dynamics_params.channels.thermal.core_offset.max_offset_c


def test_dehydration_amplifies_the_heat_load(rested_body, dynamics_params) -> None:
    dry = replace_channel(
        rested_body,
        hydration=rested_body.hydration.model_copy(update={"deficit_pct_body_mass": 5.0}),
    )
    hot = exposure(
        dynamics_params, environment=Environment(wbgt_c=31.0), load=work(0.7, *FatigueRegion)
    )
    assert (
        advance_thermal(dry, 1.0, hot).thermal.core_offset_c
        > advance_thermal(rested_body, 1.0, hot).thermal.core_offset_c
    )


# --------------------------------------------------------------------------
# Channel 8 — soreness (DOMS)
# --------------------------------------------------------------------------


def bout(body: BodyState, dynamics_params, *, activity: str = "eccentric-drill", hours: float = 1.0) -> BodyState:
    return advance(
        body,
        hours,
        exposure(
            dynamics_params,
            load=work(0.9, FatigueRegion.LEGS, eccentric=1.0, activity=activity),
        ),
    )


def soreness_curve(body: BodyState, dynamics_params, hours: tuple[float, ...]) -> dict[float, float]:
    """Expressed soreness in the legs at each hour after a single bout."""
    curve: dict[float, float] = {}
    cursor = body
    elapsed = 0.0
    for mark in hours:
        cursor = advance(cursor, mark - elapsed, exposure(dynamics_params))
        elapsed = mark
        curve[mark] = cursor.soreness.by_region[FatigueRegion.LEGS].expressed
    return curve


def test_doms_is_absent_at_six_hours_present_at_a_day_peaks_on_the_second_and_resolves_in_a_week(
    rested_body, dynamics_params
) -> None:
    """Failure mode F11: soreness that shows up during the effort is the bug.

    Onset 12-24 h, peak 24-72 h, resolution ~7 d. Asserted as the shape of the
    curve rather than as absolute values, because the window is what the
    literature gives and the magnitude is `[INT]`.
    """
    kernel = dynamics_params.channels.soreness.kernel
    worked = bout(rested_body, dynamics_params)
    assert worked.soreness.by_region[FatigueRegion.LEGS].expressed == 0.0

    curve = soreness_curve(
        worked, dynamics_params, (6.0, kernel.onset_delay_h - 1.0, 24.0, 48.0, 72.0, 168.0)
    )
    peak = max(curve.values())

    #: Absent, as a value and not as a small number: nothing may be expressed
    #: before the gate opens, at six hours or at any hour up to the delay itself.
    assert curve[6.0] == 0.0
    assert curve[kernel.onset_delay_h - 1.0] == 0.0

    #: Present at a day, with a magnitude. `> 0.0` would also pass with an onset
    #: that had drifted to 23.9 h and left a float of order 1e-6 here, which is
    #: the acceptance criterion (F11) going quiet rather than being met.
    assert 0.10 * peak < curve[24.0] < 0.50 * peak

    assert curve[48.0] > 0.90 * peak
    assert curve[168.0] < 0.15 * peak
    assert curve[24.0] < curve[48.0]
    assert curve[72.0] < curve[48.0]


def test_doms_peaks_inside_the_measured_window(rested_body, dynamics_params) -> None:
    curve = soreness_curve(bout(rested_body, dynamics_params), dynamics_params, tuple(float(h) for h in range(1, 121)))
    peak_hour = max(curve, key=lambda hour: curve[hour])
    assert 24.0 <= peak_hour <= 72.0


def worked_body(rested_body, params, *, hours: float = 8.0) -> BodyState:
    """A body that has just done `hours` of fully eccentric work on every region."""
    load = IntervalLoad(
        intensity=0.8,
        by_region={region: 1.0 for region in FatigueRegion},
        eccentric_fraction=1.0,
        activity="drill",
    )
    return advance(
        rested_body,
        hours,
        Exposure.of(load=load, traits=BodyTraits.reference(params), params=params),
    )


def pending_entries(state: BodyState) -> int:
    return sum(len(region.pending_onset) for region in state.soreness.by_region.values())


def test_the_onset_queue_is_bounded_by_the_window_not_by_the_integration_step(
    rested_body, dynamics_params
) -> None:
    """World truth may not carry an artefact of how finely the engine integrated.

    The queue is persisted in the snapshot and hashed with it, so if its length
    followed `integration.step_hours` the snapshot of a body would change when a
    numerical setting changed and nothing about the body did.
    """
    kernel = dynamics_params.channels.soreness.kernel
    ceiling = len(FatigueRegion) * (
        int(kernel.onset_delay_h / kernel.onset_resolution_h) + 1
    )

    counts = set()
    for step_hours in (0.125, 0.25, 0.5, 1.0):
        params = dynamics_params.model_copy(
            update={
                "integration": IntegrationParams(
                    step_hours=step_hours,
                    max_interval_hours=dynamics_params.integration.max_interval_hours,
                )
            }
        )
        worked = worked_body(rested_body, params)
        counts.add(pending_entries(worked))
        assert pending_entries(worked) <= ceiling

    assert len(counts) == 1, f"the queue length follows the integration step: {counts}"


def test_the_onset_queue_holds_one_entry_per_release_instant(rested_body, dynamics_params) -> None:
    """Damage admitted by many steps into the same bucket stays one entry."""
    worked = worked_body(rested_body, dynamics_params)
    for region in worked.soreness.by_region.values():
        instants = [entry.release_at_h for entry in region.pending_onset]
        assert instants == sorted(instants)
        assert len(set(instants)) == len(instants)
        assert all(entry.amount > 0.0 for entry in region.pending_onset)


def test_the_onset_grid_never_releases_damage_before_the_measured_delay(
    rested_body, dynamics_params
) -> None:
    """Quantising the release rounds up, so 12 h stays a floor (F11)."""
    kernel = dynamics_params.channels.soreness.kernel
    worked = worked_body(rested_body, dynamics_params, hours=1.0)
    for region in worked.soreness.by_region.values():
        for entry in region.pending_onset:
            assert entry.release_at_h >= kernel.onset_delay_h


def test_the_queue_drains_and_leaves_nothing_behind(rested_body, dynamics_params) -> None:
    """A body that stops working owes no pending damage once the window passes."""
    worked = worked_body(rested_body, dynamics_params, hours=1.0)
    assert pending_entries(worked) > 0
    rested = advance(
        worked,
        dynamics_params.channels.soreness.kernel.onset_delay_h * 2.0,
        exposure(dynamics_params),
    )
    assert pending_entries(rested) == 0


def test_the_same_load_hurts_less_the_second_time(rested_body, dynamics_params) -> None:
    """The repeated-bout effect, per region **and** per activity.

    Without it, someone who trains every week suffers as if it were the first
    time, forever.
    """
    first = bout(rested_body, dynamics_params)
    first_peak = max(soreness_curve(first, dynamics_params, (24.0, 48.0, 72.0)).values())

    recovered = advance(first, 14.0 * 24.0, exposure(dynamics_params))
    second = bout(recovered, dynamics_params)
    second_peak = max(soreness_curve(second, dynamics_params, (24.0, 48.0, 72.0)).values())
    assert second_peak < first_peak


def test_the_repeated_bout_effect_does_not_transfer_to_another_activity(
    rested_body, dynamics_params
) -> None:
    trained = advance(bout(rested_body, dynamics_params, activity="squats"), 14.0 * 24.0, exposure(dynamics_params))
    same = bout(trained, dynamics_params, activity="squats")
    other = bout(trained, dynamics_params, activity="hill-run")
    assert (
        max(soreness_curve(same, dynamics_params, (48.0,)).values())
        < max(soreness_curve(other, dynamics_params, (48.0,)).values())
    )


def test_soreness_stays_in_the_region_that_earned_it(rested_body, dynamics_params) -> None:
    curve_body = advance(bout(rested_body, dynamics_params), 48.0, exposure(dynamics_params))
    assert curve_body.soreness.by_region[FatigueRegion.LEGS].expressed > 0.0
    for region in (FatigueRegion.ARMS, FatigueRegion.GRIP, FatigueRegion.CORE):
        assert curve_body.soreness.by_region[region].expressed == 0.0


# --------------------------------------------------------------------------
# Channel 9 — injuries
# --------------------------------------------------------------------------


def sprained_ankle(onset_h: float = 0.0, progress: float = 0.0) -> Injury:
    return Injury(
        region=InjuryRegion.ANKLE_LEFT,
        tissue=InjuryTissue.LIGAMENT,
        mechanism=InjuryMechanism.TWIST,
        severity=InjurySeverity.MODERATE,
        onset_h=onset_h,
        healing=Healing(expected_days=14.0, progress=progress, setback_on_load=0.05),
        impairments={"sprint_speed": 0.55, "agility": 0.4},
        pain_profile=PainProfile(at_rest=0.3, on_use=0.8),
        observable_cues=("limp",),
    )


def test_an_injury_heals_with_the_clock_and_leaves_the_body_when_it_is_done(
    rested_body, dynamics_params
) -> None:
    hurt = replace_channel(rested_body, injuries=(sprained_ankle(),))
    a_week = advance(hurt, 7.0 * 24.0, exposure(dynamics_params))
    assert 0.0 < a_week.injuries[0].healing.progress < 1.0

    a_month = advance(hurt, 30.0 * 24.0, exposure(dynamics_params))
    assert a_month.injuries == ()


def test_loading_an_injured_region_costs_progress(rested_body, dynamics_params) -> None:
    hurt = replace_channel(rested_body, injuries=(sprained_ankle(progress=0.5),))
    rested = advance(hurt, 24.0, exposure(dynamics_params))
    hobbled = advance(
        hurt, 24.0, exposure(dynamics_params, load=work(0.9, FatigueRegion.LEGS, activity="run"))
    )
    assert hobbled.injuries[0].healing.progress < rested.injuries[0].healing.progress


def test_loading_an_unrelated_region_does_not_cost_progress(rested_body, dynamics_params) -> None:
    """The fine injury taxonomy reaches the coarse one through a total map.

    An ankle is a leg; a grip drill is not. The mapping is what makes that a
    rule rather than a coincidence of naming.
    """
    hurt = replace_channel(rested_body, injuries=(sprained_ankle(progress=0.5),))
    rested = advance(hurt, 24.0, exposure(dynamics_params))
    gripping = advance(
        hurt, 24.0, exposure(dynamics_params, load=work(0.9, FatigueRegion.GRIP, activity="hang"))
    )
    assert gripping.injuries[0].healing.progress == pytest.approx(rested.injuries[0].healing.progress)


def test_a_faster_recovering_body_heals_a_lesion_sooner(rested_body, dynamics_params) -> None:
    hurt = replace_channel(rested_body, injuries=(sprained_ankle(),))
    quick = BodyTraits.reference(dynamics_params).model_copy(update={"recovery_rate_h": 12.0})
    slow = BodyTraits.reference(dynamics_params).model_copy(update={"recovery_rate_h": 36.0})
    assert (
        advance(hurt, 5.0 * 24.0, exposure(dynamics_params, traits=quick)).injuries[0].healing.progress
        > advance(hurt, 5.0 * 24.0, exposure(dynamics_params, traits=slow)).injuries[0].healing.progress
    )


# --------------------------------------------------------------------------
# The interval itself
# --------------------------------------------------------------------------


def test_an_interval_decomposes_exactly(rested_body, dynamics_params) -> None:
    """Advancing 24 h once equals advancing 12 h twice, on the fixed step.

    Without this, how a caller happens to chop up a day would change the body,
    and two runs of the same world would diverge on scene boundaries.
    """
    night = exposure(
        dynamics_params, environment=Environment(asleep=True, sleep_quality=SleepQuality.POOR)
    )
    once = advance(rested_body, 12.0, night)
    twice = advance(advance(rested_body, 6.0, night), 6.0, night)
    assert once.sleep.debt_hours == pytest.approx(twice.sleep.debt_hours)
    assert once.central_fatigue == pytest.approx(twice.central_fatigue)
    assert once.t_hours == pytest.approx(twice.t_hours)


def test_the_clock_does_not_run_backwards(rested_body) -> None:
    with pytest.raises(BodyDynamicsError, match="backwards"):
        advance(rested_body, -1.0)


def test_a_single_call_cannot_swallow_a_month(rested_body, dynamics_params) -> None:
    with pytest.raises(BodyDynamicsError, match="cap"):
        advance(rested_body, dynamics_params.integration.max_interval_hours + 1.0)


def test_a_sleeping_body_cannot_be_carrying_a_load(rested_body, dynamics_params) -> None:
    with pytest.raises(BodyDynamicsError, match="sleeping"):
        advance(
            rested_body,
            1.0,
            exposure(
                dynamics_params,
                environment=Environment(asleep=True),
                load=work(0.5, FatigueRegion.LEGS),
            ),
        )


def test_a_seeded_body_starts_rested_with_a_reserve_scaled_to_its_own_mass(
    reference_profile, dynamics_params
) -> None:
    from embodiment.types import Dimension

    body = initial_body_state("npc.0001", reference_profile, params=dynamics_params)
    expected = dynamics_params.channels.w_prime_balance.capacity_j.reference_j * (
        reference_profile.value(Dimension.BODY_MASS) / dynamics_params.reference_body.body_mass_kg
    )
    assert body.w_prime_balance.capacity_j == pytest.approx(expected)
    assert body.w_prime_balance.remaining_j == body.w_prime_balance.capacity_j
    assert body.central_fatigue == 0.0
    assert body.sleep.debt_hours == 0.0
    assert body.energy.substrate_availability == 1.0
    assert body.injuries == ()
    assert body.illnesses == ()
