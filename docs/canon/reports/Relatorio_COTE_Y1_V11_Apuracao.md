# Classroom of the Elite — Ano 1, Volume 11
## Relatório de apuração, arquitetura temporal, comportamentos e Event Selection Exam

**Fonte:** arquivo fornecido pelo operador, `COTE Y1 V11.pdf` (476 páginas físicas; SHA-256 `cd2c30f42da87a0a19c0666ba87777bc7242adf66a395e5aa9a426b720e7bec0`). A página de créditos identifica **Confused** para a tradução principal, **Cinnamon** para as side stories e **Ai Hinatsuru** para a compilação. É uma **compilação de fãs em inglês**, não há evidência de que sua paginação ou redação coincidam com a tradução oficial. Páginas referidas são as páginas físicas do PDF, contadas a partir de 1. O texto integral do livro não foi versionado.

**Estado:** documentação e dados **candidatos**, sem autenticação individual por segundo avaliador. Extraídos: **56 casos comportamentais**, **74 evidências** (56 comportamentais e 18 contextuais), **15 relações**, **12 transições históricas de objetivos**, **1 Exam Pack de referência** e **2 entradas biográficas (uma nova, uma extensão de V10)**. O registro de auditoria contém **160 itens**, todos `verified:false`, `engine_memory_admission:false` e com segunda revisão `NOT_PERFORMED`.

## 1. Relevância principal para agentes

Este volume combina quatro problemas que nenhum modelo de personagem deve confundir: **(1) identidade pré-matrícula, (2) conhecimentos individuais adquiridos na escola, (3) decisões proativas com causalidade, (4) diferença entre política institucional, resultado oficialmente registrado e execução adulterada**.

O prólogo de Arisu descreve lembranças de uma visita infantil à White Room. Sua interpretação de genialidade como talento genético antecede a matrícula, mas **é uma crença dela**, não regra científica da simulação. Kiyotaka, visto através de vidro unidirecional, não obteve necessariamente lembrança da observadora; é crucial manter a assimetria. A escola posteriormente reúne os dois, mas o encontro escolar não transforma uma observação antiga em memória compartilhada. Fonte: pp. 7–12; reencontro interpretado em pp. 445–451.

A história de Hirata, já aberta no volume 10, ganha detalhes decisivos: o amigo se chama **Sugimura**; sobreviveu à tentativa de suicídio, mas permanecia em coma conforme o relato de Hirata; após novas vítimas de bullying, Hirata instituiu controle por medo na antiga escola, e a direção redistribuiu turmas e impôs vigilância (pp. 299–303). **Biografia pregressa**; a confissão feita a Kiyotaka durante Y1 V11 é **evento de conhecimento escolar**, não informação pública em Y1_START. Seu afastamento, a pressão verbal de Kiyotaka e o retorno ao trabalho coletivo são episódios posteriores (pp. 129–151, 295–315). Não forçar a crise nem sua resolução em linhas divergentes.

## 2. Capítulo a capítulo — acontecimentos e estratégias

### Prólogo — monólogo de Sakayanagi (pp. 7–12)
Arisu acompanha o pai ao centro branco, observa crianças submetidas a treinamento intenso, identifica Kiyotaka como jogador excepcional de xadrez e forma o desejo de provar que a excelência vem da herança e não da educação. O pai descreve a instalação e os responsáveis sob sua própria perspectiva; não tornar a avaliação dele automaticamente um registro validado da instituição. O contraste entre olhar infantil, hipótese genética e observação concreta é matéria essencial do `belief_model` e do `character_background`.

### Capítulo 1 — The Teachers' Battle (pp. 13–26)
Na reunião de professores, Tsukishiro promove alterações de provas para calouros e defende a imposição do Class Poll antes de a decisão ser divulgada aos alunos. **Mashima protesta** por risco injustificado de expulsões; Chabashira e Hoshinomiya participam de discussões posteriores. O volume esclarece que o resgate normal de expulsão custaria 20 milhões de pontos privados **mais 300 pontos de classe**, dispensados excepcionalmente na votação do V10 (pp. 21–22). Logo, esta nova revelação contextualiza norma antiga sem retroagir custos à operação já ocorrida. Tsukishiro menciona autoridade corporativa e perspectivas de eventos como festival cultural, mas proposta não é aprovação formal.

Os professores têm vínculos e rivalidades desde seus próprios anos escolares. Chie fala de uma antiga paixão de Sae: atribuir a fala à testemunha, não fabricar ficha biográfica detalhada nem assumir que explica todas as políticas futuras.

### Capítulo 2 — The First Years' Final Battle (pp. 27–54)
Chabashira anuncia o **Event Selection Exam** para 22 de março: dez eventos por classe sujeitos a homologação; cinco selecionados para o dia; o sistema sorteia sete dos dez candidatos combinados. Prazo 14/3, divulgação de regras à turma adversária 15/3 (pp. 29–35). Eventos exigem desempate, tempo e regras não excessivamente obscuras. Distintos números de participantes, um a vinte, no máximo duas modalidades acima de dez; quem já participou não pode voltar antes de todo o elenco participar (pp. 42–43). Comandante obrigatório não compete diretamente, mas pode interferir de acordo com regra específica de cada evento (pp. 36–38). Perder implica expulsão do comandante ou **consumo de ponto de proteção**.

Cada vitória parcial transfere 30 pontos de classe entre adversários; o vencedor geral recebe mais 100 da escola; as sete provas são executadas até o fim. Se pontos de classe ficarem negativos, o sistema da escola mantém débito oculto (pp. 34, 40–41). Ranking prévio declarado: A 1001, B 640, C 377, D 318. Suzune registra e analisa regras, desvia questionamentos sobre a proteção recebida por Kiyotaka, e ele aceita comandar porque deseja confrontar Arisu.

### Capítulo 3 — Opponents (pp. 55–92)
Ao saber das preferências, Kiyotaka negocia secretamente com Ichinose e Ishizaki. **Kaneda vence o sorteio**, escolhe B para D e deixa C contra A. Sorteio efetivamente aleatório no relato; a existência de preferências alinhadas não transforma o sorteio em fraude (pp. 62–66).

O sistema de comandantes tem interface operacional: escolhe participantes sob prazo aproximado de 30 s por pessoa; posições faltantes/excedentes acionam substituição aleatória; em cada evento só um participante recebe mensagens aprovadas por texto-para-fala via fone, com limites próprios (pp. 67–71). Esses canais são autoridade de execução, não simples chat sem validação. Kei fornece inteligência social a Kiyotaka, preservando segredos e incertezas de relacionamento.

### Capítulo 4 — What the Class Lacks (pp. 93–169)
Ayanokōji Group considera eventos de sorte; a análise apresentada no texto demonstra que vantagem individual de 50% não garante vitória da turma ao exigir seleção e quatro vitórias (pp. 100–103). Suzune prepara opções e recolhe aptidões; Keisei sugere proteção contra espionagem (pp. 110–158).

Hirata se afasta e repele tentativas de contato de Mii-chan e Kei. Mii-chan toma iniciativas próprias apesar do risco emocional (pp. 128–151). **Lacuna:** o repositório atual não possui pack canônico identificado para Mii-chan/Wang Mei-Yu; manter a referência textual e os episódios contextuais sem criar identidade fictícia ou transferir suas decisões para outro ator. A escolha de não ter um pack agora é **limitação conhecida de cobertura**, não alegação de ausência de protagonismo.

Hoshinomiya conversa com Kiyotaka a respeito da história da escola e incentiva aproximação social; suas histórias sobre antigas residências são testemunho, não norma vigente automaticamente certificada (pp. 159–166).

### Capítulo 5 — A Trap, a Home-Cooked Meal, and a Favor (pp. 170–280)
Estudantes de D pressionam B. Ichinose prefere conter escalada pública, enquanto colegas desejam resposta. Reuniões da turma C selecionam alternativas e revelam dificuldades de coesão; Kiyotaka procura Kushida para compreender suas restrições políticas, mas falas dela podem ocultar sua motivação real (pp. 170–205). O período inclui encontros pessoais de Kei e Kiyotaka durante White Day; **presente e pensamentos privados não equivalem a namoro consensualmente declarado** (pp. 187–211 e contos posteriores).

Manabu fala sobre crescimento de Suzune, sem se tornar autoridade científica sobre sua capacidade (pp. 217–226). A Classe A divulga dez eventos; C planeja participantes a partir das regras publicadas, sem saber com certeza os cinco priorizados pelo oponente (pp. 230–239). Keisei aborda Katsuragi explorando sua oposição a Arisu, mas **uma oferta/expectativa de sabotagem não significa colaboração executada** (pp. 240–280). No quinto evento, Katsuragi não entrega a vitória esperada: Arisu antecipa a exploração do ressentimento e intimida-o com riscos para outros colegas (pp. 359–366).

### Capítulo 6 — A Man's Tears (pp. 281–317)
Com a resistência de Hirata e o esforço de Mii-chan ainda sem resolver o problema, Kiyotaka procura Hirata e insiste em ouvir seu passado (pp. 295–303). O relato revela culpa, omissão inicial diante do bullying, novas vítimas e posterior regime de coerção da antiga escola. Kiyotaka escolhe **pressioná-lo de forma dura** e admite ter objetivo instrumental de restaurar liderança funcional, não simplesmente oferecer consolo (pp. 304–308). Hirata chora, reconhece que pode depender da turma e retorna à participação coletiva (pp. 309–315). Não converter isso em intervenção terapêutica recomendada nem inferir que uma estratégia de culpabilização funcionaria em qualquer pessoa ou universo.

### Capítulo 7 — Ayanokōji vs. Sakayanagi (pp. 318–367)
Suzune entrega a Kiyotaka a liberdade final de escalar participantes. A sequência observada de **C contra A** é: 1) basquete (C vence), 2) digitação (C vence), 3) prova de inglês (A vence: 651 × 443), 4) matemática (A vence: 655 × 631), 5) cálculo mental rápido (A vence), 6) arco e flecha (C vence), 7) xadrez (A vence oficialmente). O 3–3 antecede a partida decisiva; selecionar a mesma competência para eventos diferentes exaure participantes. Fonte: pp. 335–365, 400–429. O comportamento de Kōenji, que deixa respostas em branco e identifica resposta final difícil, exemplifica **capacidade exibida não aplicada** (pp. 359–366), não domínio automático do exam pack.

### Capítulo 8 — Class B vs. Class D (pp. 368–399)
Ryūen havia deixado a liderança, mas Ishizaki o procura e Hiyori organiza encontro com Ibuki. O próprio Ryūen desenvolve estratégia de pressão indireta e coleta de informações sobre as preferências de B, priorizando eventos físicos (pp. 375–398). O texto relata observação, assédio e intimidação calculada, que devem ser modelados como **ações sujeitas a regras e consequências**, jamais como heurística geral de sucesso. Durante o evento de caratê, Ichinose não consegue designar participante e a escola determina derrota por forfait (pp. 371–374). A turma D supera B por **5–2**, conforme informe no epílogo (pp. 436–437). Isso significa **+190** para D e **−90** para B no modelo de pontos declarado, sem inferir outra operação oculta.

### Epílogo — The Line Between the Winner and the Loser (pp. 400–454)
A partida de xadrez entre Suzune e Hashimoto avança com comandos de Kiyotaka e Arisu. A escola anuncia **vitória de A, 4–3**, consequência de uma instrução recebida por Suzune que não correspondeu à entrada enviada por Kiyotaka. Tsukishiro revela a adulteração do canal por intervenção interna, feita de modo a parecer um lance plausível (pp. 429, 440–444). A diferença entre `original_input`, `validated_command`, `delivered_command` e `executed_move` torna-se essencial para auditoria, replay e responsabilidade.

Arisu exige revanche **privada**, partindo da posição antes da adulteração; Kiyotaka vence essa continuação (pp. 445–446). Ela aceita a superioridade naquele desfecho, enquanto ele ressalva as vantagens posicionais prévias e ambos tratam sua força global como incerta (pp. 447–450). **Não** corrigir retrospectivamente o resultado administrativo A–C. O texto evidencia: resultado oficial A 4–3 C, diferença parcial A +130/C −30, e retorno de C à colocação D após a ascensão da antiga D pela vitória sobre B.

### Short stories e material complementar (pp. 455–476)
Os contos da compilação incluem perspectivas privadas sobre o encontro e relacionamento de Arisu/Kiyotaka, Kei e Kushida, além de uma sessão de RPG em realidade virtual com Suzune, Ike, Sotomura e Kiyotaka. Seus diálogos e ações devem ser catalogados como perspectiva subjetiva ou **jogo virtual**; derrotar um monstro em VR não comprova feito de combate físico com animais reais. A fonte fan-compilada exige cuidado ao atribuir autoria e continuidade às side stories.

## 3. Exam Pack proposto

`data/canon/consolidated/exams/y1-event-selection.json` registra regras escolares, horários, validações de propostas, seleção por sorteio, autoridade dos comandantes, adjudicação de pontuação e diferenças entre fatos públicos e privados. O objeto permanece `authority=CANON_REFERENCE_NOT_EXECUTABLE` e `executable=false`.

**Subcontratos arquiteturais essenciais:** `EventProposal` (regras e desempate), `SchoolApproval` (jurisdição da escola), `CandidateShortlist` (cinco por classe), `EventDraw` (RNG determinístico + auditoria), `ParticipantAllocation` (disponibilidade e timeout), `CommanderIntervention` (ato autorizável), `ValidatedCommand` e `DeliveredCommand` (cadeia de custódia), `OfficialEventResult` e `UnofficialCounterfactual` (mundos/resultados diferentes). O simulador não deve ter código que assimile uma adulteração clandestina à ação regular de comandante.

## 4. Fronteira memória / biografia / crenças

- **Arisu (pré-matrícula):** viu Kiyotaka da área envidraçada, formou crença de talento nato e interesse pelo xadrez; a consciência infantil dela pode ser semeada em seu `background`, mas o menino não sabia que ela o observava. Nem o resultado de março nem os diálogos posteriores entram na memória inicial.
- **Hirata (pré-matrícula):** convívio com Sugimura, bullying, tentativa de suicídio sobrevivida com coma, coerção posterior e redistribuição escolar; não presumir que Kiyotaka conhece tudo no início. O relato ao colega é dado obtido **posteriormente à matrícula**, em evento privado da linha canônica; na simulação só se torna conhecido caso haja divulgação legítima equivalente.
- **Ryūen e Sakayanagi (pós-matrícula):** suas campanhas, planos, pressões e resultados são **casos de comportamento**, não instruções para repetir intimidação ou duelo em todo futuro alternativo.
- **Tsukishiro (autoridade):** decisões oficiais e ilícitos de canal ficam distinguidos. O fato de ter poder institucional não torna legítima a manipulação; a eventual atribuição de ordens a superiores precisa manter fonte e momento de aquisição.
- **Gêneros de evidência:** observação direta, lembrança, fala de terceiro, monólogo, pensamento do narrador, resultado escolar, relato de revisão ex-post. Um mesmo evento pode possuir versões incompatíveis que não devem ser forçadamente conciliadas.

## 5. Proatividade: exemplos executáveis apenas como testes de fidelidade

A simulação deve permitir que Kiyotaka **inicie** contatos para influenciar pareamento; Suzune organize alternativas e permita delegação; Arisu espere oportunidade para duelo, preveja tentativas de cooptação de Katsuragi e adapte escalações; Hiyori reúna colegas sem ordem externa; Ishizaki persista em solicitar Ryūen; Ryūen planeje passos prévios à avaliação. Contudo, cada iniciativa exige tempo lógico, conhecimento acessível, recursos e autoridade legítimos. Ações proibidas/abusivas devem ser observadas como casos literários e avaliadas por regras de ambiente, não promovidas como estratégias sempre válidas.

Criar teste diferencial em que Ryūen não é convencido e D perde, outro em que Ichinose reage cedo, e outro em que Katsuragi coopera apenas quando não há ameaça a terceiros. Isso evita que plano histórico vire roteiro inevitável.

## 6. Propostas de implementação e negativas

Issue **#49**: especificar resolvedor de seleção de eventos, pontuação, sorteios, recursos e segurança do canal; compatibilizar com **#45** (iniciativa) e **#36** (metas). Regressões obrigatórias: modalidade duplicada; falta de desempate; número de participantes duplicado; reuso de pessoa antes de todos participarem; comandante como jogador; comando fora de autorização; resultado adulterado promovido a oficial honesto; revanche privada alterando classe; aplicação de crença genética como ciência; memória do futuro; ações de terceiros atribuídas ao executor errado.

A classe pode receber `WAIT` intencional, mas o planejador também pode agendar preparação, conversa, sondagem e ajuste de plano conforme relógio e oportunidades. Nada disso autoriza salto de turno, arbitragem não especificada ou onisciência.

## 7. Registro de auditoria, lacunas e liberação

O registro `audits/y1-v11-review-register.json` enumera **160 unidades candidatas**. Não houve segundo avaliador independente com o PDF. A verificação automática de integridade dos JSONs **não** certifica fidelidade literária; cada item necessita revisão autoral, adversarial e externa antes de `verified:true`.

**Lacunas conhecidas:** identidade computacional canônica de Mii-chan/Wang Mei-Yu ainda ausente do índice atual; detalhes não narrados do RNG escolar; método exato de abuso do sistema interno em todos os sorteios; autenticidade editorial das histórias extras; pleno conhecimento dos funcionários da White Room; regras pós-exame sobre avaliação de cada participante. Preservar `REVIEW_REQUIRED` e não inventar solução.

**Estado de release:** Draft, sem merge automático, sem admissão de evidências ao motor, sem atualização automática de objetivos e sem executar regras do exame antes da revisão e autorização humana.
