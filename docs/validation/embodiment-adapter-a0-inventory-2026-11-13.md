# CSFRV1-9A / A0 (#97) — inventário auditado da fronteira Embodiment → CSF

**Data da inspeção:** 2026-11-13  
**Issue-mãe:** [#94](https://github.com/carloshgalves/cote-ai-simulation/issues/94)  
**Fatia:** [#97](https://github.com/carloshgalves/cote-ai-simulation/issues/97)  
**Base imutável inspecionada:** `main@0047fe083ac1d013ecc3296f80610d410cb4f0f6`  
**Natureza:** A0 documental, inventário de contratos e auditoria de completude. **Não implementa adapter, protocolo, schemas nem testes novos.**  
**Estado de prova:** `SOURCE_INSPECTED`, `TREE_ENUMERATED`, `SECOND_PASS_DONE`; **não** `TESTS_EXECUTED`, `INDEPENDENTLY_REVIEWED` ou `RELEASE_APPROVED`.

## 1. Procedimento, cobertura e limites da afirmação

Foi enumerada pela API do GitHub a árvore recursiva e não truncada da `main` com **1.535 arquivos**. Foram identificados e examinados os **12 arquivos Python** de `src/embodiment/` (interfaces, fluxos principais, funções e trechos de implementação relevantes; não foi feita auditoria linha a linha de todos os módulos), os caminhos de persistência e CLI, os testes físicos e seus casos, as definições de modelos e unidades, as ADRs 0006–0011, as duas specs físicas/causais, o ticket normativo 9A, o ticket 10 e os tickets PSV1-0–8. Foram inspecionadas as superfícies do core Go que podem tocar a integração: `domain`, `ports`, `genesis`, `policy`, `identity`, `codec`, `commit`, `settlement`, `reducer`, `extensions`, `perception`, persistência e `cmd/csf-harness`; e os caminhos de testes/CI relevantes.

**Revisão adversarial em segunda passagem:** confrontamos a listagem integral de paths com buscas por `Embodiment`, `BodyState`, `ExertionIntent`, `physical adapter`, `PhysicalAdapter`, `embodimentv1`, `physical_beliefs`, `snapshot.yaml`, `events.jsonl` e `producer_unit_key`. Checamos expressamente ausência de módulos planejados e distinguimos citações documentais de código executável. A inspeção foi refeita quanto a hashes, autoridade temporal, semântica de persistência, política de RNG, dados de fonte, testes e lacunas em arquivos de modelo. **Não se afirma que cada um dos 1.535 arquivos foi lido integralmente**, mas que o universo de paths foi enumerado e as superfícies identificadas como relevantes foram cruzadas. Não houve teste local reexecutado neste trabalho por não haver checkout executável disponibilizado neste ambiente; checks antigos não são resultados de A0. Segunda passagem do mesmo agente **não é revisão independente**.

## 2. Inventário verificável: o que está implementado no Python

| Superfície real | Estado e contrato observado | Evidência |
| --- | --- | --- |
| CLI de componente | **Duas** operações: `seed-cohort` e `advance-clock`; não há servidor ou protocolo request/response do Go | `src/embodiment/__main__.py`, `cli.py` |
| Seeding | Prior populacional → `posterior_for` sem constraints → `seed_character` → baseline imutável, corpo inicial e metadados | `seeding.py`, `prior.py`, `cli.py` |
| Evidência de execução | `events.jsonl` local append-only com `seq,event,sim_time,wall_time,payload`; eventos existentes: `run.started`, `run.extended`, `capacity.sampled`, `cohort.correlation_report`, `body.advanced` | `eventlog.py` |
| Snapshot local | `snapshot.yaml` com `snapshot_version=1`, `world_seed`, `sim_time`, `characters[id].capacity_baseline`, `characters[id].body_state`, `run_metadata`, `snapshot_hash` | `snapshot.py`, `types.py` |
| Corpo persistente | `BodyState` tipado Pydantic, nove canais/estruturas de evolução e `advance` com `dt_hours`/exposure, incluindo sono, fadiga, hidratação, calor, lesões e dor | `types.py`, `dynamics.py`, `capability.py` |
| Numeração/parametrização | `SCHEMA_VERSION=1`, `SNAPSHOT_VERSION=1`; versions `prior` e `dynamics` registradas, `world_seed` e hash posterior por ator | `types.py`, `data/models/physical/*` |
| Aleatoriedade física | Substream nomeado `embodiment.substream.v1|actor|event|purpose`, BLAKE2b para spawn key e NumPy PCG64; sem RNG global | `rng.py` |
| Parâmetros | `population-prior.yaml` (0.1.0-provisional), `body-dynamics.yaml` (0.1.1-provisional), score table não homologada, header JSON Schema e validação das hipóteses `[INT]` | `data/models/physical/`, `modelfile.py` |

### 2.1 Shape efetivo dos arquivos

`events.jsonl` contém **JSONL, não envelopes CBOR globais**. A serialização ordena chaves, recusa NaN e faz `flush()`; não há promessa de `fsync` do arquivo físico nem de transação conjunta com o SQLite causal. O primeiro registro fixa `run_metadata`; continuação checa `world_seed`, hashes dos posteriors e versões, mas **essa checagem local não constitui prova de linhagem/ledger global**. A seq local é incrementada pelo writer físico; não é `LogicalSequence` da CSF.

`snapshot.yaml` é gerado com writer exclusivo e inclui `snapshot_hash = SHA-256(json.dumps(documento sem snapshot_hash, sort_keys=True, ensure_ascii=False, default=str))`. A leitura confere esse digest; `body_state_of` revalida o corpo, enquanto `read_snapshot` isoladamente verifica hash e estrutura raiz, **não** executa o validador de um snapshot causal global. A serialização `as_document` arredonda `float` a nove casas. Esses bytes/hash locais são **dados de origem e proveniência**, não `canonical_bytes_v1`, `CausalIdentityPolicy` ou `WorldRevision`.

**Limite crítico de continuidade:** `advance-clock` lê o **snapshot inicial congelado**, apenda `body.advanced` com intervalo, condições e `channels_changed`, mas deliberadamente **não atualiza o snapshot**. Repetir avanço para o mesmo personagem é recusado; reidratar estado avançado e fazer replay completo pertence ao PSV1-8 ainda pendente. Um adapter **não pode** assumir que o YAML representa o estado atual depois de eventos de avanço, nem reconstruir silenciosamente evolução de um `channels_changed` sem contrato de cobertura e determinismo.

### 2.2 Campos, unidades e disclosure inicial (inventário, não matriz normativa A3)

`CapacityProfile` possui **15 dimensões**, todas com valor e unidade. Mapeamento de armazenamento observado: `aerobic_capacity:laps`, `anaerobic_power:cm`, `sprint_speed:m_per_s`, `max_strength:kg`, `strength_endurance:reps_30s`, `agility:touches_20s`, `flexibility:cm`, `body_mass:kg`, `stature:cm`, `reaction_time:ms`, `coordination:hit_rate`, `injury_resilience:risk_multiplier`, `recovery_rate:h`, `thermoregulation:degC`, `pain_tolerance:percentile` (**exceção com coorte declarada**). Unidades das colunas de bateria podem diferir da unidade interna; `data/canon/schema/enums/capacity-dimensions.yaml` define o vocabulário compartilhado, e testes vigiam drift.

`BodyState` inclui `character_id`, `t_hours`, `w_prime_balance`, `peripheral_fatigue`, `central_fatigue`, `cumulative_load`, `sleep`, `energy`, `hydration`, `thermal`, `soreness`, `injuries`, `illnesses` e `pain`; há estruturas aninhadas com unidades e domínios próprios. `illnesses` é serializado, mas **não recebe dinâmica na PSV1-2**. O `t_hours` é físico/local e `sim_time` pode assumir formato `Y1_START+00001.000h`: **não** define `SimulationInstant` global. `wall_time` é extrínseco e sua normalização é usada em testes locais de determinismo; não pode se tornar origem de causalidade ou identidade.

**Primeira classificação de risco:** baseline, posterior/ESS, BodyState numérico, RNG/substream, intenção de ocultação e telemetria são **engine-only**; futuro contexto de personagem deve receber apenas sinais qualitativos, enviesados e endereçados ao ator/observador. **A allow-list exaustiva, inclusive nested fields, é entrega da A3, não está aqui homologada.**

## 3. Inventário de autoridade Go

| Camada | Existência comprovada | Responsabilidade/limite para o adapter |
| --- | --- | --- |
| `internal/csf/domain/types.go` | `SimulationInstant int64`, `LogicalSequence uint64`, `WorldRevision uint64`, IDs/digests e coords | Tipos e tempo **globais**; nenhum campo Python é conversão direta autoritativa |
| `internal/csf/ports/stores.go` | `Authorities`, `WorkBatch`, `ClockAdvanceBatch`, `ClockAdvanceStore` | Input/Decision/Event/Evidence/Outbox/Snapshot stores separados por papel; `CommitWork`/advance/abort/retry só por fronteira causal |
| `internal/csf/codec/codec.go`, `identity/deriver.go` | CBOR strict + `Deriver.Sum` registrado e SHA-256 | Go deriva id, digest e preimage **após** validação/admissão; hash local do YAML não substitui ID causal |
| `internal/csf/genesis/genesis.go`, `policy/bundle.go` | Verificação do Genesis, extension bundle/schema e pinagem de policies | Novo tipo físico exige versão/registro/allow-list explícitos; não inferir autorização da presença de JSON |
| `internal/csf/decision`, `admission`, `inputledger`, `clock` | Avaliação, fence, source closure e coordenada causal | Entrada física deve ter origem, fechamento e identidade repetível, sem timing de worker |
| `internal/csf/commit`, `settlement`, `persistence/sqlite` | Publicação transacional e persistência WAL/FULL | Só coordinator/port transacional pode formar `CycleCommit`, `Event`, `WorldRevision`; sem transação do Python |
| `internal/csf/reducer/testresource.go`, `extensions/testresourcev1` | **Exemplo sintético** de `resource.consume`, com reducer puro | Não há, por este exemplo, um reducer físico nem registro genérico pronto para consumo arbitrário |
| `internal/csf/perception`, `observatory` | Tasks/recibos e visualizações read-only | PerceptionTask **não** é ObservedPerformance físico já endereçado; visão privilegiada nunca é contexto de ator |
| `cmd/csf-harness/main.go` | Criar run Genesis/SQLite e rodar conformidade | **Não** invoca subprocesso físico nem cenário CSFRV1-10 |

O core Go segue as ADRs 0008/0009/0010/0011: autoridade global única para relógio, admissão, ordem, reducer e publicação; CBOR determinístico/policy registrada; transação SQLite WAL/FULL para o histórico causal. Seu RNG registrado/determinístico é distinto do substream NumPy/PCG64 físico; não mesclar os dois domínios de RNG por coincidência de nomenclatura.

**Ausências confirmadas na árvore no SHA-base:** `internal/csf/adapter/embodimentv1/`, `internal/csf/extensions/embodimentv1/`, `docs/architecture/embodiment-causal-adapter-v1.md`, `tests/architecture/embodiment_adapter_contract_test.go`, `docs/operations/csf-runtime-v1.md`. Nenhuma dessas ausências significa bug da CSF 1–9; constituem o escopo ainda não implementado 9A/10.

## 4. Progresso do PSV1, fontes e testes existentes

| Entrega | Estado de código no SHA-base | Impacto no 9A/10 |
| --- | --- | --- |
| PSV1-0 | ADR 0007 e decisões documentadas | Antecedentes aceitos |
| PSV1-1 | Seeding/priors, snapshot físico, log e RNG nomeado | Entradas reais que A1/A2 podem usar |
| PSV1-2 | Dinâmica temporal/BodyState, capability e testes de invariantes | Demonstra atualização física sem replay do estado salvo |
| PSV1-3 | **Não implementada**: `constraints` não vazias são recusadas por `NotImplementedError` | Estimador/ESS/feats não podem ser atribuídos ao runtime |
| PSV1-4 | **Não implementada**: esforço escolhido, ocultação, outcome, sorteio de lesão | Integração de resolução de desempenho requer novo contrato/implementação |
| PSV1-5 | **Não implementada**: `ObservedPerformance` e crença por observador | Sem autorização de percepções físicas genéricas |
| PSV1-6 | **Não implementada**: interocepção e `SelfPhysicalModel` executáveis | A3 deve reservar apenas projeção qualitativa segura, não afirmar que existe |
| PSV1-7 | **Não implementada**: contest por trocas | Cenário físico de disputa não pronto |
| PSV1-8 | **Não implementada**: reidratação/replay/snapshot físico completo | **Bloqueio para consumir histórico avançado como estado autoritativo** |

A classe `RunMetadata` reconhece `prior`, `dynamics`, `estimator`, `contest_resolver`, `observation_params`, mas a CLI ativa declara somente `prior` e `dynamics`. Não confundir campo previsto na estrutura com subsistema executável.

**Sourcing:** `population-prior` permanece `PROVISIONAL` e `evidence_sufficiency=0`. `body-dynamics` também `PROVISIONAL`, apesar de possuir alguns tempos de referência. `fitness-test-score-table` não foi conferida contra a publicação oficial. `SOURCING.md` documenta S1 normas, S2 correlações, S3 distância da prova de resistência, S4 aferição da tabela. `data/canon/schema/feat.schema.json` e os `open_questions` fornecem contratos de origem, não autorizam capacidades nominais canônicas.

**Testes localizados e inspecionados por funções:** `tests/embodiment/test_{cli,snapshot,seeding,prior,rng,dynamics,capability,modelfile,architecture,body_invariants}.py`, invariantes nomeadas 01/03/04/05/08/09/11/13, properties P1/P3 e cenário `test_scenario_03_exam_week.py`. Cobrem não regressão de seed/log normalizado, snapshot hash/versão, corpo persistente, regras de capacidade, no-LLM/no-RNG-global, unidades, negative constraints e condições limítrofes. Existem **dois xfails explícitos** de calibração de marginais/correlações em `test_prior.py`. A árvore não contém `tests/architecture/embodiment_adapter_contract_test.go`, nem workflow 9A específico; `.github/workflows/product-validation-on-demand.yml` roda regressões Go/Python existentes, **não** prova o adapter inexistente. Nenhum teste foi executado novamente nesta inspeção; status das suites **NOT_RUN para A0**.

## 5. Findings do inventário e decisões a levar às próximas fatias

| ID | Evidência / risco de integração | Tratamento normativo |
| --- | --- | --- |
| **A0-F01 / High** | `advance-clock` gera log posterior ao snapshot imutável; replay/rehidratação PSV1-8 ausente | A2 define fonte e política para corte/snapshot+log, prova de prefixo e recusa de material incompleto; A4 negative test; CSFRV1-10 não assume o snapshot como latest |
| **A0-F02 / High** | Hash Python por JSON de YAML e `seq/sim_time` local não equivalem a CBOR, IDs e tempos globais | A2 versiona transform/normalização, provenance/hash de **origem**, e somente Go deriva identidade, `EventOrderKey`, revisão e `SimulationInstant` |
| **A0-F03 / High** | Não existem handlers/reducer/event types físicos versionados nem interface de subprocesso | A1 define IPC sem autoridade; A2 define registro de schema/extension e pipeline candidate→settlement; implementação só em 10 |
| **A0-F04 / High** | Falta percepção física autorizada PSV1-5/6; risco de converter `BodyState`/posterior em evidência/contexto | A3 classifica nested fields e fixtures de vazamento; qualquer destinatário sem receipt deve receber zero |
| **A0-F05 / Medium** | JSONL faz `flush` mas não é transação causal/garantia de `fsync`; continuidade checa parte da origem mas não ledger/prefixo global | A1 estabelece falhas de IPC, idempotência/retry, limites de crash e autoridade; A2 vincula source hash e identidade de unidade à admissão |
| **A0-F06 / Medium** | Dados físicos provisórios, fontes S1–S4 não fechadas, SIR e exame físico não executáveis | A2/A3 distinguem fixtures sintéticas de canon/admissão real; 10 declara dependências PSV1-3–8 específicas |
| **A0-F07 / Medium** | Componentes `estimator/contest_resolver/observation_params` previstos no tipo metadata não estão ativos no CLI | A1/A2 recusar versões não instaladas, não declarar suporte prospectivo como versão de execução |
| **A0-F08 / Medium** | Testes de regressão gerais e workflow manual não certificam o contrato 9A, e não há validação independente nesta inspeção | A4 fornece contract tests e review identificável, com CI/SHA exato e aceite humano |
| **A0-F09 / Low documental** | ADR/spec históricos ainda descrevem "nenhum snapshot" ou CSFRV1-9 ausente; o código e merges têm precedência factual | Reconciliação do status operacional rastreada em [#96](https://github.com/carloshgalves/cote-ai-simulation/issues/96); preservar snapshots históricos de evidência, não reescrever ADRs |

**Disposition formal do inventário (não confundir com resolução):** F01 → A2/A4/PSV1-8; F02 → A2/A4; F03 → A1/A2/CSFRV1-10; F04 → A3/PSV1-5/6; F05 → A1/A2/CSFRV1-10; F06 → A2/A3/PSV1-3–8 e sourcing S1–S4; F07 → A1/A2; F08 → A4 e gate de review; F09 → #96. Todos estão **OPEN / TRACKED**, e **0/9 resolvidos tecnicamente** nesta A0. Esta é uma classificação de dependências, não uma correção dos comportamentos ausentes.\n\n**Remediação documental realizada na A0:** as declarações de cobertura foram restringidas às inspeções efetivamente feitas; F09 agora remete explicitamente à #96, sem confundir documento histórico com estado atual. Não há findings de defeito no *artefato de inventário* ainda sem encaminhamento, mas os nove riscos técnicos continuam em aberto.\n\nSão **findings de lacuna/integração futura**, não alegações de regressão executada nem defeitos recém-provados em CSFRV1-1–9. A0 não corrige código e não torna a CSFRV1-10 implementável por si só.

### Decisões explicitamente não tomadas na A0

- **A1:** framing, subprocess lifecycle, retries, deadline, limites de mensagem, timeout, controle de versões e crash behavior.
- **A2:** schema/extension bundle, tratamento de ordem local/tempo lógico, perfil de mapeamento numérico e digest de origem; política de reconstrução/aceite de log+snapshot que não disponha de PSV1-8.
- **A3:** classificação exaustiva de campos, política de observador/destinatário, redaction de stderr/log, evidências negativas e impossibilidade de privileged→actor.
- **A4:** runner executável de conformance 9A, mutantes, equality de bytes, CI no SHA exato, auditoria independente e aceite humano.
- **CSFRV1-10 / PSV1:** processo real e resolução física integrada, retomada de estado, cognição, exames, fonte primária e demais capacidades pendentes.

## 6. Segunda inspeção adversarial — resultado

| Controle de completude | Resultado |
| --- | --- |
| Enumerar árvore inteira do SHA-base sem truncamento | **CONFIRMADO:** 1.535 arquivos; diretórios previstos 9A/10 não presentes |
| Cruzar todos os módulos Python, entrypoints e artefatos efetivos | **CONFIRMADO:** 12 módulos, CLI de duas operações, YAML+JSONL, hash, RNG, versões e erros |
| Cruzar definição de tipos com fonte normativa de unidades e modelos | **CONFIRMADO:** 15 dimensões, BodyState e seus campos, parâmetros e sourcing S1–S4 |
| Cruzar PSV1-0–8 com existência de classes e testes | **CONFIRMADO:** implementadas 1/2; 3–8 sem resolver executável, com ressalva de estruturas parciais |
| Cruzar CBOR/identidade/ports/commit/reducer/percepção com adapter físico | **CONFIRMADO:** core disponível e testresource sintético; ausência de extensão física |
| Verificar se docs/skills/testes/CI já executam contract adapter | **CONFIRMADO:** nenhuma implementação/teste/workflow 9A encontrado; existem apenas testes de componente |
| Revisitar caminhos alternativos via buscas por símbolos e extensões | **CONFIRMADO:** searches por Embodiment, BodyState, ExertionIntent, paths YAML/JSONL, PhysicalAdapter e `embodimentv1` não acharam implementação omitida |
| Reexecutar suites / verificar CI de A0 no SHA novo | **NOT_RUN:** documento apenas, sem checkout executável e sem job A0 ainda |
| Revisão por pessoa/agente independente | **PENDING:** esta segunda passagem é revisão adversarial de completude pelo mesmo executor, não review independente |

**Veredito A0:** **inventário técnico e revisão de completude concluídos para este SHA**, sem lacuna de superfície relevante identificada após segunda passagem. Isso não significa ausência absoluta de bugs ou aprovação do protocolo. A entrega aguarda revisão humana/independente da Draft PR e merge; #94 deve permanecer aberta e A1 não recebe autorização implícita.

## Referências primárias e pontos de entrada

- Código físico: [`src/embodiment/`](../../src/embodiment/), [`types.py`](../../src/embodiment/types.py), [`cli.py`](../../src/embodiment/cli.py), [`eventlog.py`](../../src/embodiment/eventlog.py), [`snapshot.py`](../../src/embodiment/snapshot.py), [`rng.py`](../../src/embodiment/rng.py).
- Modelos: [`data/models/physical/`](../../data/models/physical/), [sourcing](../../data/models/physical/SOURCING.md), [dimensões](../../data/canon/schema/enums/capacity-dimensions.yaml).
- Core: [`internal/csf/domain`](../../internal/csf/domain), [ports](../../internal/csf/ports), [reducer](../../internal/csf/reducer), [codec](../../internal/csf/codec), [identity](../../internal/csf/identity), [genesis](../../internal/csf/genesis), [policy](../../internal/csf/policy).
- Normas: [ADR 0006](../adr/0006-physical-domain-model.md), [0007](../adr/0007-python-para-o-subdominio-embodiment.md), [0008](../adr/0008-causal-simulation-foundation-v1.md), [0009](../adr/0009-canonical-causal-codec-and-digests.md), [0010](../adr/0010-csf-causal-engine-stack.md), [0011](../adr/0011-csf-causal-persistence-atomicity.md), [spec física](../spec/physical-simulation-v1.md), [spec CSF §14.3](../spec/causal-simulation-foundation-v1.md), [ticket 9A](../tickets/csf-runtime-v1/09a-contrato-adapter-fisico.md), [ticket 10](../tickets/csf-runtime-v1/10-adapter-fisico-e-cenario.md), [tickets PSV1](../tickets/physical-simulation-v1/README.md).
- Testes: [`tests/embodiment/`](../../tests/embodiment/), [`tests/architecture/`](../../tests/architecture/), [workflow de produto](../../.github/workflows/product-validation-on-demand.yml).
