"""The rules `BodyState` has to keep that no single channel is responsible for.

Three of them are decisions that would otherwise be discovered as bugs months
later: the two region taxonomies differ on purpose and the map between them is
total (13.2), `illnesses` is serialised and never written (13.5), and
`cumulative_load` is accounting that nothing reads (spec §3.2, failure mode F14).

Each is checked twice where it can be — once as behaviour, once by reading the
package — because a decision that only holds while everybody remembers it is not
a decision.
"""

from __future__ import annotations

import ast
from pathlib import Path

import pytest
import yaml

from embodiment.capability import capability_available
from embodiment.dynamics import (
    BODY_DYNAMICS_PATH,
    BodyDynamicsError,
    BodyDynamicsParams,
    Environment,
    Exposure,
    IntervalLoad,
    advance,
)
from embodiment.types import (
    BodyState,
    CumulativeLoad,
    FatigueRegion,
    Illness,
    InjuryRegion,
    InjurySeverity,
)

DEAD_ACCOUNTING_FIELDS = ("acute_7d", "chronic_28d")


def replace_channel(state: BodyState, **sections) -> BodyState:
    fields = {name: getattr(state, name) for name in BodyState.model_fields}
    fields.update(sections)
    return BodyState(**fields)


# --------------------------------------------------------------------------
# Decision 13.2 — two taxonomies, one total map
# --------------------------------------------------------------------------


def test_every_injury_region_lands_in_exactly_one_fatigue_region(dynamics_params) -> None:
    """The asymmetry is safe only while the map is total.

    A gap here would be found as a `KeyError` on the day a lesion first had to be
    located in a fatigue reservoir — which is to say, in the middle of an exam.
    """
    mapping = dynamics_params.taxonomies.injury_region_to_fatigue_region
    missing = sorted(region.value for region in InjuryRegion if region not in mapping)
    assert not missing, f"injury regions with no fatigue region: {missing}"
    assert set(mapping.values()) <= set(FatigueRegion)
    assert set(mapping.values()) == set(FatigueRegion), (
        "a fatigue region no injury can reach is a reservoir nothing fills"
    )


def test_the_parameter_file_and_the_enums_hold_the_same_two_taxonomies(dynamics_params) -> None:
    assert set(dynamics_params.taxonomies.fatigue_regions) == set(FatigueRegion)
    assert set(dynamics_params.taxonomies.injury_regions) == set(InjuryRegion)


def test_the_asymmetry_between_the_taxonomies_is_documented_in_the_file() -> None:
    """Decision 13.2 (i): an asymmetry nobody wrote down becomes a reading bug.

    The header has to say that the two taxonomies differ **and** why, in the file
    a reader of the constants is already in.
    """
    header = BODY_DYNAMICS_PATH.read_text(encoding="utf-8").split("schema_version:")[0]
    lowered = header.lower()
    assert "taxonomies" in lowered
    assert "peripheral_fatigue" in lowered and "injury.region" in lowered
    assert "impairments" in lowered  # the mechanism that actually decides a grip exam


def test_a_partial_region_map_is_refused_at_load(tmp_path: Path) -> None:
    document = yaml.safe_load(BODY_DYNAMICS_PATH.read_text(encoding="utf-8"))
    document["taxonomies"]["injury_region_to_fatigue_region"].pop("wrist_left")
    path = tmp_path / "body-dynamics.yaml"
    path.write_text(yaml.safe_dump(document, sort_keys=False, allow_unicode=True), encoding="utf-8")
    with pytest.raises(BodyDynamicsError, match="not total"):
        BodyDynamicsParams.load(path)


# --------------------------------------------------------------------------
# Decision 13.5 — `illnesses` is serialised and never written
# --------------------------------------------------------------------------


def test_no_module_writes_an_illness(package_sources) -> None:
    """Spec §13.5: the field exists, and no V1 ticket updates it.

    Written as a test rather than a note so that the absence of dynamics is not
    read as a bug later, and so that adding one is a deliberate act.
    """
    offences: list[str] = []
    for path, tree in package_sources:
        for node in ast.walk(tree):
            if isinstance(node, ast.keyword) and node.arg == "illnesses":
                empty = isinstance(node.value, ast.Tuple) and not node.value.elts
                if not empty:
                    offences.append(f"{path.name}:{node.lineno} passes a non-empty illnesses")
            targets = node.targets if isinstance(node, ast.Assign) else []
            for target in targets:
                if isinstance(target, ast.Attribute) and target.attr == "illnesses":
                    offences.append(f"{path.name}:{node.lineno} assigns illnesses")
    assert not offences, "\n".join(offences)


def test_advancing_the_clock_leaves_an_illness_exactly_where_it_was(rested_body, dynamics_params) -> None:
    ill = replace_channel(
        rested_body,
        illnesses=(Illness(label="fever", onset_h=0.0, severity=InjurySeverity.MINOR),),
    )
    moved = advance(ill, 72.0, Exposure.of(params=dynamics_params))
    assert moved.illnesses == ill.illnesses


def test_the_parameter_file_says_illness_is_not_a_channel() -> None:
    header = BODY_DYNAMICS_PATH.read_text(encoding="utf-8").split("schema_version:")[0]
    assert "ILLNESS IS NOT A CHANNEL" in header


# --------------------------------------------------------------------------
# Spec §3.2 and failure mode F14 — `cumulative_load` is accounting nothing reads
# --------------------------------------------------------------------------


def test_the_load_accounting_is_updated(rested_body, dynamics_params) -> None:
    worked = advance(
        rested_body,
        2.0,
        Exposure.of(
            load=IntervalLoad(intensity=0.8, by_region={FatigueRegion.LEGS: 1.0}),
            params=dynamics_params,
        ),
    )
    assert worked.cumulative_load.acute_7d > 0.0
    assert worked.cumulative_load.chronic_28d > 0.0


def test_nothing_outside_the_accounting_itself_reads_the_load_windows(package_sources) -> None:
    """Spec §3.2: the field exists, is updated, and nothing depends on it.

    The one legitimate read is the accounting updating itself. This is the same
    test that keeps the acute:chronic ratio from coming back by accident (F14):
    the metric is mathematically coupled, unstable at low chronic load,
    unsupported as a causal factor, and the figure that popularised it is the
    subject of a formal retraction request.
    """
    offences: list[str] = []
    for path, tree in package_sources:
        for node in ast.walk(tree):
            if not isinstance(node, ast.Attribute) or node.attr not in DEAD_ACCOUNTING_FIELDS:
                continue
            if path.name == "dynamics.py" and _enclosing_function(tree, node) == "advance_cumulative_load":
                continue
            offences.append(
                f"{path.name}:{node.lineno} reads cumulative_load.{node.attr}, which nothing "
                f"in V1 may depend on"
            )
    assert not offences, "\n".join(offences)


def test_no_module_divides_one_load_window_by_the_other(package_sources) -> None:
    for path, tree in package_sources:
        for node in ast.walk(tree):
            if not isinstance(node, ast.BinOp) or not isinstance(node.op, ast.Div):
                continue
            mentioned = {
                child.attr
                for child in ast.walk(node)
                if isinstance(child, ast.Attribute) and child.attr in DEAD_ACCOUNTING_FIELDS
            }
            assert len(mentioned) < 2, (
                f"{path.name}:{node.lineno} computes an acute:chronic ratio. The metric was "
                f"removed from the model on 2026-09-09 (failure mode F14)."
            )


def test_capability_is_blind_to_the_load_windows(reference_profile, rested_body) -> None:
    """The behavioural half: two bodies differing only in the accounting are equal."""
    loaded = replace_channel(
        rested_body, cumulative_load=CumulativeLoad(acute_7d=40.0, chronic_28d=4.0)
    )
    unloaded = replace_channel(rested_body, cumulative_load=CumulativeLoad())
    assert capability_available(reference_profile, loaded) == capability_available(
        reference_profile, unloaded
    )


def _enclosing_function(tree: ast.Module, target: ast.AST) -> str | None:
    for node in ast.walk(tree):
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            continue
        for child in ast.walk(node):
            if child is target:
                return node.name
    return None


# --------------------------------------------------------------------------
# The state types refuse an invalid body (failure mode F15)
# --------------------------------------------------------------------------


@pytest.mark.parametrize(
    ("section", "value", "message"),
    [
        ("central_fatigue", 1.4, "less than or equal"),
        ("t_hours", -1.0, "greater than or equal"),
    ],
)
def test_the_type_refuses_a_body_outside_its_domains(rested_body, section, value, message) -> None:
    with pytest.raises(ValueError, match=message):
        replace_channel(rested_body, **{section: value})


@pytest.mark.parametrize(
    ("target", "update"),
    [
        ("body", {"central_fatigue": 9.0, "t_hours": -3.0}),
        ("hydration", {"deficit_pct_body_mass": -1.0}),
        ("sleep", {"circadian_phase": 24.0}),
        ("soreness", {"expressed": 2.0}),
    ],
)
def test_model_copy_cannot_bypass_body_state_domains(rested_body, target, update) -> None:
    state_model = {
        "body": rested_body,
        "hydration": rested_body.hydration,
        "sleep": rested_body.sleep,
        "soreness": rested_body.soreness.by_region[FatigueRegion.LEGS],
    }[target]

    with pytest.raises(ValueError):
        state_model.model_copy(update=update)


def test_a_body_missing_a_fatigue_region_is_refused(rested_body) -> None:
    with pytest.raises(ValueError, match="missing region"):
        replace_channel(rested_body, peripheral_fatigue={FatigueRegion.LEGS: 0.1})


def test_the_environment_of_an_advance_is_recorded_on_the_body(rested_body, dynamics_params) -> None:
    """`thermal` carries the conditions the last advance applied, as a receipt.

    Heat is environment; the body records what it was exposed to so the log and
    the snapshot can explain a core offset next to it.
    """
    hot = Environment(wbgt_c=29.5, work_rest_ratio=2.0, clothing_insulation=1.4)
    moved = advance(rested_body, 1.0, Exposure.of(environment=hot, params=dynamics_params))
    assert moved.thermal.wbgt == hot.wbgt_c
    assert moved.thermal.work_rest_ratio == hot.work_rest_ratio
    assert moved.thermal.clothing_insulation == hot.clothing_insulation


# --------------------------------------------------------------------------
# Knowledge boundary — this ticket has no context consumer at all
# --------------------------------------------------------------------------

#: Words that name the act of handing something to an agent. The body is world
#: truth and the translation to a qualitative signal is PSV1-6; until then, any
#: function that returns a `BodyState` number *out of the engine* is a leak.
CONTEXT_CONSUMER_WORDS = ("prompt", "context_payload", "describe", "narrat", "render_for", "to_agent")


def test_no_module_in_this_subdomain_builds_an_agent_payload(package_sources) -> None:
    """Spec §7.1: these numbers stay in the engine.

    There is nothing to filter here because there is nothing to filter *for* —
    the body has no context consumer in V1. The check is that it stays that way
    by accident of nobody adding one, rather than by everyone remembering.
    """
    offences: list[str] = []
    for path, tree in package_sources:
        for node in ast.walk(tree):
            if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
                continue
            lowered = node.name.lower()
            for word in CONTEXT_CONSUMER_WORDS:
                if word in lowered:
                    offences.append(f"{path.name}:{node.lineno} defines {node.name}")
    assert not offences, (
        "the interoception boundary is PSV1-6; a body-state number leaving the engine before "
        "then is a leak:\n" + "\n".join(offences)
    )


def test_the_event_log_holds_the_numbers_and_says_that_it_does(package_sources) -> None:
    """The log carries `BodyState` values, and the log never enters a prompt.

    Stated in the module that owns the log, because the invariant is a property
    of where the artefact goes and not of what it contains.
    """
    source = next(path for path, _ in package_sources if path.name == "eventlog.py").read_text(
        encoding="utf-8"
    )
    assert "never enter a prompt" in source
