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
| `data/canon/feats/` | desempenhos físicos observados, corpus **compartilhado** (não por personagem) |
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

## Estado atual — 2026-11-10

A frase anterior sobre *todo* o corpus provir exclusivamente de wiki/Tier 5 ficou obsoleta: há extrações com páginas e registros de revisão de PDFs fornecidos pelo operador, integradas ao corpus de referência até Y1 V11. **Extração e revisão pelo próprio autor/assistente não constituem auditoria independente nem validação de cada afirmação.** Ver as autorizações de edição e gates em `operator-source-acceptance.md`, os registros de revisão em `data/canon/consolidated/audits/` e os documentos de cada volume.

As issues #38 (V6), #39 (V7/V0) e #52 (V11) foram encerradas administrativamente (`not_planned`) por instrução do operador, **não** porque seus pareceres de fonte externos tenham sido realizados. Persistem `verified:false`, `engine_memory_admission:false`, `goal_runtime_auto_update:false` e `exam_executable:false` onde aplicáveis. A promoção para dados executáveis exige análise de origem, cronologia, divergência, divulgação e aprovação específica por registro; CI estrutural não substitui esse processo.
