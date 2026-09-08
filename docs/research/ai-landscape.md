# Pesquisa inicial — Agent Skills, RAG, evals e simulação multiagente

Data da pesquisa: 2026-09-08.

## Matt Pocock — `mattpocock/skills`

Mantém skills de engenharia como `domain-modeling`, `research`, `codebase-design`, `to-spec`, `to-tickets`, `implement`, `tdd`, `diagnosing-bugs`, `code-review` e `wayfinder`. O projeto já usou variantes adaptadas dessa família no IWrite e no AI Hub Oliveira & Nunes.

Fonte: https://github.com/mattpocock/skills

## Agent Skills open format

O formato Agent Skills usa uma pasta com `SKILL.md` e progressive disclosure: descoberta por nome/descrição, ativação sob demanda e recursos opcionais em `scripts/`, `references/` e `assets/`.

Fonte: https://agentskills.io/ e https://github.com/Open-Dot-Agents/SKILL.md

## Google DeepMind Concordia

É o framework mais próximo do problema de COTE encontrado nesta pesquisa: uma biblioteca para simulações sociais generativas com Entities, Components e um Engine/Game Master que resolve o ambiente. A separação entre intenção do agente e resolução pelo ambiente combina com nossa exigência de um árbitro determinístico.

Fonte: https://github.com/google-deepmind/concordia

**Recomendação:** prototipar/estudar Concordia antes de escolher framework definitivo. Não acoplar o domínio a ele nesta baseline.

## AgentScope

AgentScope oferece multiagente, memória, observabilidade, avaliação, skills e cenários de simulação distribuída. É mais geral e orientado a aplicações multiagente; pode ser comparado ao Concordia em um prototype, especialmente se escala/distribuição se tornar a dor principal.

Fontes: https://github.com/agentscope-ai/agentscope e https://github.com/agentscope-ai/skills

## Generative Agents

O trabalho clássico de Park et al. mostrou uma arquitetura de memória, reflexão e planejamento em uma pequena cidade de 25 agentes. A lição útil aqui não é copiar a implementação, mas manter observação, memória recuperável, reflexão e planejamento como componentes separáveis e avaliáveis.

Fonte: https://arxiv.org/abs/2304.03442

## RAG e evals

Fontes atuais de skills e metodologias reforçam dois pontos úteis:

1. avaliar retrieval separadamente da geração;
2. avaliar agentes multi-turn/trajectory e estado final do ambiente, não só respostas isoladas.

Referências:
- https://github.com/NVIDIA/skills/blob/main/skills/rag-eval/SKILL.md
- https://github.com/testland/qa/blob/main/plugins/qa-llm-evaluation/skills/ragas-evaluation/SKILL.md
- https://github.com/microsoft/eval-guide
- https://github.com/aws-samples/sample-agent-skill-eval

## Decisão desta baseline

Instalar skills de engenharia já comprovadas nos projetos anteriores e adicionar skills específicas para:

- memória/RAG de personagem;
- fidelidade comportamental;
- fronteiras de conhecimento;
- modelagem de exames;
- eval end-to-end da simulação.

Não instalar agora skills genéricas de deploy, wizard, merge-conflict, triage ou release gate: ainda não há infraestrutura/fluxo suficiente para justificá-las.
