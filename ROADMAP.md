# Roadmap — COTE AI Simulation

Este roadmap organiza as frentes de desenvolvimento sem transformar acontecimentos futuros do cânone em roteiro obrigatório. O princípio geral é: **preservar causas, regras, atores e pressões canônicas quando sustentadas; não preservar resultados que deixaram de ser causalmente necessários depois da divergência.**

## Legenda

- ✅ estabelecido/modelado
- 🔄 frente atual
- ⏳ planejado
- ❓ depende de cânone/decisão futura

---

## 0. Fundação do projeto

### Canon Knowledge Base ✅

- Separação entre verdade do mundo, visibilidade/knowledge e status epistemológico da nossa pesquisa.
- Temporalidade separa quando algo é verdadeiro, quando é revelado ao leitor e quando um ator passa a saber.
- Claims estruturados são distintos de evidence para RAG.
- Informações não verificadas permanecem explicitamente abertas; o engine não deve tratar hipótese de fandom/modelo como cânone.
- Depois da divergência, o event log da simulação determina o que os personagens sabem.

### Domínio físico ✅

- `CapacityProfile`, `BodyState`, `ExertionIntent`, `PerformanceOutcome`, `ObservedPerformance` e `SelfPhysicalModel` são conceitos distintos.
- Feats restringem capacidade latente; não atribuem scores arbitrários.
- Capacidade, habilidade, estado e disposição para agir são separados.
- Fadiga, sono, recuperação, hidratação/nutrição, dor, lesões e combate devem persistir causalmente entre eventos.
- Corpus de evidência física é compartilhado entre personagens; não existe um RAG físico por personagem.

### Physical Simulation V1 🔄

Fluxo previsto:

`/to-spec` → `/to-tickets` → `/implement` → testes/evals/review.

A V1 deve funcionar com personagens sintéticos e priors provisórios mesmo antes de perfis canônicos nominais estarem completamente calibrados.

Lacunas de sourcing e open questions permanecem abertas quando não bloqueiam a mecânica da V1.

---

## 1. Fundação causal da simulação ⏳

### 1.1 Event Model

Definir formalmente `Event` antes dos domínios cognitivo/social/exames.

Um evento é uma ocorrência temporalmente identificável que pode alterar estado do mundo, estado institucional, informação disponível ou estado de um ator e gerar consequências causais posteriores.

Regras esperadas:

- **evento não significa acontecimento público**;
- existência do evento e visibilidade do evento são dimensões separadas;
- `STATE != EVENT`: uma agenda, crença ou relação é estado; sua criação/revisão pode ser evento;
- eventos devem suportar replay, snapshots, auditoria, estatísticas e POV filtering;
- eventos privados/secretos continuam sendo world truth mesmo quando nenhum aluno os conhece.

Categorias candidatas incluem ações físicas, decisões institucionais, comunicações, observações, revisões de crença, mudanças de relação, ações de agenda/operação e eventos de exame.

### 1.2 Communication / Information Transfer

Comunicação não pode ser reduzida a fala.

Canais devem poder incluir, entre outros:

- fala presencial;
- carta ou bilhete;
- mensagem de celular;
- chamada;
- documento/aviso oficial;
- mensagem anônima;
- gesto/sinal;
- comunicação mediada por terceiro;
- rumor;
- objeto ou artefato deixado deliberadamente;
- ação realizada para ser observada.

O modelo deverá representar propriedades como destinatário, observadores possíveis, interceptação, persistência, autenticidade, falsificação, atraso e evidência deixada pelo canal.

A informação só entra no conhecimento de outro agente por uma cadeia causal de comunicação/observação.

### 1.3 Action / Affordance Model

O `ExamSpec` **não** será a lista completa de ações que um aluno pode tentar.

Agentes devem poder compor estratégias a partir de ações/affordances do mundo, inclusive ações não previstas pelo exame ou proibidas pelas regras, quando forem fisicamente e causalmente possíveis.

Pipeline esperado:

`AgentIntent → ActionProposal → Affordance/Resource Check → Rule Check → Execution → Event(s) → Consequences`

O resolvedor deve distinguir:

- possibilidade física;
- posse/acesso a recursos;
- legalidade/regra institucional;
- possibilidade de tentar uma ação proibida;
- detecção/observabilidade;
- consequências físicas, acadêmicas, sociais, disciplinares e outras aplicáveis.

Objetivo: evitar agentes estrategicamente rasos limitados a menus como `talk/observe/form_alliance/answer_exam`, sem hardcodar “jogadas inteligentes” específicas do cânone.

### 1.4 Simulation Observatory

Criar uma camada de observação para o usuário que **nunca altera o conhecimento dos agentes**.

Modos desejados:

- **Observer/Reader View:** pode inspecionar world truth, eventos secretos, estados privados, planos, crenças, relações, estado físico e autoria causal dos resultados;
- **Character POV:** mostra apenas o que o personagem pode perceber/saber, além de sua própria cognição privada;
- **Public View:** mostra acontecimentos públicos e reações agregadas das classes, professores, conselho estudantil e staff.

O observatório deve registrar cognição/estratégia em formato estruturado e auditável (objetivos, hipóteses, confiança, intenções, planos, rationale summary etc.), sem transformar esse conteúdo em conhecimento universal.

Estatísticas planejadas:

- por personagem: acadêmico, estratégico, social, físico, institucional, crenças, informação, contribuições, relações e performance;
- gerais: ranking das classes, influência, confiança, medo, alianças, conflitos, rumores, expulsões, lesões, fadiga, transferências de pontos e outros agregados;
- **actual vs perceived**, inclusive contribuição real vs contribuição publicamente atribuída;
- public reaction derivada dos estados reais dos agentes, não de uma “mente coletiva” narrada.

---

## 2. Agendas, facções e operações ocultas ⏳

Modelar atores externos/institucionais sem reduzir toda a lore da White Room a um único objetivo.

Estrutura conceitual:

`Actor/Faction → Agenda → Objectives → Strategy/Plan → Operation → Actions → Events`

### Requisitos

- facções/atores distintos podem ter objetivos distintos ou conflitantes;
- interesses individuais podem divergir dos interesses da facção;
- observar, testar, manipular, expulsar, recuperar, controlar ou expor alguém são papéis/objetivos diferentes e não devem ser fundidos automaticamente;
- objetivos/motivações devem carregar proveniência epistemológica própria:
  - `CANON_VERIFIED`;
  - `CANON_INFERRED`;
  - `CANON_CONFLICTED/UNKNOWN`;
  - `SIMULATION_AUTHORED` quando o cânone ainda não resolve a questão e a simulação precisa escolher uma verdade interna.

### Questões canônicas a investigar, não assumir

- objetivos da facção/estrutura ligada a Atsuomi;
- objetivos e interesses ligados a Kijima;
- natureza real da atuação de Tsukishiro e se “expulsar Kiyotaka” descreve de fato seu objetivo final;
- hipótese de que determinadas pressões foram desenhadas para obrigar Kiyotaka a agir/demonstrar capacidade;
- origem e papel do professor/instrutor associado a Nanase;
- existência e função de alunos colocados na ANHS principalmente para observar Ayanokōji, incluindo material recente associado a Shiraishi.

Esses itens são **perguntas de modelagem/cânone**, não fatos já aprovados do simulador.

### Tsukishiro

Não hardcodar `Y2_START → spawn Tsukishiro` nem `Tsukishiro.goal = expel_Kiyotaka` sem sustentação.

A entrada/intervenção deve decorrer de agendas e operações que continuem causalmente válidas na timeline simulada.

Se Kiyotaka já tiver sido expulso ou as circunstâncias mudarem radicalmente, operações planejadas podem ser canceladas, modificadas ou substituídas.

---

## 3. Character / Cognition Model ⏳

Transformar personagens em agentes persistentes, e não prompts interpretativos.

Abranger:

- identidade/core relativamente estável;
- objetivos e prioridades;
- crenças com confiança e proveniência;
- conhecimento inicial;
- memória episódica da simulação;
- hipóteses/inferências;
- planos e intenções;
- percepção do próprio corpo;
- incerteza e possibilidade de erro;
- evolução ao longo do tempo.

### Base de personagem

Ao criar um personagem canônico, alimentar o sistema com o máximo de informação **verificada/estruturada disponível**, sem despejar tudo no prompt de cada cena.

Usar um corpus/evidence store compartilhado e recuperação por metadata/contexto. RAG é mecanismo de recuperação; não há necessidade de um banco vetorial independente por personagem.

No runtime, o contexto deve ser montado seletivamente a partir de core, objetivos, crenças, relações, memórias recentes/relevantes, estado físico, observações da cena e evidências comportamentais pertinentes.

### Hyuzaki

Hyuzaki é o personagem baseado no usuário.

- contexto biográfico deve ser adaptado de forma coerente à elegibilidade/realidade japonesa da ANHS;
- separar `REAL_USER_EVIDENCE` de `SIMULATION_ADAPTATION`;
- histórico, personalidade, cognição e capacidades devem ser modelados pelas mesmas interfaces dos demais personagens;
- suas crenças, estratégias e pensamentos privados devem ser observáveis pelo usuário via Observatory sem vazarem para outros agentes.

Personagens originais adicionais poderão ser importados com proveniência `USER_AUTHORED`.

---

## 4. Social + Affective + Romance ⏳

Relações devem ser direcionais e persistentes.

Possíveis dimensões incluem confiança, respeito, medo, admiração, ressentimento, familiaridade, dependência, suspeita, proximidade emocional e percepção de competência.

Romance é uma consequência possível do domínio social/afetivo, não uma tabela de casais canônicos.

Separar pelo menos:

- atração;
- interesse romântico;
- apego;
- ciúme;
- disposição para perseguir uma relação;
- decisão de revelar/ocultar interesse;
- reciprocidade.

Casais do cânone não são resultados obrigatórios depois da divergência.

---

## 5. Strategic Intelligence ⏳

Inteligência estratégica deve ser separada de personalidade, motivação e conhecimento disponível.

Abranger, entre outros:

- geração e comparação de hipóteses;
- inferência sob informação parcial;
- planejamento;
- raciocínio adversarial;
- avaliação de risco;
- valoração/aquisição de informação;
- detecção e produção de engano;
- adaptação estratégica;
- decisão de ocultar ou revelar capacidade.

**Inteligência nunca concede informação secreta.** Um personagem mais capaz processa melhor evidência disponível; não recebe world truth privilegiada.

Estratégias complexas devem emergir da composição de affordances e recursos do mundo, inclusive quando envolvem ações fora do fluxo normal de um exame.

---

## 6. Special Exams + Institutional Exam Design ⏳

### ExamSpec

Exames canônicos devem ser formalizados como regras executáveis/determinísticas antes de receber agentes LLM.

Validar com puppet/scripted agents para impedir que comportamento estratégico mascare regras quebradas.

### Escola vs interferência externa

Separar o programa institucional de exames da ANHS de interferências externas.

A escola deve continuar capaz de produzir exames mesmo se Tsukishiro não aparecer ou se a timeline divergir.

Origens conceituais de exame:

- `CANON_SCHEDULED`: institucionalmente preservado e ainda causalmente aplicável;
- `CANON_ADAPTED`: objetivo institucional permanece, mas condições exigem regras diferentes;
- `SIMULATION_GENERATED`: novo exame construído a partir da filosofia/objetivos/restrições da ANHS quando o cânone não oferece um exame causalmente aplicável.

Interferência externa deve modificar/propor condições sobre um processo institucional; não ser a única razão para o exame existir.

Exames gerados precisam de estrutura formal (objetivo, participantes, recursos, informação, cooperação/competição, riscos, recompensas, penalidades, tempo, comunicação, segredos e condições terminais) e validação determinística.

### Exames originalmente associados a tentativas de expulsar Kiyotaka ❓

Investigar individualmente o terceiro trimestre do primeiro ano até o exame da ilha do segundo ano para separar:

- objetivo institucional normal da ANHS;
- influência/interferência externa comprovada;
- objetivo atribuído a Tsukishiro/Atsuomi e seu grau de certeza;
- regras que ainda fariam sentido sem Kiyotaka/Tsukishiro;
- regras que precisariam ser adaptadas ou substituídas na timeline simulada.

---

## 7. Divergência temporal, Year 2 e Year 3 ⏳

### Princípio

**Preservar causas/pressões canônicas; não forçar consequências canônicas.**

Distinguir futuramente:

- fatos já estabelecidos antes da divergência;
- eventos institucionais previamente agendados;
- agendas/pressões persistentes;
- operações já iniciadas;
- resultados canônicos não obrigatórios.

### White Roomers no segundo ano

A lore prévia continua verdadeira quando verificada, mas entrada, missão e comportamento na ANHS devem depender de operações causalmente válidas.

Se for desejável garantir a presença de certos personagens independentemente da divergência, isso deve existir como **Scenario Constraint explícita**, não como falsa emergência espontânea.

### Potencial Kiyotaka × Kōenji no terceiro ano

A disputa deve ser uma possibilidade emergente, não um checkpoint obrigatório do roteiro.

- se um deles for expulso antes, a disputa pode simplesmente não existir;
- se ambos estiverem presentes, ainda precisa haver motivo/condição causal para confronto;
- Kōenji não recebe `if expulsion_risk → full_power` hardcoded;
- sua ação deve decorrer de preferências, objetivos, percepção de risco, custo, interesse e capacidade.

O mesmo vale para Kiyotaka: capacidade disponível não implica decisão de demonstrá-la.

---

## 8. Integrated Simulation Milestones ⏳

### Primeiro marco integrado

Executar um período curto do início do primeiro ano com poucos agentes relevantes, demonstrando:

- world truth determinístico;
- event log e logical time;
- conhecimento isolado;
- estado físico persistente;
- ações/affordances;
- cognição e decisão;
- comunicação multicanal;
- relações persistentes;
- Observatory/POV/Public View;
- métricas básicas por personagem e gerais.

### Primeiro Special Exam integrado

Executar um exame formalizado com:

- regras determinísticas;
- conhecimento secreto isolado;
- agentes estratégicos;
- ações não limitadas ao menu do exame;
- física/social/cognição integradas;
- replay e causal trace;
- avaliação de contribuição real vs percebida.

### Escala

Depois da prova do vertical slice:

- personagens estratégicos centrais como agentes completos;
- secundários em representação simplificada;
- NPCs ordinários por priors/estado agregado;
- promoção de NPC para agente mais completo quando ganhar relevância causal.

### Transição de ano

Ao final do primeiro ano, snapshot/pause deve preservar estado necessário para continuação: eventos, crenças, memórias, relações, recursos, class points, expulsões, estado físico relevante, agendas/operações, configuração/seed e versões de modelos/prompts.

Só depois disso introduzir/ativar o conteúdo específico do segundo ano de acordo com a timeline resultante.

---

## Frente paralela permanente: verificação do cânone

A verificação do cânone continua em paralelo ao desenvolvimento.

Ela alimentará progressivamente:

- regras institucionais;
- cronologia;
- personagens e backgrounds;
- feats físicos;
- relações iniciais;
- conhecimento inicial;
- facções/agendas;
- eventos/exames;
- operações ocultas.

Quando a light novel não fornecer resposta suficiente, a simulação pode precisar escolher uma premissa própria. Essa decisão deve ser explicitamente marcada como `SIMULATION_AUTHORED` (ou equivalente), com a incerteza canônica preservada em documentação.