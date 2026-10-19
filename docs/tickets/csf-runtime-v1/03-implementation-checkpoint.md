# CSFRV1-3 — checkpoint de implementação (19/10/2026)

**Situação: IMPLEMENTATION IN PROGRESS — PR #19 Draft.** Nenhum merge
e nenhuma revisão independente executados. Este é o escopo executado
e testado, não um certificado de homologação.

**Base:** `main@7b7fc7edaf844ff6eeb637299fd6e00ec93f48db`.
**HEAD de implementação validado nesta atualização:** `f8c5d81a67d7bfb7b145a75697819569668271e0`;
CI de implementação e conformance verdes (`go test ./...`, 621/621 Go,
621/621 TypeScript, relatórios comparados byte a byte).
[Go CSFRV1-3](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39313289835/job/118268177434) · [conformance](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39313297126/job/118268201791).

## Implementação integrada

- `Clock` seleciona a menor pendência por coordenada causal;
  avança lacunas ordinais sem ciclo fictício, confirma `P` contra
  ingressos `Q < P` e atribui piso sucessor correto.
- SQLite persiste em **uma transação** `CycleCommit`, dois eventos
  sintéticos não perceptíveis (`clock.advanced` e `body.advanced`),
  revisão, novo `ClockState`, runtime e `TriggerActivation(PENDING)`.
  A entrada original do input ledger permanece imutável.
- `TriggerDefinition` v1 é codificada conforme `foundation.cddl`,
  com typed-value versionado `clock-threshold.v1`, dependencies
  canônicas, policy e handler ligado ao policy ref de
  `dependency-footprints` do genesis validado. O bootstrap registra
  **uma única vez** o conjunto completo num registry imutável, com
  genesis ID/digest e root hash. Nenhuma definição é enviada pelo
  host por ciclo.
- O bootstrap avalia a condição na revisão 0 e materializa as
  ativações iniciais verdadeiras com origem `GenesisRef`; elas
  permanecem `PENDING`.
- Event footprints sintéticos são funções puras versionadas:
  `clock.advanced → ["clock"]`, `body.advanced → ["body"]`.
  `causing_event_ids[]` é derivado do EventOrderKey canônico, não
  da ordem de workers. Não há acesso a belief, LLM, relógio de parede
  nem EventStore irrestrito no predicado temporal.
- `CommitClockAdvance` verifica a integridade do registry, aceita
  apenas **todas** as definições gravadas, reexecuta os predicados
  registrados dentro do commit e exige runtime/ativações exatamente
  correspondentes; não permite omitir triggers nem forjar origem.
- `ReplayClock` reconstrói `ClockState`, eventos e transições de
  triggers a partir de genesis, definições seladas, journals e eventos
  persistidos. Valida digests, causas, ativações e runtime resultante,
  sem receber `TriggerWork` ou predicados do host.
- `RearmPure` existe como transição pura e requer ID de evento
  causal explícito; não é uma operação de escrita nem uma autorização
  para alterar runtime diretamente.

## Provas executáveis (CI verde)

1. `(t,1) → CLOCK_ADVANCE(t,2) → (u,-1)` e próximo WORK em `(u,0)`;
   nenhum commit decorativo na lacuna ordinal.
2. `Q < P` entra após a seleção provisória: a transação rejeita
   o avanço e mantém a revisão anterior.
3. Crash injetado em seis fases do commit: atomicidade e retry sem
   duplicação ou efeitos parciais; `CycleCommit` no decision ledger,
   eventos no event store e world revision consistentes.
4. Reopen SQLite e permutação dos registros de definição/worker:
   mesmos bytes, IDs, digests, runtime e ativações.
5. Boot de trigger inicialmente verdadeiro na revisão de genesis:
   ativação pendente em `(t,0)`, selecionada como WORK.
6. Políticas edge, once, repeat e cadência positiva; `next_repeat_at`
   é persistido e concorre no mínimo temporal.
7. Trigger em `clock` e trigger em `body` apontam para seus
   eventos causais distintos, ambos com proveniência verificável.
8. Lote adulterado (batch digest, event digest, revision hash,
   definição imutável, runtime inventado, ativação omitida ou
   proveniência falsa) rejeitado sem mutações parciais.
9. Replay sem inputs de trigger fornecidos pelo host, inclusive do
   genesis ainda sem commits e de uma revisão já publicada.
10. `CLOCK_ADVANCE` rejeita tentativa de rearm não publicada por evento
    causal de WORK, sem alterar a revisão ou o runtime.
11. Replay de um run concluído falha fechado diante de corrupção
    dos registros SQLite de `TriggerDefinition`, runtime, ativação,
    journal, evento ou relógio.
12. Para avanço temporal puro, cada hash de revisão do mundo é
    confrontado com o hash validado no genesis; divergência em
    `world_revisions.state_hash` é detectada, mesmo após reopen.

## Gate de escopo ainda aberto

**Rearm manual durável (dependência arquitetural de WORK):** o reducer `RearmPure` tem contrato explícito
e não aceita evento causal ausente, mas publicar manual rearm e seu
evento de lifecycle exige um `WORK CycleCommit`. Isso depende do
pipeline terminal CSFRV1-4/5, que está **explicitamente fora do
CLOCK_ADVANCE** e não pode ser simulado com `UPDATE trigger_runtime`
direto ou um segundo writer provisório. O binding da transição
manual a um evento/commit persistido ainda não foi demonstrado.
A decisão de aceite deve registrar essa dependência, sem chamar
a funcionalidade completa de rearm de implementada.

**Limite adicional do V1:** os dois eventos de temporalidade usam
payloads sintéticos versionados, **não** um reducer físico de
Embodiment. Alterar a política de percepção ou footprint exige
aprovação versionada, não mudança de callback de host.

## Próxima fase

Congelar esta implementação, atualizar o review checklist e executar
`code-review` independente quando a dependência de rearm estiver
formalmente delimitada/aceita como interface futura pelo ticket.
**Estado do gate:** parte temporal de CSFRV1-3 validada; rearm manual ainda
não tem publicação causal executável e permanece pendente do WORK
CycleCommit. Não inferir aprovação do ticket inteiro a partir da CI verde.

Não mesclar à `main` sem revisão e aprovação formal. Não tocar em
CSFRV1-4/5, Canon RAG ou Examination Engine nesta branch.
