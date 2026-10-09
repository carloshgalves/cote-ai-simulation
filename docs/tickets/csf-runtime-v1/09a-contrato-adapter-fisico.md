# CSFRV1-9A — Contrato versionado do adapter físico

**Spec:** §12.2 · §14.3
**ADRs:** 0006, 0007, 0008 e 0010
**Bloqueado por:** CSFRV1-0
**Bloqueia:** CSFRV1-10; não bloqueia CSFRV1-1–9
**Natureza:** design executável com schemas e fixtures; sem integração ao runtime.

## Resultado observável

Um contrato versionado e independente do processo converte fixtures representativas de
JSONL/snapshot do `Embodiment` em typed inputs/outputs do CSF com os mesmos bytes em execuções
repetidas. O contrato separa sequência/tempo físico local da ordem/tempo causal global, declara a
allow-list perceptiva e não oferece ao processo Python nenhum port de commit, clock ou revisão.

## Escopo

- Framing e lifecycle do subprocesso local definido pelo ADR 0010: request, response, versão,
  correlation id operacional, limites e erros fail-closed.
- Schemas de request/response e extension bundle dos tipos físicos admitidos; ids/digests globais
  continuam sendo derivados pelo core Go, não fornecidos pelo Python.
- Mapeamento explícito entre snapshot/event log físico, candidate/event draft e reducer global.
- `seq`/`sim_time` físicos preservados somente como provenance local quando necessário; nunca viram
  `LogicalSequence`, `SimulationInstant`, `EventOrderKey` ou prioridade.
- Matriz de campos por papel: autoritativos para engine, elegíveis a observation qualitativa e
  proibidos em evidence/context (`BodyState`, posterior/capacidade, RNG, telemetria, estado alheio).
- Fixtures positivas, negativas e de corrupção suficientes para implementar CSFRV1-10 sem decisão
  arquitetural escondida.

## Arquivos e módulos prováveis

```text
docs/architecture/embodiment-causal-adapter-v1.md
docs/architecture/embodiment-causal-adapter-v1/{schemas,fixtures}/
tests/architecture/embodiment_adapter_contract_test.go
```

## Testes/validação determinística

- Repetir a conversão conceitual da mesma fixture produz o mesmo typed envelope esperado.
- Alterar version/hash, unidade, source snapshot hash ou campo obrigatório falha fechado.
- Fixtures com `seq`/`sim_time` diferentes não conseguem escolher sequence/order/instant globais.
- Schema não possui campo para `CycleCommit`, `WorldRevision`, mutable clock ou store handle.
- Toda saída física autoritativa exige validação/reducer do core antes de virar evento.
- Matriz de disclosure é exaustiva: campo novo sem classificação reprova o contrato.

## Evals e fronteira de conhecimento

Não há execução de LLM/RAG/simulation. A matriz de disclosure e as fixtures negativas são entradas
obrigatórias do `knowledge-boundary-audit` e do embodiment integration eval no CSFRV1-10.

## Fora do escopo

Implementar subprocesso/adapters Go ou Python; importar runs antigos; publicar evento/revisão;
refazer o modelo físico; bloquear os incrementos CSFRV1-1–9.

## Evidência de conclusão

1. Contrato versionado, schemas e fixtures revisáveis existem antes do código de integração.
2. Teste estrutural prova ausência de autoridade causal no protocolo físico.
3. Matriz de disclosure classifica todos os campos e nega telemetria proibida.
4. A spec §14.3 aponta a decisão concluída sem alterar as semânticas dos ADRs 0006–0008.
