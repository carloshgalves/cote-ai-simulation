"""Invariant 3 — absence of evidence produces a versioned cohort prior.

`physical-model.md` §15.3: "never a hand-picked attribute". The test that matters
is not that the prior is used, but that **there is no other way in**: seeding
takes a world seed, a character id and a prior, and no argument through which a
number could be supplied for a character.
"""

from __future__ import annotations

import inspect

from embodiment.seeding import posterior_for, seed_character
from embodiment.types import Dimension


def test_invariant_03_a_character_without_evidence_draws_the_cohort_prior(prior) -> None:
    posterior = posterior_for(42, "npc.9001", prior)
    assert posterior.constraint_count == 0
    assert posterior.prior_version == prior.version
    assert all(value == 0.0 for value in posterior.evidence_sufficiency.values())
    assert set(posterior.evidence_sufficiency) == set(Dimension)


def test_invariant_03_the_prior_carries_provenance_and_a_version(prior) -> None:
    assert prior.version
    assert prior.raw["not_canon"] is True
    assert prior.raw["provenance"]["calibrated_against"]
    assert prior.raw["provenance"]["research"]
    assert prior.raw["provenance"]["decision"]


def test_invariant_03_seeding_offers_no_door_for_a_hand_set_value() -> None:
    """There is no `values=`, no `overrides=`, no `profile=`."""
    parameters = set(inspect.signature(seed_character).parameters)
    assert parameters == {"world_seed", "character_id", "prior", "constraints", "store", "log"}
    forbidden = {"values", "overrides", "profile", "dimensions", "attributes", "capacity"}
    assert not parameters & forbidden


def test_invariant_03_the_sample_records_what_it_was_drawn_from(prior) -> None:
    record = seed_character(42, "npc.9001", prior)
    assert record.prior_version == prior.version
    assert record.posterior_hash
    assert record.substream
