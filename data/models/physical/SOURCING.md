# Physical models — outstanding sourcing tasks

Four gaps stand between the current provisional parameters and a `POPULATION_PRIORS` that can be
called calibrated. None of them requires reading the light novel: they are real-world measurement
sourcing, tracked here rather than in `data/canon/open_questions/` because they are not questions
about the work.

Canon questions that block per-character posteriors stay where they belong, in
[`data/canon/open_questions/`](../../canon/open_questions/) — `oq.capability.*`.

Research behind these: [`docs/research/physical-domain-v1.md`](../../../docs/research/physical-domain-v1.md).

## S1 — Cohort norms: mean and SD by age and sex · **blocks POPULATION_PRIORS**

The item score table is transcribed ([`fitness-test-score-table.yaml`](fitness-test-score-table.yaml))
but it is a *criterion* table, age-invariant within 12-19. It gives units and band boundaries; it does
not give the distribution. The distribution lives in the annual 体力・運動能力調査.

Located on e-Stat under `toukei=00402102`, `tstat=000001088875`. For FY 令和5:

| Table | `statInfId` |
|---|---|
| 年齢別テストの結果 (test results by age) | `000040216148` |
| 年齢別体格測定の結果 (anthropometry by age) | `000040216155` |
| 学校段階別テストの結果 (results by school stage — isolates senior high) | `000040216156` |

**Known blocker, verified 2026-09-08:** `/stat-search/file-download?statInfId=…&fileKind=0` returns
**404** to automated requests, including with a session cookie and correct `Referer`. A browser or a
human step is required. Do not spend more agent time on it; it is not a discovery problem.

Prefer 学校段階別 for the senior-high band and 年齢別 for age resolution; take both, they disagree in
useful ways (school stage conditions on enrolment, age does not).

## S2 — Inter-item correlation or published factor loadings · **blocks a coherent multivariate prior**

Marginals alone let the sampler draw a student who is simultaneously the heaviest, the fastest and
the best at the shuttle run. The prior needs a correlation structure — either a published correlation
matrix among the nine items for this age band, or factor loadings from a factor analysis of the
battery.

**Not located** in the 2026-09-08 search. Try J-STAGE and CiNii with Japanese terms
(新体力テスト · 因子分析 · 項目間相関 · 高校生) before falling back.

Until then the two-factor loadings in the prior are a declared modelling assumption, and the prior
file must say so in its own `evidence_sufficiency` — not only in the per-character one.

## S3 — 持久走 distances per sex

The transcribed sheet gives times but not distances. 1500 m male / 1000 m female is inferred from the
times and is consistent with the standard battery, but it is not verified. Confirm in the Sports
Agency's own 新体力テスト実施要項 (12–19歳対象):
<https://www.mext.go.jp/sports/b_menu/sports/mcatetop03/list/detail/1408001.htm>

Until confirmed, any de-normalisation of an endurance-run feat into m·s⁻¹ is unsound. The
shuttle-run column is unaffected and should be preferred meanwhile.

## S4 — Verify the score table against the official publication

[`fitness-test-score-table.yaml`](fitness-test-score-table.yaml) was transcribed from a textbook
publisher's redistribution of the 平成11年度 実施要項. The sheet identifies itself as that document,
carrying its 1999-04-19 correction, and the values are internally consistent — but the transcription
has not been checked against the Sports Agency's own PDF. Flip
`source.verification_status` when it has been.

## Not a sourcing gap: cohort selectivity

Whether the school's intake shifts the cohort away from the national distribution is a question about
the work, not about the instrument. It is tracked as
[`oq.capability.cohort-selectivity`](../../canon/open_questions/capability--cohort-selectivity.yaml).
