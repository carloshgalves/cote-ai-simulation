# Canon Knowledge Base

Base canônica do universo de *Classroom of the Elite* usada para reconstruir o estado objetivo do mundo no início do primeiro ano e para alimentar evidência comportamental dos personagens.

## Onde está o quê

| Caminho | Conteúdo |
|---|---|
| `docs/canon/` | metodologia, taxonomia, políticas, digests legíveis por humanos |
| `data/canon/schema/` | JSON Schema dos tipos de registro |
| `data/canon/sources/works.yaml` | registro bibliográfico das obras |
| `data/canon/world/` | estrutura, regras e entidades não específicas do Y1 |
| `data/canon/y1/` | estado inicial do primeiro ano e linha de eventos canônica |
| `data/canon/evidence/` | paráfrases citáveis, indexáveis por RAG |
| `data/canon/conflicts/` | discordâncias entre fontes, com hipóteses |
| `data/canon/open_questions/` | lacunas que exigem leitura direta da light novel |
| `data/canon/derived/` | snapshots compilados consumidos pelo engine (gerados) |
| `data/models/` | **não é cânone** — reconstruções nossas |

## Ordem de leitura

1. [`methodology.md`](methodology.md) — como decidir `effective_from`, `known_by` e status
2. [`taxonomy.md`](taxonomy.md) — os três eixos
3. [`temporal-model.md`](temporal-model.md) — os quatro tempos e os quatro portões
4. [`source-tiers.md`](source-tiers.md) e [`provenance.md`](provenance.md)
5. [`conflict-policy.md`](conflict-policy.md)
6. [`gaps.md`](gaps.md) — o que ainda bloqueia

Decisão de arquitetura: [`docs/adr/0005-canon-knowledge-base.md`](../adr/0005-canon-knowledge-base.md).

## Estado atual

Todo o conteúdo hoje é `UNVERIFIED`. Foi localizado por fontes Tier 5 (wiki/comunidade) que apontam volume e capítulo da light novel, mas **nenhuma afirmação foi conferida diretamente em fonte Tier 0–1**. Nada compila para o engine neste estado.
