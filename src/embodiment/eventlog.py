"""Append-only event log — the audit artefact of the subdomain.

Spec §5.4 and §9. Everything the engine decides physically is appended here, and
the numbers it holds **never enter a prompt**: the log and the snapshot are
engine artefacts, not retrievable documents (spec §7.1).

The log opens with the run metadata of spec §9.2, and **the run fails to start**
if a mandatory field is missing. That is deliberate ordering: a run that has
already sampled forty bodies before discovering it cannot say which prior they
came from has produced numbers nobody can interpret afterwards.
"""

from __future__ import annotations

import json
from collections.abc import Mapping
from datetime import UTC, datetime
from pathlib import Path
from types import TracebackType

from .types import RunMetadata, Y1_START

__all__ = [
    "EventLog",
    "RunContinuityError",
    "EVENT_RUN_STARTED",
    "EVENT_RUN_EXTENDED",
    "EVENT_CAPACITY_SAMPLED",
    "EVENT_COHORT_CORRELATION_REPORT",
    "EVENT_BODY_ADVANCED",
    "NORMALISED_WALL_TIME",
    "normalised_bytes",
    "normalised_digest",
]

EVENT_RUN_STARTED = "run.started"
#: A later phase of the same run, appended to the same log. It carries the run
#: metadata again because a phase may engage machinery the first one did not —
#: advancing a clock engages the dynamics, and spec §9.2 wants that version
#: recorded where the numbers it produced are.
EVENT_RUN_EXTENDED = "run.extended"
EVENT_CAPACITY_SAMPLED = "capacity.sampled"
EVENT_COHORT_CORRELATION_REPORT = "cohort.correlation_report"
EVENT_BODY_ADVANCED = "body.advanced"

#: Wall time is the one field that legitimately differs between two runs of the
#: same seed, so determinism is stated over the log with wall clocks replaced.
NORMALISED_WALL_TIME = "1970-01-01T00:00:00+00:00"


class RunContinuityError(RuntimeError):
    """A later phase was about to be appended to a log that describes another run.

    The log is the audit artefact. The moment it holds two phases that disagree
    about the seed, the prior or the cohort, no reader can tell which of them is
    the run that happened — and every field in it still looks well formed.
    """


class EventLog:
    """A JSONL file that is only ever appended to."""

    def __init__(
        self,
        path: str | Path,
        *,
        metadata: RunMetadata,
        required_components: tuple[str, ...],
        sim_time: str = Y1_START,
        continuing: bool = False,
    ) -> None:
        metadata.require(required_components)  # fail at START, not at first write
        self.path = Path(path)
        self.metadata = metadata
        self.sim_time = sim_time
        self._sequence = 0
        self.path.parent.mkdir(parents=True, exist_ok=True)
        if continuing:
            self._sequence, opening = _read_opening(self.path)
            _assert_same_run(self.path, opening, metadata)
            self._handle = self.path.open("a", encoding="utf-8")
            self.append(
                EVENT_RUN_EXTENDED,
                {
                    "run_metadata": metadata.as_dict(),
                    "required_components": list(required_components),
                },
            )
            return
        self._handle = self.path.open("x", encoding="utf-8")
        self.append(
            EVENT_RUN_STARTED,
            {
                "run_metadata": metadata.as_dict(),
                "required_components": list(required_components),
            },
        )

    @classmethod
    def extend(
        cls,
        path: str | Path,
        *,
        metadata: RunMetadata,
        required_components: tuple[str, ...],
        sim_time: str = Y1_START,
    ) -> EventLog:
        """Open an existing run's log to append a later phase to it.

        A run is one log. A second phase that wrote its own file would leave the
        audit trail of one world in two places, ordered by nothing.
        """
        return cls(
            path,
            metadata=metadata,
            required_components=required_components,
            sim_time=sim_time,
            continuing=True,
        )

    def append(self, event: str, payload: Mapping[str, object]) -> dict[str, object]:
        record: dict[str, object] = {
            "seq": self._sequence,
            "event": event,
            "sim_time": self.sim_time,
            "wall_time": datetime.now(UTC).isoformat(),
            "payload": dict(payload),
        }
        self._sequence += 1
        self._handle.write(json.dumps(record, ensure_ascii=False, sort_keys=True) + "\n")
        self._handle.flush()
        return record

    def close(self) -> None:
        if not self._handle.closed:
            self._handle.close()

    def __enter__(self) -> EventLog:
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        traceback: TracebackType | None,
    ) -> None:
        self.close()


def _read_opening(path: Path) -> tuple[int, Mapping[str, object]]:
    """The next sequence number, and the `run.started` this log opened with."""
    if not path.is_file():
        raise RunContinuityError(
            f"{path} does not exist: a phase can only be appended to a run that started"
        )
    sequence = 0
    opening: Mapping[str, object] | None = None
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        record = json.loads(line)
        sequence = int(record["seq"]) + 1
        if opening is None:
            if record.get("event") != EVENT_RUN_STARTED:
                raise RunContinuityError(
                    f"{path} does not open with {EVENT_RUN_STARTED}: it is not a run log"
                )
            opening = record["payload"]["run_metadata"]
    if opening is None:
        raise RunContinuityError(f"{path} is empty: there is no run here to continue")
    return sequence, opening


def _assert_same_run(path: Path, opening: Mapping[str, object], metadata: RunMetadata) -> None:
    """Refuse to continue a log that describes a different world."""
    current = metadata.as_dict()
    if opening.get("world_seed") != current.get("world_seed"):
        raise RunContinuityError(
            f"{path} opened under world_seed {opening.get('world_seed')}, but this phase "
            f"declares {current.get('world_seed')}"
        )
    if opening.get("posterior_hash_by_character") != current.get("posterior_hash_by_character"):
        raise RunContinuityError(
            f"{path} opened over a different cohort: the posterior hashes of this phase do not "
            f"match the ones the run started with"
        )
    for key, value in opening.items():
        if not key.endswith("_version"):
            continue
        if current.get(key) != value:
            raise RunContinuityError(
                f"{path} opened with {key} {value!r}, but this phase declares "
                f"{current.get(key)!r}. Bodies advanced under one version of a parameter file "
                f"do not mean the same thing under another."
            )


def normalised_bytes(path: str | Path) -> bytes:
    """The log with wall clocks flattened — the form determinism is stated over."""
    lines = []
    for line in Path(path).read_text(encoding="utf-8").splitlines():
        record = json.loads(line)
        record["wall_time"] = NORMALISED_WALL_TIME
        lines.append(json.dumps(record, ensure_ascii=False, sort_keys=True))
    return ("\n".join(lines) + "\n").encode("utf-8")


def normalised_digest(path: str | Path) -> str:
    import hashlib

    return hashlib.sha256(normalised_bytes(path)).hexdigest()
