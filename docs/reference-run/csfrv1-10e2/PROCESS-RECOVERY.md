# CSFRV1-10/E2 — crash, retomada durável e falhas atômicas

**Issues:** [E2 #141](https://github.com/carloshgalves/cote-ai-simulation/issues/141) → [pai #119](https://github.com/carloshgalves/cote-ai-simulation/issues/119) → [épica #114](https://github.com/carloshgalves/cote-ai-simulation/issues/114). **Predecessora:** E1 [#140](https://github.com/carloshgalves/cote-ai-simulation/issues/140), mesclada na PR #145 (`main@1b01dc563333aa92c78e9b62ca64a20e80d42194`).

## Autoridades e contrato de recuperação

O produtor Python A1 executa uma época, conclui e sela os dois arquivos da fonte, é validado pelo supervisor Go, assina um `ApprovedCut` vinculado à extensão física do Genesis e o ingressa no `InputLedger`, com recibo e `SourceClosure`. Somente **após esse ingresso validado** a E2 consegue recuperar a execução do terminal sem ter a chave privada do produtor nem os antigos handles `memfd`.

O ciclo congelado é recuperado pela sequência nativa `ResumeRegisteredWorkV1 → VerifySnapshotV1 → DrainPendingPerceptionV1 → ReopenFencedCycle → DispatchFencedWorkV1`. Para fonte tipo 1 (`physical.a1`), o dispatcher exige ID/digest da fonte e payload completo no **AdmissionView**, manifesto físico byte a byte igual ao bundle instalado, assinatura Ed25519 válida, Genesis/policy corretos e `ENGINE_ONLY`. `RecoverApprovedCutFromDurableIngress` valida a projeção física e cria um **cut restrito à conclusão do terminal**. `StagePhysicalInput` rejeita expressamente qualquer cut dessa categoria.

O único caminho capaz de publicar a mutação segue sendo **`CommitPhysical` → `SQLite.CommitWork`**, com CAS, journal, WorldRevision e EventStore atômicos. O novo código não permite refazer a física, criar um SourceDeclaration, enviar BodyState para agente cognitivo ou emitir uma assinatura após o crash.

## Cenários executáveis e contagem esperada

| Família | Injeção/recuperação | Oráculo |
|---|---|---|
| **Ingestão pré-fence** | Fonte A1 assinada ingressa, SQLite é fechado; nova instância reabre cursor/closure, reenvio idempotente da mesma fonte, fence só depois do reinício | Zero eventos/commits antes do fence, um depois |
| **Commit físico, sete etapas** | Injetar erro interno em `CommitPhysical` nas etapas **1–7**; conferir rollback; reabrir no **processo filho**, efetuar a conclusão e provocar `os.Exit(86)` imediatamente após commit | Transcrição canônica antes/depois do rollback igual; segundo restart sem commit novo; transcrição final idêntica à execução contínua sobre o mesmo `ApprovedCut` |
| **Mesmo run C4+D3, duas épocas** | Seis combinações `epoch 1/2 × failpoint 1/4/7`; cada falha deixa sete ou oito commits íntegros, recupera da snapshot C4 e continua até nove | Mesmos **dois cortes reais A1** assinados (congelados após supervisão inicial) em baseline e em cada execução; `referenceE1Compare` igual para **três** bancos, incluindo os proprietários A/B |
| **Fonte adulterada** | Atualizar de propósito **a linha de InputLedger cujo ID está no fence físico** e não uma linha arbitrária | `inputledger.Open` ou `ResumeRegisteredWorkV1` rejeita, sem nova WorldRevision/commit |
| **Autoridade e sigilo** | Recuperar com RunID errado, assinatura alterada, digest corporal falso, reapresentar cut recuperado como fonte nova | `FAIL_CLOSED`, nenhum registro adicional |
| **Controle cognitivo/epistêmico** | Rerun da suíte C3 abort/retry e R58 B3 processo/restore/ACK e barreira de percepção/owner em snapshot | Estados e ACKs consistentes; sem KnowledgeInput/percepção duplicados |

Os casos de recuperação em processo filho **não** compartilham objetos Go, `Coordinator`, `inputledger.Ledger`, handles da fonte nem chave privada do produtor. Recebem apenas o **caminho do SQLite, o manifesto público da extensão e os identificadores de RunID/snapshot**, reconstruindo política, ingresso e fence por leitura persistida. O código do filho usa `os.Exit(86)` após o commit e não executa `defer Close`, exercitando o journal SQLite na reabertura posterior.

A comparação E1 inclui esquema, tabelas, colunas e todas as linhas canônicas, não somente `CycleCommit` ou hash de mundo. Para comparar os bytes de duas execuções, os inputs exógenos A1 têm de ser os **mesmos**: capturas diferentes contêm `wall_time` real e não são comparáveis como se fossem idênticas.

## Reproduzir (Linux)

```bash
python -m pip install --disable-pip-version-check --no-input -e '.[dev]' \
  'numpy==2.5.4' 'pydantic==2.14.3' 'PyYAML==6.0.3' \
  'pytest==9.2.0' 'jsonschema==4.26.0' 'hypothesis==6.170.15'
python -m tests.architecture.emit_go_physical_fixture --out /tmp/physical-go-fixture-e2
find src/embodiment -type d -name __pycache__ -prune -exec rm -rf {} +
mkdir -p /tmp/e2-evidence
PHYSICAL_A1_PRODUCER_TEST=1 CSF_E2_EVIDENCE_PATH=/tmp/e2-evidence/matrix.json \
  go test -count=1 -v ./internal/csf/commit -run '^TestReferenceE2'
PHYSICAL_A1_PRODUCER_TEST=1 go test -race -count=1 ./internal/csf/commit \
  -run '^TestReferenceE2(RecoveredCutCannotCreateNewSourceOrCrossRun|IntegratedC4D3CrashResumeByteExactAfterBothEpochs)$'
go test -count=1 ./internal/csf/commit -run \
  '^(TestReferenceC3IndeterminateAbortRetryAndC1Continuation|TestReferenceC3AbortAndRetryFaultInjectionAtomic|TestCSFRV18R58B3ProcessCrashMatrix|TestCSFRV18RunnerDrainsIdlePerceptionWithoutWorkHandler|TestCSFRV18RunnerNilHandlerRejectsFrozenWork)$'
```

O arquivo de evidência é criado com `O_EXCL`/0600, **sem overwrite**: uma segunda execução deve usar novo caminho. O `CSF_E2_EVIDENCE_PATH` produz apenas contagens, nomes de casos e digests SHA-256 das estruturas verificadas; não exporta a base SQLite, BodyState, assinatura privada, checkpoint cognitivo nem payload de fontes. [Workflow E2](https://github.com/carloshgalves/cote-ai-simulation/actions/workflows/csfrv1-10e2-process-recovery.yml) publica o artefato com retenção de sete dias.

## Não escopo / riscos ainda abertos

- Retomar **o processo Python A1 enquanto ainda está produzindo arquivos** ou o estado interno PSV1-3–8 permanece **não implementado**; esta E2 admite somente uma fonte **já supervisionada e duravelmente ingressada**.
- Testes de falhas SQL são rollback simulado **antes do efetivo commit da transação**. O desligamento abrupto real do processo de recuperação ocorre **imediatamente após o commit**. Não confundir com teste de interrupção de energia física, perda do disco, `fsync` no hardware ou testes de crash de todas as chamadas do SO.
- E3 #142: matriz adicional de snapshot/resume cognitivo; E4 #143: forks; E5 #144: permutações seeded/corrupções extensivas e aceite cruzado. Nenhuma é validada por esta PR.
- Gates independentes #93/#94/#113 e revisão externa permanecem OPEN. Zero findings da revisão interna não autoriza produção nem mesclagem automática.

## Registro de revisão

A primeira execução integrada encontrou uma falha **no fixture negativo**: ele consultava o snapshot corrente do ledger, e não o `AdmissionView` congelado. Após a correção, o teste usa exatamente a visão autenticada que o dispatcher utiliza. A inspeção adversarial também exigiu mutação **da fonte física assinada específica**, teste do bloqueio explícito de restaging e recriação integral do YAML após erro de substituição. Resultados finais e SHA devem constar na descrição da PR #147 depois da reexecução da CI.
