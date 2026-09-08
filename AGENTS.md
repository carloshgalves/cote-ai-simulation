# Agent instructions

Este repositório usa Agent Skills versionadas em `.agents/skills/`.

Antes de alterar arquitetura, domínio ou comportamento de agentes:

1. Leia `CONTEXT.md`.
2. Leia ADRs aceitos em `docs/adr/` relacionados à mudança.
3. Ative apenas as skills cujo `description` corresponda à tarefa.
4. Para mudanças em personagem, memória, RAG, exames ou isolamento de informação, use as skills específicas do projeto antes de implementar.
5. Nunca trate saída de LLM como fonte de verdade do mundo quando uma regra determinística puder decidir o resultado.
6. Nunca inclua corpus integral de light novels, scans ou material protegido no repositório público. Use metadados, resumos próprios e evidências legalmente disponíveis; corpus privado/local deve permanecer ignorado pelo Git.

A pasta `.agents/skills/` é a fonte canônica das skills do projeto.
