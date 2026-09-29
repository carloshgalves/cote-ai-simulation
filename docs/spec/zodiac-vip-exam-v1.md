# Spec — Zodiac/VIP Special Exam V1

**Status:** Proposed  
**Data:** 2026-09-29  
**Arquitetura de origem:** [Examination architecture](../architecture/exams.md)  
**Fundamentação:** [Pesquisa do primeiro exame](../research/first-special-exam-zodiac-vip.md)  
**Relacionados:** ADR 0001 · ADR 0003 · ADR 0005 · ADR 0007 · ADR 0008 · `PRIVATE_POINTS_ECONOMY.md`

Esta spec define o primeiro Special Exam executável do projeto. Ela define **regras institucionais do exame**; não redefine affordances físicas, comunicação causal, percepção, cognição, RAG ou memória.

---

## 1. Problema e objetivo visível

Precisamos provar, antes de colocar personagens canônicos ou LLMs no loop, que um exame com informação privada consegue responder de forma única:

> dado este estado inicial e esta sequência de ações válidas, qual é o resultado?

e:

> duas implementações conformes produzem o mesmo resultado, o mesmo ledger econômico e a mesma projeção pública?

O primeiro exame escolhido é o **Zodiac/VIP Cruise Ship Special Exam** do primeiro ano. A escolha é arquitetural: ele exige segredo, disclosure, comunicação, deception, inferência, cooperação/traição, múltiplos atores, deadlines e PP/CP sem exigir mapa, sobrevivência, clima, inventário físico ou resolução atlética.

A V1 deve rodar sem LLM. Policies sintéticas/scripted fornecem decisões; o resolver nunca pergunta a um modelo “quem venceu”.

## 2. Autoridade e fronteiras

### 2.1 Examination Engine possui

- `ExamSpec` e sua versão/hash;
- estado interno do exame derivável do event log;
- validação de ações próprias do exame;
- classificação de Outcome 1–4;
- cálculo determinístico dos deltas de PP/CP;
- disclosure **de regras e segredos criados pelo próprio exame**;
- projeção pública do resultado do exame.

### 2.2 Causal Simulation Foundation possui

Conforme ADR 0008:
- relógio e `SimulationInstant`;
- admission fence / resolution cycle;
- `ActionProposal`, conflitos e commit;
- world state/event store;
- percepção e `Observation`;
- `Claim`, `Transmission` e `KnowledgeInput`;
- affordances externas ao menu do exame;
- ordem causal e replay.

O Examination Engine devolve candidatos/eventos. Ele **não grava** world state, clock, event store, belief state, telefone, localização ou comunicação diretamente.

### 2.3 Não pertencem a esta spec

- como um aluno decide mentir;
- como um aluno deduz o VIP;
- como uma conversa altera confiança;
- como uma mensagem chega ao destinatário;
- roubar/emprestar/coagir alguém a usar telefone;
- detectar intimidação;
- modelar punição física;
- montar prompt/RAG;
- memória e belief revision.

Esses fatos podem tornar uma `SubmitVipGuess` possível ou impossível via CSF, mas o ExamSpec não duplica o Action/Affordance Model.

## 3. Perfil canônico e perfil executável

A pesquisa detalhada das regras permanece `UNVERIFIED` sob ADR 0005 até leitura humana Tier 0–1 do Y1 V4. Portanto existem dois perfis conceituais:

- `CANON_VERIFIED`: reservado; não pode ser usado enquanto os CanonClaims necessários não compilarem.
- `CANON_ADAPTED`: executável agora a partir de `data/models/exams/zodiac-vip-v1.yaml`, `not_canon: true`; reproduz a reconstrução mais forte disponível e declara cada preenchimento de lacuna como `SIMULATION_AUTHORED`.

Nenhuma implementação pode renomear `CANON_ADAPTED` para “canon” apenas porque passa nos testes.

## 4. Exam identity e provenance

```text
exam_id             = y1.zodiac-vip
spec_version        = 0.1.0-provisional
model_profile       = CANON_ADAPTED
model_version       = 0.1.0-provisional
canonical_work      = ln.y1.v04 (dependency; registry entry not owned by this branch)
determinism_profile = csf-adr0008
```

Um run registra:
- `exam_spec_version`;
- hash da spec/model config;
- `exam_seed`;
- hash do roster e group assignments;
- hash do role assignment;
- política de identidade causal da CSF;
- versão dos adapters econômico/disclosure;
- status de proveniência usado (`CANON_ADAPTED`).

## 5. Participantes, elegibilidade e grupos

### 5.1 Elegibilidade

O perfil canônico-adaptado aceita somente estudantes ativos do primeiro ano declarados no `ExamSetupManifest`.

Invariantes de setup:
1. exatamente 159 participantes ativos;
2. exatamente quatro `class_id`;
3. cardinalidades de classe = multiconjunto `{39, 40, 40, 40}`;
4. exatamente 12 grupos;
5. tamanhos = nove grupos de 13 + três grupos de 14;
6. cada participante pertence a exatamente um grupo;
7. cada grupo contém participantes das quatro classes;
8. por grupo, cada classe contribui com 3 ou 4 participantes.

Os números são premissas `SIMULATION_AUTHORED` enquanto ZC-13 estiver `UNVERIFIED`.

### 5.2 Formação

O ExamSpec **não inventa um algoritmo de grouping**. `group_assignments` é input institucional do setup. O validator prova que é uma partição válida e balanceada; não decide como a escola chegou nela.

Os ids de grupo são os do zodíaco chinês em ordem normativa:

```text
01 RAT
02 OX
03 TIGER
04 RABBIT
05 DRAGON
06 SNAKE
07 HORSE
08 GOAT
09 MONKEY
10 ROOSTER
11 DOG
12 BOAR
```

Nomes/localização textual são apresentação; `ordinal` é parte do contrato.

## 6. Papel secreto e distribuição

### 6.1 Invariantes

- exatamente 1 VIP por grupo;
- exatamente 12 VIPs;
- exatamente 3 VIPs por classe;
- VIP deve ser participante ativo e membro do grupo;
- nenhuma pessoa é VIP em dois grupos (impossível pela partição).

### 6.2 `CANON_ADAPTED` — assignment rule

Enquanto ZC-15 não for verificado, a V1 adota explicitamente:

```text
ordered_members(group) =
  sort by (surname_kana_sort_key ASC, actor_id ASC)

vip(group) =
  ordered_members(group)[group.zodiac_ordinal - 1]
```

Depois de computar todos os 12, o validator exige exatamente 3 VIPs por classe. Se não fechar, o setup é inválido; o sistema **não** troca VIPs silenciosamente para “balancear”.

`surname_kana_sort_key` deve vir do roster institucional; o Examination Engine não romaniza nomes por heurística.

Esta regra é `SIMULATION_AUTHORED` baseada em reconstrução comunitária, não `VERIFIED`.

### 6.3 Aleatoriedade institucional

A escolha normal do VIP nesta V1 **não usa RNG**. O `exam_seed` existe para:
- desempatar submissões exatamente simultâneas (§10);
- smoke/synthetic fixtures que precisem de sorteio explicitamente declarado.

Nenhum RNG global é permitido.

## 7. Informação e disclosure

### 7.1 World truth

Engine-only:
- `vip_by_group`;
- `role_assignment_hash`;
- submissões recebidas;
- cota individual consumida;
- outcome terminal de cada grupo;
- fatos de auditoria, inclusive identidade do submitter terminal.

### 7.2 Informação pública inicial

A cada participante:
- regras públicas;
- seu `group_id`;
- roster do próprio grupo;
- agenda de meeting slots do próprio grupo;
- deadline/endpoint institucional de submissão;
- quatro outcomes e recompensas/penalidades.

A V1 **não** entrega globalmente os rosters dos outros onze grupos. Um agente só os aprende por um evento/claim causal externo.

### 7.3 Informação privada no `role_disclosure_at`

Cada participante recebe um `KnowledgeInput` endereçado:

```text
exam_id
group_id
is_vip: true | false
source = school.exam-disclosure
```

- o VIP aprende que é VIP;
- o não-VIP aprende apenas que ele próprio não é VIP;
- ninguém recebe `vip_actor_id` de outra pessoa;
- a regra oculta de seleção não é injetada em contexto de agente.

A geração/entrega do `KnowledgeInput` passa pelo mecanismo do ADR 0008. O Exam Engine fornece o payload autorizado; não escreve belief state.

### 7.4 Resultado público

Em `result_publish_at`:
- outcome por grupo;
- deltas de CP por classe;
- recompensas de PP endereçadas aos respectivos beneficiários.

A projeção pública padrão **não revela** VIP nem submitter terminal. O audit log autoritativo pode conter ambos e é access-controlled.

## 8. Tempo, fases e reuniões

A V1 não resolve a divergência de contagem “terceiro/quarto dia”. Usa tempos absolutos fornecidos pelo setup:

```text
SETUP
  -> ROLE_DISCLOSURE at role_disclosure_at
  -> ACTIVE          [role_disclosure_at, active_close_at)
  -> QUIET_GAP       [active_close_at, final_window_open)
  -> FINAL_WINDOW    [final_window_open, final_window_close)
  -> SETTLEMENT      at final_window_close
  -> RESULT_PUBLICATION at result_publish_at
  -> CLOSED
```

Regras temporais:
1. `role_disclosure_at < active_close_at`;
2. `active_close_at < final_window_open < final_window_close < result_publish_at`;
3. `final_window_open - active_close_at = 30min` no perfil V1;
4. `final_window_close - final_window_open = 30min`;
5. `result_publish_at - active_close_at = 120min`;
6. setup fornece exatamente 6 meeting slots de 60min por grupo;
7. meeting slots ocorrem antes de `active_close_at`;
8. a distribuição dos slots entre dias é config versionada, não regra reconstruída silenciosamente.

Meeting attendance é observável/auditável, mas nenhuma penalidade de CP/PP por ausência é inventada nesta V1. Se fonte Tier 0–1 trouxer penalidade, entra em versão posterior.

## 9. Ação própria do exame

Única ação V1:

```text
SubmitVipGuess {
  actor_id
  exam_id
  group_id
  guessed_actor_id
  submission_id
}
```

O tempo, cycle e provenance vêm do envelope causal da CSF, não do payload controlado pelo agente.

Conversar, mentir, compartilhar screenshot, revelar “sou VIP”, negociar, observar telefone ou tentar coagir alguém **não** são actions do ExamSpec.

## 10. Validação e prioridade de submissions

### 10.1 Camadas

**A. Envelope/schema invalid**
- id ausente/malformado;
- actor/exam inexistente;
- provenance causal inválida.

Resultado: `REJECTED_SCHEMA_OR_CAUSAL`; nenhum estado do exame muda e a cota individual não é consumida.

**B. School-received submission**
Uma mensagem bem-formada que alcançou o endpoint institucional consome a cota individual exatamente uma vez, mesmo se for depois invalidada por regra semântica. Esta escolha é `SIMULATION_AUTHORED`.

Depois da primeira `SchoolSubmissionReceived(actor_id)`, nova tentativa do ator é `REJECTED_ALREADY_SUBMITTED`.

### 10.2 Validações semânticas, nesta ordem

1. exam ainda aceita submissions;
2. grupo ainda não está terminal;
3. actor é participante ativo;
4. `group_id == actor.group_id`;
5. `guessed_actor_id` pertence ao grupo;
6. actor não é o VIP;
7. actor.class_id != vip.class_id;
8. fase é `ACTIVE` ou `FINAL_WINDOW`.

Falhas 3–8 em uma school-received submission geram `SubmissionInvalidated(reason)`, não outcome.

### 10.3 Early submission

Uma submission semanticamente válida em `ACTIVE` é terminal:
- guess == VIP → Outcome 3;
- guess != VIP → Outcome 4.

`QUIET_GAP` não aceita palpite na V1. Isso evita estender “early” para um intervalo cuja regra textual precisa de verificação. Se a leitura mostrar que o período 21:00–21:30 aceita Outcomes 3/4, isso vira mudança explícita de model_version.

### 10.4 Final submission

Em `FINAL_WINDOW`, a primeira submission semanticamente válida é terminal:
- guess == VIP → Outcome 1;
- guess != VIP → Outcome 2.

Se nenhuma submission válida existir até `final_window_close`, o settlement produz Outcome 2.

### 10.5 Simultaneidade

Ordem de chegada do transporte, ordem de conclusão de LLM e thread scheduling **não** podem decidir o resultado.

Para todas as submissions semanticamente elegíveis do mesmo `group_id`, mesma fase, mesmo `SimulationInstant` e mesmo `ResolutionCycle`:

```text
priority_key =
  H(causal_identity_policy,
    "zodiac-vip/submission-priority",
    exam_seed,
    group_id,
    submission_id)

winner = minimum(priority_key bytes)
```

Somente `winner` entra na regra terminal; as demais recebem `SUPERSEDED_BY_SIMULTANEOUS_SUBMISSION` e não geram recompensa. A cota delas já foi consumida porque chegaram ao endpoint.

Hash collision com componentes distintos é corrupção/fail-closed conforme ADR 0008.

Esta prioridade é `SIMULATION_AUTHORED` e deve ficar fora do contexto dos agentes; ela resolve um empate sem “vantagem por actor_id”.

## 11. Outcomes e scoring

Valores V1:

```text
PP_STANDARD = 500_000
PP_VIP_OUTCOME_1 = 1_000_000
CP_SWING = 50
```

### Outcome 1 — final correct

Precondição: primeira submission final válida identifica o VIP.

Efeitos:
- VIP: +1,000,000 PP;
- todo outro membro do grupo: +500,000 PP;
- CP: sem mudança;
- grupo → terminal `OUTCOME_1_SHARED_SUCCESS`.

### Outcome 2 — VIP survives final

Precondição:
- primeira submission final válida está errada; **ou**
- `final_window_close` chega sem submission final válida.

Efeitos:
- VIP: +500,000 PP;
- demais: 0 PP;
- CP: sem mudança;
- grupo → terminal `OUTCOME_2_VIP_HIDDEN`.

### Outcome 3 — early correct

Precondição: primeira submission early válida identifica o VIP.

Efeitos:
- submitter: +500,000 PP;
- submitter.class: +50 CP;
- VIP.class: -50 CP;
- grupo → terminal `OUTCOME_3_TRAITOR_CORRECT`.

### Outcome 4 — early wrong

Precondição: primeira submission early válida erra o VIP.

Efeitos:
- VIP: +500,000 PP;
- submitter.class: -50 CP;
- VIP.class: +50 CP;
- grupo → terminal `OUTCOME_4_TRAITOR_WRONG`.

### 11.1 PP ledger

Rewards são emissão institucional:
```text
category = strategic_spending? no
category = exam_reward
source = institution
beneficiary = student
amount > 0
cause = exam outcome id
```

Não são transferências entre alunos e nunca podem criar saldo negativo. Cada movimento carrega `exam_id`, `group_id`, `outcome_id`, `beneficiary`, `amount`, `sim_time`.

### 11.2 CP ledger

Outcome 3 e 4 geram sempre um par `+50/-50` entre classes **distintas**. A soma de CP do exame é zero.

O Examination Engine calcula a intenção de delta; a autoridade que aplica Class Points é dependência externa. Se a política institucional de CP recusar o delta, o ciclo falha fechado/aborta; não se aplica apenas metade.

## 12. Terminalidade, empate e ausência de ação

Por grupo, exatamente um outcome terminal.

Um grupo fecha quando:
- Outcome 3/4 acontece durante `ACTIVE`;
- Outcome 1/2 acontece em `FINAL_WINDOW`;
- settlement em `final_window_close` produz Outcome 2.

O exame global termina quando os 12 grupos estão terminais; a publicação ocorre em `result_publish_at`.

Não existe ranking global nem empate entre grupos nesta V1. Logo **não há tie-breaker de classificação**. O único empate operacional é simultaneidade de submissions, resolvido em §10.5.

Ausência universal de ação é válida e termina deterministicamente: todos os grupos chegam a Outcome 2 no settlement. Portanto inação não trava o sistema, embora possa ser estrategicamente atraente para VIPs.

## 13. Formal Validator

Uma implementação conforme expõe pelo menos:

```text
validate_spec(config) -> ValidationReport
validate_setup(config, setup) -> ValidationReport
validate_submission(state, proposal) -> SubmissionDisposition
settle_group(state, group_id, instant) -> CommitCandidate
validate_outcome(state, outcome) -> ValidationReport
audit_exam(run) -> AuditReport
```

### 13.1 Static/spec invariants

Falha antes do run se:
- versão/hash ausente;
- fases/deadlines não têm ordem total;
- simultaneous policy ausente;
- outcome não é mutuamente exclusivo/exaustivo;
- outcome não tem condição terminal;
- reward/penalty não fecha;
- qualquer outcome pode emitir PP negativo;
- Outcome 3/4 não conserva CP;
- outcome permite dois settlements para o mesmo grupo.

### 13.2 Setup invariants

Falha antes de disclosure se:
- participante duplicado/faltante;
- classes/grupos com cardinalidade inválida;
- roster não é partição;
- grupo não mistura as quatro classes;
- `surname_kana_sort_key` ausente;
- ordinal de grupo excede tamanho;
- assignment gera VIP fora do grupo;
- !=1 VIP/grupo;
- !=3 VIPs/classe;
- role assignment hash não recomputa.

### 13.3 Runtime invariants

- uma pessoa tem no máximo uma `SchoolSubmissionReceived`;
- grupo terminal rejeita novos effects;
- um grupo tem exatamente um `OutcomeCommitted`;
- um outcome tem uma única causa terminal;
- nenhuma invalid submission gera PP/CP;
- nenhum superceded simultaneous submission gera PP/CP;
- nenhum submitter same-class gera Outcome 3/4;
- result publication não vaza `vip_actor_id`/submitter;
- audit projection e public projection são tipos diferentes.

### 13.4 Economic closure

Por grupo:
- O1: `total_pp = (group_size + 1) * 500_000`;
- O2/O3/O4: `total_pp = 500_000`;
- O1/O2: `sum(cp_delta)=0` e todos deltas = 0;
- O3/O4: exatamente dois deltas de CP, `{+50,-50}`.

Global:
- PP nunca é debitado por esta spec;
- `sum(all cp_delta)=0`;
- no máximo 12 outcomes;
- CP positivo bruto total <= 600;
- CP negativo bruto absoluto <= 600;
- com 159 participantes, payout máximo ocorre se todos os grupos forem O1:
  `(159 + 12) * 500_000 = 85_500_000 PP`.

### 13.5 Reachability

Validator/model checker mínimo deve demonstrar para um grupo válido que existem trajetórias para O1, O2, O3 e O4, e que toda trajetória finita até `final_window_close` termina.

Estados proibidos:
- grupo `OPEN` depois do settlement;
- grupo com dois outcomes;
- outcome 3/4 cujo submitter e VIP são da mesma classe;
- outcome sem ledger correspondente;
- ledger sem outcome/cause;
- `CLOSED` com grupo não terminal.

## 14. Audit events

Nomes conceituais; o adapter real deve mapear para schemas versionados da CSF:

- `exam.setup.accepted`
- `exam.role.assigned` (secret world truth)
- `exam.role.disclosure.requested`
- `exam.meeting.scheduled`
- `exam.submission.received`
- `exam.submission.invalidated`
- `exam.submission.superseded`
- `exam.group.outcome.proposed`
- `exam.pp.reward.proposed`
- `exam.cp.delta.proposed`
- `exam.group.settled`
- `exam.results.publication.requested`
- `exam.closed`

O Examination Engine propõe; somente a CSF commita world truth.

Cada outcome auditável carrega:
- spec/model version + hashes;
- `exam_seed`;
- group;
- terminal cause id;
- VIP id (audit-secret);
- terminal submitter id ou null (audit-secret);
- guessed id ou null;
- outcome enum;
- PP/CP deltas;
- instant/cycle;
- provenance/status do model profile.

## 15. Contratos mínimos com outras trilhas

### 15.1 CSF

Necessário:
- registrar `SubmitVipGuess` como `ActionProposal`;
- fornecer snapshot/revision, instant e cycle;
- agrupar simultâneos sem arrival-order;
- commit atômico de outcome + ledger/lifecycle;
- agendar disclosure, settlement e publication;
- endereçar `KnowledgeInput`;
- transportar comunicação como `Claim/Transmission`.

A spec **não** altera ADR 0008.

### 15.2 Canon/RAG

Necessário depois:
- `ln.y1.v04` no source registry;
- claims verificados para regras;
- disclosure model;
- rosters/nomes/sort keys quando aplicável;
- zero recuperação de identidade de VIP a partir de “canon future” durante uma simulação divergente.

RAG nunca resolve um outcome.

### 15.3 Character Cognition

Port mínimo:
```text
DecisionContext {
  actor_id
  authorized_exam_rules
  authorized_exam_knowledge_inputs
  observations
  received_claims
  beliefs/memory supplied by cognition owner
}

DecisionOutput -> zero or more ActionProposal
```

LLM pode decidir **se** e **quem** chutar; não pode editar phase, VIP, outcome ou pontos.

### 15.4 Economy

Necessário:
- `award_pp(beneficiary, amount, cause)`;
- `apply_cp_deltas_atomically(deltas, cause)`;
- idempotência por `cause/outcome_id`;
- ledger auditável.

## 16. Knowledge-boundary tests

Obrigatórios antes de personagens reais:
1. não-VIP nunca recebe `vip_actor_id` no disclosure;
2. VIP recebe somente seu próprio papel;
3. actor de grupo X não recebe roster/segredo de Y sem causal event;
4. audit-secret não aparece na public projection;
5. receber claim “A é VIP” não transforma a proposição em world truth;
6. inferir corretamente o padrão não concede acesso mágico ao role mapping; é belief/derivation;
7. resultado final não retroativamente injeta identidade secreta se a regra pública preserva anonimato;
8. replay conserva os mesmos KnowledgeInput ids/recipients.

## 17. Adversarial/synthetic sandbox

Antes de qualquer personagem canônico, executar policies puras:

- `cooperative`: não envia early; se recebe evidência suficiente, envia candidato correto na final window;
- `selfish_vip`: sendo VIP, não revela voluntariamente; nunca pode submeter;
- `random`: escolhe membro permitido pseudoaleatoriamente por substream próprio;
- `ultra_conservative`: nunca envia early; na final, abstém se não atingir threshold;
- `simple_strategist`: agrega claims e tenta reconstruir o pattern sem acesso a world truth;
- `opportunistic_traitor`: envia early quando sua policy julga EV aceitável.

A policy recebe somente conhecimento autorizado. O harness injeta `SubmitVipGuess`; nunca chama o resolver por atalho.

Critérios:
- todos os outcomes aparecem em pelo menos um cenário;
- nenhuma policy consegue criar reward duplicado;
- inação global termina;
- random não acessa VIP;
- estrategista só acerta por dados entregues;
- same seed repete setup/tie-break;
- permutar ordem de entrega de proposals dentro do mesmo cycle não altera resultado.

Vetores normativos ficam em `data/models/exams/conformance/zodiac-vip-v1.yaml`.

## 18. Micro-exam de integração

`micro-secret-v1` é um smoke test, não um segundo produto:
- 8 atores sintéticos;
- 2 grupos de 4;
- um papel secreto por grupo;
- disclosure privado;
- uma janela de comunicação;
- uma única submission final por grupo;
- sem PP/CP;
- correto = `GROUP_SUCCESS`; errado/ausente = `GROUP_FAIL`.

Ele reutiliza as mesmas portas `KnowledgeInput -> Claim/Transmission -> ActionProposal -> validator -> outcome` e existe apenas para isolar problemas de integração antes do Zodiac completo.

## 19. Failure modes explícitos

- **Canon incompleto:** não promover para `CANON_VERIFIED`; perfil adaptado continua marcado.
- **Setup inválido:** fail before disclosure; nenhuma role é entregue.
- **Economy adapter rejeita settlement:** ciclo aborta atomically; não fecha grupo pela metade.
- **Disclosure delivery falha:** CSF mantém trabalho retomável; exam não “assume que o aluno sabe”.
- **LLM timeout/no proposal:** nenhum action; deadlines continuam; settlement resolve.
- **Replay hash mismatch:** corrupção; fail closed.
- **Duas implementações divergem num vetor de conformidade:** pelo menos uma é não conforme; não escolher pelo resultado “mais plausível”.

## 20. Decisões abertas / blockers para código

### D1 — verificação Tier 0–1
Owner: Trilha B / sessão humana de leitura. Bloqueia apenas `CANON_VERIFIED`, não a model spec provisória.

### D2 — stack do Examination Engine
ADR 0007 proíbe usar Python como precedente. Nenhuma linguagem/framework foi aceito para este subdomínio. **Bloqueia implementação de resolver/validator em `src/` nesta branch.**

### D3 — owner concreto de Class Points
A interface abstrata está suficiente para a spec, mas o adapter concreto precisa do bounded context/ledger que possuir CP.

Nenhuma dessas decisões fica escondida em acceptance criteria.

## 21. Critério de conclusão desta spec

A spec está pronta para implementação quando:
- model file e conformance suite são versionados;
- cada regra provisória informa seu status;
- os quatro outcomes são mutuamente exclusivos/exaustivos;
- setup e economics fecham matematicamente;
- simultaneidade não depende de arrival order;
- no-action termina;
- contratos CSF/RAG/Cognition/Economy estão explícitos;
- não existe regra que permita a LLM arbitrar o mundo.

A **implementação** desta spec só começa depois de D2 e de um ticket aceito conforme o workflow do repositório.
