# CSFRV1-3 — checkpoint de implementação (19/10/2026)

**Situação: REVIEW REMEDIATION IMPLEMENTED — PR #19 Draft, aguardando nova revisão independente.**
Nenhum merge ou aprovação automática. A revisão independente original foi executada
e seus seis findings foram tratados na branch; validação do implementador
não substitui re-review.

**Base:** `main@7b7fc7edaf844ff6eeb637299fd6e00ec93f48db`.
**HEAD do checkpoint anterior à revisão:** `4a1eb737dabc9c719be508b0aeed136966d06fd9`;
CI de implementação e conformance verdes (`go test ./...`, 621/621 Go,
621/621 TypeScript, relatórios comparados byte a byte).
[Go CSFRV1-3](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39313704003/job/118269598597) · [conformance](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39313712927/job/118269627412).

## Implementação integrada

- `Clock` seleciona a menor pendência por coordenada causal;
  avança lacunas ordinais sem ciclo fictício, confirma `P` contra
  ingressos `Q < P` e atribui piso sucessor correto.
- SQLite persiste em **uma transação** `CycleCommit`, dois eventos
  temporais sintéticos não perceptíveis (`clock.advanced` e `body.advanced`)
  e eventos internos de lifecycle `TRIGGER_RUNTIME`/`TRIGGER_ACTIVATION`
  para as transições registradas. Revisão, novo `ClockState`, runtime,
  `TriggerActivation(PENDING)` e o conjunto completo de eventos são indivisíveis.
  A sequência lógica cobre todos os eventos do batch e o input ledger permanece imutável.
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
- `ReplayClock` reconstrói `ClockState`, eventos (temporais e lifecycle),
  transições de triggers e ativações a partir de genesis, definições seladas,
  journals e eventos persistidos. Valida bytes canônicos integrais, causas,
  vínculo com AdmissionFence, states tipados e projeções SQL,
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
12. Para avanço temporal puro, cada hash e os typed-values pré/pós
    da revisão do mundo são confrontados com o genesis; divergência em
    `world_revisions.state_hash` ou nos bytes de estado é detectada após reopen.
13. Teste de arquitetura percorre arquivos Go de produção e exige que
    só o coordinator de `internal/csf/commit/clock_advance.go`
    invoque o port transacional `CommitClockAdvance`.

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

Após a remediação, executar uma **nova sessão independente de `code-review`**
somente quando solicitada. Não iniciar outra revisão nesta fase de correção.
**Estado do gate:** parte temporal de CSFRV1-3 validada; rearm manual ainda
não tem publicação causal executável e permanece pendente do WORK
CycleCommit. Não inferir aprovação do ticket inteiro a partir da CI verde.

Não mesclar à `main` sem revisão e aprovação formal. Não tocar em
CSFRV1-4/5, Canon RAG ou Examination Engine nesta branch.

## Checkpoint pós-revisão — 19/10/2026

**Findings congelados da revisão da PR #19:** 1–6, todos High.
**Commits de correção:** `ba5b602877f5e48d26a1b5f04207ad9e6f7ddd43`,
`decc67522f3a2523a2fd0a16931bab8b0b947a4e` e
`657dfd12238c7a06a37fe7a93f47fa3ed0ffdeab`.
O commit documental/CI subsequente é apenas o checkpoint de validação.

**Correções por finding:**

1. Fatos internos `trigger.runtime.transition` (phase 3) e
   `trigger.activation.created` (phase 4) acompanham os dois eventos
   temporais no mesmo `CycleCommit`. O replay reconstrói bytes/ids/ordem
   integralmente; lifecycle não entra nos `pre_trigger_events`.
2. `pendingClock` valida todas as linhas de occurrence/activation,
   inclusive as declaradas consumidas por índice SQL, contra record canônico,
   identidade, digest, elegibilidade e lifecycle. Replay compartilha o gate.
3. Commit e replay exigem o `AdmissionFence` durável, derivam seu ID/digest
   e vinculam plano, cursor de input e prova de fechamento às autoridades
   de fonte e a seus ingress receipts. Fence ausente não admite replay.
4. Replay refaz integralmente os bytes da `TriggerActivation` a partir
   de genesis, definição, runtime e ciclo, não apenas seu próprio digest.
5. `WorldRevision` tem transição de typed-values íntegros também na
   revisão zero; estados pré/pós e hashes são confrontados com o genesis.
6. Bootstrap das occurrences deriva ID/digest, valida coordenação e
   lifecycle e exige `created-by` correspondente ao genesis persistido.

**Regressões:** index-only tamper de lifecycle, remoção de fence,
activation re-hashed com eligibility forjada, adulteração de typed
world state mantendo hash, occurrence com ID/digest falsos, contagem
e reconstituição de lifecycle events. Permanecem as provas anteriores
de rollback, worker permutation, rearm clandestino e replay sem host.

**Gate mantido:** rearm manual causal ainda requer `WORK CycleCommit`
(CSFRV1-4/5), sem `UPDATE` paralelo de runtime. O sucesso da
remediação não declara esse feature completo nem autoriza merge.

**Próxima fase:** `$code-review CSFRV1-3` independente, apenas por
solicitação explícita após as checks da PR.

## Segunda remediação de code-review — findings 7–11 (19/10/2026)

**PR #19:** permanece Draft, sem merge ou aprovação automática.
**Base de review:** `ffdc4940f73f54213e198b1f9ec830425fe32e67`.
**Commits da remediação:** `4a4fecfcd62143b68b9e93afc2ce88f0392e6151`,
`41da1f81ccf8e476d7add2ec8da9d522038d58b5`,
`ec1ccc9be0cea67d54d148bf362d510ed91b9ab9` (instrumentação da falha),
`5f5b495f18b2bbebc9465ff5d615e526018ee357` e
`d9cfa731bf6a26c18a435335dbe8016b365c4393`.

- **#7:** tanto `AppendRoundDeclaration` quanto `AppendIngress`
  interpretam o corte do `AdmissionFence` como `cycle_plan.target_coordinate`
  quando o ciclo é `CLOCK_ADVANCE`, e como a origem quando é `WORK`.
  Regressões verificam Q concorrente pré-fence (muda o alvo) e pós-fence
  (inserção rejeitada e avanço original concluído). Nenhum abort/segunda
  identidade de retry provisório foi adicionado.
- **#8:** commit e replay verificam o conjunto completo de fontes exógenas
  declaradas e a **primeira** `SourceClosure` que cobre P por `ingress_seq`.
  Não aceitam um conjunto de provas vazio com fonte declarada, nem substituir
  a testemunha pela segunda closure após re-hash.
- **#9:** referências de policy são confrontadas com os bytes efetivamente
  fixados no genesis em commit e replay. A associação `role → policy ref`
  deixou de usar índices posicionais: utiliza `genesis_policy_bindings`
  e as refs canônicas do genesis. **Compatibilidade V1 delimitada:** o
  contrato de transições registra `perception-visible-first-v1`, porém a
  ref congelada no genesis de V1 é `perception-v1`. O runtime usa **somente**
  a ref efetivamente comprometida no genesis, sem alterar bundle, hashes
  ou a semântica de percepção.
- **#10:** eventos sintéticos de `CLOCK_ADVANCE` rejeitam parent/source/
  actor/entity refs ou location não canônicos, mesmo com `Event` e batch
  hashes recalculados.
- **#11:** `SeedGenesisOccurrence` proíbe `PENDING` elegível em instante
  anterior ao `Genesis.InitialInstant`, antes de inserir. Fixture negativa
  usa occurrence com ID/digest válidos e coordenada retroativa.

**Regressões adicionadas:** corte pós-fence, seleção pré-fence, policy roles,
fence sem closure, closure posterior usada como primeiro witness, policy
ref trocada com batch re-hash, event provenance forjada com digests re-hashados,
occurrence pré-genesis.

**Validação executada sobre `d9cfa731`:**

- [CSFRV1-3 Implementation Checkpoint](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39340606389): **success** — `git diff --check` desde o HEAD anterior à primeira revisão, `go test ./...`, harness Go, harness TS e `cmp` canônico.
- [CSF canonical conformance](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39340614237): **success**.

**Gates mantidos:** lifecycle/settlement de WORK e rearm manual durável
continuam dependentes de CSFRV1-4/5, sem bypass do CycleCommit.
Próxima etapa: novo `$code-review CSFRV1-3` independente apenas mediante
solicitação. Este checkpoint não autoriza merge nem Ready for review.

## Terceira remediação de review — findings #12–15 (19/10/2026)

**PR:** Draft #19 · **ponto revisado:** `b8950b9502b734136034d9954f47176e2bed07bf`.
**Lote congelado:** quatro findings High — #12, #13, #14, #15.
**Status de implementação:** corrigidos no checkpoint desta seção; markers da PR
são atualizados somente após execução verde de CI no HEAD final.

- **#12 — compromissos canônicos vazios.** A autoridade de
  `verifyCommittedFence`, comum ao commit e ao replay, recalcula o
  `input_digest` de `admitted_units=[]`, `decision_digest` da lista
  de decisões vazia, `perception_task_digest` da lista vazia e o
  `cohort_id` dos rounds vazios a partir do `cycle_id` e
  `base_revision`. Rejeita digests forjados mesmo quando
  `fence_digest` e `batch_digest` são consistentemente refeitos.
- **#13 — seleção do avanço na repetição.** Na execução apenas
  `CLOCK_ADVANCE`, a agenda genesis e as unidades originais de
  ingresso não são liquidadas pelo slice e permanecem observáveis.
  Replay rederiva o mínimo a partir dessas autoridades, excluindo
  somente `TriggerActivation` originadas **pelo próprio avanço**
  e o próximo repeat cadence materializado pelo trigger runtime
  posterior. O plano durável tem de coincidir com `clock.Next`
  original; desaparecimento do target e sua retrodatação falham
  fechados. Este mecanismo não é um replay de WORK/settlement:
  essa extensão permanece bloqueada pelos tickets 4/5.
- **#14 — piso de elegibilidade.** Rounds e SourceIngress
  consultam o ClockState no mesmo SQLite `tx` de publicação;
  rejeitam coordenadas anteriores ao próximo cursor do relógio.
  Se o genesis está gravado mas o relógio ainda não foi
  bootstrapped, seu `InitialInstant` já impõe o piso.
  Adaptadores SQLite sem genesis, clock nem fence inicializados
  continuam aptos a testes unitários isolados de atomicidade, sem
  presumir uma run executável.
- **#15 — cadeia de revisões.** Replay verifica a enumeração integral
  `0..ReconstructedClock.Revision` de `world_revisions`, não
  somente as linhas referenciadas por journals. Commit confirma a
  existência e hash da revisão predecessora dentro da transação,
  antes de inserir um novo `CycleCommit`.

**Regressões red/green:** `review_cycle3_test.go` comprova o estado
vermelho anterior à correção (run
[39343784391](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39343784391))
para cinco adulterações de claims vazios com hashes externos
recalculados, target ausente ou anterior, round pré-genesis,
WorldRevision órfã e predecessor apagado. Há também cobertura
positiva da aceitação no piso genesis e do caminho
pré-`BootstrapClock`.

**Artefatos preservados:** nenhum hash do bundle canônico,
schema ou conformance foi modificado; a PR não publica
WORK/settlement, perception nem segunda autoridade de escrita.
Testes e evidências do HEAD final devem ser anexados ao comentário
de conclusão da PR antes de qualquer outra fase.

## Quarta remediação de review — findings #16–18 (19/10/2026)

**Lote congelado:** findings High #16, #17 e #18.
**Base revisada:** `817e2efab65101985df7c619e54be50ebc429796`.
**PR:** [Draft #19](https://github.com/carloshgalves/cote-ai-simulation/pull/19),
sem merge ou antecipação de `WORK`/settlement.

- **#16 — único caminho de ingresso.** `Store.Append` rejeita
  `ExogenousInput` e `SourceClosure` com run e source canônicos,
  reservando `AppendIngress` para a inserção conjunta do record,
  `SourceIngressReceipt` e incremento do cursor, sob guard de fence
  e relógio. A regressão usa um input distinto e retroativo
  depois do fence, além da tentativa de inserir input/closure por
  port genérico.
- **#17 — integridade além da menor pendência.** Antes de reproduzir o
  clock, o replay reabre o `InputLedger` canônico com os validators
  existentes, obrigando identidade, `unit_digest`, origem, receipts
  e sequência contígua para toda a entrada, incluindo itens futuros.
  Como o ID de `RoundDeclaration` não inclui ator/slots, o
  `round_integrity_seals` guarda somente seu hash canônico,
  gravado atomicamente com o record na **mesma SQLite transaction**.
  Replay comprova hash e cardinalidade desses índices para rounds
  persistidos, sem criar segunda autoridade de publicação.
- **#18 — contrato fechado da tentativa inicial.** O verificador
  do `AdmissionFence` recusa `retry_ref` diferente de ABSENT e
  `rule_versions` diferente de lista vazia no único caminho
  `CLOCK_ADVANCE` de tentativa zero, mesmo com `fence_digest`
  e `batch_digest` recalculados consistentemente.

**TDD:** regressões de `review_cycle4_test.go` falharam no estado
anterior à correção — [run vermelho](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39352257316).
Após o patch: [Go/TS e diff check](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39352484836)
e [conformance canônica](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39352490091)
ficaram `success`. O HEAD final com este checkpoint também deve
passar as mesmas duas pipelines antes de fechar os markers
`RESOLVED`.

**Imutabilidade do bundle:** nenhum contrato/hash pinado ou
fixture de conformance foi reescrito. Runtime continua com
`CLOCK_ADVANCE` como único publisher terminal deste slice.

## Quinta remediação de review — findings #19–21 (19/10/2026)

**Lote congelado:** três findings `High`, #19, #20 e #21, todos
originados da quinta revisão independente da Draft PR #19.
**Base de implementação:** `6fe475b8e38a6935c33efe70b86cf4663a856ebc`.

- **#19 — fonte e recibo são provas, não argumentos.** O writer
  `Store.AppendIngress` rederiva identidade e digest de
  `ExogenousInput`/`SourceClosure`, comprova `SourceDeclaration`
  para o run e exige que o `SourceIngressReceipt` canônico tenha
  exatamente a fonte, sequência, kind, ID e digest do record. A
  verificação acontece na mesma transação que publica record,
  receipt, índice e cursor. A exceção do adapter sem
  Genesis/Clock/Fence permanece somente para testes antigos com
  records opacos e sem autoridade causal inicializada.
- **#20 — histórico exógeno monotônico e terminal.** O port
  transacional recusa qualquer novo ingresso após closure
  `final=true`; fechamento posterior não pode regredir, e
  `ExogenousInput` precisa ter elegibilidade estritamente maior
  que o último `closed_through` da própria fonte. A restauração
  `inputledger.Open` também valida essas propriedades na ordem dos
  `ingress_seq` canônicos, não somente na memória de um Ledger
  vivo. Testes usam recibos e projeções internamente coerentes
  injetados diretamente no SQLite após `final` e após fechamento
  mais avançado; replay falha fechado.
- **#21 — projeções SQL verificáveis.** A comparação de
  `SourceIngressReceipt` canônico com
  `source_ingress_receipts` e `input_source_cursors` inclui
  cardinalidade, sequência contígua, `record_id`, bytes do
  recibo e `next_seq`. É executada antes da escrita na própria
  SQLite transaction, na consulta de próxima sequência e em
  `ReplayClock`. Divergência de índice/cursor não é aceita como
  autoridade de sequência. A verificação da leitura durante
  `NextIngressSequence` não substitui o compare atômico do
  write path.

**TDD:** [CI vermelha em testes adversariais antes das
correções](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39354741700):
ingresso de fonte `ghost`, receipt incompatível, append
após `final`, closure regressiva e drift nos índices/cursores.
As duas pipelines ficaram verdes após o patch e também com
os novos testes de restauração de histórias forjadas:
[Go, TypeScript, receipts e whitespace](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39355271986) e
[conformance canônica](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39355276322).

**Fronteira preservada:** nenhum novo publicador terminal
`WORK`, handler, settlement ou percepção; nenhum hash ou
artefato pinado do codec/conformance mudou. A PR continua
Draft, sem merge, e a próxima revisão é fase separada.

## Sexta remediação de review — finding #22 (19/10/2026)

**Lote congelado:** um finding `High`, #22 (reentrega duplicada de
`SourceIngressReceipt`). **Base revisada:**
`8681158b2da4f3e635712baa0dcee6a493bbb3a1`.

- **#22 — um recibo por identidade causal.** Dentro da transação de
  `Store.AppendIngress`, antes de escrever qualquer record, recibo
  ou cursor, o adapter consulta a identidade no `input_ledger`.
  Se a identidade já está publicada com bytes idênticos, a chamada
  de um `Ledger` desatualizado é rejeitada sem mutação; reabrir o
  ledger permite recuperar o recibo original. Bytes diferentes sob
  a mesma identidade causam erro explícito de colisão.
- **Recuperação fail-closed.** `verifyIngressProjections` rejeita
  dois `SourceIngressReceipt` que referenciem o mesmo record ID,
  ainda que suas sequências individuais sejam gapless. A
  `inputledger.restore` não sobrescreve silenciosamente o
  recibo anterior no mapa indexado pelo ID causal; reporta a
  duplicata com erro específico.
- **Regressões adversariais.** Duas instâncias de `InputLedger`
  abertas antes do append reproduzem a reentrega idêntica e
  confirmam que não é criado segundo recibo, que o cursor não muda
  e que `Open`/`ReplayClock` continuam válidos. Há cobertura de
  bytes conflitantes sob a mesma identidade e de legado adulterado
  com dois recibos distintos para um único record ID.

**TDD:** [regressões vermelhas antes do patch](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39358040730)
comprovaram a segunda sequência e a projeção permissiva.
[Suíte Go, conformance Go e TypeScript, recibos idênticos e diff
check](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39358229359)
e [conformance canônica](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/39358234016)
ficaram verdes no commit de correção.

**Escopo preservado:** nenhum publisher `WORK`, settlement, handler,
percepção nem modificação no codec/schema bundle. PR permanece
Draft sem merge; a re-revisão é uma fase separada.
