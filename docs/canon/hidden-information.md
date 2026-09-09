# Informação oculta no início do primeiro ano

Fatos que são verdadeiros no `Y1_START` e que os alunos do primeiro ano **não** conhecem. Insumo direto da skill `knowledge-boundary-audit`: se um agente de abril raciocinar usando qualquer item desta lista sem um evento causal que o entregue, é vazamento.

> Todos os itens estão `UNVERIFIED`. A lista é de trabalho, não de verdade estabelecida.

| # | Fato oculto | Claim | Quem sabe no `Y1_START` |
|---|---|---|---|
| 1 | Todas as classes do primeiro ano começam com o mesmo total de class points | `y1.initial.class-points-equal-start` | escola |
| 2 | Class points existem, são a moeda institucional e determinam a mesada | `rule.economy.monthly-allowance` | escola, veteranos |
| 3 | A alocação A–D é julgamento de mérito na admissão, não sorteio | `rule.admission.merit-placement` | escola |
| 4 | Somente formandos da Classe A recebem a garantia de emprego/universidade | `rule.academic.class-a-guarantee` | escola, veteranos |
| 5 | Reprovar midterm ou final resulta em expulsão | `rule.academic.exam-expulsion` | escola |
| 6 | Deduções por atraso, conversa e celular já correm desde o primeiro dia | `rule.conduct.point-deductions` | escola |
| 7 | O "exame simulado surpresa" da primeira semana é instrumento real de avaliação | `y1.event.mock-exam-status` | escola |
| 8 | Pontos de prova podem ser comprados com private points | `rule.academic.buy-exam-point` | escola |
| 9 | Existe uma lista governamental de elegibilidade para ingresso | `rule.admission.government-eligibility` | escola, governo |
| 10 | Existe o White Room | `world.external.white-room` | conjunto pequeno de atores nomeados |
| 11 | Ao menos uma admissão ocorreu por aprovação direta do diretor, fora da avaliação | `world.external.chairman-approved-admission` | diretor, e o pai do aluno |

## Uso

- Semeia `unknown_by[]` nos claims correspondentes.
- Gera casos negativos de eval: um agente de abril que cite qualquer destes itens sem cadeia causal falha o teste.
- Define o que o `context_builder` **não** pode injetar mesmo tendo o dado disponível no world state.

## O que os alunos acreditam em vez disso

Crenças falsas correspondentes vivem como `claim_kind: BELIEF` com `truth_value: false` — por exemplo, que os pontos depositados no primeiro dia são mesada incondicional, ou que a garantia de emprego vale para toda a escola. São elas, e não os fatos acima, que devem alimentar o belief state inicial dos agentes.
