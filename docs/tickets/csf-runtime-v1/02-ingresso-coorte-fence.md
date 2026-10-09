# CSFRV1-2 — Ingresso fechado, coorte simultânea e fence verificável

**Spec:** §7.2–7.3 · §9 (fontes/slots/fence) · §11.1 (4, 6)  
**ADR 0008:** invariantes 20–22, 28, 30, 34  
**Bloqueado por:** CSFRV1-1  
**Bloqueia:** CSFRV1-3

## Resultado observável

O harness declara duas fontes exógenas, grava inputs em ordens opostas, fecha explicitamente suas
contribuições, declara dois rounds na mesma coordenada, despacha um slot por ator e grava as respostas
na ordem inversa. As duas execuções derivam a mesma `DecisionCohort`, os mesmos pares slot/resposta e
o mesmo `AdmissionFence` byte a byte antes de qualquer avaliação.

## Escopo

- `InputLedger`: fontes declaradas, `SourceIngressReceipt` gapless, input, fechamento monotônico e
  final, normalização por `open_coordinate` e prefix consistency.
- `RoundDeclaration`, coorte derivada da revisão base e lifecycle de slot: dispatch único,
  revogação durável e resposta única (`ActionProposal` ou `NoProposal`) por CAS.
- `CyclePlan(WORK)`, `AdmissionFenceLog`, closure proof append-stable, ordering de unidades,
  `unit_digest`, `input_digest` e `fence_digest`.
- Coordinator só grava fence depois de todas as closures e slots; host timing não decide corte.
- Rejeições de ingresso/resposta ficam em auditoria operacional fora dos digests causais.

## Arquivos e módulos prováveis

```text
internal/csf/{inputledger,schedule,round,admission,coordinator}/
internal/csf/{inputledger,round,admission}/**/*_test.go
tests/properties/{closure_permutations,fence_stability}_test.go
tests/scenarios/admission_and_rounds_test.go
```

## Testes determinísticos

- `SourceClosure` só avança; primeiro closure por `ingress_seq` que cobre C permanece a prova.
- Input depois de closure não-final recebe coordenada posterior; depois de `final`, input/closure é
  rejeitado atomicamente; final finito cobre coordenadas futuras.
- Fonte ausente ou não fechada mantém espera explícita, sem timeout causal.
- Round e input exógeno na mesma coordenada entram no mesmo fence independentemente da chegada.
- R1/R2 simultâneos formam uma coorte mesmo quando um round completa antes.
- Mismatch de run/cycle/round/slot/ator/revisão/instante não preenche slot.
- Timeout e resposta real disputam o mesmo CAS; exatamente uma vence. Redespacho só após revogação.
- Fonte que também responde slots fecha a fonte sem empurrar a coordenada da resposta.
- Reler `unit_id` com byte divergente falha antes de avaliação.

## Property tests

Permutar append dos inputs antes do mesmo closure, construção das provas, registro local de fontes,
ordem de rounds/slots, conclusão dos responders e momento operacional do fence preserva ids,
unidades ordenadas e todos os digests. `ingress_seq` nunca altera identidade/prioridade.

## Evals e fronteira de conhecimento

Sem eval de modelo: responders são roteirizados. O dispatch recebe somente snapshot/revisão que seu
contrato permite; fence e input ledger não são contexto de ator. Nenhuma evidência é entregue ainda.

## Fora do escopo

Avaliar propostas, resolver conflitos, avançar clock ou commitar eventos. O fence congela o corte;
não é settlement.

## Evidência de conclusão

1. Duas execuções com chegada/conclusão invertidas persistem fences byte a byte idênticos.
2. Query de auditoria mostra closure proof, coorte e exatamente uma resposta por slot.
3. Casos late/final/revoked/mismatch são recusados sem alterar o fence.
4. Nenhum validator/handler foi chamado antes do append condicional do fence.
