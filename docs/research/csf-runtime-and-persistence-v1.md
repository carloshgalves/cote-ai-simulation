# Pesquisa — stack e persistência da Causal Simulation Foundation V1

**Data da pesquisa:** 2026-10-08

**Perguntas de decisão:**

1. Qual linguagem/runtime deve implementar o Simulation Engine causal, preservando o contrato
   multilíngue do codec V1 e a fronteira de dados com `Embodiment`?
2. Qual topologia de persistência garante atomicidade, crash recovery, replay e snapshot/resume no
   notebook do desenvolvedor?

**Decisões informadas:** [ADR 0010](../adr/0010-csf-causal-engine-stack.md) e
[ADR 0011](../adr/0011-csf-causal-persistence-atomicity.md).

**Estado:** pesquisa e ADRs propostos; a aceitação dos ADRs continua sendo o gate antes de código de
produção.

## 1. Resumo executivo

Recomenda-se:

- **D1:** Go 1.25.x para o Simulation Engine causal, com o codec e os schemas do bundle V1 atrás de
  um adapter próprio. A referência de pesquisa é Go 1.25.14 (2026-08-19), com `fxamacker/cbor/v2`
  2.9.2 (2026-05-03) como implementação CBOR avaliada. O domínio não depende diretamente da API da
  biblioteca.
- **D2:** um único banco SQLite 3.53.4 (2026-07-24) por run/local workspace, em WAL com
  `PRAGMA synchronous=FULL`, contendo no mesmo arquivo as tabelas dos ledgers, outbox, revisão e
  snapshots. O commit causal é uma transação única; a outbox só é consumida depois do commit.
- **Evolução:** PostgreSQL pode receber os mesmos ports e records quando houver necessidade real de
  concorrência multi-processo/host. Stores separados só serão admissíveis com um protocolo de commit
  durável que não exponha nenhum store como autoridade antes de todos os participantes confirmarem;
  isso não é a topologia V1.

A escolha não altera a semântica dos ADRs 0008/0009. O codec, os registries, os digests, a ordem
causal, a visibilidade e os reducers continuam sendo definidos pelo contrato existente, não pela
linguagem ou pelo banco.

## 2. Restrições e critérios

O material interno consultado fixa que:

- o engine é a autoridade única de relógio, `WorldState`, eventos, causalidade, revisão e snapshots;
- `Embodiment` é um subdomínio Python, e a fronteira entre ele e o engine é de dados/processo;
- ids, digests, state hashes e replay exigem `canonical_bytes_v1`, strict decode e o bundle imutável;
- workers podem terminar em qualquer ordem, mas a admissão, ordenação e publicação são causais e
  determinísticas;
- settlement, `Event`, `PerceptionTask`, `CycleCommit` e `WorldRevision` precisam de uma única
  fronteira atômica;
- crashes devem produzir o prefixo anterior completo, o commit completo ou um abort terminal
  explícito — nunca uma mistura publicável;
- nenhum contexto de agente ou observação pode ser derivado de um estado parcial ou de uma consulta
  ao event store global.

As notas abaixo distinguem **fato verificado** nas fontes de **inferência** para este repositório.
Pontuações e recomendações são **decisões de projeto**, não propriedades garantidas pelos runtimes.

## 3. D1 — linguagem e runtime

### 3.1 Evidência verificada e versões registradas

| Alternativa | Versão/data observada | Fatos relevantes | Limitação relevante |
|---|---|---|---|
| Python | documentação Python 3.14.8, consultada em 2026-10-08 | `asyncio` oferece concorrência cooperativa, I/O, IPC, filas e subprocessos; `hashlib.sha256` é garantido na biblioteca padrão | `cbor2` 6.1.4 oferece `canonical=True`, mas a opção genérica não substitui o profile V1: o projeto exige RFC 8949 core deterministic, profile Unicode/float/registry e strict decode próprios |
| TypeScript/Node.js | Node 22.23.3 LTS, publicado em 2026-09-23; TypeScript 7.0.2, publicado em 2026-08-20 | `worker_threads` é uma API estável para paralelismo CPU; `node:crypto` expõe SHA-256; TypeScript fornece checagem estática antes da execução | tipos TypeScript são apagados em runtime; `number`, `BigInt`, `Buffer`, objetos e iteração precisam de disciplina explícita para não contaminar bytes causais; codec CBOR e validação V1 continuam sendo trabalho de adapter |
| Go | Go 1.25.14, 2026-08-19 | goroutines/channels e `testing/synctest` suportam concorrência testável; `crypto/sha256` e `database/sql` são standard library; há suporte oficial/documentado a PostgreSQL e SQLite por drivers | CBOR não está na standard library; a biblioteca avaliada ainda precisa de wrapper para records, Unicode 15.1, registries, profile e strict re-encoding |

Fontes primárias da tabela: [Python asyncio](https://docs.python.org/3.14/library/asyncio.html),
[Python hashlib](https://docs.python.org/3.14/library/hashlib.html),
[cbor2 API](https://cbor2.readthedocs.io/en/stable/api.html),
[Node.js release schedule](https://nodejs.org/en/about/previous-releases),
[Node worker_threads](https://nodejs.org/download/release/latest/docs/api/worker_threads.html),
[Node crypto](https://nodejs.org/download/release/v26.8.1/docs/api/crypto.html),
[TypeScript 7.0 release](https://github.com/microsoft/TypeScript/releases),
[Go 1.25 release notes](https://go.dev/doc/go1.25),
[Go release history](https://go.dev/doc/devel/release),
[Go database/sql guidance](https://go.dev/doc/database/) e
[fxamacker/cbor 2.9.2](https://github.com/fxamacker/cbor/releases/tag/v2.9.2).

O [RFC 8949 §4.2.1](https://www.rfc-editor.org/rfc/rfc8949.html) é a autoridade externa para o
core deterministic encoding. A implementação Go avaliada declara `CoreDetEncOptions()` e suporte a
RFC 8949, preferred serialization, ordem bytewise de map e modos de decoder configuráveis. Isso é
um bom componente, não prova de conformidade do CSF: tags, tipos, ranges, bindings e relações de
records ainda precisam ser validados contra o bundle.

### 3.2 Matriz de critérios vinculados aos invariantes

Escala: 1 = risco/custo alto ou suporte fraco; 5 = melhor ajuste. Os pesos são específicos desta
V1, não um ranking geral de linguagens.

| Critério ligado ao CSF | Peso | Python | TypeScript/Node | Go |
|---|---:|---:|---:|---:|
| Codec V1, CBOR estrito e SHA-256 | 20 | 4 | 4 | 5 |
| Ordenação, RNG, reducers e replay | 20 | 3 | 3 | 5 |
| Concorrência e simultaneidade lógica | 15 | 3 | 4 | 5 |
| Isolamento de conhecimento e autoridade | 10 | 4 | 4 | 4 |
| Fronteira de dados/processo com `Embodiment` | 10 | 5 | 4 | 4 |
| Testes, auditoria e manutenção | 15 | 4 | 4 | 5 |
| Portabilidade, dependências e custo local | 10 | 4 | 4 | 5 |
| **Total ponderado** | **100** | **3,75** | **3,80** | **4,80** |

As notas são inferências de projeto. O ponto não é que Go forneça determinismo automaticamente: o
mesmo programa pode ser não determinístico se iterar maps, usar RNG global ou permitir que completion
order escolha uma decisão. Go recebe a nota maior porque oferece uma superfície pequena para tornar
esses erros explícitos, boa concorrência CPU/IPC, binário portátil e testes de concorrência, sem
importar o cliente Python do `Embodiment`.

#### Python

É uma alternativa tecnicamente válida e tem o menor custo de fronteira conceitual com o componente
físico. `asyncio`, `multiprocessing`/subprocessos, `hashlib` e o ecossistema de testes permitem
implementar a fundação. Entretanto, o ADR 0007 não autoriza generalizar Python; escolher Python
apenas porque o repositório já contém `Embodiment` criaria a decisão por acidente.

O risco principal é organizacional e de contrato, não impossibilidade técnica: objetos mutáveis,
ordenação implícita e convenções de bibliotecas tornam mais fácil vazar representação de runtime para
o preimage. `cbor2` também não deve ser usado como “codec pronto” sem uma camada que rejeite todas as
formas não permitidas pela V1. Se Python fosse escolhido, ainda seria necessário implementar o
mesmo core puro, com records imutáveis, bytes explícitos e dois runners independentes.

#### TypeScript/Node.js

Node combina bem com IPC e com uma futura superfície de ferramentas, e workers são suficientes para
calcular candidatos em paralelo. A checagem estática do TypeScript ajuda a modelar ports. A ressalva
é que tipos desaparecem na execução; os tipos do compilador não validam CBOR, bounds, Unicode,
identidade ou proveniência. `number` também não pode representar silenciosamente todos os inteiros
causais do bundle. O código precisaria usar `BigInt`/bytes/classes fechadas e rejeitar coerções.

É uma segunda implementação de conformidade plausível, especialmente útil como runner independente,
mas é menos atraente como primeira autoridade por combinar runtime dinâmico com um domínio em que
“parece o mesmo objeto” não significa “os mesmos bytes”.

#### Go

Go oferece goroutines para validators/handlers e um coordinator serial para a fronteira autoritativa;
`testing/synctest` ajuda a testar relógio e workers sem usar wall clock. `fxamacker/cbor` fornece um
ponto de partida alinhado ao RFC 8949, e SHA-256 está na biblioteca padrão. Um binário único também
reduz variação de ambiente no notebook e no CI.

O custo é explícito: será preciso escrever o profile CSF acima da biblioteca CBOR, incluindo os
registries e os 620 vetores. O coletor de lixo e goroutines não são uma garantia de determinismo; o
commit coordinator, a política de substreams e o uso exclusivo de valores inteiros/fixed-point em
regras exatas são obrigatórios.

### 3.3 Decisão recomendada

Escolher **Go 1.25.x** para o core do Simulation Engine causal, com o patch do toolchain e as
dependências travados no primeiro ticket de implementação. O domínio importa apenas interfaces e
tipos do CSF; codec, SQLite e processo Python ficam em adapters.

O núcleo deve ter estas fronteiras:

```text
domain/core ──> ports
     │             ├── codec/canonical-bytes
     │             ├── persistence/sqlite
     │             ├── embodiment/process
     │             └── observatory/read-only
     │
     ├── Clock / CycleCoordinator
     ├── AdmissionFence / InputLedger
     ├── validators / handlers / ConflictResolver
     ├── CommitCoordinator / ReducerRegistry
     ├── PerceptionResolver / KnowledgeInputSink
     └── Snapshot / eval harness
```

Workers só calculam resultados associados a uma revisão/fence imutável. O coordinator valida a
partição, ordena por `EventOrderKey`, deriva ids/digests e chama uma transação. Nenhum worker recebe
store mutável, committer genérico, estado completo do mundo ou conhecimento de outro ator.

`Embodiment` continua Python. A integração inicial será subprocesso local com framing de dados
versionado; o payload causal que atravessa a fronteira é o envelope canônico ou uma view de dados
explicitamente validada. O processo físico não pode publicar revisão, escrever ledgers nem decidir
percepção. Nenhum cliente de LLM, framework de agentes, banco vetorial ou adapter de provedor entra no
core.

## 4. D2 — persistência e atomicidade

### 4.1 Fatos verificados

- SQLite documenta que uma transação é all-or-nothing, que há múltiplos leitores mas apenas um
  escritor por banco, e que WAL permite leitores durante a escrita. Veja
  [transactions](https://sqlite.org/lang_transaction.html),
  [isolation](https://sqlite.org/isolation.html) e
  [WAL](https://www.sqlite.org/wal.html).
- SQLite documenta que, em WAL, `synchronous=FULL` sincroniza o WAL em cada commit; `NORMAL` pode
  perder transações recentes após power loss. Para a exigência desta V1, `NORMAL` não é aceitável.
  Veja [PRAGMA synchronous](https://sqlite.org/pragma.html#pragma_synchronous).
- SQLite fornece uma [Online Backup API](https://sqlite.org/backup.html) que produz um snapshot
  consistente e bitwise idêntico ao estado observado no início do backup. O arquivo WAL deve ser
  tratado como parte do estado persistente até a cópia segura.
- PostgreSQL fornece transações all-or-nothing, isolamento serializável e retry explícito em
  serialization failure; mantém WAL para recuperação e oferece dumps consistentes. Veja
  [transactions](https://www.postgresql.org/docs/18/tutorial-transactions.html),
  [transaction isolation](https://www.postgresql.org/docs/current/transaction-iso.html),
  [WAL/PITR](https://www.postgresql.org/docs/current/continuous-archiving.html) e
  [pg_dump](https://www.postgresql.org/docs/current/app-pgdump.html).
- A atomicidade transacional de um banco não torna dois bancos independentes atômicos. Um journal
  determinístico pode tornar um commit reconciliável, mas exige estados duráveis de prepare/commit,
  recovery idempotente, fencing, retenção de journal e regras que impeçam qualquer projeção parcial de
  ser lida como autoridade. Isso é inferência de arquitetura aplicada ao requisito do ADR 0008.

### 4.2 Comparação

| Opção | Atomicidade/crash | Concorrência | Replay/snapshot | Migração/observabilidade | Notebook |
|---|---|---|---|---|---|
| SQLite, uma transação local | Forte para todos os artefatos no mesmo arquivo; WAL+FULL cobre crash/power loss dentro das hipóteses documentadas | um writer; leitores concorrentes; suficiente se o coordinator serializa commits | simples: BLOBs canônicos, cursores e snapshot no mesmo arquivo; backup oficial | SQL/backup simples; exige pin de SQLite e cuidado com WAL/FS | melhor custo e zero servidor |
| PostgreSQL, uma transação local | Forte, com WAL e isolamento/retries maduros | melhor para múltiplos writers/processos/hosts | excelente, mas adiciona servidor, roles, migrations e operação | ferramentas de backup/observabilidade fortes | custo operacional maior no notebook |
| Stores separados + journal/reconciliation | só satisfaz se o journal for a autoridade e nenhuma leitura aceite preparo parcial; sem isso viola ADR-0008 | potencialmente maior | cada store exige cursor, backup e reconciliação próprios | maior complexidade e superfície de falha | pior custo; nenhum benefício V1 demonstrado |

### 4.3 Matriz de decisão

| Critério ligado ao ADR 0008 | Peso | SQLite | PostgreSQL | Stores separados |
|---|---:|---:|---:|---:|
| Commit completo/inexistente após crash | 30 | 5 | 5 | 2 |
| Idempotência, retry e replay bit a bit | 15 | 5 | 5 | 4 |
| Concorrência e throughput de writers | 15 | 3 | 5 | 5 |
| Snapshot/resume e cursores causais | 10 | 5 | 5 | 4 |
| Migração, backup e observabilidade | 10 | 4 | 5 | 3 |
| Simplicidade/custo no notebook | 20 | 5 | 2 | 1 |
| **Total ponderado** | **100** | **4,60** | **4,40** | **2,85** |

As notas assumem implementação correta e não tratam “eventual consistency” como atomicidade.
SQLite perde em writers simultâneos, mas a V1 já exige um único `CommitCoordinator` para resolver
simultaneidade lógica. PostgreSQL ganha quando o problema real passar a ser múltiplos processos ou
hosts; isso não é requisito da primeira milestone.

### 4.4 Decisão recomendada e protocolo

Escolher **SQLite em um único arquivo principal por run/workspace**, não um arquivo por ledger. A
versão de pesquisa é SQLite 3.53.4. Os stores lógicos continuam separados por tabela/port:

- `InputLedger`;
- `DecisionLedger` e `CycleControlState`;
- `EventStore`;
- `EvidenceLedger`;
- `CausalOutbox`;
- `WorldRevision`/materializações verificáveis;
- `SnapshotStore`, genesis e checkpoints epistemológicos opacos.

Todos os records causais permanecem em BLOB canônico, acompanhados dos ids/digests e índices
operacionais. Índice, view, JSON de diagnóstico e telemetria nunca substituem o record nem decidem
replay.

O adapter de persistência deve:

1. abrir uma transação de escrita curta (`BEGIN IMMEDIATE` ou equivalente), depois que o trabalho puro
   já estiver calculado;
2. confirmar fence, revisão base, cursors e chaves idempotentes por CAS/constraints;
3. inserir settlement, `DecisionRecord`, `Event`, lifecycle/trigger, `PerceptionTask`,
   `CycleCommit`/abort e `WorldRevision` na mesma transação;
4. exigir bytes idênticos em reinsert idempotente e rejeitar digest/identity collision;
5. executar `COMMIT` com WAL + `synchronous=FULL`; um erro faz rollback e não deixa projeção parcial;
6. só entregar tarefas à outbox depois de observar o commit durável; o worker de percepção nunca
   participa da transação do mundo;
7. armazenar, na V1, o checkpoint epistemológico opaco ou sua referência verificável no mesmo banco.
   Objeto externo só poderá ser usado depois de uma decisão própria sobre durabilidade-before-ref e
   recovery.

Não usar `ATTACH` para dividir os artefatos autoritativos em bancos WAL separados: a documentação do
SQLite não oferece atomicidade do conjunto em WAL. Também não usar `synchronous=NORMAL`, `OFF`,
`UNLOGGED` ou `synchronous_commit=off` no caminho autoritativo.

Retries repetem a operação inteira a partir do fence/digest persistido. O banco fornece atomicidade;
o engine continua responsável por ids determinísticos, deduplicação, fences, partição de unidades,
ordem de eventos, state hash e validação de pre/post-state.

### 4.5 Por que não escolher PostgreSQL agora

PostgreSQL é uma evolução tecnicamente segura e deve ser mantido como adapter alternativo. Ele traz
melhor concorrência e operações de backup maduras, mas introduz servidor, lifecycle, autenticação,
roles, migrations e custos de execução/diagnóstico que não ajudam a provar os invariantes no notebook.
Migrar o adapter sem mudar os ports não reabre D1 nem altera os bytes causais.

Stores separados são ainda menos adequados à V1: o journal protocol seria uma segunda implementação
de atomicidade, com recovery e observabilidade próprias. Se um `EventStore` confirmasse e o
`CausalOutbox` não, o estado só seria correto se o journal impedisse a leitura do primeiro e pudesse
reconstruir/reverter o lote. Isso é um protocolo distribuído local, não uma economia de código.

## 5. Plano incremental de implementação

O plano segue a sequência da spec §13 e não implementa nenhum runtime nesta decisão:

1. Fixar toolchain Go, dependências, layout de módulos e processo Python de teste; não tocar em
   `Embodiment`.
2. Implementar o adapter de codec e executar os 620 vetores com um segundo runner independente;
   manter records causais como bytes.
3. Definir ports de `Clock`, ledgers, `ReducerRegistry`, `CommitCoordinator`, outbox, snapshots e
   Observatory read-only; adicionar testes de arquitetura para a direção das dependências.
4. Criar schema SQLite único, migrations versionadas e constraints de identidade/append-only;
   validar WAL + FULL em ambiente local.
5. Implementar fontes, closures, rounds, slots, fence e clock em memória; persistir inputs e
   fences numa transação curta.
6. Implementar candidatos, validators, conflitos, substreams e settlement puro; permutar ordem de
   workers e provar o mesmo digest.
7. Implementar commit terminal único de decision/event/outbox/revision e abort/retry explícitos.
8. Implementar percepção, evidência, comunicação, barrier, replay e snapshot/resume.
9. Adicionar failure injection antes/depois de cada append lógico, crash processual e power-loss
   simulation do banco; comparar prefixo, commit, hashes e cursores.
10. Integrar `Embodiment` apenas por adapter de dados/processo e executar evals de isolamento e
    determinismo; depois abrir regras de exame e cognition.

## 6. Critérios de aceitação

### Stack

- duas implementações independentes, em linguagens diferentes, passam os 620 casos do bundle;
- strict decode rejeita todos os negativos e o re-encoding é byte a byte idêntico;
- map/set/list, Unicode, floats permitidos, ids/digests e RNG não dependem de iteração, locale,
  wall clock ou completion order;
- goroutines podem terminar em permutações diferentes sem alterar fence, decisions, events,
  tasks, revisions ou state hash;
- o core não importa Python, cliente de LLM, framework de agentes ou banco vetorial;
- somente o adapter de `Embodiment` atravessa a fronteira de processo e não possui write port do
  engine.

### Persistência

- uma transação publica ou não publica o lote completo de settlement, decision, events, outbox,
  cycle journal e revision;
- failure injection em cada fronteira deixa apenas prefixo anterior, commit completo ou abort
  terminal válido;
- retry após crash é idempotente e exige bytes/digests iguais;
- replay e snapshot+resume reproduzem bytes, digests, revisões, cursores e state hash;
- backup/restauração inclui o estado WAL necessário e passa verificação de bundle, genesis e
  records;
- concorrência de leitores não altera resultados, e writers concorrentes são serializados sem
  escolher prioridade causal por chegada;
- uma migração/alteração de schema ausente, SQLite em modo inseguro ou policy/hash divergente falha
  fechado.

## 7. Riscos residuais e testes falsificadores

| Risco | Teste que pode falsificar a recomendação |
|---|---|
| biblioteca CBOR diverge do profile V1 | diferencial Go/runner independente em todos os positivos/negativos, mais fuzz de strict decode |
| FMA/float ou runtime muda resultado físico | cenários com floats permitidos sob diferentes arquiteturas/toolchains; regras causais migradas para integer/fixed-point quando exigirem igualdade |
| worker completion vira autoridade por acidente | property test que permuta goroutines, ordem de resposta, maps e callbacks |
| SQLite perde durabilidade no filesystem do notebook | matrix de crash/power-loss em filesystem suportado, `synchronous=FULL`, restore e validação de prefixo/commit |
| um índice/view divergente passa por autoridade | apagar/reconstruir índices e views a partir dos BLOBs; replay só a partir dos records canônicos |
| tamanho do WAL prejudica execução | benchmark com cenário de referência, checkpoint periódico e limite de journal; se falhar o orçamento, avaliar PostgreSQL sem mudar ports |
| checkpoint epistemológico externo quebra atomicidade | teste de crash entre blob/ref/commit; a V1 deve manter o payload/ref no mesmo arquivo até haver protocolo explícito |
| concorrência maior que um writer | benchmark de N workers; o gatilho para reavaliar SQLite é contenção medida, não preferência por servidor |

## 8. Fora de escopo

Esta pesquisa não escolhe nem implementa:

- framework de agentes, cliente/provedor de LLM, modelo, prompt ou roteamento;
- Canon Knowledge, RAG, Character Evidence, memória, belief formation ou contexto de agente;
- regras de exame, scoring, economia, relações ou política social;
- implementação do runtime causal, migrations executáveis ou schema SQL;
- alteração do `Embodiment`, do codec V1, do bundle imutável ou dos ADRs aceitos;
- replicação, alta disponibilidade, multi-host, cloud object store, sharding ou event streaming;
- adapter físico da decisão aberta 14.3.

## 9. Fontes primárias e limitações da pesquisa

Além das fontes ligadas acima, foram consultados [SQLite atomic commit](https://www.sqlite.org/atomiccommit.html),
[SQLite locking/concurrency](https://www.sqlite.org/lockingv3.html),
[SQLite appropriate uses](https://www.sqlite.org/whentouse.html),
[PostgreSQL reliability](https://www.postgresql.org/docs/17/wal-reliability.html),
[TypeScript Handbook — erased types](https://www.typescriptlang.org/docs/handbook/2/basic-types),
[RFC 8610](https://www.rfc-editor.org/rfc/rfc8610.html) e o
[bundle canônico local](../architecture/canonical-codec-v1-bundle/README.md).

Os links e versões foram registrados em 2026-10-08. Não foi executado benchmark comparativo nem
instalado framework; os números da matriz são julgamento arquitetural baseado na documentação, no
contrato do repositório e no custo de implementação esperado. O primeiro tracer bullet deve substituir
essas inferências por medições reproduzíveis sem reabrir os invariantes.
