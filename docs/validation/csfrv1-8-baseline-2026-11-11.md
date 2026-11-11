# CSFRV1-8 — reconciliação de aceite após PR #25

**Apuração:** 2026-11-11 · [Issue #54](https://github.com/carloshgalves/cote-ai-simulation/issues/54)
**Origem:** [ticket 08](../tickets/csf-runtime-v1/08-replay-snapshot-fork.md), [spec CSF V1](../spec/causal-simulation-foundation-v1.md), [ADR 0008](../adr/0008-causal-simulation-foundation-v1.md)
**Implementação testada:** `96daa15f35ad6840033ceb10ecde37e76f98377f` (HEAD da [PR #25](https://github.com/carloshgalves/cote-ai-simulation/pull/25))
**Baseline integrada:** `main@0e3f9665b9bbc33be93d9873751fc5a68cec450c` (merge em 2026-11-11)
**Classificação:** **MERGED AS ENGINEERING BASELINE**, não **INDEPENDENTLY REVIEWED**, não **PRODUCTION READY**, não **RELEASE APPROVED**.

Este registro é a **fonte de status corrente** do CSFRV1-8. O [diário adversarial](../../evals/simulation/csf-runtime-v1/csfrv18-adversarial-review.md) conserva historicamente checkpoints R1–R26 com referências a `OPEN-01/02/04/05`, `Draft`, `NO-GO` e R21–R23 ainda pendentes. Esses enunciados refletem o momento de cada revisão; não devem ser usados como situação atual sem esta reconciliação. Os findings originais permanecem disponíveis.

## Critérios e evidências (ticket 08 / C01–C10)

`COBERTO` significa que existem testes nomeados incluídos em suites com sucesso no HEAD da PR, **não** cobertura universal; `LIMITADO` indica fronteiras de escopo/integração; `PENDENTE` indica prova ou aprovação ainda não obtida.

| Critério | Implementação / testes nomeados em `main` | Situação e restrição |
| --- | --- | --- |
| **C01** Replay causal e identidade de world/clock/ledgers | `internal/csf/persistence/sqlite/clock_replay.go`; `TestCSFRV18SnapshotReopenClockAdvanceAndCanonicalBytes`, `TestCSFRV18ResourceContestTriggerScheduleContinuityCrashMatrix` em `internal/csf/commit/csfrv18_snapshot_test.go` | **COBERTO** nos cenários versionados; não atesta todas as execuções possíveis. |
| **C02** Snapshot V1, pending state, tails, fence/retry, percepção e resume | `snapshot_v1.go`, `snapshot_resume.go`, `resume_runner.go`; `TestCSFRV18SnapshotRealInFlightHaltedRetryRecovery`, `TestCSFRV18ResumeDrainsCausallyCommittedPerceptionExactlyOnce`, `TestCSFRV18PartialPerceptionBeforeCrashResumesWithoutDuplicateReceipt` | **COBERTO**, inclusive negativos e deduplicação local; integração cognitiva externa continua separada. |
| **C03** Permutações de workers/triggers, determinismo e crash points | `TestCSFRV18SnapshotDeterministicAcrossWorkerAndTriggerOrder`, `TestCSFRV18ResourceContestTriggerScheduleContinuityCrashMatrix` | **COBERTO** nas permutações testadas; stress/E2E completo pertence ao CSFRV1-10. |
| **C04** Same-policy fork, V1→V2, prefixo parental e tails independentes | `fork_bootstrap.go`, `fork_lineage.go`; `TestCSFRV18ForkIdleSamePolicyEndToEnd`, `TestCSFRV18TwoForksNeverAliasChildRunIDs`, `TestCSFRV18V2ForkSiblingInputTailsDivergeWithoutCrossRunAuthority` | **COBERTO/LIMITADO:** cross-policy exige **exact live cut**, não aceita fork histórico arbitrário após avanço do pai. |
| **C05** R19–R21: round/dispatch/revocation/response, ocorrências Event-owned e trigger evoluído | `fork_round_history_v2.go`, `fork_occurrences_v2.go`, `fork_trigger_seeds_v2.go`; `TestCSFRV18R19V2RoundHistorySurvivesReopenAndPreventsDuplicateResponse`, `TestCSFRV18R20EventClaimAndRoundShareTransactionalBootstrap`, `TestCSFRV18R21ComposedEvolvedTriggerAndPendingRoundForks`, `TestCSFRV18R21V2CascadeCapWithTriggerAndPendingRound` | **COBERTO** para autoridades expressamente migradas. Políticas/estados não suportados falham fechado. |
| **C06** R22: memória opaca, owner, Ed25519, escrow, cobertura e replay histórico | `epistemic_owner_receipt_v1.go`, `snapshot_owner_trust.go`; `TestCSFRV18R22SignedOwnerReceiptDurableBeforeSnapshot`, `TestCSFRV18R22V2ForkPreservesPinnedOwnerProofAndRequiresChildReceipt`, `TestCSFRV18R2501EvidenceHolderSignedLaterKeepsHistoricalLegacySnapshot` | **COBERTO localmente; PENDENTE para produção:** assinatura não prova `fsync` externo nem restore/ACK do serviço real ([#58](https://github.com/carloshgalves/cote-ai-simulation/issues/58)). |
| **C07** Handoff V1 em WORK/retry, crash depois do commit parental/filha e recuperação idempotente | `fork_after_recovery_v2.go`, `fenced_handoff_recovery.go`, `fork_recovery_v2.go`; `TestCSFRV18R22FencedHandoffRetryAfterCheckpointProviderFailure`, `TestCSFRV18R22HandoffSnapshotWriterLockRequiresFrozenRevisionAndIdle`, `TestCSFRV18R2602PublishedForkRetryIgnoresLaterParentWorkAndRevision` | **COBERTO localmente; LIMITADO:** V1 terminal e V2 bootstrap usam stores separados, sem transação distribuída; owner remoto ainda sem contrato operacional demonstrado. |
| **C08** Corrupção, rollback, ausência de artefato/policy e fail-closed | `TestCSFRV18IntegratedFiveLedgerTamperFailsClosed` em `csfrv18_integrated_ledger_tamper_test.go`, `TestCSFRV18ForkRollbackAndArchiveTamperReject`, `TestCSFRV18V2RejectsTamperedTransitionAndContractVectors` | **COBERTO** nas classes de falha exercitadas, sem alegação de ausência universal de defeitos. |
| **C09** R23: recurso + trigger + comunicação `IN_TRANSIT`, cinco ledgers e fork divergente | `TestCSFRV18IntegratedResourceTriggerCommunicationInTransit`, `TestCSFRV18CommunicationSnapshotInTransitReplayAndDeliveryOnce`, `TestCSFRV18ResourceContestTriggerScheduleContinuityCrashMatrix` | **COBERTO** no cenário integrado; matriz total de 41 invariantes, física e primeiro run pertencem ao CSFRV1-10. |
| **C10** Re-review formal independente e gate de release no SHA integrado | [Discussão da PR #25](https://github.com/carloshgalves/cote-ai-simulation/pull/25), auditoria histórica, [CI de continuidade](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/42920070309) | **PENDENTE**: nenhum review formal de terceiro registrado; testes da feature branch não são validação independente do merge `0e3f966` nem aprovação de produção. |

### Execuções do GitHub Actions

Seis workflows `SUCCESS` no SHA exato **`96daa15`** da PR #25:

| Workflow | Run |
| --- | --- |
| CSFRV1-8 Continuity Checkpoint (focados, `-race`, SQLite e full Go) | [42920070309](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/42920070309) |
| CSF canonical conformance (Go/TS **621 casos do bundle V1**) | [42920070242](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/42920070242) |
| CSFRV1-4 Decision Kernel | [42920070483](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/42920070483) |
| CSFRV1-5 Atomic Terminal Commit | [42920070273](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/42920070273) |
| CSFRV1-6 Epistemic Evidence | [42920070353](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/42920070353) |
| CSFRV1-7 Communication | [42920070445](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/42920070445) |

**Precisão:** o runner cross-language com 621 vetores verifica **V1**, não 621 casos independentes do V2. O V2 tem testes Go de policy e migração; cobertura independente e gatilhos de CI do V2 estão em [#59](https://github.com/carloshgalves/cote-ai-simulation/issues/59). Esta PR documental não executa nem atribui a si testes de produção.

## Findings da #25 — disposição final registrada

| Grupo | Situação registrada / prova |
| --- | --- |
| **R19-1/2/3/4** | `RESOLVED` na conversa da [PR #25](https://github.com/carloshgalves/cote-ai-simulation/pull/25); histórico de slot, preimages e autorização futura. |
| **CI-1** | `RESOLVED`, [registro](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6372872567). |
| **R22-HANDOFF-01/02/03** | `RESOLVED` para correções locais: [01](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6770699097), [02](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6770699994), [03](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6770702593). A prova **remota** de durabilidade continua em #58. |
| **R22-OWNER-DOWNGRADE-04 / FORK-GUARD-05** | `RESOLVED`: [04](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6772424899), [05](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6772729492). |
| **R24-01/02** | `RESOLVED`: [01](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6774256269), [02](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6774266618). |
| **R25-01** | `RESOLVED`, [registro](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6778666172). |
| **R26-01** | `DISMISSED`: [finding de fork histórico não atingível pelo gate exact-cut](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6780707202). |
| **R26-02** | `RESOLVED`, [retry não executa WORK alheio](https://github.com/carloshgalves/cote-ai-simulation/pull/25#issuecomment-6780741845). |

Nenhum finding individual foi deixado formalmente marcado OPEN no fim da PR; isso **não é** atestado de revisão exaustiva nem de ausência de novos defeitos. Não reabrir a PR já mesclada; issues menores fazem o rastreio residual.

## Limites contratuais que não podem ser omitidos

1. **Exact-cut:** `VerifySnapshotV1(R)` poder validar um corte histórico não autoriza `ForkIdleSnapshotV1ToV2` depois de o pai avançar. O gate exige correspondência do clock, controle e cinco prefixos ao checkpoint selecionado.
2. **Handoff em WORK:** `SettleFencedThenForkV2` assenta o ciclo V1 previamente autorizado, drena percepção e cria novo checkpoint IDLE; não move fence in-flight diretamente para a filha. Não existe transação distribuída entre V1 e V2.
3. **Owner de memória:** Ed25519 garante autenticidade de recibo e bytes verificados no escopo declarado, não comprova flush remoto/backup. Reidratação e ACK são exigências do serviço integrador (#58). Checkpoints de bancos anteriores ao selo `snapshot_owner_key_seals` exigem migração/exportação aprovada; não fabricar selos de confiança.
4. **Integridade epistemológica:** memória opaca não é regenerada por LLM, não vira `WORLD_TRUTH` nem evidence de outro ator. Replay não usa wall clock como autoridade causal.
5. **Fora de escopo:** merge de forks/timelines, fork histórico cross-policy arbitrário e conversão automática de snapshots físicos legados. Origem/caso não suportado falha fechado.

## Backlog residual e fronteira com os próximos contratos

| Issue | Entrega apartada do merge |
| --- | --- |
| [#54](https://github.com/carloshgalves/cote-ai-simulation/issues/54) | Reconciliação documental presente; fechar após merge da PR própria. |
| [#55](https://github.com/carloshgalves/cote-ai-simulation/issues/55) | Guard de completude da lista de autoridades SQLite. |
| [#56](https://github.com/carloshgalves/cote-ai-simulation/issues/56) | Decomposição do bootstrap V1→V2 por fases. |
| [#57](https://github.com/carloshgalves/cote-ai-simulation/issues/57) | Decoders tipados de snapshot e round history. |
| [#58](https://github.com/carloshgalves/cote-ai-simulation/issues/58) | **P0 de release com memória externa:** durabilidade real, restore e ACK do owner. |
| [#59](https://github.com/carloshgalves/cote-ai-simulation/issues/59) | CI V2 independente, cobertura de paths, timeout/formatação. |
| [#60](https://github.com/carloshgalves/cote-ai-simulation/issues/60) | Benchmarks de replay/forks antes de otimizar. |

[CSFRV1-9](../tickets/csf-runtime-v1/09-observatory-e-operacao.md) não foi iniciado; [CSFRV1-9A](../tickets/csf-runtime-v1/09a-contrato-adapter-fisico.md) é independente no grafo e pode aguardar por decisão do operador. [CSFRV1-10](../tickets/csf-runtime-v1/10-adapter-fisico-e-cenario.md) requer os dois e ainda inclui evals, 41 invariantes e cenário real não comprovados.

**Veredito:** a engenharia de R21–R23 está integrada e possui evidência de CI na branch; **CSFRV1-8 não está fechado como release**. A próxima decisão é concluir/homologar seus gates residuais em PRs separadas, sem transformar o merge da #25 em uma aprovação retroativa de produção.
