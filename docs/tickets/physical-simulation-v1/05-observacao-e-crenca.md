# PSV1-5 — Observação por observador e suspeita acumulada

**Spec:** §3.1 (`OBSERVATION`, `DETECTION`) · §7 · §14.6
**Modelo:** §8 (detecção e calibração), §10 (cliente B do estimador), §11 (nota sobre Glicko)
**Bloqueado por:** PSV1-4
**Bloqueia:** PSV1-6, PSV1-7

## Resultado observável

O mesmo comando do PSV1-4, agora com observadores:

```
python -m embodiment run-activity --run runs/demo/ --activity distance_run_3km \
    --field 40 --actor npc_017 --policy 'hold_rank(middle_third)' \
    --observers npc_004,npc_019,teacher_01 --world-seed 42
```

Cada observador recebe seu próprio `ObservedPerformance`, com canal, erro e pistas vazadas — não
existe "a observação" no singular. E cada um acumula log-odds de ocultação sobre `npc_017` no seu
belief state, com valores diferentes entre si.

Depois de **uma** corrida, nenhum deles tem suspeita decisiva. Depois de cinco corridas com margem
suspeitamente constante, o que assistiu a todas tem. É a diferença entre esses dois números que este
ticket entrega.

## Escopo

- **`observation.py`.** Emissão de `ObservedPerformance` por observador: canal de observação (viu de
  perto, viu de longe, ouviu falar não é observação), erro por canal, e vazamento probabilístico de
  pistas — respiração, recuperação rápida, técnica involuntária, e as `observable_cues` de uma lesão.
  Preenche o passo 9 do resolvedor, no ponto de extensão que o PSV1-4 deixou vazio.
- **`detection.py`.** Acumulador de **log-odds** de ocultação no belief do observador. Entradas:
  qualidade de inferência do observador, margem entre exibido e real, pistas vazadas, número de
  observações do mesmo alvo ao longo do tempo, atenção e suspeita prévia.
- **Calibração perto do acaso (F10).** Uma observação isolada fica perto do acaso, e há **teto de
  Δlog-odds por observação** — nenhum olhar decide a suspeita. A âncora é empírica: profissionais com
  dinamômetro e tentativas repetidas erram entre 47% e 69% das vezes ao detectar esforço insincero, e
  o coeficiente de variação da preensão, método padrão da área, não é válido. Um observador dentro do
  mundo não pode discriminar melhor que isso a partir de um olhar.
- **`CapacityBelief` sobre outro personagem** — o cliente B do modelo §10. Mesma semântica de
  restrição do PSV1-3, custo computacional diferente: atualização incremental, não SIR.
- **Atualização amortecida sob alta incerteza.** A atualização é **menor** quando a incerteza sobre o
  alvo é alta, porque pouca informação foi ganha — o comportamento do desvio de avaliação no Glicko,
  e literalmente o que se quer do `CapacityBelief` de um observador.
- **`observation-params.yaml`.** Erro por canal, razões de verossimilhança por pista, teto de log-odds
  por observação.

## Arquivos e módulos

```
src/embodiment/{types,observation,detection,effort,cli}.py
data/models/physical/observation-params.yaml     novo
tests/embodiment/{test_observation,test_detection_calibration,test_capacity_belief}.py
tests/embodiment/invariants/test_invariant_{01,03,07}_*.py
tests/embodiment/scenarios/test_scenario_01_deliberate_midpack.py
```

## Decisões que consome

Nenhuma da §13.

## Testes determinísticos

- **Uma observação por observador.** N observadores produzem N `ObservedPerformance` distintos, com
  erros distintos. Um teste falha se dois observadores em canais diferentes receberem o mesmo valor.
- **Calibração perto do acaso (F10).** Sobre muitos pares (ocultador, não-ocultador) com uma única
  observação, a discriminação fica dentro da banda declarada em `observation-params.yaml`, próxima do
  acaso. Um detector bom demais reprova o ticket tanto quanto um detector quebrado.
- **Teto por observação.** Nenhuma observação isolada, por mais extrema, move o log-odds além do teto.
- **Acúmulo funciona.** Cinco observações com margem constante produzem suspeita substancialmente
  maior que uma; o sinal vem do acúmulo, não da acuidade de um olhar.
- **Inteligência melhora inferência, não acesso (invariante 3 do `CONTEXT.md`).** Variar
  `inference_quality` do observador muda o posterior dele **sobre a mesma observação**; nunca muda a
  observação, nem lhe dá acesso a `BodyState` ou `ExertionIntent` do alvo. O teste varia a qualidade e
  assere que a entrada observacional é byte-a-byte igual.
- **Amortecimento sob incerteza.** Com `CapacityBelief` largo sobre o alvo, o Δ é menor que com
  crença estreita, para a mesma observação.
- **Pistas vazam probabilisticamente**, do substream nomeado, e o vazamento depende de dor e de
  esforço de ocultação — não é determinístico e não é gratuito.

## Cenários

**1 — o que termina no meio do pelotão de propósito.** Corre a 60% e chega 15º de 40. Um aluno comum
que chegou 15º a 100% gera **a mesma observação**, com verdade diferente. O teste assere igualdade das
observações emitidas nos dois casos; a metade do posterior — que o estimador também não os separa — é
asserida no PSV1-3. A simetria é o teste de que o modelo está certo, e por isso ela é verificada nos
dois lados.

## Checagens de fronteira de conhecimento

Este é o ticket onde a fronteira mais facilmente vaza, e as checagens são bloqueantes:

1. **O resultado do sorteio de detecção nunca chega ao ocultador** (classe 3 da spec §7.3). Ele
   percebe, no máximo, pistas secundárias — e "percebe" ali é outra observação, sujeita a erro. Teste:
   nada no estado nem no payload do ocultador muda em função do sorteio.
2. **A suspeita acumula no belief do observador, não no corpo do ocultador.** Teste: `BodyState` do
   alvo é idêntico com e sem observadores presentes.
3. **`ExertionIntent` de terceiro não é observável** (classe 4 do modelo §8). O observador vê
   desempenho e pistas; a intenção de ocultar é inferência com log-odds, nunca leitura.
4. **`BodyState` de terceiro só chega por `ObservedPerformance` e pistas visíveis.** Não há canal
   direto, e um teste de arquitetura recusa qualquer caminho de `observation.py` para o `BodyState`
   do alvo que não passe pela emissão.
5. **Log-odds são telemetria.** Vivem no event log e no belief state; nunca em payload de contexto.

## Fora do escopo

Interocepção e `SelfPhysicalModel` (PSV1-6) — este ticket trata da crença **sobre outro**. Observação
durante disputa (PSV1-7), que reusa este módulo por troca. Reputação de força, que é Social State e
não pertence ao subdomínio.

## Evidência de conclusão

1. O comando da seção de resultado emite um `observation.emitted` por observador, com canal, valor
   percebido e pistas vazadas.
2. `belief.concealment_updated` registra `observer_id`, `target_id`, Δlog-odds e total acumulado, com
   valores diferentes entre observadores.
3. O teste de calibração mostra a discriminação de uma observação isolada dentro da banda declarada, e
   o número aparece no relatório do run.
4. O cenário 1 passa: as observações emitidas para o ocultador e para o mediano são iguais.
5. As cinco checagens de fronteira acima têm teste nomeado e verde.
