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
