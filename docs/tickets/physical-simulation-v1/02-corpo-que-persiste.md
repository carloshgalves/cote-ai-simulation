# PSV1-2 — Corpo que persiste no tempo

**Spec:** §3.1 (`BODY_STATE`, `BODY_DYNAMICS`, `CAPABILITY_AVAILABLE`) · §11.1 · §11.2 · §14.3
**Modelo:** §7 integral, com a tabela de canais e escalas de tempo
**Bloqueado por:** PSV1-1
**Bloqueia:** PSV1-4, PSV1-6

## Resultado observável

```
python -m embodiment advance-clock --run runs/demo/ --character npc_017 \
    --days 3 --sleep 4h --quality poor
```

avança o relógio três dias com sono ruim e mostra, por canal, o antes e o depois; e mostra
`capability_available` por dimensão no início e no fim. `coordination` e `reaction_time` caem
visivelmente; `max_strength` quase não se move. O event log ganha um `body.advanced` por avanço, com
os canais alterados e os valores dos dois lados.

Rodar o mesmo comando com `--days 0` não muda nada — e este é o ponto do ticket tanto quanto o
anterior.

## Escopo

- **`BodyState` tipado por inteiro** (modelo §7): `w_prime_balance`, `peripheral_fatigue` por região,
  `central_fatigue`, `cumulative_load`, `sleep`, `energy`, `hydration`, `thermal`, `soreness`,
  `injuries`, `illnesses`, `pain`. Domínios validados no tipo (`pydantic`): hidratação não fica
  negativa, `w_prime` não passa do máximo, `progress` de cura fica em `0..1`, severidade no enum.
- **`dynamics.py` — nove canais, funções puras `(state, Δt, env) -> state`.** A tabela do modelo §7
  menos a deriva de `capacity_baseline`, que é out-of-scope da V1. `cumulative_load.acute_7d` e
  `chronic_28d` são **contabilizados** e ninguém os lê: o campo existe, é atualizado, e nada depende
  dele nesta versão.
- **API de escrita única.** Só o engine escreve `BodyState`, e sempre com `Δt` explícito. Não existe
  assinatura que altere o corpo sem avançar o relógio.
- **`capability.py`.** `capacity_baseline ⊗ impairments(injuries) ⊗ estado ⊗ ambiente`, **por
  dimensão**. Um `impairment` que se aplique igualmente a todas as dimensões é bug, não simplificação.
- **`body-dynamics.yaml`.** Constantes de tempo por canal, ordenação de degradação, limiares. Carrega
  literalmente a nota de honestidade do ADR 0006: a forma de dois exponenciais para fadiga entre dias
  é **contabilidade de estado consistente, não preditor validado de desempenho**. O deslocamento de
  `recovery_rate` para 15–18 anos é **interpolação** declarada, não medição.
- **`snapshot.py`, segunda metade.** A seção `characters.<id>.body_state` entra no snapshot, íntegra,
  incluindo `illnesses: []` sem dinâmica (decisão 13.5).

## Arquivos e módulos

```
src/embodiment/{types,dynamics,capability,snapshot,cli}.py
data/models/physical/body-dynamics.yaml          novo
tests/embodiment/{test_dynamics,test_capability,test_body_invariants}.py
tests/embodiment/properties/{test_p1_scene_does_not_heal,test_p3_state_validity}.py
tests/embodiment/invariants/test_invariant_{04,05}_*.py
tests/embodiment/scenarios/test_scenario_03_exam_week.py
```

## Decisões que consome

13.2 — granularidade de `peripheral_fatigue` por região. O ticket implementa a taxonomia escolhida e,
se for a opção (a), **documenta a assimetria** entre a granularidade da fadiga e a da lesão no
cabeçalho de `body-dynamics.yaml`. Uma assimetria não documentada vira bug de leitura seis meses
depois.

13.5 — `illnesses` serializado sem dinâmica.

## Testes determinísticos

- **Um teste por canal:** evolução sob `Δt`, ponto fixo em repouso, monotonicidade na direção certa,
  saturação nos limites.
- **Escalas de tempo, com os números do modelo.** τ de `W'` na faixa 380–580 s, e **maior** quanto
  mais alta a intensidade de recuperação. Hidratação sem efeito abaixo de ~2% da massa corporal e
  degradante acima, acoplada ao substrato (desidratar acelera o gasto de glicogênio). DOMS ausente em
  t+6 h, presente em t+24 h, pico em t+48 h, resolvido em t+7 d **(F11)**.
- **Ordenação de degradação por privação de sono, como razão entre efeitos.** Habilidade ≫ aeróbico ≳
  potência > velocidade ≫ força, na ordem das diferenças médias padronizadas (−0,87 / −0,66 / −0,63 /
  −0,52 / −0,35). Testar a **razão**, nunca o valor absoluto — é só a ordenação que a meta-análise
  sustenta.
- **Substrato é o canal que faz um exame de dias ser de dias.** Com dieta pobre em carboidrato, o
  glicogênio permanece baixo por vários dias; o terceiro dia começa pior que o primeiro.
- **Efeito de sessão repetida.** A mesma carga produz menos DOMS na segunda vez, por região e
  atividade.
- **Composição por dimensão (F12).** Uma lesão de punho derruba preensão e arremesso e deixa
  `sprint_speed` quase intacta. O teste falha se qualquer dimensão não relacionada se mover.
- **Sem penalidade térmica por idade (F16).** Nenhum termo da dinâmica térmica lê idade. O risco é
  indexado a WBGT e a fatores modificáveis — intensidade, duração, hidratação, razão
  trabalho:descanso. A variação individual vive em `thermoregulation`.
- **`cumulative_load` é contabilidade morta.** Um teste falha se qualquer módulo **ler** `acute_7d` ou
  `chronic_28d` nesta versão. É o mesmo teste que impede a razão aguda:crônica de voltar por descuido
  (F14, cobertura completa no PSV1-4).

## Property tests

- **P1 — cena não cura (F2).** Para qualquer estado e qualquer sequência de operações **sem avanço de
  relógio**, `BodyState` sai idêntico. Uma noite mal dormida na segunda ainda está no corpo na quarta.
- **P3 — validade de estado (F15).** Nenhuma sequência de operações produz hidratação negativa,
  `w_prime` acima do máximo, `progress` de cura maior que 1 ou severidade fora do enum.

## Cenários

**3 — semana de provas.** Três noites de quatro horas: `sleep.debt` sobe, `central_fatigue` sobe,
`reaction_time` e expressão de técnica caem, `max_strength` quase não muda. Se `BodyState` fosse por
cena isso desapareceria e descanso deixaria de ser recurso administrável — o teste é a prova de que
não é.

## Checagens de fronteira de conhecimento

O `BodyState` deste ticket é world truth e **não tem consumidor de contexto**. Nenhum módulo aqui
produz payload de agente; a tradução para sinal qualitativo é o PSV1-6, e até lá qualquer função que
devolva número de `BodyState` para fora do engine é vazamento. O `body.advanced` do event log carrega
números — e o log nunca entra em prompt.

## Fora do escopo

Esforço, ocultação, custos de atividade, sorteio de lesão (PSV1-4). Interocepção (PSV1-6). Dinâmica de
doenças e deriva de capacidade (fora da V1).

## Evidência de conclusão

1. O comando da seção de resultado roda e imprime o antes/depois por canal e por dimensão.
2. `events.jsonl` contém `body.advanced` com os canais alterados e valores dos dois lados.
3. Um teste por canal, verde, com as escalas de tempo do modelo §7 asseridas explicitamente.
4. P1 e P3 verdes sob `hypothesis`, com o `@example` do caso `Δt = 0`.
5. O cenário 3 passa com seed fixo, asserindo a **razão** entre a queda de `coordination` e a de
   `max_strength`.
