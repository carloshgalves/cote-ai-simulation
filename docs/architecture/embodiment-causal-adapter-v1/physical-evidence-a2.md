# CSFRV1-9A/A2 — contrato de evidência física semeada e mapeamento para CSF

Status: contrato A2 e validador de referência, ainda sem adapter Go/Python, reducer ou registro no bundle causal canônico. Esta entrega não autoriza First Real Run nem criação direta de world truth.

Referências: issue #99, issue-mãe #94, A0 #97, A1 #98, ADRs 0006–0011, spec physical-simulation-v1 e causal-simulation-foundation-v1.

Arquivos:
- Schema: [seed-evidence-v1.schema.json](schemas/seed-evidence-v1.schema.json)
- Registry proposto NÃO REGISTRADO: [seed-evidence-registry-proposal-v1.json](seed-evidence-registry-proposal-v1.json)
- Validador: [embodiment_a2_reference.py](../../../tests/architecture/embodiment_a2_reference.py)
- Testes: [test_embodiment_a2_contract.py](../../../tests/architecture/test_embodiment_a2_contract.py)
- Fronteira de transporte A1: [embodiment-causal-adapter-v1.md](../embodiment-causal-adapter-v1.md)

## 1. Escopo implementado e suportado

A2 aceita somente um par previamente selado e coerente de snapshot.yaml e events.jsonl gerados por seed-cohort no PSV1-1/2; recebe os dois como bytes, não como path externo. Prova de isolamento físico, cópia imutável, permissão de source e prazo pré-spawn pertencem à A1/#10. Um par válido tem snapshot_version=1, origem Y1_START, componente prior e dynamics, um run.started inicial, um capacity.sampled por personagem e um cohort.correlation_report terminal. O snapshot de seed inclui capacity_baseline completo e BodyState inicial, cujo valor local t_hours=0.

A saída physical.seed-evidence.v1 é exclusivamente ENGINE_ONLY e PENDING_GO_ADMISSION. Sua lista de subjects é ordenada pelo identificador de personagem, sem usar a ordem do JSONL como prioridade causal. Cada subject tem ponteiros JSON fechados às duas secções da origem e SHA-256 locais de suas secções. O corpo e capacidades numéricos não são transmitidos nesta projeção; permanecem nos bytes de origem que só o engine pode consumir após a A3. A presença do hash não substitui o dado fonte nem demonstra que uma autoridade externa o assinou.

O tipo ainda não possui registro Go, action schema, reducer ou EventDraft; o registro proposto marca runtime_extension_registered=false. Uma mensagem de evidência pendente não é um evento, commit, Genesis, SourceClosure nem WorldRevision. O transporte A1 continua a permitir exclusivamente inspect_run. Qualquer operação física nova requer contrato de transporte versionado explícito, testes e aceite.

## 2. Matriz de registros e decisão

| Registro fonte local | Verificação A2 | Mapeamento |
| --- | --- | --- |
| run.started | primeira posição local seq=0; run_metadata e required_components exatos, seed/roster/versions consistentes | prova de origem física apenas; NÃO Genesis/SourceClosure da CSF |
| capacity.sampled | um por ator, sem duplicatas; 15 dimensões com units exatas; posterior_hash, substream, coorte, sufficiency e valores iguais aos do baseline | subject tipado com refs, hashes locais e source_local_sample_seq para auditoria |
| cohort.correlation_report | exatamente um, terminal; prior_version e cohort_size consistentes | telemetria local apenas; nenhum evento causal |
| run.extended | recusado | PSV1-8 precisa estabelecer continuidade |
| body.advanced | recusado mesmo com snapshot hash válido | o snapshot inicial NÃO contém estado pós-avanço; PSV1-8 obrigatório |
| outro evento, feat posterior, contest, interoception ou observation | recusado/UNKNOWN | PSV1-3–8, A3 ou #10 precisam implementar |

## 3. Matriz campo a campo, tipo e unidade

| Origem física | Regra de validação | Campo de saída/destino |
| --- | --- | --- |
| arquivo snapshot.yaml | SHA-256 dos bytes completos, recebido e recalculado | source.snapshot_raw_sha256 |
| arquivo events.jsonl | SHA-256 dos bytes completos; limite e fechamento de linhas | source.events_raw_sha256 |
| snapshot_hash local | hash sobre JSON legado ordenado, sem o campo snapshot_hash | source.snapshot_document_sha256; não é CBOR |
| normalized eventlog local | JSONL com wall_time fixado em 1970-01-01T00:00:00+00:00 | source.normalised_log_sha256; não é identidade causal |
| snapshot_version | integer estrito igual a 1 | source.snapshot_version |
| world_seed | integer estrito, igual em snapshot e run_metadata/run.started | validação somente, não exportado |
| snapshot.sim_time e baseline.sim_time | Y1_START em runs seed-only | source.local_origin |
| event.seq | integer local 0-based contíguo e único | subjects[].source_local_sample_seq, apenas provenance |
| event.sim_time | Y1_START no seeding | nunca convertido em SimulationInstant global |
| event.wall_time | string operacional | omitido; normalizado somente no log fingerprint |
| run_metadata.prior_version e dynamics_version | strings presentes, restritas a tamanho e iguais às fontes | source.prior_version e source.dynamics_version |
| posterior_hash_by_character | map exatamente N personagens com hash 64 hex minúsculos | subjects[].posterior_hash, cruzado em 3 registros |
| evidence_sufficiency_by_character | exatamente 15 dimensões e zero no PSV1-1 | validado, não exposto |
| capacity_baseline.schema_version | integer estrito 1 | validado, sem confundir com versão da CSF |
| capacity_baseline.sampled_from | posterior/prior/sex/sex_source/substream fechados e compatíveis com capacity.sampled | validado, sem exportar RNG, sexo nem posterior numérico |
| capacity_baseline.dimensions | mapa exatamente 15, cada entrada com value finito e unit armazenada; Pydantic CapacityProfile | subjects[].baseline_sha256, ponteiro para a origem, valores NÃO expostos |
| body_state | conjunto de campos igual ao BodyState atual, revalidado por Pydantic; character_id igual, t_hours=0, illnesses=[] | subjects[].body_state_sha256, ponteiro para a origem, números NÃO expostos |
| cohort.correlation_report | cohort_size do roster, prior_version pinado, terminal | sem saída: somente telemetria |

Unidades de armazenamento (não confundir com unidades dos itens da bateria):
- aerobic_capacity: laps
- anaerobic_power: cm
- sprint_speed: m_per_s
- max_strength: kg
- strength_endurance: reps_30s
- agility: touches_20s
- flexibility: cm
- body_mass: kg
- stature: cm
- reaction_time: ms
- coordination: hit_rate
- injury_resilience: risk_multiplier
- recovery_rate: h
- thermoregulation: degC
- pain_tolerance: percentile, com coorte de referência declarada.

A lista real de unidades vem de DIMENSION_UNITS no Python físico; A2 não cria números canônicos próprios nem perfis nominais de personagens.

## 4. Canonicalização, hashes e tipos

O raw SHA-256 de cada arquivo verifica bytes da fonte selada recebida da A1; por si só não autentica o produtor. O snapshot_document_sha256 usa a função snapshot_hash legada, com json.dumps ordenado, ensure_ascii=False e default=str, excluindo snapshot_hash. O log normalised SHA segue o normalised_digest físico, substituindo só wall_time e preservando a sequência de registros. Os fingerprints baseline_sha256/body_state_sha256 usam json.dumps sort_keys=True, ensure_ascii=False, separators compactos e allow_nan=False, como hashes **locais de referência A2**.

Nenhum deles é canonical_bytes_v1, CausalIdentityPolicy, hash CBOR, unit_digest, event_id, WorldStateHash ou assinatura de produtor. reference_bytes do projector oferece serialização JSON estável para verificar repetibilidade de fixtures e não é byte canônico causal. Uma futura conversão Go é obrigada a usar os codecs/policies registrados da CSF e a derivação de identidade por preimage com domain separation.

## 5. Rota normativa para uma integração Go posterior

1. Produtor físico emite os arquivos; host Go da A1 obtém visão **coerente e imutável** e fixa origem autorizada no Genesis.
2. Validador A2 confere os dois SHA raw contra a solicitação, snapshot_hash interno, formato completo de origem, metadata, 15 units, amostras e BodyState.
3. A2 produz apenas physical.seed-evidence.v1, ENGINE_ONLY/PENDING_GO_ADMISSION, com referências tipadas a dados de origem; não devolve um CommitCandidate autoritativo.
4. A3 determina disclosure exaustivo por campos e recipients, com fail-closed para campo novo e telemetria; A4 valida contratos negativos/canônicos.
5. Em CSFRV1-10, **Go** registra extensão própria, escolhe política para inicialização física sem duplicar Genesis e define produtor/fonte/papel estáveis no manifesto, independente de wall_time, seq local, sim_time local, ordem de mapas, timing ou worker.
6. Somente Go atribui producer_unit_key sob fonte causal pinada, passa pelo InputLedger, AdmissionFence, ActionSchemaRegistry, AffordanceValidator, ConflictResolver, reducer de mundo, CommitCoordinator e CycleCommit. EventOrderKey, LogicalSequence, SimulationInstant, event_id e WorldRevision são derivados/publicados apenas pelo core.
7. Candidato recusado ou histórico avançado sem PSV1-8 permanece sem qualquer mutação no world-state; o caminho read-only pode continuar a consultar evidência física original sem promovê-la.

A2 **não** define ids globais para o Python nem registros CBOR que não existem no Go. O bundle proposto é um catálogo de intenções não canônicas. A escolha do reducer e schema de event draft exige decisão explícita antes de alterar os registries autoritativos.

## 6. Falhas, limites e pendências

Recusar: SHA raw trocado, snapshot_hash adulterado, YAML com duplicate key/alias/tag, JSONL com chaves duplicadas, NaN/Infinity, linha truncada/sem newline, seq duplicado/lacuna, sim_time não originário, evento desconhecido, extra global fields, samples ausentes/duplicados, mismatch em prior/posterior/sex/substream, unidade trocada, dimensão a mais/a menos, BodyState inválido, versão futura, snapshot incompleto e qualquer body.advanced/run.extended. Limites A1: snapshot 16 MiB, eventlog 64 MiB, linha 1 MiB, 100 mil records.

Permanecem bloqueadores: PSV1-3 (estimador com constraints), PSV1-4 (ExertionIntent/performance), PSV1-5 (ObservedPerformance), PSV1-6 (interocepção), PSV1-7 (contest) e PSV1-8 (replay e estado pós-advance); A3 (disclosure), A4 (contract conformance) e CSFRV1-10 (host subprocess real, registro causal, reducer e cenário). A calibração dos parâmetros físicos continua PROVISIONAL; dados do prior não são autorizações de canon.

## 7. Gates da fatia

- [x] CI focal da A2 em PR executa testes com artefatos físicos reais; baseline validada com 102 PASS e diff check, e a última revisão do documento exige novo check no HEAD.
- [x] Revisão adversarial A2-R01–R10 com remediações e re-review sem findings acionáveis identificados neste escopo; comentários individuais na PR #104.
- [x] Links relativos, 6 arquivos, schema/registry de autoridade, unidades, source provenance e testes de permutação/tamper conferidos.
- [ ] Revisão formal independente e aceite/merge humano; autoverificação não é certificação de terceiro.
- [ ] First Real Run continua NO-GO; A3/A4/#10 não foram substituídas por esta referência.
