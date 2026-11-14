# CSFRV1-9A/A4 — Registro de conformidade e decisão de gate

**Estado:** contrato implementado e sujeito a CI, revisão adversarial e gate humano.
**Rastreabilidade:** #101, #100 A3, #99 A2, #98 A1, #94 9A, #108 Go, CSFRV1-10.

| ID | Prova / cenário | Executado por | Estado esperado |
| --- | --- | --- | --- |
| A4-01 | A1 request/response estrito, operação inspect_run, campos de controle fechados | `test_embodiment_ipc_contract.py` | PASS/FAIL da CI |
| A4-02 | Duas execuções físicas verdadeiras determinísticas (2 atores; 7.3/0.1 h de sono) | `test_embodiment_a4_conformance.py` | PASS/FAIL da CI |
| A4-03 | Mesmo log normalizado, mesmos diffs, bodies e transition hashes entre execuções | `test_embodiment_a4_conformance.py` | PASS/FAIL da CI |
| A4-04 | Campos A2 unknown/new version/unit/type não podem ser promovidos | `test_embodiment_a2_transitions.py`, `test_embodiment_a4_conformance.py` | PASS/FAIL da CI |
| A4-05 | A3 matriz 88 campos + pin exato de schema + futuros campos UNKNOWN rejeitados | `test_embodiment_a3_disclosure.py` | PASS/FAIL da CI |
| A4-06 | Terceiros, corpo bruto, RNG, posterior e observação endereçada fora do sujeito rejeitados | `test_embodiment_a3_disclosure.py`, `test_embodiment_a4_conformance.py` | PASS/FAIL da CI |
| A4-07 | Python não exporta Clock/WorldRevision/CycleCommit/EventDraft/Store; Go contextboundary sem physical allowlist | `test_embodiment_ipc_contract.py`, `test_embodiment_a4_conformance.py`, `go test ./internal/csf/contextboundary` | PASS/FAIL da CI |
| A4-08 | Bundle canônico CSF atual não registra schema/reducer físico; sem declaração de admissão | `test_embodiment_a4_conformance.py` | PASS/FAIL da CI |
| A4-09 | Subprocesso A1 autorizado, selagem imutável e source Genesis-pinned | #108 e CSFRV1-10 | **NOT_RUN / BLOCKED** |
| A4-10 | Go admission → EventDraft → reducer físico → CommitCoordinator → WorldRevision | #108 e CSFRV1-10 | **NOT_RUN / BLOCKED** |
| A4-11 | Evals First Real Run, crash/replay Go com estado físico admitido | CSFRV1-10 | **NOT_RUN / BLOCKED** |
| A4-12 | Revisão formal independente + aceite humano dos contratos A3/A4 | PR #111 e PR A4 | **PENDING** |

**Regra de interpretação:** as linhas A4-01–08, mesmo aprovadas, **não** equivalem à aceitação 9A (falta review/merge humano) e não desbloqueiam a admissão causal (A4-09–11). Nenhum resultado NOT_RUN pode ser convertido em PASS sem execução verificável.

## Próxima decisão

1. A3 #100 precisa ser mesclada após revisão independente.
2. A4 #101 deverá passar em CI cumulativa e revisão adversarial formal; só então a seção 14.3 poderá apontar a decisão 9A **ACCEPTED** após aceite humano.
3. CSFRV1-10/Go #108 seguirá NO-GO até host A1, política/reducer e commit físico atômico realmente executados.
