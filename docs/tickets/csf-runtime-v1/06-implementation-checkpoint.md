# CSFRV1-6 — checkpoint de implementação (parcial)

**Branch:** `feat/csfrv1-6-epistemic-evidence`
**Draft PR:** [#23](https://github.com/carloshgalves/cote-ai-simulation/pull/23)
**Base:** `main@bf03fa2d7ce7fc65e0b2124295be8b543948b28e` (CSFRV1-5 mesclada)
**Ticket:** [06 — Evidência endereçada e barreira](06-evidencia-e-barreira.md)

## Slice implementado

- `perception.EmptyCompletion` deriva `PerceptionTaskCompletion` canônico, com ID e digest registrados; ele **não** decide a elegibilidade do observador.
- `sqlite.CompleteInternalPerception` consulta evento/outbox/genesis/CycleCommit duráveis e rederiva o task pelo predicado fixado; somente eventos internos, secretos e sem atores aceitam **completion vazio**. A transação SQLite faz CAS idempotente, falhando se os bytes divergem.
- `sqlite.assertNoAddressedOutputs` compara a ligação de `Observation` e `KnowledgeInput` ao task, IDs/digests canônicos, destinatário, referência, instante, claims e evidências. Recibos de outras tasks não constituem bloqueio global; registro desconhecido ou inconsistente falha fechado.
- `sqlite.CheckEmptyPerceptionBarrier` reconcilia refs do journal e registros outbox do run até a revisão exigida, bloqueando ausência/corrupção de task ou completion.
- Casos negativos exercitam ausência de task commitada, completion inválido, input órfão, outro destinatário/task e tentativa de completar task sem commit.

## Limites explícitos — **NÃO CONCLUÍDO**

O presente código **não libera um dispatch real**. `CheckEmptyPerceptionBarrier` é apenas uma verificação diagnóstica, não há ainda a integração atômica com `SlotDispatch`. Também não há resolver de canal que produza a observação positiva de A, `KnowledgeInputSink` de produção, particionamento de evidência com autorização por recipient nem builder allow-listed de contexto. Uma autorização de observador não pode ser fabricada a partir de `Event.confidentiality` ou de uma lista passada pelo worker.

A aceitação de CSFRV1-6 depende ainda de: cenário A/B/segredo, completions com projeção exata não vazia, leitura isolada por agente, queda após entrega parcial, dois workers concorrentes, property tests, reconciliação sob replay/reopen, bloqueio efetivo de dispatch e `knowledge-boundary-audit` completo. A presença de testes de integridade de bytes **não comprova** esses critérios.

## Revisão e validação

Revisão adversarial persistida na PR #23, findings individuais 1–3. O finding de integração completa da barreira/percepção positiva permanece aberto. Os findings de desaparecimento de task e bloqueio cruzado têm correções e testes dedicados.

Workflow dedicado `.github/workflows/csfrv1-6-implementation.yml`: diff whitespace, regressão Go focal e suíte completa. **A verificação do último commit deve ser confirmada no GitHub Actions antes de qualquer aceite; não reutilizar checks de HEAD anterior.**

**Proibição de merge:** manter a PR Draft e o ticket em andamento até cobertura dos critérios acima e revisão adversarial independente final, com todos os findings efetivamente encerrados.
