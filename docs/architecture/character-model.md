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

## `capabilities` não guarda números físicos

O campo `core.capabilities` guarda habilidades e disposições — técnica, formação, apetite a risco,
tendência a ocultar capacidade. **Não** guarda capacidade física. Capacidade é amostrada pelo engine
a partir de prior populacional mais restrições derivadas de feats canônicos, vive no world state e no
snapshot, e no máximo é referenciada aqui. Um segundo lugar guardando força seria uma segunda fonte de
verdade sobre o mesmo corpo. Ver [`physical-model.md`](physical-model.md) §12 e o
[ADR 0006](../adr/0006-physical-domain-model.md).

## Fidelidade do Kiyotaka

Não modelar fidelidade como `intelligence = 100`. Precisamos de evidências e rubricas para capacidades distintas: inferência, modelagem de adversário, autocontrole, planejamento, leitura social, tolerância a risco, tendência a ocultar capacidade, objetivos e disposição para agir.

A avaliação deve incluir **anti-cases**: situações em que uma resposta aparentemente “genial” seria in-character incorreta por ser exibicionista, onisciente, emocionalmente incompatível ou usar informação inacessível.
