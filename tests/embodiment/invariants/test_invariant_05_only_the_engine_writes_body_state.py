"""Invariant 5 — only the engine writes physical state, and only with a `Δt`.

`physical-model.md` §15.5 and the ticket's own words: *there is no signature that
alters the body without advancing the clock*. That is a property of the module's
surface, not of anyone's discipline, so it is checked by reading the surface.

Two rules, both structural:

1. `dynamics.py` is the only writer. Every other module may read a body and
   `snapshot.py` may rehydrate one, and nothing else may produce a changed body.
2. Every public function of `dynamics.py` that returns a `BodyState` takes an
   explicit interval. The single creation entry point is named here, because
   creating a body is not altering one, and a list of exceptions that lives in a
   test is a list somebody has to justify adding to.
"""

from __future__ import annotations

import ast
import inspect
from pathlib import Path

import pytest

from embodiment import dynamics
from embodiment.dynamics import BodyDynamicsError, Exposure, advance
from embodiment.types import BodyState

#: The one function that produces a body without advancing a clock. It creates;
#: it does not alter. Anything else added to this set is a second writer.
CREATION_ENTRY_POINTS = frozenset({"initial_body_state"})

#: Modules allowed to name `BodyState` as a producer at all, and why.
WRITER = "dynamics.py"
REHYDRATOR = "snapshot.py"


def body_writer_violations(path: Path, tree: ast.Module) -> list[str]:
    """Every construction or mutation path for a body in one module."""
    aliases = {"BodyState"}
    for node in ast.walk(tree):
        if isinstance(node, ast.ImportFrom):
            for alias in node.names:
                if alias.name == "BodyState":
                    aliases.add(alias.asname or alias.name)

    def names_body(node: ast.expr) -> bool:
        return (
            isinstance(node, ast.Name)
            and node.id in aliases
            or isinstance(node, ast.Attribute)
            and node.attr == "BodyState"
        )

    #: Locals known to hold a body: annotated as one, or assigned from one of the
    #: producers above. A `model_copy` on anything else is some other model.
    body_names: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.arg) and node.annotation is not None and names_body(node.annotation):
            body_names.add(node.arg)
        elif isinstance(node, ast.AnnAssign) and node.annotation is not None and names_body(node.annotation):
            if isinstance(node.target, ast.Name):
                body_names.add(node.target.id)

    violations: list[str] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Call):
            called = node.func
            constructs = names_body(called) or (
                isinstance(called, ast.Attribute)
                and called.attr in {"model_construct", "model_validate", "model_validate_json"}
                and names_body(called.value)
            )
            if constructs:
                violations.append(f"{path.name}:{node.lineno} produces a BodyState")
            elif (
                isinstance(called, ast.Attribute)
                and called.attr in {"model_copy", "copy"}
                and any(keyword.arg == "update" for keyword in node.keywords)
                and (
                    (isinstance(called.value, ast.Name) and called.value.id in body_names)
                    or (isinstance(called.value, ast.Attribute) and called.value.attr == "body_state")
                )
            ):
                violations.append(f"{path.name}:{node.lineno} alters a BodyState in place")

        targets = node.targets if isinstance(node, ast.Assign) else []
        if isinstance(node, (ast.AnnAssign, ast.AugAssign)):
            targets = [node.target]
        for target in targets:
            if isinstance(target, ast.Attribute) and target.attr == "body_state":
                violations.append(f"{path.name}:{node.lineno} assigns body_state")
    return violations


def test_invariant_05_only_the_dynamics_module_writes_a_body(package_sources) -> None:
    for path, tree in package_sources:
        if path.name in (WRITER, REHYDRATOR):
            continue
        violations = body_writer_violations(path, tree)
        assert not violations, "\n".join(violations)


def test_invariant_05_the_rehydrator_only_reads_a_persisted_body(package_sources) -> None:
    """`snapshot.py` may bring a body back; it may not change one.

    Reading a persisted body is not writing one, and the exception is narrow: the
    only producer it is allowed is `model_validate` of what was written.
    """
    source = next(tree for path, tree in package_sources if path.name == REHYDRATOR)
    for node in ast.walk(source):
        if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute):
            if node.func.attr in {"model_copy", "copy"} and any(
                keyword.arg == "update" for keyword in node.keywords
            ):
                producer = node.func.value
                assert not (
                    isinstance(producer, ast.Name) and producer.id in {"body", "state"}
                ), f"{REHYDRATOR}:{node.lineno} alters a body while reading one"


@pytest.mark.parametrize(
    "source",
    [
        "from embodiment.types import BodyState\nBodyState()\n",
        "from embodiment.types import BodyState as B\nB.model_validate({})\n",
        (
            "from embodiment.types import BodyState\n"
            "def heal(body: BodyState) -> BodyState:\n"
            "    return body.model_copy(update={'central_fatigue': 0.0})\n"
        ),
        "def heal(engine):\n    engine.body_state = None\n",
    ],
)
def test_invariant_05_the_writer_guard_catches_the_shapes_a_second_writer_takes(source: str) -> None:
    assert body_writer_violations(Path("consumer.py"), ast.parse(source))


def test_invariant_05_no_signature_changes_a_body_without_an_interval() -> None:
    """The ticket's rule, read off the module: no healing without a clock."""
    offenders: list[str] = []
    for name in dynamics.__all__:
        member = getattr(dynamics, name)
        if not callable(member) or isinstance(member, type):
            continue
        try:
            signature = inspect.signature(member)
        except (TypeError, ValueError):  # pragma: no cover - builtins
            continue
        if signature.return_annotation != "BodyState":
            continue
        if name in CREATION_ENTRY_POINTS:
            continue
        if "dt_hours" not in signature.parameters:
            offenders.append(f"{name}{signature} returns a body without taking an interval")
    assert not offenders, "\n".join(offenders)


def test_invariant_05_the_nine_channels_all_take_an_interval() -> None:
    assert len(dynamics.CHANNELS) == 9
    for channel in dynamics.CHANNELS:
        parameters = list(inspect.signature(channel).parameters)
        assert parameters[:2] == ["state", "dt_hours"], parameters


def test_invariant_05_an_interval_of_zero_returns_the_very_same_body(rested_body) -> None:
    """Not merely an equal body: the same one. A caller who spends no time gets
    no change, and there is no path through the channels that could round one."""
    assert advance(rested_body, 0.0, Exposure.of()) is rested_body


def test_invariant_05_a_negative_interval_is_refused(rested_body) -> None:
    with pytest.raises(BodyDynamicsError):
        advance(rested_body, -0.25, Exposure.of())


def test_invariant_05_a_body_is_immutable_once_written(rested_body: BodyState) -> None:
    assert rested_body.model_config["frozen"] is True
    with pytest.raises(ValueError):
        rested_body.sleep = None  # type: ignore[misc]
