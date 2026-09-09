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

## Fluxo de implementação e revisão

Use três fases separadas; não misture responsabilidades entre elas:

1. `implement` — primeira implementação de um ticket/spec aceito. Implementa, valida, **commita** o checkpoint na feature branch e faz push quando possível. Não procura findings de review.
2. `code-review` — revisão independente do checkpoint. Garante uma Draft PR como superfície persistente e publica cada finding acionável em um comentário separado. Não corrige código.
3. `address-review-findings` — entra apenas quando findings já existem na Draft PR. Valida, corrige ou justifica cada finding, roda regressões, commita e faz push das correções na mesma branch.

Depois de `address-review-findings`, rode `code-review` novamente em sessão independente até a PR ficar limpa. Commit de implementação ou de correção nunca implica autorização automática para merge.
