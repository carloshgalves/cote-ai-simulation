# Modelo físico

Domínio do corpo: capacidade, estado, esforço, desempenho, dor, lesão e disputa física.

Este documento é o modelo de domínio. A decisão que o instituiu está no
[ADR 0006](../adr/0006-physical-domain-model.md). Vocabulário de dimensões em
[`data/canon/schema/enums/capacity-dimensions.yaml`](../../data/canon/schema/enums/capacity-dimensions.yaml).

Revisado em 2026-09-09 contra [`docs/research/physical-domain-v1.md`](../research/physical-domain-v1.md),
que fundamenta a parametrização, corrige a ancoragem de duas dimensões e retira a razão aguda:crônica
do modelo de risco de lesão. Lacunas de *sourcing* que permanecem abertas estão em
[`data/models/physical/SOURCING.md`](../../data/models/physical/SOURCING.md); nada aqui as fecha.

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

A ancoragem não é livre: nove dimensões se ligam ao **item real** da bateria 新体力テスト
(12–19 anos), com a unidade real do instrumento. Uma âncora que não corresponde ao instrumento não é
âncora.

| Dimensão | Item da bateria | Unidade |
|---|---|---|
| `aerobic_capacity` | 20mシャトルラン **ou** 持久走 | voltas **ou** s |
| `anaerobic_power` | 立ち幅とび **e** ハンドボール投げ | cm e m |
| `sprint_speed` | 50m走 | s (→ m·s⁻¹) |
| `max_strength` | 握力 | kg de preensão |
| `strength_endurance` | 上体起こし | repetições em 30 s |
| `agility` | 反復横とび | toques em 20 s |
| `flexibility` | 長座体前屈 | cm |
| `body_mass`, `stature` | 体格測定 | kg, cm |
| `coordination`, `reaction_time`, `injury_resilience`, `recovery_rate`, `thermoregulation`, `pain_tolerance` | — sem item na bateria | ver abaixo |

Duas correções em relação ao primeiro rascunho, e elas importam porque a âncora precisa existir no
mundo que estamos simulando: `agility` é **contagem de toques em 20 s**, não tempo em percurso; e
`strength_endurance` é **repetições em 30 s**, não "s a % do máximo".

Como 持久走 e 20mシャトルラン pontuam na mesma escala de 1 a 10, a tabela oficial de pontuação é
também uma **tabela de equiparação** entre as duas modalidades aeróbicas: é assim que dois feats
aeróbicos diferentes se normalizam sem que inventemos uma conversão. Enquanto as distâncias do
持久走 por sexo não estiverem verificadas (SOURCING S3), prefira a coluna do vaivém.

As seis dimensões sem item da bateria declaram isso explicitamente e trazem âncora própria; nenhuma
delas toma emprestada a credibilidade do instrumento. Dimensão sem unidade física (`pain_tolerance`)
é permitida **somente** quando não existe unidade, e então precisa declarar a coorte de referência.
Isto impede que "0–100 arbitrário" volte pela porta dos fundos.

A tabela de pontuação transcrita vive em
[`data/models/physical/fitness-test-score-table.yaml`](../../data/models/physical/fitness-test-score-table.yaml)
— instrumento de medida do mundo real, **não cânone**, e jamais uma regra da escola.

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
5. **O prior é multivariado.** Marginais independentes por dimensão erram de um jeito visível:
   produzem o aluno simultaneamente mais pesado, mais rápido e melhor no vaivém, povoando uma cauda
   conjunta absurda. O prior é **dois fatores latentes mais resíduo por dimensão** — um fator geral
   de aptidão e um fator de porte, que carrega positivamente em `max_strength`, `body_mass` e
   `anaerobic_power` e negativamente em `aerobic_capacity` relativo — implementado como cópula
   gaussiana sobre as marginais oficiais, de modo a preservá-las exatamente.
6. **`evidence_sufficiency` tem definição operacional**, por dimensão:
   `1 − sd(posterior) / sd(prior)`. Zero significa "isto é o prior com outro nome"; valores altos
   significam que a evidência de fato moveu a distribuição. Um eval pode então recusar afirmações
   fortes sobre personagens cuja sufficiency na dimensão em questão esteja abaixo de um limiar. Vale
   também **para o próprio prior**: enquanto as cargas fatoriais forem suposição declarada, o arquivo
   do prior precisa dizê-lo, não só as fichas de personagem.

`population_prior` é reconstrução nossa calibrada contra a bateria japonesa de aptidão física, que é
a coorte certa (12–19 anos, por sexo). Covariáveis de condicionamento admissíveis são apenas as que o
cânone pode informar: ano, sexo, clube esportivo e qual, reputação atlética declarada, porte descrito.
Condicionar em qualquer outra coisa é inventar cânone pela porta dos fundos.

**O que ainda falta, e permanece faltando:** média e desvio-padrão por idade e sexo, e a matriz de
correlação entre itens. As tabelas estão localizadas no e-Stat e o download automatizado retorna 404
— é tarefa humana, registrada em
[`SOURCING.md`](../../data/models/physical/SOURCING.md) (S1, S2). Até lá as cargas fatoriais são
suposição declarada e o prior é explicitamente provisório.

**Seletividade da coorte.** A pesquisa nacional mede a população escolar japonesa; a escola da obra é
seletiva. Aplicar o prior nacional sem ajuste assume que a seleção não correlaciona com aptidão
física; aplicar um "bônus de escola de elite" inventa cânone na escala da coorte inteira, o que é
pior do que inventar o número de um personagem, porque desloca silenciosamente todos os sorteios.
Enquanto `oq.capability.cohort-selectivity` estiver aberta, o prior usa a coorte nacional **sem
deslocamento**, declarado como hipótese nula explícita e não como omissão.

### O estimador: censura por intervalo, não ajuste de curva

O §6 descreve, sem usar os termos, um problema clássico: limites inferiores e superiores são
**censura por intervalo**, e um parâmetro observado só por limites inferiores é **parcialmente
identificado** — os dados restringem um conjunto, não um ponto, e a cauda superior é determinada
pelo prior, não pela evidência.

Isso muda o estatuto de uma afirmação central deste modelo: "cauda superior larga é a descrição
correta da nossa ignorância" não é escolha estilística nossa, é o comportamento esperado de um
estimador correto sob identificação parcial.

Implementação da V1: **amostragem por importância com reamostragem** — sorteia N partículas do prior
com substream nomeado, pondera cada uma pela verossimilhança das restrições, reamostra. Determinística
dado o seed, auditável partícula a partícula, sem dependência externa. MCMC em PPL foi considerado e
descartado: o problema tem dezenas de personagens × ~15 dimensões, e a estocasticidade de um sampler
externo é mais difícil de honrar sob a invariante 11 (substreams nomeados).

| Restrição | Contribuição para o peso da partícula |
|---|---|
| `LOWER_BOUND` | indicadora suavizada, com margem para erro de desnormalização |
| `UPPER_BOUND` | indicadora suavizada no sentido oposto; só existe com atestação maximal |
| `INSTRUMENTED_ESTIMATE` | gaussiana em torno do valor medido, com desvio = erro do instrumento **mais** um termo de ocultação possível |
| `COMPARATIVE` | logística sobre a *diferença de desempenho* no mesmo evento, nunca diretamente sobre capacidade; a transferência exige o `effort_assumption` já obrigatório no schema |
| `TESTIMONY` | peso 1 — não entra; roteada para o belief state do `testifier` |
| `NO_INFORMATION` | peso 1, por construção |

O encolhimento hierárquico prometido para personagens com pouca evidência não é maquinaria extra: é o
comportamento padrão da amostragem por importância **desde que o prior seja o da coorte condicionada**.

## 6. Feats canônicos como restrições

Um feat é uma observação de desempenho. Ele **restringe** a capacidade; não a mede.

| Tipo de restrição | Quando se aplica | O que autoriza concluir |
|---|---|---|
| `LOWER_BOUND` | fez X, esforço desconhecido | capacidade ≥ X, desnormalizado por estado e ambiente. Nada mais. |
| `UPPER_BOUND` | tentou ao máximo e falhou, **com esforço máximo atestado**, ou houve prova de verificação | capacidade < X. Raro e valioso. |
| `INSTRUMENTED_ESTIMATE` | registro medido oficialmente (teste de aptidão da escola) | estimativa bilateral estreita, **ainda condicionada** a não haver ocultação |
| `COMPARATIVE` | A superou B no mesmo evento, mesmas condições | ordem entre *desempenhos realizados*; transfere para capacidade só com hipótese de esforço explícita |
| `TESTIMONY` | alguém afirma que X é forte | crença de um holder. Alimenta belief state, **não** o posterior |
| `NO_INFORMATION` | desempenho com esforço notoriamente submáximo e sem sinal de esforço | nada acima do piso já conhecido |

A assimetria é o ponto: **pisos são baratos, tetos são caros.** Um personagem que nunca foi visto no
limite tem cauda superior larga — e essa largura é a representação honesta da nossa ignorância, não
uma afirmação de que ele é secretamente o mais forte.

`effort.attestation` registra *como* sabemos o nível de esforço: narração de exaustão, declaração do
próprio personagem, leitura de terceiro, presença ou ausência de sinais de esforço, incentivo
estrutural para ocultar, prova de verificação, ou `UNKNOWN` — o padrão. O schema recusa `UPPER_BOUND`
sem atestação maximal, do mesmo modo que recusa `not_before` sem `not_before_support`.

### Por que sinal de esforço não licencia teto

A regra tinha justificativa interna (a analogia com o ADR 0005); ela tem também justificativa
empírica, e é forte:

- os **critérios secundários** de esforço máximo em teste de VO₂max — razão de trocas respiratórias,
  percentual da frequência cardíaca máxima — são satisfeitos em intensidades tão baixas quanto **61%
  do VO₂max**, e o platô de VO₂ é inconsistente;
- a detecção clínica de esforço insincero, com dinamômetro e tentativas repetidas, erra entre **47% e
  69%** das vezes; o coeficiente de variação da preensão, método padrão da área, **não é válido** —
  o aumento sob esforço submáximo é artefato da redução do torque.

> Sinais de esforço são os "critérios secundários" da ficção. Ofegar, cambalear, suar, "dar tudo" —
> o análogo real disso é satisfeito a 61% do máximo.

Por isso `STRAIN_CUES_PRESENT` é um valor separado que **não** qualifica para teto, e existe
`VERIFICATION_BOUT`: um segundo desempenho independente, em condições nas quais reter esforço não era
viável — tipicamente logo após um `forced_exposure` (§8). É a tradução direta da fase de verificação
de VO₂max, e é a única forma **estrutural**, não retórica, de atestar máximo sem depender de narração
ou autorrelato.

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

  w_prime_balance:               # reserva de trabalho acima da potência crítica, dentro do evento
    remaining_j: ...             # esgota acima de CP; reconstitui exponencialmente abaixo
    tau_s: ...                   # ~380-580 s, MAIOR quanto mais alta a intensidade de recuperação

  peripheral_fatigue:            # por região: legs, arms, grip, core
  central_fatigue:               # SNC: alerta, controle executivo, expressão de técnica

  cumulative_load:
    acute_7d: ...                # contabilidade de carga para deriva de capacidade.
    chronic_28d: ...             # A RAZÃO entre os dois NÃO é fator de risco de lesão (ver abaixo).

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
    deficit_pct_body_mass: ...    # limiar ~2%; acima disso degrada, e acelera o gasto de substrato

  thermal:
    wbgt: ...                     # índice ambiental de estresse térmico
    work_rest_ratio: ...
    core_offset_c: ...
    clothing_insulation: ...

  soreness:                       # DOMS: entra 12-24 h DEPOIS do esforço, pico 24-72 h, ~7 d
    by_region: {}
    repeated_bout_adaptation: {}  # a mesma carga machuca menos na segunda vez
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

- **Sono** não é booleano; é o modelo de dois processos, homeostático × circadiano, que os campos
  `sleep.debt_hours`, `hours_since_wake` e `circadian_phase` já refletem. A ordenação do efeito por
  dimensão não é palpite nosso — vem de meta-análise de privação de sono, em diferenças médias
  padronizadas: **controle de habilidade −0,87 > resistência aeróbica −0,66 > potência explosiva
  −0,63 > velocidade −0,52 > força máxima −0,35**. Use-se a razão entre elas, não o valor absoluto.
  Isso confirma numericamente a intuição do modelo: `max_strength` quase não sente, técnica e precisão
  sentem muito. Especificidade da coorte, e ela pesa: adolescentes têm atraso de fase circadiana e
  precisam de 8–10 h, então horário escolar cedo produz privação crônica sistemática. Num internato,
  isso é a linha de base do corpo de todo mundo, não cor de cena.
- **Substrato** é o canal que faz um exame de dias ser um exame de dias. A depleção de glicogênio é
  fator principal de fadiga em esforço prolongado, a taxa de degradação cresce **exponencialmente**
  com a intensidade, e com dieta pobre em carboidrato o glicogênio **permanece baixo por vários
  dias**. Racionamento não é modificador do dia; é estado que persiste.
- **Hidratação** tem limiar: abaixo de ~2% de perda de massa corporal o efeito é desprezível, acima
  degrada progressivamente (68% das observações de resistência significativamente prejudicadas, 88%
  na direção do prejuízo). Amplificada por carga térmica, e **acoplada ao substrato**: desidratar
  acelera o gasto de glicogênio.
- **Ambiente** é world truth do engine: WBGT, terreno, piso, inclinação, carga transportada,
  visibilidade, calçado. **Calor é ambiente; sede é interocepção** (§9) — o agente sente sede, não lê
  `hydration.deficit_pct_body_mass`.
- **Sem penalidade térmica por idade.** A ideia de que jovens regulam calor pior não se sustenta: com
  hidratação adequada não há diferença demonstrada de acúmulo de calor, temperatura central ou
  tolerância. O risco vem de fatores modificáveis — intensidade, duração, hidratação, razão
  trabalho:descanso — indexados a WBGT. A variação individual fica em `thermoregulation`, não numa
  penalidade de coorte.
- **Recuperação é mais rápida nesta coorte.** Adolescentes recuperam mais rápido que adultos de
  esforço intenso. A evidência mais forte é pré-púbere, então para 15–18 anos o deslocamento de
  `recovery_rate` é **interpolação**, e o arquivo de parâmetros precisa registrar isso em vez de
  apresentá-lo como medição.
- **DOMS entra atrasado.** Início 12–24 h após o esforço, pico 24–72 h, resolução em ~7 dias, com
  perda mensurável de potência na janela. E existe **efeito de sessão repetida**: a mesma carga
  produz menos dano na segunda vez — sem esse flag por região e atividade, quem treina toda semana
  sofreria como se fosse a primeira vez, para sempre.
- **Deriva de capacidade tem ordenação conhecida.** No destreinamento, o aeróbico cai cedo (~10º dia)
  e bastante; a força se preserva muito melhor. Os números vêm de atletas que cessam treino, então
  para um aluno comum servem para fixar **ordem de grandeza e ordenação** — aeróbico rápido, força
  lenta — nunca como valores literais.

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
        × estado(fadiga, dívida de sono, hidratação, soreness)
        × ambiente(piso, calçado, visibilidade, contato)
        ÷ injury_resilience
        × histórico(lesão prévia na mesma região)
```

**A razão aguda:crônica saiu do modelo.** O primeiro rascunho a listava como fator de estado. A
métrica é matematicamente acoplada (numerador contido no denominador), instável quando a carga
crônica é baixa, sem evidência de efeito causal, e a figura do "sweet spot" que a popularizou é
objeto de pedido formal de retratação. Mantê-la seria construir sobre um número que a própria área
está retirando.

O que a substitui já estava na fórmula: **carga recente absoluta relativa à `capability_available`
do próprio corpo** — que é o driver principal — mais histórico de lesão na mesma região. O
enquadramento é **dinâmico-recursivo**: cada exposição repetida altera o risco seguinte, por
adaptação ou maladaptação, e a lesão não é causada só pelo evento incitante. `acute_7d` e
`chronic_28d` permanecem no `BodyState` como contabilidade de carga para deriva de capacidade; o que
saiu foi a razão entre eles como fator de risco.

`pain_override` e `risk_acceptance` altos elevam a exposição porque removem o freio comportamental —
é assim que "ignorar a dor" tem preço em vez de ser adjetivo narrativo.

### Canais, escalas de tempo e o que cada um degrada

Um reservatório único de "stamina" não representa nenhum dos cenários do §14. As escalas de tempo
separadas são o requisito mínimo.

| Canal | Escala de tempo | Degrada, em ordem |
|---|---|---|
| `w_prime_balance` | s a min (τ ≈ 380–580 s) | `anaerobic_power`, `sprint_speed` |
| `peripheral_fatigue` | h | `max_strength`, `strength_endurance` na região |
| `central_fatigue` | h a dias | `coordination`, `reaction_time`, expressão de técnica |
| `sleep.debt` + fase circadiana | dias | habilidade ≫ aeróbico ≳ potência > velocidade ≫ força |
| `energy.substrate` | h a **dias** | `aerobic_capacity`, `strength_endurance` |
| `hydration.deficit` | h | `aerobic_capacity` a partir de ~2%, amplificado por calor |
| `thermal` | min a h | todas, via WBGT e razão trabalho:descanso |
| `soreness` | atraso 12–24 h, pico 24–72 h, ~7 d | `anaerobic_power`, `max_strength` |
| `injuries` | dias a meses | multiplicadores por dimensão |
| deriva de `capacity_baseline` | semanas | aeróbico rápido, força lenta |

Nota de honestidade sobre a fadiga entre dias: a forma de dois exponenciais com constantes distintas
(aptidão lenta, fadiga rápida) é adotada como **contabilidade de estado consistente**, não como
preditor validado de desempenho. As revisões do modelo impulso-resposta são duras — estimativas
imprecisas, mau condicionamento, parâmetros de interpretação difícil. O arquivo de parâmetros precisa
dizer isso, para que ninguém o cite adiante como física estabelecida.

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

E o custo é material, não figurado: acompanhar uma aceleração **gasta `w_prime_balance`**, e o que se
gasta acima da potência crítica não volta dentro da mesma prova. Ocultar em uma prova disputada é
literalmente caro.

`pacing` é a escolha de um **template antecipatório**: o ritmo é regulado comparando o esforço
percebido no momento com o esperado naquele ponto da prova, construído a partir do conhecimento do
ponto final e de experiências anteriores. Isso dá base principiada à falha de viabilidade do
`display_ceiling` — ela é o template quebrando, porque o agente montou o plano sobre a capacidade que
*acredita* ter e o pelotão real não cooperou.

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

**Calibração da detecção.** Se profissionais com dinamômetro e tentativas repetidas erram entre 47% e
69% das vezes (§6), a discriminação de **uma observação isolada** dentro do mundo deve ficar perto do
acaso. O sinal precisa vir do acúmulo, não da acuidade de um olhar. Daí três camadas, nenhuma delas
booleana:

| Camada | Onde vive | Forma | Regra dura |
|---|---|---|---|
| `effort.attestation` | corpus de feats (cânone) | enum sobre *o que a prosa estabelece* | sinal de esforço nunca licencia teto; `UNKNOWN` é o padrão |
| `effort_evidence` | observação em simulação | conjunto de pistas com **razões de verossimilhança**, acumuladas em log-odds na crença do observador | nenhuma observação isolada pode saturar a crença |
| `perceived_effort` | crença do próprio agente | ordinal tipo RPE, enviesado por `body_awareness` e analgesia | nunca é o `target_intensity` que o engine usou |

Isso tem uma consequência de design agradável e não planejada: a suspeita vira processo lento e
social, e um combate (§11) passa a ser o evento excepcional — por gerar muitas observações
correlacionadas de uma vez.

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

O erro de `body_awareness` deve ser **genuinamente grande**, inclusive no melhor caso. A acurácia
interoceptiva é individualmente muito variável e mal medida: até os testes-padrão de percepção
cardíaca são contestados, com uma fração dos indivíduos respondendo por fase cardíaca em vez de por
detecção. Modelar autopercepção como quase verídica seria dar telemetria por outro nome. O sinal
correto é o do tipo RPE: informativo, ordinal, enviesado — e, colhido durante ou logo após o esforço,
tende a **superestimar** a fadiga.

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

As duas instâncias diferem em uma coisa, e é a escala temporal: o cliente A roda uma vez, no seeding,
sobre um corpus fechado, e pode pagar amostragem por importância com muitas partículas (§5); o
cliente B roda continuamente e precisa de atualização incremental — log-odds sobre pistas (§8) e
atualização amortecida quando a incerteza sobre o alvo é alta (§11). Mesma semântica de restrição,
custos computacionais diferentes.

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

### Granularidade: máquina de trocas

Nem simulador biomecânico golpe a golpe (exige dados que não temos, produz precisão falsa, e a saída
que importa — quem controlou quem, quanto custou, o que os observadores viram — não depende dessa
resolução), nem rolagem oposta única sobre um escalar de "combate" (reintroduz o antipadrão do §1 e
entrega uma única observação binária a quem assistiu).

**~3 a 10 trocas.** Cada troca resolve por uma função logística sobre uma margem composta, consome
reserva, pode emitir sorteio de lesão e emite observação. A função de ligação não é invenção nossa: é
Bradley-Terry/Elo, o formalismo padrão para mapear a *diferença* entre forças latentes em
probabilidade de vitória.

**A escolha decisiva é aplicar a logística por troca, não por luta.** Dela decorrem, sem caso
especial, duas propriedades que o resolvedor precisa ter:

- disputas longas favorecem condicionamento, porque cada troca consome reserva e a degradação
  composta ao longo de muitas trocas domina;
- disputas curtas favorecem técnica e iniciativa, porque poucas trocas não dão tempo de a reserva
  importar.

```
margem = w_hab(Δcapacidade) · Δhabilidade
       + w_cap · Δcapacidade_relevante_à_modalidade
       + w_massa · Δbody_mass
       + termo_posicional (controle, distância, iniciativa)
       − penalidade(fadiga central, dor)      ← degrada habilidade mais que força
       + ruído(substream nomeado)
```

`w_hab` decrescente em `Δcapacidade` implementa "habilidade domina em diferenças pequenas de
capacidade, capacidade domina em diferenças grandes" **como função**, não como regra à parte. E
`body_mass` é termo explícito porque categorias de peso existem por um motivo: massa confere
vantagem, força absoluta é maior nos mais pesados e a relativa nos mais leves.

Evidência que sustenta os termos: em agarre, a fase de pegada consome cerca de metade do tempo de
combate e a resistência de preensão é determinante para projeções e imobilizações — logo
`strength_endurance` e preensão são dimensões de primeira classe em modalidades de agarre, não
detalhe. E condicionamento responde por **até 45%** da variância entre lutadores bem e mal sucedidos:
grande, mas longe de tudo, o que justifica habilidade e estado carregarem o resto.

**Estado mínimo entre trocas:** controle/posição, distância, iniciativa, reserva de esforço por
participante (o `w_prime_balance` do §7), dor, dano acumulado e satisfação de objetivo. Terminação por
objetivo atingido, reserva esgotada, lesão incapacitante ou intervenção de terceiro.

**Não modelar:** localização de golpe, alavancas articulares, física de impacto, ordem de iniciativa
por décimos de segundo.

### Propriedades que o resolvedor precisa ter

- **Objetivos assimétricos mudam a função objetivo**: escapar não é vencer, conter não é ferir. Um
  participante com `hold_back` ou `lose_deliberately` usa o mesmo `display_ceiling` do §8.
- **Consequências institucionais não vivem aqui.** Deduções de conduta, advertência, expulsão são do
  contexto de regras da escola. O combate emite eventos; a escola os julga.
- **O corpo não se resolve no fim da cena.** Toda saída de combate escreve em `BodyState` como
  qualquer outro esforço. A lesão vai para o próximo exame.
- **Lutar revela — e agora isso é derivado, não postulado.** Como cada troca emite observação, uma
  luta produz muitas observações correlacionadas do mesmo alvo em pouco tempo. No acumulador de
  log-odds do §8, é exatamente sob essa condição que a suspeita se torna decisiva. Para um ocultador,
  esse é o custo dominante de lutar.

Uma nota sobre a crença de quem assiste: a atualização deve ser **menor quando a incerteza sobre o
adversário é alta**, porque pouca informação foi ganha — é o comportamento do desvio de avaliação no
Glicko, e é literalmente o que se quer do `CapacityBelief` de um observador.

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
- Sorteios estocásticos — partículas do estimador, amostra de capacidade, ruído de desempenho, lesão,
  detecção de ocultação, erro de observação, ruído por troca de combate — vêm de **substreams
  nomeados** derivados de `(world_seed, character_id, event_id, purpose)`. Consequência prática:
  acrescentar um personagem irrelevante não desloca os sorteios de outro, e dois runs podem ser
  diferenciados.
- A escolha de amostragem por importância em vez de MCMC (§5) é, em boa parte, esta invariante: um
  sampler externo é uma fonte de estocasticidade fora do esquema de substreams nomeados.
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
o exame. Nenhuma cena precisou narrar isso. E não é licença dramática: entre atletas universitários
com histórico de concussão, **43%** relataram ter escondido sintomas deliberadamente para continuar
jogando. Ocultar lesão é a linha de base do comportamento real, não a exceção.

**5 — Exame de sobrevivência na ilha.** Dias de déficit calórico, sono ruim, calor, desidratação. A
diferença entre um aluno comum e um resiliente vem de `recovery_rate`, `thermoregulation` e
`injury_resilience`, não de decisão de roteiro. O canal que faz o exame ser de *dias* é o substrato:
com pouco carboidrato, o glicogênio fica baixo por vários dias, então o terceiro dia começa pior que
o primeiro. Desidratação acima de 2% amplifica isso, porque acelera o próprio gasto de glicogênio.
Também testa a fronteira ambiente/interocepção: o calor é do engine, a sede é do agente — e nenhum
personagem sofre penalidade térmica por ser adolescente, porque essa penalidade não existe.

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

### Cânone — exigem leitura Tier 0–1

| Open question | Trava |
|---|---|
| `oq.capability.effort-attestation` | **blocker.** Sem atestação de esforço, todo feat cai em `UNKNOWN` e nenhum teto existe |
| `oq.capability.fitness-test-records` | única fonte plausível de `INSTRUMENTED_ESTIMATE`; sem ela quase tudo é piso |
| `oq.capability.club-and-training-background` | separa habilidade de capacidade; define condicionamento inicial |
| `oq.capability.physical-exam-tasks` | define quais dimensões o engine precisa de fato resolver |
| `oq.capability.cohort-selectivity` | não trava a construção do prior; fixa o que ele deve assumir enquanto aberta — coorte nacional sem deslocamento |

As quatro primeiras fecham em **uma** passagem de leitura, não em quatro, se cada desempenho
encontrado for anotado com: quem, quando, **quem mais estava no mesmo evento**, forma da medida,
condições e estado corporal declarado antes, atestação de esforço, e quem observou por qual canal. O
protocolo está no §8.3 da pesquisa.

### Sourcing — não dependem de cânone

Quatro lacunas de instrumento, registradas em
[`SOURCING.md`](../../data/models/physical/SOURCING.md): média e DP por idade e sexo (S1, download
automatizado bloqueado — exige humano), matriz de correlação entre itens (S2, não localizada),
distâncias do 持久走 por sexo (S3, inferidas e não verificadas) e verificação da tabela de pontuação
contra a publicação oficial (S4). Nenhuma delas foi fechada; o prior permanece provisório e precisa
declarar isso.

Enquanto tudo isso estiver aberto: priors populacionais e dinâmica corporal podem ser construídos
(calibram contra fisiologia real, não contra cânone); **posteriores por personagem não podem ser
commitados**, e nenhum perfil de capacidade nominal compila para o engine.
