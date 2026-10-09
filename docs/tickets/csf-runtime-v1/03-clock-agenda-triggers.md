# CSFRV1-3 — Clock, agenda e triggers duráveis

**Spec:** §7.4 · §9 (agenda/trigger/cascata) · §11.1 (5, 11)  
**ADR 0008:** invariantes 6, 17, 35, 38  
**Bloqueado por:** CSFRV1-2  
**Bloqueia:** CSFRV1-4

## Resultado observável

Partindo de `(t,1)`, o harness salta uma lacuna ordinal, cria um ciclo `CLOCK_ADVANCE` persistido até
`u`, aplica `body.advanced` sintético e materializa uma `TriggerActivation` com piso `(u,0)`. O ciclo
seguinte processa a ativação sem reavaliar memória do host e sem saltar uma pendência concorrente.

## Escopo

- `Clock`, `ClockState`, `next_cycle_coordinate`, confirmação condicional da menor pendência e
  `commit_successor_floor` derivado do plano.
- `ScheduledOccurrence` e lifecycle terminal; recorrência cria sucessor com nova identidade.
- `TriggerDefinition`, dependency footprint, runtime persistido, edge/once/repeat/rearm e
  `TriggerActivation(PENDING)`.
- Ativação e runtime entram na publicação da revisão de origem; o scaffold transacional usa a
  abstração escolhida no CSFRV1-0 e será fechado com o commit completo no CSFRV1-5.
- Limite versionado de cascata no mesmo instante e abort determinístico quando excedido.

## Arquivos e módulos prováveis

```text
runtime/csf/{clock,schedule_store,triggers,coordinator}
tests/csf/{clock,schedule,triggers}/
tests/csf/properties/{next_coordinate,trigger_lifecycle}/
tests/csf/scenarios/time_and_triggers.*
```

## Testes determinísticos

- De `(t,1)` para pendência `(t,3)` não cria commit em `(t,2)`.
- Pendência futura gera plano de avanço no instante de origem, input vazio, destino ordinal `-1` e
  próximo trabalho em `(u,0)`.
- Nova pendência Q entre t e u invalida condicionalmente plano provisório para P; nenhum salto ocorre.
- Outputs do avanço nunca recebem `(t,n+1)`; piso é `(u,0)`.
- Occurrence em `REJECT`/`DEFER` termina a identidade antiga; defer/recorrência cria sucessor.
- Trigger que permanece true ativa uma vez em edge/once e somente nas cadências de repeat.
- Dois eventos simultâneos que tocam dependências produzem `causing_event_ids` na ordem do batch,
  sem parent cross-candidate.
- Cascata excedida aborta, não trunca.

## Property tests

Para qualquer conjunto de pendências, `next_cycle_coordinate` é total, escolhe o mínimo elegível e
independe da ordem de registro. Permutar workers de predicate/footprint preserva runtime, activation
id/digest e coordenadas.

## Evals e fronteira de conhecimento

Nenhum eval de modelo. Predicate lê apenas dependências registradas de world state; trigger e agenda
nunca consultam belief, event store irrestrito ou wall clock. `body.advanced` usado aqui é fixture
sintética, não integração com `Embodiment`.

## Fora do escopo

Resolver ação/conflito, reducer global completo, percepção e adapter físico real.

## Evidência de conclusão

1. Trace persistido mostra advance em t, estado em u e ativação elegível em `(u,0)`.
2. Reexecução com ordem de worker invertida produz os mesmos records e digests.
3. Crash após revisão encontra a mesma activation pendente; não a recria nem duplica.
4. Suíte demonstra lacuna ordinal, pendência concorrente e cascade limit.
