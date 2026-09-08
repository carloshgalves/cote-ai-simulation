# ADR 0001 — Separar world truth de conhecimento dos agentes

**Status:** Accepted

## Contexto

Uma simulação estratégica quebra se um personagem recebe fatos secretos apenas porque o modelo ou prompt global os conhece.

## Decisão

Manter três camadas distintas:

- **World truth:** fatos objetivos conhecidos apenas pelo engine/árbitro.
- **Agent belief state:** proposições que o agente acredita, com confiança e proveniência.
- **Observation/event history:** fatos efetivamente percebidos ou comunicados ao agente.

O prompt/contexto de decisão de um agente é montado somente a partir das duas últimas camadas e de seu perfil canônico permitido.

## Consequências

- O engine precisa filtrar percepções antes de chamar o LLM.
- Memórias e crenças devem carregar proveniência.
- Evals devem testar vazamento de fatos secretos.
- “Kiyotaka é muito inteligente” nunca autoriza injetar respostas ocultas; inteligência muda inferência, não acesso.
