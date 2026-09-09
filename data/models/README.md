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

## `physical/`

`POPULATION_PRIORS` · `UNIT_ANCHORS` · `CAPACITY_ESTIMATOR` · `BODY_DYNAMICS`.

A obra mostra desempenhos físicos; ela não declara capacidades, curvas de fadiga, tempos de
recuperação nem risco de lesão. Priors populacionais e dinâmica corporal calibram contra fisiologia
real e não dependem de leitura canônica, portanto podem ser construídos agora. Posteriores de
capacidade por personagem, não: dependem de feats verificados e ficam bloqueados por `oq.capability.*`.

Proibido em qualquer hipótese: número de capacidade escolhido à mão, e qualquer registro afirmando que
um personagem supera outro. Ordem de capacidade é saída de estimação + amostra semeada + disputa,
nunca entrada. Ver [`physical/README.md`](physical/README.md) e o
[ADR 0006](../../docs/adr/0006-physical-domain-model.md).
