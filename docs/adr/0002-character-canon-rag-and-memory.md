# ADR 0002 — Personagem = identidade estruturada + evidência canônica + memória da simulação

**Status:** Accepted

## Contexto

Usar um único arquivo gigante por personagem ou simplesmente indexar a novel inteira mistura personalidade, fatos canônicos e memória da linha simulada. Isso favorece contradições e conhecimento impossível.

## Decisão

Cada agente focal será composto por:

1. **Character Core:** traços/invariantes, prioridades, vieses, competências e limites.
2. **Canon Evidence Store:** evidências comportamentais recuperáveis por situação; RAG consulta esta camada.
3. **Static Background Knowledge:** fatos do passado que o personagem legitimamente sabe no instante de entrada.
4. **Belief State:** hipóteses e convicções sobre o mundo atual, com confiança/proveniência.
5. **Episodic Memory:** eventos vividos nesta execução da simulação.
6. **Relationship State:** relações dinâmicas produzidas pela execução atual.
7. **Current Goals/Plans:** objetivos e compromissos em curso.

RAG é um mecanismo de seleção de contexto, não a definição da personalidade.

## Consequências

- O cânone pode orientar “como essa pessoa tende a decidir” sem forçar acontecimentos futuros.
- Ao divergir da novel, a memória da simulação prevalece sobre eventos canônicos posteriores.
- Evals podem medir fidelidade comportamental separadamente de groundedness da memória.
