# CSFRV1-6 — implementação e revisão de evidência endereçada

**Branch:** `feat/csfrv1-6-epistemic-evidence`
**Draft PR:** [#23](https://github.com/carloshgalves/cote-ai-simulation/pull/23)
**Base:** `main@bf03fa2d7ce7fc65e0b2124295be8b543948b28e`
**Aceite:** [06 — Evidência endereçada e barreira](06-evidencia-e-barreira.md)

## Código implementado

- `PerceptionTaskCompletion` usa identidade e digest canônicos. O fluxo vazio é restrito a eventos internos sem observador; `round.consumed` nunca pode ser completado vazio.
- `PerceptionResolver` positivo registrado para `school.round.lifecycle.v1`: carrega a task publicada, Event, CycleCommit, políticas do Genesis e revisões WorldState pré/pós, e valida a `RoundDeclaration` e seu selo. O ator destinatário é derivado, nunca informado pelo worker.
- `ProjectRoundNotice` publica somente aviso mínimo de round encerrado. Receipts de slot, resultados privados e estado físico ficam no EventStore, fora do contexto.
- `DeliverRoundPerception` grava a Observation, o KnowledgeInput e o completion em **uma transação**, com CAS idempotente e rejeição de identidade/bytes divergentes. As rotas indexadas são particionadas por run, holder e task.
- `Store.Append` detecta candidatos a SlotDispatch antes do roteamento genérico. A validação de autoridade, o limite causal e o append ocorrem na **mesma transação**. Clock, RoundDeclaration, ator, slot e histórico de dispatch/revogação precisam concordar.
- `ActorInbox` só expõe dados do run, ator e horizonte causal do SlotDispatch ao qual foi vinculado. `contextboundary.Build` só aceita recibos tipados e endereçados, sem dependência direta de WorldState, EventStore, Observatory ou RAG.
- `ReplayClock` reconcilia tasks `PENDING`, completions, todos os registros de EvidenceLedger e rotas indexadas. Falha em dados órfãos, ausentes, extras ou adulterados.

## Provas automatizadas

- Cenário de dois eventos: A recebe exatamente uma Observation e um KnowledgeInput; B não recebe evidência destinada a A; segredo interno conclui com zero outputs.
- Permutação de ordem de tasks, número de workers e retries mantém identidade e bytes; workers concorrentes não duplicam registros.
- Fault injection de publicação parcial força rollback; reprocessamento e replay/reopen conservam a mesma projeção.
- Testes adversariais para deleção, adulteração, órfãos, cross-run, falso completion vazio, dispatch malformado/retroativo, leitor de ator errado e dispatch histórico usado para consultar conhecimento futuro.
- Audits negativos de VIP secreto, White Room, canon futuro, telemetria física sem autorização e Observatory. Evidência e comandos em [knowledge-boundary audit](../../../evals/knowledge-boundary/csf-runtime-v1/README.md).

## Verificação de aceite

O resultado **não deve ser declarado homologado apenas porque houve commit**, nem com workflow verde de HEAD anterior. Confirmar no HEAD final, depois da última correção:

1. `CSFRV1-6 Epistemic Evidence Checkpoint`, incluindo audit A/B/segredo e testes de permutação.
2. `CSFRV1-5 Atomic Terminal Commit Checkpoint` (regressão).
3. `CSF canonical conformance`.
4. `git diff --check`, revisão integral do diff e todos os comentários `cote-review-finding:v1` em situação terminal verificada.

A PR permanece **Draft e sem merge** durante a revisão. Nenhuma evidência deste documento amplia o escopo a crenças, claims, memória, RAG, percepção genérica, handlers de WORK ou outros itens posteriores da issue #22.
