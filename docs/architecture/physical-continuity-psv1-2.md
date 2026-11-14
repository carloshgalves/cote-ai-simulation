# PSV1-2/continuidade — verificação local de prefixo físico

**Escopo:** implementação de replay de registros existentes \`body.advanced\` da CLI física (PSV1-1/2), **não** a PSV1-8 completa, A2 causal, CSFRV1-10, nem qualquer gate de release.

Issue #105, relacionada à A2 restante #99 e à PR #104 reclassificada como A2.0 (seed-only).

## Superfície executável

- \`src/embodiment/continuity.py::replay_body_history(snapshot_bytes, events_bytes)\`: API pura de leitura. Retorna \`VerifiedPhysicalPrefix\`, incluindo \`bodies\` por personagem, contagem de \`body.advanced\` e hashes **raw** do par inspecionado. Os estados são engine-only.
- \`verify_body_history(run_dir)\`: conveniência para análise de arquivos locais **já quiescentes**. Não garante corte atômico entre os dois arquivos; a fronteira de origem/autorização e visão selada é obrigação separada da A1.
- \`python -m embodiment verify-history --run runs/demo/\`: mostra somente \`VERIFIED_LOCAL_PREFIX_ONLY\`, contagem e digests dos arquivos físicos. Não imprime atributos corporais, capacity/posterior, estados de terceiros, tokens de Go, revisões ou dados de agentes.

## Algoritmo

1. Verificar tamanho dos arquivos; YAML não admite chave duplicada, alias/anchor/tag nem digest de snapshot incompatível; JSONL exige UTF-8, JSON estrito, LF terminal, \`seq\` local inteiro contíguo, datas UTC e campos fechados.
2. Verificar \`snapshot_version=1\`, \`Y1_START\`, provenance de \`run_metadata\`, versões de prior/dynamics contra modelos instalados, roster de personagens, e reproduzir \`capacity.sampled\`, \`BodyState\` inicial e \`cohort.correlation_report\` sob o mesmo seed e parâmetros. Um número alterado mas autossinado não deve passar.
3. Depois do seed, cada fase começa com \`run.extended\` de metadata igual à abertura; cada \`body.advanced\` dessa fase pertence a um personagem único e não recomeça outro personagem já avançado.
4. Para cada transição, reconstruir \`Environment\` e \`Exposure\` da forma que a CLI PSV1-2 realmente produz; recusar carga arbitrária; executar **somente \`dynamics.advance(previous_verified_body, dt_hours, exposure)\`**. Exigir igualdade exata de \`t_hours_before/after\`, \`logical_sim_time\`, \`dynamics_version\` e \`channel_diff\` completo. Só então substituir o BodyState **em memória da verificação**.
5. Se qualquer evento divergir, falhar antes de devolver um conjunto de estados. Nunca escrever ou alterar o snapshot ou eventlog; ausência de dados/versão desconhecida falha fechado.

\`seq\` e \`sim_time\` são índices/relógios **locais físicos**. Nenhum deles gera \`EventOrderKey\`, \`SimulationInstant\`, \`LogicalSequence\`, \`WorldRevision\`, \`event_id\`, \`producer_unit_key\` ou \`CycleCommit\` da CSF. O Go ainda precisa admitir cada transição sob a política causal registrada.

## Limites epistemológicos e de integridade

**\`VERIFIED_LOCAL_PREFIX_ONLY\` não significa “comando concluído”.** Hoje \`advance-clock\` abre \`run.extended\` e faz \`flush()\` por evento sem \`commit\` terminal, checksum de cadeia, intenção original de \`days\`, ou checkpoint de BodyState. O último registro recebido pode ser apenas um prefixo válido de execução interrompida. Verificamos sua consistência física *sob os parâmetros presentes*, sem poder demonstrar que corresponde ao comando completo originalmente pretendido.

Este verificador **não substitui** a PSV1-8: não salva estado recuperado, não permite um novo \`advance-clock\` sobre ele, não cobre estimator/effort/observation/interoception/contest, não reconstrói crenças físicas, e não valida replay byte-a-byte de todo run. Mesmo os hashes locais não são assinatura de produtor: a autenticação do source e proteção contra mutação concorrente dependem de fonte selada/quiescente externa; um par malicioso **autoconsistente** não é necessariamente a história verdadeira. O formato atual não registra digest criptográfico dos bytes de modelo físico por evento, nem transaction-fence do produtor.

O comando não passa números de \`BodyState\` para contexto dos agentes. Esta proteção por interface é parcial: matriz de disclosure A3 e validação Go continuam necessárias antes da primeira execução real.

## Testes e gates

\`tests/embodiment/test_continuity.py\` exercita \`seed_cohort_command\` e \`advance_clock_command\` reais, 0/4/8/24 h de sono, múltiplos personagens, reconstrução exata de corpos, e negativas para mudança de sequência/tempo/versão/ambiente/carga/diff/seed/snapshot, fragmento pós-crash, e evento inesperado. A CI também executa \`test_cli.py\`, \`test_snapshot.py\` e invariante de single-writer do corpo.

- [x] CI de continuidade e regressões PSV1-2: 150 testes aprovados em [Actions #43535989220](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/43535989220), SHA `c77781a3b367a8765e6669f77a603d437c0bcd7f`; nova rodada obrigatória após edição documental.
- [x] Revisão adversarial realizada pelo executor, P2C-R01–R06 remediados com testes de regressão e re-review sem finding acionável identificado **no escopo**.
- [ ] Revisão formal independente por revisor distinto e aprovação humana de merge.
- [ ] A2 #99 permanece aberta; A3/A4/#10 e PSV1-8 completa ainda separadas.
