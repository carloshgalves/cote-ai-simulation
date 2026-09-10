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
| `ScheduledOccurrence` | trabalho causal pendente cujo vencimento é temporal |
| `TriggerDefinition` | predicate puro e política explícita de ativação |
| `TriggerRuntimeState` | memória operacional necessária para edge/rearm/repeat |
| `ActionProposal` | pedido imutável para tentar uma ação; não é fato de que a ação ocorreu |
| `AffordanceAssessment` | diagnóstico facetado da proposta contra um snapshot |
| `CommitCandidate` | unidade atômica que pode produzir zero ou mais eventos e declarar conflitos |
| `Event` | fato imutável do mundo aceito em commit |
| `CycleCommit` | envelope persistido do `EventBatch` de um ciclo, obrigatório mesmo com zero eventos |
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
  `CausalRef`, `SeedKey` e versões de schema/política.
- **Entidades/artefatos imutáveis:** `Event`, `ActionProposal`, `Claim`, `Observation` e
  `KnowledgeInput`; identidade e provenance importam mesmo quando dois payloads são iguais.
- **Agregados com lifecycle:** `ScheduledOccurrence`, `Trigger` (`Definition + RuntimeState`) e
  `Transmission`.
- **Fronteira de consistência:** `ResolutionCycle` agrega candidatos e produz um único `EventBatch` +
  `WorldRevision`, persistidos como um `CycleCommit` mesmo quando o lote tem zero eventos (§8.0).
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

Um ciclo é identificado por `(run_id, instant, cycle_ordinal)` e declara:

```text
ResolutionCycle {
  cycle_id
  instant
  base_revision
  base_state_hash
  admitted_input_ids[]
}
```

Todas as propostas do ciclo leem exatamente `base_revision`. Nenhuma vê mutações de outra proposta do
mesmo ciclo. O lote é fechado por protocolo, não pela ordem de chegada:

- uma solicitação a agente carrega `decision_round_id`, `base_revision` e `effective_at`;
- a resposta pertence àquele round mesmo que outra chamada termine antes;
- o round declara previamente seus slots e só fecha quando cada slot contém `ActionProposal` ou
  `NoProposal`; timeout operacional, quando usado, admite um `NoProposal` explícito no slot;
- depois que o round fecha, resposta atrasada é ignorada e auditada; nunca é inserida retroativamente
  nem promovida automaticamente ao ciclo seguinte. Uma nova tentativa exige novo round/input causal;
- o wall clock que levou ao `NoProposal` fica na telemetria e não aparece como causa dentro do mundo.

A membresia do ciclo é decidida por uma **barreira de admissão** única por `cycle_id`, comum a todas
as fontes de entrada. `decision_round_id` sozinho não define ciclo:

- um run tem no máximo um ciclo aberto por vez, e um ciclo hospeda no máximo um `decision_round_id`;
  rounds não se sobrepõem. O round é vinculado a `(instant, cycle_ordinal)` na abertura e todos os
  seus slots pertencem àquele ciclo, mesmo que carreguem `effective_at` iguais aos de outra fonte;
- input exógeno entra apenas pelo input ledger, nunca direto no coordinator. A admissão grava
  `(input_id → cycle_id)` antes do fechamento da barreira; essa atribuição integra o input digest
  canônico e é reutilizada em replay/resume, exatamente como as respostas de LLM já registradas;
- a barreira fecha quando todo slot do round está preenchido e nenhum input exógeno com
  `effective_at <= instant` está pendente de admissão. Fechada a barreira, `admitted_input_ids` é
  imutável;
- input exógeno que chegue depois do fechamento nunca é inserido retroativamente: ele é admitido ao
  próximo ciclo, com `effective_at` normalizado para o instante desse ciclo e o valor original
  preservado no input ledger como provenance;
- ocorrências vencidas e ativações de trigger não disputam ordem de chegada: entram por serem devidas
  contra `base_revision` no instante do ciclo, dentro da mesma barreira.

Assim, `effective_at` compartilhado por dois rounds independentes, ou por um round e um input exógeno,
não gera ambiguidade: cada entrada tem uma única atribuição de ciclo, fixada antes de qualquer
avaliação. Ordem de chegada ou de fechamento não altera `base_revision`, `ConflictSet`, vencedor nem
event log.

Essa barreira de admissão decide quando um ciclo **fecha**; o barrier epistemológico da §10.1 decide
quando um round pode **abrir**. São gates complementares e não se substituem.

Eventos commitados no mesmo ciclo recebem `LogicalSequence` em ordem canônica apenas para
serialização e replay. Essa sequência não significa que um vencedor leu o resultado do evento
anterior e nunca cria link causal entre candidatos distintos (§8.1). Prioridade de disputa precisa
vir de uma política de domínio explícita e versionada, nunca do sequence, da ordem de iteração ou da
latência.

Pode haver mais de um ciclo no mesmo instante: por exemplo, um commit entrega uma mensagem e a
entrega ativa um trigger imediato. Esses ciclos têm ordinais crescentes e revisões diferentes. Uma
configuração versionada limita a cascata de ciclos sem avanço temporal; excedê-la aborta o run com
erro determinístico, em vez de truncar silenciosamente.

#### 3.3 Avanço

O engine avança para o menor próximo instante dentre ocorrências agendadas, próxima cadência de
trigger repetível e input externo já admitido. Quando o destino é posterior ao instante corrente, um
ciclo de avanço produz `clock.advanced { from, to }` e os eventos de materialização temporal exigidos
pelos substates afetados, como `body.advanced`, no mesmo commit atômico. Só então abre o ciclo das
entradas devidas no novo instante. Não existe recuperação, expiração ou juros implícitos fora do log
causal.

Agendar no passado é erro. Agendar no instante corrente só é permitido para um ciclo posterior e
conta para o limite de cascata. Deadlines usam intervalos declarados (`<`, `<=`) e não dependem de um
evento “prazo expirou” ter sido serializado antes de uma ação no mesmo instante.

### 4. Agenda e triggers

#### 4.1 `ScheduledOccurrence`

É uma entidade durável, não um `Event` futuro:

```text
ScheduledOccurrence {
  occurrence_id
  due_at
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
a liquida conforme a §7.

Recorrência não “ressuscita” a mesma ocorrência: o consumo bem-sucedido agenda a próxima instância
com nova identidade e link causal. Criação, cancelamento e consumo são representados por eventos de
lifecycle no mesmo `EventBatch` que os causa; a fila é reconstruível pelo event store, não somente por
um snapshot da memória do scheduler.

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
  decision_round_id
  submitted_against_revision
  originating_intention_ref?
  idempotency_key
}
```

Alvos têm papéis explícitos; listas e maps usam normalização canônica. Parâmetros são validados pelo
schema versionado do `action_type`. `originating_intention_ref` é opaco para a fundação e nunca prova
motivação. A proposta não contém desfecho narrado, sucesso requerido ou mutação arbitrária.

O input ledger registra proposta e origem; o decision ledger registra sua disposição. A proposta
**não** entra no event store como world truth. Se o ator chegou a executar um movimento perceptível,
o resolvedor produz um evento de tentativa; se apenas sugeriu algo impossível ao engine, há rejeição
auditável no decision ledger e nenhum fato físico inventado.

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
negado: significa que regra, dado ou resolvedor necessário está ausente. Ele é gravado no decision
ledger com `facet`, `reason_code`, `evidence_refs` e `rule_version` do validador que faltou, junto da
disposição do ciclo, e só então o ciclo aborta antes do commit — independentemente do `policy_effect`
declarado, que descreve regra existente e não supre regra ausente. Ciclo abortado não persiste
`CycleCommit` nem publica revisão (§8.0), de modo que o comportamento fail-closed fica auditável sem
inventar estado. Isso impede que lacunas de implementação virem realidade do mundo por default.

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
freeze base revision
  → admit/deduplicate inputs
  → expand due occurrences and trigger activations
  → validate proposals against the frozen state
  → build CommitCandidates
  → detect ConflictSets from declared reads/writes/resources/invariants
  → resolve with explicit versioned policies and named randomness
  → validate the complete EventBatch and post-state
  → atomically append events + publish new WorldRevision
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
nomeado.

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
  novo input causal, com identidade nova, `effective_at` estritamente posterior — instante futuro ou
  próximo ciclo do mesmo instante, contando para o limite de cascata — e provenance para a fonte
  anterior. `DEFER` nunca reabre a identidade antiga nem deixa trabalho fantasma na memória do
  processo.

O commit é uma transação única sobre `base_revision`: ou todos os eventos dos candidatos vencedores,
incluindo eventos de lifecycle de agenda/trigger, e a nova revisão persistem, ou nada persiste. Antes
de publicar, reducers são aplicados a uma cópia de trabalho e todas as invariantes afetadas são
verificadas. Falha em evento, reducer, schema, provenance ou invariante aborta o lote inteiro. Não há
commit parcial de uma ação multi-evento nem event log adiantado em relação ao state store.

### 8. Eventos e ledgers

#### 8.0 Envelope de commit do ciclo

Todo ciclo fechado com sucesso persiste no event store um envelope do seu lote, **inclusive quando o
lote tem zero eventos**:

```text
CycleCommit {
  run_id
  cycle_id
  instant
  cycle_ordinal
  base_revision + base_state_hash
  result_revision + result_state_hash
  admitted_input_ids[]
  event_ids[]              // pode ser vazio
  next_logical_sequence
  batch_digest
}
```

Um ciclo em que todos os slots produziram `NoProposal`, ou em que todo candidato foi rejeitado sem
gerar evento de lifecycle, ainda avança `WorldRevision` e `cycle_ordinal`. O envelope é o registro que
torna esse avanço reconstruível: o replay percorre a sequência de `CycleCommit` e reaplica os eventos
de cada um, reproduzindo revisão vazia, ordinal e `next_logical_sequence` sem precisar de evento
decorativo nem de um snapshot do coordinator. Ciclo abortado não produz envelope, não publica revisão
e não avança o ordinal do instante.

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

- **Input ledger:** inputs exógenos admitidos, propostas e respostas ausentes; uma futura intervenção
  de usuário só poderá entrar aqui por contrato próprio, nunca pelo Observatory;
- **Decision ledger:** assessments, conflitos, versões de política e disposições, inclusive rejeições;
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
ciclo impede abrir novo round de decisão para o destinatário enquanto houver evidência daquele
instante pendente de entrega. Assim, crash entre event commit e percepção retoma o trabalho sem perder
nem duplicar conhecimento causal.

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
mesmo genesis snapshot/config
+ mesmos inputs admitidos em bytes canônicos, com a mesma atribuição de ciclo registrada
+ mesmas versões de schema/reducer/validator/resolver/RNG
+ mesmo world seed
→ mesmo input/decision digest
→ mesmo event log causal byte a byte
→ mesmo evidence ledger por destinatário
→ mesmo final state hash
```

“Mesmos inputs” significa reutilizar as respostas externas/LLM registradas. Reexecutar um modelo
remoto não faz parte do replay determinístico da fundação; comparar nova cognição é uma eval separada.

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
   chamada de modelo.

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
7. Resultado indeterminado de validação é `INDETERMINATE` registrado no decision ledger e aborta o
   ciclo; lacuna de implementação não vira permissão.
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
20. Toda entrada causal recebe atribuição de ciclo na barreira de admissão antes de ser avaliada;
    rounds não se sobrepõem e ordem de chegada nunca decide membresia.
21. Toda disposição (`COMMIT`, `REJECT`, `DEFER`) liquida a fonte do candidato no mesmo commit;
    nenhuma fonte vencida permanece `PENDING` no instante avaliado.
22. Todo ciclo commitado persiste um `CycleCommit`, mesmo com zero eventos.
23. Link causal dentro do mesmo ciclo só existe entre eventos do mesmo `CommitCandidate` atômico.
24. Checkpoint resumível contém ou referencia atomicamente checkpoint epistemológico versionado de
    cada ator elegível.

## Cenários de stress e provas de aceite

| Cenário | Prova exigida |
|---|---|
| Duas respostas LLM chegam em ordens opostas | mesmo `decision_round_id` + `base_revision`; log e vencedor idênticos |
| Dois rounds independentes, ou round e input exógeno, declaram o mesmo `effective_at` | barreira única atribui ciclo antes da avaliação; base revision, `ConflictSet`, vencedor e log não mudam com a ordem de chegada |
| Candidato de ocorrência vencida é rejeitado | ocorrência termina no mesmo commit; nenhum `PENDING` remanescente e nenhum redisparo a partir de estado velho |
| Ciclo em que todos os slots respondem `NoProposal` | `CycleCommit` vazio persistido; replay reproduz revisão, ordinal e `next_logical_sequence` |
| Faceta sem regra, dado ou resolvedor disponível | `INDETERMINATE` no decision ledger; nenhum commit, nenhum envelope, nenhuma revisão publicada |
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
| Reducer falha no terceiro evento do lote | nenhum evento/revisão/consumo de occurrence persiste |
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
| `CycleCoordinator` | abrir/fechar a barreira de admissão sobre uma revisão e coordenar o commit |
| `InputLedger` | admitir inputs idempotentes, fixar sua atribuição de ciclo e preservar ordem declarada do round |
| `ScheduleStore` | manter lifecycle de occurrences e consultar vencidas |
| `TriggerRegistry` | armazenar definition/version e runtime state |
| `OccurrenceHandler` | transformar occurrence vencida em candidato, sem side effect |
| `ActionSchemaRegistry` | validar forma/versionamento de propostas |
| `AffordanceValidator` | produzir facets contra a revisão congelada |
| `ConflictDetector` | construir conflict sets conservadores |
| `ConflictResolver` | produzir disposições sob política/version/seed explícitos |
| `CommitCoordinator` | validar lote/post-state e persistir `CycleCommit`+eventos+revisão atomicamente |
| `EventStore` | append/read de eventos imutáveis; sem API update/delete |
| `ReducerRegistry` | mapear cada event type ao único owner/reducer versionado |
| `PerceptionResolver` | transformar evento+acesso em observations endereçadas |
| `EvidenceLedger` | persistir observations/recibos por destinatário, append-only |
| `EpistemicOutbox` | retomar entrega idempotente e impor barrier antes do próximo round |
| `KnowledgeInputSink` | entregar idempotentemente recibos, sem formar crença |
| `SnapshotStore` | salvar/carregar checkpoint causal + referência epistemológica como unidade verificada por hash |
| `ObservatoryQueries` | leitura sem dependências transitivas de escrita |

Não haverá um `WorldService` genérico que valide, resolva, mute, perceba e comunique. Esses contratos
existem para tornar impossível que dois módulos decidam o mesmo verbo.

## Sequência de implementação recomendada

1. Value objects, envelopes canônicos, quatro ledgers e hashes.
2. Clock, `WorldRevision`, agenda e lifecycle de trigger com testes de propriedade.
3. `ActionProposal`, facets e um resolvedor de recurso mínimo, sem regra de exame.
4. `EventBatch`, reducers, transação e replay de world state.
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
- Simultaneidade passa a ser propriedade verificável por `cycle_id/base_revision`, não convenção do
  orchestrator.
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
