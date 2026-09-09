"""`python -m embodiment` — the spine of the tracer bullet, not an extra.

PSV1-1 adds one subcommand:

    python -m embodiment seed-cohort --world-seed 42 --n 40 --out runs/demo/

which seeds N anonymous students from the population prior, freezes one
`capacity_baseline` each, writes `capacity.sampled` to the event log and a
snapshot with complete `run_metadata`. Running it again with the same seed
produces the same log; running it with `--n 41` produces the same forty bodies
plus one.

Evidence of what happened is the **event log**, not this terminal output.
"""

from __future__ import annotations

import argparse
import itertools
from collections.abc import Mapping, Sequence
from pathlib import Path

import numpy as np

from .eventlog import (
    EVENT_COHORT_CORRELATION_REPORT,
    EventLog,
    normalised_digest,
)
from .prior import POPULATION_PRIOR_PATH, PopulationPrior
from .seeding import CapacityBaselineStore, Posterior, cohort_ids, posterior_for, seed_character
from .snapshot import build_snapshot, write_snapshot
from .types import Dimension, RunMetadata, Y1_START

__all__ = ["main", "seed_cohort_command", "correlation_report"]

#: The components this run engages, and therefore whose parameter versions
#: spec §9.2 makes mandatory. Seeding engages the prior and nothing else: there
#: is no estimator, no dynamics, no contest resolver and no observation in this
#: ticket, and declaring versions for machinery that did not run would be a
#: metadata that lies.
SEEDING_COMPONENTS: tuple[str, ...] = ("prior",)

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
    prior = PopulationPrior.load(prior_path)
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
            "component_versions": {"prior": prior.version},
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

    events_path = out_dir / "events.jsonl"
    snapshot_path = out_dir / "snapshot.yaml"
    store = CapacityBaselineStore()
    with EventLog(
        events_path, metadata=metadata, required_components=SEEDING_COMPONENTS, sim_time=Y1_START
    ) as log:
        for character_id in ids:
            seed_character(world_seed, character_id, prior, store=store, log=log)
        log.append(EVENT_COHORT_CORRELATION_REPORT, correlation_report(prior, store, posteriors))

    snapshot = build_snapshot(
        world_seed=world_seed, store=store, metadata=metadata, posteriors=posteriors
    )
    snapshot_digest = write_snapshot(snapshot_path, snapshot)

    return {
        "events": events_path,
        "snapshot": snapshot_path,
        "snapshot_hash": snapshot_digest,
        "normalised_log_sha256": normalised_digest(events_path),
        "seeded": len(store),
        "prior_version": prior.version,
        "prior_status": prior.status,
    }


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
    parser.error(f"unknown command {args.command!r}")  # pragma: no cover
    return 2  # pragma: no cover
