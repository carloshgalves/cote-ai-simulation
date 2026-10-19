# CSFRV1-3 — checkpoint de implementação (19/10/2026)

**Situação:** IMPLEMENTATION IN PROGRESS. Este registro descreve somente o
checkpoint verificável da branch `feat/csfrv1-3-clock-agenda-triggers`;
não constitui homologação do ticket nem revisão independente.

**Base:** `main@7b7fc7edaf844ff6eeb637299fd6e00ec93f48db`.
**Última CI com sucesso antes deste registro:** commit `05bee0dce28670ccf3dda0e6c187154963ce173c`,
[workflow](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39302151724/job/118229828468).
`go test ./...` passou; conformidade canônica Go e TypeScript: 621/621 por runtime,
com relatórios comparados byte a byte.

## Componentes implementados

- `internal/csf/clock/`: `ClockState`, `CyclePlan`, mínimo lexicográfico,
  salto de lacuna ordinal, `CLOCK_ADVANCE`, `commit_successor_floor` e
  verificação condicional de plano. Nenhuma chamada de relógio de parede.
- `internal/csf/schedule/`: seleção de `ScheduledOccurrence(PENDING)`
  ordenada por coordenada e ID; não consome ocorrências antes do settlement.
- `internal/csf/trigger/`: transições puras de rising/falling edge, once e
  repeat com cadência positiva, provenance ordenada por `EventOrderKey`.
  O coordinator não executa callback de predicado de trigger fornecido pelo host:
  o predicado deste slice depende somente do instante lógico.
- `internal/csf/admission/`: derivação de fence `CLOCK_ADVANCE` com coorte
  vazia e source closure até a coordenada-alvo.
- `internal/csf/persistence/sqlite/`: autoridades para clock, occurrences,
  trigger runtime/activation e journal; `CommitClockAdvance` compara clock,
  prefixo exato de input e menor pendência na mesma transação que publica
  `CycleCommit` em `decision_ledger`, evento em `event_store`,
  estado de trigger, ativação e revisão. A fronteira de commit recalcula
  preimages de IDs/digests, provenance e relação do evento com o journal.
- `internal/csf/commit/`: construção de um lote temporal canônico, evento
  sintético não perceptível, commit com zero decisões e zero tasks e
  classificação de triggers/ativação para próximo ciclo.

## Evidência de testes já verde

1. Avanço `(t,1) → CLOCK_ADVANCE(t,2) → (u,-1)`, salto de lacuna sem
   commit decorativo e próxima pendência de ativação em `(u,0)`.
2. Rejeição de plano provisório quando aparece `Q < P` antes do commit.
3. Rejeição de input source retroativo e mismatch de autoridades.
4. Crash simulado em seis fases dentro da transação: não publica
   decisão, evento, runtime, ativação, revisão ou relógio parcialmente;
   retry da tentativa com o mesmo fence persiste exatamente um commit.
5. Reabertura SQLite recupera o mesmo clock, journal, evento e ativações;
   permutação da ordem dos workers conserva os bytes registrados.
6. Repetição armazena `next_repeat_at` como pendência temporal; testes de
   cadence, edge, once e seleção ordenada passam.
7. Lote adulterado (digest de journal, evento, provenance de ativação,
   transição de revisão) falha antes de alterar qualquer autoridade.

## Portões ainda necessários antes da revisão independente

1. **Autoridade durável da TriggerDefinition:** definições/policies e
   thresholds ainda são fornecidos pelo caller a `TriggerWork`, sem um
   registro persistido e vinculado a uma origem causal de genesis. O
   runtime da ativação **é** durável, mas isso por si só não garante
   reexecução independente do caller. Não considerar replay de triggers
   integralmente homologado até eliminar essa dependência.
2. **Predicados/dependency footprints versionados:** o predicate clock-only
   é puro, mas seu código/schema precisa de registro e versão/hashes
   autoritativos compatíveis com os bundles congelados.
3. **Rearm explícito:** a transição manual causal de rearm não está ainda
   ligada ao mesmo `CycleCommit`; os testes existentes cobrem edge,
   once e repeat, não esse lifecycle completo.
4. **Materialização sintética:** o primeiro evento `clock.advanced`
   existe, mas o `body.advanced` sintético previsto no cenário de
   aceite e o seu schema/reducer versionados ainda precisam ser
   integrados sem introduzir uma autoridade provisória.
5. **Replay causal completo:** reopen e equivalência de bytes foram
   testados, mas resta validar a reconstrução a partir de genesis,
   journal, eventos e runtime sem depender de entradas/predicados
   novamente fornecidos pelo host.

Os portões acima são de **determinismo, execução e auditabilidade** e
portanto impedem marcar CSFRV1-3 como concluído neste checkpoint.
Não há autorização de merge para a `main` nem para iniciar
`code-review` antes de uma execução integrada que satisfaça esses critérios.

## Escopo excluído, conforme ticket

Consumo terminal de occurrence/activation, `ActionProposal`, resolução
de conflitos, settlement de WORK, percepção de eventos de domínio,
Embodiment físico e integração de modelos/LLMs. A ativação criada por
CLOCK_ADVANCE deve permanecer `PENDING`.
