# CSFRV1-10/G — executable evidence registry (G1 #160)

Parent #121 / epic #114, A–F technical merges verified. F5 #159 merged
at `902ff115b5008b6d33dec314eeb4a9a5c805412e` (22/22 checks on
`e68453765d28389fd3a170b3f80fd4ca5794e693`, acceptance artifact
14496055047). The frozen **10-A plan is not edited**.

## Scope and semantics

`registry.json` contains exactly 41 invariant IDs and 68 ADR stress IDs,
with one current owner/eval and one coverage disposition per row. Both the
ADR and original traceability bytes are pinned by Git blob identity. The
registry contains no verified PASS values. READY means reviewed candidate
witness(es) are registered; actual results are supplied by the runner.
NOT_RUN means the complete proof has not been mapped; BLOCKED requires an
issue and explicit explanation. In G1 seven rows are blocked by #22
(DEDUPLICATED / zero-event empty WORK), 100 are not yet mapped and two
are executable pilot rows. These counts describe the registration, not a
passing run. G2–G5 must inspect and extend the reviewed witnesses, not
blindly promote partial coverage. G6 aggregates four evals, conformance
and reports gaps. No changing the ADR to fit tests.

Pilot witnesses:
- STRESS-41 (simulation): actual last-unit conflict/replay and stable
  decision/stock, reversed arrival.
- Determinism pilot (no row credit): real E1 same signed A1 cuts, four
  equivalent runs and byte equality of all authoritative rows. STRESS-61
  stays NOT_RUN: the current decision evaluator changes traversal order
  in a serial loop, not true parallelism. G3 must produce an actual
  different-parallelism witness or document the gap.
- INV-16 (knowledge boundary): true queryhost dependency graph, nominal
  restricted capabilities, independent pager defense and concurrent
  real D3 queries with E1/durable-byte equality. F4 trusted operator and
  SQLite composition-root limitations remain.

G1 contains four explicit pilot eval packages, including the genuine D3
physical witness without direct row credit (partial physical evidence must
not promote STRESS-39). Package-level supplemental witnesses can be tracked
and fail an eval even if not sufficient for a normative requirement. All
four group scopes remain PILOT_ONLY; success is PASS_PILOT_ONLY and complete
is false. A later reviewed G6 gate must extend the scope contract when each
whole eval has been independently mapped and executed. G1 is infrastructure
plus initial executable rows, not completion of any whole eval or of 41/41. No LLM or network selection of causal facts.
Separate #93/#94/#113 review gates remain NOT_ATTESTED.

## Execute

From an exact **clean committed Linux checkout**, with Go 1.27.2 and
the prepared Python A1 environment (same fixture and pinned dependencies
as F5; see workflow), run:

```sh
python -B docs/reference-run/csfrv1-10g/test_run_evals.py
python -B docs/reference-run/csfrv1-10g/run_evals.py \
  --sha "$(git rev-parse HEAD)" --out /tmp/csf-g-new-evidence
```

The output directory must be fresh and outside the checkout; logs are
private local artifacts. For each selected witness the runner executes
the exact package/test normal and race, uncached; it requires every
declared test/subtest start/pass and package start/pass, and rejects
skips, extras, cache, malformed JSON, failed or missing execution.
The report includes actual command, exit status, source blob, names,
sanitized start/run/pass records with their own digest, SHA-256 of raw logs, normative wording and link, owner, status per row and
aggregate counts. A failing witness gates the linked rows; no passing
witness can silently replace a failing one. The CI only uploads
`evidence.json`, not raw logs, observation bytes or owner data.

Exit 0 means registered tests have passed and the matrix was generated,
**not** that the matrix is complete. Missing/blocked rows are explicitly
NO_GO_INCOMPLETE_MATRIX. Add `--require-complete` to receive exit 2
for an incomplete matrix or incomplete eval package; 1 for a test failure, including a supplemental pilot.  Even all passing rows with PILOT_ONLY scopes are NO_GO_INCOMPLETE_EVAL_SCOPE.
Only a future fully evidenced and complete eval scope may produce
EVIDENCE_COMPLETE_REVIEW_REQUIRED, never DoD accepted or production GO.
The registry is reviewed source code, not a signed external attestation;
the checkout, installed toolchain and CI runner remain trusted inputs.
Ignored Go sources/module controls are refused; tracked witness sources must be direct regular files matching their HEAD blobs. Sanitized event records permit audit of actual test inventories without exposing test output. This is not a hostile-code sandbox.
