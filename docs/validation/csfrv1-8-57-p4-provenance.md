# CSFRV1-8 / #57 P4 — Prova de origem e pertencimento das dez partições

**Base:** merge P3, `main@6222d4b336f727cfffddf82714a5b76d40044e22`.

## Escopo e modelo de prova

`snapshotPending` constrói os mesmos dez rosters canônicos de antes, no mesmo `*sql.Tx`. Simultaneamente, cada linha de autoridade consumida pelos três leitores (scheduled/trigger, `input_ledger` e `causal_outbox`) registra um **testemunho efêmero**: owner, RunID validado no ramo de leitura ou por SQL escopado, ID exato, digest de referência e, para trigger-runtime/completion, bytes canônicos originais.

`snapshotPendingProvenance.verify` é um limite independente entre esses testemunhos e os valores que realmente irão para o snapshot. Rejeita referência sem fonte, associação a owner/kind não registrado, run divergente, ID ou digest trocado/ausente, canonical de runtime/completion diferente, duplicata na mesma partição e falta de vínculo entre os rosters 0/2/4 e os input-ref kinds 2/3/4 do roster 7. Também exige as relações **response→dispatch** e **completion→task** usando IDs extraídos do registro fonte, com prova da mesma run, proíbe links de respostas/completions não emitidas e rejeita uma autoridade simultaneamente pendente e liquidada ou liquidada duas vezes. Input-ref kind 0 (slot-response), quando aplicável, exige testemunho próprio: a produção atual não o emite espontaneamente. IDs coincidentes entre partições distintas **não são proibidos** quando os papéis e os vínculos são diferentes.

A prova **não** transforma uma projeção em autoridade externa: os testemunhos são criados apenas a partir das linhas já escopadas e autenticadas no produtor, não saem da memória, e não substituem `ReplayClock`, `verifySnapshotSeal`, os cinco ledgers autenticados nem `validateForkLineageTx`. Não há nova tabela, SQL, migração, ponto de commit, autorização de runtime ou arquivo histórico.

## Partições e testemunhos

| Partição | Owner de proveniência | Confirmação de fonte |
| --- | --- | --- |
| 0 scheduled-occurrences | occurrences | linha da run, ID e último digest do nascimento canônico, kind causal 6 |
| 1 trigger-runtime | trigger-runtime | definition_id, bytes canônicos completos do runtime, versão/forma conferidas pelo decoder P1 |
| 2 trigger-activations | trigger-activations | linha da run, ID e último digest de ativação, kind causal 7 |
| 3 source-closures | source-closures | input ledger, RunID, ID e digest `source-closure`; último seq por source |
| 4 rounds | rounds | input ledger, RunID, ID e digest `round-declaration`, only pendingClock |
| 5 dispatches | dispatches | input ledger, RunID, ID e digest `slot-dispatch`, ainda não respondido ou revogado |
| 6 responses | responses | input ledger, RunID, ID e digest unitário, kind 5/23; vínculo ao dispatch histórico, sem dupla resposta nem dispatch ainda pendente |
| 7 pending-units | exogenous/slot-response ou referência ao roster de origem | kinds 1/0 com testemunho próprio; 2↔0, 3↔2 e 4↔4 com ID/digest idênticos e cardinalidade exata |
| 8 perception-tasks | perception-tasks | causal outbox, RunID, ID e digest `perception-task` |
| 9 perception-completions | perception-completions | causal outbox, task de origem vinculada e bytes canônicos da completion; não pode constar entre tasks ainda pendentes ou ser concluída duas vezes |

**Limite importante:** a prova confirma vínculo à fonte lida e validação existente. Não recalcula todas as preimagens de ID, relógio e digest fora do replay autoritativo; não afirma ser um verificador alternativo de consenso ou um histórico reconstruído. No caso de snapshots antigos, o replay/verification existente continua usando os prefixos selados, e não o estado corrente.

## Gates de teste

- 10/10 rosters em fixture positiva P1→P4: saída normativa idêntica, inputs relacionados e witness binding em cada owner; fixtures distinguem dispatches/tasks pendentes das já liquidadas.
- Matriz negativa de **ausência de testemunho e origem cruzada em cada uma das dez partições**, com restauração positiva.
- Matriz negativa de corrupção de digest/bytes canônicos em cada partição, source input órfão e ausência do par round/input.
- Reject duplicate witness, owner/kind inesperado, run estrangeira, ID/digest ausente/incorreto, link response→dispatch ou completion→task órfão ou alterado, estados pendente/concluído simultâneos e liquidação duplicada.
- Golden binário pré-P1 e regressão R19–R26; `go test ./...`, `-race` e os cinco GitHub Actions no SHA definitivo.
- Revisão adversarial e remediações registradas; PR separada, Draft, sem merge automático.

## Fora de escopo

Mudar wire format, IDs, policy V2, read-side de snapshots antigos, producer P3 de ordenação, migrador P2 de rounds, publicação transacional da #56, retries, desempenho sistemático #60 e novos workflows #59. Esta PR entrega a matriz de proveniência necessária à #57, mas a issue só deve fechar após todos os gates e aprovação humana.
