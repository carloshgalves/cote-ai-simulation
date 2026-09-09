# Canon data policy

O repositório público não deve armazenar cópias integrais de novels, scans, ebooks ou outros textos protegidos.

## Permitido na baseline

- perfis estruturados escritos por nós;
- fatos e resumos próprios;
- identificadores de volume/capítulo/cena para rastreabilidade;
- evidências comportamentais em paráfrase;
- pequenos trechos somente quando legalmente apropriado e necessário.

## Corpus privado/local

Caso exista um corpus de referência que o usuário possua legitimamente, mantenha-o em `data/canon/private/` ou `data/canon/raw/`, ambos ignorados pelo Git. Índices derivados ficam em `data/canon/index/` e também são ignorados.

O código deve funcionar com interfaces de corpus, sem exigir que o material protegido esteja no repositório.

## Estrutura

Esta política de corpus continua valendo. A partir do ADR 0005, o diretório também abriga a base canônica estruturada:

| Caminho | Conteúdo |
|---|---|
| `schema/` | JSON Schema dos tipos de registro |
| `sources/works.yaml` | registro bibliográfico; tier segue autoria e forma, não canal de distribuição |
| `world/` | estrutura, regras e entidades não específicas do primeiro ano |
| `y1/` | estado inicial do primeiro ano e linha de eventos canônica |
| `actors/` | stubs de entidade (sem dossiers nesta fase) |
| `exams/` | cânone dos exames especiais |
| `evidence/` | paráfrases citáveis para RAG |
| `feats/` | desempenhos físicos observados; corpus compartilhado, indexado por ator |
| `conflicts/` · `open_questions/` | discordâncias e lacunas |
| `derived/` | snapshots compilados consumidos pelo engine (gerados) |

Metodologia e políticas em [`docs/canon/`](../../docs/canon/). Decisão em [`docs/adr/0005-canon-knowledge-base.md`](../../docs/adr/0005-canon-knowledge-base.md).

**Reconstruções nossas não vivem aqui.** Fórmulas e algoritmos que a obra não declara ficam em [`data/models/`](../models/), marcados `not_canon`. A dependência é unidirecional: cânone → modelos.
