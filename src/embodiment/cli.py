"""`python -m embodiment` — the spine of the tracer bullet, not an extra.

    python -m embodiment seed-cohort --world-seed 42 --n 40 --out runs/demo/

seeds N anonymous students from the population prior, freezes one
`capacity_baseline` each, gives each of them a rested body, writes
`capacity.sampled` to the event log and a snapshot with complete `run_metadata`.
Running it again with the same seed produces the same log; running it with
`--n 41` produces the same forty bodies plus one.

    python -m embodiment advance-clock --run runs/demo/ --character npc.0017 \
        --days 3 --sleep 4h --quality poor

moves one body three days forward on four hours of bad sleep a night and shows,
per channel, the before and the after — and `capability_available` per dimension
at both ends. Coordination and reaction time fall visibly; maximal strength
barely moves. Running it with `--days 0` changes nothing at all, and that is as
much the point of the subcommand as the other case: a scene heals nothing, and
neither does looking at a body.

Evidence of what happened is the **event log**, not this terminal output.
"""

from __future__ import annotations

import argparse
import itertools
from collections.abc import Mapping, Sequence
from pathlib import Path

import numpy as np

from .capability import capability_available
from .dynamics import (
    BodyTraits,
    Environment,
    Exposure,
    advance,
    channel_diff,
    initial_body_state,
    load_params,
)
from .eventlog import (
    EVENT_BODY_ADVANCED,
    EVENT_COHORT_CORRELATION_REPORT,
    EventLog,
    body_has_advanced,
    logical_sim_time,
    normalised_digest,
)
from .prior import POPULATION_PRIOR_PATH, PopulationPrior
from .seeding import CapacityBaselineStore, Posterior, cohort_ids, posterior_for, seed_character
from .snapshot import body_state_of, capacity_profile_of, read_snapshot, write_snapshot
from .types import BodyState, Dimension, RunMetadata, SleepQuality, Y1_START, as_document

__all__ = [
    "main",
    "seed_cohort_command",
    "advance_clock_command",
    "correlation_report",
    "day_segments",
    "parse_hours",
    "print_advance",
]

#: The components a seeding run engages, and therefore whose parameter versions
#: spec §9.2 makes mandatory. The prior draws the bodies and the dynamics decide
#: what a rested one looks like; there is no estimator, no contest resolver and
#: no observation in either ticket, and declaring versions for machinery that did
#: not run would be metadata that lies.
SEEDING_COMPONENTS: tuple[str, ...] = ("prior", "dynamics")

#: Advancing a clock engages exactly the same two: the prior, because the body
#: being advanced was drawn from it, and the dynamics, because they are what
#: moves it.
ADVANCE_COMPONENTS: tuple[str, ...] = ("prior", "dynamics")


class BodyAlreadyAdvancedError(RuntimeError):
    """The seed snapshot cannot safely be advanced twice before PSV1-8."""

#: Hours in a simulated day. A day that is not 24 h is a different world model.
HOURS_PER_DAY = 24.0

#: Pairs whose sign the prior's declared loadings commit to, checked on every
#: run rather than only in the test suite (failure mode F7).
REPORTED_PAIRS: tuple[tuple[Dimension, Dimension], ...] = (
    (Dimension.MAX_STRENGTH, Dimension.BODY_MASS),
    (Dimension.BODY_MASS, Dimension.AEROBIC_CAPACITY),
    (Dimension.SPRINT_SPEED, Dimension.AEROBIC_CAPACITY),
    (Dimension.STATURE, Dimension.BODY_MASS),
    (Dimension.MAX_STRENGTH, Dimension.AEROBIC_CAPACITY),
)

#: Below this the declared structure says nothing worth checking a sign against.
MATERIAL_CORRELATION = 0.15


def expected_correlation(prior: PopulationPrior, first: Dimension, second: Dimension) -> float:
    """The latent correlation the two declared loadings imply, on the copula scale."""
    a, b = prior.loadings[first], prior.loadings[second]
    return a.general_fitness * b.general_fitness + a.build * b.build


def _copula_scale(prior: PopulationPrior, store: CapacityBaselineStore, posteriors: Mapping[str, Posterior]) -> dict[Dimension, np.ndarray]:
    """Each body's value as a z-score **within its own sex marginal**.

    Correlations are measured here rather than on raw units on purpose: on raw
    units a mixed-sex cohort would show the difference between the two marginals
    and call it structure. On the copula scale what is left is the dependence the
    prior actually declares.
    """
    columns: dict[Dimension, list[float]] = {dimension: [] for dimension in Dimension}
    for character_id, record in sorted(store.items()):
        sex = posteriors[character_id].sex
        for dimension, entry in record.profile.dimensions.items():
            columns[dimension].append(prior.z_score_of(dimension, entry.value, sex))
    return {dimension: np.asarray(values, dtype=float) for dimension, values in columns.items()}


def correlation_report(
    prior: PopulationPrior,
    store: CapacityBaselineStore,
    posteriors: Mapping[str, Posterior],
) -> dict[str, object]:
    """Does the sampled cohort show the correlation structure of the prior?

    Reported on every run, because the claim that a cohort has structure is the
    one thing a seeding run can be wrong about without anything failing.
    """
    columns = _copula_scale(prior, store, posteriors)
    size = len(next(iter(columns.values()))) if columns else 0

    def observed(first: Dimension, second: Dimension) -> float | None:
        if size < 3:
            return None
        x, y = columns[first], columns[second]
        if x.std() == 0 or y.std() == 0:
            return None
        return float(np.corrcoef(x, y)[0, 1])

    pairs = []
    for first, second in REPORTED_PAIRS:
        expected = expected_correlation(prior, first, second)
        seen = observed(first, second)
        pairs.append(
            {
                "dimensions": [first.value, second.value],
                "expected_from_loadings": round(expected, 4),
                "observed": None if seen is None else round(seen, 4),
                "sign_agrees": None if seen is None else (seen * expected > 0),
            }
        )

    agree = disagree = 0
    for first, second in itertools.combinations(list(Dimension), 2):
        expected = expected_correlation(prior, first, second)
        if abs(expected) < MATERIAL_CORRELATION:
            continue
        seen = observed(first, second)
        if seen is None:
            continue
        if seen * expected > 0:
            agree += 1
        else:
            disagree += 1

    return {
        "cohort_size": size,
        "scale": "copula (each value as a z-score within its own sex marginal)",
        "method": "pearson correlation across the sampled cohort",
        "prior_version": prior.version,
        "prior_status": prior.status,
        "pairs": pairs,
        "material_pairs_threshold": MATERIAL_CORRELATION,
        "material_pairs_sign_agreement": {"agree": agree, "disagree": disagree},
        "note": (
            "The sign of a pair is what the declared loadings commit to; the magnitude is "
            "[INT] while SOURCING S2 is open. A disagreeing sign in a cohort of this size is "
            "sampling noise unless it persists across seeds."
        ),
    }


def seed_cohort_command(
    *,
    world_seed: int,
    count: int,
    out_dir: str | Path,
    prior_path: str | Path = POPULATION_PRIOR_PATH,
    id_prefix: str = "npc",
    start_index: int = 1,
) -> dict[str, object]:
    """Seed a cohort into `out_dir/{events.jsonl,snapshot.yaml}`."""
    if count < 1:
        raise ValueError("a cohort needs at least one student: there is nothing to seed otherwise")
    out_dir = Path(out_dir)
    events_path = out_dir / "events.jsonl"
    snapshot_path = out_dir / "snapshot.yaml"
    existing = [path for path in (events_path, snapshot_path) if path.exists()]
    if existing:
        names = ", ".join(path.name for path in existing)
        raise FileExistsError(
            f"run artifact already exists in {out_dir}: {names}; choose an empty output directory"
        )
    prior = PopulationPrior.load(prior_path)
    params = load_params()
    ids = cohort_ids(count, prefix=id_prefix, start=start_index)

    # Posteriors first: the run metadata of §9.2 must be complete BEFORE the log
    # opens, so that a run missing a mandatory field fails at the start rather
    # than after sampling forty bodies nobody can interpret afterwards.
    posteriors: dict[str, Posterior] = {
        character_id: posterior_for(world_seed, character_id, prior) for character_id in ids
    }
    metadata = RunMetadata.from_mapping(
        {
            "world_seed": world_seed,
            "component_versions": {"prior": prior.version, "dynamics": params.model_version},
            "posterior_hash_by_character": {
                character_id: posterior.posterior_hash for character_id, posterior in posteriors.items()
            },
            "evidence_sufficiency_by_character": {
                character_id: dict(posterior.evidence_sufficiency)
                for character_id, posterior in posteriors.items()
            },
        },
        required_components=SEEDING_COMPONENTS,
    )

    store = CapacityBaselineStore()
    with EventLog(
        events_path, metadata=metadata, required_components=SEEDING_COMPONENTS, sim_time=Y1_START
    ) as log:
        for character_id in ids:
            seed_character(world_seed, character_id, prior, store=store, log=log)
        log.append(EVENT_COHORT_CORRELATION_REPORT, correlation_report(prior, store, posteriors))

    #: A seeded character has a body. It is rested, it is at the world's origin
    #: instant, and it goes into the snapshot with the baseline it belongs to —
    #: `advance-clock` has something to move only because this exists.
    bodies = {
        character_id: initial_body_state(character_id, store.get(character_id).profile, params=params)
        for character_id in ids
    }

    snapshot_digest = write_snapshot(
        snapshot_path,
        world_seed=world_seed,
        store=store,
        metadata=metadata,
        posteriors=posteriors,
        bodies=bodies,
    )

    return {
        "events": events_path,
        "snapshot": snapshot_path,
        "snapshot_hash": snapshot_digest,
        "normalised_log_sha256": normalised_digest(events_path),
        "seeded": len(store),
        "prior_version": prior.version,
        "prior_status": prior.status,
        "dynamics_version": params.model_version,
    }


def parse_hours(text: str) -> float:
    """Parse `4h`, `30m`, `1.5h` or a bare number of hours."""
    raw = text.strip().lower()
    for suffix, factor in (("min", 1.0 / 60.0), ("m", 1.0 / 60.0), ("h", 1.0), ("", 1.0)):
        if suffix and not raw.endswith(suffix):
            continue
        body = raw[: len(raw) - len(suffix)] if suffix else raw
        try:
            hours = float(body)
        except ValueError:
            break
        if hours < 0:
            raise ValueError(f"a duration cannot be negative: {text!r}")
        return hours * factor
    raise ValueError(f"cannot read {text!r} as a duration; try 4h, 30m or 1.5h")


def day_segments(sleep_hours: float, quality: SleepQuality, wbgt_c: float) -> tuple[tuple[str, float, Environment], ...]:
    """One simulated day, as the intervals the engine actually advances over.

    Awake first and then asleep, so that a night of four hours beginning at 03:00
    is a night and not an afternoon. Each segment is a separate `advance` with its
    own explicit interval: there is no call that covers a day and decides
    internally when the body slept.
    """
    if not 0.0 <= sleep_hours <= HOURS_PER_DAY:
        raise ValueError(f"a night of {sleep_hours} h does not fit in a day")
    awake_hours = HOURS_PER_DAY - sleep_hours
    segments: list[tuple[str, float, Environment]] = []
    if awake_hours > 0:
        segments.append(("awake", awake_hours, Environment(wbgt_c=wbgt_c)))
    if sleep_hours > 0:
        segments.append(
            ("asleep", sleep_hours, Environment(wbgt_c=wbgt_c, asleep=True, sleep_quality=quality))
        )
    return tuple(segments)


def advance_clock_command(
    *,
    run_dir: str | Path,
    character_id: str,
    days: int,
    sleep: str = "8h",
    quality: SleepQuality = SleepQuality.GOOD,
    wbgt_c: float = 21.0,
) -> dict[str, object]:
    """Advance one body by whole days, and report both sides of every channel.

    The body is read from the run's snapshot and the advance is appended to the
    run's own event log. It is **not** written back into the snapshot: rehydrating
    a run and persisting an advanced world is PSV1-8, and a command that half-did
    it would leave a snapshot whose bodies and whose `capacity.sampled` records
    came from different moments.
    """
    if days < 0:
        raise ValueError("the clock does not run backwards")
    run_dir = Path(run_dir)
    snapshot_path = run_dir / "snapshot.yaml"
    events_path = run_dir / "events.jsonl"
    if not snapshot_path.is_file():
        raise FileNotFoundError(f"no snapshot in {run_dir}: seed a cohort there first")

    snapshot = read_snapshot(snapshot_path)
    characters = snapshot.get("characters") or {}
    if character_id not in characters:
        known = ", ".join(sorted(characters)[:5])
        raise KeyError(
            f"{character_id} is not in this run; it holds {len(characters)} characters "
            f"(for example {known})"
        )
    section = characters[character_id]
    profile = capacity_profile_of(section)
    body = body_state_of(section)
    traits = BodyTraits.from_profile(profile)
    params = load_params()

    sleep_hours = parse_hours(sleep)
    segments = day_segments(sleep_hours, quality, wbgt_c) * days

    if segments and body_has_advanced(events_path, character_id):
        raise BodyAlreadyAdvancedError(
            f"{character_id} has already been advanced in this run; resume/save is owned by "
            "PSV1-8, so advancing the immutable seed snapshot again would fork its history"
        )

    before = body
    capability_before = capability_available(profile, body, traits=traits, params=params)
    advances: list[dict[str, object]] = []

    if segments:
        metadata = RunMetadata.from_document(snapshot.get("run_metadata") or {})
        with EventLog.extend(
            events_path,
            metadata=metadata,
            required_components=ADVANCE_COMPONENTS,
            sim_time=str(snapshot.get("sim_time", Y1_START)),
        ) as log:
            for label, hours, environment in segments:
                exposure = Exposure.of(environment=environment, traits=traits, params=params)
                moved = advance(body, hours, exposure)
                record = {
                    "character_id": character_id,
                    "dt_hours": hours,
                    "segment": label,
                    "t_hours_before": body.t_hours,
                    "t_hours_after": moved.t_hours,
                    "environment": as_document(environment),
                    "load": as_document(exposure.load),
                    "dynamics_version": params.model_version,
                    "channels_changed": channel_diff(body, moved),
                }
                log.append(
                    EVENT_BODY_ADVANCED,
                    record,
                    sim_time=logical_sim_time(
                        str(snapshot.get("sim_time", Y1_START)), moved.t_hours
                    ),
                )
                advances.append(record)
                body = moved

    capability_after = capability_available(profile, body, traits=traits, params=params)
    return {
        "character_id": character_id,
        "days": days,
        "sleep_hours": sleep_hours,
        "quality": quality.value,
        "advances": advances,
        "hours_advanced": body.t_hours - before.t_hours,
        "before": before,
        "after": body,
        "capability_before": capability_before,
        "capability_after": capability_after,
        "dynamics_version": params.model_version,
        "events": events_path,
    }


def _flatten(document: object, prefix: str = "") -> dict[str, object]:
    if isinstance(document, Mapping):
        flat: dict[str, object] = {}
        for key, value in document.items():
            flat.update(_flatten(value, f"{prefix}.{key}" if prefix else str(key)))
        return flat
    return {prefix: document}


def print_advance(result: Mapping[str, object]) -> None:
    """Both sides of every channel that moved, then capability per dimension."""
    before: BodyState = result["before"]  # type: ignore[assignment]
    after: BodyState = result["after"]  # type: ignore[assignment]
    print(
        f"{result['character_id']}  {result['hours_advanced']:.1f} h over {result['days']} day(s), "
        f"{result['sleep_hours']:.1f} h sleep ({result['quality']}) a night  "
        f"[dynamics {result['dynamics_version']}]"
    )
    print(f"{len(result['advances'])} body.advanced appended to {result['events']}")  # type: ignore[arg-type]

    changed = channel_diff(before, after)
    print("\nchannel                                        before          after")
    if not changed:
        print("  (nothing moved: no time passed, and a scene heals nothing)")
    for channel in sorted(changed):
        flat_before = _flatten(changed[channel]["before"])
        flat_after = _flatten(changed[channel]["after"])
        for key in sorted(set(flat_before) | set(flat_after)):
            old, new = flat_before.get(key), flat_after.get(key)
            if old == new:
                continue
            label = f"{channel}.{key}" if key else channel
            print(f"  {label:<42} {_cell(old)} {_cell(new)}")

    print("\ncapability_available                           start            end")
    capability_before: Mapping[Dimension, float] = result["capability_before"]  # type: ignore[assignment]
    capability_after: Mapping[Dimension, float] = result["capability_after"]  # type: ignore[assignment]
    for dimension in Dimension:
        start, end = capability_before[dimension], capability_after[dimension]
        change = "" if start == 0 else f"  {100.0 * (end - start) / start:+6.2f}%"
        print(f"  {dimension.value:<42} {start:>10.3f} {end:>14.3f}{change}")


def _cell(value: object) -> str:
    if isinstance(value, float):
        return f"{value:>14.4f}"
    return f"{str(value):>14}"


def _build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="embodiment",
        description="Embodiment subdomain of the simulation engine. No LLM is involved in anything here.",
    )
    subcommands = parser.add_subparsers(dest="command", required=True)

    seed = subcommands.add_parser(
        "seed-cohort",
        help="Seed N anonymous students from the population prior, frozen and reproducible.",
    )
    seed.add_argument("--world-seed", type=int, required=True)
    seed.add_argument("--n", type=int, required=True, dest="count")
    seed.add_argument("--out", required=True, dest="out_dir")
    seed.add_argument("--prior", default=str(POPULATION_PRIOR_PATH), dest="prior_path")
    seed.add_argument("--id-prefix", default="npc")
    seed.add_argument("--start-index", type=int, default=1)

    clock = subcommands.add_parser(
        "advance-clock",
        help="Advance one body by whole days and show what each channel did.",
    )
    clock.add_argument("--run", required=True, dest="run_dir")
    clock.add_argument("--character", required=True, dest="character_id")
    clock.add_argument("--days", type=int, required=True)
    clock.add_argument("--sleep", default="8h", help="Hours of sleep a night, e.g. 4h or 30m.")
    clock.add_argument(
        "--quality",
        default=SleepQuality.GOOD.value,
        choices=[quality.value for quality in SleepQuality],
    )
    clock.add_argument(
        "--wbgt",
        type=float,
        default=21.0,
        dest="wbgt_c",
        help="Ambient wet-bulb globe temperature in degrees C.",
    )
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    parser = _build_parser()
    args = parser.parse_args(argv)
    if args.command == "seed-cohort":
        result = seed_cohort_command(
            world_seed=args.world_seed,
            count=args.count,
            out_dir=args.out_dir,
            prior_path=args.prior_path,
            id_prefix=args.id_prefix,
            start_index=args.start_index,
        )
        print(f"seeded {result['seeded']} bodies from prior {result['prior_version']} ({result['prior_status']})")
        print(f"events            {result['events']}")
        print(f"snapshot          {result['snapshot']}")
        print(f"snapshot_hash     {result['snapshot_hash']}")
        print(f"normalised_log_sha256 {result['normalised_log_sha256']}")
        return 0
    if args.command == "advance-clock":
        print_advance(
            advance_clock_command(
                run_dir=args.run_dir,
                character_id=args.character_id,
                days=args.days,
                sleep=args.sleep,
                quality=SleepQuality(args.quality),
                wbgt_c=args.wbgt_c,
            )
        )
        return 0
    parser.error(f"unknown command {args.command!r}")  # pragma: no cover
    return 2  # pragma: no cover
