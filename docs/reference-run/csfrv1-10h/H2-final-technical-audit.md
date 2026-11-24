# H2 consolidated technical audit — 2026-11-24

Audit baseline: human-merged `474bbe5353ce656054f29f063438f03aa6c801c1`, identical Git tree `9c7a1445e7831cfd3963ab69e36f2584f4903b50` to H2-E exact tested head `803643649b16ff89dcd9deb9fbe505c2349e4fa8`. [Private-local exact-head job](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/45510746062/job/138857643148) succeeded; 2/2 checks at that head. This is a scoped author-side technical audit, not the independent human review required by #123.

| #177 / #122 evidence criterion | Observed at baseline / remaining work |
| --- | --- |
| Five canonical ledgers; snapshot prefix/tail and source/trigger/cursor state | H2-L #178 exports exact five streams; full SQLite authority preserves historical rows; H2-I compares every table with multiplicity. No independent portable full historical dataset beyond these private databases is claimed. |
| Signed owner receipts, barriers and snapshot seals | #178 detached owner audit; #181 same-input DB comparison; #184 same-archive worker restart. Synthetic owner keys are not production trust roots. |
| Signed A1 originals and ingress provenance | #179/#180 exact signed pair, registration and cold verification; no Python rerun on verification. |
| Repeatability | #181 second independent Go execution of the same two signed A1 cuts, all causal and owner table transcripts equal. Distinct fresh physical wall-time captures are not equivalent. |
| Fault controls | #182/#183/#184, 14/14 physical rollback/restart normal and race, strict WAL/SHM checks and pending delivery recovery; no full driver kill or power-loss proof. |
| Runbook and operational evidence | #185 direct exit states 6/6 normal/race and negative exits; #186 direct out/verify receipts, rehashed semantic mutants and actual exit 17. Receipt is separate and requires an external local hash pin; not a signed external attestation. |
| Standalone causal report with refs/digests/decisions/revisions | **Finding H2-AUD-01**: prior report.json only held D3 disclosure summary; full history could be queried from the DB, but no separately materialized reference-only report existed. This PR adds it and checks both independently executed DBs. Pending exact-head CI. |
| Final review and approval | Open. Independent human reviewer required by #123; outstanding #93/#94/#113 and normative G gates are not cleared by green H jobs. |

The baseline CI markers include `H2_E_REAL_COMMANDS=OBSERVED_OUT_AND_VERIFY_EXIT_0`, `H2_E_COLD_RECEIPT=VERIFIED_PRIVATE_NO_WRITE`, `H2_E_REAL_MUTANTS=REHASHED_RECEIPTS_REJECTED_AND_FAILED_EXIT_RECORDED`, `H2_O_REAL_CUTS=VERIFIED_EXIT_CODES_NO_WRITE_NO_GO`, `H2_C_SAME_CLI_ARCHIVE_RECOVERY=14_OF_14_NORMAL_AND_RACE` and `H2_C_SAME_CLI_ARCHIVE_PENDING_BARRIER=WORKER_RESTART_EXACTLY_ONCE_NORMAL_AND_RACE`. No private evidence is uploaded.

## Residual disposition

The first G follow-up should be #169 STRESS-64: H2-C now supplies strong evidence for interruption **before** recipient perception with independent worker processes, stable identities and barrier, but the G registry still needs its own exact-head witness and the parent driver is paused, not killed. Do not promote the row or count a skipped test as PASS. STRESS-65 competing workers, STRESS-50 in-transit same-DB recovery, and STRESS-66 logical time/latency remain distinct. #168, #172, #174 and #22 remain open; 51 READY / 50 NOT_RUN / 8 BLOCKED in G6 at baseline. No registry state changed here.

Decision: local reference execution verified at baseline; additional H2-AUD-01 fix awaiting tests; **NO_GO_INCOMPLETE_MATRIX**; **NO production approval**. #177/#122 remain open pending the required review/evidence. #121/#114/#123 remain open.
