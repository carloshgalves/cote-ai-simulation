"""Invariant 4 — `BodyState` persists between scenes, events and days.

`physical-model.md` §15.4 and `CONTEXT.md` invariant 12: physical consequence
survives the scene it was earned in. A scene heals nothing, and only the clock
moving under the recovery rules takes anything out of a body.

The invariant is worth its own file because breaking it is invisible: a body that
resets between scenes still produces plausible numbers inside each scene, and
rest quietly stops being a resource anybody has to manage.
"""

from __future__ import annotations

import pytest

from embodiment.capability import capability_available
from embodiment.dynamics import Environment, Exposure, IntervalLoad, advance
from embodiment.types import BodyState, FatigueRegion, SleepQuality, as_document


def night(quality: SleepQuality = SleepQuality.POOR) -> Exposure:
    return Exposure.of(environment=Environment(asleep=True, sleep_quality=quality))


def test_invariant_04_a_bad_monday_night_is_still_in_the_body_on_wednesday(rested_body) -> None:
    """The model's own sentence, as an assertion.

    Monday's four hours are followed by two ordinary days and nights of normal
    sleep, and the debt is still there — smaller, because time repaid some of it,
    and not gone, because two nights do not undo the first for free.
    """
    monday = advance(advance(rested_body, 20.0, Exposure.of()), 4.0, night())
    assert monday.sleep.debt_hours > 0.0

    body = monday
    for _ in range(2):
        body = advance(advance(body, 15.0, Exposure.of()), 9.0, night(SleepQuality.GOOD))
    assert body.sleep.debt_hours > 0.0
    assert body.t_hours == pytest.approx(72.0)


def test_invariant_04_nothing_that_is_not_the_clock_changes_a_body(rested_body) -> None:
    """Reading a body — repeatedly, and every way this ticket offers — is free."""
    worked = advance(
        rested_body,
        2.0,
        Exposure.of(load=IntervalLoad(intensity=0.9, by_region={FatigueRegion.LEGS: 1.0})),
    )
    document = as_document(worked)

    for _ in range(3):
        capability_available(
            _profile_of(worked), worked, environment=Environment(wbgt_c=33.0)
        )
        as_document(worked)
    assert as_document(worked) == document


def test_invariant_04_a_body_survives_the_document_it_is_persisted_as(rested_body) -> None:
    """Snapshot round trip: what comes back is the body that went in.

    Rehydrating a whole run is PSV1-8; what this ticket owes is that the body it
    persists is not lossy, because a body that arrives at the save and does not
    come out the other side makes every consequence scene-local again.
    """
    worked = advance(
        rested_body,
        6.0,
        Exposure.of(
            environment=Environment(wbgt_c=30.0),
            load=IntervalLoad(
                intensity=0.8, by_region={FatigueRegion.LEGS: 1.0}, eccentric_fraction=0.6, activity="run"
            ),
        ),
    )
    restored = BodyState.model_validate(as_document(worked))
    assert as_document(restored) == as_document(worked)


def test_invariant_04_a_frozen_body_refuses_to_be_edited_in_place(rested_body) -> None:
    with pytest.raises(ValueError):
        rested_body.central_fatigue = 0.5  # type: ignore[misc]


def _profile_of(body: BodyState):
    """The baseline a test body was built from, rebuilt from the prior it came from."""
    from embodiment.prior import PopulationPrior
    from embodiment.rng import PURPOSE_CAPACITY_SAMPLE, SEEDING_EVENT_ID, substream

    return PopulationPrior.load().sample_profile(
        substream(20260909, "npc.0001", SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE), "male"
    )
