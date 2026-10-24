# ADR 0012 — Reconciliação canônica cumulativa de revelações posteriores

**Status:** Accepted for canonical data-model evolution (operator direction, 2026-10-23)  
**Related:** ADR 0001 (truth/belief), ADR 0002 (actor memory), ADR 0003 (event time), ADR 0004 (Y1 boundary), ADR 0005 (canon four time axes)

## Decision

A base consolidada é **cumulativa e revisável**. Um volume posterior pode explicar um evento anterior e enriquecer seu dossier global sem relocalizar a citação original, apagar a afirmação histórica ou atribuir ao ator conhecimento que só foi revelado mais tarde.

Cada vínculo de revelação tardia contém **identidades independentes** de evento ocorrido, evidência de origem, evidência explicativa posterior, tempo da ocorrência, tempo da revelação narrativa, tempo de conhecimento de cada ator e a interpretação anterior. Se um elemento temporal não puder ser estabelecido, use `UNKNOWN` ou `HOLDS_BY` com base explícita (ADR 0005); não invente data.

Atribuir uma explicação posterior ao mesmo evento não torna a explicação uma observação da fonte antiga. Seu `source_work_id`, capítulo e PDF page permanecem os do **volume revelador**, mesmo quando `canonical_event_id` remete a um evento de volume anterior. Os registros antigos não são transferidos de pasta.

As relações possíveis são:
- `SUPPLEMENTS`: acrescenta motivos, condições, consequências ou circunstâncias antes omitidas;
- `RESOLVES_AMBIGUITY`: retira um ou mais estados de dúvida com uma nova fonte explícita;
- `CORRECTS_PRIOR_INTERPRETATION`: substitui interpretação contradita, preservando histórico e o status antigo;
- `CHALLENGES_WITH_CONFLICT`: suporte novo conflita com anterior; abrir conflito e bloquear promoção;
- `CONTEXTUALIZES`: apenas conexões de contexto sem força probatória direta.

O `event_id` é global e estável. Um mesmo evento pode possuir evidências de múltiplos volumes, ordenadas por **tempo de revelação**, sem restringir em qual volume a referência aparece. Um caso comportamental pertence somente ao ator da ação primária e ao episódio temporal de execução; um relato posterior de terceiro normalmente é *evidência contextual*, não um novo feito comportamental daquele ator.

### Algoritmo de atualização segura

1. Registrar nova evidência com source ID, página, modo de atribuição e tempo de revelação.
2. Localizar evento ou regra global já existente (ou criar evento durável, sem alterar fontes antigas).
3. Anexar ligação de reconciliação entre registros e especificar qual campo ou interpretação muda.
4. Preservar `previous_assertion`, `new_explanation`, `resolution_status` e ressalvas epistêmicas.
5. Atualizar o **pack global** e a autoridade institucional/exame correspondente, sem supor que as personagens conheceram o novo detalhe.
6. Reconstruir índices bidirecionais evento→fontes e fonte→eventos. Testar controles negativos de atribuição, temporalidade, vazamento, falsa eliminação de ambiguidades e transferência indevida de casos.
7. Manter engine e memória dos agentes bloqueados até revisão semântica, aceitação de autoridade e gates existentes.

### Quatro perguntas separadas

- **O que aconteceu?** Proveniência do evento e suas condições;
- **Quando aconteceu?** Tempo válido (não confundir com edição/publicação);
- **Quando soubemos pelo texto?** Volume/capítulo/página que esclareceu;
- **Quem soube dentro da história, e quando?** Apenas atores com aquisição observada.

### Exemplos e limites

O acordo Ryūen–Katsuragi da ilha (Y1 V3) pode ser complementado por cláusulas que um volume futuro venha efetivamente a revelar. Não se presume que o V4.5 esclareça tais cláusulas. O método de escolha dos VIPs do zodíaco deve continuar `UNKNOWN` até que apareça fonte normativa suficiente; uma professora deslocar Ichinose entre **grupos** não é prova do algoritmo que seleciona o **VIP** de cada grupo.

No Y1 V4.5, a narrativa final de bastidores da piscina explica ações realizadas no mesmo dia, relatadas primeiro em capítulo anterior. Isso é reconciliado pelo mesmo event ID, mas o tempo de revelação ao leitor não altera o tempo da ação nem generaliza conhecimento sobre a operação.

## Invariantes de regressão

- Nenhuma explicação futura altera automaticamente `known_by` ou `belief_state` no passado.
- Uma `BELIEF`/fala de personagem permanece atribuída até evidência independente.
- `RESOLVES_AMBIGUITY` exige campo de resolução e suporte da nova fonte.
- `CHALLENGES_WITH_CONFLICT` não compila regra executável.
- A fonte explicativa sempre mantém seu locator verdadeiro, inclusive para eventos antigos.
- Uma evidência tardia só promove capacidades do ator que executou a ação diretamente demonstrada.
- Nenhuma referência do corpus pós-`Y1_START` vira memória canônica pré-divergência sem gate explícito.
