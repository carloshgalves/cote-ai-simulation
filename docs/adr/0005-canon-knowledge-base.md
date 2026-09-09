# ADR 0005 — Canon Knowledge Base: taxonomia, tempo, proveniência e fronteira com a simulação

**Status:** Accepted
**Data:** 2026-09-08
**Relacionado:** ADR 0001 (world truth × crença), ADR 0002 (personagem = identidade + evidência + memória), ADR 0004 (fronteira do primeiro ano)

## Contexto

O simulador precisa de um estado inicial objetivo do mundo no começo do primeiro ano. O material canônico não entrega esse estado pronto: a obra revela regras e estruturas em ordem narrativa, não em ordem cronológica. Uma regra explicada no Volume 7 pode já estar em vigor no primeiro dia de aula; um fato revelado no Volume 1 pode ser falso; uma afirmação de wiki pode ser uma inferência de fã apresentada como texto.

Sem uma disciplina explícita, três falhas são inevitáveis:

1. usar o momento da revelação ao leitor como se fosse o momento em que o fato passou a valer;
2. tratar conhecimento nosso (leitores do fim da série) como conhecimento dos personagens em abril;
3. deixar afirmações não verificadas endurecerem em regras determinísticas do engine.

## Decisão

### 1. A unidade atômica é o `CanonClaim`

Afirmações canônicas são registros pequenos, versionados em Git, com identidade estável. Não usamos documentos de prosa como fonte de verdade para o engine.

### 2. Taxonomia em três eixos ortogonais

Um enum único não consegue expressar que "Classe D é a classe dos rejeitados" é ao mesmo tempo fato objetivo, oculto dos alunos em abril, e não verificado por nós.

- `claim_kind` — o que a proposição é: `WORLD_TRUTH`, `INSTITUTIONAL_RULE`, `EVENT`, `ENTITY`, `BELIEF`.
- `visibility` / `known_by` — quem pode saber, no universo, em cada instante.
- `epistemic_status` — nossa confiança: `VERIFIED`, `INFERRED`, `INTERPRETATION`, `UNVERIFIED`.

`INFERENCE` do enunciado original é desambiguada: inferência **nossa** vira `epistemic_status: INFERRED`; inferência **do personagem** vira `claim_kind: BELIEF` com `derivation: inference`.

### 3. Quatro eixos temporais

- **valid time** — desde quando vale no universo (`temporal.effective_from/to`);
- **narrative time** — onde a obra revela ao leitor (`provenance.supports[].locator`);
- **knowledge time** — desde quando cada ator sabe (`known_by[].since`);
- **transaction time** — desde quando nós afirmamos (commit Git + `revision`).

### 4. Origem desconhecida é um valor de primeira classe, não um palpite

Saber que uma regra operava em T prova que ela existia **em T ou antes** — nada além disso. É proibido preencher a origem com `SCHOOL_FOUNDING` (ou qualquer outra âncora) por conveniência.

`effective_from` é um `TimeBound` com `mode`:

- `EXACT` — o texto declara o momento da instituição;
- `HOLDS_BY` — provado que já valia em um ponto; **origem desconhecida** (caso majoritário);
- `INTERVAL` — ambos os limites têm suporte textual;
- `UNKNOWN` — nem o limite superior é conhecido.

Um limite inferior (`not_before`) só pode existir acompanhado de `not_before_support` apontando para fonte. O schema recusa o contrário.

Isto é suficiente para semear o mundo: para o snapshot de `Y1_START` a pergunta relevante é `holds_at(Y1_START)`, que se responde com o limite superior sozinho. A origem real nunca é necessária para simular, e por isso nunca precisa ser inventada.

### 5. Tier é função de autoria e forma, não de canal de distribuição

Volume 0 é prosa de light novel do autor original e ocupa o mesmo tier dos demais volumes da mesma edição, apesar de ter sido distribuído como bônus de BD. Materiais complementares oficiais (fichas do School Database, posfácios, perfis de site oficial, drama CDs, encartes) formam um tier próprio, abaixo da prosa e acima das adaptações.

### 6. Comportamento observado não invalida regra declarada

Quando uma regra declarada e o comportamento observado do mundo divergem, ambos são registrados como claims distintos e um `conflict` é aberto com hipóteses explícitas: exceção, escopo mais estreito que o enunciado, mudança temporal, fonte da declaração não confiável, observação não confiável, leitura equivocada nossa.

Enquanto o conflito estiver aberto, **nenhum dos dois lados compila** para o engine. O engine não recebe um vencedor arbitrado por conveniência.

### 7. Fatos de admissão canônicos ≠ modelo de admissão da simulação

`CANON_ADMISSION_FACTS` registra apenas o que a obra afirma: que existem dimensões avaliadas, que a alocação reflete mérito, que há uma lista governamental de elegibilidade, que houve ao menos uma admissão por aprovação direta do diretor.

`SIMULATION_ADMISSION_MODEL` é reconstrução nossa, vive fora de `data/canon/`, é marcado `not_canon: true` e é **calibrado contra** as alocações canônicas em vez de derivado delas. A obra nunca declarou uma fórmula de alocação; nós não vamos fabricar uma e depois citá-la como cânone.

Dependência unidirecional: cânone → modelos. Nenhum registro sob `data/models/` pode aparecer em `provenance.supports` de um claim canônico.

### 8. O engine lê um snapshot compilado, não as notas de pesquisa

`data/canon/derived/` contém artefatos gerados, validados por schema e identificados por hash de conteúdo. O hash entra nos metadados do run, atendendo à exigência de reprodutibilidade. Um claim só compila se tiver suporte de tier adequado e verificação humana registrada.

### 9. Quatro portões de recuperação

Toda leitura do KB por engine, context builder ou RAG passa por: `effective_at(t_sim)`, `actor_gate(actor, t_sim)`, `spoiler_horizon(work, locator)`, `divergence_gate(t_div)`.

O `known_by` canônico é autoritativo **apenas em `t = Y1_START`**, para semear o belief state inicial. Depois disso quem governa é o event log da simulação. Sem essa fronteira, o KB vira canal lateral de vazamento e o ADR 0001 deixa de valer na prática.

## Consequências

- Escrever cânone fica mais caro: cada afirmação exige limite temporal, proveniência e status epistêmico.
- Em compensação, o vazamento de informação vira uma consulta verificável em vez de uma revisão manual de prompt.
- A maior parte do KB nascerá `UNVERIFIED`. Isso é honesto e é o estado correto: wiki e memória do modelo localizam onde olhar, não sustentam afirmações.
- Regras econômicas e acadêmicas do engine ficam bloqueadas até leitura direta da light novel. É um bloqueio desejado.
