"""P3 — no sequence of operations produces an invalid body (§11.2, F15).

Hydration never goes negative, the reserve never exceeds its capacity, healing
progress never leaves `0..1`, severity never leaves its enum. The domains are
enforced by the types, so what this property really checks is that the channels
**clamp before they construct**: a body that could not be built is a body that
was never returned.

The `Δt = 0` case is pinned with an explicit `@example`, because a body that
survives a thousand random hours and then breaks on the interval a caller reaches
by accident is the worst of both.
"""

from __future__ import annotations

from body_strategies import (
    RESTED_BODY,
    body_states,
    environments,
    intervals,
    loads,
)
from hypothesis import HealthCheck, example, given, settings
from hypothesis import strategies as st

from embodiment.capability import capability_available, state_retention
from embodiment.dynamics import (
    Environment,
    Exposure,
    IntervalLoad,
    advance,
    load_params,
)
from embodiment.types import BodyState, FatigueRegion, InjurySeverity

PARAMS = load_params()

SETTINGS = settings(max_examples=50, deadline=None, suppress_health_check=[HealthCheck.too_slow])


def assert_valid(state: BodyState) -> None:
    """Every domain of `physical-model.md` §7, checked on the object itself.

    Deliberately re-checked here rather than trusted to the constructor: the
    point of the property is that the value the engine *hands back* is inside its
    domain, and reading the invariants out loud is what makes the failure message
    name the field instead of naming pydantic.
    """
    assert state.hydration.deficit_pct_body_mass >= 0.0
    assert (
        state.hydration.deficit_pct_body_mass
        <= PARAMS.channels.hydration.max_deficit_pct_body_mass
    )
    assert 0.0 <= state.w_prime_balance.remaining_j <= state.w_prime_balance.capacity_j
    assert state.w_prime_balance.capacity_j > 0.0
    assert 377.0 <= state.w_prime_balance.tau_s <= 580.0
    assert 0.0 <= state.central_fatigue <= 1.0
    assert set(state.peripheral_fatigue) == set(FatigueRegion)
    assert all(0.0 <= value <= 1.0 for value in state.peripheral_fatigue.values())
    assert 0.0 <= state.sleep.debt_hours <= PARAMS.channels.sleep.debt_cap_hours
    assert state.sleep.hours_since_wake >= 0.0
    assert 0.0 <= state.sleep.circadian_phase < 24.0
    assert 0.0 <= state.energy.substrate_availability <= 1.0
    assert 0.0 <= state.thermal.core_offset_c <= PARAMS.channels.thermal.core_offset.max_offset_c
    assert state.cumulative_load.acute_7d >= 0.0 and state.cumulative_load.chronic_28d >= 0.0
    assert 0.0 <= state.pain.global_intensity <= 1.0
    assert 0.0 <= state.pain.analgesia <= 1.0
    assert all(0.0 <= value <= 1.0 for value in state.pain.by_region.values())
    for region in state.soreness.by_region.values():
        assert 0.0 <= region.expressed <= 1.0
        assert all(stage >= 0.0 for stage in region.latency)
    for adaptation in state.soreness.repeated_bout_adaptation.values():
        assert 0.0 <= adaptation <= 1.0
    for injury in state.injuries:
        assert 0.0 <= injury.healing.progress <= 1.0
        assert injury.healing.expected_days > 0.0
        assert injury.severity in InjurySeverity
        assert all(0.0 < value <= 1.0 for value in injury.impairments.values())
    assert state.illnesses == ()
    assert state.t_hours >= 0.0


@SETTINGS
@given(state=body_states(), dt_hours=intervals, environment=environments(), load=loads())
@example(
    state=RESTED_BODY,
    dt_hours=0.0,
    environment=Environment(),
    load=IntervalLoad(),
)
def test_p3_one_advance_lands_inside_every_domain(state, dt_hours, environment, load) -> None:
    exposure = Exposure.of(
        environment=environment, load=IntervalLoad() if environment.asleep else load
    )
    assert_valid(advance(state, dt_hours, exposure))


@SETTINGS
@given(
    state=body_states(),
    steps=st.lists(
        st.tuples(intervals, environments(), loads()), min_size=1, max_size=5
    ),
)
def test_p3_a_sequence_of_advances_never_leaves_the_domains(state, steps) -> None:
    """Sequences, because the states a single advance reaches are not the states
    an engine reaches: a body driven to a boundary and then driven again is where
    a missing clamp shows up."""
    for dt_hours, environment, load in steps:
        state = advance(
            state,
            dt_hours,
            Exposure.of(
                environment=environment, load=IntervalLoad() if environment.asleep else load
            ),
        )
        assert_valid(state)


@SETTINGS
@given(state=body_states(), environment=environments())
def test_p3_capability_stays_positive_and_finite_for_any_body(state, environment) -> None:
    """A body at the floor is unable, never negative and never absent."""
    from body_strategies import REFERENCE_PROFILE

    retention = state_retention(state, environment=environment)
    floor = PARAMS.degradation.retention_floor
    assert all(floor <= value <= 1.0 for value in retention.values())

    available = capability_available(REFERENCE_PROFILE, state, environment=environment)
    assert all(value > 0.0 and value == value for value in available.values())
