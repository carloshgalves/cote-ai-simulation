# CSFRV1-9A/A2 — transições físicas verificadas → contrato tipado, pendente do Go

**Status:** contrato A2 executável com dados de `body.advanced` reconstituídos pela PSV1-2 (#105, PR #106 já integrada). **Não admite nem publica eventos causais.** Schema e proposta de registro não pertencem ao bundle Go; A3/A4/CSFRV1-10 e gate humano continuam obrigatórios. First Real Run: NO-GO.

**Revisão de escopo:** PR #104 foi inicialmente A2.0 (seed-only). Agora que a PSV1-2 executável foi integrada, o segundo parser/validador de seed da A2.0 e seu schema/registro/testes são retirados **da PR #104**, pois são redundantes. **O seeding físico real em src/embodiment/seeding.py e a validação de seed pela continuidade não foram removidos**: o corpo avançado exige um estado anterior confiável.

## 1. Entregas e arquivos

- [Schema fechado de transições](schemas/verified-transition-evidence-v1.schema.json): `physical.verified-transition-evidence.v1`, com `ENGINE_ONLY`, `PENDING_GO_ADMISSION` e `UNATTESTED_PREFIX`.
- [Registry proposto, não registrado](verified-transition-registry-proposal-v1.json): traz campos/estágios de Go que **não podem ser derivados no Python**.
- [Projetor de referência](../../../tests/architecture/embodiment_a2_transition_reference.py): usa `replay_body_history(snapshot_bytes, events_bytes)` da PSV1-2 como **único verificador físico**. Confere os quatro recibos A1; não reimplementa `seed_character`, posterior, parser YAML ou o integrador do corpo.
- [Testes físicos e negativos](../../../tests/architecture/test_embodiment_a2_transitions.py): geram runs verdadeiros por `seed_cohort_command` e `advance_clock_command`. Corrupção/alteração de recibos, sequências, tempo, sujeito, hash, canais e saída schema-valid precisa falhar.
- [Protocolo A1](../embodiment-causal-adapter-v1.md): `inspect_run` permanece a única operação IPC v1. A2 **não** adiciona operação escondida, canal de commit ou extensão CBOR.

**Fonte:** bytes de snapshot.yaml e events.jsonl provenientes de um corte **coerente, quiescente, imutável e autorizado** pelo host Go. Conferir SHA raw/documental/normalizado não autentica o produtor nem demonstra que o processo não escreveu no intervalo. A1 deve fornecer essa garantia na implementação do host.

## 2. Mapeamento do dado físico e sua autoridade

| Registro/artefato | Verificação PSV1-2 #105 | Campo A2 | Natureza |
| --- | --- | --- | --- |
| snapshot.yaml inicial | SHA raw, hash documental, seed/coorte/prior/dynamics, BodyState inicial | source.snapshot_raw_sha256, source.snapshot_document_sha256, prior_version/dynamics_version | Origem física local; não Genesis CSF |
| events.jsonl | SHA raw, UTF-8 JSONL/LF, seq 0-based contíguo, metadata e fases | source.events_raw_sha256, normalised_log_sha256, log_record_count | Fonte auditável; não event store Go |
| run.started | metadata e componentes pinados | omitido | Abertura local, não SourceClosure |
| capacity.sampled | rederivado de seed físico real e conferido | omitido | Pré-condição verificada, não evento global |
| cohort.correlation_report | recalculado e comparado | omitido | Telemetria proibida em contexto de agente |
| run.extended | identidade de fase, versão, personagem e padrão diário | omitido | Não comprova conclusão do comando |
| body.advanced | `dynamics.advance(previous_verified_body, dt_hours, exposure)` com comparação do diff integral, clock e versões | transitions[] com referências/hashes e horários físicos locais | Transição física verificada, **ainda não admitida** |
| Corpo final de cada ator avançado | resultante da reexecução de **toda** sua sequência | subject_tips[] com hash de BodyState, seq e contagem | Estado local ENGINE_ONLY, sem números no envelope |
| Evento/estrutura futura | falha fechada | omitido | PSV1-3–8 exige novo contrato |

## 3. Tipos, relógios, unidades e fingerprints

| Campo da transição A2 | Unidade/tipo | Restrição |
| --- | --- | --- |
| subject_id | ID de personagem, string fechada | Nunca renomeia nem inventa ator |
| source_local_seq | inteiro do JSONL, 0-based na origem | **Não** aloca prioridade, identidade, LogicalSequence nem EventOrderKey |
| source_local_sim_time | string física Y1_START+NNNNN.NNNh | **Não** define SimulationInstant do Go |
| source_local_after_hours | número, horas desde o seed | Apenas tempo físico local |
| source_local_delta_hours | número, horas por intervalo (0,24] | Não aciona o relógio global |
| source_segment | awake/asleep | Conforme rotina diária PSV1-2 |
| semantic_record_sha256 | hash sobre JSON de registro com wall_time normalizado | Fingerprint local, não digest canônico |
| channel_diff_sha256 | SHA256 do diff integral previamente verificado | Não expõe o estado numérico |
| subject_tips[].verified_body_sha256 | SHA256 local do BodyState final reconstruído | Não é WorldStateHash |
| completion | valor obrigatório UNATTESTED_PREFIX | Prefixo verificável, **não** prova comando terminado |

As unidades dos 15 atributos, os valores de BodyState, Environment e IntervalLoad são validados pelos tipos físicos reais de PSV1-2, não redefinidos por A2. O objeto de referência expõe apenas horários locais, dados de segmento, contagens e hashes; não inclui capacidades, lesões, fadiga, RNG, posterior, correlatos do prior ou outros dados privados de personagens.

**Canonicalização:** campos de referência têm JSON local ordenado (sort_keys=True, ensure_ascii=False, separators compactos, allow_nan=False). O `normalised_log_sha256` segue o algoritmo original `normalised_bytes` do Embodiment: wall_time fixado e registros JSON com ordenação de chaves e newline terminal. Nenhum hash A2 equivale a `canonical_bytes_v1`, digest CBOR, event_id ou autorização de fonte. Duas execuções físicas com seed igual podem ter hashes **raw** diferentes devido ao wall clock, mas digest normalizado e recibos semânticos coincidentes.

## 4. Fluxo futuro de admissão e commit Go (NÃO implementado)

1. O host Go da A1 fixa no Genesis a origem autorizada e obtém uma visão imutável/coerente dos dois arquivos. Confere hashes raw e recibos do `inspect_run`.
2. `replay_body_history` da PSV1-2 valida o prefixo inteiro e recompõe os corpos físicos. A2 somente projeta `physical.verified-transition-evidence.v1`, que declara ENGINE_ONLY/PENDING_GO_ADMISSION/UNATTESTED_PREFIX.
3. **Bloqueio atual:** o JSONL de advance-clock não tem marcador terminal, idempotência/fence, intenção de quantos dias avançar ou contrato transacional; mesmo um prefixo correto não garante que o comando terminou. **Não promover o estado como world truth por omissão.**
4. A3 fecha a matriz de disclosure e A4 verifica o contrato contra codecs/schema/registries de Go. CSFRV1-10 implementa uma operação de transporte **nova e versionada**, admissão com policy/reducer registrado e codec CBOR canônico.
5. Somente Go cria producer_unit_key, input/event/candidate IDs e ordem/tempo global a partir de autoridade de fonte/papel/ciclo — **nunca** de seq, sim_time, wall_time, PID ou chegada de worker — e aplica InputLedger → AdmissionFence → ActionSchemaRegistry → AffordanceValidator → ConflictResolver → physical reducer → CommitCoordinator → CycleCommit → WorldRevision.

As fases acima descrevem uma interface proposta, não interfaces Go registradas; não alegar que o tipo já é EventDraft/CommitCandidate, nem que Go tem reducer físico, nem que observações de agentes podem ler BodyState. Os fingerprints A2 não substituem verificação independente no Go.

## 5. Limites e testes necessários

A2 rejeita: hash raw/documental/normalizado diferente do A1, fonte incompleta, campo desconhecido, valor/tipo/unidade incompatível no verificador PSV1-2, evento local inesperado, seq repetido, diff e horários adulterados, sujeitos trocados, saída schema-valid não vinculada aos mesmos bytes, tabela de tips inconsistentemente reduzida/reordenada, ou envelope acima do limite de 4,194,304 bytes de payload IPC A1.

**PSV1-8 completa NÃO é exigida para verificar o prefixo de transições PSV1-2**, mas é exigida para reconstrução independente de **todos** os módulos físicos (estimador, esforço, observação, interocepção, disputa, recuperação/snapshot) e para as garantias finais de replay físico. A2 não recupera execução interrompida como se estivesse concluída, nem faz snapshot/resume pós-avançamento.

## Gates

- [ ] CI A2 com schema, casos físicos novos, regressões #105 e A1 no SHA exato.
- [ ] Revisão adversarial dos contratos/negativos de transições; remediação e novo review até zero achados acionáveis.
- [ ] Provar remoção dos validadores de seed redundantes sem remoção de seeding real.
- [ ] Revisão independente, aceitação humana e merge.
- [ ] A3/A4/CSFRV1-10 e First Real Run continuam NO-GO.
