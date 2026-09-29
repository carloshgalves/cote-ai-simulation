# Auditoria — Canon Knowledge Base + RAG V0

**Data:** 2026-09-29  
**Branch:** `feat/canon-rag-v0`  
**Base auditada:** `main@a669ef6172b3025167baf3a84fe687458334fd9c`

## Escopo auditado

- `data/canon/schema/`
- `data/canon/sources/`
- `data/canon/evidence/`
- `data/canon/actors/`
- `data/canon/world/`
- `data/canon/y1/`
- `data/canon/exams/`
- `data/canon/feats/`
- `data/canon/conflicts/`
- `data/canon/open_questions/`
- `data/canon/derived/`
- `docs/canon/`

Leitura normativa: `AGENTS.md`, `CONTEXT.md`, `ROADMAP.md`, ADR-0001,
ADR-0002 e ADR-0005. A restrição de linguagem do ADR-0007 também foi auditada porque
uma implementação de RAG poderia, por acidente, generalizar Python para fora do
`Embodiment`.

Skills aplicadas: `agent-memory-rag`, `rag-evals`,
`knowledge-boundary-audit`, `character-fidelity` quando necessário,
`research`, `to-spec`, `to-tickets`, `implement` e `tdd`.

## Resultado por diretório

| Caminho | Estado | Autoridade / papel |
|---|---|---|
| `schema/` | real | Contratos de claims, tempo, proveniência, eventos, evidence, feats, conflitos e questões abertas. |
| `sources/works.yaml` | real | Registry de obras e tiers. Tier é autoria/forma, não canal. |
| `evidence/` | scaffold | Só README; ainda não há unidades RAG. |
| `actors/` | scaffold | Só README; não há registry nominal executável. |
| `world/` | real, porém UNVERIFIED | Claims institucionais e de mundo. |
| `y1/initial_state/` | real, porém UNVERIFIED | Claims e beliefs candidatos ao seed de Y1_START. |
| `y1/events/` | scaffold | Só política; nenhum evento canônico estruturado. |
| `exams/` | scaffold bloqueado | Depende de `oq.exam.disclosure-model`. |
| `feats/` | scaffold deliberado | Schema e metodologia existem, mas nenhum feat nominal foi transcrito. |
| `conflicts/` | real | Quatro conflitos versionados; alguns bloqueiam compilação. |
| `open_questions/` | real | 28 lacunas, todas abertas. |
| `derived/` | vazio deliberado | Artefatos gerados. Nada atual satisfaz os gates de compilação. |
| `docs/canon/` | real | Metodologia, taxonomia, tempo, tiers, proveniência, conflitos e gaps. |

## O que os contratos já resolvem

Não reabrir na RAG V0:

1. `CanonClaim` é a unidade atômica de verdade canônica estruturada.
2. `claim_kind`, visibilidade/conhecimento e `epistemic_status` são eixos distintos.
3. valid time, narrative time, knowledge time e transaction time são distintos.
4. `UNVERIFIED` não vira `VERIFIED` sem conferência humana em Tier 0–1.
5. wiki/fandom é descoberta/localização, nunca suporte silencioso.
6. records estruturados são autoridade; `derived/`, chunks, embeddings e índices são descartáveis.
7. toda leitura passa conceitualmente por:
   `effective_at → actor_gate → spoiler_horizon → divergence_gate`.
8. `known_by` canônico é autoridade apenas para o seed de `Y1_START`.
   Depois disso, o event log da simulação deve fornecer o conhecimento corrente.

## Lacunas específicas para RAG V0

### CanonEvidence

O schema V1 já preserva `id`, paráfrase, tempo de história, atores,
claims, tags situacionais e proveniência, mas ainda não expressa suficientemente:

- papel de recuperação (evidência comportamental vs. evidência de conhecimento);
- posição narrativa comparável para um spoiler gate determinístico;
- tópicos gerais separados de tags situacionais;
- invariantes que liguem uma unidade a exatamente uma passagem primária;
- política explícita de falha fechada para metadados incompletos.

Visibilidade não deve ser duplicada como nova verdade dentro de evidence. Para fatos, o gate
de conhecimento deve derivar dos claims autorizados pelo knowledge scope do request.

### Retrieval

Não existe contrato provider-neutral para:

- request;
- resultado auditável;
- corpus manifest;
- gate trace;
- context assembly;
- abstention.

### Evals

Não existe suíte golden para hit/ranking, leakage temporal, leakage entre personagens,
divergência, conflitos, insuficiência ou completude de proveniência.

### Ordenação temporal e grupos de atores

O KB possui `TimePoint`/anchors, mas a RAG não deve inventar um novo relógio global para
resolver pontos ambíguos. O conformance harness V0 receberá uma ordem temporal versionada em
fixture/configuração e falhará fechado quando não conseguir comparar.

Também não existe ainda um actor registry capaz de expandir `group.first-years` em atores
nomeados. O retriever não deve adivinhar membership. O contrato recebe um
`knowledge_scope.allowed_claim_ids` já autorizado por um seed builder/event-log adapter.

## Decisões realmente abertas

Permanecem abertas e **não bloqueiam** a V0:

- linguagem/runtime de produção de Canon Knowledge/RAG;
- banco vetorial;
- provedor/modelo de embeddings;
- framework de agentes;
- reranker semântico;
- integração concreta com CSF/Cognition;
- expansão de grupos de atores;
- ingestion de corpus privado.

O ADR-0007 impede usar Python do `Embodiment` como precedente. Portanto qualquer Python
adicionado nesta trilha será apenas um **reference/conformance harness de testes**, sem API
de produção e sem dependência nova em `pyproject.toml`.

## Conclusão

A base não precisa de nova arquitetura de world truth. Precisa de uma camada de recuperação
derivada, rebuildable e fail-closed, com um contrato claro entre Canon Knowledge e o futuro
knowledge/event-log adapter. Esse é o escopo da spec `canon-rag-v0.md`.
