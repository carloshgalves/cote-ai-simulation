# Lacunas

Espelho humano de `data/canon/open_questions/`. Fonte de verdade são os arquivos YAML; esta tabela é para leitura e priorização.

23 questões abertas. Nenhuma respondida — nada foi conferido em fonte Tier 0–1.

## Blockers

Travam o vertical slice. Enquanto abertas, o engine não recebe regras econômicas nem acadêmicas.

| Questão | Por quê |
|---|---|
| `initial-state.day-one-briefing` | Define `known_by` de todos os alunos no `Y1_START`. Tudo que não é dito no dia 1 é informação oculta por construção. |
| `initial-state.starting-class-points` | O 1000 inicial está no texto ou é reconstrução de fã? É o número mais estrutural do snapshot. |
| `academic.expulsion-formula` | Proporção da média da turma × linha de corte fixa são mecânicas diferentes. |
| `conduct.deduction-table` | Acumulador determinístico × avaliação mensal opaca produzem incentivos muito diferentes. |
| `academic.buy-point-scope` | Regra geral é mecânica do engine; discricionariedade é afordância de um personagem e não pode ser generalizada. |
| `governance.class-label-reassignment` | Decide se rótulo de classe é identidade estável ou projeção de ranking. |
| `admission.placement-basis` | Delimita `CANON_ADMISSION_FACTS`; tudo além vira `SIMULATION_ADMISSION_MODEL`. |

## Alta prioridade

`economy.payout-timing` · `economy.transfer-rules` · `economy.class-transfer-effective-from` · `academic.protection-point-origin` · `admission.dimension-semantics` · `admission.eligibility-wording` · `roster.exact-sizes` · `exam.disclosure-model` · `initial-state.dorm-handbook`

## Média e baixa

`governance.council-authority-scope` · `observability.camera-map` · `initial-state.ceremony-date` · `initial-state.mock-exam-status` · `prehistory.school-founding` · `prehistory.white-room-vs-school-founding` · `terminology.s-system`

## Duas questões que testam o modelo temporal

`economy.class-transfer-effective-from` e `academic.protection-point-origin` são instâncias diretas da pergunta central: *isso passou a ser verdade naquele momento, ou apenas foi revelado ao leitor naquele momento?* Ambas estão registradas com `mode: UNKNOWN` — nem o limite superior é conhecido — e por isso **não** entram no snapshot de `Y1_START`. Se a leitura mostrar que as regras já vigoravam, elas passam a integrar o estado inicial do mundo sem passarem a integrar o conhecimento dos alunos.

## Sessão de leitura dirigida

A forma mais barata de fechar isto é uma passagem sobre o Volume 1 (capítulos 1–4) com as sete questões blocker abertas ao lado. Sozinho, o Volume 1 fecha ou reduz cinco delas. As de admissão exigem o Volume 7; as de pré-história, o Volume 0.
