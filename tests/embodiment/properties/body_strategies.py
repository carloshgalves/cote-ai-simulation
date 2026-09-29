"""Hypothesis strategies over `BodyState` and over what an interval can be.

Bodies are built through the real constructor, so every drawn body is a body the
engine could actually hold. Intervals are bounded by the parameter file's own cap
rather than by taste: a property that only holds for intervals the engine refuses
is not a property of the engine.
"""

from __future__ import annotations

from hypothesis import strategies as st

from embodiment.dynamics import Environment, IntervalLoad, initial_body_state, load_params
from embodiment.prior import PopulationPrior
from embodiment.rng import PURPOSE_CAPACITY_SAMPLE, SEEDING_EVENT_ID, substream
from embodiment.types import (
    BodyState,
    CumulativeLoad,
    Dimension,
    Energy,
    FatigueRegion,
    Healing,
    Hydration,
    Injury,
    InjuryMechanism,
    InjuryRegion,
    InjurySeverity,
    InjuryTissue,
    LastSleep,
    Pain,
    PainProfile,
    RegionSoreness,
    Sleep,
    SleepQuality,
    Soreness,
    SorenessOnset,
    Thermal,
    WPrimeBalance,
)

PARAMS = load_params()
STAGES = max(1, PARAMS.channels.soreness.kernel.latency_stages)
KERNEL = PARAMS.channels.soreness.kernel

#: One concrete body, for the `@example` the ticket asks each property to pin.
#: Drawn from the prior like every other body in this repository: a profile
#: written by hand in a test is the same failure mode as one in a data file.
REFERENCE_PROFILE = PopulationPrior.load().sample_profile(
    substream(20260909, "npc.0001", SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE), "male"
)
RESTED_BODY = initial_body_state("npc.0001", REFERENCE_PROFILE)

fractions = st.floats(min_value=0.0, max_value=1.0, allow_nan=False, allow_infinity=False)
small_positive = st.floats(min_value=0.1, max_value=1000.0, allow_nan=False, allow_infinity=False)


@st.composite
def w_prime_balances(draw) -> WPrimeBalance:
    capacity = draw(st.floats(min_value=1000.0, max_value=40000.0, allow_nan=False))
    return WPrimeBalance(
        remaining_j=capacity * draw(fractions),
        capacity_j=capacity,
        tau_s=draw(st.floats(min_value=377.0, max_value=580.0, allow_nan=False)),
    )


@st.composite
def sleeps(draw) -> Sleep:
    debt = draw(st.floats(min_value=0.0, max_value=PARAMS.channels.sleep.debt_cap_hours, allow_nan=False))
    return Sleep(
        debt_hours=debt,
        hours_since_wake=draw(st.floats(min_value=0.0, max_value=36.0, allow_nan=False)),
        circadian_phase=draw(st.floats(min_value=0.0, max_value=23.999, allow_nan=False)),
        last_sleep=draw(
            st.one_of(
                st.none(),
                st.builds(
                    LastSleep,
                    start_h=st.just(0.0),
                    end_h=st.floats(min_value=0.0, max_value=12.0, allow_nan=False),
                    quality=st.sampled_from(SleepQuality),
                ),
            )
        ),
    )


@st.composite
def injuries(draw) -> Injury:
    dimensions = draw(st.lists(st.sampled_from(list(Dimension)), min_size=1, max_size=3, unique=True))
    return Injury(
        region=draw(st.sampled_from(list(InjuryRegion))),
        tissue=draw(st.sampled_from(list(InjuryTissue))),
        mechanism=draw(st.sampled_from(list(InjuryMechanism))),
        severity=draw(st.sampled_from(list(InjurySeverity))),
        onset_h=0.0,
        healing=Healing(
            expected_days=draw(st.floats(min_value=0.5, max_value=90.0, allow_nan=False)),
            progress=draw(st.floats(min_value=0.0, max_value=0.99, allow_nan=False)),
            setback_on_load=draw(fractions),
        ),
        impairments={
            dimension: draw(st.floats(min_value=0.05, max_value=1.0, allow_nan=False))
            for dimension in dimensions
        },
        pain_profile=PainProfile(at_rest=draw(fractions), on_use=draw(fractions)),
    )


@st.composite
def pending_onsets(draw, t_hours: float) -> tuple[SorenessOnset, ...]:
    """Damage waiting on the onset grid, the way the channel leaves it.

    Generated relative to the body's own clock and on the declared grid, because
    a queue holding instants that already passed, or two entries sharing one, is
    a body the channel cannot produce — and P3 is about what the engine hands
    back, not about what a constructor can be talked into.
    """
    offsets = draw(
        st.lists(
            st.integers(min_value=1, max_value=int(KERNEL.onset_delay_h / KERNEL.onset_resolution_h)),
            unique=True,
            max_size=4,
        )
    )
    return tuple(
        SorenessOnset(
            release_at_h=round(t_hours + offset * KERNEL.onset_resolution_h, 9),
            amount=draw(st.floats(min_value=1e-6, max_value=1.0, allow_nan=False)),
        )
        for offset in sorted(offsets)
    )


@st.composite
def body_states(draw) -> BodyState:
    t_hours = draw(st.floats(min_value=0.0, max_value=10000.0, allow_nan=False))
    return BodyState(
        character_id="npc.0001",
        t_hours=t_hours,
        w_prime_balance=draw(w_prime_balances()),
        peripheral_fatigue={region: draw(fractions) for region in FatigueRegion},
        central_fatigue=draw(fractions),
        cumulative_load=CumulativeLoad(
            acute_7d=draw(st.floats(min_value=0.0, max_value=500.0, allow_nan=False)),
            chronic_28d=draw(st.floats(min_value=0.0, max_value=500.0, allow_nan=False)),
        ),
        sleep=draw(sleeps()),
        energy=Energy(
            substrate_availability=draw(fractions),
            balance_kcal_24h=draw(st.floats(min_value=-5000.0, max_value=5000.0, allow_nan=False)),
            last_meal_at_h=draw(st.one_of(st.none(), st.floats(min_value=0.0, max_value=100.0, allow_nan=False))),
        ),
        hydration=Hydration(
            deficit_pct_body_mass=draw(
                st.floats(
                    min_value=0.0,
                    max_value=PARAMS.channels.hydration.max_deficit_pct_body_mass,
                    allow_nan=False,
                )
            )
        ),
        thermal=Thermal(
            core_offset_c=draw(
                st.floats(min_value=0.0, max_value=PARAMS.channels.thermal.core_offset.max_offset_c, allow_nan=False)
            ),
            wbgt=draw(st.floats(min_value=-5.0, max_value=40.0, allow_nan=False)),
            work_rest_ratio=draw(st.floats(min_value=0.1, max_value=4.0, allow_nan=False)),
            clothing_insulation=draw(st.floats(min_value=0.0, max_value=3.0, allow_nan=False)),
        ),
        soreness=Soreness(
            by_region={
                region: RegionSoreness(
                    latency=tuple(draw(st.floats(min_value=0.0, max_value=1.0, allow_nan=False)) for _ in range(STAGES)),
                    pending_onset=draw(pending_onsets(t_hours)),
                    expressed=draw(fractions),
                )
                for region in FatigueRegion
            },
            repeated_bout_adaptation={f"{region.value}|drill": draw(fractions) for region in FatigueRegion},
        ),
        injuries=tuple(draw(st.lists(injuries(), max_size=2))),
        illnesses=(),
        pain=Pain(
            by_region={region: draw(fractions) for region in FatigueRegion},
            global_intensity=draw(fractions),
            analgesia=draw(fractions),
        ),
    )


@st.composite
def environments(draw) -> Environment:
    return Environment(
        wbgt_c=draw(st.floats(min_value=-5.0, max_value=40.0, allow_nan=False)),
        work_rest_ratio=draw(st.floats(min_value=0.1, max_value=4.0, allow_nan=False)),
        clothing_insulation=draw(st.floats(min_value=0.0, max_value=3.0, allow_nan=False)),
        asleep=draw(st.booleans()),
        sleep_quality=draw(st.sampled_from(SleepQuality)),
        fluid_access=draw(fractions),
        carbohydrate_intake=draw(fractions),
    )


@st.composite
def loads(draw) -> IntervalLoad:
    return IntervalLoad(
        intensity=draw(st.floats(min_value=0.0, max_value=2.0, allow_nan=False)),
        by_region={region: draw(fractions) for region in FatigueRegion},
        eccentric_fraction=draw(fractions),
        activity=draw(st.sampled_from(["drill", "run", "swim", "unspecified"])),
    )


#: Intervals the engine will actually accept, kept short enough that a property
#: run integrates in reasonable time. The cap itself is exercised in
#: `test_dynamics.py`, where it belongs.
intervals = st.floats(min_value=0.0, max_value=48.0, allow_nan=False, allow_infinity=False)
