"""Invariant 13 — physical resolution is a subdomain of the engine, not a peer.

`physical-model.md` §15.13: no second module decides what happened physically. In
PSV1-1 there is one physical decision — that a body was drawn — so the invariant
reduces to something checkable: one writer, one emitter, and no path from this
package into anything that is not the engine's own data boundary (ADR 0007 §3).
"""

from __future__ import annotations

import ast

STDLIB_AND_SUBDOMAIN_DEPENDENCIES = frozenset(
    {
        # declared toolchain of ADR 0007, plus the subdomain's only I/O format
        "numpy", "pydantic", "yaml", "jsonschema",
        # stdlib
        "argparse", "ast", "collections", "dataclasses", "datetime", "enum",
        "functools", "hashlib", "itertools", "json", "math", "pathlib", "re",
        "types", "typing", "__future__",
    }
)


def imported_roots(tree: ast.Module) -> set[str]:
    roots: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            roots.update(alias.name.split(".")[0] for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module and node.level == 0:
            roots.add(node.module.split(".")[0])
    return roots


def test_invariant_13_only_seeding_decides_that_a_body_was_drawn(package_sources) -> None:
    emitters = {
        path.name
        for path, _ in package_sources
        if path.name != "eventlog.py"
        and ("EVENT_CAPACITY_SAMPLED" in path.read_text(encoding="utf-8"))
    }
    assert emitters == {"seeding.py"}


def test_invariant_13_the_subdomain_depends_on_nothing_outside_its_boundary(package_sources) -> None:
    """What crosses the boundary is data, not an import (ADR 0007 §3).

    A dependency arriving here that only makes sense for agents, RAG or a UI is
    the sign that the boundary is being punctured, which is why the allowed set
    is written out rather than inferred.
    """
    for path, tree in package_sources:
        outside = imported_roots(tree) - STDLIB_AND_SUBDOMAIN_DEPENDENCIES - {"embodiment"}
        assert not outside, f"{path.name} imports {sorted(outside)} from outside the subdomain"


def test_invariant_13_the_package_exposes_one_seeding_entry_point() -> None:
    from embodiment import seeding

    writers = [name for name in seeding.__all__ if name.startswith("seed_")]
    assert writers == ["seed_character", "seed_cohort"]
    # seed_cohort is seed_character in a loop: one decision procedure, not two.
    import inspect

    assert "seed_character(" in inspect.getsource(seeding.seed_cohort)
