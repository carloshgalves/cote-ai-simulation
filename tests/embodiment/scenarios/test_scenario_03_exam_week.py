"""Scenario 3 of `physical-model.md` §14 — the week of exams.

Three nights of four hours. Sleep debt rises, central fatigue rises, reaction
time and the expression of technique fall, and maximal strength barely notices.

The scenario is the proof that `BodyState` is not per scene. If it were, all of
this would vanish between one class and the next, and rest would stop being a
resource anybody has to manage — which is the difference between a school where
staying up to prepare has a price and one where it is set dressing.

Seeded, and asserted on the event log the run leaves behind rather than on
terminal output.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from embodiment.capability import capability_available
from embodiment.cli import advance_clock_command, seed_cohort_command
from embodiment.eventlog import EVENT_BODY_ADVANCED, EVENT_RUN_EXTENDED
from embodiment.types import Dimension, SleepQuality

WORLD_SEED = 20260909
CHARACTER = "npc.0003"

#: The five dimensions the sleep meta-analysis measured, in the order it puts
#: them: skill control ≫ aerobic ≳ power > speed ≫ maximal strength.
ORDERING = (
    Dimension.COORDINATION,
    Dimension.AEROBIC_CAPACITY,
    Dimension.ANAEROBIC_POWER,
    Dimension.SPRINT_SPEED,
    Dimension.MAX_STRENGTH,
)


@pytest.fixture
def exam_week(tmp_path: Path) -> dict:
    seed_cohort_command(world_seed=WORLD_SEED, count=8, out_dir=tmp_path / "run")
    return advance_clock_command(
        run_dir=tmp_path / "run",
        character_id=CHARACTER,
        days=3,
        sleep="4h",
        quality=SleepQuality.POOR,
    )


def drop(result: dict, dimension: Dimension) -> float:
    """Fraction of the baseline lost, read in the direction the dimension runs."""
    start = result["capability_before"][dimension]
    end = result["capability_after"][dimension]
    if dimension is Dimension.REACTION_TIME:
        return (end - start) / start  # milliseconds: a worse body is a bigger number
    return (start - end) / start


def test_scenario_03_three_bad_nights_leave_a_debt_and_a_tired_head(exam_week) -> None:
    before, after = exam_week["before"], exam_week["after"]
    assert after.t_hours == pytest.approx(72.0)
    assert after.sleep.debt_hours > before.sleep.debt_hours > -1.0
    assert after.sleep.debt_hours > 12.0
    assert after.central_fatigue > before.central_fatigue
    assert after.sleep.last_sleep is not None
    assert after.sleep.last_sleep.quality is SleepQuality.POOR


def test_scenario_03_coordination_and_reaction_fall_while_strength_barely_moves(exam_week) -> None:
    """The central claim, as the **ratio** the meta-analysis supports.

    The absolute size of either fall is `[INT]`; that skill control loses
    several times what maximal strength does is not. The lower bound is the
    meta-analysis's own ratio, 0.87 / 0.35, and the composed body sits above it
    because central fatigue reaches coordination as well.
    """
    coordination = drop(exam_week, Dimension.COORDINATION)
    strength = drop(exam_week, Dimension.MAX_STRENGTH)
    reaction = drop(exam_week, Dimension.REACTION_TIME)

    assert coordination > 0.05
    assert reaction > 0.05
    assert 0.0 < strength < 0.05

    assert coordination / strength >= 0.87 / 0.35
    assert coordination / strength < 12.0


def test_scenario_03_the_ordering_of_the_meta_analysis_survives_composition(exam_week) -> None:
    drops = [drop(exam_week, dimension) for dimension in ORDERING]
    assert drops == sorted(drops, reverse=True), dict(zip(ORDERING, drops))


def test_scenario_03_the_untouched_dimensions_stay_untouched(exam_week) -> None:
    """A bad week does not make anyone shorter, and no lesion appeared."""
    for dimension in (Dimension.BODY_MASS, Dimension.STATURE, Dimension.PAIN_TOLERANCE):
        assert drop(exam_week, dimension) == pytest.approx(0.0)
    assert exam_week["after"].injuries == ()
    assert exam_week["after"].illnesses == ()


def test_scenario_03_the_event_log_carries_both_sides_of_every_advance(exam_week, tmp_path: Path) -> None:
    """Evidence of what happened is the log, not the terminal.

    Six intervals — three days and three nights — each with the channels that
    moved and the value on both sides.
    """
    records = [
        json.loads(line)
        for line in Path(exam_week["events"]).read_text(encoding="utf-8").splitlines()
    ]
    extended = [record for record in records if record["event"] == EVENT_RUN_EXTENDED]
    assert len(extended) == 1
    assert extended[0]["payload"]["run_metadata"]["dynamics_version"]

    advances = [record for record in records if record["event"] == EVENT_BODY_ADVANCED]
    assert len(advances) == 6
    assert [record["payload"]["segment"] for record in advances] == [
        "awake", "asleep", "awake", "asleep", "awake", "asleep"
    ]
    assert sum(record["payload"]["dt_hours"] for record in advances) == pytest.approx(72.0)

    for record in advances:
        payload = record["payload"]
        assert payload["character_id"] == CHARACTER
        changed = payload["channels_changed"]
        assert changed, "an interval that moved nothing is not an interval"
        assert "sleep" in changed
        for channel in changed.values():
            assert set(channel) == {"before", "after"}
            assert channel["before"] != channel["after"]

    nights = [record["payload"] for record in advances if record["payload"]["segment"] == "asleep"]
    debts = [night["channels_changed"]["sleep"]["after"]["debt_hours"] for night in nights]
    assert debts == sorted(debts), "the debt of a bad week only goes one way"


def test_scenario_03_a_body_state_that_reset_between_scenes_would_fail_this(exam_week) -> None:
    """The counterfactual, stated so the scenario cannot quietly stop testing it.

    Advancing the same three days one scene at a time from the *initial* body —
    which is what a per-scene body amounts to — leaves no debt at all.
    """
    from embodiment.dynamics import Environment, Exposure, advance

    scene_local = exam_week["before"]
    for _ in range(3):
        advance(scene_local, 20.0, Exposure.of())
        advance(
            scene_local,
            4.0,
            Exposure.of(environment=Environment(asleep=True, sleep_quality=SleepQuality.POOR)),
        )
    assert scene_local.sleep.debt_hours == 0.0
    assert exam_week["after"].sleep.debt_hours > 12.0
