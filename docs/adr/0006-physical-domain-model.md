# ADR 0006 — Domínio físico: capacidade latente, estado corporal persistente e esforço escolhido

**Status:** Accepted
**Data:** 2026-09-08
**Relacionado:** ADR 0001 (world truth × crença), ADR 0002 (personagem = identidade + evidência + memória), ADR 0003 (tempo lógico), ADR 0005 (base canônica: taxonomia, tempo, proveniência)

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

Personagem focal e NPC anônimo usam **o mesmo tipo**: um é prior com muitas restrições, o outro é o
mesmo prior com nenhuma. Não existem dois sistemas.

### 3. Feat é restrição tipada, e a assimetria piso/teto é estrutural

Desempenho observado dá `LOWER_BOUND`. Teto (`UPPER_BOUND`) só existe com esforço máximo atestado —
exigência imposta pelo schema, do mesmo modo que `not_before` exige `not_before_support` no ADR 0005.
Registro oficial medido dá `INSTRUMENTED_ESTIMATE`; comparação no mesmo evento dá `COMPARATIVE`;
afirmação de personagem dá `TESTIMONY`, que alimenta belief state e **não** o posterior; desempenho
notoriamente submáximo sem sinal de esforço dá `NO_INFORMATION`.

Consequência aceita: personagens que nunca foram vistos no limite têm cauda superior larga. Isso é a
descrição correta da nossa ignorância, não uma afirmação velada de superioridade.

### 4. Ausência de evidência é prior populacional, com proveniência

Alunos sem evidência sorteiam de coorte condicionada ao que o cânone informa (ano, sexo, clube ou
ausência dele, reputação atlética, porte). O prior é reconstrução nossa, vive em
`data/models/physical/`, é `not_canon: true`, é calibrado contra dados reais de aptidão física por
idade — e suas normas numéricas precisam ser transcritas da publicação, não da memória do modelo.

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

Fadiga periférica e central, carga acumulada, dívida de sono e fase circadiana, energia, hidratação,
carga térmica, dor, soreness, lesões e doenças formam um estado corporal indexado no tempo, presente
no snapshot. Cena não cura nada: apenas o avanço do relógio com regras de recuperação e eventos do
engine alteram o estado.

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

Sem atributos de combate paralelos. Habilidade domina em diferenças pequenas de capacidade e
capacidade domina em diferenças grandes; fadiga central e dor degradam técnica mais rápido que força;
objetivos assimétricos (conter, escapar, deter, perder de propósito) mudam a função objetivo.
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

## Detalhamento

Modelo completo, incluindo campos, ordem de resolução, cenários de estresse e invariantes numeradas:
[`docs/architecture/physical-model.md`](../architecture/physical-model.md).
