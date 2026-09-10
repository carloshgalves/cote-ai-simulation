"""The header and honesty validator every parameter file passes through.

Acceptance criterion 10 and failure mode F4. Two of these tests are the reason
the validator exists rather than a convention: an unmarked number is refused, and
a file that orders two characters is refused.
"""

from __future__ import annotations

import json

import pytest
import yaml

from embodiment.modelfile import (
    MODEL_FILE_SCHEMA_PATH,
    PHYSICAL_MODELS_DIR,
    ModelFileError,
    assert_no_character_ordering,
    assert_no_named_characters,
    load_model_file,
    validate_mapping,
)

HEADER = {
    "schema_version": 1,
    "not_canon": True,
    "model_kind": "BODY_DYNAMICS",
    "model_version": "0.1.0-provisional",
    "status": "PROVISIONAL",
    "provenance": {
        "calibrated_against": "physiology literature",
        "sourcing_gaps": ["S1"],
        "research": "docs/research/physical-domain-v1.md",
        "decision": "docs/adr/0006-physical-domain-model.md",
    },
    "evidence_sufficiency": {"overall": 0.0, "basis": "structure settled, numbers not"},
}


def file_with(**body: object) -> dict[str, object]:
    return {**json.loads(json.dumps(HEADER)), **body}


def test_the_shipped_prior_passes(tmp_path) -> None:
    load_model_file(PHYSICAL_MODELS_DIR / "population-prior.yaml")


def test_the_schema_file_is_valid_json() -> None:
    schema = json.loads(MODEL_FILE_SCHEMA_PATH.read_text(encoding="utf-8"))
    assert schema["title"] == "PhysicalModelFileHeader"


@pytest.mark.parametrize(
    ("mutation", "match"),
    [
        ({"not_canon": False}, "not_canon"),
        ({"model_version": "one"}, "model_version"),
        ({"status": "FINE"}, "status"),
        ({"evidence_sufficiency": {"basis": "none"}}, "evidence_sufficiency"),
        ({"provenance": {"calibrated_against": "x"}}, "provenance"),
    ],
)
def test_a_dishonest_or_incomplete_header_is_refused(mutation: dict, match: str) -> None:
    with pytest.raises(ModelFileError, match=match):
        validate_mapping(file_with(**mutation))


def test_a_calibrated_file_may_not_still_carry_a_sourcing_gap() -> None:
    with pytest.raises(ModelFileError):
        validate_mapping(file_with(status="CALIBRATED"))


def test_an_unmarked_number_is_refused() -> None:
    """A parameter that is an assumption and does not say so is the whole point."""
    with pytest.raises(ModelFileError, match="no source marking"):
        validate_mapping(file_with(channels={"w_prime": {"tau_seconds": 480}}))


def test_a_marking_covers_the_block_below_it() -> None:
    validate_mapping(
        file_with(
            channels={
                "source": "[INT] parameterisation calibrated by property, not by data",
                "w_prime": {"tau_seconds": 480, "ceiling_j": 20000},
            }
        )
    )


def test_an_unrecognised_marking_is_refused() -> None:
    with pytest.raises(ModelFileError, match="must start with one of"):
        validate_mapping(file_with(channels={"source": "roughly right", "tau_seconds": 480}))


def test_a_marking_with_no_description_is_refused() -> None:
    with pytest.raises(ModelFileError, match="must start with one of"):
        validate_mapping(file_with(channels={"source": "[INT]", "tau_seconds": 480}))


def test_f4_a_file_that_orders_two_characters_is_refused() -> None:
    with pytest.raises(ModelFileError, match="orders subjects"):
        validate_mapping(
            file_with(
                calibration={
                    "source": "[INT] fixture",
                    "ranking": ["actor.a", "actor.b"],
                }
            )
        )


def test_f3_a_file_that_names_a_character_at_all_is_refused() -> None:
    with pytest.raises(ModelFileError, match="names character"):
        validate_mapping(
            file_with(
                overrides={"source": "[INT] fixture", "subject": "actor.ayanokouji"}
            )
        )


def test_f3_a_character_id_used_as_a_mapping_key_is_refused() -> None:
    with pytest.raises(ModelFileError, match="actor.ayanokouji"):
        validate_mapping(
            file_with(
                overrides={
                    "source": "[INT] fixture",
                    "actor.ayanokouji": {"max_strength": 55.0},
                }
            )
        )


def _comparative_feat(**overrides: object) -> dict[str, object]:
    """The shape `data/canon/schema/feat.schema.json` defines for a comparison."""
    feat: dict[str, object] = {
        "source": "[INT] synthetic fixture",
        "id": "feat.sports-festival.relay",
        "story_time": {"arc": "sports-festival"},
        "actors": ["actor.a", "actor.b"],
        "inference": {
            "constraint": "COMPARATIVE",
            "posterior_use": "BOUND",
            "comparative": {
                "by_whom": "actor.a",
                "outperformed": ["actor.b"],
                "same_event": True,
            },
        },
        "margin_seconds": 0.4,
    }
    feat.update(overrides)
    return feat


def test_the_only_admissible_comparison_is_anchored_to_an_event() -> None:
    """Invariant 8: a COMPARATIVE constraint on one observed event, and nothing else."""
    validate_mapping(file_with(fixture=_comparative_feat()))


def test_a_comparison_across_two_events_is_refused() -> None:
    """`same_event: false` compares conditions, which is what the schema forbids."""
    with pytest.raises(ModelFileError, match="same_event"):
        validate_mapping(
            file_with(
                fixture=_comparative_feat(
                    inference={
                        "constraint": "COMPARATIVE",
                        "comparative": {
                            "by_whom": "actor.a",
                            "outperformed": ["actor.b"],
                            "same_event": False,
                        },
                    }
                )
            )
        )


@pytest.mark.parametrize("missing", ["id", "story_time"])
def test_a_comparative_without_its_event_anchor_is_refused(missing: str) -> None:
    feat = _comparative_feat()
    del feat[missing]
    with pytest.raises(ModelFileError, match="anchored to one observed event"):
        validate_mapping(file_with(fixture=feat))


def test_a_comparative_over_undeclared_actors_is_refused() -> None:
    with pytest.raises(ModelFileError, match="not among the feat's declared"):
        validate_mapping(file_with(fixture=_comparative_feat(actors=["actor.a"])))


def test_an_unanchored_comparative_is_refused() -> None:
    """No `comparative` block at all: a constraint label with nothing under it."""
    with pytest.raises(ModelFileError, match="inference.comparative"):
        validate_mapping(
            file_with(
                fixture=_comparative_feat(inference={"constraint": "COMPARATIVE"})
            )
        )


def test_the_invented_constraint_type_spelling_is_not_the_exemption() -> None:
    """A record the real feat validator rejects may not claim invariant 8's exception."""
    with pytest.raises(ModelFileError, match="not part of that contract"):
        validate_mapping(
            file_with(
                fixture={
                    "source": "[INT] synthetic fixture",
                    "constraint_type": "COMPARATIVE",
                    "anchored_to_event": "ev.sports-festival.relay",
                    "subjects": ["actor.a", "actor.b"],
                }
            )
        )


def test_a_root_comparative_does_not_exempt_a_sibling_subtree() -> None:
    """The exemption covers the feat's contract, not the document that carries it.

    The record here really is the one admissible comparison, and it sits at the
    root — the shape that used to hand every sibling the exemption too.
    """
    feat = _comparative_feat()
    del feat["source"]
    with pytest.raises(ModelFileError, match="actor.ayanokouji"):
        assert_no_named_characters(
            {
                **feat,
                "overrides": {
                    "actor.ayanokouji": {
                        "max_strength": 55.0,
                        "sprint_speed": 8.0,
                        "body_mass": 62.0,
                    }
                },
            }
        )


def test_a_model_file_root_may_not_claim_the_comparative_exemption() -> None:
    """`not_canon: true` is not an observed event, whatever fields it copies.

    The comparison is well formed; what refuses it is that a parameter file may
    not be the record it is anchored to, so it stays an ordinary ranking.
    """
    feat = _comparative_feat()
    del feat["source"], feat["margin_seconds"]
    with pytest.raises(ModelFileError, match="orders subjects"):
        validate_mapping(file_with(**feat))


def test_the_ordering_check_is_reusable_on_a_fixture(tmp_path) -> None:
    """Ticket rule: fixtures are held to the same rule as parameter files."""
    fixture = tmp_path / "fixture.yaml"
    fixture.write_text(
        yaml.safe_dump({"cohort": {"stronger_than": ["actor.a", "actor.b"]}}), encoding="utf-8"
    )
    with pytest.raises(ModelFileError, match="orders subjects"):
        assert_no_character_ordering(
            yaml.safe_load(fixture.read_text(encoding="utf-8")), origin=str(fixture)
        )


def test_every_shipped_physical_model_file_passes_the_ordering_check() -> None:
    for path in sorted(PHYSICAL_MODELS_DIR.glob("*.yaml")):
        data = yaml.safe_load(path.read_text(encoding="utf-8"))
        assert_no_character_ordering(data, origin=str(path))
