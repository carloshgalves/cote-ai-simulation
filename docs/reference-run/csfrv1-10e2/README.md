# CSFRV1-10/E2 — crash/reopen do mesmo cenário causal

**Issue:** [#141](https://github.com/carloshgalves/cote-ai-simulation/issues/141), filha de [#119](https://github.com/carloshgalves/cote-ai-simulation/issues/119). **Baseline:** E1 [PR #145](https://github.com/carloshgalves/cote-ai-simulation/pull/145), merge humano `1b01dc563333aa92c78e9b62ca64a20e80d42194`.

## Cenário que precisa ser preservado

O cenário é **C4 + duas épocas A1 reais autenticadas por Python supervisor + Go/SQLite**: nove commits Go; três bancos SQLite distintos (CSF e owners A/B), duas fontes exógenas, SourceClosure(final) de física somente na época 2, dois eventos `physical.advanced` ENGINE_ONLY, claim privado e checkpoint dono.

A fatia E1 disponibilizou um oráculo test-only que captura `sqlite_master`, todas as tabelas causal/owner, classes SQLite e as **linhas completas CBOR byte a byte**. A E2 usa esse oráculo no mesmo cenário, não um novo benchmark artificial.

## Falha e recuperação

- Obter dois `ApprovedCut` reais de `physicalhost.RunSupervisedPhysical` **uma vez**, sob Genesis, assinatura Ed25519 e manifesto físico verificados. Reutilizar seus bytes idênticos em todos os testes: a fonte contém `wall_time` autêntico e uma nova execução Python não é input equivalente.
- Executar a D2 com falha injetada `CommitWork` após cada um dos sete passos atômicos, primeiro na época 1, depois na época 2 (**14 casos independentes**). O runner D2 fecha o SQLite ao retornar erro.
- Em uma **nova conexão SQLite**, conferir Genesis original, reconstruir InputLedger com `inputledger.Open` e `ReplayClock`, derivar nova tentativa do mesmo `cycle_id` e `AdmissionFence` a partir de `clock_states` e `world_revisions` duráveis. O passo falho já possui `StagePhysicalInput` e `SourceClosure` persistidos, portanto **não reingressar, reexecutar nem reassinar a época falha**.
- Executar `CommitPhysical` com o `ApprovedCut` original autenticado. Se a primeira época foi interrompida, somente após seu commit completar ingressar a segunda época **já capturada pelo supervisor**, fechar a source final e admitir a nova fence. Se a segunda época falhou, **não** reabrir SourceClosure(final).
- Comparar o resultado contra uma D3 contínua: todos os bytes capturados no oráculo E1 de três bases, `ReplayClock` nove commits, `AuditReferenceD3` read-only/cognitivo/físico e source closure final. Não aceitar apenas contagem final de revisão.
- Negative tests: trocar o cut da época em tentativa congelada e tentar reexecutar uma época já completada. Ambos devem falhar sem alterar qualquer registro persistido.

## Limites e aceitação

A recuperação de E2 pressupõe o **ApprovedCut original** durável/acessível por provedor confiável; o teste congela o cut somente em memória de harness para evitar falsificar dados físicos. O `InputLedger` também contém sua versão assinada que será validada pelo Go em cada retentativa. **Não** significa reiniciar o Python PSV1-8 a partir do checkpoint de processo nem comprovar isolamentos de produção #113.

Diferenciar **falha atômica por `FailAfterStep`** (rollback SQLite + abandono/reabertura da sessão) de **SIGKILL real** entre processos. Os sete passos verificam atomicidade, mas não equivalem à prova operacional de corte de energia, durability fsync ou reinício do próprio produtor. Os estados C3 `HALTED_ON_ABORT`/`RETRY_AUTHORIZED` já contêm restart real documentado, porém E2 também precisa consolidar os testes de falhas em contest, communication, percepção e ACK; qualquer falta deve ser marcada `NOT_COVERED`, não PASS.

**Validação exigida:** RED histórico `03cdfcd7`, teste completo com 14 falhas, wrong-epoch, duplicate-restart, `go test -race` da época 2, regressão D2/D3/C4/SQLite, `go vet` e revisão adversarial após cada correção. **Nenhuma PR é mesclada automaticamente**. E3 #142 (snapshot/resume) e E4 #143 (fork) continuam separadas e gates #93/#94/#113 continuam não homologados.

### Executar localmente

```bash
python -m pip install --disable-pip-version-check --no-input -e '.[dev]' \
  'numpy==2.5.4' 'pydantic==2.14.3' 'PyYAML==6.0.3' \
  'pytest==9.2.0' 'jsonschema==4.26.0' 'hypothesis==6.170.15'
python -m tests.architecture.emit_go_physical_fixture --out /tmp/physical-go-fixture-e2
find src/embodiment -type d -name __pycache__ -prune -exec rm -rf {} +
PHYSICAL_A1_PRODUCER_TEST=1 go test -count=1 -timeout=25m -v ./internal/csf/commit -run '^TestReferenceE2'
PHYSICAL_A1_PRODUCER_TEST=1 go test -race -count=1 -timeout=20m ./internal/csf/commit \
  -run '^TestReferenceE2FourteenAtomicPhysicalFaultsReopenAndConvergeOnExactD3$/epoch2-commitwork-step7$'
```
