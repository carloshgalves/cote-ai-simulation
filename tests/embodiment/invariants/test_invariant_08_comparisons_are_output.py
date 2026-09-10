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


def test_invariant_08_a_relational_target_is_joined_to_its_enclosing_subject() -> None:
    with pytest.raises(ModelFileError, match="orders subjects"):
        assert_no_character_ordering(
            {"claim": {"subject": "actor.a", "stronger_than": "actor.b"}}
        )


@pytest.mark.parametrize("key", ["strength_order", "placement", "ranked_by"])
def test_invariant_08_semantic_ordering_aliases_are_refused(key: str) -> None:
    with pytest.raises(ModelFileError, match="orders subjects"):
        assert_no_character_ordering({"claim": {key: ["actor.a", "actor.b"]}})


def test_invariant_08_non_character_placement_is_not_a_capacity_ordering() -> None:
    assert_no_character_ordering({"school": {"placement": "Class D"}})


#: The comparison `data/canon/schema/feat.schema.json` actually defines. The gate
#: is tested against the contract canon will hold, not against a spelling the
#: tests and the validator agreed on between themselves.
CANONICAL_COMPARATIVE_FEAT = {
    "id": "feat.sports-festival.relay",
    "schema_version": 1,
    "story_time": {"arc": "sports-festival"},
    "actors": ["actor.a", "actor.b"],
    "modality": "sprint",
    "inference": {
        "constraint": "COMPARATIVE",
        "posterior_use": "BOUND",
        "comparative": {
            "by_whom": "actor.a",
            "outperformed": ["actor.b"],
            "same_event": True,
        },
    },
}


def test_invariant_08_an_anchored_comparative_is_the_one_admissible_form() -> None:
    assert_no_character_ordering(CANONICAL_COMPARATIVE_FEAT)
    assert_no_character_ordering({"feat_fixture": CANONICAL_COMPARATIVE_FEAT})


def test_invariant_08_a_root_comparative_does_not_exempt_a_sibling_ranking() -> None:
    """One anchored comparison is the exception; the file around it is not.

    The record is valid and is the root mapping, which is exactly the position
    that used to switch the gate off for every sibling in the document.
    """
    with pytest.raises(ModelFileError, match="orders subjects"):
        assert_no_character_ordering(
            {**CANONICAL_COMPARATIVE_FEAT, "calibration": {"ranking": ["actor.c", "actor.d"]}}
        )


def test_invariant_08_a_comparison_across_events_is_not_a_comparison() -> None:
    """`same_event` is `const: true` in the schema; the gate must hold it there.

    This is the shape that used to pass: a canonical record ordering two
    characters in *different* events, which is a ranking with a constraint's name.
    """
    across = {
        **CANONICAL_COMPARATIVE_FEAT,
        "inference": {
            "constraint": "COMPARATIVE",
            "comparative": {
                "by_whom": "actor.a",
                "outperformed": ["actor.b"],
                "same_event": False,
            },
        },
    }
    with pytest.raises(ModelFileError, match="same_event"):
        assert_no_character_ordering(across)


def test_invariant_08_a_truthy_stand_in_is_not_the_same_event_boolean() -> None:
    truthy = {
        **CANONICAL_COMPARATIVE_FEAT,
        "inference": {
            "constraint": "COMPARATIVE",
            "comparative": {
                "by_whom": "actor.a",
                "outperformed": ["actor.b"],
                "same_event": "yes",
            },
        },
    }
    with pytest.raises(ModelFileError, match="same_event"):
        assert_no_character_ordering(truthy)


@pytest.mark.parametrize("missing", ["id", "story_time", "actors"])
def test_invariant_08_a_comparative_must_carry_its_event_anchor(missing: str) -> None:
    feat = {key: value for key, value in CANONICAL_COMPARATIVE_FEAT.items() if key != missing}
    with pytest.raises(ModelFileError):
        assert_no_character_ordering(feat)


def test_invariant_08_the_invented_spelling_no_longer_buys_the_exemption() -> None:
    """`constraint_type`/`anchored_to_event` are not in the contract.

    The old gate accepted `anchored_to_event: true`, which names no event at all.
    """
    with pytest.raises(ModelFileError, match="not part of that contract"):
        assert_no_character_ordering(
            {
                "feat_fixture": {
                    "constraint_type": "COMPARATIVE",
                    "anchored_to_event": True,
                    "subjects": ["actor.a", "actor.b"],
                }
            }
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
