# Issue #57 / P3 — Produtor tipado de pending-state

**Base:** merge da PR #69, `main@6d682045ee42784709aecc21ff1440de082cc87b`.

Esta fatia altera somente a organização interna da projeção de snapshot: `snapshotPendingPartitions` nomeia as dez partições; `canonicalOrder` é o único limite de conversão para o array posicional CDDL e rejeita rosters ausentes/nulos e cardinalidade excedida. O produtor `snapshotPending` usa campos nomeados, não `parts[n]`. O decoder P1, migrador P2 e publicador de fork permanecem intactos.

A ordenação histórica e o pertencimento estão preservados e conferidos no seam puro de cada roster de origem (IDs estritamente crescentes e únicos): scheduled occurrence / trigger activation são selecionados pelo run e pelo lifecycle, depois filtrados contra `pendingClock`; o helper puro `retainLiveSnapshotSources` mantém a ordem original, os IDs/digests canônicos e a distinção de tags causal-ref (6/7) e input-ref (2/3). `pendingInputs` continua concatenado na mesma ordem: itens do input ledger na ordem global de ID (incluindo exógenos e rounds), seguidos por ocorrências e ativações, sem sort posterior. Source closures e dispatches continuam ordenados por source e por ID, respectivamente; tasks por ID; respostas e completions acompanham a ordem original dos ledgers.

**Não muda:** schemas V1/V2, código de persistência/transação, codec, cálculo de identidades, fontes de leitura SQL, validação de proveniência e observação de mundo. Não atribui autoridade a projeções. A ausência de uma fonte é diferente de partição ausente: partição presente e vazia sempre codifica array `[]`, jamais `null`.

## Gates
- Testes table-driven de todas as dez partições, presença/ausência, limite, ordem, filtragem e aliases; paridade canônica entre a estrutura tipada e o array posicional anterior.
- Golden pré-refactor `TestCSFRV18ForkP1GoldenBytes` e R19/R21/R22/R26, incluindo snapshots, rounds, slots, handoffs e replays.
- Cinco workflows de CI SUCCESS no SHA final, inclusive full Go e `-race -timeout=30m`.
- Revisão adversarial com correções sucessivas; sem merge automático.

**Risco residual e próxima fatia:** negativos de proveniência cruzada para todas as partições com registros autenticados, não apenas o limite produtor; a #57 permanece aberta até fechamento desse aceite.
