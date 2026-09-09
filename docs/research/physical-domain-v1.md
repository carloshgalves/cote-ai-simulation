# Pesquisa — fundamentação da V1 do domínio físico

Data da pesquisa: **2026-09-08**. Alvo: [ADR 0006](../adr/0006-physical-domain-model.md) e
[`docs/architecture/physical-model.md`](../architecture/physical-model.md).

Coorte de referência da simulação: **adolescentes de ~15–18 anos**, escolarizados, mistos por sexo,
com e sem clube esportivo.

## 0. O que esta pesquisa fecha e o que ela não fecha

| Alvo | Situação após esta pesquisa |
|---|---|
| `UNIT_ANCHORS` | **desbloqueado com dado real transcrito** — tabela oficial de pontuação 12–19 anos em [`data/models/physical/fitness-test-score-table.yaml`](../../data/models/physical/fitness-test-score-table.yaml) |
| `BODY_DYNAMICS` | **estrutura e parametrização decididas**, com fontes e uma correção ao ADR (ver §7.1) |
| `CAPACITY_ESTIMATOR` | **maquinaria decidida** (censura por intervalo + amostragem por importância); depende de feats para produzir posteriores |
| `POPULATION_PRIORS` | **esqueleto desbloqueado**; restam duas lacunas de *sourcing* (§8.1) e uma de cânone (§8.2) |
| `oq.capability.*` | **não fecháveis por pesquisa externa** — exigem leitura Tier 0–1. O que esta pesquisa entrega é o protocolo de extração do §8.3, que transforma quatro perguntas em checklists executáveis |

Convenção de status usada abaixo, análoga à de [`docs/canon/methodology.md`](../canon/methodology.md)
mas aplicada a fontes externas:

- **[V]** verificado em fonte primária ou revisão por pares consultada nesta sessão;
- **[I]** inferência nossa a partir de [V];
- **[INT]** interpretação/decisão de projeto, não afirmação empírica;
- **[U]** não verificado — precisa de fonte antes de virar parâmetro.

## 1. Ancoragem de unidades e coorte de referência

**Decisão que informa:** qual bateria de testes define as âncoras observáveis de
`capacity-dimensions.yaml`, e de onde saem as normas por idade e sexo de `POPULATION_PRIORS`.

### Alternativas

| Alternativa | A favor | Contra |
|---|---|---|
| **新体力テスト / 体力・運動能力調査** (Agência de Esportes do Japão) | coorte exata (japoneses 12–19); itens casam 1-a-1 com nossas dimensões; publicação anual desde 1964 com média e desvio-padrão por idade e sexo; tabela oficial de pontuação e de avaliação global por idade | tabelas numéricas de média/DP não baixam por automação (§8.1); coorte nacional ≠ coorte de uma escola seletiva (§8.2) |
| **ALPHA-Fitness / YFIT / FITNESSGRAM** | baterias validadas, farta literatura em inglês, valores de referência europeus (IDEFICS) | coorte errada; menos itens (tipicamente preensão + salto horizontal + vaivém 20 m); não cobre agilidade, arremesso, abdominais |
| **Valores normativos internacionais por item** (ex.: normas de vaivém 20 m em dezenas de países) | permite validação cruzada e cobre dimensões faltantes | fragmentado por item; não dá estrutura conjunta; risco de misturar protocolos |

### Achados

**[V] A bateria 12–19 anos tem nove itens**, sendo 持久走 (corrida de resistência) e 20mシャトルラン
(vaivém 20 m) um par de escolha: 握力 (preensão manual), 上体起こし (abdominais em 30 s), 長座体前屈
(sentar-e-alcançar), 反復横とび (deslocamento lateral repetido, 20 s), 持久走 **ou** 20mシャトルラン,
50m走, 立ち幅とび (salto horizontal parado), ハンドボール投げ (arremesso de handebol).

**[V] A tabela oficial de pontuação (項目別得点表) para 12–19 anos foi transcrita integralmente**
nesta sessão a partir da planilha do 実施要項 de 平成11年度 (1999), com correção de 19/04/1999,
redistribuída pelo editor 第一学習社. Ela é *invariante por idade* dentro de 12–19 e separada por sexo:
10 faixas de pontuação por item. A tabela de avaliação global (総合評価基準表), em contraste, **é**
específica por idade, com faixas A–E para 12, 13, 14, 15, 16, 17, 18 e 19 anos.

Consequências diretas, e é por isto que essa tabela vale mais do que parece:

1. **Ela ancora unidades sem inventar nada.** Cada dimensão nossa ganha um item com unidade física
   real e uma escala de referência de 10 níveis para a coorte certa.
2. **Ela dá equivalência entre itens.** Como 持久走 e 20mシャトルラン são pontuados na mesma escala,
   a tabela é uma tabela de equiparação: 10 pontos masculinos = 4'59" ou menos **ou** 125 voltas;
   5 pontos = 6'23"–6'50" **ou** 63–75 voltas. Isso resolve, dentro do próprio documento oficial, a
   normalização entre dois feats aeróbicos de modalidades diferentes.
3. **[I] Ela restringe a distribuição da coorte** mesmo sem média e DP: as faixas A–E por idade são
   critérios fixos aplicados à mesma população, então a fração em cada faixa é observável na
   publicação anual. Enquanto a série não for transcrita, isso é restrição, não estimativa.

**[I] Distâncias do 持久走:** os tempos da tabela (10 pontos masculino ≤ 4'59"; feminino ≤ 3'49") são
compatíveis com 1500 m masculino e 1000 m feminino, o padrão da bateria. A planilha transcrita não
declara as distâncias. **Não usar como fato até verificar no 実施要項 da Agência de Esportes.**

**[V] Onde estão as normas de média e DP**, verificado nesta sessão no portal e-Stat (pesquisa
`toukei=00402102`, `tstat=000001088875`): há três tabelas relevantes por ano fiscal —
`年齢別テストの結果` (resultados por idade), `年齢別体格測定の結果` (antropometria por idade) e
`学校段階別テストの結果` (por estágio escolar, que isola o ensino médio). Para o ano fiscal de 令和5,
os `statInfId` são respectivamente `000040216148`, `000040216155` e `000040216156`.

**[V] O download automatizado dessas tabelas está bloqueado.** O endpoint
`/stat-search/file-download?statInfId=…&fileKind=0` respondeu **404** a requisições automatizadas,
inclusive com cookie de sessão e `Referer` corretos. A transcrição exige navegador ou intervenção
humana. Isto está registrado como tarefa em
[`data/models/physical/SOURCING.md`](../../data/models/physical/SOURCING.md).

### Recomendação

Adotar a bateria japonesa como âncora primária de `UNIT_ANCHORS` e base de `POPULATION_PRIORS`.
Usar ALPHA-Fitness/YFIT apenas como validação cruzada e para dimensões que a bateria não cobre
(`reaction_time`, `pain_tolerance`, `injury_resilience`, `thermoregulation`, `recovery_rate`), que
permanecem sem âncora de bateria e devem declarar isso.

**Mapeamento dimensão → item**, adotado:

| Dimensão | Item da bateria | Unidade |
|---|---|---|
| `max_strength` | 握力 | kg de força de preensão |
| `strength_endurance` | 上体起こし | repetições em 30 s |
| `flexibility` | 長座体前屈 | cm |
| `agility` | 反復横とび | toques em 20 s |
| `aerobic_capacity` | 20mシャトルラン **ou** 持久走 | voltas **ou** s |
| `sprint_speed` | 50m走 | s (→ m·s⁻¹) |
| `anaerobic_power` | 立ち幅とび **e** ハンドボール投げ | cm e m |
| `body_mass`, `stature` | 体格測定 | kg, cm |
| `reaction_time`, `coordination`, `injury_resilience`, `recovery_rate`, `thermoregulation`, `pain_tolerance` | — sem item | ver §4 e §8.1 |

Nota de conflito com o modelo atual: `capacity-dimensions.yaml` ancora `agility` em "percurso
padronizado, em s" e `strength_endurance` em "abdominais, repetições". A bateria real usa 20 s de
deslocamento lateral (contagem, não tempo) e 30 s de abdominais. **Ajustar o enum ao item real**, ou
a âncora deixa de ser observável no mundo que estamos simulando.

## 2. Estrutura do prior populacional

**Decisão que informa:** como `POPULATION_PRIORS` representa uma coorte, de modo que um sorteio
produza um corpo coerente e não catorze números independentes.

### Alternativas

| Alternativa | Avaliação |
|---|---|
| **Marginais independentes por dimensão** | mais simples, e errado de um jeito visível: produz o aluno simultaneamente pesado, mais rápido de todos e melhor no vaivém. A cauda conjunta fica absurdamente povoada |
| **Cópula gaussiana sobre marginais publicadas** | preserva exatamente as marginais oficiais e injeta correlação; exige a matriz de correlação, que a publicação não traz (§8.1) |
| **Modelo de fatores latentes (1–2 fatores) + resíduos** | poucos parâmetros, interpretável, e casa com a estrutura conhecida das baterias de aptidão: um fator geral de aptidão mais um eixo de porte/força × resistência |

### Recomendação

**Dois fatores latentes mais resíduo por dimensão, implementado como cópula gaussiana** sobre as
marginais oficiais. Um fator geral de aptidão; um fator de porte, que carrega positivamente em
`max_strength`, `body_mass`, `anaerobic_power` e negativamente em `aerobic_capacity` relativo.
Marginais vêm da publicação (bloqueadas em §8.1); as cargas fatoriais são **[INT]** enquanto não
houver a matriz de correlação, e o arquivo do prior precisa dizê-lo em `evidence_sufficiency` do
próprio prior, não só dos personagens.

Covariáveis de condicionamento admissíveis, porque são as únicas que o cânone pode informar: ano,
sexo, pertencimento a clube esportivo (e qual), reputação atlética declarada, porte descrito. Nada
além disso — condicionar em qualquer outra coisa é inventar cânone pela porta dos fundos.

## 3. `CAPACITY_ESTIMATOR`: de restrições a posterior

**Decisão que informa:** qual maquinaria estatística converte `population_prior ⊗ constraints[]` em
`capacity_posterior`, e como `evidence_sufficiency` deixa de ser adjetivo.

### O problema tem nome na estatística

O §6 do modelo físico descreve, sem usar os termos, um problema clássico: `LOWER_BOUND` e
`UPPER_BOUND` são **censura por intervalo**, e um parâmetro observado apenas por limites inferiores é
**parcialmente identificado** — os dados restringem um conjunto, não um ponto, e a cauda superior é
determinada pelo prior, não pela evidência. **[V]** Essa é uma situação estudada, com tratamento
bayesiano padrão (verossimilhança de intervalo; em JAGS/Stan, o idioma `dinterval`) e com uma
literatura específica sobre inferência dirigida ao *conjunto de identificação* em vez de a um ponto.

Isso é uma boa notícia para o ADR 0006: "cauda superior larga é a descrição correta da nossa
ignorância" não é uma escolha estilística nossa, é o comportamento esperado de um estimador correto
sob identificação parcial.

### Alternativas de implementação

| Alternativa | Avaliação |
|---|---|
| **Truncar o prior ad-hoc** | rápido, mas não combina restrições de tipos diferentes nem propaga incerteza; não dá conta de `INSTRUMENTED_ESTIMATE` com erro de medida |
| **MCMC em PPL (Stan/PyMC/JAGS)** | correto e expressivo; custo é uma dependência pesada e uma fonte de estocasticidade fora do nosso esquema de substreams nomeados (invariante 11) |
| **Amostragem por importância sobre o prior** | sorteia N do prior com substream nomeado, pondera cada partícula pela verossimilhança das restrições, reamostra. Determinístico dado o seed, auditável partícula a partícula, sem dependência externa |

### Recomendação

**Amostragem por importância com reamostragem, para a V1.** O tamanho do problema — dezenas de
personagens × ~14 dimensões — não justifica MCMC, e o requisito de reprodutibilidade por substream
nomeado é mais fácil de honrar assim. Verossimilhança por tipo de restrição:

| Restrição | Contribuição para o peso da partícula |
|---|---|
| `LOWER_BOUND` | indicadora suavizada: capacidade desnormalizada ≥ limite, com margem de erro de desnormalização |
| `UPPER_BOUND` | indicadora suavizada no outro sentido — só existe com atestação maximal (§5) |
| `INSTRUMENTED_ESTIMATE` | verossimilhança gaussiana em torno do valor medido, com desvio = erro do instrumento **mais** um termo de ocultação possível |
| `COMPARATIVE` | verossimilhança logística sobre a *diferença de desempenho* no mesmo evento — Bradley-Terry (§6) — e nunca diretamente sobre capacidade; a transferência exige o `effort_assumption` já obrigatório no schema |
| `TESTIMONY` | peso 1 (não entra); roteado para o belief state do `testifier` |
| `NO_INFORMATION` | peso 1, por construção |

**`evidence_sufficiency` ganha definição operacional:** por dimensão,
`1 − sd(posterior) / sd(prior)`. Zero significa "isto é o prior com outro nome"; valores altos
significam que a evidência realmente moveu a distribuição. Um eval pode então recusar afirmações
fortes sobre personagens cuja `evidence_sufficiency` na dimensão em questão esteja abaixo de um
limiar. Alternativa considerada e descartada: divergência KL prior→posterior, mais principiada mas
menos legível em revisão, e não comparável entre dimensões com suportes diferentes.

**Encolhimento hierárquico:** com poucos feats, o posterior deve puxar para a média da coorte
condicionada, não para o meio da população geral. Isto é o que o README de `data/models/physical/`
já promete e é o comportamento padrão da amostragem por importância sobre um prior de coorte — nada
extra a construir, desde que o prior seja o condicionado.

## 4. `BODY_DYNAMICS`: canais, constantes de tempo e efeitos sobre desempenho

**Decisão que informa:** quais canais de estado corporal existem, com que equação evoluem, e como
cada um degrada quais dimensões.

### O erro que evitamos: um único "stamina"

Um reservatório único não representa nenhum dos cenários de estresse do §14 do modelo: privação de
sono degrada precisão sem degradar força; desidratação degrada aeróbico antes de qualquer outra
coisa; DOMS aparece 24 h depois do esforço que o causou. Canais separados com constantes de tempo
separadas são o requisito mínimo.

### Achados por canal

**Fadiga aguda dentro de um evento — [V] modelo de potência crítica e balanço de W'.**
Dois parâmetros: `CP`, a intensidade sustentável, e `W'`, um trabalho finito disponível acima dela.
Acima de `CP`, `W'` se esgota; abaixo, se reconstitui exponencialmente, e **a constante de tempo de
reconstituição depende de quão baixa é a intensidade de recuperação**: ~377 s recuperando a 20 W,
~452 s no domínio moderado, ~580 s no domínio pesado. É o modelo certo para pacing dentro de uma
prova, e é exatamente o que dá custo material ao passo 4 do §8 do modelo (viabilidade de
`display_ceiling`): permanecer no pelotão quando ele acelera **gasta W'**, e W' gasto não volta na
mesma prova.

**Fadiga entre dias — [V] com ressalva forte.** O modelo *fitness-fatigue* de impulso-resposta
(Banister) propõe dois traços exponenciais com constantes distintas — aptidão lenta, fadiga rápida.
As revisões consultadas são duras: estimativas de parâmetros imprecisas, mau condicionamento,
parâmetros de interpretação prática difícil (um estudo achou correlação positiva entre testosterona
e a função de fadiga, quando se esperava negativa) e incapacidade de prever desempenho futuro.
**[INT] Recomendação:** adotar a *forma* (dois exponenciais com constantes distintas) como
contabilidade de estado consistente, e **não** apresentá-la como preditora validada de desempenho.
Registrar isso no arquivo de parâmetros; é honesto e evita que alguém no futuro cite o modelo como
se fosse física estabelecida.

**Risco de lesão — [V] a razão aguda:crônica deve sair do modelo.** O ADR 0006 §7 e o §7 do
modelo físico listam "razão aguda:crônica alimenta risco de lesão". A literatura recente é
inequívoca contra: acoplamento matemático entre numerador e denominador, viés de confundimento,
instabilidade quando a carga crônica é baixa, ausência de evidência de efeito causal, e um pedido
formal de retratação/correção da figura do "sweet spot" que popularizou a métrica. Manter a ACWR
seria construir sobre um número que a própria área está retirando. **Substituição recomendada:**
carga recente absoluta *relativa à `capability_available` do próprio corpo* — que o modelo já usa
como driver principal — mais histórico de lesão na mesma região, sob o enquadramento
**dinâmico-recursivo** de etiologia (cada exposição repetida altera o risco, por adaptação ou
maladaptação; a lesão não é causada só pelo evento incitante). Ver §7.1.

**Sono — [V] modelo de dois processos.** Processo S (homeostático, exponencial: sobe acordado,
desce dormindo) interagindo com o processo C (circadiano). É o arcabouço padrão há três décadas e
mapeia diretamente nos campos `sleep.debt_hours`, `hours_since_wake` e `circadian_phase` já
previstos no `BodyState`.

Especificidade da coorte, e ela importa: **[V]** adolescentes têm atraso de fase circadiana e
acumulam pressão homeostática mais devagar conforme amadurecem; a recomendação é de 8–10 h; horários
escolares cedo produzem privação crônica sistemática. Numa escola com internato, isso não é
detalhe de cor — é a linha de base do corpo de todo mundo.

**[V] Efeito do sono sobre desempenho, por dimensão** — meta-análise de privação de sono
(diferenças médias padronizadas): controle de habilidade **−0,87**; resistência aeróbica **−0,66**;
potência explosiva **−0,63**; velocidade **−0,52**; força máxima **−0,35**. Desempenho submáximo é
mais afetado que máximo. Isso **confirma numericamente** a afirmação qualitativa do §7 do modelo
("`max_strength` é quase insensível a privação de sono; precisão e expressão de técnica não são") e
fornece os multiplicadores relativos por dimensão sem que precisemos inventá-los. Use-se a razão
entre SMDs, não os valores absolutos: são efeitos de privação aguda em protocolos de laboratório.

**Hidratação — [V] limiar de ~2% de massa corporal.** Revisão de 34 estudos / 60 observações de
resistência: 41/60 (68%) significativamente prejudicadas com perda ≥ 2% da massa corporal, e 53/60
(88%) na direção do prejuízo. Mecanismos: queda de volume plasmático e débito cardíaco, menor fluxo
sanguíneo muscular e cerebral, maior temperatura corporal, **maior uso de glicogênio** e maior
esforço percebido. **[INT] Forma funcional recomendada:** desprezível abaixo de 2%, degradação
progressiva acima, com o efeito amplificado por carga térmica — e com acoplamento explícito para
`substrate_availability`, já que a desidratação acelera o gasto de glicogênio.

**Energia / substrato — [V].** A depleção de glicogênio muscular é fator principal no início da
fadiga em esforço prolongado; a taxa de degradação cresce **exponencialmente** com a intensidade; e
com dieta pobre em carboidrato o glicogênio **permanece baixo por vários dias**. Esta última é a
peça que faz o exame de sobrevivência na ilha funcionar como exame de dias, e não como sequência de
cenas independentes: racionamento não é um modificador do dia, é um estado que persiste.

**Dor muscular tardia (DOMS) — [V].** Início 12–24 h após o esforço, pico entre 24 e 72 h,
resolução em ~7 dias, com prejuízo mensurável de potência concêntrica durante a janela de 24–72 h.
Existe **efeito de sessão repetida**: a mesma carga produz menos dano na segunda vez. **[INT]** O
canal `soreness` precisa portanto de atraso na entrada e de um flag de adaptação por região/atividade
— sem ele, um personagem que treina toda semana sofre como se fosse a primeira vez, para sempre.

**Especificidade etária, recuperação — [V] com ressalva.** Crianças e adolescentes resistem melhor
à fadiga em esforço intenso e **recuperam mais rápido** que adultos, treinados inclusive:
retorno mais rápido do equilíbrio ácido-base, menor pico de lactato e H⁺, reposição mais rápida de
fosfocreatina, cinética cardiorrespiratória mais rápida. **[I]** A evidência mais forte é
pré-púbere; para 15–18 anos o efeito é intermediário entre criança e adulto. O prior de
`recovery_rate` deve deslocar-se em relação à literatura adulta, e o arquivo precisa registrar que
esse deslocamento é interpolação, não medição.

**Termorregulação — [V] e contraintuitivo.** A visão de que jovens são termorregulatoriamente
inferiores **não se sustenta**: com hidratação adequada, não há diferença demonstrada de acúmulo de
calor, temperatura central, tolerância ao exercício ou vulnerabilidade a doença do calor. O risco é
governado por fatores modificáveis — intensidade, duração, hidratação, razão trabalho:descanso — em
relação às condições ambientais, tipicamente indexadas por WBGT (temperatura de globo e bulbo úmido).
**[INT]** Consequência de projeto: **não** aplicar penalidade térmica por ser adolescente. Indexar
`thermal` em WBGT e razão trabalho:descanso, e deixar a variação individual em `thermoregulation`.

**Deriva de capacidade ao longo do ano — [V] com ressalva de população.** Destreinamento: VO₂max
começa a cair por volta do 10º dia; ~6% em 4 semanas, ~19% em 9 semanas, 20–25% em 11 semanas em
corredores; a força se preserva bem mais que o aeróbico, e jovens retêm força melhor que idosos.
**[I]** Estes números vêm de atletas que cessam treino; para um aluno comum a amplitude é menor.
Servem para fixar a **ordem de grandeza e a ordenação** da deriva de `capacity_baseline`
(aeróbico rápido, força lenta), não valores literais.

### Tabela de canais recomendada para a V1

| Canal | Escala de tempo | Dimensões que degrada, em ordem | Fonte |
|---|---|---|---|
| `W'` / fadiga anaeróbica intra-evento | s a min (τ ≈ 380–580 s) | `anaerobic_power`, `sprint_speed` | potência crítica |
| `peripheral_fatigue` | h | `max_strength`, `strength_endurance` na região | impulso-resposta (forma) |
| `central_fatigue` | h a dias | `coordination`, `reaction_time`, expressão de técnica | impulso-resposta + sono |
| `sleep.debt` + fase circadiana | dias | habilidade ≫ aeróbico ≳ potência > velocidade ≫ força | dois processos + meta-análise |
| `energy.substrate` | h a **dias** | `aerobic_capacity`, `strength_endurance` | glicogênio |
| `hydration.deficit` | h | `aerobic_capacity` (limiar 2%), amplificado por calor | revisão de desidratação |
| `thermal` | min a h | todas, via WBGT e razão trabalho:descanso | termorregulação juvenil |
| `soreness` (DOMS) | atraso 12–24 h, pico 24–72 h, ~7 d | `anaerobic_power`, `max_strength` | DOMS + efeito de sessão repetida |
| `injuries` | dias a meses | multiplicadores por dimensão, já no schema | modelo dinâmico-recursivo |
| `capacity_baseline` (deriva) | semanas | aeróbico rápido, força lenta | destreinamento |

## 5. Evidência de esforço máximo e submáximo sem virar certeza

**Decisão que informa:** a semântica de `effort.attestation` no corpus de feats e a de inferência de
esforço dentro da simulação (§8 e §10 do modelo).

Esta é a pergunta mais carregada do domínio, porque é onde a tentação de transformar um estado
fisiológico não observável em um booleano é maior. A literatura ajuda de um jeito específico:
**ela mostra que profissionais com instrumentos e tentativas repetidas não conseguem fazer isso.**

### Achados

**[V] Detecção clínica de esforço insincero é ruim.** O coeficiente de variação da força de preensão
— método padrão em avaliação de capacidade funcional — **não é válido**: o aumento de CV sob esforço
submáximo é artefato da redução do torque, não aumento real de variabilidade. Nenhum ponto de corte
entre 2,5% e 22% oferece sensibilidade e especificidade adequadas (o corte "tradicional" de 15% dá
sensibilidade 0,55; o de 11% dá especificidade 0,74). Taxas de erro do teste de cinco posições, do
*rapid exchange grip* e do CV vão de **47% a 69%**.

**[V] Critérios de "esforço máximo" em teste de VO₂max são inválidos.** Critérios secundários —
razão de trocas respiratórias, percentual da FC máxima — são satisfeitos muito antes da exaustão
volitiva, em intensidades tão baixas quanto **61% do VO₂max**. O platô de VO₂ é inconsistente. Alguns
autores propõem a rejeição completa dos critérios secundários. O que funciona razoavelmente é a
**fase de verificação**: um segundo esforço, de carga constante, após recuperação curta.

Essas duas descobertas se combinam num princípio de projeto, e ele é forte:

> **Sinais de esforço são os "critérios secundários" da ficção.** Ofegar, cambalear, suar, "dar
> tudo" — no mundo real, o análogo disso é satisfeito a 61% do máximo. Nada disso pode licenciar um
> `UPPER_BOUND`.

O schema de feats **já** exige `NARRATED_MAXIMAL` ou `SELF_REPORTED_MAXIMAL` para `UPPER_BOUND`, e
`STRAIN_CUES_PRESENT` já é um valor separado que não qualifica. A pesquisa **confirma a regra** e
fornece a razão empírica que faltava no comentário do schema. Recomenda-se documentá-la ali.

**[V] Esforço percebido é mensurável como percepção, não como telemetria.** O método session-RPE
correlaciona-se moderada a fortemente com carga interna medida (r ≈ 0,74 com frequência cardíaca;
CR-100 r = 0,80 > CR-10 r = 0,69), mas escalas derivadas de adultos são menos adequadas a
adolescentes, e RPE colhido durante ou logo após o esforço tende a superestimar a fadiga. Ou seja:
é exatamente o tipo de sinal que o §9 do modelo quer entregar ao agente — informativo, ordinal,
enviesado.

**[V] Pacing é regulado contra um *template* antecipatório de RPE.** O modelo de Tucker: o RPE
momentâneo é comparado com o RPE esperado naquele ponto da prova, construído a partir do
conhecimento do ponto final e de experiências anteriores; ajustes de ritmo derivam do desvio.
Teleoantecipação, no arcabouço de St Clair Gibson/Noakes.

**[INT]** Isto dá base principiada para duas coisas que o modelo já queria: `ExertionIntent.pacing`
é a escolha de um template, e a falha de viabilidade do `display_ceiling` é o template quebrando —
o agente montou o plano sobre a capacidade que *acredita* ter, e o pelotão real não cooperou.

**[V] Interocepção é individualmente variável e mal medida.** Até os testes-padrão de acurácia
interoceptiva cardíaca são questionados (21% dos indivíduos respondem por fase cardíaca, não por
detecção). Suporta `body_awareness` como parâmetro por personagem com erro genuinamente grande, e
adverte contra modelar autopercepção como quase verídica.

**[V] Ocultação de lesão é comportamento documentado e comum.** Entre atletas universitários com
histórico de concussão, **43%** relataram ter escondido sintomas deliberadamente para continuar
jogando; **22%** do total disse ser improvável relatar sintomas a um técnico no futuro. Lesões
recorrentes são frequentemente ocultadas e só se tornam visíveis quando pioram a ponto de causar
ausência. O cenário 4 do §14 não é licença dramática; é a linha de base do comportamento real.

### Recomendação: três camadas, nenhuma delas booleana

| Camada | Onde vive | Forma | Regra dura |
|---|---|---|---|
| `effort.attestation` | corpus de feats (cânone) | enum sobre *o que a prosa estabelece* | sinais de esforço nunca licenciam `UPPER_BOUND`; `UNKNOWN` é o padrão |
| `effort_evidence` | observação em simulação | **conjunto de pistas com razões de verossimilhança**, acumuladas em log-odds no belief do observador | nenhuma observação isolada pode saturar a crença |
| `SelfPhysicalModel.perceived_effort` | crença do próprio agente | ordinal tipo RPE, enviesado por `body_awareness` e analgesia | nunca é o valor de `target_intensity` do engine |

**Calibração recomendada [INT], ancorada em [V]:** se profissionais com dinamômetro e tentativas
repetidas erram entre 47% e 69% das vezes, a discriminação de uma observação isolada dentro do mundo
deve ficar perto do acaso. O sinal precisa vir do **acúmulo** — que é o termo "nº de observações do
mesmo alvo ao longo do tempo" que o §8 já prevê — e não da acuidade de um único olhar. Isto tem uma
consequência de design agradável: torna a suspeita um processo lento e social, e faz de um combate
(§6) o evento excepcional, por gerar muitas observações correlacionadas de uma vez.

**Proposta de adição ao schema:** valor `VERIFICATION_BOUT` em `effort.attestation` — um segundo
desempenho independente, em condições nas quais reter esforço não era viável (tipicamente após um
`forced_exposure`). É a tradução direta da fase de verificação de VO₂max, e é a única forma
*estrutural* — não retórica — de atestar máximo sem depender de narração ou autorrelato.

## 6. Nível de abstração para confronto físico

**Decisão que informa:** a granularidade do resolvedor de disputa do §11 do modelo.

### Alternativas

| Alternativa | Avaliação |
|---|---|
| **Simulador biomecânico golpe a golpe** | exige dados que não temos e produz precisão falsa; nada no cânone calibra isso; e a saída relevante para a simulação (quem controlou quem, quanto custou, o que os observadores viram) não depende dessa resolução |
| **Rolagem oposta única sobre um escalar de "combate"** | barato, e reintroduz o antipadrão do §1: colapsa capacidade, habilidade e estado num número. Não expressa objetivo assimétrico, não gera consequência em `BodyState`, e entrega uma única observação binária a quem assistiu |
| **Máquina de trocas com vetor de estado pequeno** | ~3–10 trocas; cada troca resolve com função logística sobre uma margem composta, consome reserva, pode emitir sorteio de lesão e emite observação |

### Recomendação: máquina de trocas

**[V] A função de ligação está resolvida pela literatura de comparações pareadas.** O modelo de
Bradley-Terry e o sistema Elo mapeiam a *diferença* entre forças latentes em probabilidade de vitória
por uma logística; é o formalismo padrão e bem compreendido. O Glicko acrescenta o desvio de
avaliação, isto é, incerteza sobre a força latente — e a atualização é menor quando o adversário tem
incerteza alta, porque pouca informação foi ganha. Isso é literalmente o comportamento que queremos
do `CapacityBelief` de um observador.

**A escolha decisiva é aplicar a logística por troca, não por luta.** Dela decorrem, sem
casos especiais, duas das propriedades que o §11 exige:

- disputas longas favorecem condicionamento, porque cada troca consome reserva e a degradação
  composta ao longo de muitas trocas domina;
- disputas curtas favorecem técnica e iniciativa, porque poucas trocas não dão tempo de a reserva
  importar.

**Margem de cada troca [INT], com termos justificados:**

```
margem = w_hab(Δcapacidade) · Δhabilidade
       + w_cap · Δcapacidade_relevante_à_modalidade
       + w_massa · Δbody_mass
       + termo_posicional (controle, distância, iniciativa)
       − penalidade(fadiga central, dor)          ← degrada habilidade mais que força
       + ruído(substream nomeado)
```

`w_hab` decrescente em `Δcapacidade` implementa "habilidade domina em diferenças pequenas de
capacidade, capacidade domina em diferenças grandes" como uma função, não como uma regra à parte.

**[V] Evidência de esportes de combate que sustenta os termos:** a fase de pegada consome cerca de
metade do tempo de combate no judô, e resistência de preensão é determinante para projeções,
imobilizações e finalizações — logo `strength_endurance` e preensão precisam ser dimensões de
primeira classe em modalidades de agarre, e não um detalhe. Condicionamento físico responde por
**até 45%** da variância entre lutadores bem e mal sucedidos — grande, mas longe de tudo, o que
justifica habilidade e estado carregarem o resto. Categorias de peso existem porque massa confere
vantagem; força absoluta é maior nos mais pesados e força relativa nos mais leves — o que torna
`body_mass` um termo explícito da margem e dá mecanismo concreto ao "capacidade domina em diferenças
grandes".

**Estado mínimo entre trocas:** controle/posição, distância, iniciativa, reserva de esforço por
participante (o `W'` do §4), dor, dano acumulado, e satisfação de objetivo por participante.
Terminação por objetivo atingido, reserva esgotada, lesão incapacitante ou intervenção de terceiro.

**Não modelar:** localização de golpe, alavancas articulares, física de impacto, ordem de iniciativa
por décimos de segundo.

**Propriedade emergente, e vale registrar que ela é derivada e não postulada:** como cada troca
emite observação, uma luta produz muitas observações correlacionadas do mesmo alvo em pouco tempo.
No acumulador de log-odds do §5, é justamente sob essa condição que a suspeita se torna decisiva.
O "lutar revela" do ADR 0006 §11 deixa de ser afirmação e passa a ser consequência.

## 7. Correções e adições propostas ao ADR 0006 e ao modelo físico

Nenhuma delas foi aplicada por esta pesquisa; todas exigem decisão.

### 7.1 Remover a razão aguda:crônica do modelo de risco de lesão — **recomendado**

ADR 0006 §7 ("razão aguda:crônica alimenta risco de lesão") e `physical-model.md` §7
(`cumulative_load.acute_7d` / `chronic_28d`, e o fator `estado(... razão aguda:crônica)` na fórmula
de `p_lesao`). A métrica é matematicamente acoplada, instável com carga crônica baixa, sem evidência
de efeito causal, e alvo de pedido formal de retratação da figura que a popularizou.

Substituir por: carga recente **relativa à `capability_available` do próprio corpo** (driver que a
fórmula já tem), histórico de lesão na mesma região (já presente), e enquadramento dinâmico-recursivo
— cada exposição repetida altera o risco, por adaptação ou maladaptação. Os campos `acute_7d` e
`chronic_28d` podem permanecer como *contabilidade de carga* para deriva de capacidade (§4); o que
deve sair é a **razão entre eles como fator de risco**.

### 7.2 Ajustar as âncoras de `agility` e `strength_endurance` ao item real — **recomendado**

反復横とび é contagem de toques em 20 s, não tempo em percurso; 上体起こし é repetições em 30 s.
Manter a âncora divergente do item real quebra a promessa do enum de que toda dimensão tem âncora
observável.

### 7.3 Adicionar `VERIFICATION_BOUT` a `effort.attestation` — **recomendado**

Ver §5. Único caminho estrutural para atestar esforço máximo sem narração nem autorrelato.

### 7.4 Documentar no schema por que sinais de esforço não licenciam teto — **recomendado**

O comentário do schema justifica a regra por analogia interna (`not_before_support`). Ela tem
justificativa empírica externa: critérios secundários de esforço máximo são satisfeitos a 61% do
máximo. Citá-la torna a regra defensável para quem chegar depois.

### 7.5 Tornar `evidence_sufficiency` operacional — **recomendado**

`1 − sd(posterior)/sd(prior)` por dimensão (§3).

### 7.6 Prior multivariado, não marginais independentes — **recomendado**

§2. Precisa constar no ADR, ou a primeira implementação vai sortear catorze números independentes.

### 7.7 `body_mass` como termo explícito da margem de disputa — **recomendado**

§6.

## 8. Lacunas restantes

### 8.1 Sourcing (não dependem de cânone)

| # | Lacuna | Estado |
|---|---|---|
| S1 | Média e DP por idade e sexo das tabelas `年齢別テストの結果`, `年齢別体格測定の結果` e `学校段階別テストの結果` | tabelas e `statInfId` identificados; **download automatizado retorna 404** — exige navegador ou humano |
| S2 | Matriz de correlação entre itens da bateria (ou cargas fatoriais publicadas) | **não localizada** nesta pesquisa; sem ela, as cargas do §2 são [INT]. Buscar em J-STAGE/CiNii antes de fixar parâmetros |
| S3 | Distâncias do 持久走 por sexo | [I] 1500 m / 1000 m; verificar no 実施要項 da Agência de Esportes |
| S4 | Verificação da tabela de pontuação contra a publicação oficial | transcrita de redistribuição por editora; conteúdo se identifica como 実施要項 de 平成11年度 com correção de 19/04/1999 |

Registradas em [`data/models/physical/SOURCING.md`](../../data/models/physical/SOURCING.md).

### 8.2 Nova questão de cânone: seletividade da coorte

A coorte da pesquisa nacional é a população escolar japonesa. A escola da obra é seletiva. Aplicar o
prior nacional sem ajuste assume que a seleção não correlaciona com aptidão física; aplicar um
"bônus de escola de elite" inventa cânone e viola a invariante 11 — que proíbe atributo escolhido à
mão — na escala da coorte inteira, que é pior do que na de um personagem.

Nova questão registrada: **`oq.capability.cohort-selectivity`**. Enquanto aberta, o prior usa a
coorte nacional **sem deslocamento**, e o arquivo do prior declara essa escolha como a hipótese nula
explícita, não como ausência de decisão.

### 8.3 `oq.capability.*`: protocolo de extração

Esta pesquisa não fecha nenhuma das quatro — todas exigem leitura Tier 0–1. O que ela entrega é o
que anotar, por desempenho encontrado, para que uma única passagem de leitura feche as quatro de uma
vez em vez de quatro passagens:

**Por desempenho físico encontrado no texto:**

1. quem, o quê, quando (âncora temporal), e **quem mais estava no mesmo evento** — sem o campo de
   competidores, uma colocação não significa nada;
2. forma da medida: quantidade com unidade / colocação em um campo / apenas prosa;
3. condições: ambiente, piso, calçado, carga, e **estado corporal declarado antes** do esforço —
   campo ausente significa desconhecido, nunca neutro;
4. **atestação de esforço**, e aqui está a única coisa que fecha `oq.capability.effort-attestation`:
   a prosa narra exaustão? o personagem declara ter dado o máximo? um terceiro afirma algo sobre o
   esforço dele? existe incentivo estrutural para ocultar? nenhuma das anteriores?
   → `STRAIN_CUES_PRESENT` **não** é `NARRATED_MAXIMAL`, e essa distinção é a pesquisa inteira do §5;
5. quem observou, e por qual canal.

**Para `oq.capability.fitness-test-records`:** se a escola aplica a bateria, registrar quais itens,
se resultados individuais ou classificações aparecem, e **se os alunos sabiam que o resultado seria
registrado** — a última é o que decide se ocultação tem um canal institucional a derrotar.

**Para `oq.capability.club-and-training-background`:** clube, desde quando, com que frequência, e
com que nível declarado. Isto entra em `skill_axes` e como covariável do prior — **nunca** como
capacidade.

**Para `oq.capability.physical-exam-tasks`:** por exame, quais demandas (ritmo sustentado, arranque,
força, exposição, privação de sono, racionamento) e por quantos dias. Isto decide quais dimensões o
engine precisa mesmo resolver e quais canais de `BodyState` carregam consequência entre eventos —
uma dimensão que nenhuma atividade consome não deveria existir.

## 9. Decisões que permanecem em aberto para o time

1. **Seletividade da coorte** (§8.2) — depende de cânone, mas a *política* enquanto aberta é decisão
   nossa. Recomendação: coorte nacional sem deslocamento, declarada.
2. **Correlações do prior** (§2 / S2) — construir a V1 com cargas [INT] declaradas, ou bloquear o
   prior até achar a matriz? Recomendação: construir com cargas declaradas; o custo de errar a
   correlação é muito menor que o de não ter prior nenhum, e `evidence_sufficiency` do prior torna
   isso auditável.
3. **Retirada da ACWR** (§7.1) — altera texto de ADR aceito. Recomendação: sim, com nota de revisão.

## Fontes

Bateria e normas japonesas:
- [新体力テスト実施要項 — スポーツ庁](https://www.mext.go.jp/sports/b_menu/sports/mcatetop03/list/detail/1408001.htm)
- [新体力テスト実施要項 — 文部科学省](https://www.mext.go.jp/a_menu/sports/stamina/03040901.htm)
- [新体力テスト実施要項・得点基準表 — 第一学習社](https://www.daiichi-g.co.jp/stest/feature/youkou/index.html) · [得点基準表 (xls)](https://www.daiichi-g.co.jp/stest/feature/tokuten/index.html)
- [体力・運動能力調査 — e-Stat](https://www.e-stat.go.jp/stat-search/files?toukei=00402102&tstat=000001088875) · [年齢別テストの結果](https://www.e-stat.go.jp/dbview?sid=0003288734) · [学校段階別テストの結果](https://www.e-stat.go.jp/dbview?sid=0003289206)

Baterias alternativas:
- [ALPHA-fitness test battery](https://www.academia.edu/14101911/_ALPHA_fitness_test_battery_health_related_field_based_fitness_tests_assessment_in_children_and_adolescents_)
- [Youth Fitness International Test (YFIT), consenso Delphi](https://www.sciencedirect.com/science/article/pii/S2095254624001704)
- [IDEFICS — referências europeias de aptidão física](https://www.nature.com/articles/ijo2014136)

Fadiga, carga e recuperação:
- [Assessing the limitations of the Banister model in monitoring training (PMC)](https://pmc.ncbi.nlm.nih.gov/articles/PMC1974899/)
- [The Fitness–Fatigue Model: What's in the Numbers? (IJSPP 2022)](https://journals.humankinetics.com/view/journals/ijspp/17/5/article-p810.xml)
- [The acute-chronic workload ratio-injury figure and its 'sweet spot' are flawed](https://www.researchgate.net/publication/333589357_The_acute-chronic_workload_ratio-injury_figure_and_its_'sweet_spot'_are_flawed)
- [Acute:Chronic Workload Ratio: Conceptual Issues and Fundamental Pitfalls](https://www.semanticscholar.org/paper/Acute:Chronic-Workload-Ratio:-Conceptual-Issues-and-Impellizzeri-Tenan/ede5743a426fd6429d28f8505500a3f771dbcf8b)
- [Modeling the expenditure and reconstitution of work capacity above critical power (Skiba 2012)](https://pubmed.ncbi.nlm.nih.gov/22382171/)
- [The W′ Balance Model: Mathematical and Methodological Considerations](https://pubmed.ncbi.nlm.nih.gov/34686611/)
- [W′ reconstitution modelling during intermittent exercise performed to task failure (PMC)](https://pmc.ncbi.nlm.nih.gov/articles/PMC12948861/)
- [Muscle fatigue during high-intensity exercise in children (Ratel et al.)](https://pubmed.ncbi.nlm.nih.gov/17123327/?dopt=Abstract)
- [Metabolic and Fatigue Profiles Are Comparable Between Prepubertal Children and Well-Trained Adult Endurance Athletes](https://pmc.ncbi.nlm.nih.gov/articles/PMC5928424/)
- [Effects of Short- and Long-Term Detraining on Maximal Oxygen Uptake in Athletes (PMC)](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC9398774/)
- [Delayed Onset Muscle Soreness — overview](https://www.sciencedirect.com/topics/neuroscience/delayed-onset-muscle-soreness)

Sono:
- [The two-process model of sleep regulation: a reappraisal (Borbély et al. 2016)](https://onlinelibrary.wiley.com/doi/10.1111/jsr.12371)
- [Sleep, circadian rhythms, and delayed phase in adolescence (Carskadon)](https://pubmed.ncbi.nlm.nih.gov/17383934/)
- [AASM position statement — delaying school start times](https://jcsm.aasm.org/doi/10.5664/jcsm.6558)
- [Effects of sleep deprivation on sports performance and perceived exertion: systematic review and meta-analysis (PMC)](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC11996801/)
- [Effects of Acute Sleep Loss on Physical Performance: A Systematic and Meta-Analytical Review](https://link.springer.com/article/10.1007/s40279-022-01706-y)

Hidratação, substrato e calor:
- [Dehydration: physiology, assessment, and performance effects (Cheuvront & Kenefick)](https://www.semanticscholar.org/paper/Dehydration:-physiology,-assessment,-and-effects.-Cheuvront-Kenefick/1a4094b8934b264ac73687f0ae6f08af473174cd)
- [Fluid intake strategies for optimal hydration and performance (GSSI)](https://www.gssiweb.org/docs/default-source/sse-docs/kenefick_sse_182.pdf?sfvrsn=2)
- [Glycogen availability and skeletal muscle adaptations with endurance and resistance exercise (PMC)](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC4687103/)
- [Restoration of Muscle Glycogen and Functional Capacity (PMC)](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC5852829/)
- [Thermoregulation during exercise in the heat in children: old concepts revisited](https://journals.physiology.org/doi/full/10.1152/japplphysiol.01196.2007)
- [Climatic Heat Stress and Exercising Children and Adolescents (AAP)](https://publications.aap.org/pediatrics/article/128/3/e741/30624/Climatic-Heat-Stress-and-Exercising-Children-and)
- [Hydration and thermal strain in youth sports (GSSI)](https://www.gssiweb.org/docs/default-source/sse-docs/bergeron_sse_158.pdf?sfvrsn=2)

Lesão:
- [A Dynamic Model of Etiology in Sport Injury: The Recursive Nature of Risk and Causation (Meeuwisse et al. 2007)](https://journals.lww.com/cjsportsmed/abstract/2007/05000/a_dynamic_model_of_etiology_in_sport_injury__the.11.aspx)
- [Sports-related concussion: anonymous survey of a collegiate cohort](https://pubmed.ncbi.nlm.nih.gov/24195017/)
- [What can family medicine providers learn about concussion non-disclosure from former collegiate athletes? (PMC)](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC6064086/)

Esforço, percepção e detecção:
- [The coefficient of variation as a measure of sincerity of effort of grip strength, Part I](https://pubmed.ncbi.nlm.nih.gov/11511012/) · [Part II: sensitivity and specificity](https://pubmed.ncbi.nlm.nih.gov/11511013/)
- [Is the coefficient of variation a valid measure for detecting sincerity of effort of grip strength?](https://pubmed.ncbi.nlm.nih.gov/12441559/)
- [The Maximal Oxygen Uptake Verification Phase: a Light at the End of the Tunnel?](https://sportsmedicine-open.springeropen.com/articles/10.1186/s40798-017-0112-1)
- [Is a verification phase useful for confirming maximal oxygen uptake? Systematic review and meta-analysis (PLOS One)](https://journals.plos.org/plosone/article?id=10.1371%2Fjournal.pone.0247057)
- [A meta-analysis of the criterion-related validity of Session-RPE scales in adolescent athletes (PMC)](https://pmc.ncbi.nlm.nih.gov/articles/PMC10422765/)
- [Pacing decision-making in sport and exercise: the regulation of exercise intensity (Konings & Hettinga)](https://repository.essex.ac.uk/22097/3/Konings%20Hettinga-SportsMed(3)cleancopy.pdf)
- [Individual Differences in Heartbeat-Tone Synchronicity Judgments… (PMC)](https://pmc.ncbi.nlm.nih.gov/articles/PMC13170068/)

Estimação e resolução de disputa:
- [On Bayesian modeling of censored data in JAGS (BMC Bioinformatics)](https://bmcbioinformatics.biomedcentral.com/articles/10.1186/s12859-021-04496-8)
- [Partial Identification of Expectations with Interval Data](https://arxiv.org/pdf/1802.10490)
- [Generalized Bradley-Terry Models for Score Estimation from Paired Comparisons](https://arxiv.org/abs/2308.08644)
- [A Bayesian approach to time-varying latent strengths in pairwise comparisons (PMC)](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC8136743/)
- [The Glicko system (Glickman)](https://www.glicko.net/glicko/glicko.pdf)
- [Strength and Conditioning for Grappling Sports (Strength & Conditioning Journal)](https://doi.org/10.1519/ssc.0b013e31823732c5)
- [Physical and Physiological Profiles of Brazilian Jiu-Jitsu Athletes: a Systematic Review](https://link.springer.com/article/10.1186/s40798-016-0069-5)
