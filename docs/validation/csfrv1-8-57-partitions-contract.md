# CSFRV1-8 / #57 P1 — contrato de decodificação das partições de snapshot

Este é o **primeiro checkpoint** da issue #57: tipos e decoders internos com testes table-driven. **Não altera** a serialização CBOR, schemas V1/V2, o produtor `snapshotPending`, os migradores de round nem a transação de publicação do fork. O adaptador de leitura `forkPendingParts` utiliza a projeção tipada somente como validação estrutural, devolvendo a mesma representação original aos consumidores legados.

## Inventário normativo das dez partições (ordem indexada pelo CDDL V1/V2)

| Índice | Nome e limite | Formato | Tipagem projetada | Ordem / pertencimento |
| --- | --- | --- | --- | --- |
| 0 | scheduled-occurrences, 65535 | causal-ref kind 6 | ID, digest [32]byte | Fonte `scheduled_occurrences`, somente lifecycle ativo; ordem preservada |
| 1 | trigger-runtime, 65535 | 8 campos, definition ID e version | DefinitionID, Version | Fonte `trigger_runtime`, ordenada por definition_id |
| 2 | trigger-activations, 65535 | causal-ref kind 7 | ID, digest | Fonte `trigger_activations`; ordem preservada |
| 3 | source-closures, 1024 | causal-ref kind 2 | ID, digest | Último cursor por source ordenado por source name |
| 4 | rounds, 65535 | causal-ref kind 3 | ID, digest | RoundDeclaration pendente, fonte/input-ledger |
| 5 | dispatches, 65535 | causal-ref kind 4 | ID, digest | Dispatch ainda sem response/revocation, ordenado por ID |
| 6 | responses, 65535 | causal-ref kinds 5 / 23 | ID, digest, kind | Proposal e NoProposal históricos; ordem do input ledger |
| 7 | pending-units, 65535 | input-ref kind 0 (quatro campos) ou 1..4 (três) | ID, digest, slot-kind opcional | União de inputs elegíveis; manter ordem composta, **não ordenar** |
| 8 | perception-tasks, 65535 | causal-ref kind 15 | ID, digest | Task sem completion, ordenada por ID |
| 9 | perception-completions, 65535 | completion com 5 campos | completion ID, task ID, digest | Proof histórico, ordem do causal_outbox |

**Pertencimento:** este decoder não é um autenticador de fonte. Ele verifica tag, cardinalidade, ID/digest de 32 bytes, tipos conhecidos, versão, envelope e duplicação **dentro** de cada partição. A comparação de ID, digest e run **com registros autenticados do parent e seu snapshot** permanece obrigatória nos migradores/validadores (R19–R26). IDs podem se repetir legitimamente **entre** partições (ex.: round em #4 e como pending input em #7). Ordenar ou rejeitar globalmente seria corrupção causal. Tipos extraídos não conferem autoridade a estado externo.

A primeira PR cobre decodificação e negativos de presença/ausência, duplicata, formato, tag e corrupção em todas as dez rosters. A próxima fatia isolará a derivação de rounds/dispatch/revocation/responses em funções puras, com testes de origem cruzada e rekey contra arquivos autenticados. O golden `internal/csf/commit/testdata/csfrv18-fork-p1-golden.gz.b64` e os workflows CSFRV1-5/6/7/8 são os gates de equivalência de bytes desta fase.
