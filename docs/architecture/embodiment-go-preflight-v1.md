# CSFRV1-10 / #108 — Go intake de evidência física concluída (preflight, NÃO admissão causal)

**Status:** fatia vertical de validação no Go, sobre a PR #109 de recibos físicos. Ainda não existe extensão física registrada em Genesis/policy bundle nem reducer físico integrado ao CommitCoordinator.

## Componentes implementados

- `internal/csf/adapter/embodimentv1/verify.go`: `InspectUnadmitted(Source)` exige dois hashes de **origem já pinados por autoridade externa** e confere bytes raw de `snapshot.yaml` e `events.jsonl`. Recusa JSON não estrito, fontes truncadas, sequência irregular, relato A2 que não corresponde aos comandos terminados, mudança de ator, contagem de intervalos, relógio físico local, digest de diff e de estado final. Devolve apenas o objeto opaco `Verified`. **O tipo `Verified` não expõe método de admissão ou commit**; não há import de storage/clock/CommitCoordinator nem capacidade de fazer event/commit/revision. Fontes incompletas/sem contrato ou prova de autoridade recebem rejeição explícita no preflight.
- `tests/architecture/emit_go_physical_fixture.py`: CI gera um run verdadeiro no Embodiment com duas operações `advance-clock` concluídas, produz a projeção A2 verificada pela PSV1-2 e passa seus bytes completos ao Go.
- `internal/csf/adapter/embodimentv1/verify_test.go`: verificação positiva de origem real e negativos para hash bruto, source truncada, footer, seq, ator, diff, corpo, campos desconhecidos, relógio, versão, composição de componentes e domínios numéricos.
- O hash de dados opacos usa o módulo autorizado `internal/csf/identity.OpaqueContentHash`, **não** uma nova autoridade causal. O runtime Go não importa relógio do host como causalidade, conforme os testes arquiteturais existentes.

## Limites de segurança e veracidade

Este teste **NÃO** autentica o processo Python: os hashes A1 precisam vir de um corte **coerente, autorizado e imutável** realizado pelo supervisor Go, preferencialmente sobre descritores de arquivos somente leitura e vinculados à fonte Genesis. Checksums calculados pelo próprio chamador sem uma âncora de confiança não provam origem, e um par arbitrário autoconsistente ainda não é world truth.

O Go também **NÃO** executa o cálculo fisiológico Python: ele valida estrutura, identidade, receipts e hashes de corpo no par externo. O replay que verifica os cálculos físicos é feito pela PSV1-2 Python. A admissão verdadeira exigirá uma fronteira de processo confiável com artefato atestado e **validação independente no Go** dos payloads permitidos pelo schema pinado.

O reducer físico não pode ser implementado por uma tabela de SQLite separada. A `internal/csf/persistence/sqlite/work_commit.go` suporta modos de transação registrados até 8 e replay/rollback vinculados a Genesis; qualquer novo modo requer política/codec de mundo, source/ActionSchema, handler, DecisionRecord/EventDraft, ledger/fence, commit e replay completo. Reutilizar `testresource.stock.v1` ou gravar sem CommitCoordinator seria uma falsa admissão.

## Próximo caminho obrigatório para completar #108

1. Integrar uma origem física controlada no supervisor A1, com execução subprocesso local, root pinado em Genesis, shell-free, timeout, pipe limits, quiescence exclusiva, snapshot/log immutable view e verificação idêntica de bytes.
2. Completar A3 #100 (disclosure/recipient) e A4 #101 (conformance/controle negativo). Proibir que números de BodyState, posterior ou RNG sejam usados como KnowledgeInput do agente.
3. Registrar extensão de WorldState físico, typed ActionSchema, reducer e policies canonizados em Go, mediante versão de bundle nova. Não misturar estado físico com estrutura sintética de recursos.
4. Integrar InputLedger→AdmissionFence→resolver→CommitCoordinator→WorkBatch/SQLite com publicação atômica de event/journal/revision e verificação de replay/crash/resume/fork; derivar IDs/ordem/instante **exclusivamente no Go**.
5. Evals de First Real Run e deterministic replay completos, inclusive 41 invariantes e conhecimento por destinatário.

## Status dos gates

- [ ] CI Go/Python de dados físicos reais no HEAD final.
- [ ] Revisão adversarial, remediação de findings locais e re-review (não confundir com auditoria independente).
- [ ] Revisão formal independente e merge humano.
- [ ] Admissão causal verdadeira e First Real Run continuam **NO-GO**, sem atalhos.
