# Modelo físico

Domínio do corpo: capacidade, estado, esforço, desempenho, dor, lesão e disputa física.

Este documento é o modelo de domínio. A decisão que o instituiu está no
[ADR 0006](../adr/0006-physical-domain-model.md). Vocabulário de dimensões em
[`data/canon/schema/enums/capacity-dimensions.yaml`](../../data/canon/schema/enums/capacity-dimensions.yaml).

## 1. O erro que este modelo existe para impedir

`strength: 95`.

Um atributo escalar por personagem colapsa seis coisas diferentes em um número e, ao fazer isso,
torna impossível representar o caso mais característico da obra: alguém que rende deliberadamente
menos do que pode. Se desempenho observado *é* a capacidade, um personagem que termina no meio do
pelotão **é** medíocre, para sempre, e a simulação passa a contradizer a premissa do material de
origem. Se em vez disso escrevermos `strength: 95` porque "sabemos" que ele é forte, estamos
inventando cânone e pré-decidindo comparações que deveriam ser resultado da simulação.

As duas saídas são erradas pelo mesmo motivo: confundem **o que o corpo pode fazer** com **o que o
corpo fez** e com **o que alguém viu**.

## 2. Seis estruturas distintas

| # | Estrutura | Pergunta que responde | Observável? |
|---|---|---|---|
| 1 | `CapacityProfile` | o que este corpo pode fazer, descansado, em condição neutra, a esforço máximo | **nunca** diretamente |
| 2 | `BodyState` | em que condição este corpo está agora | não (é world truth) |
| 3 | `ExertionIntent` | quanto o agente escolheu gastar, e quanto escolheu mostrar | não |
| 4 | `PerformanceOutcome` | o que de fato aconteceu | sim, pelo engine |
| 5 | `ObservedPerformance` | o que um observador específico percebeu | é a própria observação |
| 6 | `SelfPhysicalModel` | o que o personagem acredita sobre o próprio corpo | é crença, e pode estar errada |

Nenhuma implementação pode fundir duas destas. A fusão mais tentadora — 1 com 4 — é exatamente a que
destrói o domínio.

## 3. O pipeline

O fluxo é estritamente unidirecional. Cada seta pertence a um único bounded context.

```
CapacityProfile (latente, congelada no snapshot)
        ⊗ BodyState(t)            ← sono, nutrição, hidratação, fadiga, lesão, dor
        ⊗ Environment(t)          ← calor, terreno, piso, carga, visibilidade
        = capability_available(t)                        [Simulation Engine · Embodiment]
                │
                ├── ExertionIntent (intensidade + teto de exibição + risco)   [Agent Cognition]
                │
                ▼
        PerformanceOutcome  ──► custo fisiológico ──► BodyState(t+Δ)   (persiste)
                │           └─► sorteio de lesão ──► Injury            (persiste)
                ▼
        ObservedPerformance[observador]        (canal, erro, pistas visíveis)
                │
                ▼
        CapacityBelief[observador][alvo]       (limites, não estimativas)   [Agent Memory]
```

Ler ao contrário — de desempenho para capacidade — é inferência, e inferência produz **limites**,
nunca valores. Isto vale igualmente para nós, lendo feats do cânone, e para um personagem, olhando
um rival correr. É a mesma máquina (§10).

## 4. Dimensões de capacidade

Uma dimensão só existe se algum feat canônico ou alguma atividade do engine puder se ligar a ela.
Cada dimensão tem **unidade física** e ancoragem em um teste observável; percentil de coorte é uma
*view derivada*, nunca o armazenamento primário.

| Dimensão | Ancoragem observável | Unidade interna |
|---|---|---|
| `aerobic_capacity` | corrida de resistência, ritmo sustentado | mL·kg⁻¹·min⁻¹ (proxy VO₂max) |
| `anaerobic_power` | arranque, salto, arremesso | W·kg⁻¹ de pico |
| `sprint_speed` | 50 m | m·s⁻¹ |
| `max_strength` | preensão manual, empurrar/levantar | N, e razão força/massa corporal |
| `strength_endurance` | abdominais, pendurar-se, repetições | repetições, ou s a % de máximo |
| `agility` | mudança de direção | s em percurso padronizado |
| `coordination` | precisão motora sob pressão | taxa de acerto |
| `reaction_time` | latência de resposta | ms |
| `flexibility` | amplitude articular | cm (sentar-e-alcançar) |
| `injury_resilience` | robustez tecidual | multiplicador de risco (adimensional) |
| `recovery_rate` | velocidade de dissipação de fadiga | constante de tempo, h |
| `thermoregulation` | tolerância a calor/frio | offset tolerado, °C |
| `pain_tolerance` | limiar e tolerância a estímulo nocivo | percentil de coorte |
| `body_mass`, `stature` | antropometria | kg, cm |

Dimensão sem unidade física (`pain_tolerance`) é permitida **somente** quando não existe unidade, e
então precisa declarar a coorte de referência. Isto impede que "0–100 arbitrário" volte pela porta
dos fundos.

### Capacidade não é habilidade, e nenhuma das duas é disposição

Três eixos, três donos:

| Eixo | O que é | Dono |
|---|---|---|
| **Capacidade** | o que o organismo produz | `CapacityProfile` (engine, semeado por cânone + prior) |
| **Habilidade** | técnica, tática, repertório motor (`skill.judo`, `skill.sprint_pacing`) | Canon Knowledge (formação, clube) + adaptação em simulação |
| **Disposição** | disposição a machucar, a se machucar, a se esforçar, a se entediar | Character Core / Agent Cognition |

Um aluno forte e destreinado perde de um judoca leve. Um personagem capaz que abandona a prova por
tédio não revelou um teto aeróbico — revelou disposição. Misturar os três eixos é o segundo modo de
falha mais comum depois do escalar único.

### Capacidade é lenta, não constante

`capacity_baseline(t)` é uma linha de base que **deriva devagar** ao longo do ano por adaptação ao
`cumulative_load` (treino) e por destreino. Uma simulação de um ano letivo com clubes, exames físicos
e ilha não pode tratar capacidade como constante. `capability_available(t)` é a linha de base já
modulada por estado e ambiente, e muda de hora em hora.

## 5. Capacidade latente: prior, restrições, posterior, amostra

Capacidade é **incerta para nós** (pesquisadores) e **definida para o engine** (world truth). As duas
coisas se conciliam em quatro passos:

```
population_prior(coorte)          ← data/models/physical/  (NÃO cânone)
        ⊗ constraints[]           ← feats canônicos, tipados como limites
        = capacity_posterior      ← distribuição, com versão e sufficiency registradas
        ─(amostra, seed do run)─► capacity_baseline(Y1_START)   congelada no snapshot
```

Regras:

1. **Toda a população usa o mesmo tipo.** Um personagem focal é um prior com muitas restrições; um
   NPC sem evidência é o mesmo prior com zero restrições. Não existem dois sistemas.
2. **Ausência de evidência é prior, não palpite.** Aluno anônimo sem feats recebe um sorteio da
   coorte condicionada ao que o cânone realmente informa (ano, sexo, clube ou ausência de clube,
   reputação atlética, porte). Nunca um número escolhido à mão.
3. **A amostra é congelada.** Sorteada uma vez, no world seeding, do posterior, com o seed do run;
   gravada no snapshot; nunca re-sorteada por cena. NPC criado tardiamente sorteia de um substream
   derivado de `hash(character_id, world_seed)`, de modo que a ordem de criação não altere o
   resultado.
4. **Metadados de reprodutibilidade.** Versão do prior, versão do estimador, hash do posterior e o
   seed entram nos metadados do run, como o hash do snapshot canônico do ADR 0005.
5. **`evidence_sufficiency` é campo de primeira classe.** Um perfil que é 95% prior e 5% evidência
   deve dizê-lo. Um eval pode então recusar afirmações fortes sobre personagens sem base.

`population_prior` é reconstrução nossa calibrada contra dados reais de coorte (o teste nacional
japonês de aptidão física fornece itens e normas por idade e sexo: 50 m, arremesso, salto horizontal
parado, abdominais, sentar-e-alcançar, preensão manual, vaivém/corrida de resistência). Os itens
determinam a ancoragem das dimensões; **as normas numéricas precisam ser transcritas da publicação,
não da memória do modelo.** Enquanto não transcritas, o prior é declaradamente provisório.

## 6. Feats canônicos como restrições

Um feat é uma observação de desempenho. Ele **restringe** a capacidade; não a mede.

| Tipo de restrição | Quando se aplica | O que autoriza concluir |
|---|---|---|
| `LOWER_BOUND` | fez X, esforço desconhecido | capacidade ≥ X, desnormalizado por estado e ambiente. Nada mais. |
| `UPPER_BOUND` | tentou ao máximo e falhou, **com esforço máximo atestado** | capacidade < X. Raro e valioso. |
| `INSTRUMENTED_ESTIMATE` | registro medido oficialmente (teste de aptidão da escola) | estimativa bilateral estreita, **ainda condicionada** a não haver ocultação |
| `COMPARATIVE` | A superou B no mesmo evento, mesmas condições | ordem entre *desempenhos realizados*; transfere para capacidade só com hipótese de esforço explícita |
| `TESTIMONY` | alguém afirma que X é forte | crença de um holder. Alimenta belief state, **não** o posterior |
| `NO_INFORMATION` | desempenho com esforço notoriamente submáximo e sem sinal de esforço | nada acima do piso já conhecido |

A assimetria é o ponto: **pisos são baratos, tetos são caros.** Um personagem que nunca foi visto no
limite tem cauda superior larga — e essa largura é a representação honesta da nossa ignorância, não
uma afirmação de que ele é secretamente o mais forte.

`effort.attestation` registra *como* sabemos o nível de esforço: narração de esforço/exaustão,
declaração do próprio personagem, leitura de terceiro, incentivo estrutural para ocultar, ou
`UNKNOWN` — o padrão. O schema recusa `UPPER_BOUND` sem atestação maximal, do mesmo modo que recusa
`not_before` sem `not_before_support`. É a mesma disciplina do ADR 0005 aplicada ao corpo.

### Corpus compartilhado, não RAG por personagem

Feats vivem em **uma** coleção, [`data/canon/feats/`](../../data/canon/feats/), indexada por ator,
modalidade e dimensão. Não há store por personagem. Três razões:

- restrições `COMPARATIVE` envolvem dois ou mais atores e não têm dono único;
- normalização de condições (o mesmo dia, a mesma prova, o mesmo calor) exige ver os participantes
  juntos;
- um store por personagem convida a duplicar o mesmo evento com números divergentes.

O corpus compartilhado tem dois consumidores distintos, e a distinção importa: o **estimador de
capacidade** o lê no seeding (números, restrições) e o **RAG de fidelidade** o lê em execução
(paráfrase, como aquela pessoa encara um desafio físico). O RAG nunca recupera o posterior numérico.

### Feat posterior à divergência

`divergence_gate` do ADR 0005 se aplica com uma distinção que só aparece no domínio físico: um corpo
não muda porque a linha do tempo bifurcou. Portanto:

- feat posterior à divergência **pode** informar o posterior de capacidade no seeding;
- feat posterior à divergência **nunca** entra em memória episódica nem em belief state de agente;
- depois do seeding, não informa mais nada.

Sem essa distinção, ou perdemos evidência legítima sobre um corpo, ou abrimos canal de vazamento.

## 7. `BodyState`: consequência que persiste

World truth, propriedade exclusiva do engine, presente no snapshot, indexado por tempo de simulação.

```yaml
body_state:
  character_id: ...
  t: <sim time>

  peripheral_fatigue:            # por região: legs, arms, grip, core
  central_fatigue:               # SNC: alerta, controle executivo, expressão de técnica

  cumulative_load:
    acute_7d: ...
    chronic_28d: ...             # razão aguda:crônica alimenta risco de lesão

  sleep:
    debt_hours: ...
    hours_since_wake: ...
    circadian_phase: ...
    last_sleep: {start, end, quality}

  energy:
    last_meal_at: ...
    balance_kcal_24h: ...
    substrate_availability: 0..1  # proxy de glicogênio

  hydration:
    deficit_pct_body_mass: ...    # perda rápida e severa de desempenho no calor

  thermal:
    core_offset_c: ...
    clothing_insulation: ...

  soreness: {}                    # DOMS, atraso de 24-48 h
  injuries: [Injury]
  illnesses: [Illness]
  pain:
    by_region: {}
    global_intensity: ...
    analgesia: ...                # adrenalina, medicação: suprime sinal sem curar tecido
```

Invariante de persistência: **cena não cura nada.** Só o avanço do relógio com as regras de
recuperação, e eventos do engine, alteram `BodyState`. Uma noite mal dormida na segunda-feira ainda
está no corpo na quarta.

### Sono, nutrição, hidratação, ambiente

- **Sono** não é booleano. `central_fatigue` e `reaction_time` derivam de dívida de sono, fase
  circadiana e horas acordado. `max_strength` é quase insensível a privação de sono; precisão,
  decisão e expressão de técnica não são. É por isso que a semana de provas tem consequência física
  mesmo sem esforço físico.
- **Nutrição e hidratação** são recursos de escala de horas a dias. Racionamento em exame de
  sobrevivência degrada `substrate_availability`, e daí `aerobic_capacity` disponível, antes de
  qualquer lesão. Desidratação no calor é o mecanismo mais rápido de queda de desempenho.
- **Ambiente** é world truth do engine: temperatura, umidade, terreno, piso, inclinação, carga
  transportada, visibilidade, calçado. Entra como modificador de `capability_available` e como fator
  de risco de lesão. **Calor é ambiente; sede é interocepção** (§9) — o agente sente sede, não lê
  `hydration.deficit_pct_body_mass`.

### Dor e lesão são coisas diferentes

Separadas porque se dissociam nas duas direções:

- dor sem lesão: DOMS, cãibra, esforço extremo;
- lesão sem dor proporcional: adrenalina, choque, ocultação, analgesia.

`Injury` é estrutura própria e persistente:

```yaml
injury:
  region: ankle_left
  tissue: ligament            # muscle | ligament | tendon | bone | joint | skin | head
  mechanism: twist            # overuse | impact | twist | laceration | strain
  severity: 0..3
  onset: <sim time>
  healing: {expected_days, progress: 0..1, setback_on_load: <regra>}
  impairments: {sprint_speed: 0.55, agility: 0.4}   # multiplicadores por dimensão
  pain_profile: {at_rest: ..., on_use: ...}
  observable_cues: [limp, guarding, swelling, bandage]
  concealment: {attempted: true, central_fatigue_cost: ..., leak_probability: ...}
  treated: false
```

Uma lesão restringe **dimensões específicas**, não um número global: um pulso comprometido derruba
preensão e arremesso e deixa corrida quase intacta. É isso que permite que um exame que exige força
de mão seja decidido por uma lesão que ninguém narrou.

### Risco de lesão

Estocástico, resolvido pelo engine, nunca pelo LLM:

```
p_lesao = base(atividade)
        × carga(intensidade / capability_available)      # exceder o disponível é o driver principal
        × estado(fadiga, dívida de sono, hidratação, soreness, razão aguda:crônica)
        × ambiente(piso, calçado, visibilidade, contato)
        ÷ injury_resilience
        × histórico(lesão prévia na mesma região)
```

`pain_override` e `risk_acceptance` altos elevam a exposição porque removem o freio comportamental —
é assim que "ignorar a dor" tem preço em vez de ser adjetivo narrativo.

## 8. Esforço escolhido e ocultação

O agente não conhece sua capacidade latente. Ele escolhe sobre o que percebe como disponível.

```yaml
exertion_intent:
  actor: ...
  activity_ref: ...
  target_intensity: 0..1        # fração da capability que o agente ACREDITA ter disponível
  pacing: even | conserve | negative_split | surge
  display_ceiling:              # ocultação como intenção de primeira classe
    metric: rank | time | visible_strain
    target: "terço médio do pelotão"
  risk_acceptance: low | medium | high
  pain_override: none | partial | full
  hard_limits: ["não revelar formação em artes marciais"]
```

Resolução pelo engine, na ordem, toda ela determinística exceto os sorteios nomeados:

```
1. capability_available = capacity_baseline(t) ⊗ impairments ⊗ estado ⊗ ambiente
2. validação: intent impossível é REJEITADO (nunca reinterpretado como sucesso — CONTEXT.md inv. 5)
3. alvo = capability_available × target_intensity, truncado
4. viabilidade da ocultação: o display_ceiling é alcançável dado o campo de competidores?
      → { ok | forced_exposure | forced_loss }
5. realizado = alvo ⊗ ruído(substream) ⊗ decaimento de pacing ao longo da duração
6. custos: Δfadiga periférica/central, Δsubstrato, Δhidratação, Δtérmico, Δcumulative_load
7. sorteio de lesão(substream)
8. atualização de dor
9. emissão de observação por observador (canal, erro, pistas)
10. append no event log
```

O passo 4 é o que torna ocultação um problema estratégico real em vez de enfeite: para permanecer no
terço médio quando o pelotão inteiro acelera, é preciso ou acelerar (e expor) ou perder posição (e
aceitar o custo). O engine decide qual, e o agente descobre o resultado como qualquer outro.

**Detecção.** Ocultar tem probabilidade de falhar:

```
p_deteccao = f(capacidade de inferência do observador,
               margem entre exibido e real,
               vazamento de pistas (respiração, recuperação rápida, técnica involuntária),
               nº de observações do mesmo alvo ao longo do tempo,
               atenção e suspeita prévia do observador)
```

Resultados medíocres repetidos com margem suspeitamente constante são, eles mesmos, sinal:
a suspeita acumula no belief state do observador — **não** no corpo do ocultador. O ocultador nunca
vê o resultado do sorteio de detecção; no máximo percebe pistas secundárias. Aqui vale o ADR 0001 na
íntegra: inteligência alta melhora a inferência do observador, não lhe dá acesso ao `BodyState` nem
ao `ExertionIntent` do outro.

## 9. Propriocepção: o corpo percebido

`SelfPhysicalModel` é **crença**, vive em Agent Knowledge & Memory, e é sistematicamente enviesado.

| Campo de `BodyState` | Sinal interoceptivo entregue ao agente |
|---|---|
| `peripheral_fatigue` | "pernas pesadas", "braços falhando" |
| `central_fatigue` + sono | "difícil manter foco", "reação atrasada" |
| `hydration` | sede, boca seca, tontura |
| `energy` | fome, fraqueza, tremor |
| `injury.severity` | dor localizada, instabilidade — **com viés**, frequentemente subestimando |
| `capacity_baseline` | autoavaliação calibrada ou não, conforme experiência corporal |

Regra dura: **nenhum número de `BodyState` chega ao prompt.** O context builder entrega sinais
qualitativos, enviesados por um `body_awareness` do personagem e pela analgesia corrente. Um
personagem com alta consciência corporal tem viés baixo; alguém em adrenalina subestima uma lesão e
continua correndo — e o engine, que conhece a verdade, aplica o agravamento.

E, simetricamente: `BodyState` de outro personagem só chega a um agente via `ObservedPerformance` e
pistas visíveis. Não há canal direto.

## 10. Um estimador, dois clientes

A inferência de capacidade a partir de desempenho aparece duas vezes, com a mesma matemática:

| | Cliente | Entrada | Quando | Saída |
|---|---|---|---|---|
| A | Nós, pesquisadores | feats canônicos | seeding | `capacity_posterior` → amostra congelada |
| B | Um agente, em execução | `ObservedPerformance` | continuamente | `CapacityBelief` sobre outro |

Mesmos tipos de restrição do §6, mesma assimetria piso/teto, mesmo tratamento de esforço não
atestado. A diferença é só a qualidade do inferidor: um observador arguto aplica limites mais
estreitos e desconta ocultação melhor; um observador ingênuo lê desempenho como capacidade — e
erra, exatamente como um modelo mal desenhado erraria.

Isto entrega três coisas de uma vez: ocultação passa a ter valor estratégico mensurável, "inteligência
melhora inferência e não acesso" ganha implementação concreta, e a nossa própria disciplina de
pesquisa e a cognição dos personagens compartilham um único conceito auditável.

## 11. Combate

Combate não é sistema separado com atributos próprios. É um **resolvedor de disputa** que consome os
mesmos primitivos.

```yaml
contest_spec:
  modality: scuffle | grapple | restrain | strike | pursuit | sport | physical-task
  participants:
    - actor: ...
      objective: subdue | escape | deter | hurt | hold_back | lose_deliberately
      exertion_intent: <§8>
  constraints:
    ruleset: none | sport_rules | institutional
    terrain, footing, space, visibility, objects_present, third_parties
  initiative: {surprise, mutual_awareness}
  seed_substream: ...
```

Resolução por trocas sucessivas; cada troca lê capacidade × habilidade × estado × ambiente × objetivo,
consome esforço, pode produzir controle posicional, dor e sorteio de lesão, e emite observações.
Propriedades que o resolvedor precisa ter:

- **Habilidade domina em diferenças pequenas de capacidade; capacidade domina em diferenças grandes.**
  Técnica tem retorno decrescente contra desvantagem física ampla.
- **Fadiga central e dor degradam a expressão de técnica mais rápido do que degradam força bruta.**
  Disputas longas favorecem condicionamento; disputas curtas favorecem técnica e iniciativa.
- **Objetivos assimétricos mudam a função objetivo**: escapar não é vencer, conter não é ferir. Um
  participante com `hold_back` ou `lose_deliberately` usa o mesmo `display_ceiling` do §8.
- **Consequências institucionais não vivem aqui.** Deduções de conduta, advertência, expulsão são do
  contexto de regras da escola. O combate emite eventos; a escola os julga.
- **O corpo não se resolve no fim da cena.** Toda saída de combate escreve em `BodyState` como
  qualquer outro esforço. A lesão vai para o próximo exame.
- **Lutar revela.** Combate é o evento de divulgação de capacidade mais informativo que existe; para
  um ocultador, esse é o custo dominante de lutar, e o modelo precisa deixá-lo visível na decisão.

## 12. Fronteiras: quem é dono de quê

Resolução física é **subdomínio do Simulation Engine** (`Embodiment`), não um bounded context par.
Um "Physical Engine" ao lado do engine criaria dois módulos capazes de decidir o que aconteceu, que é
precisamente o sintoma de domínio mal recortado.

| Estrutura | Dono único | Onde vive |
|---|---|---|
| `PhysicalFeat`, formação/clube, habilidade canônica | Canon Knowledge | `data/canon/feats/` |
| priors populacionais, ancoragem de unidades, estimador, dinâmica de fadiga, parâmetros do resolvedor | Simulation Models (**não cânone**) | `data/models/physical/` |
| `capacity_baseline` amostrada, e sua deriva no ano | Simulation Engine · Embodiment | world state / snapshot |
| `BodyState`, recuperação, lesão, dor | Simulation Engine · Embodiment | world state / snapshot |
| ambiente, clima, terreno | Simulation Engine | world state |
| `ExertionIntent` (proposta), ocultação, apetite a risco | Agent Cognition | ação proposta |
| resolução de esforço e de disputa | Simulation Engine · Embodiment | engine |
| `ObservedPerformance` | engine emite → Agent Memory guarda | event log / memória |
| `SelfPhysicalModel`, `CapacityBelief` sobre outros | Agent Knowledge & Memory | crenças |
| reputação de força na escola | Social State | estado social |
| tarefas físicas de exame | Examination Engine **consome** | ExamSpec |

### Armadilhas de verdade duplicada

- `core.capabilities: []` do [ADR 0002](../adr/0002-character-canon-rag-and-memory.md) **não** guarda
  números físicos. Guarda habilidades e disposições, e no máximo referencia o perfil de capacidade.
- `ExamSpec` **não** carrega atributos físicos por participante. Exame lê `BodyState`.
- Narração de LLM **não cria, não cura e não anula** lesão, fadiga ou capacidade. Texto que pressupõe
  uma lesão inexistente no estado é ação inválida, não retrocausalidade.
- Reputação (Social State) ≠ crença de um agente (Agent Memory) ≠ verdade (engine). Três registros,
  três donos, valores frequentemente diferentes — e é aí que mora o jogo.
- RAG não recupera "ficha de capacidade". Recupera feats como evidência comportamental; o posterior
  numérico é insumo de seeding, não documento recuperável.

## 13. Determinismo e reprodutibilidade

- Fadiga, recuperação, dívida de sono, hidratação, cura e modificadores ambientais são **funções puras**
  de estado e tempo: teste comum, sem LLM. Alvo direto da skill `tdd`.
- Sorteios estocásticos — amostra de capacidade, ruído de desempenho, lesão, detecção de ocultação,
  erro de observação — vêm de **substreams nomeados** derivados de `(world_seed, character_id, event_id,
  purpose)`. Consequência prática: acrescentar um personagem irrelevante não desloca os sorteios de
  outro, e dois runs podem ser diferenciados.
- Entram nos metadados do run: `world_seed`, versão do prior populacional, versão do estimador,
  versão dos parâmetros de dinâmica e do resolvedor, hash do posterior por personagem.
- Nada neste domínio precisa de LLM para decidir resultado. O LLM escolhe intenção; o engine decide
  o que aconteceu.

## 14. Cenários de estresse

Cada cenário quebra um modelo mais simples.

**1 — O que termina no meio do pelotão de propósito.** Corre a 60% e chega 15º de 40. Modelo escalar:
registra "mediano", erra para sempre. Aqui: `LOWER_BOUND` no ritmo do 15º lugar, cauda superior
intacta, `effort.attestation: STRUCTURAL_INCENTIVE_TO_CONCEAL`. Um aluno comum que chegou 15º a 100%
gera **a mesma observação** com verdade diferente — e o modelo os distingue apenas por atestação de
esforço e pistas de exaustão, que é também tudo que um observador dentro do mundo tem. A simetria é
o teste de que o modelo está certo.

**2 — O vencedor que desiste por tédio.** Lidera com folga e abandona no meio. Leitura ingênua:
resistência baixa. Aqui: o abandono é evento de **disposição** (Agent Cognition), a parcial percorrida
é `LOWER_BOUND`, e a dimensão aeróbica recebe `NO_INFORMATION`. Dois mecanismos, dois donos, nunca
fundidos.

**3 — Semana de provas.** Três noites de quatro horas. `sleep_debt` sobe, `central_fatigue` sobe,
`reaction_time` e expressão de técnica caem, `max_strength` quase não muda. Vem um exame físico. Se
`BodyState` fosse por cena, isso desaparecia e descanso deixaria de ser recurso administrável.

**4 — Lesão ocultada.** Torce o punho e esconde. Verdade: lesão + impedimento + risco de agravamento.
Autopercepção: subestima. Terceiros: nada, até uma pista vazar — e emitir pista depende de dor e de
esforço de ocultação, que custa fadiga central. Semanas depois, uma tarefa que exige preensão decide
o exame. Nenhuma cena precisou narrar isso.

**5 — Exame de sobrevivência na ilha.** Dias de déficit calórico, sono ruim, calor, desidratação. Um
aluno comum perde 25% de capacidade disponível, um resiliente perde 10% — e a diferença vem de
`recovery_rate`, `thermoregulation` e `injury_resilience`, não de decisão de roteiro. Também testa a
fronteira ambiente/interocepção: o calor é do engine, a sede é do agente.

**6 — Feat depois da divergência.** Um volume posterior ao ponto de divergência mostra um feito
físico. O corpo é o mesmo corpo: o feat informa o posterior no seeding. A memória não é a mesma
memória: nenhum agente pode recordá-lo, e depois do seeding ele não informa mais nada.

**7 — NPC sem nenhuma evidência.** Precisa correr num exame em grupo. Sorteia do prior de coorte,
uma vez, com substream derivado do id; estável pelo resto do run; reprodutível mesmo se criado
tardiamente. Nenhum atributo escolhido à mão em lugar algum.

**8 — Testemunho contra medida.** "Ele é o mais forte do ano." Se testemunho entrasse no posterior,
hype canônico viraria física. Entra como `TESTIMONY`: crença com holder, que molda expectativas
sociais e pode estar errada. No máximo corrobora fracamente, e só quando quem fala é observador
calibrado.

## 15. Invariantes

1. `CapacityProfile`, `BodyState`, `ExertionIntent`, `PerformanceOutcome`, `ObservedPerformance` e
   `SelfPhysicalModel` são seis estruturas distintas. Nenhum módulo pode fundir duas.
2. Desempenho observado restringe capacidade como **limite**, nunca como estimativa. Teto só existe
   com esforço máximo atestado.
3. Ausência de evidência produz prior de coorte com proveniência e versão registradas — nunca
   atributo escolhido à mão.
4. `BodyState` persiste entre cenas, eventos, exames e dias. Só o avanço do relógio e eventos do
   engine o alteram.
5. Só o Simulation Engine escreve estado físico. Saída de LLM não cria, cura nem narra para fora
   lesão, fadiga ou capacidade.
6. O contexto de um agente recebe sinais interoceptivos qualitativos e enviesados, nunca valores de
   `BodyState`; e nunca o `BodyState` de outro personagem exceto por observação.
7. Ocultação é intenção de primeira classe, com verificação de viabilidade e processo de detecção.
   O desempenho exibido é calculado pelo engine, não afirmado pelo agente.
8. Comparações de capacidade entre personagens são **saída**, nunca entrada. Nenhum registro pode
   afirmar que um personagem supera outro, exceto como restrição `COMPARATIVE` sobre um evento
   observado específico.
9. Dimensões têm unidade física; percentil é view derivada. Dimensão sem unidade declara a coorte.
10. Capacidade, habilidade e disposição são eixos separados, com donos separados.
11. Toda estocasticidade física vem de substreams nomeados por `(world_seed, character_id, event_id,
    purpose)`; versões de prior, estimador e parâmetros entram nos metadados do run.
12. Feat posterior à divergência informa prior de capacidade no seeding e nada mais; nunca memória
    nem crença.
13. Resolução física é subdomínio do Simulation Engine, não contexto par. Nenhum segundo módulo
    decide o que aconteceu fisicamente.

## 16. O que este modelo se recusa a responder

Quem vence entre dois personagens específicos.

Não por prudência, mas porque a resposta não é um dado de entrada. Ela é a composição de: posteriores
de capacidade construídos a partir de feats tipados, uma amostra semeada, habilidade, estado corporal
no momento, ambiente, objetivos e esforço escolhido, e um resolvedor estocástico. Onde a evidência
canônica não separa dois personagens, seeds diferentes podem dar vencedores diferentes — e isso é a
representação correta da nossa incerteza, não uma falha do modelo.

Qualquer registro que declare uma ordem de força entre personagens fora de um `COMPARATIVE` ancorado
em evento observado é violação da invariante 8, e deve ser rejeitado em review.

## 17. O que bloqueia

| Open question | Trava |
|---|---|
| `oq.capability.fitness-test-records` | única fonte plausível de `INSTRUMENTED_ESTIMATE`; sem ela quase tudo é piso |
| `oq.capability.club-and-training-background` | separa habilidade de capacidade; define condicionamento inicial |
| `oq.capability.effort-attestation` | sem sinais textuais de esforço, todo feat cai em `UNKNOWN` e nenhum teto existe |
| `oq.capability.physical-exam-tasks` | define quais dimensões o engine precisa de fato resolver |

Enquanto abertas: priors populacionais e dinâmica corporal podem ser construídos (calibram contra
fisiologia real, não contra cânone); **posteriores por personagem não podem ser commitados**, e
nenhum perfil de capacidade nominal compila para o engine.
