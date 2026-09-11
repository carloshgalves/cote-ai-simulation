"""P1 — a scene heals nothing (spec §11.2, failure mode F2).

For any body and any sequence of operations **without a clock advance**, the body
comes out identical. This is the property the whole persistence invariant rests
on: if a scene could heal, rest would stop being a resource anybody has to
manage, and a badly slept Monday would be gone by Tuesday's first scene.

The `Δt = 0` case is called out with an explicit `@example` because it is the one
a caller reaches by accident — a scene that resolves without time passing — and
it is exactly where a channel that "helpfully" recomputes something would show up.
"""

from __future__ import annotations

from body_strategies import RESTED_BODY, body_states, environments, loads
from hypothesis import HealthCheck, example, given, settings
from hypothesis import strategies as st

from embodiment.capability import capability_available, channel_losses, state_retention
from embodiment.dynamics import (
    CHANNELS,
    Environment,
    Exposure,
    IntervalLoad,
    advance,
    advance_cumulative_load,
    advance_pain,
    channel_diff,
)
from embodiment.prior import PopulationPrior
from embodiment.rng import PURPOSE_CAPACITY_SAMPLE, SEEDING_EVENT_ID, substream
from embodiment.types import as_document

PROFILE = PopulationPrior.load().sample_profile(
    substream(20260909, "npc.0001", SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE), "male"
)

SETTINGS = settings(
    max_examples=60,
    deadline=None,
    suppress_health_check=[HealthCheck.too_slow],
)


@SETTINGS
@given(state=body_states(), environment=environments(), load=loads())
@example(state=RESTED_BODY, environment=Environment(), load=IntervalLoad())
def test_p1_advancing_by_zero_returns_the_same_body(state, environment, load) -> None:
    """Neither `advance` nor any single channel moves a body over no time.

    The channels are checked one by one and not only through `advance`, because
    a channel is callable on its own and the fields most at risk are the ones a
    channel *records* rather than integrates.
    """
    exposure = Exposure.of(
        environment=environment,
        load=IntervalLoad() if environment.asleep else load,
    )
    assert advance(state, 0.0, exposure) is state
    for channel in (*CHANNELS, advance_cumulative_load, advance_pain):
        assert as_document(channel(state, 0.0, exposure)) == as_document(state)


@SETTINGS
@given(state=body_states(), environment=environments(), load=loads(), repeats=st.integers(1, 5))
def test_p1_no_sequence_of_readings_changes_a_body(state, environment, load, repeats) -> None:
    """Everything this ticket offers that is not `advance` is a reading.

    Capability composition, the loss breakdown, the diff a log record is built
    from, the document a snapshot persists — all of them, in any order, any number
    of times, and the body is the one it was.
    """
    before = as_document(state)
    for _ in range(repeats):
        capability_available(PROFILE, state, environment=environment)
        channel_losses(state, environment=environment)
        state_retention(state, environment=environment)
        channel_diff(state, state)
        as_document(state)
    assert as_document(state) == before


@SETTINGS
@given(state=body_states(), environment=environments())
def test_p1_a_body_that_owed_a_night_still_owes_it(state, environment) -> None:
    """The invariant in the shape the model states it in.

    Whatever the body owed, no operation without a clock leaves it owing less.
    """
    exposure = Exposure.of(environment=environment)
    assert advance(state, 0.0, exposure).sleep.debt_hours == state.sleep.debt_hours
    assert advance(state, 0.0, exposure).central_fatigue == state.central_fatigue
    assert advance(state, 0.0, exposure).injuries == state.injuries
