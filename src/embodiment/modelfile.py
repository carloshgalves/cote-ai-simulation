"""Header and honesty validator for `data/models/physical/`.

Every ticket that introduces a parameter file passes through here. Three checks,
each answering a failure the spec names:

1. **Header** (spec §5.1, acceptance criterion 10) — `not_canon`, version,
   provenance and the file's *own* `evidence_sufficiency`. Validated against
   `data/models/physical/schema/model-file.schema.json`.
2. **Marking** — every number in the body is covered by a `source` marking, and
   a marking is one of `[MEASURED] [TRANSCRIBED] [EQUATED] [DERIVED] [INT]`. An
   unmarked number is rejected; `[INT]` is the marking for an assumption. A
   parameter that is an assumption and does not say so is the dishonesty the
   marking exists to prevent, so the absence of a marking cannot be silent.
3. **No ordering of characters** (invariant 8, failure mode F4) — comparisons of
   capacity are an *output* of the simulation. A parameter file or fixture that
   ranks two characters is refused. The single admissible comparison is a
   `COMPARATIVE` constraint anchored to one observed event, and it must carry
   the anchor to be accepted. This rule holds for every committed record,
   canon included, and `assert_no_character_ordering` is written to be applied
   that widely.
4. **No named character in a *model* file** (failure mode F3) — a parameter file
   describes cohorts and mechanisms. A character named in one is either a
   hand-set attribute or an ordering. This rule is narrower than rule 3 on
   purpose: canon records name characters constantly, and must.
"""

from __future__ import annotations

import math
import re
from collections.abc import Iterator, Mapping, Sequence
from pathlib import Path
from typing import Any

import jsonschema
import yaml

__all__ = [
    "ModelFileError",
    "MODEL_FILE_SCHEMA_PATH",
    "PHYSICAL_MODELS_DIR",
    "SOURCE_MARKINGS",
    "ASSUMPTION_MARKING",
    "load_model_file",
    "validate_model_file",
    "validate_mapping",
    "assert_no_character_ordering",
    "assert_no_named_characters",
]

_REPO_ROOT = Path(__file__).resolve().parents[2]
PHYSICAL_MODELS_DIR = _REPO_ROOT / "data" / "models" / "physical"
MODEL_FILE_SCHEMA_PATH = PHYSICAL_MODELS_DIR / "schema" / "model-file.schema.json"

#: Header keys are declarations about the file, not parameters of the model, so
#: the marking rule does not apply inside them.
_HEADER_KEYS = frozenset(
    {
        "schema_version",
        "not_canon",
        "model_kind",
        "model_version",
        "status",
        "provenance",
        "evidence_sufficiency",
        "transcribed_at",
        "source",
    }
)

SOURCE_MARKINGS = ("[MEASURED]", "[TRANSCRIBED]", "[EQUATED]", "[DERIVED]", "[INT]")
ASSUMPTION_MARKING = "[INT]"

_MARKING_RE = re.compile(r"^\[(MEASURED|TRANSCRIBED|EQUATED|DERIVED|INT)\]\s*\S")

#: What a character id looks like. Model files describe cohorts and mechanisms;
#: a character id appearing in one is either an ordering or a hand-set attribute,
#: and both are refused.
_CHARACTER_ID_RE = re.compile(r"^(actor|char|npc|student)\.[a-z0-9][a-z0-9._-]*$")

#: Relational key names that assert one subject exceeds another. Other ranking
#: fields are recognised by their component words so aliases such as
#: `strength_order` and `ranked_by` cannot bypass the invariant.
_RELATIONAL_ORDERING_KEYS = frozenset(
    {
        "stronger_than",
        "faster_than",
        "superior_to",
        "beats",
        "outperforms",
        "better_than",
    }
)
_ORDERING_WORDS = frozenset({"order", "ordering", "rank", "ranked", "ranking", "placement"})


class ModelFileError(ValueError):
    """A model file that may not be loaded. The message names the offending path."""


def load_model_file(path: str | Path) -> dict[str, Any]:
    """Read, validate and return a parameter file. The only loading door."""
    path = Path(path)
    data = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise ModelFileError(f"{path}: a model file must be a mapping at the top level")
    validate_mapping(data, origin=str(path))
    return data


def validate_model_file(path: str | Path) -> None:
    load_model_file(path)


def validate_mapping(data: Mapping[str, Any], *, origin: str = "<mapping>") -> None:
    _validate_header(data, origin)
    _assert_every_number_is_marked(data, origin)
    assert_no_character_ordering(data, origin=origin)
    assert_no_named_characters(data, origin=origin)


def _validate_header(data: Mapping[str, Any], origin: str) -> None:
    schema = _load_schema()
    validator = jsonschema.Draft202012Validator(schema)
    errors = sorted(validator.iter_errors(data), key=lambda error: list(error.path))
    if errors:
        first = errors[0]
        location = "/".join(str(part) for part in first.path) or "<root>"
        raise ModelFileError(f"{origin}: invalid model file header at {location}: {first.message}")


_SCHEMA_CACHE: dict[str, Any] = {}


def _load_schema() -> dict[str, Any]:
    if not _SCHEMA_CACHE:
        import json

        _SCHEMA_CACHE.update(json.loads(MODEL_FILE_SCHEMA_PATH.read_text(encoding="utf-8")))
    return _SCHEMA_CACHE


def _walk(node: Any, path: str) -> Iterator[tuple[str, Any]]:
    yield path, node
    if isinstance(node, Mapping):
        for key, value in node.items():
            yield from _walk(value, f"{path}.{key}" if path else str(key))
    elif isinstance(node, Sequence) and not isinstance(node, (str, bytes)):
        for index, value in enumerate(node):
            yield from _walk(value, f"{path}[{index}]")


def _is_number(value: Any) -> bool:
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        return True
    if isinstance(value, str):
        try:
            return math.isfinite(float(value))
        except ValueError:
            return False
    return False


def _assert_every_number_is_marked(data: Mapping[str, Any], origin: str) -> None:
    """Refuse a numeric parameter that no `source` marking covers.

    A marking on an ancestor covers everything under it: parameter groups are
    sourced together far more often than one number at a time, and requiring a
    marking per number would push authors to copy a marking they stopped reading.
    """
    body = {key: value for key, value in data.items() if key not in _HEADER_KEYS}
    _check_marked(body, marking=None, path="", origin=origin)


def _check_marked(node: Any, *, marking: str | None, path: str, origin: str) -> None:
    if isinstance(node, Mapping):
        own = node.get("source")
        if own is not None:
            if not isinstance(own, str) or not _MARKING_RE.match(own):
                raise ModelFileError(
                    f"{origin}: source at {path or '<root>'} must start with one of "
                    f"{', '.join(SOURCE_MARKINGS)} followed by a description, got {own!r}"
                )
            marking = own
        for key, value in node.items():
            if key == "source":
                continue
            child = f"{path}.{key}" if path else str(key)
            if _is_number(value) and marking is None:
                raise ModelFileError(
                    f"{origin}: numeric parameter at {child} carries no source marking. "
                    f"Add a `source` on it or on an enclosing block; an assumption is "
                    f"marked {ASSUMPTION_MARKING}."
                )
            _check_marked(value, marking=marking, path=child, origin=origin)
    elif isinstance(node, Sequence) and not isinstance(node, (str, bytes)):
        for index, value in enumerate(node):
            child = f"{path}[{index}]"
            if _is_number(value) and marking is None:
                raise ModelFileError(
                    f"{origin}: numeric parameter at {child} carries no source marking. "
                    f"Add a `source` on it or on an enclosing block; an assumption is "
                    f"marked {ASSUMPTION_MARKING}."
                )
            _check_marked(value, marking=marking, path=child, origin=origin)


def assert_no_character_ordering(data: Mapping[str, Any], *, origin: str = "<mapping>") -> None:
    """Invariant 8: comparisons between characters are output, never input.

    Applies to every committed record, canon included: no record may state that
    one character exceeds another. The one exception is a `COMPARATIVE`
    constraint anchored to a specific observed event, which is the only
    admissible comparison in the whole model — and it is only an exception if it
    actually carries the anchor.
    """
    for path, node, exempt in _mappings(data):
        if exempt:
            continue
        for key, value in node.items():
            if not _looks_like_ordering_key(key):
                continue
            subject_ids = set(_character_ids_below(value))
            if _is_relational_ordering_key(key):
                for sibling_key, sibling_value in node.items():
                    if sibling_key == key:
                        continue
                    subject_ids.update(_character_ids_below(sibling_key))
                    subject_ids.update(_character_ids_below(sibling_value))
            if len(subject_ids) >= 2:
                here = f"{path}.{key}" if path else str(key)
                raise ModelFileError(
                    f"{origin}: {here} orders subjects. Capacity comparisons are an output of the "
                    f"simulation (invariant 8); the only admissible input comparison is a "
                    f"COMPARATIVE constraint anchored to one observed event."
                )


def assert_no_named_characters(data: Mapping[str, Any], *, origin: str = "<mapping>") -> None:
    """A model file names no individual (failure mode F3).

    Deliberately narrower in scope than `assert_no_character_ordering`: canon
    records name characters and must, while a parameter file that does is either
    hand-setting an attribute or ranking people.
    """
    for path, node, exempt in _mappings(data):
        if exempt:
            continue
        for key, value in node.items():
            for offender in (*_character_ids_in(key), *_character_ids_in(value)):
                here = f"{path}.{key}" if path else str(key)
                raise ModelFileError(
                    f"{origin}: {here} names character {offender!r}. Parameter files describe "
                    f"cohorts and mechanisms, never individuals: a character here is either a "
                    f"hand-set attribute (F3) or an ordering (F4)."
                )


def _mappings(data: Mapping[str, Any]) -> Iterator[tuple[str, Mapping[str, Any], bool]]:
    """Every mapping in the document, flagged if it sits inside an anchored comparative."""
    exempt_prefixes: list[str] = []
    for path, node in _walk(data, ""):
        if not isinstance(node, Mapping):
            continue
        inside = any(path == prefix or path.startswith(f"{prefix}.") for prefix in exempt_prefixes)
        if not inside and _is_anchored_comparative(node):
            exempt_prefixes.append(path)
            inside = True
        yield path, node, inside


def _is_anchored_comparative(node: Mapping[str, Any]) -> bool:
    if node.get("constraint_type") != "COMPARATIVE":
        return False
    if not node.get("anchored_to_event"):
        raise ModelFileError(
            "a COMPARATIVE constraint must carry `anchored_to_event`: a comparison that is not "
            "anchored to one observed event is a ranking (invariant 8)"
        )
    return True


def _looks_like_ordering_key(key: Any) -> bool:
    if not isinstance(key, str):
        return False
    normalised = re.sub(r"[^a-z0-9]+", "_", key.lower()).strip("_")
    words = set(normalised.split("_"))
    return (
        _is_relational_ordering_key(key)
        or bool(words & _ORDERING_WORDS)
    )


def _is_relational_ordering_key(key: Any) -> bool:
    if not isinstance(key, str):
        return False
    normalised = re.sub(r"[^a-z0-9]+", "_", key.lower()).strip("_")
    return normalised in _RELATIONAL_ORDERING_KEYS or normalised.endswith("_than")


def _character_ids_below(value: Any) -> Iterator[str]:
    """Character ids anywhere below one candidate ordering field."""
    if isinstance(value, str):
        if _CHARACTER_ID_RE.match(value):
            yield value
    elif isinstance(value, Mapping):
        for key, child in value.items():
            yield from _character_ids_below(key)
            yield from _character_ids_below(child)
    elif isinstance(value, Sequence) and not isinstance(value, (str, bytes)):
        for item in value:
            yield from _character_ids_below(item)


def _character_ids_in(value: Any) -> Iterator[str]:
    if isinstance(value, str):
        if _CHARACTER_ID_RE.match(value):
            yield value
    elif isinstance(value, Sequence) and not isinstance(value, (str, bytes)):
        for item in value:
            yield from _character_ids_in(item)
