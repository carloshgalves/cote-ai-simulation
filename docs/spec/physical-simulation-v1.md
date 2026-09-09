# Spec — Physical Simulation V1

**Status:** Proposed
**Data:** 2026-09-09
**Decisão de origem:** [ADR 0006](../adr/0006-physical-domain-model.md) — domínio físico
**Modelo de domínio:** [`docs/architecture/physical-model.md`](../architecture/physical-model.md)
**Fundamentação:** [`docs/research/physical-domain-v1.md`](../research/physical-domain-v1.md)
**Relacionados:** ADR 0001 (world truth × crença) · ADR 0003 (tempo lógico) · ADR 0005 (base canônica)

Esta spec converte o ADR 0006 e o modelo físico em uma implementação. Ela **não** reabre nenhuma das
15 decisões do ADR; onde precisa escolher algo que o ADR deixou em aberto, a escolha aparece na
§13 (*Decisões abertas*) e **não** dentro dos critérios de aceitação.

---

## 1. Problema e objetivo visível

Hoje o repositório sabe descrever o corpo e não sabe resolvê-lo. `docs/architecture/physical-model.md`
define seis estruturas, dez canais de estado e uma ordem de resolução; nada disso executa. Sem
execução, a primeira tentativa de rodar um exame físico vai improvisar um atributo escalar dentro do
código do exame — que é exatamente o `strength: 95` que o ADR 0006 existe para impedir, só que
escondido em outro módulo.

**Objetivo visível:** dado um mundo semeado com um seed, um conjunto de personagens (focais e NPCs
anônimos) e uma atividade física, o sistema resolve *o que aconteceu com cada corpo*, *o que cada
observador viu* e *como o corpo ficou depois*, de forma reprodutível e sem nenhuma chamada de LLM.

Concretamente, ao fim da V1 é possível, por linha de comando e sem modelo de linguagem:

1. semear uma coorte de N alunos a partir do prior populacional, com um `world_seed`;
2. rodar uma corrida de fundo em que um participante tenta terminar no terço médio do pelotão;
3. obter do engine se a ocultação foi viável (`ok | forced_exposure | forced_loss`), o que cada
   observador percebeu, e quanta suspeita acumulou no belief state de cada um;
4. avançar o relógio três dias com sono ruim e ver `central_fatigue`, `sleep.debt` e DOMS evoluírem
   com as escalas de tempo certas — e ver `max_strength` quase não se mover enquanto `coordination`
   cai;
5. rodar um confronto de contenção entre dois personagens e ver a lesão resultante persistir para o
   evento seguinte;
6. reexecutar tudo com o mesmo seed e obter byte-a-byte o mesmo event log.

O item 6 é o objetivo real. Os outros cinco são a superfície pela qual ele é observável.

---

## 2. Termos e invariantes afetados

### Termos que esta spec implementa

`CapacityProfile` · `capacity_baseline(t)` · `capability_available(t)` · `BodyState` ·
`ExertionIntent` · `PerformanceOutcome` · `ObservedPerformance` · `SelfPhysicalModel` ·
`CapacityBelief` · `Injury` · `ContestSpec` · `population_prior` · `capacity_posterior` ·
`evidence_sufficiency` · substream nomeado.

Definições normativas: `physical-model.md` §2, §4, §5, §7, §8, §9, §11. Esta spec não redefine
nenhuma delas; onde houver divergência entre este documento e o modelo, **o modelo prevalece** e a
divergência é bug desta spec.

### Invariantes do `CONTEXT.md` sob teste

| # | Invariante | Como esta spec a exercita |
|---|---|---|
| 1 | world truth ≠ belief | `BodyState`/`PerformanceOutcome` (engine) vs `SelfPhysicalModel`/`CapacityBelief` (memória) |
| 3 | inteligência não concede fatos ocultos | detector de ocultação varia `inference_quality`, nunca acesso |
| 5 | ação inválida é rejeitada, não reinterpretada | passo 2 do resolvedor de esforço |
| 9 | seis estruturas distintas | tipos separados, sem conversão implícita entre 1 e 4 |
| 10 | desempenho restringe como limite | estimador aceita `LOWER_BOUND`/`UPPER_BOUND` como censura |
| 11 | ausência de evidência → prior versionado | seeding de NPC sem feats |
| 12 | consequência física persiste; só o engine escreve | API de escrita única; cena não cura |
| 13 | interocepção qualitativa, nunca telemetria | context payload sem números de `BodyState` |
| 14 | comparações são saída | nenhum arquivo de entrada ordena personagens |

### Invariantes do modelo físico (§15) sob teste

As 13 invariantes numeradas do `physical-model.md` §15 são requisitos desta spec. Cada uma tem pelo
menos um teste nomeado na §11.4.

---

## 3. Escopo

### 3.1 In-scope

| Componente | Entrega |
|---|---|
| `POPULATION_PRIOR` | prior multivariado: marginais por dimensão × idade × sexo, dois fatores latentes, cópula gaussiana; arquivo versionado, `not_canon: true`, com `evidence_sufficiency` do próprio prior |
| `CAPACITY_ESTIMATOR` | amostragem por importância com reamostragem (SIR) sobre substream nomeado; converte `constraints[]` tipadas em `capacity_posterior`; reporta ESS e `evidence_sufficiency` por dimensão |
| `WORLD_SEEDING` | amostra `capacity_baseline(Y1_START)` do posterior, uma vez, congelada no snapshot; substream derivado de `hash(character_id, world_seed)` para NPC criado tardiamente |
| `BODY_STATE` | o estado do `physical-model.md` §7 tipado por inteiro, incluindo `cumulative_load` (contabilidade) e `pain`; propriedade exclusiva do engine |
| `BODY_DYNAMICS` | funções puras `(state, Δt, env) -> state` para os **nove canais dinâmicos** da tabela §7 — a tabela inteira menos a deriva de `capacity_baseline`, que é out-of-scope (§3.2) — com as escalas de tempo de lá; recuperação, sono, substrato, hidratação, térmico, DOMS atrasado, cura de lesão |
| `CAPABILITY_AVAILABLE` | composição `capacity_baseline ⊗ impairments ⊗ estado ⊗ ambiente` por dimensão |
| `EFFORT_RESOLVER` | os dez passos do §8, incluindo validação de intent, viabilidade de `display_ceiling`, custos fisiológicos e sorteio de lesão |
| `CONTEST_RESOLVER` | máquina de 3 a 10 trocas do §11: logística por troca, margem com `body_mass`, `w_hab` decrescente, objetivos assimétricos, terminação, escrita em `BodyState` |
| `OBSERVATION` | emissão de `ObservedPerformance` por observador, com canal, erro e vazamento de pistas |
| `DETECTION` | acumulador de log-odds de ocultação no belief do observador; calibrado perto do acaso para observação isolada |
| `INTEROCEPTION` | tradução determinística de `BodyState` em sinais qualitativos enviesados, e `SelfPhysicalModel` |
| `RNG` | substreams nomeados por `(world_seed, character_id, event_id, purpose)` |
| `SNAPSHOT` | serialização/desserialização de `capacity_baseline` + `BodyState` + belief físico, com hash |
| `RUN_METADATA` | seed, versões de prior/estimador/dinâmica/resolvedor, hash de posterior por personagem |

### 3.2 Out-of-scope

- **Deriva de `capacity_baseline` ao longo do ano** (adaptação a treino e destreinamento,
  `physical-model.md` §4). A V1 trata `capacity_baseline` como constante dentro do run e mantém
  `cumulative_load.acute_7d`/`chronic_28d` sendo **contabilizados** sem ainda alimentar deriva. O
  campo existe, é atualizado, e nada o lê. Motivo: a ordenação (aeróbico rápido, força lenta) tem
  fonte, mas a magnitude para um aluno comum não — os números vêm de atletas que cessam treino.
- **Ingestão de feats canônicos reais.** `data/canon/feats/` permanece vazio (bloqueado por
  `oq.capability.*`). O estimador é exercitado por fixtures sintéticos, §11.3.
- **Posteriores por personagem commitados.** Proibido enquanto S1/S2 e `oq.capability.*` estiverem
  abertos. Ver §12.
- **Integração com `ExamSpec`.** O Examination Engine consome resolução física; a V1 entrega a API
  que ele vai chamar e não o chamador.
- **Agent Cognition escolhendo `ExertionIntent` por LLM.** A V1 usa políticas roteirizadas (§4).
- **UI, visualização, animação.** Nada.
- **Doenças (`illnesses`).** O campo existe no `BodyState` e é serializado; nenhuma dinâmica o
  atualiza na V1. Lesão sim, doença não.
- **Reputação de força (Social State).** Fora do subdomínio.

---

## 4. Determinístico × dependente de modelo

A fronteira aqui é atipicamente limpa, e isso é o ponto: **nada nesta spec precisa de LLM para
decidir um resultado.**

| Camada | Natureza | Onde |
|---|---|---|
| dinâmica corporal, recuperação, cura, modificadores ambientais | **função pura** de estado e tempo | `dynamics.py` |
| composição de `capability_available` | **função pura** | `capability.py` |
| validação de `ExertionIntent`, viabilidade de `display_ceiling` | **determinístico** | `effort.py` |
| amostra de capacidade, ruído de desempenho, lesão, detecção, erro de observação, ruído por troca | **estocástico, substream nomeado** — determinístico dado o seed | `rng.py` + consumidores |
| partículas do estimador | **estocástico, substream nomeado** | `estimator.py` |
| escolha de `ExertionIntent` (intensidade, pacing, teto de exibição, apetite a risco) | **dependente de modelo** — fora da V1 | Agent Cognition |
| redação da narração da cena | **dependente de modelo** — não escreve estado | fora da V1 |

Para a V1, `ExertionIntent` vem de **políticas roteirizadas** (`policies/scripted.py`): `all_out`,
`conserve(fraction)`, `hold_rank(band)`, `hide_injury`. São fixtures de teste, não personagens.
Quando o Agent Cognition entrar, ele substitui a política e **não** ganha nenhum campo novo: se um
`ExertionIntent` produzido por LLM precisar de um campo que a política roteirizada não tem, isso é
sinal de que autoridade narrativa vazou para a intenção.

**Regra dura, testada:** o pacote `src/embodiment/` não importa nenhum cliente de LLM. Um teste de
arquitetura falha se importar.

---

## 5. Contratos, dados e mudanças de estado

### 5.1 Novos arquivos de parâmetro — `data/models/physical/`

Todos `not_canon: true`, versionados, com `provenance` e `evidence_sufficiency` próprios. Nenhum
deles pode aparecer em `provenance.supports` de um claim canônico (ADR 0005 §7).

| Arquivo | Conteúdo | Estado de fonte |
|---|---|---|
| `population-prior.yaml` | marginais por dimensão/idade/sexo, cargas dos dois fatores latentes, matriz de correlação residual | **provisório**: médias e DP dependem de S1, correlações de S2. Deve declarar `status: PROVISIONAL` e a hipótese nula de coorte (§12) |
| `body-dynamics.yaml` | constantes de tempo por canal, ordenação de degradação, limiares | τ de `W'` 380–580 s; limiar de hidratação ~2% da massa; DOMS início 12–24 h, pico 24–72 h, resolução ~7 d; razões de sono do §7 |
| `contest-params.yaml` | escala da logística por troca, forma de `w_hab(Δcapacidade)`, `w_cap`, `w_massa`, penalidades de fadiga e dor, faixa de trocas (3–10) | **[INT]** — parametrização declarada, calibrada por propriedade e não por dado |
| `estimator-params.yaml` | nº de partículas, largura das indicadoras suavizadas, erro de instrumento, termo de ocultação, limiar de ESS | **[INT]** |
| `interoception-params.yaml` | viés por `body_awareness`, ganho de analgesia, mapa canal → vocabulário qualitativo | **[INT]** |
| `observation-params.yaml` | erro por canal de observação, razões de verossimilhança por pista, teto de log-odds por observação | calibrado pela §8 do modelo: uma observação isolada fica perto do acaso |

**Cada arquivo declara em seu próprio cabeçalho o que é medido e o que é suposição.** Um parâmetro
`[INT]` que não se declare como tal é bug. O `body-dynamics.yaml` carrega literalmente a nota de
honestidade do ADR 0006: a forma de dois exponenciais para fadiga entre dias é **contabilidade de
estado consistente, não preditor validado de desempenho**.

### 5.2 Módulos — `src/embodiment/`

```
src/embodiment/
  types.py        CapacityProfile, BodyState, ExertionIntent, PerformanceOutcome,
                  ObservedPerformance, SelfPhysicalModel, CapacityBelief, Injury
  rng.py          substream(world_seed, character_id, event_id, purpose) -> Generator
  prior.py        carrega population-prior.yaml; amostra via cópula gaussiana
  constraints.py  tipos de restrição do §6 e sua verossimilhança
  estimator.py    SIR: prior ⊗ constraints -> posterior; ESS; evidence_sufficiency
  seeding.py      posterior -> capacity_baseline congelada
  dynamics.py     funções puras por canal: advance(state, dt, env) -> state
  capability.py   capability_available(baseline, state, env) -> por dimensão
  effort.py       resolve(intent, capability, env, field) -> PerformanceOutcome
  contest.py      máquina de trocas
  observation.py  emissão de ObservedPerformance + pistas
  detection.py    acumulador de log-odds de ocultação
  interoception.py BodyState -> sinais qualitativos (SelfPhysicalModel)
  snapshot.py     serialização + hash
  policies/scripted.py   políticas de intenção para teste (NÃO é Agent Cognition)
tests/embodiment/
```

Ferramentas: `pydantic` para tipos de estado, `numpy` para cópula e SIR, `pytest` + `hypothesis`
para os testes. `numpy.random.Generator` com `SeedSequence.spawn` derivado do nome do substream —
nunca `numpy.random` global, nunca `random` da stdlib.

**A escolha de Python vale para este subdomínio e não decide a stack do projeto.** Ver §13.4.

### 5.3 Estado persistido — snapshot

O snapshot ganha duas seções novas por personagem e uma global.

```yaml
snapshot_version: 1
world_seed: <int>
sim_time: <sim clock>

characters:
  <character_id>:
    capacity_baseline:            # amostra congelada, world truth
      schema_version: 1
      sampled_from: {posterior_hash: <sha256>, prior_version: <semver>}
      dimensions: {aerobic_capacity: {value: ..., unit: laps}, ...}
    body_state:                   # physical-model.md §7, integral
      t: <sim time>
      w_prime_balance: {...}
      peripheral_fatigue: {...}
      central_fatigue: ...
      cumulative_load: {acute_7d: ..., chronic_28d: ...}
      sleep: {...}
      energy: {...}
      hydration: {...}
      thermal: {...}
      soreness: {by_region: {}, repeated_bout_adaptation: {}}
      injuries: [Injury]
      illnesses: []               # serializado, sem dinâmica na V1
      pain: {...}

physical_beliefs:                 # Agent Knowledge & Memory, NÃO world truth
  <holder_id>:
    self_physical_model: {...}
    capacity_beliefs:
      <target_id>: {<dimension>: {lower: ..., upper: ..., log_odds_concealment: ...}}

run_metadata:
  prior_version: <semver>
  estimator_version: <semver>
  dynamics_version: <semver>
  contest_resolver_version: <semver>
  observation_params_version: <semver>
  posterior_hash_by_character: {<character_id>: <sha256>}
  estimator_ess_by_character: {<character_id>: <float>}
```

`capacity_baseline` e `body_state` ficam sob `characters`; crenças físicas ficam em uma árvore
separada, com dono explícito. **A separação de árvore é estrutural, não estilística:** um serializador
que consiga escrever crença dentro de `characters.<id>` já perdeu a invariante 1.

### 5.4 Event log

Todo resultado físico é apendado. Registros novos:

| Evento | Campos essenciais |
|---|---|
| `capacity.sampled` | character_id, posterior_hash, prior_version, substream, evidence_sufficiency por dimensão |
| `effort.rejected` | intent, motivo (impossível, pressupõe estado inexistente) |
| `effort.resolved` | intent, capability_available usado, alvo, realizado, veredito de ocultação, custos aplicados |
| `injury.rolled` | p_lesao e seus fatores, resultado, substream |
| `injury.opened` / `injury.healed` | Injury completo |
| `contest.exchange` | índice da troca, margem e seus termos, resultado, custos, observações emitidas |
| `contest.ended` | motivo de terminação, satisfação de objetivo por participante |
| `observation.emitted` | observer_id, target_id, canal, valor percebido, pistas vazadas |
| `belief.concealment_updated` | observer_id, target_id, Δlog-odds, total acumulado |
| `body.advanced` | Δt, canais alterados, valores antes/depois |

O log é o artefato de auditoria da §10. Ele contém números que **nunca** entram em prompt.

---

## 6. Ordem de resolução (normativa)

Reprodução da §8 do modelo, com os pontos que a implementação precisa acertar.

```
1. capability_available = capacity_baseline(t) ⊗ impairments(injuries) ⊗ estado ⊗ ambiente
2. validação do intent → inválido é REJEITADO (evento effort.rejected), nunca reinterpretado
3. alvo = capability_available × target_intensity, truncado
4. viabilidade de display_ceiling dado o campo → {ok | forced_exposure | forced_loss}
5. realizado = alvo ⊗ ruído(substream) ⊗ decaimento de pacing
6. custos: Δfadiga periférica/central, Δsubstrato, Δhidratação, Δtérmico, Δcumulative_load
7. sorteio de lesão(substream)
8. atualização de dor
9. emissão de observação por observador
10. append no event log
```

Requisitos que a ordem impõe e que os testes verificam:

- **Passo 1 compõe por dimensão, nunca por escalar global.** Um punho lesionado derruba preensão e
  arremesso e deixa corrida quase intacta. Um `impairment` que se aplique a todas as dimensões
  igualmente é bug.
- **Passo 3 usa a capacidade que o agente *acredita* ter** como base de `target_intensity`, e o
  engine trunca contra a real. É daí que sai a falha de template de pacing do §8.
- **Passo 4 tem custo material.** Acompanhar uma aceleração para manter o teto de exibição gasta
  `w_prime_balance`, e o que se gasta acima da potência crítica não volta dentro da mesma prova.
  Um `forced_exposure` que não cobre reserva é bug.
- **Passo 5 é o único lugar onde o desempenho é decidido.** Nenhum outro módulo escreve
  `PerformanceOutcome`.
- **Passo 7 usa a fórmula do modelo §7 sem a razão aguda:crônica.** Um fator de risco que leia
  `acute_7d/chronic_28d` é bug — a métrica foi retirada na revisão de 2026-09-09.
- **Passo 9 emite por observador, com canal.** Não existe "a observação" no singular.

Para o `CONTEST_RESOLVER`, a ordem por troca é: margem → logística → resultado → consumo de reserva
→ sorteio de lesão → observação → teste de terminação. A logística é aplicada **por troca, não por
luta**; aplicar por luta é a violação estrutural que apaga as duas propriedades exigidas (disputas
longas favorecem condicionamento, curtas favorecem técnica).

---

## 7. Conhecimento e visibilidade

Esta é a seção onde o subdomínio físico mais facilmente vaza, e por isso ela tem regras duras e
testes dedicados.

### 7.1 O que nunca sai do engine

1. **Nenhum valor numérico de `BodyState` entra em contexto de agente** — nem o próprio, nem o de
   terceiro. O context builder recebe `SelfPhysicalModel`, que é crença.
2. **Nenhum valor de `capacity_baseline` ou `capacity_posterior` entra em contexto.** O posterior
   numérico não é documento recuperável por RAG; o corpus de feats é, em paráfrase.
3. **O resultado do sorteio de detecção nunca chega ao ocultador.** Ele percebe, no máximo, pistas
   secundárias — e "percebe" aqui é outra observação, sujeita a erro.
4. **`ExertionIntent` de terceiros não é observável.** Um observador vê desempenho e pistas; a
   intenção de ocultar é inferência com log-odds, nunca leitura.
5. **`p_lesao`, ESS, log-odds e substreams são telemetria de engine.** Vivem no event log e nos
   metadados do run, jamais no payload de contexto.

### 7.2 O que chega ao agente

| Canal do `BodyState` | Sinal entregue |
|---|---|
| `peripheral_fatigue` | "pernas pesadas", "braços falhando" |
| `central_fatigue` + sono | "difícil manter foco", "reação atrasada" |
| `hydration` | sede, boca seca, tontura |
| `energy` | fome, fraqueza, tremor |
| `injury.severity` | dor localizada, instabilidade — **enviesado, frequentemente subestimando** |

O viés precisa ser **genuinamente grande, inclusive no melhor caso**. Acurácia interoceptiva é
individualmente muito variável e mal medida; modelar autopercepção como quase verídica é dar
telemetria por outro nome. O sinal correto é do tipo RPE — informativo, ordinal, enviesado — e,
colhido durante ou logo após o esforço, tende a **superestimar** a fadiga.

### 7.3 Classe de vazamento nova para a `knowledge-boundary-audit`

O ADR 0006 já antecipou três; a V1 as torna testáveis e acrescenta duas:

1. telemetria corporal própria em prompt;
2. `BodyState` de terceiro em prompt;
3. resultado do sorteio de detecção visível ao ocultador;
4. **posterior de capacidade recuperado por RAG** (o corpus de feats é paráfrase; o posterior não é
   documento);
5. **feat posterior à divergência aparecendo como memória.** O mesmo registro é legítimo no seeding e
   proibido na memória — é a única estrutura do repositório com essa assimetria, e portanto a que
   mais provavelmente será implementada errada.

---

## 8. Modos de falha

Ordenados por dano ao domínio, não por probabilidade.

| # | Falha | Sintoma | Mitigação nesta spec |
|---|---|---|---|
| F1 | fusão de `CapacityProfile` com `PerformanceOutcome` | um `.capacity` derivado do último desempenho aparece em algum módulo | tipos distintos sem conversão; teste de arquitetura |
| F2 | cena curando o corpo | `BodyState` volta ao neutro entre eventos | única API de escrita, sempre com `Δt` explícito; property test "sem avanço de relógio, sem mudança" |
| F3 | atributo escolhido à mão | `capacity_baseline` gravado fora do seeding | escrita de `capacity_baseline` só por `seeding.py`; teste de arquitetura |
| F4 | comparação como entrada | arquivo de parâmetro ou fixture ordenando personagens | validador que recusa ordenação fora de `COMPARATIVE` ancorado |
| F5 | re-sorteio de capacidade | mesmo personagem com valores diferentes em duas cenas | `capacity_baseline` imutável após seeding; teste |
| F6 | dependência da ordem de criação | acrescentar um NPC desloca sorteios de outro | substream por `hash(character_id, world_seed)`; teste que insere NPC irrelevante e compara logs |
| F7 | marginais independentes | aluno simultaneamente o mais pesado, o mais rápido e o melhor no vaivém | cópula gaussiana; teste estatístico sobre correlação da coorte amostrada |
| F8 | degeneração de partículas no SIR | posterior colapsa em uma partícula sob restrições apertadas | ESS reportado e limiar; abaixo dele o estimador **falha alto**, não devolve um posterior ruim |
| F9 | teto sem atestação | `UPPER_BOUND` entrando no posterior sem esforço máximo atestado | schema de feat já recusa; o estimador **revalida** e recusa também |
| F10 | detecção saturando numa observação | um olhar decide a suspeita | teto de Δlog-odds por observação; teste de calibração perto do acaso |
| F11 | DOMS imediato | dor de esforço aparecendo no mesmo instante | atraso 12–24 h no canal; property test |
| F12 | `impairment` global | lesão de punho degradando corrida | impairments por dimensão; teste do cenário 4 do modelo §14 |
| F13 | LLM narrando estado | texto pressupondo lesão inexistente | validação de ação: pressupor estado inexistente é ação inválida (fora da V1 como integração, mas o validador é entregue) |
| F14 | razão aguda:crônica ressurgindo | risco de lesão lendo `acute/chronic` | teste que falha se a razão for computada em `effort.py` |
| F15 | estado corporal inválido | hidratação negativa, `w_prime` acima do máximo, `progress` de cura > 1 | invariantes de tipo em `pydantic` + property tests |
| F16 | penalidade térmica por idade | coorte adolescente sofrendo por ser adolescente | proibido: risco é indexado a WBGT e a fatores modificáveis; teste |

---

## 9. Observabilidade e reprodutibilidade

### 9.1 Determinismo

- Todo sorteio vem de `substream(world_seed, character_id, event_id, purpose)`. Nenhum uso de RNG
  global. Um teste de arquitetura recusa `numpy.random.<fn>` e `random.<fn>` no pacote.
- **Critério operacional:** dois runs com o mesmo `world_seed`, a mesma configuração e as mesmas
  versões de parâmetro produzem event logs idênticos após normalização de timestamps de parede.
- **Critério de isolamento:** acrescentar um personagem que não participa de nenhum evento não altera
  nenhum sorteio de nenhum outro personagem. Este é o teste que justifica o esquema de substreams.

### 9.2 Metadados do run

Obrigatórios, e o run **falha ao iniciar** se algum faltar: `world_seed`, versão do prior
populacional, versão do estimador, versão dos parâmetros de dinâmica, versão dos parâmetros do
resolvedor de disputa, versão dos parâmetros de observação, hash do posterior por personagem.

Junto vão, por personagem, `evidence_sufficiency` por dimensão e o ESS do estimador — porque um
posterior com sufficiency ≈ 0 é o prior com outro nome, e quem lê o run precisa saber disso sem
reconstruir a inferência.

### 9.3 Auditabilidade

- O SIR é auditável partícula a partícula: com o mesmo substream, é possível reexecutar e inspecionar
  por que uma restrição empurrou o posterior.
- Cada `contest.exchange` registra os termos da margem separadamente (habilidade, capacidade, massa,
  posicional, penalidade, ruído). Uma luta cujo resultado não se explique pelos termos registrados é
  bug de instrumentação, não de balanceamento.
- `effort.resolved` registra o `capability_available` efetivamente usado. Sem isso, um desempenho
  estranho é indistinguível entre "corpo degradado" e "composição errada".

---

## 10. Migração e compatibilidade

**Nenhum snapshot persistido existe.** O repositório não tem código; esta é a primeira spec que
compila para execução. Portanto não há migração a fazer, e a compatibilidade a estabelecer é para a
frente:

1. `snapshot_version: 1` é introduzido por esta spec. Toda mudança futura na forma de
   `capacity_baseline`, `BodyState` ou `physical_beliefs` incrementa a versão.
2. Um snapshot só é carregável se as versões de parâmetro registradas em `run_metadata` estiverem
   disponíveis. Carregar um snapshot com `prior_version` desconhecida é **erro**, não aviso: os
   números do corpo foram amostrados sob aquele prior e não significam a mesma coisa sob outro.
3. `data/canon/schema/feat.schema.json` e `capacity-dimensions.yaml` **não mudam**. A V1 é
   consumidora dos dois. Se o estimador precisar de um campo que o schema de feat não tem, isso é
   sinal de que o estimador está inventando evidência.
4. Quando S1/S2 fecharem e o prior for recalibrado, `prior_version` incrementa e **snapshots antigos
   não são migrados** — são reexecutados a partir do seed. O corpo amostrado de um prior provisório
   não deve ser reinterpretado sob um prior calibrado; a amostra não é convertível.

---

## 11. Testes e evals

### 11.1 Testes determinísticos (`pytest`) — a maioria

Alvo direto da skill `tdd`. Sem LLM em lugar nenhum.

- **Funções puras por canal.** Para cada um dos nove canais dinâmicos: evolução sob `Δt`, ponto fixo em repouso,
  monotonicidade na direção certa, e limites de saturação.
- **Ordenação de degradação por canal.** Privação de sono degrada na ordem
  habilidade ≫ aeróbico ≳ potência > velocidade ≫ força — testada como **razão entre efeitos**, não
  como valor absoluto (é o que a meta-análise sustenta).
- **Escalas de tempo.** τ de `W'` na faixa de 380–580 s e maior quanto mais alta a intensidade de
  recuperação; hidratação sem efeito abaixo de ~2% e degradante acima; DOMS ausente em t+6 h,
  presente em t+24 h, pico em t+48 h, resolvido em t+7 d.
- **Composição de `capability_available`** por dimensão, com lesão localizada.
- **Resolvedor de esforço**, os dez passos, incluindo rejeição de intent impossível.
- **Viabilidade de ocultação**: os três vereditos, e o custo de reserva do `forced_exposure`.
- **Resolvedor de disputa**: terminação, escrita em `BodyState`, objetivos assimétricos.

### 11.2 Property tests (`hypothesis`)

- **P1 — cena não cura.** Para qualquer estado e qualquer sequência de eventos sem avanço de relógio,
  `BodyState` sai idêntico.
- **P2 — monotonicidade de custo.** Maior `target_intensity` nunca produz menos custo fisiológico.
- **P3 — validade de estado.** Nenhuma sequência de operações produz estado fora dos domínios
  (hidratação, reserva, `progress` de cura, severidade).
- **P4 — piso nunca vira teto.** Para qualquer conjunto de restrições sem atestação maximal, o
  posterior mantém massa acima do maior piso observado.
- **P5 — isolamento de substream.** Para qualquer conjunto de personagens, acrescentar um que não
  participa de eventos não altera o log dos demais.
- **P6 — determinismo.** Mesmo seed, mesmo log.
- **P7 — sem telemetria.** Para qualquer `BodyState`, o payload de contexto gerado por
  `interoception.py` não contém nenhum número presente no estado.

### 11.3 Recuperação do estimador contra verdade sintética

Como `data/canon/feats/` está vazio e permanecerá vazio na V1, o estimador é validado contra uma
população sintética com verdade conhecida:

1. amostra-se uma coorte do prior com um seed — essa é a verdade;
2. simulam-se desempenhos com esforço conhecido e geram-se feats tipados a partir deles;
3. roda-se o estimador **sem** ver a verdade;
4. verifica-se:
   - **cobertura**: o intervalo de credibilidade contém a verdade na frequência nominal;
   - **assimetria**: com apenas `LOWER_BOUND`, a cauda superior permanece larga — o posterior **não**
     converge para a verdade, e isso é o comportamento correto sob identificação parcial;
   - **ocultador vs. mediano**: um personagem forte que rendeu 60% e um mediano que rendeu 100%
     produzem a **mesma** observação; o estimador não os separa sem atestação de esforço. Se separar,
     está usando informação que ninguém dentro do mundo tem (cenário 1 do modelo §14 — a simetria é o
     teste de que o modelo está certo);
   - **valor do teto**: acrescentar um `VERIFICATION_BOUT` estreita a cauda superior de forma
     mensurável, e é a única coisa que estreita.

### 11.4 Testes de invariante nomeados

Um teste por invariante do `physical-model.md` §15, nomeado `test_invariant_<n>_<slug>`, para que uma
falha aponte diretamente para o texto que ela viola. As invariantes 1, 2, 3, 4, 5, 7, 8, 11 e 13 são
verificáveis inteiramente dentro da V1; 6, 9, 10 e 12 são verificáveis na parte que a V1 entrega
(interocepção, unidades, separação de eixos, gate de divergência no seeding).

### 11.5 Cenários de estresse do modelo §14

Cada um vira um teste de integração com seed fixo e asserção sobre o event log:

| Cenário | Asserção central |
|---|---|
| 1 — meio do pelotão de propósito | mesma observação que o mediano; posterior não os separa |
| 2 — desiste por tédio | parcial é `LOWER_BOUND`; dimensão aeróbica recebe `NO_INFORMATION` |
| 3 — semana de provas | `coordination` e `reaction_time` caem; `max_strength` quase não muda |
| 4 — lesão ocultada | impairment por dimensão persiste; pista vaza probabilisticamente; ocultar custa fadiga central |
| 5 — sobrevivência na ilha | terceiro dia começa pior que o primeiro por substrato; sem penalidade térmica por idade |
| 6 — feat pós-divergência | informa o posterior no seeding; ausente de memória e crença |
| 7 — NPC sem evidência | sorteio estável e reprodutível mesmo criado tardiamente |
| 8 — testemunho | `TESTIMONY` não move o posterior; move o belief do `testifier` |

### 11.6 Evals

A V1 não tem comportamento dependente de modelo, então **não há eval de LLM sobre o resolvedor**.
Duas famílias de eval são definidas aqui para a integração seguinte, e ficam versionadas vazias:

- **`knowledge-boundary`** (bloqueante quando o Agent Cognition entrar): dado um agente com lesão
  oculta e um observador, nenhum prompt contém número de `BodyState`, e o agente não consegue
  reproduzir sua própria capacidade nominal quando perguntado diretamente.
- **`simulation-evals`** (comparação entre runs, não história única): sobre ≥ 30 seeds, verificar que
  onde a evidência não separa dois personagens, os vencedores variam; e que a suspeita de ocultação
  cresce com o número de observações, não com um único evento — exceto após um combate, onde muitas
  observações correlacionadas são emitidas de uma vez.

A segunda existe porque a propriedade "lutar revela" é **derivada** no modelo §11, e uma propriedade
derivada precisa ser medida, não afirmada.

---

## 12. O que bloqueia, e o que esta spec faz enquanto bloqueado

Nada aqui é contornado silenciosamente.

| Bloqueio | Efeito na V1 | Conduta |
|---|---|---|
| S1 — normas de coorte (média/DP por idade e sexo) | prior sem calibração numérica | construir com valores provisórios **declarados**; `status: PROVISIONAL` no arquivo; testes de estrutura e de propriedade passam, testes de calibração ficam `xfail` com motivo |
| S2 — correlações entre itens | cargas fatoriais são suposição | declarar no `evidence_sufficiency` do próprio prior; testar que a estrutura de correlação existe, não que ela está certa |
| S3 — distâncias do 持久走 | desnormalização de feat de corrida de fundo insegura | preferir a coluna do vaivém; o estimador recusa desnormalizar 持久走 enquanto S3 estiver aberta |
| S4 — verificação da tabela de pontuação | equiparação entre modalidades aeróbicas não verificada | usável, marcada `verification_status` |
| `oq.capability.effort-attestation` | todo feat cai em `UNKNOWN`; nenhum teto existe | o estimador funciona; simplesmente não recebe `UPPER_BOUND` de dado real |
| `oq.capability.fitness-test-records` | quase nenhuma `INSTRUMENTED_ESTIMATE` | idem |
| `oq.capability.club-and-training-background` | habilidade inicial não separável de capacidade | V1 aceita habilidade como fixture nos testes de disputa |
| `oq.capability.physical-exam-tasks` | não se sabe quais dimensões um exame real exige | V1 implementa as 15 dimensões do enum, sem priorizar |
| `oq.capability.cohort-selectivity` | prior nacional sem deslocamento | hipótese nula **declarada** no arquivo do prior, nunca omissão |

**Regra dura, e ela é o que separa esta V1 de uma V1 desonesta:** nenhum `capacity_posterior` por
personagem canônico é commitado, e nenhum perfil de capacidade nominal compila para o engine
enquanto os bloqueios acima estiverem abertos. A V1 entrega a máquina, exercitada por coortes
sintéticas e por NPCs anônimos sorteados do prior. Personagens focais entram quando a evidência
entrar.

---

## 13. Decisões abertas

Estas **não** estão resolvidas, e nenhuma delas está escondida nos critérios de aceitação da §14.
Cada uma precisa de decisão antes ou durante a implementação, e as duas primeiras são pré-requisito
de tickets específicos.

### 13.1 Forma funcional de `w_hab(Δcapacidade)` — *bloqueia o ticket do resolvedor de disputa*

O modelo exige que o peso da habilidade seja **decrescente na diferença de capacidade** ("habilidade
domina em diferenças pequenas, capacidade em diferenças grandes"), e não diz com que forma. A escolha
entre uma sigmoide invertida, um decaimento exponencial e uma função por partes muda o comportamento
na faixa intermediária, que é justamente a faixa onde quase todo confronto escolar acontece.

*Recomendação:* decaimento exponencial com um único parâmetro de escala, calibrado por propriedade —
"um judoca leve vence um aluno forte e destreinado" e "uma diferença grande de capacidade não é
compensável por técnica" — e não por dado. Registrar a calibração por propriedade como tal.

**Decidido:** 2026-09-09 — decaimento exponencial de parâmetro único sobre o **módulo** da diferença:
`w_hab(Δcap) = w_hab_0 · exp(−|Δcap| / κ)`, com `Δcap` em desvios-padrão do prior da capacidade
relevante à modalidade. É monotonicamente decrescente em todo o domínio e assintótica a zero, de modo
que habilidade nunca vira desvantagem e nenhum ponto de corte separa uma "faixa da técnica" de uma
"faixa da força". Descartadas a sigmoide invertida (dois parâmetros para produzir um platô que
nenhuma propriedade exigida pede) e a função por partes (o joelho vira regra à parte, que é
exatamente o que o modelo §11 evita ao pedir a dominância como *função*). `κ` é **`[INT]`**,
declarado em `contest-params.yaml` (PSV1-7) e **calibrado por propriedade, não por dado**: (P-A) com
vantagem grande de habilidade e desvantagem pequena de capacidade, a margem favorece o técnico — "um
judoca leve vence um aluno forte e destreinado"; (P-B) a partir de uma diferença grande de
capacidade, nenhuma diferença de habilidade representável no modelo inverte a margem. As duas viram
testes executáveis, e o cabeçalho do arquivo registra a origem da calibração **como calibração por
propriedade** — um `κ` que se apresente como medido é a desonestidade que o `[INT]` existe para
impedir.

### 13.2 Granularidade de `peripheral_fatigue` por região — *bloqueia o ticket de `BodyState`*

O modelo cita `legs, arms, grip, core`. Quatro regiões bastam para corrida, agarre e trabalho
sustentado, mas não distinguem punho de mão — e o cenário 4 (lesão de punho decidindo um exame de
preensão) opera exatamente nessa distinção. Aumentar a granularidade de fadiga para acompanhar a de
lesão tem custo em toda a dinâmica.

*Opções:* (a) manter quatro regiões para fadiga e permitir regiões finas apenas em `Injury`;
(b) unificar as duas taxonomias. A (a) é mais barata e cria uma assimetria que precisa ser
documentada; a (b) é mais coerente e mais cara.

**Decidido:** 2026-09-09 — opção (a). `peripheral_fatigue` fica com quatro regiões (`legs`, `arms`,
`grip`, `core`) e `Injury.region` mantém granularidade fina (`wrist_left`, `hand_right`,
`ankle_left`, …). A assimetria é deliberada e fica documentada em duas partes, não em uma nota:
(i) o cabeçalho de `body-dynamics.yaml` declara as duas taxonomias e por que diferem; (ii) o mesmo
arquivo carrega um mapa **total** `Injury.region → região de fadiga`, e um teste do PSV1-2 falha se
alguma região de lesão ficar sem imagem. O cenário 4 do modelo §14 sobrevive porque não depende de
fadiga: punho e mão já se distinguem em `Injury`, e o que decide um exame de preensão é o campo
`impairments` **por dimensão** (modelo §7), não a região de fadiga. Unificar as taxonomias custaria a
dinâmica inteira de fadiga por região fina para comprar uma distinção que o mecanismo que a usa nem
lê. Fadiga **não** ganha granularidade na V1; se um exame futuro exigir fadiga distinta entre punho e
mão, isso é revisão de spec, não ajuste de parâmetro.

### 13.3 Limiar de ESS e o que fazer abaixo dele

O estimador reporta ESS. Falta decidir o limiar e a conduta: falhar alto, reamostrar com mais
partículas, ou devolver o posterior marcado como degenerado. *Recomendação:* falhar alto na V1 — um
posterior degenerado que circula é pior que um seeding que não completa, porque o primeiro produz
números plausíveis e errados.

**Decidido:** 2026-09-09 — falhar alto, com limiar duplo. O gate é o ESS **conjunto** do vetor de
pesos: degeneração é propriedade do conjunto de partículas, não de uma dimensão isolada; o ESS por
dimensão que o PSV1-3 reporta é diagnóstico e não abre nem fecha o gate. O posterior é reprovado se
`ESS < 500` **ou** `ESS < 0,05 · N`, com `N = 10 000` partículas por padrão; os três números entram
em `estimator-params.yaml` marcados `[INT]`. Os dois limiares coincidem no `N` padrão e só divergem
quando alguém o muda — que é justamente quando um limiar único enganaria: com `N` grande a fração
impede a falsa sensação de amostra, com `N` pequeno o piso absoluto impede que 50 partículas efetivas
passem por posterior bem-comportado. O piso de 500 mantém o erro padrão de Monte Carlo de uma média
em torno de 4–5% do desvio do posterior, uma ordem de grandeza abaixo da largura que a identificação
parcial legitimamente produz; abaixo disso o ruído do estimador começa a competir com a incerteza que
ele deveria estar reportando. **Conduta abaixo do limiar:** não reamostrar com mais partículas e não
devolver posterior marcado — escrever `capacity.posterior.degenerate` no event log com
`character_id`, ESS, limiar violado, `N` e as restrições aplicadas, e levantar erro que interrompe o
run. Nenhum posterior degenerado é amostrado, congelado em `capacity_baseline` ou serializado em
snapshot. As correções admissíveis são revisar o conjunto de restrições ou alargar as indicadoras
suavizadas, ambas visíveis no diff; **baixar o limiar é mudança de spec**, não ajuste de arquivo.

### 13.4 Python fixa a stack do projeto? — *decisão de escopo, não técnica*

O `README.md` afirma que o projeto ainda não escolheu framework. Esta spec escolhe Python **para o
subdomínio `Embodiment`**, cuja natureza (amostragem multivariada, funções puras, zero LLM) torna a
escolha quase independente do resto. Se o Agent Cognition ou o Examination Engine forem para outra
linguagem, esta decisão vira uma fronteira de processo.

*Encaminhamento sugerido:* registrar como ADR 0007 antes de `/to-tickets`, com escopo explicitamente
limitado a este subdomínio. Não bloqueia a implementação; bloqueia a generalização.

**Decidido:** 2026-09-09 — sim para o subdomínio, não para o projeto. Registrado no
[ADR 0007](../adr/0007-python-para-o-subdominio-embodiment.md), status `Accepted`, com o limite de
escopo no próprio título. Cobre `src/embodiment/`, sua toolchain (`numpy`, `pydantic`, `pytest`,
`hypothesis`) e os arquivos de parâmetro em `data/models/physical/`. Não cobre Agent Cognition,
Examination Engine, Social State, framework de agentes, provedor de LLM, banco vetorial nem UI —
todos seguem em aberto como o `README.md` afirma. A fronteira do subdomínio é **dados** (event log,
snapshot, YAML de parâmetros), não importação de módulo, de modo que uma stack diferente adiante vira
fronteira de processo e não reescrita. Citar o ADR 0007 como precedente para "o projeto é Python" é
uso indevido dele.

### 13.5 `illnesses` sem dinâmica

O campo é serializado e nada o atualiza. Alternativa seria omiti-lo do `snapshot_version: 1` e
introduzi-lo depois, ao custo de um incremento de versão. *Recomendação:* manter o campo, porque um
exame de sobrevivência vai precisar dele e o incremento de versão é mais caro que um campo vazio.

**Decidido:** 2026-09-09 — confirmada a recomendação: o campo fica. `illnesses` é serializado em
`snapshot_version: 1` e **nenhum ticket da V1 o atualiza**; todo run da V1 o escreve como `[]`. Para
que a ausência de dinâmica não seja lida adiante como bug, ela é declarada em dois lugares: o
cabeçalho de `body-dynamics.yaml` registra que doença não é canal dinâmico nesta versão (os nove
canais do modelo §7 não a incluem), e um teste do PSV1-2 falha se qualquer módulo escrever no campo.
Reverter custaria um incremento de `snapshot_version` no primeiro exame de sobrevivência, que é mais
caro que um campo vazio já versionado.

---

## 14. Critérios de aceitação

A V1 está pronta quando:

1. **Seeding.** É possível semear uma coorte de N alunos anônimos a partir do prior, com um
   `world_seed`, e a coorte amostrada exibe a estrutura de correlação do prior — não existe o aluno
   simultaneamente mais pesado, mais rápido e melhor no vaivém.
2. **Estimador.** O SIR converte restrições tipadas em posterior, reporta ESS e
   `evidence_sufficiency` por dimensão, recusa `UPPER_BOUND` sem atestação maximal, e passa os quatro
   testes de recuperação sintética da §11.3 — incluindo o de **não** separar o ocultador do mediano.
3. **Corpo.** Os nove canais dinâmicos evoluem com as escalas de tempo da tabela do modelo §7, e a suíte de
   funções puras e de property tests da §11.1–11.2 passa, com P1 (cena não cura) e P7 (sem
   telemetria) entre elas.
4. **Esforço.** Os dez passos resolvem na ordem normativa da §6; intent impossível é rejeitado com
   evento próprio; os três vereditos de ocultação são alcançáveis e o `forced_exposure` cobra
   reserva.
5. **Disputa.** A máquina de 3 a 10 trocas resolve com logística **por troca**, e as duas propriedades
   emergentes são medidas e não assumidas: disputas longas favorecem condicionamento, curtas
   favorecem técnica e iniciativa.
6. **Observação e crença.** Cada evento emite `ObservedPerformance` por observador; a suspeita de
   ocultação acumula em log-odds no belief do observador, com uma observação isolada perto do acaso;
   o ocultador nunca vê o sorteio.
7. **Fronteira de conhecimento.** P7 passa, e a `knowledge-boundary-audit` sobre as cinco classes de
   vazamento da §7.3 não encontra nenhuma.
8. **Reprodutibilidade.** Mesmo seed → mesmo event log; acrescentar personagem irrelevante não
   desloca sorteio de ninguém; o run falha ao iniciar sem os metadados obrigatórios da §9.2.
9. **Cenários.** Os oito cenários de estresse do modelo §14 passam como testes de integração com seed
   fixo.
10. **Honestidade dos parâmetros.** Todo arquivo em `data/models/physical/` declara `not_canon`, sua
    versão, e o que nele é medido e o que é `[INT]`. O `population-prior.yaml` declara
    `status: PROVISIONAL`, a hipótese nula de seletividade de coorte, e o `evidence_sufficiency` do
    próprio prior. O `body-dynamics.yaml` carrega a nota de que a forma de dois exponenciais é
    contabilidade de estado, não preditor validado.
11. **Nenhum posterior por personagem canônico está commitado**, e nenhum perfil de capacidade
    nominal compila para o engine.
12. **`src/embodiment/` não importa nenhum cliente de LLM**, e não usa RNG global. Ambos verificados
    por teste de arquitetura.

Os critérios acima pressupõem que as decisões da §13 tenham sido tomadas — não que tenham sido
tomadas de alguma maneira em particular. Um critério de aceitação que dependa de 13.1 ou 13.2 não
pode ser avaliado antes delas, e é assim que deve ser.

---

## 15. Próximo passo

Fatiada em [`docs/tickets/physical-simulation-v1/`](../tickets/physical-simulation-v1/) — nove tickets, `PSV1-0` a `PSV1-8`, com matriz de cobertura dos critérios de aceitação, dos cenários, das property tests e dos modos de falha.

`/to-tickets` sobre esta spec. Ordem de dependência sugerida: `rng` → `types` → `dynamics` →
`capability` → `prior` → `estimator` → `seeding` → `effort` → `observation`/`detection` →
`interoception` → `contest` → `snapshot`. As decisões 13.1 e 13.2 precisam estar fechadas antes dos
tickets de `contest` e de `types`, respectivamente.
