"""Named substreams — the only source of randomness in this subdomain.

`physical-model.md` §13 and invariant 11: every physical draw comes from a
substream named by `(world_seed, character_id, event_id, purpose)`. The scheme
exists for one testable consequence (spec §9.1): *adding a character who takes
part in no event moves no other character's draws*. Without that property the
naming would be ceremony; `tests/embodiment/test_rng.py` is what makes it a rule.

The derivation is `SeedSequence`'s own `spawn_key` mechanism, keyed by the
**name** of the substream instead of by creation order. `SeedSequence.spawn(n)`
appends a counter, which is exactly the order dependence failure mode F6
describes; hashing the name into the same slot keeps the mechanism and drops the
order.

Global RNGs are forbidden here: no `numpy.random.<fn>`, no `random`. A test of
architecture refuses them, because a single global draw makes a whole run
irreproducible without failing anything.
"""

from __future__ import annotations

import hashlib

import numpy as np

__all__ = [
    "SUBSTREAM_NAMESPACE",
    "PURPOSE_CAPACITY_SAMPLE",
    "PURPOSE_COHORT_SEX",
    "SEEDING_EVENT_ID",
    "substream_name",
    "substream",
]

#: Versioned namespace. Changing it re-rolls every body in every run, so it moves
#: only with a spec change that intends exactly that.
SUBSTREAM_NAMESPACE = "embodiment.substream.v1"

#: Purposes used by this ticket. One purpose per kind of draw: sharing a purpose
#: between two kinds of draw couples them, and coupled draws break isolation.
PURPOSE_CAPACITY_SAMPLE = "capacity.sample"
PURPOSE_COHORT_SEX = "cohort.sex"

#: Seeding is not an in-world event; it is the instant the world is written.
SEEDING_EVENT_ID = "world.seeding"

_SPAWN_KEY_WORDS = 4  # 128 bits of name digest, as four uint32 words


def substream_name(character_id: str, event_id: str, purpose: str) -> str:
    """The canonical name of a substream. It is logged, so it must be readable."""
    for part, label in ((character_id, "character_id"), (event_id, "event_id"), (purpose, "purpose")):
        if not part:
            raise ValueError(f"{label} must not be empty")
        if "|" in part:
            raise ValueError(f"{label} must not contain '|': it separates the parts of a substream name")
    return f"{SUBSTREAM_NAMESPACE}|{character_id}|{event_id}|{purpose}"


def _spawn_key(name: str) -> tuple[int, ...]:
    digest = hashlib.blake2b(name.encode("utf-8"), digest_size=4 * _SPAWN_KEY_WORDS).digest()
    return tuple(
        int.from_bytes(digest[i * 4 : (i + 1) * 4], "big") for i in range(_SPAWN_KEY_WORDS)
    )


def substream(world_seed: int, character_id: str, event_id: str, purpose: str) -> np.random.Generator:
    """A generator for one `(character, event, purpose)` triple under one world.

    Independent of when the character was created, of how many characters exist,
    and of the order draws are taken in — which is what makes a late NPC
    reproducible (`physical-model.md` §14 scenario 7).
    """
    if not isinstance(world_seed, int) or isinstance(world_seed, bool):
        raise TypeError("world_seed must be an int")
    name = substream_name(character_id, event_id, purpose)
    sequence = np.random.SeedSequence(entropy=world_seed, spawn_key=_spawn_key(name))
    return np.random.Generator(np.random.PCG64(sequence))
