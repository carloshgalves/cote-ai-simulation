# COTE AI Simulation

Simulação multiagente inspirada em *Classroom of the Elite*, com personagens persistentes, conhecimento isolado por agente, exames especiais determinísticos e uma linha do tempo capaz de divergir do cânone.

## Princípios

1. **O motor da simulação é a fonte da verdade.** LLMs propõem ações; não arbitram pontuação, regras, posse de itens, presença física ou fatos secretos.
2. **Personagem não é prompt.** Cada personagem combina perfil canônico, evidências comportamentais recuperáveis, crenças, memória episódica, relações e objetivos dinâmicos.
3. **RAG não é personalidade.** RAG recupera evidências relevantes para compor contexto; a identidade do agente e seus invariantes ficam estruturados e versionados.
4. **Conhecimento é local.** Nenhum agente recebe fatos que não poderia saber pela história da simulação.
5. **Exames são executáveis.** Regras, deadlines, pontuação, distribuição de papéis e condições de vitória devem ser determinísticas e testáveis.
6. **Tempo é lógico/event-driven.** 24 horas simuladas não equivalem a 24 horas reais; o relógio avança por eventos e cenas relevantes.
7. **O primeiro ano é o escopo inicial.** Ao final, fazemos snapshot/pause antes de introduzir o elenco do segundo ano.
8. **A simulação pode divergir do cânone.** O cânone fornece pessoas, regras e antecedentes; não força resultados que já deixaram de ser causalmente possíveis.

## Estado atual

Este bootstrap instala as Agent Skills e registra as primeiras decisões arquiteturais. Ainda não escolhe framework, provedor de LLM, banco vetorial ou stack de UI.

Veja `CONTEXT.md`, `docs/adr/` e `docs/research/ai-landscape.md`.
