"""Invariant 8 — comparisons between characters are output, never input.

`physical-model.md` §15.8 and failure mode F4. No committed record may state that
one character exceeds another; the only admissible comparison is a `COMPARATIVE`
constraint anchored to one observed event. The validator that enforces this ships
with PSV1-1 and is inherited by every later ticket that adds a parameter file.
"""

from __future__ import annotations

import pytest

from embodiment.modelfile import ModelFileError, assert_no_character_ordering


def test_invariant_08_a_ranking_is_refused() -> None:
    with pytest.raises(ModelFileError, match="orders subjects"):
        assert_no_character_ordering({"cohort": {"ranking": ["actor.a", "actor.b"]}})


def test_invariant_08_an_assertion_that_one_exceeds_another_is_refused() -> None:
    for key in ("stronger_than", "faster_than", "outperforms", "superior_to"):
        with pytest.raises(ModelFileError, match="orders subjects"):
            assert_no_character_ordering({"claim": {key: ["actor.a", "actor.b"]}})


def test_invariant_08_an_anchored_comparative_is_the_one_admissible_form() -> None:
    assert_no_character_ordering(
        {
            "feat_fixture": {
                "constraint_type": "COMPARATIVE",
                "anchored_to_event": "ev.sports-festival.relay",
                "subjects": ["actor.a", "actor.b"],
            }
        }
    )
    with pytest.raises(ModelFileError, match="anchored"):
        assert_no_character_ordering(
            {"feat_fixture": {"constraint_type": "COMPARATIVE", "subjects": ["actor.a", "actor.b"]}}
        )


def test_invariant_08_no_committed_data_file_orders_two_characters(data_documents) -> None:
    """Stated over the repository, not only over the files this ticket wrote."""
    for path, document in data_documents:
        if not isinstance(document, dict):
            continue
        try:
            assert_no_character_ordering(document, origin=str(path))
        except ModelFileError as error:  # pragma: no cover - fails loudly when it happens
            raise AssertionError(str(error)) from error


def test_invariant_08_a_percentile_is_derived_and_a_ranking_is_not_stored(prior) -> None:
    """The engine's own comparison is computed on demand, against the cohort."""
    from embodiment.types import Dimension

    percentile = prior.percentile_of(Dimension.MAX_STRENGTH, 40.0, "male")
    assert 0.0 <= percentile <= 100.0
