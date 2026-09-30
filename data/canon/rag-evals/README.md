# RAG evals V0

Golden cases live in `golden/`. `v0-config.json` stores the versioned reference backend and
the caller-defined `CANON_SEED` instant. `v0-fixtures.json` contains synthetic evidence,
claims and source-registry entries only; none of it is canon.

Each eval report records the corpus manifest, suite/fixture digest, backend identity,
normalized request, returned evidence IDs and exclusion reason codes. Filter lists are
allow-lists when non-empty; an absent or empty list means no restriction.

`hit_at_k` is `1` when a case returns at least one expected evidence ID. For a case
whose required outcome is abstention and whose expected set is empty, abstaining is a
hit. The aggregate is the mean across cases; `recall_at_k` remains micro-averaged over
the expected evidence IDs.
