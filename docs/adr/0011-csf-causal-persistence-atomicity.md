# ADR 0011 — Persistência e atomicidade da Fundação causal V1

**Status:** Accepted

**Data:** 2026-10-08

**Relacionado:** ADR 0003 (snapshots e tempo lógico), ADR 0008 (commit causal e ledgers) e ADR 0009
(bytes canônicos e replay)

**Pesquisa:** [stack e persistência da CSF V1](../research/csf-runtime-and-persistence-v1.md)

## Contexto

O ADR 0008 exige uma fronteira atômica entre settlement, `DecisionLedger`, `EventStore`,
`CausalOutbox` e `WorldRevision`. Um crash não pode deixar evento sem journal, tarefa sem commit,
revisão sem eventos ou decisão terminal sem o mundo correspondente. O sistema também precisa de
idempotência, retry, replay bit a bit, snapshot/resume e execução barata no notebook.

A topologia não foi escolhida. “Eventual consistency” não é uma interpretação aceitável do requisito:
uma reconciliação posterior não pode permitir que um store parcial seja observado como verdade causal.

## Decisão proposta

Usar **SQLite 3.53.4** (referência pesquisada em 2026-10-08; release de 2026-07-24) em **um único
arquivo principal por run/workspace**, com:

- `journal_mode=WAL`;
- `synchronous=FULL` no caminho autoritativo;
- transação local única para o commit causal;
- BLOBs contendo os bytes canônicos, com ids/digests e índices derivados;
- tabelas separadas por responsabilidade, mas não arquivos/bancos separados;
- snapshot e checkpoint epistemológico opaco na mesma unidade de persistência na V1.

O adapter Go V1 fixa `modernc.org/sqlite` **v1.60.1**, CGo-free, cujo pacote publicado para os alvos
suportados carrega SQLite 3.53.4. A identidade esperada do core SQLite é
`SQLITE_SOURCE_ID=2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`,
com SHA3-256 de `sqlite3.c`
`67f423e9ebbbdc473cbc4772c872ee6b89f31fde4ed0279a5c25d5f65c043a16`. O primeiro ticket de
implementação deve piná-los no `go.mod`/`go.sum` e falhar fechado se `sqlite_version()` ou
`sqlite_source_id()` divergir.

O arquivo inclui as autoridades lógicas `InputLedger`, `DecisionLedger`, `EventStore`,
`EvidenceLedger`, `CausalOutbox`, `WorldRevision`, `CycleControlState`, genesis e `SnapshotStore`.
Views, índices, JSON de diagnóstico, métricas e caches podem ser reconstruídos e não são autoridade.

## Fronteira transacional

O `CommitCoordinator` calcula e valida todo o lote fora da transação. A transação curta então:

1. adquire a escrita e verifica fence, revisão base, cursor e chaves idempotentes;
2. insere settlement e records terminais no `DecisionLedger`;
3. insere `Event` imutável e atualiza a revisão verificável pelos reducers pinados;
4. insere lifecycle/trigger derivados e `PerceptionTask` na `CausalOutbox`;
5. insere `CycleCommit` e `WorldRevision` — ou, no caminho de falha explícita, um
   `CycleAbortRecord` válido sem settlement parcial;
6. faz commit WAL com `synchronous=FULL`.

O commit lógico só é confirmado ao caller após o `COMMIT` bem-sucedido. Workers de percepção só
consomem tasks depois dessa fronteira. Retry reutiliza fence/digests e exige que uma repetição produza
bytes idênticos; uma identity key com bytes diferentes falha fechado.

Para snapshots V1, o checkpoint causal e o payload/ref epistemológico opaco são persistidos ou
verificados na mesma unidade SQLite. Um artefato externo só será permitido por ADR futuro que defina
durable-before-ref, recovery e failure injection; um locator sozinho não fecha atomicidade.

## Concorrência e configuração

SQLite permite leitores concorrentes e um writer por banco. Isso é suficiente para a V1 porque o
engine já possui um `CommitCoordinator` único para a autoridade de mundo; cálculo de candidatos,
validators, reducers puros e queries podem ocorrer em paralelo. A espera de lock deve gerar retry
operacional sem atribuir prioridade causal ao primeiro worker que chegou.

O adapter não usa `sql.Open` com um DSN solto. Ele cria um `sqlite.NewConnector` e chama `sql.OpenDB`;
o writer autoritativo usa uma conexão dedicada (`SetMaxOpenConns(1)`) e não compartilha o pool com
leitores. Toda conexão que o connector abrir recebe, antes de ser entregue ao caller, uma
connection hook que executa e verifica `PRAGMA journal_mode=WAL` e `PRAGMA synchronous=FULL`; o
resultado efetivo deve ser `WAL` e `2` (`FULL`). Falha em qualquer verificação impede a conexão e o
commit autoritativo. A configuração é repetida/verificada quando uma conexão é criada, não apenas na
conexão de bootstrap.

O teste de configuração também coleta `PRAGMA compile_options`, ordena o resultado e o compara com
um manifesto pinado por `GOOS/GOARCH`. O manifesto registra, no mínimo, `THREADSAFE`,
`DEFAULT_SYNCHRONOUS`, `DEFAULT_WAL_SYNCHRONOUS` e os diagnósticos de compile options; nenhuma
diferença nesses itens, nem substituição de source/vendor do driver, é aceita sem atualizar a policy
e os hashes. Os defaults de compilação não substituem as PRAGMAs explícitas verificadas acima.

Não usar `synchronous=NORMAL`, `OFF`, tabelas `UNLOGGED`, `ATTACH` para dividir os quatro artefatos
autoritativos, nem copiar apenas o arquivo `.db` enquanto um WAL necessário está separado. Backups
usam a Online Backup API ou procedimento equivalente que inclua o estado WAL e são validados por
replay/hash.

## Alternativas consideradas

### PostgreSQL com transação local

Também satisfaz a atomicidade e oferece maior concorrência, isolamento, WAL/PITR e ferramentas de
backup. Foi reservado como adapter compatível para quando houver múltiplos processos/hosts ou
contenção medida. Na V1, exige servidor, roles, lifecycle e migrations operacionais sem benefício
necessário para provar os invariantes no notebook.

### Stores separados com journal/commit protocol

Pode ser correto somente se o journal for a autoridade durável: cada store precisa de prepare/commit,
fence, recovery idempotente, retenção e uma regra que esconda qualquer preparo parcial. Sem isso,
reconciliação posterior é eventual consistency e viola o ADR 0008. A complexidade é maior do que a
transação local e não há requisito V1 que a justifique.

### SQLite em múltiplos arquivos

Não escolhido. Mesmo que SQLite tenha mecanismos de atomicidade entre arquivos em alguns modos, WAL não
deve ser tratado como transação atômica do conjunto de bancos. Todos os artefatos autoritativos ficam
no mesmo arquivo para que o commit dependa de uma única autoridade transacional.

## Consequências

Positivas:

- transação curta e explícita cobre a unidade causal completa;
- crash recovery, backup e execução local têm baixa complexidade operacional;
- BLOB canônico preserva os preimages exigidos para strict replay;
- PostgreSQL pode ser adicionado depois atrás dos mesmos ports.

Custos e limites:

- apenas um writer por arquivo; commits precisam ser curtos e o coordinator único;
- WAL, `synchronous=FULL`, filesystem e backup precisam de testes de failure injection;
- migrações SQL e constraints não substituem validators/reducers do domínio;
- checkpoint externo e replicação ficam deliberadamente fora da V1.

## Critérios de aceitação

1. Cada commit causal publica ou não publica atomicamente settlement, decision, event, outbox,
   cycle journal e revision.
2. Crash antes/depois de cada append lógico restaura prefixo completo, commit completo ou abort
   terminal — nunca combinação parcial.
3. Reexecução após crash é idempotente por fence, identity key, id e digest; bytes divergentes falham.
4. Replay contínuo e snapshot+resume produzem os mesmos bytes canônicos, digests, revisões, cursores
   e state hash.
5. Dois workers que tentam o mesmo commit não duplicam event/task/knowledge input.
6. Backup/restauração preserva o arquivo/WAL necessário e passa verificação de genesis, policy,
   schema bundle e todos os records.
7. Teste de configuração rejeita modo de journal/synchronous incompatível com a decisão.
8. Um benchmark mostra que a contenção do writer é compatível com o primeiro ano; se não for,
   PostgreSQL é avaliado sem alterar os ports ou a semântica causal.

## Fora de escopo

Este ADR não implementa schema SQL, migrations, backup tooling ou runtime; não decide replicação,
multi-host, cloud storage, sharding, event streaming, retenção de longo prazo, UI, memória/RAG,
`Embodiment` ou o adapter físico. Também não autoriza eventual consistency como substituto da
atomicidade.
