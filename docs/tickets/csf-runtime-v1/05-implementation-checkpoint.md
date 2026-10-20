# CSFRV1-5 — checkpoint de implementação (WIP)

**Branch:** `feat/csfrv1-5-atomic-terminal-commit`
**Draft PR:** [#21](https://github.com/carloshgalves/cote-ai-simulation/pull/21)
**Base:** `main` após o merge da CSFRV1-4 (#20).
**Contrato:** [05-commit-atomico-e-retry.md](05-commit-atomico-e-retry.md); ADR-0008/0011.

> **Status: PARCIAL — NÃO APROVADO PARA MERGE.** O caminho **Abort/Retry**
> está implementado e testado. A publicação **bem-sucedida** de WORK com
> `CycleCommit`, `DecisionRecord`, efeitos e revisão permanece por fazer.
> O teste verde da branch NÃO constitui evidência de conclusão do ticket.

## Entregue, versionado e testado

- [x] Branch dedicada, commits remotos e PR Draft imediatamente no início.
- [x] Mesmo `sqlite.Store` transacional de `CLOCK_ADVANCE` e extensão
  do **mesmo** `ports.ClockAdvanceStore`; nenhum novo banco de autoridade.
- [x] `AdmissionFence` autenticado e `CycleControlState` levado a
  `ATTEMPT_IN_FLIGHT` no mesmo CAS SQLite. Revisão, relógio e world hash
  conferidos no ponto de admissão; reentrega exata da fence é idempotente
  enquanto a tentativa está em andamento.
- [x] `CycleAbortRecord` com ID/digest registrado, vínculo ao fence
  persistido, input digest e base revision, publish atômico no
  `DecisionLedger`, sem `Event`, settlement nem avanço de world/clock.
- [x] `IndeterminateRecord` de unidades admitidas vinculado exatamente a
  cada (ID,digest) do fence; sujeitos de `CommitCandidate` exigem
  roots canônicos, fontes/identidades/provenance autenticadas e
  material auditável persistido junto ao abort, no mesmo SQLite commit.
- [x] `HALTED_ON_ABORT` durável, único terminal abort por attempt e falha
  fechada no caso de fence ausente, cruzado ou caducado.
- [x] `AttemptRetryRecord` derivado da referência exata ao abort, com
  `from -> from+1`, inserção atômica e estado
  `RETRY_AUTHORIZED`. Duplo retry rejeitado.
- [x] Novo fence depois de abort somente com retry pré-publicado e
  referência autenticada; mesma janela semântica de input, revisão,
  políticas e prova de closure da tentativa anterior.
- [x] `CLOCK_ADVANCE` anterior continua fechando `CycleControlState`
  em `IDLE` dentro da própria transação (sem segundo commit).
- [x] Failpoints de abort/retry testam rollback do journal e controle em
  duas barreiras de gravação, com reabertura por conexão SQLite independente.
- [x] Integração real de duas `ActionProposal` em WORK, admitidas no
  ledger/fence: recurso sem validator gera `INDETERMINATE` para ambas,
  seguido de **somente** `CycleAbortRecord`, sem Event/DecisionRecord/
  nova revisão. Teste adicional de três candidatas força resolução
  ausente e autentica roots de candidato; prova forjada aborta a transação.

**CI dos testes anteriores a este documento:**
- [CSFRV1-5 Atomic Terminal Commit — 39448499336](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39448499336): **success**.
- [Canonical Go/TypeScript — 39448499347](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39448499347): **success**.
- [CSFRV1-4 regression — 39448499323](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39448499323): **success**.
- Full Go suite + runners canônicos Go/TypeScript (621 casos por runner),
  recibos idênticos e `git diff --check` cobertos no CI.

## NÃO implementado / critérios de aceite ainda abertos

- [ ] Transação de **sucesso WORK**: exatamente um `DecisionRecord`
  por unidade do fence e um `CycleCommit`, com revision, clock,
  source lifecycle, eventos e outbox atômicos.
- [ ] `EventDraft` -> `EventOrderKey` -> `event_id`, logical sequence
  contínua, reducers puros versionados, world working copy,
  state hash e invariantes.
- [ ] `PerceptionTask` proporcional aos eventos realmente visíveis,
  vinculada à mesma publicação.
- [ ] `ScheduledOccurrence` `REJECT/DEFER`, successor de `DEFER`,
  source lifecycle, round/activation terminal e operação bem-sucedida
  para `TriggerActivation(PENDING)`.
- [ ] Transição total do journal via **fold verificado no próprio
  armazenamento** e revalidação de replay/resume, inclusive erros
  de schema/reducer e decisões vencedoras, não só abort/retry.
- [ ] Limite versionado de cascata em mesmo instante e abort associado.
- [ ] Suite de fault injection em cada barreira da transação **de sucesso**,
  worker/event order permutations, world reducer invariants e
  scenario trace das seis partes publicadas.

### Restrições técnicas observadas na integração

1. A atual `decision.FromPersistedFence` da CSFRV1-4 admite **somente
   SlotResponse**. Recusa outras categorias de unidade do fence; não existe
   ainda um handler registrado que gere candidatos de
   `ScheduledOccurrence` ou `TriggerActivation` para o caminho WORK.
2. O candidato sintético `resource.consume` derivado em CSFRV1-4 possui
   lista de `EventDraft` vazia. Promover um consumo que altera resource
   stock sem evento registrado e sem reducer seria **estado causal inválido**.
   Não fabricamos um `CycleCommit` verde por cima dessa lacuna.
3. O ledger provisório de candidates no abort preserva auditabilidade,
   mas não substitui ainda o `DecisionRecord`, `Event` nem a
   `WorldRevision` do caminho de sucesso.
4. Nenhuma mudança nesta PR publica material de conflito, sorteios ou
   estado interno dos agentes como `KnowledgeInput`.

## Próxima implementação dentro desta mesma Draft PR

1. Registrar o `EventDraft`/typed resource state + reducer
   deterministicamente autorizado para o cenário mínimo; estender
   handlers de fontes conforme o contrato, sem bypass de política.
2. Derivar os `DecisionRecord` da partição completa e settlement,
   incluindo lifecycle e succesor normativos.
3. Estender **a mesma** transação de commit SQLite para eventos, reducers,
   state/clock revision, pending source transitions, tasks e journal final.
4. Provar atomicidade de seis partes por failpoints, replay e permutation;
   somente depois submeter a revisão independente da CSFRV1-5.

A revisão independente `$code-review CSFRV1-5` só deverá começar quando
os critérios de conclusão do ticket estiverem efetivamente satisfeitos.
