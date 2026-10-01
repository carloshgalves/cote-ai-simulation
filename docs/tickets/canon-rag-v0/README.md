# Tickets — Canon RAG V0

**Spec:** [canon-rag-v0.md](../../spec/canon-rag-v0.md)  
**Decisões de origem:** ADR-0001, ADR-0002, ADR-0005

A V0 cabe em um tracer bullet porque o corpus é deliberadamente pequeno e o backend é somente
um oracle de conformidade. A implementação não cria um runtime de produção.

| ID | Título | Resultado |
|---|---|---|
| [CRAGV0-1](01-gated-retrieval-reference.md) | Corpus vertical + gated retrieval reference | Prova gates, diferenças por ator, abstention e leakage fail-closed. |

O próximo passo depois do checkpoint implementado é `$code-review CRAGV0-1` em uma sessão/fase
separada, conforme `AGENTS.md`.
