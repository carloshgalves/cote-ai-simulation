# CSFRV1-8 / issue #57 — P2: fases do histórico de rounds

**Base fixada:** `main@be5674760757d997d91946d43f8aee00e4168ec3` após integração de #57/P1 (PR #68).

Esta entrega separa o algoritmo de `migrateRoundHistoryV2` em funções internas sobre dados históricos já autenticados, sem operações SQL, trabalho de agentes, consultas a modelos ou alterações no codec/policy. A primeira fase recebe as dez partições do snapshot após a validação estrutural da P1.

| Fase | Função | Invariantes |
| --- | --- | --- |
| Projeção de round/eligibilidade | `projectForkRoundHistoryV2` | Rounds migrados por `migratePendingRoundRecords` e snapshot exato; corrige ordinal relativo somente ao corte |
| Autenticação de dispatch | `authenticateForkSlotDispatchesV2` | ID pela preimagem registrada V1, RunID parental, actor, slot e relógio causal no journal histórico |
| Associação de receipts | `attachForkSlotOutcomesV2` | Revogação com hash do registro; uma resposta por dispatch; registro não-array rejeitado em vez de panic |
| Closure e ordenação | `validateAndOrderForkSlotHistoriesV2` | Pendentes abertos correspondem ao roster da fonte; revogado e respondido são mutuamente excludentes; chave (round ID, slot, ordinal) ordena e cadeia de revogações não possui lacunas |
| Rekey de autoridade infantil | `remintForkSlotHistoriesV2` | Novo ciclo e IDs de dispatch/resposta, revisão zero, slot e intenção preservados; validações são anteriores à derivação |

`forkRoundHistoryProjection` é uma estrutura efêmera de planejamento, **não uma autoridade**. A reconstrução mantém os campos `[]any` do codec nos locais onde a autenticação já existe, e converte apenas as fronteiras de responsabilidade em um estado tipado. A issue não encerra sem provas de pertencimento cruzado e análise posterior do produtor `snapshotPending`.

## Evidência de equivalência e negativos

- Captura binária pré-refactor: `internal/csf/commit/testdata/csfrv18-fork-p1-golden.gz.b64`, assert `TestCSFRV18ForkP1GoldenBytes` (não recodifica bytes SQL esperados).
- Comparação literal de blocos: `authenticateForkSlotDispatchesV2`, `attachForkSlotOutcomesV2`, `validateAndOrderForkSlotHistoriesV2` e `remintForkSlotHistoriesV2` são extratos do código anterior (somente assinaturas e return-arity mudaram), exceto alterações intencionais: fail-closed em recibos CBOR não-array e verificação da cadeia de ordinais antecipada para a fase de validação.
- Negativos novos em `fork_round_history_phases_v2_test.go`: double revocation, corrupt receipt ID, CBOR não-array, revogado + respondido, resposta ID incompatível, ordenação não dependente de map Go, lacuna ordinal, elegibilidade fora do corte, rekey e source run divergente.
- Suítes existentes R19: round+dispatch+responses, NoProposal e proposal/intent, reabertura, slot preenchido, revocation/re-dispatch, replay, snapshot antigo com escrita posterior, rollback, memória/native roots, relógio histórico, e contaminação cruzada. A regra de 5/5 CI no **SHA final** inclui golden, cross-policy, Go full e `-race -timeout=30m`.

**Escopo proibido:** alteração de schema, policy V2, código de publicação transacional, ID causal canônico, autorização de resposta nova, leitor de world truth, redesenho de intent e otimizações especulativas. Risco residual: a mudança não conclui a tipagem profunda de `snapshotPending` e dos campos de dispatch; uma próxima fatia deve rastrear proveniência de todas as partições de snapshot.

**Status:** a PR só poderá ser marcada pronta depois de CI e re-review sem achado acionável; este documento é mapa de gates, não declaração de PASS.
