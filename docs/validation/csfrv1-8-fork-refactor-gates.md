# CSFRV1-8 / #56 — baseline e gates de regressão para o fork

**Baseline de engenharia:** `main@badc48495f74b83ff7ad4dfe58a7f9ce91e78b2e` (PR #62 já integrada, checks 5/5).  
**Propósito:** congelar uma referência **antes** da primeira mudança de fluxo em `forkSnapshotWithInstalledPolicies`.  
**Escopo desta fase 0:** mapa de provas existentes e contrato da futura captura de bytes; **não há novo golden materializado nesta entrega documental**. Captura de golden binário (ver abaixo) é um **gate obrigatório antes da PR P1**, não algo a marcar como PASS sem execução.  
**Plano de extração:** [fases do bootstrap](../architecture/csf-fork-bootstrap-phases-v1.md).

## Material já versionado

- Policy e vetores do [bundle V1](../architecture/canonical-codec-v1-bundle/BUNDLE.sha256): `codec_policy_hash=513111dc82a5e58c07aecdabf410633f5aa5418908d2461ef0dff0d9ae5d8203`.
- Policy e vetores do [bundle V2](../architecture/canonical-codec-v2-bundle/BUNDLE.sha256): `codec_policy_hash=528168c4032e53d4c8b50a9a890b0b857dfc85b3722211c39b523b8de645da89`.
- Os **621 casos cross-language** Go/TS são do **bundle V1**, não uma certificação independente do V2. As migrações V2 dispõem de testes Go.
- [Workflow CSFRV1-8](../../.github/workflows/csfrv1-8-continuity.yml) contém Go completo, SQLite, `TestCSFRV18` e `-race -timeout=30m`; [#59](https://github.com/carloshgalves/cote-ai-simulation/issues/59) acompanha seu custo.
- Comprovação da baseline: [PR #62](https://github.com/carloshgalves/cote-ai-simulation/pull/62) no HEAD `17a9fb8aa513a647f1f9fa778728416d77da830f`, com **cinco workflows SUCCESS**; a evidência da branch não é execução nova da `main`.

## Matriz mínima por estágio

| Observável / autoridade | Cenário de referência já existente | Condição após cada refactor |
| --- | --- | --- |
| Genesis V1 parental, snapshot ID/canonical, owner signer roster | `TestCSFRV18SnapshotReopenClockAdvanceAndCanonicalBytes`, `TestCSFRV18R22SignedOwnerReceiptDurableBeforeSnapshot` | SHA-256 de bytes canônicos fixado; mesmo ID e policy |
| Genesis V2, lineage e parent CSFHREF | `TestCSFRV18R21V2ForkPreservesEvolvedTriggerSeed`, `TestCSFRV18TwoForksNeverAliasChildRunIDs` | Bytes de Genesis/CSFHREF fixos, parent não rehashado |
| Cinco ledgers (input/decision/event/evidence/outbox) e respectivos prefixos | `TestCSFRV18IntegratedResourceTriggerCommunicationInTransit`, `TestCSFRV18IntegratedFiveLedgerTamperFailsClosed` | Prefixos e registros literais; corruptos rejeitados |
| Migração de rounds, slots, dispatchs e respostas | `TestCSFRV18R19V2RoundHistorySurvivesReopenAndPreventsDuplicateResponse`, `TestCSFRV18R19V2ActionResponseIsRebuiltWithoutChangingIntent` | IDs/ordinais/bytes fixos; revocations preservadas |
| Migradores de ocorrências, claims e transmissões `IN_TRANSIT` | `TestCSFRV18R20EventClaimAndRoundShareTransactionalBootstrap`, `TestCSFRV18CommunicationSnapshotInTransitReplayAndDeliveryOnce` | No double-send; reminting somente do child |
| Evolução de trigger definition/runtime e import manifests | `TestCSFRV18R21ComposedEvolvedTriggerAndPendingRoundForks`, `TestCSFRV18R21V2TriggerImportRollbackAndRetry` | Sem rearmar/repetir no nascimento e sem modificar cadence |
| Corrupção de cada ledger, bad policy, source/root, rollback | `TestCSFRV18IntegratedFiveLedgerTamperFailsClosed`, `TestCSFRV18ForkRollbackAndArchiveTamperReject`, `TestCSFRV18R19V2RoundHistoryRollback` | Mesmo fail-closed, zero Genesis parcial |
| Recovery pós-commit, retry sem WORK alheio, siblings | `TestCSFRV18R2602PublishedForkRetryIgnoresLaterParentWorkAndRevision`, `TestCSFRV18R22FencedHandoffRetryAfterCheckpointProviderFailure` | Child existente validado read-only; sem dispatch novo |
| Forte stress de cascata com trigger e round futuro | `TestCSFRV18R21V2CascadeCapWithTriggerAndPendingRound` | Cap de 64 WORK, 65º abort determinístico; `-race` completo |

## Captura de golden bytes — checklist de bloqueio antes da P1

1. Em um **commit anterior à primeira extração**, executar fixtures determinísticas V1→V1 e V1→V2 (cobrir source finalizado, R19–R23 e signer quando aplicável). Registrar `SHA` exato, `go version`, SQLite source ID, versão de policy e um seed fixo. **Não** registrar valores que dependam de path temporário, horário ou ordem não causal.
2. Ler em ordem determinística, sem normalizar bytes, de cada SQLite: `genesis_records.canonical`; as **cinco** tabelas de ledgers (ordem pelo cursor/snapshot, não `rowid`); `snapshots.canonical`; `fork_lineage.reference_canonical`; `fork_history_records` com `ledger_name,ordinal`; `fork_history_roots`; `input_slot_responses`; `communication_claims`; `communication_transmissions`; `fork_trigger_imports`, `trigger_definitions`, `trigger_runtime`, `trigger_activations`; `cognitive_owner_receipts` e `fork_cognitive_owner_proofs` quando presentes.
3. Escrever fixture **binária ou hex + SHA-256** para cada record e ordem, com um manifest de contagens. Zero registros também deve ser declarado para evitar falso PASS por tabela vazia. Não serializar `[]any` de volta para produzir o esperado; comparar **bytes armazenados**. Evitar persistir chaves privadas, originais protegidos e locator com segredo.
4. Fazer um teste de guarda que prove **RED** contra um único byte alterado e **GREEN** quando restaurado; validar também delete/duplicate/extra e corrupção de policy.
5. Medir, na mesma máquina e ferramenta, duração total, alocações e tamanho do arquivo em >=3 amostras sem race, e duração com `-race` separada; relatórios com SHA, CPU, `GOMAXPROCS` e flags. Comparar baseline e cada PR de fase, justificando regressão material com evidência. Não otimizar sem profiling (#60).
6. Rodar `go test -count=1 ./internal/csf/persistence/sqlite ./internal/csf/commit`, `go test -count=1 ./...`, `go test -race -count=1 -timeout=30m ./internal/csf/commit -run '^TestCSFRV18'`, Go/TS V1 conformance e `git diff --check`. P0 regressões = **NO-GO**. Caso a CI não execute por runner, classificar `CI_NOT_RUN`, nunca PASS.

**Status da captura completa de goldens do fork:** **PENDENTE / NÃO EXECUTADA nesta PR de desenho.** As suites existentes validam propriedades de bytes e corrupção, mas não substituem automaticamente um arquivo fixado de golden por fixture para a #56. Nenhuma PR de extração P1–P4 deverá ser aprovada sem a captura e comparação aplicáveis.

## Critério de passagem entre fases

- **P0:** interfaces e limitações revisadas, sem modificação executável; baseline de CI anterior preservada. Não encerra #56.
- **P1:** golden materializado *antes* do primeiro refactor; fase cut implementada com negativos; `-race`, regressão e fingerprints idênticos.
- **P2:** plano de migração provado livre de mutação SQLite depois da coleta autenticada; ouro por ocorrência, round, claim, slot e trigger permanece.
- **P3:** inserções transacionais sob um único guard, testes de falha em cada `INSERT`, FK-off, rollback, reopen e retry do mesmo child.
- **P4:** `validateForkLineageTx` permanece pré-commit; replay/recovery pós-commit, sem corrigir corrupções por escrita ou executar WORK não relacionado.

A análise do diff deve ser limitada à fase e às dependências imediatas; nenhum PASS de P0 é prova de P1–P4. Bloqueios novos e não relacionados devem virar issues independentes.
