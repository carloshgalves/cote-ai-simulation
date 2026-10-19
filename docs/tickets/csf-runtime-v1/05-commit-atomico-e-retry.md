# CSFRV1-5 — Commit terminal atômico, abort e retry explícito

**Spec:** §6.4–6.5 · §7.5–7.6 · §9 · §11.1 (8–10) · §11.2 (atomicidade)  
**ADRs:** 0008 invariantes 7, 12, 17, 23–26, 29, 32, 35, 37, 39 e revalidação de 1–2/13 · 0011
**Bloqueado por:** CSFRV1-4
**Bloqueia:** CSFRV1-6

## Resultado observável

O cenário do recurso publica, em uma fronteira indivisível, exatamente um `DecisionRecord` por
unidade, eventos ordenados, lifecycle das fontes, tarefas de percepção, um `CycleCommit` e a nova
`WorldRevision`. Uma falha injetada em qualquer passo deixa ou o prefixo anterior completo ou o
commit completo. Um `INDETERMINATE` produz somente `CycleAbortRecord`; resume para parado e só um
`AttemptRetryRecord` durável autoriza nova tentativa sobre o mesmo fence semântico. `REJECT`/`DEFER`
liquidam occurrences no mesmo envelope, uma `TriggerActivation(PENDING)` é consumida somente pelo
caminho terminal completo, e exceder o limite de cascata produz abort auditável.

## Escopo

- Ampliar a mesma primitiva SQLite/`CommitCoordinator` entregue no CSFRV1-3; não criar outro port,
  envelope, store combinado ou caminho de publicação.
- `DecisionLedger` como journal canônico; `EventStore` contém somente eventos.
- `EventDraft` → `EventOrderKey` → `event_id`/`LogicalSequence` contíguos → `EventBatch`.
- Reducer registry com owner, versão/hash e dependency footprint; cópia de trabalho e invariantes.
- Settlement total: `COMMIT|REJECT|DEFER|NO_PROPOSAL|DEDUPLICATED`, lifecycle e sucessores.
- Processamento bem-sucedido de `TriggerActivation(PENDING)` pelo handler/candidato entregue no
  CSFRV1-4: partição da unidade, `DecisionRecord(COMMIT)`, `SOURCE_LIFECYCLE`, eventos do candidato,
  tasks aplicáveis, `CycleCommit` e revisão na mesma transação.
- Lifecycle terminal de `ScheduledOccurrence`: `REJECT` encerra a identidade; `DEFER` e recorrência
  criam sucessor com nova identidade, provenance e coordenada explícita dentro do mesmo commit.
- Publicação única das seis partes da spec §6.5, inclusive tasks ainda não processadas.
- `INDETERMINATE` e demais falhas viram `CycleAbortRecord` autoritativo antes de qualquer disposição;
  evidence normalizada, `CycleControlState` total e retry explícito.
- Limite versionado de cascata no mesmo instante; excedê-lo depois do fence produz
  `CycleAbortRecord`, sem truncamento, settlement, lifecycle ou revisão parcial.
- Commit vazio válido e revisão/ordinal/sequence reconstruíveis pelo journal.

## Arquivos e módulos prováveis

```text
internal/csf/{decisionledger,eventstore,worldstate,reducer,commit,controlstate}/
internal/csf/persistence/sqlite/
internal/csf/{commit,reducer,settlement,controlstate}/**/*_test.go
tests/failureinjection/
tests/scenarios/{terminal_commit,abort_and_retry,event_ordering}_test.go
```

## Testes determinísticos

- Bijeção entre unidades do fence e decisions; nenhum settlement parcial.
- Evento projeta schema/provenance/visibilidade de candidate + fontes verificadas, nunca metadado
  auxiliar/caller.
- Outputs A/B invertidos preservam order keys, event ids, sequences, batch digest e tasks.
- Refs set-like permutadas/deduplicadas preservam envelope; colisão de identidade falha fechado.
- Eventos de candidatos distintos no mesmo ciclo nunca viram pais entre si.
- Reducer puro não emite eventos, usa RNG/rede/LLM/wall clock nem escreve store.
- Falha no terceiro reducer, schema, provenance ou invariante aborta tudo.
- `NoProposal`/`DEDUPLICATED` ainda encerram round/occurrence/activation via lifecycle.
- `TriggerActivation(PENDING)` bem-sucedida só vira `CONSUMED` quando candidato, decision, lifecycle,
  eventos, tasks, commit e revisão passam juntos pela fronteira terminal; crash preserva tudo ou nada.
- Occurrence em `REJECT`/`DEFER` termina a identidade antiga; `DEFER`/recorrência cria exatamente um
  sucessor e o `DecisionRecord` referencia o candidato normativo.
- Fence tem exatamente um terminal envelope; abort não avança ordinal, consome fonte ou cria task.
- Cascata excedida depois do fence termina em `CycleAbortRecord`; não trunca outputs nem publica
  `DecisionRecord`, lifecycle, evento ou revisão.
- `INDETERMINATE` persiste dentro do abort auditável, sem nenhuma disposição ou publicação parcial;
  somente aqui a invariante 7 entra na matriz 41/41.
- Dobra cobre `ATTEMPT_IN_FLIGHT`, `HALTED_ON_ABORT`, `RETRY_AUTHORIZED`, `IDLE` e precedência.

## Property tests e failure injection

Injetar crash antes/depois de cada append lógico e CAS/publicação. Recuperação nunca observa evento
sem settlement/task/commit/revisão, nem decisão sem mundo. Permutações de candidatos, reducers
registrados e evidências de falha preservam bytes/digests. Replay mínimo recalcula ids, ordem,
sequence, revisão e state hash antes de reduzir.

## Evals e fronteira de conhecimento

Determinism eval do cenário do recurso sob seeds/paralelismos fixos. `PerceptionTask` não contém
contexto nem payload liberado a ator; decision material e world state seguem sem rota para agentes.

## Fora do escopo

Processar tasks, formar observations, comunicação, snapshot completo ou Observatory.

## Evidência de conclusão

1. Relatório de failure injection prova prefixo anterior ou commit completo em todos os pontos.
2. Auditoria reconstrói fence → decisions → events → revision e verifica digests.
3. Trace prova o consumo atômico de `TriggerActivation` por candidato/decision/lifecycle e as seis
   partes do commit, sem caminho especial de mutação.
4. Trace prova settlement/lifecycle atômico de `REJECT`/`DEFER` e identidade nova do sucessor.
5. Abort por cascade limit e abort/restore não retentam; retry explícito abre exatamente
   `attempt + 1` com corte preservado.
6. Event store não contém `CycleCommit`; journal não contém cópia autoritativa de eventos.
7. Teste de arquitetura encontra um único write path para events/revisions.
