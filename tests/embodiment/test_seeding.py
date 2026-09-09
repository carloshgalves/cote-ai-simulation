"""Seeding: one sample, frozen, reproducible, and free of creation order.

Invariant 3 and failure modes F3, F5, F6. The scenario at the end (7 — an NPC
with no evidence at all, created late) is the one the whole substream scheme
exists for.
"""

from __future__ import annotations

import pytest
from hypothesis import HealthCheck, given, settings
from hypothesis import strategies as st

from embodiment.prior import PopulationPrior
from embodiment.seeding import (
    AlreadySeededError,
    CapacityBaselineStore,
    cohort_ids,
    posterior_for,
    seed_character,
    seed_cohort,
)
from embodiment.types import Dimension


def test_a_character_with_no_evidence_draws_the_prior(prior: PopulationPrior) -> None:
    """Invariant 3: absence of evidence is a versioned prior, not a guess."""
    posterior = posterior_for(42, "npc.0001", prior)
    assert posterior.constraint_count == 0
    assert posterior.prior_version == prior.version
    assert set(posterior.evidence_sufficiency) == set(Dimension)
    assert all(value == 0.0 for value in posterior.evidence_sufficiency.values())
    # No particle filter ran, so there is no effective sample size to report.
    # Reporting one would be inventing a number about an inference that did not happen.
    assert posterior.effective_sample_size is None


def test_posterior_evidence_cannot_change_after_its_hash_is_fixed(
    prior: PopulationPrior,
) -> None:
    posterior = posterior_for(42, "npc.0001", prior)
    original_hash = posterior.posterior_hash
    with pytest.raises(TypeError):
        posterior.evidence_sufficiency[Dimension.MAX_STRENGTH] = 1.0  # type: ignore[index]
    assert posterior.posterior_hash == original_hash


def test_the_posterior_of_zero_evidence_is_the_prior_under_another_name(prior: PopulationPrior) -> None:
    """Two evidence-free characters of the same sex share a posterior, and say so."""
    same_sex = [
        posterior_for(42, character_id, prior)
        for character_id in cohort_ids(20)
    ]
    males = {posterior.posterior_hash for posterior in same_sex if posterior.sex == "male"}
    assert len(males) == 1, "characters with identical evidence must share a posterior hash"


def test_constraints_are_refused_rather_than_dropped(prior: PopulationPrior) -> None:
    """PSV1-3 enters behind this signature; until it does, evidence is not swallowed."""
    with pytest.raises(NotImplementedError, match="PSV1-3"):
        posterior_for(42, "npc.0001", prior, constraints=[{"type": "LOWER_BOUND"}])


def test_f5_the_sample_is_frozen(prior: PopulationPrior) -> None:
    store = CapacityBaselineStore()
    first = seed_character(42, "npc.0001", prior, store=store)
    assert store.get("npc.0001") is first
    assert store.get("npc.0001").profile == first.profile
    with pytest.raises(AlreadySeededError):
        seed_character(42, "npc.0001", prior, store=store)


def test_f5_the_record_itself_cannot_be_rewritten(prior: PopulationPrior) -> None:
    record = seed_character(42, "npc.0001", prior)
    with pytest.raises(Exception):
        record.posterior_hash = "rewritten"  # type: ignore[misc]
    with pytest.raises(Exception):
        record.profile.dimensions[Dimension.MAX_STRENGTH] = None  # type: ignore[index]


def test_f6_scenario_7_a_late_npc_gets_the_body_it_would_have_had(prior: PopulationPrior) -> None:
    """Model §14 scenario 7 — no evidence, created late, and displacing nobody."""
    world_seed = 42
    cohort = cohort_ids(40)
    before = seed_cohort(world_seed, cohort, prior)

    latecomer = "npc.9999"
    after = seed_cohort(world_seed, (*cohort, latecomer), prior)
    for character_id in cohort:
        assert after.get(character_id).profile == before.get(character_id).profile

    # The same body it would have drawn had it been created first.
    first_instead = seed_cohort(world_seed, (latecomer, *cohort), prior)
    assert first_instead.get(latecomer).profile == after.get(latecomer).profile


def test_growing_the_cohort_keeps_the_bodies_already_drawn(prior: PopulationPrior) -> None:
    """`--n 41` is `--n 40` plus one, which is what makes ids the address."""
    forty = seed_cohort(42, cohort_ids(40), prior)
    forty_one = seed_cohort(42, cohort_ids(41), prior)
    for character_id in cohort_ids(40):
        assert forty_one.get(character_id).profile == forty.get(character_id).profile
    assert "npc.0041" in forty_one


def test_the_record_carries_its_provenance(prior: PopulationPrior) -> None:
    record = seed_character(42, "npc.0001", prior)
    assert record.prior_version == prior.version
    assert record.substream.endswith("|npc.0001|world.seeding|capacity.sample")
    assert len(record.posterior_hash) == 64
    assert set(record.evidence_sufficiency) == set(Dimension)


@settings(max_examples=25, deadline=None, suppress_health_check=[HealthCheck.function_scoped_fixture])
@given(
    world_seed=st.integers(min_value=0, max_value=2**32 - 1),
    size=st.integers(min_value=1, max_value=12),
)
def test_p6_same_seed_same_bodies(prior: PopulationPrior, world_seed: int, size: int) -> None:
    """P6, partial — determinism of seeding. PSV1-8 states it over a whole run."""
    ids = cohort_ids(size)
    first = seed_cohort(world_seed, ids, prior)
    second = seed_cohort(world_seed, ids, prior)
    for character_id in ids:
        assert first.get(character_id) == second.get(character_id)


def test_cohort_ids_are_stable_and_ordered() -> None:
    assert cohort_ids(3) == ("npc.0001", "npc.0002", "npc.0003")
    assert cohort_ids(3, start=2) == ("npc.0002", "npc.0003", "npc.0004")
    assert cohort_ids(0) == ()
