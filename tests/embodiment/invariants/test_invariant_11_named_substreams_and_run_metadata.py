"""Invariant 11 — named substreams, and parameter versions in the run metadata.

`physical-model.md` §15.11. Two halves, and both are load-bearing: the substream
naming is what makes a run reproducible and order-free, and the run metadata is
what makes the numbers in it interpretable afterwards. A run with one and not the
other is reproducible but meaningless, or meaningful but unrepeatable.
"""

from __future__ import annotations

import ast

import pytest

from embodiment.rng import substream, substream_name
from embodiment.seeding import cohort_ids, posterior_for, seed_character
from embodiment.types import MissingRunMetadataError, RunMetadata, SPEC_9_2_COMPONENTS


def test_invariant_11_a_substream_is_named_by_all_four_parts() -> None:
    name = substream_name("npc.0001", "exam.relay", "injury.roll")
    assert "npc.0001" in name and "exam.relay" in name and "injury.roll" in name
    # The world seed is the entropy, not part of the name, so the same named
    # stream under two worlds is two different streams.
    first = substream(1, "npc.0001", "exam.relay", "injury.roll").standard_normal(3)
    second = substream(2, "npc.0001", "exam.relay", "injury.roll").standard_normal(3)
    assert list(first) != list(second)


def test_invariant_11_every_draw_in_the_package_comes_from_a_substream(package_sources) -> None:
    """`substream` is the only constructor of a generator in the subdomain."""
    constructors = set()
    for path, tree in package_sources:
        for node in ast.walk(tree):
            if (
                isinstance(node, ast.Call)
                and isinstance(node.func, ast.Attribute)
                and node.func.attr in {"Generator", "PCG64", "SeedSequence"}
            ):
                constructors.add(path.name)
    assert constructors == {"rng.py"}


def test_invariant_11_the_sample_records_its_substream(prior) -> None:
    record = seed_character(42, "npc.0001", prior)
    assert record.substream == substream_name("npc.0001", "world.seeding", "capacity.sample")


def test_invariant_11_run_metadata_carries_the_parameter_versions(prior) -> None:
    ids = cohort_ids(3)
    metadata = RunMetadata.from_mapping(
        {
            "world_seed": 42,
            "component_versions": {"prior": prior.version},
            "posterior_hash_by_character": {
                character_id: posterior_for(42, character_id, prior).posterior_hash
                for character_id in ids
            },
        },
        required_components=("prior",),
    )
    serialised = metadata.as_dict()
    assert serialised["world_seed"] == 42
    assert serialised["prior_version"] == prior.version
    assert set(serialised["posterior_hash_by_character"]) == set(ids)


def test_invariant_11_a_missing_parameter_version_stops_the_run(prior) -> None:
    metadata = RunMetadata(world_seed=42, posterior_hash_by_character={"npc.0001": "0" * 64})
    with pytest.raises(MissingRunMetadataError):
        metadata.require(("prior",))
    # And every component spec §9.2 names is declared, so a later ticket adding
    # one cannot ship without its version.
    assert "estimator" in SPEC_9_2_COMPONENTS and "dynamics" in SPEC_9_2_COMPONENTS
