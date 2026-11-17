# CSFRV1-10/E1 — oráculo canônico de igualdade byte a byte

**Issue técnica:** [#140](https://github.com/carloshgalves/cote-ai-simulation/issues/140), filha textual de [#119](https://github.com/carloshgalves/cote-ai-simulation/issues/119), épica [#114](https://github.com/carloshgalves/cote-ai-simulation/issues/114). Baseline local D3: PR #139 mesclada pelo operador em `main@9818bf1c8a66edf2d6e5ad3b8474431ea0d0c09d`.

**Escopo:** comparador interno **somente para testes Go** do mesmo C4+D2+D3 integrado (9 commits Go e duas épocas reais do produtor físico Python A1). Não cria um serviço de publicação ou um write port; não autoriza disponibilizar bytes canônicos para ator, LLM, RAG, Observatory público ou telemetria.

## Entradas e comparação

1. Invocar o supervisor Python A1 duas vezes, obter dois `ApprovedCuts` realmente assinados e verificáveis. Congelar estes **mesmos dois cuts** apenas no campo `frozenSupervisedCuts` de teste package-private. O runner normal D2/D3 chama Python de fato a cada época.
2. Executar quatro D3 integrais independentes, mudando somente as opções: normal, `ReverseSources`, `ReverseResponses`, ambas invertidas. Cada run cria um **único** banco causal e os dois arquivos privados `owner-b.db`/`owner-b.db.actor-a`, com os mesmos controles de admissão, replay, checkpoint e sigilo de D3.
3. Após a validação D3, abrir cada SQLite com `mode=ro` e uma transação `ReadOnly`. Capturar `sqlite_master` (schema/tabelas/índices), depois todas as tabelas autoritativas existentes; a fonte é o estado real persistido.
4. Preservar **nome e ordem das colunas**; normalizar cada valor SQLite com tag de classe (`blob`, `text`, `integer`, `float`, `null`), serializar a linha em CBOR canônico, e ordenar linhas por seus **bytes canônicos**. O formato é um multiconjunto de linhas, não `rowid`/ordem física de páginas SQLite.
5. Comparar **todos os bytes** das linhas e colunas. Apenas depois de verificar igualdade, produzir manifesto com `database`, `table`, `rows` e digest SHA-256 length-delimited. Nenhuma linha, checkpoint privado ou `PhysicalCompletedState` é exportada. O manifesto é evidência operacional de teste, **não** um produto para agentes.
6. Negativos: banco/tabela ausente, linha adulterada, duas capturas vazias, leitura com tentativa de escrita, CBOR corrompido ou estrutura de clock semanticamente inválida. `clock.Decode` é necessário: CBOR válido como `0x80` não é ClockState válido.
7. Retestar D2/D3, `perception`, `contextboundary`, SQLite e `go vet`, mais `-race` no oráculo integrado.

**Conjunto mínimo exigido:** `genesis_records`, os cinco ledgers, `clock_states`, `world_revisions`, `source_ingress_receipts`, `input_source_cursors`, `scheduled_occurrences`, `trigger_activations`, `committed_cycles`, `snapshots`, `snapshot_prefix_members`, `cognitive_owner_receipts`, `evidence_routes` e, nos owners A/B, `checkpoints` e `restore_acks`. E1 aceita exatamente 10 WorldRevisions (Genesis + 9 commits), sem simular commit extra.

## RED, verde e evidência

- **RED histórico:** commit `03c1dfb499250643a5aaf21353f3fcf5143a49b4` contém testes E1 antes de existirem os tipos e funções do oráculo. O workflow reproduz esse commit e exige falha por símbolo ausente; esta falha *esperada* não é contada como bug de ambiente ou PASSED de runtime.
- **GREEN:** [workflow E1](https://github.com/carloshgalves/cote-ai-simulation/actions/workflows/csfrv1-10e1-byte-oracle.yml) executa Python A1 real, quatro cenários, negativos, `-race`, regressões e `go vet` no HEAD do PR.
- **Artefato:** `csfrv1-10e1-safe-hash-manifest`, JSON exclusivo/0600 gerado em `/tmp/e1-evidence/manifest.json` e preservado como artifact GitHub Actions por sete dias. Contém **somente contagens e digests**, não o banco ou dados dos atores. Não é substituto de atestação de sigilo independente.

## Reproduzir localmente (Linux)

```bash
python -m pip install --disable-pip-version-check --no-input -e '.[dev]' \
  'numpy==2.5.4' 'pydantic==2.14.3' 'PyYAML==6.0.3' \
  'pytest==9.2.0' 'jsonschema==4.26.0' 'hypothesis==6.170.15'
python -m tests.architecture.emit_go_physical_fixture --out /tmp/physical-go-fixture-e1
find src/embodiment -type d -name __pycache__ -prune -exec rm -rf {} +
mkdir -p /tmp/e1-evidence
PHYSICAL_A1_PRODUCER_TEST=1 CSF_E1_EVIDENCE_PATH=/tmp/e1-evidence/manifest.json \
  go test -count=1 -v ./internal/csf/commit -run '^TestReferenceE1'
PHYSICAL_A1_PRODUCER_TEST=1 go test -race -count=1 ./internal/csf/commit \
  -run '^TestReferenceE1SameSignedSourcesWholeCausalAndOwnerByteTranscript$'
go test -count=1 ./internal/csf/perception ./internal/csf/contextboundary ./internal/csf/persistence/sqlite
go vet ./internal/csf/commit ./internal/csf/persistence/sqlite
```

O path de manifesto usa `O_EXCL`: nova execução requer um nome **novo**, não sobrescreve um documento anterior. `PHYSICAL_A1_PRODUCER_TEST=1` é obrigatório para que SKIP do produtor não seja registrado como sucesso.

## Limites do que E1 demonstra

- **Não** comparar arquivos `*.db` byte a byte: páginas/WAL são detalhes SQLite de armazenamento, não o estado causal normativo. Comparar a transcrição canônica de *todas* as linhas, índices/schemas e classes de storage sem omitir tabelas.
- **Não** comparar cortes Python A1 recém-executados em horas diferentes como se fossem inputs idênticos: `wall_time` da fonte é autenticado e pode mudar. O experimento controlado compartilha os mesmos inputs assinados.
- Uma captura test-only de SQLite após D3 não comprova restart processual após crash, source tamper antes do ingresso, snapshot/resume ou fork em duas branches. São os itens **E2 #141, E3 #142, E4 #143, E5 #144**, ainda abertos.
- **Não** atribuir a uma revisão interna a condição de revisão humana independente nem declarar que gates #93/#94/#113 estão homologados. Merge continua exclusivamente humano.

### Achados da implementação

A primeira rodada de testes com Python A1 real confirmou igualdade de 4 runs/46 estruturas mas recusou um teste adversarial: `StrictDecodePayload` aceitava `clock_states.canonical=x'80'` como CBOR sintático válido. Corrigido pela passagem adicional no decoder normativo `clock.Decode`; o negativo precisa ser reexecutado e registrado no HEAD final. A revisão também passou a rejeitar igualdade vacuamente verdadeira entre duas capturas vazias. A PR registra demais findings e o SHA de conclusão.
