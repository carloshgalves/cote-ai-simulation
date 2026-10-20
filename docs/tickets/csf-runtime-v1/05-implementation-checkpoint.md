# CSFRV1-5 — checkpoint de implementação

**Branch:** `feat/csfrv1-5-atomic-terminal-commit`
**Draft PR:** [#21](https://github.com/carloshgalves/cote-ai-simulation/pull/21)
**Contrato:** [05-commit-atomico-e-retry.md](05-commit-atomico-e-retry.md), ADR-0008 e ADR-0011.

> **STATUS: PARCIAL — PR NÃO APROVADA PARA MERGE.** Os caminhos
> `NoProposal`, consumo único de estoque e disputa determinística entre dois
> agentes estão implementados com publicação atômica e replay. O ticket
> completo ainda exige lifecycle de occurrence/trigger, `REJECT/DEFER`,
> política de cascata e cobertura integral dos critérios.

## Autoridade de execução entregue

- O mesmo `sqlite.Store` da CSFRV1-3 recebe a `AdmissionFence` e controla
  `ATTEMPT_IN_FLIGHT → IDLE` na transação de `CycleCommit`.
- `INDETERMINATE` e outras causas de abort preservam world/clock/event
  state, registram `CycleAbortRecord` e `HALTED_ON_ABORT`.
  `AttemptRetryRecord` autoriza exclusivamente `attempt + 1` com corte
  semântico preservado. O fluxo abort → retry → novo fence → sucesso WORK
  está testado.
- `NoProposal` fecha exatamente cada slot dos rounds selecionados com
  `DecisionRecord(NO_PROPOSAL)` e evento secreto
  `SOURCE_LIFECYCLE(round.consumed)`, journal, revisão e sequência lógica.
  Não admite sources exógenas, occurrences nem ativações cujo lifecycle
  ainda não tenha handler — falha fechado.
- Um `ActionProposal(resource.consume)` com capacidade autenticada no
  `WorldState` e exatamente uma unidade gera `COMMIT`,
  `resource.consumed`, `round.consumed`, nova `WorldRevision`
  (estoque 1 → 0), sequência contígua e `CycleCommit` na mesma transação.
- Dois agentes de escola com propostas concorrentes passam por
  `CommitCandidate`, assessments, `ConflictSet` de recurso e
  `RngDraw` de substream nomeada pelo seed pinado no genesis.
  Exatamente um `COMMIT` e um `REJECT` produzem um evento de consumo
  e dois encerramentos de round; o estoque passa de 1 para 0.
- Os candidatos, assessments, conflict e draw de uma disputa são
  publicados no DecisionLedger junto com os `DecisionRecord` e
  `CycleCommit`. O EventStore contém somente eventos; nenhum
  `CycleCommit` aparece ali.
- Os eventos do recurso são `privacy=2` (internos), sem observador
  registrado, e portanto `PerceptionTask`/outbox vazios são explícitos e
  autenticados. Não há rota de `KnowledgeInput` expondo estado secreto
  aos agentes.
- A interpretação do typed stock é fechada e versionada em
  `internal/csf/reducer/testresource.go`; estoque negativo, tipo desconhecido,
  digest inconsistente e tentativa de tratar estado caller como mundo
  autoritativo são rejeitados.
- `CLOCK_ADVANCE` usa a revisão mundial corrente, não o snapshot de
  genesis. O teste WORK com consumo → avanço temporal → replay mantém o
  estoque consumido.
- A dobra de replay/retomada verifica `CycleControlState`, vínculos
  fence/abort/retry, decisão, Evento, typed `WorldRevision`, batch digest,
  políticas pinadas e sequência. Um assessment faltante na disputa faz
  o replay falhar, em vez de aceitar um vencedor sem provas.
- O vínculo dos papéis da CSFRV1-4 com políticas foi corrigido: utiliza
  ID/pin declarado no genesis, não posição ordinal em listas distintas.
  Há teste de regressão específico para esse erro.

## Evidência de testes

- Execução de `go test ./...` e conformance Go/TypeScript;
  fixtures canônicas Go/TS idênticas e `git diff --check`.
- `NoProposal`: sete failpoints no publisher, adulterações de source,
  decisão, regras e world hash, retry autorizado e permutação de
  respostas com bytes idênticos.
- Recurso único: sete failpoints, sete adulterações, replay e verificação
  de world stock após reabertura.
- Disputa A/B: sete failpoints; provas adulteradas (evento, vencedor,
  assessor/candidato, ordem, digest de estado, lifecycle, prefixo);
  permutação A→B/B→A com mesmo fence, decisores, eventos e batch digest.
  Replay retém vencedor/derrotado e não reabre slots liquidados.
- Replay adversarial: remover `AffordanceAssessment` persistido
  invalida a prova do vencedor, sem tolerar auditoria parcial.

**Último checkpoint conhecido verde anterior ao hardening atual:**

- [CSFRV1-5 — 39493082054](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39493082054).
- [Conformance canônica — 39493081590](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39493081590).
- [CSFRV1-4 regressão — 39493081444](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39493081444).

## Critérios ainda abertos (bloqueadores reais para CSFRV1-5)

- [ ] Processar `ScheduledOccurrence` como unidade admitida,
  `REJECT/DEFER` com encerramento irrevogável da identidade antiga e
  sucessor novo, recurrence e provenance explícita.
- [ ] Processar `TriggerActivation(PENDING)` com handler/candidato
  autenticado, DecisionRecord, SOURCE_LIFECYCLE e transição para
  `CONSUMED` no mesmo commit — jamais por caminho paralelo.
- [ ] Integrar as fontes acima à partição completa de `AdmissionFence`
  e ao fold de replay, mantendo validações e testes de rollback em cada
  barreira de publication/settlement.
- [ ] Limite **versionado** de cascata no mesmo instante, falha fechada
  como `CycleAbortRecord(CASCADE_LIMIT)`, replay sem truncamento.
- [ ] Evidência end-to-end da publicação total das seis partes exigidas
  pela spec, inclusive tarefas perceptivas **quando realmente aplicáveis**
  (as atuais ações secretas não geram tasks).
- [ ] Auditoria/review independente e ciclos de correção até não
  restar finding bloqueador; só então avaliar merge.

**Atenção:** um sucesso na variante estreita do último recurso não equivale
a um handler genérico para todas as fontes e dispositions de WORK. Esta PR
permanece Draft até que o ticket seja demonstravelmente satisfeito.
