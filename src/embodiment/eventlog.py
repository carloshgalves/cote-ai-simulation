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
    "EVENT_RUN_STARTED",
    "EVENT_CAPACITY_SAMPLED",
    "EVENT_COHORT_CORRELATION_REPORT",
    "NORMALISED_WALL_TIME",
    "normalised_bytes",
    "normalised_digest",
]

EVENT_RUN_STARTED = "run.started"
EVENT_CAPACITY_SAMPLED = "capacity.sampled"
EVENT_COHORT_CORRELATION_REPORT = "cohort.correlation_report"

#: Wall time is the one field that legitimately differs between two runs of the
#: same seed, so determinism is stated over the log with wall clocks replaced.
NORMALISED_WALL_TIME = "1970-01-01T00:00:00+00:00"


class EventLog:
    """A JSONL file that is only ever appended to."""

    def __init__(
        self,
        path: str | Path,
        *,
        metadata: RunMetadata,
        required_components: tuple[str, ...],
        sim_time: str = Y1_START,
    ) -> None:
        metadata.require(required_components)  # fail at START, not at first write
        self.path = Path(path)
        self.metadata = metadata
        self.sim_time = sim_time
        self._sequence = 0
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._handle = self.path.open("x", encoding="utf-8")
        self.append(
            EVENT_RUN_STARTED,
            {
                "run_metadata": metadata.as_dict(),
                "required_components": list(required_components),
            },
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
