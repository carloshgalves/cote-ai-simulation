# CSFRV1-8 — auditoria adversarial de implementação

**Ticket:** [08 — replay, snapshot/resume e fork](../../../docs/tickets/csf-runtime-v1/08-replay-snapshot-fork.md)
**PR:** #25 (Draft; não mesclar enquanto houver blockers)
**Base:** main @ `e3d0b18865e2eb4f2c37c4eb3a98321909d282f0`
**Estado:** em implementação/review, **não concluído**.

## Revisões e correções já incorporadas

| Finding | Mecanismo | Correção/prova |
| --- | --- | --- |
| R1/F1 | Coordinator chamava `ValidatedGenesis` sem declarar o port | Interface `Store` exige resolução verificável; CI Go completo |
| R2/F2 | Pending-state usava digests não registrados e omitira rosters relevantes | Partições exatas e refs registrados; testes de snapshot e tamper |
| R2/F3 | SQL `lifecycle=0` era confundido com unidade ainda pendente após commit | Derivação via journal de assentamento; teste de transmissão entregue |
| R3/F4 | Identidade de snapshot confundida com hash do envelope histórico cross-policy | CSFHREF vincula SHA256 dos bytes exatos e confere digest registrado do corpo |
| R3/F5 | Snapshot antigo era invalidado apenas por haver novos registros | Seal transacional e prova dos cinco prefixos originais; replay do tail |
| R3/F6 | Fork podia depender de um parent ref sem validação de proveniência | Genesis com bootstrap distinto, prefixo histórico arquivado e verificação de parent; corrupção rejeitada |
| R4/F7 | Resume retornava status de controle do snapshot antigo como se fosse o estado atual | Separação entre estado do checkpoint e estado do tip; diretiva fail-closed |
| R4/F8 | Abort/halt e retry sem plano explícito de reentrada | Retoma com estado/fence/retry congelados, drenagem epistemológica e bloqueio de HALTED; testes reais e negativos |
| R5/F9 | Fork de source terminal confundia closure/completion histórica com trabalho pendente | Apenas closures finais podem ficar no prefixo parental; nunca transplantar IDs do parent à filha |
| R6/F10 | Fonte exógena declarada, sem closure, não aparecia no roster de closures | Fork agora rejeita histórico com source declaration sem SourceClosure final |
| R6/F11 | Filho podia reabrir com mesmo ID uma fonte finalizada no pai | Guard transacional de source declaration lê prefixo parental, rejeita reabertura e falha fechado em archive corrupto |
| R6/F12 | A equivalência entre execução contínua e processo reaberto não era comparada diretamente | Cenário de envio/entrega é executado em quatro rotas, com snapshots finais comparados byte a byte e restore por SQLite novo |
| R7/F13 | Ator cognitivo sem `evidence_routes` não era obrigado a fornecer checkpoint | `cognitive_actor_registry` imutável por run e roster selado em snapshot; testes de ausência e corrupção |
| R7/F14 | Snapshot existente tornava-se não-resumível após abort/retry sem incremento de revisão | Histórico `snapshot_prefix_members` + replay causal das alterações posteriores; crash points na mesma revisão e colisões V1 testados |
| R8/F15 | Resume parava antes de concluir `PerceptionTask` pendente | `DrainPendingPerceptionV1` com dispatch por evento autenticado e idempotência após recibo parcial |
| R8/F16 | `ResumeAction` dependia de implementação manual para restaurar o fence do WORK | `Coordinator.ReopenFencedCycle` reconstrói bytes e coordenada originais sem chamar seleção ou modelo |
| R8/F17 | Retry autenticado não tinha caminho operacional de reenfileiramento do mesmo corte | `Coordinator.ReopenAuthorizedRetry` exige `AttemptRetryRecord`, drena tasks e publica só o fence sucessor |
| R9/F18 | Evals comparavam digests de prefixo, mas não bytes literais de cada ledger | Comparação de todos os registros canônicos em `input_ledger`, `decision_ledger`, `event_store`, `evidence_ledger` e `causal_outbox` após quatro rotas |

## Evidência automatizada

- Testes `TestCSFRV18*` em `internal/csf/commit`: snapshot canônico, clock/reopen, cinco prefixos, corrupções, crash de publicação, tail divergente, percepção, envio e entrega em trânsito, dois forks, trilha parental e determinismo sob permutação de triggers.
- `TestCSFRV18*` em `internal/csf/persistence/sqlite`: diretrizes de IDLE, IN_FLIGHT, RETRY_AUTHORIZED e HALTED, inclusive fence ausente e barreira perceptiva.
- `internal/csf/fork/history_ref_test.go`: vetor normativo CSFHREF e negativos de magic/version/length/algorithm.
- Workflow `CSFRV1-8 Continuity Checkpoint`: testes focados, ref cross-policy, race regression, SQLite e `go test -count=1 ./...`.
- Regressão CSFRV1-5/6/7 e conformidade canônica no mesmo HEAD, quando a suite terminar.

**Critério de conclusão:** os cinco workflows verdes no HEAD exato **e** findings de contrato abaixo resolvidos, com matriz E2E tri-via e crash/fork.

## Findings ainda abertos (não marcar Ready)

1. **OPEN-01 — CROSS-POLICY:** suporte atual verifica framing/hashes da policy de origem, mas só executa fork V1→V1. Runtime de policy filha distinta, migrador versionado e policy-resolution V2 não estão implementados. Rejeição é fail-closed, não aceite positivo.
2. **OPEN-02 — FORK NON-IDLE:** com triggers ativas, schedule/activation/round/dispatch pendentes ou tasks perceptivas incompletas, o bootstrap rejeita. Falta migração determinística dessas autoridades para novos IDs e replay do tail divergente, sem compartilhamento de estado filho/pai.
3. **RESOLVED-03 — ACTOR REGISTRY:** `RegisterCognitiveActor` persiste o roster de agentes cognitivos elegíveis, incluindo atores sem observação. O selo de snapshot fixa o roster; snapshot sem checkpoint completo ou com membro removido falha fechado. A elegibilidade só existe após registro explícito do ator pelo agente proprietário.
4. **OPEN-04 — EXECUTOR DE RESUME:** há drenagem idempotente de `PerceptionTask` e reabertura operacional de `AdmissionFence`/retry autorizado a partir de bytes verificados. A coordenação geral de todos os handlers de `CommitWork`, inclusive o cenário misto, ainda não foi integrada ao orquestrador de execução.
5. **OPEN-05 — MATRIZ DIFERENCIAL COMPLETA:** estão presentes provas por componente e por cenários, mas falta o mesmo cenário integrado com conflito de recurso + trigger + comunicação em trânsito, comparando execução contínua, replay e múltiplos resumes para todos os cinco ledgers, hash/world e permutações/crash points.
6. **RESOLVED-06 — SNAPSHOT V1 DE UMA REVISÃO:** preservada a identidade normativa [run, version, revision], sem reemitir um segundo snapshot diferente na mesma revisão. O checkpoint imutável mais os tails autenticados de fence/abort/retry/percepção reproduzem pontos de crash na mesma revisão; tentativa de sobrescrever o snapshot original falha fechado. Um futuro schema V2 é necessário somente para múltiplos snapshots físicos distintos da mesma revisão.

Os findings ainda marcados OPEN (01, 02, 04 e 05) não podem ser mascarados por CI verde. Não é válido afirmar “zero findings”, concluir ticket ou realizar merge antes da resolução com evidência nova.


## R10 — continuidade adversarial: migradores, política histórica e destino limpo

**Verificação de execução:** a suíte completa estava aprovada no HEAD anterior `f5f002fd3030be7d041d41308cc00ee8c05f7bcc`. As alterações R10 posteriores adicionaram provas e guards; só poderão ser consideradas aceitas depois de cinco workflows verdes no **novo** HEAD, incluindo `TestCSFRV18*`, race e regressão completa. Nunca reutilizar o CI do HEAD antigo como evidência de R10.

| Frente | Evidência nova | Limite explícito |
| --- | --- | --- |
| OPEN-01 | `InstalledPolicyRegistry` realiza lookup exato por `codec_policy_id` + `CodecPolicyHash`, e `ForkIdleSnapshotV1` autentica o CSFHREF através de `VerifyOriginWithResolver`. Testes rejeitam ausência, hash trocado, identidade futura não instalada e mutação. | **OPEN:** isso não disponibiliza política V2. Não existe bundle V2 normativo verificado, nem codec/reducer/schema/Genesis V2 executável ou migrador V1→V2; não proclamar suporte positivo cross-policy. |
| OPEN-02 | Genesis-owned schedule/activation passam por migrador tipado e registries selados; runtime de trigger precisa ser reproduzível a partir do Genesis da filha. Tests de falha de gravação, sibling tail, identidade/roster e rollback; guard impede destino com outras autoridades preexistentes. | **OPEN:** Claim/Event-owned schedule, transmissão IN_TRANSIT, round/dispatch/resposta pendentes, controle in-flight, snapshots cognitivos e trigger evoluído não são migrados. Rejeição atômica é correta, não uma migração positiva. |
| OPEN-04 | Executor `ResumeRegisteredWorkV1` seleciona handlers através do fence autenticado; provas de NoProposal, recurso/contest, sistema e comunicação em fases send/due. | Código implementado; revisão independente do dispatcher e suite no HEAD atual ainda necessárias antes de mudar o finding para RESOLVED. |
| OPEN-05 | Cenário misto inclui permutações, recuperação, sete crash points adicionais e comparação dos cinco ledgers; novo teste injeta corrupção por ledger e exige rejeição/reversibilidade. | Resultado de R10/novas mutações ainda depende da suíte final e da matriz transições pós-fork/dispatch pendentes. |

**Invariantes não negociáveis:** prefixo parental permanece sob a policy original e não é rehashado; a filha recebe novos IDs e transações próprias; nunca converter um checkpoint epistemológico opaco em world truth; nenhum rollback publica metade de um fork; fonte/round/claim sem migrador versionado deve bloquear, nunca sumir. Mantenha Draft e os findings OPEN até evidência positiva concreta.


## R14 — migração positiva de rounds Genesis e auditoria do histórico de dispatch

**Última base completamente aprovada:** `d84a982533bd108ca7084ddd6ca2ae7ae9be4c05` (5/5 workflows, incluindo race e regressão). **Este R14 não recebe aceite desse HEAD antigo.** A suite do HEAD que contém R14 precisa passar integralmente.

### Nova fatia positiva de OPEN-02

- `fork_pending_round.go` revalida digest registrado no checkpoint, ID derivado sob o Genesis parental, run, origem, role/ordinal, elegibilidade e roster canônico de slots. Deriva novos ID/Genesis ref sob o `childRun`, com hash da exata autoridade canônica.
- `fork_bootstrap.go` reconcilia em bijeção as partições de `RoundDeclaration` e `pendingInput`; publica rounds e `round_integrity_seals` na **mesma transação** de Genesis/clock, archive, ocorrências e triggers.
- Testes positivos verificam o round como unidade `PendingClock` no filho, `inputledger.Open`, `ReplayClock`, snapshot do filho, idempotência e colisão de roster, sem alterar os bytes do checkpoint parental.
- Revisão adversarial encontrou risco de perda silenciosa: round ainda pendente pode ter histórico de SlotDispatch revogado ou respondido. A migração agora lê a autoridade parental e **rejeita qualquer round previamente despachado**, mesmo sem dispatch ativo no snapshot; negativo específico documenta a situação.
- Uma falha injetada pelo SQLite ao inserir `round_integrity_seals` deve reverter todo o bootstrap. O helper de teste percorre todas as tabelas efetivas de autoridade do workspace para detectar resíduo, não uma lista manual parcial.
- O workflow CSFRV1-8 passou a usar `concurrency` para cancelar execuções redundantes ou superadas no mesmo branch (push/PR), reduzindo consumo de Actions sem dispensar um CI verde no HEAD final.

### Findings e limites que permanecem

| Finding | Estado R14 | Aceite que falta |
| --- | --- | --- |
| OPEN-01 | **OPEN** | Bundle V2 normativo realmente executável com manifests/registries/conformance, pinning Genesis sob a política filha, migrador explícito V1→V2 e replay sob ambas as políticas. Não rehashar o prefixo parental. |
| OPEN-02 | **OPEN (cobertura positiva ampliada)** | Event/Claim-owned occurrences, transmissão IN_TRANSIT, rounds com dispatch/resposta/revogação, ciclo/fence in-flight, trigger runtime evoluído e checkpoints cognitivos; cada classe precisa de rekey, causal provenance, settlement e rollback próprio. |
| OPEN-04 | **OPEN para aceite formal** | Verificar no HEAD final o `ResumeRegisteredWorkV1` com handlers autenticados para NoProposal, recurso, contest, sistema, comunicação send/due e cenário misto. Provar idempotência e falha fechada em fontes desconhecidas. |
| OPEN-05 | **OPEN para aceite integrado** | Reexecutar a matriz das cinco ledgers, world/state hash, permutações e crash points no HEAD final; cobertura diferencial dos migradores ainda ausentes não pode ser alegada. |

**Gate:** PR permanece Draft/NO-GO. Um CI verde valida a fatia efetivamente implementada, não substitui a conformidade positiva exigida nos blockers.

## R19 — histórico de rounds parcialmente executados (slice inicial)

O bootstrap V1→V2 agora migra rounds Genesis pendentes com dispatch aberto,
SlotResponse (`NoProposal` ou `ActionProposal`) e revogações duráveis. A mesma
função deriva novamente os registros a partir do arquivo parental no bootstrap e
na validação da linhagem. Os registros e as chaves condicionais de resposta são
publicados na transação do filho. O prefixo original continua sob a policy V1.

O corte é o prefixo congelado do snapshot, mesmo que o pai receba respostas
posteriores na mesma revisão. O migrador exige dispatches elegíveis e vinculados
à revisão/instante/ciclo desse corte. Os ordinais de dispatch e a política de
revogação são preservados; as coordenadas de elegibilidade dos rounds no instante
corrente são rebaseadas em conjunto com o ciclo e a revisão iniciais do filho.

Evidência local: `TestCSFRV18R19*` cobre ambos os tipos de resposta, resposta
idempotente e duplicada, revogação/redespacho, commit real de round, replay,
snapshot+tail, reabertura, rollback com retry, corrupção de resposta/dispatch/
revogação/chave condicional e isolamento de resposta posterior do pai. Um teste
adicional parte de revisão 1/ordinal 0 e exige commit positivo no filho.

REDs observados: partição de dispatch sem migrador; preparador de comunicação
bloqueando corte de round sem ocorrência pendente; ordinal parental retido na
filha após ciclo prévio. A implementação positiva corrige esses caminhos.

**Limites:** rounds Event-owned, requests de cortes anteriores e proposals com
`originating-intention-ref` dependem de migradores próprios. R20–R23 continuam
pendentes. OPEN-02 e OPEN-05 permanecem **OPEN**; esta slice não aprova a matriz
integrada nem implica autorização para merge. A validação do HEAD exato da PR
deve constar no checkpoint da conversa; não reutilizar os workflows do R18.

Contrato operacional: [fork-round-history-v2](../../../docs/architecture/fork-round-history-v2.md).

### R19-1 — restauração deve validar lifecycle, não apenas cada ID

A revisão do checkpoint `b8f65f3` identificou que `InputLedger.restore` reconstruía
respostas e dispatches sem validar todas as transições de slot. A PR contém o
finding marcado R19-1. Seis corrupções com registros individualmente válidos
reproduziram RED: resposta a dispatch revogado, redespacho sem revogação,
resposta a dispatch superado, revogação após resposta, revogação repetida e
primeiro ordinal não zero. O teste inclui também troca de tipo de resposta.

A correção aplica o estado de slot durante restore: primeiro ordinal zero,
sucessão imediata somente após revogação sem resposta, limite uint32 antes do
cast, resposta única contra o dispatch corrente aberto e revogação única de
request ainda sem resposta. A cadeia positiva com três revogações/reabertura
permanece aceita. A resolução formal só deve apontar o commit da correção após
validação e push; OPEN-02/OPEN-05 permanecem abertos.
