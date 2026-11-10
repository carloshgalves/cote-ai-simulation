# Product readiness and integrated validation — COTE AI Simulation

**As-of:** 2026-11-10. **Baseline:** `main@4f95c902b984418f186ef7d8e83cad36dbd2719c`; this document must be re-evaluated for any newer HEAD.  
**Status:** TEST PLAN / NO-GO for First Real Run. This file does **not** assert that a test was run.

## 1. Distinct release gates

| Gate | Proof required | Baseline assessment |
| --- | --- | --- |
| G0: historical policies | Go + TypeScript independent canonical 621-case conformance, exact bytes and manifest | Previously green (PR #53, main CI); re-run at release HEAD |
| G1: causal core | exact admissibility/fence, deterministic settle, clock, triggers, claims, evidence and SQLite failure injection | CSFRV1-1..7 merged; full integrated scenario pending |
| G2: continuity | PR #25 R21–R23, 5 ledger bytes, replay vs repeated resume vs fork branches, snapshots, corruption, crash points; independent review | **BLOCKED**: PR #25 Draft; not merged |
| G3: observability | privileged/actor/public views, no read-side mutation or knowledge leakage | **NOT IMPLEMENTED**: CSFRV1-9 |
| G4: physical integration | 9A versioned process/data protocol; 10 full Go↔Python causal adapter, calibrated test-only cohort, no physical telemetry in actor context | **NOT IMPLEMENTED** |
| G5: exam and economic rules | deterministic micro-secret and Zodiac/VIP exam, PP/CP consistent ledgers, explicit unknown-rule failures, 15/8/7 declared vectors executed rather than merely present | **NOT IMPLEMENTED** |
| G6: cognition/social | two or more agents with distinct beliefs, episodic memory, proactivity, goal revisions and action plans; recipient-scoped context | **NOT IMPLEMENTED** |
| G7: product end-to-end | initial Year-1 run, independent actions, school calendar, economic exchanges, full exam scoring, pause/resume, POV observatory, no scripted final outcome | **NOT IMPLEMENTED** |
| G8: source authority | per-record source review, operator sign-off for runtime admission, protected texts kept private; audits closed administratively do not certify records | **NOT APPROVED** |

Every gate requires: exact commit SHA; toolchain; command and exit status; test counts; archive digest; signed/reviewer-linked findings; unresolved issue IDs. A green G0/G1 CI result never means G7 passed.

## 2. On-demand automated baseline (existing components only)

Run via `.github/workflows/product-validation-on-demand.yml` with **manual** `workflow_dispatch`; it does not auto-consume Actions minutes. Inspect run results for exact SHA; do not substitute an earlier HEAD.

The job runs:

```bash
go test -count=1 ./...
go test -race -count=1 ./internal/csf/commit -run '^TestCSFRV18'  # only if the PR #25 code is in the tested ref
go run ./cmd/csf-harness --bundle docs/architecture/canonical-codec-v1-bundle --report /tmp/csf-go.json
node --no-warnings --experimental-loader ./tools/csf-conformance-ts/loader.mjs tools/csf-conformance-ts/runner.ts docs/architecture/canonical-codec-v1-bundle /tmp/csf-ts.json
cmp /tmp/csf-go.json /tmp/csf-ts.json
python -m pytest -q
python tools/validate_canon_consolidated_v2.py
python tools/rebuild_canon_indexes_v2.py --check
```

For the unmerged PR #25, run the existing branch-specific `CSFRV1-8 Continuity Checkpoint`, inspect its exact merge/PR HEAD, then run a separate human-independent review of the full diff. No release/merge from partial R21B success.

For canon content that uses private source PDFs, the public CI can check indexes, format and semantic invariants, **not** independent primary-text attribution; operators must use the private review packets, not upload protected text to Actions.

## 3. G2 / PR #25 acceptance matrix

| Trace | Input/permutation | Must be compared/rejected |
| --- | --- | --- |
| C01 | continuous versus causal replay | all five ledgers exact bytes, world/state hashes, clock, events and cursors |
| C02 | snapshot at pre-fence, after fence, after terminal, mid-perception | resumed bytes and one-shot terminality; no duplicate observation/claim |
| C03 | worker completion order reversed; source arrival permutations | identical canonical state and seed/substreams |
| C04 | source and child forks with different tails | parent archival prefix unchanged, separate ID spaces, no child knowledge from parent-only history |
| C05 | evolved rising/repeat/once triggers + pending rounds/communications | rekey and cadence preserved, no duplicate or lost activation |
| C06 | incomplete external cognitive checkpoint, missing owner, stale content, corruption | fail closed before publication; exact opaque bytes and coverage verified |
| C07 | crash before/after durable checkpoint reference, restart/retry | no irreversible unreferenced state, durable-before-ref protocol and recovery proof |
| C08 | corrupted each of five ledgers, Genesis, snapshot, manifests, policy, source root | fail closed, no partially published child or half-applied parent repair |
| C09 | divergent V1/V2 policy and cross-policy replay | original policy remains pinned to parent; child must verify installed V2 and replay without rehashing V1 |
| C10 | full-pr independent reviewer against ADR-0008/0009/0010/0011 and ticket 08 | zero new actionable OPEN findings, with review provenance |

Current evidence in PR #25 covers *parts* of C01–C09; **C06/C07 and full combined C01–C10 are not certified**. Its description explicitly retains R21 composition and R22/R23 blockers. Closing C10 requires a genuinely distinct reviewer.

## 4. First Real Run: acceptance scenario (future E2E)

1. Seed a short first-year window with cohort, institutions, rules, world state and actor-specific secret knowledge.
2. At least two focal agents and two lightweight NPCs independently form or revise plans; neither receives omniscient inputs.
3. The school announces an exam; agents can communicate, negotiate, wait, spend available PP and choose an action with admissible cost/time.
4. Resolve legal and competing actions and exam outcomes deterministically. Log any UNKNOWN rule rather than inventing a scoring method.
5. Simulate body fatigue/response across scene transitions; each actor observes only eligible qualitative results.
6. Save pre-exam and mid-exam snapshots, crash/restart twice, replay and fork into a counterfactual. Compare all authoritative ledgers and ensure differing, causally earned outcomes are allowed.
7. Inspect public, focal-agent and privileged Observatory projections: secrets remain redacted and queries cannot modify canonical hashes.
8. Validate evals for character fidelity, RAG relevance, knowledge boundaries, strategic contrast, resource balance, rollback and no future canon leakage.

**Fail conditions:** any secret leakage, deterministic divergence for identical inputs, invented exam authority, skipped causal proof, stale cognitive checkpoint, resource minting or duplicated commit. E2E cannot pass while G3–G6 are absent.

## 5. Dependencies, governance and operational constraints

- No merge of PR #25 until R21–R23 and final independent review. Its six workflow successes on `95e62db` are slice-level evidence only.
- External auditor issues #38 (Y1 V6), #39 (Y1 V7/V0) and #52 (Y1 V11) were closed **administratively (not planned)** at operator request; `verified:false`, `engine_memory_admission:false`, `goal_runtime_auto_update:false` and `exam_executable:false` remain as applicable. Reopen/create source-approval work before runtime promotion.
- Issues #15/#16 distinguish already merged legacy packs/spec from their separate runtime and verification gates.
- The repository is public; original novels/translations and private source windows must not be committed.
- `README.md`, `ROADMAP.md` and `docs/canon/README.md` are human dashboards; executable schemas, tickets and source-level approvals take precedence over optimistic text.
- Record Go version, Node version, Python version, SQLite policy hashes, seed, host/CPU memory for reproducible performance measurements.
- If the runner has no available toolchain, record `VALIDATION_BLOCKED`, not PASS/FAIL.

## 6. No ungrounded progress claims

| Status | Definition |
| --- | --- |
| IMPLEMENTED | code exists on specified commit |
| VALIDATED | cited tests executed on that commit with exit code 0 |
| REVIEWED | independent reviewer and findings provenance linked |
| APPROVED | human release gate explicitly granted |
| NOT RUN | tests or E2E not executed |
| BLOCKED | required implementation, evidence or reviewer absent |

**Product release verdict on 2026-11-10: NO-GO.** The on-demand workflow prepares reusable proof for existing modules; it cannot prove missing functionality.
