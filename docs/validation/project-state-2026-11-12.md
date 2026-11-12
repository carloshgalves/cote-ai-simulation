# COTE AI Simulation — estado auditado e próxima entrega (2026-11-12)

> **Escopo desta apuração:** árvore da `main@78bfd267ba70a6d5f8acb36fdadf24c33407d8ad`, PRs/Issues, contratos, testes versionados e status do GitHub Actions. **Não** é certificação de produção, revisão independente completa de código nem execução local de suites nesta tarefa documental. O HEAD pode avançar; revalidar antes de implementar/mesclar.

## Resumo executivo

**Produto: NO-GO para First Real Run.** Existe uma base causal relevante, não um simulador multiagente plenamente operacional. Separar sempre `IMPLEMENTED` (código integrado), `VALIDATED` (teste executado no SHA indicado), `REVIEWED` (revisor independente identificável), `APPROVED` (aceite humano para o alvo) e `NOT RUN`.

### Entregas integradas

| Frente | Evidência localizada | Status e limite |
| --- | --- | --- |
| CSF causal V1 | `internal/csf/`, `cmd/csf-harness/`; PRs #11, #14, #19–#25 | **CSFRV1-1–8 integradas:** Genesis/identidade, coortes/fence, clock/triggers, recurso, terminal/retry, evidências, comunicação, replay/snapshot/resume/fork; não equivalem a E2E do produto |
| Continuidade/fork V1→V2 | PR #25; PRs #61–#71; `docs/validation/csfrv1-8-baseline-2026-11-11.md` | Baseline #25 mesclada em 11/11; inventário de autoridades (#55), fases de bootstrap (#56) e tipagem/proveniência de dez partições (#57) integrados em 11–12/11; os testes e relatórios não certificam owner remoto |
| Owner cognitivo — S0 | PR #72; `internal/csf/ownercheckpoint/`; `docs/architecture/cognitive-owner-durable-adapter-s0.md` | **Contrato e gate in-process integrados**, com `Stage`, `RestoreAndAcknowledge` e `RunWork`, falhando fechado em cenários exercitados. O owner injetado nos testes é fake; **não** há implementação de serviço remoto, prova de flush externo ou obrigatoriedade em todos os pontos de despacho |
| Conformidade canônica | `docs/architecture/canonical-codec-v1-bundle/`; `.github/workflows/csf-conformance.yml` | Dois runners independentes Go/TypeScript sobre **621 casos V1**, não equivalência cross-language do bundle V2 (issue #59) |
| Corpo físico | `src/embodiment/`; PRs #1/#4; tickets `physical-simulation-v1/` | PSV1-1/2 integrados (semente/coorte e evolução corporal); PSV1-3–8 e adapter Go↔Python ainda pendentes |
| Corpus/retrieval | `data/canon/consolidated/`, `tests/canon_rag/`; PRs #7, #27–#50 | Canon RAG V0 e packs de referência até Y1 V11 integrados. São evidências/casos, **não** memórias vividas nem regras ou agentes executáveis por simples presença de JSON |
| Exames | `docs/architecture/exams.md`, PR #17 e dados consolidados | Zodiac/VIP: spec e vetores/modelos; Class Poll (#48) e Event Selection (#49): issues abertas; Examination Engine e economia operacional completa pendentes |
| Agentes e operação | `.agents/skills/`, `docs/agents/`, issue #45/#36 | Skills/orquestração disponíveis; **cognição persistente, planejamento autônomo, memória do run e observatório de produto não são implementações E2E** |

### Evidência de execução observada

- PR #72, HEAD `79198953c2097f13dfabd89f239501d0de055649`: cinco workflows concluídos com `success` (CSFRV1-5, -6, -7, -8 e conformidade). Isso demonstra execução desses workflows no **HEAD da PR**, não durabilidade externa real.
- `main@78bfd267ba70a6d5f8acb36fdadf24c33407d8ad`: workflow **CSF canonical conformance** concluído com `success`, [run 43226400594](https://github.com/carloshgalves/cote-ai-simulation/actions/runs/43226400594). Não foi realizado nesta apuração um `product-validation-on-demand` completo nesse SHA.
- O workflow `csf-conformance.yml` não possui filtro de alterações para `canonical-codec-v2-bundle/**` em PR; o contrato de 621 casos é V1. Não extrapolar esta evidência para V2 (#59).
- Fechamentos administrativos das issues #38, #39 e #52 **não** constituem validação por fonte primária. Os gates por registro, `verified:false`, `engine_memory_admission:false` e equivalentes permanecem pertinentes.

## Lacunas — ordem de risco e dependências

1. **P0 release — issue #58:** demonstrar checkpoint externo durável, restore/ACK idempotente e dispatch cognitivo obrigatoriamente bloqueado até ACK. A PR #72 entrega apenas S0 (interface/gate). Faltam escolha e implementação da fronteira owner, integração dos pontos de entrada e drill de crash/recuperação com persistência real.
2. **P1 — issue #59:** incluir mutações somente do bundle V2 nos filtros de CI, definir validação independente V2 sem confundir com 621 V1; medir/ajustar timeout e conformidade no SHA exato.
3. **CSFRV1-9 → 9A → 10:** Observatory read-only, contrato e integração física causal com o Go; seguir os tickets `docs/tickets/csf-runtime-v1/`.
4. **Simulação/atores e domínio:** PSV1-3–8, autonomia e memória (#45), objetivos contingentes (#36), economia PP/CP e execução dos exames (#16, #48, #49) com autorização de fonte.
5. **P2 — issue #60:** benchmarks antes de qualquer otimização de replay, snapshot ou forks. Sem desempenho medido, não declarar defeito.
6. **First Real Run:** atender G0–G8 da [matriz de prontidão](product-readiness-2026-11-10.md), inclusive cenário multiagente, regras, observabilidade, replay e ausência de vazamento.

## Próxima tarefa recomendada — #58, slice S1

**Objetivo:** obter um primeiro **fluxo concreto e recuperável de checkpoint cognitivo durável**, conectado à porta `DurableOwner` do S0, **sem** fingir que um fake de memória confirma infraestrutura remota. Esta entrega não deve escolher fornecedor/cloud unilateralmente nem ampliar o codec V1/V2.

**Contrato mínimo para autorizar implementação:**
1. Mapear todos os caminhos de entrada no host para `WORK/LLM`; identificar quais existem hoje e quais ainda são somente interfaces. Não atribuir ao CSF responsabilidade por manter memória privada.
2. Definir, em ADR curto, owner da memória, armazenamento durável concreto (local com semântica demonstrável para teste vs remoto, com limitações explícitas), persistência/assinatura após flush, isolamento ator/run/tenant, política de chaves/backup e falha de rede. Escolha de provedor externo e operação real exigem decisão do operador.
3. Implementar **uma fatia vertical pequena** com `PersistAndSign` → escrow SQLite → snapshot → restart → `Restore`/ACK → **apenas então** `RunWork`. Não introduzir chamadas reais a LLM nesta primeira prova, usar callback contado. Integrar aos pontos de despacho presentes no escopo; registrar expressamente eventuais entradas não cobertas.
4. Exercitar falhas antes/depois do persist/assinatura/escrow/snapshot/ACK e após restart; negar bytes trocados, locator incorreto, receipt/actor/run/fork diferente, snapshot fora do tip e bypass do gate. Retry deve ser idempotente, sem nova geração cognitiva.
5. Testar `go test` focal e suite completa pertinente, `-race` onde aplicável, regressões de replay/fork/cinco ledgers, `gofmt` e `git diff --check`; coletar SHA/exit codes; revisão adversarial independente; manter PR Draft até autorização humana.
6. **Critério de parada:** prova reprodutível do fluxo no ambiente declarado, nenhum caminho de WORK coberto sem gate, nenhum finding acionável do escopo; **sem** reivindicar durabilidade remota ou release se só houver owner local de teste.

Se uma definição de owner/provider impedir S1, parar na proposta/ADR com decisão explícita pendente. **Alternativa pronta para execução, sem dependência do owner:** issue #59 (CI V2), em PR independente. Depois: CSFRV1-9 (observabilidade), sem criar uma UI antes da projeção read-only.

## Skills e ferramentas (seleção contextual)

- **Projeto**: `.agents/skills/wayfinder` + `research` e `to-spec` para ingresso/ADR; `to-tickets`/ `domain-modeling` para particionar S1; `implement` + `tdd` para fatia codificada; `knowledge-boundary-audit` e `simulation-evals` para regressões de isolamento/causalidade; `code-review` e `address-review-findings` em **fases separadas**, conforme `AGENTS.md`.
- **Codex Engineering Guardrails**: `code-work` na implementação autorizada, `code-verification` na avaliação independente; usar o segundo somente para constatar evidência, não corrigir.
- **Codex Dev Workflows**: `feature-development` ou `new-project`/planejamento para S1, `code-review`/ `pre-release-review` e `session-handoff` para passagem de fase; não substituir controles do repositório.
- **GitHub conectado**: Issues para escopo/aceite, Draft PR para diff e findings, checks/Actions para SHA exato. Nenhum plugin de hospedagem, base vetorial ou LLM adicional é requisito para esta fatia; adotar só quando houver contrato aprovado.

**Governança:** `AGENTS.md`, `CONTEXT.md`, ADRs aceitos e specs/tickets são a hierarquia do repositório. A proposta documental **não autoriza** invocar fases automaticamente, editar material privado ou fazer merge. O projeto é público; preservar fora do Git os PDFs integrais protegidos e dados de memória privada.
