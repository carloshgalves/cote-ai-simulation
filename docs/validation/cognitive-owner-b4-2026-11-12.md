# B4 — matriz integrada de aceitação S1b do owner cognitivo local

**Issue:** [#79](https://github.com/carloshgalves/cote-ai-simulation/issues/79) · **Draft PR:** [#83](https://github.com/carloshgalves/cote-ai-simulation/pull/83) · **Base:** `main@6c09ef2393ec87535d69c3c7c2204b13a16d9458` (merge da B3/#82). **Norma:** [ADR 0013](../adr/0013-cognitive-owner-durability-s1.md), uso exclusivamente local, por um operador humano.

## 1. Limites do objeto aceito

A B4 avalia **o demonstrador sintético de persistência e admissão de cognição**, e não a disponibilidade do primeiro exame real com personagens autônomos. O único callback cognitivo concreto desta fatia está em `internal/csf/ownercheckpoint/localdemo/host.go`. O projeto **não tem um host produtivo de LLM** e as funções `DispatchFencedWorkV1` e `ResumeRunV1` executam WORK **causal determinístico**, não pensamento de agentes.

Dois arquivos SQLite permanecem distintos: o owner persiste estado e ACK privadamente; a CSF registra escrow assinado e snapshot. **A CSF mantém cópia opaca dos bytes de memória no escrow**, por contrato R22: opaco não significa ausente da CSF. Esses bytes não se tornam world truth, percepção ou relato de agente. Um chamador privilegiado de `LoadCognitiveCheckpointV1` precisa respeitar run/ator; o demonstrador não fornece API pública de leitura nem multi-tenancy humana.

## 2. Matriz de requisitos e comandos exatos

| ID | Invariante a conferir | Evidência versionada | Job/comando de validação |
| --- | --- | --- | --- |
| B4-C01 | Owner SQLite WAL/FULL, diretório privado, checkpoint imutável; assinatura após COMMIT | `localowner/sqlite.go`; testes B1 e `commit_order_b4_test.go` | `go test -count=1 ./internal/csf/ownercheckpoint/...` |
| B4-C02 | Escrow Ed25519 e snapshot selado exigem a mesma identidade e bytes | `owner58_s0_integration_test.go`, `owner58_b2_integration_test.go` | `go test -count=1 ./internal/csf/commit -run '^TestCSFRV18R58(S0\|B[23])'` |
| B4-C03 | Sem WORK antes de Restore+ACK e sem permissão de sessões anteriores | `gate_test.go`, B2 e B3; `localdemo.Host.Run` | Testes S0, B2, B3 e AST de `localdemo/host_b4_test.go` |
| B4-C04 | Crash real de processo em seis pontos; reabrir ambos os bancos, recuperar bytes e ACK | `owner58_b3_process_test.go`, `cognitive-owner-b3-2026-11-12.md` | B3 focal e `-race` |
| B4-C05 | Fork V1→V2 precisa autenticar origem e reatestar recibo novo ligado ao run filho | `owner58_b3_fork_test.go`, `owner58_b3_forkcrash_test.go` | B3 focal, `TestCSFRV18` race e fork vectors |
| B4-C06 | Corridas Stage/Restore/ACK não concedem permissão antiga | `gate_test.go`; barreiras de canal B3 | `go test -race -count=1 ...` focal |
| B4-C07 | Rejeitar actor/run/receipt/snapshot cruzados, adulteração, key errada e histórico fora do tip | Testes B1–B3, S0 e verificação do snapshot | Gate focal + `TestCSFRV18` |
| B4-C08 | Cinco ledgers causais não são alterados por owner/escrow/ACK ou fork | `b3LedgerDigest` e controle de lineage/replay | `TestCSFRV18R58B3` + regressão V1/V2 |
| B4-C09 | O callback do *host sintético* só é referenciado por `Host.Run` → `Gate.RunWork` | `localdemo/host_b4_test.go`, com fixtures positiva e negativa | `go test -count=1 ./internal/csf/ownercheckpoint/...` |
| B4-C10 | Versões canônicas Go e TypeScript produzem bytes/resultados iguais em **621 vetores V1** | `cmd/csf-harness`, `tools/csf-conformance-ts`, bundle V1 | Executar ambos, `cmp` de relatórios JSON e SHA-256 |
| B4-C11 | O Go não sofreu regressão global no HEAD da aceitação | Pacotes Go do repo | `go test -count=1 -timeout=30m ./...` |
| B4-C12 | Continuidade V1/V2, resume, fork e corrompidos passam pelo detector de corrida | `internal/csf/commit`, `internal/csf/fork` | `go test -race -count=1 -timeout=30m ./internal/csf/commit -run '^TestCSFRV18'` |
| B4-C13 | Código versionado formatado, diff limpo e análise estática | Go B1/B2/B3/B4 + workflow | `gofmt -l`, `git diff --check origin/main HEAD`, `go vet` |
| B4-C14 | Resultado auditável no **SHA real da branch**, não na ref sintética `pull/*/merge` | Workflow B4 usa `github.event.pull_request.head.sha` e verifica `git rev-parse HEAD` | Todos os 4 jobs da CI, mesmo HEAD |
| B4-C15 | Zero findings acionáveis de B4 após review e remediação | Comentários `cote-review-finding:v1` em PR #83 | Re-review do diff B4, sem atribuir independência ao implementador |

**Orquestração:** [workflow B4](../../.github/workflows/b4-owner-acceptance.yml) apresenta quatro jobs paralelos: `owner-integration`, `full-go`, `v1-v2-continuity`, `canonical-contract`. Cada job falha em qualquer comando não aprovado. Os resultados e links de execução do **HEAD final** serão registrados na discussão da PR #83 e na issue #79, porque um link a um run antigo não prova a revisão atual.

## 3. Inventário de entrypoints cognitivos — limite preciso

| Superfície atual | Tipo | Cobre gate cognitivo? | Parecer |
| --- | --- | --- | --- |
| `ownercheckpoint.Gate.Stage`, `RestoreAndAcknowledge`, `RunWork` | Contrato de confiança | Gate normativo | Testes S0, B1–B3 e negativa B4 |
| `localdemo.Host.StageSnapshot`, `Restore`, `Run` | Host **sintético local** | **Sim**: o callback armazenado é despachado por `Gate.RunWork` | Proteção por API, testes + guarda AST limitada ao pacote |
| `localowner.Store.PersistAndSign`, `Restore`, `Acknowledge` | Owner de bytes privados | Adaptador concreto, não despacha LLM | Testes B1/B3; não confundir commit com decisão de agente |
| `commit.Coordinator.DispatchFencedWorkV1`, `ResumeRunV1`, `SettleFencedThenForkV2` | Causalidade/WORK **determinístico** | Usa seus próprios fences; **não** é chamada de modelo | Não acoplar a LLM sem novo ADR/testes |
| Host de LLM de personagem, memória semântica real, loop de decisão autónomo | **Ausente nesta baseline** | **Não comprovado** | Impede alegação de proteção universal de cognição e primeiro run real |

O teste AST B4 é propositalmente conservador quanto a acessos ao campo `work` dentro de `localdemo`; só protege esse pequeno pacote. Ele não demonstra que outra linguagem, futuro host, script ou biblioteca de modelos nunca invocará cognição sem Gate. O usuário é o único operador humano, **não** o único personagem.

## 4. Limitações que não podem ser convertidas em PASS fictício

1. **Commit → assinatura:** B1 executa `tx.Commit()` antes de `ed25519.Sign`; um teste AST com negativo invertido verifica a ordem na fonte. Os subprocessos B3 morrem antes do persist ou **após `PersistAndSign` retornar**, mas **não** no intervalo exato entre COMMIT e assinatura. O teste estático não equivale a um crash real nesse intervalo.
2. **Host/power-loss:** `os.Exit(86)` prova processo encerrado sem `Close`, não kernel crash, queda de energia ou perda física do computador. Esta última não é requisito do projeto pessoal.
3. **Concorrência:** a prova assume um host autorizado por run; `sync.RWMutex` não implementa fence distribuído nem congela transações de outro processo durante um callback. Sem SaaS, multi-tenant ou múltiplos writers humanos.
4. **Conteúdo privado:** o owner usa bytes sintéticos; nenhuma LLM real foi executada, logo não há score de fidelidade de personagem, RAG ou avaliação de conteúdo real. Credenciais dos testes não são chaves de produção.
5. **Canônico V2:** o gate independente de 621 vetores refere-se **somente ao bundle V1**. Não afirmar equivalência TS/Go V2 sem #59.
6. **Produto completo:** faltam G3–G8 (observabilidade, integração física, mecanismos de exame, agentes, primeira execução auditável etc.) conforme [matriz geral](product-readiness-2026-11-10.md). Mesmo uma CI B4 integralmente verde **não libera First Real Run**.
7. **Independência:** a revisão adversarial feita pelo implementador pode resolver findings, mas **não** satisfaz por si só a revisão independente da issue #79. Merge e qualquer liberação continuam decisão humana.

## 5. Decisão operacional

- **GO técnico condicional — demonstrador local S1b:** somente depois de quatro jobs B4 concluídos com `success` no mesmo HEAD, sem findings acionáveis abertos, evidências verificadas, revisão independente e aceite humano explícito. O GO vale apenas para teste local **sintético**, um operador, sem LLM real.
- **NO-GO — primeiro run real do COTE AI Simulation:** até fechar gates de produto independentes, integrar cognição efetiva e fazer revisão/validação próprias. A B4 não converte um host sintético em plataforma de simulação.
- **Sem conclusão automática:** issue #58 (P0-local), #74 e #79, assim como PR #83, permanecem sob decisão de revisão/merge; não fechar ou publicar automaticamente.

**Protocolo de evidência:** cada linha deve referir comando, exit code, workflow/job, versão e commit real. Falhas de runner sem execução são `CI_NOT_RUN` (não `SUCCESS`); execuções parciais são `IN_PROGRESS` e não validadas. Nenhum dado privado nem corpus protegido integra os logs.
