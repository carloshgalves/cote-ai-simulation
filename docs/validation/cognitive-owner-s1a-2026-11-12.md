# Issue #74 — S1a: inventário de entrypoints, lacunas e plano de implementação

**Data:** 2026-11-12  
**Baseline efetivamente inspecionada:** `main@5474413e4a2c43bdb3d89f7298b272d58342157e` (após merge da [PR #73](https://github.com/carloshgalves/cote-ai-simulation/pull/73)).  
**Escopo:** investigação estática do GitHub, contratos aceitos, arquivos Go e testes versionados; **nenhuma execução local, implantação, provider real, alteração de código, aprovação de ADR ou aceite de release** nesta S1a.  
**ADR proposto:** [0013 — owner cognitivo durável](../adr/0013-cognitive-owner-durability-s1.md) (**PROPOSED**, não `Accepted`).  
**Hierarquia:** `AGENTS.md`, `CONTEXT.md`, ADR 0001/0002/0008–0011 aceitos, [S0](../architecture/cognitive-owner-durable-adapter-s0.md), [R22](../architecture/epistemic-owner-receipt-v1.md) e issue #74.

## 1. Inventário factual da superfície de integração

| Caminho / símbolo real | Quem o usa hoje | Classificação | Impacto de S1 |
| --- | --- | --- | --- |
| `internal/csf/ownercheckpoint/gate.go` — `DurableOwner`, `Gate.Stage`, `RestoreAndAcknowledge`, `RunWork` | Tests `ownercheckpoint/gate_test.go` e `commit/owner58_s0_integration_test.go`; não foi encontrada chamada de `RunWork` em aplicação de produto no código indexado | **Contrato S0 presente, não um owner produtivo** | Implementar adapter concreto separado do core; integração do host deve tornar `RunWork` obrigatório para callbacks cognitivos |
| `internal/csf/persistence/sqlite/epistemic_owner_receipt_v1.go` — `RegisterCognitiveOwnerKeyV1`, `PersistCognitiveCheckpointV1`, `LoadCognitiveCheckpointV1` | SQLite CSF e testes reais de escrow | **Autoridade de assinatura e bytes opacos da CSF** | Reutilizar; não converter CSF em owner de crenças, nem expor `Load*` como API pública |
| `internal/csf/persistence/sqlite/cognitive_actors.go` — `RegisterCognitiveActor` | CSF/testes | **Roster de atores elegíveis** | Garantir actor+key registration antes do primeiro snapshot coberto; sem downgrade para legado em produção |
| `internal/csf/persistence/sqlite/snapshot_v1.go` — `CreateSnapshotV1` e `VerifySnapshotV1` | CSF/testes/fork | **Selo causal** | Publicar somente depois de owner commit+assinatura e escrow; snapshot deve referenciar a mesma cobertura/bytes |
| `internal/csf/commit/resume_runner.go` — `ResumeRunV1(handler)` | Core e testes | **Retomada de terminal determinístico** | Não chamar LLM para replay; exigir owner restore/ACK **antes** de novo trabalho cognitivo, não antes da recuperação determinística válida |
| `internal/csf/commit/resume_dispatch.go` — `ResumeRegisteredWorkV1`, `DispatchFencedWorkV1` | Core e testes | **Executor tipado de WORK causal** | Não classificar como chamada a LLM; preserva autoridade do fence, handlers registrados e `CommitWork` |
| `internal/csf/commit/fork_after_recovery_v2.go` — `SettleFencedThenForkV2(... buildCheckpoints func(...))` | Core e testes | **Seam de aquisição de checkpoints na transição V1→V2** | Integrar owner do host com verificação exata e reatestado da filha; não gerar memória por LLM em recovery |
| `cmd/csf-harness/main.go` | CLI de conformidade e criação de Genesis | **Sem host de LLM** | Não transformar o harness em framework de cognição |
| `cmd/csf-resource/main.go` | CLI de evidência sintética não autoritativa | **Sem host de LLM** | Fora do escopo |
| `src/embodiment/` | Subdomínio físico Python, sem LLM | **Não é owner de memória cognitiva** | Não alterar ADR 0007/integração física nesta S1 |

**Resultado do inventário:** o **local de despacho do modelo pelo produto ainda não existe**. A busca indexada por `RunWork(`, `Stage(` e `RestoreAndAcknowledge(` retornou apenas o S0 e testes do repositório inspecionado. Essa busca não prova formalmente a ausência de qualquer integração fora do GitHub; portanto o aceite de S1b deve listar e fiscalizar cada ingress presente no host criado, sem declarar cobertura de componentes ainda inexistentes.

### Fluxo de integração que falta

```text
[host/operador autorizado; NOVO EM S1b, modo demonstrador]
   ├─ resolve tenant -> run -> actor e autorização (produtivo: PENDENTE)
   ├─ recupera estado do owner; valida chave pública registrada
   ├─ prepara SnapshotCheckpoint (bytes opacos; cobertura)
   ├─ Gate.Stage -> DurableOwner.PersistAndSign REAL -> escrow CSF
   ├─ CreateSnapshotV1 + VerifySnapshotV1
   ├─ encerra/reabre owner e CSF
   ├─ Gate.RestoreAndAcknowledge -> owner.Restore -> owner.Acknowledge durável
   └─ Gate.RunWork -> callback cognitivo sintético CONTADO, sem LLM
```

O core recebe proposta/resultado por portas existentes; o host futuro será responsável por **admitir a ação antes do settlement** e proibir bypass cognitivo. Determinismo de `CommitWork` não substitui autorização para pensar/consultar memória.

## 2. Mapa de riscos e requisitos ainda não satisfeitos

| ID | Prioridade | Lacuna (evidência) | Evidência para fechamento |
| --- | --- | --- | --- |
| L01 | P0 | `DurableOwner` só possui fake de teste; não há persistência owner concreta separada da CSF | Adapter com commit/reopen real e journal/ACK durável, testes de perda de processo com bytes exatos |
| L02 | P0 | Sem host/orquestrador cognitivo conectado a `Gate.RunWork` | Fluxo demonstrador com `RunWork` obrigatório; busca estática/arquitetural que identifique eventuais bypasses |
| L03 | P0 | Recibo Ed25519 certifica owner/signature, **não** durabilidade física de outro serviço | Contrato e failure injection do owner declarado; produção exige ensaio real da infraestrutura aprovada |
| L04 | P0 | `Scope` não possui tenant e `LoadCognitiveCheckpointV1` é infraestrutura privilegiada | Host aplica ACL autenticada run↔tenant↔actor e testes cross-tenant; sem cliente poder forjar scope |
| L05 | P0 | Restore/ACK atual é uma interface, não prova de restauração de memória real | Owner concreto reidrata estado persistido, registra ACK idempotente e barra callback sem ACK |
| L06 | P1 | Testes S0 usam mesmo processo/fake e não comprovam perda de host/processo owner independente | Abrir owner novo e CSF novo após crash/restart com injeção por etapa |
| L07 | P1 | Retenção, backup combinado owner↔CSF, signer/KMS, rotação e RPO/RTO não estão definidos | Runbook e restore drill com provas; riscos de perda de chave e owner sem downgrade |
| L08 | P1 | Sem garantia interprocesso para callbacks: gate usa mutex/permit in-process; tip verificado antes do callback | Definir single-dispatcher na prova; multi-host só após fencing e teste de concorrência distribuída |
| L09 | P1 | Fork child tem outro run e precisa checkpoint/recibo próprio; `buildCheckpoints` é callback sensível | Testes fork V1→V2/child, crash após commit da filha e recuperação sem novo modelo |
| L10 | P1 | Idempotência de checkpoint não faz chamadas LLM exatamente uma vez | Simular callback e registrar limite; no futuro persistir resultado/response id antes de efeito externo ou exigir idempotency token |
| L11 | P2 | Custo de `VerifySnapshotV1` a cada `RunWork` e owner DB ainda não medido | Benchmarks #60; não otimizar antes de medir |
| L12 | P0 de produto | Sem framework/modelo/agent memory ou First Real Run integrado | S1b só prova owner/host sintético; G3–G8 da readiness permanecem NÃO SATISFEITOS |

## 3. Decisões e recomendações: resolvido x pendente

| ID | Recomendação / pergunta | Estado S1a |
| --- | --- | --- |
| D01 | Owner é aplicação cognitiva externa ao CSF, CSF valida apenas recibo e bytes opacos | **FIXADO** pelos ADRs aceitos + S0 |
| D02 | Prova mínima: **SQLite do owner separada do SQLite CSF**, WAL/FULL, duas transações independentes, bytes sintéticos | **PROPOSTO**, requer aceite humano do ADR 0013 |
| D03 | Nenhum LLM real, dado de personagem real, cloud, chave de produção, deploy ou credenciais na S1b inicial | **RECOMENDADO** para reduzir risco/custo |
| D04 | `RunWork` é gate cognitivo; `DispatchFencedWorkV1`/resume são terminais causais determinísticos | **CONSTATADO** no código; contrato de host ainda a aprovar |
| D05 | Teste inicialmente single-operator, single-host, um dispatcher por run; ACL multi-tenant de produção separada | **PROPOSTO**; não confundir com aprovação de segurança multiusuário |
| D06 | Qual host de cognição, domínio de dados privados, validação de tenant e armazenamento de chaves existirão em produção? | **PENDENTE**: design do produto ainda não existe |
| D07 | Onde armazenar dados remotamente, qual contrato de flush/restore/backup e qual custo/infra? | **PENDENTE** de decisão do operador (#58); fora do demonstrador |
| D08 | RPO/RTO, retenção, rotação/revogação de chaves, disaster recovery e restore drill | **PENDENTE** de política operacional e arquitetura de produção |
| D09 | Sem alterar wire schema V1/V2/codec/identity; locator opaco conforme `machine-id`; chaves de owner fora do core | **FIXADO** pelos contratos atuais |

**Decisão humana mínima para liberar S1b:** aceitar/rejeitar a arquitetura local proposta; se rejeitada, selecionar alternativa que demonstre durabilidade com os mesmos negativos. S1a **não** conclui #58 ou #74; somente fornece pacote de decisão.

## 4. Plano faseado após aprovação (sem execução nesta PR)

### Etapa B0 — intake/contrato (pré-condição)

- Aprovar ADR 0013, baseline da issue #74 e quem pode abrir o owner local. Não inventar um serviço produtivo. Registrar tenancy e armazenamento de chave **para testes sintéticos** (pode ser ephemeral key em ambiente isolado).
- Definir o menor ponto de integração do host **fora do core**, e uma matriz `entrypoint -> autorização -> Gate.RunWork / operação determinística -> teste`. Não introduzir dependência circular entre `internal/csf` e cognition.
- Prever comportamento em caso de owner perdido: erro fechado; **nunca** reconstruir a partir do modelo ou converter escrow CSF em owner autônomo.

### Etapa B1 — persistência owner concreta, sem host/modelo

- Implementar um storage *owner-owned* independente, com transação e WAL/FULL validados, checkpoint imutável, locator opaco e ACK persistente. Contrato imutável de cobertura, identidade, hash e assinatura Ed25519 já existente.
- Provar `PersistAndSign` depois de commit durável; novo processo carrega checkpoint exato; retry idempotente; rejeição de conteúdo conflituoso; teste de arquivo ausente, corrupção, colisão e key mismatch.
- Não afirmar sobrevivência a power-loss/host loss sem teste representativo.

### Etapa B2 — tracer bullet Stage → snapshot → restart → Restore/ACK → Work

- Comprovante inicial: sintéticos `run A/actor A` com chave registrada, primeiro snapshot válido e callback `workCount` que não roda até ACK. Reabrir **CSF e owner** após encerrar processos; revalidar mesmo bytes, receipt, snapshot/tip.
- Executar no mesmo caminho autorizado somente um callback de trabalho cognitivo sintético. Não chamar LLM. Bloquear bypasses na superfície demonstradora e não afirmar cobertura de host que ainda não existe.
- E2E negativo: `tenant/actor/run` trocados (tenant só existe no host), recibo/locator/snapshot trocados, ACK perdido, snapshot histórico, modificações concorrentes e callback fora do gate.

### Etapa B3 — failure injection, forks, auditoria

- Falhas antes/depois de owner persist, assinatura, escrow, snapshot, restore e ACK; recomeçar processos em cada caso; verificar **nenhum WORK indevido** e nenhum estado CSF parcial.
- Fork após V1 terminal, child V2 reatesta e restaura sob child run; falha no child bootstrap exige replay e recuperação sem novo callback cognitivo.
- Não altere codec, schemas, cinco ledgers ou policy. Compare replay, snapshot, headers/cursors/hashes e isolamento; controles negativos sem reduzir asserções.

### Etapa B4 — validação e gates de fase

- Comandos a confirmar pela implementação/CI: `gofmt -l` (arquivos Go tocados), `git diff --check`, `go test -count=1 ./internal/csf/ownercheckpoint ./internal/csf/commit ./internal/csf/persistence/sqlite`, `go test -race -count=1 ./internal/csf/ownercheckpoint ./internal/csf/commit` e `go test -count=1 ./...`. Usar versões pinadas e limitar concorrência para testes com SQLite compartilhado.
- Comprovar cenários em processos independentes e fault injection com comandos e exit codes, não apenas mocks. CI no HEAD exato; fallback de billing deve ser `CI_NOT_RUN` se job não executou.
- Só depois implementar/revisar/corrigir em fases separadas conforme `AGENTS.md`; sem merge/produção automáticos. Se um achado não convergir, registrar NO-GO em vez de esconder.

## 5. Matriz de testes a exigir de S1b (testes ainda NÃO executados)

| Cenário | Esperado |
| --- | --- |
| Same cut + bytes iguais após crash em owner commit pré-assinatura | Mesmo locator/receipt quando retried; sem modelo e sem segunda publicação |
| Crash após assinatura mas antes de escrow | Checkpoint órfão no owner, nenhum novo snapshot; retry idempotente |
| Crash após escrow antes de snapshot | Escrow íntegro e snapshot ausente; nova sessão verifica e publica no corte correto |
| Crash depois do snapshot antes de restore/ACK | Novo processo sem permit; restauração exata e ACK necessários |
| ACK perdido, atraso ou concorrência de restore/stage | Permissão revogada ou não concedida; callback não chamado |
| Key falsa, locator arbitrário, recibo de outro ator/run/fork | Rejeição antes de WORK e sem vazamento de bytes |
| Tenant não autenticado ou tenant diferente do run | Host não chama owner/CSF privilegiados (quando ACL existir); prova local deve ser explicitamente single-operator |
| Snapshot corrompido, alterado ou fora do live tip | Falha fechada; não há retorno silencioso à memória antiga |
| Reinício do owner e do host, reabertura do CSF | Bytes idênticos + restore efetivo + ACK persistente + novo permit apenas após verificação |
| Fork divergente de filho V2 | Child run separado, keys/receipts válidos, sem memória do pai como estado novo do filho |
| Replay contínuo × replay × múltiplos resumes | Estado e cinco ledgers canônicos idênticos, sem geração por LLM |
| Dupla sessão/writer ou callback durante avanço do tip | Prova de exclusão no demonstrador; capacidade multi-host é NO-GO até fencing separado |
| Content leaked to actor B / world truth / logs | Rejeitar / redigir / não produzir evento de mundo; CSF continua opaco |

## 6. Avaliação desta S1a — o que foi feito e o que não foi

| Critério da issue #74 / S1a | Evidência deste pacote | Situação |
| --- | --- | --- |
| Inventariar entradas WORK/LLM, recovery, fork, provider | Tabela §1 com símbolos e paths observados | **DOCUMENTADO**; prova de completude runtime futura pendente |
| Propor ADR curto com ownership, persist/restore/ACK/keys | [ADR 0013](../adr/0013-cognitive-owner-durability-s1.md) | **PROPOSTO**, não aprovado |
| Escolher recomendação local vs remoto, riscos/custos | ADR §Alternativas e decisões D02/D07 | **PROPOSTO**: SQLite owner local sintético; remoto pendente |
| Fronteira CSF/host/owner, isolamento e privacidade | ADR + mapas §1–3 | **ESPECIFICADO**, não implementado |
| Testar o adapter e provar recuperação externa | Etapas B1–B4 e §5 | **NÃO EXECUTADO**; pertence S1b/#58 |
| Autorizar S1b e liberar release | Requer decisão humana + testes/review posteriores | **NÃO AUTORIZADO / NO-GO** |

**Próximo ato técnico após aceitar o ADR:** Etapa B1 da S1b, isolada e sem LLM.  
**Próximo ato se a decisão de storage/ownership não for aceita:** registrar a alternativa escolhida e atualizar ADR/ticket, **não** implementar por suposição.

### Referências de rastreabilidade

- [Issue #74](https://github.com/carloshgalves/cote-ai-simulation/issues/74) · [Issue-mãe #58](https://github.com/carloshgalves/cote-ai-simulation/issues/58) · [PR S0 #72](https://github.com/carloshgalves/cote-ai-simulation/pull/72)
- `internal/csf/ownercheckpoint/gate.go`, `internal/csf/commit/owner58_s0_integration_test.go`, `internal/csf/commit/resume_runner.go`, `internal/csf/commit/resume_dispatch.go`, `internal/csf/commit/fork_after_recovery_v2.go`, `internal/csf/persistence/sqlite/epistemic_owner_receipt_v1.go`.
- [Matriz do primeiro run](product-readiness-2026-11-10.md) · [Estado do projeto 12/11](project-state-2026-11-12.md).
