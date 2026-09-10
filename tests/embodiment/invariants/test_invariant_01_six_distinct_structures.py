"""Invariant 1 — six distinct structures, and no module may fuse two.

`physical-model.md` §2 and §15.1. The tempting fusion is 1 with 4 —
`CapacityProfile` derived from the last `PerformanceOutcome` — and it is the one
that destroys the domain, because reading backwards from performance to capacity
is inference, and inference yields bounds and never values.

PSV1-1 owns exactly one of the six. What it can therefore test is that the one it
owns has no door for the others to come through, and that no such door is added
later without this test noticing.
"""

from __future__ import annotations

import ast

from embodiment.types import CapacityProfile, DimensionValue

#: Names of the other five structures. None of them exists yet; each arrives with
#: the ticket that has behaviour for it. What must never appear is a conversion
#: between two of them.
OTHER_STRUCTURES = (
    "BodyState",
    "ExertionIntent",
    "PerformanceOutcome",
    "ObservedPerformance",
    "SelfPhysicalModel",
)


def test_invariant_01_capacity_profile_has_no_route_from_a_performance() -> None:
    forbidden = {
        "from_performance",
        "from_outcome",
        "from_observation",
        "update_from_performance",
        "observed",
        "performance",
    }
    assert not forbidden & set(dir(CapacityProfile))
    assert not forbidden & set(CapacityProfile.model_fields)
    assert not forbidden & set(DimensionValue.model_fields)


def test_invariant_01_no_module_converts_between_two_structures(package_sources) -> None:
    """A function taking one structure and returning another is the fusion itself."""
    for path, tree in package_sources:
        for node in ast.walk(tree):
            if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
                continue
            annotations = [
                ast.unparse(argument.annotation)
                for argument in node.args.args
                if argument.annotation is not None
            ]
            returns = ast.unparse(node.returns) if node.returns else ""
            takes = {name for name in OTHER_STRUCTURES if any(name in text for text in annotations)}
            gives = "CapacityProfile" in returns or "CapacityBaselineRecord" in returns
            assert not (takes and gives), (
                f"{path.name}:{node.lineno} `{node.name}` derives capacity from {sorted(takes)}. "
                f"Performance constrains capacity as a bound (invariant 2), never as a value."
            )


def test_invariant_01_capacity_values_stay_immutable(prior) -> None:
    """Structure 1 is world truth, frozen at seeding: nothing may edit it in place."""
    from embodiment.rng import substream

    profile = prior.sample_profile(substream(1, "npc.0001", "world.seeding", "capacity.sample"), "male")
    try:
        profile.dimensions.popitem()  # type: ignore[union-attr]
    except (AttributeError, TypeError):
        pass
    else:  # pragma: no cover
        raise AssertionError("a CapacityProfile's dimensions must not be mutable")
