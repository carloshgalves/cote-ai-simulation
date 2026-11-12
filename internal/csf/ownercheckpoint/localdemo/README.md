# B2 — Host cognitivo local sintético

Este pacote implementa a **superfície de integração demonstrável** entre owner privado (`ownercheckpoint.DurableOwner` da B1), autoridade causal CSF (`sqlite.Store`), `ownercheckpoint.Gate` e um callback cognitivo **sintético**. Não há LLM, driver de modelo, runtime de produto, API web, tenant humano ou memória real de personagens.

## Fluxo

1. O operador inicializa Genesis/CSF, registra o ator e a chave pública Ed25519; mantém a chave privada somente no owner. Owner e CSF são **dois arquivos SQLite distintos**. O host revalida essa fronteira em `StageSnapshot`, `Restore` e `Run`: se a CSF tiver sido aberta posteriormente sobre o banco do owner, os três caminhos falham fechados.
2. `localdemo.New(owner, causal, bundle, scope, worker)` fixa o `run/namespace/actor` e recebe um callback de teste. O campo callback é privado; o host não exporta método que o invoca sem o gate.
3. `Host.StageSnapshot(ctx, cp)` copia os bytes privados, executa `Gate.Stage` (commit owner → assinatura → escrow CSF), **depois** `CreateSnapshotV1` com a mesma cópia e retorna `Reference{Snapshot,Receipt}`. Resultado parcial/erro **não** concede permissão de WORK.
4. Encerre os dois stores, reabra-os e construa um **novo** Host. `Host.Run` falha fechado antes de `Host.Restore(ctx, ref)`; o Restore valida o snapshot selado no tip e o recibo CSF, recupera os bytes exatos do owner e exige ACK persistente. Somente então `Host.Run(ctx,ref)` invoca o callback via `Gate.RunWork`.
5. `Reference` não é credencial ou permit; troca de snapshot, receipt, ator ou run falha. Acesso direto ao callback **fora** deste host não é monitorado: um host real futuro deverá garantir que nenhum outro caminho de despacho exista.

**B2 comprova reabertura de ambos os bancos e reconstrução dos objetos de sessão no processo de testes.** O experimento de encerramento/reinício de **processos distintos**, fault injection, concorrência adversarial e fork de filho pertence à B3 (#78). Não se pretende recuperação de perda física do host, backup remoto, SaaS, isolamento multi-tenant humano, `exactly-once` de modelo nem garantia de energia/FS.

## Evidência

`go test -count=1 ./internal/csf/commit -run '^TestCSFRV18R58B2'` testa com CSF SQLite real, owner SQLite real, Genesis registrado, Ed25519, stage/snapshot/restore/ACK, callback contado, abertura nova e controles negativos. `go test -race -count=1 -timeout=10m ./internal/csf/commit -run '^TestCSFRV18R58B2'` exercita o mesmo caminho com instrumentação de corrida. Revisões da PR #81 e os workflows do HEAD determinam o estado real dos gates.

`DispatchFencedWorkV1` e `ResumeRunV1` continuam processos causais **determinísticos** e não são envelopados neste gate. Não transformar memória do owner em `WorldTruth` ou evidência global.
