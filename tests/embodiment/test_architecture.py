"""Architecture tests — acceptance criteria 11 and 12, failure modes F3 and F6.

These are the guards that keep working when nobody is looking at them. Each one
answers a rule that is easy to state, easy to violate by accident, and invisible
in a diff review: no LLM client, no global RNG, one writer of
`capacity_baseline`, no per-character posterior committed.

They are also, per ADR 0007 §5, the guard of the subdomain boundary itself.
"""

from __future__ import annotations

import ast
import math
from pathlib import Path

import pytest

from embodiment.types import DIMENSION_UNITS, CapacityProfile, Dimension

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

    def is_numpy_random_module(node: ast.expr) -> bool:
        return (
            isinstance(node, ast.Attribute)
            and node.attr == "random"
            and isinstance(node.value, ast.Name)
            and node.value.id in numpy_aliases
        )

    for node in ast.walk(tree):
        value: ast.expr | None = None
        if isinstance(node, ast.Assign):
            value = node.value
        elif isinstance(node, ast.AnnAssign):
            value = node.value
        if value is None:
            continue
        if is_numpy_random_module(value) or (
            isinstance(value, ast.Attribute) and is_numpy_random_module(value.value)
        ):
            violations.append(
                f"{path.name}:{node.lineno} binds a local alias derived from numpy.random"
            )

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


@pytest.mark.parametrize(
    "source",
    [
        "import numpy as np\ndraw = np.random.default_rng\ndraw().normal()\n",
        "import numpy as np\nr = np.random\nr.default_rng().normal()\n",
    ],
)
def test_local_numpy_random_aliases_are_rejected(source: str) -> None:
    violations = global_rng_violations(Path("consumer.py"), ast.parse(source))
    assert violations


def capacity_baseline_writer_violations(path: Path, tree: ast.Module) -> list[str]:
    """Find every construction path for the frozen baseline record.

    The copy branch resolves its **receiver** before flagging it, and covers every
    copy API the type exposes (`model_copy`, and Pydantic 1's still-present
    `copy`). Copying is ordinary Pydantic on every other model — `BodyState`
    transitions in PSV1-2 are exactly that — and a guard that bans the method name
    outright bans a legal operation the spec never restricted, which is how a
    guard gets deleted rather than tightened. What F3/F5 forbid is a second writer
    of `capacity_baseline`, and `CapacityBaselineRecord.model_copy` refuses
    `update` at runtime regardless of what this static pass can see.
    """
    record_aliases = {"CapacityBaselineRecord"}
    violations: list[str] = []
    pydantic_constructors = {"model_construct", "model_validate", "model_validate_json"}
    #: Every copy API the type exposes. Pydantic 2 still carries `copy` from
    #: Pydantic 1, deprecated but working, so a guard that knew only `model_copy`
    #: left a second, quieter writer open.
    copy_methods = {"model_copy", "copy"}

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

    def annotation_names_record(annotation: ast.expr | None) -> bool:
        """Does this annotation mention the record? `X | None` and strings included."""
        if annotation is None:
            return False
        if isinstance(annotation, ast.Constant) and isinstance(annotation.value, str):
            try:
                annotation = ast.parse(annotation.value, mode="eval").body
            except SyntaxError:  # pragma: no cover - not a type expression
                return False
        if is_record_reference(annotation):
            return True
        if isinstance(annotation, ast.BinOp):
            return annotation_names_record(annotation.left) or annotation_names_record(
                annotation.right
            )
        if isinstance(annotation, ast.Subscript):
            return annotation_names_record(annotation.slice)
        if isinstance(annotation, ast.Tuple):
            return any(annotation_names_record(element) for element in annotation.elts)
        return False

    #: Names this module has bound to a baseline record — by annotation, or by
    #: assignment from a constructor call. A receiver outside this set is some
    #: other model, and copying it is not a write of `capacity_baseline`.
    baseline_names: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.arg) and annotation_names_record(node.annotation):
            baseline_names.add(node.arg)
        elif isinstance(node, ast.AnnAssign) and annotation_names_record(node.annotation):
            if isinstance(node.target, ast.Name):
                baseline_names.add(node.target.id)
        elif isinstance(node, ast.Assign) and isinstance(node.value, ast.Call):
            called = node.value.func
            constructs = is_record_reference(called) or (
                isinstance(called, ast.Attribute)
                and called.attr in pydantic_constructors
                and is_record_reference(called.value)
            )
            if constructs:
                for target in node.targets:
                    if isinstance(target, ast.Name):
                        baseline_names.add(target.id)

    def is_baseline_receiver(node: ast.expr) -> bool:
        if isinstance(node, ast.Name):
            return node.id in baseline_names
        if isinstance(node, ast.Attribute):
            #: `self.capacity_baseline.model_copy(...)` names the field outright.
            return node.attr == "capacity_baseline" or is_record_reference(node)
        if isinstance(node, ast.Call):
            return is_record_reference(node.func)
        return False

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
            elif (
                isinstance(node.func, ast.Attribute)
                and node.func.attr in copy_methods
                and any(keyword.arg == "update" for keyword in node.keywords)
                and is_baseline_receiver(node.func.value)
            ):
                violations.append(
                    f"{path.name}:{node.lineno} updates a frozen model through "
                    f"{node.func.attr} outside seeding.py (F3/F5)"
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
        (
            "from embodiment.types import CapacityBaselineRecord\n"
            "def forge(record: CapacityBaselineRecord):\n"
            "    return record.model_copy(update={'posterior_hash': 'x'})\n"
        ),
        (
            "from embodiment.types import CapacityBaselineRecord\n"
            "def forge(record: CapacityBaselineRecord | None):\n"
            "    return record.model_copy(update={'posterior_hash': 'x'})\n"
        ),
        (
            "from embodiment.types import CapacityBaselineRecord\n"
            "def forge(store):\n"
            "    record = CapacityBaselineRecord.model_validate({})\n"
            "    return record.model_copy(update={'posterior_hash': 'x'})\n"
        ),
        "def forge(state):\n    return state.capacity_baseline.model_copy(update={'x': 1})\n",
        (
            "from embodiment.types import CapacityBaselineRecord\n"
            "def forge(record: CapacityBaselineRecord):\n"
            "    return record.copy(update={'posterior_hash': 'x'})\n"
        ),
        "def forge(state):\n    return state.capacity_baseline.copy(update={'x': 1})\n",
    ],
)
def test_f3_writer_guard_resolves_aliases_and_alternative_constructors(source: str) -> None:
    violations = capacity_baseline_writer_violations(Path("consumer.py"), ast.parse(source))
    assert violations


@pytest.mark.parametrize(
    "source",
    [
        # PSV1-2's own shape: a pure transition on a different frozen model.
        "def update_unrelated(state):\n    return state.model_copy(update={'fatigue': 0.5})\n",
        (
            "from embodiment.types import BodyState\n"
            "def rest(body: BodyState) -> BodyState:\n"
            "    return body.model_copy(update={'fatigue': 0.0})\n"
        ),
        "def bump(self):\n    self.body_state = self.body_state.model_copy(update={'t': 1})\n",
        "def update_unrelated(state):\n    return state.copy(update={'fatigue': 0.5})\n",
    ],
)
def test_f3_writer_guard_leaves_other_models_alone(source: str) -> None:
    """`model_copy` on a model that is not the baseline is not a second writer.

    The runtime guard on `CapacityBaselineRecord.model_copy` still refuses
    `update` for the real type; this pass only decides what the architecture test
    is allowed to forbid statically.
    """
    violations = capacity_baseline_writer_violations(Path("dynamics.py"), ast.parse(source))
    assert not violations, violations


# --------------------------------------------------------------------------
# Repository lint — acceptance criterion 11.
# --------------------------------------------------------------------------

def test_no_capacity_posterior_is_committed(data_documents, walk_document) -> None:
    """Spec §12: no per-character posterior while the blocking questions are open."""
    for path, document in data_documents:
        for where, node in walk_document(document):
            assert "capacity_posterior" not in where, f"{path}: {where}"


#: Units that are themselves fractions, where a value inside `[0, 1]` is a
#: plausible capacity rather than a coefficient. Everything else is scale
#: bearing: laps, centimetres, kilograms, milliseconds, hours, degrees.
FRACTION_UNITS = frozenset({"hit_rate", "risk_multiplier"})


def nominal_capacity_dimensions(node: object) -> set[str]:
    """Dimensions in this mapping that carry a value **in their physical unit**.

    A parameter file may legitimately key a table by dimension — how sensitive
    each dimension is to lost sleep, what a fatigued region costs each of them —
    and such a table is a mechanism, not a body. What it may never carry is a
    capacity *magnitude*: 40 kg of grip, 170 cm of stature, 250 ms of reaction.
    So the lint asks for the unit, not for the key: a value tagged with a unit,
    or a bare number too large to be a dimensionless coefficient in a dimension
    whose unit has a scale.

    The residual gap is a hand-written profile whose every dimension happens to
    sit inside `[0, 1]` in its own physical unit — a student with half a
    kilogram of grip strength and one centimetre of stature. That body cannot be
    meant, and `CapacityProfile` would take it, which is why this is a lint and
    the invariant is enforced by there being exactly one writer (above).
    """
    if not isinstance(node, dict):
        return set()
    units = {dimension.value: DIMENSION_UNITS[dimension].unit for dimension in Dimension}
    return {
        key
        for key, value in node.items()
        if key in units and is_capacity_magnitude(value, units[key])
    }


def is_capacity_magnitude(value: object, unit: str) -> bool:
    if isinstance(value, dict):
        #: `{value: …, unit: …}` is the serialised shape of a capacity value, and
        #: nothing else in this repository writes it.
        return "unit" in value and is_serialised_capacity_value(value)
    if not is_serialised_capacity_value(value):
        return False
    return unit not in FRACTION_UNITS and abs(float(value)) > 1.0  # type: ignore[arg-type]


def is_serialised_capacity_value(value: object) -> bool:
    candidate = value.get("value") if isinstance(value, dict) else value
    if isinstance(candidate, bool):
        return False
    try:
        return math.isfinite(float(candidate))  # type: ignore[arg-type]
    except (TypeError, ValueError):
        return False


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


def test_nominal_profile_lint_leaves_a_coefficient_table_alone() -> None:
    """A mechanism keyed by dimension is not a body (PSV1-2's own shape).

    `body-dynamics.yaml` says how sensitive each dimension is to lost sleep. Those
    are ratios between effects, and a lint that could not tell them from 40 kg of
    grip strength would force the mechanism to be written somewhere it cannot be
    reviewed next to its own constants.
    """
    sensitivities = {
        "coordination": 0.87,
        "aerobic_capacity": 0.66,
        "anaerobic_power": 0.63,
        "sprint_speed": 0.52,
        "max_strength": 0.35,
    }
    assert nominal_capacity_dimensions(sensitivities) == set()


def test_nominal_profile_lint_recognises_the_serialised_capacity_shape() -> None:
    dimensions = {
        "max_strength": {"value": 55.0, "unit": "kg"},
        "sprint_speed": {"value": 8.0, "unit": "m_per_s"},
        "body_mass": {"value": 62.0, "unit": "kg"},
    }
    assert nominal_capacity_dimensions(dimensions) == set(dimensions)


def test_nominal_profile_lint_matches_numeric_strings_accepted_by_the_model() -> None:
    dimensions = {
        dimension.value: {"value": "55.0", "unit": DIMENSION_UNITS[dimension].unit}
        for dimension in Dimension
    }
    profile = CapacityProfile.model_validate({"dimensions": dimensions})
    assert set(profile.dimensions) == set(Dimension)
    assert nominal_capacity_dimensions(dimensions) == set(dimensions)


def test_runs_are_not_committed(repo_root: Path) -> None:
    """The event log and snapshot are run artefacts; `runs/` stays out of git."""
    gitignore = (repo_root / ".gitignore").read_text(encoding="utf-8").splitlines()
    assert "runs/" in gitignore
