# CSFRV1-5 — checkpoint de implementação

**Branch:** `feat/csfrv1-5-atomic-terminal-commit`
**Draft PR:** [#21](https://github.com/carloshgalves/cote-ai-simulation/pull/21)
**Contrato:** [05-commit-atomico-e-retry.md](05-commit-atomico-e-retry.md), ADR-0008 e ADR-0011.

> **STATUS: IMPLEMENTAÇÃO DA PRIMEIRA RUN VALIDADA; PR AINDA DRAFT.**
> `NoProposal`, recurso único, disputa entre dois agentes,
> `ScheduledOccurrence(REJECT/DEFER)`, `TriggerActivation(COMMIT)`,
> `CASCADE_LIMIT` e publicação `PerceptionTask(PENDING)` passaram pelo
> mesmo SQLite/replay. Coortes simultâneas de sistema, `NoProposal` +
> sistema, recurso único + sistema e **duas propostas concorrentes +
> ocorrência + trigger** são commitadas como uma única partição causal.
> O marco da primeira run não depende de `DEDUPLICATED` ou de um
> WORK vazio sem fontes admitidas; essas variantes continuam dívida
> de escopo, sem alegação de suporte implementado. Observations reais
> e processamento das tasks continuam fora desta etapa.

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

**Checkpoint verde após publicação atômica de tasks e TODAS as coortes do primeiro experimento:**

- [CSFRV1-5 — 39611426425](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39611426425).
- [Conformance Go/TypeScript — 39611426566](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39611426566).
- [Regressão do kernel — 39611426449](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39611426449).

**HEAD verificado:** `f5315ad0d1121510e4ba45f9cb3b316467d29358`.
A suíte inclui cenários com duas propostas disputando estoque, ocorrência
agendada e trigger na mesma coorte; a versão invertida de chegada dos
agentes mantém bytes do `AdmissionFence`, `CycleCommit` e eventos.
O `WORK Mode 6` reconstroi também as seis provas de assessment/conflito/RNG
a partir do `DecisionLedger` durável.

## Revisão adversarial e critérios abertos

- [x] Ocorrências agendadas: `REJECT/DEFER`, sucessor, lifecycle e
  replay sem mutação parcial.
- [x] Ativações de trigger: handler pinado, consumo terminal e replay.
- [x] Limite de cascata versionado, `CASCADE_LIMIT` e replay fail-closed.
- [x] Admissão de múltiplas fontes **de sistema** na mesma coordenada,
  com bijeção de eventos/decisões e failpoints.
- [x] **Finding 1 — High:** mesmo `CommitWork` fecha todos os
  tipos combinados necessários à primeira run: `NoProposal` + sistema,
  proposta única + sistema e **duas propostas em disputa + sistema**.
  `Mode 5/6` preservam a prova do redutor/recurso, conflito/RNG,
  decisões de todas as fontes, `DEFER` e outbox; SQL e replay
  rederivam a partição. Testes de sete failpoints, oito/nove adulterações,
  reabertura de SQLite, trigger + ocorrência na mesma coorte, e
  permutação de chegada A/B demonstram atomicidade e determinismo.
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
- [ ] **Dívida pós-primeira-run:** commit vazio sem fontes elegíveis,
  disposition `DEDUPLICATED` e variantes futuras de reducer/settlement
  genéricos. O clock atual só admite WORK com fonte causal elegível,
  portanto essas variantes não são exercidas pelo primeiro experimento.
  Rastreada na [issue #22](https://github.com/carloshgalves/cote-ai-simulation/issues/22).
  Não confundir dívida rastreada com funcionalidade implementada.
- [x] Planner puro de tarefas potencialmente perceptíveis, com
  publicação/rollback/replay das tasks em ambas as variantes de ciclo.
  O predicado V1 é o ordinal local do evento, não a confidencialidade;
  a tarefa não constitui `Observation` nem autoriza conhecimento.
- [x] Replayed causal_outbox é uma partição exata dos task IDs de
  CycleCommits: eventos sem task, task órfã, task com conteúdo alterado
  e task duplicada tornam o histórico inválido.
- [x] Revisão adversarial do marco da primeira run concluída após
  os cenários de coorte mista e adversarial mutations. Os três
  findings congelados (#1–#3) estão `RESOLVED` em seus comentários
  originais, com zero findings **OPEN** dentro do escopo estrito da
  primeira execução. Não equivale a prova de ausência universal de bugs.

**Comentários de revisão:** [Finding 1](https://github.com/carloshgalves/cote-ai-simulation/pull/21#issuecomment-6305195593),
[Finding 2](https://github.com/carloshgalves/cote-ai-simulation/pull/21#issuecomment-6305215141)
e [Finding 3](https://github.com/carloshgalves/cote-ai-simulation/pull/21#issuecomment-6305215875).

**Atenção:** um sucesso na variante estreita do último recurso não equivale
a um handler genérico para todas as fontes e dispositions de WORK. Esta PR
permanece Draft até que o ticket seja demonstravelmente satisfeito.
