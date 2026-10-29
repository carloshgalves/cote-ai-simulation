# Revisão semântica dos packs — COTE Y1 Volume 11
**Data:** 28/10/2026. **Autor:** mesma assistência que montou a extração anterior; esta **não é uma avaliação independente**. **Fonte:** `COTE Y1 V11.pdf`, 476 páginas físicas, SHA-256 `cd2c30f42da87a0a19c0666ba87777bc7242adf66a395e5aa9a426b720e7bec0`.

## Conclusão e alcance do trabalho
**REPROVADO no gate de profundidade comportamental para integração ao motor.** O núcleo da estrutura está correto (links por personagem/tema/página, status não executável), mas 53 de 56 casos ainda são, na prática, **resumos de cenas**. Três casos (C028, C044, C054) receberam detalhe localizado e correção factual durante esta revisão; estes três não estão automaticamente homologados. A revisão da qualidade dos dados foi ampla, mas **não** correspondeu a conferir cada palavra das 160 unidades com avaliador independente. Os trechos críticos foram examinados diretamente no PDF.

### Estatísticas verificadas no código de origem
- 56 casos; 74 evidências; 15 relações; 12 transições; exame e duas fichas biográficas/extensões.
- Nos 56 casos, o campo `other_actors` está vazio; os participantes efetivos e seus papéis não são representados dentro do caso.
- 53/56 casos continuam com `observed_actions` repetindo `title`, `observed_result` histórico genérico e `counterfactual_test` universal.
- As 12 transições possuem a mesma alternativa contrafactual genérica; as 15 relações têm `source_refs` de apenas um registro, mesmo quando descrevem dois ou mais momentos.
- `goal_transition_refs` e `y1_v11_behavioral_case_refs` no pack global não resolvem a falta de causalidade dos objetos que apontam.
- `audits/y1-v11-semantic-depth-matrix.json` enumera os 56 casos, status de profundidade e prioridade; distingue `SPOT_CHECKED` de `STATIC_RECORD_REVIEW_ONLY`.

## Findings corrigidos no corpus desta PR
**F1 — HIGH — Caratê ≠ judô (pp. 371–374).** Quinta prova: caratê com participantes escalados e uma repetição autorizada, vence D. Sexta prova: judô; Ryūen escolhe Albert, Ichinose não seleciona adversário antes de 30s; Mashima decreta derrota de B por padrão, sem sorteio aleatório por risco de segurança. Corrigidos casos, evidência, Exam Pack, auditoria, manifesto e relatório; controles negativos acrescentados.

**F2 — HIGH — Hirata (pp. 299–304).** Conteúdo anterior à matrícula é biografia; **o ato de revelá-lo a Kiyotaka é posterior à matrícula**. O caso C028 agora representa divulgação privada no Ano 1, e o dossiê anterior continua fonte da pregressa experiência. Note a diferença entre `event_time`, `revelation_time` e `knowledge_acquisition_time`.

**F3 — MEDIUM — Arisu (pp. 446–454).** A aceitação da habilidade de Kiyotaka não demonstra, sozinha, refutação da tese genética da personagem. Ela também retira uma justificativa para evitar confronto direto e revela motivação pessoal adicional (pp. 453–454). Corrigidos caso C054, goal G001 e página 454 no índice.

## Findings abertos (não homologar)
**F4 — BLOCKER: casos não detalhados.** O formato exige, por cena: `trigger`, `objective_at_time`, `actor_beliefs_and_uncertainties`, `actually_known_information`, `available_choices`, `plan_and_sequence`, `other_actor_roles`, `observable_result`, `causal_evidence_vs_inference`, `ability_limits` e `counterfactual_tests`. Não basta preencher `capabilities_supported=["strategy"]`: sem comportamentos operacionais, o LLM recebe um rótulo abstrato.

**F5 — HIGH: atores auxiliares e autoria.** `other_actors: []` em 56/56. C010 depende de Kaneda, Ichinose e Ishizaki sem transformar sorteio em fraude; C025–C026–C039 exigem papéis distintos de Keisei, Katsuragi e Arisu; C041–C042 distinguem iniciativa de Ishizaki e Hiyori de execução de Ryūen; C049–C052 distinguem jogador, comandante, funcionário e sistema adulterado.

**F6 — HIGH: relações multi-época e transições sem trilha suficiente.** `rel.001` une olhar unilateral infantil (pp. 7–12) e relacionamento escolar recíproco (pp. 445–450), mas só cita E001. Separar fases e evidências; identificar autor/recipiente do conhecimento, causa real de atualização e incerteza. Goals como G001 precisam de estado inferido e ação observada, não uma assertiva universal de mudança.

**F7 — HIGH: Mii-chan sem pack.** A personagem procura Hirata, propõe refeição e sofre rejeição (pp. 129–133), e continua tomando iniciativa (pp. 148–151). O corpus não tem a identidade formalizada. Ref. #51; não atribuir as ações a outro agente.

**F8 — MEDIUM: regulamento e adjudicação ainda incompletos.** O PDF p. 43 especifica quantidades de participantes diferentes por proposta **excluindo reservas**, mas mínimo/máximo **inclui reservas**. A adjudicação extraordinária do judô p. 374 mostra que regra de preenchimento aleatório não é incondicional em cenário de perigo. A cadeia `submitted_command` → `approved` → `delivered` → `executed` exige trilha verificável; o abuso de Tsukishiro não é ação regular permitida pelo contrato.

**F9 — MEDIUM: pressão de Ryūen e atribuição de causa.** O roteiro de pressão aparece em pp. 391–398, sintomas e conflitos em pp. 368–370. Evitar inferir automaticamente que Ryūen causou clinicamente todos os sintomas. É possível modelar observação, intimidação, coleta de informação e consequência institucional, com limites e condições.

**F10 — MEDIUM: evidências, objetivos e instituições.** As 18 evidências contextuais usam um único `source_mode` genérico para fala particular, testemunho, norma e resultado oficial. Os packs ANHS/White Room têm bons limites negativos, mas faltam vigência temporal, autoridade e divulgação a cada ator; as biografias precisam manter voz do narrador distinta de relato de terceiro.

## Exemplos do nível de detalhe exigido
**C040 — Ryūen:** detectar a preferência e a vulnerabilidade de B; avaliar possibilidade de vantagem em provas físicas; elaborar etapas de pressão sem agressão ostensiva, sondagem e coleta de informação; escolher eventos e comandar. Descrever participantes, risco de sanção, a hipótese de ataque psicológico e o resultado **observado**. No fork: se Ichinose ou a escola reagirem cedo, o plano pode falhar.

**C042 — Hiyori:** ela percebe necessidade de reunir Ryūen/Ibuki/Ishizaki, reserva encontro no karaokê, comunica convidados, insiste para Ibuki esperar, tolera várias horas de atraso e consegue colocar os atores juntos (pp. 382–389). O sucesso é **reunião iniciada**, não autoria do plano de pressão posterior.

**C038 — Kōenji:** grande capacidade de cálculo e decisão consciente de não responder são variáveis separadas. O teste deve permitir `can_do=true` junto de `chooses_not_to_do=true`; supor participação ideal por um score alto destrói a fidelidade.

**C029 — Kiyotaka/Hirata:** a abordagem inclui atribuição deliberada de culpa extrema e pressão emocional (pp. 305–309), seguida de oportunidade de dividir o fardo. O próprio narrador admite interesse na função futura de Hirata. Não classificar como terapia geral, nem prever recuperação inevitável em toda simulação.

**C049–C052 — xadrez:** o comando original de Kiyotaka não é o comando entregue à jogadora; Tsukishiro interfere. A vitória oficial de A permanece, e a continuação particular vencida por Kiyotaka não a substitui (pp. 417–450).

## Aceite ainda pendente
1. Enriquecer os 53 casos genéricos e completar papéis/alternativas nos três parcialmente corrigidos.
2. Revisar 74 evidências, 15 relações, 12 goals, biografias e Exam Pack com proveniência por afirmação.
3. Tratar #51 e resolver #52; implantar controles negativos por mecanismo, além de testes estruturalmente verdes.
4. Segunda leitura independente por avaliador diferente com acesso ao PDF original; findings e re-review.
5. Aceite humano para sair de Draft. Manter `verified:false` e `engine_memory_admission:false` até os gates.


## Remediação concluída na branch — 28/10/2026

O bloqueio **F4** referente à ausência de detalhamento foi tratado nos **56 casos originais** e foram acrescentados **3 casos de Mii-chan (Mei-Yu Wang)**, totalizando **59 casos estruturados**. Cada caso agora possui gatilho específico, objetivo, informação disponível, plano/ações, resultado observado, participantes e papéis, limites epistêmicos e uma alternativa contrafactual. Os registros seguem `verified:false` e não executáveis: esta é **remediação semântica do próprio autor**, não segunda opinião independente sobre todas as páginas.

O bloqueio **F5** foi tratado pela atribuição explícita de participantes nos 59 casos. F6 passou a ter **16 relações com 24 fases de evidência** e **13 transições condicionais de objetivos**, com fronteira temporal e guardas de conhecimento. A identidade Mii-chan = Mei-Yu Wang foi confirmada explicitamente pelo PDF físico **p.58**. Foram criados os casos C057–C059 (pp.58–59, 129–151, 283–314), a relação rel.007 e transição G013. Isso resolve o escopo de identidade e cobertura de #51 sem presumir reciprocidade romântica.

O Exam Pack recebeu regras sobre reservas/participantes da p.43 e exceção de segurança da prova de **judô**, sexta modalidade contra a Classe D (pp.372–374), enquanto o caratê foi o quinto evento efetivamente disputado. O canal adulterado de xadrez foi mantido como evento irregular, distinguindo resultados oficiais, entrada enviada e instrução entregue. As 18 evidências contextuais receberam modos epistêmicos individualizados.

**Total revisado estruturalmente:** 59 casos, 77 evidências, 16 relações, 13 metas, um Exam Pack e dois eventos/atualizações de biografia = 168 entradas de auditoria. A matriz `y1-v11-semantic-depth-matrix.json` registra **0 casos com placeholders genéricos**. O conteúdo permanece sujeito à leitura de fonte e parecer independente antes de promoção para `verified:true`/motor. Não confundir entrega de casos ricos com validação editorial concluída.
