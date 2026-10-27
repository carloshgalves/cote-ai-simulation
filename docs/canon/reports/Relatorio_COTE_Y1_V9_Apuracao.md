# Apuração Year 1 Volume 9 — integração candidata

**Fonte privada:** 238 páginas físicas, SHA-256 `51c4bdcea2cfa0389617a56de52970a7db878c4e37d4e07a02b7863df9c1d6db`. O texto integral não é versionado. **Status:** 24 casos comportamentais candidatos, 10 evidências contextuais e 11 vínculos relacionais sem revisão externa independente; `verified:false` e sem admissão ao engine.

## Achado principal: fronteira temporal
O furto de presilha por Ichinose e isolamento subsequente antecedem a matrícula (pp. 6–7, 172–176). A confissão a Nagumo, informações a Arisu, confidência a Kiyotaka e exposição pública ocorrem em eventos escolares posteriores com canais de conhecimento independentes (pp. 7–11, 16–18, 168–179, 207–213). História contada depois não significa evento ocorrido depois; leitor informado não implica personagem informado. NÃO registrar antecedentes como `simulation_episodic_memory`, tampouco as experiências escolares canônicas como memórias da execução divergente.

## Estratégias e proatividade
Arisu articula ofensiva reputacional, neutralidade política e resposta induzida de Kiyotaka (pp. 14–18, 183–186, 232–238). Kiyotaka articula informação de Kushida, suporte de Kei, intermediação de Kiriyama e persistência diante de Ichinose (pp. 187–213). Essas ações são exemplos de **geração de iniciativas/planos** independentes de ordens de terceiros; não obrigam repetição em outro mundo. As falas de Nagumo e deduções de Hashimoto são crenças, não verdade global.

## Exame e instituições
O volume não introduz novo **exame especial** resolvível; há referências a prova provisória e exames regulares (pp. 107–109, 165–167, 214–216). `exams/y1-v09-academic-context.json` é somente contexto. A escola anuncia penalização prospectiva por boatos na fase final (pp. 179–181). A discussão de Nagumo e Sakayanagi sobre pontos de veteranos e eventual mudança de regras não prova adoção normativa (pp. 20–23).

## Packs e estado
- `behavioral_cases/<primary-actor>/v09-c*.json`: ações atribuídas ao ator primário, não a todos os mencionados.
- `evidence/y1-v09.json`: 34 registros referenciais, todos não verificados.
- `characters/*.json`: referências globais append-only, sem reescrever experiências prévias e com guard de temporalidade.
- `institutions/anhs.json` e `student-council.json`: alegações/declarações e limites.
- `relationships/y1-v09.json`: relações contextuais sem reciprocidade ou estabilidade inventadas.
- `retrospective/events/evt.pre-y1.ichinose-hairclip-shoplifting.json`: ocorrência pré-ingresso e canais posteriores.
- Indexação determinística e revisão individual obrigatórias antes de liberação.

## Gates
Assistant source review: **CANDIDATE, NOT ITEMIZED**; avaliador independente: **NOT PERFORMED**; operador: **PENDING**; runtime: **BLOCKED**; merge: **NÃO AUTORIZADO**. Implementação do planejador proativo será acompanhada em #45; atualização de objetivos em #36.
