# ADR 0010 — Stack do Simulation Engine causal V1

**Status:** Proposed

**Data:** 2026-10-08

**Relacionado:** ADR 0003 (tempo lógico), ADR 0006 (domínio físico), ADR 0007 (Python limitado ao
`Embodiment`), ADR 0008 (fundação causal) e ADR 0009 (codec e digests)

**Pesquisa:** [stack e persistência da CSF V1](../research/csf-runtime-and-persistence-v1.md)

## Contexto

A spec da Fundação causal deixou a linguagem do novo Simulation Engine em aberto. O ADR 0007 escolhe
Python somente para `Embodiment`; ele não autoriza transformar a linguagem do subdomínio físico na
linguagem do engine. O engine precisa executar reducers, fences, validação, ordenação, replay,
percepção e snapshots determinísticos, além de despachar trabalho concorrente sem transformar ordem
de completion em ordem causal.

O ADR 0009 também torna o engine potencialmente multilíngue: pelo menos duas implementações, em
linguagens diferentes, devem executar os 620 vetores/casos do bundle. Portanto a serialização default
da linguagem, o enum runtime, a iteração de map e o RNG do host não podem ser autoridade.

## Decisão proposta

Escolher **Go 1.27.2** para o Simulation Engine causal V1. A pesquisa de 2026-10-08 registra essa
revisão como a patch release corrente da linha suportada; o toolchain exato deve permanecer pinado no
primeiro ticket de implementação e no CI.

O core usará:

- tipos/records e funções puras próprios do domínio;
- `crypto/sha256`, sincronização e testes da standard library quando adequados;
- `fxamacker/cbor/v2` 2.9.2 como adapter CBOR avaliado, usando o modo RFC 8949 core deterministic
  como ponto de partida;
- validação adicional obrigatória contra `profile.json`, `foundation.cddl`, `registries.json`,
  `causal-transition-contracts.json`, Unicode 15.1.0 e os fixtures do bundle;
- `database/sql` somente através do port de persistência definido pelo ADR 0011, usando o driver
  pinado `modernc.org/sqlite` v1.60.1 no adapter SQLite V1;
- subprocesso local e framing de dados para consumir `Embodiment` Python.

O domínio não importa diretamente `fxamacker/cbor`, SQLite, Python, um cliente de LLM, framework de
agentes ou banco vetorial. Esses são adapters substituíveis.

## Fronteiras e autoridade

```text
domain/core → ports
                ├─ canonical codec adapter
                ├─ SQLite persistence adapter
                ├─ Embodiment process adapter
                └─ read-only Observatory adapter
```

Os ports preservam as seams da skill `codebase-design`:

- `WorldState`/simulation authority e `ReducerRegistry`;
- perception/disclosure e `PerceptionResolver`;
- agent context builder e model adapter somente fora do core causal;
- memory/retrieval ports somente como consumidores futuros de `KnowledgeInput`;
- exam rules/scoring como validators/handlers determinísticos;
- event log/snapshot e `ObservatoryQueries` read-only;
- eval harness sem cliente LLM.

Workers podem executar handlers, validators e resolvers contra uma revisão congelada. Só o
`CommitCoordinator` pode fechar a partição, ordenar `EventDraft`, derivar outputs causais e chamar o
port transacional. Ele não escolhe por latência do worker.

## Regras de determinismo exigidas

1. Nenhum map Go, goroutine, callback, scheduler ou channel decide ordem causal. Coleções passam pelo
   ordering/duplicate policy do bundle.
2. Nenhum RNG global ou wall clock entra em decisão. A implementação usa substreams nomeados,
   algoritmo/versão/hash fixados no genesis e `RngDraw` persistido.
3. Regras que exigem igualdade cross-language usam integer/fixed-point. Float permitido segue o
   profile V1 e recebe determinism eval em arquiteturas/toolchains relevantes.
4. Todo record causal é strict-decoded, re-encodado e comparado byte a byte antes de reducer/replay.
5. Respostas externas, inclusive do futuro Agent Cognition e do `Embodiment`, são dados admitidos por
   schema; nunca são autoridade direta sobre mundo, crença ou revisão.
6. O `Observatory` só depende de query ports; não recebe write port transitivo.

## Alternativas consideradas

### Python

Válido tecnicamente, com menor distância de dados ao `Embodiment` e bom ecossistema de testes. Não foi
escolhido porque a proximidade existente não é critério suficiente para ampliar o ADR 0007, e o core
teria de impor a mesma disciplina de bytes/imutabilidade sobre objetos Python. A fronteira de
processo com `Embodiment` já é explicitamente suportada pelo ADR 0007.

### TypeScript/Node.js

Válido tecnicamente e bom para IPC/ferramentas. Não foi escolhido como primeira autoridade porque
tipos são apagados em runtime e `number`/objetos JavaScript aumentam o risco de coerções silenciosas
no domínio canônico. Continua uma alternativa adequada para o segundo runner independente.

### Go

Escolhido por combinar concorrência explícita, binário portátil, SHA-256 padrão, testes de concorrência
e uma implementação CBOR RFC 8949 com modo determinístico documentado. Isso reduz risco operacional,
mas não elimina a necessidade do adapter/profile e dos 620 vetores.

## Consequências

Positivas:

- o core causal fica separado de Python sem reescrever `Embodiment`;
- a concorrência de cálculo pode ser ampla e o commit autoritativo permanece uma fronteira pequena;
- um binário e toolchain pinados simplificam CI, notebook e auditoria;
- Go e TypeScript/Node podem formar os dois runners independentes exigidos pelo ADR 0009.

Custos:

- novo toolchain e conhecimento na equipe;
- adapter próprio para codec, profile, Unicode e registries;
- framing/processo para o componente Python;
- determinismo numérico e RNG exigem testes próprios; não podem ser inferidos do runtime.

## Critérios de aceitação

- duas implementações em linguagens diferentes passam os 620 vetores/casos;
- permutações de workers, maps, callbacks e entradas irrelevantes não alteram bytes autoritativos;
- architecture tests provam que o core não importa cliente de LLM, framework, RAG ou Python;
- `Embodiment` só atravessa a fronteira via dados/processo e não escreve stores;
- replay/resume não chama modelo nem usa wall clock;
- mudança de toolchain/dependência sem policy/hash compatível falha fechado.

## Fora de escopo

Este ADR não escolhe framework de agentes, provedor/modelo de LLM, banco vetorial, UI, memória,
retrieval, regras de exame, schema do adapter físico ou política de migração do Embodiment. Também não
implementa o runtime.
