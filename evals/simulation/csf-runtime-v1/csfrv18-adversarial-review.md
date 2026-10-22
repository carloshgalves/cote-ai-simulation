# CSFRV1-8 — auditoria adversarial de implementação

**Ticket:** [08 — replay, snapshot/resume e fork](../../../docs/tickets/csf-runtime-v1/08-replay-snapshot-fork.md)
**PR:** #25 (Draft; não mesclar enquanto houver blockers)
**Base:** main @ `e3d0b18865e2eb4f2c37c4eb3a98321909d282f0`
**Estado:** em implementação/review, **não concluído**.

## Revisões e correções já incorporadas

| Finding | Mecanismo | Correção/prova |
| --- | --- | --- |
| R1/F1 | Coordinator chamava `ValidatedGenesis` sem declarar o port | Interface `Store` exige resolução verificável; CI Go completo |
| R2/F2 | Pending-state usava digests não registrados e omitira rosters relevantes | Partições exatas e refs registrados; testes de snapshot e tamper |
| R2/F3 | SQL `lifecycle=0` era confundido com unidade ainda pendente após commit | Derivação via journal de assentamento; teste de transmissão entregue |
| R3/F4 | Identidade de snapshot confundida com hash do envelope histórico cross-policy | CSFHREF vincula SHA256 dos bytes exatos e confere digest registrado do corpo |
| R3/F5 | Snapshot antigo era invalidado apenas por haver novos registros | Seal transacional e prova dos cinco prefixos originais; replay do tail |
| R3/F6 | Fork podia depender de um parent ref sem validação de proveniência | Genesis com bootstrap distinto, prefixo histórico arquivado e verificação de parent; corrupção rejeitada |
| R4/F7 | Resume retornava status de controle do snapshot antigo como se fosse o estado atual | Separação entre estado do checkpoint e estado do tip; diretiva fail-closed |
| R4/F8 | Abort/halt e retry sem plano explícito de reentrada | Retoma com estado/fence/retry congelados, drenagem epistemológica e bloqueio de HALTED; testes reais e negativos |
| R5/F9 | Fork de source terminal confundia closure/completion histórica com trabalho pendente | Apenas closures finais podem ficar no prefixo parental; nunca transplantar IDs do parent à filha |

## Evidência automatizada

- Testes `TestCSFRV18*` em `internal/csf/commit`: snapshot canônico, clock/reopen, cinco prefixos, corrupções, crash de publicação, tail divergente, percepção, envio e entrega em trânsito, dois forks, trilha parental e determinismo sob permutação de triggers.
- `TestCSFRV18*` em `internal/csf/persistence/sqlite`: diretrizes de IDLE, IN_FLIGHT, RETRY_AUTHORIZED e HALTED, inclusive fence ausente e barreira perceptiva.
- `internal/csf/fork/history_ref_test.go`: vetor normativo CSFHREF e negativos de magic/version/length/algorithm.
- Workflow `CSFRV1-8 Continuity Checkpoint`: testes focados, ref cross-policy, race regression, SQLite e `go test -count=1 ./...`.
- Regressão CSFRV1-5/6/7 e conformidade canônica no mesmo HEAD, quando a suite terminar.

**Critério de conclusão:** os cinco workflows verdes no HEAD exato **e** findings de contrato abaixo resolvidos, com matriz E2E tri-via e crash/fork.

## Findings ainda abertos (não marcar Ready)

1. **OPEN-01 — CROSS-POLICY:** suporte atual verifica framing/hashes da policy de origem, mas só executa fork V1→V1. Runtime de policy filha distinta, migrador versionado e policy-resolution V2 não estão implementados. Rejeição é fail-closed, não aceite positivo.
2. **OPEN-02 — FORK NON-IDLE:** com triggers ativas, schedule/activation/round/dispatch pendentes ou tasks perceptivas incompletas, o bootstrap rejeita. Falta migração determinística dessas autoridades para novos IDs e replay do tail divergente, sem compartilhamento de estado filho/pai.
3. **OPEN-03 — ACTOR REGISTRY:** os checkpoints epistemológicos são comparados ao roster dos holders de evidence, e não a um actor registry epistemológico autoritativo cobrindo todos os atores cognitivos elegíveis. Essa diferença pode omitir ator sem evidence.
4. **OPEN-04 — EXECUTOR DE RESUME:** `VerifySnapshotV1` reconcilia e retorna plano seguro, mas ainda não há executor de retomada acionado automaticamente que reaplique o fence in-flight, gere o fence de retry autorizado e drene todas as tasks antes de novo dispatch.
5. **OPEN-05 — MATRIZ DIFERENCIAL COMPLETA:** estão presentes provas por componente e por cenários, mas falta o mesmo cenário integrado com conflito de recurso + trigger + comunicação em trânsito, comparando execução contínua, replay e múltiplos resumes para todos os cinco ledgers, hash/world e permutações/crash points.
6. **OPEN-06 — SNAPSHOTS NO MESMO WORLD REVISION:** a identidade normativa V1 usa [run, version, revision]. Ingress/percepção/abort podem mudar autoridades sem incrementar revision. A segunda publicação diferente da mesma revisão falha por identidade (seguro), mas snapshots em vários pontos do mesmo rev não são suportados sem decisão explícita de contrato/versionamento.

Nenhum desses OPEN deve ser mascarado por CI verde. Não é válido afirmar “zero findings”, concluir ticket ou realizar merge antes da resolução com evidência nova.
