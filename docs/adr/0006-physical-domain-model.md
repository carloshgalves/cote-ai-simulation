# ADR 0006 — Domínio físico: capacidade latente, estado corporal persistente e esforço escolhido

**Status:** Accepted
**Data:** 2026-09-08 · **Revisado:** 2026-09-09 (ver [Revisão](#revisão--2026-09-09))
**Relacionado:** ADR 0001 (world truth × crença), ADR 0002 (personagem = identidade + evidência + memória), ADR 0003 (tempo lógico), ADR 0005 (base canônica: taxonomia, tempo, proveniência)
**Fundamentação:** [`docs/research/physical-domain-v1.md`](../research/physical-domain-v1.md)

## Contexto

A simulação precisa resolver corridas, exames de sobrevivência, esforço prolongado, privação de sono,
fome, calor, dor, lesão e confronto físico. Precisa fazê-lo para personagens canônicos de capacidade
excepcional e para centenas de alunos comuns sobre os quais a obra não diz nada.

A modelagem ingênua — um atributo escalar por personagem, derivado do desempenho que a obra mostra —
falha de três maneiras simultâneas:

1. **Confunde desempenho com capacidade.** Um personagem que rende deliberadamente menos do que pode
   ficaria registrado como mediano de forma permanente, contradizendo a premissa do material de origem.
2. **Convida a inventar cânone.** Corrigir isso à mão (`strength: 95` porque "sabemos" que ele é forte)
   fabrica um número que a obra nunca declarou e pré-decide comparações que deveriam ser resultado da
   simulação.
3. **Perde consequência.** Fadiga, dor e lesão tratadas como cor de cena desaparecem no corte, e
   descanso deixa de ser recurso administrável — o que remove justamente a camada estratégica que exames
   de vários dias deveriam ter.

Há ainda um problema de fronteira: capacidade física é a área onde é mais tentador dar ao LLM
autoridade narrativa sobre o mundo ("ele ignora a dor e vence"), o que violaria o ADR 0001 e a
invariante 5 do `CONTEXT.md`.

## Decisão

### 1. Seis estruturas, nunca fundidas

`CapacityProfile` (latente), `BodyState` (atual), `ExertionIntent` (escolhido), `PerformanceOutcome`
(realizado), `ObservedPerformance` (percebido por um observador) e `SelfPhysicalModel` (autopercepção)
são tipos distintos com donos distintos. O pipeline é unidirecional: capacidade × estado × ambiente ×
esforço → desempenho → observação → crença.

### 2. Capacidade é distribuição para nós e amostra congelada para o engine

`population_prior(coorte) ⊗ constraints[] = capacity_posterior`, amostrado uma vez no world seeding
com o seed do run e congelado no snapshot. O engine opera sobre um valor definido; a incerteza fica
registrada como distribuição versionada, não como palpite endurecido.

O prior é **multivariado** — dois fatores latentes (aptidão geral e porte) mais resíduo por dimensão,
como cópula gaussiana sobre as marginais oficiais. Marginais independentes sorteariam o aluno
simultaneamente mais pesado, mais rápido e melhor no vaivém.

O estimador que converte restrições em posterior é **amostragem por importância com reamostragem**
sobre substream nomeado — não MCMC, porque um sampler externo fica fora do esquema de reprodutibilidade
da decisão 13. `LOWER_BOUND` e `UPPER_BOUND` são censura por intervalo, e capacidade observada só por
pisos é um parâmetro **parcialmente identificado**: a cauda superior larga é o comportamento correto
do estimador, não uma escolha estilística nossa.

Personagem focal e NPC anônimo usam **o mesmo tipo**: um é prior com muitas restrições, o outro é o
mesmo prior com nenhuma. Não existem dois sistemas.

### 3. Feat é restrição tipada, e a assimetria piso/teto é estrutural

Desempenho observado dá `LOWER_BOUND`. Teto (`UPPER_BOUND`) só existe com esforço máximo atestado —
exigência imposta pelo schema, do mesmo modo que `not_before` exige `not_before_support` no ADR 0005.
Registro oficial medido dá `INSTRUMENTED_ESTIMATE`; comparação no mesmo evento dá `COMPARATIVE`;
afirmação de personagem dá `TESTIMONY`, que alimenta belief state e **não** o posterior; desempenho
notoriamente submáximo sem sinal de esforço dá `NO_INFORMATION`.

A regra tem também justificativa empírica, e não apenas a analogia interna com o ADR 0005: critérios
secundários de esforço máximo são satisfeitos a **61% do VO₂max**, e a detecção clínica de esforço
insincero por profissionais com dinamômetro erra entre **47% e 69%** das vezes. Sinal de esforço é o
critério secundário da ficção; nunca licencia teto. A exceção estrutural é `VERIFICATION_BOUT` — um
segundo desempenho independente em condições nas quais reter esforço não era viável — análogo direto
da fase de verificação de VO₂max.

Consequência aceita: personagens que nunca foram vistos no limite têm cauda superior larga. Isso é a
descrição correta da nossa ignorância, não uma afirmação velada de superioridade.

### 4. Ausência de evidência é prior populacional, com proveniência

Alunos sem evidência sorteiam de coorte condicionada ao que o cânone informa (ano, sexo, clube ou
ausência dele, reputação atlética, porte). O prior é reconstrução nossa, vive em
`data/models/physical/`, é `not_canon: true`, é calibrado contra dados reais de aptidão física por
idade — e suas normas numéricas precisam ser transcritas da publicação, não da memória do modelo.

`evidence_sufficiency` deixa de ser adjetivo e passa a ser `1 − sd(posterior)/sd(prior)` por dimensão.
Vale também para o próprio prior: enquanto as cargas fatoriais forem suposição declarada, o arquivo do
prior precisa dizê-lo.

**Seletividade da coorte** é questão aberta (`oq.capability.cohort-selectivity`). Enquanto aberta, o
prior usa a coorte nacional **sem deslocamento**, declarado como hipótese nula. Inventar um "bônus de
escola de elite" violaria a decisão 15 na escala da coorte inteira, que é pior do que na de um
personagem, porque desloca silenciosamente todos os sorteios.

### 5. Corpus de feats é compartilhado, não um store por personagem

`data/canon/feats/` é uma coleção única indexada por ator, modalidade e dimensão. Restrições
comparativas não têm dono único, normalização de condições exige ver os participantes juntos, e um
store por personagem duplicaria o mesmo evento com números divergentes. O corpus tem dois
consumidores separados: o estimador de capacidade (números, no seeding) e o RAG de fidelidade
(paráfrase, em execução). O posterior numérico nunca é documento recuperável por RAG.

### 6. Capacidade, habilidade e disposição são eixos separados

Capacidade pertence ao engine; habilidade (técnica, formação, clube) ao Canon Knowledge mais
adaptação em simulação; disposição (querer lutar, aceitar dor, entediar-se) ao Character Core e ao
Agent Cognition. Abandonar uma prova por tédio não é teto aeróbico. Ser forte não é saber lutar.

### 7. `BodyState` persiste e só o engine o escreve

Reserva de trabalho acima da potência crítica, fadiga periférica e central, carga acumulada, dívida de
sono e fase circadiana, energia, hidratação, carga térmica, dor, soreness, lesões e doenças formam um
estado corporal indexado no tempo, presente no snapshot. Cada canal tem sua própria escala de tempo —
segundos para a reserva, dias para o substrato, um atraso de 12–24 h para a dor tardia — porque um
reservatório único de "stamina" não representa nenhum dos cenários de estresse do modelo.

Cena não cura nada: apenas o avanço do relógio com regras de recuperação e eventos do engine alteram
o estado.

**A razão aguda:crônica não é fator de risco de lesão.** A primeira versão desta decisão a listava
como tal; ela saiu (ver revisão 2026-09-09). O risco é dirigido por carga recente relativa à
`capability_available` do próprio corpo, mais histórico de lesão na mesma região, sob enquadramento
dinâmico-recursivo.

Saída de LLM não cria, cura nem anula lesão, fadiga ou capacidade. Narração que pressupõe estado
inexistente é ação inválida.

### 8. O agente recebe interocepção, não telemetria

Nenhum número de `BodyState` entra em prompt. O context builder entrega sinais qualitativos e
enviesados ("pernas pesadas", "sede", "dor no punho"), com viés modulado por consciência corporal e
analgesia. Estado corporal de terceiros chega apenas por observação e pistas visíveis.

Este é o ADR 0001 aplicado ao corpo: um personagem pode subestimar sua própria lesão e o engine, que
conhece a verdade, aplica o agravamento.

### 9. Ocultação de capacidade é intenção de primeira classe

`ExertionIntent` inclui `display_ceiling`: uma banda de desempenho observável alvo. O engine verifica
**viabilidade** (manter-se no terço médio quando o pelotão acelera pode ser impossível) e resolve
`{ok | forced_exposure | forced_loss}`. Detecção é processo estocástico função da capacidade de
inferência do observador, da margem entre exibido e real, do vazamento de pistas e do número de
observações acumuladas. A suspeita acumula no belief state do observador, não no corpo do ocultador,
que nunca vê o resultado do sorteio.

### 10. Um estimador de capacidade, dois clientes

A mesma inferência limite-a-partir-de-desempenho serve a nós sobre o cânone (no seeding) e a cada
agente sobre eventos da simulação (em execução). A diferença entre um observador arguto e um ingênuo
é a qualidade da inferência, não o acesso à informação — implementação concreta do ADR 0001.

### 11. Combate é resolvedor de disputa sobre os mesmos primitivos

Sem atributos de combate paralelos. Máquina de ~3 a 10 trocas, com função logística
(Bradley-Terry/Elo) aplicada **por troca, não por luta** — daí decorrem, sem caso especial, que
disputas longas favorecem condicionamento e curtas favorecem técnica e iniciativa. A margem de cada
troca inclui `body_mass` como termo explícito, porque categorias de peso existem por um motivo, e um
peso de habilidade decrescente na diferença de capacidade, que implementa "habilidade domina em
diferenças pequenas, capacidade em diferenças grandes" como função e não como regra à parte. Fadiga
central e dor degradam técnica mais rápido que força; objetivos assimétricos (conter, escapar, deter,
perder de propósito) mudam a função objetivo.
Consequências institucionais pertencem às regras da escola, não ao corpo. Toda saída de combate
escreve em `BodyState`, e lutar é o evento mais informativo que existe sobre capacidade — para um
ocultador, esse é o custo dominante de lutar.

### 12. Resolução física é subdomínio do Simulation Engine

`Embodiment` fica dentro do Simulation Engine, não como bounded context par. Um "Physical Engine" ao
lado criaria dois módulos capazes de decidir o que aconteceu, que é exatamente o sintoma de domínio
mal recortado. O Examination Engine **consome** resolução física; não a reimplementa. `ExamSpec` não
carrega atributos físicos por participante.

### 13. Determinismo, substreams e metadados

Fadiga, recuperação, sono, hidratação, cura e modificadores ambientais são funções puras de estado e
tempo — teste comum, sem LLM. Sorteios (amostra de capacidade, ruído de desempenho, lesão, detecção,
erro de observação) usam substreams nomeados derivados de `(world_seed, character_id, event_id,
purpose)`, de modo que acrescentar um personagem irrelevante não desloque os sorteios de outro.
Versões de prior, estimador, dinâmica e resolvedor, mais o hash do posterior por personagem, entram
nos metadados do run.

### 14. Feat posterior à divergência informa prior, nunca memória

Um corpo não muda porque a linha do tempo bifurcou: feat posterior à divergência pode informar o
posterior de capacidade no seeding. Uma memória sim muda: esse feat nunca entra em memória episódica
nem em belief state, e depois do seeding não informa mais nada. Refinamento do `divergence_gate` do
ADR 0005 específico do domínio físico.

### 15. Comparações entre personagens são saída, nunca entrada

Nenhum registro pode afirmar que um personagem supera outro, exceto como `COMPARATIVE` ancorado em
evento observado específico. Onde a evidência não separa dois personagens, seeds diferentes podem
produzir vencedores diferentes; essa é a representação correta da incerteza. Um registro que declare
ordem de força fora dessa forma é rejeitado em review.

## Alternativas descartadas

| Alternativa | Por que não |
|---|---|
| Atributos escalares 0–100 por personagem | colapsa capacidade, estado e desempenho; impossibilita ocultação; obriga a inventar números |
| Derivar atributos diretamente dos feats mostrados | trata desempenho como máximo, punindo permanentemente quem se esconde |
| Modificadores só dentro de cada exame | perde persistência; descanso e lesão deixam de ser recursos; contradiz consequência entre eventos |
| Sistema de combate com stats próprios | segunda fonte de verdade sobre o mesmo corpo; divergências inevitáveis |
| LLM narra o desfecho físico | viola ADR 0001 e a invariante 5; capacidade passaria a ser função de retórica |
| RAG por personagem para evidência física | restrição comparativa não tem dono único; normalização de condições fica impossível |

## Consequências

- Semear o mundo passa a exigir um prior populacional versionado e um estimador, além do snapshot
  canônico. Mais infraestrutura antes do primeiro exame físico.
- Feats ficam mais caros de registrar: exigem dimensão, unidade, condições e atestação de esforço.
  Em troca, ocultação e incerteza passam a ser auditáveis por consulta em vez de por leitura de prompt.
- `knowledge-boundary-audit` ganha uma classe nova de vazamento para testar: telemetria corporal em
  prompt, `BodyState` de terceiros, e resultado de sorteio de detecção visível ao ocultador.
- A maioria dos personagens começará com capacidade dominada pelo prior. `evidence_sufficiency`
  torna isso explícito em vez de disfarçá-lo.
- Perguntas como "quem vence entre X e Y" deixam de ter resposta no repositório e passam a ter
  resposta na execução, com seed registrado.
- Enums existentes ganham valores: `domain: physical` em `claim.schema.json` e
  `category: capability` em `open_question.schema.json`. Novos tipos de registro:
  `feat.schema.json` e o vocabulário `enums/capacity-dimensions.yaml`.
- Quatro lacunas de *sourcing* de instrumento ficam abertas fora do fluxo de cânone, em
  [`data/models/physical/SOURCING.md`](../../data/models/physical/SOURCING.md). Elas não são perguntas
  sobre a obra e por isso não vivem em `open_questions/`; mas o prior permanece provisório enquanto
  não fecharem, e precisa declará-lo.

## Revisão — 2026-09-09

A pesquisa [`physical-domain-v1.md`](../research/physical-domain-v1.md) fundamentou a parametrização
do domínio e produziu sete correções. Todas foram aceitas. Nenhuma altera as 15 decisões em sua
estrutura; uma delas corrige um erro factual e as demais tornam concreto o que estava enunciado.

| # | Correção | Onde bateu |
|---|---|---|
| 1 | **Razão aguda:crônica retirada do risco de lesão** — matematicamente acoplada, instável com carga crônica baixa, sem efeito causal demonstrado, e a figura do "sweet spot" que a popularizou é objeto de pedido formal de retratação. Substituída por carga recente relativa à `capability_available` mais histórico regional, sob etiologia dinâmico-recursiva. `acute_7d`/`chronic_28d` seguem como contabilidade de carga para deriva de capacidade | decisão 7 · modelo §7 |
| 2 | **Âncoras de `agility` e `strength_endurance` corrigidas para o item real** — 反復横とび é contagem de toques em 20 s, não tempo em percurso; 上体起こし é repetições em 30 s. Uma âncora que não corresponde ao instrumento não é âncora | modelo §4 · `capacity-dimensions.yaml` |
| 3 | **`VERIFICATION_BOUT` adicionado a `effort.attestation`** — única forma estrutural, não retórica, de atestar esforço máximo | decisão 3 · `feat.schema.json` |
| 4 | **Justificativa empírica de por que sinal de esforço não licencia teto** — critérios secundários satisfeitos a 61% do máximo; detecção clínica erra 47–69% | decisão 3 · modelo §6 · comentário do schema |
| 5 | **`evidence_sufficiency` operacional**: `1 − sd(posterior)/sd(prior)` por dimensão, aplicável também ao próprio prior | decisão 4 · modelo §5 |
| 6 | **Prior multivariado**, dois fatores latentes como cópula gaussiana sobre as marginais oficiais — sem isso a primeira implementação sortearia quinze números independentes | decisão 2 · modelo §5 |
| 7 | **`body_mass` como termo explícito da margem de disputa**, e `strength_endurance`/preensão como dimensões de primeira classe em agarre | decisão 11 · modelo §11 |

Além das correções, a pesquisa fechou escolhas que o ADR original deixava em aberto: o estimador é
amostragem por importância sobre substream nomeado (identificação parcial sob censura por intervalo);
a resolução de disputa aplica a logística de Bradley-Terry **por troca**, e é daí que saem as duas
propriedades que a decisão 11 exigia; a detecção de ocultação é acumulação de log-odds, calibrada
perto do acaso para uma observação isolada; e as constantes de tempo de cada canal corporal têm agora
fonte.

**O que a revisão deliberadamente não fechou:** as quatro lacunas de sourcing (S1–S4) e
`oq.capability.cohort-selectivity`. Enquanto abertas, o prior usa a coorte nacional sem deslocamento
como hipótese nula declarada, as cargas fatoriais são suposição declarada, e nenhum posterior por
personagem é commitado. A pesquisa também não fecha nenhuma das quatro `oq.capability.*` originais —
elas exigem leitura Tier 0–1, e o que existe agora é o protocolo de extração que permite fechá-las em
uma passagem em vez de quatro.

Uma nota de honestidade que acompanha a parametrização: a forma de dois exponenciais para fadiga
entre dias é adotada como contabilidade de estado consistente, **não** como preditor validado de
desempenho. As revisões do modelo impulso-resposta são duras com sua capacidade preditiva, e o arquivo
de parâmetros precisa registrar isso para que ninguém o cite adiante como física estabelecida.

## Detalhamento

Modelo completo, incluindo campos, ordem de resolução, cenários de estresse e invariantes numeradas:
[`docs/architecture/physical-model.md`](../architecture/physical-model.md).
Fundamentação, alternativas avaliadas e fontes:
[`docs/research/physical-domain-v1.md`](../research/physical-domain-v1.md).
