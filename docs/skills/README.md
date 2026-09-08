# Skills selecionadas

As skills vivem em `.agents/skills/` e são carregadas por progressive disclosure.

## Engenharia — base já usada em outros projetos

- `domain-modeling`
- `research`
- `wayfinder`
- `codebase-design`
- `to-spec`
- `to-tickets`
- `implement`
- `tdd`
- `diagnosing-bugs`
- `code-review`

Essas skills seguem a disciplina que já usamos no IWrite/AI Hub e foram adaptadas à terminologia deste repositório. A principal referência upstream é `mattpocock/skills` (MIT): https://github.com/mattpocock/skills

## IA / simulação

- `llm-evals` — regressões de comportamento dependente de modelo/prompt.
- `rag-evals` — retrieval, grounding e isolamento de corpus.
- `agent-memory-rag` — desenho de memória/cânone/retrieval sem misturar linhas temporais.
- `character-fidelity` — fidelidade de personagem baseada em evidência e anti-cases.
- `knowledge-boundary-audit` — evita onisciência e vazamento de informação.
- `exam-modeling` — converte exames existentes em specs determinísticas.
- `simulation-evals` — avalia trajetórias, causalidade, reprodução e resultado do mundo.

## Deliberadamente adiadas

`grill-with-docs`, `prototype`, `handoff`, `triage`, `improve-codebase-architecture`, `resolving-merge-conflicts`, `wizard` e `release-gate` não entram na baseline. Podemos adicioná-las quando houver necessidade concreta, reduzindo sobreposição de gatilhos no começo.
