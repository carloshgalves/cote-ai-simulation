# Simulation models — NOT CANON

Reconstruções nossas: fórmulas, algoritmos, distribuições e preenchimentos de lacuna que a obra **não** declara.

## Regras

1. Todo registro aqui carrega `not_canon: true` e um `model_kind` explícito.
2. **Dependência unidirecional: cânone → modelos.** Nenhum registro sob `data/models/` pode aparecer em `provenance.supports` de um claim canônico. Um modelo pode citar claims canônicos como alvo de calibração; o inverso é proibido.
3. Um modelo nunca é apresentado, em prompt, documento ou saída, como regra da escola.
4. Modelos são calibrados **contra** o cânone, não derivados dele: as alocações canônicas conhecidas são fixtures de teste, não insumos da fórmula.

## `admission/`

`SIMULATION_ADMISSION_MODEL`. A obra afirma que existem cinco dimensões avaliadas e que a alocação em classes reflete mérito. Ela **não** declara pesos, agregação ou pontos de corte. Se construirmos um alocador, ele é invenção nossa e é identificado como tal.

Alvos de calibração: as alocações canônicas registradas em `data/canon/world/rules/admission.yaml` e as fichas de dimensões por aluno. Critério de aceite: reproduzir as alocações canônicas conhecidas sem que a fórmula seja jamais citada como cânone.

Enquanto `oq.admission.dimension-semantics` e `oq.admission.placement-basis` estiverem abertas, não construir o alocador.
