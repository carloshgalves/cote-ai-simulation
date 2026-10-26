# Y1 Volume 6 — revisão semântica/adversarial final da Draft PR #37

Data: 2026-10-26. Estado: revisão do assistente com passagens de maior risco verificadas no PDF; **não** corresponde a revisão independente, aprovação humana ou certificação para execução.

## Proveniência
- Fonte primária: `COTE Y1 V6.pdf`, SHA-256 `e85ebfe1efb68282f8067768b8d81b289492c295720d157f77afdd4e10ee706b`, 211 páginas físicas.
- Relatório de base: `Relatorio_COTE_Y1_V6.md`, SHA-256 `b637e9625e3287dd53a6c3f6bbfe5a4071751bec6fca3cf8b9be6f227223d320`.
- Arquivos incorporados: 80 evidências candidatas, 43 casos por ator, 15 transições históricas de metas/crenças, dossiês retrospectivos e `exams/y1-paper-shuffle.json`.
- Auditoria atual: 50 evidências com janela primária diretamente conferida em alguma etapa; 31 com segunda verificação semântica dirigida aos riscos; 43 casos com atribuição/links de fonte verificados estruturalmente; 15 transições com causalidade/alternativas verificadas estruturalmente. **107 verificações semânticas profundas específicas permanecem pendentes; 138 avaliações independentes permanecem pendentes.** Nenhuma afirmação recebe `verified:true`.

## Achados remediados na revisão final

| ID | PDF físico | Achado e correção |
|---|---|---|
| V6-F011 | 14–16 | Nagumo **Miyabi** assume publicamente a presidência. A fala anuncia reformas, mas ele próprio reconhece que não consegue implementá-las de imediato. Nome, mandato e incerteza de execução separados. |
| V6-F012 | 150–152 | Hasebe é **Haruka**, Miyake é **Akito**, Yukimura é **Teruhiko** e prefere **Keisei** porque foi o nome querido pelo pai. Identidades permanentes `actor.*` preservadas; o apelido não vira nome legal nem altera memórias de atores que não ouviram o diálogo. |
| V6-F013 | 13, 29 | Fichas escolares ilustradas: Yukimura nasceu em **11 de julho**, Satō em **28 de fevereiro**. Notas de habilidade continuam ordinais, históricas, não convertidas em valores numéricos do simulador. |
| V6-F014 | 207–208 | Ryūen passa a fornecer respostas falsas a Kushida, mas o texto explicita que **não abandonou a meta de alcançar a Classe A**. É mudança de tática/prioridade local, não substituição de objetivo final. |

## Riscos de alto impacto preservados

1. **Regras do Paper Shuffle (pp34–39, 77–83, 188):** quiz diagnóstico com 100 questões e pareamento do primeiro com o último, em ordem de pontuação; 0/1/até 80/85+ são metas estratégicas de Horikita, não um método de pareamento estabelecido pela escola. As duas condições de reprovação da dupla são 60 pontos por disciplina e **692 pontos totais**; cerca de 700 era uma aproximação anterior.
2. **Autoridade das questões (pp177–185):** Chabashira revela as duas entregas e a aprovação anterior ligada à estratégia **autônoma de Horikita**. Não colapsar sua contramedida institucional com a operação anônima X–Ryūen.
3. **Aposta e resultado (pp118–129, 194–199):** Manabu testemunhou a aposta, não a explicação detalhada de Kushida. Kushida reconhece derrota e promete não interferir, mas continua hostil; resultados oficiais completos do Paper Shuffle não foram publicados no volume.
4. **Kushida (pp10–12, 125–129):** blog e confidências são relato histórico de primeira pessoa, com distinção entre informação verdadeira usada maliciosamente e diagnóstico/propensão inevitável. Não transferir os segredos da antiga turma para personagens que não os receberam.
5. **Ryūen e X (pp200–208):** mensagem `Transaction`, alternativas, cópias de provas, respostas falsas e ameaça com fotografia não conferem a Ryūen a identidade do remetente. A autoria material da folha escondida no uniforme também não é demonstrada.
6. **Ichinose (p174):** o colégio oficialmente rejeita alegação de fraude, mas a assinatura impressa de Ryūen e sua negativa posterior não certificam autoria dos panfletos.
7. **Hiyori (pp140–142, 157–158):** informação social anterior sobre açúcar fornece hipótese de como impressionou o grupo; a fonte não comprova observação sobre-humana ou quem ordenou a coleta.
8. **Objetivos de personagens:** mudanças observadas são contingentes, mediadas por experiência, informação e relação. Não forçar reforma definitiva de Sudō/Kushida, amizade irreversível, romance recíproco ou mudança arbitrária de objetivo para chegar à Classe A.

## Pendências herdadas da #34 (resolvidas administrativamente)
A primeira extração do Volume 5 (#34) foi substituída pela #35. Antes de fechá-la, foram trazidos para a branch da #37 os dois únicos detalhes que exigiam explicitação:
- Páginas 43–44 do Volume 5: **16 votos** para Horikita, **13** para Karuizawa, com abstenções.
- Página 18 do Volume 5 (regra do perdedor `-100`) versus páginas 190–191 (narração de `-50` para a Classe B). **Contradição da fonte ainda aberta**; não inventar exceção nem substituir resultados narrados.

## Controles e decisão

- `tools/test_canon_y1_v06.py`: 17 controles adversariais preexistentes.
- `tools/test_canon_y1_v06_semantic_adversarial.py`: 28 novos controles negativos para identidades, arquivos escolares, inferência X, objetivo persistente de Ryūen, limites de resultados, avaliação independente e as duas ressalvas herdadas da #34.
- As suítes de V5 e regressão cumulativa seguem obrigatórias no CI; toda promoção de memória, definição executável de exame e mecanismo executável de metas permanece **desabilitada**.
- **Estado recomendado:** manter PR #37 como Draft, sem merge, até resolver a revisão independente restante e o gate do operador. O relatório documenta uma revisão assistida substancial, sem reivindicar revisão por terceiro.
