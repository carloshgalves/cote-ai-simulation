"""The population prior: marginals preserved, structure present, calibration absent.

The distinction this file keeps is the one spec §12 demands: it tests **that the
correlation structure exists**, not that it is right. While S2 is open the
loadings are a declared assumption, so the calibration tests are `xfail` with the
reason written out rather than absent.
"""

from __future__ import annotations

import numpy as np
import pytest

from embodiment.prior import POPULATION_PRIOR_PATH, PopulationPrior, PriorError
from embodiment.rng import substream
from embodiment.types import DIMENSION_UNITS, Dimension

SEXES = ("male", "female")

#: A single generator, because what is under test is the copula and not the
#: seeding. Seeding's per-character substreams are tested in test_seeding.py.
def cohort(prior: PopulationPrior, sex: str, size: int, seed: int = 7) -> dict[Dimension, np.ndarray]:
    return prior.sample_matrix(substream(seed, "test.cohort", "test.event", "test.purpose"), sex, size)


def test_prior_declares_every_dimension_in_its_stored_unit(prior: PopulationPrior) -> None:
    for sex in SEXES:
        for dimension in Dimension:
            marginal = prior.marginal(dimension, sex)
            assert marginal.unit == DIMENSION_UNITS[dimension].unit
        assert set(prior.marginals[sex]) == set(Dimension)
    assert set(prior.loadings) == set(Dimension)


def test_prior_declares_itself_provisional_and_says_why(prior: PopulationPrior) -> None:
    """Acceptance criterion 10: the file's honesty is part of the deliverable."""
    assert prior.status == "PROVISIONAL"
    assert prior.raw["not_canon"] is True
    assert tuple(prior.raw["provenance"]["sourcing_gaps"]) == ("S1", "S2")
    assert prior.raw["evidence_sufficiency"]["overall"] == 0.0
    # The null hypothesis of cohort selectivity is declared, not omitted.
    assert "oq.capability.cohort-selectivity" in prior.raw["provenance"]["open_questions"]
    assert "no selectivity shift" in prior.raw["cohort"]["description"].lower()


def test_loaded_prior_cannot_drift_after_its_content_hash_is_fixed(prior: PopulationPrior) -> None:
    original_hash = prior.content_hash
    with pytest.raises(TypeError):
        prior.marginals["male"][Dimension.MAX_STRENGTH] = prior.marginal(  # type: ignore[index]
            Dimension.BODY_MASS, "male"
        )
    with pytest.raises(AttributeError):
        prior.raw["provenance"]["sourcing_gaps"].append("S3")  # type: ignore[union-attr]
    assert prior.content_hash == original_hash


@pytest.mark.parametrize("sex", SEXES)
def test_copula_preserves_the_marginals(prior: PopulationPrior, sex: str) -> None:
    """A gaussian copula must leave each declared marginal exactly where it was.

    Checked against the marginal's own quantile function rather than against its
    moments, because that is the claim (`preserves the marginals`) and because it
    holds uniformly across the four families the file uses.
    """
    size = 20_000
    columns = cohort(prior, sex, size)
    quantiles = (0.05, 0.25, 0.5, 0.75, 0.95)
    for dimension, values in columns.items():
        marginal = prior.marginal(dimension, sex)
        expected = np.asarray(marginal.ppf(np.asarray(quantiles)), dtype=float)
        observed = np.quantile(values, quantiles)
        spread = float(np.asarray(marginal.ppf(np.asarray([0.75])))[0] - np.asarray(marginal.ppf(np.asarray([0.25])))[0])
        # Tolerance declared as a fraction of the marginal's own interquartile
        # range: an absolute tolerance would mean something different in kg, in
        # laps and in milliseconds.
        assert np.allclose(observed, expected, atol=0.06 * abs(spread)), dimension.value


@pytest.mark.parametrize("sex", SEXES)
def test_f7_the_correlation_structure_exists(prior: PopulationPrior, sex: str) -> None:
    """Failure mode F7 — the sample is not fifteen independent numbers.

    The signs are the prior's declared commitment; the magnitudes are [INT] while
    S2 is open, so no magnitude is asserted here.
    """
    columns = cohort(prior, sex, 10_000)

    def correlation(first: Dimension, second: Dimension) -> float:
        return float(np.corrcoef(columns[first], columns[second])[0, 1])

    assert correlation(Dimension.MAX_STRENGTH, Dimension.BODY_MASS) > 0.2
    assert correlation(Dimension.BODY_MASS, Dimension.AEROBIC_CAPACITY) < -0.2
    assert correlation(Dimension.SPRINT_SPEED, Dimension.AEROBIC_CAPACITY) > 0.1
    # reaction_time is stored in ms, where lower is better: a fitter body is a
    # faster one, so the loading is negative and this correlation must be too.
    assert correlation(Dimension.COORDINATION, Dimension.REACTION_TIME) < 0.0


@pytest.mark.parametrize("sex", SEXES)
def test_f7_no_student_sits_in_an_absurd_joint_tail(prior: PopulationPrior, sex: str) -> None:
    """Nobody is at once the heaviest, the fastest and the best at the shuttle run.

    A cohort of forty would pass this on independent marginals about 99.5% of the
    time, so the claim is made where it has teeth: over a large cohort, where
    independence would produce the triple tail some 2-3 times.
    """
    size = 20_000
    columns = cohort(prior, sex, size)
    watched = (Dimension.BODY_MASS, Dimension.SPRINT_SPEED, Dimension.AEROBIC_CAPACITY)
    above = np.ones(size, dtype=bool)
    for dimension in watched:
        above &= columns[dimension] > np.quantile(columns[dimension], 0.95)
    independent_expectation = size * 0.05**3
    assert above.sum() < 0.4 * independent_expectation


def test_the_demo_cohort_itself_has_no_such_student(prior: PopulationPrior) -> None:
    """The same claim over the cohort the ticket's own command seeds."""
    from embodiment.seeding import cohort_ids, posterior_for, seed_cohort

    world_seed = 42
    ids = cohort_ids(40)
    store = seed_cohort(world_seed, ids, prior)
    watched = (Dimension.BODY_MASS, Dimension.SPRINT_SPEED, Dimension.AEROBIC_CAPACITY)
    percentiles = []
    for character_id, record in store.items():
        sex = posterior_for(world_seed, character_id, prior).sex
        percentiles.append(
            [prior.percentile_of(dimension, record.profile.value(dimension), sex) for dimension in watched]
        )
    assert not any(all(value > 95.0 for value in row) for row in percentiles)


def test_percentile_is_a_derived_view(prior: PopulationPrior) -> None:
    """Invariant 9: percentile is computed from the marginal, never stored."""
    median = float(np.asarray(prior.marginal(Dimension.MAX_STRENGTH, "male").ppf(np.asarray([0.5])))[0])
    assert prior.percentile_of(Dimension.MAX_STRENGTH, median, "male") == pytest.approx(50.0, abs=0.5)
    profile = prior.sample_profile(substream(3, "npc.0001", "e", "p"), "male")
    for entry in profile.dimensions.values():
        assert not hasattr(entry, "percentile")


def test_loadings_leaving_no_residual_are_refused(prior: PopulationPrior, tmp_path) -> None:
    """A dimension that is a deterministic function of the factors is not a dimension."""
    import yaml

    data = yaml.safe_load(POPULATION_PRIOR_PATH.read_text(encoding="utf-8"))
    data["loadings"]["body_mass"] = {"general_fitness": 0.8, "build": 0.7}
    path = tmp_path / "bad-prior.yaml"
    path.write_text(yaml.safe_dump(data), encoding="utf-8")
    with pytest.raises(PriorError, match="residual"):
        PopulationPrior.load(path)


def test_a_marginal_in_the_wrong_unit_is_refused(tmp_path) -> None:
    """Invariant 9: the prior may not quietly restate a dimension in another unit."""
    import yaml

    data = yaml.safe_load(POPULATION_PRIOR_PATH.read_text(encoding="utf-8"))
    data["marginals"]["male"]["max_strength"]["unit"] = "newtons"
    path = tmp_path / "bad-unit.yaml"
    path.write_text(yaml.safe_dump(data), encoding="utf-8")
    with pytest.raises(PriorError, match="unit"):
        PopulationPrior.load(path)


# --------------------------------------------------------------------------
# Calibration. Blocked, and saying so.
# --------------------------------------------------------------------------

class CalibrationDataUnavailable(RuntimeError):
    """The published numbers these tests need have not been transcribed."""


def load_published_cohort_norms() -> dict[str, object]:
    raise CalibrationDataUnavailable(
        "SOURCING S1: the cohort mean and SD by age and sex live in the annual "
        "体力・運動能力調査 on e-Stat, whose file download returns 404 to automated "
        "requests (verified 2026-09-08). A human step is required. Until it happens the "
        "marginals in population-prior.yaml are [INT] and this test cannot run."
    )


def load_published_item_correlations() -> dict[str, object]:
    raise CalibrationDataUnavailable(
        "SOURCING S2: no published inter-item correlation matrix or factor loadings were "
        "located for this battery and age band. Until one is, the loadings in "
        "population-prior.yaml are a declared assumption, and only their SIGNS are tested "
        "(see test_f7_the_correlation_structure_exists)."
    )


@pytest.mark.xfail(
    raises=CalibrationDataUnavailable,
    strict=True,
    reason="S1 open: cohort norms untranscribed, so the marginals are [INT] and cannot be checked against a publication.",
)
@pytest.mark.parametrize("sex", SEXES)
def test_marginals_match_the_published_cohort_norms(prior: PopulationPrior, sex: str) -> None:
    published = load_published_cohort_norms()
    for dimension in Dimension:  # pragma: no cover - runs when S1 closes
        marginal = prior.marginal(dimension, sex)
        norm = published[sex][dimension.value]  # type: ignore[index]
        assert marginal.mean == pytest.approx(norm["mean"], rel=0.02)
        assert marginal.sd == pytest.approx(norm["sd"], rel=0.05)


@pytest.mark.xfail(
    raises=CalibrationDataUnavailable,
    strict=True,
    reason="S2 open: no published correlation matrix was located, so the factor loadings are [INT] and only their signs are asserted.",
)
def test_loadings_match_the_published_correlation_matrix(prior: PopulationPrior) -> None:
    published = load_published_item_correlations()
    columns = cohort(prior, "male", 20_000)
    for (first, second), expected in published.items():  # pragma: no cover - runs when S2 closes
        observed = float(np.corrcoef(columns[first], columns[second])[0, 1])
        assert observed == pytest.approx(expected, abs=0.05)
