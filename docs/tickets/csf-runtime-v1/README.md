# CSF Runtime V1 — plano de tickets

**Spec:** [Causal Simulation Foundation V1](../../spec/causal-simulation-foundation-v1.md)  
**ADR normativo:** [ADR 0008](../../adr/0008-causal-simulation-foundation-v1.md)  
**Codec normativo:** [ADR 0009](../../adr/0009-canonical-causal-codec-and-digests.md) e
[`canonical-codec-v1-bundle`](../../architecture/canonical-codec-v1-bundle/README.md)  
**Stack:** [ADR 0010](../../adr/0010-csf-causal-engine-stack.md) — Go 1.27.2
**Persistência:** [ADR 0011](../../adr/0011-csf-causal-persistence-atomicity.md) — SQLite/modernc,
transação local única
**Estado:** stack e persistência aceitas; CSFRV1-0 alinha a spec para liberar 1–9, enquanto
CSFRV1-9A fecha somente o gate físico do CSFRV1-10.

Este conjunto entrega a fundação causal como dez slices executáveis e dois gates documentais.
CSFRV1-0 não repete pesquisa: incorpora na spec as decisões já aceitas dos ADRs 0010/0011 e explicita
que a interface física continua bloqueando apenas a integração final. CSFRV1-9A fecha esse contrato
com schemas/fixtures antes do CSFRV1-10. Nenhum ticket escolhe stack ou autoridade por omissão.

## Ordem e dependências

```text
CSFRV1-0
    ├─► CSFRV1-1 ─► 2 ─► 3 ─► 4 ─► 5 ─► 6 ─► 7 ─► 8 ─► 9 ─┐
    └─► CSFRV1-9A ──────────────────────────────────────────┼─► CSFRV1-10
```

O grafo é deliberadamente quase linear: cada slice executa o caminho entregue antes e endurece seu
contrato. O design do adapter, CSFRV1-9A, pode ocorrer em paralelo depois do alinhamento inicial e
não bloqueia os tickets 1–9. CSFRV1-10 só começa quando tanto Observatory quanto o contrato físico
estiverem concluídos.

| Ticket | Slice observável | Bloqueado por |
|---|---|---|
| [CSFRV1-0](00-decisoes-bloqueantes.md) | ADRs 0010/0011 refletidos na spec/plano; runtime 1–9 liberado | — |
| [CSFRV1-1](01-contrato-canonico.md) | genesis validado e 620 casos do codec em dois runtimes | 0 |
| [CSFRV1-2](02-ingresso-coorte-fence.md) | fontes fechadas + dois rounds simultâneos produzem um fence estável | 1 |
| [CSFRV1-3](03-clock-agenda-triggers.md) | clock avança por `CycleCommit` atômico e materializa trigger posterior | 2 |
| [CSFRV1-4](04-decisao-recurso-minimo.md) | duas ações disputam a última unidade contra a mesma revisão | 3 |
| [CSFRV1-5](05-commit-atomico-e-retry.md) | settlement/eventos/revisão/outbox commitam juntos ou abortam juntos | 4 |
| [CSFRV1-6](06-evidencia-e-barreira.md) | evento vira evidência endereçada sem vazar segredo | 5 |
| [CSFRV1-7](07-comunicacao-e-claims.md) | claim falso/falsificado é entregue sem virar world truth | 6 |
| [CSFRV1-8](08-replay-snapshot-fork.md) | replay e snapshot+resume igualam execução contínua sob crashes | 7 |
| [CSFRV1-9](09-observatory-e-operacao.md) | auditoria/POV/telemetria consultam sem mutar hashes | 8 |
| [CSFRV1-9A](09a-contrato-adapter-fisico.md) | contrato físico versionado com schemas e fixtures | 0 |
| [CSFRV1-10](10-adapter-fisico-e-cenario.md) | cenário de referência completo, adapter físico e evals finais | 9, 9A |

## Convenções comuns

1. O runtime é Go 1.27.2 em `internal/csf/`, com comandos em `cmd/`, conforme ADR 0010. O adapter de
   persistência é SQLite/modernc conforme ADR 0011; nenhuma implementação pode trocar esses choices
   de dentro de um critério de aceitação.
2. Cada ticket acrescenta um cenário ao mesmo harness sem LLM. Resposta de slot é roteirizada e
   persistida; replay nunca chama modelo.
3. Records, ids, digests, enums, ordering e transições não são redesenhados nos tickets: vêm do
   bundle normativo. Divergência é falha fechada.
4. A evidência de conclusão é persistida (ledgers, snapshot ou relatório verificável), nunca apenas
   stdout, memória do processo ou telemetria.
5. Todo estado autoritativo tem um único owner. Não se cria `WorldService` nem store combinado.
6. Testes de propriedade permutam ingresso, registro local, conclusão de workers e crash points.
   Mesmos fatos persistidos devem produzir os mesmos bytes.
7. Nenhum ticket adiciona cliente de LLM ao core, regra concreta de exame, belief inference ou RAG.
8. Integrações de percepção, comunicação, snapshot epistemológico e adapter físico só fecham após
   `knowledge-boundary-audit`, conforme a spec §11.3.

## Donos das invariantes do ADR 0008 §14

O primeiro ticket listado implementa; tickets posteriores podem reexercitar em integração.

| Invariantes | Ticket dono | Invariantes | Ticket dono |
|---|---|---|---|
| 1–2 autoridade de commit/revisão | CSFRV1-3 | 22 coorte simultânea | CSFRV1-2 |
| 3–4 snapshot comum/sequence não prioriza | CSFRV1-4 | 23 settlement total | CSFRV1-5 |
| 5 handlers sem side effect | CSFRV1-4 | 24 envelope terminal único | CSFRV1-5 |
| 6 trigger durável | CSFRV1-3 | 25 readmissão por coordenada | CSFRV1-5 |
| 7 `INDETERMINATE` aborta | CSFRV1-5 | 26 pais intra-candidato | CSFRV1-5 |
| 8 proibição ≠ impossibilidade | CSFRV1-4 | 27 checkpoint epistemológico | CSFRV1-8 |
| 9 fato ≠ claim ≠ evidence | CSFRV1-7 | 28 fechamento exógeno | CSFRV1-2 |
| 10 evento exige rota causal | CSFRV1-6 | 29 control state/retry | CSFRV1-5 |
| 11 envio ≠ entrega | CSFRV1-7 | 30 resposta/dispatch únicos | CSFRV1-2 |
| 12 reducer não emite | CSFRV1-5 | 31 idempotência/alias | CSFRV1-4 |
| 13 commit multi-evento atômico | CSFRV1-3 | 32 journal ≠ event store | CSFRV1-5 |
| 14 RNG nomeado | CSFRV1-4 | 33 identidade causal | CSFRV1-1 |
| 15 replay sem modelo/wall clock | CSFRV1-8 | 34 digest/topologia do fence | CSFRV1-2 |
| 16 Observatory sem escrita | CSFRV1-9 | 35 ativação atômica | CSFRV1-3 |
| 17 schedule/trigger autoritativos | CSFRV1-3 | 36 task/evidence/completion | CSFRV1-6 |
| 18 barrier antes de round | CSFRV1-6 | 37 candidato/event ordering | CSFRV1-5 |
| 19 contexto allow-list | CSFRV1-6 | 38 próxima coordenada total | CSFRV1-3 |
| 20 fence antes de avaliação | CSFRV1-2 | 39 refs set-like canônicas | CSFRV1-5 |
| 21 ingresso não prioriza | CSFRV1-2 | 40 indeterminate por sujeito | CSFRV1-4 |
| 41 cronologia epistemológica | CSFRV1-6 | | |

## Cobertura dos testes e cenários de stress

- **Codec, identidade e corrupção:** CSFRV1-1 cobre spec §11.1 itens 1–3, 15–17 e os casos do
  bundle; CSFRV1-2 reexercita permutações na topologia do fence.
- **Ingresso, closure, rounds, slots e fence:** CSFRV1-2 possui os cenários desde respostas em ordens
  opostas até dispatch revogado, incluindo fonte que também responde slot.
- **Clock, agenda e triggers:** CSFRV1-3 cobre a primitiva única de commit atômico, saltos de ordinal,
  avanço, menor pendência concorrente, trigger no destino, edge/once/repeat e o `COMMIT` bem-sucedido
  de uma ativação. Settlement terminal de occurrences e abort de cascata ficam no CSFRV1-5.
- **Decisão:** CSFRV1-4 cobre recurso único, resultado provisório `INDETERMINATE`, partição inválida,
  alias e proibição fisicamente possível. O abort normativo só fecha no CSFRV1-5.
- **Commit e terminalidade:** CSFRV1-5 cobre ordem de outputs, refs permutadas, reducer que falha,
  commits vazios, `REJECT`/`DEFER` e sucessores de occurrence, abort por cascade limit, lifecycle e os
  quatro estados de controle.
- **Evidência e isolamento:** CSFRV1-6 cobre segredo sem observador, papel secreto, partição privada,
  White Room/canon futuro, workers concorrentes e latência operacional.
- **Comunicação:** CSFRV1-7 cobre falsidade, remetente apresentado, atraso e pausa antes da entrega.
- **Continuidade:** CSFRV1-8 cobre todos os crash points, rumor como hipótese, comunicação em
  trânsito, forks e paralelismo variável.
- **Observatory:** CSFRV1-9 cobre leitura privilegiada/POV/pública sem mudança de hash.
- **Aceite integrado:** CSFRV1-10 roda o cenário visível da spec §1 e rastreia as 41 invariantes e
  todos os cenários do ADR em um relatório final.

## Evals

Não há `llm-evals` nem `rag-evals` nesta V1: nenhum prompt, modelo ou retrieval é introduzido.
`simulation-evals` e determinism eval são materializados no CSFRV1-8 e fechados no CSFRV1-10. O
`knowledge-boundary-audit` é obrigatório nos tickets 6, 7, 8 e 10. CSFRV1-9A define a allow-list do
adapter; a implementação recebe determinism/isolation eval no CSFRV1-10.

## Fora do escopo de todos os tickets

Escolha de ação por LLM; formação de belief/reflexão; regras concretas de exame; economia PP/CP;
relações; UI; intervenção de usuário; narrativa/cena como unidade causal; merge de timelines; banco
vetorial; framework de agentes; promoção automática de runs JSONL antigos do `Embodiment`.
