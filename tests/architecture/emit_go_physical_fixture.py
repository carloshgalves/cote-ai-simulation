"""Generate actual Python physical source + A2 evidence for the Go gate test.

Fixtures are temporary CI outputs, not committed proof or a way to bypass A1.
"""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path

import yaml

from embodiment.cli import advance_clock_command, seed_cohort_command
from embodiment.eventlog import normalised_bytes
from embodiment.types import SleepQuality
from tests.architecture.embodiment_a2_transition_reference import (
    project_verified_transitions, reference_bytes,
)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    root = args.out
    root.mkdir(parents=True, exist_ok=True)
    run = root / "physical"
    seed_cohort_command(world_seed=42, count=3, out_dir=run)
    advance_clock_command(
        run_dir=run, character_id="npc.0001", days=2,
        sleep="4h", quality=SleepQuality.POOR,
    )
    advance_clock_command(
        run_dir=run, character_id="npc.0002", days=1,
        sleep="8h", quality=SleepQuality.GOOD,
    )
    snapshot = (run / "snapshot.yaml").read_bytes()
    events = (run / "events.jsonl").read_bytes()
    sha = lambda data: hashlib.sha256(data).hexdigest()
    receipts = {
        "snapshot_raw_sha256": sha(snapshot),
        "events_raw_sha256": sha(events),
        "snapshot_document_sha256": yaml.safe_load(snapshot)["snapshot_hash"],
        "normalised_log_sha256": sha(normalised_bytes(run / "events.jsonl")),
    }
    evidence = project_verified_transitions(snapshot, events, **receipts)
    assert evidence["completion"] == "ATTESTED_COMPLETE"
    (root / "snapshot.yaml").write_bytes(snapshot)
    (root / "events.jsonl").write_bytes(events)
    (root / "evidence.json").write_bytes(reference_bytes(evidence))
    (root / "pins.json").write_text(json.dumps(receipts), encoding="utf-8")


if __name__ == "__main__":
    main()
