"""Named substreams (`physical-model.md` §13, invariant 11).

The scheme earns its complexity through exactly one property — isolation — so
that is the test that matters most here. Without it, naming substreams would be
ceremony around a plain seed.
"""

from __future__ import annotations

import numpy as np
import pytest
from hypothesis import HealthCheck, given, settings
from hypothesis import strategies as st

from embodiment.rng import (
    PURPOSE_CAPACITY_SAMPLE,
    SEEDING_EVENT_ID,
    substream,
    substream_name,
)

CHARACTER_IDS = st.text(
    alphabet=st.characters(min_codepoint=97, max_codepoint=122), min_size=1, max_size=8
).map(lambda tail: f"npc.{tail}")


def draws(world_seed: int, character_id: str, count: int = 5) -> list[float]:
    generator = substream(world_seed, character_id, SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE)
    return [float(value) for value in generator.standard_normal(count)]


def test_same_name_same_stream() -> None:
    assert draws(42, "npc.0001") == draws(42, "npc.0001")


def test_world_seed_changes_every_stream() -> None:
    assert draws(42, "npc.0001") != draws(43, "npc.0001")


def test_character_event_and_purpose_each_separate_the_stream() -> None:
    base = substream(42, "npc.0001", SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE).standard_normal(4)
    for other in (
        substream(42, "npc.0002", SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE),
        substream(42, "npc.0001", "exam.swim-relay", PURPOSE_CAPACITY_SAMPLE),
        substream(42, "npc.0001", SEEDING_EVENT_ID, "injury.roll"),
    ):
        assert not np.array_equal(base, other.standard_normal(4))


def test_creation_order_does_not_touch_a_stream() -> None:
    """Failure mode F6: substreams are named, not spawned in sequence."""
    forwards = [draws(42, f"npc.{index:04d}") for index in range(1, 6)]
    backwards = [draws(42, f"npc.{index:04d}") for index in range(5, 0, -1)]
    assert forwards == backwards[::-1]


def test_substream_name_is_readable_and_refuses_the_separator() -> None:
    assert substream_name("npc.0001", SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE).endswith(
        "|npc.0001|world.seeding|capacity.sample"
    )
    with pytest.raises(ValueError, match="separates"):
        substream_name("npc|0001", SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE)


def test_world_seed_must_be_an_int() -> None:
    with pytest.raises(TypeError):
        substream("42", "npc.0001", SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE)  # type: ignore[arg-type]


@settings(max_examples=50, suppress_health_check=[HealthCheck.function_scoped_fixture])
@given(
    world_seed=st.integers(min_value=0, max_value=2**32 - 1),
    present=st.lists(CHARACTER_IDS, min_size=1, max_size=6, unique=True),
    newcomer=CHARACTER_IDS,
)
def test_p5_adding_a_character_moves_nobody(
    world_seed: int, present: list[str], newcomer: str
) -> None:
    """P5, partial — spec §9.1's isolation criterion.

    A character who takes part in no event changes no other character's draws.
    Stated over draws here; PSV1-8 states the same property over a whole run.
    """
    before = {character_id: draws(world_seed, character_id) for character_id in present}
    _ = draws(world_seed, newcomer)
    after = {character_id: draws(world_seed, character_id) for character_id in present}
    assert before == after
