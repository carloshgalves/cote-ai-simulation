# CSFRV1-8 / #56 — arquitetura de fases do bootstrap de fork

- **Proposta de decomposição, fase 0 (sem mudança de comportamento):** 2026-11-11
- **Código-base observado:** `main@badc48495f74b83ff7ad4dfe58a7f9ce91e78b2e` ([merge #62](https://github.com/carloshgalves/cote-ai-simulation/pull/62)).
- **Orquestrador existente:** `internal/csf/persistence/sqlite/fork_bootstrap.go::forkSnapshotWithInstalledPolicies`.
- **Contrato:** [ticket 08](../tickets/csf-runtime-v1/08-replay-snapshot-fork.md), [ADR 0008](../adr/0008-causal-simulation-foundation-v1.md), [ADR 0009](../adr/0009-canonical-causal-codec-and-digests.md), [ADR 0011](../adr/0011-csf-causal-persistence-atomicity.md).
- **Gates verificáveis:** [protocolo de regressão](../validation/csfrv1-8-fork-refactor-gates.md).
- **Issue:** [#56](https://github.com/carloshgalves/cote-ai-simulation/issues/56).

Este documento **desenha** os tipos internos e as fronteiras de extração; **não** altera implementações, schemas SQL, bytes canônicos ou contratos públicos. As estruturas abaixo são *propostas*, não tipos compilados nem funcionalidades já implementadas. Não fazer todas as extrações na mesma PR. A #56 permanece aberta até as quatro fases terem sido implementadas/validadas.

## Estado atual e origem do acoplamento

O método reúne responsabilidades hoje espalhadas pelo mesmo fluxo:

1. Valida parent/child, policy V1/V2, IDs e snapshot V1 por `VerifySnapshotV1`, `ValidatedGenesis` e `fork.VerifyOriginWithResolver`; lê/decodifica o snapshot, suas dez partições de pendências, roster de memória, fontes finais e prova de prefixos.
2. Deriva o Genesis filho, migra ocorrências Genesis/Event/Claim, transmissões `IN_TRANSIT`, rounds com dispatch/revocation/response e estado evoluído de triggers; prepara manifests e raízes históricos.
3. Abre transação **read-only** no parent para autenticar e copiar os cinco prefixos na ordem exata; abre transação **de escrita no child**, chama `ensureForkDestinationEmpty`, publica todas as autoridades e selos.
4. Chama `validateForkLineageTx` **dentro da transação filha e antes de `tx.Commit()`**; somente depois o Genesis pode ser publicado. Reabertura e `ReplayClock` são verificações adicionais, não substitutos desse gate.

O código alterna `[]any` dos decoders canônicos com dados tipados de migração e queries SQLite. Um erro de classificação ou de ordem de execução pode contaminar outra autoridade. O projeto **não** possui transação distribuída entre parent e child; falhas depois da publicação da filha usam o caminho de recuperação somente leitura, `RecoverPublishedForkV1ToV2`.

## Grafo de dependências e fronteiras propostas

```text
inputs versionados (origin, child policy, run_id, parent snapshot id)
     |
[1] VerifiedSourceCut  -- snapshot/prefixos autenticados / somente leitura
     |
[2] ForkMigrationPlan  -- reads explícitos de autoridades + derivação pura
     |
[3] ChildTransaction   -- guard de destino limpo; todos INSERTs e selos
     |
[4] VerifyLineageTx    -- verifica a candidata ainda não publicada
     |
   tx.Commit()         -- única publicação atômica da filha
     |
   Recovery/Replay     -- somente leitura; não repete terminal V1
```

A fase 2 só pode se declarar **pura** depois de separar as leituras de SQLite da derivação. Helpers hoje consultam o banco, como `PrepareV2CommunicationBirths`, `prepareNativeRoundIntentions`, `prepareTriggerManifestV2` e os readers de histórico; **não** chamá-los de funções puras por renomeação. Migrar as leituras para um conjunto tipado *read-only* explicitamente autenticado, mantendo a mesma ordenação/validadores, antes de extrair a parte determinística.

### Estruturas internas propostas (esboço Go, não API implementada)

```go
// Só construído após VerifySnapshotV1, ValidatedGenesis e policy pinning.
type verifiedForkCut struct {
    sourceRun, snapshotID [32]byte
    parentGenesisCanonical, parentSnapshotCanonical []byte // cópias próprias
    snapshotClock clock.State
    snapshotControlStatus uint64 // IDLE obrigatório no entrypoint idle
    pending forkPendingRosters   // somente decoders validados
    owners []forkCognitiveOwnerRef
    ledgerPrefixes [5]forkVerifiedPrefix
}

// Guarda IDs/digests e bytes verificados; sem []any do codec fora do decoder.
// Definir campos adicionais somente após mapear contratos R19–R26.
type forkPendingRosters struct {
    occurrences []forkPendingOccurrenceRef
    activations []forkPendingTriggerRef
    rounds []forkPendingRoundRef
    inputs []forkPendingInputRef
}
type forkVerifiedPrefix struct {
    ledger string // exatamente um dos cinco nomes normativos
    cursorCanonical []byte
    records []forkArchiveRecord // ordem por ordinal verificado
}
type forkArchiveRecord struct {
    id [32]byte
    canonical []byte
}

// Derivação: 0 writes; entradas não podem ser reusadas por referência mutável.
type forkMigrationPlan struct {
    childGenesisCanonical []byte
    migratedOccurrences []forkGenesisOccurrence
    communicationBirths []V2CommunicationBirth
    migratedRounds []forkGenesisRound
    migratedSlots []forkSlotRecord
    historicalRoots []forkHistoricalRoot
    occurrenceRoots []ports.CanonicalRecord
    triggerMigration forkTriggerMigration
    triggerManifestCanonical []byte
    archivedSource [5]forkVerifiedPrefix
    cognitiveOwnerProofs []forkCognitiveOwnerProof
}

// O comando de publicação não pode receber Store parental para escrita.
// Contrato de persistência: uma *sql.Tx do child, guard, INSERTs, verify, commit.
type forkChildPublishRequest struct {
    sourceRun, childRun, sourceSnapshotID [32]byte
    parentGenesisCanonical, parentSnapshotCanonical []byte
    plan forkMigrationPlan
}
```

Estes nomes/campos **não** reescrevem `[]any` normativos nem `SnapshotV1` no storage. Cada fase deverá provar que os novos decoders mantêm aridade, discriminantes, bounds, `strict decode`, ordem canônica e `fail closed`. O desenho de decoders tipados para partições e round history também é objeto da [#57](https://github.com/carloshgalves/cote-ai-simulation/issues/57); coordenar interfaces para evitar duplicação ou retrabalho.

## Plano de implementação — exatamente uma fase por PR posterior

| PR futura | Extrair / entradas | Saída e invariantes verificáveis | Não tocar |
| --- | --- | --- | --- |
| **#56/P1 — verified cut** | Identidades, validação de origem e snapshot, finalização de fontes, checks de partições, `CSFHREF`, prefixos arquiváveis | `verifiedForkCut` construído **somente** após validação; preservar erro, origem sob policy antiga, exact live cut e read-only parent. Testes de corrupção e snapshot histórico. | Writers do child e migração sem relação com corte |
| **#56/P2 — migration planning** | Leituras autenticadas do parent separadas dos migradores; construção de `forkMigrationPlan` e Genesis V2 | Mesmos births, IDs, epochs, epochs de triggers, ordinais e bytes de manifests. Ordem dos records e source-root checks idêntica; nenhum writer. Testes R19–R23 e recomposição de sibling fork. | Guard do child, INSERTs, codec e política |
| **#56/P3 — child publication** | Uma função proprietária de `*sql.Tx` do child; `ensureForkDestinationEmpty` da #55; inserções ordenadas e selos | `BEGIN -> guard -> writes -> validate -> COMMIT/ROLLBACK` sem Genesis parcial. Todos os fault points e FK-off negatives devem rejeitar e permitir retry. | Auditoria read-only do pai e composição de bytes |
| **#56/P4 — lineage verification** | Endurecer fronteira de `validateForkLineageTx` como pré-commit, com recovery/replay apenas após commit | Rejeitar corrupção/missing evidence antes de publicar; não deslocar checks para pós-commit, não reabrir WORK unrelated, revalidar lineage e parent prefix no reopen. | Redesign do replay, domínio, regras V2 |

**Atenção à fase 1/2:** hoje a cópia dos cinco prefixos é uma transação de leitura do parent **posterior** às derivações. Mover a coleta para antes pode mudar concorrência/erros. A PR deve provar que a transposição preserva o *exact-cut* verificado sob alterações concorrentes ou manter a ordem atual; não assumir que as queries são intercambiáveis. O contrato dos stores parent/child continua independente, sem 2PC nem rollback entre bancos.

## Regras de composição e revisão

- Uma PR toca **um único estágio**, preferencialmente até 400 linhas de código produtivo e até 10 arquivos (fixtures normativas excluídas), exceto justificativa explícita; não extrair a função de 383 linhas em bloco.
- `plan` é dado interno, **nunca autoridade independente**: a validação de origem, registros, roster de signing keys, hashes e replay continuam necessárias.
- Ordem de publicação da filha, digest, IDs, snapshot, canonical CBOR, `fork_history_records` e fallback legado permanecem idênticos.
- Falha em qualquer `INSERT`, selamento ou `validateForkLineageTx` continua retornando erro e abortando a transação. Sem flush externo inventado; a #58 cobre produção com owner cognitivo.
- R26-02: retry de filha já publicada é **somente leitura**, sem novo WORK no parent; R26-01: fork V1→V2 de corte histórico com parent avançado continua inelegível.
- Para cada PR: inspecionar diff focal + dependências imediatas, executar negativos e mutantes relevantes, conferir fingerprints e CI do SHA exato; findings fora do estágio viram issue distinta. A #56 fecha **somente** quando as quatro fases e gates estiverem aceitos pelo operador.
