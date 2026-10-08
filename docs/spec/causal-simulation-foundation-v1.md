# Spec — Causal Simulation Foundation V1

**Status:** Proposed — bloqueada somente pelas decisões ainda abertas da §14

**Decisão de origem:** [ADR 0008 — Fundação causal da simulação V1](../adr/0008-causal-simulation-foundation-v1.md)

**Decisões relacionadas:** [ADR 0001](../adr/0001-world-truth-and-knowledge-boundaries.md),
[ADR 0002](../adr/0002-character-canon-rag-and-memory.md),
[ADR 0003](../adr/0003-logical-event-driven-time.md),
[ADR 0005](../adr/0005-canon-knowledge-base.md),
[ADR 0006](../adr/0006-physical-domain-model.md),
[ADR 0007](../adr/0007-python-para-o-subdominio-embodiment.md) e
[ADR 0009](../adr/0009-canonical-causal-codec-and-digests.md)

Esta spec traduz o ADR 0008 em uma entrega fatiável. O ADR é normativo para a semântica causal e
para os schemas conceituais; esta spec define a superfície demonstrável, os incrementos de entrega e
a validação exigida. Divergência entre ambos é defeito desta spec, não licença para reinterpretar o
ADR.

---

## 1. Problema e objetivo visível

O repositório já possui tempo lógico, um subdomínio físico executável e a separação conceitual entre
world truth e conhecimento. Ainda não existe a fundação que responda, de forma única e auditável:

1. quando uma entrada se tornou elegível;
2. quais decisões foram simultâneas;
3. o que foi tentado, rejeitado, adiado ou commitado;
4. quais fatos alteraram o mundo;
5. quem recebeu qual evidência e por qual cadeia causal;
6. como retomar ou reproduzir o run sem usar latência, wall clock ou nova chamada de LLM.

Sem esses contratos, o primeiro exame integrado distribuiria autoridade entre orchestrator,
scheduler, domínio físico, regras de exame e workers de percepção. Ordem de chegada passaria a ser
prioridade causal; crashes poderiam deixar decisão sem mundo ou mundo sem evidência retomável; e um
evento secreto poderia entrar em contexto apenas porque existe no event store.

**Objetivo visível:** dado um manifesto de genesis, um world seed e um input ledger gravado, executar
um cenário causal completo e obter os mesmos ledgers, hashes, estado final e evidência endereçada
independentemente da ordem de conclusão dos workers, de crash/resume e do paralelismo usado.

Ao fim da V1, um harness sem LLM deve conseguir:

1. criar um run com duas fontes exógenas declaradas e fechar explicitamente sua contribuição;
2. declarar dois rounds simultâneos, despachar um slot por ator e gravar suas respostas em ordem
   inversa à ordem dos atores;
3. resolver duas ações que disputam a última unidade de um recurso contra a mesma revisão base;
4. commitar settlement, fatos do mundo, revisão e tarefas de percepção em uma única fronteira
   atômica;
5. avançar o relógio até uma entrega agendada, materializar um trigger e processá-lo em ciclo
   posterior;
6. entregar um claim verdadeiro ou falso sem promover a alegação a world truth e sem revelar um
   evento secreto a ator não elegível;
7. parar de forma auditável diante de validação indeterminada e retentar apenas após autorização
   durável;
8. cair depois do fence, depois do commit ou no meio da projeção epistemológica e retomar sem
   duplicar input, evento, decisão, observation ou `KnowledgeInput`;
9. fazer causal replay e snapshot+resume, produzindo os mesmos bytes canônicos, digests, revisões,
   cursores e state hash da execução contínua;
10. consultar o resultado por um Observatory read-only sem alterar qualquer hash autoritativo.

O cenário de referência usa políticas roteirizadas e payloads sintéticos. Nenhuma chamada de modelo
é necessária para provar a fundação.

---

## 2. Termos, autoridade e invariantes afetados

### 2.1 Termos normativos

Esta spec implementa o vocabulário do ADR 0008 §2. Os grupos abaixo orientam ownership e tickets;
eles não criam sinônimos:

| Grupo | Tipos principais | Autoridade |
|---|---|---|
| tempo e corte | `SimulationInstant`, `EligibilityCoordinate`, `ResolutionCycle`, `CyclePlan`, `WorldRevision` | `Clock` + `CycleCoordinator` |
| ingresso e coorte | `CausalSource`, `SourceClosure`, `RoundDeclaration`, `DecisionCohort`, `SlotDispatch`, `SlotResponse`, `AdmissionFence` | `InputLedger`, `ScheduleStore`, `AdmissionFenceLog` |
| trabalho causal | `ScheduledOccurrence`, `TriggerDefinition`, `TriggerRuntimeState`, `TriggerActivation`, `ActionProposal` | owners específicos; nenhum muta o mundo diretamente |
| avaliação | `AffordanceAssessment`, `ConflictSet`, `CommitCandidate`, `DecisionRecord` | validators/resolvers + `DecisionLedger` |
| terminalidade | `CycleCommit`, `CycleAbortRecord`, `AttemptRetryRecord`, `CycleControlState` | commit journal do `DecisionLedger` |
| world truth | `Event`, `EventOrderKey`, `WorldState` | `EventStore` + reducers; publicação pelo `CommitCoordinator` |
| evidência | `PerceptionTask`, `Observation`, `Claim`, `Transmission`, `KnowledgeInput` | causal outbox + `EvidenceLedger` |
| continuidade | `Snapshot`, `EpistemicCheckpointRef` | `SnapshotStore`; conteúdo epistemológico continua opaco à fundação |

`scene`, `turn` e momento narrativo podem existir como projeções de UX, mas não participam de
identidade, elegibilidade, simultaneidade ou commit.

### 2.2 Invariantes do repositório exercitadas

| Invariante | Obrigação nesta entrega |
|---|---|
| world truth ≠ belief | `Event`/`WorldState`, `Observation`/`KnowledgeInput` e estado epistemológico usam stores, schemas e owners distintos |
| informação tem proveniência | todo input epistemológico resolve para observation, claim/publicação e cadeia de evidência endereçada |
| inteligência não concede acesso | não existe query do event store global na montagem de contexto; inferência futura recebe apenas evidência permitida |
| regras determinísticas são autoridade | validators, conflict resolvers, reducers, clock, agenda e triggers não delegam decisão a LLM |
| ação impossível é rejeitada | proposta não vira fato; `INDETERMINATE` aborta fail-closed e não é tratado como permissão |
| memória pós-divergência é proibida | allow-list preserva timeline, gates e provenance do ADR 0001/0005 |
| consequência física persiste | integração do `Embodiment` entra como eventos/reducer do engine, nunca como segunda autoridade de commit |
| interocepção não é telemetria | percepção física atravessa adapter endereçado; payload autoritativo não é copiado para contexto |

### 2.3 Invariantes causais normativas

As 41 invariantes do ADR 0008 §14 são requisitos de aceite integrais. Para planejamento, elas se
agrupam nas seguintes propriedades, sem reduzir sua força:

1. **autoridade única:** somente o commit coordinator publica evento e revisão; handlers, validators,
   reducers, projectors e Observatory não escrevem fatos;
2. **simultaneidade fechada:** coorte e unidades vêm de ledgers, coordenadas, fechamentos e revisão
   base, nunca de timing;
3. **settlement total e atômico:** toda unidade admitida recebe exatamente um `DecisionRecord` em
   `CycleCommit`, ou a tentativa inteira termina em `CycleAbortRecord` sem settlement parcial;
4. **identidade e ordem canônicas:** ids, digests, ordem de admissão e ordem de eventos não dependem
   de UUID aleatório, append position, `ingress_seq`, map iteration ou conclusão de worker;
5. **tempo causal:** trabalho derivado só nasce em `commit_successor_floor` ou depois; avanço temporal
   tem plano durável e não salta sobre pendência;
6. **lifecycle durável:** occurrences, rounds, ativações, dispatches e retentativas não existem apenas
   em memória e não ressuscitam por retry técnico;
7. **causalidade explícita:** eventos simultâneos de candidatos diferentes não se tornam pais apenas
   por `LogicalSequence`; reação ocorre em ciclo posterior;
8. **fronteira epistemológica:** evento só chega a ator por tarefa, observation e recibo endereçado;
   claim continua alegação;
9. **replay sem modelo:** replay e resume usam bytes já persistidos, versões e checkpoint
   epistemológico; nunca chamam LLM nem consultam wall clock;
10. **falha fechada:** schema, versão, digest, provenance, reducer, identidade ou invariante ausente ou
    divergente impedem publicação parcial.

---

## 3. Escopo

### 3.1 In-scope

| Capacidade | Entrega V1 |
|---|---|
| manifesto do run | genesis versionado com fontes, seed, timezone, limites e versões/hashes de todas as políticas causais |
| bytes e identidade | [Canonical Causal Codec V1](../architecture/canonical-codec-v1.md), `derive_id`, hashes de estado/envelope e detecção fail-closed de colisão |
| clock e ciclos | `SimulationInstant`, coordenadas, `CyclePlan`, seleção da menor pendência, salto de ordinal e ciclo de avanço |
| ingresso exógeno | fontes declaradas, chaves estáveis do produtor, prefix consistency, `SourceClosure`, normalização e `final` |
| rounds e slots | lifecycle de `RoundDeclaration`, coorte derivada, barrier epistemológico, dispatch único, revogação e resposta única |
| admissão | derivação/verificação do `AdmissionFence`, topologia e ordens canônicas, `unit_digest`, `input_digest`, `fence_digest` |
| agenda e triggers | occurrence durável, recorrência por sucessor, runtime persistido e ativação atômica com a revisão de origem |
| propostas e affordance | schemas de ação, facets, distinção possibilidade/proibição, `INDETERMINATE` e um resolvedor mínimo de recurso |
| candidatos e conflitos | partição exata unidade→candidato, read/write/resource claims, conflito, prioridades e randomness nomeado |
| terminalidade | `DecisionRecord`, `CycleCommit`, `CycleAbortRecord`, dobra de `CycleControlState` e retentativa explícita |
| eventos e estado | `EventOrderKey`, event batch, event store append-only, reducers puros/versionados, state hash e revisão |
| percepção | causal outbox atômica, resolver de acesso, observations/inputs idempotentes, completion e barrier |
| comunicação | `Proposition`, `Claim`, `Transmission`, envio, entrega/falha agendada e identidade apresentada separada da real |
| snapshot/replay | causal replay, resume equivalence, checkpoint epistemológico opaco, fork por novo `run_id` |
| observabilidade | traces e métricas operacionais fora de digests; queries privilegiada, POV e pública sem side effects |
| integração física | adapter explícito do log/snapshot do `Embodiment` para evento/revisão/snapshot causal canônico |

### 3.2 Out-of-scope

- escolha de ação, personalidade, intenção ou estratégia por LLM;
- inferência de crença, confiança, suspeita, esquecimento e reflexão;
- regras concretas de exames, PP/CP, economia, clubes, facções ou relações;
- modelo detalhado de atenção;
- consequências institucionais específicas além de registrar a infração tipada;
- intervenção de usuário como fonte causal;
- UI, narrativa e cenas como unidade causal;
- merge de timelines;
- escolha do framework de agentes, provedor de LLM ou banco vetorial;
- transformar o Observatory em command surface;
- refazer o modelo físico: a V1 apenas adapta a autoridade já definida pelos ADRs 0006/0007.

---

## 4. Determinístico × dependente de modelo

| Componente | Natureza na V1 |
|---|---|
| derivação de ids, digests, coorte, fence, plano, ordem e revisão | função pura de bytes/versionamento persistidos |
| elegibilidade, agenda, trigger e reducers | determinístico |
| validação de affordance e detecção de conflito | determinístico contra `base_revision` |
| desempate ou ruído de domínio | estocástico, mas determinístico dado substream nomeado, versão e seed |
| percepção com ruído | estocástica sob a mesma disciplina; resultado persistido |
| resposta de slot do harness | roteirizada e persistida; simula o contrato externo de um respondedor |
| resposta futura de Agent Cognition | dependente de modelo, fora da V1; entra somente como `SlotResponse` já gravada |
| belief update, reflexão e memória | dependentes de contexto/modelo, fora da V1; checkpoint é opaco |
| replay/resume | nunca executa modelo; reutiliza resposta e checkpoint persistidos |
| narração | fora da V1 e sem autoridade de escrita |

O core causal não importa cliente de LLM. Uma futura integração pode chamar modelo apenas depois de
um `SlotDispatch` e precisa persistir a resposta antes de qualquer avaliação. Latência, timeout e
origem operacional ficam na telemetria; o conteúdo admitido e seu vínculo causal ficam no ledger.

---

## 5. Arquitetura, ownership e ports

`Causal Simulation Foundation` é subdomínio do Simulation Engine. A implementação pode mudar nomes
concretos, mas preserva os ports e a direção de dependência abaixo.

```text
exogenous adapters ──> InputLedger <── slot responders
                              │
ScheduleStore + TriggerRegistry + Clock
                              │
                       CycleCoordinator
                              │ AdmissionFence
validators/handlers ──> CommitCandidate ──> ConflictResolver
                              │
                       CommitCoordinator
             ┌────────────────┼────────────────┐
       DecisionLedger      EventStore     EpistemicOutbox
             │                 │                 │
             │          ReducerRegistry    PerceptionResolver
             │                 │                 │
             └────────── WorldRevision      EvidenceLedger
                                                   │
                                          KnowledgeInputSink
```

### 5.1 Ports obrigatórios

| Port | Responsabilidade |
|---|---|
| `Clock` | estado temporal e aplicação validada do `CyclePlan`; não seleciona input |
| `CycleCoordinator` | derivar plano/coorte/fence e dirigir uma tentativa a envelope terminal |
| `AdmissionFenceLog` | escrita condicional e verificação de um fence por tentativa |
| `InputLedger` | fontes/closures/inputs e registro de dispatch/revogação/resposta sob regras distintas |
| `ScheduleStore` | lifecycle e consulta por coordenada de occurrences e rounds |
| `TriggerRegistry` | definitions/runtime/ativações e dependências versionadas |
| `ActionSchemaRegistry` | forma canônica e versão de cada ação |
| `OccurrenceHandler` | occurrence/activation/input → candidato sem side effects |
| `AffordanceValidator` | facets atribuídas a sujeito e evidência sob versão/hash |
| `ConflictDetector` | `ConflictSet` conservador a partir de claims/invariantes |
| `ConflictResolver` | disposição determinística sob política e substream declarados |
| `DecisionLedger` | material provisório, retries e journal terminal único |
| `CommitCoordinator` | partição, ordem, reducers, invariantes, lifecycle, percepção e transação atômica |
| `EventStore` | append/read de `Event`; sem update/delete e sem cópia de `CycleCommit` |
| `ReducerRegistry` | owner/reducer/footprint puros e versionados por event type |
| `PerceptionResolver` | evento + acesso → observations/inputs endereçados, nunca belief |
| `EvidenceLedger` | evidência append-only particionada por destinatário |
| `EpistemicOutbox` | task/completion, reconciliação e barrier |
| `KnowledgeInputSink` | entrega idempotente sem formar crença |
| `SnapshotStore` | checkpoint causal + refs epistemológicas verificados como unidade |
| `ObservatoryQueries` | consultas sem dependência transitiva de escrita |

Um `WorldService` que acumule validação, resolução, mutação, percepção e comunicação viola a
separação de autoridade mesmo que passe pelos mesmos tipos.

### 5.2 Regra de dependência para domínios consumidores

Domínios futuros registram schemas, handlers, validators, reducers, footprints e resolvers por ports.
Eles podem produzir candidato ou consumir observation/`KnowledgeInput`; não recebem store mutável,
clock mutável, committer genérico ou inbox de outro ator. `Embodiment` é um resolvedor especializado
dentro desse limite, não um committer paralelo.

---

## 6. Dados e mudanças de estado

### 6.1 Manifesto imutável do run

O genesis persiste, no mínimo:

- `run_id`, `world_seed`, `timezone=Asia/Tokyo` para o cenário COTE V1 e instante inicial;
- schema/hash do genesis e, quando houver fork cross-policy, o
  [`parent_checkpoint_history_ref`](../architecture/cross-policy-checkpoint-reference-v1.md);
- fontes exógenas declaradas;
- `codec_policy_id=cote.csf.codec.cbor-det.v1`, `codec_version=1`,
  `identity_algorithm_version=cote.csf.sha256.v1`, NFC pelo Normalization Process for Stabilized
  Strings (NPSS) Unicode 15.1.0,
  `codec_policy_hash=513111dc82a5e58c07aecdabf410633f5aa5418908d2461ef0dff0d9ae5d8203`
  e `schema_bundle_hash=ef7c1e4cec18f1491e4b72bd7dd35f49a1c28d77a5bdd89d435122074f5cba18`;
- causal identity, admission order, fence, coordinate, event order, idempotency, RNG, perception e
  perception identity policy: versão **e** hash;
- pares `(schema_id, schema_version)` sob `schema_bundle_hash`, mais versões/hashes de reducers,
  validators, resolvers e dependency footprints;
- limite versionado de cascata sem avanço temporal;
- snapshot inicial e estado/runtime inicial de triggers;
- atores elegíveis e refs de seus checkpoints epistemológicos iniciais.

Configuração causal não muda dentro do run. Nova política ou schema incompatível exige migração
explícita ou fork.

### 6.2 Cinco artefatos append-only

| Artefato | Contém | Não contém/decide |
|---|---|---|
| input ledger | inputs e closures exógenos; dispatch, revogação e resposta de slot | world truth, disposição terminal, prioridade por arrival |
| decision ledger | fence, material provisório, retry, `CycleCommit`/`DecisionRecord`, abort | cópia de eventos ou beliefs |
| event store | eventos imutáveis referenciados pelo commit journal | propostas, commits, observations, telemetria |
| evidence ledger | observations e `KnowledgeInput` particionados | verdade global ou belief inferida |
| causal outbox | `PerceptionTask` e completion receipts | payload irrestrito para agente ou autoridade de mundo |

Agenda, rounds, transmission, trigger runtime e ativações são estado autoritativo reconstruível por
eventos/reducers. Implementações podem manter índices materiais, mas eles não substituem o event
store nem os envelopes do decision ledger.

### 6.3 Objetos persistidos

Os campos e condições normativas são os schemas conceituais do ADR 0008. A implementação V1 fornece
schemas versionados e canonicalização para, no mínimo:

- `EligibilityCoordinate`, `CyclePlan`, `ExogenousInput`, `SourceClosure`, `RoundDeclaration`;
- `SlotDispatch`, `SlotDispatchRevocation`, `ActionProposal`, `NoProposal`;
- `ScheduledOccurrence`, `TriggerDefinition`, `TriggerRuntimeState`, `TriggerActivation`;
- `AdmissionFence`, `AffordanceAssessment`, `CommitCandidate`, `ConflictSet`;
- `RngDraw`, `ProvisionalDisposition`, `AttemptFailure`;
- `DecisionRecord`, `CycleCommit`, `CycleAbortRecord`, `AttemptRetryRecord`;
- `EventOrderKey`, `Event`, `PerceptionTask`, `PerceptionTaskCompletion`;
- `Proposition`, `Claim`, `Transmission`, `Observation`, `KnowledgeInput`;
- `Snapshot`, `EpistemicCheckpointRef` e manifesto de genesis.

Todo record causal imutável possui schema/version ou é envolvido por um envelope que os comprometa.
Campos set-like declaram ordering key, duplicate policy e identity key separadas. Maps são ordenados
por chave canônica. Representação default
da linguagem nunca é usada para digest, identidade ou state hash. O contrato exato de records,
primitivos, Unicode, floats, maps, sets, ids e strict decoding é o
[Canonical Causal Codec V1](../architecture/canonical-codec-v1.md); CDDL, registries e golden vectors
são a fonte independente de linguagem no
[`canonical-codec-v1-bundle`](../architecture/canonical-codec-v1-bundle/README.md). Todo texto é
classificado como `machine-id` ASCII, `domain-tag`/`schema-id` sob a ABNF mais restrita do envelope,
`iana-timezone` sob a gramática IANA versionada, `human-text` Unicode ou `ascii-uri`; human text nunca
participa de identidade, map key ou ordering.

### 6.4 Estado do ciclo

O pipeline obrigatório é:

```text
derive provisional CyclePlan and closure coordinate
→ await durable source closure
→ derive cohort and await one response per slot
→ persist and verify AdmissionFence
→ validate idempotency and expand the exact unit partition
→ assess affordances and conflicts against one base revision
→ resolve candidates and derive source lifecycle
→ build/order/validate EventBatch and working post-state
→ materialize trigger runtime/activations and perception tasks
→ atomically persist settlement + events + outbox + commit + revision
```

Qualquer falha após o fence produz `CycleAbortRecord` ou, se o processo caiu antes do envelope,
deixa `ATTEMPT_IN_FLIGHT` para reexecução a partir do mesmo fence. Nenhuma etapa intermediária é
autoritativa para settlement ou world state.

### 6.5 Fronteira transacional

Um commit bem-sucedido publica indivisivelmente:

1. consumo das unidades do fence;
2. exatamente um `DecisionRecord` por unidade;
3. eventos vencedores e todos os eventos de lifecycle obrigatórios;
4. `PerceptionTask` de cada evento potencialmente perceptível;
5. um `CycleCommit` no journal canônico do decision ledger;
6. a nova `WorldRevision` e seus hashes/cursores.

Se a tecnologia escolhida não oferece transação local sobre esses artefatos, o design precisa
oferecer commit determinístico por `fence_digest` e impedir que ciclo, replay, resume ou barrier
ultrapasse reconciliação incompleta. A escolha concreta está aberta na §14; enfraquecer atomicidade
não é opção.

### 6.6 Mudanças no snapshot

O snapshot causal V1 inclui:

- relógio, revisão, próximo logical sequence e ordinal;
- world state e state hash, incluindo o snapshot físico aninhado/adaptado;
- cursores/digests dos cinco artefatos;
- pendências de agenda, rounds, slots, triggers e inputs, com coordenadas/digests;
- `CycleControlState`, último envelope e refs de fence/retry quando aplicável;
- versões/hashes necessários a replay/resume;
- tasks sem completion e completion receipts; todo id de `KnowledgeInput` listado resolve um record
  durável, sem flag/lista paralela de confirmação;
- `EpistemicCheckpointRef` coberto para cada ator elegível;
- hash canônico do snapshot inteiro.

Cache e projeção reconstruível ficam fora. Belief/memória não são descartados como cache: entram por
referência epistemológica opaca e verificada.

---

## 7. Comportamento normativo e critérios por capacidade

### 7.1 Identidade, bytes e idempotência

- `canonical_bytes_v1` é o RFC 8949 core deterministic CBOR do envelope de seis campos
  `[h'43534600', 1, domain_tag, schema_id, schema_version, payload]`.
- `derive_id(domain_tag, components...)` é SHA-256 desse envelope sob domain tag ASCII exclusivo;
  schema id/version e policy hash são parte do contrato versionado.
- Records são arrays de aridade fixa; maps usam a ordem bytewise lexicográfica do encoding canônico
  da chave; ordered lists preservam ordem e set-like arrays seguem a total ordering/duplicate policy
  do schema.
- `human-text` é UTF-8 após NFC/NPSS Unicode 15.1.0; identificadores causais são `machine-id` ASCII.
  Integers têm range no schema; float é binary64
  finito em preferred encoding, com `-0.0` normalizado e non-finite rejeitado.
- IDs/digests dentro de records são byte strings de 32 bytes. A forma
  `csf1:<domain-tag>:sha-256:<lowercase-hex>` é somente view e nunca substitui esses bytes no hash.
- Input exógeno exige `producer_unit_key` estável; reentrega idêntica preserva id, e mesma chave com
  bytes distintos falha fechado.
- Identidade de entrega e `IdempotencyIdentity` são distintas. Alias novo entra em fence e recebe
  settlement próprio; nunca é pré-consumido no append.
- `idempotency_key` é optional discriminado: `ABSENT` não cria identidade; `PRESENT` persiste
  `IdempotencyIdentity` com run, kind, producer, ator, key, digest e policy ref. Mesmo namespace com
  digest lógico divergente aborta como `IDEMPOTENCY_CONFLICT`.
- Todo `CausalRef` resolve pelo `reference_kind` exatamente um root persistido e uma operação/preimage
  de id total e reconstruível dos fields persistidos; roots não possuem aliases canônicos. `InputRef`
  de slot inclui `slot_response_kind`; replay rejeita ref cujo dispatch ou identidade dependa de
  contexto externo implícito.
- UUID aleatório, wall clock, `ingress_seq`, posição de append e ordem de iteração são proibidos na
  alocação causal.

### 7.2 Fonte, fechamento e admissão

- Fonte exógena não declarada é rejeitada.
- `SourceClosure` é monotônico; o primeiro closure por `ingress_seq` que cobre a coordenada é a prova
  canônica e estável sob append.
- Input após closure não final recebe coordenada aberta posterior; após `final`, input ou novo
  closure é rejeitado.
- Slots são unidades derivadas e respostas são preenchimentos: não recebem coordenada nem closure.
- O fence só é gravado quando toda fonte cobre a coordenada exigida pelo plano e todo slot da coorte
  tem uma resposta única.
- Derivar o fence com os mesmos ledgers, revisão e versões produz os mesmos bytes e digest em qualquer
  ordem local de construção.

### 7.3 Rounds, dispatch e simultaneidade

- `RoundDeclaration` nasce no genesis ou em commit anterior, nunca diretamente de input, e persiste
  `run_id`, papel/ordinal de criação, slots e coordenada imutáveis para recomputar sua identidade.
- Todos os rounds elegíveis da revisão base formam uma única coorte e leem o mesmo snapshot.
- Um slot tem no máximo um dispatch aberto e uma resposta. Redespacho exige revogação durável.
- Resposta precisa coincidir em run, ciclo, round, slot, ator, revisão e instante com o dispatch
  aberto; mismatch não preenche o slot.
- Timeout é `NoProposal` contra o dispatch aberto. A escrita condicional determina qual resposta
  externa foi persistida; `derive()` não desempata entre respostas.
- Barrier epistemológico adia dispatch, mas não muda declaração, coordenada ou membresia da coorte.

### 7.4 Clock, agenda e triggers

- A menor coordenada pendente decide o próximo ciclo. Lacunas de ordinal são saltadas sem commit
  decorativo.
- Trabalho futuro produz ciclo `CLOCK_ADVANCE` no instante de origem, com plano durável e input vazio;
  o destino reseta ordinal para `-1`.
- Outputs do avanço têm piso `(to, 0)`; o coordinator recalcula a menor pendência no destino.
- Occurrence só termina por commit; `REJECT` e `DEFER` também encerram a identidade antiga.
- Recorrência e defer criam sucessor novo com provenance e coordenada explícita.
- Trigger avalia predicate puro apenas sob versão/dependências registradas. Edge, once e repeat têm
  runtime persistido; reavaliação isolada não duplica efeito.
- `REPEAT_WHILE_TRUE` exige `repeat_every` explícito e positivo; as demais policies proíbem esse
  campo.
- Transição que ativa trigger publica runtime e `TriggerActivation(PENDING)` na mesma transação da
  revisão de origem, elegível somente a ciclo posterior.

### 7.5 Validação, conflito e settlement

- Toda proposta produz facets tipadas. Proibição pode registrar violação sem bloquear; somente
  mecanismo causal de enforcement bloqueia ação fisicamente possível.
- `INDETERMINATE` aborta a tentativa inteira e identifica sujeito, validator, versão/hash, razão e
  evidência; nunca vira `SATISFIED`/`UNSATISFIED` implícito.
- Depois de `NoProposal` e deduplicação, `source_unit_ids` dos candidatos formam partição exata das
  unidades produtoras. Gap, overlap ou ref externa é `PROVENANCE_FAILURE`.
- Conflitos usam claims/invariantes e políticas explícitas. `LogicalSequence`, arrival e worker
  completion não são prioridade.
- Cada unidade admitida termina em `COMMIT`, `REJECT`, `DEFER`, `NO_PROPOSAL` ou `DEDUPLICATED` no
  mesmo `CycleCommit`; lifecycle obrigatório não é suprimido por ausência de evento de domínio.
- Abort não cria `DecisionRecord`, não consome fonte, não publica evento/revisão e não avança ordinal.
- `AttemptFailure` deriva identidade de tentativa + stage + component policy + ordinal nomeado pelo
  schema do componente; scheduling/completion de worker nunca aloca esse ordinal.
- Retentativa requer `AttemptRetryRecord`; attempt 1 não tem retry, attempt posterior resolve o retry
  que avança exatamente um ordinal desde o último abort do run. O novo fence preserva byte a byte
  plano, base, closure, coorte, unidades, input e policies do fence abortado, fora dos campos de
  retentativa autorizados. Resume sozinho nunca sai de `HALTED_ON_ABORT`.

### 7.6 Eventos, reducers e revisão

- Cada event type tem um owner e reducer puro/versionado. Reducer não emite evento, usa RNG, rede,
  LLM ou wall clock.
- `EventOrderKey` é persistida; sua ordem total define `event_id`, `LogicalSequence` e `event_ids[]`.
- `event_id` é recalculado em todas as fases antes de reducer. Evento `CANDIDATE_DOMAIN` é a projeção
  exata e versionada de draft + candidate + records-fonte por id/digest: schema, sources, atores,
  entidades, localização e confidentiality não podem ser escolhidos depois do candidate. Os records
  vêm dos owners registrados e verificam digest; o fence rejeita qualquer metadado além dos cinco
  campos canônicos de `admitted-unit`.
- `commit_successor_floor` é derivado de instant/ordinal/`CyclePlan`; não existe input livre do caller.
- Refs set-like são normalizadas antes de bytes/digest. Ordem com significado vive no payload
  tipado.
- Pais no mesmo ciclo só ligam eventos do mesmo candidato atômico e na ordem interna declarada.
- Um ciclo bem-sucedido publica nova revisão mesmo quando `event_ids=[]`; o commit journal, não um
  evento decorativo, prova esse avanço.
- Replay valida chave, id, posição, sequência, digests, revisão e state hash antes de reduzir.

### 7.7 Percepção, claims e comunicação

- Evento potencialmente perceptível cria `PerceptionTask` atomicamente; task não contém contexto de
  agente nem concede acesso.
- Resolver carrega o evento por port autorizado e produz observations/inputs por papel e ordinal de
  schema. Task resolve seu evento; cada observation resolve essa task, tem exatamente esse evento
  como fonte e conserva seu instante; knowledge input resolve a observation, destina-se ao mesmo
  observer e só projeta claims/evidence ali divulgados. Retry/concorrência derivam os mesmos ids.
- Policy/resolver deriva uma allow-list e uma projeção exata por observer a partir de task, evento,
  pre/post state e canal. Append rejeita observer não autorizado e percept/claim/evidence/omissão
  diferente da projeção, inclusive para eventos `SECRET`/`RESTRICTED`; allow-list/output final
  fornecido pelo caller não é authority.
- `observed_at` deriva do evento que tornou a evidência disponível; `received_at` é igual a ele.
  Latência operacional não altera cronologia.
- Completion só é gravado quando suas duas listas são as projeções canônicas exatas de todos os
  observations e knowledge inputs da task, sem extras, duplicatas ou omissões; completion vazio é
  obrigatório quando não há output elegível. Depois do completion, nenhum novo output para a task é
  aceito; task/commit repetem a mesma policy de identidade e observation/task o mesmo resolver.
- Persistir `KnowledgeInput` é a confirmação durável de entrega. Crash antes mantém a task sem
  completion; crash depois reutiliza o mesmo record idempotente. Não existe segundo ack/flag.
- `Claim` não possui verdade embutida. Comparação com world truth, quando uma regra exige, produz novo
  fato e nova cadeia de acesso.
- Envio não é entrega, entrega não é necessariamente leitura, e falsificação não revela autoria real.
- Context builder futuro recebe apenas background permitido, inbox do próprio holder e estado
  cognitivo próprio; nunca consulta event store global e redige depois.

### 7.8 Replay, resume, fork e Observatory

- Causal replay usa genesis + `CycleCommit` + eventos referenciados e reconstrói revisão, cursores e
  state hash, inclusive commits vazios.
- Snapshot+tails reproduz execução contínua. Checkpoint epistemológico é restaurado por hash e nunca
  regenerado por modelo.
- `ATTEMPT_IN_FLIGHT`, `RETRY_AUTHORIZED`, `HALTED_ON_ABORT` e `IDLE` são derivados pela precedência
  normativa do ADR; presença das refs, kind do último envelope e relações de ordinal são validados
  pela variante, e estado persistido divergente falha fechado.
- Fork cria novo `run_id`; entre policies, o genesis usa a referência histórica autocontida com
  policy/domain/schema/algoritmo de origem. Nenhum id ou evento pós-fork cruza timelines.
- Observatory tem somente query ports. Consultar segredo, POV ou view pública não cria observation,
  atualiza cursor de ator ou altera hash causal.

---

## 8. Conhecimento e visibilidade

### 8.1 Matriz de acesso

| Artefato | Engine/árbitro | Perception | Ator destinatário | Observatory privilegiado |
|---|---:|---:|---:|---:|
| world event completo | sim | por port autorizado | não | sim, read-only |
| `PerceptionTask` | coordinator/outbox | sim | nunca | diagnóstico read-only |
| `Observation` | evidence services | própria resolução | somente se `observer_id`/rota permitir | sim, read-only |
| `KnowledgeInput` | sink/store | produz | somente `recipient_id` | sim, read-only |
| observation/input de terceiro | somente infraestrutura autorizada | conforme tarefa | nunca | sim, read-only |
| `Claim` recebido | sim | por cadeia causal | sim, sem verdade autoritativa | sim |
| affordance, conflict, RNG e pormenor de decisão | decision ledger | não por default | nunca | sim, read-only |
| checkpoint epistemológico | hash/cobertura apenas | não | owner do contexto | conteúdo conforme modo privilegiado |

### 8.2 Allow-list de contexto

Nenhum prompt é implementado nesta V1, mas a fundação precisa tornar estruturalmente possível apenas
o caminho seguro. Um datum elegível a contexto deve resolver para exatamente uma destas origens:

1. background estático que passou pelos gates canônicos no genesis;
2. `KnowledgeInput` cujo `recipient_id` é o holder e cuja timeline/provenance são válidas;
3. estado cognitivo ou memória pertencente ao próprio holder, preservando evidence chain.

Event payload, task, decision material, telemetria, observation privada de terceiro, canon posterior
à divergência e query do Observatory nunca são origens permitidas.

### 8.3 Evals obrigatórias de fronteira

- evento `SECRET` sem observador produz completion vazio e nenhum recibo;
- evento `PUBLIC` sem canal/publicação não é implicitamente copiado a todos;
- papel secreto de exame não aparece antes de evento de divulgação endereçado;
- claim falso chega como claim e não cria o evento alegado;
- `presented_sender` pode divergir de `actual_sender` sem revelar este último;
- evento físico não entrega `BodyState`, posterior, RNG ou telemetria ao personagem;
- POV de A nunca lê partição de B;
- consulta privilegiada não altera nenhum ledger, checkpoint ou state hash.

---

## 9. Modos de falha

| Falha | Comportamento exigido |
|---|---|
| fonte não fecha coordenada | run aguarda; alerta operacional, sem corte semântico por timeout do coordinator |
| source/input depois de `final` | rejeição atômica e auditoria fora do causal digest |
| resposta atrasada, duplicada ou de dispatch revogado | rejeição; slot/fence permanecem inalterados |
| identidade igual com bytes diferentes | corrupção/provenance failure; falha fechado |
| idempotency namespace com digest divergente | `CycleAbortRecord(IDEMPOTENCY_CONFLICT)` |
| regra/dado/resolvedor ausente | `INDETERMINATE_FACET`; zero settlement e zero mutação |
| partição de candidatos inválida | `PROVENANCE_FAILURE` antes de resolução |
| reducer/schema/invariante falha | abort do lote inteiro; nenhuma das seis partes do commit persiste |
| cascade limit excedido | abort determinístico; nunca truncamento silencioso |
| crash com fence sem envelope | `ATTEMPT_IN_FLIGHT`; reexecuta mesmo plano/corte |
| crash depois de abort | `HALTED_ON_ABORT`; nenhuma retentativa implícita |
| crash entre retry e novo fence | `RETRY_AUTHORIZED`; cria exatamente o fence autorizado |
| crash depois do commit e antes da percepção | task pendente já existe; entrega idempotente antes de novo dispatch afetado |
| snapshot/checkpoint epistemológico ausente ou defasado | snapshot inválido; resume proibido |
| versão/hash/policy indisponível | replay/resume/migração falha fechado |
| event ref, ordem, sequence ou digest divergente | replay para antes de aplicar reducer |
| trabalho vencido encontrado em `IDLE` | violação de lifecycle; não é reaproveitado silenciosamente |
| segredo sem rota perceptiva | zero evidence; nunca fallback “todos viram” |

Telemetria deve distinguir espera legítima, rejeição de ingresso, tentativa abortada, crash em voo e
run parado. Nenhum desses estados pode ser inferido apenas por ausência de evento no event store.

---

## 10. Observabilidade e reprodutibilidade

### 10.1 Contrato reproduzível

Mesmos genesis/config/seed, bytes de input ledger, closures, registro de slots, coordenadas, retries e
versões/hashes devem produzir, byte a byte:

- mesma sequência de `AdmissionFence` e `CyclePlan`;
- mesmos input/fence/decision/abort/batch/perception digests;
- mesmos `DecisionRecord`, `Event`, `PerceptionTask`, `Observation`, `KnowledgeInput` e completions;
- mesmas revisões, cursores e state hashes;
- mesmo snapshot final.

Reexecutar modelo remoto não integra esse contrato. Uma nova resposta é outro experimento, não replay.

### 10.2 Auditoria causal mínima

Para cada ciclo, uma query de auditoria precisa reconstruir:

```text
coordinate + cycle_plan + base_revision
→ closure proof + cohort + admitted units
→ assessments + conflict sets + rng refs
→ one terminal outcome per admitted unit OR one abort envelope
→ ordered events + resulting revision
→ perception tasks + completions + addressed evidence
```

A query pode combinar stores como projeção descartável, mas precisa mostrar qual ledger é autoridade
de cada linha e verificar os digests antes de apresentar sucesso.

### 10.3 Telemetria operacional

Wall time, duração/custo de LLM, host, retries de transporte, tamanho de fila e worker id ficam fora
dos hashes causais. Métricas mínimas:

- idade da coordenada aguardando closure, por fonte;
- idade de dispatch aberto e task sem completion;
- contagem de respostas rejeitadas por razão;
- ciclos/attempts por status e abort reason;
- profundidade de cascata no instante;
- duração operacional de derive/evaluate/commit/perception/replay;
- divergências de digest, versão ou checkpoint como alertas críticos.

---

## 11. Testes e evals

### 11.1 Testes unitários e de propriedade

1. canonical encoding para inteiros, UTF-8 NFC, ids, records, maps e sets;
2. estabilidade de ids/digests sob permutação de map, coleção e worker completion;
3. colisão de identidade e bytes incompatíveis falham fechado;
4. monotonicidade/finalidade de `SourceClosure` e estabilidade de `covering_closure`;
5. totalidade de `next_cycle_coordinate` e `CycleControlState`;
6. unicidade de dispatch/resposta e vínculo fail-closed;
7. partição exata de unidades por candidatos;
8. bijeção admitted unit ↔ `DecisionRecord`;
9. ordem total de `EventOrderKey`, ids e logical sequences contíguos;
10. reducers puros, ownership único e state hash estável;
11. edge/once/repeat/rearm e lifecycle monotônico de agenda/trigger;
12. ids/completion de percepção idempotentes e instante epistemológico causal;
13. substreams independentes de avaliações/atores irrelevantes;
14. arquitetura sem cliente LLM no core e sem write port transitivo no Observatory;
15. os 620 vetores/casos de conformidade do bundle executados por pelo menos duas
    implementações independentes e linguagens diferentes;
16. strict decode rejeita CBOR não preferido, indefinite, tag, `undefined`, duplicate key, Unicode
    fora do profile, non-finite, negative zero, schema/enum desconhecido e id com tamanho incorreto;
17. mesmo valor sob domain/schema/version diferentes produz bytes/digests diferentes.

Property tests devem gerar permutações de ingresso sob o mesmo fechamento, registro local de
plugins, construção de refs, conclusão de validators/workers e crash points. Quando os fatos
persistidos forem os mesmos, os bytes canônicos também precisam ser.

### 11.2 Testes de integração e crash consistency

Cada cenário da tabela “Cenários de stress e provas de aceite” do ADR 0008 ganha teste nomeado. No
mínimo, a suíte cobre:

- respostas LLM simuladas em ordens opostas;
- rounds e input exógeno na mesma coordenada;
- input antes/depois de closure e closure final;
- `NoProposal`, alias no mesmo ciclo e alias de canônica histórica;
- `INDETERMINATE` depois de disposição provisória de outro candidato;
- crash em cada fronteira do commit atômico;
- os quatro estados de controle e retry explícito;
- advancement com nova pendência/trigger materializado no destino;
- outputs de candidatos e validators entregues em ordens opostas;
- task perceptiva parcialmente processada por workers concorrentes;
- comunicação atrasada, falsificada e não observada;
- commit sem evento seguido de causal replay;
- snapshot+resume no meio de transmission e com checkpoint epistemológico já produzido.

Um failure-injection harness derruba a execução antes/depois de cada append lógico e prova que o
estado recuperado é ou o prefixo anterior completo ou o commit completo — nunca uma combinação.

### 11.3 Knowledge-boundary audit

A integração de percepção, comunicação, snapshot epistemológico e adapter físico exige a skill
`knowledge-boundary-audit` antes de ser considerada pronta. A suíte inspeciona payloads por
destinatário e tenta explicitamente acessar segredo, observation de terceiro, canon futuro,
telemetria física e output privilegiado do Observatory.

### 11.4 Evals de LLM, RAG e simulação

| Eval | Exigência V1 |
|---|---|
| LLM eval | não roda no core. Um contract eval futuro pode trocar responder roteirizado por modelo e verificar somente schema/vínculo; comportamento de personagem pertence a outra spec |
| RAG eval | não aplicável ao core; nenhuma recuperação é adicionada. A allow-list deve rejeitar chunk sem gates, mas qualidade de retrieval fica fora |
| simulation eval | obrigatório: cenário de referência, invariantes causais, isolamento, replay e resume sob seeds/paralelismos/crash points variados |
| determinism eval | obrigatório: comparar ledgers e snapshots byte a byte, não apenas estado semanticamente equivalente |
| embodiment integration eval | obrigatório: mesma resolução física resulta no mesmo envelope/revisão; nenhum número proibido alcança evidence/context payload |

Mudança posterior em prompt, modelo, roteamento ou formação de belief que altere decisão ativa
`llm-evals`; mudança em retrieval ativa `rag-evals`. Essas avaliações não são antecipadas dentro do
core causal.

### 11.5 Definition of done

A V1 está aceita quando:

1. todas as 41 invariantes do ADR possuem teste rastreável;
2. todos os cenários de stress do ADR possuem teste de integração ou propriedade rastreável;
3. o cenário visível da §1 roda sem LLM e gera relatório causal verificável;
4. permutar workers/paralelismo não altera nenhum byte autoritativo;
5. causal replay e snapshot+resume igualam execução contínua;
6. failure injection não encontra estado parcial publicável;
7. knowledge-boundary audit não encontra vazamento;
8. Observatory não alcança qualquer write port;
9. adapter físico passa determinism e isolation evals;
10. documentação de políticas, schemas, migração e operação permite diagnosticar toda parada sem
    consultar memória do processo.

---

## 12. Migração e compatibilidade

### 12.1 Estado atual

Não existe snapshot causal completo anterior. Existe `snapshot_version: 1` e JSONL locais do
`Embodiment`; eles são artefatos de componente, não `WorldRevision`, event store global ou commit
journal. A V1 não pode reinterpretar silenciosamente `seq`, `sim_time` textual ou ordem de arquivo
como `LogicalSequence`, `SimulationInstant` ou causal order globais.

### 12.2 Estratégia V1

- o snapshot causal recebe namespace e versão próprios;
- o snapshot físico é aninhado ou convertido por adapter versionado, preservando seu hash de origem;
- eventos físicos novos entram por adapter com `EventOrderKey`, ids, provenance e reducer globais;
- runs demo anteriores permanecem legíveis pelas ferramentas do `Embodiment`, mas não são
  automaticamente promovidos a runs causais resumíveis;
- ferramenta de importação, se adicionada, cria um novo genesis/fork explícito e relatório de campos
  convertidos, sintetizados ou indisponíveis; não inventa history/settlement retroativo;
- versão ou migration function ausente falha fechado;
- snapshots causais V1 são forward-incompatible por default: nova versão exige migrator determinista
  que valide hashes antes e depois, ou fork a partir de checkpoint suportado.

### 12.3 Compatibilidade de políticas

Replay exige a implementação exata identificada por versão **e** hash. “Mesma semver” com bytes de
política diferentes é corrupção. Remover uma política antiga do runtime torna o run não resumível até
que o artefato seja restaurado ou uma migração/fork explícito seja executado.

O codec é imutável dentro do run. Mudança local de layout/type/range, enum/role ou ordering/duplicate
policy cria schema version/bundle novo; mudança global em framing, profile CBOR, primitivos, Unicode,
identifier grammar, map ordering, float, digest ou sua forma textual cria `canonical_bytes_v2`.
Nenhuma versão nova entra numa run se não estava pinada no genesis. A migração verifica o checkpoint
V1 e abre um fork com novo `run_id`/policy, preservando o prefixo e os ids V1 para replay; entre
policies, o genesis usa o
[`Cross-policy Checkpoint Reference V1`](../architecture/cross-policy-checkpoint-reference-v1.md),
conforme o [contrato do codec](../architecture/canonical-codec-v1.md#10-evolução-e-migração).

---

## 13. Sequência de entrega recomendada

Cada item deve ser um tracer bullet com testes e artefato executável; a divisão final cabe a
`/to-tickets` depois que a §14 estiver resolvida.

1. **Contrato canônico:** manifesto, value objects, canonical bytes, ids/digests, policies e stores
   append-only em memória para teste.
2. **Ingresso e corte:** fontes, closures, coordenadas, rounds, slots, coorte e fence verificável.
3. **Tempo e lifecycle:** clock, avanço, schedule, trigger runtime/activation e cascade limit.
4. **Decisão mínima:** propostas, facets, recurso único, candidatos, partição, conflitos e
   idempotência.
5. **Commit terminal:** event ordering, reducers, lifecycle, commit/abort, control state e atomicidade.
6. **Evidência:** outbox, observation, `KnowledgeInput`, barrier e allow-list estrutural.
7. **Comunicação:** claim, transmission e entrega/falha agendada sem promoção a verdade.
8. **Continuidade:** replay, snapshot/resume, checkpoint epistemológico opaco, fork e failure injection.
9. **Superfícies:** Observatory read-only, relatório causal e telemetria operacional.
10. **Integração:** adapter do `Embodiment`, cenário de referência e evals fim a fim.

Regras de exame, cognition e memória sofisticada entram somente depois que esses contratos tiverem
provas de determinismo, crash consistency e isolamento.

---

## 14. Decisões e bloqueios

O [ADR 0008](../adr/0008-causal-simulation-foundation-v1.md) está `Accepted` desde o merge
`a669ef6`; sua aceitação não é bloqueio. Codec/digest também está fechado pelo
[ADR 0009](../adr/0009-canonical-causal-codec-and-digests.md) e pelo
[Canonical Causal Codec V1](../architecture/canonical-codec-v1.md), inclusive seu
[policy/schema bundle imutável](../architecture/canonical-codec-v1-bundle/README.md).

As três decisões abaixo continuam abertas. Esta revisão não as antecipa.

### 14.1 Stack da fundação — bloqueante para código de produção

O ADR 0007 escolhe Python somente para `Embodiment`; não é precedente para esta fundação. Um ADR de
stack precisa escolher a linguagem/runtime do Simulation Engine causal e explicar a fronteira com o
pacote físico. O protótipo de contratos pode usar uma implementação de teste descartável, mas não
deve transformar essa conveniência em decisão arquitetural implícita.

### 14.2 Topologia de persistência e atomicidade — bloqueante para o incremento 5

O ADR exige atomicidade entre decision ledger, event store, causal outbox e revisão, mas não escolhe
banco ou topologia. Uma pesquisa curta seguida de ADR deve comparar pelo menos:

- uma transação local sobre tabelas/streams no mesmo banco;
- commit determinístico/reconciliável por `fence_digest` sobre stores separados;
- estratégia de snapshot, retenção, backup e failure injection para cada opção.

A decisão precisa demonstrar que ciclo posterior, replay, resume e barrier não ultrapassam um commit
incompleto. “Eventual consistency” sem esse protocolo não satisfaz o ADR.

### 14.3 Interface do adapter físico — bloqueante apenas para o incremento 10

Antes da integração, um contrato versionado precisa mapear o JSONL/snapshot atual do `Embodiment`
para schemas globais sem confundir sequência local com global, nem permitir que o pacote publique
revisão diretamente. A decisão pode ser tomada em um ticket de design do adapter, desde que produza
schema/fixtures revisáveis antes do código de integração e preserve os ADRs 0006/0007.

Nenhuma das decisões 14.1–14.3 altera a semântica causal do ADR 0008. Se uma alternativa exigir
enfraquecer identidade, atomicidade, isolamento ou replay, ela reabre o ADR em vez de ser resolvida
localmente.
