# CSFRV1-10/D3 — ENGINE_ONLY e aceite físico+cognitivo cruzado

Issue [#136](https://github.com/carloshgalves/cote-ai-simulation/issues/136), pai [#118](https://github.com/carloshgalves/cote-ai-simulation/issues/118), épica [#114](https://github.com/carloshgalves/cote-ai-simulation/issues/114). Continuação da D2 [PR #138](https://github.com/carloshgalves/cote-ai-simulation/pull/138), mesclada pelo operador em `main@2778bef76ed02858792555f3fd57d620ba226562`.

**Escopo:** teste local experimental; esta fatia NÃO é homologação A1, release, sandbox de produção nem revisão independente.

## Contrato do teste integrado

`RunReferenceD3` exige uma execução autêntica de `RunReferenceD2`, que por sua vez executa C4+C2+C1+C3 e duas épocas reais A1 Python supervisionadas: **um SQLite, um RunID, um Genesis, nove CycleCommits (6 + NoProposal + 2 físicos)**. O auditor lê o banco persistido, não um relatório sintético do produtor.

| Fronteira | Evidência independente e falha fechada |
|---|---|
| Entrada física | Duas identidades `physical.advanced` referenciadas nos journals Go e realmente persistidas no EventStore; produtor `physical.a1`, classificação fechada, nenhum observer/rota |
| Planner genérico | Varredura de `causal_outbox`: zero `PerceptionTask` apontando para eventos físicos; versões com `kind`, `producer` ou `EventOrderKey` físicos reclassificados são rejeitadas, não desclassificadas para rota ordinária |
| EvidenceLedger | Varredura completa, inclusive evidência posterior ao snapshot: não pode conter marcadores, identidades nomeadas nem blob integral do estado físico |
| A / B | Leitores `OpenPOVObservatory` vinculados às duas `SlotDispatch` C2 autenticamente persistidas; nenhuma exposição de EventStore, BodyState ou tokens físicos; Claim falsa exclusiva de B, recebida em t=35; A não recebe essa Claim |
| Observatory público | `OpenPublicObservatory` valida replay e devolve `NO_REGISTERED_PUBLIC_ROUTE` com zero publicações; não faz fallback de EventStore para publicação |
| Replay e snapshot | `ReplayClock` verifica 9 commits; SnapshotV1 C4 de dois proprietários verifica três commits de cauda e prefixos históricos; cinco ledgers re-hasheados antes/depois de leitores somente-leitura |
| Output | `ReferenceD3PublicReport` independente e explicitamente allowlisted, sem corpo, capacidade, RNG, seed, identidades físicas, payload de Claim, world hash, chaves ou contagem de evidência por ator |
| Falhas adversariais | Rotas de holder B falsificadas, tarefa genérica forjada, tipo/produtor físico adulterado, consulta pública de RunID estrangeiro, `SlotDispatch` B apresentada como A e controles de classificação do planner: recusados |
| Permutação | As **mesmas duas capturas reais A1 já supervisionadas e assinadas** são reaplicadas com ordens de fontes e workers invertidas; comparar digest dos cinco ledgers, EventIDs, WorldState final, SnapshotID e matriz de disclosure |

**Limite de determinismo:** capturas A1 Python independentes têm `wall_time` genuíno, portanto hashes brutos não precisam coincidir. A prova de permutação reutiliza os *mesmos cortes reais* assinados somente em fixture package-private; a execução normal chama Python duas vezes. Não foi provada recuperação A1 por snapshot entre processos.

## Matriz de aceite pai #118

| Critério do #118 | D2/D3 e ressalva |
|---|---|
| Python A1 real, cut autenticado, Go, InputLedger, AdmissionFence, duas épocas no mesmo RunID | **D2:** `TestReferenceD2RealTwoEpochsOnSameC4RunID`, consumido no caminho D3 real; rev. 8/9 |
| Plan incompleto, falsa assinatura, hash errado, run estrangeiro, arquivo alterado, source aberta | **D2:** controles negativos; D3 não duplica autoridade de A1 |
| Policy, versão, Genesis, import sem fallback; digest rastreável | **D2:** validações A1 e re-check D3 de registration ENGINE_ONLY; gate externo pendente |
| Campos físicos confidenciais A/B/público e Observatory | **D3:** auditor de ledger, planner, POV, Observatório e permutação |
| Laboratório versus produtor operacional atestado | **NO-GO externo:** #94/#113, same-UID, descendentes de process group, dependências transitivas e auditoria independente permanecem não homologados |
| Revisão adversarial, remediação, re-review com SHA | **D3:** workflow dedicado e revisão documentada na PR; somente indicar concluído após CI na HEAD exata |

Não confundir este snapshot C4 com a prova de recuperação/fork exigida na [#119](https://github.com/carloshgalves/cote-ai-simulation/issues/119), nem este Observatory com homologação completa da [#120](https://github.com/carloshgalves/cote-ai-simulation/issues/120).

## Reproduzir no Linux

Ambiente de referência: Go 1.27.2, Python 3.13.18, SQLite moderno autenticado. Instale dependências do harness:

```bash
python -m pip install --disable-pip-version-check --no-input -e '.[dev]' \
  'numpy==2.5.4' 'pydantic==2.14.3' 'PyYAML==6.0.3' \
  'pytest==9.2.0' 'jsonschema==4.26.0' 'hypothesis==6.170.15'
python -m tests.architecture.emit_go_physical_fixture --out /tmp/physical-go-fixture-d3
find src/embodiment -type d -name __pycache__ -prune -exec rm -rf {} +
PHYSICAL_A1_PRODUCER_TEST=1 go test -count=1 -v ./internal/csf/commit -run '^TestReferenceD3'
PHYSICAL_A1_PRODUCER_TEST=1 go test -race -count=1 ./internal/csf/commit -run '^TestReferenceD3RealPythonPrivateEventAndLiveObservatory$'
go test -count=1 ./internal/csf/perception ./internal/csf/contextboundary ./internal/csf/persistence/sqlite
go vet ./internal/csf/commit ./internal/csf/persistence/sqlite
```

`PHYSICAL_A1_PRODUCER_TEST=1` é obrigatório para não contar SKIP do produtor real como PASS. A validação de CI dedicada é [D3 Actions](https://github.com/carloshgalves/cote-ai-simulation/actions/workflows/csfrv1-10d3-disclosure.yml). Consulte resultado/SHA da execução, não presume verde em job ainda pendente.

## Governança

A PR da D3 permanece **Draft e merge exclusivamente humano**. Um `go test` local não homologa A1; esta revisão, mesmo adversarial, é interna e não equivale a revisão independente. As issues #94, #113, #119–#123 e #114 só mudam de estado pelos próprios critérios documentados.
