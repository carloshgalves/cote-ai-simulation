# Specs

Especificações implementáveis. Uma spec traduz um ADR aceito e seu modelo de domínio em algo que
`/to-tickets` consegue fatiar, e não reabre a decisão que a originou.

Toda spec traz: problema e objetivo visível; termos e invariantes afetados; escopo e não-escopo;
determinístico × dependente de modelo; mudanças de dados e estado; implicações de conhecimento e
visibilidade; modos de falha; observabilidade e reprodutibilidade; testes e evals; migração; e uma
seção própria de **decisões abertas** — que nunca ficam escondidas dentro dos critérios de aceitação.

| Spec | Status | Decisão de origem |
|---|---|---|
| [Physical Simulation V1](physical-simulation-v1.md) | Proposed | [ADR 0006](../adr/0006-physical-domain-model.md) |
| [Canon RAG V0](canon-rag-v0.md) | Accepted | [ADR 0001](../adr/0001-world-truth-and-knowledge-boundaries.md), [ADR 0002](../adr/0002-character-canon-rag-and-memory.md), [ADR 0005](../adr/0005-canon-knowledge-base.md) |
