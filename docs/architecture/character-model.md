# Character model

## Não usar “um prompt por personagem” como estado completo

Um personagem focal deve ter estado versionável e consultável. Exemplo conceitual:

```yaml
character_id: kiyotaka
canon_version: first-year-entry
core:
  values: []
  stable_traits: []
  behavioral_constraints: []
  capabilities: []
  blind_spots: []
background_knowledge: []
beliefs:
  - proposition: "..."
    confidence: 0.65
    source_event_ids: []
relationships: {}
current_goals: []
plans: []
```

A cada decisão, um **Context Builder** reúne apenas:

1. situação percebida agora;
2. memórias episódicas relevantes;
3. crenças e relações relevantes;
4. evidências canônicas úteis para aquela classe de situação;
5. regras/instrumentos que o personagem legitimamente conhece.

## Fidelidade do Kiyotaka

Não modelar fidelidade como `intelligence = 100`. Precisamos de evidências e rubricas para capacidades distintas: inferência, modelagem de adversário, autocontrole, planejamento, leitura social, tolerância a risco, tendência a ocultar capacidade, objetivos e disposição para agir.

A avaliação deve incluir **anti-cases**: situações em que uma resposta aparentemente “genial” seria in-character incorreta por ser exibicionista, onisciente, emocionalmente incompatível ou usar informação inacessível.
