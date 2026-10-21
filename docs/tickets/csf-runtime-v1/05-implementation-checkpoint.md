# CSFRV1-5 — checkpoint de implementação

**Branch:** `feat/csfrv1-5-atomic-terminal-commit`
**Draft PR:** [#21](https://github.com/carloshgalves/cote-ai-simulation/pull/21)
**Contrato:** [05-commit-atomico-e-retry.md](05-commit-atomico-e-retry.md), ADR-0008 e ADR-0011.

> **STATUS: PARCIAL — PR NÃO APROVADA PARA MERGE.** Os caminhos
> `NoProposal`, consumo do recurso, disputa entre agentes,
> `ScheduledOccurrence(REJECT/DEFER)`, `TriggerActivation(COMMIT)` e
> `CASCADE_LIMIT` estão implementados e exercitados com SQLite/replay.
> Coortes simultâneas de sistema e coortes mistas `NoProposal` +
> sistema funcionam. Restam `ActionProposal(resource.consume)` combinado
> com sistema e o aceite integral da CSFRV1-5. A publicação de
> PerceptionTask PENDING para WORK e CLOCK_ADVANCE está integrada e
> auditável; observations reais continuam fora desta etapa.

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

## Evidência de agenda, triggers e limite de cascata

- A lista de ocorrências iniciais é selada no primeiro fence, inclusive
  unidades que só vencerão no futuro. Novos sucessores têm sua identidade
  e seu nascimento reconstituídos a partir do evento e da decisão DEFER
  efetivamente persistidos. Um ID/hash autêntico, sem causalidade, falha.
- `ScheduledOccurrence(test.occurrence)` suporta `REJECT` não recorrente
  e `DEFER` de intervalo fixo com sucessor de identidade distinta e
  elegibilidade futura. `TriggerActivation` é validada contra a definição
  pinada e encerrada por `DecisionRecord` e `SOURCE_LIFECYCLE`.
- Coortes de duas ocorrências, ocorrência + trigger e coortes mistas
  `NoProposal` + sistema publicam uma partição completa de decisões/eventos
  numa transação. O híbrido usa `CommitWork Mode 4` no mesmo port, ordenação
  global de `EventOrderKey`, replay com prova de ambas as classes e
  testes de rollback em sete failpoints. `DEFER` combinado com dois slots
  deixa somente o novo sucessor futuro pendente.
- Permutar a chegada dos trabalhadores no cenário misto preserva
  exatamente os bytes do `CycleCommit` e dos eventos; manipular candidato,
  decisão, ordem de eventos ou hash de mundo falha fechado.
- A regra `CascadePolicyVersionV1` permite no máximo 64 WORKs no mesmo
  instante; a 65ª tentativa emite `CycleAbortRecord(CASCADE_LIMIT)`,
  preserva a fonte ainda pendente e não altera estado. Commit e replay
  exigem o pin exato `cycle-coordinate-v1@1` do Genesis, com teste
  adversarial de ID, versão e hash.
- Regressões: adulteração de candidato, decisão, evento, sucessor e
  WorldState em cada caminho terminal; remoção de affordance persistida;
  tentativa de inserção retroativa de fonte de genesis ou sucessor sem
  decisão `DEFER`; replay após reabertura de SQLite.

**Último checkpoint verde incluindo PerceptionTask e outbox completa:**

- [CSFRV1-5 — 39535453327](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39535453327).
- [Conformance Go/TypeScript — 39535453339](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39535453339).
- [Regressão do kernel — 39535453460](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39535453460).

## Revisão adversarial e critérios abertos

- [x] Ocorrências agendadas: `REJECT/DEFER`, sucessor, lifecycle e
  replay sem mutação parcial.
- [x] Ativações de trigger: handler pinado, consumo terminal e replay.
- [x] Limite de cascata versionado, `CASCADE_LIMIT` e replay fail-closed.
- [x] Admissão de múltiplas fontes **de sistema** na mesma coordenada,
  com bijeção de eventos/decisões e failpoints.
- [ ] **Finding 1 — High:** coortes mistas `NoProposal` +
  fontes de sistema resolvidas com WORK híbrido e replay. Permanece
  aberta a composição de `ActionProposal(resource.consume)` + sistema
  no mesmo fence; não há settlement parcial silencioso.
- [x] **Finding 2 — Medium:** o limiar V1 agora exige pin canônico
  exato do `cycle-coordinate-v1`; comentário original RESOLVED.
- [x] **Finding 3 — High:** classificador puro de `PerceptionTask`
  `event-local-ordinal=0` do `perception-visible-first-v1`, com
  política, resolver e identidade vinculados ao Genesis; publicação
  atômica para WORK e CLOCK_ADVANCE na `causal_outbox`, hashes e
  referências do `CycleCommit`, rederivação pelo SQLite e replay.
  Testes provam tasks não vazias, sete failpoints, remoção de task e
  inserção de task órfã. Uma `PerceptionTask` é **somente PENDING**:
  não concede Observation, destinatário ou KnowledgeInput; o processamento
  downstream permanece fora deste ticket. Comentário original RESOLVED.
- [ ] Commit vazio canônico, `DEDUPLICATED` e demais variantes de
  settlement e matriz completa de fault injection, se exigidos pelo
  ticket de aceite.
- [x] Planner puro de tarefas potencialmente perceptíveis, com
  publicação/rollback/replay das tasks em ambas as variantes de ciclo.
  O predicado V1 é o ordinal local do evento, não a confidencialidade;
  a tarefa não constitui `Observation` nem autoriza conhecimento.
- [x] Replayed causal_outbox é uma partição exata dos task IDs de
  CycleCommits: eventos sem task, task órfã, task com conteúdo alterado
  e task duplicada tornam o histórico inválido.
- [ ] Uma última revisão separada depois de fechar os bloqueios
  anteriores; manter Draft até isso acontecer.

**Comentários de revisão:** [Finding 1](https://github.com/carloshgalves/cote-ai-simulation/pull/21#issuecomment-6305195593),
[Finding 2](https://github.com/carloshgalves/cote-ai-simulation/pull/21#issuecomment-6305215141)
e [Finding 3](https://github.com/carloshgalves/cote-ai-simulation/pull/21#issuecomment-6305215875).

**Atenção:** um sucesso na variante estreita do último recurso não equivale
a um handler genérico para todas as fontes e dispositions de WORK. Esta PR
permanece Draft até que o ticket seja demonstravelmente satisfeito.
