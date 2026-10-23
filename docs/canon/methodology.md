# Metodologia

Como uma afirmação vira um registro canônico.

## Fluxo

```
1. DESCOBRIR   fonte qualquer (wiki, anime, memória) → nunca vira suporte
2. LOCALIZAR   obra + volume + capítulo/cena onde a afirmação deve estar
3. CLASSIFICAR claim_kind · visibility · epistemic_status  (três eixos)
4. DATAR       effective_from como TimeBound, com origem UNKNOWN se desconhecida
5. ATRIBUIR    known_by / unknown_by, com canal e evento de aquisição
6. SUSTENTAR   supports[] em Tier 0–3 · discovered_via[] separado
7. VERIFICAR   leitura direta em Tier 0–1 → verified_by, verified_at
8. COMPILAR    somente VERIFIED, sem conflito aberto, chega ao engine
```

Etapas 1–6 podem ser feitas por um agente. A etapa 7 exige um humano com o volume aberto.

## As três perguntas obrigatórias

Para cada afirmação importante:

**1. Isso passou a ser verdade naquele momento, ou apenas foi revelado ao leitor naquele momento?**

Determina `effective_from`. Se a resposta for "apenas revelado", o momento da revelação vira `holds_by` e a origem fica `UNKNOWN` — não `SCHOOL_FOUNDING`.

**2. Quem sabia disso, e desde quando?**

Determina `known_by` / `unknown_by`. A resposta quase nunca é "todo mundo". Se ninguém do escopo sabia, `visibility: CONCEALED` e o claim entra em [`hidden-information.md`](hidden-information.md).

**3. O que sustenta isso, e o que apenas apontou para isso?**

Determina `supports` × `discovered_via`. Se a única resposta for wiki, anime ou memória do modelo, o status é `UNVERIFIED` e abre-se uma open question.

## Exemplos resolvidos

### Regra revelada tarde, vigente cedo

*"Cada aluno recebe mensalmente private points iguais a 100 × os class points da classe."*

- revelada à turma em maio do primeiro ano;
- mas a mesada de abril já foi calculada por ela, e a dedução do primeiro mês já operava;
- `effective_from: {mode: HOLDS_BY, holds_by: Y1_START, origin: UNKNOWN}`;
- `visibility_default: CONCEALED`; `unknown_by: [alunos do 1º ano até Y1_M02]`;
- pertence ao estado inicial do mundo, **não** ao conhecimento inicial dos alunos.

### Fato anterior à escola, revelado no fim

*"O White Room foi fundado como projeto secreto sob controle direto do governo."*

- `effective_from` muito anterior ao início do primeiro ano;
- `source_time` num volume tardio;
- `known_by` no `Y1_START`: um conjunto pequeno de atores nomeados;
- é `WORLD_TRUTH` no `Y1_START` sem ser conhecimento de praticamente ninguém.

### Enunciado sem operação observada

Uma regra que só aparece enunciada, sem o mundo jamais aplicá-la dentro do escopo:

- `holds_by` = ponto da revelação (é tudo que temos);
- abre-se `open_question` perguntando se vigorava antes;
- não entra no snapshot de `Y1_START` até a lacuna fechar.

## Fronteira com a simulação

O cânone descreve o que a obra afirma. Reconstruções nossas — fórmulas, algoritmos, distribuições, preenchimento de lacunas — vivem em `data/models/`, marcadas `not_canon: true`, e são **calibradas contra** o cânone em vez de derivadas dele.

A dependência é unidirecional: cânone → modelos. Nenhum registro sob `data/models/` pode aparecer em `provenance.supports` de um claim canônico.

O caso disciplinador é a admissão: a obra afirma que existem dimensões avaliadas e que a alocação em classes reflete mérito. Ela **não** declara uma fórmula. Se construirmos um alocador, ele é modelo de simulação, identificado como tal, validado contra as alocações canônicas conhecidas — nunca apresentado como regra da escola.

## Exceção de fonte aceita pelo operador

Para edições autorizadas pelo operador, a verificação bibliográfica Tier 0–1 não é exigência absoluta: é substituída por auditoria de passagem **na edição autorizada**, mantendo edition, locator, narrativa, `verified_by`/`verified_at` e bloqueios de conhecimento/conflito. A autorização da edição, isoladamente, não promove registros individuais a `VERIFIED` e o compilador atual só pode usar esses registros após adaptador/gate explícito. Detalhes: [operator-source-acceptance.md](operator-source-acceptance.md).

## Ingestão operator-approved V2

Ao receber uma nova edição autorizada pelo operador, a etapa de localização já pode usar diretamente seu texto, mas cada unidade deve ser registrada com localizador e classificação `NARRATED`, `SELF_DESCRIPTION`, `REPORTED_BY_CHARACTER` ou `INTERPRETATION`. Não inferir comportamento de um personagem apenas por aparecer mencionado. A depuração V2 separa `context_evidence_refs` de `behavioral_case_refs`.

O processo de ingestão passa a atualizar **um único read-model consolidado por personagem ou instituição**, com índice regenerado por ator/assunto, em vez de abrir sequencialmente todos os volumes. A versão da fonte autorizada e os registros atômicos por cena permanecem separados. A promoção a `VERIFIED` no contrato legado requer migração de gates e conferência individual; autorização de fonte não equivale a auditoria semântica.
