"""A1 closed producer: create one complete PSV1-2 run in a private host stage.

The Go host owns the directory, process lifetime, source hashes and signing key.
This process has no CSF ledger, global clock, event or commit capability.
"""
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path

import yaml

from .a2_projection import project_verified_transitions, reference_bytes
from .cli import advance_clock_command, seed_cohort_command
from .eventlog import normalised_bytes
from .types import SleepQuality

def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--plan", type=Path, required=True)
    args=parser.parse_args()
    if not args.out.is_absolute() or not args.plan.is_absolute():
        raise ValueError("A1 source staging requires host absolute paths")
    plan=json.loads(args.plan.read_text(encoding="utf-8"))
    if type(plan) is not dict or set(plan)!={"world_seed","count","advances"}:
        raise ValueError("A1 plan has an unregistered field")
    seed,count,advances=plan["world_seed"],plan["count"],plan["advances"]
    if type(seed) is not int or type(count) is not int or not 1<=count<=4096:
        raise ValueError("A1 seed/count out of registered bounds")
    if type(advances) is not list or not 1<=len(advances)<=count:
        raise ValueError("A1 requires a bounded positive set of completed advances")
    seen=set()
    for cmd in advances:
        if type(cmd) is not dict or set(cmd)!={"subject","days","sleep","quality"}:
            raise ValueError("A1 has an unknown advance command")
        subject,days,sleep,quality=(cmd[k] for k in ("subject","days","sleep","quality"))
        if (type(subject) is not str or subject in seen or
            type(days) is not int or not 1<=days<=30 or
            type(sleep) is not str or sleep not in ("4h","6h","8h") or
            quality not in ("good","fair","poor")):
            raise ValueError("A1 command not within registered deterministic plan")
        seen.add(subject)
    # Do not create files before validating the full closed plan.
    seed_cohort_command(world_seed=seed,count=count,out_dir=args.out)
    for cmd in advances:
        advance_clock_command(run_dir=args.out,character_id=cmd["subject"],
            days=cmd["days"],sleep=cmd["sleep"],quality=SleepQuality(cmd["quality"]))
    snap=(args.out/"snapshot.yaml").read_bytes()
    events=(args.out/"events.jsonl").read_bytes()
    h=lambda raw:hashlib.sha256(raw).hexdigest()
    proof=project_verified_transitions(snap,events,
        snapshot_raw_sha256=h(snap),events_raw_sha256=h(events),
        snapshot_document_sha256=yaml.safe_load(snap)["snapshot_hash"],
        normalised_log_sha256=h(normalised_bytes(args.out/"events.jsonl")))
    (args.out/"evidence.json").write_bytes(reference_bytes(proof))

if __name__=="__main__":
    main()
