"""Embodiment — the physical subdomain of the Simulation Engine.

Not a peer context: a physical module beside the engine would create a second
place able to decide what happened (`physical-model.md` invariant 13). Nothing
outside this package imports it; what crosses the boundary is data — the
append-only event log, the snapshot, and the versioned parameter files in
`data/models/physical/` (ADR 0007 §3).

No LLM client is imported here, and no global RNG is used. Both are enforced by
`tests/embodiment/test_architecture.py`, which is as much the guard of that
boundary as it is of determinism.
"""

__all__ = ["__version__"]

__version__ = "0.1.0"
