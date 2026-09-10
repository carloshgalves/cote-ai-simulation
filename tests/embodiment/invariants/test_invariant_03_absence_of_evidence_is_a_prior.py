"""Invariant 3 — absence of evidence produces a versioned cohort prior.

`physical-model.md` §15.3: "never a hand-picked attribute". The test that matters
is not that the prior is used, but that **there is no other way in**: seeding
takes a world seed, a character id and a prior, and no argument through which a
number could be supplied for a character.
"""

from __future__ import annotations

import inspect

import pytest
from pydantic import ValidationError

from embodiment.seeding import CohortCovariates, posterior_for, seed_character
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
    """There is no `values=`, no `overrides=`, no `profile=`.

    `covariates=` is not such a door and the test says so on purpose: ADR 0006
    decision 4 conditions the prior on what canon states, so refusing every input
    would violate the invariant rather than protect it. What the invariant forbids
    is a *capacity* arriving by hand, which is why the covariate type is checked
    below to be incapable of carrying one.
    """
    parameters = set(inspect.signature(seed_character).parameters)
    assert parameters == {
        "world_seed",
        "character_id",
        "prior",
        "constraints",
        "covariates",
        "store",
        "log",
    }
    forbidden = {"values", "overrides", "profile", "dimensions", "attributes", "capacity"}
    assert not parameters & forbidden


def test_invariant_03_a_covariate_cannot_smuggle_a_capacity() -> None:
    """The conditioning input carries cohort facts, never numbers for a body."""
    assert set(CohortCovariates.model_fields) == {"sex"}
    for field in ("max_strength", "sprint_speed", "body_mass", "percentile", "profile"):
        with pytest.raises(ValidationError):
            CohortCovariates(**{field: 55.0})
    for dimension in Dimension:
        with pytest.raises(ValidationError):
            CohortCovariates(**{dimension.value: 1.0})


def test_invariant_03_a_known_covariate_is_conditioned_on_not_drawn(prior) -> None:
    """ADR 0006 §4: a sex canon states is not a coin toss.

    Checked across seeds because a single seed agreeing with the draw proves
    nothing — the defect this pins is precisely that some seeds disagreed.
    """
    for world_seed in range(8):
        posterior = posterior_for(world_seed, "npc.known-male", prior, covariates=CohortCovariates(sex="male"))
        assert posterior.sex == "male"
        assert posterior.sex_source == "KNOWN"


def test_invariant_03_an_unknown_covariate_is_still_drawn(prior) -> None:
    posterior = posterior_for(42, "npc.9001", prior)
    assert posterior.sex in ("male", "female")
    assert posterior.sex_source == "DRAWN"


def test_invariant_03_conditioning_yields_the_same_posterior_as_drawing_it(prior) -> None:
    """Sex is the conditioning; where it came from is provenance, not identity.

    A body conditioned on a known male draws from the same marginals as one whose
    male was drawn, so the two posteriors must be the same distribution — and the
    run artefacts must still be able to tell them apart.
    """
    drawn = posterior_for(42, "npc.9001", prior)
    stated = posterior_for(42, "npc.9001", prior, covariates=CohortCovariates(sex=drawn.sex))
    assert stated.posterior_hash == drawn.posterior_hash
    assert stated.sex_source != drawn.sex_source


def test_invariant_03_the_sample_records_what_it_was_drawn_from(prior) -> None:
    record = seed_character(42, "npc.9001", prior)
    assert record.prior_version == prior.version
    assert record.posterior_hash
    assert record.substream
