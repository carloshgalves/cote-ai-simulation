# CSFRV1-6 — Evidência endereçada e barreira epistemológica

**Spec:** §7.7 (percepção) · §8 · §9 (outbox/barrier) · §11.1 (12) · §11.3  
**ADR 0008:** invariantes 10, 18–19, 36, 41  
**Bloqueado por:** CSFRV1-5  
**Bloqueia:** CSFRV1-7

## Resultado observável

Um commit contém um evento visível apenas a A e outro secreto sem observador. Workers concorrentes
processam as tasks: A recebe exatamente uma `Observation` e um `KnowledgeInput`; B recebe zero; a
task secreta termina com completion vazio. Novo dispatch de A aguarda todos os recibos causalmente
anteriores, enquanto B nunca obtém acesso à partição de A.

## Escopo

- `PerceptionResolver` observer-specific com policy/version/hash, pre/post state e canal autenticado.
- `EvidenceLedger` particionado, append-only e idempotente; `KnowledgeInputSink` sem belief update.
- Ids derivados por task/event/observer/role/ordinal; cronologia pelo evento causal.
- Completion exato após todos os records duráveis, inclusive completion vazio.
- Reconciliação commit/task/observation/input/completion e barrier antes de novo round afetado.
- Allow-list estrutural de contexto: background gated, próprio inbox e próprio estado cognitivo.
- Audits negativos para segredo, terceiro, canon futuro, telemetria física e Observatory.

## Arquivos e módulos prováveis

```text
internal/csf/{perception,evidenceledger,knowledgesink,epistemicoutbox,contextboundary}/
internal/csf/{perception,evidence,barrier}/**/*_test.go
tests/failureinjection/perception_test.go
evals/knowledge-boundary/csf-runtime-v1/
```

## Testes determinísticos

- Evento secreto sem elegível: task + completion vazio, zero observation/input.
- `PUBLIC` sem publicação/canal não é broadcast implícito.
- Output, omission ou observer diferente da projeção recalculada é rejeitado.
- Observation resolve exatamente event/task e conserva instante; input resolve observation e holder.
- `received_at == observed_at`; processamento depois de u mantém tempo t.
- Dois workers/queda após recibo parcial derivam mesmos ids; um CAS de completion; zero duplicata.
- Completion com extra, falta ou record ainda não durável é recusado; após completion, append fecha.
- Barrier não libera round com task anterior pendente ou input listado ausente.
- Context builder de A não possui dependência para event store global ou partição de B.

## Property tests

Permutar observers elegíveis, workers, retries e ordem de append dos recibos preserva outputs
endereçados e completion. Acrescentar ator não elegível não altera outputs dos elegíveis.

## Evals e checagens de conhecimento

Executar `knowledge-boundary-audit`. Casos obrigatórios: papel secreto pré-divulgação, observation de
A inacessível a B, White Room sem gate, evento canônico pós-divergência, evento físico sem
`BodyState`/posterior/RNG/telemetria e query privilegiada ausente do context path. Não há LLM/RAG
eval porque nenhuma inferência/recuperação é adicionada.

## Fora do escopo

Formar crença, confiança, memória ou resumo; transmitir claim; snapshot epistemológico.

## Evidência de conclusão

1. Ledgers mostram um input só para A e completion vazio do segredo.
2. Reprocessamento concorrente mantém ids/bytes e contagens.
3. Barrier bloqueia/libera nos pontos esperados sem alterar coorte declarada.
4. Relatório do audit lista cada payload por destinatário e zero vazamentos.
