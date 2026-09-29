"""Shared fixtures. No LLM, no network, no global RNG — none of it is needed here.

The source- and data-walking fixtures live here rather than in a helper module so
that the tests under `invariants/` can use them too: an invariant that can only
be checked from one directory is an invariant with a blind spot.
"""

from __future__ import annotations

import ast
from collections.abc import Iterator
from pathlib import Path
from typing import Any

import pytest
import yaml

from embodiment.prior import PopulationPrior

REPO_ROOT = Path(__file__).resolve().parents[2]
PACKAGE_ROOT = REPO_ROOT / "src" / "embodiment"


@pytest.fixture(scope="session")
def prior() -> PopulationPrior:
    return PopulationPrior.load()


@pytest.fixture(scope="session")
def repo_root() -> Path:
    return REPO_ROOT


@pytest.fixture(scope="session")
def package_root() -> Path:
    return PACKAGE_ROOT


@pytest.fixture(scope="session")
def package_sources() -> tuple[tuple[Path, ast.Module], ...]:
    """Every module of `src/embodiment`, parsed."""
    return tuple(
        (path, ast.parse(path.read_text(encoding="utf-8"), filename=str(path)))
        for path in sorted(PACKAGE_ROOT.rglob("*.py"))
    )


@pytest.fixture(scope="session")
def data_documents() -> tuple[tuple[Path, Any], ...]:
    """Every committed YAML record under `data/`."""
    documents = []
    for path in sorted((REPO_ROOT / "data").rglob("*")):
        if path.is_file() and path.suffix.lower() in (".yaml", ".yml"):
            documents.append((path, yaml.safe_load(path.read_text(encoding="utf-8"))))
    return tuple(documents)


def walk(node: Any, path: str = "") -> Iterator[tuple[str, Any]]:
    """Yield `(dotted path, node)` for every node of a parsed document."""
    yield path, node
    if isinstance(node, dict):
        for key, value in node.items():
            yield from walk(value, f"{path}.{key}" if path else str(key))
    elif isinstance(node, list):
        for index, value in enumerate(node):
            yield from walk(value, f"{path}[{index}]")


@pytest.fixture(scope="session")
def walk_document():
    return walk


@pytest.fixture(scope="session")
def dynamics_params():
    from embodiment.dynamics import load_params

    return load_params()


@pytest.fixture(scope="session")
def reference_profile(prior):
    """One concrete body from the prior, for tests that need real units.

    Drawn rather than written down: a hand-made profile in a test is the same
    failure mode (F3) as one in a data file, and it would drift from the prior
    the moment the marginals move.
    """
    from embodiment.rng import PURPOSE_CAPACITY_SAMPLE, SEEDING_EVENT_ID, substream

    return prior.sample_profile(
        substream(20260909, "npc.0001", SEEDING_EVENT_ID, PURPOSE_CAPACITY_SAMPLE), "male"
    )


@pytest.fixture(scope="session")
def rested_body(reference_profile):
    from embodiment.dynamics import initial_body_state

    return initial_body_state("npc.0001", reference_profile)
