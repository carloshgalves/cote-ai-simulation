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
   `COMPARATIVE` constraint anchored to one observed event, in the shape
   `data/canon/schema/feat.schema.json` defines — `inference.constraint`,
   `inference.comparative.same_event`, and actors declared on the record. The
   gate consumes that contract rather than a spelling of its own, so a record
   the real feat validator would reject cannot be green here. This rule holds
   for every committed record, canon included, and `assert_no_character_ordering`
   is written to be applied that widely.
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

#: `feat.schema.json` `properties.id.pattern`. The feat record *is* the observed
#: event a COMPARATIVE constraint anchors to, so its id is the anchor.
_FEAT_ID_RE = re.compile(r"^feat\.[a-z0-9.-]+$")

#: `feat.schema.json` `properties`. A record recognised as the one admissible
#: comparison is exempt *inside its own contract* and nowhere else: the schema is
#: `additionalProperties: false`, so a field the real feat validator would reject
#: cannot borrow the exception invariant 8 grants to the comparison itself.
_FEAT_RECORD_FIELDS = frozenset(
    {
        "actors",
        "conditions",
        "divergence_sensitive",
        "effort",
        "evidence_refs",
        "id",
        "inference",
        "measurement",
        "modality",
        "notes_ref",
        "observers",
        "provenance",
        "revision",
        "schema_version",
        "story_time",
        "supports_claims",
        "tags",
    }
)

#: Header keys that make a document one of ours rather than canon. A model file
#: is `not_canon: true` by construction, so it is never the observed event a
#: COMPARATIVE constraint anchors to, and its root may not claim the exemption.
_MODEL_FILE_HEADER_KEYS = frozenset({"not_canon", "model_kind", "model_version"})

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
        #: The spelling `feat.schema.json` actually uses for the relation.
        "outperformed",
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
    for path, node, exempt_keys in _mappings(data):
        for key, value in node.items():
            if key in exempt_keys or not _looks_like_ordering_key(key):
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
    for path, node, exempt_keys in _mappings(data):
        for key, value in node.items():
            if key in exempt_keys:
                continue
            for offender in (*_character_ids_in(key), *_character_ids_in(value)):
                here = f"{path}.{key}" if path else str(key)
                raise ModelFileError(
                    f"{origin}: {here} names character {offender!r}. Parameter files describe "
                    f"cohorts and mechanisms, never individuals: a character here is either a "
                    f"hand-set attribute (F3) or an ordering (F4)."
                )


def _mappings(data: Mapping[str, Any]) -> Iterator[tuple[str, Mapping[str, Any], frozenset[str]]]:
    """Every mapping in the document, with the keys an anchored comparative exempts.

    The exemption is granted per key rather than over a subtree prefix. A prefix
    of `""` — a feat record committed as its own file, which is the root mapping —
    used to match every path in the document, so one `COMPARATIVE` record at the
    root switched both gates off for its siblings too. Invariant 8 admits *one
    anchored comparison*, not the file that carries it, so the exemption now
    reaches exactly the fields `feat.schema.json` defines and stops there.
    """
    exempt = _exempt_paths(data)
    for path, node in _walk(data, ""):
        if not isinstance(node, Mapping):
            continue
        yield path, node, frozenset(key for key in node if _child_path(path, key) in exempt)


def _child_path(path: str, key: Any) -> str:
    return f"{path}.{key}" if path else str(key)


def _exempt_paths(data: Mapping[str, Any]) -> frozenset[str]:
    """Paths covered by an anchored comparative's own contract."""
    is_model_file = bool(_MODEL_FILE_HEADER_KEYS & set(data))
    exempt: set[str] = set()
    for path, node in _walk(data, ""):
        if not isinstance(node, Mapping) or path in exempt:
            continue
        if path == "" and is_model_file:
            #: A parameter file describing a cohort is not an observed event. Were
            #: its root allowed to claim the exception, a model file could wear a
            #: feat's fields and carry hand-authored profiles as siblings.
            continue
        if _is_anchored_comparative(node):
            exempt.update(_contract_paths(node, path))
    return frozenset(exempt)


def _contract_paths(node: Mapping[str, Any], path: str) -> Iterator[str]:
    """The comparative record itself, and everything under its declared fields."""
    yield path
    for key, value in node.items():
        if key not in _FEAT_RECORD_FIELDS:
            continue
        for child, _ in _walk(value, _child_path(path, key)):
            yield child


def _is_anchored_comparative(node: Mapping[str, Any]) -> bool:
    """Is this record the one admissible comparison, in the shape the schema defines?

    The contract is `data/canon/schema/feat.schema.json`, not a spelling invented
    here: a feat naming its `actors`, carrying `inference.constraint:
    COMPARATIVE` and an `inference.comparative` block whose `same_event` is true.
    Recognising anything else would leave the gate green on records the real
    validator rejects, and red on the records canon will actually hold.

    A record that opens the exception and then fails to satisfy it is an error,
    not a non-match: `COMPARATIVE` is the only way past invariant 8, so a
    malformed one is a ranking wearing the exemption's name.
    """
    if node.get("constraint_type") == "COMPARATIVE":
        #: A spelling `feat.schema.json` does not define, and the schema is
        #: `additionalProperties: false`. Refused rather than ignored: a record the
        #: real validator would reject must not be able to claim the one exemption
        #: invariant 8 grants, and silently not recognising it is how a ranking
        #: gets in under a constraint's name.
        raise ModelFileError(
            "a COMPARATIVE constraint is declared as `inference.constraint: COMPARATIVE` with an "
            "`inference.comparative` block, per data/canon/schema/feat.schema.json. "
            "`constraint_type` is not part of that contract, so this record is not the anchored "
            "comparison invariant 8 admits."
        )

    inference = node.get("inference")
    if not isinstance(inference, Mapping):
        return False
    if inference.get("constraint") != "COMPARATIVE":
        return False
    _assert_comparative_contract(node, inference)
    return True


def _assert_comparative_contract(node: Mapping[str, Any], inference: Mapping[str, Any]) -> None:
    comparative = inference.get("comparative")
    if not isinstance(comparative, Mapping):
        raise ModelFileError(
            "a COMPARATIVE constraint must carry `inference.comparative`, naming who "
            "outperformed whom: the constraint alone is not anchored to anything (invariant 8)"
        )

    #: The schema pins this to `const: true` for a reason it states itself: "a
    #: comparison across different conditions is not a comparison". `is not True`
    #: rather than falsiness, so a truthy string cannot stand in for the boolean.
    if comparative.get("same_event") is not True:
        raise ModelFileError(
            "a COMPARATIVE constraint must set `same_event: true`: a comparison anchored to "
            "two different events compares conditions, not capacities (invariant 8)"
        )

    by_whom = comparative.get("by_whom")
    outperformed = comparative.get("outperformed")
    if not isinstance(by_whom, str) or not by_whom:
        raise ModelFileError(
            "a COMPARATIVE constraint must name `by_whom` as a single actor id (invariant 8)"
        )
    if (
        not isinstance(outperformed, Sequence)
        or isinstance(outperformed, (str, bytes))
        or not outperformed
        or not all(isinstance(item, str) and item for item in outperformed)
    ):
        raise ModelFileError(
            "a COMPARATIVE constraint must list at least one actor id in `outperformed` "
            "(invariant 8)"
        )

    actors = node.get("actors")
    if (
        not isinstance(actors, Sequence)
        or isinstance(actors, (str, bytes))
        or not actors
    ):
        raise ModelFileError(
            "a COMPARATIVE feat must declare its `actors`: a comparison whose participants "
            "are not on the record cannot be checked against the event (invariant 8)"
        )
    undeclared = sorted({by_whom, *outperformed} - set(actors))
    if undeclared:
        raise ModelFileError(
            f"a COMPARATIVE constraint compares {undeclared}, who are not among the feat's "
            f"declared `actors`: the comparison is not anchored to the event it claims "
            f"(invariant 8)"
        )

    #: The anchor the contract provides is the feat record itself — its id and the
    #: story time it happened at. A comparison with neither is a ranking with a
    #: constraint label on it.
    record_id = node.get("id")
    if not isinstance(record_id, str) or not _FEAT_ID_RE.match(record_id):
        raise ModelFileError(
            "a COMPARATIVE constraint must be anchored to one observed event: the feat needs "
            "an `id` of the form `feat.<slug>` (invariant 8)"
        )
    if not node.get("story_time"):
        raise ModelFileError(
            "a COMPARATIVE constraint must be anchored to one observed event: the feat needs "
            "a `story_time` (invariant 8)"
        )


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
