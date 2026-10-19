# CSFRV1-4 — Decisão determinística sobre um recurso mínimo

**Spec:** §7.5 · §9 (`INDETERMINATE`, idempotência, provenance) · §11.1 (7, 13)  
**ADR 0008:** invariantes 3–5, 8, 14, 31, 40; cobertura preparatória da 7
**Bloqueado por:** CSFRV1-3  
**Bloqueia:** CSFRV1-5

## Resultado observável

Duas ações simultâneas tentam consumir a última unidade de um recurso contra a mesma
`base_revision`. Handlers puros criam candidatos, validators produzem facets, o detector cria um
`ConflictSet` e a policy pinada escolhe sempre o mesmo vencedor sem saldo negativo. Inverter a ordem
dos workers não muda assessments, candidatos, conflito, RNG ou disposições provisórias.

## Escopo

- Registry de action schemas e um tipo sintético `resource.consume` em extension bundle de teste.
- Handlers sem side effects, `ActionProposal`/`NoProposal`, `AffordanceAssessment` com facets de
  possibilidade, acesso e proibição separadas.
- `CommitCandidate` com identidade por unidades-fonte/papel/ordinal e partição exata.
- Read/write/resource/invariant claims, detector conservador e resolvedor mínimo de recurso.
- Substream RNG nomeado/versionado e `RngDraw` persistido como material provisório.
- Idempotency namespace/digest, alias no mesmo ciclo e alias de canônica histórica.
- `INDETERMINATE`/`IndeterminateRecord` por sujeito; neste ticket o resultado é somente material
  provisório fail-closed. A invariante 7 não é considerada implementada até o `CycleAbortRecord`
  autoritativo do CSFRV1-5.

## Arquivos e módulos prováveis

```text
internal/csf/{action,handler,affordance,candidate,conflict,rng,idempotency}/
internal/csf/extensions/testresourcev1/
internal/csf/{action,affordance,candidate,conflict,idempotency}/**/*_test.go
tests/properties/{candidate_partition,rng_isolation,decision_permutations}_test.go
tests/scenarios/last_resource_test.go
```

## Testes determinísticos

- Todas as propostas leem a mesma revisão; `LogicalSequence`, arrival e completion não são input da
  policy.
- Duas ações pela última unidade geram um conflito e saldo pós-cópia nunca negativo.
- Regra proíbe agressão sem tornar impossível a ação; uma violação tipada pode acompanhar commit.
- Faceta sem regra/dado/resolvedor gera `INDETERMINATE`, nunca permissão.
- Dois sujeitos com mesma facet/validator/reason mantêm dois records atribuíveis.
- Gap, overlap ou unidade externa na partição gera `PROVENANCE_FAILURE` antes da resolução.
- Mesmo namespace idempotente com bytes divergentes gera conflito; aliases legítimos mantêm
  identidades de entrega e canonical unit por ordem normativa, não append.

## Property tests

Permutar propostas, plugin registration, validators e conclusão de workers preserva candidate ids,
facets, conflicts, named draws e dispositions. Acrescentar candidato/validator irrelevante não muda
substreams existentes.

## Evals e fronteira de conhecimento

Simulation eval parcial compara vencedor e material provisório em seeds/paralelismos distintos.
Facets, saldo autoritativo, conflitos e RNG nunca entram em contexto de ator.

## Fora do escopo

Publicar evento, settlement terminal ou revisão. As disposições deste ticket são provisórias e podem
ser descartadas integralmente pelo CSFRV1-5.

## Evidência de conclusão

1. Cenário persistido contém dois candidates, um conflict set, draw/policy e um vencedor estável.
2. Execuções com workers invertidos produzem material provisório byte a byte idêntico.
3. Casos indeterminate e partição inválida não publicam qualquer fato do mundo; o relatório os
   marca como cobertura preparatória, não como invariante 7 concluída.
4. Teste de arquitetura prova que handlers/validators/resolvers não recebem stores mutáveis.
