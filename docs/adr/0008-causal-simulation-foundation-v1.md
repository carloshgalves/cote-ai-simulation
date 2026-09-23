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
| `CyclePlan` | discriminante durável `WORK | CLOCK_ADVANCE`; no avanço, fixa origem, destino e menor pendência confirmada |
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
| `TriggerActivation` | entrada causal derivada e durável que materializa uma ativação de trigger antes de ela produzir candidato |
| `ActionProposal` | pedido imutável para tentar uma ação; não é fato de que a ação ocorreu |
| `AffordanceAssessment` | diagnóstico facetado da proposta contra um snapshot |
| `CommitCandidate` | unidade atômica que pode produzir zero ou mais eventos e declarar conflitos |
| `Event` | fato imutável do mundo aceito em commit |
| `CycleCommit` | envelope persistido uma única vez no commit journal do `DecisionLedger`, obrigatório mesmo com zero eventos |
| `DecisionRecord` | desfecho terminal canônico de exatamente uma unidade admitida; existe somente dentro de um `CycleCommit` |
| `CycleAbortRecord` | envelope durável de tentativa abortada; audita `INDETERMINATE` e a causa sem publicar revisão nem liquidar nada |
| `AttemptRetryRecord` | decisão explícita e durável de abrir `attempt_ordinal + 1` para um ciclo parado após abort; consumido pelo fence que abre essa tentativa |
| `CycleControlState` | estado do coordinator derivado por dobra total e ordenada sobre fences, envelopes e retentativas: ocioso, tentativa em voo, retentativa autorizada ou parado após abort |
| `WorldState` | redução dos eventos autoritativos até uma revisão |
| `Observation` | evidência parcial disponibilizada a um observador por acesso perceptivo causal |
| `PerceptionTask` | trabalho epistemológico determinístico criado atomicamente com o commit de um evento potencialmente perceptível |
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
  `CausalRef`, `SeedKey`, `EligibilityCoordinate`, `CyclePlan`, `EventOrderKey` e versões de
  schema/política.
- **Entidades/artefatos imutáveis:** `Event`, `ActionProposal`, `NoProposal`, `Claim`,
  `Observation`, `KnowledgeInput`, `SourceClosure`, `SlotDispatch`, `SlotDispatchRevocation`,
  `PerceptionTask`, `AdmissionFence`, `DecisionRecord`, `CycleAbortRecord` e `AttemptRetryRecord`;
  identidade e provenance importam mesmo quando dois payloads são iguais.
- **Agregados com lifecycle:** `ScheduledOccurrence`, `RoundDeclaration`, `TriggerActivation`,
  `Trigger` (`Definition + RuntimeState`) e `Transmission`.
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

#### 2.2 Identidade causal e bytes canônicos

Todo identificador que integra um digest, aparece no `AdmissionFence` ou serve de origem para outro
identificador obedece à mesma `CausalIdentityPolicy`, fixada na configuração imutável do run por
`causal_identity_policy_version + causal_identity_policy_hash`. A operação normativa é:

```text
derive_id(domain_tag, canonical_components...) =
  digest(identity_algorithm_version, canonical_bytes_v1(domain_tag, canonical_components...))
```

`canonical_bytes_v1` é uma codificação tipada e length-prefixed: inteiros têm largura/sinal definidos
pelo schema; strings são UTF-8 NFC; ids e digests são bytes, não texto reformatado; campos de record
seguem a ordem do schema; maps são ordenados pela chave canônica; e coleções set-like usam a ordem
total declarada pelo contrato que as possui. Nenhuma implementação pode usar serialização default da
linguagem. Domain tags, algoritmo, schemas e regras de ordenação integram o hash da política; colisão
de um id derivado com componentes diferentes é corrupção e falha fechado.

As origens são estáveis e independentes de execução:

- `run_id` e ids de fontes/definitions vêm do manifesto de genesis versionado;
- toda entrada exógena traz `producer_unit_key`, única no namespace da fonte, estável em retries e
  independente de transporte; `input_id = derive_id(EXOGENOUS_INPUT, run_id, source_id,
  producer_unit_key)`. Reentrega reutiliza a mesma chave e identidade; dois atos semanticamente
  idênticos porém distintos no produtor usam chaves distintas e, portanto, ids distintos. A mesma
  chave com bytes de unidade diferentes é conflito de proveniência e falha fechado. Essa identidade
  de entrega não é a identidade lógica opcional de idempotência da §7.1: duas chaves de produtor
  distintas podem declarar o mesmo `idempotency_key` para afirmar que são aliases da mesma operação;
- `SourceClosure` traz `producer_closure_key` sob a mesma disciplina e deriva `closure_id` de
  `(run_id, source_id, producer_closure_key)`; `ingress_seq` seleciona a primeira prova que cobre uma
  coordenada, mas nunca aloca identidade;
- ids derivados usam a referência causal que os criou mais papel e ordinal **canônico do schema**:
  `cycle_id` deriva de run + coordenada do ciclo; `occurrence_id` e `decision_round_id` derivam do
  commit/genesis de origem + papel local; `activation_id` deriva de trigger/version + revisão de
  origem + `activation_count`; `proposal_id`/`no_proposal_id` derivam de
  `dispatch_id + response_kind`; `cohort_id` deriva de ciclo, revisão base e ids de rounds já
  ordenados; `decision_id` deriva de ciclo + tentativa + unit id; `candidate_id` deriva de ciclo +
  unidades-fonte ordenadas + produtor/versão + papel/ordinal local do candidato; `event_id` deriva de
  ciclo + `EventOrderKey` canônica (§7). Ordinal local nunca é ordem de iteração: o schema do produtor
  precisa nomear/ordenar os outputs;
- `dispatch_id`, `observation_id`, `knowledge_input_id` e ids de tarefa/completion seguem a mesma
  função, com seus componentes causais completos definidos nas seções correspondentes; papel e
  ordinal de output vêm do schema, nunca da ordem do worker.

UUID aleatório, relógio de parede, posição de append, `ingress_seq`, ordem de chegada e ordem de map
são proibidos na alocação desses ids. Uma unidade sem chave externa estável ou origem derivável não é
admitida. Essa regra mantém aliases semanticamente iguais distinguíveis por suas chaves de produtor,
sem permitir que a permutação de ingresso troque suas identidades.

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
  cycle_ordinal_at_instant: integer   // último ordinal commitado; -1 se nenhum no instante atual
}
```

O relógio não usa tempo de parede para semântica. Pausa, velocidade de execução e duração de chamada
LLM são operacionais.

#### 3.2 `ResolutionCycle`

Um ciclo é identificado por `(run_id, instant, cycle_ordinal)`; cada tentativa dele é identificada
por `attempt_ordinal`. Antes de gravar o fence, o coordinator materializa um plano discriminado:

```text
CyclePlan =
  { cycle_kind: WORK }
  | { cycle_kind: CLOCK_ADVANCE,
      clock_advance: {
        from: SimulationInstant,
        to: SimulationInstant,
        target_coordinate: EligibilityCoordinate
      } }
```

`clock_advance` é obrigatório exatamente para `CLOCK_ADVANCE` e proibido para `WORK`. No plano de
avanço, `from` é o `instant` do ciclo, `target_coordinate` é a menor pendência `P` confirmada pela
escrita condicional da §3.3 e `to = P.not_before_instant > from`. A origem canônica
`CLOCK_ADVANCE(run_id, from, to, cycle_id)` deriva somente desses campos persistidos. O ciclo então
declara:

```text
ResolutionCycle {
  cycle_id
  attempt_ordinal
  instant
  cycle_plan
  base_revision
  base_state_hash
  cohort_id
  fence_digest
  admitted_input_ids[]
  input_digest
}
```

Todas as propostas do ciclo leem exatamente `base_revision`. Nenhuma vê mutações de outra proposta do
mesmo ciclo. O lote é fechado por protocolo, não pela ordem de chegada:

- uma solicitação a agente é um `SlotDispatch` durável (§3.2.5) e carrega `dispatch_id`,
  `decision_round_id`, `slot_id`, `actor_id`, `base_revision`, `effective_at` e a coordenada de
  elegibilidade da `RoundDeclaration` à qual o slot pertence;
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
é gravada de forma durável. A mesma `cycle_coordinate_policy_version + hash` define o piso de toda
entrada derivada por um commit:

```text
commit_successor_floor(cycle_commit) =
  (cycle_commit.instant, cycle_commit.cycle_ordinal + 1)       // cycle_plan.cycle_kind = WORK
  (cycle_commit.cycle_plan.clock_advance.to, 0)                 // CLOCK_ADVANCE
```

O segundo caso é reconhecido pelo discriminante persistido em `cycle_plan`, cuja origem é
`CLOCK_ADVANCE` (§3.3), e usa o estado de relógio
**resultante**, não o sucessor ordinal no instante de origem. Toda entrada causal criada por um
`CycleCommit` recebe coordenada `>= commit_successor_floor(commit)`. Assim nenhum efeito de
materialização temporal pode nascer já vencido depois que o relógio salta. Aplicam-se então as
regras específicas:

- input exógeno recebe `max((declared_effective_at, 0), open_coordinate(source_id))`, onde
  `open_coordinate` é o sucessor da última coordenada que a própria fonte fechou (§3.2.2). O valor
  declarado permanece no input ledger como provenance. Nunca se admite entrada no passado, e a
  normalização é uma função do ledger da fonte — não de qual fence o coordinator já gravou; se a
  fonte já publicou `final`, a entrada é rejeitada em vez de normalizada;
- `RoundDeclaration` recebe a coordenada escrita explicitamente por quem a criou: o genesis, ou o
  `CycleCommit` que a originou, com valor igual ou maior que seu `commit_successor_floor` (§3.2.3); os slots
  herdam a coordenada da declaração, e a resposta que preenche um slot não recebe coordenada própria
  nem passa pela normalização de ingresso (§3.2.5);
- ocorrência agendada pelo genesis recebe `(due_at, 0)`; quando criada por commit, recebe
  `max((due_at, 0), commit_successor_floor(commit))`, salvo quando uma política de origem versionada
  exigir coordenada ainda maior;
- `TriggerActivation` criada no genesis recebe a coordenada inicial declarada; a criada por um
  `CycleCommit` recebe seu `commit_successor_floor` ou coordenada maior, persistida na mesma transação que a
  revisão cuja transição do predicate a originou (§4.2);
- sucessor de `DEFER` recebe a coordenada escrita explicitamente pelo commit que o criou (§7).

A coordenada integra o input digest canônico e é reutilizada em replay/resume. Ela substitui qualquer
heurística de “foi criado depois, logo entra depois”: elegibilidade é fato declarado, não momento de
inserção.

##### 3.2.2 `CausalSource` e `SourceClosure`

Toda entrada causal pertence a exatamente uma fonte declarada. Há duas classes:

- **Fontes derivadas:** agenda (`ScheduledOccurrence`), registro de triggers, `RoundDeclaration`,
  sucessores de `DEFER` e qualquer outra entrada escrita por um `CycleCommit` ou pelo genesis. O
  conteúdo dessas fontes para qualquer coordenada é função pura de `base_revision` e dos ledgers
  commitados. Elas são fechadas **por construção**: um `CycleCommit` só pode criar entradas com
  coordenada `>= commit_successor_floor(commit)` da §3.2.1, logo, quando um ciclo abre sobre a revisão
  resultante, o conjunto de entradas derivadas elegíveis a ele já está fixado. Para um ciclo comum o
  piso é o próximo ordinal no mesmo instante; para avanço temporal é `(to, 0)`;
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
  closure_id                              // derivado de (run_id, source_id, producer_closure_key)
  source_id
  producer_closure_key                    // estável na fonte; reusada somente em reentrega idêntica
  closed_through: EligibilityCoordinate     // monotônico por fonte; pode saltar à frente
  final: boolean                            // true = a fonte nunca mais grava
  ingress_seq
}
```

Regras:

- `closed_through` nunca regride. `ingress_seq` é alocado sem lacunas na mesma escrita atômica do
  registro, e a leitura por fonte é prefix-consistente: observar o item `N` implica observar todos os
  itens válidos `<= N`. Uma fonte pode fechar coordenadas muito à frente ou declarar-se `final`; um
  run sem fontes exógenas — mesmo com rounds e respondedores de slot — tem a condição abaixo
  trivialmente satisfeita;
- **condição de fechamento:** seja `closure_coordinate(C, cycle_plan) = C` em `WORK` e
  `cycle_plan.clock_advance.target_coordinate` em `CLOCK_ADVANCE`. Um ciclo só confirma sua
  membresia e grava o plano quando toda fonte exógena declarada tem um `SourceClosure` que satisfaz
  `final = true` **ou** `closed_through >= closure_coordinate`. Um marcador final cobre essa
  coordenada e toda coordenada posterior, independentemente do valor finito de `closed_through`.
  Até então o coordinator **aguarda**; ele nunca corta por conta própria. Uma fonte parada é condição
  operacional (telemetria, alerta), não decisão semântica;
- **normalização de ingresso:** `open_coordinate(source_id)` é o sucessor
  `(closed_through.instant, closed_through.ordinal + 1)` do último fechamento da fonte, ou a
  coordenada mínima do run quando ainda não houve fechamento. Uma entrada gravada depois do
  `SourceClosure` da própria fonte recebe coordenada `>= open_coordinate` por regra, então nunca cai
  sob um fechamento já declarado. Depois de um marcador `final`, `open_coordinate` é indefinida e o
  `InputLedger` rejeita atomicamente qualquer novo input ou `SourceClosure` daquela fonte; nem
  normalização nem reabertura são permitidas. `ingress_seq` participa apenas dessa ordem durável e da
  seleção da prova de fechamento, nunca da ordem/prioridade de unidades admitidas;
- **prova canônica:** para cada `(source_id, C)`, `covering_closure(source_id, C)` é o primeiro
  `SourceClosure` na ordem crescente de `ingress_seq` que satisfaz `final = true` ou
  `closed_through >= C`. Esse seletor é total quando a condição de fechamento vale e é estável sob
  append: marcadores posteriores nunca substituem o primeiro que cruzou `C`;
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
  source_id                          // fonte derivada canônica; slots herdam este valor
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
valor `>= commit_successor_floor(commit)`, na mesma transação que publica a revisão (§7.1). Criação,
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
tem coordenada igual ou maior que `commit_successor_floor(commit)` e cai numa coorte posterior. Em
ciclo comum isso conta para o limite de cascata no mesmo instante; em ciclo de avanço o round nasce
no instante de destino, nunca num ordinal vencido do instante de origem.

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
  cycle_plan
  base_revision + base_state_hash
  closure_proof: [{ source_id, closure_ref, ingress_seq, closed_through, final }]
                  // covering_closure(source_id, closure_coordinate) de toda fonte exógena declarada
  cohort: { cohort_id, rounds: [{ decision_round_id,
                                  slots: [{ slot_id, response_ref }] }] }
  admitted_units: [{ unit_id, unit_digest }]  // ordem canônica; ids são a projeção deste array
  input_digest
  admission_order_policy_version + admission_order_policy_hash
  causal_identity_policy_version + causal_identity_policy_hash
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

Toda unidade persistida possui `unit_digest`, calculado sobre os bytes canônicos de **todo** o record
causal imutável — schema/tipo, fonte, chave estável do produtor quando houver, ator, payload,
coordenada/instante, provenance e identidade de idempotência — excluindo somente o próprio digest,
`ingress_seq` e telemetria de transporte. Para slots, `response_digest` de `ActionProposal` ou
`NoProposal` é esse `unit_digest`; occurrences, ativações e inputs tipados seguem a mesma função no
schema correspondente. O ledger rejeita a mesma identidade com outro digest.

`admitted_input_ids[]` é apenas a projeção ordenada `admitted_units[].unit_id`, usada nos contratos de
settlement; não é uma segunda coleção serializada. A operação normativa é
`input_digest = digest(canonical_bytes_v1(admitted_units[]))`, comprometendo simultaneamente
identidade, conteúdo e ordem. Cada ref do cohort ou do fence precisa resolver para o mesmo
record/digest antes da escrita condicional; ausência ou mismatch falha fechado. `fence_digest` cobre
`input_digest` e o array de pares, então resume nunca reavalia um mesmo id contra bytes diferentes.

O fence é uma **função**: `derive(ledgers, base_revision, C, cycle_plan, versions)`. O coordinator não escolhe
um high-water; para cada fonte ele registra em `closure_proof` exatamente o
`covering_closure(source_id, closure_coordinate)` da §3.2.2, onde `closure_coordinate = C` para
`WORK` e `closure_coordinate = cycle_plan.clock_advance.target_coordinate` para `CLOCK_ADVANCE`.
Neste segundo caso, a mesma escrita condicional verifica também que a coordenada persistida no plano
continua sendo a menor pendência e que `from`/`to` ainda correspondem ao clock state. A leitura do
ledger é prefix-consistente e a escrita verifica novamente base revision, versões, plano e essas
referências antes do append. O fence persistido precisa ser igual à derivação; `cycle_plan` integra
seus bytes canônicos e `fence_digest`. Replay e resume recomputam a derivação e verificam o digest;
uma divergência falha fechado como corrupção ou deriva de versão (§11).

Consequências:

- `ingress_seq` participa apenas da normalização de ingresso e da escolha append-stable do primeiro
  fechamento que cobre `C` na §3.2.2. Ele nunca ordena, prioriza nem desempata unidades admitidas. A
  ordem canônica de `admitted_units` é a ordenação lexicográfica total por
  `(not_before_instant, not_before_cycle_ordinal, unit_kind_tag, canonical_source_id,
  unit_digest, unit_id)`. A política V1 fixa `unit_kind_tag` em `00=SLOT`,
  `01=EXOGENOUS_INPUT`, `02=SCHEDULED_OCCURRENCE`, `03=TRIGGER_ACTIVATION`; uma nova categoria exige
  nova versão com tag única. `canonical_source_id` é o identificador namespaced imutável da fonte em
  bytes UTF-8 NFC — o `source_id` persistido em toda unidade conforme a §3.2.2; slots herdam o da
  `RoundDeclaration`, occurrences persistem o da fonte derivada que as criou e ativações usam
  `trigger:<definition_id>`. Colisão de IDs namespaced é configuração inválida. Digests e IDs são
  comparados por seus bytes canônicos unsigned, nunca por locale, ordem de registro, enum local ou
  iteração de map;
- `admission_order_policy_version` e o hash da tabela/serialização acima pertencem à configuração
  imutável da run, entram no `AdmissionFence` e em `fence_digest`, e precisam estar disponíveis em
  replay/resume. Empate até `unit_id` implica identidade duplicada/corrupção e falha fechado; assim a
  chave é total sem recorrer a ordem incidental;
- a topologia inteira do fence tem ordem normativa, não só `admitted_units`:
  `closure_proof` é ordenada por `canonical_source_id`; `cohort.rounds` por `decision_round_id` em
  bytes canônicos; e cada round contém pares `{slot_id, response_ref}` ordenados por `slot_id`, em vez
  de arrays paralelos. `rule_versions` e qualquer map são ordenados pela chave canônica; toda outra
  coleção set-like coberta pelo digest declara sua chave no schema ou é inválida. O
  hash de `admission_order_policy` cobre essas ordens topológicas além da chave das unidades, e o
  `fence_digest` cobre a serialização `canonical_bytes_v1` do record inteiro, inclusive os hashes das
  políticas de identidade, admissão e fence e o `cycle_plan`; ordem de registro, construção de map
  e iteração local não alteram bytes;
- inverter a ordem de entrega de duas entradas gravadas antes do mesmo `SourceClosure` não muda o
  conjunto admitido nem o `fence_digest`: ambas ficam sob o fechamento e a ordenação não olha para
  `ingress_seq`;
- gravar o fence cedo ou tarde — host rápido ou lento, coordinator reiniciado, rede congestionada —
  não muda o fence: cada prova usa o primeiro fechamento que cobre `closure_coordinate`, estável sob
  append; inputs posteriores a esse fechamento recebem coordenada `> closure_coordinate`, e a
  escrita condicional impede gravar uma derivação sobre leitura incompleta ou revisão/versão
  divergente. O que muda é telemetria;
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
coordenada e cada slot já contém sua única resposta, a membresia e os pares
`{slot_id, response_ref}` do novo fence são **idênticos por construção** e nenhum slot é
redespachado; mudam apenas `attempt_ordinal`,
`retry_ref` e `rule_versions`.
O array `admitted_units` e o `input_digest` também permanecem idênticos; uma retentativa nunca troca
bytes de uma unidade sob a mesma identidade.

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
  actor_id + eligibility       // herdados da RoundDeclaration
  base_revision + effective_at // herdados do ResolutionCycle; effective_at = instant do ciclo
  request_digest
}

SlotDispatchRevocation {
  dispatch_id
  reason: RESUME_REISSUE | LIVENESS_POLICY | OPERATOR
  policy_version?
}

NoProposal {
  no_proposal_id
  dispatch_id
  run_id + cycle_id
  decision_round_id + slot_id
  actor_id
  effective_at
  submitted_against_revision
  reason: ACTOR_DECLINED | TIMEOUT | RESPONDER_FAILURE
  response_digest
}

SlotResponse := ActionProposal | NoProposal   // ambos carregam o vínculo causal completo acima
```

Regras, todas verificadas pelo `InputLedger` no momento da admissão, em escrita condicional única:

1. **um dispatch aberto por slot:** um `SlotDispatch` só é gravado para slot da coorte derivada,
   vazio e sem outro dispatch aberto. Um dispatch está **aberto** enquanto o slot está vazio e ele
   não foi revogado; o preenchimento do slot o encerra;
2. **vínculo causal fail-closed:** ao criar o dispatch, o ledger verifica que `run_id`, `cycle_id`,
   `decision_round_id`, `slot_id`, `actor_id` e `eligibility` correspondem ao ciclo, à
   `RoundDeclaration` e ao slot da coorte, e que `base_revision` e `effective_at` correspondem à
   revisão base e ao instante do ciclo. Ao admitir a resposta, exige igualdade exata de
   `dispatch_id`, `run_id`, `cycle_id`, `decision_round_id`, `slot_id`, `actor_id`, `effective_at` e
   `submitted_against_revision` com o dispatch aberto. A resposta não pode declarar elegibilidade
   própria; se um envelope de transporte a repetir, o valor precisa ser igual ao do dispatch e é
   descartado antes da persistência canônica. `ActionProposal` e `NoProposal` obedecem à mesma
   vinculação. Qualquer mismatch é rejeitado e auditado antes de preencher o slot ou participar de
   `cohort.rounds[].slots[].response_ref`, `input_digest` ou `fence_digest`;
3. **uma resposta por slot:** satisfeita a vinculação da regra 2, o ledger aceita uma
   `SlotResponse` somente se o slot está vazio. A escrita é condicional e atômica: ou preenche o slot,
   ou é rejeitada. Reentrega bit a bit idêntica da resposta já gravada é idempotente; qualquer outra
   escrita para o mesmo slot — segunda resposta, resposta atrasada, resposta a dispatch revogado — é
   rejeitada e vai para a auditoria de respostas rejeitadas, fora do digest causal. A chave de
   idempotência de uma resposta é o slot vinculado ao dispatch, não o payload;
4. **redespacho só após revogação durável:** emitir um novo dispatch para o mesmo slot exige antes
   um `SlotDispatchRevocation` do anterior. A partir da revogação, a resposta ao dispatch antigo não
   pode mais preencher o slot, mesmo que chegue antes da resposta ao novo. Revogar é ato registrado
   — de resume, de política de liveness versionada ou de operador —, nunca efeito de timing
   observado pelo coordinator;
5. **timeout é preenchimento, não concorrência:** o `NoProposal` de timeout é uma `SlotResponse`
   comum, gravada contra o dispatch aberto. Ela e a resposta real do mesmo dispatch disputam o slot
   apenas na escrita condicional da regra 3, que admite exatamente uma delas;
6. **sem coordenada, sem fechamento:** a resposta não recebe `EligibilityCoordinate` própria, não
   passa pela normalização de ingresso da §3.2.2 e não está sujeita a `SourceClosure`. O
   `ingress_seq` que o ledger lhe atribui é auditoria; nenhuma regra o consulta.

Consequências:

- `cohort.rounds[].slots[]` é função do ledger: cada par liga um `slot_id` à sua única
  `response_ref`, e a derivação nunca escolhe entre respostas porque o ledger nunca contém duas.
  Nenhum desempate por `ingress_seq` ou por conteúdo é necessário ou permitido (invariante 21);
- qual escrita venceu a condicional da regra 3 — resposta real ou `NoProposal` de timeout — é um fato
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

`next_cycle_coordinate(clock_state, pending_ledgers)` é uma função total sob
`cycle_coordinate_policy_version + cycle_coordinate_policy_hash`, imutável na configuração do run.
Ela considera todas as unidades persistidas, pendentes e não consumidas: occurrences,
`RoundDeclaration`, `TriggerActivation`, sucessores de `DEFER` e inputs exógenos. Seja `P` a menor
`EligibilityCoordinate` lexicográfica desse conjunto, após leitura prefix-consistente e condição de
fechamento das fontes para `P`:

1. se não existe `P`, o run fica ocioso e aguarda novo input/derivação; não fabrica ciclo vazio;
2. se `P.not_before_instant == current_instant`, o próximo ciclo tem coordenada exatamente `P`.
   Seu fence persiste `cycle_plan={cycle_kind: WORK}`.
   Ordinais sem unidade pendente são saltados: depois de `(t,1)`, uma única pendência em `(t,3)` abre
   `(t,3)`, nunca um commit decorativo em `(t,2)`;
3. se `P.not_before_instant > current_instant`, o engine primeiro grava um **ciclo de avanço** na
   coordenada `(current_instant, cycle_ordinal_at_instant + 1)`. Esse ciclo tem
   `admitted_units=[]`, `input_digest` vazio canônico e
   `cycle_plan={cycle_kind: CLOCK_ADVANCE, clock_advance: {from: current_instant,
   to: P.not_before_instant, target_coordinate: P}}`. A origem de coordenação
   `CLOCK_ADVANCE(run_id, from, to, advance_cycle_id)` é derivada desse plano; o fence só pode ser
   gravado depois de toda fonte exógena fechar pelo menos até `P` e de uma escrita condicional
   confirmar que o plano ainda parte do clock state corrente e que `P` continua sendo a menor
   pendência. O lote contém `clock.advanced { from, to }` e os eventos de
   materialização temporal exigidos pelos substates, como `body.advanced`. No estado resultante,
   `current_instant = P.not_before_instant` e `cycle_ordinal_at_instant = -1`. Toda occurrence,
   `RoundDeclaration`, `TriggerActivation` ou outra entrada derivada por esse commit recebe
   coordenada `>= (P.not_before_instant, 0)` por `commit_successor_floor` (§3.2.1). Depois de publicar
   o lote, o coordinator recalcula a menor pendência entre a `P` original e as entradas recém-criadas;
   o próximo ciclo de trabalho abre exatamente nessa nova menor coordenada, nunca no instante de origem.

Em estado `IDLE`, encontrar `P <= (current_instant, cycle_ordinal_at_instant)` é violação de
lifecycle/settlement e falha fechado; trabalho vencido não é silenciosamente reaproveitado. Logo o
salto do item 2 é sempre para ordinal estritamente maior.

Logo o ciclo de avanço pertence ao instante de origem e o primeiro ciclo no destino não é consumido
por housekeeping. Uma entrada derivada pela materialização pode preceder `P` somente no ordinal do
instante de destino — por exemplo, `(to,0)` quando `P=(to,3)` — e então é corretamente escolhida pela
nova derivação. Se uma fonte acrescenta, antes do fechamento, uma unidade `Q < P`, a escrita
condicional falha e a função é reexecutada com `Q`; input gravado depois do fechamento recebe
coordenada posterior pela §3.2.2. Replay lê as coordenadas e a versão/hash da mesma política, e nunca
decide entre salto e ciclo vazio por convenção local. Não existe recuperação, expiração ou juros
implícitos fora do log causal.

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
  source_id
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
é avaliado no bootstrap e, durante a preparação de cada commit que intersecte suas dependências
declaradas, sobre a cópia de trabalho da revisão resultante; runtime e ativação são então publicados
atomicamente com esse commit. Uma implementação pode avaliar mais predicates, mas não pode alterar o
resultado.

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

Uma ativação não é um instante efêmero entre avaliar o predicate e construir um candidato. É uma
entrada causal derivada e durável:

```text
TriggerActivation {
  activation_id
  source_id                         // trigger:<definition_id>
  trigger_definition_id + trigger_version
  origin: { cycle_id, result_revision, causing_event_ids[] } | GenesisRef
  eligibility: EligibilityCoordinate
  activation_count
  predicate_result_digest
  lifecycle: PENDING | CONSUMED
  idempotency_key
}
```

Antes de publicar um `CycleCommit`, o coordinator aplica o lote validado a uma cópia de trabalho,
avalia nela os predicates cujas dependências foram tocadas e materializa, **na mesma transação do
commit**, as mudanças de `TriggerRuntimeState` e cada `TriggerActivation` exigida pela transição. A
ativação recebe coordenada `>= commit_successor_floor(commit)` da §3.2.1 e identidade derivada de
`(run_id, trigger_definition_id, trigger_version, result_revision, activation_count)` pela §2.2.
`causing_event_ids[]` usa a ordem canônica do `EventBatch`. No bootstrap, o genesis executa a mesma
função sobre o snapshot inicial e persiste runtime + ativações iniciais no manifesto/eventos de
genesis, com identidade e coordenada declaradas; não há janela não registrada.

Uma ativação `PENDING` participa do próximo fence elegível como `TriggerActivationRef`. Só o
`CycleCommit` que registra seu `DecisionRecord` a move para `CONSUMED`; abort não a consome. O handler
puro da ativação admitida produz um `CommitCandidate` ou um candidato que cria uma
`ScheduledOccurrence`. Ele não grava ocorrência, runtime ou mundo diretamente. `REJECT` consome a
ativação sem efeito de domínio; `DEFER` a consome e cria sucessor de identidade/coordenada novas pelas
regras da §7.

`REPEAT_WHILE_TRUE` ativa uma vez na entrada em verdadeiro e sua ativação propõe agendar a próxima
verificação para `current_instant + repeat_every`. Se a condição ficar falsa antes disso, a ocorrência
pendente é cancelada por evento de lifecycle. No vencimento o predicate é revalidado contra o snapshot
do ciclo; falso consome/cancela sem produzir o efeito de domínio. Se condição e vencimento mudarem no
mesmo ciclo, ambos leem o mesmo snapshot e o resultado pertence à semântica simultânea daquele lote.

Uma ativação admitida produz um `CommitCandidate` diretamente ou propõe agendar uma ocorrência. Ela
não cria uma `ActionProposal` fictícia. Assim, início de aula, prazo expirado, pagamento periódico e
entrega atrasada permanecem fatos determinísticos sem ator artificial. Se o processo cai depois do
commit que tornou um edge verdadeiro, a ativação já está no ledger; resume a admite uma vez, sem
reavaliar host state para recriá-la e sem poder perdê-la ou duplicá-la.

### 5. `ActionProposal`

É um input imutável e neutro de domínio:

```text
ActionProposal {
  proposal_id
  dispatch_id
  run_id + cycle_id
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
  response_digest             // unit_digest da SlotResponse
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
freeze base revision; derive the provisional CyclePlan from the smallest pending coordinate
  → await the closure condition for closure_coordinate(C, CyclePlan)
  → derive DecisionCohort and admitted units from base_revision + ledgers
  → open one SlotDispatch per slot with no response and no open dispatch (after the §10.1 barrier);
      admit the unique response per slot (§3.2.5)
  → record the AdmissionFence durably
      and conditionally confirm CyclePlan + closure proof + cohort + canonical admitted_units + input_digest
  → validate scoped idempotency identities; abort on key/content conflict
  → deduplicate semantically identical admitted units (canonical survivor; aliases settle as DEDUPLICATED)
  → expand occurrences, trigger activations and exogenous inputs into CommitCandidates
  → validate proposals against the frozen state
  → build CommitCandidates
  → validate that candidate source sets partition exactly the candidate-producing admitted units
  → detect ConflictSets from declared reads/writes/resources/invariants
  → resolve with explicit versioned policies and named randomness
  → validate the candidate EventBatch and working post-state
  → derive trigger runtime transitions/TriggerActivation into the EventBatch
  → persist every final EventOrderKey; validate ids, total order, LogicalSequence, batch digest
      and final post-state; derive PerceptionTask records from the canonical events
  → atomically append exactly one terminal DecisionRecord per admitted unit
      + events + PerceptionTasks + CycleCommit + new WorldRevision
  → on any failure: atomically append a CycleAbortRecord;
      no DecisionRecord, no events, no PerceptionTask, no revision, no ordinal, no consumption
```

`ScheduledOccurrence`/`Trigger` e `ActionProposal` são dois ramos que convergem em
`CommitCandidate`. Nenhum ramo tem permissão especial para mutar o mundo antes do outro. Um candidato
declara:

- `candidate_id`, `source_unit_ids[]` não vazio e na ordem do fence, `producer + producer_version`,
  `candidate_role` e `candidate_local_ordinal` definidos pelo schema do produtor;
- unidade atômica e eventos propostos, cada output nomeado por `event_role + event_local_ordinal` no
  schema do produtor;
- recursos consumidos/reservados e chaves de conflito;
- read/write set conservador ou invariantes tocadas;
- provenance de ocorrência, trigger ou proposta;
- prioridade de domínio explícita, se houver;
- política de empate e versão do resolvedor.

`candidate_id = derive_id(COMMIT_CANDIDATE, run_id, cycle_id, source_unit_ids[], producer,
producer_version, candidate_role, candidate_local_ordinal)`. `source_unit_ids[]` usa a ordem do
`AdmissionFence`; papel e ordinal são constantes do schema, nunca posição de append, ordem de
iteração ou ordem em que handlers terminaram. Reutilizar os mesmos componentes com bytes diferentes
de candidato é colisão de identidade e aborta fail-closed.

Depois de retirar `NoProposal` e aliases `DEDUPLICATED`, os `source_unit_ids[]` dos candidatos
precisam formar uma **partição exata** das unidades admitidas restantes: toda unidade que produz
candidato aparece em exatamente um candidato, nenhum candidato referencia unidade fora do fence e
dois candidatos nunca se sobrepõem. Um candidato pode agregar várias unidades somente quando elas
compartilham uma única disposição atômica; cada unidade da partição recebe um `DecisionRecord`
próprio apontando ao mesmo `candidate_id`, com a mesma disposition, reason/conflict refs,
`produced_event_ids[]` e, em `DEFER`, o mesmo sucessor causal. Se uma unidade precisaria de outcomes
diferentes, o produtor deve modelá-los como unidades causais distintas antes do fence; não pode
fatiar uma unidade em candidatos concorrentes. Gap, overlap, ref externa ou combinação incompatível
é `PROVENANCE_FAILURE` antes de detectar conflitos e encerra a tentativa em `CycleAbortRecord`, sem
settlement parcial.

Antes de atribuir `LogicalSequence`, o coordinator converte todo output em `EventDraft` com uma chave
independente de payload e de ordem de construção:

```text
EventOrderKey {
  phase_tag                    // 00=TEMPORAL_ADVANCE, 01=CANDIDATE_DOMAIN,
                               // 02=SOURCE_LIFECYCLE, 03=TRIGGER_RUNTIME,
                               // 04=TRIGGER_ACTIVATION
  origin_ref                   // candidate_id; ou unit/round/trigger ref canônica na fase derivada
  producer + producer_version
  event_local_ordinal + event_role
}
```

A política `event_order_policy_version + event_order_policy_hash`, imutável na configuração da run,
fixa tags, schemas e comparação byte a byte. Dentro de um candidato, `event_local_ordinal` respeita a
ordem causal declarada pelo produtor; entre candidatos, `origin_ref=candidate_id` dá a ordem total.
Eventos de avanço usam como origem o `CLOCK_ADVANCE(run_id, from, to, cycle_id)` derivado do
`cycle_plan` persistido (§3.3); eventos de settlement/lifecycle usam a
unidade ou round liquidado como origem; mudanças de runtime e criação de ativações usam
`trigger_definition_id + trigger_version + activation_count`. Duas drafts com a mesma chave são
corrupção. O `EventBatch` é a ordenação lexicográfica total por
`EventOrderKey`; `event_id = derive_id(EVENT, run_id, cycle_id, EventOrderKey)` e
`LogicalSequence` é atribuído somente depois dessa ordenação. Assim eventos candidatos e derivados
compartilham uma única regra, sem depender da coleção retornada pelo resolvedor.

Dois candidatos sem conflito podem ambos vencer. Em conflito, o resolvedor produz uma disposição
determinística (`COMMIT`, `REJECT`, `DEFER`) para cada candidato. Empate estocástico só usa substream
nomeado. Unidades admitidas que não geram candidato — slot com `NoProposal` e entrada removida pela
deduplicação — recebem desfecho terminal próprio (`NO_PROPOSAL`, `DEDUPLICATED`, §7.1) na mesma
transação; para as demais, a disposição do único candidato da partição é projetada em exatamente um
registro por `source_unit_id`. Nenhuma unidade admitida sai do ciclo sem desfecho.

Toda disposição liquida a fonte do candidato no mesmo commit atômico. Nenhuma delas pode deixar a
fonte pendente no instante já avaliado:

- `COMMIT`: os eventos do candidato entram no lote; ocorrência ou `TriggerActivation` vai a
  `CONSUMED` e a proposta recebe disposição no decision ledger — as transições de agenda/trigger
  entram como eventos de lifecycle no mesmo `EventBatch`; novas ativações causadas pelo post-state
  são artefatos distintos, criados atomicamente para ciclo posterior conforme a §4.2;
- `REJECT`: nenhum evento de domínio do candidato entra no lote, mas os eventos de lifecycle da fonte
  entram. Ocorrência vencida ou `TriggerActivation` vai a estado terminal — `CONSUMED` quando foi
  avaliada e descartada, `CANCELLED` quando a política da ocorrência a retira — e a rejeição fica no
  decision ledger com `reason_code`. Assim nenhuma fonte continua `PENDING` no instante corrente e
  nenhum trigger redispara a partir de estado velho;
- `DEFER`: a fonte antiga também vai a estado terminal e o mesmo commit cria a nova ocorrência ou o
  novo input causal, com identidade nova, provenance para a fonte anterior e **coordenada de
  elegibilidade explícita**, estritamente maior que a do ciclo corrente — `(instante_futuro, 0)` para
  adiamento temporal, `commit_successor_floor(commit) = (current_instant, cycle_ordinal + 1)` para
  adiamento de mesmo instante em ciclo comum, contando para o limite de cascata. A coordenada é
  gravada na mesma transação que o `CycleCommit`,
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
  candidate_id?               // obrigatório em COMMIT | REJECT | DEFER; ausente nos demais
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
auditoria/replay distinguem “slot sem proposta” e “duplicata descartada” de “fonte omitida por bug”.

Antes da expansão, toda unidade que usa `idempotency_key` é vinculada a uma identidade persistente:

```text
IdempotencyIdentity {
  run_id
  unit_kind                    // tag canônica: EXOGENOUS_INPUT | ACTION | OCCURRENCE | TRIGGER ...
  producer_scope               // source_id exógeno ou source_id derivado canônico
  actor_scope                  // actor_id quando a unidade tem ator; NONE nos demais casos
  idempotency_key
  idempotency_digest
  idempotency_policy_version + idempotency_policy_hash
}
```

O namespace de uma chave é `(run_id, unit_kind, producer_scope, actor_scope, idempotency_key)`. Logo,
tipos, fontes ou atores distintos nunca colidem por reutilizarem a mesma string.
`idempotency_digest` é diferente do `unit_digest` do fence: ele representa a operação lógica e
exclui toda identidade de entrega (`producer_unit_key`, `input_id`, `ingress_seq`, ids de tentativa de
transporte, `EligibilityCoordinate` normalizada pelo ledger e telemetria), mas inclui tipo/schema,
fonte, ator, payload, `declared_effective_at`/`due_at` sem normalização e provenance **causal** como
dispatch, evento/claim de origem e predecessor de `DEFER`. Assim uma reentrega posterior ao
`SourceClosure` não muda de operação apenas porque recebeu outra coordenada de admissão. Campos
incluídos e excluídos, sua ordem e normalização pertencem a `idempotency_policy_version + hash`. Duas
entregas são aliases somente quando namespace, versão/hash da política e esses bytes lógicos são
iguais; usar o mesmo `idempotency_key` declara que representam a mesma operação, não dois atos
intencionais iguais.

O ledger mantém esse vínculo durante toda a run. Reentrega com a mesma `producer_unit_key` reutiliza
o mesmo `input_id` e não cria segunda unidade. Chaves de produtor distintas com o mesmo namespace e
`idempotency_digest` são aliases. Se ainda não existe unidade canônica liquidada, a primeira pela
ordem canônica da §3.2.4 no primeiro fence que contém a identidade é a sobrevivente e as demais
recebem `DEDUPLICATED`. Se a canônica já foi liquidada em ciclo anterior, toda nova identidade de
entrega continua sendo unidade causal normal: recebe sua coordenada persistida, entra no primeiro
fence elegível e recebe seu próprio `DecisionRecord(DEDUPLICATED, canonical_unit_id)`; nunca é
consumida ou retirada de `pending_ledgers` no append do `InputLedger`. O recibo terminal anterior é
apenas o alvo canônico do novo settlement, não substituto para ele.
Reuso do mesmo namespace com `idempotency_digest` diferente é `IDEMPOTENCY_CONFLICT`, nunca
duplicata: o conflito é registrado com as duas referências, a tentativa termina em
`CycleAbortRecord` antes de gerar candidatos, e nenhuma unidade do fence corrente é consumida nem
recebe `DecisionRecord`; eventual settlement histórico da unidade canônica permanece imutável. O
registro append-only não escolhe conteúdo por hash, id, ordem de chegada ou latência. Slots continuam
sob a unicidade mais forte de `(decision_round_id, slot_id, dispatch_id)` da §3.2.5; uma chave
arbitrária de proposta não contorna esse vínculo.

- `COMMIT`, `REJECT`, `DEFER`: disposições do resolvedor sobre candidatos, com a liquidação da fonte
  descrita na §7;
- `NO_PROPOSAL`: o slot foi respondido com `NoProposal`; nenhum candidato existiu, nenhum evento de
  domínio é produzido, o slot é consumido e o round vai a `CONSUMED` por evento de lifecycle quando
  todos os seus slots estão liquidados;
- `DEDUPLICATED`: a unidade compartilha a mesma `IdempotencyIdentity`, inclusive
  `idempotency_digest`, com a unidade canônica da operação. A canônica é a sobrevivente escolhida no
  primeiro fence da identidade e pode pertencer ao fence corrente ou a um ciclo anterior; o registro
  do alias aponta para ela em `canonical_unit_id`. O alias é sempre admitido e consumido pelo seu
  próprio `CycleCommit`, sem candidato e sem evento; a disposição de fato está no registro da
  canônica. Mesmo `idempotency_key` com namespace ou digest diferente nunca chega a este desfecho.

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
   trigger, inclusive criação das `TriggerActivation` derivadas do post-state;
4. o `CycleCommit`, gravado uma única vez no commit journal canônico do `DecisionLedger`, que
   referencia `fence_digest`, `decision_record_ids[]`, `decision_digest`, `perception_task_ids[]` e
   `perception_task_digest`;
5. uma `PerceptionTask` na causal outbox para cada evento potencialmente perceptível, com ids na
   mesma ordem canônica de `event_ids[]`;
6. a publicação da nova `WorldRevision`.

Ou as seis partes persistem, ou nenhuma persiste. Não existe outbox de decisão nem tarefa de percepção
que possa ficar para trás ou adiantar-se ao mundo: settlement e criação do trabalho epistemológico
não são passos posteriores ao commit. Isso impõe um requisito de persistência, não uma escolha de
banco: event store, decision ledger e causal outbox precisam compartilhar um domínio transacional, ou
ser unidos por um commit determinístico chaveado por `fence_digest` que nenhum ciclo posterior,
replay, resume ou barrier possa ultrapassar sem reconciliar. Este ADR não escolhe a tecnologia, mas
exclui topologias em que essas escritas possam divergir. Antes de publicar, reducers são aplicados a
uma cópia de trabalho e todas as invariantes afetadas são verificadas. Falha em evento, reducer,
schema, provenance ou invariante aborta o lote inteiro e leva
ao envelope de abort da §8.0.1. Não há commit parcial de uma ação multi-evento, event log adiantado
em relação ao state store, nem disposição gravada sem o mundo correspondente.

### 8. Eventos e ledgers

#### 8.0 Envelope de commit do ciclo

Todo ciclo fechado com sucesso persiste uma única vez no **commit journal append-only do
`DecisionLedger`** um envelope do seu lote, **inclusive quando o lote tem zero eventos**. O event
store não contém cópia desse envelope; contém somente os `Event` referenciados por ele:

```text
CycleCommit {
  run_id
  cycle_id
  attempt_ordinal
  instant
  cycle_ordinal
  cycle_plan
  base_revision + base_state_hash
  result_revision + result_state_hash
  fence_digest
  admitted_input_ids[]
  input_digest
  cycle_coordinate_policy_version + cycle_coordinate_policy_hash
  event_order_policy_version + event_order_policy_hash
  event_ids[]                // pode ser vazio
  decision_record_ids[]      // exatamente um por unidade admitida, na mesma ordem canônica
  decision_digest
  perception_task_ids[]      // uma tarefa por evento perceptível, na ordem de event_ids[]
  perception_task_digest
  perception_policy_version + perception_policy_hash
  perception_identity_policy_version + perception_identity_policy_hash
  next_logical_sequence
  batch_digest
}
```

O `cycle_plan` do commit é byte a byte igual ao do `AdmissionFence` referenciado; divergência rejeita
a transação. `batch_digest` cobre `canonical_bytes_v1` do `CycleCommit` inteiro exceto o próprio
digest e, para cada `event_id` na ordem gravada, os bytes canônicos do `Event` referenciado — inclusive
sua `event_order_key`. Portanto ele compromete plano, envelopes de estado/settlement/percepção, ordem
e conteúdo do lote sem depender de estado transitório do produtor.

Um ciclo em que todos os slots produziram `NoProposal`, ou em que todo candidato foi rejeitado sem
gerar evento de lifecycle, ainda avança `WorldRevision` e `cycle_ordinal`. O envelope é o registro que
torna esse avanço reconstruível: o replay causal percorre a sequência canônica de `CycleCommit` no
commit journal e reaplica, na ordem de `event_ids[]`, os eventos referenciados no event store,
reproduzindo revisão vazia, ordinal e `next_logical_sequence` sem precisar de evento decorativo nem de
um snapshot do coordinator. Ausência, duplicação ou divergência entre uma referência do commit e o
evento correspondente falha fechado; uma view que combine os dois stores é projeção descartável, não
segunda cópia autoritativa. O envelope é também o recibo de settlement:
`fence_digest` e `input_digest` provam qual corte e quais bytes foram avaliados;
`decision_record_ids[]`/`decision_digest` provam a bijeção da §7.1 — toda unidade admitida por aquele
corte, inclusive slots `NoProposal` e duplicatas, recebeu um desfecho terminal na mesma transação.
`perception_task_ids[]` e seu digest provam, sob a
`perception_policy_version + hash` e a política de identidade epistemológica, a correspondência
ordenada entre todo `event_id` potencialmente perceptível e sua tarefa já persistida; ausência,
tarefa extra ou digest divergente falha fechado.
Tentativa abortada não produz `CycleCommit`, não publica revisão e não avança o ordinal do instante;
ela produz o envelope da §8.0.1.

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
  cycle_plan
  base_revision + base_state_hash
  fence_digest
  input_digest
  abort_reason: INDETERMINATE_FACET | INVARIANT_VIOLATION | REDUCER_FAILURE
              | SCHEMA_FAILURE | PROVENANCE_FAILURE | IDEMPOTENCY_CONFLICT | CASCADE_LIMIT
  indeterminate_records[]    // facet, reason_code, evidence_refs, rule_version
  failure_evidence_refs[]    // material provisório da tentativa: facets, ConflictSets, sorteios,
                             // disposições provisórias, erro de reducer/invariante
  rule_versions
  abort_digest
}
```

O `cycle_plan` do abort também precisa ser byte a byte igual ao do fence, e `abort_digest` cobre a
serialização canônica do record inteiro exceto o próprio digest. Assim commit, abort e resume
concordam sobre o tipo e, quando aplicável, o destino da tentativa sem consultar seleção transitória.

O envelope de abort cobre **somente evidência de tentativa**. Ele **não** publica `WorldRevision`,
não acrescenta evento ao event store nem `PerceptionTask` à causal outbox, não avança
`cycle_ordinal`, não consome unidade admitida e **não persiste nem referencia `DecisionRecord`**.
Disposições que o resolvedor já havia calculado
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
resume, um fence sem envelope terminal significa crash no meio da tentativa; como nenhuma das seis
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
  gravadas até um envelope terminal. O coordinator reutiliza o `cycle_plan` do fence; em
  `CLOCK_ADVANCE`, não chama `next_cycle_coordinate` nem seleciona outro `P`;
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
nova tentativa grava fence com `to_attempt_ordinal`, `cycle_plan` e membresia idênticos (§3.2.4);
`attempt_ordinal` é contínuo e auditável por ciclo.

#### 8.1 Envelope de `Event`

```text
Event {
  event_id
  event_order_key: EventOrderKey
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

`event_order_key` é a preimagem canônica persistida da identidade e precisa repetir o mesmo
`producer + producer_version` do envelope; ausência ou divergência é corrupção. No replay,
`event_id` é recalculado de run/ciclo/`event_order_key` exatamente pelas políticas de identidade e
ordem das §§2.2 e 7; os eventos referenciados por `event_ids[]` são reordenados pelas chaves
persistidas, e a ordem resultante precisa ser byte a byte igual à lista do `CycleCommit`.
`logical_sequence` precisa ser a sequência contígua atribuída por essa ordem a partir do cursor de
entrada do commit. Chave duplicada, id, posição ou sequência divergente falha fechado antes de reduzir
o mundo. O `CycleCommit` compromete a versão/hash da política e `batch_digest` compromete os envelopes
completos, incluindo as chaves. Pais causais precisam existir e formar DAG, e só
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

#### 8.2 Cinco artefatos, cinco responsabilidades

- **Input ledger:** dois registros com admissão distinta. (i) Entradas de fontes exógenas
  declaradas — inputs tipados e `SourceClosure` —, sujeitas a `ingress_seq`, normalização de ingresso
  e condição de fechamento. (ii) O registro de slot — `SlotDispatch`, `SlotDispatchRevocation` e a
  única `SlotResponse` por slot (`ActionProposal`, `NoProposal`) —, admitido por escrita condicional
  contra o dispatch aberto, sem coordenada própria e sem fechamento (§3.2.5). A auditoria de
  respostas rejeitadas fica fora do digest causal. Uma futura intervenção de usuário só poderá entrar
  aqui por contrato próprio, nunca pelo Observatory;
- **Decision ledger:** `AdmissionFence`, material provisório de avaliação (assessments, conflitos,
  sorteios, disposições provisórias), `AttemptRetryRecord` e o **único commit journal canônico** dos
  envelopes terminais — `CycleCommit` com seus `DecisionRecord` e `CycleAbortRecord`. `CycleCommit`
  não é duplicado no event store. `DecisionRecord` é autoritativo somente quando coberto pelo
  `decision_digest` de um `CycleCommit`; `CycleAbortRecord` é autoritativo para o fato do abort e sua
  evidência, nunca para settlement;
- **Event store:** somente `Event` imutável do mundo commitado, com `EventOrderKey` persistida,
  identificado e referenciado pelo `event_ids[]` do `CycleCommit`; é a autoridade dos fatos para
  redução de `WorldState`, não journal de commits nem autoridade de revisão/ordinal;
- **Evidence ledger:** observations e recibos `KnowledgeInput`, sempre particionados por destinatário;
  é a trilha epistemológica, não world truth nem belief inferida;
- **Causal outbox:** `PerceptionTask` criada atomicamente com o `CycleCommit` e completion receipts
  append-only; coordena projeção epistemológica idempotente a partir de `Event`, sem copiar payload de
  world truth para contexto de agente e sem autoridade para criar ou alterar fatos do mundo.

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

Fora da materialização determinística de runtime/`TriggerActivation` exigida pela §4.2, reações que
possam criar novos fatos pertencem a handlers de unidades admitidas em ciclo posterior, com links
causais. Projectors constroem índices/views descartáveis e não são fonte de verdade. Uma
invariante que envolve mais de um subestado é verificada pelo commit coordinator; isso não autoriza um
“generic world service” a decidir regras de domínio.

O hash de estado usa serialização canônica, schemas e versões registradas. Maps não dependem da ordem
de iteração; números de domínio têm representação determinística; campos operacionais não entram no
hash.

### 10. Percepção, claims e comunicação

#### 10.1 `Observation`

No commit, cada evento que possa gerar percepção cria uma tarefa determinística em causal outbox,
referenciando tanto `base_revision` quanto `result_revision`:

```text
PerceptionTask {
  task_id                         // derive_id(PERCEPTION_TASK, run_id, event_id,
                                  //           perception_policy_version + hash,
                                  //           resolver_id + version + hash,
                                  //           perception_identity_policy_version + hash)
  run_id + cycle_id + event_id
  base_revision + result_revision
  perception_policy_version + perception_policy_hash
  resolver_id + resolver_version + resolver_hash
  perception_identity_policy_version + perception_identity_policy_hash
  lifecycle: PENDING              // completion é record append-only separado
}

PerceptionTaskCompletion {
  completion_id                   // derive_id(PERCEPTION_COMPLETION, task_id)
  task_id
  observation_ids[]               // ordem normativa descrita abaixo
  knowledge_input_ids[]           // ordem normativa descrita abaixo
  completion_digest
}
```

`PerceptionTask` não contém payload irrestrito nem lista pré-computada de observadores e só é legível
pelos componentes de percepção; um agente nunca a recebe em contexto. Ela é a referência de trabalho
para o `PerceptionResolver`, que carrega o evento pelo port autorizado e avalia acesso por evento e
observador potencial. O adapter versionado do `event_type` declara se cada pista depende do estado
anterior, do resultado ou da transição; assim, uma pessoa que sai de uma sala não desaparece antes que
os presentes possam vê-la sair. O resultado é imutável:

```text
Observation {
  observation_id
  task_id
  observer_id
  observation_role + observation_local_ordinal
  observed_at
  source_event_refs[]
  modality + channel_ref?
  percepts[]
  claim_refs[]
  omissions/redactions
  evidence_chain[]
  resolver_id + resolver_version + resolver_hash
}
```

A política imutável `perception_identity_policy_version + perception_identity_policy_hash` define:

```text
observation_id = derive_id(OBSERVATION, task_id, observer_id,
                           observation_role, observation_local_ordinal)
knowledge_input_id = derive_id(KNOWLEDGE_INPUT, observation_id, recipient_id, kind,
                               input_role, input_local_ordinal)
completion_id = derive_id(PERCEPTION_COMPLETION, task_id)
```

`observation_role`/`input_role` e seus ordinais são saídas nomeadas pelo schema versionado do
resolver, não posições de iteração. Um resolver pode produzir mais de uma observation para o mesmo
ator somente com pares papel/ordinal distintos. `Observation` no completion é ordenada por
`(observer_id, observation_role, observation_local_ordinal, observation_id)`; `KnowledgeInput`, por
`(recipient_id, kind, input_role, input_local_ordinal, knowledge_input_id)`, sempre em bytes
canônicos. Mesmos componentes com bytes diferentes são colisão/corrupção e falham fechado; workers
concorrentes calculam os mesmos ids e apenas uma escrita idempotente vence. Versão sem hash
disponível, ou mudança de resolver sem nova versão/hash, também falha fechado.

`percepts` são descrições estruturadas do que ficou disponível aos sentidos, possivelmente parciais
ou ruidosas; não são cópia irrestrita do payload secreto. Atenção, interpretação psicológica,
confiança e inferência ficam para Cognition/Knowledge. Randomness perceptiva, quando existir, usa
substream nomeado e o resultado fica persistido para replay.

Outbox, `Observation` e `KnowledgeInput` usam ids determinísticos e entrega idempotente. O completion
só é anexado por compare-and-set depois que todos os `Observation` e `KnowledgeInput` referenciados
estão duravelmente presentes no evidence ledger; completion vazio é obrigatório quando nenhum
observador é elegível. Se houver crash durante a entrega, resume relê as tarefas commitadas sem
completion, reinsere os mesmos ids idempotentemente e só então completa a tarefa. Referência ausente
ou digest divergente falha fechado.

Um barrier do ciclo consulta a causal outbox durável — nunca uma fila em memória — e impede
**despachar** a solicitação de um round já declarado para o destinatário enquanto existir tarefa
causalmente anterior sem completion ou `KnowledgeInput` endereçado ainda não confirmado. Antes de
liberar o barrier, o coordinator reconcilia `CycleCommit.perception_task_ids[]` com a outbox e
`PerceptionTaskCompletion` com o evidence ledger. A `RoundDeclaration`, sua coordenada e sua
pertinência à coorte não mudam por causa do barrier. Assim, crash depois do commit e antes da
percepção encontra a tarefa já persistida e retoma o trabalho sem perder nem duplicar conhecimento
causal.

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
  knowledge_input_id
  input_role + input_local_ordinal
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
+ mesmas versões/hashes de schema/reducer/validator/resolver/RNG/fence, identidade causal,
  admission-order, coordenada de ciclo, ordem de eventos, percepção/identidade epistemológica e
  idempotency policy
+ mesmo world seed
→ mesma DecisionCohort e mesmo AdmissionFence derivados por ciclo (fence_digest verificável)
→ mesmo CyclePlan persistido do fence ao envelope terminal
→ mesmo input digest e mesmo decision_digest terminal
→ mesmas EventOrderKey persistidas e mesmo event log causal byte a byte (batch_digest verificável)
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
- fila de `ScheduledOccurrence`, estado runtime de triggers e `TriggerActivation` pendentes;
- cursores/digests do input, decision, event, evidence e causal-outbox ledgers;
- `CycleControlState` (§8.0.2) com referência ao último envelope terminal e, conforme o estado, ao
  `AdmissionFence` sem envelope terminal (`ATTEMPT_IN_FLIGHT`) ou ao `AttemptRetryRecord` ainda sem
  fence (`RETRY_AUTHORIZED`), com o `attempt_ordinal` correspondente;
- cursor do ledger e último `SourceClosure` de cada fonte exógena declarada, inclusive `final`,
  preservando `closure_id`/`ingress_seq` para recomputar `covering_closure(source_id, C)`;
- fila de `RoundDeclaration` pendentes e, para seus slots, dispatches abertos ou revogados e a
  resposta já gravada, quando houver;
- coordenadas de elegibilidade e `unit_digest` de toda entrada gravada e ainda não consumida;
- schemas e versões/hashes de reducers, validators, resolvers, RNG, ordem de admissão, coordenada de
  ciclo, ordem de eventos, percepção/identidade epistemológica e idempotência necessários;
- `PerceptionTask` sem completion, seus completion receipts e inboxes causais cuja entrega ainda não
  foi confirmada;
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

1. **causal replay:** genesis + sequência de `CycleCommit` do commit journal do `DecisionLedger` +
   `Event` referenciados no event store reconstrói exatamente `WorldState`/hash, `WorldRevision`,
   ordinal do ciclo e `next_logical_sequence`; reduzir somente os eventos reconstrói o conteúdo do
   mundo, mas não substitui o journal nem prova commits vazios. Antes de qualquer reducer, o replay
   valida igualdade do `cycle_plan` entre fence/envelope, `event_id`, ordem e `LogicalSequence` a
   partir das `event_order_key` persistidas, além de `batch_digest`;
2. **resume equivalence:** snapshot + tails dos ledgers produz o mesmo log e estado que a execução
   contínua. Essa prova inclui o estado epistemológico restaurado do checkpoint, nunca regenerado por
   chamada de modelo, e a reconciliação dos quatro estados da §8.0.2. O resume recomputa
   `CycleControlState` pela dobra total e ordenada sobre os ledgers e falha fechado se divergir do
   snapshot; então: `IDLE` abre o próximo ciclo conforme §3.3; `ATTEMPT_IN_FLIGHT` reexecuta a
   tentativa a partir do `cycle_plan` e do corte gravados no fence, sem recalcular `P` nem trocar o
   destino de um `CLOCK_ADVANCE`, antes de qualquer ciclo novo; `RETRY_AUTHORIZED` grava o fence
   de `to_attempt_ordinal` para o **mesmo** `cycle_id`, sem seleção de coordenada e sem segundo
   `AttemptRetryRecord`; `HALTED_ON_ABORT` restaura o run **parado**, com `next_attempt_ordinal`
   preservado, e só um `AttemptRetryRecord` ou fork explícito o move. Slots com dispatch aberto e sem
   resposta seguem a §3.2.5: aguardar ou revogar e redespachar, nunca um segundo dispatch aberto.
   Resume nunca converte `HALTED_ON_ABORT` em `IDLE` nem em retentativa implícita. Antes de qualquer
   novo dispatch, também reconcilia os `perception_task_ids[]` de cada commit com a causal outbox e
   retoma toda tarefa sem completion conforme a §10.1.

O causal replay reconstrói agenda, trigger runtime e `TriggerActivation` porque suas transições são
eventos. O evidence ledger pode ser verificado diretamente ou regenerado deterministicamente a
partir dos eventos e das `PerceptionTask` referenciadas pelos commits, seed e versões; regeneração
nunca consulta um LLM e precisa produzir os mesmos completion receipts.

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
6. Trigger repetível tem rearm/cadência persistida; reavaliação sozinha nunca repete efeito, e toda
   ativação é uma `TriggerActivation` durável antes de produzir candidato.
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
17. Toda transição autoritativa de schedule/trigger é representada no event store; runtime e
    `TriggerActivation` causados por uma revisão são publicados atomicamente no `CycleCommit` dessa
    revisão, com elegibilidade `>= commit_successor_floor(commit)`; avanço temporal usa `(to, 0)` e
    nunca cria trabalho pendente no instante de origem já abandonado.
18. Nenhum ator começa novo round enquanto houver `KnowledgeInput` causalmente anterior pendente para
    ele.
19. Contexto de agente é allow-list por holder/timeline/provenance; não é uma view redigida do estado
    global.
20. Toda unidade causal é admitida por um `AdmissionFence` durável, derivado dos ledgers e gravado
    antes de qualquer avaliação; membresia é função da coordenada de elegibilidade, dos
    `SourceClosure` gravados e de `base_revision`, nunca da ordem de chegada, do wall clock do
    coordinator nem da ausência momentânea de pendências. Para cada fonte, a prova usa o primeiro
    `SourceClosure` por `ingress_seq` que cobre a coordenada exigida pelo `cycle_plan` — a do ciclo
    em `WORK`, ou a pendência-alvo em `CLOCK_ADVANCE` —, estável sob append; o fence persistido é
    verificável contra a derivação por leitura prefix-consistente e escrita condicional. Nova
    identidade de entrega continua sendo unidade causal mesmo quando é alias idempotente de unidade
    já liquidada: não pode ser consumida nem retirada das pendências antes do próprio fence.
21. `ingress_seq` só participa da normalização de ingresso e da seleção append-stable da prova de
    fechamento da própria fonte. A ordem total de `admitted_units` usa a chave byte a byte e a
    política versionada/hash da §3.2.4; prioridade e desempate nunca derivam de ordem de registro,
    enum local ou iteração de map. Nenhuma derivação escolhe entre respostas de slot, porque o ledger
    nunca contém duas.
22. A `DecisionCohort` de um ciclo é o conjunto derivado das `RoundDeclaration` elegíveis em
    `base_revision`; todos os seus rounds são admitidos no mesmo ciclo, leem a mesma `base_revision`
    e concorrem no mesmo espaço de `ConflictSet`. Latência nunca separa rounds simultâneos nem
    exclui um round da coorte.
23. Todo desfecho terminal (`COMMIT`, `REJECT`, `DEFER`, `NO_PROPOSAL`, `DEDUPLICATED`) é um
    `DecisionRecord` canônico persistido exclusivamente na transação de um `CycleCommit`, junto de
    fence closure, eventos e `WorldRevision`; há exatamente um por unidade admitida e nenhuma fonte
    vencida permanece `PENDING` no instante avaliado. Em `COMMIT|REJECT|DEFER`, o registro aponta ao
    único candidato cuja partição contém a unidade; em `DEDUPLICATED`, aponta à unidade canônica ainda
    que ela pertença a ciclo anterior.
24. Todo fence gravado termina em exatamente um envelope: `CycleCommit` (mesmo com zero eventos) ou
    `CycleAbortRecord` (sem revisão, sem evento, sem `PerceptionTask`, sem avanço de ordinal, sem
    consumo e sem `DecisionRecord`). O envelope repete o `cycle_plan` do fence byte a byte e seu
    digest terminal o cobre.
25. Elegibilidade de readmissão é a coordenada persistida `(not_before_instant,
    not_before_cycle_ordinal)`; `DEFER` de mesmo instante usa ordinal maior, nunca momento de
    inserção.
26. Link causal dentro do mesmo ciclo só existe entre eventos do mesmo `CommitCandidate` atômico.
27. Checkpoint resumível contém ou referencia atomicamente checkpoint epistemológico versionado de
    cada ator elegível.
28. Nenhum ciclo de coordenada `C` deriva membresia ou grava fence enquanto alguma fonte exógena
    declarada não tiver um fechamento não final com
    `closed_through >= closure_coordinate(C, cycle_plan)` ou um fechamento `final`.
    Entrada após fechamento não final recebe coordenada posterior por regra de ingresso; depois de
    `final`, qualquer input ou fechamento da fonte é rejeitado atomicamente.
29. `CycleControlState` é uma dobra total e ordenada sobre fences, envelopes terminais e
    `AttemptRetryRecord`: fence sem envelope ⇒ `ATTEMPT_IN_FLIGHT`; abort com retentativa
    registrada e sem fence ⇒ `RETRY_AUTHORIZED`; abort sem retentativa ⇒ `HALTED_ON_ABORT`; senão
    `IDLE`. Resume restaura `HALTED_ON_ABORT` como run parado; só `AttemptRetryRecord` ou fork
    explícito abre `attempt_ordinal + 1`; `RETRY_AUTHORIZED` reabre o mesmo `cycle_id` na tentativa
    autorizada sem seleção de coordenada; `ATTEMPT_IN_FLIGHT` é reexecutado a partir do fence e de
    seu `cycle_plan`, sem recalcular o alvo temporal. Cada `AttemptRetryRecord` referencia o último
    envelope terminal do run e é consumido por exatamente um fence.
30. A resposta de slot é o preenchimento de uma unidade derivada, não entrada de fonte exógena: não
    tem coordenada própria, não está sujeita a `SourceClosure` nem à normalização de ingresso. Cada
    slot tem no máximo um `SlotDispatch` aberto e no máximo uma `SlotResponse`, admitida por escrita
    condicional contra o dispatch aberto; dispatch e resposta correspondem exatamente em run, ciclo,
    round, slot, ator, revisão e instante, e mismatch é rejeitado antes de preencher o slot.
    Redespacho exige `SlotDispatchRevocation` durável.
31. `idempotency_key` tem namespace por run, tipo, fonte e ator e é comprometida a
    `idempotency_digest`. Apenas aliases da mesma operação lógica podem ser `DEDUPLICATED`; reuso do
    mesmo namespace com digest diferente aborta fail-closed sem `DecisionRecord` nem consumo no
    fence corrente. Toda identidade de entrega distinta entra em um fence e recebe settlement próprio;
    recibo terminal anterior nunca a consome por atalho no `InputLedger`.
32. `CycleCommit` existe uma única vez no commit journal canônico do `DecisionLedger`; o `EventStore`
    contém somente `Event` referenciado. Replay causal lê ambos e nenhuma projeção combinada é fonte
    de verdade.
33. Todo id causal segue a `CausalIdentityPolicy` versionada: input externo exige chave estável do
    produtor e id derivado usa somente origem causal/semântica canônica. UUID aleatório, wall clock,
    append position, `ingress_seq` e ordem de iteração nunca alocam identidade; aliases iguais mas
    distintos preservam ids próprios por chaves de produtor distintas.
34. `fence_digest` cobre bytes canônicos de toda a topologia do `AdmissionFence`: provas ordenadas por
    fonte, rounds por id, pares slot/resposta por slot, `admitted_units` como pares
    `(unit_id, unit_digest)`, `input_digest`, `cycle_plan` e maps/set-like collections por chave
    normativa. Mesmo id com bytes diferentes falha antes da avaliação; mesmos conjuntos e políticas
    produzem os mesmos bytes independentemente da ordem local de registro ou iteração.
35. Uma transição de predicate que ativa trigger cria exatamente uma `TriggerActivation` `PENDING`
    na mesma transação da revisão de origem; o `DecisionRecord` da ativação é a única forma de
    consumi-la. Crash/replay não recria, perde nem duplica ativação a partir de memória do processo.
36. Todo evento potencialmente perceptível possui `PerceptionTask` criada na mesma transação do
    `CycleCommit` e referenciada por ele. Completion só existe depois dos recibos idempotentes no
    evidence ledger; barrier e resume reconciliam commits, tarefas, completions e entregas antes de
    despachar novo round. A tarefa não é conteúdo de agente e nenhuma rota concede observação sem
    avaliação de acesso endereçada. Task, observation, `KnowledgeInput` e completion derivam ids dos
    componentes normativos da §10.1; concorrência/retry nunca alocam nova identidade.
37. Todo `CommitCandidate` tem identidade derivada de unidades-fonte e papel/ordinal de schema. Todo
    evento persiste sua `EventOrderKey`; `EventBatch`, `event_id` e `LogicalSequence` usam a única
    ordem total da §7, versionada e independente da ordem em que candidatos/workers terminam. Replay
    recalcula id, ordem e sequência a partir dessas preimagens, e `batch_digest` cobre os envelopes.
    Os conjuntos
    `source_unit_ids[]` formam partição exata das unidades produtoras; overlap, gap ou referência fora
    do fence aborta antes da resolução, e cada unidade aponta a exatamente um candidato.
38. A próxima coordenada é função total da §3.3: lacunas de ordinal são saltadas sem commit vazio; o
    ciclo de avanço pertence ao instante de origem, tem input vazio e reseta o destino para ordinal
    `-1`. Seu `CyclePlan` persiste `from`, `to` e a pendência-alvo desde o fence até o envelope
    terminal. Outputs derivados pelo avanço têm piso `(to, 0)`; depois de materializá-los, a menor
    pendência é recalculada e abre exatamente na sua coordenada, nunca numa coordenada vencida.

## Cenários de stress e provas de aceite

| Cenário | Prova exigida |
|---|---|
| Duas respostas LLM chegam em ordens opostas | mesmo `decision_round_id` + `base_revision`; log e vencedor idênticos |
| Duas `RoundDeclaration` R1 e R2 com a mesma coordenada; R1 completa antes de o coordinator “ver” R2 | a coorte é derivada de `base_revision`, onde R1 e R2 já existem; ambos entram na mesma coorte e no mesmo ciclo em qualquer execução; mesma `base_revision`; seus candidatos disputam o mesmo `ConflictSet`; qual completou primeiro não muda nada |
| Round e input exógeno declaram o mesmo `effective_at` | o fence deriva de `base_revision` mais o `SourceClosure` da fonte do input; membresia, `ConflictSet`, vencedor e log não mudam com a ordem de chegada |
| Dois inputs exógenos E1 e E2, com `producer_unit_key` estáveis, gravados antes do mesmo `SourceClosure` em ordens opostas | `input_id` deriva das chaves do produtor, não do append; mesmos `admitted_input_ids`, mesmo `fence_digest`, mesmo log; `ingress_seq` não entra em identidade nem ordenação |
| Record admitido é relido com o mesmo `unit_id`, mas um byte causal diferente | `unit_digest` diverge do par persistido em `admitted_units`; `input_digest`/`fence_digest` não validam e resume falha fechado antes de avaliar qualquer candidato |
| Execução A grava E1, o coordinator deriva o fence, E2 chega; execução B grava E1 e E2 antes do fence | impossível divergir: sem `SourceClosure` cobrindo a coordenada o coordinator ainda não derivou nada; com o fechamento, E2 gravado depois dele recebe coordenada posterior por regra em ambas as execuções. Mesmo ledger ⇒ mesmo fence |
| Coordinator grava o fence cedo ou tarde (host rápido/lento, reinício) | `fence_digest` idêntico; replay recomputa a derivação e verifica; só telemetria muda |
| Fonte grava `close(C)` e depois `close(C+10)`; um coordinator deriva entre os dois e outro depois | ambos usam o primeiro fechamento por `ingress_seq` que cobre `C`; mesmos `closure_ref` e `fence_digest` |
| Input exógeno gravado depois do `SourceClosure` da própria fonte | não é inserido retroativamente; recebe `open_coordinate` por regra de ingresso e é admitido pelo primeiro fence posterior que o torne elegível, com o valor declarado preservado como provenance |
| Fonte grava `SourceClosure(final=true)` com `closed_through=C` e o relógio avança além de C | o marcador satisfaz todo fechamento futuro; qualquer input ou novo fechamento dessa fonte é rejeitado atomicamente; nenhum deadlock por `closed_through` finito |
| Candidato de ocorrência vencida é rejeitado | ocorrência termina no mesmo commit; nenhum `PENDING` remanescente e nenhum redisparo a partir de estado velho |
| Ciclo em que todos os slots respondem `NoProposal` | `CycleCommit` vazio persistido com um `DecisionRecord(NO_PROPOSAL)` por slot; bijeção `admitted_input_ids ↔ decision_record_ids`; replay reproduz revisão, ordinal e `next_logical_sequence` |
| Duas entregas usam `producer_unit_key` distintas, mas mesmo tipo/fonte/ator, `idempotency_key` e bytes lógicos | `input_id`/`unit_digest` distintos preservam cada entrega; `idempotency_digest` igual prova a mesma operação lógica; no mesmo fence, a primeira pela ordem total recebe a disposição e o alias recebe `DecisionRecord(DEDUPLICATED, canonical_unit_id)` |
| Alias da mesma operação é reentregue depois de a canônica ter sido liquidada e recebe coordenada de admissão posterior | `unit_digest` reflete a nova coordenada, mas `idempotency_digest` usa o tempo declarado e permanece igual; a nova identidade continua pendente, entra no primeiro fence elegível e recebe seu próprio `DecisionRecord(DEDUPLICATED)` apontando à canônica do ciclo anterior; só esse commit a consome |
| Mesmo namespace de `idempotency_key` é reutilizado com payload, tempo efetivo declarado/due time ou provenance causal diferente | `IDEMPOTENCY_CONFLICT`; `CycleAbortRecord` referencia a unidade corrente e a canônica/conflitante; nenhuma unidade do fence corrente recebe `DecisionRecord` ou é consumida, e settlement histórico permanece imutável; só a coordenada normalizada pelo ledger pode diferir sem mudar a operação |
| Faceta sem regra, dado ou resolvedor disponível | `INDETERMINATE` canônico dentro de um `CycleAbortRecord`; nenhum `CycleCommit`, nenhum `DecisionRecord`, nenhuma revisão publicada, nenhum avanço de ordinal |
| Candidato A já tem `COMMIT` provisório quando B encontra `INDETERMINATE` | `CycleAbortRecord` sem nenhum `DecisionRecord`; o `COMMIT` de A é material provisório não autoritativo; A continua elegível e é reavaliado na tentativa seguinte junto com B |
| Crash entre append de eventos, settlement, criação da outbox e publicação da revisão | impossível: as seis partes são uma transação; fence sem envelope terminal é reexecutado a partir do corte gravado |
| Resume encontra fence gravado sem `CycleCommit` nem `CycleAbortRecord` | `CycleControlState = ATTEMPT_IN_FLIGHT`; tentativa reexecutada com o mesmo `fence_digest` e as respostas já registradas; mesmo resultado, sem duplicar mundo nem decisão |
| Resume encontra fence de avanço sem envelope terminal depois de o ledger pendente receber novos appends | usa o `cycle_plan` coberto por `fence_digest`, preserva `from`, `to` e `target_coordinate` e não chama `next_cycle_coordinate`; appends permitidos depois dos fechamentos têm coordenada posterior e não trocam retroativamente o alvo |
| Snapshot tirado depois de um `CycleAbortRecord` | `CycleControlState = HALTED_ON_ABORT` com `next_attempt_ordinal`; resume restaura o run parado, não abre ciclo nem retenta; um `AttemptRetryRecord` explícito leva a `RETRY_AUTHORIZED` e o fence seguinte abre `attempt_ordinal + 1` com membresia idêntica |
| Snapshot ou crash entre o `AttemptRetryRecord` e o novo fence | dobra devolve `RETRY_AUTHORIZED` (`cycle_id`, `to_attempt_ordinal`, `retry_ref`), nunca `HALTED_ON_ABORT` nem `IDLE`; resume grava o fence de `to_attempt_ordinal` para o mesmo `cycle_id`, sem §3.3 e sem segundo `AttemptRetryRecord`; `next_attempt_ordinal` continua `N + 1` até o fence existir |
| Fence de `N + 1` gravado sem envelope enquanto o último envelope do run é o abort de `N` | regra 1 prevalece: `ATTEMPT_IN_FLIGHT`; o abort de `N` foi consumido pelo `AttemptRetryRecord` em `retry_ref`; se `N + 1` abortar, a regra 3 devolve `HALTED_ON_ABORT` com `next_attempt_ordinal = N + 2` |
| Timeout grava `NoProposal(s1)` e a `ActionProposal(s1)` real chega antes de o round fechar | ambas apontam para o mesmo dispatch aberto; a escrita condicional admite exatamente uma e rejeita a outra para auditoria; o par `{slot_id, response_ref}` e `fence_digest` são função do ledger; qual venceu é input durável e visível no digest, não desempate do coordinator |
| Resposta ao dispatch de A repete B como ator, outro round/slot, revisão ou instante | admissão rejeita fail-closed antes de preencher o slot; resposta não entra no par `{slot_id, response_ref}`, em `input_digest` ou `fence_digest` |
| Crash após o dispatch de `s1` e antes da resposta; resume enquanto a chamada original ainda está em voo | resume encontra dispatch aberto e slot vazio; ou aguarda, ou grava `SlotDispatchRevocation` antes de redespachar; a resposta ao dispatch revogado é rejeitada mesmo chegando primeiro; nunca dois dispatches abertos nem duas respostas por slot |
| Adapter que é fonte exógena declarada e também respondedor de slot fecha `closed_through >= C` antes de responder | suas respostas de slot não recebem `open_coordinate > C`: são preenchimento de unidade derivada, fora da condição de fechamento; o fence admite os slots com a coordenada herdada da `RoundDeclaration` |
| `DEFER` para o próximo ciclo do mesmo instante | sucessor recebe `commit_successor_floor(commit) = (instant, cycle_ordinal + 1)` persistido no mesmo commit; resume e replay readmitem exatamente naquele ciclo |
| Dois candidatos C1/C2 referenciam a mesma unidade U, ou uma unidade produtora não aparece em candidato algum | validação da partição falha com `PROVENANCE_FAILURE` antes de conflito/resolução; `CycleAbortRecord` sem `DecisionRecord`, evento ou consumo; nenhuma implementação escolhe ou agrega dispositions localmente |
| Dois candidatos vencem no mesmo ciclo | nenhum evento de um aparece como `causal_parent` do outro; reação exige ciclo posterior |
| Dois candidatos vencedores A/B e seus outputs são entregues ao coordinator em ordens opostas | mesmos `candidate_id`; ordenação por `EventOrderKey` produz os mesmos `event_id`, `LogicalSequence`, `event_ids[]`, tarefas de percepção e `batch_digest` |
| Event store devolve evento com `EventOrderKey` ausente/alterada, ou commit permuta dois `event_ids[]` | replay recalcula cada `event_id`, reordena pelas chaves persistidas, valida `LogicalSequence` e `batch_digest`; qualquer ausência, id, posição, sequência ou digest divergente falha fechado antes do reducer |
| Último ciclo foi `(t,1)` e a única pendência está em `(t,3)` | `next_cycle_coordinate` retorna `(t,3)`; não existe `CycleCommit` vazio em `(t,2)` e replay faz o mesmo salto |
| Último ciclo foi `(t,1)` e a menor pendência é `(u,0)`, com `u > t`, sem novo trabalho materializado | após fechamento até o alvo, ciclo de avanço em `(t,2)` tem input vazio e commita `clock.advanced`; estado resultante é `(current_instant=u, cycle_ordinal_at_instant=-1)` e a nova derivação abre o ciclo de trabalho em `(u,0)` |
| Menor pendência é `P=(u,3)` e `body.advanced` no ciclo de avanço torna um trigger verdadeiro | a `TriggerActivation` nasce com coordenada mínima `(u,0)`, nunca `(t,n+1)`; depois do commit, `next_cycle_coordinate` recalcula e abre `(u,0)` com a ativação, preserva `P` para `(u,3)` e não encontra trabalho vencido |
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
| Commit R torna um `RISING_EDGE` verdadeiro e o processo cai antes de expandir a ativação | R publicou atomicamente runtime + `TriggerActivation(PENDING)` com coordenada do ciclo posterior; resume admite a mesma `activation_id` uma vez, sem reavaliar memória do host nem duplicar candidato |
| Trigger cria outro trigger no mesmo instante | ciclos ordinais diferentes, DAG causal e limite de cascata determinístico |
| Reducer falha no terceiro evento do lote | nenhum evento, revisão, disposição terminal ou consumo de occurrence persiste; a tentativa termina em `CycleAbortRecord` |
| Observatory lê segredo e depois fecha | hashes de world/knowledge/ledgers permanecem inalterados |
| Reflexão resume um rumor como hipótese | provenance continua apontando ao claim; nenhum Event/world truth é criado |
| Snapshot no meio de comunicação em trânsito | contínuo e snapshot+resume geram mesmos tails e state hash |
| Duas runs partem do mesmo checkpoint com inputs diferentes | prefixo/hash parental igual; tails e estados isolados; nenhum evento cruza a branch |
| Mesmo seed/inputs com paralelismo diferente | event log causal byte a byte e final state hash idênticos |
| Duas entregas semanticamente idênticas são atos de produção distintos da mesma fonte | chaves estáveis do produtor distintas geram `input_id`/`unit_digest` distintos; se o produtor declara o mesmo `idempotency_key`, o `idempotency_digest` exclui identidade de entrega e o alias canônico é escolhido pelos ids, nunca pelo append |
| Duas implementações registram fontes/categorias ou constroem `closure_proof`, rounds e slots em ordens locais diferentes | mesma política de identidade/ordem, provas por fonte, rounds por id e pares slot/resposta por slot; mesmos pares de `admitted_units`, `input_digest` e `fence_digest`; `source_rank` e ordem de map não existem no contrato |
| Crash depois do `CycleCommit` de evento perceptível e antes de qualquer percepção | `perception_task_ids[]` já referencia tarefa `PENDING` na outbox; resume a processa com ids idempotentes, persiste observations/inputs e completion antes de liberar o barrier; nenhuma evidência é perdida ou duplicada |
| Dois workers processam a mesma `PerceptionTask` e um cai após gravar parte dos recibos | ambos derivam os mesmos ids de task/observation/`KnowledgeInput`/completion por papel e ordinal de schema; reinsert é idempotente, CAS publica um completion e nenhum destinatário recebe duplicata |
| Evento secreto não possui observador elegível | tarefa commitada termina com completion vazio; nenhum `Observation`/`KnowledgeInput` é criado e o barrier pode avançar sem fallback de divulgação |
| `CycleCommit` vazio seguido de replay causal | journal do `DecisionLedger` restaura revisão, ordinal e `next_logical_sequence`; o `EventStore` permanece vazio para esse commit e não contém cópia do envelope |

## Contratos/ports mínimos para implementação

Os nomes concretos podem variar, mas a responsabilidade não:

| Contrato | Responsabilidade única |
|---|---|
| `Clock` | expor/avançar `SimulationInstant` pela função versionada `next_cycle_coordinate`, incluindo `CyclePlan` temporal no instante de origem, reset ordinal no destino e `commit_successor_floor=(to,0)` para todo trabalho derivado por esse avanço |
| `CycleCoordinator` | congelar a revisão base, aguardar a condição de fechamento, derivar a próxima coordenada/coorte/`CyclePlan`/fence e coordenar a tentativa até um envelope terminal; manter `CycleControlState` pela dobra total da §8.0.2 e nunca recalcular o plano de um fence em voo |
| `AdmissionFenceLog` | persistir por escrita condicional o `AdmissionFence` por `(cycle_id, attempt_ordinal)` antes de qualquer avaliação e servi-lo a replay/resume para verificar `CyclePlan`, prova append-stable, pares `(unit_id, unit_digest)`, `input_digest`, identidade causal e serialização total versionadas |
| `InputLedger` | registrar fontes exógenas declaradas em sequências prefix-consistentes, exigir chaves estáveis do produtor, derivar ids/`unit_digest` pela `CausalIdentityPolicy`, separar identidade de entrega de `IdempotencyIdentity` sem pré-consumir aliases de canônica já liquidada, atribuir `ingress_seq`/coordenada, persistir `SourceClosure` monotônicos/finais e selecionar o primeiro que cobre cada coordenada; rejeitar append após `final`; manter o registro de slot — `SlotDispatch`, `SlotDispatchRevocation` e no máximo uma `SlotResponse` por `(decision_round_id, slot_id)`, vinculada fail-closed ao dispatch aberto (§3.2.5) |
| `ScheduleStore` | manter lifecycle de occurrences e `RoundDeclaration` e consultar as elegíveis à coordenada do ciclo |
| `TriggerRegistry` | armazenar definition/version, runtime state e lifecycle das `TriggerActivation`; materializar runtime + ativação atomicamente com a revisão de origem e consultar ativações pendentes |
| `OccurrenceHandler` | transformar occurrence vencida em candidato, sem side effect |
| `ActionSchemaRegistry` | validar forma/versionamento de propostas |
| `AffordanceValidator` | produzir facets contra a revisão congelada |
| `ConflictDetector` | construir conflict sets conservadores |
| `ConflictResolver` | produzir disposições sob política/version/seed explícitos |
| `DecisionLedger` | registrar material provisório de avaliação, `AttemptRetryRecord` e o commit journal canônico/único dos envelopes terminais (`CycleCommit` com `DecisionRecord` e referências/digest de `PerceptionTask`, `CycleAbortRecord`) |
| `CommitCoordinator` | validar a partição exata unidade→candidato, plano/lote/post-state, ordenar toda `EventDraft` pela `EventOrderKey`, persistir a chave no `Event`, validar ids/sequências e `batch_digest`, aplicar `commit_successor_floor` a todo output causal derivado, derivar transições/ativações de trigger e tarefas de percepção, e persistir atomicamente fence closure + um `DecisionRecord` por unidade admitida + eventos + `PerceptionTask` + `CycleCommit` + revisão, ou o `CycleAbortRecord` sem nenhum `DecisionRecord`/tarefa |
| `EventStore` | append/read somente de `Event` imutável com `EventOrderKey` persistida e referenciado pelo commit journal; sem `CycleCommit`, API update ou delete |
| `ReducerRegistry` | mapear cada event type ao único owner/reducer versionado |
| `PerceptionResolver` | transformar evento+acesso em outputs endereçados com papel/ordinal de schema e ids derivados pela política epistemológica |
| `EvidenceLedger` | persistir observations/recibos por destinatário e identidade determinística, append-only e idempotente |
| `EpistemicOutbox` | persistir `PerceptionTask` com versões/hashes atomicamente com o commit, registrar um completion determinístico somente após recibos verificáveis, reconciliar commit/tarefa/evidence e impor barrier antes do próximo round |
| `KnowledgeInputSink` | entregar idempotentemente recibos, sem formar crença |
| `SnapshotStore` | salvar/carregar checkpoint causal (inclusive `CycleControlState`) + referência epistemológica como unidade verificada por hash |
| `ObservatoryQueries` | leitura sem dependências transitivas de escrita |

Não haverá um `WorldService` genérico que valide, resolva, mute, perceba e comunique. Esses contratos
existem para tornar impossível que dois módulos decidam o mesmo verbo.

## Sequência de implementação recomendada

1. Value objects, `CausalIdentityPolicy`, `unit_digest`/`input_digest`, políticas de coordenada
   (inclusive `commit_successor_floor`) e
   ordem de evento, envelopes canônicos, cinco ledgers e hashes.
2. Clock/`next_cycle_coordinate`, `WorldRevision`, fontes/`SourceClosure`, `RoundDeclaration`/coorte, registro de slot
   (`SlotDispatch`, revogação, resposta única por escrita condicional), derivação verificável do
   `AdmissionFence`, agenda e lifecycle durável de `TriggerActivation` com testes de propriedade —
   incluindo a prova de que permutar a ordem de gravação/construção e o momento do fence não altera
   ids, bytes canônicos ou `fence_digest`.
3. `ActionProposal`, facets e um resolvedor de recurso mínimo, sem regra de exame.
4. `CommitCandidate` identificado com partição exata das unidades, `EventOrderKey`/`EventBatch`, reducers, transação única (um `DecisionRecord` por unidade admitida + eventos +
   `PerceptionTask` + `CycleCommit` + revisão), envelope de abort sem settlement,
   `CycleControlState`/`AttemptRetryRecord` e replay de world state.
5. `Observation`/`KnowledgeInput` com ids por papel/ordinal, completion da causal outbox, `Claim`,
   `Transmission` e entrega agendada.
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

**Manter o alvo de `CLOCK_ADVANCE` somente na memória do coordinator.** Um crash depois do fence
obriga o resume a executar novamente `next_cycle_coordinate` sobre outro prefixo de ledger ou a
adivinhar o destino a partir de eventos ainda não commitados. `CyclePlan` pertence ao fence e aos
envelopes terminais, não à pilha do processo.

**Persistir apenas `event_id` e o hash da política de ordem.** O hash identifica a função, mas não
recupera a `EventOrderKey` que foi sua entrada. Sem a preimagem em cada `Event`, replay só confia na
ordem recebida; não recalcula identidade, posição nem `LogicalSequence`.

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

**Criar a causal outbox depois do commit do mundo.** Uma projeção reconstruível poderia ser correta,
mas exigiria cursor, scan e reconciliação normativos antes de qualquer barrier. Na V1, referenciar e
persistir `PerceptionTask` na própria transação do `CycleCommit` é a topologia menor: mundo e trabalho
epistemológico nunca divergem, enquanto observations e entregas continuam assíncronas/idempotentes.

**Aplicar evento agendado antes das ações apenas porque saiu primeiro da fila.** Reintroduz ordem
incidental. Deadline e precedência precisam estar na regra temporal/conflito, enquanto candidatos do
mesmo ciclo leem a mesma revisão.

**Criar ativação de trigger depois de publicar a revisão que mudou o predicate.** Abre uma janela em
que crash perde a ativação ou resume precisa inferi-la outra vez sem recibo de identidade/consumo. A
revisão de origem precisa publicar runtime e `TriggerActivation` na mesma transação, elegível apenas
ao ciclo posterior.

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

- A fundação requer cinco artefatos append-only e uma transação entre event append, criação da causal
  outbox e publicação de revisão, em vez de um único JSONL usado para fatos, decisões, evidência e
  telemetria.
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
- Ativações de trigger deixam de existir apenas na pilha do processo: a revisão que detecta a
  transição publica a entrada durável do ciclo seguinte, e seu custo é um artefato/evento de lifecycle.
- Todo evento potencialmente perceptível cria trabalho de outbox no mesmo commit. Isso amplia a
  fronteira transacional, mas elimina a janela em que world truth existe sem rota epistemológica
  retomável.
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
