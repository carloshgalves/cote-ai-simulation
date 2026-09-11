# ADR 0008 — Fundação causal da simulação V1

**Status:** Proposed
**Data:** 2026-09-10
**Relacionado:** ADR 0001 (world truth e conhecimento), ADR 0003 (tempo lógico), ADR 0006
(resolução física dentro do Simulation Engine), ADR 0007 (fronteira de dados do `Embodiment`)

## Contexto

Cognição, relações, economia, exames e facções precisam compartilhar uma resposta única e auditável
para quatro perguntas: **quando algo pôde acontecer, o que foi tentado, o que de fato aconteceu e quem
teve acesso a qual evidência**. Se qualquer domínio consumidor puder responder novamente “o que
aconteceu”, o projeto terá fontes de verdade concorrentes.

Os ADRs 0001 e 0003 já separam world truth de conhecimento e adotam tempo lógico orientado a eventos.
Ainda faltam contratos para simultaneidade, agenda, triggers, propostas, affordances, conflitos,
commit, mutação, percepção, comunicação, determinismo e replay. Em particular:

- ordem de conclusão de chamadas de LLM não pode dar prioridade causal;
- estar proibido não é o mesmo que ser impossível;
- uma condição verdadeira não pode disparar infinitamente por ser reavaliada;
- comunicação transporta uma alegação, não a torna verdade;
- evento secreto não pode entrar implicitamente no estado de um agente;
- reducers não podem inventar novos fatos enquanto projetam estado;
- telemetria operacional, rejeições de proposta e fatos do mundo não são o mesmo ledger.

Esta decisão modela somente a fundação causal. Ela fornece pontos de extensão tipados para regras e
subdomínios futuros, mas não define personalidade, raciocínio, relações, exames, PP/CP, clubes ou
facções.

## Decisão

### 1. Fronteira e autoridade

`Causal Simulation Foundation` é um subdomínio do **Simulation Engine**, não um bounded context par.
Ele é a única autoridade que:

1. avança o tempo da simulação;
2. fecha ciclos de resolução;
3. decide quais candidatos podem coexistir;
4. faz o commit de eventos como world truth;
5. aplica esses eventos ao estado autoritativo.

Os contextos consumidores podem:

- fornecer `ActionProposal`, regras versionadas, predicates de trigger, handlers de ocorrência,
  validadores e reducers de seus próprios tipos;
- receber `Observation` ou `KnowledgeInput` endereçado;
- consultar projeções para as quais tenham permissão.

Eles não podem gravar diretamente no event store, no world state, no relógio, na agenda ou no estado
epistemológico de outro ator. Um handler nunca “faz acontecer”: ele devolve um candidato ao commit.

O `Embodiment` continua dentro do Simulation Engine conforme o ADR 0006. Sua resolução física é um
validador/resolvedor especializado chamado pela fundação, não uma segunda autoridade de commit.

### 2. Vocabulário canônico

| Termo | Significado e dono |
|---|---|
| `SimulationInstant` | instante de calendário dentro do mundo; pertence ao Clock |
| `LogicalSequence` | posição global monotônica de um evento já commitado; nunca prioridade de uma proposta |
| `WorldRevision` | versão imutável do world state depois de um commit |
| `ResolutionCycle` | conjunto fechado de entradas avaliadas contra a mesma `WorldRevision` |
| `EligibilityCoordinate` | `(not_before_instant, not_before_cycle_ordinal)` durável de uma entrada causal |
| `CausalSource` | produtor declarado de entradas causais; toda entrada gravada pertence a exatamente uma fonte |
| `SourceClosure` | marcador durável e monotônico pelo qual uma fonte exógena declara “nada mais até esta coordenada” |
| `RoundDeclaration` | entrada causal durável que abre um decision round: ator, slots e coordenada de elegibilidade |
| `DecisionCohort` | conjunto das `RoundDeclaration` elegíveis a um ciclo; derivado dos ledgers, nunca do que o host observou |
| `AdmissionFence` | registro durável, derivado dos ledgers, que materializa a membresia de um ciclo antes de qualquer avaliação |
| `SlotDispatch` | registro durável que abre exatamente um pedido de resposta para um slot `(decision_round_id, slot_id)`; no máximo um dispatch aberto por slot; redespacho exige revogação durável |
| `SlotResponse` | preenchimento único de um slot — `ActionProposal` ou `NoProposal` — admitido somente contra o dispatch aberto; não é entrada de fonte exógena e não tem coordenada própria |
| `ScheduledOccurrence` | trabalho causal pendente cujo vencimento é temporal |
| `TriggerDefinition` | predicate puro e política explícita de ativação |
| `TriggerRuntimeState` | memória operacional necessária para edge/rearm/repeat |
| `ActionProposal` | pedido imutável para tentar uma ação; não é fato de que a ação ocorreu |
| `AffordanceAssessment` | diagnóstico facetado da proposta contra um snapshot |
| `CommitCandidate` | unidade atômica que pode produzir zero ou mais eventos e declarar conflitos |
| `Event` | fato imutável do mundo aceito em commit |
| `CycleCommit` | envelope persistido do `EventBatch` de um ciclo, obrigatório mesmo com zero eventos |
| `DecisionRecord` | desfecho terminal canônico de exatamente uma unidade admitida; existe somente dentro de um `CycleCommit` |
| `CycleAbortRecord` | envelope durável de tentativa abortada; audita `INDETERMINATE` e a causa sem publicar revisão nem liquidar nada |
| `AttemptRetryRecord` | decisão explícita e durável de abrir `attempt_ordinal + 1` para um ciclo parado após abort; consumido pelo fence que abre essa tentativa |
| `CycleControlState` | estado do coordinator derivado por dobra total e ordenada sobre fences, envelopes e retentativas: ocioso, tentativa em voo, retentativa autorizada ou parado após abort |
| `WorldState` | redução dos eventos autoritativos até uma revisão |
| `Observation` | evidência parcial disponibilizada a um observador por acesso perceptivo causal |
| `Proposition` | conteúdo semântico normalizado de uma alegação; value object sem verdade embutida |
| `Claim` | ato/artefato imutável de afirmar uma `Proposition`; não carrega verdade autoritativa |
| `Transmission` | comunicação em trânsito, persistente no world state |
| `KnowledgeInput` | recibo endereçado de evidência para o contexto de conhecimento; não é crença inferida |
| `Snapshot` | checkpoint versionado de estado autoritativo mais cursores necessários para continuar |

`scene`, `turn` e “momento narrativo” podem ser projeções de UX ou políticas de granularidade. Não são
unidades causais e não dão autoridade de commit.

#### 2.1 Entidades, value objects e agregados

- **Value objects:** `SimulationInstant`, `Duration`, `LogicalSequence`, `WorldRevision`,
  `WorldStateHash`, `ActorRef`, `EntityRef`, `LocationRef`, `ResourceClaim`, `Proposition`,
  `CausalRef`, `SeedKey`, `EligibilityCoordinate` e versões de schema/política.
- **Entidades/artefatos imutáveis:** `Event`, `ActionProposal`, `NoProposal`, `Claim`,
  `Observation`, `KnowledgeInput`, `SourceClosure`, `SlotDispatch`, `SlotDispatchRevocation`,
  `AdmissionFence`, `DecisionRecord`, `CycleAbortRecord` e `AttemptRetryRecord`; identidade e
  provenance importam mesmo quando dois payloads são iguais.
- **Agregados com lifecycle:** `ScheduledOccurrence`, `RoundDeclaration`, `Trigger`
  (`Definition + RuntimeState`) e `Transmission`.
- **Estado derivado do coordinator:** `CycleControlState` é uma dobra determinística, total e com
  precedência fixa sobre fences, envelopes terminais e `AttemptRetryRecord` (§8.0.2); o snapshot o
  persiste como cursor verificável (§12).
- **Fronteira de consistência:** `ResolutionCycle` agrega candidatos e produz um único `EventBatch` +
  `WorldRevision`, persistidos como um `CycleCommit` mesmo quando o lote tem zero eventos (§8.0). Sua
  membresia é derivada dos ledgers e materializada por um `AdmissionFence` durável antes da avaliação
  (§3.2.4) e a tentativa termina em exatamente um envelope: `CycleCommit` ou `CycleAbortRecord`
  (§8.0.1).
  `CausalRun` dá identidade ao histórico inteiro, mas não deve virar um objeto gigante carregado em
  memória; seus substates continuam com owners/reducers separados.

O `WorldState@revision` é a composição autoritativa desses substates. Atomicidade global no commit não
transfere a responsabilidade das invariantes locais ao coordinator.

### 3. Tempo, revisão e simultaneidade

#### 3.1 `SimulationInstant`

`SimulationInstant` é um inteiro assinado de microssegundos desde o Unix epoch em UTC, sob calendário
gregoriano proléptico. O timezone IANA faz parte da configuração imutável do run; para o cenário COTE
V1 é `Asia/Tokyo`. ISO-8601 local é somente view. Entrada com precisão mais fina é rejeitada em vez de
arredondada implicitamente. Comparações, duração e agenda operam sobre o inteiro canônico; ponto
flutuante é proibido.

O relógio mantém:

```text
ClockState {
  current_instant: SimulationInstant
  current_revision: WorldRevision
  next_logical_sequence: LogicalSequence
  cycle_ordinal_at_instant: integer
}
```

O relógio não usa tempo de parede para semântica. Pausa, velocidade de execução e duração de chamada
LLM são operacionais.

#### 3.2 `ResolutionCycle`

Um ciclo é identificado por `(run_id, instant, cycle_ordinal)`; cada tentativa dele é identificada
por `attempt_ordinal`. O ciclo declara:

```text
ResolutionCycle {
  cycle_id
  attempt_ordinal
  instant
  base_revision
  base_state_hash
  cohort_id
  fence_digest
  admitted_input_ids[]
}
```

Todas as propostas do ciclo leem exatamente `base_revision`. Nenhuma vê mutações de outra proposta do
mesmo ciclo. O lote é fechado por protocolo, não pela ordem de chegada:

- uma solicitação a agente é um `SlotDispatch` durável (§3.2.5) e carrega `dispatch_id`,
  `decision_round_id`, `slot_id`, `base_revision`, `effective_at` e a coordenada de elegibilidade da
  `RoundDeclaration` à qual o slot pertence;
- a resposta pertence àquele slot mesmo que outra chamada termine antes; um slot tem no máximo um
  dispatch aberto e no máximo uma resposta gravada;
- os slots do round são declarados na criação da `RoundDeclaration` e o round só fecha quando cada
  slot contém sua única resposta gravada: `ActionProposal` ou `NoProposal`; timeout operacional,
  quando usado, grava um `NoProposal` explícito contra o dispatch aberto. A resposta gravada é fato
  de ledger com o mesmo status qualquer que seja a origem — LLM, timeout ou operador: o que a
  produziu é telemetria, o que foi gravado é fato. Ela **não** é entrada de fonte exógena: é o
  preenchimento de uma unidade derivada e não passa por `SourceClosure` nem por normalização de
  ingresso (§3.2.5);
- uma segunda escrita para o mesmo slot — resposta atrasada, duplicada ou dirigida a um dispatch
  revogado — é rejeitada na admissão e apenas auditada, antes ou depois de o round fechar; nunca é
  inserida retroativamente nem promovida automaticamente ao ciclo seguinte. Uma nova tentativa exige
  novo round/input causal;
- o wall clock que levou ao `NoProposal` fica na telemetria e não aparece como causa dentro do mundo.

A membresia do ciclo não é decidida por ordem de chegada, pela ausência momentânea de trabalho
pendente nem por qualquer observação que o coordinator faça do host. Ela é **derivada** de quatro
construtos duráveis, definidos a seguir: a coordenada de elegibilidade de cada entrada, o fechamento
declarado de cada fonte exógena, as declarações de round que formam a coorte e o fence de admissão
que materializa o resultado dessa derivação. O coordinator nunca escolhe o corte; ele o calcula.

##### 3.2.1 Coordenada de elegibilidade

Toda entrada causal — input exógeno, `RoundDeclaration` e seus slots, ocorrência agendada, ativação
de trigger e sucessor criado por `DEFER` — carrega uma coordenada persistida:

```text
EligibilityCoordinate {
  not_before_instant: SimulationInstant
  not_before_cycle_ordinal: integer   // >= 0
}
```

A ordem é lexicográfica sobre `(not_before_instant, not_before_cycle_ordinal)`. Uma entrada é
elegível ao ciclo `(instant, cycle_ordinal)` quando sua coordenada é menor ou igual à coordenada do
ciclo. A coordenada é atribuída **uma única vez**, por regra versionada, no momento em que a entrada
é gravada de forma durável:

- input exógeno recebe `max((declared_effective_at, 0), open_coordinate(source_id))`, onde
  `open_coordinate` é o sucessor da última coordenada que a própria fonte fechou (§3.2.2). O valor
  declarado permanece no input ledger como provenance. Nunca se admite entrada no passado, e a
  normalização é uma função do ledger da fonte — não de qual fence o coordinator já gravou;
- `RoundDeclaration` recebe a coordenada escrita explicitamente por quem a criou: o genesis, ou o
  `CycleCommit` que a originou, com valor `(instant, cycle_ordinal + 1)` ou maior (§3.2.3); os slots
  herdam a coordenada da declaração, e a resposta que preenche um slot não recebe coordenada própria
  nem passa pela normalização de ingresso (§3.2.5);
- ocorrência agendada recebe `(due_at, 0)`, salvo quando a política de origem exigir ordinal maior;
- sucessor de `DEFER` recebe a coordenada escrita explicitamente pelo commit que o criou (§7).

A coordenada integra o input digest canônico e é reutilizada em replay/resume. Ela substitui qualquer
heurística de “foi criado depois, logo entra depois”: elegibilidade é fato declarado, não momento de
inserção.

##### 3.2.2 `CausalSource` e `SourceClosure`

Toda entrada causal pertence a exatamente uma fonte declarada. Há duas classes:

- **Fontes derivadas:** agenda (`ScheduledOccurrence`), registro de triggers, `RoundDeclaration`,
  sucessores de `DEFER` e qualquer outra entrada escrita por um `CycleCommit` ou pelo genesis. O
  conteúdo dessas fontes para qualquer coordenada é função pura de `base_revision` e dos ledgers
  commitados. Elas são fechadas **por construção**: um `CycleCommit` em `(instant, cycle_ordinal)` só
  pode criar entradas com coordenada `>= (instant, cycle_ordinal + 1)`, logo, quando um ciclo abre
  sobre a revisão resultante, o conjunto de entradas derivadas elegíveis a ele já está fixado;
- **Fontes exógenas:** adapters de harness/driver, genesis-only sources e, no futuro, intervenção de
  usuário por contrato próprio. Elas são declaradas na configuração imutável do run
  (`exogenous_sources[]`, possivelmente vazia); uma fonte não declarada não escreve entradas tipadas
  no input ledger. Cada fonte grava entradas com `ingress_seq` durável e monotônico por `source_id`.

Respostas de slot não pertencem a nenhuma das duas classes como entrada própria: o slot é unidade de
fonte **derivada** — nasce com a `RoundDeclaration`, herda sua coordenada e seu fechamento por
construção — e a resposta é o seu **preenchimento** (§3.2.5). Quem responde — gateway de LLM, adapter
de timeout, operador — é um **respondedor de slot**, não uma `CausalSource`: não grava `SourceClosure`,
não entra em `exogenous_sources[]` por responder e não tem `ingress_seq` com efeito de coordenada. Um
adapter que seja, ao mesmo tempo, fonte exógena declarada e respondedor de slot tem obrigação de
fechamento **apenas** sobre seus inputs exógenos tipados; suas respostas de slot ficam fora da
condição de fechamento abaixo e nunca recebem coordenada `> C` por terem sido gravadas depois do
fechamento da fonte.

Uma fonte exógena declara o fim de sua contribuição a uma coordenada com um marcador durável:

```text
SourceClosure {
  source_id
  closed_through: EligibilityCoordinate     // monotônico por fonte; pode saltar à frente
  final: boolean                            // true = a fonte nunca mais grava
  ingress_seq
}
```

Regras:

- `closed_through` nunca regride. Uma fonte pode fechar coordenadas muito à frente ou declarar-se
  `final`; um run sem fontes exógenas — mesmo com rounds e respondedores de slot — tem a condição
  abaixo trivialmente satisfeita;
- **condição de fechamento:** um ciclo de coordenada `C` só deriva sua membresia quando toda fonte
  exógena declarada tem `closed_through >= C`. Até então o coordinator **aguarda**; ele nunca corta
  por conta própria. Uma fonte parada é condição operacional (telemetria, alerta), não decisão
  semântica;
- **normalização de ingresso:** `open_coordinate(source_id)` é o sucessor
  `(closed_through.instant, closed_through.ordinal + 1)` do último fechamento da fonte, ou a
  coordenada mínima do run quando ainda não houve fechamento. Uma entrada gravada depois do
  `SourceClosure` da própria fonte recebe coordenada `>= open_coordinate` por regra, então nunca cai
  sob um fechamento já declarado. `ingress_seq` participa **apenas** desse antes/depois;
- o coordinator publica, como sinal operacional, a coordenada cujo fechamento aguarda; uma fonte
  pode consultá-lo, mas o que vale é o marcador que ela grava.

Consequência: o momento em que uma fonte exógena fecha uma coordenada é um ato declarado **dela**,
gravado no ledger, e o coordinator só consome fatos do ledger. Se um adapter usa relógio de parede
para decidir quando fechar, esse relógio é ato externo registrado — exatamente o status de uma
resposta de slot (§3.2.5) —, não semântica da simulação. Duas execuções com o mesmo input ledger
(entradas mais fechamentos) têm a mesma membresia em todo ciclo, qualquer que seja a latência de
host, rede, coordinator ou entrega de agente. Duas execuções ao vivo em que a fonte fecha em momentos
diferentes têm **inputs diferentes**, e essa diferença é durável, auditável e visível no digest do
input ledger. Este ADR não torna determinístico o comportamento de uma fonte externa; ele garante
que a simulação nunca acrescenta uma segunda escolha, oculta e dependente de timing, por cima dela.

##### 3.2.3 `RoundDeclaration` e `DecisionCohort`

Um decision round não é aberto pelo orchestrator “quando ele acha que deve”: ele é uma entrada
causal durável, com lifecycle igual ao de uma `ScheduledOccurrence` (§4.1):

```text
RoundDeclaration {
  decision_round_id
  actor_id
  slot_ids[]                          // declarados na criação; imutáveis
  eligibility: EligibilityCoordinate
  created_by: EventRef | GenesisRef
  lifecycle: PENDING | CANCELLED | CONSUMED
  idempotency_key
}
```

Uma `RoundDeclaration` é criada pelo genesis ou por um `CycleCommit` — por exemplo, o commit que
entrega uma observação a um ator e lhe abre a oportunidade de reagir, ou o commit de uma ocorrência
agendada de “oportunidade de decisão”. O commit que a cria escreve sua coordenada explicitamente, com
valor `>= (instant, cycle_ordinal + 1)`, na mesma transação que publica a revisão (§7.1). Criação,
cancelamento e consumo são eventos de lifecycle no `EventBatch`, de modo que o conjunto de rounds é
reconstruível pelo event store (invariante 17). Uma fonte exógena que queira um round grava um input
exógeno tipado; o candidato que o expande cria a declaração no ciclo seguinte. Nenhuma fonte cria
round “diretamente na coorte corrente”.

Um ciclo hospeda exatamente uma **coorte de decisão**:

- a coorte do ciclo de coordenada `C` é o conjunto canônico das `RoundDeclaration` em `PENDING` com
  coordenada `<= C`, calculado sobre `base_revision` depois que a condição de fechamento da §3.2.2 vale.
  É uma **derivação**, não uma observação: rounds não “chegam” à coorte, eles já estão nos ledgers
  quando o ciclo abre;
- como toda `RoundDeclaration` vem de fonte derivada, não existe caminho pelo qual um round elegível a
  `C` passe a existir depois que o ciclo `C` abriu; a dobra sobre os ledgers devolve o mesmo conjunto
  em qualquer execução;
- cada round mantém seus slots declarados e só está completo quando cada slot contém sua única
  resposta gravada — `ActionProposal` ou `NoProposal`. A solicitação ao agente é um `SlotDispatch`
  (§3.2.5), emitido somente depois que a coorte foi derivada e o barrier epistemológico da §10.1
  liberou o destinatário;
- todos os rounds da coorte leem exatamente a mesma `base_revision` e seus candidatos entram no mesmo
  espaço de `ConflictSet`;
- qual round completou primeiro é telemetria. Latência de LLM ou de orchestrator não escolhe qual
  round recebe a revisão mais antiga, porque dentro da coorte nenhum round recebe revisão diferente e
  nenhum round fica de fora por ter sido “visto” mais tarde;
- resposta dirigida a slot já preenchido ou a dispatch revogado — antes ou depois do fence — é
  rejeitada na admissão e auditada; nunca é inserida retroativamente (§3.2.5).

Rounds só se serializam por fato causal declarado: um round criado por evento commitado neste ciclo
tem coordenada `(instant, cycle_ordinal + 1)` ou maior e cai na coorte seguinte, contando para o
limite de cascata.

##### 3.2.4 `AdmissionFence`

A membresia é materializada por um **fence de admissão** durável, gravado **antes** de qualquer
avaliação de affordance, candidato ou conflito:

```text
AdmissionFence {
  run_id
  cycle_id
  attempt_ordinal
  retry_ref?                  // AttemptRetryRecord consumido; obrigatório quando attempt_ordinal > 1
  instant
  cycle_ordinal
  base_revision + base_state_hash
  closure_proof: [{ source_id, closure_ref, closed_through }]   // toda fonte exógena declarada
  cohort: { cohort_id, rounds: [{ decision_round_id, slot_ids[], response_refs[] }] }
  admitted_input_ids[]        // unidades admitidas, em ordem canônica
  fence_policy_version + rule_versions
  fence_digest
}
```

Uma **unidade admitida** é um slot preenchido — identificado pela sua única `SlotResponse`
(`ActionProposal` ou `NoProposal`) —, um input exógeno, uma `ScheduledOccurrence` vencida ou uma
ativação de trigger elegível. As `RoundDeclaration` da coorte são consumidas como contêineres e
listadas em `cohort`; a unidade é o slot. Uma unidade é admitida quando, e somente quando:

1. a condição de fechamento da sua fonte vale para a coordenada do ciclo — por construção, para
   fontes derivadas, inclusive os slots (§3.2.5); por `SourceClosure` gravado, para fontes exógenas;
2. sua coordenada de elegibilidade é menor ou igual à coordenada do ciclo — para um slot, a
   coordenada herdada da `RoundDeclaration`;
3. ela ainda não foi consumida por um `CycleCommit` anterior;
4. sendo slot, ele contém sua única resposta gravada (§3.2.5).

O fence é uma **função**: `derive(ledgers, base_revision, C, versions)`. O coordinator não escolhe
um high-water; ele registra em `closure_proof` os fechamentos dos quais a derivação dependeu. O fence
persistido precisa ser igual à derivação; replay e resume recomputam a derivação e verificam
`fence_digest`, e uma divergência falha fechado como corrupção ou deriva de versão (§11).

Consequências:

- `ingress_seq` participa **apenas** da normalização de ingresso da §3.2.2. Ele nunca ordena,
  prioriza nem desempata. A ordem canônica de `admitted_input_ids` é derivada de conteúdo:
  `(not_before_instant, not_before_cycle_ordinal, source_rank, payload_digest, input_id)`;
- inverter a ordem de entrega de duas entradas gravadas antes do mesmo `SourceClosure` não muda o
  conjunto admitido nem o `fence_digest`: ambas ficam sob o fechamento e a ordenação não olha para
  `ingress_seq`;
- gravar o fence cedo ou tarde — host rápido ou lento, coordinator reiniciado, rede congestionada —
  não muda o fence: os fatos dos quais ele depende já estão nos ledgers e não se alteram entre a
  derivação e a gravação. O que muda é telemetria;
- entrada gravada depois do fechamento da própria fonte nunca é inserida retroativamente. Ela
  permanece no ledger com sua coordenada normalizada e é admitida pelo primeiro fence posterior que
  a torne elegível;
- ocorrências vencidas e ativações de trigger entram pelo mesmo fence, por elegibilidade contra
  `base_revision`, e não por ordem de chegada.

Um run tem no máximo um ciclo aberto por vez e um ciclo tem no máximo um fence vigente. O fence é
gravado assim que duas condições, ambas fatos de ledger, valem: (a) a condição de fechamento da
§3.2.2 para a coordenada do ciclo; (b) todo slot da coorte contém sua única resposta gravada. Nenhuma
das duas depende de tempo de parede do coordinator para decidir **o que** entra — (a) é marcador de
fonte, (b) é preenchimento de slot admitido pelo ledger (§3.2.5); ambos são atos externos duráveis,
nunca observação do coordinator. Tempo de parede afeta somente **quando** as condições passam a
valer e, portanto, quando o fence é gravado. Depois do fence, nada no ciclo depende de tempo de
parede: `base_revision`, `admitted_input_ids`, `ConflictSet`, vencedor, event log e digests são
função exclusiva do fence gravado e das versões declaradas.

O fence de admissão decide quando um ciclo **fecha a entrada**; o barrier epistemológico da §10.1
decide quando a solicitação de um round pode ser **despachada**. São gates complementares e não se
substituem.

Se a tentativa aborta (§8.0.1), o `CycleAbortRecord` encerra aquele fence e o run fica parado
(`HALTED_ON_ABORT`, §8.0.2). Uma tentativa reparada, autorizada por `AttemptRetryRecord` explícito
(`RETRY_AUTHORIZED`), grava um novo fence com `attempt_ordinal + 1` e `retry_ref` apontando para o
registro que a autorizou. Como o abort não consome nada, as fontes já estavam fechadas para a
coordenada e cada slot já contém sua única resposta, a membresia e as `response_refs` do novo fence
são **idênticas por construção** e nenhum slot é redespachado; mudam apenas `attempt_ordinal`,
`retry_ref` e `rule_versions`.

Eventos commitados no mesmo ciclo recebem `LogicalSequence` em ordem canônica apenas para
serialização e replay. Essa sequência não significa que um vencedor leu o resultado do evento
anterior e nunca cria link causal entre candidatos distintos (§8.1). Prioridade de disputa precisa
vir de uma política de domínio explícita e versionada, nunca do sequence, da ordem de iteração ou da
latência.

Pode haver mais de um ciclo no mesmo instante: por exemplo, um commit entrega uma mensagem e a
entrega ativa um trigger imediato. Esses ciclos têm ordinais crescentes e revisões diferentes. Uma
configuração versionada limita a cascata de ciclos sem avanço temporal; excedê-la aborta o run com
erro determinístico, em vez de truncar silenciosamente.

##### 3.2.5 Resposta de slot e `SlotDispatch`

Um slot `(decision_round_id, slot_id)` nasce com a `RoundDeclaration` e herda dela fonte, coordenada
e fechamento por construção. O que lhe falta é o **preenchimento**: exatamente uma resposta gravada.
O input ledger governa esse preenchimento com um registro de slot próprio, separado das entradas de
fonte exógena:

```text
SlotDispatch {
  dispatch_id                 // determinístico: (run_id, cycle_id, decision_round_id, slot_id,
                              //                  dispatch_ordinal); dispatch_ordinal conta os
                              //                  dispatches anteriores do slot, todos revogados
  run_id + cycle_id
  decision_round_id + slot_id
  base_revision + eligibility  // herdadas da RoundDeclaration
  request_digest
}

SlotDispatchRevocation {
  dispatch_id
  reason: RESUME_REISSUE | LIVENESS_POLICY | OPERATOR
  policy_version?
}

SlotResponse := ActionProposal | NoProposal   // ambos carregam dispatch_id obrigatório
```

Regras, todas verificadas pelo `InputLedger` no momento da admissão, em escrita condicional única:

1. **um dispatch aberto por slot:** um `SlotDispatch` só é gravado para slot da coorte derivada,
   vazio e sem outro dispatch aberto. Um dispatch está **aberto** enquanto o slot está vazio e ele
   não foi revogado; o preenchimento do slot o encerra;
2. **uma resposta por slot:** o ledger aceita uma `SlotResponse` somente se `dispatch_id` aponta
   para o dispatch aberto do slot **e** o slot está vazio. A escrita é condicional e atômica: ou
   preenche o slot, ou é rejeitada. Reentrega bit a bit idêntica da resposta já gravada é
   idempotente; qualquer outra escrita para o mesmo slot — segunda resposta, resposta atrasada,
   resposta a dispatch revogado — é rejeitada e vai para a auditoria de respostas rejeitadas, fora do
   digest causal. A chave de idempotência de uma resposta é o slot, não o payload;
3. **redespacho só após revogação durável:** emitir um novo dispatch para o mesmo slot exige antes
   um `SlotDispatchRevocation` do anterior. A partir da revogação, a resposta ao dispatch antigo não
   pode mais preencher o slot, mesmo que chegue antes da resposta ao novo. Revogar é ato registrado
   — de resume, de política de liveness versionada ou de operador —, nunca efeito de timing
   observado pelo coordinator;
4. **timeout é preenchimento, não concorrência:** o `NoProposal` de timeout é uma `SlotResponse`
   comum, gravada contra o dispatch aberto. Ela e a resposta real do mesmo dispatch disputam o slot
   apenas na escrita condicional da regra 2, que admite exatamente uma delas;
5. **sem coordenada, sem fechamento:** a resposta não recebe `EligibilityCoordinate` própria, não
   passa pela normalização de ingresso da §3.2.2 e não está sujeita a `SourceClosure`. O
   `ingress_seq` que o ledger lhe atribui é auditoria; nenhuma regra o consulta.

Consequências:

- `cohort.rounds[].response_refs[]` é função do ledger: cada slot tem zero ou uma resposta, e a
  derivação nunca escolhe entre respostas porque o ledger nunca contém duas. Nenhum desempate por
  `ingress_seq` ou por conteúdo é necessário ou permitido (invariante 21);
- qual escrita venceu a condicional da regra 2 — resposta real ou `NoProposal` de timeout — é um fato
  externo durável com o mesmo status de um `SourceClosure`: duas execuções ao vivo em que
  respondedores diferentes vencem têm **inputs diferentes**, visíveis no digest do input ledger; duas
  execuções com o mesmo input ledger têm o mesmo fence. Timing decide **qual input existe**; nunca
  decide prioridade causal de um input existente frente a outro, e nunca é consultado por
  `derive()`;
- **crash após o dispatch e antes da resposta:** o resume encontra dispatch aberto e slot vazio. Ele
  aguarda a resposta ou grava `SlotDispatchRevocation` e redespacha; nunca emite um segundo dispatch
  com o primeiro aberto. Se a chamada original ainda estiver em voo, sua resposta preenche o slot
  (dispatch ainda aberto) ou é rejeitada (dispatch revogado); em nenhum caso duas respostas
  coexistem, e em nenhum caso duas chamadas emitidas pela simulação disputam o mesmo slot;
- um run sem `exogenous_sources[]` continua tendo dispatches e respostas de slot, e continua sem
  `SourceClosure`: o ônus do respondedor é preencher cada dispatch aberto, não fechar coordenadas.

#### 3.3 Avanço

O engine avança para o menor `not_before_instant` entre as coordenadas de elegibilidade ainda não
consumidas: ocorrências agendadas, `RoundDeclaration` pendentes, próxima cadência de trigger
repetível, sucessores de `DEFER` e inputs exógenos já gravados no input ledger. O avanço também
respeita a condição de fechamento: nenhum ciclo abre em coordenada que alguma fonte exógena declarada
ainda não fechou (§3.2.2). Quando o destino é posterior ao instante corrente, um
ciclo de avanço produz `clock.advanced { from, to }` e os eventos de materialização temporal exigidos
pelos substates afetados, como `body.advanced`, no mesmo commit atômico. Só então abre o ciclo das
entradas devidas no novo instante. Não existe recuperação, expiração ou juros implícitos fora do log
causal.

Agendar no passado é erro: coordenada de elegibilidade menor que a do ciclo corrente é rejeitada.
Agendar no instante corrente exige `(current_instant, cycle_ordinal + n)` com `n >= 1` e conta para o
limite de cascata. Deadlines usam intervalos declarados (`<`, `<=`) e não dependem de um
evento “prazo expirou” ter sido serializado antes de uma ação no mesmo instante.

### 4. Agenda e triggers

#### 4.1 `ScheduledOccurrence`

É uma entidade durável, não um `Event` futuro:

```text
ScheduledOccurrence {
  occurrence_id
  due_at
  eligibility: EligibilityCoordinate   // (due_at, 0), salvo coordenada explícita de DEFER
  kind + schema_version
  payload
  lifecycle: PENDING | CANCELLED | CONSUMED
  created_by: EventRef | InputRef | GenesisRef
  recurrence?: RecurrencePolicy
  idempotency_key
}
```

Transições são monotônicas. Uma ocorrência cancelada ou consumida nunca volta a `PENDING`; reagendar
cria outra identidade com provenance para a anterior. No vencimento, o handler puro devolve um
`CommitCandidate` determinístico ou falha de domínio. Só depois de commit bem-sucedido a ocorrência é
consumida. Retentativa técnica não duplica efeito por causa da identidade/idempotency key. Uma
ocorrência vencida nunca permanece `PENDING` no instante em que venceu: qualquer disposição do ciclo
a liquida conforme a §7. A expansão usa a coordenada de elegibilidade, não `due_at <= instant`
isolado: duas ocorrências do mesmo instante podem pertencer a ciclos distintos quando suas
coordenadas declaram ordinais distintos.

Recorrência não “ressuscita” a mesma ocorrência: o consumo bem-sucedido agenda a próxima instância
com nova identidade e link causal. Criação, cancelamento e consumo são representados por eventos de
lifecycle no mesmo `EventBatch` que os causa; a fila é reconstruível pelo event store, não somente por
um snapshot da memória do scheduler. `RoundDeclaration` (§3.2.3) segue exatamente a mesma disciplina
de lifecycle e é mantida pelo mesmo `ScheduleStore`: um round consumido nunca volta a `PENDING`, e
a coorte de qualquer ciclo é reconstruível pelo event store.

#### 4.2 `TriggerDefinition` e estado de runtime

O predicate de um trigger é puro sobre `(WorldState@revision, SimulationInstant)` e possui versão. Ele
é avaliado no bootstrap e depois de cada commit que intersecte suas dependências declaradas; uma
implementação pode avaliar mais predicates, mas não pode alterar o resultado.

Toda definição escolhe exatamente uma política:

- `RISING_EDGE`: ativa em `false → true`; o valor inicial verdadeiro só ativa se
  `fire_on_initial_true=true`;
- `FALLING_EDGE`: ativa em `true → false`;
- `ONCE_WHEN_TRUE`: ativa na primeira avaliação verdadeira e fica exaurido;
- `REPEAT_WHILE_TRUE`: quando verdadeiro, cria uma ocorrência na cadência positiva explícita; nunca
  ativa novamente apenas porque houve outra avaliação.

`TriggerRuntimeState` persiste `last_value`, `armed/exhausted`, `activation_count` e, quando aplicável,
`next_repeat_at`. O decision ledger registra quais revisões foram avaliadas; mudanças no runtime são
eventos de lifecycle, de modo que edge/rearm/repeat também sejam replayable. Re-arm manual é uma
transição causal explícita. `repeat_every > 0`; repetição de intervalo zero é inválida.

`REPEAT_WHILE_TRUE` ativa uma vez na entrada em verdadeiro e agenda a próxima ativação para
`current_instant + repeat_every`. Se a condição ficar falsa antes disso, a ocorrência pendente é
cancelada por evento de lifecycle. No vencimento o predicate é revalidado contra o snapshot do ciclo;
falso consome/cancela sem produzir o efeito de domínio. Se condição e vencimento mudarem no mesmo
ciclo, ambos leem o mesmo snapshot e o resultado pertence à semântica simultânea daquele lote.

Uma ativação produz um `CommitCandidate` diretamente ou agenda uma ocorrência. Ela não cria uma
`ActionProposal` fictícia. Assim, início de aula, prazo expirado, pagamento periódico e entrega
atrasada permanecem fatos determinísticos sem ator artificial.

### 5. `ActionProposal`

É um input imutável e neutro de domínio:

```text
ActionProposal {
  proposal_id
  action_type + schema_version
  actor_id
  targets: [{ role, entity_id }]
  parameters
  intended_resources: [ResourceClaim]
  location_ref
  effective_at
  expected_duration?
  decision_round_id + slot_id
  submitted_against_revision
  originating_intention_ref?
  idempotency_key
}
```

Alvos têm papéis explícitos; listas e maps usam normalização canônica. Parâmetros são validados pelo
schema versionado do `action_type`. `originating_intention_ref` é opaco para a fundação e nunca prova
motivação. A proposta não contém desfecho narrado, sucesso requerido ou mutação arbitrária.

O input ledger registra proposta e origem; o decision ledger registra sua disposição. `NoProposal`
é a resposta gravada de um slot que não produziu proposta — por decisão do ator ou por timeout
operacional — e ocupa o slot com a mesma finalidade: fechar o round e receber um desfecho terminal
próprio (`NO_PROPOSAL`, §7.1). A proposta **não** entra no event store como world truth. Se o ator
chegou a executar um movimento perceptível, o resolvedor produz um evento de tentativa; se apenas
sugeriu algo impossível ao engine, há rejeição auditável no decision ledger e nenhum fato físico
inventado.

### 6. Affordance e regras

Cada proposta produz um `AffordanceAssessment` facetado:

```text
FacetResult {
  facet: PHYSICAL | ACCESS | RESOURCE | TEMPORAL | INSTITUTIONAL | LEGAL
  status: SATISFIED | UNSATISFIED | NOT_APPLICABLE | INDETERMINATE
  policy_effect: BLOCKS_EXECUTION | RECORDS_VIOLATION | INFORMATIONAL
  reason_code
  evidence_refs[]
  rule_version
}
```

`INDETERMINATE` é status de primeira classe da faceta, não um resultado silenciosamente aceito ou
negado: significa que regra, dado ou resolvedor necessário está ausente. Ele é gravado com `facet`,
`reason_code`, `evidence_refs` e `rule_version` do validador que faltou dentro do `CycleAbortRecord`
que encerra a tentativa (§8.0.1), e a tentativa aborta antes que qualquer disposição exista —
independentemente do `policy_effect` declarado, que descreve regra existente e não supre regra
ausente. A tentativa abortada não persiste `CycleCommit`, `DecisionRecord` nem revisão (§8.0); o
envelope de abort torna o comportamento fail-closed auditável sem inventar estado nem liquidar
fonte alguma. Isso impede que lacunas de implementação virem realidade do mundo por default.

O efeito da faceta é definido pelo tipo de ação/regra:

- impossibilidade física, falta de acesso efetivo, falta de recurso consumível ou janela temporal
  fechada normalmente bloqueiam;
- proibição institucional ou ilegalidade normalmente registram violação, mas não bloqueiam uma ação
  fisicamente possível;
- uma regra só bloqueia quando existe mecanismo causal de enforcement — por exemplo, porta trancada,
  autenticação negada ou contenção física. O texto da proibição sozinho não exerce força física.

Conflito entre propostas não é uma faceta individual conhecida antes do lote. Ele é produzido em uma
segunda passagem como `ConflictSet`, com ids das propostas/candidatos, recurso ou invariante disputado
e política resolvedora.

Quando uma ação proibida é executada, o mesmo candidato inclui os eventos factuais da ação e um fato
institucional tipado, como `institution.rule_violated`, referenciando regra e versão. Tornar qualquer
desses fatos observável ainda é responsabilidade da percepção. Detecção por uma autoridade, denúncia
e punição são cadeias causais posteriores; a infração não pune a si mesma.

### 7. Resolução e commit atômico

O pipeline de um ciclo é:

```text
freeze base revision; await the closure condition for the cycle coordinate
  → derive the DecisionCohort and the admitted units from base_revision + ledgers
  → open one SlotDispatch per slot with no response and no open dispatch (after the §10.1 barrier);
      admit the unique response per slot (§3.2.5)
  → record the AdmissionFence durably (closure proof + cohort + canonical admitted_input_ids)
  → deduplicate admitted units (canonical survivor; duplicates settle as DEDUPLICATED)
  → expand occurrences, trigger activations and exogenous inputs into CommitCandidates
  → validate proposals against the frozen state
  → build CommitCandidates
  → detect ConflictSets from declared reads/writes/resources/invariants
  → resolve with explicit versioned policies and named randomness
  → validate the complete EventBatch and post-state
  → atomically append exactly one terminal DecisionRecord per admitted unit
      + events + CycleCommit + new WorldRevision
  → on any failure: atomically append a CycleAbortRecord;
      no DecisionRecord, no events, no revision, no ordinal, no consumption
```

`ScheduledOccurrence`/`Trigger` e `ActionProposal` são dois ramos que convergem em
`CommitCandidate`. Nenhum ramo tem permissão especial para mutar o mundo antes do outro. Um candidato
declara:

- unidade atômica e eventos propostos;
- recursos consumidos/reservados e chaves de conflito;
- read/write set conservador ou invariantes tocadas;
- provenance de ocorrência, trigger ou proposta;
- prioridade de domínio explícita, se houver;
- política de empate e versão do resolvedor.

Dois candidatos sem conflito podem ambos vencer. Em conflito, o resolvedor produz uma disposição
determinística (`COMMIT`, `REJECT`, `DEFER`) para cada candidato. Empate estocástico só usa substream
nomeado. Unidades admitidas que não geram candidato — slot com `NoProposal` e entrada removida pela
deduplicação — recebem desfecho terminal próprio (`NO_PROPOSAL`, `DEDUPLICATED`, §7.1) na mesma
transação; nenhuma unidade admitida sai do ciclo sem desfecho.

Toda disposição liquida a fonte do candidato no mesmo commit atômico. Nenhuma delas pode deixar a
fonte pendente no instante já avaliado:

- `COMMIT`: os eventos do candidato entram no lote; a ocorrência vai a `CONSUMED`, o
  `TriggerRuntimeState` registra a ativação e a proposta recebe disposição no decision ledger — as
  transições de agenda/trigger como eventos de lifecycle no mesmo `EventBatch`;
- `REJECT`: nenhum evento de domínio do candidato entra no lote, mas os eventos de lifecycle da fonte
  entram. A ocorrência vencida vai a estado terminal — `CONSUMED` quando foi avaliada e descartada,
  `CANCELLED` quando a política a retira —, o trigger registra a avaliação e o rearm previsto pela
  sua própria política, e a rejeição fica no decision ledger com `reason_code`. Assim nenhuma fonte
  continua `PENDING` no instante corrente e nenhum trigger redispara a partir de estado velho;
- `DEFER`: a fonte antiga também vai a estado terminal e o mesmo commit cria a nova ocorrência ou o
  novo input causal, com identidade nova, provenance para a fonte anterior e **coordenada de
  elegibilidade explícita**, estritamente maior que a do ciclo corrente — `(instante_futuro, 0)` para
  adiamento temporal, `(current_instant, cycle_ordinal + 1)` para adiamento de mesmo instante,
  contando para o limite de cascata. A coordenada é gravada na mesma transação que o `CycleCommit`,
  então resume e replay readmitem o sucessor exatamente no mesmo ciclo. `DEFER` nunca reabre a
  identidade antiga, nunca depende do momento de inserção e nunca deixa trabalho fantasma na memória
  do processo.

#### 7.1 Desfechos terminais e unidade atômica

O desfecho de cada unidade admitida é um registro canônico, não um efeito colateral do resolvedor:

```text
DecisionRecord {
  decision_id
  run_id + cycle_id + attempt_ordinal
  subject: ProposalRef | NoProposalRef | OccurrenceRef | TriggerActivationRef | InputRef
  disposition: COMMIT | REJECT | DEFER | NO_PROPOSAL | DEDUPLICATED
  reason_code
  conflict_set_refs[]         // vazio fora de COMMIT | REJECT | DEFER
  policy_version + resolver_version + rng_draw_refs[]
  produced_event_ids[]        // pode ser vazio
  successor_input_id?         // obrigatório em DEFER
  canonical_unit_id?          // obrigatório em DEDUPLICATED
  record_digest
}
```

Vale uma **bijeção**: para cada id em `admitted_input_ids` existe exatamente um `DecisionRecord` cujo
`subject` o referencia, e nenhum `DecisionRecord` referencia unidade fora do fence. `CycleCommit`
lista `decision_record_ids[]` na mesma ordem canônica de `admitted_input_ids`, e `decision_digest`
cobre os pares `(unidade, desfecho)`. Assim o recibo de settlement prova a cobertura total, e
auditoria/replay distinguem “slot sem proposta” e “duplicata descartada” de “fonte omitida por bug”:

- `COMMIT`, `REJECT`, `DEFER`: disposições do resolvedor sobre candidatos, com a liquidação da fonte
  descrita na §7;
- `NO_PROPOSAL`: o slot foi respondido com `NoProposal`; nenhum candidato existiu, nenhum evento de
  domínio é produzido, o slot é consumido e o round vai a `CONSUMED` por evento de lifecycle quando
  todos os seus slots estão liquidados;
- `DEDUPLICATED`: a unidade compartilha `idempotency_key` com outra unidade do mesmo fence; a
  sobrevivente canônica é a primeira na ordem canônica de conteúdo, e o registro da duplicata aponta
  para ela em `canonical_unit_id`. A duplicata é consumida sem candidato e sem evento; a disposição
  de fato está no registro da sobrevivente.

`DecisionRecord` existe **exclusivamente** dentro da transação de um `CycleCommit`. Nenhuma outra
escrita — envelope de abort, material provisório, telemetria — pode criar um. O decision ledger
continua recebendo material **provisório** durante a avaliação: facets, `ConflictSet` calculados,
sorteios, disposições ainda não commitadas. Esse material é auditoria de processo e não liquida nada.
Replay e resume ignoram qualquer registro provisório que não esteja coberto pelo `decision_digest` de
um `CycleCommit`.

O commit é uma transação única sobre `base_revision` e cobre, indivisivelmente:

1. o fechamento do fence — as unidades de `admitted_input_ids` passam a consumidas;
2. exatamente um `DecisionRecord` terminal por unidade admitida, vencedora ou não;
3. todos os eventos dos candidatos vencedores, incluindo os eventos de lifecycle de agenda, round e
   trigger;
4. o `CycleCommit`, que referencia `fence_digest`, `decision_record_ids[]` e `decision_digest`;
5. a publicação da nova `WorldRevision`.

Ou as cinco partes persistem, ou nenhuma persiste. Não existe outbox de decisão que possa ficar para
trás nem adiantar-se ao mundo: settlement não é um passo posterior ao commit, é parte dele. Isso
impõe um requisito de persistência, não uma escolha de banco: event store e decision ledger precisam
compartilhar um domínio transacional, ou ser unidos por um commit determinístico chaveado por
`fence_digest` que nenhum ciclo posterior, replay ou resume possa ultrapassar sem reconciliar. Este
ADR não escolhe a tecnologia, mas exclui topologias em que essas duas escritas possam divergir. Antes de
publicar, reducers são aplicados a uma cópia de trabalho e todas as invariantes afetadas são
verificadas. Falha em evento, reducer, schema, provenance ou invariante aborta o lote inteiro e leva
ao envelope de abort da §8.0.1. Não há commit parcial de uma ação multi-evento, event log adiantado
em relação ao state store, nem disposição gravada sem o mundo correspondente.

### 8. Eventos e ledgers

#### 8.0 Envelope de commit do ciclo

Todo ciclo fechado com sucesso persiste no event store um envelope do seu lote, **inclusive quando o
lote tem zero eventos**:

```text
CycleCommit {
  run_id
  cycle_id
  attempt_ordinal
  instant
  cycle_ordinal
  base_revision + base_state_hash
  result_revision + result_state_hash
  fence_digest
  admitted_input_ids[]
  event_ids[]                // pode ser vazio
  decision_record_ids[]      // exatamente um por unidade admitida, na mesma ordem canônica
  decision_digest
  next_logical_sequence
  batch_digest
}
```

Um ciclo em que todos os slots produziram `NoProposal`, ou em que todo candidato foi rejeitado sem
gerar evento de lifecycle, ainda avança `WorldRevision` e `cycle_ordinal`. O envelope é o registro que
torna esse avanço reconstruível: o replay percorre a sequência de `CycleCommit` e reaplica os eventos
de cada um, reproduzindo revisão vazia, ordinal e `next_logical_sequence` sem precisar de evento
decorativo nem de um snapshot do coordinator. O envelope é também o recibo de settlement:
`fence_digest` prova qual corte foi avaliado e `decision_record_ids[]`/`decision_digest` provam a
bijeção da §7.1 — toda unidade admitida por aquele corte, inclusive slots `NoProposal` e duplicatas,
recebeu um desfecho terminal na mesma transação. Tentativa abortada não produz `CycleCommit`, não
publica revisão e não avança o ordinal do instante; ela produz o envelope da §8.0.1.

##### 8.0.1 Envelope de abort do ciclo

Uma tentativa abortada não pode desaparecer. Sem registro durável, `INDETERMINATE` viraria log
operacional e o resume não conseguiria distinguir “abortou de forma auditável” de “caiu no meio”.
Por isso o abort é persistido no decision ledger como envelope próprio, em transação única:

```text
CycleAbortRecord {
  run_id
  cycle_id
  attempt_ordinal
  instant
  cycle_ordinal
  base_revision + base_state_hash
  fence_digest
  abort_reason: INDETERMINATE_FACET | INVARIANT_VIOLATION | REDUCER_FAILURE
              | SCHEMA_FAILURE | PROVENANCE_FAILURE | CASCADE_LIMIT
  indeterminate_records[]    // facet, reason_code, evidence_refs, rule_version
  failure_evidence_refs[]    // material provisório da tentativa: facets, ConflictSets, sorteios,
                             // disposições provisórias, erro de reducer/invariante
  rule_versions
  abort_digest
}
```

O envelope de abort cobre **somente evidência de tentativa**. Ele **não** publica `WorldRevision`,
não acrescenta evento ao event store, não avança `cycle_ordinal`, não consome unidade admitida e
**não persiste nem referencia `DecisionRecord`**. Disposições que o resolvedor já havia calculado
para outros candidatos da mesma tentativa — um `COMMIT` provisório de A quando B encontrou
`INDETERMINATE` — são material provisório: podem ser apontadas por `failure_evidence_refs` para
auditoria, mas não são `DecisionRecord`, não liquidam fonte alguma e não apontam para eventos, porque
nenhum evento existe. A fonte de A continua elegível e será reavaliada na próxima tentativa junto
com B. O abort é autoritativo para exatamente um fato — a tentativa `attempt_ordinal` do ciclo
abortou por `abort_reason` com esta evidência — e para nada mais.

Ele encerra a tentativa, preserva a auditoria fail-closed exigida pela §6 e leva o run ao estado
`HALTED_ON_ABORT` (§8.0.2), onde permanece até uma decisão explícita.

Vale então a regra terminal do ciclo: para todo fence gravado existe **exatamente um** envelope
terminal — `CycleCommit` ou `CycleAbortRecord` —, nunca ambos e nunca nenhum em estado estável. No
resume, um fence sem envelope terminal significa crash no meio da tentativa; como nenhuma das cinco
partes da transação da §7.1 pode persistir isoladamente, o coordinator reexecuta a tentativa a partir
do fence gravado e das respostas externas já registradas e obtém o mesmo resultado. Não existe estado
em que o mundo avançou e a decisão não, nem o inverso.

##### 8.0.2 Estado de controle do ciclo e retentativa explícita

O coordinator tem um estado de controle **derivado**, obtido por dobra determinística sobre o fence
log, os envelopes terminais e os registros de retentativa:

```text
CycleControlState {
  status: IDLE | ATTEMPT_IN_FLIGHT | RETRY_AUTHORIZED | HALTED_ON_ABORT
  cycle_id?                    // ausente somente em IDLE
  attempt_ordinal?             // tentativa em voo, autorizada ou abortada
  fence_ref?                   // somente ATTEMPT_IN_FLIGHT: fence sem envelope terminal
  retry_ref?                   // somente RETRY_AUTHORIZED: AttemptRetryRecord ainda sem fence
  last_terminal_envelope_ref?  // envelope terminal mais recente do run; ausente no genesis
  next_attempt_ordinal         // 1 + maior attempt_ordinal entre os fences de cycle_id; 1 sem fence
}
```

A dobra é uma função **total** dos ledgers, definida por regras **ordenadas**; a primeira regra cuja
condição vale decide, e nenhuma configuração de ledgers escapa às quatro:

1. existe fence sem envelope terminal ⇒ `ATTEMPT_IN_FLIGHT` com `cycle_id`, `attempt_ordinal` e
   `fence_ref` desse fence. Por construção há no máximo um: um run tem um ciclo aberto por vez, e
   nenhum fence é gravado enquanto o anterior não tiver envelope. Esta regra prevalece mesmo quando o
   último envelope do run é o `CycleAbortRecord` da tentativa anterior do mesmo ciclo — esse abort já
   foi consumido pelo `AttemptRetryRecord` que o fence referencia em `retry_ref`;
2. senão, o último envelope terminal do run é um `CycleAbortRecord` `A` **e** existe
   `AttemptRetryRecord` com `aborted_envelope_ref = A` ⇒ `RETRY_AUTHORIZED` com o mesmo `cycle_id`,
   `attempt_ordinal = to_attempt_ordinal` e `retry_ref`;
3. senão, o último envelope terminal do run é um `CycleAbortRecord` ⇒ `HALTED_ON_ABORT` com o
   `cycle_id` e o `attempt_ordinal` abortados;
4. senão — o último envelope é um `CycleCommit`, ou o run está no genesis — ⇒ `IDLE`.

`next_attempt_ordinal` é definido em todos os estados pela mesma expressão — `1 +` o maior
`attempt_ordinal` entre os fences gravados para `cycle_id`, ou `1` quando não há fence —, logo não
muda ao gravar um `AttemptRetryRecord` (continua `N + 1` após o abort de `N`) e só avança quando o
fence de `N + 1` existe. Transições admissíveis por estado:

- `IDLE`: o próximo ciclo pode abrir conforme §3.3, com `attempt_ordinal = 1`;
- `ATTEMPT_IN_FLIGHT`: a única transição é reexecutar a tentativa a partir do fence e das respostas
  gravadas até um envelope terminal;
- `RETRY_AUTHORIZED`: a única transição é gravar o fence de `(cycle_id, to_attempt_ordinal)`, com
  `retry_ref` apontando para o registro. Esse estado **não** passa pela seleção de coordenada da
  §3.3, não avança relógio e não abre ciclo novo: reabre o mesmo `cycle_id` na tentativa autorizada.
  Um segundo `AttemptRetryRecord` para o mesmo abort é rejeitado na escrita;
- `HALTED_ON_ABORT`: o run está parado. Não abre ciclo novo, não avança relógio, não retenta
  implicitamente.

Sair de `HALTED_ON_ABORT` exige um ato explícito e durável, gravado no decision ledger **antes** do
novo fence:

```text
AttemptRetryRecord {
  run_id + cycle_id
  from_attempt_ordinal → to_attempt_ordinal      // from = abortada; to = next_attempt_ordinal
  aborted_envelope_ref                           // obrigatoriamente o último envelope terminal do run
  reason
  rule_versions                                  // conjunto de versões da nova tentativa
}
```

O registro é válido apenas se `aborted_envelope_ref` for o último envelope terminal do run e um
`CycleAbortRecord`, se `to_attempt_ordinal` for igual a `next_attempt_ordinal` e se nenhum outro
`AttemptRetryRecord` referenciar o mesmo abort; a escrita falha fechado fora dessas condições. Ele é
**consumido** pelo fence que o referencia em `retry_ref`: depois desse fence, a regra 1 rege a
tentativa e, se ela também abortar, o novo abort não tem retentativa e a regra 3 devolve
`HALTED_ON_ABORT` novamente.

A alternativa é fork de run (§12). Nenhum resume, watchdog ou política de liveness pode emitir um
`AttemptRetryRecord`: retentativa é decisão de operador ou de política de domínio explicitamente
versionada e registrada como tal. O resume que encontra `RETRY_AUTHORIZED` apenas completa a
transição já autorizada — grava o fence de `to_attempt_ordinal` — sem emitir um segundo registro. A
nova tentativa grava fence com `to_attempt_ordinal` e membresia idêntica (§3.2.4); `attempt_ordinal`
é contínuo e auditável por ciclo.

#### 8.1 Envelope de `Event`

```text
Event {
  event_id
  event_type + schema_version
  occurred_at: SimulationInstant
  logical_sequence: LogicalSequence
  cycle_id
  base_revision
  payload
  causal_parents: [EventRef]
  source_inputs: [InputRef]
  actor_refs[]
  entity_refs[]
  location_ref?
  confidentiality: PUBLIC | RESTRICTED | SECRET
  producer + producer_version
}
```

`event_id` é derivado deterministicamente do run/ciclo/candidato/posição canônica, ou usa outro
esquema de identidade determinístico equivalente. Pais causais precisam existir e formar DAG, e só
podem estar em um ciclo anterior — ou no mesmo ciclo quando pai e filho vêm do **mesmo
`CommitCandidate` atômico**, com o pai em posição canônica anterior dentro daquele candidato. Link
causal entre candidatos distintos do mesmo ciclo é proibido: eles leram a mesma `base_revision` e
nenhum pôde observar o resultado do outro, portanto `LogicalSequence` menor não é causa. Reação ao
evento de outro candidato pertence a um `ResolutionCycle` posterior, como já exige a §9. Provenance
sem evento pai usa `InputRef` ou `GenesisRef`; não se inventa evento decorativo só para preencher a
cadeia.

Eventos são imutáveis, factuais e no passado perfeito do mundo: `door.opened`, `message.sent`,
`resource.transferred`. Intenção, manipulação, confiança e interpretação não entram como fato físico.
Uma inferência futura pode referenciar os eventos que a sustentam, mas não reescrevê-los.

`confidentiality` é política de acesso ao registro, não prova de conhecimento e não é lista de atores
automaticamente informados. Um evento secreto só alcança um agente por `Observation` causal. Um ato
secreto pode deixar pistas perceptíveis; um evento público ainda precisa de uma rota de publicação ou
percepção, salvo quando o contrato do canal público produz essas observações explicitamente.

#### 8.2 Quatro artefatos, quatro responsabilidades

- **Input ledger:** dois registros com admissão distinta. (i) Entradas de fontes exógenas
  declaradas — inputs tipados e `SourceClosure` —, sujeitas a `ingress_seq`, normalização de ingresso
  e condição de fechamento. (ii) O registro de slot — `SlotDispatch`, `SlotDispatchRevocation` e a
  única `SlotResponse` por slot (`ActionProposal`, `NoProposal`) —, admitido por escrita condicional
  contra o dispatch aberto, sem coordenada própria e sem fechamento (§3.2.5). A auditoria de
  respostas rejeitadas fica fora do digest causal. Uma futura intervenção de usuário só poderá entrar
  aqui por contrato próprio, nunca pelo Observatory;
- **Decision ledger:** `AdmissionFence`, material provisório de avaliação (assessments, conflitos,
  sorteios, disposições provisórias), os envelopes terminais — `CycleCommit` com seus
  `DecisionRecord` e `CycleAbortRecord` — e `AttemptRetryRecord`. `DecisionRecord` é autoritativo
  somente quando coberto pelo `decision_digest` de um `CycleCommit`; `CycleAbortRecord` é autoritativo
  para o fato do abort e sua evidência, nunca para settlement;
- **Event store:** somente fatos do mundo commitados, fonte autoritativa para replay de estado.
- **Evidence ledger:** observations e recibos `KnowledgeInput`, sempre particionados por destinatário;
  é a trilha epistemológica, não world truth nem belief inferida.

Telemetria operacional (wall time, custo, host, duração de LLM) fica fora do envelope autoritativo ou
em namespace excluído do digest causal. O log físico atual do `Embodiment` normaliza `wall_time` antes
de comparar determinismo; na integração, seu `seq` local e `sim_time` textual precisam ser adaptados
ao envelope canônico e não devem ser reinterpretados silenciosamente como `LogicalSequence` global.

### 9. World mutation

Cada `event_type` tem exatamente um owner de mutação e um reducer puro/versionado:

```text
reduce(previous_substate, event) -> next_substate
```

Reducers:

- não leem relógio de parede, rede, LLM ou RNG;
- não emitem eventos e não chamam outros contexts;
- rejeitam versão/schema desconhecido;
- preservam ids e invariantes referenciais;
- produzem o mesmo estado para os mesmos bytes canônicos de entrada.

Reações que possam criar novos fatos pertencem a handlers pós-commit e entram em ciclo posterior,
com links causais. Projectors constroem índices/views descartáveis e não são fonte de verdade. Uma
invariante que envolve mais de um subestado é verificada pelo commit coordinator; isso não autoriza um
“generic world service” a decidir regras de domínio.

O hash de estado usa serialização canônica, schemas e versões registradas. Maps não dependem da ordem
de iteração; números de domínio têm representação determinística; campos operacionais não entram no
hash.

### 10. Percepção, claims e comunicação

#### 10.1 `Observation`

No commit, cada evento que possa gerar percepção cria uma tarefa determinística em causal outbox,
referenciando tanto `base_revision` quanto `result_revision`. O `PerceptionResolver` consome essa
tarefa e avalia acesso por evento e observador potencial. O adapter versionado do `event_type` declara
se cada pista depende do estado anterior, do resultado ou da transição; assim, uma pessoa que sai de
uma sala não desaparece antes que os presentes possam vê-la sair. O resultado é imutável:

```text
Observation {
  observation_id
  observer_id
  observed_at
  source_event_refs[]
  modality + channel_ref?
  percepts[]
  claim_refs[]
  omissions/redactions
  evidence_chain[]
  resolver_version
}
```

`percepts` são descrições estruturadas do que ficou disponível aos sentidos, possivelmente parciais
ou ruidosas; não são cópia irrestrita do payload secreto. Atenção, interpretação psicológica,
confiança e inferência ficam para Cognition/Knowledge. Randomness perceptiva, quando existir, usa
substream nomeado e o resultado fica persistido para replay.

Outbox, `Observation` e `KnowledgeInput` usam ids determinísticos e entrega idempotente. Um barrier do
ciclo impede **despachar** a solicitação de um round já declarado para o destinatário enquanto houver
evidência daquele instante pendente de entrega; a `RoundDeclaration`, sua coordenada e sua
pertinência à coorte não mudam por causa do barrier. Assim, crash entre event commit e percepção
retoma o trabalho sem perder nem duplicar conhecimento causal.

Não existe fallback “todos viram o evento”. Sem observação, publicação endereçada ou evidência
prévia, o agente não recebe input.

#### 10.2 `Claim`

`Proposition` é o value object de conteúdo tipado, polaridade e escopo temporal. `Claim` é o artefato
imutável e identificado que registra alguém/canal afirmando essa proposition, com `claim_id`,
`asserted_by` (quando conhecido), instante da asserção e referências a claims/evidências anteriores.
Repetir o mesmo conteúdo pode criar outro claim; encaminhar sem nova asserção preserva a referência.
Nenhum dos dois possui campo autoritativo `is_true`.

O engine pode comparar um claim a world truth para regras específicas (por exemplo, verificação de um
documento), mas o resultado dessa comparação é outro fato/evento acessível apenas por cadeia causal;
não é anexado ao claim recebido pelo personagem.

#### 10.3 Comunicação e entrega

Uma comunicação aceita produz `communication.sent` e, quando há atraso/persistência, uma
`Transmission` no world state:

```text
Transmission {
  transmission_id
  claim_refs[] | encoded_content_ref
  actual_sender_id
  presented_sender
  intended_recipients[]
  channel
  sent_at
  delivery_occurrence_id?
  persistence/authenticity/forgery/interception facts
  status: IN_TRANSIT | DELIVERED | FAILED | CANCELLED
}
```

`actual_sender_id`, falsificação e interceptação são world truth e só aparecem numa observação se o
canal deixar evidência correspondente. `presented_sender` é o que o conteúdo/canal apresenta.

Entrega atrasada segue:

```text
communication.sent
  → ScheduledOccurrence(delivery)
  → communication.delivered | communication.delivery_failed
  → Observation(recipient/interceptor, se houver acesso)
  → KnowledgeInput
```

`sent` sozinho nunca cria observation do destinatário. `delivered` significa que o meio chegou ao
ponto/conta/dispositivo previsto; leitura/audição ainda depende do contrato do canal. Fala imediata é
o mesmo modelo com entrega no ciclo causal seguinte no mesmo instante, não um atalho que escreve
crença.

Uma mensagem forjada entrega o claim e a identidade apresentada. Ela não cria o evento alegado no
claim e não revela automaticamente a autoria real.

#### 10.4 `KnowledgeInput`

A fronteira entrega ao `Agent Knowledge & Memory` somente:

```text
KnowledgeInput {
  input_id
  recipient_id
  received_at
  kind: DIRECT_PERCEPT | COMMUNICATED_CLAIM | PUBLICATION
  channel
  observation_ref
  claim_refs[]
  evidence_chain[]
}
```

Persistir esse recibo prova apenas “evidência X ficou disponível ao ator Y neste instante”. Formar
crença, estimar confiança, suspeitar, esquecer, conciliar contradições ou inferir intenção pertence a
outro contexto. O context builder de decisão lê o inbox/estado epistemológico do ator; nunca consulta
o event store global como atalho.

#### 10.5 Allow-list para contexto de agente

Informação é uma fronteira de segurança. Cada datum que futuramente entrar no contexto de um agente
precisa responder por estrutura, não por comentário:

- qual `Observation`, documento estático permitido ou output cognitivo próprio o originou;
- em qual instante ficou disponível ao ator;
- por qual canal;
- se é percepção direta, claim/report, rumor, publicação ou inferência do próprio ator;
- se o destinatário e a timeline ainda permitem o acesso naquele instante.

O contexto é construído por allow-list a partir de três fontes: background estático que passou pelos
quatro gates do ADR 0005 no genesis; `KnowledgeInput` particionado pelo próprio `recipient_id`; e
estado cognitivo/memória pertencente ao ator. Construir prompt global e redigir segredos depois é
proibido.

`CanonClaim` do ADR 0005 (registro de pesquisa que pode semear o mundo) e `Claim` runtime (algo
afirmado dentro da simulação) são tipos e stores distintos. Canon futuro após a divergência, fato
oculto de world state, observation privada de terceiro, output do Observatory e chunk fora do
`actor_gate/spoiler_horizon/divergence_gate` não são fontes admissíveis. Resumos e reflexões futuras
preservam `evidence_chain` e continuam epistemic state; nunca são promovidos a `Event` ou world truth.

### 11. Determinismo e randomness

O contrato reproduzível é:

```text
mesmo genesis snapshot/config (inclusive fontes exógenas declaradas)
+ mesmo input ledger em bytes canônicos: inputs exógenos, SourceClosure, registro de slot
  (SlotDispatch, revogações e a única resposta por slot)
+ mesmas coordenadas de elegibilidade persistidas
+ mesmos AttemptRetryRecord
+ mesmas versões de schema/reducer/validator/resolver/RNG/fence policy
+ mesmo world seed
→ mesma DecisionCohort e mesmo AdmissionFence derivados por ciclo (fence_digest verificável)
→ mesmo input digest e mesmo decision_digest terminal
→ mesmo event log causal byte a byte
→ mesmo evidence ledger por destinatário
→ mesmo final state hash
```

O contrato vale para a **primeira execução** e para o replay pela mesma razão: ambos calculam a
membresia de cada ciclo pela mesma função sobre os mesmos fatos de ledger (§3.2.2–§3.2.4). Latência de
host, rede, coordinator ou entrega de agente pode mudar quando o coordinator espera e quando grava o
fence; não pode mudar o que o fence contém. Replay deriva o fence dos ledgers e verifica o
`fence_digest` gravado — nunca recomputa um corte a partir de observação do host, porque tal
observação não existe no protocolo. “Mesmos inputs” significa reutilizar as respostas externas/LLM
registradas e os `SourceClosure` gravados. Reexecutar um modelo remoto não faz parte do replay
determinístico da fundação; comparar nova cognição é uma eval separada.

Todo sorteio deriva de substream estável, por exemplo:

```text
derive(world_seed, subsystem, decision_key, entity_ids_canonical, purpose, algorithm_version)
```

Não há RNG global mutável. Adicionar ator ou avaliação irrelevante não desloca sorteios existentes.
Cada decisão estocástica registra chave/purpose, versão do algoritmo e resultado no decision/event
artefato apropriado. Iteração concorrente pode calcular candidatos em paralelo, mas o fechamento,
ordenação canônica e commit são determinísticos.

### 12. Snapshot e replay

Um snapshot causal contém, no mínimo:

- `run_id`, world seed e configuração versionada;
- `SimulationInstant`, `WorldRevision`, próximo `LogicalSequence` e ordinal de ciclo;
- todo world state autoritativo, incluindo o subestado físico;
- fila de `ScheduledOccurrence` e estado runtime de triggers;
- cursores/digests do input, decision, event e evidence ledgers;
- `CycleControlState` (§8.0.2) com referência ao último envelope terminal e, conforme o estado, ao
  `AdmissionFence` sem envelope terminal (`ATTEMPT_IN_FLIGHT`) ou ao `AttemptRetryRecord` ainda sem
  fence (`RETRY_AUTHORIZED`), com o `attempt_ordinal` correspondente;
- último `SourceClosure` de cada fonte exógena declarada;
- fila de `RoundDeclaration` pendentes e, para seus slots, dispatches abertos ou revogados e a
  resposta já gravada, quando houver;
- coordenadas de elegibilidade de toda entrada gravada e ainda não consumida;
- schemas e versões de reducers, validators, resolvers e RNG necessários;
- outboxes/inboxes causais cuja entrega ainda não foi confirmada;
- `EpistemicCheckpointRef` versionado de cada ator com estado epistemológico;
- hash canônico do snapshot.

Projeções reconstruíveis e caches não entram como autoridade. Estado de crença, memória e reflexão,
porém, não é projeção reconstruível: ele deriva de LLM e o replay é proibido de chamar modelo (§11).
Por isso um checkpoint resumível **precisa** conter, ou referenciar atomicamente, um checkpoint
epistemológico versionado de todo ator que possua esse estado:

```text
EpistemicCheckpointRef {
  actor_id
  store_id + schema_version
  content_ref | inline_payload
  content_hash
  producer + producer_version
  covers_through: { instant, revision, evidence_cursor }
}
```

Para a fundação esse checkpoint é **opaco**: a CSF verifica apenas presença, versão, hash e cobertura
— `covers_through` precisa alcançar o instante, a revisão e o cursor de evidência do snapshot — e
falha fechado quando um ator elegível está ausente ou defasado. Conteúdo, modelo de crença, reflexão
e política de esquecimento continuam propriedade de `Agent Knowledge & Memory`/Cognition, em árvore e
ownership separados do world truth, conforme ADR 0001; este ADR não os modela. Referência atômica
significa que checkpoint causal e checkpoint epistemológico são salvos e carregados como unidade
verificada por hash: um sem o outro é snapshot inválido, não snapshot parcial.

Há duas provas distintas:

1. **event replay:** genesis + event store reconstrói exatamente o mesmo `WorldState`/hash;
2. **resume equivalence:** snapshot + tails dos ledgers produz o mesmo log e estado que a execução
   contínua. Essa prova inclui o estado epistemológico restaurado do checkpoint, nunca regenerado por
   chamada de modelo, e a reconciliação dos quatro estados da §8.0.2. O resume recomputa
   `CycleControlState` pela dobra total e ordenada sobre os ledgers e falha fechado se divergir do
   snapshot; então: `IDLE` abre o próximo ciclo conforme §3.3; `ATTEMPT_IN_FLIGHT` reexecuta a
   tentativa a partir do fence gravado antes de qualquer ciclo novo; `RETRY_AUTHORIZED` grava o fence
   de `to_attempt_ordinal` para o **mesmo** `cycle_id`, sem seleção de coordenada e sem segundo
   `AttemptRetryRecord`; `HALTED_ON_ABORT` restaura o run **parado**, com `next_attempt_ordinal`
   preservado, e só um `AttemptRetryRecord` ou fork explícito o move. Slots com dispatch aberto e sem
   resposta seguem a §3.2.5: aguardar ou revogar e redespachar, nunca um segundo dispatch aberto.
   Resume nunca converte `HALTED_ON_ABORT` em `IDLE` nem em retentativa implícita.

O event replay reconstrói agenda e trigger runtime porque suas transições são eventos. O evidence
ledger pode ser verificado diretamente ou regenerado deterministicamente a partir de eventos,
outboxes, seed e versões; regeneração nunca consulta um LLM.

Snapshot sem versão disponível falha fechado; migração exige função/versionamento explícito. O
`snapshot_version: 1` atual do `Embodiment` é um artefato de componente, não deve ser confundido com
o futuro schema do snapshot causal completo. A integração deve aninhá-lo ou adaptá-lo com versão
declarada, nunca fundir árvores por coincidência de nomes.

Criar uma linha divergente gera novo `run_id` com `parent_checkpoint_ref/hash`. O prefixo histórico é
imutável e compartilhável; inputs, ids derivados, ledgers e revisões depois do fork pertencem somente
à nova run. Merge de timelines não existe na V1.

### 13. Observatory

O `Observatory` é um conjunto de query ports read-only sobre snapshots, event store, decision ledger,
observações e estados dos bounded contexts. O modo privilegiado pode ler inclusive world truth e
eventos secretos. Isso é autoridade de leitura, não uma identidade dentro do mundo.

O pacote/processo do Observatory:

- não recebe referência a command bus, committer, scheduler, RNG ou knowledge sink;
- não pode criar input causal nem confirmar outbox;
- não materializa `Observation` ao consultar um evento;
- não atualiza `last_seen`, memória, belief ou métrica autoritativa de ator;
- mantém cache/telemetria própria fora do snapshot causal.

`Character POV` é uma projeção de `Observation`/`KnowledgeInput` daquele ator. Não é event-store query
com filtro aplicado depois. `Public View` usa eventos/publicações explicitamente publicáveis. Assim,
uma leitura privilegiada não pode vazar por efeito colateral.

### 14. Invariantes normativas

1. Só o commit coordinator acrescenta world events e publica `WorldRevision`.
2. Todo evento pertence a exatamente um commit e toda revisão publicada aponta para o lote que a
   produziu.
3. Propostas simultâneas usam o mesmo snapshot; ordem/latência de execução não é prioridade.
4. `LogicalSequence` ordena fatos commitados; não decide disputa.
5. Occurrence/trigger handlers, validators e reducers não mutam stores diretamente.
6. Trigger repetível tem rearm/cadência persistida; reavaliação sozinha nunca repete efeito.
7. Resultado indeterminado de validação é `INDETERMINATE` registrado dentro de um `CycleAbortRecord`
   auditável e aborta a tentativa antes que qualquer disposição exista; lacuna de implementação não
   vira permissão.
8. Proibição institucional/legal não implica impossibilidade física.
9. Eventos são fatos do mundo; claims são alegações; observations são evidência disponível.
10. Evento existente, público ou secreto não altera conhecimento sem rota causal registrada.
11. Envio sem entrega/percepção não produz `KnowledgeInput` no destinatário.
12. Reducer muda estado e nunca decide/emite novo fato.
13. Commit multi-evento e commit do ciclo são atômicos.
14. Todo uso de randomness é seedado, nomeado, versionado e independente de ordem incidental.
15. Replay não chama LLM nem usa wall time.
16. Observatory não possui nenhum port de escrita causal.
17. Toda transição autoritativa de schedule/trigger é representada no event store.
18. Nenhum ator começa novo round enquanto houver `KnowledgeInput` causalmente anterior pendente para
    ele.
19. Contexto de agente é allow-list por holder/timeline/provenance; não é uma view redigida do estado
    global.
20. Toda unidade causal é admitida por um `AdmissionFence` durável, derivado dos ledgers e gravado
    antes de qualquer avaliação; membresia é função da coordenada de elegibilidade, dos
    `SourceClosure` gravados e de `base_revision`, nunca da ordem de chegada, do wall clock do
    coordinator nem da ausência momentânea de pendências. O fence persistido é verificável contra a
    derivação.
21. `ingress_seq` só participa da normalização de ingresso — antes ou depois do `SourceClosure` da
    própria fonte. Ordem canônica, prioridade e desempate derivam de conteúdo e de política
    versionada; nenhuma derivação escolhe entre respostas de slot, porque o ledger nunca contém duas.
22. A `DecisionCohort` de um ciclo é o conjunto derivado das `RoundDeclaration` elegíveis em
    `base_revision`; todos os seus rounds são admitidos no mesmo ciclo, leem a mesma `base_revision`
    e concorrem no mesmo espaço de `ConflictSet`. Latência nunca separa rounds simultâneos nem
    exclui um round da coorte.
23. Todo desfecho terminal (`COMMIT`, `REJECT`, `DEFER`, `NO_PROPOSAL`, `DEDUPLICATED`) é um
    `DecisionRecord` canônico persistido exclusivamente na transação de um `CycleCommit`, junto de
    fence closure, eventos e `WorldRevision`; há exatamente um por unidade admitida e nenhuma fonte
    vencida permanece `PENDING` no instante avaliado.
24. Todo fence gravado termina em exatamente um envelope: `CycleCommit` (mesmo com zero eventos) ou
    `CycleAbortRecord` (sem revisão, sem evento, sem avanço de ordinal, sem consumo e sem
    `DecisionRecord`).
25. Elegibilidade de readmissão é a coordenada persistida `(not_before_instant,
    not_before_cycle_ordinal)`; `DEFER` de mesmo instante usa ordinal maior, nunca momento de
    inserção.
26. Link causal dentro do mesmo ciclo só existe entre eventos do mesmo `CommitCandidate` atômico.
27. Checkpoint resumível contém ou referencia atomicamente checkpoint epistemológico versionado de
    cada ator elegível.
28. Nenhum ciclo de coordenada `C` deriva membresia ou grava fence enquanto alguma fonte exógena
    declarada tiver `closed_through < C`; entrada gravada após o fechamento da própria fonte recebe
    coordenada posterior por regra de ingresso, nunca por observação do coordinator.
29. `CycleControlState` é uma dobra total e ordenada sobre fences, envelopes terminais e
    `AttemptRetryRecord`: fence sem envelope ⇒ `ATTEMPT_IN_FLIGHT`; abort com retentativa
    registrada e sem fence ⇒ `RETRY_AUTHORIZED`; abort sem retentativa ⇒ `HALTED_ON_ABORT`; senão
    `IDLE`. Resume restaura `HALTED_ON_ABORT` como run parado; só `AttemptRetryRecord` ou fork
    explícito abre `attempt_ordinal + 1`; `RETRY_AUTHORIZED` reabre o mesmo `cycle_id` na tentativa
    autorizada sem seleção de coordenada; `ATTEMPT_IN_FLIGHT` é reexecutado a partir do fence
    gravado. Cada `AttemptRetryRecord` referencia o último envelope terminal do run e é consumido por
    exatamente um fence.
30. A resposta de slot é o preenchimento de uma unidade derivada, não entrada de fonte exógena: não
    tem coordenada própria, não está sujeita a `SourceClosure` nem à normalização de ingresso. Cada
    slot tem no máximo um `SlotDispatch` aberto e no máximo uma `SlotResponse`, admitida por escrita
    condicional contra o dispatch aberto; redespacho exige `SlotDispatchRevocation` durável.

## Cenários de stress e provas de aceite

| Cenário | Prova exigida |
|---|---|
| Duas respostas LLM chegam em ordens opostas | mesmo `decision_round_id` + `base_revision`; log e vencedor idênticos |
| Duas `RoundDeclaration` R1 e R2 com a mesma coordenada; R1 completa antes de o coordinator “ver” R2 | a coorte é derivada de `base_revision`, onde R1 e R2 já existem; ambos entram na mesma coorte e no mesmo ciclo em qualquer execução; mesma `base_revision`; seus candidatos disputam o mesmo `ConflictSet`; qual completou primeiro não muda nada |
| Round e input exógeno declaram o mesmo `effective_at` | o fence deriva de `base_revision` mais o `SourceClosure` da fonte do input; membresia, `ConflictSet`, vencedor e log não mudam com a ordem de chegada |
| Dois inputs exógenos E1 e E2 gravados antes do mesmo `SourceClosure`, em ordens opostas | mesmo `admitted_input_ids`, mesmo `fence_digest`, mesmo log; `ingress_seq` não entra em nenhuma ordenação |
| Execução A grava E1, o coordinator deriva o fence, E2 chega; execução B grava E1 e E2 antes do fence | impossível divergir: sem `SourceClosure` cobrindo a coordenada o coordinator ainda não derivou nada; com o fechamento, E2 gravado depois dele recebe coordenada posterior por regra em ambas as execuções. Mesmo ledger ⇒ mesmo fence |
| Coordinator grava o fence cedo ou tarde (host rápido/lento, reinício) | `fence_digest` idêntico; replay recomputa a derivação e verifica; só telemetria muda |
| Input exógeno gravado depois do `SourceClosure` da própria fonte | não é inserido retroativamente; recebe `open_coordinate` por regra de ingresso e é admitido pelo primeiro fence posterior que o torne elegível, com o valor declarado preservado como provenance |
| Candidato de ocorrência vencida é rejeitado | ocorrência termina no mesmo commit; nenhum `PENDING` remanescente e nenhum redisparo a partir de estado velho |
| Ciclo em que todos os slots respondem `NoProposal` | `CycleCommit` vazio persistido com um `DecisionRecord(NO_PROPOSAL)` por slot; bijeção `admitted_input_ids ↔ decision_record_ids`; replay reproduz revisão, ordinal e `next_logical_sequence` |
| Duas entradas admitidas com a mesma `idempotency_key` | a sobrevivente canônica recebe a disposição; a duplicata recebe `DecisionRecord(DEDUPLICATED, canonical_unit_id)`; `decision_digest` cobre ambas |
| Faceta sem regra, dado ou resolvedor disponível | `INDETERMINATE` canônico dentro de um `CycleAbortRecord`; nenhum `CycleCommit`, nenhum `DecisionRecord`, nenhuma revisão publicada, nenhum avanço de ordinal |
| Candidato A já tem `COMMIT` provisório quando B encontra `INDETERMINATE` | `CycleAbortRecord` sem nenhum `DecisionRecord`; o `COMMIT` de A é material provisório não autoritativo; A continua elegível e é reavaliado na tentativa seguinte junto com B |
| Crash entre append de eventos e settlement da decisão | impossível: as cinco partes são uma transação; fence sem envelope terminal é reexecutado a partir do corte gravado |
| Resume encontra fence gravado sem `CycleCommit` nem `CycleAbortRecord` | `CycleControlState = ATTEMPT_IN_FLIGHT`; tentativa reexecutada com o mesmo `fence_digest` e as respostas já registradas; mesmo resultado, sem duplicar mundo nem decisão |
| Snapshot tirado depois de um `CycleAbortRecord` | `CycleControlState = HALTED_ON_ABORT` com `next_attempt_ordinal`; resume restaura o run parado, não abre ciclo nem retenta; um `AttemptRetryRecord` explícito leva a `RETRY_AUTHORIZED` e o fence seguinte abre `attempt_ordinal + 1` com membresia idêntica |
| Snapshot ou crash entre o `AttemptRetryRecord` e o novo fence | dobra devolve `RETRY_AUTHORIZED` (`cycle_id`, `to_attempt_ordinal`, `retry_ref`), nunca `HALTED_ON_ABORT` nem `IDLE`; resume grava o fence de `to_attempt_ordinal` para o mesmo `cycle_id`, sem §3.3 e sem segundo `AttemptRetryRecord`; `next_attempt_ordinal` continua `N + 1` até o fence existir |
| Fence de `N + 1` gravado sem envelope enquanto o último envelope do run é o abort de `N` | regra 1 prevalece: `ATTEMPT_IN_FLIGHT`; o abort de `N` foi consumido pelo `AttemptRetryRecord` em `retry_ref`; se `N + 1` abortar, a regra 3 devolve `HALTED_ON_ABORT` com `next_attempt_ordinal = N + 2` |
| Timeout grava `NoProposal(s1)` e a `ActionProposal(s1)` real chega antes de o round fechar | ambas apontam para o mesmo dispatch aberto; a escrita condicional admite exatamente uma e rejeita a outra para auditoria; `response_refs` e `fence_digest` são função do ledger; qual venceu é input durável e visível no digest, não desempate do coordinator |
| Crash após o dispatch de `s1` e antes da resposta; resume enquanto a chamada original ainda está em voo | resume encontra dispatch aberto e slot vazio; ou aguarda, ou grava `SlotDispatchRevocation` antes de redespachar; a resposta ao dispatch revogado é rejeitada mesmo chegando primeiro; nunca dois dispatches abertos nem duas respostas por slot |
| Adapter que é fonte exógena declarada e também respondedor de slot fecha `closed_through >= C` antes de responder | suas respostas de slot não recebem `open_coordinate > C`: são preenchimento de unidade derivada, fora da condição de fechamento; o fence admite os slots com a coordenada herdada da `RoundDeclaration` |
| `DEFER` para o próximo ciclo do mesmo instante | sucessor recebe `(instant, cycle_ordinal + 1)` persistido no mesmo commit; resume e replay readmitem exatamente naquele ciclo |
| Dois candidatos vencem no mesmo ciclo | nenhum evento de um aparece como `causal_parent` do outro; reação exige ciclo posterior |
| Resume de snapshot com crença formada por LLM | checkpoint epistemológico presente, versionado e coberto por hash; resume equivalente sem chamar modelo |
| Duas ações consomem a última unidade | um `ConflictSet`; política/seed fixa seleciona o mesmo resultado; saldo nunca negativo |
| Regra proíbe agressão, mas corpos e acesso permitem | ação pode commitar; `institution.rule_violated` existe; punição só após cadeia de detecção/resposta |
| Evento secreto sem observador elegível | evento existe; zero `Observation`/`KnowledgeInput` para ator não relacionado |
| Papel VIP/segredo de exame antes da divulgação | ausente do evidence ledger dos não autorizados e de seus contextos |
| Observation privada de A | partição de A contém o registro; partições/contextos dos demais não |
| White Room existe no world/background truth | nenhum aluno sem gate/evento causal recebe datum ou chunk correspondente |
| Evento canônico posterior à divergência | não entra em memória, retrieval ou `KnowledgeInput` da timeline simulada |
| A diz falsamente que B roubou | `communication.sent/delivered` + claim recebido; nenhum `resource.stolen` é criado |
| A falsifica remetente C | destinatário vê `presented_sender=C`; `actual_sender=A` continua world truth não observado |
| Mensagem enviada com atraso e run pausado antes da entrega | `Transmission=IN_TRANSIT`; destinatário sem input; resume dispara exatamente uma entrega |
| Trigger permanece verdadeiro por dez commits | edge/once ativa uma vez; repeat ativa apenas nas cadências persistidas |
| Trigger cria outro trigger no mesmo instante | ciclos ordinais diferentes, DAG causal e limite de cascata determinístico |
| Reducer falha no terceiro evento do lote | nenhum evento, revisão, disposição terminal ou consumo de occurrence persiste; a tentativa termina em `CycleAbortRecord` |
| Observatory lê segredo e depois fecha | hashes de world/knowledge/ledgers permanecem inalterados |
| Reflexão resume um rumor como hipótese | provenance continua apontando ao claim; nenhum Event/world truth é criado |
| Snapshot no meio de comunicação em trânsito | contínuo e snapshot+resume geram mesmos tails e state hash |
| Duas runs partem do mesmo checkpoint com inputs diferentes | prefixo/hash parental igual; tails e estados isolados; nenhum evento cruza a branch |
| Mesmo seed/inputs com paralelismo diferente | event log causal byte a byte e final state hash idênticos |

## Contratos/ports mínimos para implementação

Os nomes concretos podem variar, mas a responsabilidade não:

| Contrato | Responsabilidade única |
|---|---|
| `Clock` | expor/avançar `SimulationInstant` conforme próxima entrada causal |
| `CycleCoordinator` | congelar a revisão base, aguardar a condição de fechamento, derivar coorte e fence e coordenar a tentativa até um envelope terminal; manter `CycleControlState` pela dobra total da §8.0.2 |
| `AdmissionFenceLog` | persistir o `AdmissionFence` por `(cycle_id, attempt_ordinal)` antes de qualquer avaliação e servi-lo a replay/resume para verificação contra a derivação |
| `InputLedger` | registrar fontes exógenas declaradas, admitir entradas tipadas idempotentes, atribuir `ingress_seq` e coordenada normalizada, persistir `SourceClosure` monotônicos e expor a condição de fechamento; manter o registro de slot — `SlotDispatch`, `SlotDispatchRevocation` e no máximo uma `SlotResponse` por `(decision_round_id, slot_id)`, admitida por escrita condicional contra o dispatch aberto (§3.2.5) |
| `ScheduleStore` | manter lifecycle de occurrences e `RoundDeclaration` e consultar as elegíveis à coordenada do ciclo |
| `TriggerRegistry` | armazenar definition/version e runtime state |
| `OccurrenceHandler` | transformar occurrence vencida em candidato, sem side effect |
| `ActionSchemaRegistry` | validar forma/versionamento de propostas |
| `AffordanceValidator` | produzir facets contra a revisão congelada |
| `ConflictDetector` | construir conflict sets conservadores |
| `ConflictResolver` | produzir disposições sob política/version/seed explícitos |
| `DecisionLedger` | registrar material provisório de avaliação, os envelopes terminais (`CycleCommit` com `DecisionRecord`, `CycleAbortRecord`) e `AttemptRetryRecord` |
| `CommitCoordinator` | validar lote/post-state e persistir atomicamente fence closure + um `DecisionRecord` por unidade admitida + eventos + `CycleCommit` + revisão, ou o `CycleAbortRecord` sem nenhum `DecisionRecord` |
| `EventStore` | append/read de eventos imutáveis; sem API update/delete |
| `ReducerRegistry` | mapear cada event type ao único owner/reducer versionado |
| `PerceptionResolver` | transformar evento+acesso em observations endereçadas |
| `EvidenceLedger` | persistir observations/recibos por destinatário, append-only |
| `EpistemicOutbox` | retomar entrega idempotente e impor barrier antes do próximo round |
| `KnowledgeInputSink` | entregar idempotentemente recibos, sem formar crença |
| `SnapshotStore` | salvar/carregar checkpoint causal (inclusive `CycleControlState`) + referência epistemológica como unidade verificada por hash |
| `ObservatoryQueries` | leitura sem dependências transitivas de escrita |

Não haverá um `WorldService` genérico que valide, resolva, mute, perceba e comunique. Esses contratos
existem para tornar impossível que dois módulos decidam o mesmo verbo.

## Sequência de implementação recomendada

1. Value objects, coordenada de elegibilidade, envelopes canônicos, quatro ledgers e hashes.
2. Clock, `WorldRevision`, fontes/`SourceClosure`, `RoundDeclaration`/coorte, registro de slot
   (`SlotDispatch`, revogação, resposta única por escrita condicional), derivação verificável do
   `AdmissionFence`, agenda e lifecycle de trigger com testes de propriedade — incluindo a prova de
   que permutar a ordem de gravação e o momento do fence não altera `fence_digest`.
3. `ActionProposal`, facets e um resolvedor de recurso mínimo, sem regra de exame.
4. `EventBatch`, reducers, transação única (um `DecisionRecord` por unidade admitida + eventos +
   `CycleCommit` + revisão), envelope de abort sem settlement, `CycleControlState`/`AttemptRetryRecord`
   e replay de world state.
5. `Observation`, `Claim`, `Transmission` e entrega agendada.
6. `KnowledgeInput` idempotente e testes de isolamento.
7. Snapshot/resume completo, incluindo o contrato opaco de checkpoint epistemológico, e Observatory
   estritamente read-only.
8. Adapter do `Embodiment` para envelope/revisão canônicos.

Cada passo deve ser um tracer bullet testável; as regras concretas de exames, economia e relações vêm
depois como plugins de domínio consumidores desses contratos.

## Alternativas descartadas

**Uma fila única de “eventos futuros”.** Mistura trabalho pendente com fatos já ocorridos e torna
cancelamento/retry semanticamente ambíguos.

**Converter toda ocorrência em `ActionProposal` de um ator `SYSTEM`.** Falsifica agência e obriga
pagamento, deadline e entrega a atravessarem validação concebida para tentativas.

**Resolver propostas conforme chegam.** Dá vantagem à latência e impede snapshot comum.

**Fechar o ciclo quando “não há input pendente”.** A ausência de pendência é uma observação de wall
clock que não fica registrada: os mesmos inputs produzem ciclos diferentes conforme a latência de
entrega, e o replay apenas reproduz uma atribuição já sorteada. O corte precisa ser um fato gravado.

**Um `decision_round_id` por ciclo.** Força rounds causalmente simultâneos a ciclos e revisões
diferentes, e quem abre primeiro passa a decidir prioridade causal. A coorte preserva a
simultaneidade prometida sem inventar ordem.

**Fence cortado por observação do coordinator (“gravar quando a coorte parecer completa”).** Persistir
o corte torna-o reproduzível, mas não determinístico: o wall clock do coordinator ainda escolhe a
membresia semântica na primeira execução, e dois hosts com o mesmo input produzem fences diferentes.
O corte precisa ser derivado de fatos que as fontes declararam — `SourceClosure` e `RoundDeclaration`
— e verificável por recomputação.

**Rounds abertos ad hoc pelo orchestrator.** O conjunto de rounds de uma coordenada passa a ser o
conjunto que o host viu a tempo; latência decide quem participa da coorte. Declarar rounds como
entradas causais duráveis fecha o conjunto por construção.

**Abort que canoniza disposições parciais.** Um `COMMIT` autoritativo sem evento nem revisão é
settlement sem mundo, exatamente a divergência que a transação única existe para impedir; a fonte
ficaria terminal e elegível ao mesmo tempo. O abort cobre apenas evidência de tentativa.

**Registro terminal só para fontes com candidato.** Deixa `NoProposal` e duplicatas sem desfecho
representável, e o `decision_digest` não consegue distinguir omissão legítima de bug. Um desfecho por
unidade admitida restaura a bijeção.

**Resume que trata abort como “fence sem envelope”.** Confunde crash em andamento com parada
deliberada e transforma o resume em retentativa implícita. O estado de controle precisa distinguir os
dois e a retentativa precisa ser um ato durável.

**Estado de controle definido por predicados independentes sobre “o último envelope”.** Deixa a
janela entre `AttemptRetryRecord` e o novo fence sem estado e faz “fence em voo” e “último envelope é
abort” valerem ao mesmo tempo. A dobra precisa ser total e ordenada, com um estado próprio para a
retentativa autorizada e ainda não iniciada.

**Resposta de slot como entrada de fonte exógena.** Ou o respondedor precisa fechar a coordenada
antes de responder — e a resposta, gravada depois do fechamento, recebe coordenada posterior à do
ciclo e nunca é admitida —, ou o protocolo isenta a resposta por convenção não escrita. O slot é
unidade derivada; a resposta é preenchimento, sem coordenada e sem fechamento.

**“Primeira resposta gravada vence” sem dispatch vinculado.** Resolve o duplo `NoProposal` ×
proposta real, mas não o redespacho após crash: duas chamadas emitidas pela própria simulação passam
a disputar o slot e a latência de host escolhe o conteúdo. Com um único dispatch aberto por slot e
revogação durável antes de redespachar, a única disputa restante é entre atos externos ao mesmo
dispatch, e ela é resolvida pelo ledger na admissão, não por `derive()`.

**Desempatar respostas duplicadas por `ingress_seq` ou por conteúdo.** Reintroduz ordem de chegada na
membresia ou atribui significado causal a bytes de payload para escolher entre `NoProposal` e
`ActionProposal`. O ledger não pode conter duas respostas para o mesmo slot.

**Liquidar disposições em outbox depois do commit do mundo.** Permite mundo sem disposição — ou
disposição sem mundo — após crash, e transforma retentativa técnica em duplicação causal.

**Aplicar evento agendado antes das ações apenas porque saiu primeiro da fila.** Reintroduz ordem
incidental. Deadline e precedência precisam estar na regra temporal/conflito, enquanto candidatos do
mesmo ciclo leem a mesma revisão.

**`valid: boolean`.** Colapsa impossibilidade, enforcement e infração; não representa tentativa
proibida porém possível.

**Reducers que emitem eventos.** Oculta causalidade dentro de projeção e torna replay dependente de
side effects. Reações entram no ciclo seguinte.

**Copiar evento para memória de todos os atores.** Viola o ADR 0001 e torna segredo impossível.

**Marcar claim como verdadeiro/falso no objeto recebido.** Entrega world truth ao destinatário por
schema, inclusive em falsificações.

**RNG global sequencial.** Faz mudanças irrelevantes deslocarem todos os resultados posteriores.

**Observatory como ator onisciente.** Consultar passaria a ser ato causal e criaria rotas acidentais
de vazamento.

## Consequências

- A fundação requer quatro artefatos append-only e uma transação entre event append e publicação de
  revisão, em vez de um único JSONL usado para fatos, decisões, evidência e telemetria.
- Simultaneidade passa a ser propriedade verificável por `cycle_id/base_revision` e pelo
  `fence_digest`, não convenção do orchestrator.
- Cada tentativa de ciclo custa duas escritas duráveis de coordenação — o fence antes da avaliação e
  um envelope terminal depois dela —, em troca de remover wall clock e ordem de chegada da semântica
  causal e de tornar o settlement crash-consistente com o mundo.
- Toda fonte exógena declarada carrega o ônus de liveness: ela precisa gravar `SourceClosure` para
  que o run avance. Isso move o relógio de parede de dentro do coordinator para o adapter, onde vira
  input registrado; runs sem fontes exógenas não pagam fechamento algum. O respondedor de slot tem
  ônus diferente — preencher cada dispatch aberto — e nunca fecha coordenada.
- Cada slot custa um `SlotDispatch` durável antes da chamada e uma escrita condicional na resposta;
  em troca, o ledger nunca contém duas respostas para o mesmo slot e o resume nunca cria duas
  chamadas concorrentes para ele.
- Rounds deixam de ser um detalhe do orchestrator e passam a ser entradas causais com lifecycle no
  event store; abrir um round custa um commit anterior que o declare.
- Domínios futuros precisam declarar schemas, reads/writes, invariantes e versões; isso aumenta o
  custo inicial, mas impede autoridade implícita.
- Percepção/comunicação ganham persistência própria antes da cognição sofisticada, o mínimo necessário
  para provar não-vazamento.
- A integração do log/snapshot físico atual exige adapter e versionamento explícitos. Este ADR não
  escolhe linguagem, framework, banco ou topologia de processo para a fundação.

## Fora de escopo

- escolha de ação coerente com personalidade;
- inferência, confiança, suspeita, revisão de crença e esquecimento;
- dinâmica social/relacional;
- regras concretas de exames, economia, clubes ou facções;
- modelo detalhado de atenção;
- consequências institucionais específicas;
- intervenção do usuário;
- stack de aplicação, persistência ou orquestração LLM.
