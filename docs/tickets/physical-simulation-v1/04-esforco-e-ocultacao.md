# PSV1-4 — Esforço resolvido e ocultação viável

**Spec:** §3.1 (`EFFORT_RESOLVER`) · §6 (ordem normativa) · §11.1 · §14.4
**Modelo:** §8 (esforço escolhido e ocultação), §7 (risco de lesão)
**Bloqueado por:** PSV1-2
**Bloqueia:** PSV1-5

## Resultado observável

```
python -m embodiment run-activity --run runs/demo/ --activity distance_run_3km \
    --field 40 --actor npc_017 --policy 'hold_rank(middle_third)' --world-seed 42
```

resolve a corrida para todo o pelotão e devolve, para o participante que tentou ficar no terço médio,
o veredito da ocultação: `ok`, `forced_exposure` ou `forced_loss`. No `forced_exposure`, o event log
mostra `w_prime_balance` cobrado — acompanhar uma aceleração gasta reserva, e o que se gasta acima da
potência crítica não volta dentro da mesma prova. Ocultar numa prova disputada é literalmente caro.

Um intent impossível — correr com uma perna que o `BodyState` diz estar quebrada, sustentar
intensidade acima do disponível — sai como `effort.rejected`, com motivo, e **não** como um sucesso
reinterpretado.

## Escopo

- **`effort.py`, os dez passos da §6, nesta ordem.** A ordem é normativa; um resolvedor que produza os
  mesmos números em outra ordem está errado, porque a ordem é o que garante que o passo 5 seja o
  único lugar onde desempenho é decidido.
  1. `capability_available` **por dimensão** (do PSV1-2), nunca por escalar global.
  2. Validação do intent → inválido é **rejeitado**, nunca reinterpretado (`CONTEXT.md` inv. 5).
  3. Alvo = `capability_available × target_intensity`, truncado. `target_intensity` é fração da
     capacidade que o agente **acredita** ter; o engine trunca contra a real. É daí que sai a falha
     de template de pacing.
  4. Viabilidade do `display_ceiling` dado o campo → `{ok | forced_exposure | forced_loss}`.
  5. Realizado = alvo ⊗ ruído(substream) ⊗ decaimento de pacing ao longo da duração.
  6. Custos: Δfadiga periférica e central, Δsubstrato, Δhidratação, Δtérmico, Δ`cumulative_load`.
  7. Sorteio de lesão(substream).
  8. Atualização de dor.
  9. Emissão de observação por observador — **ponto de extensão entregue vazio**, com zero observadores
     registrados. O PSV1-5 o preenche sem reestruturar o resolvedor.
  10. Append no event log.
- **`ExertionIntent` como tipo**, com `target_intensity`, `pacing`, `display_ceiling`,
  `risk_acceptance`, `pain_override`, `hard_limits`.
- **`policies/scripted.py`.** `all_out`, `conserve(fraction)`, `hold_rank(band)`, `hide_injury`. São
  **fixtures de teste, não personagens**, e o módulo diz isso no topo do arquivo. Quando o Agent
  Cognition entrar, ele substitui a política e **não** ganha campo novo: um `ExertionIntent` de LLM que
  precise de um campo que a política roteirizada não tem é sinal de que autoridade narrativa vazou
  para a intenção.
- **Risco de lesão** pela fórmula do modelo §7: base(atividade) × carga(intensidade /
  `capability_available`) × estado × ambiente ÷ `injury_resilience` × histórico na mesma região.
  `pain_override` e `risk_acceptance` altos elevam a exposição porque removem o freio comportamental.
- **`Injury` completa**, com `impairments` por dimensão, `pain_profile`, `observable_cues`,
  `concealment` e cura ao longo do tempo — a cura é dinâmica do PSV1-2, o `Injury` que a alimenta é
  daqui.

## Arquivos e módulos

```
src/embodiment/{types,effort,cli}.py
src/embodiment/policies/scripted.py              novo
tests/embodiment/{test_effort,test_effort_rejection,test_concealment_feasibility,test_injury_roll}.py
tests/embodiment/properties/test_p2_cost_monotonicity.py
tests/embodiment/invariants/test_invariant_{01,05,07,10}_*.py
tests/embodiment/scenarios/test_scenario_05_island_survival.py
```

## Decisões que consome

Nenhuma da §13 diretamente. Consome a taxonomia de regiões fixada em 13.2 pelo PSV1-2.

## Testes determinísticos

- **Os dez passos, um teste por passo**, mais um teste de **ordem**: reordenar dois passos quebra
  pelo menos um teste. Sem isso, a ordem normativa é comentário.
- **Rejeição de intent impossível.** Intent que pressupõe estado inexistente — uma lesão que o
  `BodyState` não tem, um recurso ausente — produz `effort.rejected` com motivo tipado **(F13)**. Este
  é o validador que o Agent Cognition vai consumir quando entrar; ele é entregue aqui mesmo sem
  integração.
- **Os três vereditos são alcançáveis**, cada um com uma fixture de campo que o produz. Um veredito
  que nenhum teste alcança é código morto se apresentando como regra.
- **`forced_exposure` cobra reserva.** O `w_prime_balance` depois é estritamente menor que o de um
  `ok` equivalente. Um `forced_exposure` que não cobre reserva é bug.
- **Passo 5 é o único que escreve `PerformanceOutcome` (F1).** Teste de arquitetura: nenhum outro
  módulo constrói o tipo. E `CapacityProfile` não tem conversão para `PerformanceOutcome` em direção
  nenhuma.
- **Passo 1 compõe por dimensão (F12).** Um punho lesionado derruba preensão e arremesso e deixa
  corrida quase intacta, ponta a ponta pelo resolvedor — o PSV1-2 testou a composição, este testa que
  o resolvedor a usa.
- **Razão aguda:crônica não ressurge (F14).** Um teste falha se `acute_7d` ou `chronic_28d` forem
  lidos em `effort.py`. A métrica é matematicamente acoplada, instável a baixa carga crônica, sem
  efeito causal demonstrado, e a figura que a popularizou tem pedido formal de retratação. Ela saiu
  do modelo na revisão de 2026-09-09 e o teste existe para que ela não volte por descuido.
- **Truncamento contra a capacidade real.** Um agente que acredita ter mais do que tem produz alvo
  truncado, e o log registra os dois valores — sem isso, um desempenho estranho é indistinguível
  entre "corpo degradado" e "composição errada".
- **Custos escritos com `Δt` explícito.** Nenhum custo aplicado fora da API única de escrita do
  PSV1-2 (invariante 5).

## Property tests

- **P2 — monotonicidade de custo.** Maior `target_intensity` nunca produz menos custo fisiológico, em
  nenhum canal, para nenhum estado inicial válido.

## Cenários

**5 — exame de sobrevivência na ilha.** Dias de déficit calórico, sono ruim, calor e desidratação, com
esforço real aplicado a cada dia. O terceiro dia **começa** pior que o primeiro, e o canal responsável
é o substrato: com pouco carboidrato o glicogênio fica baixo por vários dias, e desidratação acima de
2% amplifica isso porque acelera o próprio gasto de glicogênio. E nenhum personagem sofre penalidade
térmica por ser adolescente **(F16)** — o teste falha se a diferença entre dois personagens de idades
diferentes, com o mesmo estado, for diferente de zero.

## Checagens de fronteira de conhecimento

- **O sorteio de detecção não existe ainda** (é PSV1-5), mas o sorteio de **lesão** já existe, e seu
  `p_lesao` é telemetria de engine: vive no `injury.rolled` do event log, nunca em payload de contexto
  (classe de vazamento 5 da spec §7.1).
- **`ExertionIntent` de terceiro não é observável.** Nada em `effort.py` expõe o intent de um
  personagem a outro; o que existe é desempenho e, a partir do PSV1-5, pistas.
- O resolvedor não constrói nenhum payload de agente. Se precisar construir, é sinal de que a
  fronteira foi cruzada no lugar errado.

## Fora do escopo

Emissão de observação por observador e detecção (PSV1-5). Interocepção (PSV1-6). Disputa (PSV1-7).
`ExertionIntent` escolhido por LLM — as políticas roteirizadas são fixture, e o ticket não abre espaço
para uma "política inteligente".

## Evidência de conclusão

1. O comando da seção de resultado roda e devolve os três vereditos conforme a configuração do campo.
2. `events.jsonl` contém `effort.resolved` com intent, `capability_available` efetivamente usado,
   alvo, realizado, veredito de ocultação e custos aplicados; e `effort.rejected` com motivo para o
   caso inválido.
3. `injury.rolled` registra `p_lesao` **e seus fatores separadamente**, mais o substream.
4. P2 verde sob `hypothesis`.
5. O cenário 5 passa com seed fixo, com a asserção de igualdade entre idades explícita.
