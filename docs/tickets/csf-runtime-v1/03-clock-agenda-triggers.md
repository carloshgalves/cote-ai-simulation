# CSFRV1-3 — Clock, agenda e triggers duráveis

**Spec:** §7.4 · §11.1 (5, 11)<br>
**ADRs:** 0008 invariantes 1–2, 6, 13, 38 e cobertura preparatória de 17/35 · 0011
**Bloqueado por:** CSFRV1-2  
**Bloqueia:** CSFRV1-4

## Resultado observável

Partindo de `(t,1)`, o harness salta uma lacuna ordinal e publica um ciclo `CLOCK_ADVANCE` por uma
primitiva mínima, completa e única de `CycleCommit` SQLite. O mesmo commit contém eventos internos
sintéticos, revisão, trigger runtime e `TriggerActivation` com piso `(u,0)`. O ciclo seguinte
seleciona essa ativação como próxima pendência elegível, sem reavaliar memória do host nem
consumi-la antes do pipeline terminal entregue pelos CSFRV1-4/5.

## Escopo

- `Clock`, `ClockState`, `next_cycle_coordinate`, confirmação condicional da menor pendência e
  `commit_successor_floor` derivado do plano.
- `ScheduledOccurrence(PENDING)`, consulta ordenada e elegibilidade por coordenada; settlement,
  lifecycle terminal e sucessores de occurrence entram no CSFRV1-5.
- `TriggerDefinition`, dependency footprint, runtime persistido, edge/once/repeat/rearm e
  `TriggerActivation(PENDING)`.
- Primeira implementação do único port transacional do ADR 0011: um SQLite, uma transação curta e
  um `CommitCoordinator` mínimo para `CLOCK_ADVANCE`, com journal, eventos, runtime/ativação e
  revisão indivisíveis. Não existe store, envelope ou autoridade provisória.
- O subconjunto terminal deste slice fecha somente `CLOCK_ADVANCE` com input vazio. A ativação
  criada permanece `PENDING`: handler, `CommitCandidate`, settlement, lifecycle e consumo terminal
  entram depois da decisão mínima do CSFRV1-4, no CSFRV1-5. Não existe atalho ou mutação direta.
- O evento sintético de avanço é classificado de forma pinada como não perceptível neste slice; o
  conjunto de tasks é canonicamente vazio. Nova classificação exige policy/version e o caminho
  completo de percepção do CSFRV1-6.

## Arquivos e módulos prováveis

```text
internal/csf/{clock,schedule,trigger,coordinator,commit}/
internal/csf/persistence/sqlite/
internal/csf/{clock,schedule,trigger,commit}/**/*_test.go
tests/properties/{next_coordinate,trigger_lifecycle}_test.go
tests/scenarios/time_and_triggers_test.go
```

## Testes determinísticos

- De `(t,1)` para pendência `(t,3)` não cria commit em `(t,2)`.
- Pendência futura gera plano de avanço no instante de origem, input vazio, destino ordinal `-1` e
  próximo trabalho em `(u,0)`.
- Nova pendência Q entre t e u invalida condicionalmente plano provisório para P; nenhum salto ocorre.
- Outputs do avanço nunca recebem `(t,n+1)`; piso é `(u,0)`.
- Trigger que permanece true ativa uma vez em edge/once e somente nas cadências de repeat.
- Depois do commit de origem, a `TriggerActivation(PENDING)` é a próxima unidade elegível em
  `(u,0)`; este ticket não a expande em candidato nem a consome.
- Dois eventos simultâneos que tocam dependências produzem `causing_event_ids` na ordem do batch,
  sem parent cross-candidate.
- Falha antes/depois de journal, eventos, runtime/ativação ou revisão recupera o prefixo anterior ou
  o `CycleCommit` completo; nada parcial fica observável.
- Teste de arquitetura prova que somente o `CommitCoordinator` chama o port transacional e publica
  eventos/revisão.

## Property tests

Para qualquer conjunto de pendências, `next_cycle_coordinate` é total, escolhe o mínimo elegível e
independe da ordem de registro. Permutar workers de predicate/footprint preserva runtime, activation
id/digest e coordenadas.

## Evals e fronteira de conhecimento

Nenhum eval de modelo. Predicate lê apenas dependências registradas de world state; trigger e agenda
nunca consultam belief, event store irrestrito ou wall clock. `body.advanced` usado aqui é fixture
sintética, não integração com `Embodiment`.

## Fora do escopo

Expandir occurrence/ativação em candidato, resolver ação/conflito, settlement/lifecycle terminal,
reducer global completo, percepção e adapter físico real.

## Evidência de conclusão

1. Trace persistido mostra advance em t, estado em u e ativação elegível em `(u,0)`.
2. Reexecução com ordem de worker invertida produz os mesmos records e digests.
3. Failure injection prova atomicidade do primeiro `CycleCommit`; crash após commit encontra a mesma
   activation pendente, sem recriar nem duplicar.
4. Journal/event store/revisão validam o mesmo commit digest; não há mecanismo provisório.
5. Suíte demonstra lacuna ordinal, pendência concorrente e elegibilidade da ativação ainda
   `PENDING`, sem caminho antecipado de consumo.
