# COTE AI Simulation — Apuração e integração do Ano 1, Volume 8

**Fonte:** *Classroom of the Elite*, Y1 V8 (PDF inglês Seven Seas fornecido pelo operador, 244 páginas físicas, SHA-256 `b17ba426631d97fea69e5d0df3e231fa01f39c4651460affeb3cbb48a1410167`). **Paginação:** a posição física no PDF começa em 1; pode diferir da página editorial impressa. **PDF original não está no GitHub.**

**Status:** apuração de referência, sem homologação independente de cada proposição e sem execução automática no simulador. O registro por item está em `data/canon/consolidated/audits/y1-v08-review-register.json`, com **88 decisões**: 42 candidatos comportamentais/mudanças internas, 14 relacionais, 17 de exame, 7 institucionais e 8 filtros. Os 42 primeiros resultaram em **38 casos comportamentais individualizados** e **4 mudanças de estado sem feat autônomo**.

## 1. A contribuição principal do V8

O Mixed Training Camp promove interação forçada entre **as três séries** e classes normalmente rivais. A separação de alojamentos e exercícios por gênero, a retenção dos celulares e o encontro diário na refeição tornam a **aquisição de informações** um recurso estratégico. O resultado de uma turma depende também dos estudantes mais velhos que integrarem o mesmo grande grupo.

O caso central separa **vitória na pontuação** de **vitória política**: o grande grupo masculino de Ninomiya/Manabu termina em primeiro; o de Nagumo/Keisei termina em segundo. Paralelamente, o último grupo feminino de Ikari permite nomear Tachibana pela regra de solidariedade; o resgate custa à classe de Manabu grande quantidade de pontos, e Ishikura também recorre à revogação para impedir a saída de Ikari. O grupo político de Nagumo se beneficia do estrangulamento de liquidez de adversários, segundo a reconstrução de Kiyotaka (PDF pp. 227–240).

**Fronteira de conhecimento:** o livro mostra regras anunciadas e efeitos observados, mas *não* apresenta as atas de acordos secretos, transferências privadas individualizadas ou a rubrica exata que a escola aplicou a cada estudante. Inferir conluio e um pagamento antecipado é diferente de observar o pagamento.

## 2. Exam Pack — Mixed Training Camp

O pack de referência, já existente na `main` ao início desta apuração, é `data/canon/consolidated/exams/y1-mixed-training-camp.json`. Esta integração o vinculou a **17 evidências do V8** e preservou a autoridade `CANON_REFERENCE_NOT_EXECUTABLE`.

### Constituição dos grupos e duração

- Acampamento de **8 dias/7 noites** com três séries; turmas e gêneros formam **seis pequenos grupos por série e gênero**, unidos depois em seis grandes grupos de cada lado, aproximadamente **30–45** pessoas por grande grupo (pp. 14–20).
- Cada pequeno grupo tem integrantes de **pelo menos duas classes**, depende de acordo **unânime** e não pode trocar pessoas depois; ausências por doença não geram substituição (pp. 15–19).
- Faixas de participantes por população do mesmo gênero/série: a partir de **60: 8–13**, **70: 9–14**, **80: 10–15** por grupo; a tabela para menos de 60 não é recuperada integralmente (pp. 15–16).
- Eixos declarados: **ética, fortitude, disciplina e iniciativa**. Atividades incluem zazen, discursos, trabalhos cotidianos, prova escrita e corrida de revezamento; peso da nota por componente **não divulgado** (pp. 18–19, 217–226).

### Tabela de base por aluno (PDF pp. 20–22)

| Posição do grande grupo | Pontos privados | Pontos de classe |
|---|---:|---:|
| 1º | +10.000 | +3 |
| 2º | +5.000 | +1 |
| 3º | +3.000 | 0 |
| 4º | −5.000 | 0 |
| 5º | −10.000 | −3 |
| 6º | −20.000 | −5 |

Recompensas positivas variam por diversidade de classes (duas: **×1**; três: **×2**; quatro: **×3**) e tamanho do pequeno grupo (dez estudantes: **×1**; quinze: **×1,5**; nove: **×0,9**). Multiplicadores **não** incidem sobre descontos. A turma que reúne doze de seus integrantes em grupo de quinze e nomeia um representante dessa turma recebe, na hipótese de primeiro lugar da fonte, **1.080.000 pontos privados e 336 pontos de classe**. Trata-se de exemplo, não resultado real ou fórmula para todos os totais (pp. 20–23).

### Representante, solidariedade e revogação

A escolha de representante é obrigatória até a manhã seguinte; nenhum escolhido significa **desqualificação e expulsão de todos do pequeno grupo**. O representante e os colegas da mesma classe obtêm prêmio dobrado quando aplicável. Na última colocação de grande grupo, só o representante de pequeno grupo **abaixo do limiar escolar** fica sujeito à expulsão; sua indicação solidária só pode alcançar pessoa que a escola considere contributiva para o mau resultado (pp. 22–24).

Expulsão causa **−100 pontos de classe**. A `Revocation of Expulsion` pode impedir a saída mediante **20 milhões de pontos privados e mais 300 pontos de classe**, sem apagar as demais penalidades. Isso é diferente da aquisição de transferência de classe, mesmo quando cifras se parecem (pp. 24–26).

### Resultados observados

- **Meninos:** 1º Ninomiya (com Manabu), 2º Nagumo (com Kiyotaka/Keisei), nenhum expulso (pp. 227–229).
- **Meninas:** 1º Ayase Natsu (com Suzune e Kushida); grupo de Ikari Momoko em último, abaixo do limiar (pp. 229–231).
- **Solidariedade:** Tachibana Akane é indicada; Manabu usa resgate; Ishikura utiliza resgate para Ikari. Não houve expulsão definitiva dessas duas na sequência narrada (pp. 231–240).
- **Não publicados:** médias numéricas completas, todos os rankings intermediários, rubricas de nota, listas integrais de participantes e razão/registro oficial individual de cada imputação de responsabilidade.

## 3. Estratégias individuais e reuniões noturnas

**Nagumo:** desafio público a Manabu pelo desempenho médio masculino (pp. 57–60), jogos de cartas com apostas em tarefas e leitura de comportamento de oponentes (pp. 96–109), pressão noturna sobre encontros de outros alunos (pp. 208–211). O efeito mais importante ocorre no resultado feminino e nas despesas de resgate, não na classificação masculina. A coordenação completa com Ishikura e todos os estudantes é **hipótese retrospectiva de Kiyotaka** (pp. 237–240).

**Manabu:** protege um compromisso de competição sem sabotagem ao aceitar o duelo (pp. 59–60), conversa reservadamente com Kiyotaka (pp. 142–147), e finalmente prefere garantir a permanência de Tachibana a preservar a vantagem financeira de curto prazo de sua classe (pp. 235–240). O monólogo inicial revela arrependimento por ter priorizado o papel exemplar e ter deixado amizades em segundo plano (p. 10). Isso é **estado privado**, não incapacidade social universal.

**Kiyotaka–Kei:** instrução por telefone antes da retenção dos aparelhos; durante a única janela diária de refeição, Kei administra discretamente a saída das amigas e relata os arranjos dos grupos femininos (pp. 30–32, 120–127). Kiyotaka procura protegê-la dentro de um conjunto de prioridades individuais; ela simultaneamente demonstra intimidade, insegurança e capacidade analítica. Após os resultados, pergunta por que a representante escolhida era da Classe B, levando a análise de custo/credibilidade (pp. 237–240). A promessa de proteção **não constitui namoro anunciado nem capacidade infalível de salvamento**.

**Hashimoto–Ryūen:** não fundir o encontro clandestino de madrugada (pp. 203–209) à conversa de dormitório no escuro (pp. 182–185). O primeiro testa oportunidades de mudança de facção; o segundo reduz atritos e ajuda na coesão. Nenhuma deserção se concretiza nesse ponto.

**Kōenji:** recusa responsabilidades representativas, expõe tentativa projetada de comprar pontos de alunos mais velhos e limita contribuição a atividades da equipe (pp. 63–68, 189–191, 223–226). O plano de compra de pontos **não foi executado** no trecho; proibição futura **não foi formalmente aprovada**.

**Keisei, Ishizaki, Akito e Hirata:** Keisei assume a liderança de pequeno grupo considerado frágil, Ishizaki passa da hostilidade à cooperação local, Akito assume risco de representação, e Hirata tenta mecanismos de consenso e de redistribuição dos ganhos, rejeitados em votação (pp. 27–29, 49–51, 68–88, 168–193).

**Suzune e Kushida:** Suzune confia publicamente uma função de apoio a Kushida, mas a interação posterior confirma hostilidade privada e interesse em continuar reconhecendo as circunstâncias do passado dela. Não transformar a parceria em confiança adquirida (pp. 30–31, 212–215).

**Asahina, Ichinose e Kiriyama:** informação parcial sobre arranjos do segundo ano, facilitação de interação das meninas e oposição ambígua ao presidente do conselho; contexto de cada conversa deve ter seu próprio grupo de conhecedores (pp. 116–118, 148–152, 173–181).

## 4. Relações, personagens introduzidos e índices

O grafo `relationships/y1-v08.json` contém **14 transições contextuais**: Kiyotaka/Kei; Kiyotaka/Manabu; Manabu/Nagumo; Manabu/Tachibana; Nagumo/Kiriyama; Kiyotaka/Asahina; Keisei/Ishizaki; Hashimoto/Ryūen; Suzune/Kushida; Ichinose e conversas femininas; amizades do Ayanokōji Group; Arisu/Yamauchi; Nagumo/Ishikura/Ikari (hipótese atribuído); Kiyotaka/Kōenji.

Foram adicionados packs de **Hashimoto, Nazuna Asahina, Ishikura, Ikari Momoko, Kuranosuke Ninomiya e Natsu Ayase**, além dos vínculos dos atores já presentes. Estado relacional, conhecimento, simpatia e risco político não são automatizados a partir do resultado histórico.

## 5. O que não virei feat — e por quê

1. Conversas e comparações corporais no banho: **ambiente social**, sem estatísticas universais de força.
2. Divisão de limpeza/café: **contexto cotidiano**, salvo decisões realmente distinguíveis.
3. Leitura de Kiyotaka depois do resultado: **dedução contingente**, não previsão perfeita.
4. Suposta paixão de Ichinose por Kiyotaka: **conjectura/indício**, não compromisso amoroso certificado.
5. Emendas futuras anticolusão: **hipótese**, não regra que a escola já implementou.
6. Turmas femininas completas: **roster parcial**, completar seria inventar.
7. Premiações: **tabela anunciada** não comprova créditos específicos lançados em cada conta.
8. Controle formal de Nagumo sobre todo currículo: **influência/participação atribuída**, não ato normativo certificado.

## 6. Artefatos, integridade e gate

- `exams/y1-mixed-training-camp.json`: regras, resultados, incertezas, agora indexadas pelas 17 evidências da apuração.
- `evidence/y1-v08.json`: **80 registros com localizador**, excluídos os oito filtros autônomos.
- `audits/y1-v08-review-register.json`: **88 decisões individuais**, incluindo os oito filtros.
- `audits/y1-v08-case-manifest.json`: **38 casos comportamentais**; quatro mudanças pessoais permanecem como contexto/estado em packs de atores.
- `relationships/y1-v08.json` e índices globais, inclusive índice de páginas.
- **Nenhum registro `verified:true` / runtime `engine_memory_admission:true`** foi autorizado. Um segundo revisor independente com o PDF original ainda não se pronunciou. O aceite da PR pelo autor é um gate separado.
