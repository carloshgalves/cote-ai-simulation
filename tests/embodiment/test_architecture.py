"""Architecture tests — acceptance criteria 11 and 12, failure modes F3 and F6.

These are the guards that keep working when nobody is looking at them. Each one
answers a rule that is easy to state, easy to violate by accident, and invisible
in a diff review: no LLM client, no global RNG, one writer of
`capacity_baseline`, no per-character posterior committed.

They are also, per ADR 0007 §5, the guard of the subdomain boundary itself.
"""

from __future__ import annotations

import ast
from pathlib import Path

import pytest

from embodiment.types import Dimension

#: Import roots that would make this subdomain depend on a language model.
#: Spec §4: nothing here needs an LLM to decide a result, and a test refuses one
#: rather than a code review being expected to notice.
LLM_CLIENT_ROOTS = frozenset(
    {
        "anthropic", "openai", "azure_openai", "cohere", "mistralai", "groq", "together",
        "google", "vertexai", "litellm", "ollama", "llama_cpp", "transformers",
        "huggingface_hub", "sentence_transformers", "langchain", "langchain_core",
        "langgraph", "llama_index", "haystack", "dspy", "instructor", "guidance",
        "autogen", "crewai", "semantic_kernel", "chromadb", "qdrant_client", "pinecone",
        "weaviate", "faiss",
    }
)

#: The only members of `numpy.random` this subdomain may touch: the pieces that
#: build a *named* stream. Everything else on that module is a global draw.
ALLOWED_NUMPY_RANDOM = frozenset({"Generator", "PCG64", "SeedSequence"})


def imported_roots(tree: ast.Module) -> set[str]:
    roots: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            roots.update(alias.name.split(".")[0] for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module and node.level == 0:
            roots.add(node.module.split(".")[0])
    return roots


def global_rng_violations(path: Path, tree: ast.Module) -> list[str]:
    """Return uses that can obtain randomness outside the named-stream module."""
    violations: list[str] = []
    numpy_aliases: set[str] = set()

    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                if alias.name == "numpy":
                    numpy_aliases.add(alias.asname or "numpy")
                elif alias.name.startswith("numpy.random"):
                    violations.append(f"{path.name}:{node.lineno} imports {alias.name} directly")
                elif alias.name == "random":
                    violations.append(f"{path.name}:{node.lineno} imports stdlib random")
        elif isinstance(node, ast.ImportFrom) and node.level == 0:
            if node.module == "numpy":
                for alias in node.names:
                    if alias.name == "random":
                        violations.append(
                            f"{path.name}:{node.lineno} imports numpy.random directly"
                        )
            elif node.module and node.module.startswith("numpy.random"):
                violations.append(f"{path.name}:{node.lineno} imports from {node.module}")
            elif node.module == "random":
                violations.append(f"{path.name}:{node.lineno} imports from stdlib random")

    for node in ast.walk(tree):
        if not isinstance(node, ast.Call) or not isinstance(node.func, ast.Attribute):
            continue
        owner = node.func.value
        if not (
            isinstance(owner, ast.Attribute)
            and owner.attr == "random"
            and isinstance(owner.value, ast.Name)
            and owner.value.id in numpy_aliases
        ):
            continue
        if path.name != "rng.py" or node.func.attr not in ALLOWED_NUMPY_RANDOM:
            violations.append(
                f"{path.name}:{node.lineno} calls numpy.random.{node.func.attr} outside "
                f"the named-stream implementation"
            )

    return violations


def test_the_package_imports_no_llm_client(package_sources) -> None:
    for path, tree in package_sources:
        offending = imported_roots(tree) & LLM_CLIENT_ROOTS
        assert not offending, f"{path.name} imports {sorted(offending)}"


def test_the_package_never_uses_a_global_rng(package_sources) -> None:
    """Invariant 11: every draw comes from a named substream.

    One global draw makes a whole run irreproducible without failing anything,
    which is exactly the class of bug a test has to catch instead of a reviewer.
    """
    for path, tree in package_sources:
        violations = global_rng_violations(path, tree)
        assert not violations, "\n".join(violations)


@pytest.mark.parametrize(
    "source",
    [
        "from numpy.random import default_rng\ndefault_rng().normal()\n",
        "import numpy.random as npr\nnpr.default_rng().normal()\n",
        "from numpy import random as npr\nnpr.default_rng().normal()\n",
    ],
)
def test_direct_numpy_random_imports_are_rejected(source: str) -> None:
    violations = global_rng_violations(Path("consumer.py"), ast.parse(source))
    assert violations


def capacity_baseline_writer_violations(path: Path, tree: ast.Module) -> list[str]:
    """Find every construction path for the frozen baseline record."""
    record_aliases = {"CapacityBaselineRecord"}
    violations: list[str] = []
    pydantic_constructors = {"model_construct", "model_validate", "model_validate_json"}

    for node in ast.walk(tree):
        if isinstance(node, ast.ImportFrom):
            for alias in node.names:
                if alias.name == "CapacityBaselineRecord":
                    record_aliases.add(alias.asname or alias.name)

    def is_record_reference(node: ast.expr) -> bool:
        return (
            isinstance(node, ast.Name)
            and node.id in record_aliases
            or isinstance(node, ast.Attribute)
            and node.attr == "CapacityBaselineRecord"
        )

    for node in ast.walk(tree):
        if isinstance(node, ast.Call):
            constructor = False
            if is_record_reference(node.func):
                constructor = True
            elif (
                isinstance(node.func, ast.Attribute)
                and node.func.attr in pydantic_constructors
                and is_record_reference(node.func.value)
            ):
                constructor = True
            if constructor:
                violations.append(
                    f"{path.name}:{node.lineno} constructs a CapacityBaselineRecord outside "
                    f"seeding.py (F3)"
                )

        targets: list[ast.expr] = []
        if isinstance(node, ast.Assign):
            targets = list(node.targets)
        elif isinstance(node, (ast.AnnAssign, ast.AugAssign)):
            targets = [node.target]
        for target in targets:
            name = target.attr if isinstance(target, ast.Attribute) else getattr(target, "id", "")
            if name == "capacity_baseline":
                violations.append(
                    f"{path.name}:{node.lineno} assigns capacity_baseline outside seeding.py (F3)"
                )

    return violations


def test_f3_only_seeding_writes_a_capacity_baseline(package_sources) -> None:
    """A hand-set attribute can only enter through a second writer, so there is one."""
    for path, tree in package_sources:
        if path.name == "seeding.py":
            continue
        violations = capacity_baseline_writer_violations(path, tree)
        assert not violations, "\n".join(violations)


@pytest.mark.parametrize(
    "source",
    [
        "from embodiment.types import CapacityBaselineRecord as Record\nRecord()\n",
        "import embodiment.types as types\ntypes.CapacityBaselineRecord()\n",
        (
            "from embodiment.types import CapacityBaselineRecord\n"
            "CapacityBaselineRecord.model_validate({})\n"
        ),
    ],
)
def test_f3_writer_guard_resolves_aliases_and_alternative_constructors(source: str) -> None:
    violations = capacity_baseline_writer_violations(Path("consumer.py"), ast.parse(source))
    assert violations


# --------------------------------------------------------------------------
# Repository lint — acceptance criterion 11.
# --------------------------------------------------------------------------

def test_no_capacity_posterior_is_committed(data_documents, walk_document) -> None:
    """Spec §12: no per-character posterior while the blocking questions are open."""
    for path, document in data_documents:
        for where, node in walk_document(document):
            assert "capacity_posterior" not in where, f"{path}: {where}"


def nominal_capacity_dimensions(node: object) -> set[str]:
    if not isinstance(node, dict):
        return set()
    dimension_names = {dimension.value for dimension in Dimension}
    return {
        key
        for key, value in node.items()
        if key in dimension_names and is_serialised_capacity_value(value)
    }


def is_serialised_capacity_value(value: object) -> bool:
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        return True
    return (
        isinstance(value, dict)
        and isinstance(value.get("value"), (int, float))
        and not isinstance(value.get("value"), bool)
    )


def test_no_nominal_capacity_profile_compiles(data_documents, walk_document) -> None:
    """Nor a hand-written profile wearing another name.

    A mapping of several capacity dimensions to numbers is a nominal profile
    whatever it is called, and only `seeding.py` may produce one — at runtime,
    from the prior, never from a committed file.
    """
    for path, document in data_documents:
        if path.name == "population-prior.yaml":
            continue  # the cohort distribution, which is exactly what may exist
        for where, node in walk_document(document):
            if not isinstance(node, dict):
                continue
            numeric_dimensions = nominal_capacity_dimensions(node)
            assert len(numeric_dimensions) < 3, (
                f"{path}: {where} looks like a nominal capacity profile "
                f"({sorted(numeric_dimensions)}). Capacity is sampled, never written down."
            )


def test_nominal_profile_lint_recognises_the_serialised_capacity_shape() -> None:
    dimensions = {
        "max_strength": {"value": 55.0, "unit": "kg"},
        "sprint_speed": {"value": 8.0, "unit": "m_per_s"},
        "body_mass": {"value": 62.0, "unit": "kg"},
    }
    assert nominal_capacity_dimensions(dimensions) == set(dimensions)


def test_runs_are_not_committed(repo_root: Path) -> None:
    """The event log and snapshot are run artefacts; `runs/` stays out of git."""
    gitignore = (repo_root / ".gitignore").read_text(encoding="utf-8").splitlines()
    assert "runs/" in gitignore
