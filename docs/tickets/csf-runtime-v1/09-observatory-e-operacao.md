# CSFRV1-9 — Observatory read-only e diagnóstico operacional

**Spec:** §7.8 (Observatory) · §8.1 · §10.2–10.3 · §11.5 (8, 10)  
**ADR 0008:** invariante 16  
**Bloqueado por:** CSFRV1-8  
**Bloqueia:** CSFRV1-10

## Resultado observável

O operador consulta o mesmo run em três projeções — privilegiada, POV de A e pública — e obtém um
relatório que encadeia coordenada/fence/decisão/evento/revisão/evidência com autoridade e digests
verificados. Fechar as consultas não altera world hash, knowledge partitions, ledgers, checkpoint,
cursores ou qualquer byte causal.

## Escopo

- `ObservatoryQueries` sem dependência transitiva de command/write ports.
- Views privilegiada, POV e pública derivadas das autoridades; query não cria observation.
- Relatório causal mínimo da spec §10.2, incluindo terminal abort e espera por closure/barrier.
- Diagnóstico de `ATTEMPT_IN_FLIGHT`, `HALTED_ON_ABORT`, `RETRY_AUTHORIZED`, `IDLE`, rejeição de
  ingresso e task pendente sem inferir estado apenas por ausência de event.
- Métricas operacionais da spec §10.3 fora dos digests/hashes causais.
- Verificação de digest/ref antes de apresentar linha como válida.

## Arquivos e módulos prováveis

```text
runtime/csf/observatory/{queries,views,audit_report,metrics}
tests/csf/observatory/{architecture,views,audit_report,metrics}/
tests/csf/scenarios/observatory_no_side_effects.*
```

## Testes determinísticos

- POV de A contém somente evidence própria; pública não amplia `PUBLIC` sem publicação/canal.
- View privilegiada pode mostrar segredo para auditoria sem gravar receipt/cursor.
- Relatório resolve refs/digests e nomeia owner de cada linha; corrupção interrompe no ponto exato.
- Snapshots de todos os hashes/cursors antes/depois de cada query são idênticos.
- Métricas de wall time, host, custo, fila e worker nunca aparecem em preimage causal.
- Teste de arquitetura percorre dependency graph e falha se Observatory alcança write port, sink ou
  store mutável.

## Property tests

Qualquer ordem/repetição/combinação de queries deixa o estado autoritativo idêntico. Permutar a
materialização da projeção não muda conteúdo ordenado do relatório.

## Evals e fronteira de conhecimento

Audit negativo confirma que resultado privilegiado não é origem válida de contexto. Observatory não
é submetido a LLM/RAG eval; suas permissões e projeções são determinísticas.

## Fora do escopo

Command surface, retry/repair, criação de observation por leitura, UI e narrativa.

## Evidência de conclusão

1. Relatório de um commit e um abort fecha todos os refs/digests até suas autoridades.
2. Hash/cursor diff antes/depois de cada query é vazio.
3. Teste arquitetural prova ausência transitiva de escrita.
4. Dashboard/test report distingue espera, rejeição, abort, crash in-flight e run parado.
