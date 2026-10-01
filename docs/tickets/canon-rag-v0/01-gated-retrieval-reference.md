# CRAGV0-1 — Corpus vertical + gated retrieval reference

**Spec:** [canon-rag-v0.md](../../spec/canon-rag-v0.md)  
**Bloqueado por:** —  
**Natureza:** data contract + conformance implementation

## Resultado observável

Uma suíte determinística prova que:

1. o mesmo query/time para atores diferentes pode devolver evidence sets diferentes;
2. evidence proibida é eliminada antes do ranking;
3. facts UNVERIFIED não chegam à lane factual;
4. pós-divergência factual, spoiler e segredo de outro ator falham fechados;
5. cada item elegível permanece rastreável até evidence + source locator.

## Escopo

- CanonEvidence schema V2 e collection schema;
- seis evidence records focais, todos honestamente UNVERIFIED;
- manifest de corpus V0;
- golden suite com os oito cenários da spec;
- reference harness em `tests/canon_rag/`;
- tests determinísticos e métricas;
- documentação do corpus/evals.

## Fora do escopo

- `src/` de produção para RAG;
- alterar `pyproject.toml` com dependências de RAG;
- embeddings, vector DB ou framework;
- memória episódica/beliefs pós-seed;
- integração com CSF;
- verificar fontes Tier 0–1 por conta própria;
- resolver open questions;
- criar Character Core completo.

## Arquivos prováveis

- `data/canon/schema/evidence.schema.json`
- `data/canon/schema/evidence_collection.schema.json`
- `data/canon/evidence/focal-v0.yaml`
- `data/canon/evidence/README.md`
- `data/canon/rag-evals/v0-golden.json`
- `data/canon/rag-evals/README.md`
- `tests/canon_rag/reference_harness.py`
- `tests/canon_rag/test_rag_v0.py`

## Dependências

O harness usa apenas standard library e fixtures JSON. Ele não é um runtime público e não estabelece
linguagem para Canon Knowledge.

O knowledge scope é fornecido pelo request de teste. Um futuro seed builder/event-log adapter será
responsável por produzir `allowed_claim_ids`.

## Testes

- schema invariants;
- gate-before-rank;
- normal retrieval;
- future spoiler;
- wrong-character secret;
- post-divergence factual exclusion;
- open conflict exclusion;
- insufficient evidence + abstention;
- two-character contrast;
- no-evidence case;
- provenance completeness;
- deterministic manifest digest;
- corpus text size / paraphrase-only guard.

## Evals

O golden runner calcula hit/recall@K, reciprocal rank e irrelevant-context rate e zera a execução
se detectar qualquer leakage crítico.

## Knowledge-boundary checks

- `WRONG_ACTOR`;
- `CLAIM_NOT_ALLOWED`;
- `SPOILER`;
- `POST_DIVERGENCE_FACT`;
- `UNVERIFIED_FACT`;
- `OPEN_CONFLICT`;
- missing metadata fail-closed.

## Evidência de conclusão

- `pytest tests/canon_rag -q` verde;
- suite golden com zero leakage;
- corpus contém exatamente os seis atores focais na primeira fatia;
- diff não toca a branch/PR CSF nem `src/embodiment/`;
- implementation checkpoint commitado e enviado à branch `feat/canon-rag-v0`.
