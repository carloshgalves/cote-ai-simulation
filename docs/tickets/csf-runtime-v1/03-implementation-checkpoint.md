# CSFRV1-3 — checkpoint de implementação (19/10/2026)

**Situação: REVIEW REMEDIATION IMPLEMENTED — PR #19 Draft, aguardando nova revisão independente.**
Nenhum merge ou aprovação automática. A revisão independente original foi executada
e seus seis findings foram tratados na branch; validação do implementador
não substitui re-review.

**Base:** `main@7b7fc7edaf844ff6eeb637299fd6e00ec93f48db`.
**HEAD do checkpoint anterior à revisão:** `4a1eb737dabc9c719be508b0aeed136966d06fd9`;
CI de implementação e conformance verdes (`go test ./...`, 621/621 Go,
621/621 TypeScript, relatórios comparados byte a byte).
[Go CSFRV1-3](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39313704003/job/118269598597) · [conformance](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39313712927/job/118269627412).

## Implementação integrada

- `Clock` seleciona a menor pendência por coordenada causal;
  avança lacunas ordinais sem ciclo fictício, confirma `P` contra
  ingressos `Q < P` e atribui piso sucessor correto.
- SQLite persiste em **uma transação** `CycleCommit`, dois eventos
  temporais sintéticos não perceptíveis (`clock.advanced` e `body.advanced`)
  e eventos internos de lifecycle `TRIGGER_RUNTIME`/`TRIGGER_ACTIVATION`
  para as transições registradas. Revisão, novo `ClockState`, runtime,
  `TriggerActivation(PENDING)` e o conjunto completo de eventos são indivisíveis.
  A sequência lógica cobre todos os eventos do batch e o input ledger permanece imutável.
- `TriggerDefinition` v1 é codificada conforme `foundation.cddl`,
  com typed-value versionado `clock-threshold.v1`, dependencies
  canônicas, policy e handler ligado ao policy ref de
  `dependency-footprints` do genesis validado. O bootstrap registra
  **uma única vez** o conjunto completo num registry imutável, com
  genesis ID/digest e root hash. Nenhuma definição é enviada pelo
  host por ciclo.
- O bootstrap avalia a condição na revisão 0 e materializa as
  ativações iniciais verdadeiras com origem `GenesisRef`; elas
  permanecem `PENDING`.
- Event footprints sintéticos são funções puras versionadas:
  `clock.advanced → ["clock"]`, `body.advanced → ["body"]`.
  `causing_event_ids[]` é derivado do EventOrderKey canônico, não
  da ordem de workers. Não há acesso a belief, LLM, relógio de parede
  nem EventStore irrestrito no predicado temporal.
- `CommitClockAdvance` verifica a integridade do registry, aceita
  apenas **todas** as definições gravadas, reexecuta os predicados
  registrados dentro do commit e exige runtime/ativações exatamente
  correspondentes; não permite omitir triggers nem forjar origem.
- `ReplayClock` reconstrói `ClockState`, eventos (temporais e lifecycle),
  transições de triggers e ativações a partir de genesis, definições seladas,
  journals e eventos persistidos. Valida bytes canônicos integrais, causas,
  vínculo com AdmissionFence, states tipados e projeções SQL,
  sem receber `TriggerWork` ou predicados do host.
- `RearmPure` existe como transição pura e requer ID de evento
  causal explícito; não é uma operação de escrita nem uma autorização
  para alterar runtime diretamente.

## Provas executáveis (CI verde)

1. `(t,1) → CLOCK_ADVANCE(t,2) → (u,-1)` e próximo WORK em `(u,0)`;
   nenhum commit decorativo na lacuna ordinal.
2. `Q < P` entra após a seleção provisória: a transação rejeita
   o avanço e mantém a revisão anterior.
3. Crash injetado em seis fases do commit: atomicidade e retry sem
   duplicação ou efeitos parciais; `CycleCommit` no decision ledger,
   eventos no event store e world revision consistentes.
4. Reopen SQLite e permutação dos registros de definição/worker:
   mesmos bytes, IDs, digests, runtime e ativações.
5. Boot de trigger inicialmente verdadeiro na revisão de genesis:
   ativação pendente em `(t,0)`, selecionada como WORK.
6. Políticas edge, once, repeat e cadência positiva; `next_repeat_at`
   é persistido e concorre no mínimo temporal.
7. Trigger em `clock` e trigger em `body` apontam para seus
   eventos causais distintos, ambos com proveniência verificável.
8. Lote adulterado (batch digest, event digest, revision hash,
   definição imutável, runtime inventado, ativação omitida ou
   proveniência falsa) rejeitado sem mutações parciais.
9. Replay sem inputs de trigger fornecidos pelo host, inclusive do
   genesis ainda sem commits e de uma revisão já publicada.
10. `CLOCK_ADVANCE` rejeita tentativa de rearm não publicada por evento
    causal de WORK, sem alterar a revisão ou o runtime.
11. Replay de um run concluído falha fechado diante de corrupção
    dos registros SQLite de `TriggerDefinition`, runtime, ativação,
    journal, evento ou relógio.
12. Para avanço temporal puro, cada hash e os typed-values pré/pós
    da revisão do mundo são confrontados com o genesis; divergência em
    `world_revisions.state_hash` ou nos bytes de estado é detectada após reopen.
13. Teste de arquitetura percorre arquivos Go de produção e exige que
    só o coordinator de `internal/csf/commit/clock_advance.go`
    invoque o port transacional `CommitClockAdvance`.

## Gate de escopo ainda aberto

**Rearm manual durável (dependência arquitetural de WORK):** o reducer `RearmPure` tem contrato explícito
e não aceita evento causal ausente, mas publicar manual rearm e seu
evento de lifecycle exige um `WORK CycleCommit`. Isso depende do
pipeline terminal CSFRV1-4/5, que está **explicitamente fora do
CLOCK_ADVANCE** e não pode ser simulado com `UPDATE trigger_runtime`
direto ou um segundo writer provisório. O binding da transição
manual a um evento/commit persistido ainda não foi demonstrado.
A decisão de aceite deve registrar essa dependência, sem chamar
a funcionalidade completa de rearm de implementada.

**Limite adicional do V1:** os dois eventos de temporalidade usam
payloads sintéticos versionados, **não** um reducer físico de
Embodiment. Alterar a política de percepção ou footprint exige
aprovação versionada, não mudança de callback de host.

## Próxima fase

Após a remediação, executar uma **nova sessão independente de `code-review`**
somente quando solicitada. Não iniciar outra revisão nesta fase de correção.
**Estado do gate:** parte temporal de CSFRV1-3 validada; rearm manual ainda
não tem publicação causal executável e permanece pendente do WORK
CycleCommit. Não inferir aprovação do ticket inteiro a partir da CI verde.

Não mesclar à `main` sem revisão e aprovação formal. Não tocar em
CSFRV1-4/5, Canon RAG ou Examination Engine nesta branch.

## Checkpoint pós-revisão — 19/10/2026

**Findings congelados da revisão da PR #19:** 1–6, todos High.
**Commits de correção:** `ba5b602877f5e48d26a1b5f04207ad9e6f7ddd43`,
`decc67522f3a2523a2fd0a16931bab8b0b947a4e` e
`657dfd12238c7a06a37fe7a93f47fa3ed0ffdeab`.
O commit documental/CI subsequente é apenas o checkpoint de validação.

**Correções por finding:**

1. Fatos internos `trigger.runtime.transition` (phase 3) e
   `trigger.activation.created` (phase 4) acompanham os dois eventos
   temporais no mesmo `CycleCommit`. O replay reconstrói bytes/ids/ordem
   integralmente; lifecycle não entra nos `pre_trigger_events`.
2. `pendingClock` valida todas as linhas de occurrence/activation,
   inclusive as declaradas consumidas por índice SQL, contra record canônico,
   identidade, digest, elegibilidade e lifecycle. Replay compartilha o gate.
3. Commit e replay exigem o `AdmissionFence` durável, derivam seu ID/digest
   e vinculam plano, cursor de input e prova de fechamento às autoridades
   de fonte e a seus ingress receipts. Fence ausente não admite replay.
4. Replay refaz integralmente os bytes da `TriggerActivation` a partir
   de genesis, definição, runtime e ciclo, não apenas seu próprio digest.
5. `WorldRevision` tem transição de typed-values íntegros também na
   revisão zero; estados pré/pós e hashes são confrontados com o genesis.
6. Bootstrap das occurrences deriva ID/digest, valida coordenação e
   lifecycle e exige `created-by` correspondente ao genesis persistido.

**Regressões:** index-only tamper de lifecycle, remoção de fence,
activation re-hashed com eligibility forjada, adulteração de typed
world state mantendo hash, occurrence com ID/digest falsos, contagem
e reconstituição de lifecycle events. Permanecem as provas anteriores
de rollback, worker permutation, rearm clandestino e replay sem host.

**Gate mantido:** rearm manual causal ainda requer `WORK CycleCommit`
(CSFRV1-4/5), sem `UPDATE` paralelo de runtime. O sucesso da
remediação não declara esse feature completo nem autoriza merge.

**Próxima fase:** `$code-review CSFRV1-3` independente, apenas por
solicitação explícita após as checks da PR.
