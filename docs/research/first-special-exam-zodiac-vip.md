# Pesquisa — primeiro Special Exam executável

**Data:** 2026-09-29  
**Decisão informada:** qual exame deve ser o primeiro vertical slice do `Examination Engine` e quais regras podem ser formalizadas sem inventar cânone.  
**Resultado:** usar o **Cruise Ship / Zodiac / VIP Exam** do primeiro ano, Volume 4, sob um perfil executável `CANON_ADAPTED` até a verificação humana da fonte Tier 0–1.

## 1. Regra de evidência

Este documento segue ADR 0005 e `docs/canon/`: fonte de comunidade serve para **descobrir/localizar**, não para promover regra institucional a `VERIFIED`. Em 2026-09-29 o registry `data/canon/sources/works.yaml` ainda não contém `ln.y1.v04`, e `data/canon/exams/` contém somente o README.

Consequência: a pesquisa pode reconstruir uma candidata a regra, mas a branch **não altera o Canon KB** nem declara que a leitura Tier 0–1 foi feita. Regras detalhadas abaixo ficam `UNVERIFIED` até um humano abrir o Volume 4 e registrar `verified_by` / `verified_at`. Premissas necessárias para executar antes disso vivem em `data/models/exams/`, com `not_canon: true` e `SIMULATION_AUTHORED`.

Não foram armazenados trechos integrais do livro nem links para cópias não autorizadas.

## 2. Por que Zodiac/VIP

A editora licenciada descreve o Volume 4 como a segunda metade do teste de verão no navio, com os alunos das quatro classes divididos em doze grupos baseados nos signos do zodíaco e submetidos a um teste de raciocínio:
https://sevenseasentertainment.com/books/classroom-of-the-elite-light-novel-vol-4/

Comparação arquitetural:

| candidato | interfaces centrais exercitadas | subsistemas extras exigidos antes de o exame ser útil |
|---|---|---|
| **Zodiac/VIP (Y1 V4)** | segredo por agente, disclosure, comunicação, inferência, deception, coalizões, submissão, PP/CP, deadlines | quase nenhum além de conhecimento/comunicação/economia |
| ilha deserta (Y1 V3) | estratégia, segredo, recursos, liderança | mapa, localização, inventário, sobrevivência, saúde, ambiente, atividades físicas |
| Sports Festival (Y1 V5) | coordenação, desempenho, scoring | Embodiment integrado, provas físicas, equipes, calendário de eventos, lesões |
| Paper Shuffle (Y1 V6) | coordenação de classe, prova acadêmica, risco de expulsão | capacidade acadêmica, geração/validação de questões, pareamento, provas e anti-cheating |

Referências oficiais de contexto:  
- Volume 3: https://sevenseasentertainment.com/books/classroom-of-the-elite-light-novel-vol-3/  
- Volume 5: https://sevenseasentertainment.com/books/classroom-of-the-elite-light-novel-vol-5/  
- Volume 6: https://sevenseasentertainment.com/books/classroom-of-the-elite-light-novel-vol-6/

**Escolha fechada para esta trilha:** Zodiac/VIP. Ele testa as interfaces que tornam a simulação interessante sem transformar a primeira execução em teste de cinco subdomínios ainda imaturos.

## 3. Reconstrução candidata do cânone

A tabela não é um conjunto de claims `VERIFIED`; é backlog de leitura dirigida.

| id local | reconstrução em paráfrase | categoria | status agora | onde verificar |
|---|---|---|---|---|
| ZC-01 | o exame reúne alunos do primeiro ano em 12 grupos temáticos do zodíaco, misturando as quatro classes | regra/setup | `UNVERIFIED` (o número 12 e as quatro classes também aparecem na sinopse oficial) | Y1 V4, briefing inicial |
| ZC-02 | cada grupo tem exatamente um VIP/target secreto | regra secreta | `UNVERIFIED` | Y1 V4, briefing |
| ZC-03 | o participante recebe disclosure individual informando sua própria condição de VIP/não-VIP; a identidade dos demais não é entregue | disclosure | `UNVERIFIED` | Y1 V4, instruções recebidas por telefone |
| ZC-04 | grupos têm reuniões obrigatórias, duas por dia nos dias ativos, com conteúdo da conversa livre | agenda/comunicação | `UNVERIFIED` | Y1 V4, briefing |
| ZC-05 | há uma janela final após o encerramento; antes dela uma submissão válida pode terminar o grupo antecipadamente | deadline | `UNVERIFIED` | Y1 V4, regras de resposta |
| ZC-06 | VIP não pode submeter; resposta só vale para o próprio grupo e pelo dispositivo/canal autorizado | validação | `UNVERIFIED` | Y1 V4, regras de resposta |
| ZC-07 | Outcome 1: primeira resposta final válida correta encerra com recompensa de PP para o grupo, VIP recebendo o dobro | scoring | `UNVERIFIED` | Y1 V4, Outcome 1 |
| ZC-08 | Outcome 2: ausência de resposta final válida ou resposta final válida incorreta recompensa apenas o VIP | scoring | `UNVERIFIED` | Y1 V4, Outcome 2 |
| ZC-09 | Outcome 3: palpite antecipado correto por aluno de outra classe encerra o grupo; +50 CP à classe do respondente, -50 CP à classe do VIP, +500k PP ao respondente | scoring | `UNVERIFIED` | Y1 V4, Outcome 3 |
| ZC-10 | Outcome 4: palpite antecipado incorreto por aluno de outra classe encerra o grupo; -50 CP à classe do respondente, +50 CP à classe do VIP, +500k PP ao VIP | scoring | `UNVERIFIED` | Y1 V4, Outcome 4 |
| ZC-11 | palpite feito por colega de classe do VIP é inválido e não encerra o grupo | validação | `UNVERIFIED` | Y1 V4, Outcomes 3/4 e regra final |
| ZC-12 | resultado publicado não revela a identidade do VIP nem a do respondente/“traidor” | disclosure pós-exame | `UNVERIFIED` | Y1 V4, encerramento |
| ZC-13 | existem 159 participantes ativos, em nove grupos de 13 e três de 14, devido a uma ausência | setup observado | `UNVERIFIED` | Y1 V4, rosters |
| ZC-14 | há 12 VIPs, três por classe | balanceamento | `UNVERIFIED` | Y1 V4, briefing + rosters |
| ZC-15 | a escolha do VIP segue um padrão inferível: ordenar sobrenomes pela leitura japonesa e usar a posição ordinal do signo | mecanismo oculto | `UNVERIFIED`; forte reconstrução comunitária, ainda não `INFERRED` no sentido do ADR 0005 | Y1 V4, rosters + revelações posteriores |

Descoberta/triangulação, sem valor de `supports`:
- https://you-zitsu.fandom.com/wiki/Cruise_Ship_Special_Test
- https://you-zitsu.fandom.com/wiki/Light_Novel_Volume_4/Summary
- discussões comunitárias sobre o padrão de VIP foram usadas apenas para localizar a hipótese a verificar.

## 4. Comportamento observado que deve ser conferido separadamente

O resumo comunitário relata que grupos diferentes terminaram nos quatro outcomes, e que o agregado final de CP/PP é compatível com Outcomes 1–4. Isso é útil como **oráculo de regressão futuro**, não como suporte atual.

A verificação Tier 0–1 deve reconstruir, em separado:
1. outcome terminal de cada um dos 12 grupos;
2. classe do VIP e classe do submitter terminal;
3. deltas de CP por classe;
4. recompensas de PP;
5. se o agregado fecha exatamente com as regras anunciadas.

Não hardcodar esse resultado no resolver. O objetivo é que a mesma regra consiga reproduzi-lo quando alimentada pela trajetória canônica e produzir outro resultado quando as decisões mudarem.

## 5. Lacunas e conflitos relevantes à executabilidade

### G1 — Volume 4 não está no source registry
Dependência da Trilha B. Esta branch referencia conceitualmente `ln.y1.v04`, mas não edita `works.yaml`.

### G2 — cronograma textual
Fontes de descoberta divergem na forma de contar “terceiro/quarto dia” e em qual dia é livre. O resolver não precisa dessa interpretação: a spec usa âncoras relativas (`role_disclosure_at`, `active_close_at`, `final_window_open/close`, `result_publish_at`) e recebe os seis meeting slots no setup.

### G3 — ordenação de submissões
A reconstrução aponta que somente a primeira resposta válida do grupo decide. O cânone não fornece semântica para duas submissões no **mesmo instante lógico**. Host arrival order é proibida pelo ADR 0008. A V1 precisa de um desempate reprodutível, explicitamente `SIMULATION_AUTHORED`.

### G4 — efeito de uma submissão inválida sobre a cota individual
“Uma resposta por indivíduo” e “resposta inválida” não determinam, por si, se uma tentativa inválida consome a cota. A V1 escolhe que uma mensagem sintaticamente válida que chegou ao endpoint oficial **consome** a cota; falha de transporte/schema não consome. `SIMULATION_AUTHORED`.

### G5 — formação dos grupos
Não há algoritmo canônico verificado para gerar os 12 rosters. A V1 recebe `group_assignments` como setup institucional e valida a topologia; não inventa um agrupador.

### G6 — padrão oculto de VIP
O padrão kana × ordinal é central para a estratégia observada, mas ainda não está verificado. O perfil `CANON_ADAPTED` o usa como premissa de simulação versionada. Um perfil puramente canônico não poderá compilá-lo até a leitura Tier 0–1.

### G7 — ações proibidas externas ao menu do exame
Há indicação comunitária de regras contra coerção/roubo/uso não autorizado de telefone. O ExamSpec registra os hooks institucionais conhecidos, mas não modela “roubar um telefone”, “ameaçar” ou qualquer affordance física/social. Isso pertence à CSF; o exame apenas valida a submissão que chega ao endpoint e pode solicitar consequência disciplinar se a CSF provar uma condição relevante.

### G8 — Class Points
`PRIVATE_POINTS_ECONOMY.md` cobre PP, não define toda a semântica de CP. O exame produz deltas de CP; o owner econômico/institucional continua responsável por validar/aplicar o ledger autorizado.

## 6. Premissas `SIMULATION_AUTHORED` da V1

1. usar âncoras de tempo relativas em vez de resolver a ambiguidade terceiro/quarto dia;
2. aceitar roster institucional como input, exigindo a topologia canônico-adaptada;
3. usar a regra kana × ordinal para o perfil executável provisório, sem chamá-la de cânone verificado;
4. consumir a cota individual quando uma submissão bem-formada chega ao endpoint, mesmo se depois for invalidada por role/class/group;
5. resolver colisões no mesmo instante por uma prioridade pseudoaleatória derivada de `exam_seed + group_id + submission_id`, nunca por ordem de chegada;
6. tratar recompensas de PP como emissão institucional (`exam_reward`), não transferência entre estudantes.

Todas ficam no model file `not_canon`; mudar qualquer uma altera `model_version`/hash do run.

## 7. Sessão de leitura dirigida necessária para retirar o provisório

Abrir uma edição Tier 0 ou Tier 1 do Y1 Volume 4 e preencher, no mínimo:
- texto/paráfrase das regras e quatro outcomes;
- número e composição dos grupos;
- disclosure de VIP;
- horários e reuniões;
- semântica de primeira submissão e invalidação;
- pattern/critério de escolha do VIP, se declarado ou inferido no texto;
- proibições e expulsão;
- publicação final e anonimato;
- resultados por grupo para regressão.

Depois: Trilha B registra `ln.y1.v04`, cria CanonClaims, fecha/ajusta `oq.exam.disclosure-model`, e só então um perfil `CANON_VERIFIED` pode substituir as premissas correspondentes.
