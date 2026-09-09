# Roadmap — COTE AI Simulation

Este roadmap organiza as frentes de desenvolvimento sem transformar acontecimentos futuros do cânone em roteiro obrigatório.

Princípio geral:

> **Preservar causas, regras, atores e pressões canônicas quando sustentadas; não preservar resultados que deixaram de ser causalmente necessários depois da divergência.**

O objetivo do projeto não é reproduzir a light novel. É construir uma ANHS causalmente executável, povoada por personagens persistentes, capaz de produzir uma timeline própria a partir das mesmas pressões, regras, capacidades e conhecimentos disponíveis.

## Legenda

- ✅ estabelecido/modelado
- 🔄 frente atual
- ⏳ planejado
- ❓ depende de cânone/decisão futura
- 🧪 requer validação/evals
- 🚫 não bloqueia o primeiro run

---

# 0. Fundação do projeto

## 0.1 Canon Knowledge Base ✅

- Separar `WORLD_TRUTH`, conhecimento/visibilidade dos atores e status epistemológico da pesquisa.
- Separar quando algo é verdadeiro, quando é revelado ao leitor e quando um ator passa a saber.
- Claims estruturados são distintos de evidence para RAG.
- Informações não verificadas permanecem abertas; fandom, hipótese ou inferência não entram silenciosamente como cânone.
- Status previstos incluem:
  - `CANON_VERIFIED`;
  - `CANON_INFERRED`;
  - `CANON_CONFLICTED/UNKNOWN`;
  - `SIMULATION_AUTHORED`;
  - `USER_AUTHORED` quando aplicável.
- Depois da divergência, o event log da simulação passa a ser autoridade sobre a história ocorrida.

## 0.2 Domínio físico ✅ / 🔄

Já estabelecido conceitualmente:

- `CapacityProfile`;
- `BodyState`;
- `ExertionIntent`;
- `PerformanceOutcome`;
- `ObservedPerformance`;
- `SelfPhysicalModel`.

Regras:

- feats restringem capacidade latente; não atribuem scores arbitrários;
- capacidade, habilidade, estado e disposição para agir são separados;
- fadiga, sono, recuperação, hidratação/nutrição, dor e lesões persistem causalmente;
- capacidade física pode ser um asset estratégico, mas disponibilidade não implica decisão de usá-la.

### Physical Simulation V1 🔄

Fluxo:

`/to-spec → /to-tickets → /implement → tests/evals → review → fix`

A V1 deve funcionar com personagens sintéticos e priors provisórios antes da calibração nominal completa.

## 0.3 Private Points Economy ✅ conceitualmente

Regras consolidadas em `PRIVATE_POINTS_ECONOMY.md`.

- PP são persistentes;
- renda mensal deriva dos Class Points aplicáveis no momento do pagamento;
- gastos cotidianos afetam liquidez estratégica futura;
- transferências são zero-sum entre participantes;
- saldo nunca fica negativo;
- necessidades básicas gratuitas/alternativas mínimas evitam dívida artificial;
- pagamentos estratégicos usam o saldo real acumulado pela história da simulação;
- o ledger deve ser replayable e auditável.

---

# 1. Fundação causal da simulação ⏳

Esta é a próxima fundação obrigatória depois do domínio físico. Nenhum domínio cognitivo/social deve depender de uma noção vaga de “cena”.

## 1.1 Simulation Clock

Representar tempo lógico e calendário escolar.

Requisitos:

- data e hora da simulação;
- ordenação causal de eventos;
- eventos simultâneos não recebem vantagem pela ordem de execução das chamadas LLM;
- checkpoints e retomada reproduzível;
- avanço de tempo compatível com calendário, aulas, exames, clubes, refeições, sono e eventos externos.

## 1.2 Event Model

Um `Event` é uma ocorrência temporalmente identificável capaz de alterar mundo, instituição, informação ou estado de ator.

Regras:

- evento não significa acontecimento público;
- `STATE != EVENT`;
- existência e observabilidade são separadas;
- eventos secretos continuam sendo world truth;
- eventos suportam replay, snapshots, auditoria, métricas e POV filtering;
- relação, crença, agenda e plano são estados; criação/revisão/execução deles pode gerar eventos.

## 1.3 Communication / Information Transfer

Comunicação não se reduz a fala.

Canais possíveis:

- conversa presencial;
- carta/bilhete;
- celular/mensagem;
- chamada;
- aviso oficial;
- mensagem anônima;
- gesto/sinal;
- rumor;
- terceiro intermediário;
- documento/objeto;
- ação deliberadamente observável.

Representar destinatário, observadores, interceptação, persistência, autenticidade, falsificação, atraso e evidência deixada.

Informação só entra no conhecimento de outro ator por cadeia causal válida de observação/comunicação.

## 1.4 Action / Affordance Model

O `ExamSpec` não define tudo que um estudante pode tentar.

Pipeline-base:

`AgentIntent → ActionProposal → Affordance/Resource Check → Rule Check → Execution → Event(s) → Consequences`

Distinguir:

- possibilidade física;
- posse/acesso a recursos;
- legalidade/regra institucional;
- ação proibida mas fisicamente possível;
- detecção;
- consequência física;
- consequência acadêmica;
- consequência social;
- consequência disciplinar/legal quando aplicável.

Objetivo: permitir estratégias compostas sem hardcodar jogadas canônicas.

## 1.5 Consequence Resolver

Ação grave não recebe consequência fixa simplista.

Avaliar:

- observação/detecção;
- força da evidência;
- regra escolar violada;
- dano produzido;
- denúncia/comunicação;
- resposta institucional;
- fallout social;
- efeitos relacionais;
- encaminhamento externo quando causalmente aplicável.

---

# 2. Character / Cognition Model ⏳

Transformar personagens em agentes persistentes, não prompts de interpretação.

## 2.1 Character Core

Representar:

- identidade;
- tendências relativamente estáveis;
- capacidades;
- preferências;
- valores/inibições;
- objetivos e prioridades;
- tolerância a risco;
- conhecimento inicial;
- autoimagem e percepção de terceiros.

Capacidade e motivação são separadas.

> **Capability does not imply manifestation.**

Ayanokōji, Yuuichi, Kōenji ou qualquer outro personagem podem possuir enorme capacidade e nunca encontrar razão suficiente para demonstrá-la.

## 2.2 Motivations + Behavioral Triggers

Motivações não podem ser apenas “chegar à Classe A”.

Representar objetivos específicos e gatilhos contextuais capazes de alterar a política de ação.

Exemplo conceitual:

`trusted_person_under_credible_attack → increase strategic engagement / decrease passivity / expand acceptable tactics`

O gatilho não define uma estratégia pronta; apenas altera prioridades, limites, disposição e orçamento de planejamento.

Targets de proteção devem derivar do estado real das relações, não de listas roteirizadas.

## 2.3 Beliefs, Hypotheses and Episodic Memory

Cada ator mantém:

- fatos conhecidos;
- crenças;
- confiança;
- proveniência;
- hipóteses;
- memórias episódicas;
- planos e intenções.

Inteligência nunca concede informação secreta.

## 2.4 Cross-canon characters ⏳

Contrato universal de importação:

`canon start → knowledge horizon → actual history → school-visible dossier → capability evidence → feats`

Exemplo estrutural para Yuuichi:

- estado mental/memórias: capítulo inicial de *Tomodachi Game*;
- evidência de capacidade: pode usar feats posteriores apenas quando demonstrarem capacidade plausivelmente preexistente;
- feitos futuros nunca viram memória retroativa;
- aprendizado genuinamente adquirido depois do ponto inicial não é retroalimentado.

A classificação escolar usa apenas o dossier que a escola poderia conhecer, não world truth oculto.

## 2.5 Hyuzaki ⏳

Personagem baseado no usuário.

- adaptar biografia para elegibilidade/realidade japonesa da ANHS;
- separar `REAL_USER_EVIDENCE` de `SIMULATION_ADAPTATION`;
- usar as mesmas interfaces dos demais personagens;
- pensamentos e estratégias privados podem ser vistos pelo Observatory sem vazar para outros atores.

---

# 3. Strategic Intelligence ⏳ 🧪

Uma das frentes mais críticas do projeto.

Estratégia não deve vir de uma única pergunta “o que você faria?”.

## 3.1 Strategic Asset Scan

Antes de gerar planos, o personagem inventaria os assets que acredita possuir:

- informação;
- aliados;
- relações;
- autoridade/status;
- PP/recursos;
- Class Points e recursos coletivos quando acessíveis;
- força/capacidade física;
- reputação/intimidação;
- deception;
- regras institucionais;
- loopholes percebidos;
- tempo;
- vulnerabilidades percebidas dos oponentes.

Para cada asset:

`Can I use it? → Should I use it? → What happens if I use it?`

## 3.2 Planning pipeline

Pipeline esperado:

1. definir objetivo;
2. identificar constraints;
3. inventariar assets;
4. modelar atores relevantes;
5. gerar candidatos;
6. buscar combinações;
7. simular reações prováveis;
8. buscar counterplay;
9. procurar falhas ocultas;
10. avaliar custo/risco;
11. comparar com não agir;
12. revisar melhores candidatos;
13. commit da ação/intenção.

## 3.3 Planning Budget

Profundidade deve depender de personagem **e** situação.

Parâmetros possíveis:

- breadth de candidatos;
- depth de rollout;
- opponent modeling;
- counterfactual depth;
- rule exploitation search;
- information valuation;
- revision passes.

Personagens excepcionais recebem maior orçamento quando a situação justifica. Isso não significa fornecer informação adicional.

## 3.4 Critic without intelligence leakage

O crítico pode encontrar falhas, contradições, riscos e counterplay.

Ele **não pode fornecer gratuitamente uma solução que o personagem não teria capacidade de conceber**.

Fluxo correto:

`character plan → critic exposes weakness → character replans within own capability`

Não:

`mediocre plan → super-critic invents genius plan → character executes borrowed intelligence`

## 3.5 Strategic evals 🧪

Construir benchmarks com situações canônicas removendo a solução canônica.

Avaliar se o agente:

- encontra soluções fortes;
- preserva estilo;
- não recebe conhecimento proibido;
- não copia roteiro futuro;
- mantém qualidade sob novos problemas;
- diferencia personagens estrategicamente.

Não exigir reprodução exata do plano da novel; exigir qualidade compatível com evidência disponível da capacidade.

## 3.6 Severe / unethical strategies

Não usar moralidade como bloqueio binário, pois destruiria a caracterização de diversos atores.

Separar:

- ação concebível no mundo;
- capacidade do personagem de aceitá-la;
- risco e consequências;
- nível de detalhe permitido ao planner.

Ações graves podem ser tratadas estrategicamente/abstratamente quando coerentes com o personagem, sem transformar o sistema em otimizador operacional de abuso, tortura, violência extrema ou material íntimo não consentido.

Consequências escolares, sociais e legais devem ser causalmente modeladas quando aplicáveis.

---

# 4. Social + Affective + Relationships ⏳

Relações são direcionais, históricas e persistentes.

Dimensões candidatas:

- confiança;
- respeito;
- medo;
- admiração;
- ressentimento;
- familiaridade;
- dependência;
- suspeita;
- proximidade emocional;
- percepção de competência;
- atração/interesse quando aplicável.

## 4.1 Relationship Events

Estado de relação deriva de acontecimentos reais.

Exemplos:

- estudaram juntos;
- alguém protegeu outro;
- mentira descoberta;
- ajuda em exame;
- humilhação pública;
- segredo compartilhado;
- dívida criada/paga;
- conflito repetido.

O sistema deve preservar histórico suficiente para explicar por que uma relação chegou ao estado atual.

Romance é consequência possível do domínio social, não tabela de casais canônicos.

---

# 5. Vida escolar e instituição ⏳

## 5.1 School Calendar

Representar:

- aulas;
- dias letivos;
- feriados;
- refeições;
- dormitórios;
- lojas/serviços;
- períodos de estudo;
- eventos institucionais;
- exames;
- reuniões escolares relevantes;
- entrada/saída de gerações.

Calendário é infraestrutura causal, não decoração.

## 5.2 Clubs ⏳

Clubes entram já no primeiro run em versão simples.

Representar:

- identidade do clube;
- horários;
- requisitos;
- advisor quando aplicável;
- membros;
- processo de entrada/saída;
- presença/ausência;
- eventos de relacionamento;
- treino e efeitos físicos/skill quando aplicáveis;
- custos de tempo e oportunidade.

Um personagem pode descobrir, considerar, entrar, frequentar, faltar ou abandonar um clube por decisão própria.

## 5.3 OAA ⏳

Tratar como mudança institucional ligada ao início do segundo ano, não como criação de Tsukishiro.

A proveniência temporal exata e dependência de Nagumo devem ser modeladas explicitamente. Não confundir “proposta de Nagumo” com “evento causado pelo enredo de expulsão de Kiyotaka”.

## 5.4 Protection Points ❓

Não hardcodar por data.

A introdução deve ser acionada por condição institucional relacionada à ausência anormal de expulsões.

Regra de simulação a formalizar:

- se uma geração alcançar a condição institucional correspondente sem expulsões, a escola pode introduzir o sistema;
- uma vez introduzido como regra escolar, alunos de outras grades também podem ser afetados quando a regra aplicável assim determinar;
- marcar explicitamente a generalização além do caso canônico como interpretação/simulation rule, não como fato provado pelo cânone.

## 5.5 Student IDs + unresolved mysteries ⏳ / ❓

Todo estudante deve possuir ID escolar.

Preservar pares canonicamente iguais quando verificados.

A regra por trás de IDs duplicados permanece `UNKNOWN` enquanto não houver evidência suficiente.

---

# 6. Hypothesis Engine ⏳

Mistérios e regras não resolvidos não devem receber verdade arbitrária porque um personagem escolheu uma hipótese.

Pipeline:

`facts/observations → hypothesis generation → evidence testing → consistency/coverage/contradictions/predictive power/parsimony → supported set`

Separar:

- `actor_belief`;
- hipóteses avaliadas pelo Engine;
- world truth selecionado posteriormente.

Se várias hipóteses permanecerem compatíveis:

`UNDERDETERMINED`

O Engine pode apresentá-las ao usuário. Uma hipótese escolhida para a simulação recebe provenance explícita como `SIMULATION_AUTHORED`/`SIMULATION_ASSUMPTION`.

Personagens não descobrem automaticamente essa verdade só porque ela foi aprovada fora do mundo.

---

# 7. Special Exams + Institutional Exam Design ⏳ 🧪

## 7.1 ExamSpec

Exames canônicos devem virar regras executáveis antes de receber agentes LLM.

Estrutura mínima:

- objetivo;
- participantes;
- agrupamento;
- recursos;
- informação pública/privada;
- pontuação;
- CP/PP/recompensas;
- penalidades;
- expulsão quando aplicável;
- comunicação;
- regras de empate;
- condições terminais;
- regras institucionais aplicáveis.

## 7.2 Exam provenance

Tipos:

- `CANON_SCHEDULED`;
- `CANON_ADAPTED`;
- `SIMULATION_GENERATED`;
- `USER_SUGGESTED`;
- `USER_INTERVENTION` quando o usuário força sua ocorrência.

A escola deve continuar capaz de gerar exames mesmo se Tsukishiro nunca entrar na timeline.

## 7.3 User exam suggestions ⏳

O usuário pode fornecer apenas um intent, por exemplo:

> “Crie um exame envolvendo artes marciais.”

Isso **não é ainda um exame válido**.

Pipeline:

`USER IDEA → Exam Generator → Formal Validator → Adversarial Tester → Simulation Sandbox → USER APPROVAL → eligible ExamSpec`

## 7.4 Formal Validator

Verificar deterministicamente:

- fechamento matemático;
- estados impossíveis;
- regras contraditórias;
- empates;
- transações CP/PP;
- penalidades/recompensas;
- condições de saída;
- caminhos sem resolução.

## 7.5 Adversarial Tester

Procurar:

- estratégia dominante trivial;
- exploit infinito;
- coalizão que torna resultado inevitável;
- incentivo universal a não participar;
- vantagem estrutural absurda;
- expulsão inevitável sem resposta;
- loop de pontuação;
- loophole contornando penalidade.

## 7.6 Synthetic sandbox 🧪

Rodar perfis artificiais antes dos personagens reais:

- forte acadêmico/fraco físico;
- forte físico/fraco estratégico;
- equilibrado;
- cooperativo;
- conflitivo;
- altamente estratégico.

Objetivo: garantir que comportamento interessante não esteja mascarando regras ruins.

---

# 8. Population Model + outras grades ⏳

## 8.1 Variable resolution population

`exists in world != full LLM inference`

Níveis:

- full agent;
- lightweight named actor;
- lightweight student;
- class/cohort aggregate.

Promoção de resolução ocorre antes de um resultado importante ser finalizado quando o ator se torna causalmente relevante.

Nunca:

`unexpected result → invent genius background retroactively`

Correto:

`latent profile already exists → causal concentration detected → promote actor → resolve in higher detail → commit result`

## 8.2 Cohort Generator

Novas gerações recebem perfis latentes fixados no nascimento da coorte:

- capacidades multidimensionais;
- tendências;
- school-visible dossier;
- possíveis hooks latentes;
- seed reproduzível.

Excepcionalidade é decidida pela geração da coorte/distribuições, não pelo Background Resolver depois de um resultado.

Detalhes biográficos podem ser expandidos posteriormente, mas devem permanecer compatíveis com todos os fatos já fixados.

## 8.3 Year 2 / Year 3 background simulation

Outras grades devem acumular história sem exigir catálogo completo de exames.

Background resolver pode registrar:

- ranking de exames;
- class-point deltas;
- vitórias/derrotas;
- dominance/leadership effects;
- causal factors agregados.

Não inventar retrospectivamente estratégias detalhadas que nunca foram simuladas.

Quando um exame envolver várias grades ou causar impacto grande, promover para resolução `ACTIVE`.

---

# 9. Relevance Scheduler + computational scaling ⏳

Importância narrativa não decide se algo aconteceu. Decide **quanta resolução computacional e observabilidade** será dada ao que aconteceu.

Níveis candidatos:

- `BACKGROUND`;
- `SUMMARY`;
- `ACTIVE`;
- `SPOTLIGHT`.

Fatores de relevância:

- focal actor involvement;
- causal impact;
- institutional impact;
- relationship impact;
- faction impact;
- strategic novelty;
- future dependency.

A ativação deve ser event-driven.

Personagens dormindo, assistindo aula comum ou sem evento relevante não executam planejamento profundo.

## 9.1 Simultaneous cognition

Atores que agem no mesmo quantum recebem o mesmo snapshot de mundo.

Pipeline:

`WORLD t → independent intents → deterministic collision/resolution → WORLD t+1`

A ordem de conclusão das chamadas LLM não pode criar vantagem temporal artificial.

---

# 10. Agendas, facções e atores externos ⏳

Estrutura:

`Actor/Faction → Agenda → Objectives → Strategy/Plan → Operation → Actions → Events`

Requisitos:

- agendas individuais podem divergir das facções;
- objetivos carregam proveniência epistemológica;
- White Room não é reduzida a “expulsar Kiyotaka”;
- Tsukishiro não deve spawnar automaticamente por calendário se a cadeia causal não existir;
- família Kōenji, Kijima e demais forças externas devem ser modeladas conforme cânone/inferência distinguível;
- uma facção relacionada a *Tomodachi Game* pode existir no crossover apenas com premissas explicitamente definidas;
- atores externos permanecem baratos/inativos até um gate causal permitir ação.

Eventos escolares como reuniões, visitas e outros access windows podem funcionar como activation gates para pais/facções.

---

# 11. Simulation Observatory ⏳

Camada somente de observação. Nunca altera conhecimento dos agentes.

Modos:

- **Observer/Reader View** — world truth, eventos secretos, planos, crenças, relações, física e autoria causal;
- **Character POV** — somente o que aquele ator pode perceber/saber, além da própria cognição;
- **Public View** — acontecimentos públicos e estados agregados observáveis.

Registrar cognição de forma estruturada/auditável sem expor chain-of-thought bruto:

- objective;
- hypotheses;
- confidence;
- candidate-plan summaries;
- chosen intent;
- known constraints;
- rationale summary;
- causal result.

Estatísticas:

- performance acadêmica/estratégica/social/física;
- influência;
- confiança/medo;
- relações;
- alianças/conflitos;
- rumores;
- expulsões;
- PP/CP;
- contribuição real vs contribuição percebida.

---

# 12. Checkpoints, replay e fronteira da simulação ⏳

## 12.1 Checkpoints

Salvar:

- world state;
- event log;
- beliefs;
- memories;
- relationships;
- PP/CP;
- physical state relevante;
- agendas/operações;
- school state;
- configuration/seed;
- versões de modelos/prompts/regras necessárias à reprodutibilidade.

## 12.2 Pause/resume

Uma execução pode ser pausada e retomada sem normalizar estados ou reconstruir retrospectivamente o mundo.

## 12.3 Terminal checkpoint

A run principal termina quando a **coorte focal inicial se forma**.

O mundo não termina.

O `Terminal World Snapshot` preserva:

- resultado da coorte focal;
- expulsões;
- classes finais;
- relações;
- recursos;
- estado institucional;
- gerações ainda cursando a escola;
- conflitos/agendas ainda abertos.

Uma continuação posterior nasce como nova run com `parent_checkpoint` apontando para o estado terminal anterior.

---

# 13. Delivery Milestones

Estimativas abaixo são de ordem de grandeza e devem ser recalibradas após os primeiros vertical slices. Não são calendário contratual.

## 13.1 Prototype Run — ~3–5 semanas

Objetivo: provar o motor causal básico com poucos agentes.

Requer:

- Simulation Clock;
- Event Model;
- world truth / knowledge isolation;
- action/affordance básico;
- comunicação/observação;
- memória e decisão simples;
- snapshot/replay inicial;
- Physical Simulation V1 integrada em escala pequena.

Não precisa representar a ANHS completa.

## 13.2 First Real Run — alvo principal — ~8–12 semanas

Definição de pronto:

- world/event engine estável;
- knowledge isolation;
- Character Core;
- motivations + behavioral triggers;
- beliefs/hypotheses/memory;
- relationship events;
- Strategic Planner com asset scan, candidate generation, counterplay e critic sem intelligence leakage;
- Physical Simulation V1;
- PP/CP básicos;
- vida escolar/calendário mínimo;
- clubes básicos;
- 160 alunos de Y1 existentes, com centrais full e demais lightweight;
- pelo menos um Special Exam formalizado e validado;
- Formal Validator;
- deterministic Exam Resolver;
- checkpoints;
- replay;
- Observatory básico;
- seeded reproducibility;
- pause/resume.

Escopo recomendado da primeira execução: **um período curto de abril/maio do primeiro ano**, não três anos inteiros.

Critério qualitativo:

> Soltar uma pequena ANHS, avançar dias, observar personagens lembrando, criando relações, gastando recursos, perseguindo objetivos, formulando estratégias e entrando em um exame cujo resultado não foi previamente escrito.

## 13.3 Year-1 Capable — ~3–5 meses

Depois do First Real Run:

- maior cobertura do calendário;
- múltiplos exames;
- clubes mais ricos;
- background simulation das outras grades;
- personagens institucionais importantes;
- facções básicas;
- maior cobertura de regras/economia;
- exam generation/adaptation;
- relevance scheduler mais maduro;
- calibração estratégica extensa.

## 13.4 Three-Year Capable — ~5–8+ meses

- transição de gerações;
- Cohort Generator;
- Year 2/Year 3 completos;
- OAA;
- Protection Points quando causalmente ativados;
- atores/facções externas maduras;
- pais e access windows;
- terminal graduation checkpoint;
- continuidade por child runs;
- exames entre grades quando aplicável.

---

# 14. Não bloqueia o First Real Run 🚫

Não esperar implementar estes itens para começar a experiência:

- geração seguinte completa de 160 alunos;
- catálogo de exames de Y2/Y3;
- Nagumo/Manabu plenamente ativos em toda a rotina;
- Tsukishiro;
- White Room operacional completa;
- família Kōenji completa;
- facção *Tomodachi Game*;
- OAA;
- Protection Points;
- resolução do mistério dos IDs;
- Hypothesis Engine completo;
- Exam Generator irrestrito;
- user exam suggestions em UI final;
- graduation;
- torneios/intercâmbio entre escolas.

Esses subsistemas devem encaixar nas interfaces fundamentais sem serem pré-requisitos para validar a primeira sociedade funcional.

---

# 15. Expansões futuras 🚫

## 15.1 External Institutions / Inter-School Events

Possibilidade futura de outras Advanced Nurturing High Schools ou instituições equivalentes participarem de:

- torneios;
- intercâmbios;
- exames compartilhados;
- cooperação/competição institucional;
- circulação temporária de estudantes;
- reputação entre escolas.

Isso exigiria novos domínios de acesso, população externa, calendário, viagens, informação e regras institucionais. Portanto não deve entrar no caminho crítico do primeiro run.

---

# 16. Frente paralela permanente: verificação do cânone

A pesquisa canônica continua em paralelo e alimenta progressivamente:

- regras institucionais;
- cronologia;
- personagens/backgrounds;
- feats físicos e cognitivos;
- relações iniciais;
- conhecimento inicial;
- facções/agendas;
- eventos/exames;
- operações ocultas;
- calendário;
- clubes;
- IDs e mistérios.

Quando a light novel não fornecer resposta suficiente, o sistema pode manter `UNKNOWN`, gerar hipóteses ou exigir uma premissa própria. Se uma verdade interna precisar ser escolhida, registrar explicitamente como `SIMULATION_AUTHORED`/`SIMULATION_ASSUMPTION` e preservar a incerteza canônica na documentação.

---

# Princípio de encerramento

O maior salto do projeto é:

`zero → primeira sociedade funcional`

Depois que `World Engine + Characters + Strategic Cognition + Relationships + Institution + Exam Engine` estiverem integrados, a maior parte das novas ideias deve entrar como extensão das mesmas interfaces, não como motores paralelos.

O primeiro run não precisa provar que a simulação consegue viver três anos.

Ele precisa provar que, durante algumas semanas, **o mundo continua existindo e produzindo consequências mesmo quando nenhum de nós já sabe qual será a próxima cena**.
